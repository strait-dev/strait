package worker

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/pubsub"
	"strait/internal/telemetry"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/require"
)

// emit() tests.

func TestEmit_NoSubscribers_NoOp(t *testing.T) {
	t.Parallel()
	exec := &Executor{
		eventCh: make(chan runEventEnvelope, 256),
		logger:  slog.Default(),
		// No subscribers.
	}
	// Should return immediately without sending to channel.
	exec.emit(context.Background(), RunLifecycleEvent{
		Type: EventCompleted,
		Run:  &domain.JobRun{ID: "run-1"},
	})

	select {
	case <-exec.eventCh:
		require.Fail(t, "event should not be sent when there are no subscribers")
	default:
		// Good: channel is empty.
	}
}

func TestEmit_NonBlocking_ChannelFull(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	// Small buffer for test speed.
	bufSize := 4
	exec := &Executor{
		eventCh:     make(chan runEventEnvelope, bufSize),
		logger:      slog.Default(),
		subscribers: []RunEventSubscriber{func(_ context.Context, _ RunLifecycleEvent) {}},
	}

	run := &domain.JobRun{ID: "run-1"}
	// Fill the channel.
	for range bufSize {
		exec.eventCh <- runEventEnvelope{ctx: context.Background(), event: RunLifecycleEvent{Run: run}}
	}

	// This emit should not block.
	done := make(chan struct{})
	concWG.Go(func() {
		exec.emit(context.Background(), RunLifecycleEvent{Type: EventCompleted, Run: run})
		close(done)
	})

	select {
	case <-done:
		// Good: non-blocking.
	case <-time.After(time.Second):
		require.Fail(t, "emit blocked on full channel")
	}
}

func TestEmit_ClosedChannel_Recovers(t *testing.T) {
	t.Parallel()
	exec := &Executor{
		eventCh:     make(chan runEventEnvelope, 256),
		logger:      slog.Default(),
		subscribers: []RunEventSubscriber{func(_ context.Context, _ RunLifecycleEvent) {}},
	}
	close(exec.eventCh)

	// Should not panic due to defer/recover in emit.
	exec.emit(context.Background(), RunLifecycleEvent{
		Type: EventCompleted,
		Run:  &domain.JobRun{ID: "run-1"},
	})
}

func TestEmit_DeliversToChannel(t *testing.T) {
	t.Parallel()
	exec := &Executor{
		eventCh:     make(chan runEventEnvelope, 256),
		logger:      slog.Default(),
		subscribers: []RunEventSubscriber{func(_ context.Context, _ RunLifecycleEvent) {}},
	}

	event := RunLifecycleEvent{Type: EventCompleted, Run: &domain.JobRun{ID: "run-42"}}
	exec.emit(context.Background(), event)

	select {
	case env := <-exec.eventCh:
		if env.event.Run.ID != "run-42" {
			require.Failf(t, "test failure",

				"expected run-42, got %s", env.event.Run.ID)
		}
		if env.event.Type != EventCompleted {
			require.Failf(t, "test failure",

				"expected EventCompleted, got %s", env.event.Type)
		}
	default:
		require.Fail(t, "event was not delivered to channel")
	}
}

// runEventLoop tests.

func TestRunEventLoop_FansOutToAll(t *testing.T) {
	t.Parallel()
	ch1 := make(chan RunLifecycleEvent, 1)
	ch2 := make(chan RunLifecycleEvent, 1)
	ch3 := make(chan RunLifecycleEvent, 1)

	exec := &Executor{
		eventCh: make(chan runEventEnvelope, 256),
		subscribers: []RunEventSubscriber{
			func(_ context.Context, e RunLifecycleEvent) { ch1 <- e },
			func(_ context.Context, e RunLifecycleEvent) { ch2 <- e },
			func(_ context.Context, e RunLifecycleEvent) { ch3 <- e },
		},
	}

	go exec.runEventLoop()

	event := RunLifecycleEvent{Type: EventCompleted, Run: &domain.JobRun{ID: "run-1"}}
	exec.eventCh <- runEventEnvelope{ctx: context.Background(), event: event}

	for i, ch := range []chan RunLifecycleEvent{ch1, ch2, ch3} {
		select {
		case got := <-ch:
			if got.Run.ID != "run-1" {
				require.Failf(t, "test failure",

					"subscriber %d: expected run-1, got %s", i, got.Run.ID)
			}
		case <-time.After(time.Second):
			require.Failf(t, "test failure", "subscriber %d did not receive event", i)
		}
	}

	close(exec.eventCh)
}

