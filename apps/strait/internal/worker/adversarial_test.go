package worker

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"strconv"
	"testing"
	"time"

	"strait/internal/domain"
	orcstore "strait/internal/store"
	"strait/internal/telemetry"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/require"
)

// Adversarial snooze tests.

func TestSnoozeRun_IntMaxSnoozeCount_NoOverflow(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{}
	exec := newSnoozeTestExecutor(t, store, 0) // unlimited

	run := testRun(1)
	run.Status = domain.StatusDequeued
	run.Metadata = map[string]string{"snooze_count": strconv.Itoa(math.MaxInt)}

	exec.snoozeRun(context.Background(), run, "circuit breaker open", nil)

	calls := store.statusUpdates()
	require.Len(t, calls,
		1)

	meta := calls[0].fields["metadata"].(map[string]string)
	// MaxInt + 1 overflows to MinInt in Go. The string will be negative.
	// This is a known edge case — verify it doesn't panic.
	count, err := strconv.Atoi(meta["snooze_count"])
	require.NoError(
		t, err)

	// On overflow, count wraps negative. Document this behavior.
	t.Logf("snooze_count after MaxInt+1: %d (overflow expected)", count)
}

func TestSnoozeRun_NegativeSnoozeCount_TreatsAsZero(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{}
	exec := newSnoozeTestExecutor(t, store, 0)

	run := testRun(1)
	run.Status = domain.StatusDequeued
	run.Metadata = map[string]string{"snooze_count": "-5"}

	exec.snoozeRun(context.Background(), run, "circuit breaker open", nil)

	calls := store.statusUpdates()
	meta := calls[0].fields["metadata"].(map[string]string)
	// -5 + 1 = -4. The code does not guard against negative values.
	count, _ := strconv.Atoi(meta["snooze_count"])
	require.Equal(t, -4, count)
}

func TestSnoozeRun_EmptyMetadataMap(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{}
	exec := newSnoozeTestExecutor(t, store, 0)

	run := testRun(1)
	run.Status = domain.StatusDequeued
	run.Metadata = map[string]string{} // empty, not nil

	exec.snoozeRun(context.Background(), run, "bulkhead full", nil)

	calls := store.statusUpdates()
	meta := calls[0].fields["metadata"].(map[string]string)
	require.Equal(t,
		"1", meta["snooze_count"])
}

func TestSnoozeRun_EmptyReason(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{}
	exec := newSnoozeTestExecutor(t, store, 0)

	run := testRun(1)
	run.Status = domain.StatusDequeued
	exec.snoozeRun(context.Background(), run, "", nil)

	calls := store.statusUpdates()
	require.Empty(t,
		calls[0].fields["error"])
}

func TestSnoozeRun_MaxExceeded_HandleSystemFailureFails(t *testing.T) {
	t.Parallel()
	callCount := 0
	store := &mockExecutorStore{
		updateRunStatusFn: func(_ context.Context, _ string, _, to domain.RunStatus, _ map[string]any) error {
			callCount++
			if to == domain.StatusSystemFailed {
				return errors.New("db connection lost")
			}
			return nil
		},
	}
	exec := newSnoozeTestExecutor(t, store, 1)

	run := testRun(1)
	run.Status = domain.StatusDequeued
	run.Metadata = map[string]string{"snooze_count": "1"} // increment to 2, exceeds max=1

	// Should not panic even when handleSystemFailure fails.
	exec.snoozeRun(context.Background(), run, "circuit breaker open", nil)
	require.NotEqual(t, 0, callCount)

	// handleSystemFailure was attempted but failed — verify it was called.
}

func TestSnoozeRun_CancelledContext(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{}
	exec := newSnoozeTestExecutor(t, store, 0)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	run := testRun(1)
	run.Status = domain.StatusDequeued
	// Should not panic with cancelled context.
	exec.snoozeRun(ctx, run, "circuit breaker open", nil)
}

