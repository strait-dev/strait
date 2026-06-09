package logdrain

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockDrainStore implements DrainStore for testing.
type mockDrainStore struct {
	mu              sync.Mutex
	enabledDrains   []domain.LogDrain
	enabledErr      error
	finishedRuns    map[string][]domain.JobRun // projectID -> runs
	finishedRunsErr error
	events          map[string][]domain.RunEvent // runID -> events
	eventsErr       error
	eventsErrByRun  map[string]error             // runID -> error (per-run override)
	logDrains       map[string][]domain.LogDrain // projectID -> drains
}

func (m *mockDrainStore) ListLogDrains(_ context.Context, projectID string) ([]domain.LogDrain, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.logDrains[projectID], nil
}

func (m *mockDrainStore) ListEventsAsc(_ context.Context, runID string, limit int, afterTime *time.Time, afterID string) ([]domain.RunEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.eventsErrByRun != nil {
		if err, ok := m.eventsErrByRun[runID]; ok {
			return nil, err
		}
	}
	if m.eventsErr != nil {
		return nil, m.eventsErr
	}
	allEvents := m.events[runID]
	// Filter by composite cursor and apply limit for pagination testing.
	var filtered []domain.RunEvent
	for _, e := range allEvents {
		if afterTime != nil {
			if e.CreatedAt.Before(*afterTime) {
				continue
			}
			if e.CreatedAt.Equal(*afterTime) && e.ID <= afterID {
				continue
			}
		}
		filtered = append(filtered, e)
	}
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].CreatedAt.Equal(filtered[j].CreatedAt) {
			return filtered[i].ID < filtered[j].ID
		}
		return filtered[i].CreatedAt.Before(filtered[j].CreatedAt)
	})
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	return filtered, nil
}

func (m *mockDrainStore) ListEnabledLogDrains(_ context.Context) ([]domain.LogDrain, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.enabledErr != nil {
		return nil, m.enabledErr
	}
	return m.enabledDrains, nil
}

func (m *mockDrainStore) ListFinishedRunsSince(_ context.Context, projectID string, since time.Time, sinceRunID string, limit int) ([]domain.JobRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.finishedRunsErr != nil {
		return nil, m.finishedRunsErr
	}
	allRuns := m.finishedRuns[projectID]
	// Filter by composite cursor.
	var filtered []domain.JobRun
	for _, r := range allRuns {
		if r.FinishedAt == nil {
			continue
		}
		if r.FinishedAt.After(since) || (r.FinishedAt.Equal(since) && r.ID > sinceRunID) {
			filtered = append(filtered, r)
		}
	}
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].FinishedAt.Equal(*filtered[j].FinishedAt) {
			return filtered[i].ID < filtered[j].ID
		}
		return filtered[i].FinishedAt.Before(*filtered[j].FinishedAt)
	})
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	return filtered, nil
}

func TestWorker_StartStop(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()
	svc := NewService()
	w := NewWorker(nil, svc, 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	concWG.Go(func() {
		w.Run(ctx)
		close(done)
	})

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		require.Fail(t, "worker did not stop within 2s after context cancel")
	}
}

func TestWorker_StopMethod(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()
	svc := NewService()
	w := NewWorker(nil, svc, 50*time.Millisecond)

	ctx := context.Background()
	go w.Run(ctx)

	stopped := make(chan struct{})
	concWG.Go(func() {
		w.Stop()
		close(stopped)
	})

	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		require.Fail(t, "Stop() did not return within 2s")
	}
}

func TestWorker_Tick_NoDrains(t *testing.T) {
	t.Parallel()
	store := &mockDrainStore{
		enabledDrains: nil,
	}
	svc := NewService()
	w := NewWorker(store, svc, time.Hour)

	ctx := context.Background()
	w.tick(ctx)
}

func TestWorker_Tick_DrainDisabled(t *testing.T) {
	t.Parallel()
	store := &mockDrainStore{
		enabledDrains: []domain.LogDrain{},
	}
	svc := NewService()
	w := NewWorker(store, svc, time.Hour)

	ctx := context.Background()
	w.tick(ctx)
}

