package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

type mockBatchStore struct {
	flushable         []store.FlushableBatch
	drainedItems      []domain.BatchBufferItem
	deletedItems      []string
	jobs              map[string]*domain.Job
	getJobCalls       atomic.Int32
	tryAdvisoryLockFn func(ctx context.Context, lockID int64) (bool, error)
}

func (m *mockBatchStore) ListFlushableBatches(_ context.Context) ([]store.FlushableBatch, error) {
	return m.flushable, nil
}

func (m *mockBatchStore) DrainBatchBuffer(_ context.Context, _, _ string, _ int) ([]domain.BatchBufferItem, error) {
	return m.drainedItems, nil
}

func (m *mockBatchStore) ListBatchBufferItems(_ context.Context, _, _ string, _ int) ([]domain.BatchBufferItem, error) {
	return m.drainedItems, nil
}

func (m *mockBatchStore) DeleteBatchBufferItems(_ context.Context, ids []string) error {
	m.deletedItems = append(m.deletedItems, ids...)
	return nil
}

func (m *mockBatchStore) GetJob(_ context.Context, id string) (*domain.Job, error) {
	m.getJobCalls.Add(1)
	if m.jobs != nil {
		if job, ok := m.jobs[id]; ok {
			return job, nil
		}
	}
	return nil, nil
}

func (m *mockBatchStore) CreateRun(_ context.Context, _ *domain.JobRun) error {
	return nil
}

func (m *mockBatchStore) TryAdvisoryLock(ctx context.Context, lockID int64) (bool, error) {
	if m.tryAdvisoryLockFn != nil {
		return m.tryAdvisoryLockFn(ctx, lockID)
	}
	return true, nil
}

func (m *mockBatchStore) ReleaseAdvisoryLock(_ context.Context, _ int64) error {
	return nil
}

type mockTransactionalBatchStore struct {
	*mockBatchStore
	withTxCalled  bool
	drainTxCalled bool
	committed     bool
	rolledBack    bool
}

func (m *mockTransactionalBatchStore) WithTx(ctx context.Context, fn func(context.Context, store.DBTX) error) error {
	m.withTxCalled = true
	err := fn(ctx, nil)
	if err != nil {
		m.rolledBack = true
		return err
	}
	m.committed = true
	return nil
}

func (m *mockTransactionalBatchStore) DrainBatchBufferInTx(ctx context.Context, _ store.DBTX, jobID, batchKey string, limit int) ([]domain.BatchBufferItem, error) {
	m.drainTxCalled = true
	return m.DrainBatchBuffer(ctx, jobID, batchKey, limit)
}

