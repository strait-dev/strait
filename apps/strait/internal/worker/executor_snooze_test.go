package worker

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
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
			require.Equal(t,
				tt.want,
				state.count,
			)

		})
	}
}

func TestSnoozeTransitionStateFields(t *testing.T) {
	t.Parallel()

	state := snoozeTransitionState{reason: "endpoint health score below threshold", count: 3}
	fields := state.fields()
	require.Equal(t,
		state.reason,
		fields["error"])
	require.Equal(t,
		domain.ErrorClassTransient,

		fields["error_class"])
	require.Nil(t, fields["started_at"])
	require.Nil(t, fields["finished_at"])

	metadata, ok := fields["metadata"].(map[string]string)
	require.True(t,
		ok)
	require.Equal(t,
		"3", metadata["snooze_count"])

	if _, ok := fields["next_retry_at"]; ok {
		require.Fail(t,

			"next_retry_at must not be stored on job_runs fields")
	}
}

func TestSnoozeTransitionStateExceeds(t *testing.T) {
	t.Parallel()
	require.True(t,
		((snoozeTransitionState{count: 4}).exceeds(3)))
	require.False(t,
		(snoozeTransitionState{count: 3}).exceeds(3))
	require.False(t,
		(snoozeTransitionState{count: 100}).exceeds(0))

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
	require.Len(t, calls,
		1)

	c := calls[0]
	require.False(t,
		c.from !=
			domain.StatusDequeued ||
			c.to !=
				domain.
					StatusQueued)

	meta, ok := c.fields["metadata"].(map[string]string)
	require.True(t,
		ok)
	require.Equal(t,
		"1", meta["snooze_count"])

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
	require.Len(t, calls,
		1)

	meta := calls[0].fields["metadata"].(map[string]string)
	require.Equal(t,
		"6", meta["snooze_count"])

}

func TestSnoozeRun_DoesNotIncrementAttempt(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{}
	exec := newSnoozeTestExecutor(t, store, 0)

	run := testRun(3)
	run.Status = domain.StatusDequeued
	exec.snoozeRun(context.Background(), run, "circuit breaker open", nil)

	calls := store.statusUpdates()
	require.Len(t, calls,
		1)

	if _, hasAttempt := calls[0].fields["attempt"]; hasAttempt {
		require.Fail(t,

			"snooze should not set attempt field")
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
		require.Failf(t, "test failure",

			"next_retry_at must not be in fields map; retry schedule lives in job_retries")
	}
	scheduled := store.scheduleRetries()
	require.Len(t, scheduled,

		1)
	require.True(t,
		scheduled[0].at.Equal(retryAt))

}

func TestSnoozeRun_EmitsReadyEventOnlyForImmediateRequeue(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{}
	exec := newSnoozeTestExecutor(t, store, 0)
	var readyEvents int
	exec.queue = &mockExecQueue{
		enqueueExistingFn: func(_ context.Context, run *domain.JobRun) error {
			require.Equal(t,
				domain.StatusQueued,

				run.Status,
			)

			readyEvents++
			return nil
		},
	}

	immediate := testRun(1)
	immediate.Status = domain.StatusDequeued
	exec.snoozeRun(context.Background(), immediate, "circuit breaker open", nil)
	require.EqualValues(t, 1, readyEvents)

	scheduled := testRun(2)
	scheduled.Status = domain.StatusDequeued
	retryAt := time.Now().Add(time.Minute)
	exec.snoozeRun(context.Background(), scheduled, "health score low", &retryAt)
	require.EqualValues(t, 1, readyEvents)

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
	require.Len(t, calls,
		1)

	f := calls[0].fields

	t.Run("ClearsStartedAt", func(t *testing.T) {
		require.Nil(t, f["started_at"])

	})
	t.Run("ClearsFinishedAt", func(t *testing.T) {
		require.Nil(t, f["finished_at"])

	})
	t.Run("SetsTransientErrorClass", func(t *testing.T) {
		require.Equal(t,
			"transient",
			f["error_class"])

	})
	t.Run("SetsErrorReason", func(t *testing.T) {
		require.Equal(t,
			reason, f["error"])

	})
	t.Run("NoNextRetryAtInFields", func(t *testing.T) {
		if _, ok := f["next_retry_at"]; ok {
			require.Failf(t, "test failure",

				"next_retry_at must not appear in fields map; lives in job_retries side table")
		}
		if scheduled := store.scheduleRetries(); len(scheduled) != 0 {
			require.Failf(t, "test failure",

				"expected no ScheduleRetry calls for nil retryAt, got %d", len(scheduled))
		}
		if cleared := store.clearRetries(); len(cleared) != 1 || cleared[0] != run.ID {
			require.Failf(t, "test failure",

				"expected ClearRetry for run %s, got %v", run.ID, cleared)
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
	require.NotEmpty(t, calls)

	// handleSystemFailure transitions from current status to SystemFailed
	last := calls[len(calls)-1]
	require.Equal(t,
		domain.StatusSystemFailed,

		last.
			to)

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
	require.Len(t, calls,
		1)
	require.Equal(t,
		domain.StatusQueued,

		calls[0].to)

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
	require.Len(t, calls,
		1)
	require.Equal(t,
		domain.StatusQueued,

		calls[0].to)

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
	require.Equal(t,
		"1", meta["snooze_count"])

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
	require.Len(t, events,
		1)

	ev := events[0]
	require.Equal(t,
		EventSnoozed,
		ev.Type,
	)
	require.Equal(t,
		domain.StatusDequeued,

		ev.FromStatus,
	)
	require.Equal(t,
		domain.StatusQueued,

		ev.ToStatus,
	)
	require.EqualValues(t, 2, ev.Attempt)

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
	require.Len(t, events,
		0)

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
	require.Len(t, calls,
		1)
	require.False(t,
		calls[0].
			from != domain.
			StatusExecuting ||
			calls[0].to != domain.StatusQueued,
	)

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
	require.Equal(t,
		"4", meta["snooze_count"])

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
	require.NotEmpty(t, calls)

	last := calls[len(calls)-1]
	require.Equal(t,
		domain.StatusSystemFailed,

		last.
			to)

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
	require.Len(t, events,
		1)

	ev := events[0]
	require.Equal(t,
		EventSnoozed,
		ev.Type,
	)
	require.Equal(t,
		domain.StatusExecuting,

		ev.FromStatus,
	)
	require.Equal(t,
		domain.StatusQueued,

		ev.ToStatus,
	)

}

// errForcedFailure is a sentinel error for tests that need store operations to fail.
var errForcedFailure = errors.New("forced test failure")