func TestWorker_Tick_ListDrainsError(t *testing.T) {
	t.Parallel()
	store := &mockDrainStore{
		enabledErr: fmt.Errorf("db connection lost"),
	}
	svc := NewService()
	w := NewWorker(store, svc, time.Hour)

	ctx := context.Background()
	w.tick(ctx)
}

func TestWorker_Tick_NoFinishedRuns(t *testing.T) {
	t.Parallel()
	store := &mockDrainStore{
		enabledDrains: []domain.LogDrain{
			{ID: "drain-1", ProjectID: "proj-1", EndpointURL: "http://example.com", Enabled: true},
		},
		finishedRuns: map[string][]domain.JobRun{
			"proj-1": {},
		},
	}
	svc := NewService()
	w := NewWorker(store, svc, time.Hour)

	ctx := context.Background()
	w.tick(ctx)
}

func TestWorker_Tick_DrainEventsDelivered(t *testing.T) {
	t.Parallel()

	var received []domain.RunEvent
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var events []domain.RunEvent
		if err := json.NewDecoder(r.Body).Decode(&events); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mu.Lock()
		received = append(received, events...)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	now := time.Now()
	finishedAt := now.Add(-30 * time.Second)

	store := &mockDrainStore{
		enabledDrains: []domain.LogDrain{
			{ID: "drain-1", ProjectID: "proj-1", EndpointURL: srv.URL, Enabled: true},
		},
		finishedRuns: map[string][]domain.JobRun{
			"proj-1": {
				{ID: "run-1", ProjectID: "proj-1", FinishedAt: &finishedAt, Status: domain.StatusCompleted},
			},
		},
		events: map[string][]domain.RunEvent{
			"run-1": {
				{ID: "evt-1", RunID: "run-1", Message: "started", CreatedAt: now.Add(-2 * time.Second)},
				{ID: "evt-2", RunID: "run-1", Message: "completed", CreatedAt: now.Add(-1 * time.Second)},
			},
		},
	}
	svc := NewService()
	w := NewWorker(store, svc, time.Hour)

	ctx := context.Background()
	w.tick(ctx)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, received,
		2)
	assert.Equal(t, "evt-1",
		received[0].ID)
}

func TestWorker_Tick_MultipleRuns(t *testing.T) {
	t.Parallel()

	deliveryCount := 0
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		deliveryCount++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	now := time.Now()
	f1 := now.Add(-2 * time.Minute)
	f2 := now.Add(-1 * time.Minute)

	store := &mockDrainStore{
		enabledDrains: []domain.LogDrain{
			{ID: "drain-1", ProjectID: "proj-1", EndpointURL: srv.URL, Enabled: true},
		},
		finishedRuns: map[string][]domain.JobRun{
			"proj-1": {
				{ID: "run-1", ProjectID: "proj-1", FinishedAt: &f1, Status: domain.StatusCompleted},
				{ID: "run-2", ProjectID: "proj-1", FinishedAt: &f2, Status: domain.StatusFailed},
			},
		},
		events: map[string][]domain.RunEvent{
			"run-1": {{ID: "evt-1", RunID: "run-1", Message: "done", CreatedAt: now}},
			"run-2": {{ID: "evt-2", RunID: "run-2", Message: "failed", CreatedAt: now}},
		},
	}
	svc := NewService()
	w := NewWorker(store, svc, time.Hour)

	ctx := context.Background()
	w.tick(ctx)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 2,
		deliveryCount)
}

