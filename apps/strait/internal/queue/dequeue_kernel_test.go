package queue

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestExecuteDequeue_ZeroN(t *testing.T) {
	t.Parallel()
	q := NewPostgresQueue(&mockDBTX{})
	runs, err := executeDequeue(context.Background(), q, 0, dequeueSpec{
		spanName:      "test.zero",
		candidatesSQL: "SELECT 1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("expected 0 runs, got %d", len(runs))
	}
}

func TestExecuteDequeue_NegativeN(t *testing.T) {
	t.Parallel()
	q := NewPostgresQueue(&mockDBTX{})
	runs, err := executeDequeue(context.Background(), q, -1, dequeueSpec{
		spanName:      "test.neg",
		candidatesSQL: "SELECT 1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("expected 0 runs, got %d", len(runs))
	}
}

func TestExecuteDequeue_SQLShapeWithCTEs(t *testing.T) {
	t.Parallel()
	var captured string
	db := &mockDBTX{
		queryFn: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
			captured = sql
			return nil, errors.New("captured")
		},
	}
	q := NewPostgresQueue(db)
	_, _ = executeDequeue(context.Background(), q, 5, dequeueSpec{
		spanName: "test.cte",
		candidatesSQL: `SELECT jr.id FROM job_runs jr
			ORDER BY jr.created_at ASC
			FOR UPDATE OF jr SKIP LOCKED
			LIMIT $1`,
	})
	assertDequeueKernelShape(t, captured, true)
}

func TestExecuteDequeue_SQLShapeSkipCTEs(t *testing.T) {
	t.Parallel()
	var captured string
	db := &mockDBTX{
		queryFn: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
			captured = sql
			return nil, errors.New("captured")
		},
	}
	q := NewPostgresQueue(db)
	_, _ = executeDequeue(context.Background(), q, 5, dequeueSpec{
		spanName:            "test.nocte",
		skipConcurrencyCTEs: true,
		candidatesSQL: `SELECT jr.id FROM job_runs jr
			ORDER BY jr.created_at ASC
			FOR UPDATE OF jr SKIP LOCKED
			LIMIT $1`,
	})
	assertDequeueKernelShape(t, captured, false)
}

func TestExecuteDequeueFair_SQLShape(t *testing.T) {
	t.Parallel()
	var captured string
	db := &mockDBTX{
		queryFn: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
			captured = sql
			return nil, errors.New("captured")
		},
	}
	q := NewPostgresQueue(db)
	_, _ = executeDequeueFair(context.Background(), q, 5, dequeueSpec{
		spanName: "test.fair",
		candidatesSQL: `SELECT DISTINCT ON (jr.job_id) jr.id
			FROM job_runs jr
			ORDER BY jr.job_id, jr.created_at ASC`,
	})
	assertDequeueKernelShape(t, captured, true)
	if !strings.Contains(captured, "candidates") {
		t.Error("fair SQL should contain 'candidates' CTE")
	}
}

func TestExecuteDequeue_PostScanFnCalled(t *testing.T) {
	t.Parallel()
	db := &mockDBTX{
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			return nil, errors.New("no rows")
		},
	}
	q := NewPostgresQueue(db)

	called := false
	_, _ = executeDequeue(context.Background(), q, 5, dequeueSpec{
		spanName:      "test.postscan",
		candidatesSQL: "SELECT 1 LIMIT $1",
		postScanFn: func(runs []domain.JobRun) error {
			called = true
			return nil
		},
	})
	// postScanFn is only called on success, so with the error above it won't be called.
	if called {
		t.Error("postScanFn should not be called when query errors")
	}
}

func TestWithStatementTimeout_NoTxBeginner(t *testing.T) {
	t.Parallel()
	q := NewPostgresQueue(&mockDBTX{}, WithStatementTimeout(5*time.Second))
	db, tx, err := withStatementTimeout(context.Background(), q, "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tx != nil {
		t.Error("should return nil tx when DBTX does not implement TxBeginner")
	}
	if db != q.db {
		t.Error("should return original db when DBTX does not implement TxBeginner")
	}
}