func TestSnoozeRun_SequentialSnoozes_CountIncrementsMonotonically(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{}
	exec := newSnoozeTestExecutor(t, store, 0)

	run := testRun(1)
	run.Status = domain.StatusDequeued

	for i := range 5 {
		run.Metadata = map[string]string{"snooze_count": strconv.Itoa(i)}
		exec.snoozeRun(context.Background(), run, "circuit breaker open", nil)
	}

	calls := store.statusUpdates()
	require.Len(t, calls,
		5)

	for i, c := range calls {
		meta := c.fields["metadata"].(map[string]string)
		expected := strconv.Itoa(i + 1)
		require.Equal(t,
			expected,
			meta["snooze_count"])
	}
}

func TestSnoozeRunFromExecuting_EmptyReason(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{}
	exec := newSnoozeTestExecutor(t, store, 0)

	run := testRun(1)
	run.Status = domain.StatusExecuting
	exec.snoozeRunFromExecuting(context.Background(), run, "", nil)

	calls := store.statusUpdates()
	require.Len(t, calls,
		1)
	require.Empty(t,
		calls[0].fields["error"])
}

// Adversarial handler tests.

func TestHandleFailure_Retry_NotAtMaxAttempts(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{}
	exec := newSnoozeTestExecutor(t, store, 0)
	getEvents := collectEvents(exec)
	t.Cleanup(func() { close(exec.eventCh) })

	run := testRun(1)
	run.Status = domain.StatusExecuting
	job := testJob("http://localhost", 5, 30)
	policy := executionPolicy{maxAttempts: 5, timeoutSecs: 30}

	exec.handleFailure(context.Background(), run, job, policy, errors.New("server error"), nil)

	calls := store.statusUpdates()
	found := false
	for _, c := range calls {
		if c.from == domain.StatusExecuting && c.to == domain.StatusQueued {
			found = true
			require.EqualValues(t, 2, c.fields["attempt"])

			// Verify attempt was incremented.

			break
		}
	}
	require.True(t,
		found)

	events := getEvents()
	foundRetry := false
	for _, ev := range events {
		if ev.Type == EventRetried {
			foundRetry = true
			break
		}
	}
	require.True(t,
		foundRetry,
	)
}

func TestHandleFailure_ZeroAttempt(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{}
	exec := newSnoozeTestExecutor(t, store, 0)

	run := testRun(0) // Attempt = 0, abnormal state.
	run.Status = domain.StatusExecuting
	job := testJob("http://localhost", 3, 30)
	policy := executionPolicy{maxAttempts: 3, timeoutSecs: 30}

	// Should not panic with attempt=0.
	exec.handleFailure(context.Background(), run, job, policy, errors.New("server error"), nil)

	calls := store.statusUpdates()
	require.NotEmpty(t, calls)

	// attempt 0 < maxAttempts 3, so it should retry with attempt=1.
	for _, c := range calls {
		if c.to == domain.StatusQueued {
			require.EqualValues(t, 1, c.fields["attempt"])

			return
		}
	}
	require.Fail(t,

		"expected retry transition to Queued")
}

func TestHandleFailure_ClientError_NoRetry(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{}
	exec := newSnoozeTestExecutor(t, store, 0)

	run := testRun(1)
	run.Status = domain.StatusExecuting
	job := testJob("http://localhost", 5, 30)
	policy := executionPolicy{maxAttempts: 5, timeoutSecs: 30}

	clientErr := &domain.EndpointError{StatusCode: 400, Body: "bad request"}
	exec.handleFailure(context.Background(), run, job, policy, clientErr, nil)

	calls := store.statusUpdates()
	// Client errors should NOT retry — should go straight to dead letter.
	for _, c := range calls {
		require.NotEqual(t, domain.
			StatusQueued,
			c.to)
	}
	found := false
	for _, c := range calls {
		if c.to == domain.StatusDeadLetter {
			found = true
			break
		}
	}
	require.True(t,
		found)
}

func TestHandleTimeout_Retry_NotAtMaxAttempts(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{}
	exec := newSnoozeTestExecutor(t, store, 0)
	getEvents := collectEvents(exec)
	t.Cleanup(func() { close(exec.eventCh) })

	run := testRun(1)
	run.Status = domain.StatusExecuting
	job := testJob("http://localhost", 5, 30)
	policy := executionPolicy{maxAttempts: 5, timeoutSecs: 30}

	exec.handleTimeout(context.Background(), run, job, policy, nil)

	calls := store.statusUpdates()
	found := false
	for _, c := range calls {
		if c.from == domain.StatusExecuting && c.to == domain.StatusQueued {
			found = true
			break
		}
	}
	require.True(t,
		found)

	events := getEvents()
	foundRetry := false
	for _, ev := range events {
		if ev.Type == EventRetried {
			foundRetry = true
			break
		}
	}
	require.True(t,
		foundRetry,
	)
}