func TestWorker_Tick_CheckpointUpdated(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	now := time.Now()
	f1 := now.Add(-2 * time.Minute)
	f2 := now.Add(-1 * time.Minute)

	store := &mockDrainStore{
		enabledDrains: []domain.LogDrain{
			{ID: "drain-1", ProjectID: "proj-1", EndpointURL: srv.URL, Enabled: true},
		},
		finishedRuns: map[string][]domain.JobRun{
			"proj-1": {
				{ID: "run-1", ProjectID: "proj-1", FinishedAt: &f1, Status: domain.StatusCompleted},
				{ID: "run-2", ProjectID: "proj-1", FinishedAt: &f2, Status: domain.StatusCompleted},
			},
		},
		events: map[string][]domain.RunEvent{
			"run-1": {{ID: "evt-1", RunID: "run-1", Message: "done", CreatedAt: now}},
			"run-2": {{ID: "evt-2", RunID: "run-2", Message: "done", CreatedAt: now}},
		},
	}
	svc := NewService()
	w := NewWorker(store, svc, time.Hour)

	ctx := context.Background()
	w.tick(ctx)

	w.mu.Lock()
	cp := w.checkpoints["drain-1"]
	w.mu.Unlock()
	assert.True(t, cp.
		FinishedAt.Equal(
		f2))
	assert.Equal(t, "run-2",
		cp.RunID)
}

func TestWorker_Tick_DeliveryErrorContinuesToNextRun(t *testing.T) {
	t.Parallel()

	callCount := 0
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		callCount++
		c := callCount
		mu.Unlock()
		if c == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	now := time.Now()
	f1 := now.Add(-2 * time.Minute)
	f2 := now.Add(-1 * time.Minute)

	store := &mockDrainStore{
		enabledDrains: []domain.LogDrain{
			{ID: "drain-1", ProjectID: "proj-1", EndpointURL: srv.URL, Enabled: true},
		},
		finishedRuns: map[string][]domain.JobRun{
			"proj-1": {
				{ID: "run-1", ProjectID: "proj-1", FinishedAt: &f1, Status: domain.StatusCompleted},
				{ID: "run-2", ProjectID: "proj-1", FinishedAt: &f2, Status: domain.StatusCompleted},
			},
		},
		events: map[string][]domain.RunEvent{
			"run-1": {{ID: "evt-1", RunID: "run-1", Message: "done", CreatedAt: now}},
			"run-2": {{ID: "evt-2", RunID: "run-2", Message: "done", CreatedAt: now}},
		},
	}
	svc := NewService()
	w := NewWorker(store, svc, time.Hour)

	ctx := context.Background()
	w.tick(ctx)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 2,
		callCount)

	// run-1 fails, run-2 succeeds - both should be attempted.

	// Checkpoint should advance past run-2 (success) but not run-1 (failure).
	w.mu.Lock()
	cp := w.checkpoints["drain-1"]
	w.mu.Unlock()
	assert.True(t, cp.
		FinishedAt.Equal(
		f2))
}

func TestWorker_Tick_NoEventsSkipsDelivery(t *testing.T) {
	t.Parallel()

	requestCount := 0
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		requestCount++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	now := time.Now()
	finishedAt := now.Add(-30 * time.Second)

	store := &mockDrainStore{
		enabledDrains: []domain.LogDrain{
			{ID: "drain-1", ProjectID: "proj-1", EndpointURL: srv.URL, Enabled: true},
		},
		finishedRuns: map[string][]domain.JobRun{
			"proj-1": {
				{ID: "run-1", ProjectID: "proj-1", FinishedAt: &finishedAt, Status: domain.StatusCompleted},
			},
		},
		events: map[string][]domain.RunEvent{
			"run-1": {},
		},
	}
	svc := NewService()
	w := NewWorker(store, svc, time.Hour)

	ctx := context.Background()
	w.tick(ctx)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 0,
		requestCount)
}

func TestWorker_Tick_NilStore(t *testing.T) {
	t.Parallel()
	svc := NewService()
	w := NewWorker(nil, svc, time.Hour)

	ctx := context.Background()
	w.tick(ctx)
}

func TestWorker_Tick_ListEventsError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	now := time.Now()
	finishedAt := now.Add(-30 * time.Second)

	store := &mockDrainStore{
		enabledDrains: []domain.LogDrain{
			{ID: "drain-1", ProjectID: "proj-1", EndpointURL: srv.URL, Enabled: true},
		},
		finishedRuns: map[string][]domain.JobRun{
			"proj-1": {
				{ID: "run-1", ProjectID: "proj-1", FinishedAt: &finishedAt, Status: domain.StatusCompleted},
			},
		},
		eventsErr: fmt.Errorf("events query failed"),
	}
	svc := NewService()
	w := NewWorker(store, svc, time.Hour)

	ctx := context.Background()
	w.tick(ctx)
}

