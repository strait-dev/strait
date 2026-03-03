package worker

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"orchestrator/internal/domain"
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

// SendWebhook sends a webhook notification for a run that reached a terminal state.
// It is fire-and-forget — errors are logged but not returned.
func SendWebhook(ctx context.Context, job *domain.Job, run *domain.JobRun) {
	if job.WebhookURL == "" {
		return
	}

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
		slog.Error("webhook marshal failed", "run_id", run.ID, "error", err)
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, job.WebhookURL, bytes.NewReader(body))
	if err != nil {
		slog.Error("webhook request build failed", "run_id", run.ID, "error", err)
		return
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

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		slog.Error("webhook delivery failed", "run_id", run.ID, "url", job.WebhookURL, "error", err)
		return
	}
	defer resp.Body.Close()

	slog.Info("webhook delivered",
		"run_id", run.ID,
		"url", job.WebhookURL,
		"status", resp.StatusCode,
	)
}
