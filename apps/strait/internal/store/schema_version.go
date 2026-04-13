package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

// Schema version check.

// GetSchemaVersion returns the current schema version from the
// schema_version table. Returns 0 if the table doesn't exist yet
// (pre-migration-196 databases).
func (q *Queries) GetSchemaVersion(ctx context.Context) (int, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetSchemaVersion")
	defer span.End()

	var version int
	err := q.db.QueryRow(ctx, `SELECT version FROM schema_version WHERE id = 1`).Scan(&version)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}
		return 0, fmt.Errorf("get schema version: %w", err)
	}
	return version, nil
}

// ErrSchemaMismatch is returned when the binary's expected schema
// version doesn't match the database.
var ErrSchemaMismatch = errors.New("schema version mismatch")

// CheckSchemaVersion compares the DB schema version against the expected
// version. Returns ErrSchemaMismatch with a descriptive message on
// mismatch. expected=0 skips the check (for tests that don't care).
func (q *Queries) CheckSchemaVersion(ctx context.Context, expected int) error {
	if expected == 0 {
		return nil
	}
	actual, err := q.GetSchemaVersion(ctx)
	if err != nil {
		return err
	}
	if actual == 0 {
		return nil
	}
	if actual != expected {
		if actual > expected {
			return fmt.Errorf("%w: binary expects schema %d but database is at %d; deploy the matching binary", ErrSchemaMismatch, expected, actual)
		}
		return fmt.Errorf("%w: binary expects schema %d but database is at %d; run migrations", ErrSchemaMismatch, expected, actual)
	}
	return nil
}
