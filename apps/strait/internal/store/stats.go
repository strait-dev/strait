package store

import (
	"context"
	"fmt"

	"strait/internal/domain"

	"go.opentelemetry.io/otel"
)

type QueueStats struct {
	Queued    int `json:"queued"`
	Executing int `json:"executing"`
	Delayed   int `json:"delayed"`
}

func (q *Queries) QueueStats(ctx context.Context) (*QueueStats, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.QueueStats")
	defer span.End()

	query := fmt.Sprintf(`
		SELECT
			COALESCE(SUM(CASE WHEN status = '%s' THEN 1 ELSE 0 END), 0) AS queued,
			COALESCE(SUM(CASE WHEN status = '%s' THEN 1 ELSE 0 END), 0) AS executing,
			COALESCE(SUM(CASE WHEN status = '%s' THEN 1 ELSE 0 END), 0) AS delayed
		FROM job_run_read_state
		WHERE status IN ('%s', '%s', '%s')`,
		domain.StatusQueued, domain.StatusExecuting, domain.StatusDelayed,
		domain.StatusQueued, domain.StatusExecuting, domain.StatusDelayed)

	var stats QueueStats
	err := q.db.QueryRow(ctx, query).Scan(&stats.Queued, &stats.Executing, &stats.Delayed)
	if err != nil {
		return nil, fmt.Errorf("queue stats: %w", err)
	}

	return &stats, nil
}
