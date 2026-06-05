package worker

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"strait/internal/clickhouse"
	"strait/internal/domain"

	"github.com/sourcegraph/conc"
)

// EventLister fetches run events from the store. Implemented by *store.Queries.
type EventLister interface {
	ListEvents(ctx context.Context, runID string, limit int, cursor *time.Time) ([]domain.RunEvent, error)
}

// maxConcurrentEventFetches limits how many background ListEvents goroutines
// can run concurrently to prevent DB pool exhaustion under burst load.
const maxConcurrentEventFetches = 20

// ClickHouseSubscriberHandle wraps a ClickHouse subscriber and tracks its
// background goroutines so callers can wait for graceful drain on shutdown.
type ClickHouseSubscriberHandle struct {
	wg  conc.WaitGroup
	sub RunEventSubscriber
}

// Subscriber returns the RunEventSubscriber function for use with Executor.Subscribe.
func (h *ClickHouseSubscriberHandle) Subscriber() RunEventSubscriber {
	return h.sub
}

// Wait blocks until all background goroutines launched by the subscriber have
// completed. Call this during graceful shutdown after the executor has stopped
// dispatching events.
func (h *ClickHouseSubscriberHandle) Wait() {
	h.wg.Wait()
}

// ClickHouseSubscriber enqueues run analytics and individual run events into
// the ClickHouse exporter on terminal run events (completed, failed, timed out, etc.).
func ClickHouseSubscriber(exporter *clickhouse.Exporter, events EventLister) RunEventSubscriber {
	handle := NewClickHouseSubscriberHandle(exporter, events)
	return handle.Subscriber()
}

// NewClickHouseSubscriberHandle creates a ClickHouseSubscriberHandle that
// tracks background goroutines and exposes a Wait method for graceful drain.
func NewClickHouseSubscriberHandle(exporter *clickhouse.Exporter, events EventLister) *ClickHouseSubscriberHandle {
	// Semaphore to bound concurrent ListEvents goroutines.
	sem := make(chan struct{}, maxConcurrentEventFetches)

	h := &ClickHouseSubscriberHandle{}
	h.sub = func(_ context.Context, event RunLifecycleEvent) {
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
				h.wg.Go(func() {
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
				})
			case <-time.After(5 * time.Second):
				slog.Warn("clickhouse: event fetch semaphore timeout", "run_id", run.ID)
			}
		}
	}

	return h
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
