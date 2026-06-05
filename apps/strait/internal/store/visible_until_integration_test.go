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

// Integration tests for soft-delete retention via append-only visibility events.
// These tests complement the existing TestDeleteRunsByOrgOlderThan which
// already asserts RowsAffected — they additionally verify that the rows
// physically remain in the table (masked, not deleted).

func TestMaskRunsByOrgOlderThan_RowsPhysicallyRemain(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-mask-phys-" + newID()
	projectID := "proj-mask-phys-" + newID()
	require.NoError(t, q.CreateProject(ctx, &domain.
		Project{ID: projectID,

		OrgID: orgID, Name: "P",
	}))

	job := baseJob(newID(), projectID)
	require.NoError(t, q.CreateJob(ctx,
		job))

	run := baseRun(job, newID())
	run.Status = domain.StatusCompleted
	require.NoError(t, q.CreateRun(ctx,
		run))

	past := time.Now().UTC().Add(-48 * time.Hour)
	if _, err := testDB.Pool.Exec(ctx,
		"UPDATE job_runs SET status='completed', finished_at=$1 WHERE id=$2", past, run.ID); err != nil {
		require.Failf(t, "test failure",

			"update finished_at: %v", err)
	}

	masked, err := q.DeleteRunsByOrgOlderThan(ctx, orgID, 24*time.Hour)
	require.NoError(t, err)
	require.EqualValues(t, 1, masked)

	// Physically still there.
	var count int
	err = testDB.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM job_runs WHERE id = $1", run.ID).Scan(&count)
	require.NoError(t, err)
	assert.EqualValues(t, 1, count)

	var ledgerVisibleUntil *time.Time
	err = testDB.Pool.QueryRow(ctx, "SELECT visible_until FROM job_runs WHERE id = $1", run.ID).Scan(&ledgerVisibleUntil)
	require.NoError(t, err)
	require.Nil(t, ledgerVisibleUntil)

	var eventVisibleUntil *time.Time
	err = testDB.Pool.QueryRow(ctx, `
		SELECT visible_until
		FROM job_run_visibility_events
		WHERE run_id = $1
		ORDER BY id DESC
		LIMIT 1
	`, run.ID).Scan(&eventVisibleUntil)
	require.NoError(t, err)
	assert.NotNil(t, eventVisibleUntil)

}

func TestMaskRunsByOrgOlderThan_IdempotentWithinSameCutoff(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-mask-idem-" + newID()
	projectID := "proj-mask-idem-" + newID()
	require.NoError(t, q.CreateProject(ctx, &domain.
		Project{ID: projectID,

		OrgID: orgID, Name: "P",
	}))

	job := baseJob(newID(), projectID)
	require.NoError(t, q.CreateJob(ctx,
		job))

	run := baseRun(job, newID())
	run.Status = domain.StatusCompleted
	require.NoError(t, q.CreateRun(ctx,
		run))

	past := time.Now().UTC().Add(-48 * time.Hour)
	_, _ = testDB.Pool.Exec(ctx,
		"UPDATE job_runs SET status='completed', finished_at=$1 WHERE id=$2", past, run.ID)

	first, err := q.DeleteRunsByOrgOlderThan(ctx, orgID, 24*time.Hour)
	require.NoError(t, err)
	assert.EqualValues(t, 1, first)

	// Second call should see the visibility event and skip the row.
	second, err := q.DeleteRunsByOrgOlderThan(ctx, orgID, 24*time.Hour)
	require.NoError(t, err)
	assert.EqualValues(t, 0, second)

}

func TestMaskRunsByOrgOlderThan_DoesNotUpdateLedger(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-mask-hot-" + newID()
	projectID := "proj-mask-hot-" + newID()
	require.NoError(t, q.CreateProject(ctx, &domain.
		Project{ID: projectID,

		OrgID: orgID, Name: "P",
	}))

	job := baseJob(newID(), projectID)
	require.NoError(t, q.CreateJob(ctx,
		job))

	// Enqueue a batch and mark them all as old terminal.
	for range 50 {
		r := baseRun(job, newID())
		r.Status = domain.StatusCompleted
		require.NoError(t, q.CreateRun(ctx,
			r))

	}
	past := time.Now().UTC().Add(-48 * time.Hour)
	_, _ = testDB.Pool.Exec(ctx,
		"UPDATE job_runs SET status='completed', finished_at=$1 WHERE project_id=$2", past, projectID)

	_, err := q.DeleteRunsByOrgOlderThan(ctx, orgID, 24*time.Hour)
	require.NoError(t, err)

	var ledgerMasks int
	err = testDB.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM job_runs
		WHERE project_id = $1
		  AND visible_until IS NOT NULL
	`, projectID).Scan(&ledgerMasks)
	require.NoError(t, err)
	require.EqualValues(t, 0, ledgerMasks)

	var events int
	err = testDB.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM job_run_visibility_events
		WHERE run_id IN (SELECT id FROM job_runs WHERE project_id = $1)
	`, projectID).Scan(&events)
	require.NoError(t, err)
	require.EqualValues(t, 50, events)

}
