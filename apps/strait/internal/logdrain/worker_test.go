package logdrain

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"strait/internal/domain"
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
	logDrains       map[string][]domain.LogDrain // projectID -> drains
}

func (m *mockDrainStore) ListLogDrains(_ context.Context, projectID string) ([]domain.LogDrain, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.logDrains[projectID], nil
}

func (m *mockDrainStore) ListEvents(_ context.Context, runID string, _ int, _ *time.Time) ([]domain.RunEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.eventsErr != nil {
		return nil, m.eventsErr
	}
	return m.events[runID], nil
}

func (m *mockDrainStore) ListEnabledLogDrains(_ context.Context) ([]domain.LogDrain, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.enabledErr != nil {
		return nil, m.enabledErr
	}
	return m.enabledDrains, nil
}

func (m *mockDrainStore) ListFinishedRunsSince(_ context.Context, projectID string, _ time.Time, _ int) ([]domain.JobRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.finishedRunsErr != nil {
		return nil, m.finishedRunsErr
	}
	return m.finishedRuns[projectID], nil
}

func TestWorker_StartStop(t *testing.T) {
	t.Parallel()
	svc := NewService()
	w := NewWorker(nil, svc, 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not stop within 2s after context cancel")
	}
}

func TestWorker_StopMethod(t *testing.T) {
	t.Parallel()
	svc := NewService()
	w := NewWorker(nil, svc, 50*time.Millisecond)

	ctx := context.Background()
	go w.Run(ctx)
	time.Sleep(100 * time.Millisecond)

	stopped := make(chan struct{})
	go func() {
		w.Stop()
		close(stopped)
	}()

	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() did not return within 2s")
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
	// No panic, no error - just returns.
}

func TestWorker_Tick_DrainDisabled(t *testing.T) {
	t.Parallel()
	store := &mockDrainStore{
		enabledDrains: []domain.LogDrain{}, // empty - all disabled
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
	// Should log error and return without panic.
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
				{ID: "evt-1", RunID: "run-1", Message: "started"},
				{ID: "evt-2", RunID: "run-1", Message: "completed"},
			},
		},
	}
	svc := NewService()
	w := NewWorker(store, svc, time.Hour)

	ctx := context.Background()
	w.tick(ctx)

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 2 {
		t.Fatalf("expected 2 events, got %d", len(received))
	}
	if received[0].ID != "evt-1" {
		t.Errorf("first event ID = %q, want evt-1", received[0].ID)
	}
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
			"run-1": {{ID: "evt-1", RunID: "run-1", Message: "done"}},
			"run-2": {{ID: "evt-2", RunID: "run-2", Message: "failed"}},
		},
	}
	svc := NewService()
	w := NewWorker(store, svc, time.Hour)

	ctx := context.Background()
	w.tick(ctx)

	mu.Lock()
	defer mu.Unlock()
	if deliveryCount != 2 {
		t.Errorf("expected 2 deliveries, got %d", deliveryCount)
	}
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
			"run-1": {{ID: "evt-1", RunID: "run-1", Message: "done"}},
			"run-2": {{ID: "evt-2", RunID: "run-2", Message: "done"}},
		},
	}
	svc := NewService()
	w := NewWorker(store, svc, time.Hour)

	ctx := context.Background()
	w.tick(ctx)

	w.mu.Lock()
	cp := w.checkpoints["drain-1"]
	w.mu.Unlock()

	if !cp.Equal(f2) {
		t.Errorf("checkpoint = %v, want %v", cp, f2)
	}
}

func TestWorker_Tick_DeliveryErrorStopsCheckpointUpdate(t *testing.T) {
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
			"run-1": {{ID: "evt-1", RunID: "run-1", Message: "done"}},
		},
	}
	svc := NewService()
	w := NewWorker(store, svc, time.Hour)

	ctx := context.Background()
	w.tick(ctx)

	w.mu.Lock()
	cp := w.checkpoints["drain-1"]
	w.mu.Unlock()

	if !cp.IsZero() {
		t.Errorf("checkpoint should not be updated on delivery failure, got %v", cp)
	}
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
			"run-1": {}, // no events
		},
	}
	svc := NewService()
	w := NewWorker(store, svc, time.Hour)

	ctx := context.Background()
	w.tick(ctx)

	mu.Lock()
	defer mu.Unlock()
	if requestCount != 0 {
		t.Errorf("expected 0 HTTP requests for empty events, got %d", requestCount)
	}
}

func TestWorker_Tick_NilStore(t *testing.T) {
	t.Parallel()
	svc := NewService()
	w := NewWorker(nil, svc, time.Hour)

	ctx := context.Background()
	w.tick(ctx)
	// Should return immediately without panic.
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
	// Should log error and continue without panic.
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
			"run-1": {{ID: "evt-1", RunID: "run-1", Message: "done"}},
			"run-2": {{ID: "evt-2", RunID: "run-2", Message: "done"}},
		},
	}
	svc := NewService()
	w := NewWorker(store, svc, time.Hour)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	w.tick(ctx)
	// Should return early without processing all drains.
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
	// Should log error and return.
}
