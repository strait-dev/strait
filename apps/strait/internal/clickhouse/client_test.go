package clickhouse

import (
	"context"
	"log/slog"
	"testing"
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

	e.Enqueue("test")
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

	e.Enqueue("record1")
	e.Enqueue("record2")

	if e.PendingCount() != 2 {
		t.Errorf("pending count = %d, want 2", e.PendingCount())
	}
}

func TestExporter_Backpressure(t *testing.T) {
	t.Parallel()
	e := NewExporter(&Client{}, ExporterConfig{Enabled: true, BatchSize: 2}, slog.Default())

	// Fill beyond 10x batch size (20).
	for i := range 25 {
		e.Enqueue(i)
	}

	if e.PendingCount() > 20 {
		t.Errorf("pending count = %d, should be capped at 20", e.PendingCount())
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
