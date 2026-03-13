//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
)

func TestCreateBatchOperation(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-batch-create")

	op := &domain.BatchOperation{
		ID:        newID(),
		ProjectID: job.ProjectID,
		JobID:     job.ID,
		ItemCount: 10,
		CreatedBy: "user-1",
	}
	if err := q.CreateBatchOperation(ctx, op); err != nil {
		t.Fatalf("CreateBatchOperation() error = %v", err)
	}

	got, err := q.GetBatchOperation(ctx, op.ID, op.ProjectID)
	if err != nil {
		t.Fatalf("GetBatchOperation() error = %v", err)
	}
	if got.ID != op.ID {
		t.Fatalf("ID = %q, want %q", got.ID, op.ID)
	}
	if got.ProjectID != op.ProjectID {
		t.Fatalf("ProjectID = %q, want %q", got.ProjectID, op.ProjectID)
	}
	if got.JobID != op.JobID {
		t.Fatalf("JobID = %q, want %q", got.JobID, op.JobID)
	}
	if got.ItemCount != 10 {
		t.Fatalf("ItemCount = %d, want 10", got.ItemCount)
	}
	if got.CreatedCount != 0 {
		t.Fatalf("CreatedCount = %d, want 0", got.CreatedCount)
	}
	if got.CreatedAt.IsZero() {
		t.Fatalf("CreatedAt is zero")
	}
	if got.FinishedAt != nil {
		t.Fatalf("FinishedAt = %v, want nil", got.FinishedAt)
	}
}

func TestFinalizeBatchOperation(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-batch-finalize")

	op := &domain.BatchOperation{
		ID:        newID(),
		ProjectID: job.ProjectID,
		JobID:     job.ID,
		ItemCount: 5,
		CreatedBy: "user-1",
	}
	if err := q.CreateBatchOperation(ctx, op); err != nil {
		t.Fatalf("CreateBatchOperation() error = %v", err)
	}

	if err := q.FinalizeBatchOperation(ctx, op.ID, 3); err != nil {
		t.Fatalf("FinalizeBatchOperation() error = %v", err)
	}

	got, err := q.GetBatchOperation(ctx, op.ID, op.ProjectID)
	if err != nil {
		t.Fatalf("GetBatchOperation() error = %v", err)
	}
	if got.CreatedCount != 3 {
		t.Fatalf("CreatedCount = %d, want 3", got.CreatedCount)
	}
	if got.FinishedAt == nil {
		t.Fatalf("FinishedAt is nil, want non-nil")
	}
}

func TestGetBatchOperation_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, err := q.GetBatchOperation(ctx, newID(), "nonexistent-project")
	if err == nil {
		t.Fatalf("GetBatchOperation() expected error, got nil")
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("GetBatchOperation() error = %v, want pgx.ErrNoRows", err)
	}
}

func TestListBatchOperations(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-batch-list")

	for i := 0; i < 3; i++ {
		op := &domain.BatchOperation{
			ID:        newID(),
			ProjectID: job.ProjectID,
			JobID:     job.ID,
			ItemCount: i + 1,
			CreatedBy: "user-1",
		}
		if err := q.CreateBatchOperation(ctx, op); err != nil {
			t.Fatalf("CreateBatchOperation()[%d] error = %v", i, err)
		}
		time.Sleep(time.Millisecond)
	}

	// First page: limit 2
	page1, err := q.ListBatchOperations(ctx, job.ProjectID, 2, nil)
	if err != nil {
		t.Fatalf("ListBatchOperations() page1 error = %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("ListBatchOperations() page1 len = %d, want 2", len(page1))
	}
	if !page1[0].CreatedAt.After(page1[1].CreatedAt) {
		t.Fatalf("expected descending order, got %v then %v", page1[0].CreatedAt, page1[1].CreatedAt)
	}

	// Second page: cursor = last item's CreatedAt
	cursor := page1[1].CreatedAt
	page2, err := q.ListBatchOperations(ctx, job.ProjectID, 2, &cursor)
	if err != nil {
		t.Fatalf("ListBatchOperations() page2 error = %v", err)
	}
	if len(page2) != 1 {
		t.Fatalf("ListBatchOperations() page2 len = %d, want 1", len(page2))
	}
}

func TestListBatchOperations_CrossProject(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	jobA := mustCreateJob(t, ctx, q, "project-batch-a")
	jobB := mustCreateJob(t, ctx, q, "project-batch-b")

	opA := &domain.BatchOperation{
		ID:        newID(),
		ProjectID: jobA.ProjectID,
		JobID:     jobA.ID,
		ItemCount: 1,
		CreatedBy: "user-1",
	}
	if err := q.CreateBatchOperation(ctx, opA); err != nil {
		t.Fatalf("CreateBatchOperation() A error = %v", err)
	}

	opB := &domain.BatchOperation{
		ID:        newID(),
		ProjectID: jobB.ProjectID,
		JobID:     jobB.ID,
		ItemCount: 1,
		CreatedBy: "user-1",
	}
	if err := q.CreateBatchOperation(ctx, opB); err != nil {
		t.Fatalf("CreateBatchOperation() B error = %v", err)
	}

	ops, err := q.ListBatchOperations(ctx, jobA.ProjectID, 10, nil)
	if err != nil {
		t.Fatalf("ListBatchOperations() error = %v", err)
	}
	if len(ops) != 1 {
		t.Fatalf("ListBatchOperations() len = %d, want 1", len(ops))
	}
	if ops[0].ProjectID != jobA.ProjectID {
		t.Fatalf("ProjectID = %q, want %q", ops[0].ProjectID, jobA.ProjectID)
	}
}
