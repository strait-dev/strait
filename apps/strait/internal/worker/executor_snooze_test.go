package worker

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"

	"strait/internal/domain"
)

// newSnoozeTestExecutor creates an Executor with the given maxSnoozeCount
// and event channel, suitable for testing snoozeRun and snoozeRunFromExecuting.
func newSnoozeTestExecutor(t *testing.T, store *mockExecutorStore, maxSnoozeCount int) *Executor {
	t.Helper()

	pool := NewPool(4)
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })

	exec := NewExecutor(ExecutorConfig{
		Pool:           pool,
		Queue:          &mockExecQueue{},
		Store:          store,
		PollInterval:   time.Millisecond,
		MaxSnoozeCount: maxSnoozeCount,
	})
	return exec
}

func TestSnoozeTransitionState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		metadata map[string]string
		want     int
	}{
		{name: "nil metadata", want: 1},
		{name: "empty metadata", metadata: map[string]string{}, want: 1},
		{name: "existing count", metadata: map[string]string{"snooze_count": "5"}, want: 6},
		{name: "malformed count", metadata: map[string]string{"snooze_count": "not-a-number"}, want: 1},
		{name: "negative count", metadata: map[string]string{"snooze_count": "-5"}, want: -4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			run := testRun(1)
			run.Metadata = tt.metadata
			state := newSnoozeTransitionState(run, "reason")
			if state.count != tt.want {
				t.Fatalf("count = %d, want %d", state.count, tt.want)
			}
		})
	}
}

func TestSnoozeTransitionStateFields(t *testing.T) {
	t.Parallel()

	state := snoozeTransitionState{reason: "endpoint health score below threshold", count: 3}
	fields := state.fields()

	if fields["error"] != state.reason {
		t.Fatalf("error = %v, want %q", fields["error"], state.reason)
	}
	if fields["error_class"] != domain.ErrorClassTransient {
		t.Fatalf("error_class = %v, want %q", fields["error_class"], domain.ErrorClassTransient)
	}
	if fields["started_at"] != nil {
		t.Fatalf("started_at = %v, want nil", fields["started_at"])
	}
	if fields["finished_at"] != nil {
		t.Fatalf("finished_at = %v, want nil", fields["finished_at"])
	}
	metadata, ok := fields["metadata"].(map[string]string)
	if !ok {
		t.Fatalf("metadata = %T, want map[string]string", fields["metadata"])
	}
	if metadata["snooze_count"] != "3" {
		t.Fatalf("snooze_count = %q, want 3", metadata["snooze_count"])
	}
	if _, ok := fields["next_retry_at"]; ok {
		t.Fatal("next_retry_at must not be stored on job_runs fields")
	}
}

func TestSnoozeTransitionStateExceeds(t *testing.T) {
	t.Parallel()

	if !((snoozeTransitionState{count: 4}).exceeds(3)) {
		t.Fatal("count 4 should exceed max 3")
	}
	if (snoozeTransitionState{count: 3}).exceeds(3) {
		t.Fatal("count 3 should not exceed max 3")
	}
	if (snoozeTransitionState{count: 100}).exceeds(0) {
		t.Fatal("max 0 disables snooze cap")
	}
}

// collectEvents subscribes a test subscriber that collects events into a slice.
// Returns a function that polls until at least one event arrives (or 2s timeout),
// then returns all collected events. Thread-safe.
func collectEvents(exec *Executor) func() []RunLifecycleEvent {
	var mu sync.Mutex
	var events []RunLifecycleEvent

	exec.Subscribe(func(_ context.Context, event RunLifecycleEvent) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	})
	go exec.runEventLoop()

	return func() []RunLifecycleEvent {
		deadline := time.After(2 * time.Second)
		ticker := time.NewTicker(time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				mu.Lock()
				n := len(events)
				mu.Unlock()
				if n > 0 {
					// Yield to let any in-flight concurrent emits land.
					runtime.Gosched()
					mu.Lock()
					out := make([]RunLifecycleEvent, len(events))
					copy(out, events)
					mu.Unlock()
					return out
				}
			case <-deadline:
				mu.Lock()
				out := make([]RunLifecycleEvent, len(events))
				copy(out, events)
				mu.Unlock()
				return out
			}
		}
	}
}

// snoozeRun tests (StatusDequeued -> StatusQueued).

