//go:build integration

package queue_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

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
	if _, err := testDB.Pool.Exec(ctx,
		`UPDATE job_run_state SET job_max_concurrency = 1 WHERE run_id = $1`,
		run.ID,
	); err != nil {
		t.Fatalf("enable active count tracking: %v", err)
	}

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

	var activeUpdatedAt time.Time
	if err := testDB.Pool.QueryRow(ctx,
		`SELECT updated_at FROM job_active_counts WHERE job_id = $1 AND concurrency_key = ''`,
		job.ID,
	).Scan(&activeUpdatedAt); err != nil {
		t.Fatalf("query active count updated_at after dequeue: %v", err)
	}

	setStateStatus(domain.StatusExecuting)
	assertCount(1)

	var afterExecutingUpdatedAt time.Time
	if err := testDB.Pool.QueryRow(ctx,
		`SELECT updated_at FROM job_active_counts WHERE job_id = $1 AND concurrency_key = ''`,
		job.ID,
	).Scan(&afterExecutingUpdatedAt); err != nil {
		t.Fatalf("query active count updated_at after executing: %v", err)
	}
	if !afterExecutingUpdatedAt.Equal(activeUpdatedAt) {
		t.Fatalf("active count updated_at changed on active-to-active transition: before=%s after=%s", activeUpdatedAt, afterExecutingUpdatedAt)
	}

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
			t.Fatalf("update job_run_state status %s: %v", status, err)
		}
	}

	var countRows int
	if err := testDB.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM job_active_counts WHERE job_id = $1`,
		job.ID,
	).Scan(&countRows); err != nil {
		t.Fatalf("query active count rows: %v", err)
	}
	if countRows != 0 {
		t.Fatalf("job_active_counts rows = %d, want 0 for unconstrained job", countRows)
	}
}

func TestJobRunState_TerminalTransitionAppendsColdStateOverlay(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-terminal-state-cold-storage")
	q := mustQueue(t)
	run := mustEnqueueRun(t, ctx, q, job)

	if err := st.UpdateRunStatus(ctx, run.ID, domain.StatusQueued, domain.StatusDequeued, nil); err != nil {
		t.Fatalf("UpdateRunStatus queued->dequeued: %v", err)
	}
	if err := st.UpdateRunStatus(ctx, run.ID, domain.StatusDequeued, domain.StatusExecuting, nil); err != nil {
		t.Fatalf("UpdateRunStatus dequeued->executing: %v", err)
	}

	finishedAt := time.Now().UTC()
	result := json.RawMessage(`{"ok":true}`)
	if err := st.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusCompleted, map[string]any{
		"finished_at": finishedAt,
		"result":      result,
	}); err != nil {
		t.Fatalf("UpdateRunStatus executing->completed: %v", err)
	}

	var hotStatus domain.RunStatus
	if err := testDB.Pool.QueryRow(ctx,
		`SELECT status FROM job_run_state WHERE run_id = $1`,
		run.ID,
	).Scan(&hotStatus); err != nil {
		t.Fatalf("query hot state status: %v", err)
	}
	if hotStatus != domain.StatusExecuting {
		t.Fatalf("hot state status = %s, want retained %s", hotStatus, domain.StatusExecuting)
	}

	var coldStatus domain.RunStatus
	if err := testDB.Pool.QueryRow(ctx,
		`SELECT status FROM job_run_terminal_state WHERE run_id = $1`,
		run.ID,
	).Scan(&coldStatus); err != nil {
		t.Fatalf("query cold terminal state: %v", err)
	}
	if coldStatus != domain.StatusCompleted {
		t.Fatalf("cold terminal status = %s, want %s", coldStatus, domain.StatusCompleted)
	}

	var readStatus domain.RunStatus
	if err := testDB.Pool.QueryRow(ctx,
		`SELECT status FROM job_run_read_state WHERE run_id = $1`,
		run.ID,
	).Scan(&readStatus); err != nil {
		t.Fatalf("query read state status: %v", err)
	}
	if readStatus != domain.StatusCompleted {
		t.Fatalf("read state status = %s, want %s", readStatus, domain.StatusCompleted)
	}

	got, err := st.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error: %v", err)
	}
	if got.Status != domain.StatusCompleted {
		t.Fatalf("GetRun status = %s, want %s", got.Status, domain.StatusCompleted)
	}
	var gotResult, wantResult map[string]bool
	if err := json.Unmarshal(got.Result, &gotResult); err != nil {
		t.Fatalf("unmarshal GetRun result: %v", err)
	}
	if err := json.Unmarshal(result, &wantResult); err != nil {
		t.Fatalf("unmarshal want result: %v", err)
	}
	if gotResult["ok"] != wantResult["ok"] {
		t.Fatalf("GetRun result = %s, want %s", string(got.Result), string(result))
	}
	if got.FinishedAt == nil {
		t.Fatal("GetRun finished_at is nil after terminal transition")
	}
}
