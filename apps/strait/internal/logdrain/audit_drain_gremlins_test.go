package logdrain

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

type warnSpy struct{ called atomic.Bool }

func (s *warnSpy) Enabled(context.Context, slog.Level) bool { return true }
func (s *warnSpy) Handle(_ context.Context, r slog.Record) error {
	if r.Level >= slog.LevelWarn {
		s.called.Store(true)
	}
	return nil
}
func (s *warnSpy) WithAttrs([]slog.Attr) slog.Handler { return s }
func (s *warnSpy) WithGroup(string) slog.Handler      { return s }

func TestFailureRingBuffer_WrapAround_SnapshotOrder(t *testing.T) {
	t.Parallel()
	rb := newFailureRingBuffer(3)
	for i := range 5 {
		rb.add(domain.AuditEvent{ID: itoa(i)}, "r")
	}
	snap := rb.snapshot()
	require.Len(t, snap,
		3)

	want := []string{"2", "3", "4"}
	for i, w := range want {
		assert.Equal(t, w,
			snap[i].Event.ID,
		)

	}
}

func TestFailureRingBuffer_ExactlyFull(t *testing.T) {
	t.Parallel()
	rb := newFailureRingBuffer(4)
	for i := range 4 {
		rb.add(domain.AuditEvent{ID: itoa(i)}, "r")
	}
	snap := rb.snapshot()
	require.Len(t, snap,
		4)

	for i := range 4 {
		assert.Equal(t, itoa(i), snap[i].Event.
			ID,
		)

	}
}

func TestFailureRingBuffer_MultipleLaps(t *testing.T) {
	t.Parallel()
	rb := newFailureRingBuffer(2)
	for i := range 7 {
		rb.add(domain.AuditEvent{ID: itoa(i)}, "r")
	}
	snap := rb.snapshot()
	require.Len(t, snap,
		2)
	assert.False(t, snap[0].Event.ID !=
		"5" ||
		snap[1].Event.
			ID != "6",
	)

}

func TestFailureRingBuffer_ZeroCapacity_UsesDefault(t *testing.T) {
	t.Parallel()
	rb := newFailureRingBuffer(0)
	assert.Equal(t, siemSubDLQCapacity,

		rb.cap,
	)

}

func TestNewAuditSIEMDrain_ZeroDefaults(t *testing.T) {
	t.Parallel()
	d := NewAuditSIEMDrain("https://x.example.com", "tok", 0, 0)
	assert.Equal(t, defaultSIEMBatchSize,

		d.batchSize,
	)
	assert.Equal(t, defaultSIEMFlushInterval,

		d.flushInterval,
	)

}

func TestNewAuditSIEMDrain_NegativeDefaults(t *testing.T) {
	t.Parallel()
	d := NewAuditSIEMDrain("https://x.example.com", "tok", -1, -time.Second)
	assert.Equal(t, defaultSIEMBatchSize,

		d.batchSize,
	)
	assert.Equal(t, defaultSIEMFlushInterval,

		d.flushInterval,
	)

}

func TestNewAuditSIEMDrain_ClientTimeout(t *testing.T) {
	t.Parallel()
	d := NewAuditSIEMDrain("https://x.example.com", "tok", 0, 0)
	assert.Equal(t, 30*
		time.Second, d.
		client.
		Timeout)

}

func TestNewAuditSIEMDrain_PositivePreserved(t *testing.T) {
	t.Parallel()
	d := NewAuditSIEMDrain("https://x.example.com", "tok", 50, 3*time.Second)
	assert.Equal(t, 50,
		d.batchSize)
	assert.Equal(t, 3*
		time.Second, d.
		flushInterval,
	)

}

func TestAuditSIEMDrain_Start_ChannelCapacity(t *testing.T) {
	t.Parallel()
	t.Run("floor", func(t *testing.T) {
		t.Parallel()
		d := NewAuditSIEMDrain("https://x.example.com", "tok", 5, time.Hour)
		d.Start(context.Background())
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			defer cancel()
			d.Stop(ctx)
		})
		assert.Equal(t, minSIEMBufferSize,

			cap(d.
				ch))

	})
	t.Run("above_min", func(t *testing.T) {
		t.Parallel()
		d := NewAuditSIEMDrain("https://x.example.com", "tok", 100, time.Hour)
		d.Start(context.Background())
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			defer cancel()
			d.Stop(ctx)
		})
		assert.Equal(t, 400,
			cap(d.ch))

	})
}

