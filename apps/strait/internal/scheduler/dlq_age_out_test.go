package scheduler

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/queue"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

type fakeDLQAgeOutStore struct {
	masked         int64
	err            error
	panicRun       bool
	oldestAge      float64
	oldestAgeErr   error
	calls          int
	oldestAgeCalls int
}

func (f *fakeDLQAgeOutStore) MaskOldDLQRows(_ context.Context, _ time.Duration, _ int) (int64, error) {
	f.calls++
	if f.panicRun {
		panic("dlq store panic")
	}
	return f.masked, f.err
}

func (f *fakeDLQAgeOutStore) OldestUnmaskedDLQAge(_ context.Context) (float64, error) {
	f.oldestAgeCalls++
	return f.oldestAge, f.oldestAgeErr
}

func TestDLQAgeOut_Defaults(t *testing.T) {
	a := NewDLQAgeOut(&fakeDLQAgeOutStore{}, DLQAgeOutConfig{})
	assert.Equal(t, 24*
		time.Hour,
		a.interval,
	)
	assert.Equal(t, 30*
		24*time.
		Hour,
		a.retention)
	assert.Equal(t, 1000,
		a.batchLimit,
	)
}

func TestDLQAgeOut_RunOnceMasksRows(t *testing.T) {
	s := &fakeDLQAgeOutStore{masked: 7, oldestAge: 42}
	a := NewDLQAgeOut(s, DLQAgeOutConfig{Retention: time.Hour, BatchLimit: 100})
	require.NoError(t,
		a.runOnce(
			context.
				Background()))
	assert.Equal(t, 1,
		s.calls)
	assert.EqualValues(t, 7,
		a.TotalMasked())
	assert.Equal(t, 1,
		s.oldestAgeCalls)
}

func TestDLQAgeOut_AccumulatesAcrossCycles(t *testing.T) {
	s := &fakeDLQAgeOutStore{masked: 3}
	a := NewDLQAgeOut(s, DLQAgeOutConfig{})
	_ = a.runOnce(context.Background())
	_ = a.runOnce(context.Background())
	assert.EqualValues(t, 6,
		a.TotalMasked())
}

func TestDLQAgeOut_StoreErrorPropagates(t *testing.T) {
	s := &fakeDLQAgeOutStore{err: errors.New("locked")}
	a := NewDLQAgeOut(s, DLQAgeOutConfig{})
	assert.Error(t, a.runOnce(context.
		Background()))
}

func TestDLQAgeOut_OldestAgeSampleErrorDoesNotFailRun(t *testing.T) {
	s := &fakeDLQAgeOutStore{oldestAgeErr: errors.New("age sample failed")}
	a := NewDLQAgeOut(s, DLQAgeOutConfig{})

	require.NoError(t, a.runOnce(context.Background()))
	assert.Equal(t, 1, s.oldestAgeCalls)
}

func TestDLQAgeOut_RecordsOldestUnmaskedAgeMetric(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	originalProvider := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)
	queue.ResetMetricsForTest()
	t.Cleanup(func() {
		queue.ResetMetricsForTest()
		otel.SetMeterProvider(originalProvider)
		require.NoError(t, provider.Shutdown(context.Background()))
	})

	s := &fakeDLQAgeOutStore{oldestAge: 123.5}
	a := NewDLQAgeOut(s, DLQAgeOutConfig{})

	require.NoError(t, a.runOnce(context.Background()))
	assert.Equal(t, 1, s.oldestAgeCalls)
	assert.InDelta(t, 123.5, dlqOldestAgeMetricValue(t, reader), 0)
}

func TestDLQAgeOut_PanicReturnsError(t *testing.T) {
	s := &fakeDLQAgeOutStore{panicRun: true}
	a := NewDLQAgeOut(s, DLQAgeOutConfig{})
	require.Error(t, a.
		runOnce(context.
			Background()))
	require.EqualValues(t, 1,
		a.Iterations())
}

func TestDLQAgeOut_LockNotAcquired(t *testing.T) {
	s := &fakeDLQAgeOutStore{masked: 5}
	locker := &fakeLocker{acquireOK: false}
	a := NewDLQAgeOut(s, DLQAgeOutConfig{}).WithAdvisoryLocker(locker)
	_ = a.runOnce(context.Background())
	assert.Equal(t, 0,
		s.calls)
}

