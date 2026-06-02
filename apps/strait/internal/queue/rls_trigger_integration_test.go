//go:build integration

package queue_test

import (
	"context"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Verify our counter and notify triggers still fire correctly
// when the connection issuing the write is running under
// FORCE ROW LEVEL SECURITY as the non-superuser `strait_app` role.
//
// Why this matters: the testutil test role (`strait_app`) is NOBYPASSRLS.
// Under `FORCE ROW LEVEL SECURITY` the owner is also subject to the
// policy. If a trigger's internal SELECT/INSERT/UPDATE hits a RLS
// check with an unset `app.current_project_id`, the trigger body silently
// filters everything and the counter stays stale. These tests prove our
// triggers don't fall into that trap.

// withRLSSession runs fn inside a transaction that has switched to the
// strait_app role and set the project context.
func withRLSSession(t *testing.T, ctx context.Context, projectID string, fn func(tx pgx.Tx)) {
	t.Helper()
	tx, err := testDB.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, "SELECT set_config('app.current_project_id', $1, true)", projectID); err != nil {
		t.Fatalf("set project: %v", err)
	}
	if _, err := tx.Exec(ctx, "SET LOCAL ROLE strait_app"); err != nil {
		t.Fatalf("set role: %v", err)
	}
	fn(tx)
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

func TestRLS_ActiveCountsTriggerFiresUnderStraitAppRole(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-rls-ac")
	if _, err := testDB.Pool.Exec(ctx, `UPDATE jobs SET max_concurrency = 1000 WHERE id = $1`, job.ID); err != nil {
		t.Fatalf("set max concurrency: %v", err)
	}
	q := mustQueue(t)
	run := mustEnqueueRun(t, ctx, q, job)

	// Transition under a strait_app session with project context set.
	withRLSSession(t, ctx, job.ProjectID, func(tx pgx.Tx) {
		_, err := tx.Exec(ctx,
			`UPDATE job_runs SET status='executing', started_at=NOW() WHERE id=$1`, run.ID)
		if err != nil {
			t.Fatalf("update under RLS: %v", err)
		}
	})

	// After commit, verify the trigger-maintained counter reflects the
	// transition. Use the superuser pool so the verification query isn't
	// itself filtered by RLS.
	var count int
	err := testDB.Pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(count), 0) FROM job_active_counts WHERE job_id = $1`, job.ID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("counter = %d, want 1 (trigger should have fired under RLS session)", count)
	}
}

func TestRLS_DLQCountsTriggerFiresUnderStraitAppRole(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-rls-dlq")
	q := mustQueue(t)
	run := mustEnqueueRun(t, ctx, q, job)

	withRLSSession(t, ctx, job.ProjectID, func(tx pgx.Tx) {
		_, err := tx.Exec(ctx,
			`UPDATE job_runs SET status='dead_letter', finished_at=NOW() WHERE id=$1`, run.ID)
		if err != nil {
			t.Fatalf("dlq under RLS: %v", err)
		}
	})

	var count int
	err := testDB.Pool.QueryRow(ctx,
		`SELECT COALESCE(count, 0) FROM dlq_counts WHERE job_id=$1`, job.ID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("dlq counter = %d, want 1", count)
	}
}

func TestRLS_NotifyTriggerFiresUnderStraitAppRole(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-rls-notify")

	// Open a LISTEN connection outside the RLS session so we can observe
	// notifications.
	listener, err := pgx.Connect(ctx, testDB.ConnStr)
	if err != nil {
		t.Fatalf("listen conn: %v", err)
	}
	defer listener.Close(context.Background())
	if _, err := listener.Exec(ctx, "LISTEN strait_queue_wake"); err != nil {
		t.Fatalf("listen: %v", err)
	}

	// Enqueue via RLS session; the AFTER INSERT trigger should fire
	// pg_notify despite RLS.
	withRLSSession(t, ctx, job.ProjectID, func(tx pgx.Tx) {
		id := uuid.Must(uuid.NewV7()).String()
		_, err := tx.Exec(ctx, `
			INSERT INTO job_runs (id, job_id, project_id, status, attempt, triggered_by, created_at)
			VALUES ($1, $2, $3, 'queued', 1, 'manual', NOW())
		`, id, job.ID, job.ProjectID)
		if err != nil {
			t.Fatalf("insert under RLS: %v", err)
		}
	})

	waitCtx, waitCancel := context.WithTimeout(ctx, 2*time.Second)
	defer waitCancel()
	note, err := listener.WaitForNotification(waitCtx)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if note == nil || note.Channel != "strait_queue_wake" {
		t.Errorf("unexpected notification: %+v", note)
	}
}

func TestRLS_FanoutJobConfigRespectsProjectIsolation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)

	jobA := mustCreateJob(t, ctx, st, "project-rls-fanout-a")
	jobB := mustCreateJob(t, ctx, st, "project-rls-fanout-b")
	q := mustQueue(t)
	_ = mustEnqueueRun(t, ctx, q, jobA)
	_ = mustEnqueueRun(t, ctx, q, jobB)

	// Update jobA.paused under a session scoped to project A. The fanout
	// trigger runs as the table owner (Postgres default), not as the
	// strait_app role, so the trigger body itself is not RLS-filtered.
	// But the UPDATE on jobs would be filtered: the strait_app role can
	// only see jobA. That's expected; the trigger fires only for updates
	// visible to the session.
	withRLSSession(t, ctx, jobA.ProjectID, func(tx pgx.Tx) {
		_, err := tx.Exec(ctx, `UPDATE jobs SET paused = true WHERE id = $1`, jobA.ID)
		if err != nil {
			t.Fatalf("update jobA: %v", err)
		}
	})

	// jobA's queued run should reflect paused=true.
	var pausedA bool
	err := testDB.Pool.QueryRow(ctx,
		`SELECT COALESCE(job_paused, false) FROM job_runs WHERE job_id = $1 LIMIT 1`, jobA.ID,
	).Scan(&pausedA)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if !pausedA {
		t.Errorf("jobA paused not fanned out to queued runs")
	}

	// jobB's queued run should NOT be affected.
	var pausedB bool
	err = testDB.Pool.QueryRow(ctx,
		`SELECT COALESCE(job_paused, false) FROM job_runs WHERE job_id = $1 LIMIT 1`, jobB.ID,
	).Scan(&pausedB)
	if err != nil {
		t.Fatalf("scan B: %v", err)
	}
	if pausedB {
		t.Errorf("jobB paused unexpectedly — fanout bleed across projects")
	}
}

func TestRLS_CounterTriggerNotFiredWhenWriteBlockedByPolicy(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	jobA := mustCreateJob(t, ctx, st, "project-rls-cross-a")
	jobB := mustCreateJob(t, ctx, st, "project-rls-cross-b")
	q := mustQueue(t)
	runB := mustEnqueueRun(t, ctx, q, jobB)

	// Attempt to update jobB's run from a session scoped to project A.
	// RLS should make the UPDATE find zero rows; the counter trigger
	// must not fire.
	var before int
	_ = testDB.Pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(count), 0) FROM job_active_counts WHERE job_id = $1`, jobB.ID,
	).Scan(&before)

	withRLSSession(t, ctx, jobA.ProjectID, func(tx pgx.Tx) {
		tag, err := tx.Exec(ctx,
			`UPDATE job_runs SET status='executing', started_at=NOW() WHERE id=$1`, runB.ID)
		if err != nil {
			t.Fatalf("cross-project update: %v", err)
		}
		if tag.RowsAffected() != 0 {
			t.Errorf("cross-project update affected %d rows, want 0 (RLS should filter)", tag.RowsAffected())
		}
	})

	var after int
	_ = testDB.Pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(count), 0) FROM job_active_counts WHERE job_id = $1`, jobB.ID,
	).Scan(&after)
	if after != before {
		t.Errorf("counter drifted from %d to %d despite RLS-blocked update", before, after)
	}
	_ = domain.StatusQueued // silence unused import if above import list changes
}
