//go:build integration

package store_test

import (
	"context"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
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
// store call inside fn runs under the tenant context.
//
// The tx temporarily drops to the strait_app role via SET LOCAL ROLE.
// This is required for RLS to actually enforce: the testcontainers
// postgres image creates the POSTGRES_USER as a superuser, and superusers
// bypass RLS even under FORCE ROW LEVEL SECURITY. strait_app is a
// non-superuser, non-BYPASSRLS role created in testutil.SetupTestDB with
// the DML grants tests need. The superuser seeding runs before the
// SET LOCAL ROLE so data creation is unaffected.
//
// If commit is false the tx is rolled back after fn returns; this is
// useful for read-only assertions that should not persist any side
// effects.
func runAsProject(t *testing.T, ctx context.Context, projectID string, commit bool, fn func(q *store.Queries)) {
	t.Helper()
	tx, err := testDB.Pool.Begin(ctx)
	require.NoError(t, err)

	defer func() {
		if !commit {
			_ = tx.Rollback(ctx)
		}
	}()
	if _, err := tx.Exec(ctx, "SELECT set_config('app.current_project_id', $1, true)", projectID); err != nil {
		_ = tx.Rollback(ctx)
		require.Failf(t, "test failure",

			"set project context: %v", err)
	}
	if _, err := tx.Exec(ctx, "SET LOCAL ROLE strait_app"); err != nil {
		_ = tx.Rollback(ctx)
		require.Failf(t, "test failure",

			"set local role strait_app: %v", err)
	}
	q := store.New(tx)
	fn(q)
	if commit {
		require.NoError(t, tx.Commit(ctx))

	}
}

// countAsProject runs a raw COUNT query under a tenant context so tests
// can assert row visibility without depending on a specific store method
// supporting the table. Drops to the strait_app role for the same reason
// as runAsProject — see that function's comment for the full rationale.
func countAsProject(t *testing.T, ctx context.Context, pool *pgxpool.Pool, projectID, sqlQuery string, args ...any) int {
	t.Helper()
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)

	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SELECT set_config('app.current_project_id', $1, true)", projectID); err != nil {
		require.Failf(t, "test failure",

			"set project context: %v", err)
	}
	if _, err := tx.Exec(ctx, "SET LOCAL ROLE strait_app"); err != nil {
		require.Failf(t, "test failure",

			"set local role strait_app: %v", err)
	}
	var count int
	require.NoError(t, tx.QueryRow(ctx,
		sqlQuery,
		args...).
		Scan(&count))

	return count
}

// Core mechanism: jobs table (from migration 000097 + FORCE in 000182)

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
		require.NoError(t, err)
		require.Len(t, jobs, 1)
		require.Equal(t, jobA.ID,

			jobs[0].
				ID)

	})

	// Query as B: only B's job visible.
	runAsProject(t, ctx, projB, false, func(q *store.Queries) {
		jobs, err := q.ListJobs(ctx, projB, 100, nil)
		require.NoError(t, err)
		require.Len(t, jobs, 1)
		require.Equal(t, jobB.ID,

			jobs[0].
				ID)

	})

	// Cross-tenant direct lookup: project A asks for job B's ID.
	runAsProject(t, ctx, projA, false, func(q *store.Queries) {
		_, err := q.GetJob(ctx, jobB.ID)
		require.Error(t, err)

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
		require.NoError(t, q.CreateRun(ctx,
			run))

	})
	runAsProject(t, ctx, projB, true, func(q *store.Queries) {
		jobB = mustCreateJob(t, ctx, q, projB)
		run := baseRun(jobB, newID())
		require.NoError(t, q.CreateRun(ctx,
			run))

	})

	runAsProject(t, ctx, projA, false, func(q *store.Queries) {
		runs, err := q.ListRunsByJob(ctx, jobA.ID, 100, 0)
		require.NoError(t, err)
		require.Len(t, runs, 1)

	})

	// Cross-tenant: project A asking for job B's runs returns empty.
	runAsProject(t, ctx, projA, false, func(q *store.Queries) {
		runs, err := q.ListRunsByJob(ctx, jobB.ID, 100, 0)
		require.NoError(t, err)
		require.Len(t, runs, 0)

	})
}

