package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"strait/internal/domain"
	"strait/internal/httputil"
	"strait/internal/webhook"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

// WebhookPayload is sent to the job's webhook URL on terminal states.
type WebhookPayload struct {
	RunID     string          `json:"run_id"`
	JobID     string          `json:"job_id"`
	ProjectID string          `json:"project_id"`
	Status    string          `json:"status"`
	Attempt   int             `json:"attempt"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     string          `json:"error,omitempty"`
	Timestamp time.Time       `json:"timestamp"`
}

func sendWebhookOnceWith(ctx context.Context, client *http.Client, job *domain.Job, run *domain.JobRun) WebhookResult {
	ctx, span := otel.Tracer("strait").Start(ctx, "webhook.Deliver")
	defer span.End()
	span.SetAttributes(
		attribute.String("webhook.run_id", run.ID),
		attribute.String("webhook.job_id", run.JobID),
		attribute.String("webhook.url", httputil.RedactURLForLog(job.WebhookURL)),
	)
	payload := WebhookPayload{
		RunID:     run.ID,
		JobID:     run.JobID,
		ProjectID: run.ProjectID,
		Status:    string(run.Status),
		Attempt:   run.Attempt,
		Result:    run.Result,
		Error:     run.Error,
		Timestamp: time.Now().UTC(),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return WebhookResult{Error: "marshal failed: " + err.Error()}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, job.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return WebhookResult{Error: "request build failed: " + err.Error()}
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Run-ID", run.ID)
	// Stable across retries of the same run so subscribers can dedup
	// replays on a signal that does not change with attempt count. The
	// run-terminal webhook path has no subscription secret available,
	// so we use the unsigned helper. See internal/webhook for the
	// HMAC-bound variant used by subscription deliveries.
	req.Header.Set("X-Strait-Replay-Key", webhook.ComputeReplayKeyUnsigned(run.ID))
	applyWebhookSignature(req, job.WebhookSecret, body)

	resp, err := client.Do(req)
	if err != nil {
		return WebhookResult{Error: "delivery failed: " + httputil.SanitizeHTTPClientError(err)}
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return WebhookResult{StatusCode: resp.StatusCode, Delivered: true}
	}

	return WebhookResult{StatusCode: resp.StatusCode, Error: fmt.Sprintf("HTTP %d", resp.StatusCode)}
}
