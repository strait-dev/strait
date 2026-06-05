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

func TestMaskOldDLQRows_MasksOnlyStale(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-dlq-age-" + newID()
	require.NoError(t, q.CreateProject(ctx, &domain.
		Project{ID: projectID,

		OrgID: "org", Name: "p",
	}))

	job := baseJob(newID(), projectID)
	require.NoError(t, q.CreateJob(ctx,
		job))

	// Insert 3 DLQ rows: one old, one borderline, one fresh.
	ages := []time.Duration{48 * time.Hour, 26 * time.Hour, 1 * time.Hour}
	var ids []string
	for _, age := range ages {
		id := newID()
		_, err := testDB.Pool.Exec(ctx, `
			INSERT INTO job_runs (id, job_id, project_id, status, attempt, triggered_by, created_at, finished_at)
			VALUES ($1, $2, $3, 'dead_letter', 1, 'manual', NOW(), NOW() - $4::interval)
		`, id, job.ID, projectID, age.String())
		require.NoError(t, err)

		ids = append(ids, id)
	}

	// Mask older than 24h.
	n, err := q.MaskOldDLQRows(ctx, 24*time.Hour, 100)
	require.NoError(t, err)
	assert.EqualValues(t, 2, n)

	// Verify masking is append-only: the two old rows get visibility events,
	// while the fat ledger rows stay untouched.
	for i, id := range ids {
		var ledgerVisible *time.Time
		require.NoError(t, testDB.
			Pool.QueryRow(ctx,
			`SELECT visible_until FROM job_runs WHERE id = $1`,

			id).Scan(&ledgerVisible))
		require.Nil(t, ledgerVisible)

		var events int
		require.NoError(t, testDB.
			Pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM job_run_visibility_events WHERE run_id = $1`,

			id).Scan(&events))

		if i < 2 {
			assert.EqualValues(t, 1, events)

		} else {
			assert.EqualValues(t, 0, events)

		}
	}
}

func TestMaskOldDLQRows_UsesSplitTerminalState(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "proj-dlq-age-split")
	run := baseRun(job, newID())
	run.Status = domain.StatusExecuting
	require.NoError(t, q.CreateRun(ctx,
		run))

	finishedAt := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Microsecond)
	require.NoError(t, q.UpdateRunStatus(ctx, run.
		ID, domain.
		StatusExecuting,

		domain.StatusDeadLetter,

		map[string]any{"finished_at": finishedAt}))

	var ledgerStatus domain.RunStatus
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT status FROM job_runs WHERE id = $1`,

		run.
			ID).Scan(&ledgerStatus))
	require.Equal(t, domain.
		StatusExecuting,
		ledgerStatus,
	)

	masked, err := q.MaskOldDLQRows(ctx, 24*time.Hour, 100)
	require.NoError(t, err)
	require.EqualValues(t, 1, masked)

	var ledgerVisibleUntil *time.Time
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT visible_until FROM job_runs WHERE id = $1`,

		run.ID).Scan(&ledgerVisibleUntil))
	require.Nil(t, ledgerVisibleUntil)

	var eventVisibleUntil *time.Time
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT visible_until
		FROM job_run_visibility_events
		WHERE run_id = $1
		ORDER BY id DESC
		LIMIT 1
	`,
		run.ID).Scan(&eventVisibleUntil))
	require.NotNil(t, eventVisibleUntil)

	depth, err := q.DLQDepth(ctx, job.ProjectID, job.ID)
	require.NoError(t, err)
	require.EqualValues(t, 0, depth)

}

func TestMaskOldDLQRows_Idempotent(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-dlq-idem-" + newID()
	_ = q.CreateProject(ctx, &domain.Project{ID: projectID, OrgID: "org", Name: "p"})
	job := baseJob(newID(), projectID)
	_ = q.CreateJob(ctx, job)

	_, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_runs (id, job_id, project_id, status, attempt, triggered_by, created_at, finished_at)
		VALUES ($1, $2, $3, 'dead_letter', 1, 'manual', NOW(), NOW() - INTERVAL '48 hours')
	`, newID(), job.ID, projectID)
	require.NoError(t, err)

	first, _ := q.MaskOldDLQRows(ctx, 24*time.Hour, 100)
	second, _ := q.MaskOldDLQRows(ctx, 24*time.Hour, 100)
	assert.False(t, first !=

		1 || second !=
		0)

}

func TestMaskOldDLQRows_DecrementsCounterViaTrigger(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-dlq-count-" + newID()
	_ = q.CreateProject(ctx, &domain.Project{ID: projectID, OrgID: "org", Name: "p"})
	job := baseJob(newID(), projectID)
	_ = q.CreateJob(ctx, job)

	_, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_runs (id, job_id, project_id, status, attempt, triggered_by, created_at, finished_at)
		VALUES ($1, $2, $3, 'dead_letter', 1, 'manual', NOW(), NOW() - INTERVAL '48 hours')
	`, newID(), job.ID, projectID)
	require.NoError(t, err)

	var before, after int
	_ = testDB.Pool.QueryRow(ctx, `SELECT COALESCE(count, 0) FROM dlq_counts WHERE job_id = $1`, job.ID).Scan(&before)
	require.EqualValues(t, 1, before)

	if _, err := q.MaskOldDLQRows(ctx, 24*time.Hour, 100); err != nil {
		require.Failf(t, "test failure",

			"mask: %v", err)
	}

	_ = testDB.Pool.QueryRow(ctx, `SELECT COALESCE(count, 0) FROM dlq_counts WHERE job_id = $1`, job.ID).Scan(&after)
	assert.EqualValues(t, 0, after)

}

func TestMaskOldDLQRows_RespectsLimit(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-dlq-limit-" + newID()
	_ = q.CreateProject(ctx, &domain.Project{ID: projectID, OrgID: "org", Name: "p"})
	job := baseJob(newID(), projectID)
	_ = q.CreateJob(ctx, job)

	for range 10 {
		_, _ = testDB.Pool.Exec(ctx, `
			INSERT INTO job_runs (id, job_id, project_id, status, attempt, triggered_by, created_at, finished_at)
			VALUES ($1, $2, $3, 'dead_letter', 1, 'manual', NOW(), NOW() - INTERVAL '48 hours')
		`, newID(), job.ID, projectID)
	}

	first, _ := q.MaskOldDLQRows(ctx, 24*time.Hour, 3)
	assert.EqualValues(t, 3, first)

	second, _ := q.MaskOldDLQRows(ctx, 24*time.Hour, 3)
	assert.EqualValues(t, 3, second)

}
