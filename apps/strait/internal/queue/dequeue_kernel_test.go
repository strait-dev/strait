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

func TestExecuteDequeue_CommitFailureReturnsError(t *testing.T) {
	t.Parallel()

	commitErr := errors.New("connection reset by peer")
	callCount := 0
	tx := &mockTx{
		mockDBTX: mockDBTX{
			execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
				return pgconn.CommandTag{}, nil
			},
			queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
				return &emptyRows{}, nil
			},
		},
		commitFn: func(_ context.Context) error {
			callCount++
			return commitErr
		},
		rollbackFn: func(_ context.Context) error {
			return nil
		},
	}
	db := &mockTxDBTX{
		beginFn: func(_ context.Context) (pgx.Tx, error) {
			return tx, nil
		},
	}
	q := NewPostgresQueue(db, WithStatementTimeout(5*time.Second))

	runs, err := executeDequeue(context.Background(), q, 10, dequeueSpec{
		spanName:            "test.commit_fail",
		candidatesSQL:       "SELECT jr.id FROM job_runs jr LIMIT $1",
		skipConcurrencyCTEs: true,
	})
	if err == nil {
		t.Fatal("expected error from failed commit")
	}
	if !errors.Is(err, commitErr) {
		t.Errorf("error = %v, want wrapping %v", err, commitErr)
	}
	if runs != nil {
		t.Errorf("runs should be nil on commit failure, got %d runs", len(runs))
	}
	if callCount != 1 {
		t.Errorf("commit called %d times, want 1", callCount)
	}
}

func TestExecuteDequeueFair_CommitFailureReturnsError(t *testing.T) {
	t.Parallel()

	commitErr := errors.New("connection reset by peer")
	tx := &mockTx{
		mockDBTX: mockDBTX{
			execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
				return pgconn.CommandTag{}, nil
			},
			queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
				return &emptyRows{}, nil
			},
		},
		commitFn: func(_ context.Context) error {
			return commitErr
		},
		rollbackFn: func(_ context.Context) error {
			return nil
		},
	}
	db := &mockTxDBTX{
		beginFn: func(_ context.Context) (pgx.Tx, error) {
			return tx, nil
		},
	}
	q := NewPostgresQueue(db, WithStatementTimeout(5*time.Second))

	runs, err := executeDequeueFair(context.Background(), q, 10, dequeueSpec{
		spanName: "test.fair_commit_fail",
		candidatesSQL: `SELECT DISTINCT ON (jr.job_id) jr.id
			FROM job_runs jr
			ORDER BY jr.job_id, jr.created_at ASC`,
	})
	if err == nil {
		t.Fatal("expected error from failed commit")
	}
	if !errors.Is(err, commitErr) {
		t.Errorf("error = %v, want wrapping %v", err, commitErr)
	}
	if runs != nil {
		t.Errorf("runs should be nil on commit failure, got %d runs", len(runs))
	}
}

// emptyRows implements pgx.Rows returning zero rows.
type emptyRows struct{}

func (e *emptyRows) Close()                                       {}
func (e *emptyRows) Err() error                                   { return nil }
func (e *emptyRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (e *emptyRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (e *emptyRows) Next() bool                                   { return false }
func (e *emptyRows) Scan(_ ...any) error                          { return errors.New("no rows") }
func (e *emptyRows) Values() ([]any, error)                       { return nil, nil }
func (e *emptyRows) RawValues() [][]byte                          { return nil }
func (e *emptyRows) Conn() *pgx.Conn                              { return nil }

// singleRunRows implements pgx.Rows yielding exactly one row with the given
// ID (scan position 0) and CreatedAt (scan position 21), matching the field
// order in dbscan.ScanRun. All other fields are left at zero values.
type singleRunRows struct {
	id        string
	createdAt time.Time
	done      bool
}

func (r *singleRunRows) Close()                                       {}
func (r *singleRunRows) Err() error                                   { return nil }
func (r *singleRunRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *singleRunRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *singleRunRows) Next() bool {
	if r.done {
		return false
	}
	r.done = true
	return true
}
func (r *singleRunRows) Scan(dest ...any) error {
	if p, ok := dest[0].(*string); ok {
		*p = r.id
	}
	if p, ok := dest[21].(*time.Time); ok {
		*p = r.createdAt
	}
	return nil
}
func (r *singleRunRows) Values() ([]any, error) { return nil, nil }
func (r *singleRunRows) RawValues() [][]byte    { return nil }
func (r *singleRunRows) Conn() *pgx.Conn        { return nil }

func TestExecuteDequeue_CommitFailureDoesNotCallPostScanFn(t *testing.T) {
	t.Parallel()

	commitErr := errors.New("connection lost")
	tx := &mockTx{
		mockDBTX: mockDBTX{
			execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
				return pgconn.CommandTag{}, nil
			},
			queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
				return &emptyRows{}, nil
			},
		},
		commitFn:   func(_ context.Context) error { return commitErr },
		rollbackFn: func(_ context.Context) error { return nil },
	}
	db := &mockTxDBTX{
		beginFn: func(_ context.Context) (pgx.Tx, error) { return tx, nil },
	}
	q := NewPostgresQueue(db, WithStatementTimeout(5*time.Second))

	postScanCalled := false
	_, err := executeDequeue(context.Background(), q, 10, dequeueSpec{
		spanName:            "test.postscan_commit_fail",
		candidatesSQL:       "SELECT jr.id FROM job_runs jr LIMIT $1",
		skipConcurrencyCTEs: true,
		postScanFn: func(_ []domain.JobRun) error {
			postScanCalled = true
			return nil
		},
	})
	if err == nil {
		t.Fatal("expected error from failed commit")
	}
	if postScanCalled {
		t.Error("postScanFn must not be called when commit fails")
	}
}

