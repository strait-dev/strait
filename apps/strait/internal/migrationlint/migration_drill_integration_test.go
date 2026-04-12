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

// R4 Phase 13: migration up/down drill.
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

	migrationsPath := filepath.Join("..", "..", "migrations")
	sourceURL := "file://" + migrationsPath

	// Phase 1: all up (already done by SetupTestDB, but let's confirm).
	m, err := migrate.New(sourceURL, tdb.ConnStr)
	if err != nil {
		t.Fatalf("create migrator: %v", err)
	}
	defer func() { _, _ = m.Close() }()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatalf("up: %v", err)
	}

	// Phase 2: all down to 0.
	if err := m.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatalf("down: %v", err)
	}

	// Phase 3: all up again.
	m2, err := migrate.New(sourceURL, tdb.ConnStr)
	if err != nil {
		t.Fatalf("create migrator for re-up: %v", err)
	}
	defer func() { _, _ = m2.Close() }()

	if err := m2.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatalf("re-up: %v", err)
	}
}
