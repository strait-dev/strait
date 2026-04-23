package store

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/sourcegraph/conc"
)

// fakeTx is a minimal pgx.Tx test double. Only Exec, Query, and QueryRow
// are implemented — any other method call panics via the nil embedded
// interface, which is the desired behavior in unit tests (any unexpected
// call is a test bug).
type fakeTx struct {
	pgx.Tx
	execCalls     int
	queryCalls    int
	queryRowCalls int
	execFn        func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	queryFn       func(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	queryRowFn    func(ctx context.Context, sql string, args ...any) pgx.Row
}

func (f *fakeTx) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	f.execCalls++
	if f.execFn != nil {
		return f.execFn(ctx, sql, args...)
	}
	return pgconn.CommandTag{}, nil
}

func (f *fakeTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	f.queryCalls++
	if f.queryFn != nil {
		return f.queryFn(ctx, sql, args...)
	}
	return nil, nil
}

func (f *fakeTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	f.queryRowCalls++
	if f.queryRowFn != nil {
		return f.queryRowFn(ctx, sql, args...)
	}
	return &mockRow{}
}

func TestCtxAwareDBTX_NoContext_RoutesToPool(t *testing.T) {
	t.Parallel()

	poolCalls := 0
	pool := &mockDBTX{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			poolCalls++
			return pgconn.CommandTag{}, nil
		},
	}
	wrapper := &ctxAwareDBTX{pool: pool}

	ctx := context.Background() // no tx bound
	if _, err := wrapper.Exec(ctx, "SELECT 1"); err != nil {
		t.Fatalf("Exec error = %v", err)
	}
	if poolCalls != 1 {
		t.Fatalf("pool Exec called %d times, want 1", poolCalls)
	}
}

func TestCtxAwareDBTX_WithContext_RoutesToTx(t *testing.T) {
	t.Parallel()

	poolCalls := 0
	pool := &mockDBTX{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			poolCalls++
			return pgconn.CommandTag{}, nil
		},
	}
	tx := &fakeTx{}
	wrapper := &ctxAwareDBTX{pool: pool}

	ctx := ContextWithTx(context.Background(), tx)
	if _, err := wrapper.Exec(ctx, "SELECT 1"); err != nil {
		t.Fatalf("Exec error = %v", err)
	}
	rows, err := wrapper.Query(ctx, "SELECT 2")
	if err != nil {
		t.Fatalf("Query error = %v", err)
	}
	if rows != nil {
		rows.Close()
	}
	_ = wrapper.QueryRow(ctx, "SELECT 3")

	if tx.execCalls != 1 {
		t.Fatalf("tx Exec calls = %d, want 1", tx.execCalls)
	}
	if tx.queryCalls != 1 {
		t.Fatalf("tx Query calls = %d, want 1", tx.queryCalls)
	}
	if tx.queryRowCalls != 1 {
		t.Fatalf("tx QueryRow calls = %d, want 1", tx.queryRowCalls)
	}
	if poolCalls != 0 {
		t.Fatalf("pool Exec calls = %d, want 0 (should route to tx)", poolCalls)
	}
}

func TestCtxAwareDBTX_ContextsAreIndependent(t *testing.T) {
	t.Parallel()

	// A single ctxAwareDBTX instance serving two concurrent "requests" with
	// different contexts must route each request to its own tx, never the
	// pool, and never cross-contaminate.
	pool := &mockDBTX{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			t.Errorf("pool should never be called when every request has a tx")
			return pgconn.CommandTag{}, nil
		},
	}
	wrapper := &ctxAwareDBTX{pool: pool}

	txA := &fakeTx{}
	txB := &fakeTx{}
	ctxA := ContextWithTx(context.Background(), txA)
	ctxB := ContextWithTx(context.Background(), txB)

	var wg conc.WaitGroup
	const iters = 50
	wg.Go(func() {
		for range iters {
			_, _ = wrapper.Exec(ctxA, "SELECT 'a'")
		}
	})
	wg.Go(func() {
		for range iters {
			_, _ = wrapper.Exec(ctxB, "SELECT 'b'")
		}
	})
	wg.Wait()

	if txA.execCalls != iters {
		t.Fatalf("txA exec calls = %d, want %d", txA.execCalls, iters)
	}
	if txB.execCalls != iters {
		t.Fatalf("txB exec calls = %d, want %d", txB.execCalls, iters)
	}
}

func TestCtxAwareDBTX_TxErrorPropagates(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("tx blew up")
	tx := &fakeTx{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, sentinel
		},
	}
	wrapper := &ctxAwareDBTX{pool: &mockDBTX{}}
	ctx := ContextWithTx(context.Background(), tx)

	_, err := wrapper.Exec(ctx, "SELECT 1")
	if !errors.Is(err, sentinel) {
		t.Fatalf("error = %v, want %v", err, sentinel)
	}
}

func TestNewWithContextRouting_FallsThroughToPoolWithoutTx(t *testing.T) {
	t.Parallel()

	pool := &mockDBTX{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, nil
		},
	}
	q := NewWithContextRouting(pool)

	// Call a store method that issues q.db.Exec under the hood. Use a simple
	// generic path: SetProjectContext runs q.db.Exec.
	if err := q.SetProjectContext(context.Background(), "any-project"); err != nil {
		t.Fatalf("SetProjectContext error = %v", err)
	}
}

func TestTxFromContext_MissingReturnsFalse(t *testing.T) {
	t.Parallel()

	_, ok := TxFromContext(context.Background())
	if ok {
		t.Fatal("expected no tx in bare context")
	}
}
