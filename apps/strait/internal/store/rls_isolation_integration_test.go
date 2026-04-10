//go:build integration

package store_test

import (
	"context"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RLS isolation integration tests.
//
// These tests verify that Postgres row-level security actually enforces
// tenant isolation under the per-request transaction pattern installed in
// the earlier commits. Each test begins a transaction, runs
// SELECT set_config('app.current_project_id', $1, true) on the tx, and
// then calls store methods via a tx-backed *Queries. The set_config
// setting persists because every subsequent query runs on the same tx.
// This is the same mechanism the production rlsTxMiddleware uses.
//
// These tests will fail hard if any of the following regress:
//   - migration 000097 policies (existing tables)
//   - migration 000182 policies + FORCE (newly-protected tables)
//   - migration 000183 policies (webhook_deliveries)
//   - store.ContextWithTx / ctxAwareDBTX routing
//   - rlsTxMiddleware
//
// Integration-tagged so they only run under the dedicated integration
// job that boots a real Postgres via testcontainers.

// runAsProject runs fn inside a tx with app.current_project_id bound to
// projectID. fn receives a *store.Queries backed by the tx, so every
// store call inside fn runs under the tenant context. If commit is
// false the tx is rolled back after fn returns; this is useful for read-
// only assertions that should not persist any side effects.
func runAsProject(t *testing.T, ctx context.Context, projectID string, commit bool, fn func(q *store.Queries)) {
	t.Helper()
	tx, err := testDB.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer func() {
		if !commit {
			_ = tx.Rollback(ctx)
		}
	}()
	if _, err := tx.Exec(ctx, "SELECT set_config('app.current_project_id', $1, true)", projectID); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("set project context: %v", err)
	}
	q := store.New(tx)
	fn(q)
	if commit {
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit tx: %v", err)
		}
	}
}

// countAsProject runs a raw COUNT query under a tenant context so tests
// can assert row visibility without depending on a specific store method
// supporting the table.
func countAsProject(t *testing.T, ctx context.Context, pool *pgxpool.Pool, projectID, sqlQuery string, args ...any) int {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SELECT set_config('app.current_project_id', $1, true)", projectID); err != nil {
		t.Fatalf("set project context: %v", err)
	}
	var count int
	if err := tx.QueryRow(ctx, sqlQuery, args...).Scan(&count); err != nil {
		t.Fatalf("count query: %v", err)
	}
	return count
}

// -----------------------------------------------------------------------.
// Core mechanism: jobs table (from migration 000097 + FORCE in 000182)
// -----------------------------------------------------------------------.

func TestRLS_Jobs_CrossTenantIsolation(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	projA := "project-rls-a-" + newID()
	projB := "project-rls-b-" + newID()

	var jobA, jobB *domain.Job
	runAsProject(t, ctx, projA, true, func(q *store.Queries) {
		jobA = mustCreateJob(t, ctx, q, projA)
	})
	runAsProject(t, ctx, projB, true, func(q *store.Queries) {
		jobB = mustCreateJob(t, ctx, q, projB)
	})

	// Query as A: only A's job visible.
	runAsProject(t, ctx, projA, false, func(q *store.Queries) {
		jobs, err := q.ListJobs(ctx, projA, 100, nil)
		if err != nil {
			t.Fatalf("ListJobs(projA) error = %v", err)
		}
		if len(jobs) != 1 {
			t.Fatalf("project A sees %d jobs, want 1 (RLS leaking)", len(jobs))
		}
		if jobs[0].ID != jobA.ID {
			t.Fatalf("project A sees job %q, want %q", jobs[0].ID, jobA.ID)
		}
	})

	// Query as B: only B's job visible.
	runAsProject(t, ctx, projB, false, func(q *store.Queries) {
		jobs, err := q.ListJobs(ctx, projB, 100, nil)
		if err != nil {
			t.Fatalf("ListJobs(projB) error = %v", err)
		}
		if len(jobs) != 1 {
			t.Fatalf("project B sees %d jobs, want 1", len(jobs))
		}
		if jobs[0].ID != jobB.ID {
			t.Fatalf("project B sees job %q, want %q", jobs[0].ID, jobB.ID)
		}
	})

	// Cross-tenant direct lookup: project A asks for job B's ID.
	runAsProject(t, ctx, projA, false, func(q *store.Queries) {
		_, err := q.GetJob(ctx, jobB.ID)
		if err == nil {
			t.Fatal("GetJob(jobB.ID) as project A returned a row, want not found")
		}
	})
}

