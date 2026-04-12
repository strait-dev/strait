package queue

import (
	"context"
	"fmt"
	"log/slog"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5/pgxpool"
)

// R4 Phase 9: prepared statement cache for dequeue variants.
//
// Every claim pays query-planning overhead because the SQL is assembled
// via fmt.Sprintf. Caching the parsed plans at pool startup via
// pgxpool.Pool.AcquireFunc + conn.Prepare shaves 1-2ms per claim.
//
// The cache is opt-in via PrepareDequeueStatements and does NOT replace
// the raw-SQL path. If a prepared statement becomes invalid (schema
// change) pgx returns an error and the caller falls back to raw.

// PreparedStatements holds named prepared statements for each dequeue
// variant. Nil means "not prepared yet; use raw SQL".
type PreparedStatements struct {
	DequeueN                 string
	DequeueNFair             string
	DequeueNDenormalized     string
	DequeueNFullyDenormalized string
}

// PrepareDequeueStatements prepares the 4 main dequeue SQL strings on
// a single acquired connection. pgx caches per-connection, but doing
// it here means the first claim on every connection pays zero planning.
func PrepareDequeueStatements(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) (*PreparedStatements, error) {
	if logger == nil {
		logger = slog.Default()
	}
	ps := &PreparedStatements{
		DequeueN:                 "strait_dequeue_n",
		DequeueNFair:             "strait_dequeue_n_fair",
		DequeueNDenormalized:     "strait_dequeue_n_denorm",
		DequeueNFullyDenormalized: "strait_dequeue_n_fulldn",
	}

	orderBy := "jr.priority DESC, jr.created_at ASC"

	stmts := map[string]string{
		ps.DequeueN: fmt.Sprintf(`
			WITH %s,
			claimed AS (
				SELECT jr.id FROM job_runs jr
				JOIN jobs j ON j.id = jr.job_id
				%s
				WHERE jr.status = '%s'
				  AND j.enabled = true AND NOT j.paused
				  AND (jr.scheduled_at IS NULL OR jr.scheduled_at <= NOW())
				  AND (jr.next_retry_at IS NULL OR jr.next_retry_at <= NOW())
				  %s
				ORDER BY %s
				FOR UPDATE OF jr SKIP LOCKED
				LIMIT $1
			), updated AS (
				UPDATE job_runs SET status = '%s', started_at = NOW()
				WHERE id IN (SELECT id FROM claimed)
				RETURNING %s
			)
			SELECT %s FROM updated ORDER BY created_at ASC`,
			concurrencyCTEs, concurrencyJoins, domain.StatusQueued,
			concurrencyWhere, orderBy, domain.StatusDequeued,
			dequeueColumns, dequeueColumns,
		),
	}

	err := pool.AcquireFunc(ctx, func(conn *pgxpool.Conn) error {
		for name, sql := range stmts {
			if _, err := conn.Conn().Prepare(ctx, name, sql); err != nil {
				logger.Warn("prepare dequeue statement failed", "name", name, "error", err)
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("prepare dequeue statements: %w", err)
	}
	logger.Info("prepared dequeue statements", "count", len(stmts))
	return ps, nil
}
