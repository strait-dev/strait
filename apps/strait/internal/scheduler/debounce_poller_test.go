package scheduler

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"strait/internal/domain"
)

type mockDebounceStore struct {
	mu                sync.Mutex
	duePending        []domain.DebouncePending
	deleted           []string
	jobs              map[string]*domain.Job
	tryAdvisoryLockFn func(ctx context.Context, lockID int64) (bool, error)
}

func (m *mockDebounceStore) ListDueDebouncePending(_ context.Context) ([]domain.DebouncePending, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.duePending, nil
}

func (m *mockDebounceStore) DeleteDebouncePending(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deleted = append(m.deleted, id)
	return nil
}

func (m *mockDebounceStore) GetJob(_ context.Context, id string) (*domain.Job, error) {
	if m.jobs != nil {
		if job, ok := m.jobs[id]; ok {
			return job, nil
		}
	}
	return nil, nil
}

func (m *mockDebounceStore) CreateRun(_ context.Context, _ *domain.JobRun) error {
	return nil
}

func (m *mockDebounceStore) TryAdvisoryLock(ctx context.Context, lockID int64) (bool, error) {
	if m.tryAdvisoryLockFn != nil {
		return m.tryAdvisoryLockFn(ctx, lockID)
	}
	return true, nil
}

func (m *mockDebounceStore) ReleaseAdvisoryLock(_ context.Context, _ int64) error {
	return nil
}

func TestDebouncePoller_FiresDuePending(t *testing.T) {
	t.Parallel()

	ds := &mockDebounceStore{
		duePending: []domain.DebouncePending{
			{
				ID:          "dp-1",
				JobID:       "job-1",
				ProjectID:   "proj-1",
				DebounceKey: "",
				Payload:     json.RawMessage(`{"action":"reindex"}`),
				Priority:    5,
				TriggeredBy: "debounce",
				CreatedBy:   "user-1",
				FireAt:      time.Now().Add(-time.Second),
			},
		},
		jobs: map[string]*domain.Job{
			"job-1": {
				ID:            "job-1",
				ProjectID:     "proj-1",
				Enabled:       true,
				TimeoutSecs:   300,
				Version:       2,
				VersionID:     "v-2",
				ExecutionMode: domain.ExecutionModeWorker,
				Queue:         "priority",
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
	poller := NewDebouncePoller(ds, q, time.Second)
	poller.poll(context.Background())

	if len(enqueued) != 1 {
		t.Fatalf("expected 1 enqueued run, got %d", len(enqueued))
	}

	run := enqueued[0]
	if run.JobID != "job-1" {
		t.Fatalf("expected job_id=job-1, got %s", run.JobID)
	}
	if run.TriggeredBy != domain.TriggerDebounce {
		t.Fatalf("expected triggered_by=debounce, got %s", run.TriggeredBy)
	}
	if run.Priority != 5 {
		t.Fatalf("expected priority=5, got %d", run.Priority)
	}
	if run.CreatedBy != "user-1" {
		t.Fatalf("expected created_by=user-1, got %s", run.CreatedBy)
	}
	if string(run.Payload) != `{"action":"reindex"}` {
		t.Fatalf("expected payload preserved, got %s", string(run.Payload))
	}
	if run.ExecutionMode != domain.ExecutionModeWorker {
		t.Fatalf("expected execution_mode worker, got %q", run.ExecutionMode)
	}
	if run.QueueName != "priority" {
		t.Fatalf("expected queue_name priority, got %q", run.QueueName)
	}

	if len(ds.deleted) != 1 || ds.deleted[0] != "dp-1" {
		t.Fatalf("expected dp-1 to be deleted, got %v", ds.deleted)
	}
}

func TestDebouncePoller_SkipsDisabledJob(t *testing.T) {
	t.Parallel()

	ds := &mockDebounceStore{
		duePending: []domain.DebouncePending{
			{ID: "dp-1", JobID: "job-1", ProjectID: "proj-1", FireAt: time.Now().Add(-time.Second)},
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
	poller := NewDebouncePoller(ds, q, time.Second)
	poller.poll(context.Background())

	if len(enqueued) != 0 {
		t.Fatalf("expected no runs for disabled job, got %d", len(enqueued))
	}
	if len(ds.deleted) != 1 {
		t.Fatalf("expected pending deleted even for disabled job, got %d deleted", len(ds.deleted))
	}
}

func TestDebouncePoller_SkipsPausedJob(t *testing.T) {
	t.Parallel()

	ds := &mockDebounceStore{
		duePending: []domain.DebouncePending{
			{ID: "dp-1", JobID: "job-1", ProjectID: "proj-1", FireAt: time.Now().Add(-time.Second)},
		},
		jobs: map[string]*domain.Job{
			"job-1": {ID: "job-1", Enabled: true, Paused: true},
		},
	}

	var enqueued []*domain.JobRun
	q := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			enqueued = append(enqueued, run)
			return nil
		},
	}
	poller := NewDebouncePoller(ds, q, time.Second)
	poller.poll(context.Background())

	if len(enqueued) != 0 {
		t.Fatalf("expected no runs for paused job, got %d", len(enqueued))
	}
	if len(ds.deleted) != 1 {
		t.Fatalf("expected pending deleted for paused job, got %d deleted", len(ds.deleted))
	}
}

func TestDebouncePoller_UsesPendingIDAsIdempotencyKey(t *testing.T) {
	t.Parallel()

	ds := &mockDebounceStore{
		duePending: []domain.DebouncePending{
			{ID: "dp-1", JobID: "job-1", ProjectID: "proj-1", FireAt: time.Now().Add(-time.Second)},
		},
		jobs: map[string]*domain.Job{
			"job-1": {ID: "job-1", Enabled: true, TimeoutSecs: 60},
		},
	}

	var key string
	q := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			key = run.IdempotencyKey
			return nil
		},
	}
	poller := NewDebouncePoller(ds, q, time.Second)
	poller.poll(context.Background())

	if key != "debounce:dp-1" {
		t.Fatalf("idempotency key = %q, want debounce:dp-1", key)
	}
}

func TestDebouncePoller_SkipsWhenLockNotAcquired(t *testing.T) {
	t.Parallel()

	ds := &mockDebounceStore{
		duePending: []domain.DebouncePending{
			{ID: "dp-1", JobID: "job-1", ProjectID: "proj-1", FireAt: time.Now().Add(-time.Second)},
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
	poller := NewDebouncePoller(ds, q, time.Second)
	poller.poll(context.Background())

	if len(enqueued) != 0 {
		t.Fatal("expected no runs when lock not acquired")
	}
}

func TestDebouncePoller_NoDuePending(t *testing.T) {
	t.Parallel()

	ds := &mockDebounceStore{}
	var enqueued []*domain.JobRun
	q := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			enqueued = append(enqueued, run)
			return nil
		},
	}
	poller := NewDebouncePoller(ds, q, time.Second)
	poller.poll(context.Background())

	if len(enqueued) != 0 {
		t.Fatal("expected no runs when nothing due")
	}
}
