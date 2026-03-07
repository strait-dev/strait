package worker

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"orchestrator/internal/domain"
)

const (
	webhookTimeout         = 10 * time.Second
	webhookMaxIdleConns    = 20
	webhookMaxIdlePerHost  = 5
	webhookIdleConnTimeout = 60 * time.Second
)

// webhookClient is a shared HTTP client for webhook delivery.
var webhookClient = &http.Client{
	Timeout: webhookTimeout,
	Transport: &http.Transport{
		MaxIdleConns:        webhookMaxIdleConns,
		MaxIdleConnsPerHost: webhookMaxIdlePerHost,
		IdleConnTimeout:     webhookIdleConnTimeout,
	},
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

func SendWebhook(ctx context.Context, job *domain.Job, run *domain.JobRun) {
	SendWebhookWithRetry(ctx, job, run, 3)
}

func SendWebhookWithRetry(ctx context.Context, job *domain.Job, run *domain.JobRun, maxAttempts int) WebhookResult {
	if job.WebhookURL == "" {
		return WebhookResult{Delivered: true}
	}

	if maxAttempts <= 0 {
		maxAttempts = 3
	}

	var result WebhookResult
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		result = sendWebhookOnce(ctx, job, run)
		if result.Delivered {
			slog.Info("webhook delivered",
				"run_id", run.ID,
				"url", job.WebhookURL,
				"status", result.StatusCode,
				"attempt", attempt,
			)
			return result
		}

		if result.StatusCode >= 400 && result.StatusCode < 500 {
			slog.Warn("webhook delivery failed with client error, not retrying",
				"run_id", run.ID,
				"url", job.WebhookURL,
				"status", result.StatusCode,
				"attempt", attempt,
			)
			return result
		}

		if attempt < maxAttempts {
			backoff := time.Duration(1) * time.Second
			for i := 1; i < attempt; i++ {
				backoff *= 5
			}

			slog.Warn("webhook delivery failed, retrying",
				"run_id", run.ID,
				"url", job.WebhookURL,
				"status", result.StatusCode,
				"error", result.Error,
				"attempt", attempt,
				"next_retry_in", backoff,
			)

			select {
			case <-ctx.Done():
				result.Error = "context canceled during retry"
				return result
			case <-time.After(backoff):
			}
		}
	}

	slog.Error("webhook delivery exhausted all retries",
		"run_id", run.ID,
		"url", job.WebhookURL,
		"attempts", maxAttempts,
		"last_error", result.Error,
	)

	return result
}

func sendWebhookOnce(ctx context.Context, job *domain.Job, run *domain.JobRun) WebhookResult {
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

	// HMAC-SHA256 signature if webhook secret is configured
	if job.WebhookSecret != "" {
		mac := hmac.New(sha256.New, []byte(job.WebhookSecret))
		mac.Write(body)
		sig := hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-Webhook-Signature", "sha256="+sig)
	}

	resp, err := webhookClient.Do(req)
	if err != nil {
		return WebhookResult{Error: "delivery failed: " + err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return WebhookResult{StatusCode: resp.StatusCode, Delivered: true}
	}

	return WebhookResult{StatusCode: resp.StatusCode, Error: fmt.Sprintf("HTTP %d", resp.StatusCode)}
}
