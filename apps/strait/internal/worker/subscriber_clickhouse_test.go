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

	if exporter.PendingCount() != 0 {
		t.Errorf("expected 0 pending for non-terminal event, got %d", exporter.PendingCount())
	}
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

	if exporter.PendingCount() != 0 {
		t.Errorf("expected 0 pending for nil run, got %d", exporter.PendingCount())
	}
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
			ID:          "run-1",
			JobID:       "job-1",
			ProjectID:   "proj-1",
			Status:      domain.StatusCompleted,
			Attempt:     1,
			StartedAt:   &started,
			FinishedAt:  &now,
			TriggeredBy: "manual",
		},
		Job: &domain.Job{
			ExecutionMode: "http",
			MachinePreset: "standard",
		},
		QueueWait: 200 * time.Millisecond,
	})

	if exporter.PendingCount() != 1 {
		t.Errorf("expected 1 pending for terminal event, got %d", exporter.PendingCount())
	}
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

			if exporter.PendingCount() != 1 {
				t.Errorf("expected 1 pending for %s, got %d", et, exporter.PendingCount())
			}
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

	if exporter.PendingCount() != 3 {
		t.Errorf("expected 3 pending (1 analytics + 2 events), got %d", exporter.PendingCount())
	}
}

func TestClickHouseSubscriber_NilEventLister(t *testing.T) {
	t.Parallel()
	exporter := clickhouse.NewExporter(&clickhouse.Client{}, clickhouse.ExporterConfig{
		Enabled:   true,
		BatchSize: 100,
	}, slog.Default())

	sub := ClickHouseSubscriber(exporter, nil)
	sub(context.Background(), RunLifecycleEvent{
		Type: EventCompleted,
		Run:  &domain.JobRun{ID: "run-1", ProjectID: "proj-1"},
	})

	// Only the analytics record should be enqueued (no event lister).
	time.Sleep(50 * time.Millisecond)
	if exporter.PendingCount() != 1 {
		t.Errorf("expected 1 pending (analytics only), got %d", exporter.PendingCount())
	}
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

	sub := ClickHouseSubscriber(exporter, lister)
	sub(context.Background(), RunLifecycleEvent{
		Type: EventCompleted,
		Run:  &domain.JobRun{ID: "run-1", ProjectID: "proj-1"},
	})

	// Wait for goroutine to finish.
	time.Sleep(50 * time.Millisecond)

	// Only the analytics record should be present; event fetch failed.
	if exporter.PendingCount() != 1 {
		t.Errorf("expected 1 pending (analytics only, event fetch failed), got %d", exporter.PendingCount())
	}
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

	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}

	r := records[0]
	if r.EventID != "evt-1" || r.RunID != "run-1" || r.ProjectID != "proj-1" || r.JobID != "job-1" {
		t.Errorf("record 0 IDs mismatch: %+v", r)
	}
	if r.EventType != "log" || r.Level != "info" || r.Message != "hello" {
		t.Errorf("record 0 fields mismatch: %+v", r)
	}
	if r.Metadata != `{"key":"val"}` {
		t.Errorf("record 0 metadata = %q, want %q", r.Metadata, `{"key":"val"}`)
	}

	r2 := records[1]
	if r2.Metadata != "" {
		t.Errorf("record 1 metadata = %q, want empty", r2.Metadata)
	}
}

func TestClickHouseSubscriber_SemaphoreWaitsBeforeDropping(t *testing.T) {
	t.Parallel()
	exporter := clickhouse.NewExporter(&clickhouse.Client{}, clickhouse.ExporterConfig{
		Enabled:   true,
		BatchSize: 100,
	}, slog.Default())

	// Use a slow lister that blocks until cancelled to fill the semaphore.
	cancelCtx, cancelAll := context.WithCancel(context.Background())
	defer cancelAll()

	slowLister := &blockingEventLister{
		cancel: cancelCtx,
		events: []domain.RunEvent{{ID: "evt-1", RunID: "run-1"}},
	}

	sub := ClickHouseSubscriber(exporter, slowLister)

	// Fill the semaphore by launching maxConcurrentEventFetches goroutines.
	for i := range maxConcurrentEventFetches {
		sub(context.Background(), RunLifecycleEvent{
			Type: EventCompleted,
			Run:  &domain.JobRun{ID: "run-fill-" + string(rune('A'+i)), ProjectID: "proj-1"},
		})
	}
	time.Sleep(50 * time.Millisecond)

	// Next call should block (waiting for semaphore), not return instantly.
	start := time.Now()
	done := make(chan struct{})
	go func() {
		sub(context.Background(), RunLifecycleEvent{
			Type: EventCompleted,
			Run:  &domain.JobRun{ID: "run-blocked", ProjectID: "proj-1"},
		})
		close(done)
	}()

	select {
	case <-done:
		elapsed := time.Since(start)
		// Should have waited at least a bit (the 5s timeout), not returned instantly.
		if elapsed < 100*time.Millisecond {
			t.Errorf("subscriber returned too quickly (%v), expected to wait on semaphore", elapsed)
		}
	case <-time.After(10 * time.Second):
		// The 5-second timeout should have fired by now.
		t.Fatal("subscriber did not return within expected timeout")
	}

	// Clean up all blocked goroutines.
	cancelAll()
	time.Sleep(100 * time.Millisecond)
}

// blockingEventLister blocks ListEvents until its cancel context is done.
type blockingEventLister struct {
	cancel context.Context
	events []domain.RunEvent
}

func (b *blockingEventLister) ListEvents(ctx context.Context, _ string, _ int, _ *time.Time) ([]domain.RunEvent, error) {
	select {
	case <-b.cancel.Done():
		return nil, b.cancel.Err()
	case <-ctx.Done():
		return nil, ctx.Err()
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
			if got := isTerminalEvent(tt.eventType); got != tt.want {
				t.Errorf("isTerminalEvent(%s) = %v, want %v", tt.eventType, got, tt.want)
			}
		})
	}
}
