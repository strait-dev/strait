package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel"
)

var ErrIndexNotFound = errors.New("index not found")

func (q *Queries) ReindexIndexConcurrently(ctx context.Context, indexName string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ReindexIndexConcurrently")
	defer span.End()

	if indexName == "" {
		return fmt.Errorf("reindex index concurrently: index name is required")
	}

	var exists bool
	if err := q.db.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, indexName).Scan(&exists); err != nil {
		return fmt.Errorf("check index exists before reindex %s: %w", indexName, err)
	}
	if !exists {
		return ErrIndexNotFound
	}

	query := fmt.Sprintf("REINDEX INDEX CONCURRENTLY %s", pgx.Identifier{indexName}.Sanitize())
	if _, err := q.db.Exec(ctx, query); err != nil {
		if isMissingIndexError(err) {
			return ErrIndexNotFound
		}
		return fmt.Errorf("reindex index concurrently %s: %w", indexName, err)
	}

	return nil
}

func isMissingIndexError(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return isMissingIndexErrorCode(pgErr.Code)
}

func isMissingIndexErrorCode(code string) bool {
	switch code {
	case "42P01", "42704":
		return true
	default:
		return false
	}
}
