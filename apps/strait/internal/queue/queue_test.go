package queue

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

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

func TestDequeueN_QueryUsesPriorityAgingWhenEnabled(t *testing.T) {
	t.Parallel()

	var query string
	db := &mockDBTX{
		queryFn: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
			query = sql
			return nil, errors.New("forced query error")
		},
	}

	q := NewPostgresQueue(db, WithPriorityAging(true))
	_, _ = q.DequeueN(context.Background(), 10)

	if !strings.Contains(query, "jr.priority + EXTRACT(EPOCH FROM (NOW() - jr.created_at)) / 3600") {
		t.Fatalf("DequeueN() query missing priority aging formula: %s", query)
	}
}

func TestDequeueNByProject_QueryUsesPriorityAgingWhenEnabled(t *testing.T) {
	t.Parallel()

	var query string
	db := &mockDBTX{
		queryFn: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
			query = sql
			return nil, errors.New("forced query error")
		},
	}

	q := NewPostgresQueue(db, WithPriorityAging(true))
	_, _ = q.DequeueNByProject(context.Background(), 10, "proj-1")

	if !strings.Contains(query, "jr.priority + EXTRACT(EPOCH FROM (NOW() - jr.created_at)) / 3600") {
		t.Fatalf("DequeueNByProject() query missing priority aging formula: %s", query)
	}
}

func TestDequeueN_QueryUsesStaticPriorityWhenAgingDisabled(t *testing.T) {
	t.Parallel()

	var query string
	db := &mockDBTX{
		queryFn: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
			query = sql
			return nil, errors.New("forced query error")
		},
	}

	q := NewPostgresQueue(db, WithPriorityAging(false))
	_, _ = q.DequeueN(context.Background(), 10)

	if !strings.Contains(query, "ORDER BY jr.priority DESC, jr.created_at ASC") {
		t.Fatalf("DequeueN() query missing static priority ordering: %s", query)
	}
	if strings.Contains(query, "EXTRACT(EPOCH FROM (NOW() - jr.created_at)) / 3600") {
		t.Fatalf("DequeueN() query unexpectedly contains priority aging formula: %s", query)
	}
}

func TestWithStatementTimeout_Default(t *testing.T) {
	t.Parallel()

	db := &mockDBTX{}
	q := NewPostgresQueue(db)
	if q.statementTimeout != 0 {
		t.Fatalf("expected zero timeout by default, got %v", q.statementTimeout)
	}
}

func TestWithStatementTimeout_Custom(t *testing.T) {
	t.Parallel()

	db := &mockDBTX{}
	q := NewPostgresQueue(db, WithStatementTimeout(15*time.Second))
	if q.statementTimeout != 15*time.Second {
		t.Fatalf("expected 15s timeout, got %v", q.statementTimeout)
	}
}

func TestDequeueN_SetsStatementTimeout(t *testing.T) {
	t.Parallel()

	var execSQL string
	db := &mockDBTX{
		execFn: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
			execSQL = sql
			return pgconn.CommandTag{}, nil
		},
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			return nil, errors.New("no rows") // short-circuit
		},
	}

	q := NewPostgresQueue(db, WithStatementTimeout(30*time.Second))
	_, _ = q.DequeueN(context.Background(), 5)

	if !strings.Contains(execSQL, "SET LOCAL statement_timeout = 30000") {
		t.Fatalf("expected SET LOCAL statement_timeout = 30000, got %q", execSQL)
	}
}

func TestDequeueN_NoTimeoutWhenZero(t *testing.T) {
	t.Parallel()

	var execCalled bool
	db := &mockDBTX{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			execCalled = true
			return pgconn.CommandTag{}, nil
		},
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			return nil, errors.New("no rows")
		},
	}

	q := NewPostgresQueue(db) // no timeout set
	_, _ = q.DequeueN(context.Background(), 5)

	if execCalled {
		t.Fatal("expected no Exec call when statement timeout is zero")
	}
}

func TestDequeueN_SkipsDisabledJobs(t *testing.T) {
	t.Parallel()

	var capturedQuery string
	db := &mockDBTX{
		queryFn: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
			capturedQuery = sql
			return nil, errors.New("forced query error")
		},
	}

	q := NewPostgresQueue(db)
	_, _ = q.DequeueN(context.Background(), 10)

	if !strings.Contains(capturedQuery, "j.enabled = true") {
		t.Fatalf("DequeueN() query missing j.enabled = true filter: %s", capturedQuery)
	}
}

func TestDequeue_SkipsDisabledJobs(t *testing.T) {
	t.Parallel()

	var capturedQuery string
	db := &mockDBTX{
		queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
			capturedQuery = sql
			return &mockRow{
				scanFn: func(_ ...any) error { return pgx.ErrNoRows },
			}
		},
	}

	q := NewPostgresQueue(db)
	_, _ = q.Dequeue(context.Background())

	if !strings.Contains(capturedQuery, "j.enabled = true") {
		t.Fatalf("Dequeue() query missing j.enabled = true filter: %s", capturedQuery)
	}
}

func TestDequeueNByProject_SkipsDisabledJobs(t *testing.T) {
	t.Parallel()

	var capturedQuery string
	db := &mockDBTX{
		queryFn: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
			capturedQuery = sql
			return nil, errors.New("forced query error")
		},
	}

	q := NewPostgresQueue(db)
	_, _ = q.DequeueNByProject(context.Background(), 10, "proj-1")

	if !strings.Contains(capturedQuery, "j.enabled = true") {
		t.Fatalf("DequeueNByProject() query missing j.enabled = true filter: %s", capturedQuery)
	}
}