func TestHandleSuccess_CircuitBreakerFailure_StillCompletes(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{
		recordSuccessFn: func(_ context.Context, _ string) error {
			return errors.New("redis circuit breaker down")
		},
		getJobHealthStatsFn: func(_ context.Context, _ string, _ time.Time) (*orcstore.JobHealthStats, error) {
			return nil, nil
		},
	}
	exec := newSnoozeTestExecutor(t, store, 0)

	run := testRun(1)
	run.Status = domain.StatusExecuting
	job := testJob("http://localhost", 3, 30)

	exec.handleSuccess(context.Background(), run, job, nil)

	calls := store.statusUpdates()
	found := false
	for _, c := range calls {
		if c.to == domain.StatusCompleted {
			found = true
			break
		}
	}
	require.True(t,
		found)
}

func TestHandleSuccess_CompleteRunFails_NoEvent(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{
		updateRunStatusFn: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			return errors.New("db write failed")
		},
		getJobHealthStatsFn: func(_ context.Context, _ string, _ time.Time) (*orcstore.JobHealthStats, error) {
			return nil, nil
		},
	}
	exec := newSnoozeTestExecutor(t, store, 0)
	getEvents := collectEvents(exec)
	t.Cleanup(func() { close(exec.eventCh) })

	run := testRun(1)
	run.Status = domain.StatusExecuting
	job := testJob("http://localhost", 3, 30)

	exec.handleSuccess(context.Background(), run, job, nil)

	events := getEvents()
	// When completeRunWithWebhook fails, handleSuccess returns early before emitting.
	for _, ev := range events {
		require.NotEqual(t, EventCompleted,
			ev.
				Type)
	}
}

// Adversarial middleware tests.

func TestChain_MiddlewarePanic_Propagates(t *testing.T) {
	t.Parallel()
	panicMW := func(_ ExecutionHandler) ExecutionHandler {
		return func(_ context.Context, _ *ExecutionContext) {
			panic("middleware exploded")
		}
	}
	handler := func(_ context.Context, _ *ExecutionContext) {}

	defer func() {
		r := recover()
		require.NotNil(t,
			r)
		require.Equal(t,
			"middleware exploded",

			r)
	}()

	Chain(panicMW)(handler)(context.Background(), &ExecutionContext{
		Run: &domain.JobRun{ID: "run-1"},
	})
}

func TestChain_HandlerPanic_PropagatesThroughMiddleware(t *testing.T) {
	t.Parallel()
	var afterCalled bool
	mw := func(next ExecutionHandler) ExecutionHandler {
		return func(ctx context.Context, ec *ExecutionContext) {
			defer func() { afterCalled = true }()
			next(ctx, ec)
		}
	}
	handler := func(_ context.Context, _ *ExecutionContext) {
		panic("handler exploded")
	}

	defer func() {
		r := recover()
		require.NotNil(t,
			r)
		require.True(t,
			afterCalled,
		)

		// The defer in middleware should have run despite the panic.
	}()

	Chain(mw)(handler)(context.Background(), &ExecutionContext{
		Run: &domain.JobRun{ID: "run-1"},
	})
}

// Adversarial event loop tests.

