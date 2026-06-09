package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

type mockDebounceStore struct {
	mu                sync.Mutex
	duePending        []domain.DebouncePending
	deleted           []string
	claimed           []string
	completed         []string
	rescheduled       []debounceReschedule
	restored          []string
	jobs              map[string]*domain.Job
	runs              map[string]*domain.JobRun
	quota             *store.ProjectQuota
	queuedRuns        int
	activeRuns        int
	runsSince         int
	dailyCost         int64
	tryAdvisoryLockFn func(ctx context.Context, lockID int64) (bool, error)
	txCalls           int
	txLockIDs         []int64
}

type debounceReschedule struct {
	id         string
	oldFireAt  time.Time
	nextFireAt time.Time
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

func (m *mockDebounceStore) ClaimDueDebouncePending(_ context.Context, id string) (*domain.DebouncePending, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.duePending {
		if m.duePending[i].ID == id {
			if m.duePending[i].FireAt.After(time.Now()) {
				return nil, false, nil
			}
			claimed := m.duePending[i]
			m.claimed = append(m.claimed, id)
			return &claimed, true, nil
		}
	}
	return nil, false, nil
}

func (m *mockDebounceStore) CompleteDebouncePending(_ context.Context, id string, fireAt time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.duePending {
		if m.duePending[i].ID == id && m.duePending[i].FireAt.Equal(fireAt) {
			m.duePending = append(m.duePending[:i], m.duePending[i+1:]...)
			m.completed = append(m.completed, id)
			return true, nil
		}
	}
	return false, nil
}

func (m *mockDebounceStore) RescheduleDebouncePending(_ context.Context, id string, oldFireAt, nextFireAt time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.duePending {
		if m.duePending[i].ID == id && m.duePending[i].FireAt.Equal(oldFireAt) {
			m.duePending[i].FireAt = nextFireAt
			m.rescheduled = append(m.rescheduled, debounceReschedule{
				id:         id,
				oldFireAt:  oldFireAt,
				nextFireAt: nextFireAt,
			})
			return true, nil
		}
	}
	return false, nil
}

func (m *mockDebounceStore) InsertDebouncePendingIfAbsent(_ context.Context, d *domain.DebouncePending) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, existing := range m.duePending {
		if existing.JobID == d.JobID && existing.DebounceKey == d.DebounceKey {
			return false, nil
		}
	}
	m.duePending = append(m.duePending, *d)
	m.restored = append(m.restored, d.ID)
	return true, nil
}

func (m *mockDebounceStore) GetJob(_ context.Context, id string) (*domain.Job, error) {
	if m.jobs != nil {
		if job, ok := m.jobs[id]; ok {
			return job, nil
		}
	}
	return nil, nil
}

func (m *mockDebounceStore) GetRun(_ context.Context, id string) (*domain.JobRun, error) {
	if m.runs != nil {
		if run, ok := m.runs[id]; ok {
			return run, nil
		}
	}
	return nil, store.ErrRunNotFound
}

func (m *mockDebounceStore) GetProjectQuota(context.Context, string) (*store.ProjectQuota, error) {
	return m.quota, nil
}

func (m *mockDebounceStore) CountProjectQueuedRuns(context.Context, string) (int, error) {
	return m.queuedRuns, nil
}

func (m *mockDebounceStore) CountProjectActiveRuns(context.Context, string) (int, error) {
	return m.activeRuns, nil
}

func (m *mockDebounceStore) CountRunsForJobSince(context.Context, string, time.Time) (int, error) {
	return m.runsSince, nil
}

