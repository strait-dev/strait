package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"strait/internal/domain"
	"strait/internal/httputil"
	"strait/internal/webhook"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/retrypolicy"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

const (
	webhookTimeout         = 10 * time.Second
	webhookMaxIdleConns    = 20
	webhookMaxIdlePerHost  = 5
	webhookIdleConnTimeout = 60 * time.Second
)

// noFollowWebhookRedirects refuses to follow HTTP redirects on outbound
// webhook deliveries. Following 3xx without re-validating the destination
// IP would let a public webhook target bounce the request to internal
// addresses (cloud metadata, 10.x, 127.x) after the initial SSRF check.
func noFollowWebhookRedirects(_ *http.Request, _ []*http.Request) error {
	return http.ErrUseLastResponse
}

func newSafeWebhookTransport() *http.Transport {
	transport := httputil.NewExternalTransport(false)
	transport.MaxIdleConns = webhookMaxIdleConns
	transport.MaxIdleConnsPerHost = webhookMaxIdlePerHost
	transport.IdleConnTimeout = webhookIdleConnTimeout
	return transport
}

var webhookClient = &http.Client{
	Timeout:       webhookTimeout,
	Transport:     otelhttp.NewTransport(newSafeWebhookTransport()),
	CheckRedirect: noFollowWebhookRedirects,
}

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

type WebhookResult struct {
	StatusCode int
	Delivered  bool
	Error      string
}

func newWebhookRetryPolicy(maxAttempts int, job *domain.Job, run *domain.JobRun) retrypolicy.RetryPolicy[WebhookResult] {
	return retrypolicy.NewBuilder[WebhookResult]().
		WithMaxRetries(maxAttempts-1).
		WithBackoffFactor(time.Second, 25*time.Second, 5.0).
		HandleIf(func(result WebhookResult, _ error) bool {
			if result.StatusCode >= 400 && result.StatusCode < 500 {
				return false
			}
			return !result.Delivered
		}).
		OnRetry(func(e failsafe.ExecutionEvent[WebhookResult]) {
			prev := e.LastResult()
			slog.Warn("webhook delivery failed, retrying",
				"run_id", run.ID,
				"url", httputil.RedactURLForLog(job.WebhookURL),
				"status", prev.StatusCode,
				"error", prev.Error,
				"attempt", e.Attempts(),
			)
		}).
		ReturnLastFailure().
		Build()
}

func SendWebhook(ctx context.Context, job *domain.Job, run *domain.JobRun) {
	SendWebhookWithRetry(ctx, job, run, 3)
}

func SendWebhookWithRetry(ctx context.Context, job *domain.Job, run *domain.JobRun, maxAttempts int) WebhookResult {
	if job.WebhookURL == "" {
		return WebhookResult{Delivered: true}
	}

	ctx, span := otel.Tracer("strait").Start(ctx, "webhook.SendWithRetry")
	defer span.End()

	if maxAttempts <= 0 {
		maxAttempts = 3
	}

	rp := newWebhookRetryPolicy(maxAttempts, job, run)
	return sendWithRetryPolicy(ctx, rp, job, run, func(ctx context.Context) WebhookResult {
		return sendWebhookOnceWith(ctx, webhookClient, job, run)
	})
}

// sendWithRetryPolicy executes a webhook send function with the given retry policy.
// The sendFn parameter performs a single delivery attempt.
func sendWithRetryPolicy(
	ctx context.Context,
	rp retrypolicy.RetryPolicy[WebhookResult],
	job *domain.Job,
	run *domain.JobRun,
	sendFn func(ctx context.Context) WebhookResult,
) WebhookResult {
	result, err := failsafe.With[WebhookResult](rp).
		WithContext(ctx).
		Get(func() (WebhookResult, error) {
			return sendFn(ctx), nil
		})
	if err != nil {
		return WebhookResult{Error: "context canceled during retry"}
	}

	if result.Delivered {
		slog.Info("webhook delivered",
			"run_id", run.ID,
			"url", httputil.RedactURLForLog(job.WebhookURL),
			"status", result.StatusCode,
		)
	} else {
		slog.Error("webhook delivery exhausted all retries",
			"run_id", run.ID,
			"url", httputil.RedactURLForLog(job.WebhookURL),
			"last_error", result.Error,
		)
	}

	return result
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
