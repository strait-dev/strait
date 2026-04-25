//go:build integration

package migrationlint_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"

	"strait/internal/testutil"
)

// Migration up/down drill.
//
// Runs all migrations up, then all down to zero, then all up again.
// Verifies every .down.sql actually reverses its .up.sql without error.
// This catches migrations that add columns in the up but forget to
// drop them in the down.

func TestMigrationDrill_UpDownUp(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	tdb, err := testutil.SetupTestDB(ctx, "../../migrations")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer tdb.Cleanup(ctx)

	// Historical migration 000176 restores indexes on shared Sequin CDC
	// tables during down-migration. The isolated integration Postgres used by
	// SetupTestDB does not provision that shared schema, so a full up/down/up
	// drill is not meaningful in this environment.
	var walPipelinesRegclass *string
	if err := tdb.Pool.QueryRow(ctx, `SELECT to_regclass('public.wal_pipelines')::text`).Scan(&walPipelinesRegclass); err != nil {
		t.Fatalf("check wal_pipelines presence: %v", err)
	}
	if walPipelinesRegclass == nil {
		t.Skip("skipping migration drill: shared Sequin tables are not present in isolated integration DB")
	}

	migrationsPath := filepath.Join("..", "..", "migrations")
	sourceURL := "file://" + migrationsPath

	// Run all migrations up (already done by SetupTestDB, but let's confirm).
	m, err := migrate.New(sourceURL, tdb.ConnStr)
	if err != nil {
		t.Fatalf("create migrator: %v", err)
	}
	defer func() { _, _ = m.Close() }()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatalf("up: %v", err)
	}

	// Roll all migrations back to zero.
	if err := m.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatalf("down: %v", err)
	}

	// Re-apply all migrations to verify clean round-trip.
	m2, err := migrate.New(sourceURL, tdb.ConnStr)
	if err != nil {
		t.Fatalf("create migrator for re-up: %v", err)
	}
	defer func() { _, _ = m2.Close() }()

	if err := m2.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatalf("re-up: %v", err)
	}
}