// Newly protected tables from migration 000182

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
		require.NoError(t, q.CreateAuditEvent(ctx, ev))

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
		require.NoError(t, q.CreateAuditEvent(ctx, ev))

	})

	got := countAsProject(t, ctx, testDB.Pool, projA, `SELECT COUNT(*) FROM audit_events`)
	require.EqualValues(t, 1, got)

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
			require.Failf(t, "test failure",

				"insert log_drain A: %v", err)
		}
	})
	runAsProject(t, ctx, projB, true, func(_ *store.Queries) {
		if _, err := testDB.Pool.Exec(ctx,
			`INSERT INTO log_drains (id, project_id, name, drain_type, endpoint_url) VALUES ($1, $2, $3, $4, $5)`,
			"drain-"+newID(), projB, "b-drain", "http", "https://b.example.com",
		); err != nil {
			require.Failf(t, "test failure",

				"insert log_drain B: %v", err)
		}
	})

	countA := countAsProject(t, ctx, testDB.Pool, projA, `SELECT COUNT(*) FROM log_drains`)
	countB := countAsProject(t, ctx, testDB.Pool, projB, `SELECT COUNT(*) FROM log_drains`)
	require.EqualValues(t, 1, countA)
	require.EqualValues(t, 1, countB)

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
			require.Failf(t, "test failure",

				"insert job_memory A: %v", err)
		}
	})
	runAsProject(t, ctx, projB, true, func(q *store.Queries) {
		job := mustCreateJob(t, ctx, q, projB)
		if _, err := testDB.Pool.Exec(ctx,
			`INSERT INTO job_memory (id, job_id, project_id, memory_key, value) VALUES ($1, $2, $3, $4, $5::jsonb)`,
			"mem-"+newID(), job.ID, projB, "k", `"v-b"`,
		); err != nil {
			require.Failf(t, "test failure",

				"insert job_memory B: %v", err)
		}
	})

	countA := countAsProject(t, ctx, testDB.Pool, projA, `SELECT COUNT(*) FROM job_memory`)
	require.EqualValues(t, 1, countA)

}

// Transitive policy: job_slo_evaluations isolates via parent job_slos.