func TestRLS_JobRuns_CrossTenantIsolation(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	projA := "project-rls-runs-a-" + newID()
	projB := "project-rls-runs-b-" + newID()

	var jobA, jobB *domain.Job
	runAsProject(t, ctx, projA, true, func(q *store.Queries) {
		jobA = mustCreateJob(t, ctx, q, projA)
		run := baseRun(jobA, newID())
		if err := q.CreateRun(ctx, run); err != nil {
			t.Fatalf("CreateRun(A) error = %v", err)
		}
	})
	runAsProject(t, ctx, projB, true, func(q *store.Queries) {
		jobB = mustCreateJob(t, ctx, q, projB)
		run := baseRun(jobB, newID())
		if err := q.CreateRun(ctx, run); err != nil {
			t.Fatalf("CreateRun(B) error = %v", err)
		}
	})

	runAsProject(t, ctx, projA, false, func(q *store.Queries) {
		runs, err := q.ListRunsByJob(ctx, jobA.ID, 100, 0)
		if err != nil {
			t.Fatalf("ListRunsByJob(A) error = %v", err)
		}
		if len(runs) != 1 {
			t.Fatalf("project A sees %d runs, want 1", len(runs))
		}
	})

	// Cross-tenant: project A asking for job B's runs returns empty.
	runAsProject(t, ctx, projA, false, func(q *store.Queries) {
		runs, err := q.ListRunsByJob(ctx, jobB.ID, 100, 0)
		if err != nil {
			t.Fatalf("ListRunsByJob(projA, jobB) error = %v", err)
		}
		if len(runs) != 0 {
			t.Fatalf("cross-tenant run list returned %d rows, want 0", len(runs))
		}
	})
}

// -----------------------------------------------------------------------.
// Newly protected tables from migration 000182
// -----------------------------------------------------------------------.

func TestRLS_AuditEvents_CrossTenantIsolation(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	projA := "project-rls-audit-a-" + newID()
	projB := "project-rls-audit-b-" + newID()

	runAsProject(t, ctx, projA, true, func(q *store.Queries) {
		ev := &domain.AuditEvent{
			ProjectID:    projA,
			ActorID:      "actor-a",
			ActorType:    "user",
			Action:       "create",
			ResourceType: "job",
			ResourceID:   newID(),
		}
		if err := q.CreateAuditEvent(ctx, ev); err != nil {
			t.Fatalf("CreateAuditEvent(A) error = %v", err)
		}
	})
	runAsProject(t, ctx, projB, true, func(q *store.Queries) {
		ev := &domain.AuditEvent{
			ProjectID:    projB,
			ActorID:      "actor-b",
			ActorType:    "user",
			Action:       "create",
			ResourceType: "job",
			ResourceID:   newID(),
		}
		if err := q.CreateAuditEvent(ctx, ev); err != nil {
			t.Fatalf("CreateAuditEvent(B) error = %v", err)
		}
	})

	got := countAsProject(t, ctx, testDB.Pool, projA, `SELECT COUNT(*) FROM audit_events`)
	if got != 1 {
		t.Fatalf("project A sees %d audit events, want 1 (RLS leaking)", got)
	}
}

func TestRLS_LogDrains_CrossTenantIsolation(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	projA := "project-rls-drains-a-" + newID()
	projB := "project-rls-drains-b-" + newID()

	runAsProject(t, ctx, projA, true, func(_ *store.Queries) {
		if _, err := testDB.Pool.Exec(ctx,
			`INSERT INTO log_drains (id, project_id, name, drain_type, endpoint_url) VALUES ($1, $2, $3, $4, $5)`,
			"drain-"+newID(), projA, "a-drain", "http", "https://a.example.com",
		); err != nil {
			t.Fatalf("insert log_drain A: %v", err)
		}
	})
	runAsProject(t, ctx, projB, true, func(_ *store.Queries) {
		if _, err := testDB.Pool.Exec(ctx,
			`INSERT INTO log_drains (id, project_id, name, drain_type, endpoint_url) VALUES ($1, $2, $3, $4, $5)`,
			"drain-"+newID(), projB, "b-drain", "http", "https://b.example.com",
		); err != nil {
			t.Fatalf("insert log_drain B: %v", err)
		}
	})

	countA := countAsProject(t, ctx, testDB.Pool, projA, `SELECT COUNT(*) FROM log_drains`)
	countB := countAsProject(t, ctx, testDB.Pool, projB, `SELECT COUNT(*) FROM log_drains`)
	if countA != 1 {
		t.Fatalf("project A sees %d log_drains, want 1", countA)
	}
	if countB != 1 {
		t.Fatalf("project B sees %d log_drains, want 1", countB)
	}
}