func TestRunEventLoop_ExitsOnClose(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()
	exec := &Executor{
		eventCh:     make(chan runEventEnvelope, 256),
		subscribers: []RunEventSubscriber{func(_ context.Context, _ RunLifecycleEvent) {}},
	}

	done := make(chan struct{})
	concWG.Go(func() {
		exec.runEventLoop()
		close(done)
	})

	close(exec.eventCh)

	select {
	case <-done:
		// Good: loop exited.
	case <-time.After(time.Second):
		require.Fail(t, "runEventLoop did not exit after channel close")
	}
}

// MetricsSubscriber tests.

func TestMetricsSubscriber_RunTransitions(t *testing.T) {
	t.Parallel()
	m, _, _, _ := telemetry.InitMetrics("test-sub", "test")
	sub := MetricsSubscriber(m)

	// Should not panic.
	sub(context.Background(), RunLifecycleEvent{
		Type:       EventCompleted,
		FromStatus: domain.StatusExecuting,
		ToStatus:   domain.StatusCompleted,
		Run:        &domain.JobRun{ID: "run-1"},
	})
}

func TestMetricsSubscriber_SnoozeEvent_IncrementsSnoozeTotal(t *testing.T) {
	t.Parallel()
	m, _, _, _ := telemetry.InitMetrics("test-sub-snooze", "test")
	sub := MetricsSubscriber(m)

	// Should not panic and should increment snooze counter.
	sub(context.Background(), RunLifecycleEvent{
		Type:       EventSnoozed,
		FromStatus: domain.StatusDequeued,
		ToStatus:   domain.StatusQueued,
		Run:        &domain.JobRun{ID: "run-1"},
	})
}

func TestMetricsSubscriber_NonSnooze_NoSnoozeTotal(t *testing.T) {
	t.Parallel()
	m, _, _, _ := telemetry.InitMetrics("test-sub-nonsnooze", "test")
	sub := MetricsSubscriber(m)

	// EventCompleted should not touch snooze counter. Should not panic.
	sub(context.Background(), RunLifecycleEvent{
		Type:       EventCompleted,
		FromStatus: domain.StatusExecuting,
		ToStatus:   domain.StatusCompleted,
		Run:        &domain.JobRun{ID: "run-1"},
	})
}

func TestMetricsSubscriber_ExecTrace_RecordsHistograms(t *testing.T) {
	t.Parallel()
	m, _, _, _ := telemetry.InitMetrics("test-sub-trace", "test")
	sub := MetricsSubscriber(m)

	sub(context.Background(), RunLifecycleEvent{
		Type:       EventCompleted,
		FromStatus: domain.StatusExecuting,
		ToStatus:   domain.StatusCompleted,
		Run:        &domain.JobRun{ID: "run-1"},
		ExecTrace:  &domain.ExecutionTrace{DispatchMs: 42, QueueWaitMs: 100},
	})
}

func TestMetricsSubscriber_NilExecTrace_Skipped(t *testing.T) {
	t.Parallel()
	m, _, _, _ := telemetry.InitMetrics("test-sub-niltrace", "test")
	sub := MetricsSubscriber(m)

	// Should not panic with nil ExecTrace.
	sub(context.Background(), RunLifecycleEvent{
		Type:       EventCompleted,
		FromStatus: domain.StatusExecuting,
		ToStatus:   domain.StatusCompleted,
		Run:        &domain.JobRun{ID: "run-1"},
		ExecTrace:  nil,
	})
}

func TestMetricsSubscriber_TerminalEvent_RecordsRunDuration(t *testing.T) {
	t.Parallel()
	m, _, _, _ := telemetry.InitMetrics("test-sub-duration", "test")
	sub := MetricsSubscriber(m)

	start := time.Now().Add(-5 * time.Second)
	end := time.Now()
	sub(context.Background(), RunLifecycleEvent{
		Type:       EventCompleted,
		FromStatus: domain.StatusExecuting,
		ToStatus:   domain.StatusCompleted,
		Run: &domain.JobRun{
			ID:         "run-1",
			StartedAt:  &start,
			FinishedAt: &end,
		},
	})
}

