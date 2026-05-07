package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ctxTxKey is the unexported context key used to carry a per-request
// transaction through the call chain. Keeping it unexported means store
// callers cannot accidentally stash other values under the same key.
type ctxTxKey struct{}

// ContextWithTx returns a new context carrying the given transaction.
// Store methods called with this context will route their queries through
// the transaction rather than the pool, which is how per-request RLS
// tenant isolation is enforced: the middleware begins a tx, sets
// app.current_project_id on it, and every subsequent query inside the
// request runs on the same tx and therefore sees the same session variable.
func ContextWithTx(ctx context.Context, tx pgx.Tx) context.Context {
	return context.WithValue(ctx, ctxTxKey{}, tx)
}

// TxFromContext returns the transaction previously stored via ContextWithTx,
// or nil/false if none was set. Exported so middleware and tests can inspect
// the bound transaction without reaching into the store package internals.
func TxFromContext(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(ctxTxKey{}).(pgx.Tx)
	return tx, ok
}

// ctxAwareDBTX wraps a pool-level DBTX and transparently routes each call
// through the transaction in the context when one is present. Store methods
// continue to invoke q.db.Exec/Query/QueryRow unchanged; the wrapper picks
// the correct target at call time based on whether the ambient context has
// a bound transaction.
//
// This is the mechanism that makes Postgres RLS actually enforce tenant
// isolation under a connection pool. Without it, SetProjectContext's
// transaction-local set_config call has no effect on subsequent queries
// because each pool.Exec runs in its own implicit transaction.
type ctxAwareDBTX struct {
	pool DBTX
}

func (c *ctxAwareDBTX) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	if tx, ok := TxFromContext(ctx); ok {
		return tx.Exec(ctx, sql, arguments...)
	}
	return c.pool.Exec(ctx, sql, arguments...)
}

func (c *ctxAwareDBTX) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if tx, ok := TxFromContext(ctx); ok {
		return tx.Query(ctx, sql, args...)
	}
	return c.pool.Query(ctx, sql, args...)
}

func (c *ctxAwareDBTX) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if tx, ok := TxFromContext(ctx); ok {
		return tx.QueryRow(ctx, sql, args...)
	}
	return c.pool.QueryRow(ctx, sql, args...)
}

func (c *ctxAwareDBTX) Begin(ctx context.Context) (pgx.Tx, error) {
	beginner, ok := c.pool.(TxBeginner)
	if !ok {
		return nil, fmt.Errorf("underlying db does not support transactions")
	}
	return beginner.Begin(ctx)
}

func (c *ctxAwareDBTX) BeginTx(ctx context.Context, opts pgx.TxOptions) (pgx.Tx, error) {
	beginner, ok := c.pool.(TxBeginnerOptions)
	if !ok {
		return nil, fmt.Errorf("underlying db does not support transaction options")
	}
	return beginner.BeginTx(ctx, opts)
}

// NewWithContextRouting constructs a Queries whose underlying DBTX routes
// through a per-request transaction when the caller's context carries one,
// and otherwise falls through to the given pool. Use this constructor for
// the API server, where the rls middleware wraps each request in a tx.
// Use the plain New constructor for worker / scheduler processes that do
// not have per-request scope.
func NewWithContextRouting(pool DBTX) *Queries {
	return New(&ctxAwareDBTX{pool: pool})
}
