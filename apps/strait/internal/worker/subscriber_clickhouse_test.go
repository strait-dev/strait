package worker

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"strait/internal/clickhouse"
	"strait/internal/domain"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockEventLister implements EventLister for testing.
type mockEventLister struct {
	events []domain.RunEvent
	err    error
	called bool
}

func (m *mockEventLister) ListEvents(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.RunEvent, error) {
	m.called = true
	return m.events, m.err
}

func TestClickHouseSubscriber_NilExporter(t *testing.T) {
	t.Parallel()
	sub := ClickHouseSubscriber(nil, nil)
	// Should not panic.
	sub(context.Background(), RunLifecycleEvent{
		Type: EventCompleted,
		Run:  &domain.JobRun{ID: "run-1"},
	})
}

func TestClickHouseSubscriber_NonTerminalEvent(t *testing.T) {
	t.Parallel()
	exporter := clickhouse.NewExporter(&clickhouse.Client{}, clickhouse.ExporterConfig{
		Enabled:   true,
		BatchSize: 100,
	}, slog.Default())

	sub := ClickHouseSubscriber(exporter, nil)
	sub(context.Background(), RunLifecycleEvent{
		Type: EventSnoozed,
		Run:  &domain.JobRun{ID: "run-1"},
	})
	assert.Equal(t, 0, exporter.PendingCount())
}

func TestClickHouseSubscriber_NilRun(t *testing.T) {
	t.Parallel()
	exporter := clickhouse.NewExporter(&clickhouse.Client{}, clickhouse.ExporterConfig{
		Enabled:   true,
		BatchSize: 100,
	}, slog.Default())

	sub := ClickHouseSubscriber(exporter, nil)
	sub(context.Background(), RunLifecycleEvent{
		Type: EventCompleted,
		Run:  nil,
	})
	assert.Equal(t, 0, exporter.PendingCount())
}

func TestClickHouseSubscriber_TerminalEvent_EnqueuesRecord(t *testing.T) {
	t.Parallel()
	exporter := clickhouse.NewExporter(&clickhouse.Client{}, clickhouse.ExporterConfig{
		Enabled:   true,
		BatchSize: 100,
	}, slog.Default())

	now := time.Now()
	started := now.Add(-5 * time.Second)

	sub := ClickHouseSubscriber(exporter, nil)
	sub(context.Background(), RunLifecycleEvent{
		Type: EventCompleted,
		Run: &domain.JobRun{
			ID:           "run-1",
			JobID:        "job-1",
			ProjectID:    "proj-1",
			Status:       domain.StatusCompleted,
			Attempt:      1,
			StartedAt:    &started,
			FinishedAt:   &now,
			TriggeredBy:  "manual",
			Tags:         map[string]string{"env": "prod", "team": "backend"},
			JobVersionID: "ver-abc",
		},
		Job: &domain.Job{
			ExecutionMode: "http",
		},
		QueueWait: 200 * time.Millisecond,
	})
	assert.Equal(t, 1, exporter.PendingCount())
}

func TestClickHouseSubscriber_TagsAndVersionPopulated(t *testing.T) {
	t.Parallel()
	exporter := clickhouse.NewExporter(&clickhouse.Client{}, clickhouse.ExporterConfig{
		Enabled:   true,
		BatchSize: 100,
	}, slog.Default())

	now := time.Now()
	started := now.Add(-2 * time.Second)

	sub := ClickHouseSubscriber(exporter, nil)

	// With tags and version
	sub(context.Background(), RunLifecycleEvent{
		Type: EventCompleted,
		Run: &domain.JobRun{
			ID:           "run-tags",
			JobID:        "job-1",
			ProjectID:    "proj-1",
			Status:       domain.StatusCompleted,
			StartedAt:    &started,
			FinishedAt:   &now,
			Tags:         map[string]string{"env": "staging"},
			JobVersionID: "ver-123",
		},
	})
	assert.Equal(t, 1, exporter.PendingCount())
}

