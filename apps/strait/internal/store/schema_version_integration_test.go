//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Schema version mismatch integration tests.

func TestSchemaVersion_MatchAfterMigrations(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	version, err := q.GetSchemaVersion(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t,

		version,
		196)

	// After all migrations, the version should be >= 196 (or whatever the
	// latest migration set it to). We don't hard-code 197 here because
	// future migrations in parallel PRs could bump it further.

}

func TestSchemaVersion_MismatchDetected(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	// Tamper the version to simulate a binary-behind scenario.
	_, err := testDB.Pool.Exec(ctx, `UPDATE schema_version SET version = 999 WHERE id = 1`)
	require.NoError(t, err)

	defer func() {
		// Restore so other tests aren't affected.
		_, _ = testDB.Pool.Exec(ctx, `UPDATE schema_version SET version = $1 WHERE id = 1`, domain.ExpectedSchemaVersion)
	}()

	err = q.CheckSchemaVersion(ctx, domain.ExpectedSchemaVersion)
	assert.True(t, errors.Is(err, store.
		ErrSchemaMismatch,
	),
	)

}

func TestSchemaVersion_BinaryAheadDetected(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	_, err := testDB.Pool.Exec(ctx, `UPDATE schema_version SET version = 100 WHERE id = 1`)
	require.NoError(t, err)

	defer func() {
		_, _ = testDB.Pool.Exec(ctx, `UPDATE schema_version SET version = $1 WHERE id = 1`, domain.ExpectedSchemaVersion)
	}()

	err = q.CheckSchemaVersion(ctx, domain.ExpectedSchemaVersion)
	assert.True(t, errors.Is(err, store.
		ErrSchemaMismatch,
	),
	)

}