func TestBatchFlusher_FlushesReadyBatch(t *testing.T) {
	t.Parallel()

	bs := &mockBatchStore{
		flushable: []store.FlushableBatch{
			{JobID: "job-1", ProjectID: "proj-1", BatchKey: "", ItemCount: 3, OldestAt: time.Now().Add(-10 * time.Second)},
		},
		drainedItems: []domain.BatchBufferItem{
			{ID: "i1", JobID: "job-1", ProjectID: "proj-1", Payload: json.RawMessage(`{"a":1}`), Priority: 3, CreatedBy: "user-1"},
			{ID: "i2", JobID: "job-1", ProjectID: "proj-1", Payload: json.RawMessage(`{"a":2}`), Priority: 3},
			{ID: "i3", JobID: "job-1", ProjectID: "proj-1", Payload: json.RawMessage(`{"a":3}`), Priority: 3},
		},
		jobs: map[string]*domain.Job{
			"job-1": {
				ID:              "job-1",
				ProjectID:       "proj-1",
				Enabled:         true,
				TimeoutSecs:     300,
				BatchMaxSize:    3,
				BatchWindowSecs: 10,
				Version:         1,
				VersionID:       "v-1",
				Tags:            map[string]string{"env": "prod"},
				ExecutionMode:   domain.ExecutionModeWorker,
				Queue:           "priority",
			},
		},
	}

	var enqueued []*domain.JobRun
	q := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			enqueued = append(enqueued, run)
			return nil
		},
	}
	flusher := NewBatchFlusher(bs, q, time.Second)
	flusher.poll(context.Background())

	if len(enqueued) != 1 {
		t.Fatalf("expected 1 batch run, got %d", len(enqueued))
	}

	run := enqueued[0]
	if run.JobID != "job-1" {
		t.Fatalf("expected job_id=job-1, got %s", run.JobID)
	}
	if run.TriggeredBy != "batch" {
		t.Fatalf("expected triggered_by=batch, got %s", run.TriggeredBy)
	}
	if run.Priority != 3 {
		t.Fatalf("expected priority=3, got %d", run.Priority)
	}
	if run.CreatedBy != "user-1" {
		t.Fatalf("expected created_by from first item, got %q", run.CreatedBy)
	}
	if run.Tags["env"] != "prod" {
		t.Fatalf("expected tags from job, got %v", run.Tags)
	}
	if run.ExecutionMode != domain.ExecutionModeWorker {
		t.Fatalf("expected execution_mode worker, got %q", run.ExecutionMode)
	}
	if run.QueueName != "priority" {
		t.Fatalf("expected queue_name priority, got %q", run.QueueName)
	}
	if run.IdempotencyKey != "batch:job-1::i1:i3" {
		t.Fatalf("expected deterministic idempotency key, got %q", run.IdempotencyKey)
	}
	if len(bs.deletedItems) != 3 {
		t.Fatalf("expected flushed items to be deleted after enqueue, got %v", bs.deletedItems)
	}

	var payload map[string]any
	if err := json.Unmarshal(run.Payload, &payload); err != nil {
		t.Fatalf("invalid batch payload: %v", err)
	}
	items, ok := payload["items"].([]any)
	if !ok {
		t.Fatalf("expected items array in payload, got %T", payload["items"])
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items in batch, got %d", len(items))
	}
}

func TestBatchFlusher_TransactionalFlushDrainsAndEnqueuesInOneCommit(t *testing.T) {
	t.Parallel()

	bs := &mockTransactionalBatchStore{mockBatchStore: &mockBatchStore{
		flushable: []store.FlushableBatch{
			{JobID: "job-1", ProjectID: "proj-1", BatchKey: "key", ItemCount: 2},
		},
		drainedItems: []domain.BatchBufferItem{
			{ID: "item-1", JobID: "job-1", ProjectID: "proj-1", BatchKey: "key", Payload: json.RawMessage(`{"a":1}`)},
			{ID: "item-2", JobID: "job-1", ProjectID: "proj-1", BatchKey: "key", Payload: json.RawMessage(`{"a":2}`)},
		},
		jobs: map[string]*domain.Job{
			"job-1": {ID: "job-1", ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60, BatchMaxSize: 2},
		},
	}}
	var enqueued []*domain.JobRun
	q := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			enqueued = append(enqueued, run)
			return nil
		},
	}

	NewBatchFlusher(bs, q, time.Second).poll(context.Background())

	if !bs.withTxCalled || !bs.drainTxCalled || !bs.committed || bs.rolledBack {
		t.Fatalf("transaction state: withTx=%t drainTx=%t committed=%t rolledBack=%t", bs.withTxCalled, bs.drainTxCalled, bs.committed, bs.rolledBack)
	}
	if len(enqueued) != 1 {
		t.Fatalf("enqueued runs = %d, want 1", len(enqueued))
	}
	if enqueued[0].IdempotencyKey != "batch:job-1:key:item-1:item-2" {
		t.Fatalf("idempotency key = %q", enqueued[0].IdempotencyKey)
	}
	if len(bs.deletedItems) != 0 {
		t.Fatalf("transactional flush should not issue a second delete, got %v", bs.deletedItems)
	}
}