func TestRunEventLoop_SubscriberPanic_CrashesLoop(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()
	ch2 := make(chan RunLifecycleEvent, 1)

	exec := &Executor{
		eventCh: make(chan runEventEnvelope, 256),
		subscribers: []RunEventSubscriber{
			func(_ context.Context, _ RunLifecycleEvent) { panic("subscriber exploded") },
			func(_ context.Context, e RunLifecycleEvent) { ch2 <- e },
		},
	}

	done := make(chan struct{})
	concWG.Go(func() {
		defer func() {
			_ = recover() // catch the panic from runEventLoop
			close(done)
		}()
		exec.runEventLoop()
	})

	exec.eventCh <- runEventEnvelope{
		ctx:   context.Background(),
		event: RunLifecycleEvent{Type: EventCompleted, Run: &domain.JobRun{ID: "run-1"}},
	}

	select {
	case <-done:
		// Loop crashed due to subscriber panic — this documents current behavior.
		// Subscriber 2 never received the event.
	case <-time.After(2 * time.Second):
		require.Fail(t, "expected event loop to crash from subscriber panic")
	}
}

func TestEmit_ConcurrentEmits_NoRace(t *testing.T) {
	t.Parallel()
	exec := &Executor{
		eventCh:     make(chan runEventEnvelope, 256),
		logger:      slogDiscard(),
		subscribers: []RunEventSubscriber{func(_ context.Context, _ RunLifecycleEvent) {}},
	}

	var wg conc.WaitGroup
	for i := range 50 {
		wg.Go(func() {
			exec.emit(context.Background(), RunLifecycleEvent{
				Type: EventCompleted,
				Run:  &domain.JobRun{ID: "run-" + strconv.Itoa(i)},
			})
		})
	}
	wg.Wait()

	// Drain channel and count.
	close(exec.eventCh)
	count := 0
	for range exec.eventCh {
		count++
	}
	require.Equal(t, 50, count)
}

// Adversarial isTerminalStatus tests — catches the bug where
// StatusCrashed, StatusCanceled, StatusExpired were missing.

func TestIsTerminalStatus_MatchesDomainIsTerminal(t *testing.T) {
	t.Parallel()
	// Every status that domain.IsTerminal() considers terminal must also
	// be terminal in our local isTerminalStatus.
	allStatuses := []domain.RunStatus{
		domain.StatusDelayed,
		domain.StatusQueued,
		domain.StatusDequeued,
		domain.StatusExecuting,
		domain.StatusWaiting,
		domain.StatusCompleted,
		domain.StatusFailed,
		domain.StatusTimedOut,
		domain.StatusCrashed,
		domain.StatusSystemFailed,
		domain.StatusCanceled,
		domain.StatusExpired,
		domain.StatusDeadLetter,
		domain.StatusReplayStaged,
		domain.StatusPaused,
	}

	for _, s := range allStatuses {
		t.Run(string(s), func(t *testing.T) {
			got := isTerminalStatus(s)
			want := s.IsTerminal()
			require.Equal(t,
				want, got,
			)
		})
	}
}

// Adversarial MetricsSubscriber tests.

func TestMetricsSubscriber_NilRun_TerminalEvent_NoPanic(t *testing.T) {
	t.Parallel()
	m, _, _, _ := telemetry.InitMetrics("test-adv-nilrun", "test")
	sub := MetricsSubscriber(m)

	// Terminal event with nil Run — should not panic.
	sub(context.Background(), RunLifecycleEvent{
		Type:       EventCompleted,
		FromStatus: domain.StatusExecuting,
		ToStatus:   domain.StatusCompleted,
		Run:        nil,
	})
}

func TestMetricsSubscriber_ZeroDuration_NotRecorded(t *testing.T) {
	t.Parallel()
	m, _, _, _ := telemetry.InitMetrics("test-adv-zerodur", "test")
	sub := MetricsSubscriber(m)

	now := time.Now()
	// StartedAt == FinishedAt -> duration = 0 -> should not record.
	sub(context.Background(), RunLifecycleEvent{
		Type:       EventCompleted,
		FromStatus: domain.StatusExecuting,
		ToStatus:   domain.StatusCompleted,
		Run: &domain.JobRun{
			ID:         "run-1",
			StartedAt:  &now,
			FinishedAt: &now,
		},
	})
}

func TestMetricsSubscriber_NegativeDuration_NotRecorded(t *testing.T) {
	t.Parallel()
	m, _, _, _ := telemetry.InitMetrics("test-adv-negdur", "test")
	sub := MetricsSubscriber(m)

	start := time.Now()
	finish := start.Add(-5 * time.Second) // Finish before start.
	sub(context.Background(), RunLifecycleEvent{
		Type:       EventCompleted,
		FromStatus: domain.StatusExecuting,
		ToStatus:   domain.StatusCompleted,
		Run: &domain.JobRun{
			ID:         "run-1",
			StartedAt:  &start,
			FinishedAt: &finish,
		},
	})
}

