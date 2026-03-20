package worker

import (
	"context"
	"time"

	"strait/internal/clickhouse"
	"strait/internal/domain"
)

// ClickHouseSubscriber enqueues run analytics into the ClickHouse exporter
// on terminal run events (completed, failed, timed out, etc.).
func ClickHouseSubscriber(exporter *clickhouse.Exporter) RunEventSubscriber {
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

// RunEventsFromDomain converts domain RunEvents to ClickHouse RunEventRecords.
func RunEventsFromDomain(run *domain.JobRun, events []domain.RunEvent) []clickhouse.RunEventRecord {
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