func TestExecuteDequeueFair_CommitFailureDoesNotCallPostScanFn(t *testing.T) {
	t.Parallel()

	commitErr := errors.New("connection lost")
	tx := &mockTx{
		mockDBTX: mockDBTX{
			execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
				return pgconn.CommandTag{}, nil
			},
			queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
				return &emptyRows{}, nil
			},
		},
		commitFn:   func(_ context.Context) error { return commitErr },
		rollbackFn: func(_ context.Context) error { return nil },
	}
	db := &mockTxDBTX{
		beginFn: func(_ context.Context) (pgx.Tx, error) { return tx, nil },
	}
	q := NewPostgresQueue(db, WithStatementTimeout(5*time.Second))

	postScanCalled := false
	_, err := executeDequeueFair(context.Background(), q, 10, dequeueSpec{
		spanName: "test.fair_postscan_commit_fail",
		candidatesSQL: `SELECT DISTINCT ON (jr.job_id) jr.id
			FROM job_runs jr
			ORDER BY jr.job_id, jr.created_at ASC`,
		postScanFn: func(_ []domain.JobRun) error {
			postScanCalled = true
			return nil
		},
	})
	if err == nil {
		t.Fatal("expected error from failed commit")
	}
	if postScanCalled {
		t.Error("postScanFn must not be called when commit fails")
	}
}

func TestExecuteDequeue_CommitFailureCursorUnchanged(t *testing.T) {
	t.Parallel()

	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cursor := NewClaimCursor(60 * time.Second)
	cursor.Advance(t0, "run-pre")

	createdPre, idPre, okPre := cursor.Snapshot()
	if !okPre || idPre != "run-pre" {
		t.Fatalf("cursor setup failed: ok=%v id=%q", okPre, idPre)
	}

	commitErr := errors.New("connection lost")
	tx := &mockTx{
		mockDBTX: mockDBTX{
			execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
				return pgconn.CommandTag{}, nil
			},
			queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
				return &singleRunRows{
					id:        "run-new",
					createdAt: t0.Add(time.Hour),
				}, nil
			},
		},
		commitFn:   func(_ context.Context) error { return commitErr },
		rollbackFn: func(_ context.Context) error { return nil },
	}
	db := &mockTxDBTX{
		beginFn: func(_ context.Context) (pgx.Tx, error) { return tx, nil },
	}
	q := NewPostgresQueue(db, WithStatementTimeout(5*time.Second))

	_, err := executeDequeue(context.Background(), q, 10, dequeueSpec{
		spanName:            "test.cursor_commit_fail",
		candidatesSQL:       "SELECT jr.id FROM job_runs jr LIMIT $1",
		skipConcurrencyCTEs: true,
		postScanFn: func(runs []domain.JobRun) error {
			for i := range runs {
				cursor.Advance(runs[i].CreatedAt, runs[i].ID)
			}
			return nil
		},
	})
	if err == nil {
		t.Fatal("expected error from failed commit")
	}

	createdPost, idPost, okPost := cursor.Snapshot()
	if !okPost {
		t.Fatal("cursor should still be valid after commit failure")
	}
	if idPost != idPre {
		t.Errorf("cursor ID changed from %q to %q on commit failure", idPre, idPost)
	}
	if !createdPost.Equal(createdPre) {
		t.Errorf("cursor CreatedAt changed from %v to %v on commit failure", createdPre, createdPost)
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
