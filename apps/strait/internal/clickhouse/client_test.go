package clickhouse

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"
)

func TestNew_Disabled(t *testing.T) {
	t.Parallel()
	c, err := New(Config{Enabled: false}, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if c != nil {
		t.Error("expected nil client when disabled")
	}
}

func TestNew_EnabledWithoutURL(t *testing.T) {
	t.Parallel()
	_, err := New(Config{Enabled: true, URL: ""}, nil)
	if err == nil {
		t.Error("expected error when enabled without URL")
	}
}

func TestClient_Nil_Operations(t *testing.T) {
	t.Parallel()
	var c *Client

	if c.Healthy(context.Background()) {
		t.Error("nil client should not be healthy")
	}
	if err := c.Close(); err != nil {
		t.Errorf("nil client Close() error = %v", err)
	}
	if c.DB() != nil {
		t.Error("nil client DB() should return nil")
	}
	if err := c.Exec(context.Background(), "SELECT 1"); err != nil {
		t.Errorf("nil client Exec() error = %v", err)
	}
}

func TestExporter_Nil_Operations(t *testing.T) {
	t.Parallel()
	var e *Exporter

	if e.Enqueue("test") {
		t.Error("nil exporter Enqueue should return false")
	}
	e.Start(context.Background())
	e.Stop()
	if e.PendingCount() != 0 {
		t.Error("nil exporter should have 0 pending")
	}
}

func TestExporter_Enqueue_And_PendingCount(t *testing.T) {
	t.Parallel()
	e := NewExporter(&Client{}, ExporterConfig{Enabled: true, BatchSize: 100}, slog.Default())
	if e == nil {
		t.Fatal("expected non-nil exporter")
	}

	if !e.Enqueue("record1") {
		t.Error("Enqueue should return true")
	}
	e.Enqueue("record2")

	if e.PendingCount() != 2 {
		t.Errorf("pending count = %d, want 2", e.PendingCount())
	}
}

func TestExporter_Backpressure_CapsAndReallocates(t *testing.T) {
	t.Parallel()
	e := NewExporter(&Client{}, ExporterConfig{Enabled: true, BatchSize: 2}, slog.Default())

	// Fill beyond 10x batch size (20).
	for i := range 25 {
		e.Enqueue(i)
	}

	count := e.PendingCount()
	if count != 20 {
		t.Errorf("pending count = %d, want exactly 20", count)
	}
}

func TestExporter_StopRejectsNewEnqueues(t *testing.T) {
	t.Parallel()
	e := NewExporter(&Client{}, ExporterConfig{Enabled: true, BatchSize: 100, FlushInterval: time.Hour}, slog.Default())

	ctx, cancel := context.WithCancel(context.Background())
	e.Start(ctx)

	e.Enqueue("before-stop")
	if e.PendingCount() != 1 {
		t.Fatalf("pending = %d, want 1", e.PendingCount())
	}

	cancel()
	e.Stop()

	if e.Enqueue("after-stop") {
		t.Error("Enqueue after Stop should return false")
	}
}

func TestExporter_ConcurrentEnqueue(t *testing.T) {
	t.Parallel()
	e := NewExporter(&Client{}, ExporterConfig{Enabled: true, BatchSize: 1000}, slog.Default())

	const goroutines = 10
	const perGoroutine = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			for range perGoroutine {
				e.Enqueue("x")
			}
		}()
	}
	wg.Wait()

	if e.PendingCount() != goroutines*perGoroutine {
		t.Errorf("pending = %d, want %d", e.PendingCount(), goroutines*perGoroutine)
	}
}

func TestExporter_FlushDrainsPending(t *testing.T) {
	t.Parallel()
	e := NewExporter(&Client{}, ExporterConfig{Enabled: true, BatchSize: 100, FlushInterval: 10 * time.Millisecond}, slog.Default())

	e.Enqueue("a")
	e.Enqueue("b")

	ctx := context.Background()
	e.Start(ctx)
	time.Sleep(50 * time.Millisecond)
	e.Stop()

	if e.PendingCount() != 0 {
		t.Errorf("after stop, pending = %d, want 0", e.PendingCount())
	}
}