func TestSnoozeRun_IncrementsSnoozeCount(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{}
	exec := newSnoozeTestExecutor(t, store, 0)

	run := testRun(1)
	run.Status = domain.StatusDequeued
	exec.snoozeRun(context.Background(), run, "circuit breaker open", nil)

	calls := store.statusUpdates()
	if len(calls) != 1 {
		t.Fatalf("expected 1 status update, got %d", len(calls))
	}
	c := calls[0]
	if c.from != domain.StatusDequeued || c.to != domain.StatusQueued {
		t.Fatalf("expected Dequeued->Queued, got %s->%s", c.from, c.to)
	}
	meta, ok := c.fields["metadata"].(map[string]string)
	if !ok {
		t.Fatalf("expected metadata to be map[string]string, got %T", c.fields["metadata"])
	}
	if meta["snooze_count"] != "1" {
		t.Fatalf("expected snooze_count=1, got %q", meta["snooze_count"])
	}
}

func TestSnoozeRun_IncrementsExistingSnoozeCount(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{}
	exec := newSnoozeTestExecutor(t, store, 0)

	run := testRun(1)
	run.Status = domain.StatusDequeued
	run.Metadata = map[string]string{"snooze_count": "5"}
	exec.snoozeRun(context.Background(), run, "bulkhead full", nil)

	calls := store.statusUpdates()
	if len(calls) != 1 {
		t.Fatalf("expected 1 status update, got %d", len(calls))
	}
	meta := calls[0].fields["metadata"].(map[string]string)
	if meta["snooze_count"] != "6" {
		t.Fatalf("expected snooze_count=6, got %q", meta["snooze_count"])
	}
}

func TestSnoozeRun_DoesNotIncrementAttempt(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{}
	exec := newSnoozeTestExecutor(t, store, 0)

	run := testRun(3)
	run.Status = domain.StatusDequeued
	exec.snoozeRun(context.Background(), run, "circuit breaker open", nil)

	calls := store.statusUpdates()
	if len(calls) != 1 {
		t.Fatalf("expected 1 status update, got %d", len(calls))
	}
	if _, hasAttempt := calls[0].fields["attempt"]; hasAttempt {
		t.Fatal("snooze should not set attempt field")
	}
}

func TestSnoozeRun_SetsRetryAt(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{}
	exec := newSnoozeTestExecutor(t, store, 0)

	retryAt := time.Now().Add(30 * time.Second)
	run := testRun(1)
	run.Status = domain.StatusDequeued
	exec.snoozeRun(context.Background(), run, "health score low", &retryAt)

	calls := store.statusUpdates()
	if _, ok := calls[0].fields["next_retry_at"]; ok {
		t.Fatalf("next_retry_at must not be in fields map; retry schedule lives in job_retries")
	}
	scheduled := store.scheduleRetries()
	if len(scheduled) != 1 {
		t.Fatalf("expected 1 ScheduleRetry call, got %d", len(scheduled))
	}
	if !scheduled[0].at.Equal(retryAt) {
		t.Fatalf("expected ScheduleRetry at=%v, got %v", retryAt, scheduled[0].at)
	}
}

func TestSnoozeRun_EmitsReadyEventOnlyForImmediateRequeue(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{}
	exec := newSnoozeTestExecutor(t, store, 0)
	var readyEvents int
	exec.queue = &mockExecQueue{
		enqueueExistingFn: func(_ context.Context, run *domain.JobRun) error {
			if run.Status != domain.StatusQueued {
				t.Fatalf("ready event status = %q, want queued", run.Status)
			}
			readyEvents++
			return nil
		},
	}

	immediate := testRun(1)
	immediate.Status = domain.StatusDequeued
	exec.snoozeRun(context.Background(), immediate, "circuit breaker open", nil)
	if readyEvents != 1 {
		t.Fatalf("ready events after immediate snooze = %d, want 1", readyEvents)
	}

	scheduled := testRun(2)
	scheduled.Status = domain.StatusDequeued
	retryAt := time.Now().Add(time.Minute)
	exec.snoozeRun(context.Background(), scheduled, "health score low", &retryAt)
	if readyEvents != 1 {
		t.Fatalf("ready events after scheduled retry = %d, want still 1", readyEvents)
	}
}

