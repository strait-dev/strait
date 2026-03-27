package logdrain

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"strait/internal/domain"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
)

func TestWithEventsCounter(t *testing.T) {
	t.Parallel()

	mp := noop.NewMeterProvider()
	meter := mp.Meter("test")
	counter, err := meter.Int64Counter("test_counter")
	if err != nil {
		t.Fatalf("create counter: %v", err)
	}

	svc := NewService()
	w := NewWorker(nil, svc, time.Hour)
	if w.eventsCounter != nil {
		t.Fatal("eventsCounter should be nil before WithEventsCounter")
	}

	got := w.WithEventsCounter(counter)
	if got != w {
		t.Fatal("WithEventsCounter should return the same worker for chaining")
	}
	if w.eventsCounter == nil {
		t.Fatal("eventsCounter should be set after WithEventsCounter")
	}
}

func TestDrainRunEvents_MarshalError(t *testing.T) {
	// Not parallel: modifies package-level jsonMarshal.
	orig := jsonMarshal
	t.Cleanup(func() { jsonMarshal = orig })

	jsonMarshal = func(v any) ([]byte, error) {
		return nil, fmt.Errorf("injected marshal failure")
	}

	drain := &domain.LogDrain{
		EndpointURL: "http://example.com",
		AuthType:    "",
	}

	svc := NewService()
	err := svc.DrainRunEvents(context.Background(), drain, []domain.RunEvent{
		{ID: "evt-1", RunID: "run-1", Message: "test"},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); got != "marshal events: injected marshal failure" {
		t.Errorf("error = %q, want 'marshal events: injected marshal failure'", got)
	}
}

func TestDrainRunEvents_BearerAuthWithoutToken(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "" {
			t.Errorf("Authorization header should be empty, got %q", auth)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	drain := &domain.LogDrain{
		EndpointURL: srv.URL,
		AuthType:    "bearer",
		AuthConfig:  map[string]string{}, // no "token" key
	}

	svc := NewService()
	err := svc.DrainRunEvents(context.Background(), drain, []domain.RunEvent{
		{ID: "evt-1", RunID: "run-1", Message: "test"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDrainRunEvents_InvalidURL(t *testing.T) {
	t.Parallel()

	drain := &domain.LogDrain{
		EndpointURL: "://invalid-url",
		AuthType:    "",
	}

	svc := NewService()
	err := svc.DrainRunEvents(context.Background(), drain, []domain.RunEvent{
		{ID: "evt-1", RunID: "run-1", Message: "test"},
	})
	if err == nil {
		t.Fatal("expected error for invalid URL, got nil")
	}
}

func TestAdvanceCursor_NoAdvanceWhenBehind(t *testing.T) {
	t.Parallel()

	now := time.Now()

	tests := []struct {
		name       string
		cur        drainCursor
		finishedAt time.Time
		runID      string
		wantCur    drainCursor
	}{
		{
			name:       "earlier timestamp does not advance",
			cur:        drainCursor{FinishedAt: now, RunID: "run-b"},
			finishedAt: now.Add(-1 * time.Minute),
			runID:      "run-z",
			wantCur:    drainCursor{FinishedAt: now, RunID: "run-b"},
		},
		{
			name:       "same timestamp with smaller run ID does not advance",
			cur:        drainCursor{FinishedAt: now, RunID: "run-b"},
			finishedAt: now,
			runID:      "run-a",
			wantCur:    drainCursor{FinishedAt: now, RunID: "run-b"},
		},
		{
			name:       "same timestamp with same run ID does not advance",
			cur:        drainCursor{FinishedAt: now, RunID: "run-b"},
			finishedAt: now,
			runID:      "run-b",
			wantCur:    drainCursor{FinishedAt: now, RunID: "run-b"},
		},
		{
			name:       "same timestamp with larger run ID advances",
			cur:        drainCursor{FinishedAt: now, RunID: "run-b"},
			finishedAt: now,
			runID:      "run-c",
			wantCur:    drainCursor{FinishedAt: now, RunID: "run-c"},
		},
		{
			name:       "later timestamp advances",
			cur:        drainCursor{FinishedAt: now, RunID: "run-z"},
			finishedAt: now.Add(1 * time.Minute),
			runID:      "run-a",
			wantCur:    drainCursor{FinishedAt: now.Add(1 * time.Minute), RunID: "run-a"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := advanceCursor(tc.cur, tc.finishedAt, tc.runID)
			if !got.FinishedAt.Equal(tc.wantCur.FinishedAt) || got.RunID != tc.wantCur.RunID {
				t.Errorf("advanceCursor() = {%v, %q}, want {%v, %q}",
					got.FinishedAt, got.RunID, tc.wantCur.FinishedAt, tc.wantCur.RunID)
			}
		})
	}
}

// nilFinishedAtStore returns runs with nil FinishedAt, bypassing the mock's
// filtering that skips nil FinishedAt runs.
type nilFinishedAtStore struct {
	runs   []domain.JobRun
	events map[string][]domain.RunEvent
	called bool
}

func (s *nilFinishedAtStore) ListLogDrains(_ context.Context, _ string) ([]domain.LogDrain, error) {
	return nil, nil
}

func (s *nilFinishedAtStore) ListEnabledLogDrains(_ context.Context) ([]domain.LogDrain, error) {
	return nil, nil
}

func (s *nilFinishedAtStore) ListFinishedRunsSince(_ context.Context, _ string, _ time.Time, _ string, _ int) ([]domain.JobRun, error) {
	if s.called {
		return nil, nil
	}
	s.called = true
	return s.runs, nil
}

func (s *nilFinishedAtStore) ListEventsAsc(_ context.Context, runID string, limit int, _ *time.Time, _ string) ([]domain.RunEvent, error) {
	evts := s.events[runID]
	if len(evts) > limit {
		evts = evts[:limit]
	}
	return evts, nil
}

func TestProcessDrain_RunWithNilFinishedAt(t *testing.T) {
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

	store := &nilFinishedAtStore{
		runs: []domain.JobRun{
			{ID: "run-1", ProjectID: "proj-1", FinishedAt: nil, Status: domain.StatusCompleted},
		},
		events: map[string][]domain.RunEvent{
			"run-1": {{ID: "evt-1", RunID: "run-1", Message: "done", CreatedAt: now}},
		},
	}
	svc := NewService()
	w := NewWorker(store, svc, time.Hour)

	drain := &domain.LogDrain{ID: "drain-1", ProjectID: "proj-1", EndpointURL: srv.URL, Enabled: true}
	w.processDrain(context.Background(), drain)

	mu.Lock()
	defer mu.Unlock()
	if requestCount != 1 {
		t.Errorf("expected 1 HTTP request, got %d", requestCount)
	}

	w.mu.Lock()
	cp := w.checkpoints["drain-1"]
	w.mu.Unlock()
	if !cp.FinishedAt.IsZero() {
		t.Errorf("checkpoint should remain zero when FinishedAt is nil, got %v", cp.FinishedAt)
	}
}

func TestProcessDrain_EmptyEventsRunWithNilFinishedAt(t *testing.T) {
	t.Parallel()

	store := &nilFinishedAtStore{
		runs: []domain.JobRun{
			{ID: "run-1", ProjectID: "proj-1", FinishedAt: nil, Status: domain.StatusCompleted},
		},
		events: map[string][]domain.RunEvent{
			"run-1": {},
		},
	}
	svc := NewService()
	w := NewWorker(store, svc, time.Hour)

	drain := &domain.LogDrain{ID: "drain-1", ProjectID: "proj-1", EndpointURL: "http://example.com", Enabled: true}
	w.processDrain(context.Background(), drain)

	w.mu.Lock()
	cp := w.checkpoints["drain-1"]
	w.mu.Unlock()
	if !cp.FinishedAt.IsZero() {
		t.Errorf("checkpoint should remain zero for nil FinishedAt, got %v", cp.FinishedAt)
	}
}

func TestProcessDrain_PoisonRunWithNilFinishedAt(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	now := time.Now()

	store := &nilFinishedAtStore{
		runs: []domain.JobRun{
			{ID: "run-1", ProjectID: "proj-1", FinishedAt: nil, Status: domain.StatusCompleted},
		},
		events: map[string][]domain.RunEvent{
			"run-1": {{ID: "evt-1", RunID: "run-1", Message: "done", CreatedAt: now}},
		},
	}
	svc := NewService()
	w := NewWorker(store, svc, time.Hour)

	drain := &domain.LogDrain{ID: "drain-1", ProjectID: "proj-1", EndpointURL: srv.URL, Enabled: true}
	ctx := context.Background()

	// Exhaust retries -- each processDrain call will see the run and fail.
	for range maxRunRetries {
		store.called = false
		w.processDrain(ctx, drain)
	}
	// One more call triggers the poison skip path with nil FinishedAt.
	store.called = false
	w.processDrain(ctx, drain)

	w.mu.Lock()
	cp := w.checkpoints["drain-1"]
	w.mu.Unlock()
	if !cp.FinishedAt.IsZero() {
		t.Errorf("checkpoint should remain zero for poison run with nil FinishedAt, got %v", cp.FinishedAt)
	}
}

func TestProcessDrain_ContextCancelledInInnerLoop(t *testing.T) {
	t.Parallel()

	callCount := 0
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		callCount++
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

	// Cancel context before processDrain's inner loop can process run-2.
	ctx, cancel := context.WithCancel(context.Background())

	// Override the store to cancel context after first run's events are fetched.
	cancellingStore := &contextCancellingStore{
		mockDrainStore: store,
		cancelAfterRun: "run-1",
		cancel:         cancel,
	}
	w.store = cancellingStore

	w.processDrain(ctx, &store.enabledDrains[0])

	// Only run-1 should have been delivered; run-2 should be skipped due to cancelled context.
	mu.Lock()
	defer mu.Unlock()
	if callCount > 1 {
		t.Errorf("expected at most 1 delivery before context cancel, got %d", callCount)
	}
}

// contextCancellingStore wraps mockDrainStore and cancels context after
// events are fetched for a specific run.
type contextCancellingStore struct {
	*mockDrainStore
	cancelAfterRun string
	cancel         context.CancelFunc
	cancelled      bool
}

func (s *contextCancellingStore) ListEventsAsc(ctx context.Context, runID string, limit int, afterTime *time.Time, afterID string) ([]domain.RunEvent, error) {
	result, err := s.mockDrainStore.ListEventsAsc(ctx, runID, limit, afterTime, afterID)
	if runID == s.cancelAfterRun && !s.cancelled {
		s.cancelled = true
		s.cancel()
	}
	return result, err
}

func TestProcessDrain_DrainErrorWithEventsCounter(t *testing.T) {
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

	mp := noop.NewMeterProvider()
	meter := mp.Meter("test")
	counter, err := meter.Int64Counter("log_drain_events")
	if err != nil {
		t.Fatalf("create counter: %v", err)
	}

	svc := NewService()
	w := NewWorker(store, svc, time.Hour).WithEventsCounter(counter)

	ctx := context.Background()
	w.tick(ctx)

	// Verify fail count was incremented (error path with counter).
	w.mu.Lock()
	fc := w.failCounts["drain-1:run-1"]
	w.mu.Unlock()
	if fc != 1 {
		t.Errorf("fail count = %d, want 1", fc)
	}
}

func TestProcessDrain_SuccessWithEventsCounter(t *testing.T) {
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
		events: map[string][]domain.RunEvent{
			"run-1": {{ID: "evt-1", RunID: "run-1", Message: "done", CreatedAt: now}},
		},
	}

	mp := noop.NewMeterProvider()
	meter := mp.Meter("test")
	counter, err := meter.Int64Counter("log_drain_events")
	if err != nil {
		t.Fatalf("create counter: %v", err)
	}

	svc := NewService()
	w := NewWorker(store, svc, time.Hour).WithEventsCounter(counter)

	ctx := context.Background()
	w.tick(ctx)

	// Verify checkpoint advanced (success path with counter).
	w.mu.Lock()
	cp := w.checkpoints["drain-1"]
	w.mu.Unlock()
	if !cp.FinishedAt.Equal(finishedAt) {
		t.Errorf("checkpoint.FinishedAt = %v, want %v", cp.FinishedAt, finishedAt)
	}
}

// Ensure the unused import is satisfied.
var _ metric.Int64Counter = (metric.Int64Counter)(nil)
var _ = fmt.Sprintf
