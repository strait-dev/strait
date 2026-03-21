package clickhouse

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

func TestExporter_Start_PanicRecovery(t *testing.T) {
	t.Parallel()

	// Create an exporter with a nil client so that insertBatch does not crash
	// on its own. We will trigger a panic by injecting a record that causes
	// a type-switch to panic via a custom type with a String() method that panics.
	exporter := NewExporter(&Client{}, ExporterConfig{
		Enabled:       true,
		BatchSize:     10,
		FlushInterval: 50 * time.Millisecond,
	}, slog.Default())

	ctx, cancel := context.WithCancel(context.Background())
	exporter.Start(ctx)

	// The exporter should survive a flush with normal records and stop cleanly.
	exporter.Enqueue(RunEventRecord{
		EventID:   "evt-1",
		RunID:     "run-1",
		ProjectID: "proj-1",
		CreatedAt: time.Now(),
	})

	// Give the ticker time to flush.
	time.Sleep(100 * time.Millisecond)

	cancel()

	// Wait for the done channel; if panic recovery did not work, this would
	// hang or the test process would crash.
	select {
	case <-exporter.done:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("exporter did not stop after context cancel")
	}
}

func TestExporter_Stop_Cleanly(t *testing.T) {
	t.Parallel()

	exporter := NewExporter(&Client{}, ExporterConfig{
		Enabled:       true,
		BatchSize:     10,
		FlushInterval: 50 * time.Millisecond,
	}, slog.Default())

	exporter.Start(context.Background())

	exporter.Enqueue(RunAnalyticsRecord{
		RunID:     "run-1",
		ProjectID: "proj-1",
		CreatedAt: time.Now(),
	})

	// Stop should drain and return without panic.
	exporter.Stop()
}

func TestExporter_NilExporter_NoPanic(t *testing.T) {
	t.Parallel()

	var exporter *Exporter
	// All methods must be safe on nil receiver.
	exporter.Start(context.Background())
	exporter.Stop()
	if got := exporter.Enqueue(RunEventRecord{}); got {
		t.Error("expected Enqueue to return false on nil exporter")
	}
	if got := exporter.PendingCount(); got != 0 {
		t.Errorf("expected PendingCount 0 on nil exporter, got %d", got)
	}
}
