package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/config"
	"strait/internal/telemetry"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// fakePoolStatter lets tests drive the sampler with deterministic counters.
type fakePoolStatter struct {
	mu              sync.Mutex
	acquired        int32
	maxConns        int32
	count           int64
	waitTotal       time.Duration
	emptyCountReads chan struct{}
}

func (f *fakePoolStatter) AcquiredConns() int32 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.acquired
}

func (f *fakePoolStatter) MaxConns() int32 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.maxConns
}

func (f *fakePoolStatter) EmptyAcquireCount() int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.emptyCountReads != nil {
		select {
		case f.emptyCountReads <- struct{}{}:
		default:
		}
	}
	return f.count
}

func (f *fakePoolStatter) EmptyAcquireWaitTime() time.Duration {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.waitTotal
}

func (f *fakePoolStatter) set(count int64, waitTotal time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.count, f.waitTotal = count, waitTotal
}

func (f *fakePoolStatter) setOccupancy(acquired, max int32) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.acquired, f.maxConns = acquired, max
}

type snapshotPoolStatter struct {
	stats         PoolBackpressureStats
	snapshotReads atomic.Int32
	fallbackReads atomic.Int32
}

func (s *snapshotPoolStatter) BackpressureStats() PoolBackpressureStats {
	s.snapshotReads.Add(1)
	return s.stats
}

func (s *snapshotPoolStatter) AcquiredConns() int32 {
	s.fallbackReads.Add(1)
	return s.stats.AcquiredConns
}

func (s *snapshotPoolStatter) MaxConns() int32 {
	s.fallbackReads.Add(1)
	return s.stats.MaxConns
}

func (s *snapshotPoolStatter) EmptyAcquireCount() int64 {
	s.fallbackReads.Add(1)
	return s.stats.EmptyAcquireCount
}

func (s *snapshotPoolStatter) EmptyAcquireWaitTime() time.Duration {
	s.fallbackReads.Add(1)
	return s.stats.EmptyAcquireWaitTime
}

// Baseline tick (no delta) keeps shedding false.
func TestPoolBackpressureSampler_NoDeltaAdmits(t *testing.T) {
	ps := &fakePoolStatter{}
	s := newPoolBackpressureSampler(ps, time.Second, dbBackpressureAcquireWaitThreshold)
	s.lastCount = ps.EmptyAcquireCount()
	s.lastWait = ps.EmptyAcquireWaitTime()

	s.sampleOnce()
	require.False(t, s.Shedding())

	// counters unchanged → no signal
}

// Avg wait below threshold keeps shedding false; at or above the threshold
// flips it to true.
func TestPoolBackpressureSampler_ThresholdGate(t *testing.T) {
	ps := &fakePoolStatter{}
	s := newPoolBackpressureSampler(ps, time.Second, 50*time.Millisecond)
	s.lastCount = 0
	s.lastWait = 0

	// 10 acquires, total wait 400ms → avg 40ms (below threshold).
	ps.set(10, 400*time.Millisecond)
	s.sampleOnce()
	require.False(t, s.Shedding())

	// 5 more acquires, +300ms wait → avg 60ms in window (≥ threshold).
	ps.set(15, 700*time.Millisecond)
	s.sampleOnce()
	require.True(t, s.Shedding())

	// Wait subsides: next window adds counts but no extra wait → reset.
	ps.set(20, 700*time.Millisecond)
	s.sampleOnce()
	require.False(t, s.Shedding())
}

func TestPoolBackpressureStats_UsesSingleSnapshotWhenAvailable(t *testing.T) {
	ps := &snapshotPoolStatter{stats: PoolBackpressureStats{
		AcquiredConns:        91,
		MaxConns:             100,
		EmptyAcquireCount:    10,
		EmptyAcquireWaitTime: time.Second,
	}}

	stats := poolBackpressureStats(ps)

	require.Equal(t, ps.stats, stats)
	require.EqualValues(t, 1, ps.snapshotReads.Load())
	require.EqualValues(t, 0, ps.fallbackReads.Load())
}

