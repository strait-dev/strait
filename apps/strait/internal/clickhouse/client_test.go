package clickhouse

import (
	"context"
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
