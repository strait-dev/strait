package worker

import (
	"context"
	"log/slog"
	"time"

	"strait/internal/domain"
	"strait/internal/httputil"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/retrypolicy"
	"go.opentelemetry.io/otel"
)

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