func TestClickHouseSubscriber_EmptyTags(t *testing.T) {
	t.Parallel()
	exporter := clickhouse.NewExporter(&clickhouse.Client{}, clickhouse.ExporterConfig{
		Enabled:   true,
		BatchSize: 100,
	}, slog.Default())

	sub := ClickHouseSubscriber(exporter, nil)
	sub(context.Background(), RunLifecycleEvent{
		Type: EventCompleted,
		Run: &domain.JobRun{
			ID:        "run-no-tags",
			JobID:     "job-1",
			ProjectID: "proj-1",
			Status:    domain.StatusCompleted,
		},
	})
	assert.Equal(t, 1, exporter.PendingCount())
}

func TestClickHouseSubscriber_AllTerminalTypes(t *testing.T) {
	t.Parallel()
	terminalTypes := []RunEventType{EventCompleted, EventTimedOut, EventDeadLettered, EventSystemFailed}

	for _, et := range terminalTypes {
		t.Run(string(et), func(t *testing.T) {
			t.Parallel()
			exporter := clickhouse.NewExporter(&clickhouse.Client{}, clickhouse.ExporterConfig{
				Enabled:   true,
				BatchSize: 100,
			}, slog.Default())

			sub := ClickHouseSubscriber(exporter, nil)
			sub(context.Background(), RunLifecycleEvent{
				Type: et,
				Run:  &domain.JobRun{ID: "run-1", ProjectID: "proj-1"},
			})
			assert.Equal(t, 1, exporter.PendingCount())
		})
	}
}

func TestClickHouseSubscriber_EnqueuesRunEvents(t *testing.T) {
	t.Parallel()
	exporter := clickhouse.NewExporter(&clickhouse.Client{}, clickhouse.ExporterConfig{
		Enabled:   true,
		BatchSize: 100,
	}, slog.Default())

	now := time.Now()
	lister := &mockEventLister{
		events: []domain.RunEvent{
			{ID: "evt-1", RunID: "run-1", Type: "log", Level: "info", Message: "hello", CreatedAt: now},
			{ID: "evt-2", RunID: "run-1", Type: "log", Level: "error", Message: "boom", CreatedAt: now},
		},
	}

	sub := ClickHouseSubscriber(exporter, lister)
	sub(context.Background(), RunLifecycleEvent{
		Type: EventCompleted,
		Run:  &domain.JobRun{ID: "run-1", ProjectID: "proj-1", JobID: "job-1"},
	})

	// Wait for the background goroutine to finish.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		// 1 analytics record + 2 event records = 3
		if exporter.PendingCount() >= 3 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	assert.Equal(t, 3, exporter.PendingCount())
}

func TestClickHouseSubscriber_NilEventLister(t *testing.T) {
	t.Parallel()
	exporter := clickhouse.NewExporter(&clickhouse.Client{}, clickhouse.ExporterConfig{
		Enabled:   true,
		BatchSize: 100,
	}, slog.Default())

	handle := NewClickHouseSubscriberHandle(exporter, nil)
	handle.Subscriber()(context.Background(), RunLifecycleEvent{
		Type: EventCompleted,
		Run:  &domain.JobRun{ID: "run-1", ProjectID: "proj-1"},
	})
	handle.Wait()
	assert.Equal(t, 1, exporter.PendingCount())
}

func TestClickHouseSubscriber_EventListError(t *testing.T) {
	t.Parallel()
	exporter := clickhouse.NewExporter(&clickhouse.Client{}, clickhouse.ExporterConfig{
		Enabled:   true,
		BatchSize: 100,
	}, slog.Default())

	lister := &mockEventLister{
		err: errors.New("db error"),
	}

	handle := NewClickHouseSubscriberHandle(exporter, lister)
	handle.Subscriber()(context.Background(), RunLifecycleEvent{
		Type: EventCompleted,
		Run:  &domain.JobRun{ID: "run-1", ProjectID: "proj-1"},
	})
	handle.Wait()
	assert.Equal(t, 1, exporter.PendingCount())
}

