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
	q := NewPostgresQueue(&mockBatchDB{})
	n, err := q.EnqueueBatch(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0, got %d", n)
	}
}

func TestEnqueueBatch_SingleRun(t *testing.T) {
	t.Parallel()
	db := &mockBatchDB{}
	q := NewPostgresQueue(db)

	runs := []*domain.JobRun{{JobID: "job-1", ProjectID: "proj-1"}}
	n, err := q.EnqueueBatch(context.Background(), runs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1, got %d", n)
	}
}

func TestEnqueueBatch_AssignsIDs(t *testing.T) {
	t.Parallel()
	db := &mockBatchDB{}
	q := NewPostgresQueue(db)

	runs := []*domain.JobRun{
		{JobID: "job-1", ProjectID: "proj-1"},
		{JobID: "job-1", ProjectID: "proj-1"},
	}
	_, err := q.EnqueueBatch(context.Background(), runs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i, run := range runs {
		if run.ID == "" {
			t.Fatalf("run %d: expected ID to be assigned", i)
		}
	}
	if runs[0].ID == runs[1].ID {
		t.Fatal("expected different IDs for different runs")
	}
}

func TestEnqueueBatch_PreservesExistingIDs(t *testing.T) {
	t.Parallel()
	db := &mockBatchDB{}
	q := NewPostgresQueue(db)

	runs := []*domain.JobRun{
		{ID: "preset-id-1", JobID: "job-1", ProjectID: "proj-1"},
	}
	_, err := q.EnqueueBatch(context.Background(), runs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if runs[0].ID != "preset-id-1" {
		t.Fatalf("expected ID to be preserved, got %s", runs[0].ID)
	}
}

func TestEnqueueBatch_DefaultAttemptToOne(t *testing.T) {
	t.Parallel()
	db := &mockBatchDB{}
	q := NewPostgresQueue(db)

	runs := []*domain.JobRun{{JobID: "job-1", ProjectID: "proj-1", Attempt: 0}}
	_, err := q.EnqueueBatch(context.Background(), runs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if runs[0].Attempt != 1 {
		t.Fatalf("expected attempt=1, got %d", runs[0].Attempt)
	}
}

func TestEnqueueBatch_DefaultTriggeredByManual(t *testing.T) {
	t.Parallel()
	db := &mockBatchDB{}
	q := NewPostgresQueue(db)

	runs := []*domain.JobRun{{JobID: "job-1", ProjectID: "proj-1"}}
	_, err := q.EnqueueBatch(context.Background(), runs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if runs[0].TriggeredBy != domain.TriggerManual {
		t.Fatalf("expected triggered_by=manual, got %s", runs[0].TriggeredBy)
	}
}

func TestEnqueueBatch_FutureScheduledAt_StatusDelayed(t *testing.T) {
	t.Parallel()
	db := &mockBatchDB{}
	q := NewPostgresQueue(db)

	future := time.Now().Add(time.Hour)
	runs := []*domain.JobRun{{JobID: "job-1", ProjectID: "proj-1", ScheduledAt: &future}}
	_, err := q.EnqueueBatch(context.Background(), runs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if runs[0].Status != domain.StatusDelayed {
		t.Fatalf("expected StatusDelayed for future scheduled_at, got %s", runs[0].Status)
	}
}

func TestEnqueueBatch_PastScheduledAt_StatusQueued(t *testing.T) {
	t.Parallel()
	db := &mockBatchDB{}
	q := NewPostgresQueue(db)

	past := time.Now().Add(-time.Hour)
	runs := []*domain.JobRun{{JobID: "job-1", ProjectID: "proj-1", ScheduledAt: &past}}
	_, err := q.EnqueueBatch(context.Background(), runs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if runs[0].Status != domain.StatusQueued {
		t.Fatalf("expected StatusQueued for past scheduled_at, got %s", runs[0].Status)
	}
}

func TestEnqueueBatch_CopyFromError(t *testing.T) {
	t.Parallel()
	db := &mockBatchDB{
		copyFromErr: errors.New("copy protocol error"),
	}
	q := NewPostgresQueue(db)

	runs := []*domain.JobRun{{JobID: "job-1", ProjectID: "proj-1"}}
	_, err := q.EnqueueBatch(context.Background(), runs)
	if err == nil {
		t.Fatal("expected error from CopyFrom")
	}
	if got := err.Error(); got != "enqueue batch: copy from: copy protocol error" {
		t.Fatalf("unexpected error message: %s", got)
	}
}

func TestEnqueueBatch_NoCopyFromSupport(t *testing.T) {
	t.Parallel()
	q := NewPostgresQueue(&mockNoCopyDB{})

	runs := []*domain.JobRun{{JobID: "job-1", ProjectID: "proj-1"}}
	_, err := q.EnqueueBatch(context.Background(), runs)
	if err == nil {
		t.Fatal("expected error when db lacks CopyFromer")
	}
	if got := err.Error(); got != "enqueue batch: underlying db does not support CopyFrom" {
		t.Fatalf("unexpected error: %s", got)
	}
}

func TestEnqueueBatch_PgNotifyCalledAfterInsert(t *testing.T) {
	t.Parallel()
	db := &mockBatchDB{}
	q := NewPostgresQueue(db)

	runs := []*domain.JobRun{
		{JobID: "job-1", ProjectID: "proj-1"},
		{JobID: "job-1", ProjectID: "proj-1"},
	}
	n, err := q.EnqueueBatch(context.Background(), runs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2, got %d", n)
	}

	calls := db.getExecCalls()
	// Expect claim-row inserts (one per run) + pg_notify.
	var notifyCalls []struct {
		sql  string
		args []any
	}
	for _, c := range calls {
		if c.sql == "SELECT pg_notify($1, $2)" {
			notifyCalls = append(notifyCalls, c)
		}
	}
	if len(notifyCalls) != 1 {
		t.Fatalf("expected 1 pg_notify call, got %d (total exec calls: %d)", len(notifyCalls), len(calls))
	}
}

func TestEnqueueBatch_PgNotifyFailure_NonFatal(t *testing.T) {
	t.Parallel()
	db := &mockBatchDB{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag(""), errors.New("pg_notify failed")
		},
	}
	q := NewPostgresQueue(db)

	runs := []*domain.JobRun{{JobID: "job-1", ProjectID: "proj-1"}}
	n, err := q.EnqueueBatch(context.Background(), runs)
	if err != nil {
		t.Fatalf("pg_notify failure should be non-fatal, got error: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1, got %d", n)
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
	q := NewPostgresQueue(db)

	runs := []*domain.JobRun{{
		JobID:     "job-1",
		ProjectID: "proj-1",
		Tags:      map[string]string{"env": "prod", "region": "us-east-1"},
	}}
	_, err := q.EnqueueBatch(context.Background(), runs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(capturedRows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(capturedRows))
	}
	// tags is at index 23 in copyFromColumns.
	tagsVal, ok := capturedRows[0][23].([]byte)
	if !ok {
		t.Fatalf("expected tags as []byte, got %T", capturedRows[0][23])
	}
	tagsStr := string(tagsVal)
	// JSON should contain both keys (order may vary).
	if len(tagsStr) <= 2 {
		t.Fatalf("expected non-empty tags JSON, got %q", tagsStr)
	}
}

// Adversarial batch tests.

func TestEnqueueBatch_LargeBatch_100Runs(t *testing.T) {
	t.Parallel()
	db := &mockBatchDB{}
	q := NewPostgresQueue(db)

	runs := make([]*domain.JobRun, 100)
	for i := range runs {
		runs[i] = &domain.JobRun{
			JobID:     "job-1",
			ProjectID: "proj-1",
			Tags:      map[string]string{"index": strconv.Itoa(i)},
		}
	}

	n, err := q.EnqueueBatch(context.Background(), runs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 100 {
		t.Fatalf("expected 100, got %d", n)
	}
	ids := make(map[string]bool)
	for _, run := range runs {
		if ids[run.ID] {
			t.Fatalf("duplicate ID: %s", run.ID)
		}
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
	q := NewPostgresQueue(db)

	runs := []*domain.JobRun{
		{JobID: "job-1", ProjectID: "proj-1", Tags: nil},
		{JobID: "job-1", ProjectID: "proj-1", Tags: map[string]string{}},
	}
	_, err := q.EnqueueBatch(context.Background(), runs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for i, row := range capturedRows {
		tagsVal := row[23].([]byte)
		if string(tagsVal) != "{}" {
			t.Fatalf("row %d: expected '{}', got %q", i, string(tagsVal))
		}
	}
}

func TestEnqueueBatch_MixedScheduledAt(t *testing.T) {
	t.Parallel()
	db := &mockBatchDB{}
	q := NewPostgresQueue(db)

	future := time.Now().Add(time.Hour)
	past := time.Now().Add(-time.Hour)
	runs := []*domain.JobRun{
		{JobID: "job-1", ProjectID: "proj-1", ScheduledAt: &future},
		{JobID: "job-1", ProjectID: "proj-1", ScheduledAt: &past},
		{JobID: "job-1", ProjectID: "proj-1"},
	}

	_, err := q.EnqueueBatch(context.Background(), runs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if runs[0].Status != domain.StatusDelayed {
		t.Fatalf("future: expected Delayed, got %s", runs[0].Status)
	}
	if runs[1].Status != domain.StatusQueued {
		t.Fatalf("past: expected Queued, got %s", runs[1].Status)
	}
	if runs[2].Status != domain.StatusQueued {
		t.Fatalf("nil: expected Queued, got %s", runs[2].Status)
	}
}

func TestEnqueueBatch_PreservesExistingAttempt(t *testing.T) {
	t.Parallel()
	db := &mockBatchDB{}
	q := NewPostgresQueue(db)

	runs := []*domain.JobRun{
		{JobID: "job-1", ProjectID: "proj-1", Attempt: 5},
	}

	_, err := q.EnqueueBatch(context.Background(), runs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if runs[0].Attempt != 5 {
		t.Fatalf("expected attempt=5 preserved, got %d", runs[0].Attempt)
	}
}