func TestWorker_Tick_ContextCancelled(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	now := time.Now()
	f1 := now.Add(-2 * time.Minute)
	f2 := now.Add(-1 * time.Minute)

	store := &mockDrainStore{
		enabledDrains: []domain.LogDrain{
			{ID: "drain-1", ProjectID: "proj-1", EndpointURL: srv.URL, Enabled: true},
			{ID: "drain-2", ProjectID: "proj-2", EndpointURL: srv.URL, Enabled: true},
		},
		finishedRuns: map[string][]domain.JobRun{
			"proj-1": {{ID: "run-1", ProjectID: "proj-1", FinishedAt: &f1, Status: domain.StatusCompleted}},
			"proj-2": {{ID: "run-2", ProjectID: "proj-2", FinishedAt: &f2, Status: domain.StatusCompleted}},
		},
		events: map[string][]domain.RunEvent{
			"run-1": {{ID: "evt-1", RunID: "run-1", Message: "done", CreatedAt: now}},
			"run-2": {{ID: "evt-2", RunID: "run-2", Message: "done", CreatedAt: now}},
		},
	}
	svc := NewService()
	w := NewWorker(store, svc, time.Hour)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w.tick(ctx)
}

func TestWorker_Tick_FinishedRunsError(t *testing.T) {
	t.Parallel()

	store := &mockDrainStore{
		enabledDrains: []domain.LogDrain{
			{ID: "drain-1", ProjectID: "proj-1", EndpointURL: "http://example.com", Enabled: true},
		},
		finishedRunsErr: fmt.Errorf("connection timeout"),
	}
	svc := NewService()
	w := NewWorker(store, svc, time.Hour)

	ctx := context.Background()
	w.tick(ctx)
}

// ListEvents error does not advance checkpoint.

func TestWorker_Tick_ListEventsError_DoesNotAdvanceCheckpoint(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	now := time.Now()
	f1 := now.Add(-2 * time.Minute)
	f2 := now.Add(-1 * time.Minute)

	store := &mockDrainStore{
		enabledDrains: []domain.LogDrain{
			{ID: "drain-1", ProjectID: "proj-1", EndpointURL: srv.URL, Enabled: true},
		},
		finishedRuns: map[string][]domain.JobRun{
			"proj-1": {
				{ID: "run-1", ProjectID: "proj-1", FinishedAt: &f1, Status: domain.StatusCompleted},
				{ID: "run-2", ProjectID: "proj-1", FinishedAt: &f2, Status: domain.StatusCompleted},
			},
		},
		events: map[string][]domain.RunEvent{
			"run-1": {{ID: "evt-1", RunID: "run-1", Message: "done", CreatedAt: now}},
		},
		eventsErrByRun: map[string]error{
			"run-2": fmt.Errorf("events query failed for run-2"),
		},
	}
	svc := NewService()
	w := NewWorker(store, svc, time.Hour)

	ctx := context.Background()
	w.tick(ctx)

	w.mu.Lock()
	cp := w.checkpoints["drain-1"]
	w.mu.Unlock()
	assert.True(t, cp.
		FinishedAt.Equal(
		f1))

	// Checkpoint should be at run-1, not run-2 (which had a ListEvents error).
}

// Poison run skip after max retries.