func TestRunEventsFromDomain(t *testing.T) {
	t.Parallel()
	now := time.Now()
	run := &domain.JobRun{
		ID:        "run-1",
		JobID:     "job-1",
		ProjectID: "proj-1",
	}
	events := []domain.RunEvent{
		{
			ID:        "evt-1",
			RunID:     "run-1",
			Type:      "log",
			Level:     "info",
			Message:   "hello",
			Data:      json.RawMessage(`{"key":"val"}`),
			CreatedAt: now,
		},
		{
			ID:        "evt-2",
			RunID:     "run-1",
			Type:      "error",
			Level:     "error",
			Message:   "boom",
			Data:      nil,
			CreatedAt: now,
		},
	}

	records := runEventsFromDomain(run, events)
	require.Len(t, records,
		2)

	r := records[0]
	assert.False(t,
		r.EventID != "evt-1" ||
			r.RunID !=
				"run-1" ||
			r.ProjectID !=
				"proj-1" || r.JobID != "job-1",
	)
	assert.False(t,
		r.EventType != "log" ||
			r.Level !=
				"info" ||
			r.Message !=
				"hello",
	)
	assert.JSONEq(t,
		`{"key":"val"}`,
		r.Metadata,
	)

	r2 := records[1]
	assert.Empty(t,
		r2.Metadata)
}

func TestRunAnalyticsRecordFromLifecycleEvent(t *testing.T) {
	t.Parallel()
	finishedAt := time.Now()
	startedAt := finishedAt.Add(-1500 * time.Millisecond)
	createdAt := finishedAt.Add(time.Second)

	record := runAnalyticsRecordFromLifecycleEvent(RunLifecycleEvent{
		Type: EventCompleted,
		Run: &domain.JobRun{
			ID:           "run-1",
			JobID:        "job-1",
			ProjectID:    "proj-1",
			Status:       domain.StatusCompleted,
			Attempt:      2,
			StartedAt:    &startedAt,
			FinishedAt:   &finishedAt,
			TriggeredBy:  "manual",
			Tags:         map[string]string{"env": "prod"},
			JobVersionID: "version-1",
		},
		Job:       &domain.Job{ExecutionMode: domain.ExecutionModeHTTP},
		QueueWait: 250 * time.Millisecond,
	}, createdAt)
	require.False(t,
		record.RunID !=
			"run-1" ||
			record.JobID !=
				"job-1" ||
			record.
				ProjectID != "proj-1")
	require.False(t,
		record.Status !=
			string(domain.
				StatusCompleted,
			) ||
			record.
				ExecutionMode != string(domain.
				ExecutionModeHTTP))
	require.False(t,
		record.Attempt !=
			2 || record.
			DurationMs !=
			1500 ||
			record.
				QueueWaitMs != 250)
	require.False(t,
		record.TriggeredBy !=
			"manual" ||
			record.
				Tags !=
				`{"env":"prod"}` ||
			record.JobVersionID !=
				"version-1")
	require.False(t,
		!record.CreatedAt.
			Equal(createdAt) ||
			record.StartedAt !=

				&startedAt || record.FinishedAt !=
			&finishedAt)
}