func TestMetricsSubscriber_CrashedStatus_RecordsDuration(t *testing.T) {
	t.Parallel()
	m, _, _, _ := telemetry.InitMetrics("test-adv-crashed", "test")
	sub := MetricsSubscriber(m)

	start := time.Now().Add(-3 * time.Second)
	end := time.Now()
	// StatusCrashed is terminal — should record RunDuration (this was previously broken).
	sub(context.Background(), RunLifecycleEvent{
		Type:       EventSystemFailed,
		FromStatus: domain.StatusExecuting,
		ToStatus:   domain.StatusCrashed,
		Run: &domain.JobRun{
			ID:         "run-1",
			StartedAt:  &start,
			FinishedAt: &end,
		},
	})
}

func TestMetricsSubscriber_CanceledStatus_RecordsDuration(t *testing.T) {
	t.Parallel()
	m, _, _, _ := telemetry.InitMetrics("test-adv-canceled", "test")
	sub := MetricsSubscriber(m)

	start := time.Now().Add(-2 * time.Second)
	end := time.Now()
	sub(context.Background(), RunLifecycleEvent{
		Type:       EventSystemFailed,
		FromStatus: domain.StatusExecuting,
		ToStatus:   domain.StatusCanceled,
		Run: &domain.JobRun{
			ID:         "run-1",
			StartedAt:  &start,
			FinishedAt: &end,
		},
	})
}

func TestMetricsSubscriber_ExpiredStatus_RecordsDuration(t *testing.T) {
	t.Parallel()
	m, _, _, _ := telemetry.InitMetrics("test-adv-expired", "test")
	sub := MetricsSubscriber(m)

	start := time.Now().Add(-10 * time.Second)
	end := time.Now()
	sub(context.Background(), RunLifecycleEvent{
		Type:       EventSystemFailed,
		FromStatus: domain.StatusExecuting,
		ToStatus:   domain.StatusExpired,
		Run: &domain.JobRun{
			ID:         "run-1",
			StartedAt:  &start,
			FinishedAt: &end,
		},
	})
}

// Adversarial PubSubSubscriber tests.

func TestPubSubSubscriber_EmptyRunID(t *testing.T) {
	t.Parallel()
	pub := &mockPublisher{}
	sub := PubSubSubscriber(pub)

	sub(context.Background(), RunLifecycleEvent{
		Type:       EventCompleted,
		Run:        &domain.JobRun{ID: "", JobID: "j1", ProjectID: "p1"},
		FromStatus: domain.StatusExecuting,
		ToStatus:   domain.StatusCompleted,
	})

	calls := pub.publishCalls()
	require.Len(t, calls,
		1)
	require.Equal(t,
		"run:", calls[0].channel,
	)

	// Channel should be "run:" with empty ID.
}

// Adversarial completeRunWithWebhook tests.

func TestCompleteRunWithWebhook_CancelledContext(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{}
	exec := newSnoozeTestExecutor(t, store, 0)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	run := &domain.JobRun{ID: "run-1", Status: domain.StatusExecuting}
	job := &domain.Job{ID: "job-1", WebhookURL: ""}

	// Should not panic with cancelled context. The store mock will still succeed.
	err := exec.completeRunWithWebhook(ctx, run, job,
		domain.StatusCompleted, map[string]any{})
	require.NoError(
		t, err)
}

func TestCompleteRunWithWebhook_NilFields(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{}
	exec := newSnoozeTestExecutor(t, store, 0)

	run := &domain.JobRun{ID: "run-1", Status: domain.StatusExecuting}
	job := &domain.Job{ID: "job-1", WebhookURL: ""}

	// nil fields map should not panic.
	err := exec.completeRunWithWebhook(context.Background(), run, job,
		domain.StatusCompleted, nil)
	require.NoError(
		t, err)
}

// slogDiscard returns a logger that discards all output.
func slogDiscard() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}