func TestPoolBackpressureSampler_UsesSingleSnapshotPerSample(t *testing.T) {
	ps := &snapshotPoolStatter{stats: PoolBackpressureStats{
		EmptyAcquireCount:    10,
		EmptyAcquireWaitTime: time.Second,
	}}
	s := newPoolBackpressureSampler(ps, time.Second, 50*time.Millisecond)
	s.lastCount = 0
	s.lastWait = 0

	s.sampleOnce()

	require.True(t, s.Shedding())
	require.EqualValues(t, 1, ps.snapshotReads.Load())
	require.EqualValues(t, 0, ps.fallbackReads.Load())
}

// All concurrent readers must observe the same verdict — this is the property
// the old delta-in-middleware design failed to provide.
func TestPoolBackpressureSampler_ConcurrentReadsAreConsistent(t *testing.T) {
	ps := &fakePoolStatter{}
	s := newPoolBackpressureSampler(ps, time.Second, 50*time.Millisecond)
	ps.set(0, 0)
	s.sampleOnce()

	// Force shedding state.
	ps.set(10, 1*time.Second) // avg = 100ms
	s.sampleOnce()
	require.True(t, s.Shedding())

	const fanout = 200
	var sheds atomic.Int32
	var wg sync.WaitGroup
	wg.Add(fanout)
	for range fanout {
		go func() {
			defer wg.Done()
			if s.Shedding() {
				sheds.Add(1)
			}
		}()
	}
	wg.Wait()
	require.EqualValues(t, fanout,
		sheds.Load())
}

// Stop must release the goroutine and not block on a second call.
func TestPoolBackpressureSampler_StopIsIdempotent(t *testing.T) {
	ps := &fakePoolStatter{emptyCountReads: make(chan struct{}, 2)}
	s := newPoolBackpressureSampler(ps, 10*time.Millisecond, dbBackpressureAcquireWaitThreshold)
	s.Start()
	waitForEmptyAcquireCountReads(t, ps.emptyCountReads, 2)
	s.Stop()
	s.Stop() // second call must not block or panic
}

func waitForEmptyAcquireCountReads(t *testing.T, ch <-chan struct{}, want int) {
	t.Helper()
	timeout := time.After(time.Second)
	for got := 0; got < want; got++ {
		select {
		case <-ch:
		case <-timeout:
			require.Failf(t, "test failure", "timed out waiting for %d EmptyAcquireCount reads, got %d", want, got)
		}
	}
}

func TestShouldApplyDBBackpressure_NoPoolStatterAdmits(t *testing.T) {
	srv := &Server{}

	require.False(t, srv.shouldApplyDBBackpressure())
}

// Verifies that shouldApplyDBBackpressure reaches the same verdict regardless
// of how many concurrent callers fire — the bug N7 fixes is that the previous
// implementation admitted under load whenever any other caller had just
// updated the baseline within the same instant.
func TestShouldApplyDBBackpressure_AllConcurrentRequestsAgreeUnderPressure(t *testing.T) {
	ps := &fakePoolStatter{}
	ps.setOccupancy(0, 100) // occupancy clear, so the verdict is driven purely by the sampler
	srv := &Server{poolStatter: ps}
	srv.poolBackpressure = newPoolBackpressureSampler(ps, time.Second, 50*time.Millisecond)

	// Seed shedding=true via one synchronous sample tick.
	srv.poolBackpressure.lastCount = 0
	srv.poolBackpressure.lastWait = 0
	ps.set(10, 1*time.Second) // avg 100ms wait
	srv.poolBackpressure.sampleOnce()
	require.True(t, srv.poolBackpressure.
		Shedding(),
	)

	const fanout = 200
	var admitted atomic.Int32
	var wg sync.WaitGroup
	wg.Add(fanout)
	for range fanout {
		go func() {
			defer wg.Done()
			if !srv.shouldApplyDBBackpressure() {
				admitted.Add(1)
			}
		}()
	}
	wg.Wait()
	require.EqualValues(t, 0, admitted.
		Load())
}

// Occupancy >90% should shed regardless of sampler verdict — the snapshot
// signal is an independent safety net.
func TestShouldApplyDBBackpressure_HighOccupancyShortCircuits(t *testing.T) {
	ps := &fakePoolStatter{}
	ps.setOccupancy(91, 100)
	srv := &Server{poolStatter: ps}
	srv.poolBackpressure = newPoolBackpressureSampler(ps, time.Second, 50*time.Millisecond)
	require.True(t, srv.shouldApplyDBBackpressure())

	// Sampler is in admit state (no data) but occupancy alone should shed.
}