func TestMetricsSubscriber_NonTerminal_NoRunDuration(t *testing.T) {
	t.Parallel()
	m, _, _, _ := telemetry.InitMetrics("test-sub-nonterm", "test")
	sub := MetricsSubscriber(m)

	start := time.Now().Add(-5 * time.Second)
	end := time.Now()
	// EventRetried is non-terminal — should not record RunDuration.
	sub(context.Background(), RunLifecycleEvent{
		Type:       EventRetried,
		FromStatus: domain.StatusExecuting,
		ToStatus:   domain.StatusQueued,
		Run: &domain.JobRun{
			ID:         "run-1",
			StartedAt:  &start,
			FinishedAt: &end,
		},
	})
}

func TestMetricsSubscriber_Terminal_NilStartedAt_NoRunDuration(t *testing.T) {
	t.Parallel()
	m, _, _, _ := telemetry.InitMetrics("test-sub-nilstart", "test")
	sub := MetricsSubscriber(m)

	end := time.Now()
	// Terminal but StartedAt is nil — should not panic or record duration.
	sub(context.Background(), RunLifecycleEvent{
		Type:       EventCompleted,
		FromStatus: domain.StatusExecuting,
		ToStatus:   domain.StatusCompleted,
		Run: &domain.JobRun{
			ID:         "run-1",
			StartedAt:  nil,
			FinishedAt: &end,
		},
	})
}

// PubSubSubscriber tests.

type mockPublisher struct {
	mu         sync.Mutex
	publishFn  func(ctx context.Context, channel string, data []byte) error
	publishErr error
	calls      []publishCall
}

type publishCall struct {
	channel string
	data    []byte
}

func (m *mockPublisher) Publish(ctx context.Context, channel string, data []byte) error {
	m.mu.Lock()
	m.calls = append(m.calls, publishCall{channel: channel, data: data})
	m.mu.Unlock()
	if m.publishFn != nil {
		return m.publishFn(ctx, channel, data)
	}
	return m.publishErr
}
func (m *mockPublisher) PublishBatch(_ context.Context, _ []pubsub.PubSubMessage) error {
	return nil
}
func (m *mockPublisher) Subscribe(_ context.Context, _ string) (*pubsub.Subscription, error) {
	return nil, nil
}
func (m *mockPublisher) Close() error { return nil }

func (m *mockPublisher) publishCalls() []publishCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]publishCall, len(m.calls))
	copy(out, m.calls)
	return out
}

func TestPubSubSubscriber_PublishesStatusChange(t *testing.T) {
	t.Parallel()
	pub := &mockPublisher{}
	sub := PubSubSubscriber(pub)

	sub(context.Background(), RunLifecycleEvent{
		Type:       EventCompleted,
		Run:        &domain.JobRun{ID: "run-42", JobID: "job-7", ProjectID: "proj-3"},
		FromStatus: domain.StatusExecuting,
		ToStatus:   domain.StatusCompleted,
	})

	calls := pub.publishCalls()
	require.Len(t, calls,
		1)
	require.Equal(t,
		"run:run-42", calls[0].channel,
	)

	var payload map[string]any
	require.NoError(
		t, json.Unmarshal(calls[0].
			data, &payload,
		))
	require.Equal(t,
		"status_change",
		payload["type"])
	require.Equal(t,
		"run-42", payload["run_id"])
	require.Equal(t,
		"job-7", payload["job_id"],
	)
	require.Equal(t,
		"proj-3", payload["project_id"])
	require.Equal(t,
		string(domain.StatusExecuting), payload["from"])
	require.Equal(t,
		string(domain.StatusCompleted), payload["to"])

	if _, ok := payload["timestamp"]; !ok {
		require.Fail(t,

			"expected timestamp in payload")
	}
}

func TestPubSubSubscriber_NilRun_NoPublish(t *testing.T) {
	t.Parallel()
	pub := &mockPublisher{}
	sub := PubSubSubscriber(pub)

	sub(context.Background(), RunLifecycleEvent{
		Type: EventCompleted,
		Run:  nil,
	})

	calls := pub.publishCalls()
	require.Empty(t, calls)
}

func TestPubSubSubscriber_PublishError_NoPanic(t *testing.T) {
	t.Parallel()
	pub := &mockPublisher{
		publishErr: errors.New("redis down"),
	}
	sub := PubSubSubscriber(pub)

	// Should not panic.
	sub(context.Background(), RunLifecycleEvent{
		Type:       EventCompleted,
		Run:        &domain.JobRun{ID: "run-1", JobID: "job-1", ProjectID: "proj-1"},
		FromStatus: domain.StatusExecuting,
		ToStatus:   domain.StatusCompleted,
	})
}

