package cdc

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"strait/internal/clickhouse"
)

// testExporter creates a non-nil Exporter for testing by using an exported
// constructor helper. Since NewExporter returns nil for nil clients, we
// build the struct through a test-friendly wrapper.
func newTestExporter() *clickhouse.Exporter {
	// Use the package-level test helper if available; otherwise construct
	// a minimal exporter that can buffer Enqueue calls.
	return clickhouse.NewTestExporter()
}

func TestAnalyticsHandler_CompletedRun_Enqueues(t *testing.T) {
	t.Parallel()
	exp := newTestExporter()
	h := NewAnalyticsHandler(exp, nil)

	record, _ := json.Marshal(map[string]any{
		"id":          "run-1",
		"job_id":      "job-1",
		"project_id":  "p1",
		"status":      "completed",
		"attempt":     1,
		"started_at":  "2026-03-26T10:00:00Z",
		"finished_at": "2026-03-26T10:00:05Z",
		"created_at":  "2026-03-26T09:59:00Z",
	})
	msg := Message{
		AckID:    "ack-1",
		Action:   ActionUpdate,
		Record:   record,
		Metadata: Metadata{TableName: "job_runs"},
	}

	err := h.Handle(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pending := exp.PendingLen()
	if pending != 1 {
		t.Fatalf("expected 1 enqueued record, got %d", pending)
	}
}

func TestAnalyticsHandler_NonTerminal_Skipped(t *testing.T) {
	t.Parallel()
	exp := newTestExporter()
	h := NewAnalyticsHandler(exp, nil)

	err := h.Handle(context.Background(), cdcUpdateMsg("executing", "p1", "run-1", "job-1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if exp.PendingLen() != 0 {
		t.Fatal("expected no enqueued records for non-terminal status")
	}
}

func TestAnalyticsHandler_NilExporter_NoError(t *testing.T) {
	t.Parallel()
	h := NewAnalyticsHandler(nil, nil)

	err := h.Handle(context.Background(), cdcUpdateMsg("completed", "p1", "run-1", "job-1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAnalyticsHandler_ComputesDuration(t *testing.T) {
	t.Parallel()
	exp := newTestExporter()
	h := NewAnalyticsHandler(exp, nil)

	started := time.Date(2026, 3, 26, 10, 0, 0, 0, time.UTC)
	finished := started.Add(5 * time.Second)

	record, _ := json.Marshal(map[string]any{
		"id":          "run-1",
		"job_id":      "job-1",
		"project_id":  "p1",
		"status":      "completed",
		"attempt":     1,
		"started_at":  started.Format(time.RFC3339),
		"finished_at": finished.Format(time.RFC3339),
		"created_at":  started.Add(-time.Minute).Format(time.RFC3339),
	})
	msg := Message{
		AckID:    "ack-1",
		Action:   ActionUpdate,
		Record:   record,
		Metadata: Metadata{TableName: "job_runs"},
	}

	err := h.Handle(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if exp.PendingLen() != 1 {
		t.Fatal("expected 1 enqueued record")
	}

	rec, ok := exp.PendingAt(0).(clickhouse.RunAnalyticsRecord)
	if !ok {
		t.Fatal("expected RunAnalyticsRecord type")
	}
	if rec.DurationMs != 5000 {
		t.Errorf("expected DurationMs=5000, got %d", rec.DurationMs)
	}
}

func TestAnalyticsHandler_InvalidJSON_ReturnsError(t *testing.T) {
	t.Parallel()
	exp := newTestExporter()
	h := NewAnalyticsHandler(exp, nil)

	msg := Message{
		Action:   ActionUpdate,
		Record:   json.RawMessage(`not valid json`),
		Metadata: Metadata{TableName: "job_runs"},
	}

	err := h.Handle(context.Background(), msg)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestAnalyticsHandler_ZeroDuration(t *testing.T) {
	t.Parallel()
	exp := newTestExporter()
	h := NewAnalyticsHandler(exp, nil)

	ts := "2026-03-26T10:00:00Z"
	record, _ := json.Marshal(map[string]any{
		"id": "run-1", "job_id": "job-1", "project_id": "p1",
		"status": "completed", "attempt": 1,
		"started_at": ts, "finished_at": ts,
		"created_at": ts,
	})
	msg := Message{AckID: "ack-1", Action: ActionUpdate, Record: record, Metadata: Metadata{TableName: "job_runs"}}

	if err := h.Handle(context.Background(), msg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exp.PendingLen() != 1 {
		t.Fatal("expected 1 enqueued record")
	}
	rec, ok := exp.PendingAt(0).(clickhouse.RunAnalyticsRecord)
	if !ok {
		t.Fatal("expected RunAnalyticsRecord type")
	}
	if rec.DurationMs != 0 {
		t.Errorf("expected DurationMs=0 when started_at == finished_at, got %d", rec.DurationMs)
	}
}

func TestAnalyticsHandler_NegativeDuration(t *testing.T) {
	t.Parallel()
	exp := newTestExporter()
	h := NewAnalyticsHandler(exp, nil)

	started := time.Date(2026, 3, 26, 10, 0, 5, 0, time.UTC)
	finished := time.Date(2026, 3, 26, 10, 0, 0, 0, time.UTC)

	record, _ := json.Marshal(map[string]any{
		"id": "run-1", "job_id": "job-1", "project_id": "p1",
		"status": "completed", "attempt": 1,
		"started_at":  started.Format(time.RFC3339),
		"finished_at": finished.Format(time.RFC3339),
		"created_at":  started.Add(-time.Minute).Format(time.RFC3339),
	})
	msg := Message{AckID: "ack-1", Action: ActionUpdate, Record: record, Metadata: Metadata{TableName: "job_runs"}}

	if err := h.Handle(context.Background(), msg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rec, ok := exp.PendingAt(0).(clickhouse.RunAnalyticsRecord)
	if !ok {
		t.Fatal("expected RunAnalyticsRecord type")
	}
	if rec.DurationMs != 0 {
		t.Errorf("expected DurationMs=0 for negative duration (clock skew), got %d", rec.DurationMs)
	}
}

func TestAnalyticsHandler_NilStartedAt(t *testing.T) {
	t.Parallel()
	exp := newTestExporter()
	h := NewAnalyticsHandler(exp, nil)

	record, _ := json.Marshal(map[string]any{
		"id": "run-1", "job_id": "job-1", "project_id": "p1",
		"status": "completed", "attempt": 1,
		"started_at": "", "finished_at": "2026-03-26T10:00:05Z",
		"created_at": "2026-03-26T09:59:00Z",
	})
	msg := Message{AckID: "ack-1", Action: ActionUpdate, Record: record, Metadata: Metadata{TableName: "job_runs"}}

	if err := h.Handle(context.Background(), msg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rec, ok := exp.PendingAt(0).(clickhouse.RunAnalyticsRecord)
	if !ok {
		t.Fatal("expected RunAnalyticsRecord type")
	}
	if rec.DurationMs != 0 {
		t.Errorf("expected DurationMs=0 when started_at is empty, got %d", rec.DurationMs)
	}
}

func TestAnalyticsHandler_NilFinishedAt(t *testing.T) {
	t.Parallel()
	exp := newTestExporter()
	h := NewAnalyticsHandler(exp, nil)

	record, _ := json.Marshal(map[string]any{
		"id": "run-1", "job_id": "job-1", "project_id": "p1",
		"status": "completed", "attempt": 1,
		"started_at": "2026-03-26T10:00:00Z", "finished_at": "",
		"created_at": "2026-03-26T09:59:00Z",
	})
	msg := Message{AckID: "ack-1", Action: ActionUpdate, Record: record, Metadata: Metadata{TableName: "job_runs"}}

	if err := h.Handle(context.Background(), msg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rec, ok := exp.PendingAt(0).(clickhouse.RunAnalyticsRecord)
	if !ok {
		t.Fatal("expected RunAnalyticsRecord type")
	}
	if rec.DurationMs != 0 {
		t.Errorf("expected DurationMs=0 when finished_at is empty, got %d", rec.DurationMs)
	}
}

func TestAnalyticsHandler_EnqueueFails(t *testing.T) {
	t.Parallel()
	exp := clickhouse.NewTestExporterStopping()
	h := NewAnalyticsHandler(exp, nil)

	record, _ := json.Marshal(map[string]any{
		"id": "run-1", "job_id": "job-1", "project_id": "p1",
		"status": "completed", "attempt": 1,
		"started_at":  "2026-03-26T10:00:00Z",
		"finished_at": "2026-03-26T10:00:05Z",
		"created_at":  "2026-03-26T09:59:00Z",
	})
	msg := Message{AckID: "ack-1", Action: ActionUpdate, Record: record, Metadata: Metadata{TableName: "job_runs"}}

	err := h.Handle(context.Background(), msg)
	if err != nil {
		t.Fatalf("expected no error when enqueue fails (warn only), got %v", err)
	}
	if exp.PendingLen() != 0 {
		t.Fatal("expected no buffered records in stopping exporter")
	}
}