func TestDLQAgeOut_LockAcquiredAndReleased(t *testing.T) {
	s := &fakeDLQAgeOutStore{masked: 5}
	locker := &fakeLocker{acquireOK: true}
	a := NewDLQAgeOut(s, DLQAgeOutConfig{}).WithAdvisoryLocker(locker)
	_ = a.runOnce(context.Background())
	assert.False(t, !locker.
		acquired ||
		!locker.released,
	)
}

// scanningDLQStore implements DLQAgeOutStore and DLQPartitionScanner to
// exercise the parallel-scan path. It tracks peak concurrent scans and
// verifies that ScanDLQPartitionCandidates runs before MaskOldDLQRows.
type scanningDLQStore struct {
	partitions []string
	masked     int64
	scanDelay  time.Duration

	mu                sync.Mutex
	scannedPartitions []string
	maskCalls         int
	scansFinishedAt   time.Time
	maskCalledAt      time.Time
	active            atomic.Int32
	peak              atomic.Int32
}

func (s *scanningDLQStore) MaskOldDLQRows(_ context.Context, _ time.Duration, _ int) (int64, error) {
	s.mu.Lock()
	s.maskCalls++
	s.maskCalledAt = time.Now()
	s.mu.Unlock()
	return s.masked, nil
}

func (s *scanningDLQStore) ListDLQPartitions(_ context.Context) ([]string, error) {
	return s.partitions, nil
}

func (s *scanningDLQStore) ScanDLQPartitionCandidates(_ context.Context, partition string, _ time.Duration, _ int) (int64, error) {
	n := s.active.Add(1)
	defer s.active.Add(-1)
	for {
		peak := s.peak.Load()
		if n <= peak || s.peak.CompareAndSwap(peak, n) {
			break
		}
	}
	time.Sleep(s.scanDelay)
	s.mu.Lock()
	s.scannedPartitions = append(s.scannedPartitions, partition)
	s.scansFinishedAt = time.Now()
	s.mu.Unlock()
	return 1, nil
}

func TestDLQAgeOut_ParallelPartitionScan(t *testing.T) {
	parts := []string{"p1", "p2", "p3", "p4", "p5", "p6", "p7", "p8"}
	s := &scanningDLQStore{partitions: parts, masked: 3, scanDelay: 10 * time.Millisecond}
	a := NewDLQAgeOut(s, DLQAgeOutConfig{})
	start := time.Now()
	require.NoError(t,
		a.runOnce(
			context.
				Background()))

	elapsed := time.Since(start)
	assert.LessOrEqual(
		t, elapsed,
		70*time.
			Millisecond,
	)
	assert.GreaterOrEqual(t, s.peak.
		Load(), int32(2))
	assert.LessOrEqual(
		t, s.peak.
			Load(),
		int32(dlqAgeOutScanPoolSize),
	)

	// Serial = 80ms, parallel (pool 4) ~= 20ms + slack.

	s.mu.Lock()
	defer s.mu.Unlock()
	assert.Len(t, s.scannedPartitions,

		len(parts))
	assert.Equal(t, 1,
		s.maskCalls,
	)
	assert.False(t, !s.
		maskCalledAt.
		After(s.scansFinishedAt) && !s.
		maskCalledAt.
		Equal(s.scansFinishedAt))
	assert.EqualValues(t, 3,
		a.TotalMasked())

	// Scans should finish before the serial mask.
}

func TestDLQAgeOut_RunExitsOnCancel(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	s := &fakeDLQAgeOutStore{}
	a := NewDLQAgeOut(s, DLQAgeOutConfig{Interval: 5 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	concWG.Go(func() {
		a.Run(ctx)
		close(done)
	})
	time.Sleep(30 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		require.Fail(t, "did not exit")
	}
}

func dlqOldestAgeMetricValue(t *testing.T, reader *sdkmetric.ManualReader) float64 {
	t.Helper()

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))
	for _, scope := range rm.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != "strait_queue_dlq_oldest_unmasked_age_seconds" {
				continue
			}
			gauge, ok := metric.Data.(metricdata.Gauge[float64])
			require.True(t, ok)
			require.NotEmpty(t, gauge.DataPoints)
			return gauge.DataPoints[len(gauge.DataPoints)-1].Value
		}
	}
	require.FailNow(t, "dlq oldest unmasked age metric not collected")
	return 0
}
