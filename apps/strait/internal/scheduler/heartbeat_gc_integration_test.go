//go:build integration

package scheduler_test

import (
	"context"
	"testing"

	"strait/internal/domain"
	"strait/internal/scheduler"
	"strait/internal/store"
	"strait/internal/testutil"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupHeartbeatGC(t *testing.T) (*testutil.TestDB, *store.Queries) {
	t.Helper()
	ctx := context.Background()
	tdb := getTestDB(t)
	intTestClean(t, ctx)
	return tdb, store.New(tdb.Pool)
}

func setupHeartbeatGCIsolated(t *testing.T) (*testutil.TestDB, *store.Queries) {
	t.Helper()
	ctx := context.Background()
	tdb, err := testutil.SetupTestDB(ctx, "../../migrations")
	require.NoError(t, err)

	t.Cleanup(func() { tdb.Cleanup(context.Background()) })
	return tdb, store.New(tdb.Pool)
}

func hbGCMakeJob(t *testing.T, st *store.Queries, ctx context.Context, projectID string) *domain.Job {
	t.Helper()
	job := &domain.Job{
		ID:          uuid.Must(uuid.NewV7()).String(),
		ProjectID:   projectID,
		Name:        "gc-job",
		Slug:        "gc-" + uuid.Must(uuid.NewV7()).String()[:8],
		EndpointURL: "https://example.com/x",
		MaxAttempts: 3,
		TimeoutSecs: 60,
		Enabled:     true,
	}
	require.NoError(t, st.CreateJob(ctx,
		job))

	return job
}

func TestHeartbeatGC_DeletesOrphansPreservesLive(t *testing.T) {
	tdb, st := setupHeartbeatGC(t)
	ctx := context.Background()

	// Create a job and two runs.
	projectID := "gc-" + uuid.Must(uuid.NewV7()).String()
	var ready int
	require.NoError(t, tdb.Pool.
		QueryRow(
			ctx, `SELECT 1`,
		).Scan(
		&ready))

	job := hbGCMakeJob(t, st, ctx, projectID)

	liveID := uuid.Must(uuid.NewV7()).String()
	orphanID := uuid.Must(uuid.NewV7()).String()

	// Insert two runs, one executing (live heartbeat) and one completed (orphan).
	for _, id := range []string{liveID, orphanID} {
		_, err := tdb.Pool.Exec(ctx, `
			INSERT INTO job_runs (id, job_id, project_id, status, attempt, triggered_by, created_at, started_at)
			VALUES ($1, $2, $3, 'executing', 1, 'manual', NOW(), NOW())
		`, id, job.ID, job.ProjectID)
		require.NoError(t, err)

	}
	require.NoError(t, st.BatchUpsertHeartbeatSideTable(ctx, []string{liveID,

		orphanID}))

	// Register both heartbeats.

	// Transition the orphan to completed.
	_, err := tdb.Pool.Exec(ctx, `UPDATE job_runs SET status='completed', finished_at=NOW() WHERE id=$1`, orphanID)
	require.NoError(t, err)

	// Run GC.
	gc := scheduler.NewHeartbeatGC(st, scheduler.HeartbeatGCConfig{})
	require.NoError(t, gc.RunOnceForTest(
		ctx))
	assert.EqualValues(t, 2, gc.TotalDeleted())

	// live row still present.
	var count int
	_ = tdb.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM (
			SELECT cleared
			FROM job_run_heartbeats
			WHERE run_id = $1
			ORDER BY id DESC
			LIMIT 1
		) latest
		WHERE cleared = FALSE`, liveID).Scan(&count)
	assert.EqualValues(t, 1, count)

	// orphan is logically cleared.
	_ = tdb.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM (
			SELECT cleared
			FROM job_run_heartbeats
			WHERE run_id = $1
			ORDER BY id DESC
			LIMIT 1
		) latest
		WHERE cleared = FALSE`, orphanID).Scan(&count)
	assert.EqualValues(t, 0, count)

}

func TestHeartbeatGC_BatchLimitRespected(t *testing.T) {
	tdb, st := setupHeartbeatGC(t)
	ctx := context.Background()

	projectID := "gc-batch-" + uuid.Must(uuid.NewV7()).String()
	job := hbGCMakeJob(t, st, ctx, projectID)
	// Create 10 orphan heartbeats.
	var ids []string
	for range 10 {
		id := uuid.Must(uuid.NewV7()).String()
		_, err := tdb.Pool.Exec(ctx, `
			INSERT INTO job_runs (id, job_id, project_id, status, attempt, triggered_by, created_at, finished_at)
			VALUES ($1, $2, $3, 'completed', 1, 'manual', NOW(), NOW())
		`, id, job.ID, job.ProjectID)
		require.NoError(t, err)

		ids = append(ids, id)
	}
	require.NoError(t, st.BatchUpsertHeartbeatSideTable(ctx, ids))

	gc := scheduler.NewHeartbeatGC(st, scheduler.HeartbeatGCConfig{BatchLimit: 5})
	_ = gc.RunOnceForTest(ctx)
	assert.EqualValues(t, 10, gc.TotalDeleted())

	_ = gc.RunOnceForTest(ctx)
	assert.EqualValues(t, 20, gc.TotalDeleted())

}

func TestEnsureQueueTriggersPresent_Happy(t *testing.T) {
	tdb, _ := setupHeartbeatGCIsolated(t)
	assert.NoError(t, scheduler.
		EnsureQueueTriggersPresent(context.
			Background(), tdb.Pool))

}

func TestEnsureQueueTriggersPresent_MissingFailsLoud(t *testing.T) {
	tdb, _ := setupHeartbeatGCIsolated(t)
	ctx := context.Background()
	// Drop one trigger and assert the check fails.
	_, err := tdb.Pool.Exec(ctx, `DROP TRIGGER IF EXISTS trg_job_runs_queue_wake_insert_notify ON job_runs`)
	require.NoError(t, err)

	err = scheduler.EnsureQueueTriggersPresent(ctx, tdb.Pool)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "trg_job_runs_queue_wake_insert_notify")

}

func TestEnsureQueueTriggersPresent_DisabledFailsLoud(t *testing.T) {
	tdb, _ := setupHeartbeatGCIsolated(t)
	ctx := context.Background()
	_, err := tdb.Pool.Exec(ctx, `ALTER TABLE job_run_state DISABLE TRIGGER job_run_state_active_counts_trg`)
	require.NoError(t, err)

	err = scheduler.EnsureQueueTriggersPresent(ctx, tdb.Pool)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "job_run_state_active_counts_trg")

}

func TestEnsureQueueTriggersPresent_DecoyTriggerDoesNotPass(t *testing.T) {
	tdb, _ := setupHeartbeatGCIsolated(t)
	ctx := context.Background()
	_, err := tdb.Pool.Exec(ctx, `
		DROP TRIGGER IF EXISTS trg_job_runs_queue_wake_insert_notify ON job_runs;
		CREATE TEMP TABLE job_runs_trigger_decoy (LIKE job_runs INCLUDING ALL);
		CREATE TRIGGER trg_job_runs_queue_wake_insert_notify
		AFTER INSERT ON job_runs_trigger_decoy
		REFERENCING NEW TABLE AS new_rows
		FOR EACH STATEMENT
		EXECUTE FUNCTION notify_queue_wake_insert_stmt();
	`)
	require.NoError(t, err)

	err = scheduler.EnsureQueueTriggersPresent(ctx, tdb.Pool)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "trg_job_runs_queue_wake_insert_notify")

}
