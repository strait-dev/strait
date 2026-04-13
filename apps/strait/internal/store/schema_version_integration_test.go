//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"

	"strait/internal/store"
)

// Schema version mismatch integration tests.

func TestSchemaVersion_MatchAfterMigrations(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	version, err := q.GetSchemaVersion(ctx)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	// After all migrations, the version should be >= 196 (or whatever the
	// latest migration set it to). We don't hard-code 197 here because
	// future migrations in parallel PRs could bump it further.
	if version < 196 {
		t.Errorf("version = %d, want >= 196", version)
	}
}

func TestSchemaVersion_MismatchDetected(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	// Tamper the version to simulate a binary-behind scenario.
	_, err := testDB.Pool.Exec(ctx, `UPDATE schema_version SET version = 999 WHERE id = 1`)
	if err != nil {
		t.Fatalf("tamper: %v", err)
	}
	defer func() {
		// Restore so other tests aren't affected.
		_, _ = testDB.Pool.Exec(ctx, `UPDATE schema_version SET version = 197 WHERE id = 1`)
	}()

	err = q.CheckSchemaVersion(ctx, 196)
	if !errors.Is(err, store.ErrSchemaMismatch) {
		t.Errorf("expected ErrSchemaMismatch, got %v", err)
	}
}

func TestSchemaVersion_BinaryAheadDetected(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	_, err := testDB.Pool.Exec(ctx, `UPDATE schema_version SET version = 100 WHERE id = 1`)
	if err != nil {
		t.Fatalf("tamper: %v", err)
	}
	defer func() {
		_, _ = testDB.Pool.Exec(ctx, `UPDATE schema_version SET version = 197 WHERE id = 1`)
	}()

	err = q.CheckSchemaVersion(ctx, 196)
	if !errors.Is(err, store.ErrSchemaMismatch) {
		t.Errorf("expected ErrSchemaMismatch, got %v", err)
	}
}
