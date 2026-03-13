package store

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type queryRower interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type CacheWarmer struct {
	db     queryRower
	logger *slog.Logger
}

func NewCacheWarmer(db *pgxpool.Pool, logger *slog.Logger) (*CacheWarmer, error) {
	if db == nil {
		return nil, fmt.Errorf("new cache warmer: db is required")
	}
	if logger == nil {
		logger = slog.Default()
	}

	return &CacheWarmer{db: db, logger: logger}, nil
}

func (w *CacheWarmer) Warm(ctx context.Context) error {
	type warmQuery struct {
		name string
		sql  string
	}

	queries := []warmQuery{
		{name: "queued_job_runs", sql: "SELECT COUNT(*) FROM job_runs WHERE status = 'queued'"},
		{name: "pending_webhook_deliveries", sql: "SELECT COUNT(*) FROM webhook_deliveries WHERE status = 'pending'"},
		{name: "jobs_table_pages", sql: "SELECT 1 FROM jobs LIMIT 1"},
		{name: "workflows_table_pages", sql: "SELECT 1 FROM workflows LIMIT 1"},
	}

	for _, q := range queries {
		started := time.Now()
		var sink int64
		err := w.db.QueryRow(ctx, q.sql).Scan(&sink)
		duration := time.Since(started)

		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("cache warm query %s: %w", q.name, err)
		}

		w.logger.Info("cache warm query completed",
			"query", q.name,
			"duration", duration.String(),
		)
	}

	w.logger.Info("query cache warming completed")
	return nil
}
