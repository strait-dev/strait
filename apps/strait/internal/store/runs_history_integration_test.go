//go:build integration

package store_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_ArchiveTerminalRunRoundTrip(t *testing.T) {
	ctx := context.Background()
	q := store.New(testDB.Pool)

	projectID := "proj-archive-rt-" + t.Name()
	job := baseJob("job-archive-rt", projectID)
	require.NoError(t, q.CreateJob(ctx,
		job))

	run := baseRun(job, "run-archive-rt")
	run.Status = domain.StatusCompleted
	now := time.Now().UTC()
	run.FinishedAt = &now
	require.NoError(t, q.CreateRun(ctx,
		run))

	seedRetentionSideRows(t, ctx, run.ID)

	tx, err := testDB.Pool.Begin(ctx)
	require.NoError(t, err)

	defer tx.Rollback(ctx)
	require.NoError(t, q.ArchiveTerminalRun(ctx,
		tx, run.ID,
	))
	require.NoError(t, tx.Commit(ctx))

	//nolint:errcheck

	// Verify the run is gone from the hot table by querying directly.
	// GetRun has a transparent history fallback, so we bypass it here.
	var hotCount int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM job_runs WHERE id = $1`,

		run.ID).Scan(&hotCount))
	assert.EqualValues(t, 0, hotCount)

	// GetRun should still find the run via history fallback.
	gotViaFallback, err := q.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.NotNil(t, gotViaFallback)

	gotHistory, err := q.GetRunFromHistory(ctx, run.ID)
	require.NoError(t, err)
	require.NotNil(t, gotHistory)
	assert.Equal(t, run.ID,

		gotHistory.
			ID)
	assert.Equal(t, domain.
		StatusCompleted,

		gotHistory.
			Status,
	)

	assertNoRunRetentionSideRows(t, ctx, run.ID)
}

func TestIntegration_ArchiveIdempotent(t *testing.T) {
	ctx := context.Background()
	q := store.New(testDB.Pool)

	projectID := "proj-archive-idem-" + t.Name()
	job := baseJob("job-archive-idem", projectID)
	require.NoError(t, q.CreateJob(ctx,
		job))

	run := baseRun(job, "run-archive-idem")
	run.Status = domain.StatusCompleted
	now := time.Now().UTC()
	run.FinishedAt = &now
	require.NoError(t, q.CreateRun(ctx,
		run))

	tx1, err := testDB.Pool.Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, q.ArchiveTerminalRun(ctx,
		tx1, run.ID,
	))
	require.NoError(t, tx1.
		Commit(ctx))

	tx2, err := testDB.Pool.Begin(ctx)
	require.NoError(t, err)

	defer tx2.Rollback(ctx)
	require.NoError(t, q.ArchiveTerminalRun(ctx,
		tx2, run.ID,
	))
	require.NoError(t, tx2.
		Commit(ctx))

	//nolint:errcheck

}

func TestIntegration_ArchiveTerminalRunsBatchAndRetention(t *testing.T) {
	ctx := context.Background()
	q := store.New(testDB.Pool)

	projectID := "proj-archive-batch-" + t.Name()
	job := baseJob("job-archive-batch", projectID)
	require.NoError(t, q.CreateJob(ctx,
		job))

	past := time.Now().UTC().Add(-45 * 24 * time.Hour) // must be in a prior month for cold-partition filter
	for i := range 5 {
		run := baseRun(job, "run-archive-batch-"+string(rune('a'+i)))
		run.Status = domain.StatusCompleted
		finished := past.Add(time.Duration(i) * time.Minute)
		run.FinishedAt = &finished
		require.NoError(t, q.CreateRun(ctx,
			run))

		seedRetentionSideRows(t, ctx, run.ID)
	}
	// Backdate created_at so runs are in cold partitions (reaper's
	// hot-partition filter skips the current month).
	for i := range 5 {
		id := "run-archive-batch-" + string(rune('a'+i))
		createdAt := past.Add(time.Duration(i) * time.Minute)
		if _, err := testDB.Pool.Exec(ctx, `UPDATE job_runs SET created_at = $1 WHERE id = $2`, createdAt, id); err != nil {
			require.Failf(t, "test failure",

				"backdate run %d: %v", i, err)
		}
	}

	archived, err := q.ArchiveTerminalRunsPastRetention(ctx, time.Hour, time.Hour, 10)
	require.NoError(t, err)
	assert.EqualValues(t, 5, archived)

	for i := range 5 {
		id := "run-archive-batch-" + string(rune('a'+i))
		assertNoRunRetentionSideRows(t, ctx, id)
	}

	deleted, err := q.DeleteHistoryRunsPastRetention(ctx, time.Now().Add(time.Hour), 100)
	require.NoError(t, err)
	assert.GreaterOrEqual(t,

		deleted,
		int64(5))

}

func TestIntegration_HistoryTableColumnSync(t *testing.T) {
	ctx := context.Background()

	type colInfo struct {
		Name string
		Type string
	}

	fetchColumns := func(table string) (map[string]colInfo, error) {
		rows, err := testDB.Pool.Query(ctx, `
			SELECT column_name, data_type
			FROM information_schema.columns
			WHERE table_name = $1
			ORDER BY ordinal_position`, table)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		cols := make(map[string]colInfo)
		for rows.Next() {
			var c colInfo
			if err := rows.Scan(&c.Name, &c.Type); err != nil {
				return nil, err
			}
			cols[c.Name] = c
		}
		return cols, rows.Err()
	}

	hotCols, err := fetchColumns("job_runs")
	require.NoError(t, err)

	historyCols, err := fetchColumns("job_runs_history")
	require.NoError(t, err)

	for name, hot := range hotCols {
		hist, ok := historyCols[name]
		if !ok {
			assert.Failf(t, "test failure",

				"column %q exists in job_runs but not in job_runs_history", name)
			continue
		}
		assert.Equal(t, hist.Type,

			hot.Type,
		)

	}

	allowed := map[string]bool{"archived_at": true}
	for name := range historyCols {
		if _, ok := hotCols[name]; !ok && !allowed[name] {
			assert.Failf(t, "test failure",

				"column %q exists in job_runs_history but not in job_runs (and not in allowed set)", name)
		}
	}
}

func TestIntegration_HistoryArchiveColumnsMatchSchema(t *testing.T) {
	ctx := context.Background()

	rows, err := testDB.Pool.Query(ctx, `
		SELECT column_name
		FROM information_schema.columns
		WHERE table_name = 'job_runs_history'
		ORDER BY ordinal_position`)
	require.NoError(t, err)

	defer rows.Close()

	schemaCols := make(map[string]bool)
	for rows.Next() {
		var name string
		require.NoError(t, rows.
			Scan(&name))

		schemaCols[name] = true
	}
	require.NoError(t, rows.
		Err())

	archiveCols := make(map[string]bool)
	for raw := range strings.SplitSeq(store.HistoryArchiveColumnsForTest, ",") {
		col := strings.TrimSpace(raw)
		if col != "" {
			archiveCols[col] = true
		}
	}

	for col := range schemaCols {
		if col == "archived_at" {
			continue
		}
		assert.True(t, archiveCols[col])

	}

	for col := range archiveCols {
		assert.True(t, schemaCols[col])

	}
}

func TestIntegration_BackfillTerminalRunsToHistory(t *testing.T) {
	ctx := context.Background()
	q := store.New(testDB.Pool)
	require.NoError(t, testDB.
		CleanTables(ctx))

	projectID := "proj-backfill-" + t.Name()
	job := baseJob("job-backfill", projectID)
	require.NoError(t, q.CreateJob(ctx,
		job))

	past := time.Now().UTC().Add(-72 * time.Hour)
	for i := range 3 {
		run := baseRun(job, "run-backfill-"+string(rune('a'+i)))
		run.Status = domain.StatusFailed
		finished := past.Add(time.Duration(i) * time.Hour)
		run.FinishedAt = &finished
		require.NoError(t, q.CreateRun(ctx,
			run))

		seedRetentionSideRows(t, ctx, run.ID)
	}

	activeRun := baseRun(job, "run-backfill-active")
	activeRun.Status = domain.StatusQueued
	require.NoError(t, q.CreateRun(ctx,
		activeRun,
	))

	moved, err := q.BackfillTerminalRunsToHistory(ctx, time.Now(), 100)
	require.NoError(t, err)
	assert.EqualValues(t, 3, moved)

	active, err := q.GetRun(ctx, activeRun.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.
		StatusQueued,

		active.
			Status)

	for i := range 3 {
		id := "run-backfill-" + string(rune('a'+i))
		assertNoRunRetentionSideRows(t, ctx, id)
	}
}

func TestIntegration_RepairOrphanedHistoryRuns(t *testing.T) {
	ctx := context.Background()
	q := store.New(testDB.Pool)

	projectID := "proj-repair-" + t.Name()
	job := baseJob("job-repair", projectID)
	require.NoError(t, q.CreateJob(ctx,
		job))

	run := baseRun(job, "run-repair-dupe")
	run.Status = domain.StatusCompleted
	now := time.Now().UTC()
	run.FinishedAt = &now
	require.NoError(t, q.CreateRun(ctx,
		run))

	seedRetentionSideRows(t, ctx, run.ID)

	// Manually copy the run into history to create a duplicate.
	_, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_runs_history (
			id, job_id, project_id, status, attempt, payload, result, metadata,
			error, error_class, triggered_by, scheduled_at, started_at, finished_at,
			heartbeat_at, next_retry_at, expires_at, parent_run_id, priority,
			idempotency_key, job_version, workflow_step_run_id, execution_trace,
			debug_mode, continuation_of, lineage_depth, tags, job_version_id,
			created_by, concurrency_key, batch_id, execution_mode, is_rollback,
			replayed_run_id, max_attempts_override, timeout_secs_override,
			retry_backoff, retry_initial_delay_secs, retry_max_delay_secs,
			visible_until, job_enabled, job_paused, job_max_concurrency, job_max_concurrency_per_key,
			created_at
		)
		SELECT
			id, job_id, project_id, status, attempt, payload, result, metadata,
			error, error_class, triggered_by, scheduled_at, started_at, finished_at,
			heartbeat_at, next_retry_at, expires_at, parent_run_id, priority,
			idempotency_key, job_version, workflow_step_run_id, execution_trace,
			debug_mode, continuation_of, lineage_depth, tags, job_version_id,
			created_by, concurrency_key, batch_id, execution_mode, is_rollback,
			replayed_run_id, max_attempts_override, timeout_secs_override,
			retry_backoff, retry_initial_delay_secs, retry_max_delay_secs,
			visible_until, job_enabled, job_paused, job_max_concurrency, job_max_concurrency_per_key,
			created_at
		FROM job_runs WHERE id = $1`, run.ID)
	require.NoError(t, err)

	repaired, err := q.RepairOrphanedHistoryRuns(ctx, 10)
	require.NoError(t, err)
	assert.EqualValues(t, 1, repaired)

	dupes, err := q.CountDuplicateHistoryRuns(ctx)
	require.NoError(t, err)
	assert.EqualValues(t, 0, dupes)

	assertNoRunRetentionSideRows(t, ctx, run.ID)
}
