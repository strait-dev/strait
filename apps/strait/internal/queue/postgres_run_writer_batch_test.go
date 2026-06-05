package queue

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

// mockBatchDB implements store.DBTX + CopyFromer for unit testing EnqueueBatch.
type mockBatchDB struct {
	mu          sync.Mutex
	copyFromFn  func(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error)
	execFn      func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	execCalls   []execCall
	copyFromN   int64
	copyFromErr error
}

type execCall struct {
	sql  string
	args []any
}

func (m *mockBatchDB) CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error) {
	if m.copyFromFn != nil {
		return m.copyFromFn(ctx, tableName, columnNames, rowSrc)
	}
	// Count rows from source to simulate COPY.
	var count int64
	for rowSrc.Next() {
		if _, err := rowSrc.Values(); err != nil {
			return 0, err
		}
		count++
	}
	if err := rowSrc.Err(); err != nil {
		return 0, err
	}
	if m.copyFromErr != nil {
		return 0, m.copyFromErr
	}
	if m.copyFromN > 0 {
		return m.copyFromN, nil
	}
	return count, nil
}

func (m *mockBatchDB) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	m.mu.Lock()
	m.execCalls = append(m.execCalls, execCall{sql: sql, args: args})
	m.mu.Unlock()
	if m.execFn != nil {
		return m.execFn(ctx, sql, args...)
	}
	return pgconn.NewCommandTag("SELECT 1"), nil
}

func (m *mockBatchDB) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	return nil, nil
}

func (m *mockBatchDB) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row {
	return nil
}

func (m *mockBatchDB) getExecCalls() []execCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]execCall, len(m.execCalls))
	copy(out, m.execCalls)
	return out
}

var _ store.DBTX = (*mockBatchDB)(nil)
var _ CopyFromer = (*mockBatchDB)(nil)

// mockNoCopyDB implements store.DBTX but NOT CopyFromer.
type mockNoCopyDB struct{}

func (m *mockNoCopyDB) Exec(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	return pgconn.NewCommandTag(""), nil
}
func (m *mockNoCopyDB) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	return nil, nil
}
func (m *mockNoCopyDB) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row { return nil }

// Tests.

func TestEnqueueBatch_EmptySlice(t *testing.T) {
	t.Parallel()
	q := NewPostgresRunWriter(&mockBatchDB{})
	n, err := q.EnqueueBatch(context.Background(), nil)
	require.NoError(t, err)
	require.EqualValues(t, 0, n)
}

func TestEnqueueBatch_SingleRun(t *testing.T) {
	t.Parallel()
	db := &mockBatchDB{}
	q := NewPostgresRunWriter(db)

	runs := []*domain.JobRun{{JobID: "job-1", ProjectID: "proj-1"}}
	n, err := q.EnqueueBatch(context.Background(), runs)
	require.NoError(t, err)
	require.EqualValues(t, 1, n)
}

func TestEnqueueBatch_AssignsIDs(t *testing.T) {
	t.Parallel()
	db := &mockBatchDB{}
	q := NewPostgresRunWriter(db)

	runs := []*domain.JobRun{
		{JobID: "job-1", ProjectID: "proj-1"},
		{JobID: "job-1", ProjectID: "proj-1"},
	}
	_, err := q.EnqueueBatch(context.Background(), runs)
	require.NoError(t, err)

	for _, run := range runs {
		require.NotEmpty(t, run.
			ID)
	}
	require.NotEqual(t, runs[1].ID, runs[0].ID)
}

func TestEnqueueBatch_PreservesExistingIDs(t *testing.T) {
	t.Parallel()
	db := &mockBatchDB{}
	q := NewPostgresRunWriter(db)

	runs := []*domain.JobRun{
		{ID: "preset-id-1", JobID: "job-1", ProjectID: "proj-1"},
	}
	_, err := q.EnqueueBatch(context.Background(), runs)
	require.NoError(t, err)
	require.Equal(t,
		"preset-id-1",
		runs[0].ID)
}

