//go:build integration

package queue_test

import (
	"context"
	"strings"
	"testing"

	"strait/internal/domain"
)

// TestMigration_JobRunStateStorage_AppliesParams guards the hot mutable
// run-state table tuning used to keep claim, heartbeat, retry, and terminal
// updates from bloating the immutable job_runs ledger path.
func TestMigration_JobRunStateStorage_AppliesParams(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	var opts []string
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT reloptions
		FROM pg_class
		WHERE relname = 'job_run_state'
	`).Scan(&opts); err != nil {
		t.Fatalf("query job_run_state reloptions: %v", err)
	}

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
		if !strings.Contains(gotOptions, want) {
			t.Fatalf("job_run_state reloptions missing %q; got %v", want, opts)
		}
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
		  AND indexname LIKE 'idx_job_run_state%claim%'`)
	if err != nil {
		t.Fatalf("query job_run_state indexes: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var name, def string
		if err := rows.Scan(&name, &def); err != nil {
			t.Fatalf("scan job_run_state index: %v", err)
		}
		t.Fatalf("job_run_state still has claim index %s: %s", name, def)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("job_run_state index rows: %v", err)
	}
}

func TestJobActiveCounts_TracksJobRunStateTransitions(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-state-active-counts")
	q := mustQueue(t)
	run := mustEnqueueRun(t, ctx, q, job)

	setStateStatus := func(status domain.RunStatus) {
		t.Helper()
		if _, err := testDB.Pool.Exec(ctx,
			`UPDATE job_run_state SET status = $1, updated_at = NOW() WHERE run_id = $2`,
			status,
			run.ID,
		); err != nil {
			t.Fatalf("update job_run_state status %s: %v", status, err)
		}
	}
	assertCount := func(want int) {
		t.Helper()
		var got int
		if err := testDB.Pool.QueryRow(ctx,
			`SELECT COALESCE(SUM(count), 0) FROM job_active_counts WHERE job_id = $1`,
			job.ID,
		).Scan(&got); err != nil {
			t.Fatalf("query active count: %v", err)
		}
		if got != want {
			t.Fatalf("active count = %d, want %d", got, want)
		}
	}

	assertCount(0)
	setStateStatus(domain.StatusDequeued)
	assertCount(1)
	setStateStatus(domain.StatusExecuting)
	assertCount(1)
	setStateStatus(domain.StatusCompleted)
	assertCount(0)
}
