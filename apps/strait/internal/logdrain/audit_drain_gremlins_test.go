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
	if len(snap) != 3 {
		t.Fatalf("snapshot len = %d, want 3", len(snap))
	}
	want := []string{"2", "3", "4"}
	for i, w := range want {
		if snap[i].Event.ID != w {
			t.Errorf("snap[%d].Event.ID = %q, want %q", i, snap[i].Event.ID, w)
		}
	}
}

func TestFailureRingBuffer_ExactlyFull(t *testing.T) {
	t.Parallel()
	rb := newFailureRingBuffer(4)
	for i := range 4 {
		rb.add(domain.AuditEvent{ID: itoa(i)}, "r")
	}
	snap := rb.snapshot()
	if len(snap) != 4 {
		t.Fatalf("snapshot len = %d, want 4", len(snap))
	}
	for i := range 4 {
		if snap[i].Event.ID != itoa(i) {
			t.Errorf("snap[%d].Event.ID = %q, want %q", i, snap[i].Event.ID, itoa(i))
		}
	}
}

func TestFailureRingBuffer_MultipleLaps(t *testing.T) {
	t.Parallel()
	rb := newFailureRingBuffer(2)
	for i := range 7 {
		rb.add(domain.AuditEvent{ID: itoa(i)}, "r")
	}
	snap := rb.snapshot()
	if len(snap) != 2 {
		t.Fatalf("snapshot len = %d, want 2", len(snap))
	}
	if snap[0].Event.ID != "5" || snap[1].Event.ID != "6" {
		t.Errorf("snap IDs = [%s, %s], want [5, 6]", snap[0].Event.ID, snap[1].Event.ID)
	}
}

func TestFailureRingBuffer_ZeroCapacity_UsesDefault(t *testing.T) {
	t.Parallel()
	rb := newFailureRingBuffer(0)
	if rb.cap != siemSubDLQCapacity {
		t.Errorf("cap = %d, want %d (default)", rb.cap, siemSubDLQCapacity)
	}
}

func TestNewAuditSIEMDrain_ZeroDefaults(t *testing.T) {
	t.Parallel()
	d := NewAuditSIEMDrain("https://x.example.com", "tok", 0, 0)
	if d.batchSize != defaultSIEMBatchSize {
		t.Errorf("batchSize = %d, want %d", d.batchSize, defaultSIEMBatchSize)
	}
	if d.flushInterval != defaultSIEMFlushInterval {
		t.Errorf("flushInterval = %v, want %v", d.flushInterval, defaultSIEMFlushInterval)
	}
}

func TestNewAuditSIEMDrain_NegativeDefaults(t *testing.T) {
	t.Parallel()
	d := NewAuditSIEMDrain("https://x.example.com", "tok", -1, -time.Second)
	if d.batchSize != defaultSIEMBatchSize {
		t.Errorf("batchSize = %d, want %d", d.batchSize, defaultSIEMBatchSize)
	}
	if d.flushInterval != defaultSIEMFlushInterval {
		t.Errorf("flushInterval = %v, want %v", d.flushInterval, defaultSIEMFlushInterval)
	}
}

func TestNewAuditSIEMDrain_ClientTimeout(t *testing.T) {
	t.Parallel()
	d := NewAuditSIEMDrain("https://x.example.com", "tok", 0, 0)
	if d.client.Timeout != 30*time.Second {
		t.Errorf("client.Timeout = %v, want 30s", d.client.Timeout)
	}
}

func TestNewAuditSIEMDrain_PositivePreserved(t *testing.T) {
	t.Parallel()
	d := NewAuditSIEMDrain("https://x.example.com", "tok", 50, 3*time.Second)
	if d.batchSize != 50 {
		t.Errorf("batchSize = %d, want 50", d.batchSize)
	}
	if d.flushInterval != 3*time.Second {
		t.Errorf("flushInterval = %v, want 3s", d.flushInterval)
	}
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
		if cap(d.ch) != minSIEMBufferSize {
			t.Errorf("cap(ch) = %d, want %d (floor)", cap(d.ch), minSIEMBufferSize)
		}
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
		if cap(d.ch) != 400 {
			t.Errorf("cap(ch) = %d, want 400 (100*4)", cap(d.ch))
		}
	})
}

func TestAuditSIEMDrain_DrainRemainingToSubDLQ_MetricsFire(t *testing.T) {
	t.Parallel()
	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader))
	meter := provider.Meter("test")
	failCounter, err := meter.Int64Counter("test_drain_failed")
	if err != nil {
		t.Fatalf("create counter: %v", err)
	}

	d := NewAuditSIEMDrain("https://x.example.com", "tok", 100, time.Hour)
	d.SetMetrics(nil, failCounter, nil, nil)
	d.ch = make(chan domain.AuditEvent, 16)

	for i := range 3 {
		d.ch <- domain.AuditEvent{ID: itoa(i)}
	}
	d.drainRemainingToSubDLQ()

	if count := d.DrainedFailureCount(); count != 3 {
		t.Errorf("sub-DLQ count = %d, want 3", count)
	}

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}
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
	if failedVal != 3 {
		t.Errorf("failed counter = %d, want 3", failedVal)
	}
}

func TestAuditSIEMDrain_DrainRemainingToSubDLQ_Empty_NoWarn(t *testing.T) {
	t.Parallel()
	spy := &warnSpy{}
	d := NewAuditSIEMDrain("https://x.example.com", "tok", 100, time.Hour)
	d.logger = slog.New(spy)
	d.ch = make(chan domain.AuditEvent, 16)
	d.drainRemainingToSubDLQ()
	if spy.called.Load() {
		t.Error("Warn emitted on empty channel; want no warn when abandoned == 0")
	}
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
		t.Fatal("run goroutine did not exit")
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
	if !hasFullBatch {
		t.Errorf("no batch of size 2 observed; sizes = %v", sizes)
	}
	var total int
	for _, s := range sizes {
		total += s
	}
	if total != 6 {
		t.Errorf("total events = %d, want 6", total)
	}
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
	if !hasFullBatch {
		t.Errorf("no batch of size 2 observed; sizes = %v", sizes)
	}
	var total int
	for _, s := range sizes {
		total += s
	}
	if total != 6 {
		t.Errorf("total events = %d, want 6", total)
	}
}
