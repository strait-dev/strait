package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

func (q *Queries) ReindexIndexConcurrently(ctx context.Context, indexName string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ReindexIndexConcurrently")
	defer span.End()

	if indexName == "" {
		return fmt.Errorf("reindex index concurrently: index name is required")
	}

	query := fmt.Sprintf("REINDEX INDEX CONCURRENTLY %s", pgx.Identifier{indexName}.Sanitize())
	if _, err := q.db.Exec(ctx, query); err != nil {
		return fmt.Errorf("reindex index concurrently %s: %w", indexName, err)
	}

	return nil
}
