package queue

import (
	"context"
	"errors"
	"testing"
	"time"

	"orchestrator/internal/domain"
	"orchestrator/internal/store"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// mockDBTX implements store.DBTX for unit testing queue operations.
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

// Verify mockDBTX implements store.DBTX at compile time.
var _ store.DBTX = (*mockDBTX)(nil)

// mockRow implements pgx.Row.
type mockRow struct {
	scanFn func(dest ...any) error
}

func (m *mockRow) Scan(dest ...any) error {
	if m.scanFn != nil {
		return m.scanFn(dest...)
	}
	return nil
}

func TestNewPostgresQueue(t *testing.T) {
	t.Parallel()
	q := NewPostgresQueue(nil)
	if q == nil {
		t.Fatal("NewPostgresQueue(nil) returned nil")
	}
}

func TestEnqueue_SetsDefaults(t *testing.T) {
	t.Parallel()
	db := &mockDBTX{
		queryRowFn: func(_ context.Context, _ string, args ...any) pgx.Row {
			return &mockRow{
				scanFn: func(dest ...any) error {
					if tp, ok := dest[0].(*time.Time); ok {
						*tp = time.Now()
					}
					return nil
				},
			}
		},
	}

	q := NewPostgresQueue(db)
	run := &domain.JobRun{
		JobID:     "job-1",
		ProjectID: "proj-1",
	}

	if err := q.Enqueue(context.Background(), run); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	if run.ID == "" {
		t.Error("ID should be auto-generated")
	}
	if run.Attempt != 1 {
		t.Errorf("Attempt = %d, want 1", run.Attempt)
	}
	if run.TriggeredBy != domain.TriggerManual {
		t.Errorf("TriggeredBy = %q, want %q", run.TriggeredBy, domain.TriggerManual)
	}
	if run.Status != domain.StatusQueued {
		t.Errorf("Status = %q, want %q", run.Status, domain.StatusQueued)
	}
	if run.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set after Enqueue")
	}
}

func TestEnqueue_PreservesExistingValues(t *testing.T) {
	t.Parallel()
	db := &mockDBTX{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{
				scanFn: func(dest ...any) error {
					if tp, ok := dest[0].(*time.Time); ok {
						*tp = time.Now()
					}
					return nil
				},
			}
		},
	}

	q := NewPostgresQueue(db)
	run := &domain.JobRun{
		ID:          "custom-id",
		JobID:       "job-1",
		ProjectID:   "proj-1",
		Attempt:     3,
		TriggeredBy: domain.TriggerCron,
	}

	if err := q.Enqueue(context.Background(), run); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	if run.ID != "custom-id" {
		t.Errorf("ID = %q, want %q (should preserve existing)", run.ID, "custom-id")
	}
	if run.Attempt != 3 {
		t.Errorf("Attempt = %d, want 3 (should preserve existing)", run.Attempt)
	}
	if run.TriggeredBy != domain.TriggerCron {
		t.Errorf("TriggeredBy = %q, want %q (should preserve existing)", run.TriggeredBy, domain.TriggerCron)
	}
}

func TestEnqueue_DelayedStatus(t *testing.T) {
	t.Parallel()
	db := &mockDBTX{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{
				scanFn: func(dest ...any) error {
					if tp, ok := dest[0].(*time.Time); ok {
						*tp = time.Now()
					}
					return nil
				},
			}
		},
	}

	q := NewPostgresQueue(db)
	future := time.Now().Add(time.Hour)
	run := &domain.JobRun{
		JobID:       "job-1",
		ProjectID:   "proj-1",
		ScheduledAt: &future,
	}

	if err := q.Enqueue(context.Background(), run); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	if run.Status != domain.StatusDelayed {
		t.Errorf("Status = %q, want %q for future ScheduledAt", run.Status, domain.StatusDelayed)
	}
}

func TestEnqueue_PastScheduleIsQueued(t *testing.T) {
	t.Parallel()
	db := &mockDBTX{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{
				scanFn: func(dest ...any) error {
					if tp, ok := dest[0].(*time.Time); ok {
						*tp = time.Now()
					}
					return nil
				},
			}
		},
	}

	q := NewPostgresQueue(db)
	past := time.Now().Add(-time.Hour)
	run := &domain.JobRun{
		JobID:       "job-1",
		ProjectID:   "proj-1",
		ScheduledAt: &past,
	}

	if err := q.Enqueue(context.Background(), run); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	if run.Status != domain.StatusQueued {
		t.Errorf("Status = %q, want %q for past ScheduledAt", run.Status, domain.StatusQueued)
	}
}

func TestEnqueue_DBError(t *testing.T) {
	t.Parallel()
	db := &mockDBTX{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{
				scanFn: func(_ ...any) error { return errors.New("connection refused") },
			}
		},
	}

	q := NewPostgresQueue(db)
	run := &domain.JobRun{
		JobID:     "job-1",
		ProjectID: "proj-1",
	}

	err := q.Enqueue(context.Background(), run)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestDequeue_NoRows(t *testing.T) {
	t.Parallel()
	db := &mockDBTX{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{
				scanFn: func(_ ...any) error { return pgx.ErrNoRows },
			}
		},
	}

	q := NewPostgresQueue(db)
	run, err := q.Dequeue(context.Background())
	if err != nil {
		t.Fatalf("Dequeue() error = %v, want nil for empty queue", err)
	}
	if run != nil {
		t.Errorf("Dequeue() run = %v, want nil for empty queue", run)
	}
}

func TestDequeue_DBError(t *testing.T) {
	t.Parallel()
	db := &mockDBTX{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{
				scanFn: func(_ ...any) error { return errors.New("deadlock detected") },
			}
		},
	}

	q := NewPostgresQueue(db)
	run, err := q.Dequeue(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if run != nil {
		t.Errorf("run = %v, want nil on error", run)
	}
}

func TestDequeueN_DBError(t *testing.T) {
	t.Parallel()
	db := &mockDBTX{
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			return nil, errors.New("connection timeout")
		},
	}

	q := NewPostgresQueue(db)
	runs, err := q.DequeueN(context.Background(), 5)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if runs != nil {
		t.Errorf("runs = %v, want nil on error", runs)
	}
}
