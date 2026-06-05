package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"time"

	"github.com/getsentry/sentry-go"

	"strait/internal/domain"
)

// Subscribe registers a run lifecycle event subscriber. Must be called before Run().
func (e *Executor) Subscribe(sub RunEventSubscriber) {
	e.subscribers = append(e.subscribers, sub)
}

// emit sends a lifecycle event to all subscribers via the buffered channel.
// Non-blocking: drops the event with a warning if the channel is full or closed.
func (e *Executor) emit(ctx context.Context, event RunLifecycleEvent) {
	if len(e.subscribers) == 0 {
		return
	}

	// Recover from send-on-closed-channel if the executor is shutting down
	// and a pool goroutine emits after eventCh is closed.
	defer func() {
		if r := recover(); r != nil {
			sentry.CurrentHub().Recover(r)
			sentry.Flush(2 * time.Second)
			e.logger.Warn("event channel closed, dropping event",
				"type", event.Type,
				"run_id", event.Run.ID,
			)
			e.recordEventChannelDrop(ctx, eventChannelKindClosed)
		}
	}()

	select {
	case e.eventCh <- runEventEnvelope{ctx: ctx, event: event}:
		e.sampleEventChannelSaturation(ctx, eventChannelKind(event.Type))
	default:
		kind := eventChannelKind(event.Type)
		if e.shouldLogSaturation(kind) {
			e.logger.Warn("event channel full, dropping event",
				"type", event.Type,
				"run_id", event.Run.ID,
			)
		}
		e.recordEventChannelDrop(ctx, kind)
	}
}

// runEventLoop drains the event channel and fans out to all subscribers.
// Exits when eventCh is closed (during shutdown or when Run exits).
func (e *Executor) runEventLoop() {
	for env := range e.eventCh {
		for _, sub := range e.subscribers {
			func() {
				defer func() {
					if r := recover(); r != nil {
						e.logger.Error("event subscriber panicked", "panic", r)
					}
				}()
				sub(env.ctx, env.event)
			}()
		}
	}
}

func (e *Executor) notifyWorkflowCallback(ctx context.Context, run *domain.JobRun) {
	if e.workflowCallback == nil {
		return
	}

	e.callbackWG.Go(func() {
		if err := e.workflowCallback.OnJobRunTerminal(ctx, run); err != nil {
			e.logger.Error("workflow callback failed", "run_id", run.ID, "error", err)
		}
	})
}

func (e *Executor) publishEvent(ctx context.Context, run *domain.JobRun, data map[string]any) {
	if e.publisher == nil {
		return
	}

	event := map[string]any{
		"type":       "status_change",
		"run_id":     run.ID,
		"job_id":     run.JobID,
		"project_id": run.ProjectID,
		"timestamp":  time.Now().UTC(),
	}
	maps.Copy(event, data)

	payload, err := json.Marshal(event)
	if err != nil {
		e.logger.Error("failed to marshal event", "error", err)
		return
	}

	channel := fmt.Sprintf("run:%s", run.ID)
	if err := e.publisher.Publish(ctx, channel, payload); err != nil {
		e.logger.Error("failed to publish event", "run_id", run.ID, "error", err)
	}
}
