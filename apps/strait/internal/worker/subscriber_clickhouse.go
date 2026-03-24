package worker

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"strait/internal/clickhouse"
	"strait/internal/domain"
)

// EventLister fetches run events from the store. Implemented by *store.Queries.
type EventLister interface {
	ListEvents(ctx context.Context, runID string, limit int, cursor *time.Time) ([]domain.RunEvent, error)
}

// UsageLister fetches run usage records from the store. Implemented by *store.Queries.
type UsageLister interface {
	ListRunUsage(ctx context.Context, runID string, limit int, cursor *time.Time) ([]domain.RunUsage, error)
}

// ComputeUsageLister fetches committed compute usage records for a run. Implemented by *store.Queries.
type ComputeUsageLister interface {
	ListRunComputeUsage(ctx context.Context, runID string) ([]domain.RunComputeUsage, error)
}

// maxConcurrentEventFetches limits how many background ListEvents goroutines
// can run concurrently to prevent DB pool exhaustion under burst load.
const maxConcurrentEventFetches = 20

// ClickHouseSubscriberDeps holds optional dependencies for the ClickHouse subscriber.
type ClickHouseSubscriberDeps struct {
	UsageLister        UsageLister
	ComputeUsageLister ComputeUsageLister
}

// ClickHouseSubscriber enqueues run analytics and individual run events into
// the ClickHouse exporter on terminal run events (completed, failed, timed out, etc.).
//
//nolint:gocognit
func ClickHouseSubscriber(exporter *clickhouse.Exporter, events EventLister, deps ...ClickHouseSubscriberDeps) RunEventSubscriber {
	var usage UsageLister
	var computeUsage ComputeUsageLister
	if len(deps) > 0 {
		usage = deps[0].UsageLister
		computeUsage = deps[0].ComputeUsageLister
	}
	// Semaphore to bound concurrent ListEvents goroutines.
	sem := make(chan struct{}, maxConcurrentEventFetches)

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

		var tagsJSON string
		if len(run.Tags) > 0 {
			if b, err := json.Marshal(run.Tags); err == nil {
				tagsJSON = string(b)
			}
		}
		if tagsJSON == "" {
			tagsJSON = "{}"
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
			Tags:          tagsJSON,
			JobVersionID:  run.JobVersionID,
			CreatedAt:     time.Now(),
			StartedAt:     run.StartedAt,
			FinishedAt:    run.FinishedAt,
		})

		// Enqueue individual run events in background so we don't block the subscriber.
		// Semaphore bounds concurrent DB queries to prevent pool exhaustion under burst.
		// Use a timeout instead of instant drop to tolerate short bursts.
		if events != nil {
			select {
			case sem <- struct{}{}:
				go func() { //nolint:gosec // G118: intentionally detached from request ctx; subscriber must not block.
					defer func() { <-sem }()
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel()

					// Use a high single-query limit because ListEvents uses backward
					// pagination (created_at < cursor), so we fetch all events in one call.
					evts, err := events.ListEvents(ctx, run.ID, 10000, nil)
					if err != nil {
						slog.Error("clickhouse: list run events", "run_id", run.ID, "error", err)
						return
					}
					for _, rec := range runEventsFromDomain(run, evts) {
						exporter.Enqueue(rec)
					}
				}()
			case <-time.After(5 * time.Second):
				slog.Warn("clickhouse: event fetch semaphore timeout", "run_id", run.ID)
			}
		}

		// Enqueue run usage records in background so AI cost data flows to ClickHouse.
		if usage != nil {
			select {
			case sem <- struct{}{}:
				go func() { //nolint:gosec // G118: intentionally detached from request ctx; subscriber must not block.
					defer func() { <-sem }()
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel()

					records, err := usage.ListRunUsage(ctx, run.ID, 10000, nil)
					if err != nil {
						slog.Error("clickhouse: list run usage", "run_id", run.ID, "error", err)
						return
					}
					for _, u := range records {
						exporter.Enqueue(clickhouse.RunUsageEventRecord{
							RunID:            u.RunID,
							JobID:            run.JobID,
							ProjectID:        run.ProjectID,
							Provider:         u.Provider,
							Model:            u.Model,
							PromptTokens:     uint32(u.PromptTokens),     //nolint:gosec // tokens are always non-negative
							CompletionTokens: uint32(u.CompletionTokens), //nolint:gosec // tokens are always non-negative
							TotalTokens:      uint32(u.TotalTokens),      //nolint:gosec // tokens are always non-negative
							CostMicrousd:     u.CostMicrousd,
							CreatedAt:        u.CreatedAt,
						})
					}
				}()
			case <-time.After(5 * time.Second):
				slog.Warn("clickhouse: usage fetch semaphore timeout", "run_id", run.ID)
			}
		}

		// Enqueue compute usage records so managed execution costs flow to ClickHouse.
		if computeUsage != nil {
			select {
			case sem <- struct{}{}:
				go func() { //nolint:gosec // G118: intentionally detached from request ctx; subscriber must not block.
					defer func() { <-sem }()
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel()

					records, err := computeUsage.ListRunComputeUsage(ctx, run.ID)
					if err != nil {
						slog.Error("clickhouse: list run compute usage", "run_id", run.ID, "error", err)
						return
					}
					for _, cu := range records {
						var startedAt, finishedAt time.Time
						if cu.StartedAt != nil {
							startedAt = *cu.StartedAt
						}
						if cu.FinishedAt != nil {
							finishedAt = *cu.FinishedAt
						}
						exporter.Enqueue(clickhouse.ComputeUsageRecord{
							RunID:         cu.RunID,
							ProjectID:     cu.ProjectID,
							MachinePreset: cu.MachinePreset,
							MachineID:     cu.MachineID,
							DurationSecs:  cu.DurationSecs,
							CostMicrousd:  cu.CostMicrousd,
							StartedAt:     startedAt,
							FinishedAt:    finishedAt,
						})
					}
				}()
			case <-time.After(5 * time.Second):
				slog.Warn("clickhouse: compute usage fetch semaphore timeout", "run_id", run.ID)
			}
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