func TestBatchFlusher_TransactionalEnqueueFailureRollsBackDrain(t *testing.T) {
	t.Parallel()

	bs := &mockTransactionalBatchStore{mockBatchStore: &mockBatchStore{
		flushable: []store.FlushableBatch{
			{JobID: "job-1", ProjectID: "proj-1", BatchKey: "key", ItemCount: 1},
		},
		drainedItems: []domain.BatchBufferItem{
			{ID: "item-1", JobID: "job-1", ProjectID: "proj-1", BatchKey: "key", Payload: json.RawMessage(`{"a":1}`)},
		},
		jobs: map[string]*domain.Job{
			"job-1": {ID: "job-1", ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60, BatchMaxSize: 1},
		},
	}}
	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			return errors.New("enqueue failed")
		},
	}

	NewBatchFlusher(bs, q, time.Second).poll(context.Background())

	if !bs.rolledBack || bs.committed {
		t.Fatalf("transaction state: committed=%t rolledBack=%t", bs.committed, bs.rolledBack)
	}
	if len(bs.deletedItems) != 0 {
		t.Fatalf("failed transactional flush should not issue out-of-tx deletes, got %v", bs.deletedItems)
	}
}

func TestBatchFlusher_TransactionalIdempotencyConflictCommitsDrain(t *testing.T) {
	t.Parallel()

	bs := &mockTransactionalBatchStore{mockBatchStore: &mockBatchStore{
		flushable: []store.FlushableBatch{
			{JobID: "job-1", ProjectID: "proj-1", BatchKey: "key", ItemCount: 1},
		},
		drainedItems: []domain.BatchBufferItem{
			{ID: "item-1", JobID: "job-1", ProjectID: "proj-1", BatchKey: "key", Payload: json.RawMessage(`{"a":1}`)},
		},
		jobs: map[string]*domain.Job{
			"job-1": {ID: "job-1", ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60, BatchMaxSize: 1},
		},
	}}
	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			return domain.ErrIdempotencyConflict
		},
	}

	NewBatchFlusher(bs, q, time.Second).poll(context.Background())

	if !bs.committed || bs.rolledBack {
		t.Fatalf("idempotency conflict should commit the drain: committed=%t rolledBack=%t", bs.committed, bs.rolledBack)
	}
}

func TestBatchFlusher_EnqueueFailureDoesNotDeleteBufferedItems(t *testing.T) {
	t.Parallel()

	bs := &mockBatchStore{
		flushable: []store.FlushableBatch{
			{JobID: "job-1", ProjectID: "proj-1", BatchKey: "key", ItemCount: 1},
		},
		drainedItems: []domain.BatchBufferItem{
			{ID: "item-1", JobID: "job-1", ProjectID: "proj-1", BatchKey: "key", Payload: json.RawMessage(`{"a":1}`)},
		},
		jobs: map[string]*domain.Job{
			"job-1": {ID: "job-1", Enabled: true, TimeoutSecs: 60, BatchMaxSize: 10},
		},
	}
	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			return errors.New("enqueue failed")
		},
	}
	flusher := NewBatchFlusher(bs, q, time.Second)
	flusher.poll(context.Background())

	if len(bs.deletedItems) != 0 {
		t.Fatalf("buffered items deleted after enqueue failure: %v", bs.deletedItems)
	}
}

func TestBatchFlusher_IdempotencyConflictDeletesPreviouslyFlushedItems(t *testing.T) {
	t.Parallel()

	bs := &mockBatchStore{
		flushable: []store.FlushableBatch{
			{JobID: "job-1", ProjectID: "proj-1", BatchKey: "key", ItemCount: 1},
		},
		drainedItems: []domain.BatchBufferItem{
			{ID: "item-1", JobID: "job-1", ProjectID: "proj-1", BatchKey: "key", Payload: json.RawMessage(`{"a":1}`)},
		},
		jobs: map[string]*domain.Job{
			"job-1": {ID: "job-1", Enabled: true, TimeoutSecs: 60, BatchMaxSize: 10},
		},
	}
	q := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			return domain.ErrIdempotencyConflict
		},
	}
	flusher := NewBatchFlusher(bs, q, time.Second)
	flusher.poll(context.Background())

	if len(bs.deletedItems) != 1 || bs.deletedItems[0] != "item-1" {
		t.Fatalf("expected prior flushed item to be deleted, got %v", bs.deletedItems)
	}
}