func TestLoad_DefaultStatementTimeout(t *testing.T) {
	// This is tested via config_test.go - just verify the option works
	q := NewPostgresQueue(&mockDBTX{}, WithStatementTimeout(30*time.Second))
	if q.statementTimeout != 30*time.Second {
		t.Fatalf("expected 30s, got %v", q.statementTimeout)
	}
}

func TestCopyFromColumnsIncludesMetadata(t *testing.T) {
	t.Parallel()
	if !slices.Contains(copyFromColumns, "metadata") {
		t.Fatalf("copyFromColumns does not contain \"metadata\"; columns = %v", copyFromColumns)
	}
}

func TestEnqueue_TagsJSON_NonEmpty(t *testing.T) {
	t.Parallel()
	var capturedArgs []any
	db := &mockDBTX{
		queryRowFn: func(_ context.Context, _ string, args ...any) pgx.Row {
			capturedArgs = args
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
		Tags:      map[string]string{"env": "prod", "team": "core"},
	}

	if err := q.Enqueue(context.Background(), run); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	tagsArg, ok := capturedArgs[23].([]byte)
	if !ok {
		t.Fatalf("arg[23] (tags) type = %T, want []byte", capturedArgs[23])
	}
	if string(tagsArg) == "{}" {
		t.Error("tags JSON should not be empty when run.Tags is non-empty")
	}
	if !strings.Contains(string(tagsArg), "env") {
		t.Errorf("tags JSON missing key 'env': %s", string(tagsArg))
	}
}

func TestEnqueue_TagsJSON_Empty(t *testing.T) {
	t.Parallel()
	var capturedArgs []any
	db := &mockDBTX{
		queryRowFn: func(_ context.Context, _ string, args ...any) pgx.Row {
			capturedArgs = args
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

	tagsArg, ok := capturedArgs[23].([]byte)
	if !ok {
		t.Fatalf("arg[23] (tags) type = %T, want []byte", capturedArgs[23])
	}
	if string(tagsArg) != "{}" {
		t.Errorf("tags JSON should be '{}' for nil tags, got %s", string(tagsArg))
	}
}

func TestEnqueue_MetadataJSON_NonEmpty(t *testing.T) {
	t.Parallel()
	var capturedArgs []any
	db := &mockDBTX{
		queryRowFn: func(_ context.Context, _ string, args ...any) pgx.Row {
			capturedArgs = args
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
		Metadata:  map[string]string{"source": "api"},
	}

	if err := q.Enqueue(context.Background(), run); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	metaArg, ok := capturedArgs[30].([]byte)
	if !ok {
		t.Fatalf("arg[30] (metadata) type = %T, want []byte", capturedArgs[30])
	}
	if string(metaArg) == "{}" {
		t.Error("metadata JSON should not be empty when run.Metadata is non-empty")
	}
	if !strings.Contains(string(metaArg), "source") {
		t.Errorf("metadata JSON missing key 'source': %s", string(metaArg))
	}
}

func TestEnqueue_MetadataJSON_Empty(t *testing.T) {
	t.Parallel()
	var capturedArgs []any
	db := &mockDBTX{
		queryRowFn: func(_ context.Context, _ string, args ...any) pgx.Row {
			capturedArgs = args
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

	metaArg, ok := capturedArgs[30].([]byte)
	if !ok {
		t.Fatalf("arg[30] (metadata) type = %T, want []byte", capturedArgs[30])
	}
	if string(metaArg) != "{}" {
		t.Errorf("metadata JSON should be '{}' for nil metadata, got %s", string(metaArg))
	}
}

func TestEnqueue_DefaultExecutionMode_HTTP(t *testing.T) {
	t.Parallel()
	var capturedArgs []any
	db := &mockDBTX{
		queryRowFn: func(_ context.Context, _ string, args ...any) pgx.Row {
			capturedArgs = args
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

	execMode, ok := capturedArgs[28].(string)
	if !ok {
		t.Fatalf("arg[28] (execution_mode) type = %T, want string", capturedArgs[28])
	}
	if execMode != string(domain.ExecutionModeHTTP) {
		t.Errorf("default execution mode = %q, want %q", execMode, domain.ExecutionModeHTTP)
	}
}

func TestEnqueue_ExplicitExecutionMode_Preserved(t *testing.T) {
	t.Parallel()
	var capturedArgs []any
	db := &mockDBTX{
		queryRowFn: func(_ context.Context, _ string, args ...any) pgx.Row {
			capturedArgs = args
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
		JobID:         "job-1",
		ProjectID:     "proj-1",
		ExecutionMode: domain.ExecutionModeManaged,
	}

	if err := q.Enqueue(context.Background(), run); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	execMode, ok := capturedArgs[28].(string)
	if !ok {
		t.Fatalf("arg[28] (execution_mode) type = %T, want string", capturedArgs[28])
	}
	if execMode != string(domain.ExecutionModeManaged) {
		t.Errorf("execution mode = %q, want %q", execMode, domain.ExecutionModeManaged)
	}
}

func TestBackoffDelay_ExactlyAtMaxDelay(t *testing.T) {
	t.Parallel()
	n := &QueueNotifier{
		initialDelay: 16 * time.Second,
		maxDelay:     16 * time.Second,
	}
	for range 50 {
		delay := n.backoffDelay(0)
		maxWithJitter := time.Duration(float64(16*time.Second) * 1.26)
		if delay > maxWithJitter {
			t.Fatalf("delay %v exceeds max with jitter at exact boundary", delay)
		}
		minWithJitter := time.Duration(float64(16*time.Second) * 0.74)
		if delay < minWithJitter {
			t.Fatalf("delay %v below min at exact boundary", delay)
		}
	}
}
