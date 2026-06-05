//go:build integration

package queue_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

// TestMigration_JobRunStateStorage_AppliesParams guards the hot mutable
// run-state table tuning used to keep claim, heartbeat, retry, and terminal
// updates from bloating the immutable job_runs ledger path.
func TestMigration_JobRunStateStorage_AppliesParams(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	var opts []string
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT reloptions
		FROM pg_class
		WHERE relname = 'job_run_state'
	`,
	).
		Scan(&opts))

	wantOptions := []string{
		"fillfactor=70",
		"autovacuum_vacuum_threshold=50",
		"autovacuum_vacuum_scale_factor=0.005",
		"autovacuum_vacuum_cost_delay=0",
		"autovacuum_vacuum_cost_limit=2000",
		"autovacuum_analyze_threshold=50",
		"autovacuum_analyze_scale_factor=0.005",
	}
	gotOptions := strings.Join(opts, ",")
	for _, want := range wantOptions {
		require.True(t, strings.Contains(
			gotOptions,
			want))

	}
}

func TestMigration_JobRunStateStorage_DropsStatusClaimIndexes(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	rows, err := testDB.Pool.Query(ctx, `
		SELECT indexname, indexdef
		FROM pg_indexes
		WHERE schemaname = 'public'
		  AND tablename = 'job_run_state'
		  AND indexname LIKE 'idx_job_run_state%claim%'
		  AND indexname <> 'idx_job_run_state_pgque_active_claim_counts'`)
	require.NoError(t, err)

	defer rows.Close()

	for rows.Next() {
		var name, def string
		require.NoError(t, rows.
			Scan(&name,
				&def))
		require.Failf(t, "test failure",

			"job_run_state still has claim index %s: %s", name, def)
	}
	require.NoError(t, rows.
		Err())

}

func TestMigration_JobRunActiveClaims_UsesMinimalPayload(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	rows, err := testDB.Pool.Query(ctx, `
		SELECT column_name
		FROM information_schema.columns
		WHERE table_schema = 'public'
		  AND table_name = 'job_run_active_claims'
		ORDER BY ordinal_position`)
	require.NoError(t, err)

	defer rows.Close()

	var got []string
	for rows.Next() {
		var column string
		require.NoError(t, rows.
			Scan(&column))

		got = append(got, column)
	}
	require.NoError(t, rows.
		Err())

	want := []string{"run_id", "ready_generation", "attempt", "started_at"}
	require.Equal(t, strings.Join(want,
		","), strings.Join(got, ","))

}

func TestJobActiveCounts_TracksJobRunStateTransitions(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-state-active-counts")
	q := mustQueue(t)
	run := mustEnqueueRun(t, ctx, q, job)
	if _, err := testDB.Pool.Exec(ctx,
		`UPDATE job_run_state SET job_max_concurrency = 1 WHERE run_id = $1`,
		run.ID,
	); err != nil {
		require.Failf(t, "test failure",

			"enable active count tracking: %v", err)
	}

	setStateStatus := func(status domain.RunStatus) {
		t.Helper()
		if _, err := testDB.Pool.Exec(ctx,
			`UPDATE job_run_state SET status = $1, updated_at = NOW() WHERE run_id = $2`,
			status,
			run.ID,
		); err != nil {
			require.Failf(t, "test failure",

				"update job_run_state status %s: %v", status, err)
		}
	}
	assertCount := func(want int) {
		t.Helper()
		var got int
		require.NoError(t, testDB.
			Pool.QueryRow(ctx,
			`SELECT COALESCE(SUM(count), 0) FROM job_active_counts WHERE job_id = $1`,

			job.
				ID).Scan(&got))
		require.Equal(t, want,
			got,
		)

	}

	assertCount(0)
	setStateStatus(domain.StatusDequeued)
	assertCount(1)

	var activeUpdatedAt time.Time
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT updated_at FROM job_active_counts WHERE job_id = $1 AND concurrency_key = ''`,

		job.ID).Scan(&activeUpdatedAt))

	setStateStatus(domain.StatusExecuting)
	assertCount(1)

	var afterExecutingUpdatedAt time.Time
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT updated_at FROM job_active_counts WHERE job_id = $1 AND concurrency_key = ''`,

		job.ID).Scan(&afterExecutingUpdatedAt))
	require.True(t, afterExecutingUpdatedAt.
		Equal(activeUpdatedAt))

	setStateStatus(domain.StatusCompleted)
	assertCount(0)
}

func TestJobActiveCounts_SkipsUnconstrainedRuns(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-state-active-counts-unconstrained")
	q := mustQueue(t)
	run := mustEnqueueRun(t, ctx, q, job)

	for _, status := range []domain.RunStatus{domain.StatusDequeued, domain.StatusExecuting, domain.StatusCompleted} {
		if _, err := testDB.Pool.Exec(ctx,
			`UPDATE job_run_state SET status = $1, updated_at = NOW() WHERE run_id = $2`,
			status,
			run.ID,
		); err != nil {
			require.Failf(t, "test failure",

				"update job_run_state status %s: %v", status, err)
		}
	}

	var countRows int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM job_active_counts WHERE job_id = $1`,

		job.ID).Scan(
		&countRows,
	))
	require.EqualValues(t, 0, countRows)

}

func TestJobRunState_TerminalTransitionAppendsColdStateOverlay(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-terminal-state-cold-storage")
	q := mustQueue(t)
	run := mustEnqueueRun(t, ctx, q, job)
	require.NoError(t, st.UpdateRunStatus(ctx, run.
		ID, domain.StatusQueued,

		domain.StatusDequeued,

		nil))
	require.NoError(t, st.UpdateRunStatus(ctx, run.
		ID, domain.StatusDequeued,

		domain.StatusExecuting,

		nil))

	finishedAt := time.Now().UTC()
	result := json.RawMessage(`{"ok":true}`)
	require.NoError(t, st.UpdateRunStatus(ctx, run.
		ID, domain.StatusExecuting,

		domain.StatusCompleted,

		map[string]any{"finished_at": finishedAt, "result": result}))

	var hotStatus domain.RunStatus
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT status FROM job_run_state WHERE run_id = $1`,

		run.ID).Scan(&hotStatus))
	require.Equal(t, domain.
		StatusExecuting,
		hotStatus,
	)

	var coldStatus domain.RunStatus
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT status FROM job_run_terminal_state WHERE run_id = $1`,

		run.ID).Scan(
		&coldStatus))
	require.Equal(t, domain.
		StatusCompleted,
		coldStatus,
	)

	var readStatus domain.RunStatus
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT status FROM job_run_read_state WHERE run_id = $1`,

		run.ID).Scan(&readStatus))
	require.Equal(t, domain.
		StatusCompleted,
		readStatus,
	)

	got, err := st.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		StatusCompleted,
		got.
			Status)

	var gotResult, wantResult map[string]bool
	require.NoError(t, json.
		Unmarshal(got.Result,
			&gotResult))
	require.NoError(t, json.
		Unmarshal(result, &wantResult))
	require.Equal(t, wantResult["ok"],
		gotResult["ok"])
	require.NotNil(t, got.FinishedAt)

}
