package worker

import (
	"context"
	"encoding/json"
	"maps"
	"strconv"
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

	payload, err := marshalRunStatusChangePayload(run.ID, run.JobID, run.ProjectID, data, time.Now().UTC())
	if err != nil {
		e.logger.Error("failed to marshal event", "error", err)
		return
	}

	channel := runPubSubChannel(run.ID)
	if err := e.publisher.Publish(ctx, channel, payload); err != nil {
		e.logger.Error("failed to publish event", "run_id", run.ID, "error", err)
	}
}

func marshalRunStatusChangePayload(runID, jobID, projectID string, data map[string]any, timestamp time.Time) ([]byte, error) {
	if len(data) == 2 {
		from, fromOK := data["from"].(string)
		to, toOK := data["to"].(string)
		if fromOK && toOK {
			return marshalRunStatusTransitionPayload(runID, jobID, projectID, from, to, timestamp)
		}
	}

	event := map[string]any{
		"type":       "status_change",
		"run_id":     runID,
		"job_id":     jobID,
		"project_id": projectID,
		"timestamp":  timestamp,
	}
	maps.Copy(event, data)
	return json.Marshal(event)
}

func marshalRunStatusTransitionPayload(runID, jobID, projectID, from, to string, timestamp time.Time) ([]byte, error) {
	capacity := len(`{"type":"status_change","run_id":"","job_id":"","project_id":"","from":"","to":"","timestamp":""}`) +
		len(runID) + len(jobID) + len(projectID) + len(from) + len(to) + len(time.RFC3339Nano)
	out := make([]byte, 0, capacity)
	out = append(out, `{"type":"status_change","run_id":`...)
	out = strconv.AppendQuote(out, runID)
	out = append(out, `,"job_id":`...)
	out = strconv.AppendQuote(out, jobID)
	out = append(out, `,"project_id":`...)
	out = strconv.AppendQuote(out, projectID)
	out = append(out, `,"from":`...)
	out = strconv.AppendQuote(out, from)
	out = append(out, `,"to":`...)
	out = strconv.AppendQuote(out, to)
	out = append(out, `,"timestamp":"`...)
	out = timestamp.AppendFormat(out, time.RFC3339Nano)
	out = append(out, `"}`...)
	return out, nil
}

func runPubSubChannel(runID string) string {
	return "run:" + runID
}