func TestRLS_JobSLOEvaluations_ViaParentJobSLO(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	projA := "project-rls-slo-a-" + newID()
	projB := "project-rls-slo-b-" + newID()

	// Seed the jobs, job_slos, and job_slo_evaluations via the pool
	// (superuser path, bypasses RLS) in a single committed sequence.
	// The earlier version seeded through runAsProject's uncommitted tx
	// and then tried to reference the job via a pool-level INSERT,
	// which hit a FK violation because the pool connection couldn't
	// see the uncommitted parent row.
	q := mustStore(t)
	jobA := mustCreateJob(t, ctx, q, projA)
	jobB := mustCreateJob(t, ctx, q, projB)

	sloA := "slo-" + newID()
	if _, err := testDB.Pool.Exec(ctx,
		`INSERT INTO job_slos (id, job_id, project_id, metric, target, window_hours) VALUES ($1, $2, $3, $4, $5, $6)`,
		sloA, jobA.ID, projA, "success_rate", 0.99, 24,
	); err != nil {
		require.Failf(t, "test failure",

			"insert job_slos A: %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx,
		`INSERT INTO job_slo_evaluations (id, slo_id, current_value, budget_remaining) VALUES ($1, $2, $3, $4)`,
		"eval-"+newID(), sloA, 0.98, 0.95,
	); err != nil {
		require.Failf(t, "test failure",

			"insert eval A: %v", err)
	}

	sloB := "slo-" + newID()
	if _, err := testDB.Pool.Exec(ctx,
		`INSERT INTO job_slos (id, job_id, project_id, metric, target, window_hours) VALUES ($1, $2, $3, $4, $5, $6)`,
		sloB, jobB.ID, projB, "success_rate", 0.99, 24,
	); err != nil {
		require.Failf(t, "test failure",

			"insert job_slos B: %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx,
		`INSERT INTO job_slo_evaluations (id, slo_id, current_value, budget_remaining) VALUES ($1, $2, $3, $4)`,
		"eval-"+newID(), sloB, 0.97, 0.90,
	); err != nil {
		require.Failf(t, "test failure",

			"insert eval B: %v", err)
	}

	// The EXISTS subquery in the job_slo_evaluations policy routes RLS
	// through the parent job_slos. Under tenant A, only A's evaluation
	// should be visible, even though job_slo_evaluations has no direct
	// project_id column. countAsProject drops to strait_app inside a
	// tx with app.current_project_id bound, so the policy actually
	// enforces.
	got := countAsProject(t, ctx, testDB.Pool, projA, `SELECT COUNT(*) FROM job_slo_evaluations`)
	require.EqualValues(t, 1, got)

	gotB := countAsProject(t, ctx, testDB.Pool, projB, `SELECT COUNT(*) FROM job_slo_evaluations`)
	require.EqualValues(t, 1, gotB)

}

// webhook_deliveries: project_id populated via COALESCE at insert time.

func TestRLS_WebhookDeliveries_ProjectIDBackfilledViaFK(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	projA := "project-rls-wd-a-" + newID()
	projB := "project-rls-wd-b-" + newID()

	runAsProject(t, ctx, projA, true, func(q *store.Queries) {
		job := mustCreateJob(t, ctx, q, projA)
		run := baseRun(job, newID())
		require.NoError(t, q.CreateRun(ctx,
			run))

		d := &domain.WebhookDelivery{
			RunID:       run.ID,
			JobID:       job.ID,
			WebhookURL:  "https://example.com/a",
			Status:      domain.WebhookStatusPending,
			MaxAttempts: 3,
		}
		require.NoError(t, q.CreateWebhookDelivery(ctx,
			d))

	})
	runAsProject(t, ctx, projB, true, func(q *store.Queries) {
		job := mustCreateJob(t, ctx, q, projB)
		run := baseRun(job, newID())
		require.NoError(t, q.CreateRun(ctx,
			run))

		d := &domain.WebhookDelivery{
			RunID:       run.ID,
			JobID:       job.ID,
			WebhookURL:  "https://example.com/b",
			Status:      domain.WebhookStatusPending,
			MaxAttempts: 3,
		}
		require.NoError(t, q.CreateWebhookDelivery(ctx,
			d))

	})

	countA := countAsProject(t, ctx, testDB.Pool, projA, `SELECT COUNT(*) FROM webhook_deliveries`)
	countB := countAsProject(t, ctx, testDB.Pool, projB, `SELECT COUNT(*) FROM webhook_deliveries`)
	require.EqualValues(t, 1, countA)
	require.EqualValues(t, 1, countB)

}

// Multi-query in one request: the fix target. This is the test that
// would have FAILED against the pre-fix store — set_config's transaction-
// local setting was lost between each pool.Exec. Now every store call
// inside the same tx sees the same project context.

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
		require.NoError(t, err)
		require.Len(t, firstA,
			2,
		)

		// Sanity check: listing project B from inside project A's tx
		// should also be empty because the B rows are not visible.
		b, err := q.ListJobs(ctx, projB, 100, nil)
		require.NoError(t, err)
		require.Len(t, b, 0)

		secondA, err := q.ListJobs(ctx, projA, 100, nil)
		require.NoError(t, err)
		require.Len(t, secondA,

			2)

	})
}

// Sanity: pgx.Tx does satisfy store.DBTX so we can use store.New(tx).

var _ store.DBTX = (pgx.Tx)(nil)
