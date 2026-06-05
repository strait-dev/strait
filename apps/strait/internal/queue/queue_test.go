package queue

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NotNil(
		t, q)
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

			args := workerQueueRefArgs(tt.refs)
			require.True(t,
				slices.Equal(args.ProjectIDs,

					tt.wantProjectIDs,
				))
			require.True(t,
				slices.Equal(args.QueueNames,

					tt.wantQueueNames,
				))
			require.True(t,
				slices.Equal(args.EnvironmentIDs,

					tt.wantEnvironment,
				))
		})
	}
}

func TestNormalizePgQueWorkerQueueRefs(t *testing.T) {
	t.Parallel()

	refs := []domain.WorkerQueueRef{
		{ProjectID: "project-a", QueueName: ""},
		{ProjectID: "", QueueName: "ignored"},
		{ProjectID: "project-a", QueueName: "default"},
		{ProjectID: "project-a", QueueName: "default"},
		{ProjectID: "project-a", QueueName: "critical", EnvironmentID: "prod"},
		{ProjectID: "project-b", QueueName: "bulk", EnvironmentID: "staging"},
	}

	got := normalizePgQueWorkerQueueRefs(refs)
	want := []domain.WorkerQueueRef{
		{ProjectID: "project-a", QueueName: "default"},
		{ProjectID: "project-a", QueueName: "critical", EnvironmentID: "prod"},
		{ProjectID: "project-b", QueueName: "bulk", EnvironmentID: "staging"},
	}
	require.True(t,
		slices.Equal(got, want))
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
	require.NoError(t, q.Enqueue(context.
		Background(), run))
	assert.NotEmpty(t, run.
		ID)
	assert.Equal(t, 1, run.Attempt)
	assert.Equal(t,
		domain.TriggerManual,

		run.TriggeredBy,
	)
	assert.Equal(t,
		domain.StatusQueued,

		run.Status,
	)
	assert.False(t,
		run.CreatedAt.
			IsZero())
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
	require.NoError(t, q.Enqueue(context.
		Background(), run))
	assert.Equal(t,
		"custom-id",
		run.ID,
	)
	assert.Equal(t, 3, run.Attempt)
	assert.Equal(t,
		domain.TriggerCron,

		run.TriggeredBy,
	)
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
	require.NoError(t, q.Enqueue(context.
		Background(), run))
	assert.Equal(t,
		domain.StatusDelayed,

		run.Status,
	)
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
	require.NoError(t, q.Enqueue(context.
		Background(), run))
	assert.Equal(t,
		domain.StatusQueued,

		run.Status,
	)
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
	require.Error(t,
		err)
}

func TestLoad_DefaultStatementTimeout(t *testing.T) {
	// This is tested via config_test.go - just verify the option works
	q := NewPostgresRunWriter(&mockDBTX{}, WithStatementTimeout(30*time.Second))
	require.Equal(t,
		30*time.
			Second,
		q.statementTimeout,
	)
}

func TestCopyFromColumnsIncludesMetadata(t *testing.T) {
	t.Parallel()
	require.True(t,
		slices.Contains(copyFromColumns,

			"metadata",
		))
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
	require.NoError(t, q.Enqueue(context.
		Background(), run))

	tagsArg, ok := capturedArgs[23].([]byte)
	require.True(t,
		ok)
	assert.NotEqual(t, "{}",
		string(tagsArg))
	assert.Contains(t,
		string(tagsArg), "env")
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
	require.NoError(t, q.Enqueue(context.
		Background(), run))

	tagsArg, ok := capturedArgs[23].([]byte)
	require.True(t,
		ok)
	assert.Equal(t,
		"{}", string(tagsArg))
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
	require.NoError(t, q.Enqueue(context.
		Background(), run))

	metaArg, ok := capturedArgs[30].([]byte)
	require.True(t,
		ok)
	assert.NotEqual(t, "{}",
		string(metaArg))
	assert.Contains(t,
		string(metaArg), "source")
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
	require.NoError(t, q.Enqueue(context.
		Background(), run))

	metaArg, ok := capturedArgs[30].([]byte)
	require.True(t,
		ok)
	assert.Equal(t,
		"{}", string(metaArg))
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
	require.NoError(t, q.Enqueue(context.
		Background(), run))

	execMode, ok := capturedArgs[28].(string)
	require.True(t,
		ok)
	assert.Equal(t,
		string(domain.
			ExecutionModeHTTP,
		), execMode,
	)
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
	require.NoError(t, q.Enqueue(context.
		Background(), run))

	execMode, ok := capturedArgs[28].(string)
	require.True(t,
		ok)
	assert.Equal(t,
		string(domain.
			ExecutionModeWorker,
		), execMode,
	)
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
		require.LessOrEqual(t, delay,
			maxWithJitter,
		)

		minWithJitter := time.Duration(float64(16*time.Second) * 0.74)
		require.GreaterOrEqual(t,
			delay, minWithJitter,
		)
	}
}
