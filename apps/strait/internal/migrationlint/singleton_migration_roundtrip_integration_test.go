//go:build integration

package migrationlint_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"

	"strait/internal/testutil"
)

// Focused down/up round-trip for the singleton migrations (000310 lock table +
// NOT VALID constraints + job_runs waiter index, 000311 VALIDATE CONSTRAINT,
// 000312 CONCURRENTLY workflow_runs waiter index).
//
// The full TestMigrationDrill_UpDownUp skips in the isolated integration DB
// because rolling all the way to zero hits 000176's dependency on the shared
// Sequin tables. This test instead brackets only the three singleton migrations
// with Steps, so it exercises their .down.sql (including the CONCURRENTLY index
// drop and the NOT VALID constraint teardown) without touching 000176.
func TestSingletonMigrations_DownUpRoundTrip(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	tdb, err := testutil.SetupTestDB(ctx, "../../migrations")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer tdb.Cleanup(ctx)

	// SetupTestDB has already applied every migration up, so the schema is at the
	// latest version with the singleton objects present.
	assertSingletonObjects(ctx, t, tdb, true)

	sourceURL := "file://" + filepath.Join("..", "..", "migrations")
	m, err := migrate.New(sourceURL, tdb.ConnStr)
	if err != nil {
		t.Fatalf("create migrator: %v", err)
	}
	defer func() { _, _ = m.Close() }()

	// Roll back the three singleton migrations (312, 311, 310).
	if err := m.Steps(-3); err != nil {
		t.Fatalf("steps down: %v", err)
	}
	assertSingletonObjects(ctx, t, tdb, false)

	// Re-apply them.
	if err := m.Steps(3); err != nil {
		t.Fatalf("steps up: %v", err)
	}
	assertSingletonObjects(ctx, t, tdb, true)
}

// assertSingletonObjects checks that the singleton schema objects either all
// exist or are all gone, depending on want.
func assertSingletonObjects(ctx context.Context, t *testing.T, tdb *testutil.TestDB, want bool) {
	t.Helper()

	checks := []struct {
		name string
		sql  string
	}{
		{"singleton_locks table", `SELECT to_regclass('public.singleton_locks') IS NOT NULL`},
		{"jobs.singleton_on_conflict column", `SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_name = 'jobs' AND column_name = 'singleton_on_conflict')`},
		{"workflow_runs.singleton_key column", `SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_name = 'workflow_runs' AND column_name = 'singleton_key')`},
		{"jobs_singleton_on_conflict_check constraint", `SELECT EXISTS (
			SELECT 1 FROM pg_constraint WHERE conname = 'jobs_singleton_on_conflict_check')`},
		{"idx_job_runs_singleton_waiters index", `SELECT to_regclass('public.idx_job_runs_singleton_waiters') IS NOT NULL`},
		{"idx_workflow_runs_singleton_waiters index", `SELECT to_regclass('public.idx_workflow_runs_singleton_waiters') IS NOT NULL`},
	}

	for _, c := range checks {
		var got bool
		if err := tdb.Pool.QueryRow(ctx, c.sql).Scan(&got); err != nil {
			t.Fatalf("%s probe: %v", c.name, err)
		}
		if got != want {
			t.Errorf("%s present = %v, want %v", c.name, got, want)
		}
	}

	// When the objects exist, the constraint must be VALID (000311), not just
	// created NOT VALID by 000310.
	if want {
		var validated bool
		if err := tdb.Pool.QueryRow(ctx, `
			SELECT convalidated FROM pg_constraint
			WHERE conname = 'jobs_singleton_on_conflict_check'`).Scan(&validated); err != nil {
			t.Fatalf("constraint validated probe: %v", err)
		}
		if !validated {
			t.Error("jobs_singleton_on_conflict_check is NOT VALID; 000311 VALIDATE did not run")
		}
	}
}