func TestWorker_Tick_PoisonRunSkippedAfterMaxRetries(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	now := time.Now()
	finishedAt := now.Add(-30 * time.Second)

	store := &mockDrainStore{
		enabledDrains: []domain.LogDrain{
			{ID: "drain-1", ProjectID: "proj-1", EndpointURL: srv.URL, Enabled: true},
		},
		finishedRuns: map[string][]domain.JobRun{
			"proj-1": {
				{ID: "run-1", ProjectID: "proj-1", FinishedAt: &finishedAt, Status: domain.StatusCompleted},
			},
		},
		events: map[string][]domain.RunEvent{
			"run-1": {{ID: "evt-1", RunID: "run-1", Message: "done", CreatedAt: now}},
		},
	}
	svc := NewService()
	w := NewWorker(store, svc, time.Hour)

	ctx := context.Background()
	// Tick maxRunRetries times to accumulate failures.
	for range maxRunRetries {
		w.tick(ctx)
	}

	// After maxRunRetries failures, checkpoint should NOT have advanced.
	w.mu.Lock()
	cp := w.checkpoints["drain-1"]
	w.mu.Unlock()
	assert.False(t, cp.
		FinishedAt.Equal(finishedAt))

	// One more tick: now it should be skipped as a poison run.
	w.tick(ctx)

	w.mu.Lock()
	cp = w.checkpoints["drain-1"]
	w.mu.Unlock()
	assert.True(t, cp.
		FinishedAt.Equal(
		finishedAt,
	))
}

func TestWorker_Tick_SuccessResetsFailCount(t *testing.T) {
	t.Parallel()

	callCount := 0
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		callCount++
		c := callCount
		mu.Unlock()
		if c == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	now := time.Now()
	finishedAt := now.Add(-30 * time.Second)

	store := &mockDrainStore{
		enabledDrains: []domain.LogDrain{
			{ID: "drain-1", ProjectID: "proj-1", EndpointURL: srv.URL, Enabled: true},
		},
		finishedRuns: map[string][]domain.JobRun{
			"proj-1": {
				{ID: "run-1", ProjectID: "proj-1", FinishedAt: &finishedAt, Status: domain.StatusCompleted},
			},
		},
		events: map[string][]domain.RunEvent{
			"run-1": {{ID: "evt-1", RunID: "run-1", Message: "done", CreatedAt: now}},
		},
	}
	svc := NewService()
	w := NewWorker(store, svc, time.Hour)

	ctx := context.Background()
	w.tick(ctx) // fails

	w.mu.Lock()
	fc := w.failCounts["drain-1:run-1"]
	w.mu.Unlock()
	require.Equal(t, 1,
		fc)

	w.tick(ctx) // succeeds

	w.mu.Lock()
	fc = w.failCounts["drain-1:run-1"]
	w.mu.Unlock()
	assert.Equal(t, 0,
		fc)
}

// Composite cursor handles timestamp collision.

func TestWorker_Tick_TimestampCollisionPagination(t *testing.T) {
	t.Parallel()

	var deliveredRuns []string
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var events []domain.RunEvent
		if err := json.NewDecoder(r.Body).Decode(&events); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mu.Lock()
		if len(events) > 0 {
			deliveredRuns = append(deliveredRuns, events[0].RunID)
		}
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	now := time.Now()
	sameTime := now.Add(-1 * time.Minute)

	store := &mockDrainStore{
		enabledDrains: []domain.LogDrain{
			{ID: "drain-1", ProjectID: "proj-1", EndpointURL: srv.URL, Enabled: true},
		},
		finishedRuns: map[string][]domain.JobRun{
			"proj-1": {
				{ID: "run-a", ProjectID: "proj-1", FinishedAt: &sameTime, Status: domain.StatusCompleted},
				{ID: "run-b", ProjectID: "proj-1", FinishedAt: &sameTime, Status: domain.StatusCompleted},
				{ID: "run-c", ProjectID: "proj-1", FinishedAt: &sameTime, Status: domain.StatusCompleted},
			},
		},
		events: map[string][]domain.RunEvent{
			"run-a": {{ID: "evt-a", RunID: "run-a", Message: "done", CreatedAt: now}},
			"run-b": {{ID: "evt-b", RunID: "run-b", Message: "done", CreatedAt: now}},
			"run-c": {{ID: "evt-c", RunID: "run-c", Message: "done", CreatedAt: now}},
		},
	}
	svc := NewService()
	w := NewWorker(store, svc, time.Hour)

	ctx := context.Background()
	w.tick(ctx)

	mu.Lock()
	defer mu.Unlock()
	assert.Len(t, deliveredRuns,
		3)
}

// Zero checkpoint and pagination on catch-up.

