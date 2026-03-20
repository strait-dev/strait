package worker

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"strait/internal/clickhouse"
	"strait/internal/domain"
)

func TestClickHouseSubscriber_NilExporter(t *testing.T) {
	t.Parallel()
	sub := ClickHouseSubscriber(nil)
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

	sub := ClickHouseSubscriber(exporter)
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

	sub := ClickHouseSubscriber(exporter)
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

	sub := ClickHouseSubscriber(exporter)
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

			sub := ClickHouseSubscriber(exporter)
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
