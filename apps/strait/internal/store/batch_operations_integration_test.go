//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, q.CreateBatchOperation(ctx,
		op))

	got, err := q.GetBatchOperation(ctx, op.ID, op.ProjectID)
	require.NoError(t, err)
	require.Equal(t, op.ID,

		got.ID)
	require.Equal(t, op.ProjectID,

		got.
			ProjectID,
	)
	require.Equal(t, op.JobID,

		got.JobID,
	)
	require.EqualValues(t, 10, got.
		ItemCount,
	)
	require.EqualValues(t, 0, got.
		CreatedCount,
	)
	require.False(t, got.CreatedAt.
		IsZero())
	require.Nil(t, got.
		FinishedAt,
	)

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
	require.NoError(t, q.CreateBatchOperation(ctx,
		op))
	require.NoError(t, q.FinalizeBatchOperation(
		ctx, op.ID,
		3))

	got, err := q.GetBatchOperation(ctx, op.ID, op.ProjectID)
	require.NoError(t, err)
	require.EqualValues(t, 3, got.
		CreatedCount,
	)
	require.NotNil(t, got.FinishedAt)

}

func TestGetBatchOperation_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, err := q.GetBatchOperation(ctx, newID(), "nonexistent-project")
	require.Error(t, err)
	require.True(t, errors.Is(err, pgx.
		ErrNoRows,
	))

}

func TestListBatchOperations(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-batch-list")

	for i := range 3 {
		op := &domain.BatchOperation{
			ID:        newID(),
			ProjectID: job.ProjectID,
			JobID:     job.ID,
			ItemCount: i + 1,
			CreatedBy: "user-1",
		}
		require.NoError(t, q.CreateBatchOperation(ctx,
			op))

		time.Sleep(time.Millisecond)
	}

	// First page: limit 2
	page1, err := q.ListBatchOperations(ctx, job.ProjectID, 2, nil)
	require.NoError(t, err)
	require.Len(t, page1, 2)
	require.True(t, page1[0].
		CreatedAt.
		After(page1[1].CreatedAt))

	// Second page: cursor = last item's CreatedAt
	cursor := page1[1].CreatedAt
	page2, err := q.ListBatchOperations(ctx, job.ProjectID, 2, &cursor)
	require.NoError(t, err)
	require.Len(t, page2, 1)

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
	require.NoError(t, q.CreateBatchOperation(ctx,
		opA))

	opB := &domain.BatchOperation{
		ID:        newID(),
		ProjectID: jobB.ProjectID,
		JobID:     jobB.ID,
		ItemCount: 1,
		CreatedBy: "user-1",
	}
	require.NoError(t, q.CreateBatchOperation(ctx,
		opB))

	ops, err := q.ListBatchOperations(ctx, jobA.ProjectID, 10, nil)
	require.NoError(t, err)
	require.Len(t, ops, 1)
	require.Equal(t, jobA.ProjectID,

		ops[0].ProjectID,
	)

}
