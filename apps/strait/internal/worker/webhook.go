package worker

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"strait/internal/domain"

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

var webhookClient = &http.Client{
	Timeout: webhookTimeout,
	Transport: otelhttp.NewTransport(&http.Transport{
		MaxIdleConns:        webhookMaxIdleConns,
		MaxIdleConnsPerHost: webhookMaxIdlePerHost,
		IdleConnTimeout:     webhookIdleConnTimeout,
	}),
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

func newWebhookRetryPolicy(maxAttempts int) retrypolicy.RetryPolicy[WebhookResult] {
	return retrypolicy.NewBuilder[WebhookResult]().
		WithMaxRetries(maxAttempts-1).
		WithBackoffFactor(time.Second, 25*time.Second, 5.0).
		HandleIf(func(result WebhookResult, _ error) bool {
			if result.StatusCode >= 400 && result.StatusCode < 500 {
				return false
			}
			return !result.Delivered
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

	rp := newWebhookRetryPolicy(maxAttempts)
	return sendWithRetryPolicy(ctx, rp, job, run, func(ctx context.Context) WebhookResult {
		return sendWebhookOnce(ctx, job, run)
	})
}

// SendWebhookWithClient sends a webhook using the provided HTTP client and retry count.
func SendWebhookWithClient(ctx context.Context, client *http.Client, job *domain.Job, run *domain.JobRun, maxAttempts int) WebhookResult {
	if job.WebhookURL == "" {
		return WebhookResult{Delivered: true}
	}

	ctx, span := otel.Tracer("strait").Start(ctx, "webhook.SendWithRetry")
	defer span.End()

	if maxAttempts <= 0 {
		maxAttempts = 3
	}

	rp := newWebhookRetryPolicy(maxAttempts)
	return sendWithRetryPolicy(ctx, rp, job, run, func(ctx context.Context) WebhookResult {
		return sendWebhookOnceWith(ctx, client, job, run)
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
			"url", job.WebhookURL,
			"status", result.StatusCode,
		)
	} else {
		slog.Error("webhook delivery exhausted all retries",
			"run_id", run.ID,
			"url", job.WebhookURL,
			"last_error", result.Error,
		)
	}

	return result
}

func sendWebhookOnce(ctx context.Context, job *domain.Job, run *domain.JobRun) WebhookResult {
	ctx, span := otel.Tracer("strait").Start(ctx, "webhook.Deliver")
	defer span.End()
	span.SetAttributes(
		attribute.String("webhook.run_id", run.ID),
		attribute.String("webhook.job_id", run.JobID),
		attribute.String("webhook.url", job.WebhookURL),
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
	applyWebhookSignature(req, job.WebhookSecret, body)

	resp, err := webhookClient.Do(req)
	if err != nil {
		return WebhookResult{Error: "delivery failed: " + err.Error()}
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

func applyWebhookSignature(req *http.Request, webhookSecret string, body []byte) {
	if webhookSecret == "" {
		return
	}
	ts := strconv.FormatInt(time.Now().UTC().Unix(), 10)
	payload := append([]byte(ts+"."), body...)
	mac := hmac.New(sha256.New, []byte(webhookSecret))
	_, _ = mac.Write(payload)
	sig := hex.EncodeToString(mac.Sum(nil))

	// New headers.
	req.Header.Set("X-Strait-Timestamp", ts)
	req.Header.Set("X-Strait-Signature", "v1="+sig)
	req.Header.Set("X-Webhook-Signature", "v1="+sig)
}

func sendWebhookOnceWith(ctx context.Context, client *http.Client, job *domain.Job, run *domain.JobRun) WebhookResult {
	ctx, span := otel.Tracer("strait").Start(ctx, "webhook.Deliver")
	defer span.End()
	span.SetAttributes(
		attribute.String("webhook.run_id", run.ID),
		attribute.String("webhook.job_id", run.JobID),
		attribute.String("webhook.url", job.WebhookURL),
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
	applyWebhookSignature(req, job.WebhookSecret, body)

	resp, err := client.Do(req)
	if err != nil {
		return WebhookResult{Error: "delivery failed: " + err.Error()}
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
