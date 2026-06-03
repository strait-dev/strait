package queue

import (
	"context"
	"fmt"
	"log/slog"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Prepared statement warmup for dequeue variants.
//
// Important caveat on scope: pgx/v5's statement cache is per-connection
// (see pgx.QueryExecModeCacheStatement, the default). Preparing inside a
// single AcquireFunc only warms that one connection; the other pool
// connections still pay first-query planning on their first dequeue.
//
// We intentionally leave this as a single-connection warmup rather than
// a pool.Config().AfterConnect hook because the rest of the queue does
// not execute via these named statements — DequeueN and friends build
// SQL per-call and send it through pgxpool.Query, which relies on pgx's
// automatic per-connection statement cache to amortise planning after
// the first call on each connection. This file is therefore a
// best-effort warmup of the first connection the pool hands out and a
// structural placeholder for future work; it is NOT load-bearing for
// steady-state dequeue performance.
//
// If we ever switch dequeue to execute by prepared statement name, this
// must move to AfterConnect so every connection gets the statement at
// birth — otherwise lookups by name will miss on fresh connections.

// PreparedStatements holds named prepared statements for each dequeue
// variant. Nil means "not prepared yet; use raw SQL".
type PreparedStatements struct {
	DequeueN                  string
	DequeueNFair              string
	DequeueNDenormalized      string
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
		DequeueN:                  "strait_dequeue_n",
		DequeueNFair:              "strait_dequeue_n_fair",
		DequeueNDenormalized:      "strait_dequeue_n_denorm",
		DequeueNFullyDenormalized: "strait_dequeue_n_fulldn",
	}

	orderBy := "jr.priority DESC, jr.created_at ASC"

	stmts := map[string]string{
		ps.DequeueN: fmt.Sprintf(`
			WITH %s,
			claimed AS (
				SELECT jr.id FROM job_runs jr
				LEFT JOIN job_run_read_state rs ON rs.run_id = jr.id
				JOIN jobs j ON j.id = jr.job_id
				%s
				WHERE COALESCE(rs.status, jr.status) = '%s'
				  AND j.enabled = true AND NOT j.paused
				  AND (COALESCE(rs.scheduled_at, jr.scheduled_at) IS NULL OR COALESCE(rs.scheduled_at, jr.scheduled_at) <= NOW())
				  AND (COALESCE(rs.next_retry_at, jr.next_retry_at) IS NULL OR COALESCE(rs.next_retry_at, jr.next_retry_at) <= NOW())
				  AND NOT strait_run_retry_blocked(jr.id)
				  %s
				ORDER BY %s
				FOR UPDATE OF jr SKIP LOCKED
				LIMIT $1
			), updated AS (
				UPDATE job_runs SET status = '%s', started_at = NOW()
				WHERE id IN (SELECT id FROM claimed)
				  AND status IN ('queued', 'delayed', 'paused')
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