func TestPubSubSubscriber_ErrorCounter_Incremented(t *testing.T) {
	t.Parallel()
	pub := &mockPublisher{
		publishErr: errors.New("redis down"),
	}
	// Use the real telemetry metrics to get a real counter.
	m, _, _, _ := telemetry.InitMetrics("test-pubsub-errctr", "test")
	sub := PubSubSubscriber(pub, m.DispatchErrors) // Reuse an existing counter for test.

	sub(context.Background(), RunLifecycleEvent{
		Type:       EventCompleted,
		Run:        &domain.JobRun{ID: "run-1", JobID: "job-1", ProjectID: "proj-1"},
		FromStatus: domain.StatusExecuting,
		ToStatus:   domain.StatusCompleted,
	})

	// Counter was called — we can't easily read the value from OTel counters,
	// but the test verifies no panic when errCounter is provided.
}

func TestPubSubSubscriber_NoErrorCounter_NoPanic(t *testing.T) {
	t.Parallel()
	pub := &mockPublisher{
		publishErr: errors.New("redis down"),
	}
	// No error counter passed (empty variadic).
	sub := PubSubSubscriber(pub)

	// Should not panic.
	sub(context.Background(), RunLifecycleEvent{
		Type:       EventCompleted,
		Run:        &domain.JobRun{ID: "run-1", JobID: "job-1", ProjectID: "proj-1"},
		FromStatus: domain.StatusExecuting,
		ToStatus:   domain.StatusCompleted,
	})
}

func TestPubSubSubscriber_ChannelFormat(t *testing.T) {
	t.Parallel()
	pub := &mockPublisher{}
	sub := PubSubSubscriber(pub)

	sub(context.Background(), RunLifecycleEvent{
		Type:       EventCompleted,
		Run:        &domain.JobRun{ID: "abc-123-def", JobID: "j1", ProjectID: "p1"},
		FromStatus: domain.StatusExecuting,
		ToStatus:   domain.StatusCompleted,
	})

	calls := pub.publishCalls()
	require.Equal(t,
		"run:abc-123-def",
		calls[0].channel)
}

// isTerminalStatus tests.

func TestIsTerminalStatus_AllCases(t *testing.T) {
	t.Parallel()
	cases := []struct {
		status   domain.RunStatus
		terminal bool
	}{
		{domain.StatusCompleted, true},
		{domain.StatusFailed, true},
		{domain.StatusTimedOut, true},
		{domain.StatusCrashed, true},
		{domain.StatusSystemFailed, true},
		{domain.StatusCanceled, true},
		{domain.StatusExpired, true},
		{domain.StatusQueued, false},
		{domain.StatusDequeued, false},
		{domain.StatusExecuting, false},
		{domain.StatusDeadLetter, true},
		{domain.StatusDelayed, false},
		{domain.StatusWaiting, false},
		{domain.StatusPaused, false},
	}
	for _, tc := range cases {
		t.Run(string(tc.status), func(t *testing.T) {
			got := isTerminalStatus(tc.status)
			require.Equal(t,
				tc.terminal, got,
			)
		})
	}
}

// TestEmit_ChannelFull_DropCounterNoDeadlock verifies that when the event
// channel is saturated, emit returns promptly (no deadlock) and the drop
// counter hook is exercised without panicking.
func TestEmit_ChannelFull_DropCounterNoDeadlock(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()
	exec := &Executor{
		eventCh:     make(chan runEventEnvelope, 1),
		logger:      slog.Default(),
		subscribers: []RunEventSubscriber{func(_ context.Context, _ RunLifecycleEvent) {}},
	}
	run := &domain.JobRun{ID: "run-drop"}

	// Fill the single slot.
	exec.emit(context.Background(), RunLifecycleEvent{Type: EventCompleted, Run: run})

	// Further emits must not block and must not panic even with no OTEL
	// meter provider installed (queue metrics noop to a noop meter).
	done := make(chan struct{})
	concWG.Go(func() {
		for range 32 {
			exec.emit(context.Background(), RunLifecycleEvent{Type: EventCompleted, Run: run})
		}
		close(done)
	})
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		require.Fail(t, "emit blocked on full channel")
	}
}
