package queue

import (
	"context"
	"errors"
	"strings"
	"testing"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
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
