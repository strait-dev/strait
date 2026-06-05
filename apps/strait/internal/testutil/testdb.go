//go:build integration

package testutil

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang-migrate/migrate/v4"
	// Required for golang-migrate postgres driver registration.
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	// Required for golang-migrate file source driver registration.
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

type TestDB struct {
	Pool      *pgxpool.Pool
	Container *postgres.PostgresContainer
	ConnStr   string
	cleanup   func(context.Context) error
}

// SetupTestDB returns an isolated migrated database. By default it reuses a
// process-local/testcontainers Postgres container and clones a migrated
// template database, so callers keep isolation without paying container and
// migration startup on every test. Set STRAIT_TEST_FRESH_CONTAINER=1 to force
// the historical one-container-per-call behavior.
func SetupTestDB(ctx context.Context, migrationsPath string) (*TestDB, error) {
	if os.Getenv("STRAIT_TEST_FRESH_CONTAINER") == "1" {
		return SetupFreshTestDB(ctx, migrationsPath)
	}
	return SetupSharedTestDB(ctx, migrationsPath, "default")
}

func SetupSharedTestDB(ctx context.Context, migrationsPath, namespace string) (*TestDB, error) {
	defer testTiming("SetupSharedTestDB " + namespace)()

	shared, err := getSharedPostgres(ctx)
	if err != nil {
		return nil, err
	}

	hash, err := migrationHash(migrationsPath)
	if err != nil {
		return nil, fmt.Errorf("hash migrations: %w", err)
	}
	templateDB := "template_strait_" + hash[:16]

	adminPool, err := newTestDBPool(ctx, shared.ConnStr)
	if err != nil {
		return nil, fmt.Errorf("create shared postgres admin pool: %w", err)
	}

	if err := ensureTemplateDB(ctx, adminPool, shared.ConnStr, templateDB, migrationsPath); err != nil {
		adminPool.Close()
		return nil, err
	}

	dbName := isolatedDBName(namespace)
	if _, err := adminPool.Exec(ctx, fmt.Sprintf("CREATE DATABASE %s TEMPLATE %s", quoteIdent(dbName), quoteIdent(templateDB))); err != nil {
		adminPool.Close()
		return nil, fmt.Errorf("create isolated test database %s: %w", dbName, err)
	}

	connStr, err := connStringForDB(shared.ConnStr, dbName)
	if err != nil {
		adminPool.Close()
		return nil, err
	}
	pool, err := newTestDBPool(ctx, connStr)
	if err != nil {
		_ = dropDatabase(ctx, adminPool, dbName)
		adminPool.Close()
		return nil, fmt.Errorf("create isolated test pool: %w", err)
	}

	return &TestDB{
		Pool:    pool,
		ConnStr: connStr,
		cleanup: func(ctx context.Context) error {
			defer adminPool.Close()
			return dropDatabase(ctx, adminPool, dbName)
		},
	}, nil
}