func TestSnoozeRun_FieldsMap(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{}
	exec := newSnoozeTestExecutor(t, store, 0)

	reason := "circuit breaker open for endpoint"
	run := testRun(1)
	run.Status = domain.StatusDequeued
	exec.snoozeRun(context.Background(), run, reason, nil)

	calls := store.statusUpdates()
	if len(calls) != 1 {
		t.Fatalf("expected 1 status update, got %d", len(calls))
	}
	f := calls[0].fields

	t.Run("ClearsStartedAt", func(t *testing.T) {
		if f["started_at"] != nil {
			t.Fatalf("expected started_at=nil, got %v", f["started_at"])
		}
	})
	t.Run("ClearsFinishedAt", func(t *testing.T) {
		if f["finished_at"] != nil {
			t.Fatalf("expected finished_at=nil, got %v", f["finished_at"])
		}
	})
	t.Run("SetsTransientErrorClass", func(t *testing.T) {
		if f["error_class"] != "transient" {
			t.Fatalf("expected error_class=transient, got %v", f["error_class"])
		}
	})
	t.Run("SetsErrorReason", func(t *testing.T) {
		if f["error"] != reason {
			t.Fatalf("expected error=%q, got %v", reason, f["error"])
		}
	})
	t.Run("NoNextRetryAtInFields", func(t *testing.T) {
		if _, ok := f["next_retry_at"]; ok {
			t.Fatalf("next_retry_at must not appear in fields map; lives in job_retries side table")
		}
		if scheduled := store.scheduleRetries(); len(scheduled) != 0 {
			t.Fatalf("expected no ScheduleRetry calls for nil retryAt, got %d", len(scheduled))
		}
		if cleared := store.clearRetries(); len(cleared) != 1 || cleared[0] != run.ID {
			t.Fatalf("expected ClearRetry for run %s, got %v", run.ID, cleared)
		}
	})
}

func TestSnoozeRun_MaxSnoozeExceeded_SystemFails(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{}
	exec := newSnoozeTestExecutor(t, store, 2) // max=2

	run := testRun(1)
	run.Status = domain.StatusDequeued
	run.Metadata = map[string]string{"snooze_count": "2"} // will increment to 3, exceeds 2

	exec.snoozeRun(context.Background(), run, "circuit breaker open", nil)

	calls := store.statusUpdates()
	if len(calls) == 0 {
		t.Fatal("expected at least one status update")
	}
	// handleSystemFailure transitions from current status to SystemFailed
	last := calls[len(calls)-1]
	if last.to != domain.StatusSystemFailed {
		t.Fatalf("expected transition to SystemFailed, got %s", last.to)
	}
}

func TestSnoozeRun_MaxSnoozeAtBoundary_StillSnoozes(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{}
	exec := newSnoozeTestExecutor(t, store, 3) // max=3

	run := testRun(1)
	run.Status = domain.StatusDequeued
	run.Metadata = map[string]string{"snooze_count": "2"} // will increment to 3, equals max

	exec.snoozeRun(context.Background(), run, "circuit breaker open", nil)

	calls := store.statusUpdates()
	if len(calls) != 1 {
		t.Fatalf("expected 1 status update, got %d", len(calls))
	}
	if calls[0].to != domain.StatusQueued {
		t.Fatalf("expected Queued at boundary (3 == max 3), got %s", calls[0].to)
	}
}

func TestSnoozeRun_MaxSnoozeZero_Disabled(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{}
	exec := newSnoozeTestExecutor(t, store, 0) // 0 = disabled

	run := testRun(1)
	run.Status = domain.StatusDequeued
	run.Metadata = map[string]string{"snooze_count": "100"}

	exec.snoozeRun(context.Background(), run, "circuit breaker open", nil)

	calls := store.statusUpdates()
	if len(calls) != 1 {
		t.Fatalf("expected 1 status update, got %d", len(calls))
	}
	if calls[0].to != domain.StatusQueued {
		t.Fatalf("expected Queued (max disabled), got %s", calls[0].to)
	}
}

func TestSnoozeRun_MalformedMetadata_TreatsAsZero(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{}
	exec := newSnoozeTestExecutor(t, store, 0)

	run := testRun(1)
	run.Status = domain.StatusDequeued
	run.Metadata = map[string]string{"snooze_count": "not-a-number"}

	exec.snoozeRun(context.Background(), run, "circuit breaker open", nil)

	calls := store.statusUpdates()
	meta := calls[0].fields["metadata"].(map[string]string)
	if meta["snooze_count"] != "1" {
		t.Fatalf("expected snooze_count=1 after malformed input, got %q", meta["snooze_count"])
	}
}