func TestClickHouseSubscriber_SemaphoreWaitsBeforeDropping(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()
	exporter := clickhouse.NewExporter(&clickhouse.Client{}, clickhouse.ExporterConfig{
		Enabled:   true,
		BatchSize: 100,
	}, slog.Default())

	// Use a slow lister that blocks until cancelled to fill the semaphore.
	cancelCtx, cancelAll := context.WithCancel(context.Background())
	defer cancelAll()

	slowLister := &blockingEventLister{
		cancel:  cancelCtx,
		events:  []domain.RunEvent{{ID: "evt-1", RunID: "run-1"}},
		started: make(chan struct{}, maxConcurrentEventFetches),
	}

	handle := NewClickHouseSubscriberHandle(exporter, slowLister)
	sub := handle.Subscriber()

	// Fill the semaphore by launching maxConcurrentEventFetches goroutines.
	for i := range maxConcurrentEventFetches {
		sub(context.Background(), RunLifecycleEvent{
			Type: EventCompleted,
			Run:  &domain.JobRun{ID: "run-fill-" + string(rune('A'+i)), ProjectID: "proj-1"},
		})
	}

	// Wait for all goroutines to enter ListEvents (and block on the cancel context).
	for range maxConcurrentEventFetches {
		select {
		case <-slowLister.started:
		case <-time.After(2 * time.Second):
			require.Fail(t, "not all goroutines entered ListEvents within 2s")
		}
	}

	// Next call should block (waiting for semaphore), not return instantly.
	start := time.Now()
	done := make(chan struct{})
	concWG.Go(func() {
		sub(context.Background(), RunLifecycleEvent{
			Type: EventCompleted,
			Run:  &domain.JobRun{ID: "run-blocked", ProjectID: "proj-1"},
		})
		close(done)
	})

	select {
	case <-done:
		elapsed := time.Since(start)
		if elapsed < 100*time.Millisecond {
			assert.Failf(t, "test failure",

				"subscriber returned too quickly (%v), expected to wait on semaphore", elapsed)
		}
	case <-time.After(10 * time.Second):
		require.Fail(t, "subscriber did not return within expected timeout")
	}

	cancelAll()
	handle.Wait()
}

// blockingEventLister blocks ListEvents until its cancel context is done.
type blockingEventLister struct {
	cancel  context.Context
	events  []domain.RunEvent
	started chan struct{}
}

func (b *blockingEventLister) ListEvents(ctx context.Context, _ string, _ int, _ *time.Time) ([]domain.RunEvent, error) {
	if b.started != nil {
		select {
		case b.started <- struct{}{}:
		default:
		}
	}
	select {
	case <-b.cancel.Done():
		return nil, b.cancel.Err()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestClickHouseSubscriberHandle_WaitDrainsGoroutines(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()
	exporter := clickhouse.NewExporter(&clickhouse.Client{}, clickhouse.ExporterConfig{
		Enabled:   true,
		BatchSize: 100,
	}, slog.Default())

	now := time.Now()
	lister := &mockEventLister{
		events: []domain.RunEvent{
			{ID: "evt-1", RunID: "run-1", Type: "log", Level: "info", Message: "hello", CreatedAt: now},
		},
	}

	handle := NewClickHouseSubscriberHandle(exporter, lister)
	sub := handle.Subscriber()

	sub(context.Background(), RunLifecycleEvent{
		Type: EventCompleted,
		Run:  &domain.JobRun{ID: "run-1", ProjectID: "proj-1", JobID: "job-1"},
	})

	// Wait must return once all background goroutines finish.
	done := make(chan struct{})
	concWG.Go(func() {
		handle.Wait()
		close(done)
	})

	select {
	case <-done:
		// All goroutines drained successfully.
	case <-time.After(5 * time.Second):
		require.Fail(t, "Wait did not return within 5 seconds")
	}
	assert.Equal(t, 2, exporter.PendingCount())

	// Verify the events were enqueued: 1 analytics + 1 event record = 2.
}

func TestClickHouseSubscriberHandle_WaitNoGoroutines(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	handle := NewClickHouseSubscriberHandle(nil, nil)
	// Wait on a handle with no goroutines launched should return immediately.
	done := make(chan struct{})
	concWG.Go(func() {
		handle.Wait()
		close(done)
	})

	select {
	case <-done:
		// Returned immediately.
	case <-time.After(time.Second):
		require.Fail(t, "Wait blocked on handle with no goroutines")
	}
}

func TestIsTerminalEvent(t *testing.T) {
	t.Parallel()
	tests := []struct {
		eventType RunEventType
		want      bool
	}{
		{EventCompleted, true},
		{EventTimedOut, true},
		{EventDeadLettered, true},
		{EventSystemFailed, true},
		{EventSnoozed, false},
		{EventRetried, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.eventType), func(t *testing.T) {
			t.Parallel()
			assert.Equal(t,
				tt.want, isTerminalEvent(tt.
					eventType))
		})
	}
}