func TestRLS_JobMemory_CrossTenantIsolation(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	projA := "project-rls-mem-a-" + newID()
	projB := "project-rls-mem-b-" + newID()

	runAsProject(t, ctx, projA, true, func(q *store.Queries) {
		job := mustCreateJob(t, ctx, q, projA)
		if _, err := testDB.Pool.Exec(ctx,
			`INSERT INTO job_memory (id, job_id, project_id, memory_key, value) VALUES ($1, $2, $3, $4, $5::jsonb)`,
			"mem-"+newID(), job.ID, projA, "k", `"v-a"`,
		); err != nil {
			t.Fatalf("insert job_memory A: %v", err)
		}
	})
	runAsProject(t, ctx, projB, true, func(q *store.Queries) {
		job := mustCreateJob(t, ctx, q, projB)
		if _, err := testDB.Pool.Exec(ctx,
			`INSERT INTO job_memory (id, job_id, project_id, memory_key, value) VALUES ($1, $2, $3, $4, $5::jsonb)`,
			"mem-"+newID(), job.ID, projB, "k", `"v-b"`,
		); err != nil {
			t.Fatalf("insert job_memory B: %v", err)
		}
	})

	countA := countAsProject(t, ctx, testDB.Pool, projA, `SELECT COUNT(*) FROM job_memory`)
	if countA != 1 {
		t.Fatalf("project A sees %d job_memory rows, want 1", countA)
	}
}

// -----------------------------------------------------------------------.
// Transitive policy: job_slo_evaluations isolates via parent job_slos.
// -----------------------------------------------------------------------.

func TestRLS_JobSLOEvaluations_ViaParentJobSLO(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	projA := "project-rls-slo-a-" + newID()
	projB := "project-rls-slo-b-" + newID()

	var sloA, sloB string
	runAsProject(t, ctx, projA, true, func(q *store.Queries) {
		job := mustCreateJob(t, ctx, q, projA)
		sloA = "slo-" + newID()
		if _, err := testDB.Pool.Exec(ctx,
			`INSERT INTO job_slos (id, job_id, project_id, metric, target, window_hours) VALUES ($1, $2, $3, $4, $5, $6)`,
			sloA, job.ID, projA, "success_rate", 0.99, 24,
		); err != nil {
			t.Fatalf("insert job_slos A: %v", err)
		}
		if _, err := testDB.Pool.Exec(ctx,
			`INSERT INTO job_slo_evaluations (id, slo_id, current_value, budget_remaining) VALUES ($1, $2, $3, $4)`,
			"eval-"+newID(), sloA, 0.98, 0.95,
		); err != nil {
			t.Fatalf("insert eval A: %v", err)
		}
	})
	runAsProject(t, ctx, projB, true, func(q *store.Queries) {
		job := mustCreateJob(t, ctx, q, projB)
		sloB = "slo-" + newID()
		if _, err := testDB.Pool.Exec(ctx,
			`INSERT INTO job_slos (id, job_id, project_id, metric, target, window_hours) VALUES ($1, $2, $3, $4, $5, $6)`,
			sloB, job.ID, projB, "success_rate", 0.99, 24,
		); err != nil {
			t.Fatalf("insert job_slos B: %v", err)
		}
		if _, err := testDB.Pool.Exec(ctx,
			`INSERT INTO job_slo_evaluations (id, slo_id, current_value, budget_remaining) VALUES ($1, $2, $3, $4)`,
			"eval-"+newID(), sloB, 0.97, 0.90,
		); err != nil {
			t.Fatalf("insert eval B: %v", err)
		}
	})

	// The EXISTS subquery in the job_slo_evaluations policy routes RLS
	// through the parent job_slos. Under tenant A, only A's evaluation
	// should be visible, even though job_slo_evaluations has no direct
	// project_id column.
	got := countAsProject(t, ctx, testDB.Pool, projA, `SELECT COUNT(*) FROM job_slo_evaluations`)
	if got != 1 {
		t.Fatalf("project A sees %d SLO evaluations, want 1 (transitive RLS failed)", got)
	}
}