func TestEnqueueBatch_DefaultAttemptToOne(t *testing.T) {
	t.Parallel()
	db := &mockBatchDB{}
	q := NewPostgresRunWriter(db)

	runs := []*domain.JobRun{{JobID: "job-1", ProjectID: "proj-1", Attempt: 0}}
	_, err := q.EnqueueBatch(context.Background(), runs)
	require.NoError(t, err)
	require.Equal(t, 1, runs[0].Attempt)
}

func TestEnqueueBatch_DefaultTriggeredByManual(t *testing.T) {
	t.Parallel()
	db := &mockBatchDB{}
	q := NewPostgresRunWriter(db)

	runs := []*domain.JobRun{{JobID: "job-1", ProjectID: "proj-1"}}
	_, err := q.EnqueueBatch(context.Background(), runs)
	require.NoError(t, err)
	require.Equal(t,
		domain.TriggerManual,

		runs[0].
			TriggeredBy,
	)
}

func TestEnqueueBatch_FutureScheduledAt_StatusDelayed(t *testing.T) {
	t.Parallel()
	db := &mockBatchDB{}
	q := NewPostgresRunWriter(db)

	future := time.Now().Add(time.Hour)
	runs := []*domain.JobRun{{JobID: "job-1", ProjectID: "proj-1", ScheduledAt: &future}}
	_, err := q.EnqueueBatch(context.Background(), runs)
	require.NoError(t, err)
	require.Equal(t,
		domain.StatusDelayed,

		runs[0].
			Status)
}

func TestEnqueueBatch_PastScheduledAt_StatusQueued(t *testing.T) {
	t.Parallel()
	db := &mockBatchDB{}
	q := NewPostgresRunWriter(db)

	past := time.Now().Add(-time.Hour)
	runs := []*domain.JobRun{{JobID: "job-1", ProjectID: "proj-1", ScheduledAt: &past}}
	_, err := q.EnqueueBatch(context.Background(), runs)
	require.NoError(t, err)
	require.Equal(t,
		domain.StatusQueued,

		runs[0].
			Status)
}

func TestEnqueueBatch_CopyFromError(t *testing.T) {
	t.Parallel()
	db := &mockBatchDB{
		copyFromErr: errors.New("copy protocol error"),
	}
	q := NewPostgresRunWriter(db)

	runs := []*domain.JobRun{{JobID: "job-1", ProjectID: "proj-1"}}
	_, err := q.EnqueueBatch(context.Background(), runs)
	require.Error(t,
		err)
	require.Equal(t,
		"enqueue batch: copy from: copy protocol error",

		err.Error())
}

func TestEnqueueBatch_NoCopyFromSupport(t *testing.T) {
	t.Parallel()
	q := NewPostgresRunWriter(&mockNoCopyDB{})

	runs := []*domain.JobRun{{JobID: "job-1", ProjectID: "proj-1"}}
	_, err := q.EnqueueBatch(context.Background(), runs)
	require.Error(t,
		err)
	require.Equal(t,
		"enqueue batch: underlying db does not support CopyFrom",

		err.Error())
}

func TestEnqueueBatch_DoesNotIssueExplicitNotify(t *testing.T) {
	t.Parallel()
	db := &mockBatchDB{}
	q := NewPostgresRunWriter(db)

	runs := []*domain.JobRun{
		{JobID: "job-1", ProjectID: "proj-1"},
		{JobID: "job-1", ProjectID: "proj-1"},
	}
	n, err := q.EnqueueBatch(context.Background(), runs)
	require.NoError(t, err)
	require.EqualValues(t, 2, n)

	calls := db.getExecCalls()
	for _, c := range calls {
		require.NotEqual(t, "SELECT pg_notify($1, $2)",

			c.sql)
	}
}

