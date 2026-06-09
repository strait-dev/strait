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

	"github.com/stretchr/testify/require"
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
	require.Len(t, enqueued,
		1)

	run := enqueued[0]
	require.Equal(t, "job-1",
		run.JobID)
	require.Equal(t, "batch",
		run.TriggeredBy,
	)
	require.Equal(t, 3,
		run.Priority)
	require.Equal(t, "user-1",
		run.CreatedBy,
	)
	require.Equal(t, "prod",
		run.Tags["env"])
	require.Equal(t, domain.
		ExecutionModeWorker,
		run.
			ExecutionMode,
	)
	require.Equal(t, "priority",
		run.QueueName,
	)
	require.Equal(t, "batch:job-1::i1:i3",

		run.IdempotencyKey,
	)
	require.Len(t, bs.deletedItems,
		3)

	var payload map[string]any
	require.NoError(t,
		json.Unmarshal(run.
			Payload,
			&payload))

	items, ok := payload["items"].([]any)
	require.True(t, ok)
	require.Len(t, items,
		3)
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
	require.False(t, !bs.
		withTxCalled ||
		!bs.drainTxCalled ||
		!bs.
			committed ||
		bs.rolledBack,
	)
	require.Len(t, enqueued,
		1)
	require.Equal(t, "batch:job-1:key:item-1:item-2",

		enqueued[0].
			IdempotencyKey,
	)
	require.Empty(t, bs.deletedItems)
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
	require.False(t, !bs.
		rolledBack || bs.
		committed,
	)
	require.Empty(t, bs.deletedItems)
}

func TestMarshalBatchPayloadPreservesRawItems(t *testing.T) {
	t.Parallel()

	payload, err := marshalBatchPayload([]domain.BatchBufferItem{
		{ID: "item-1", Payload: json.RawMessage(`{"a":1}`)},
		{ID: "item-2", Payload: json.RawMessage(`"quoted"`)},
		{ID: "item-3"},
		{ID: "item-4", Payload: json.RawMessage(`[true,false]`)},
	})
	require.NoError(t, err)
	require.JSONEq(t, `{"items":[{"a":1},"quoted",null,[true,false]]}`, string(payload))
}

func TestMarshalBatchPayloadRejectsInvalidRawItem(t *testing.T) {
	t.Parallel()

	payload, err := marshalBatchPayload([]domain.BatchBufferItem{
		{ID: "bad-item", Payload: json.RawMessage(`{"broken"`)},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "bad-item")
	require.Nil(t, payload)
}

func BenchmarkMarshalBatchPayload(b *testing.B) {
	items := make([]domain.BatchBufferItem, 128)
	for i := range items {
		items[i] = domain.BatchBufferItem{Payload: json.RawMessage(`{"value":123,"status":"queued"}`)}
	}

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		payload, err := marshalBatchPayload(items)
		if err != nil {
			b.Fatalf("marshalBatchPayload() error = %v", err)
		}
		if len(payload) == 0 {
			b.Fatal("marshalBatchPayload() returned empty payload")
		}
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
	require.False(t, !bs.
		committed || bs.
		rolledBack,
	)
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
	require.Empty(t, bs.deletedItems)
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
	require.False(t, len(bs.deletedItems) != 1 ||
		bs.deletedItems[0] != "item-1",
	)
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
	require.Empty(t, enqueued)
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
	require.Empty(t, enqueued)
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
	require.Empty(t, enqueued)
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
	require.EqualValues(t, 1,
		bs.getJobCalls.Load())
}
