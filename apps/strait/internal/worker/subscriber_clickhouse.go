package worker

import (
	"context"
	"log/slog"
	"time"

	"strait/internal/clickhouse"
	"strait/internal/domain"
)

// EventLister fetches run events from the store. Implemented by *store.Queries.
type EventLister interface {
	ListEvents(ctx context.Context, runID string, limit int, cursor *time.Time) ([]domain.RunEvent, error)
}

// ClickHouseSubscriber enqueues run analytics and individual run events into
// the ClickHouse exporter on terminal run events (completed, failed, timed out, etc.).
func ClickHouseSubscriber(exporter *clickhouse.Exporter, events EventLister) RunEventSubscriber {
	return func(_ context.Context, event RunLifecycleEvent) {
		if exporter == nil {
			return
		}
		if !isTerminalEvent(event.Type) {
			return
		}

		run := event.Run
		if run == nil {
			return
		}

		var durationMs uint64
		if run.StartedAt != nil && run.FinishedAt != nil {
			if ms := run.FinishedAt.Sub(*run.StartedAt).Milliseconds(); ms > 0 {
				durationMs = uint64(ms)
			}
		}

		var queueWaitMs uint64
		if event.QueueWait > 0 {
			queueWaitMs = uint64(event.QueueWait.Milliseconds()) //nolint:gosec // queue wait is always positive
		}

		var executionMode string
		if event.Job != nil {
			executionMode = string(event.Job.ExecutionMode)
		}

		var machinePreset string
		if event.Job != nil {
			machinePreset = string(event.Job.MachinePreset)
		}

		exporter.Enqueue(clickhouse.RunAnalyticsRecord{
			RunID:         run.ID,
			JobID:         run.JobID,
			ProjectID:     run.ProjectID,
			Status:        string(run.Status),
			ExecutionMode: executionMode,
			MachinePreset: machinePreset,
			Attempt:       run.Attempt,
			DurationMs:    durationMs,
			QueueWaitMs:   queueWaitMs,
			TriggeredBy:   run.TriggeredBy,
			CreatedAt:     time.Now(),
			StartedAt:     run.StartedAt,
			FinishedAt:    run.FinishedAt,
		})

		// Enqueue individual run events in background so we don't block the subscriber.
		if events != nil {
			go func() { //nolint:gosec // G118: intentionally detached from request ctx; subscriber must not block.
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()

				evts, err := events.ListEvents(ctx, run.ID, 1000, nil)
				if err != nil {
					slog.Error("clickhouse: list run events", "run_id", run.ID, "error", err)
					return
				}
				for _, rec := range runEventsFromDomain(run, evts) {
					exporter.Enqueue(rec)
				}
			}()
		}
	}
}

func isTerminalEvent(t RunEventType) bool {
	switch t {
	case EventCompleted, EventTimedOut, EventDeadLettered, EventSystemFailed:
		return true
	default:
		return false
	}
}

// runEventsFromDomain converts domain RunEvents to ClickHouse RunEventRecords.
func runEventsFromDomain(run *domain.JobRun, events []domain.RunEvent) []clickhouse.RunEventRecord {
	records := make([]clickhouse.RunEventRecord, 0, len(events))
	for _, e := range events {
		var metadata string
		if e.Data != nil {
			metadata = string(e.Data)
		}
		records = append(records, clickhouse.RunEventRecord{
			EventID:   e.ID,
			RunID:     e.RunID,
			ProjectID: run.ProjectID,
			JobID:     run.JobID,
			EventType: string(e.Type),
			Level:     e.Level,
			Message:   e.Message,
			Metadata:  metadata,
			CreatedAt: e.CreatedAt,
		})
	}
	return records
}