func TestWithStatementTimeout_ZeroDuration(t *testing.T) {
	t.Parallel()
	q := NewPostgresQueue(&mockDBTX{})
	db, tx, err := withStatementTimeout(context.Background(), q, "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tx != nil {
		t.Error("should return nil tx when timeout is zero")
	}
	if db != q.db {
		t.Error("should return original db when timeout is zero")
	}
}

func TestWithStatementTimeout_BeginError(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("begin failed")
	db := &mockTxDBTX{
		beginFn: func(_ context.Context) (pgx.Tx, error) {
			return nil, wantErr
		},
	}
	q := NewPostgresQueue(db, WithStatementTimeout(5*time.Second))
	_, _, err := withStatementTimeout(context.Background(), q, "test.begin")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("got %v, want %v", err, wantErr)
	}
}

func TestExecuteDequeue_LargeN(t *testing.T) {
	t.Parallel()
	var captured []any
	db := &mockDBTX{
		queryFn: func(_ context.Context, _ string, args ...any) (pgx.Rows, error) {
			captured = args
			return nil, errors.New("captured")
		},
	}
	q := NewPostgresQueue(db)
	_, _ = executeDequeue(context.Background(), q, 1<<30, dequeueSpec{
		spanName:      "test.large",
		candidatesSQL: "SELECT 1 LIMIT $1",
	})
	if len(captured) == 0 {
		t.Fatal("query was not called")
	}
	if n, ok := captured[0].(int); !ok || n != 1<<30 {
		t.Errorf("N arg = %v, want %d", captured[0], 1<<30)
	}
}

func TestExecuteDequeue_ExtraArgsPassedThrough(t *testing.T) {
	t.Parallel()
	var captured []any
	db := &mockDBTX{
		queryFn: func(_ context.Context, _ string, args ...any) (pgx.Rows, error) {
			captured = args
			return nil, errors.New("captured")
		},
	}
	q := NewPostgresQueue(db)
	_, _ = executeDequeue(context.Background(), q, 5, dequeueSpec{
		spanName:      "test.extra",
		candidatesSQL: "SELECT 1 WHERE project_id = $2 LIMIT $1",
		extraArgs:     []any{"proj_123"},
	})
	if len(captured) != 2 {
		t.Fatalf("expected 2 args, got %d: %v", len(captured), captured)
	}
	if captured[1] != "proj_123" {
		t.Errorf("extra arg = %v, want proj_123", captured[1])
	}
}

func TestWithStatementTimeout_ReturnsTxForExplicitCommit(t *testing.T) {
	t.Parallel()
	commitErr := errors.New("connection lost")
	tx := &mockTx{
		mockDBTX: mockDBTX{
			execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
				return pgconn.CommandTag{}, nil
			},
		},
		commitFn: func(_ context.Context) error {
			return commitErr
		},
	}
	db := &mockTxDBTX{
		beginFn: func(_ context.Context) (pgx.Tx, error) {
			return tx, nil
		},
	}
	q := NewPostgresQueue(db, WithStatementTimeout(5*time.Second))
	_, txOut, err := withStatementTimeout(context.Background(), q, "test.commit")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if txOut == nil {
		t.Fatal("expected non-nil tx for explicit commit control")
	}
	if err := txOut.Commit(context.Background()); !errors.Is(err, commitErr) {
		t.Errorf("commit error = %v, want %v", err, commitErr)
	}
}

func assertDequeueKernelShape(t *testing.T, sql string, expectCTEs bool) {
	t.Helper()
	if sql == "" {
		t.Fatal("SQL was not captured")
	}
	for _, required := range []string{"claimed", "updated", "status", "started_at"} {
		if !strings.Contains(sql, required) {
			t.Errorf("SQL missing %q:\n%s", required, sql)
		}
	}
	if expectCTEs {
		if !strings.Contains(sql, "active_by_job") {
			t.Errorf("expected concurrency CTEs, SQL:\n%s", sql)
		}
	} else {
		if strings.Contains(sql, "active_by_job") {
			t.Errorf("did not expect concurrency CTEs, SQL:\n%s", sql)
		}
	}
}