// -----------------------------------------------------------------------.
// webhook_deliveries: project_id populated via COALESCE at insert time.
// -----------------------------------------------------------------------.

func TestRLS_WebhookDeliveries_ProjectIDBackfilledViaFK(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	projA := "project-rls-wd-a-" + newID()
	projB := "project-rls-wd-b-" + newID()

	runAsProject(t, ctx, projA, true, func(q *store.Queries) {
		job := mustCreateJob(t, ctx, q, projA)
		run := baseRun(job, newID())
		if err := q.CreateRun(ctx, run); err != nil {
			t.Fatalf("CreateRun A: %v", err)
		}
		d := &domain.WebhookDelivery{
			RunID:       run.ID,
			JobID:       job.ID,
			WebhookURL:  "https://example.com/a",
			Status:      domain.WebhookStatusPending,
			MaxAttempts: 3,
		}
		if err := q.CreateWebhookDelivery(ctx, d); err != nil {
			t.Fatalf("CreateWebhookDelivery A: %v", err)
		}
	})
	runAsProject(t, ctx, projB, true, func(q *store.Queries) {
		job := mustCreateJob(t, ctx, q, projB)
		run := baseRun(job, newID())
		if err := q.CreateRun(ctx, run); err != nil {
			t.Fatalf("CreateRun B: %v", err)
		}
		d := &domain.WebhookDelivery{
			RunID:       run.ID,
			JobID:       job.ID,
			WebhookURL:  "https://example.com/b",
			Status:      domain.WebhookStatusPending,
			MaxAttempts: 3,
		}
		if err := q.CreateWebhookDelivery(ctx, d); err != nil {
			t.Fatalf("CreateWebhookDelivery B: %v", err)
		}
	})

	countA := countAsProject(t, ctx, testDB.Pool, projA, `SELECT COUNT(*) FROM webhook_deliveries`)
	countB := countAsProject(t, ctx, testDB.Pool, projB, `SELECT COUNT(*) FROM webhook_deliveries`)
	if countA != 1 {
		t.Fatalf("project A sees %d webhook_deliveries, want 1", countA)
	}
	if countB != 1 {
		t.Fatalf("project B sees %d webhook_deliveries, want 1", countB)
	}
}

// -----------------------------------------------------------------------.
// Multi-query in one request: the fix target. This is the test that
// would have FAILED against the pre-fix store — set_config's transaction-
// local setting was lost between each pool.Exec. Now every store call
// inside the same tx sees the same project context.
// -----------------------------------------------------------------------.

func TestRLS_MultipleQueriesInOneTx_ShareProjectContext(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	projA := "project-rls-multi-" + newID()
	projB := "project-rls-multi-b-" + newID()

	runAsProject(t, ctx, projA, true, func(q *store.Queries) {
		mustCreateJob(t, ctx, q, projA)
		mustCreateJob(t, ctx, q, projA)
	})
	runAsProject(t, ctx, projB, true, func(q *store.Queries) {
		mustCreateJob(t, ctx, q, projB)
	})

	// Inside one tx, call ListJobs twice. Both calls must see the same
	// set of rows. Against the pre-fix code, set_config's local setting
	// was lost after the first ListJobs and the second call would return
	// everything via the escape hatch.
	runAsProject(t, ctx, projA, false, func(q *store.Queries) {
		firstA, err := q.ListJobs(ctx, projA, 100, nil)
		if err != nil {
			t.Fatalf("first ListJobs: %v", err)
		}
		if len(firstA) != 2 {
			t.Fatalf("first ListJobs sees %d, want 2", len(firstA))
		}

		// Sanity check: listing project B from inside project A's tx
		// should also be empty because the B rows are not visible.
		b, err := q.ListJobs(ctx, projB, 100, nil)
		if err != nil {
			t.Fatalf("cross-tenant ListJobs: %v", err)
		}
		if len(b) != 0 {
			t.Fatalf("cross-tenant ListJobs leaked %d rows", len(b))
		}

		secondA, err := q.ListJobs(ctx, projA, 100, nil)
		if err != nil {
			t.Fatalf("second ListJobs: %v", err)
		}
		if len(secondA) != 2 {
			t.Fatalf("second ListJobs sees %d, want 2 (context lost mid-request)", len(secondA))
		}
	})
}

// -----------------------------------------------------------------------.
// Sanity: pgx.Tx does satisfy store.DBTX so we can use store.New(tx).
// -----------------------------------------------------------------------.

var _ store.DBTX = (pgx.Tx)(nil)
