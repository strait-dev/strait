//go:build integration

package store_test

import (
	"context"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Integration tests for the dlq_counts trigger + store helpers.

func TestDLQCounts_TriggerMaintainsCounter(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-dlq-trg-" + newID()
	require.NoError(t, q.CreateProject(ctx, &domain.
		Project{ID: projectID,

		OrgID: "org-x", Name: "P"}))

	job := baseJob(newID(), projectID)
	require.NoError(t, q.CreateJob(ctx,
		job))

	// Transition 3 runs straight to dead_letter.
	var ids []string
	for range 3 {
		r := baseRun(job, newID())
		r.Status = domain.StatusDeadLetter
		require.NoError(t, q.CreateRun(ctx,
			r))

		ids = append(ids, r.ID)
	}

	depth, err := q.DLQDepth(ctx, projectID, job.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 3, depth)

	// Soft-delete one row; counter should drop.
	_, err = testDB.Pool.Exec(ctx, `UPDATE job_runs SET visible_until = NOW() WHERE id = $1`, ids[0])
	require.NoError(t, err)

	depth, _ = q.DLQDepth(ctx, projectID, job.ID)
	assert.EqualValues(t, 2, depth)

	// Project-level aggregate.
	pd, err := q.DLQDepthByProject(ctx, projectID)
	require.NoError(t, err)
	assert.EqualValues(t, 2, pd)

}

func TestDLQCounts_SplitStateTransitionsMaintainCounter(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "proj-dlq-split-counter")
	run := baseRun(job, newID())
	run.Status = domain.StatusExecuting
	require.NoError(t, q.CreateRun(ctx,
		run))
	require.NoError(t, q.UpdateRunStatus(ctx, run.
		ID, domain.
		StatusExecuting,

		domain.StatusDeadLetter,

		map[string]any{"finished_at": time.Now().UTC().Truncate(time.
			Microsecond)}))

	var ledgerStatus, readStatus domain.RunStatus
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT jr.status, s.status
		FROM job_runs jr
		JOIN job_run_read_state s ON s.run_id = jr.id
		WHERE jr.id = $1`,

		run.ID).Scan(&ledgerStatus, &readStatus))
	require.Equal(t, domain.
		StatusExecuting,
		ledgerStatus,
	)
	require.Equal(t, domain.
		StatusDeadLetter,
		readStatus,
	)

	depth, err := q.DLQDepth(ctx, job.ProjectID, job.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, depth)

	replayed, err := q.ReplayDeadLetterRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		StatusQueued,
		replayed.
			Status)

	depth, err = q.DLQDepth(ctx, job.ProjectID, job.ID)
	require.NoError(t, err)
	require.EqualValues(t, 0, depth)

}

func TestMaskOldestDLQRow_PicksOldest(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-dlq-oldest-" + newID()
	require.NoError(t, q.CreateProject(ctx, &domain.
		Project{ID: projectID,

		OrgID: "org-x", Name: "P"}))

	job := baseJob(newID(), projectID)
	require.NoError(t, q.CreateJob(ctx,
		job))

	// Three DLQ rows with distinct finished_at timestamps.
	base := time.Now().UTC().Add(-1 * time.Hour)
	var oldest, middle, newest domain.JobRun
	for i, run := range []*domain.JobRun{&oldest, &middle, &newest} {
		run.ID = newID()
		run.JobID = job.ID
		run.ProjectID = projectID
		run.Status = domain.StatusDeadLetter
		run.TriggeredBy = domain.TriggerManual
		run.Attempt = 1
		require.NoError(t, q.CreateRun(ctx,
			run))

		_, err := testDB.Pool.Exec(ctx,
			`UPDATE job_runs SET finished_at = $1 WHERE id = $2`,
			base.Add(time.Duration(i)*time.Minute),
			run.ID,
		)
		require.NoError(t, err)

	}

	got, err := q.MaskOldestDLQRow(ctx, projectID, job.ID)
	require.NoError(t, err)
	assert.Equal(t, oldest.
		ID,
		got)

	// Counter should drop by one.
	depth, _ := q.DLQDepth(ctx, projectID, job.ID)
	assert.EqualValues(t, 2, depth)

	// Calling again picks the next oldest.
	got2, _ := q.MaskOldestDLQRow(ctx, projectID, job.ID)
	assert.Equal(t, middle.
		ID,
		got2)

}

func TestDLQDepth_MissingRowReturnsZero(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	depth, err := q.DLQDepth(ctx, "no-project", "no-job")
	require.NoError(t, err)
	assert.EqualValues(t, 0, depth)

}