func TestSnoozeRun_EmitsEventSnoozed(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{}
	exec := newSnoozeTestExecutor(t, store, 0)
	getEvents := collectEvents(exec)
	t.Cleanup(func() { close(exec.eventCh) })

	run := testRun(2)
	run.Status = domain.StatusDequeued
	exec.snoozeRun(context.Background(), run, "bulkhead full", nil)

	events := getEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	ev := events[0]
	if ev.Type != EventSnoozed {
		t.Fatalf("expected EventSnoozed, got %s", ev.Type)
	}
	if ev.FromStatus != domain.StatusDequeued {
		t.Fatalf("expected FromStatus=Dequeued, got %s", ev.FromStatus)
	}
	if ev.ToStatus != domain.StatusQueued {
		t.Fatalf("expected ToStatus=Queued, got %s", ev.ToStatus)
	}
	if ev.Attempt != 2 {
		t.Fatalf("expected Attempt=2, got %d", ev.Attempt)
	}
}

func TestSnoozeRun_UpdateStatusError_NoEmit(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{
		updateRunStatusFn: func(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error {
			return errForcedFailure
		},
	}
	exec := newSnoozeTestExecutor(t, store, 0)
	getEvents := collectEvents(exec)
	t.Cleanup(func() { close(exec.eventCh) })

	run := testRun(1)
	run.Status = domain.StatusDequeued
	exec.snoozeRun(context.Background(), run, "circuit breaker open", nil)

	events := getEvents()
	if len(events) != 0 {
		t.Fatalf("expected 0 events after store error, got %d", len(events))
	}
}

// snoozeRunFromExecuting tests (StatusExecuting -> StatusQueued).

func TestSnoozeRunFromExecuting_TransitionsCorrectly(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{}
	exec := newSnoozeTestExecutor(t, store, 0)

	run := testRun(1)
	run.Status = domain.StatusExecuting
	exec.snoozeRunFromExecuting(context.Background(), run, "container create timeout", nil)

	calls := store.statusUpdates()
	if len(calls) != 1 {
		t.Fatalf("expected 1 status update, got %d", len(calls))
	}
	if calls[0].from != domain.StatusExecuting || calls[0].to != domain.StatusQueued {
		t.Fatalf("expected Executing->Queued, got %s->%s", calls[0].from, calls[0].to)
	}
}

func TestSnoozeRunFromExecuting_IncrementsSnoozeCount(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{}
	exec := newSnoozeTestExecutor(t, store, 0)

	run := testRun(1)
	run.Status = domain.StatusExecuting
	run.Metadata = map[string]string{"snooze_count": "3"}
	exec.snoozeRunFromExecuting(context.Background(), run, "container error", nil)

	calls := store.statusUpdates()
	meta := calls[0].fields["metadata"].(map[string]string)
	if meta["snooze_count"] != "4" {
		t.Fatalf("expected snooze_count=4, got %q", meta["snooze_count"])
	}
}

func TestSnoozeRunFromExecuting_MaxSnoozeExceeded(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{}
	exec := newSnoozeTestExecutor(t, store, 2)

	run := testRun(1)
	run.Status = domain.StatusExecuting
	run.Metadata = map[string]string{"snooze_count": "2"}

	exec.snoozeRunFromExecuting(context.Background(), run, "container error", nil)

	calls := store.statusUpdates()
	if len(calls) == 0 {
		t.Fatal("expected at least one status update")
	}
	last := calls[len(calls)-1]
	if last.to != domain.StatusSystemFailed {
		t.Fatalf("expected SystemFailed, got %s", last.to)
	}
}

func TestSnoozeRunFromExecuting_EmitsEventSnoozed(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{}
	exec := newSnoozeTestExecutor(t, store, 0)
	getEvents := collectEvents(exec)
	t.Cleanup(func() { close(exec.eventCh) })

	run := testRun(1)
	run.Status = domain.StatusExecuting
	exec.snoozeRunFromExecuting(context.Background(), run, "container error", nil)

	events := getEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	ev := events[0]
	if ev.Type != EventSnoozed {
		t.Fatalf("expected EventSnoozed, got %s", ev.Type)
	}
	if ev.FromStatus != domain.StatusExecuting {
		t.Fatalf("expected FromStatus=Executing, got %s", ev.FromStatus)
	}
	if ev.ToStatus != domain.StatusQueued {
		t.Fatalf("expected ToStatus=Queued, got %s", ev.ToStatus)
	}
}

// errForcedFailure is a sentinel error for tests that need store operations to fail.
var errForcedFailure = errors.New("forced test failure")
