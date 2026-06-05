package worker

import (
	"encoding/json"
	"time"

	"strait/internal/clickhouse"
	"strait/internal/domain"
)

// runAnalyticsRecordFromLifecycleEvent assumes event.Run was nil-checked by the subscriber guard.
func runAnalyticsRecordFromLifecycleEvent(event RunLifecycleEvent, createdAt time.Time) clickhouse.RunAnalyticsRecord {
	run := event.Run

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

	tagsJSON := "{}"
	if len(run.Tags) > 0 {
		if b, err := json.Marshal(run.Tags); err == nil {
			tagsJSON = string(b)
		}
	}

	return clickhouse.RunAnalyticsRecord{
		RunID:         run.ID,
		JobID:         run.JobID,
		ProjectID:     run.ProjectID,
		Status:        string(run.Status),
		ExecutionMode: executionMode,
		Attempt:       run.Attempt,
		DurationMs:    durationMs,
		QueueWaitMs:   queueWaitMs,
		TriggeredBy:   run.TriggeredBy,
		Tags:          tagsJSON,
		JobVersionID:  run.JobVersionID,
		CreatedAt:     createdAt,
		StartedAt:     run.StartedAt,
		FinishedAt:    run.FinishedAt,
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