func TestShouldApplyDBBackpressure_UsesConfiguredOccupancyThreshold(t *testing.T) {
	ps := &fakePoolStatter{}
	ps.setOccupancy(95, 100)
	srv := &Server{
		config:      &config.Config{DBBackpressureOccupancyThreshold: 0.98},
		poolStatter: ps,
	}

	require.False(t, srv.shouldApplyDBBackpressure())

	ps.setOccupancy(99, 100)
	require.True(t, srv.shouldApplyDBBackpressure())
}

func TestDBBackpressureMetric_RecordsOccupancyReason(t *testing.T) {
	t.Parallel()

	metrics, reader := newDBBackpressureMetricsHarness(t)
	srv := NewServer(ServerDeps{
		Config: &config.Config{
			InternalSecret:      "test-secret-value",
			MaxBulkTriggerItems: 500,
			JWTSigningKey:       testJWTSigningKey,
		},
		Store:       &APIStoreMock{},
		Queue:       &mockQueue{},
		Metrics:     metrics,
		PoolStatter: newMockPoolStatter(24, 25),
	})
	t.Cleanup(srv.Close)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/jobs/job-1/trigger", nil))

	require.Equal(t, http.StatusTooManyRequests, w.Code)
	require.EqualValues(t, 1, sumDBBackpressureMetric(t, reader, "occupancy"))
	require.EqualValues(t, 0, sumDBBackpressureMetric(t, reader, "acquire_wait"))
}

func TestDBBackpressureMetric_RecordsAcquireWaitReason(t *testing.T) {
	t.Parallel()

	metrics, reader := newDBBackpressureMetricsHarness(t)
	statter := newMockPoolStatter(2, 25)
	srv := NewServer(ServerDeps{
		Config: &config.Config{
			InternalSecret:      "test-secret-value",
			MaxBulkTriggerItems: 500,
			JWTSigningKey:       testJWTSigningKey,
		},
		Store:       &APIStoreMock{},
		Queue:       &mockQueue{},
		Metrics:     metrics,
		PoolStatter: statter,
	})
	t.Cleanup(srv.Close)

	srv.poolBackpressure.Stop()
	statter.emptyAcquire.Store(10)
	statter.emptyAcquireWait.Store(int64(time.Second))
	srv.poolBackpressure.sampleOnce()

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/jobs/job-1/trigger", nil))

	require.Equal(t, http.StatusTooManyRequests, w.Code)
	require.EqualValues(t, 0, sumDBBackpressureMetric(t, reader, "occupancy"))
	require.EqualValues(t, 1, sumDBBackpressureMetric(t, reader, "acquire_wait"))
}

func newDBBackpressureMetricsHarness(t *testing.T) (*telemetry.Metrics, *sdkmetric.ManualReader) {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	meter := provider.Meter("db-backpressure-metrics-harness")

	shed, err := meter.Int64Counter("strait_db_backpressure_shed_total")
	require.NoError(t, err)

	httpDuration, err := meter.Float64Histogram("strait_http_request_duration_seconds")
	require.NoError(t, err)

	httpInflight, err := meter.Int64UpDownCounter("strait_http_inflight_requests")
	require.NoError(t, err)

	return &telemetry.Metrics{
		DBBackpressureShed:   shed,
		HTTPRequestDuration:  httpDuration,
		HTTPInflightRequests: httpInflight,
	}, reader
}

func sumDBBackpressureMetric(t *testing.T, reader *sdkmetric.ManualReader, reason string) int64 {
	t.Helper()

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))

	var total int64
	for _, sm := range rm.ScopeMetrics {
		for _, inst := range sm.Metrics {
			if inst.Name != "strait_db_backpressure_shed_total" {
				continue
			}
			sum, ok := inst.Data.(metricdata.Sum[int64])
			if !ok {
				continue
			}
			for _, dp := range sum.DataPoints {
				v, present := dp.Attributes.Value(attribute.Key("reason"))
				if present && v.AsString() == reason {
					total += dp.Value
				}
			}
		}
	}
	return total
}
