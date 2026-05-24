package worker

import (
	"context"
	"net/http"

	"strait/internal/domain"

	"go.opentelemetry.io/otel"
)

// sendWebhookWithClientForTest is a test-only entrypoint that bypasses
// the SSRF-safe webhookClient so loopback httptest servers can exercise
// retry, HMAC, redaction, and timeout behavior. Lives in a _test.go
// file so the production binary never links it; webhook_unexported_test
// also pins that the helper stays unexported.
func sendWebhookWithClientForTest(ctx context.Context, client *http.Client, job *domain.Job, run *domain.JobRun, maxAttempts int) WebhookResult {
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
		return sendWebhookOnceWith(ctx, client, job, run)
	})
}