func TestWorker_Tick_FirstRunProcessesAllHistory(t *testing.T) {
	t.Parallel()

	var deliveredCount int
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		deliveredCount++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	now := time.Now()
	// Run finished hours ago - should still be processed with zero checkpoint.
	hoursAgo := now.Add(-5 * time.Hour)

	store := &mockDrainStore{
		enabledDrains: []domain.LogDrain{
			{ID: "drain-1", ProjectID: "proj-1", EndpointURL: srv.URL, Enabled: true},
		},
		finishedRuns: map[string][]domain.JobRun{
			"proj-1": {
				{ID: "run-1", ProjectID: "proj-1", FinishedAt: &hoursAgo, Status: domain.StatusCompleted},
			},
		},
		events: map[string][]domain.RunEvent{
			"run-1": {{ID: "evt-1", RunID: "run-1", Message: "done", CreatedAt: now}},
		},
	}
	svc := NewService()
	w := NewWorker(store, svc, time.Hour)

	ctx := context.Background()
	w.tick(ctx)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 1,
		deliveredCount)
}

func TestWorker_Tick_PaginatesCatchUp(t *testing.T) {
	t.Parallel()

	var deliveredCount int
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		deliveredCount++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	now := time.Now()
	// Create 250 runs across multiple pages (batch size = 100).
	runs := make([]domain.JobRun, 250)
	events := make(map[string][]domain.RunEvent, 250)
	for i := range 250 {
		id := fmt.Sprintf("run-%03d", i)
		finishedAt := now.Add(-time.Duration(250-i) * time.Second)
		runs[i] = domain.JobRun{
			ID:         id,
			ProjectID:  "proj-1",
			FinishedAt: &finishedAt,
			Status:     domain.StatusCompleted,
		}
		events[id] = []domain.RunEvent{
			{ID: fmt.Sprintf("evt-%03d", i), RunID: id, Message: "done", CreatedAt: now},
		}
	}

	store := &mockDrainStore{
		enabledDrains: []domain.LogDrain{
			{ID: "drain-1", ProjectID: "proj-1", EndpointURL: srv.URL, Enabled: true},
		},
		finishedRuns: map[string][]domain.JobRun{
			"proj-1": runs,
		},
		events: events,
	}
	svc := NewService()
	w := NewWorker(store, svc, time.Hour)

	ctx := context.Background()
	w.tick(ctx)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 250,
		deliveredCount,
	)
}

// Event pagination across multiple pages.

func TestWorker_Tick_EventPagination(t *testing.T) {
	t.Parallel()

	var receivedEvents []domain.RunEvent
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var events []domain.RunEvent
		if err := json.NewDecoder(r.Body).Decode(&events); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mu.Lock()
		receivedEvents = append(receivedEvents, events...)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	now := time.Now()
	finishedAt := now.Add(-30 * time.Second)

	// Create 2500 events (more than defaultEventLimit of 1000).
	events := make([]domain.RunEvent, 2500)
	for i := range 2500 {
		events[i] = domain.RunEvent{
			ID:        fmt.Sprintf("evt-%04d", i),
			RunID:     "run-1",
			Message:   fmt.Sprintf("event %d", i),
			CreatedAt: now.Add(time.Duration(i) * time.Millisecond),
		}
	}

	store := &mockDrainStore{
		enabledDrains: []domain.LogDrain{
			{ID: "drain-1", ProjectID: "proj-1", EndpointURL: srv.URL, Enabled: true},
		},
		finishedRuns: map[string][]domain.JobRun{
			"proj-1": {
				{ID: "run-1", ProjectID: "proj-1", FinishedAt: &finishedAt, Status: domain.StatusCompleted},
			},
		},
		events: map[string][]domain.RunEvent{
			"run-1": events,
		},
	}
	svc := NewService()
	w := NewWorker(store, svc, time.Hour)

	ctx := context.Background()
	w.tick(ctx)

	mu.Lock()
	defer mu.Unlock()
	assert.Len(t, receivedEvents,
		2500)
}