func TestBatchFlusher_SkipsDisabledJob(t *testing.T) {
	t.Parallel()

	bs := &mockBatchStore{
		flushable: []store.FlushableBatch{
			{JobID: "job-1", ProjectID: "proj-1", BatchKey: "", ItemCount: 2},
		},
		jobs: map[string]*domain.Job{
			"job-1": {ID: "job-1", Enabled: false},
		},
	}

	var enqueued []*domain.JobRun
	q := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			enqueued = append(enqueued, run)
			return nil
		},
	}
	flusher := NewBatchFlusher(bs, q, time.Second)
	flusher.poll(context.Background())

	if len(enqueued) != 0 {
		t.Fatal("expected no runs for disabled job")
	}
}

func TestBatchFlusher_SkipsWhenLockNotAcquired(t *testing.T) {
	t.Parallel()

	bs := &mockBatchStore{
		flushable: []store.FlushableBatch{
			{JobID: "job-1", ProjectID: "proj-1", BatchKey: "", ItemCount: 1},
		},
		tryAdvisoryLockFn: func(context.Context, int64) (bool, error) {
			return false, nil
		},
	}

	var enqueued []*domain.JobRun
	q := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			enqueued = append(enqueued, run)
			return nil
		},
	}
	flusher := NewBatchFlusher(bs, q, time.Second)
	flusher.poll(context.Background())

	if len(enqueued) != 0 {
		t.Fatal("expected no runs when lock not acquired")
	}
}

func TestBatchFlusher_EmptyDrain(t *testing.T) {
	t.Parallel()

	bs := &mockBatchStore{
		flushable: []store.FlushableBatch{
			{JobID: "job-1", ProjectID: "proj-1", BatchKey: "", ItemCount: 2},
		},
		drainedItems: nil,
		jobs: map[string]*domain.Job{
			"job-1": {ID: "job-1", Enabled: true, TimeoutSecs: 60, BatchMaxSize: 10},
		},
	}

	var enqueued []*domain.JobRun
	q := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			enqueued = append(enqueued, run)
			return nil
		},
	}
	flusher := NewBatchFlusher(bs, q, time.Second)
	flusher.poll(context.Background())

	if len(enqueued) != 0 {
		t.Fatal("expected no runs when drain returns empty")
	}
}

func TestBatchFlusher_CachesJobLookupWithinPoll(t *testing.T) {
	t.Parallel()

	bs := &mockBatchStore{
		flushable: []store.FlushableBatch{
			{JobID: "job-1", ProjectID: "proj-1", BatchKey: "a", ItemCount: 1},
			{JobID: "job-1", ProjectID: "proj-1", BatchKey: "b", ItemCount: 1},
			{JobID: "job-1", ProjectID: "proj-1", BatchKey: "c", ItemCount: 1},
		},
		drainedItems: []domain.BatchBufferItem{
			{ID: "i1", JobID: "job-1", ProjectID: "proj-1", Payload: json.RawMessage(`{"a":1}`), Priority: 1},
		},
		jobs: map[string]*domain.Job{
			"job-1": {ID: "job-1", ProjectID: "proj-1", Enabled: true, TimeoutSecs: 30, BatchMaxSize: 1},
		},
	}
	q := &mockQueue{enqueueFn: func(context.Context, *domain.JobRun) error { return nil }}
	flusher := NewBatchFlusher(bs, q, time.Second)

	flusher.poll(context.Background())

	if got := bs.getJobCalls.Load(); got != 1 {
		t.Fatalf("GetJob calls = %d, want 1 for repeated batches on same job", got)
	}
}
