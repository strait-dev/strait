//go:build integration

package store_test

import (
	"context"
	"testing"
)

func TestGetJobCostEstimate_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	est, err := q.GetJobCostEstimate(ctx, newID())
	if err != nil {
		t.Fatalf("GetJobCostEstimate() error = %v", err)
	}
	if est != nil {
		t.Fatalf("GetJobCostEstimate() = %+v, want nil", est)
	}
}

func TestUpsertJobCostEstimate(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-cost-estimate-upsert")

	// Should succeed even with no completed runs.
	if err := q.UpsertJobCostEstimate(ctx, job.ID); err != nil {
		t.Fatalf("UpsertJobCostEstimate() error = %v", err)
	}

	est, err := q.GetJobCostEstimate(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetJobCostEstimate() error = %v", err)
	}
	if est == nil {
		t.Fatal("GetJobCostEstimate() returned nil after upsert")
	}
	if est.JobID != job.ID {
		t.Fatalf("JobID = %q, want %q", est.JobID, job.ID)
	}
	if est.SampleCount != 0 {
		t.Fatalf("SampleCount = %d, want 0", est.SampleCount)
	}
}

func TestUpsertJobCostEstimate_Idempotent(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-cost-estimate-idempotent")

	// Run twice to test ON CONFLICT.
	if err := q.UpsertJobCostEstimate(ctx, job.ID); err != nil {
		t.Fatalf("UpsertJobCostEstimate(1) error = %v", err)
	}
	if err := q.UpsertJobCostEstimate(ctx, job.ID); err != nil {
		t.Fatalf("UpsertJobCostEstimate(2) error = %v", err)
	}
}

func TestListActiveJobIDs_ContainsCreatedJob(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-active-job-ids")

	ids, err := q.ListActiveJobIDs(ctx)
	if err != nil {
		t.Fatalf("ListActiveJobIDs() error = %v", err)
	}

	found := false
	for _, id := range ids {
		if id == job.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("ListActiveJobIDs() did not contain %q", job.ID)
	}
}