func TestExporter_DisabledClient(t *testing.T) {
	t.Parallel()
	e := NewExporter(nil, ExporterConfig{Enabled: true}, nil)
	if e != nil {
		t.Error("expected nil exporter when client is nil")
	}
}

func TestExporter_DisabledConfig(t *testing.T) {
	t.Parallel()
	e := NewExporter(&Client{}, ExporterConfig{Enabled: false}, nil)
	if e != nil {
		t.Error("expected nil exporter when disabled")
	}
}

func TestConfig_CustomPoolSize(t *testing.T) {
	t.Parallel()
	cfg := Config{MaxOpenConns: 20, MaxIdleConns: 10}
	if cfg.MaxOpenConns != 20 || cfg.MaxIdleConns != 10 {
		t.Error("pool config not set")
	}
}

func TestExporter_InsertBatch_TypeRouting(t *testing.T) {
	t.Parallel()
	// Use a nil-db client so Exec is a no-op.
	e := NewExporter(&Client{}, ExporterConfig{Enabled: true, BatchSize: 100}, slog.Default())

	now := time.Now()
	e.Enqueue(RunEventRecord{
		EventID:   "evt-1",
		RunID:     "run-1",
		ProjectID: "proj-1",
		EventType: "log",
		Level:     "info",
		Message:   "test",
		CreatedAt: now,
	})
	e.Enqueue(RunAnalyticsRecord{
		RunID:     "run-1",
		ProjectID: "proj-1",
		Status:    "completed",
		CreatedAt: now,
	})
	e.Enqueue(ComputeUsageRecord{
		RunID:        "run-1",
		ProjectID:    "proj-1",
		DurationSecs: 10.5,
		StartedAt:    now,
		FinishedAt:   now,
	})
	// Unknown type should be silently logged.
	e.Enqueue("unknown-type")

	if e.PendingCount() != 4 {
		t.Fatalf("pending = %d, want 4", e.PendingCount())
	}

	// Flush manually - nil db means Exec is a no-op, so this verifies
	// the type-switching and batching logic without a real DB.
	e.flush(context.Background())

	if e.PendingCount() != 0 {
		t.Errorf("after flush, pending = %d, want 0", e.PendingCount())
	}
}

func TestExporter_InsertBatch_EmptyBatch(t *testing.T) {
	t.Parallel()
	e := NewExporter(&Client{}, ExporterConfig{Enabled: true, BatchSize: 100}, slog.Default())

	// flush with no pending records should be a no-op.
	e.flush(context.Background())
	if e.PendingCount() != 0 {
		t.Errorf("pending = %d, want 0", e.PendingCount())
	}
}

func TestExporter_InsertBatch_NilClient(t *testing.T) {
	t.Parallel()
	e := &Exporter{
		client:  nil,
		config:  ExporterConfig{BatchSize: 100},
		logger:  slog.Default(),
		pending: make([]any, 0),
		stopCh:  make(chan struct{}),
		done:    make(chan struct{}),
	}

	err := e.insertBatch(context.Background(), []any{RunEventRecord{EventID: "evt-1"}})
	if err != nil {
		t.Errorf("insertBatch with nil client should return nil, got %v", err)
	}
}

func TestExporter_MultipleBatchFlushes(t *testing.T) {
	t.Parallel()
	e := NewExporter(&Client{}, ExporterConfig{Enabled: true, BatchSize: 100, FlushInterval: 10 * time.Millisecond}, slog.Default())

	now := time.Now()

	ctx := context.Background()
	e.Start(ctx)

	// First batch.
	for i := range 5 {
		e.Enqueue(RunEventRecord{EventID: fmt.Sprintf("evt-%d", i), CreatedAt: now})
	}
	time.Sleep(30 * time.Millisecond)

	// Second batch.
	for i := range 3 {
		e.Enqueue(RunAnalyticsRecord{RunID: fmt.Sprintf("run-%d", i), CreatedAt: now})
	}
	time.Sleep(30 * time.Millisecond)

	e.Stop()

	if e.PendingCount() != 0 {
		t.Errorf("after stop, pending = %d, want 0", e.PendingCount())
	}
}

func TestCreateSchema_NilClient(t *testing.T) {
	t.Parallel()
	err := CreateSchema(context.Background(), nil)
	if err != nil {
		t.Errorf("CreateSchema(nil) error = %v", err)
	}
}