func TestEnqueueBatch_TagsSerialized(t *testing.T) {
	t.Parallel()
	var capturedRows [][]any
	db := &mockBatchDB{
		copyFromFn: func(_ context.Context, _ pgx.Identifier, _ []string, rowSrc pgx.CopyFromSource) (int64, error) {
			for rowSrc.Next() {
				vals, _ := rowSrc.Values()
				capturedRows = append(capturedRows, slices.Clone(vals))
			}
			return int64(len(capturedRows)), rowSrc.Err()
		},
	}
	q := NewPostgresRunWriter(db)

	runs := []*domain.JobRun{{
		JobID:     "job-1",
		ProjectID: "proj-1",
		Tags:      map[string]string{"env": "prod", "region": "us-east-1"},
	}}
	_, err := q.EnqueueBatch(context.Background(), runs)
	require.NoError(t, err)
	require.Len(t,
		capturedRows,
		1)

	// tags is at index 23 in copyFromColumns.
	tagsVal, ok := capturedRows[0][23].([]byte)
	require.True(t,
		ok)

	tagsStr := string(tagsVal)
	require.Greater(t,
		len(tagsStr), 2)

	// JSON should contain both keys (order may vary).
}

// Adversarial batch tests.

func TestEnqueueBatch_LargeBatch_100Runs(t *testing.T) {
	t.Parallel()
	db := &mockBatchDB{}
	q := NewPostgresRunWriter(db)

	runs := make([]*domain.JobRun, 100)
	for i := range runs {
		runs[i] = &domain.JobRun{
			JobID:     "job-1",
			ProjectID: "proj-1",
			Tags:      map[string]string{"index": strconv.Itoa(i)},
		}
	}

	n, err := q.EnqueueBatch(context.Background(), runs)
	require.NoError(t, err)
	require.EqualValues(t, 100, n)

	ids := make(map[string]bool)
	for _, run := range runs {
		require.False(t,
			ids[run.
				ID])

		ids[run.ID] = true
	}
}

func TestEnqueueBatch_NilTags_DefaultsToEmptyJSON(t *testing.T) {
	t.Parallel()
	var capturedRows [][]any
	db := &mockBatchDB{
		copyFromFn: func(_ context.Context, _ pgx.Identifier, _ []string, rowSrc pgx.CopyFromSource) (int64, error) {
			for rowSrc.Next() {
				vals, _ := rowSrc.Values()
				capturedRows = append(capturedRows, slices.Clone(vals))
			}
			return int64(len(capturedRows)), rowSrc.Err()
		},
	}
	q := NewPostgresRunWriter(db)

	runs := []*domain.JobRun{
		{JobID: "job-1", ProjectID: "proj-1", Tags: nil},
		{JobID: "job-1", ProjectID: "proj-1", Tags: map[string]string{}},
	}
	_, err := q.EnqueueBatch(context.Background(), runs)
	require.NoError(t, err)

	for _, row := range capturedRows {
		tagsVal, ok := row[23].([]byte)
		require.True(t,
			ok)
		require.Equal(t,
			"{}", string(tagsVal))
	}
}

func TestEnqueueBatch_MixedScheduledAt(t *testing.T) {
	t.Parallel()
	db := &mockBatchDB{}
	q := NewPostgresRunWriter(db)

	future := time.Now().Add(time.Hour)
	past := time.Now().Add(-time.Hour)
	runs := []*domain.JobRun{
		{JobID: "job-1", ProjectID: "proj-1", ScheduledAt: &future},
		{JobID: "job-1", ProjectID: "proj-1", ScheduledAt: &past},
		{JobID: "job-1", ProjectID: "proj-1"},
	}

	_, err := q.EnqueueBatch(context.Background(), runs)
	require.NoError(t, err)
	require.Equal(t,
		domain.StatusDelayed,

		runs[0].
			Status)
	require.Equal(t,
		domain.StatusQueued,

		runs[1].
			Status)
	require.Equal(t,
		domain.StatusQueued,

		runs[2].
			Status)
}

func TestEnqueueBatch_PreservesExistingAttempt(t *testing.T) {
	t.Parallel()
	db := &mockBatchDB{}
	q := NewPostgresRunWriter(db)

	runs := []*domain.JobRun{
		{JobID: "job-1", ProjectID: "proj-1", Attempt: 5},
	}

	_, err := q.EnqueueBatch(context.Background(), runs)
	require.NoError(t, err)
	require.Equal(t, 5, runs[0].Attempt)
}