func TestAuditSIEMDrain_DrainRemainingToSubDLQ_MetricsFire(t *testing.T) {
	t.Parallel()
	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader))
	meter := provider.Meter("test")
	failCounter, err := meter.Int64Counter("test_drain_failed")
	require.NoError(t,
		err)

	d := NewAuditSIEMDrain("https://x.example.com", "tok", 100, time.Hour)
	d.SetMetrics(nil, failCounter, nil, nil)
	d.ch = make(chan domain.AuditEvent, 16)

	for i := range 3 {
		d.ch <- domain.AuditEvent{ID: itoa(i)}
	}
	d.drainRemainingToSubDLQ()

	assert.Equal(t, 3, d.DrainedFailureCount())

	var rm metricdata.ResourceMetrics
	require.NoError(t,
		reader.Collect(context.
			Background(), &rm))

	var failedVal int64
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == "test_drain_failed" {
				if sum, ok := m.Data.(metricdata.Sum[int64]); ok {
					for _, dp := range sum.DataPoints {
						failedVal += dp.Value
					}
				}
			}
		}
	}
	assert.Equal(t, int64(3),
		failedVal)

}

func TestAuditSIEMDrain_DrainRemainingToSubDLQ_Empty_NoWarn(t *testing.T) {
	t.Parallel()
	spy := &warnSpy{}
	d := NewAuditSIEMDrain("https://x.example.com", "tok", 100, time.Hour)
	d.logger = slog.New(spy)
	d.ch = make(chan domain.AuditEvent, 16)
	d.drainRemainingToSubDLQ()
	assert.False(t, spy.
		called.Load())

}

func TestAuditSIEMDrain_FlushLocked_NilParentCtx(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := NewAuditSIEMDrain(srv.URL, "tok", 1, time.Hour)
	d.ch = make(chan domain.AuditEvent, 16)
	d.done = make(chan struct{})
	d.shutdownCh = make(chan struct{})
	d.startOnce.Do(func() {})

	d.ch <- domain.AuditEvent{ID: "ev-1"}
	close(d.shutdownCh)

	go d.run(context.Background())

	select {
	case <-d.done:
	case <-time.After(5 * time.Second):
		require.Fail(t, "run goroutine did not exit")
	}
}

func TestAuditSIEMDrain_ShutdownCh_BatchFullFlush(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var sizes []int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		lines := strings.Split(strings.TrimSpace(string(body)), "\n")
		n := 0
		for _, line := range lines {
			var ev domain.AuditEvent
			if json.Unmarshal([]byte(line), &ev) == nil && ev.ID != "" {
				n++
			}
		}
		mu.Lock()
		sizes = append(sizes, n)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := NewAuditSIEMDrain(srv.URL, "tok", 2, time.Hour)
	d.ch = make(chan domain.AuditEvent, 64)
	d.done = make(chan struct{})
	d.shutdownCh = make(chan struct{})
	d.parentCtx = context.Background()
	d.startOnce.Do(func() {})

	for i := range 6 {
		d.ch <- domain.AuditEvent{ID: itoa(i)}
	}
	close(d.shutdownCh)

	go d.run(context.Background())
	<-d.done

	mu.Lock()
	defer mu.Unlock()
	hasFullBatch := false
	for _, s := range sizes {
		if s == 2 {
			hasFullBatch = true
		}
	}
	assert.True(t, hasFullBatch)

	var total int
	for _, s := range sizes {
		total += s
	}
	assert.Equal(t, 6,
		total)

}

func TestAuditSIEMDrain_CtxDone_BatchFullFlush_Deterministic(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var sizes []int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		lines := strings.Split(strings.TrimSpace(string(body)), "\n")
		n := 0
		for _, line := range lines {
			var ev domain.AuditEvent
			if json.Unmarshal([]byte(line), &ev) == nil && ev.ID != "" {
				n++
			}
		}
		mu.Lock()
		sizes = append(sizes, n)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	d := NewAuditSIEMDrain(srv.URL, "tok", 2, time.Hour)
	d.ch = make(chan domain.AuditEvent, 64)
	d.done = make(chan struct{})
	d.shutdownCh = make(chan struct{})
	d.parentCtx = context.Background()
	d.startOnce.Do(func() {})

	for i := range 6 {
		d.ch <- domain.AuditEvent{ID: itoa(i)}
	}

	go d.run(ctx)
	<-d.done

	mu.Lock()
	defer mu.Unlock()
	hasFullBatch := false
	for _, s := range sizes {
		if s == 2 {
			hasFullBatch = true
		}
	}
	assert.True(t, hasFullBatch)

	var total int
	for _, s := range sizes {
		total += s
	}
	assert.Equal(t, 6,
		total)

}
