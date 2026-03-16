package store

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestNew(t *testing.T) {
	t.Parallel()
	q := New(nil)
	if q == nil {
		t.Fatal("New(nil) returned nil")
	}
}

func TestSentinelErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		msg  string
	}{
		{"ErrJobNotFound", ErrJobNotFound, "job not found"},
		{"ErrRunNotFound", ErrRunNotFound, "run not found"},
		{"ErrRunConflict", ErrRunConflict, "run status update conflict"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.err.Error() != tt.msg {
				t.Errorf("Error() = %q, want %q", tt.err.Error(), tt.msg)
			}
		})
	}
}

func TestSentinelErrors_Wrapping(t *testing.T) {
	t.Parallel()
	sentinels := []error{ErrJobNotFound, ErrRunNotFound, ErrRunConflict}
	for _, sentinel := range sentinels {
		t.Run(sentinel.Error(), func(t *testing.T) {
			wrapped := fmt.Errorf("outer: %w", sentinel)
			if !errors.Is(wrapped, sentinel) {
				t.Errorf("wrapped error should match %v via errors.Is", sentinel)
			}
		})
	}
}

func TestSentinelErrors_NotEqual(t *testing.T) {
	t.Parallel()
	if errors.Is(ErrJobNotFound, ErrRunNotFound) {
		t.Error("ErrJobNotFound should not equal ErrRunNotFound")
	}
	if errors.Is(ErrRunNotFound, ErrRunConflict) {
		t.Error("ErrRunNotFound should not equal ErrRunConflict")
	}
}

// mockDBTX implements DBTX for unit testing store queries.
type mockDBTX struct {
	execFn     func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	queryFn    func(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	queryRowFn func(ctx context.Context, sql string, args ...any) pgx.Row
}

func (m *mockDBTX) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if m.execFn != nil {
		return m.execFn(ctx, sql, args...)
	}
	return pgconn.CommandTag{}, nil
}

func (m *mockDBTX) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if m.queryFn != nil {
		return m.queryFn(ctx, sql, args...)
	}
	return nil, nil
}

func (m *mockDBTX) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if m.queryRowFn != nil {
		return m.queryRowFn(ctx, sql, args...)
	}
	return &mockRow{}
}

// mockRow implements pgx.Row for unit testing.
type mockRow struct {
	scanFn func(dest ...any) error
}

func (m *mockRow) Scan(dest ...any) error {
	if m.scanFn != nil {
		return m.scanFn(dest...)
	}
	return nil
}

func TestUpdateRunStatus_IdempotentSameTarget(t *testing.T) {
	t.Parallel()

	callCount := 0
	db := &mockDBTX{
		execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			// Simulate 0 rows affected (optimistic lock miss)
			return pgconn.NewCommandTag("UPDATE 0"), nil
		},
		queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
			callCount++
			return &mockRow{
				scanFn: func(dest ...any) error {
					// Re-read finds run already in target state
					if p, ok := dest[0].(*domain.RunStatus); ok {
						*p = domain.StatusCompleted
					}
					return nil
				},
			}
		},
	}

	q := New(db)
	err := q.UpdateRunStatus(context.Background(), "run-1", domain.StatusExecuting, domain.StatusCompleted, nil)
	if err != nil {
		t.Fatalf("expected nil (idempotent), got %v", err)
	}
}

func TestUpdateRunStatus_ConflictDifferentTarget(t *testing.T) {
	t.Parallel()

	db := &mockDBTX{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("UPDATE 0"), nil
		},
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{
				scanFn: func(dest ...any) error {
					if p, ok := dest[0].(*domain.RunStatus); ok {
						*p = domain.StatusFailed // different from target (completed)
					}
					return nil
				},
			}
		},
	}

	q := New(db)
	err := q.UpdateRunStatus(context.Background(), "run-1", domain.StatusExecuting, domain.StatusCompleted, nil)
	if !errors.Is(err, ErrRunConflict) {
		t.Fatalf("expected ErrRunConflict, got %v", err)
	}
}

func TestUpdateRunStatus_NotFound(t *testing.T) {
	t.Parallel()

	db := &mockDBTX{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("UPDATE 0"), nil
		},
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{
				scanFn: func(_ ...any) error {
					return pgx.ErrNoRows
				},
			}
		},
	}

	q := New(db)
	err := q.UpdateRunStatus(context.Background(), "run-nonexistent", domain.StatusExecuting, domain.StatusCompleted, nil)
	if err == nil {
		t.Fatal("expected error for non-existent run, got nil")
	}
}

func TestUpdateRunStatus_NormalTransition(t *testing.T) {
	t.Parallel()

	db := &mockDBTX{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("UPDATE 1"), nil
		},
	}

	q := New(db)
	err := q.UpdateRunStatus(context.Background(), "run-1", domain.StatusExecuting, domain.StatusCompleted, nil)
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestQueueStats_Success(t *testing.T) {
	t.Parallel()
	db := &mockDBTX{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{
				scanFn: func(dest ...any) error {
					*dest[0].(*int) = 5
					*dest[1].(*int) = 3
					*dest[2].(*int) = 2
					return nil
				},
			}
		},
	}

	q := New(db)
	stats, err := q.QueueStats(context.Background())
	if err != nil {
		t.Fatalf("QueueStats() error = %v", err)
	}
	if stats.Queued != 5 {
		t.Errorf("Queued = %d, want 5", stats.Queued)
	}
	if stats.Executing != 3 {
		t.Errorf("Executing = %d, want 3", stats.Executing)
	}
	if stats.Delayed != 2 {
		t.Errorf("Delayed = %d, want 2", stats.Delayed)
	}
}

func TestQueueStats_ZeroValues(t *testing.T) {
	t.Parallel()
	db := &mockDBTX{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{
				scanFn: func(dest ...any) error {
					*dest[0].(*int) = 0
					*dest[1].(*int) = 0
					*dest[2].(*int) = 0
					return nil
				},
			}
		},
	}

	q := New(db)
	stats, err := q.QueueStats(context.Background())
	if err != nil {
		t.Fatalf("QueueStats() error = %v", err)
	}
	if stats.Queued != 0 || stats.Executing != 0 || stats.Delayed != 0 {
		t.Errorf("expected all zeros, got queued=%d executing=%d delayed=%d",
			stats.Queued, stats.Executing, stats.Delayed)
	}
}

func TestQueueStats_DBError(t *testing.T) {
	t.Parallel()
	db := &mockDBTX{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{
				scanFn: func(_ ...any) error { return errors.New("connection refused") },
			}
		},
	}

	q := New(db)
	stats, err := q.QueueStats(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if stats != nil {
		t.Errorf("stats = %v, want nil on error", stats)
	}
}