func SetupFreshTestDB(ctx context.Context, migrationsPath string) (*TestDB, error) {
	defer testTiming("SetupFreshTestDB")()

	pgContainer, err := postgres.Run(ctx, "postgres:18-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithCmd(
			"postgres",
			"-c", "fsync=off",
			"-c", "wal_level=logical",
			"-c", "max_replication_slots=10",
			"-c", "max_wal_senders=10",
		),
		testcontainers.WithWaitStrategy(
			wait.ForMappedPort("5432/tcp").
				WithStartupTimeout(180*time.Second),
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

	m, err := runMigrationsWhenReady(connStr, migrationsPath)
	if err != nil {
		_ = pgContainer.Terminate(ctx)
		return nil, err
	}
	defer func() {
		_, _ = m.Close()
	}()

	pool, err := newTestDBPool(ctx, connStr)
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

type sharedPostgres struct {
	Container *postgres.PostgresContainer
	ConnStr   string
}

var (
	sharedPostgresMu               sync.Mutex
	sharedPostgresVal              *sharedPostgres
	persistentTestcontainersOnce   sync.Once
	persistentTestcontainersSetErr error
)

func getSharedPostgres(ctx context.Context) (*sharedPostgres, error) {
	sharedPostgresMu.Lock()
	defer sharedPostgresMu.Unlock()
	if sharedPostgresVal != nil {
		return sharedPostgresVal, nil
	}
	if err := configurePersistentTestcontainers(); err != nil {
		return nil, err
	}

	container, err := postgres.Run(ctx, "postgres:18-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithCmd(
			"postgres",
			"-c", "fsync=off",
			"-c", "wal_level=logical",
			"-c", "max_replication_slots=10",
			"-c", "max_wal_senders=10",
		),
		testcontainers.WithReuseByName(sharedContainerName("postgres")),
		testcontainers.WithWaitStrategy(
			wait.ForMappedPort("5432/tcp").
				WithStartupTimeout(180*time.Second),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("start shared postgres container: %w", err)
	}

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return nil, fmt.Errorf("get shared postgres connection string: %w", err)
	}
	if err := waitForPostgresReady(ctx, connStr); err != nil {
		return nil, fmt.Errorf("wait for shared postgres readiness: %w", err)
	}

	sharedPostgresVal = &sharedPostgres{Container: container, ConnStr: connStr}
	return sharedPostgresVal, nil
}

func configurePersistentTestcontainers() error {
	persistentTestcontainersOnce.Do(func() {
		if os.Getenv("STRAIT_TEST_PERSIST_CONTAINERS") != "1" {
			return
		}
		if v := os.Getenv("TESTCONTAINERS_RYUK_DISABLED"); v != "" && v != "true" {
			persistentTestcontainersSetErr = fmt.Errorf("STRAIT_TEST_PERSIST_CONTAINERS=1 requires TESTCONTAINERS_RYUK_DISABLED to be unset or true, got %q", v)
			return
		}
		_ = os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
	})
	return persistentTestcontainersSetErr
}

func waitForPostgresReady(ctx context.Context, connStr string) error {
	deadline := time.Now().Add(180 * time.Second)
	var lastErr error
	for {
		pool, err := newTestDBPool(ctx, connStr)
		if err == nil {
			err = pool.Ping(ctx)
			pool.Close()
			if err == nil {
				return nil
			}
		}
		lastErr = err
		if time.Now().After(deadline) {
			return fmt.Errorf("ping postgres: %w", lastErr)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func ensureTemplateDB(ctx context.Context, adminPool *pgxpool.Pool, adminConnStr, templateDB, migrationsPath string) error {
	defer testTiming("ensureTemplateDB " + templateDB)()

	if _, err := adminPool.Exec(ctx, `SELECT pg_advisory_lock(hashtext($1))`, templateDB); err != nil {
		return fmt.Errorf("lock template database %s: %w", templateDB, err)
	}
	defer func() {
		_, _ = adminPool.Exec(context.Background(), `SELECT pg_advisory_unlock(hashtext($1))`, templateDB)
	}()

	var exists bool
	if err := adminPool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)`, templateDB).Scan(&exists); err != nil {
		return fmt.Errorf("check template database %s: %w", templateDB, err)
	}
	if exists {
		ready, err := templateDBReady(ctx, adminConnStr, templateDB)
		if err != nil {
			return fmt.Errorf("check template database readiness %s: %w", templateDB, err)
		}
		if ready {
			return nil
		}
		if err := dropDatabase(ctx, adminPool, templateDB); err != nil {
			return err
		}
	}

	if _, err := adminPool.Exec(ctx, fmt.Sprintf("CREATE DATABASE %s", quoteIdent(templateDB))); err != nil {
		return fmt.Errorf("create template database %s: %w", templateDB, err)
	}

	templateConnStr, err := connStringForDB(adminConnStr, templateDB)
	if err != nil {
		return err
	}
	m, err := runMigrationsWhenReady(templateConnStr, migrationsPath)
	if err != nil {
		_ = dropDatabase(ctx, adminPool, templateDB)
		return err
	}
	_, _ = m.Close()

	templatePool, err := newTestDBPool(ctx, templateConnStr)
	if err != nil {
		_ = dropDatabase(ctx, adminPool, templateDB)
		return fmt.Errorf("create template pool: %w", err)
	}
	defer templatePool.Close()
	if err := setupRLSTestRole(ctx, templatePool); err != nil {
		_ = dropDatabase(ctx, adminPool, templateDB)
		return fmt.Errorf("setup template rls role: %w", err)
	}
	return nil
}

func templateDBReady(ctx context.Context, adminConnStr, templateDB string) (bool, error) {
	templateConnStr, err := connStringForDB(adminConnStr, templateDB)
	if err != nil {
		return false, err
	}
	pool, err := newTestDBPool(ctx, templateConnStr)
	if err != nil {
		return false, fmt.Errorf("create template readiness pool: %w", err)
	}
	defer pool.Close()

	var dirty bool
	err = pool.QueryRow(ctx, `SELECT dirty FROM schema_migrations LIMIT 1`).Scan(&dirty)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return !dirty, nil
}

func newTestDBPool(ctx context.Context, connStr string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = testDBMaxConns()
	return pgxpool.NewWithConfig(ctx, cfg)
}

func testDBMaxConns() int32 {
	const defaultMaxConns = int32(4)
	raw := os.Getenv("STRAIT_TEST_DB_MAX_CONNS")
	if raw == "" {
		return defaultMaxConns
	}
	n, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || n < 1 {
		return defaultMaxConns
	}
	return int32(n)
}

func dropDatabase(ctx context.Context, pool *pgxpool.Pool, dbName string) error {
	if _, err := pool.Exec(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %s WITH (FORCE)", quoteIdent(dbName))); err != nil {
		return fmt.Errorf("drop test database %s: %w", dbName, err)
	}
	return nil
}

func migrationHash(migrationsPath string) (string, error) {
	h := sha256.New()
	err := filepath.WalkDir(migrationsPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".sql") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(migrationsPath, path)
		if err != nil {
			return err
		}
		_, _ = h.Write([]byte(rel))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write(b)
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func connStringForDB(connStr, dbName string) (string, error) {
	u, err := url.Parse(connStr)
	if err != nil {
		return "", fmt.Errorf("parse postgres connection string: %w", err)
	}
	u.Path = "/" + dbName
	return u.String(), nil
}

func isolatedDBName(namespace string) string {
	name := sanitizeIdentifierPart(namespace)
	if name == "" {
		name = "test"
	}
	if len(name) > 30 {
		name = name[:30]
	}
	return "strait_" + name + "_" + randomHex(6)
}

func sharedContainerName(kind string) string {
	scope := os.Getenv("STRAIT_TEST_CONTAINER_SCOPE")
	if scope == "" {
		scope = moduleRootForScope()
	}
	sum := sha256.Sum256([]byte(scope + "\x00" + kind + "\x00" + sharedContainerConfigVersion(kind)))
	return "strait-test-" + kind + "-" + hex.EncodeToString(sum[:])[:12]
}

func sharedContainerConfigVersion(kind string) string {
	switch kind {
	case "postgres":
		return "postgres18-template-v2"
	case "redis":
		return "redis8-db4096-v2"
	default:
		return "default-v2"
	}
}

func moduleRootForScope() string {
	wd, err := os.Getwd()
	if err != nil {
		return "unknown"
	}
	for dir := wd; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return wd
		}
	}
}

func sanitizeIdentifierPart(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return strings.Trim(b.String(), "_")
}

func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func testTiming(name string) func() {
	if os.Getenv("STRAIT_TEST_TIMING") != "1" {
		return func() {}
	}
	start := time.Now()
	return func() {
		fmt.Fprintf(os.Stderr, "testutil timing: %s took %s\n", name, time.Since(start).Round(time.Millisecond))
	}
}

func runMigrationsWhenReady(connStr, migrationsPath string) (*migrate.Migrate, error) {
	deadline := time.Now().Add(180 * time.Second)
	var lastErr error
	for {
		m, err := migrate.New("file://"+migrationsPath, connStr)
		if err == nil {
			err = m.Up()
			if err == nil || errors.Is(err, migrate.ErrNoChange) {
				return m, nil
			}
			_, _ = m.Close()
		}
		lastErr = err
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("run migrations after postgres became ready: %w", lastErr)
		}
		time.Sleep(500 * time.Millisecond)
	}
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
		workflow_progression_events,
		workflow_step_runs, workflow_runs, workflow_version_steps,
		workflow_versions, workflow_steps, workflows,
		webhook_deliveries, webhook_subscriptions,
		api_keys, cli_device_codes, job_versions, run_events,
		event_triggers, audit_events, audit_events_deadletter, audit_signing_keys, audit_chain_checkpoints, tag_policies,
		event_subscriptions, event_sources, log_drains, batch_operations,
		worker_tasks, workers,
		job_runs, job_secrets, job_dependencies, jobs, job_groups,
		environments, endpoint_circuit_state, project_quotas,
		run_checkpoints, run_outputs,
		projects, organization_subscriptions, usage_records,
		organization_addons, sent_usage_reports, processed_webhook_messages,
		enterprise_contracts,
		queue_entries, queue_batch_ticks, queue_batches, queue_batch_seal_state,
		job_active_counts, dlq_counts, job_run_heartbeats, job_run_queue,
		job_run_active_claims, job_run_lifecycle_events, job_run_ready_events,
		job_run_priority_events, job_run_visibility_events, job_run_cache_versions,
		job_run_terminal_state, job_run_state,
		job_retries, outbox_claims, outbox_batches, enqueue_outbox,
		enqueue_outbox_history, project_rate_limits,
		strait_pgque_routes,
		query_plan_baselines
		CASCADE`)
	if err != nil {
		return fmt.Errorf("clean tables: %w", err)
	}

	if err := tdb.cleanPgQueTables(ctx); err != nil {
		return err
	}

	return nil
}

func (tdb *TestDB) cleanPgQueTables(ctx context.Context) error {
	rows, err := tdb.Pool.Query(ctx, `
		SELECT tablename
		FROM pg_tables
		WHERE schemaname = 'pgque'
		  AND (
		      tablename IN ('retry_queue', 'dead_letter')
		      OR (tablename LIKE 'event\_%' AND tablename <> 'event_template')
		  )
		ORDER BY tablename`)
	if err != nil {
		return fmt.Errorf("list pgque tables: %w", err)
	}
	defer rows.Close()

	tables := make([]string, 0)
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return fmt.Errorf("scan pgque table: %w", err)
		}
		tables = append(tables, pgx.Identifier{"pgque", tableName}.Sanitize())
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("list pgque table rows: %w", err)
	}
	if len(tables) == 0 {
		return nil
	}

	if _, err := tdb.Pool.Exec(ctx, `TRUNCATE TABLE `+strings.Join(tables, ", ")+` CASCADE`); err != nil {
		return fmt.Errorf("clean pgque tables: %w", err)
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

	if tdb.cleanup != nil {
		_ = tdb.cleanup(ctx)
	}

	if tdb.Container != nil {
		_ = tdb.Container.Terminate(ctx)
	}
}
