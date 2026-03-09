package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"strait/internal/domain"

	"go.opentelemetry.io/otel"
)

// cancelWebhookTimeout is the timeout for cancel endpoint dispatch.
// We use a shorter timeout than normal execution since this is best-effort.
const cancelWebhookTimeout = 10 * time.Second

// cancelWebhookPayload is the JSON body sent to the cancel endpoint.
type cancelWebhookPayload struct {
	RunID   string `json:"run_id"`
	JobID   string `json:"job_id"`
	JobSlug string `json:"job_slug"`
}

// dispatchCancel sends a cancellation webhook to the job's cancel endpoint.
// This is a best-effort operation — errors are logged but do not prevent
// the run from being canceled. A fresh context is used because the parent
// context may already be canceled.
func (e *Executor) dispatchCancel(job *domain.Job, run *domain.JobRun) {
	_, span := otel.Tracer("strait").Start(context.Background(), "executor.DispatchCancel")
	defer span.End()

	if job.CancelEndpointURL == "" {
		return
	}

	payload := cancelWebhookPayload{
		RunID:   run.ID,
		JobID:   job.ID,
		JobSlug: job.Slug,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		e.logger.Error("failed to marshal cancel payload",
			"run_id", run.ID,
			"job_id", job.ID,
			"error", err,
		)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), cancelWebhookTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, job.CancelEndpointURL, bytes.NewReader(body))
	if err != nil {
		e.logger.Error("failed to create cancel request",
			"run_id", run.ID,
			"cancel_url", job.CancelEndpointURL,
			"error", err,
		)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		e.logger.Warn("cancel endpoint dispatch failed",
			"run_id", run.ID,
			"cancel_url", job.CancelEndpointURL,
			"error", err,
		)
		return
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= 400 {
		e.logger.Warn("cancel endpoint returned error",
			"run_id", run.ID,
			"cancel_url", job.CancelEndpointURL,
			"status", resp.StatusCode,
		)
		return
	}

	e.logger.Info("cancel endpoint dispatched",
		"run_id", run.ID,
		"cancel_url", job.CancelEndpointURL,
		"status", resp.StatusCode,
	)
}

// transitionToCanceling moves a run to the canceling state, dispatches the
// cancel webhook, then moves to canceled. If the job has no cancel endpoint,
// it transitions directly to canceled.
func (e *Executor) transitionToCanceling(ctx context.Context, job *domain.Job, run *domain.JobRun) {
	now := time.Now()

	if job.CancelEndpointURL != "" {
		// Attempt graceful cancel: executing → canceling → canceled
		err := e.store.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusCanceling, map[string]any{
			"error": "canceled",
		})
		if err == nil {
			run.Status = domain.StatusCanceling
			e.publishEvent(ctx, run, map[string]any{"from": "executing", "to": "canceling"})

			e.dispatchCancel(job, run)

			_ = e.store.UpdateRunStatus(ctx, run.ID, domain.StatusCanceling, domain.StatusCanceled, map[string]any{
				"finished_at": now,
			})
			run.Status = domain.StatusCanceled
			e.publishEvent(ctx, run, map[string]any{"from": "canceling", "to": "canceled"})
			e.recordRunTransition(ctx, domain.StatusCanceling, domain.StatusCanceled)
			return
		}
		// If transition to canceling failed (race), fall through to direct cancel
		e.logger.Warn("failed to transition to canceling, falling back to direct cancel",
			"run_id", run.ID,
			"error", err,
		)
	}

	// Direct cancel: executing → canceled
	if err := e.store.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusCanceled, map[string]any{
		"finished_at": now,
		"error":       "canceled",
	}); err != nil {
		e.logger.Error("failed to cancel run",
			"run_id", run.ID,
			"error", err,
		)
		return
	}
	run.Status = domain.StatusCanceled
	e.publishEvent(ctx, run, map[string]any{"from": "executing", "to": "canceled"})
	e.recordRunTransition(ctx, domain.StatusExecuting, domain.StatusCanceled)
}
