package scheduler

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

type mockBatchStore struct {
	flushable         []store.FlushableBatch
	drainedItems      []domain.BatchBufferItem
	jobs              map[string]*domain.Job
	tryAdvisoryLockFn func(ctx context.Context, lockID int64) (bool, error)
}

func (m *mockBatchStore) ListFlushableBatches(_ context.Context) ([]store.FlushableBatch, error) {
	return m.flushable, nil
}

func (m *mockBatchStore) DrainBatchBuffer(_ context.Context, _, _ string, _ int) ([]domain.BatchBufferItem, error) {
	return m.drainedItems, nil
}

func (m *mockBatchStore) GetJob(_ context.Context, id string) (*domain.Job, error) {
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
