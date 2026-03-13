//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"testing"

	"strait/internal/domain"
)

func TestCreateRun_WithBatchIDAndConcurrencyKey(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	job := mustCreateJob(t, ctx, q, "project-run-new-cols")

	batchOp := &domain.BatchOperation{
		ID:        newID(),
		ProjectID: job.ProjectID,
		JobID:     job.ID,
		ItemCount: 5,
		CreatedBy: "test",
	}
	if err := q.CreateBatchOperation(ctx, batchOp); err != nil {
		t.Fatalf("CreateBatchOperation() error = %v", err)
	}

	run := baseRun(job, newID())
	run.BatchID = batchOp.ID
	run.ConcurrencyKey = "tenant-123"
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	got, err := q.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if got.BatchID != batchOp.ID {
		t.Fatalf("BatchID = %q, want %q", got.BatchID, batchOp.ID)
	}
	if got.ConcurrencyKey != "tenant-123" {
		t.Fatalf("ConcurrencyKey = %q, want %q", got.ConcurrencyKey, "tenant-123")
	}
}

func TestCreateRun_NilBatchIDAndConcurrencyKey(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	job := mustCreateJob(t, ctx, q, "project-run-nil-cols")

	run := baseRun(job, newID())
	// Leave BatchID and ConcurrencyKey at zero values.
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	got, err := q.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if got.BatchID != "" {
		t.Fatalf("BatchID = %q, want empty", got.BatchID)
	}
	if got.ConcurrencyKey != "" {
		t.Fatalf("ConcurrencyKey = %q, want empty", got.ConcurrencyKey)
	}
}

func TestListRunsByProject_TriggeredByFilter(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	job := mustCreateJob(t, ctx, q, "project-triggered-filter")

	manualRun := baseRun(job, newID())
	manualRun.TriggeredBy = domain.TriggerManual
	if err := q.CreateRun(ctx, manualRun); err != nil {
		t.Fatalf("CreateRun(manual) error = %v", err)
	}

	cronRun := baseRun(job, newID())
	cronRun.TriggeredBy = domain.TriggerCron
	if err := q.CreateRun(ctx, cronRun); err != nil {
		t.Fatalf("CreateRun(cron) error = %v", err)
	}

	triggeredBy := domain.TriggerManual
	runs, err := q.ListRunsByProject(ctx, job.ProjectID, nil, nil, nil, &triggeredBy, nil, nil, 20, nil)
	if err != nil {
		t.Fatalf("ListRunsByProject() error = %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("len(runs) = %d, want 1", len(runs))
	}
	if runs[0].TriggeredBy != domain.TriggerManual {
		t.Fatalf("TriggeredBy = %q, want %q", runs[0].TriggeredBy, domain.TriggerManual)
	}
}

func TestListRunsByProject_PayloadContainsFilter(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	job := mustCreateJob(t, ctx, q, "project-payload-filter")

	run1 := baseRun(job, newID())
	run1.Payload = json.RawMessage(`{"hello":"world","extra":"data"}`)
	if err := q.CreateRun(ctx, run1); err != nil {
		t.Fatalf("CreateRun(run1) error = %v", err)
	}

	run2 := baseRun(job, newID())
	run2.Payload = json.RawMessage(`{"other":"payload"}`)
	if err := q.CreateRun(ctx, run2); err != nil {
		t.Fatalf("CreateRun(run2) error = %v", err)
	}

	runs, err := q.ListRunsByProject(ctx, job.ProjectID, nil, nil, nil, nil, nil, json.RawMessage(`{"hello":"world"}`), 20, nil)
	if err != nil {
		t.Fatalf("ListRunsByProject() error = %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("len(runs) = %d, want 1", len(runs))
	}
	if runs[0].ID != run1.ID {
		t.Fatalf("run ID = %q, want %q", runs[0].ID, run1.ID)
	}
}