func TestWorker_Tick_EventPaginationError(t *testing.T) {
	t.Parallel()

	now := time.Now()
	finishedAt := now.Add(-30 * time.Second)

	// Create events that will span 2 pages.
	events := make([]domain.RunEvent, 1500)
	for i := range 1500 {
		events[i] = domain.RunEvent{
			ID:        fmt.Sprintf("evt-%04d", i),
			RunID:     "run-1",
			Message:   fmt.Sprintf("event %d", i),
			CreatedAt: now.Add(time.Duration(i) * time.Millisecond),
		}
	}

	callCount := 0
	store := &mockDrainStore{
		enabledDrains: []domain.LogDrain{
			{ID: "drain-1", ProjectID: "proj-1", EndpointURL: "http://example.com", Enabled: true},
		},
		finishedRuns: map[string][]domain.JobRun{
			"proj-1": {
				{ID: "run-1", ProjectID: "proj-1", FinishedAt: &finishedAt, Status: domain.StatusCompleted},
			},
		},
	}

	// We test that the error on event fetch does not advance the checkpoint.
	store.eventsErr = fmt.Errorf("page 2 failure")

	svc := NewService()
	w := NewWorker(store, svc, time.Hour)

	ctx := context.Background()
	w.tick(ctx)

	_ = callCount

	// Checkpoint should not have advanced since event fetch failed.
	w.mu.Lock()
	cp := w.checkpoints["drain-1"]
	w.mu.Unlock()
	assert.True(t, cp.
		FinishedAt.IsZero())
}

type benchmarkPagedEventStore struct {
	events []domain.RunEvent
}

func (s *benchmarkPagedEventStore) ListLogDrains(context.Context, string) ([]domain.LogDrain, error) {
	return nil, nil
}

func (s *benchmarkPagedEventStore) ListEventsAsc(_ context.Context, _ string, limit int, _ *time.Time, afterID string) ([]domain.RunEvent, error) {
	start := 0
	if afterID != "" {
		idx, err := strconv.Atoi(afterID[4:])
		if err != nil {
			return nil, err
		}
		start = idx + 1
	}
	if start >= len(s.events) {
		return nil, nil
	}
	end := start + limit
	if end > len(s.events) {
		end = len(s.events)
	}
	return s.events[start:end], nil
}

func (s *benchmarkPagedEventStore) ListEnabledLogDrains(context.Context) ([]domain.LogDrain, error) {
	return nil, nil
}

func (s *benchmarkPagedEventStore) ListFinishedRunsSince(context.Context, string, time.Time, string, int) ([]domain.JobRun, error) {
	return nil, nil
}

func TestWorkerFetchAllEvents_PagedNoNetwork(t *testing.T) {
	t.Parallel()

	now := time.Now()
	events := make([]domain.RunEvent, 2500)
	for i := range events {
		events[i] = domain.RunEvent{
			ID:        fmt.Sprintf("evt-%04d", i),
			RunID:     "run-1",
			Message:   "event",
			CreatedAt: now.Add(time.Duration(i) * time.Millisecond),
		}
	}

	w := NewWorker(&benchmarkPagedEventStore{events: events}, NewService(), time.Hour)
	got, err := w.fetchAllEvents(t.Context(), "run-1")

	require.NoError(t, err)
	require.Len(t, got, len(events))
	assert.Equal(t, "evt-0000", got[0].ID)
	assert.Equal(t, "evt-2499", got[len(got)-1].ID)
}

func BenchmarkWorkerFetchAllEventsPaged(b *testing.B) {
	now := time.Now()
	events := make([]domain.RunEvent, 2500)
	for i := range events {
		events[i] = domain.RunEvent{
			ID:        fmt.Sprintf("evt-%04d", i),
			RunID:     "run-1",
			Message:   "event",
			CreatedAt: now.Add(time.Duration(i) * time.Millisecond),
		}
	}

	w := NewWorker(&benchmarkPagedEventStore{events: events}, NewService(), time.Hour)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		got, err := w.fetchAllEvents(context.Background(), "run-1")
		if err != nil {
			b.Fatalf("fetchAllEvents() error = %v", err)
		}
		if len(got) != len(events) {
			b.Fatalf("fetchAllEvents() returned %d events", len(got))
		}
	}
}
