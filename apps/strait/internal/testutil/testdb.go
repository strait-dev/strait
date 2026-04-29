//go:build integration

package testutil

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/golang-migrate/migrate/v4"
	// Required for golang-migrate postgres driver registration.
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	// Required for golang-migrate file source driver registration.
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

type TestDB struct {
	Pool      *pgxpool.Pool
	Container *postgres.PostgresContainer
	ConnStr   string
}

func SetupTestDB(ctx context.Context, migrationsPath string) (*TestDB, error) {
	pgContainer, err := postgres.Run(ctx, "postgres:18-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("start postgres container: %w", err)
	}

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = pgContainer.Terminate(ctx)
		return nil, fmt.Errorf("get connection string: %w", err)
	}

	m, err := migrate.New("file://"+migrationsPath, connStr)
	if err != nil {
		_ = pgContainer.Terminate(ctx)
		return nil, fmt.Errorf("create migrator: %w", err)
	}
	defer func() {
		_, _ = m.Close()
	}()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		_ = pgContainer.Terminate(ctx)
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		_ = pgContainer.Terminate(ctx)
		return nil, fmt.Errorf("create pool: %w", err)
	}

	// Create a non-superuser role for RLS tests. The POSTGRES_USER the
	// testcontainers image creates (`test`) is a superuser, which bypasses
	// row-level security even under FORCE ROW LEVEL SECURITY. Without a
	// non-superuser role, we can't actually test that RLS policies filter
	// cross-tenant rows. Tests that need to verify RLS enforcement do
	// `SET LOCAL ROLE strait_app` inside their transaction to temporarily
	// drop to this role. The superuser that seeded the data is implicitly
	// a member of all roles so SET LOCAL ROLE just works.
	if err := setupRLSTestRole(ctx, pool); err != nil {
		pool.Close()
		_ = pgContainer.Terminate(ctx)
		return nil, fmt.Errorf("setup rls test role: %w", err)
	}

	return &TestDB{
		Pool:      pool,
		Container: pgContainer,
		ConnStr:   connStr,
	}, nil
}

// setupRLSTestRole creates a non-superuser, non-BYPASSRLS role and grants
// it the DML privileges integration tests need. This runs after migrations
// so every table exists at grant time.
func setupRLSTestRole(ctx context.Context, pool *pgxpool.Pool) error {
	stmts := []string{
		`DO $$ BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'strait_app') THEN
				CREATE ROLE strait_app NOLOGIN NOBYPASSRLS;
			END IF;
		END $$`,
		`GRANT USAGE ON SCHEMA public TO strait_app`,
		`GRANT SELECT, INSERT, UPDATE, DELETE, TRUNCATE ON ALL TABLES IN SCHEMA public TO strait_app`,
		`GRANT USAGE, SELECT, UPDATE ON ALL SEQUENCES IN SCHEMA public TO strait_app`,
		`ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO strait_app`,
		`ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT USAGE, SELECT, UPDATE ON SEQUENCES TO strait_app`,
	}
	for _, s := range stmts {
		if _, err := pool.Exec(ctx, s); err != nil {
			return fmt.Errorf("exec %q: %w", s, err)
		}
	}
	return nil
}

func (tdb *TestDB) CleanTables(ctx context.Context) error {
	if tdb == nil || tdb.Pool == nil {
		return nil
	}

	_, err := tdb.Pool.Exec(ctx, `TRUNCATE TABLE
		resource_policies, project_member_roles, project_roles,
		known_actors,
		workflow_run_labels, workflow_step_approvals,
		workflow_step_runs, workflow_runs, workflow_version_steps,
		workflow_versions, workflow_steps, workflows,
		webhook_deliveries, webhook_subscriptions,
		api_keys, job_versions, run_events,
		event_triggers, audit_events, audit_events_deadletter, tag_policies,
		event_subscriptions, event_sources, log_drains, batch_operations,
		job_runs, job_secrets, job_dependencies, jobs, job_groups,
		environments, endpoint_circuit_state, project_quotas,
		run_checkpoints, run_outputs, run_tool_calls, run_usage,
		pricing_catalog, run_compute_usage, job_preset_recommendations,
		projects, organization_subscriptions, usage_records,
		organization_addons, sent_usage_reports, processed_webhook_messages,
		enterprise_contracts,
		job_active_counts, dlq_counts, job_run_heartbeats, job_run_queue,
		job_retries, enqueue_outbox, project_rate_limits,
		query_plan_baselines
		CASCADE`)
	if err != nil {
		return fmt.Errorf("clean tables: %w", err)
	}

	return nil
}

func (tdb *TestDB) Cleanup(ctx context.Context) {
	if tdb == nil {
		return
	}

	if tdb.Pool != nil {
		tdb.Pool.Close()
	}

	if tdb.Container != nil {
		_ = tdb.Container.Terminate(ctx)
	}
}