func (m *mockDebounceStore) SumProjectDailyCostMicrousd(context.Context, string, string) (int64, error) {
	return m.dailyCost, nil
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

func (m *mockDebounceStore) WithTx(ctx context.Context, fn func(context.Context, store.DBTX) error) error {
	m.mu.Lock()
	m.txCalls++
	m.mu.Unlock()
	return fn(ctx, &mockDebounceTx{store: m})
}

type mockDebounceTx struct {
	store *mockDebounceStore
}

func (m *mockDebounceTx) Exec(_ context.Context, _ string, arguments ...any) (pgconn.CommandTag, error) {
	if len(arguments) > 0 {
		if lockID, ok := arguments[0].(int64); ok {
			m.store.mu.Lock()
			m.store.txLockIDs = append(m.store.txLockIDs, lockID)
			m.store.mu.Unlock()
		}
	}
	return pgconn.NewCommandTag("SELECT 1"), nil
}

func (m *mockDebounceTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("unexpected query")
}

func (m *mockDebounceTx) QueryRow(context.Context, string, ...any) pgx.Row {
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
	require.Len(t, enqueued, 1)

	run := enqueued[0]
	require.Equal(t, "job-1", run.JobID)
	require.Equal(t, "dp-1", run.ID)
	require.Equal(t, domain.TriggerDebounce, run.TriggeredBy)
	require.Equal(t, 5, run.Priority)
	require.Equal(t, "user-1", run.CreatedBy)
	require.JSONEq(t, `{"action":"reindex"}`, string(run.Payload))
	require.Equal(t, domain.ExecutionModeWorker, run.ExecutionMode)
	require.Equal(t, "priority", run.QueueName)
	require.Equal(t, []string{"dp-1"}, ds.claimed)
	require.Equal(t, []string{"dp-1"}, ds.completed)
	require.Empty(t, ds.restored)
	require.Equal(t, 1, ds.txCalls)
	require.Equal(t, []int64{cronAdmissionLockID("proj-1")}, ds.txLockIDs)
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
	require.Empty(t, enqueued)
	require.Len(t, ds.claimed, 1)
	require.Len(t, ds.completed, 1)
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
	require.Empty(t, enqueued)
	require.Len(t, ds.claimed, 1)
	require.Len(t, ds.completed, 1)
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
	require.Equal(t, "debounce:dp-1", key)
}

func TestDebouncePoller_DeletesPendingWhenRunAlreadyExists(t *testing.T) {
	t.Parallel()

	ds := &mockDebounceStore{
		duePending: []domain.DebouncePending{
			{ID: "dp-1", JobID: "job-1", ProjectID: "proj-1", FireAt: time.Now().Add(-time.Second)},
		},
		jobs: map[string]*domain.Job{
			"job-1": {ID: "job-1", Enabled: true, TimeoutSecs: 60},
		},
		runs: map[string]*domain.JobRun{
			"dp-1": {ID: "dp-1", JobID: "job-1", ProjectID: "proj-1"},
		},
	}

	q := &mockQueue{
		enqueueFn: func(context.Context, *domain.JobRun) error {
			return errors.New("duplicate key")
		},
	}
	poller := NewDebouncePoller(ds, q, time.Second)
	poller.poll(context.Background())
	require.Empty(t, ds.restored)
}

func TestDebouncePoller_ReschedulesPendingWhenFireTimeProjectQuotaExceeded(t *testing.T) {
	t.Parallel()

	originalFireAt := time.Now().Add(-time.Second)
	ds := &mockDebounceStore{
		duePending: []domain.DebouncePending{
			{ID: "dp-1", JobID: "job-1", ProjectID: "proj-1", FireAt: originalFireAt},
		},
		jobs: map[string]*domain.Job{
			"job-1": {ID: "job-1", ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60},
		},
		quota:      &store.ProjectQuota{MaxQueuedRuns: 1},
		queuedRuns: 1,
	}

	var enqueued int
	q := &mockQueue{enqueueFn: func(context.Context, *domain.JobRun) error {
		enqueued++
		return nil
	}}
	poller := NewDebouncePoller(ds, q, time.Second)
	poller.poll(context.Background())
	require.Equal(t, 0, enqueued)
	require.Empty(t, ds.completed)
	require.Len(t, ds.rescheduled, 1)
	require.Equal(t, "dp-1", ds.rescheduled[0].id)
	require.True(t, ds.rescheduled[0].oldFireAt.Equal(originalFireAt))
	require.True(t, ds.rescheduled[0].nextFireAt.After(time.Now().UTC()))
	require.Len(t, ds.duePending, 1)
	require.Equal(t, "dp-1", ds.duePending[0].ID)
	require.True(t, ds.duePending[0].FireAt.After(time.Now().UTC()))
}

func TestDebouncePoller_ReschedulesPendingWhenFireTimeRateLimitExceeded(t *testing.T) {
	t.Parallel()

	originalFireAt := time.Now().Add(-time.Second)
	ds := &mockDebounceStore{
		duePending: []domain.DebouncePending{
			{ID: "dp-1", JobID: "job-1", ProjectID: "proj-1", FireAt: originalFireAt},
		},
		jobs: map[string]*domain.Job{
			"job-1": {
				ID:                  "job-1",
				ProjectID:           "proj-1",
				Enabled:             true,
				TimeoutSecs:         60,
				RateLimitMax:        1,
				RateLimitWindowSecs: 60,
			},
		},
		runsSince: 1,
	}

	var enqueued int
	q := &mockQueue{enqueueFn: func(context.Context, *domain.JobRun) error {
		enqueued++
		return nil
	}}
	poller := NewDebouncePoller(ds, q, time.Second)
	poller.poll(context.Background())
	require.Equal(t, 0, enqueued)
	require.Empty(t, ds.completed)
	require.Len(t, ds.rescheduled, 1)
	require.Equal(t, "dp-1", ds.rescheduled[0].id)
	require.True(t, ds.rescheduled[0].oldFireAt.Equal(originalFireAt))
	require.True(t, ds.rescheduled[0].nextFireAt.After(time.Now().UTC()))
	require.Len(t, ds.duePending, 1)
	require.Equal(t, "dp-1", ds.duePending[0].ID)
	require.True(t, ds.duePending[0].FireAt.After(time.Now().UTC()))
}

func TestDebouncePoller_SkipsPendingExtendedAfterDueList(t *testing.T) {
	t.Parallel()

	ds := &mockDebounceStore{
		duePending: []domain.DebouncePending{
			{ID: "dp-1", JobID: "job-1", ProjectID: "proj-1", FireAt: time.Now().Add(10 * time.Minute)},
		},
		jobs: map[string]*domain.Job{
			"job-1": {ID: "job-1", ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60},
		},
	}

	var enqueued int
	q := &mockQueue{enqueueFn: func(context.Context, *domain.JobRun) error {
		enqueued++
		return nil
	}}
	poller := NewDebouncePoller(ds, q, time.Second)
	require.NoError(t, poller.pollLocked(context.Background()))
	require.Equal(t, 0, enqueued)
	require.Empty(t, ds.claimed)
}

func TestDebouncePoller_RestoreDoesNotOverwriteNewerPending(t *testing.T) {
	t.Parallel()

	past := time.Now().Add(-time.Second)
	future := time.Now().Add(10 * time.Minute)
	ds := &mockDebounceStore{
		duePending: []domain.DebouncePending{
			{ID: "dp-old", JobID: "job-1", ProjectID: "proj-1", DebounceKey: "key", FireAt: past},
		},
		jobs: map[string]*domain.Job{
			"job-1": {ID: "job-1", ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60},
		},
	}

	q := &mockQueue{enqueueFn: func(context.Context, *domain.JobRun) error {
		ds.mu.Lock()
		ds.duePending[0].FireAt = future
		ds.mu.Unlock()
		return errors.New("temporary queue failure")
	}}
	poller := NewDebouncePoller(ds, q, time.Second)
	poller.poll(context.Background())
	require.Empty(t, ds.restored)
	require.Empty(t, ds.completed)
	require.Len(t, ds.duePending, 1)
	require.Equal(t, "dp-old", ds.duePending[0].ID)
	require.True(t, ds.duePending[0].FireAt.Equal(future))
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
	require.Empty(t, enqueued)
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
	require.Empty(t, enqueued)
}
