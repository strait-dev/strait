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

// mockTxDBTX wraps mockDBTX and adds TxBeginner support for statement timeout tests.
type mockTxDBTX struct {
	mockDBTX
	beginFn func(ctx context.Context) (pgx.Tx, error)
}

func (m *mockTxDBTX) Begin(ctx context.Context) (pgx.Tx, error) {
	if m.beginFn != nil {
		return m.beginFn(ctx)
	}
	return nil, errors.New("not implemented")
}

var _ store.TxBeginner = (*mockTxDBTX)(nil)

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

func TestNewPostgresRunWriter(t *testing.T) {
	t.Parallel()
	q := NewPostgresRunWriter(nil)
	if q == nil {
		t.Fatal("NewPostgresRunWriter(nil) returned nil")
	}
}

func TestWorkerQueueRefArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		refs            []domain.WorkerQueueRef
		wantProjectIDs  []string
		wantQueueNames  []string
		wantEnvironment []string
	}{
		{
			name:            "empty",
			refs:            nil,
			wantProjectIDs:  nil,
			wantQueueNames:  nil,
			wantEnvironment: nil,
		},
		{
			name: "single valid scope",
			refs: []domain.WorkerQueueRef{
				{ProjectID: "project-a", QueueName: "priority", EnvironmentID: "env-prod"},
			},
			wantProjectIDs:  []string{"project-a"},
			wantQueueNames:  []string{"priority"},
			wantEnvironment: []string{"env-prod"},
		},
		{
			name: "single invalid scope",
			refs: []domain.WorkerQueueRef{
				{ProjectID: "project-a"},
			},
			wantProjectIDs:  nil,
			wantQueueNames:  nil,
			wantEnvironment: nil,
		},
		{
			name: "deduplicates and drops invalid scopes",
			refs: []domain.WorkerQueueRef{
				{ProjectID: "project-a", QueueName: "default"},
				{ProjectID: "project-a", QueueName: "default"},
				{ProjectID: "project-a", QueueName: "priority", EnvironmentID: "env-prod"},
				{ProjectID: "project-b", QueueName: "priority", EnvironmentID: "env-staging"},
				{ProjectID: "", QueueName: "ignored"},
				{ProjectID: "project-c", QueueName: ""},
			},
			wantProjectIDs:  []string{"project-a", "project-a", "project-b"},
			wantQueueNames:  []string{"default", "priority", "priority"},
			wantEnvironment: []string{"", "env-prod", "env-staging"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			projectIDs, queueNames, environmentIDs := workerQueueRefArgs(tt.refs)
			if !slices.Equal(projectIDs, tt.wantProjectIDs) {
				t.Fatalf("projectIDs = %v, want %v", projectIDs, tt.wantProjectIDs)
			}
			if !slices.Equal(queueNames, tt.wantQueueNames) {
				t.Fatalf("queueNames = %v, want %v", queueNames, tt.wantQueueNames)
			}
			if !slices.Equal(environmentIDs, tt.wantEnvironment) {
				t.Fatalf("environmentIDs = %v, want %v", environmentIDs, tt.wantEnvironment)
			}
		})
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

	q := NewPostgresRunWriter(db)
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

	q := NewPostgresRunWriter(db)
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

	q := NewPostgresRunWriter(db)
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

	q := NewPostgresRunWriter(db)
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

	q := NewPostgresRunWriter(db)
	run := &domain.JobRun{
		JobID:     "job-1",
		ProjectID: "proj-1",
	}

	err := q.Enqueue(context.Background(), run)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestLoad_DefaultStatementTimeout(t *testing.T) {
	// This is tested via config_test.go - just verify the option works
	q := NewPostgresRunWriter(&mockDBTX{}, WithStatementTimeout(30*time.Second))
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

	q := NewPostgresRunWriter(db)
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

	q := NewPostgresRunWriter(db)
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

	q := NewPostgresRunWriter(db)
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

	q := NewPostgresRunWriter(db)
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

	q := NewPostgresRunWriter(db)
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

	q := NewPostgresRunWriter(db)
	run := &domain.JobRun{
		JobID:         "job-1",
		ProjectID:     "proj-1",
		ExecutionMode: domain.ExecutionModeWorker,
	}

	if err := q.Enqueue(context.Background(), run); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	execMode, ok := capturedArgs[28].(string)
	if !ok {
		t.Fatalf("arg[28] (execution_mode) type = %T, want string", capturedArgs[28])
	}
	if execMode != string(domain.ExecutionModeWorker) {
		t.Errorf("execution mode = %q, want %q", execMode, domain.ExecutionModeWorker)
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
