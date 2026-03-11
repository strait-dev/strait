// Package webhook provides durable webhook delivery for event triggers.
//
// Deliveries are persisted to the webhook_deliveries table on creation,
// then processed by a background worker with exponential backoff. If all
// attempts are exhausted, the delivery moves to "dead" status (DLQ).
// This survives process restarts — no in-memory state is required.
package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"strait/internal/domain"

	concpool "github.com/sourcegraph/conc/pool"
	"go.opentelemetry.io/otel"
)

// DeliveryStore is the subset of store operations needed by the webhook worker.
type DeliveryStore interface {
	CreateWebhookDelivery(ctx context.Context, d *domain.WebhookDelivery) error
	UpdateWebhookDelivery(ctx context.Context, d *domain.WebhookDelivery) error
	ListPendingWebhookRetries(ctx context.Context) ([]domain.WebhookDelivery, error)
	UpdateEventTriggerNotifyStatus(ctx context.Context, id string, notifyStatus string) error
}

const defaultDeliveryConcurrency = 5

type DeliveryWorker struct {
	client *http.Client
	store  DeliveryStore
	logger *slog.Logger

	concurrency        int
	defaultRetryPolicy string
	circuitBreaker     WebhookCircuitBreaker
}

type EventNotifier = DeliveryWorker

type DeliveryWorkerOption func(*DeliveryWorker)

func WithConcurrency(n int) DeliveryWorkerOption {
	return func(w *DeliveryWorker) {
		if n > 0 {
			w.concurrency = n
		}
	}
}

func WithRetryPolicy(policy string) DeliveryWorkerOption {
	return func(w *DeliveryWorker) {
		switch policy {
		case domain.WebhookRetryPolicyExponential, domain.WebhookRetryPolicyLinear, domain.WebhookRetryPolicyFixed:
			w.defaultRetryPolicy = policy
		}
	}
}

func WithCircuitBreaker(circuitBreaker WebhookCircuitBreaker) DeliveryWorkerOption {
	return func(w *DeliveryWorker) {
		w.circuitBreaker = circuitBreaker
	}
}

func NewDeliveryWorker(store DeliveryStore, logger *slog.Logger, opts ...DeliveryWorkerOption) *DeliveryWorker {
	if logger == nil {
		logger = slog.Default()
	}
	w := &DeliveryWorker{
		client:             &http.Client{Timeout: 10 * time.Second},
		store:              store,
		logger:             logger,
		concurrency:        defaultDeliveryConcurrency,
		defaultRetryPolicy: domain.WebhookRetryPolicyExponential,
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

// NewEventNotifier creates a new event notifier.
func NewEventNotifier(store DeliveryStore, logger *slog.Logger, opts ...DeliveryWorkerOption) *DeliveryWorker {
	return NewDeliveryWorker(store, logger, opts...)
}

// NotifyAsync persists a webhook delivery for the given trigger.
// This is the synchronous entry point called during trigger creation
// via the onTriggerCreate callback. The actual HTTP delivery happens
// asynchronously via RunWorker.
func (n *DeliveryWorker) NotifyAsync(trigger *domain.EventTrigger) {
	if trigger.NotifyURL == "" {
		return
	}

	payload, err := json.Marshal(map[string]any{
		"event_key":    trigger.EventKey,
		"trigger_id":   trigger.ID,
		"project_id":   trigger.ProjectID,
		"expires_at":   trigger.ExpiresAt,
		"callback_url": fmt.Sprintf("/v1/events/%s/send", trigger.EventKey),
	})
	if err != nil {
		n.logger.Error("failed to marshal notify payload", "trigger_id", trigger.ID, "error", err)
		return
	}

	now := time.Now()
	d := &domain.WebhookDelivery{
		EventTriggerID: trigger.ID,
		WebhookURL:     trigger.NotifyURL,
		RetryPolicy:    n.defaultRetryPolicy,
		Status:         domain.WebhookStatusPending,
		Attempts:       0,
		MaxAttempts:    5,
		NextRetryAt:    &now,
	}

	// Store the payload as the last_error field temporarily — the worker reads
	// it from there. Better: we use a dedicated approach below.
	// Actually, we POST the payload directly from the delivery record.
	// We need to stash the payload somewhere. Since the existing schema doesn't
	// have a payload column, we'll reconstruct it from the trigger in the worker.
	// For now, store a marker so the worker can look up the trigger.
	d.LastError = string(payload) // stash payload in last_error temporarily on creation

	if err := n.store.CreateWebhookDelivery(context.Background(), d); err != nil {
		n.logger.Error("failed to enqueue webhook delivery", "trigger_id", trigger.ID, "error", err)
		return
	}

	// Clear the last_error now that we've stored it (the worker will use it as payload).
	n.logger.Info("webhook delivery enqueued", "delivery_id", d.ID, "trigger_id", trigger.ID, "url", trigger.NotifyURL)
}

func (n *DeliveryWorker) EnqueueRunWebhook(ctx context.Context, job *domain.Job, run *domain.JobRun) error {
	if job == nil || run == nil {
		return fmt.Errorf("enqueue run webhook: job and run are required")
	}
	if job.WebhookURL == "" || !run.Status.IsTerminal() {
		return nil
	}

	payload, err := json.Marshal(map[string]any{
		"run_id":     run.ID,
		"job_id":     run.JobID,
		"project_id": run.ProjectID,
		"status":     string(run.Status),
		"attempt":    run.Attempt,
		"result":     run.Result,
		"error":      run.Error,
		"timestamp":  time.Now().UTC(),
	})
	if err != nil {
		return fmt.Errorf("enqueue run webhook: marshal payload: %w", err)
	}

	now := time.Now()
	d := &domain.WebhookDelivery{
		RunID:       run.ID,
		JobID:       run.JobID,
		WebhookURL:  job.WebhookURL,
		RetryPolicy: n.defaultRetryPolicy,
		Status:      domain.WebhookStatusPending,
		Attempts:    0,
		MaxAttempts: 5,
		NextRetryAt: &now,
		LastError:   string(payload),
	}

	if err := n.store.CreateWebhookDelivery(ctx, d); err != nil {
		return fmt.Errorf("enqueue run webhook: create delivery: %w", err)
	}

	return nil
}

// RunWorker polls for pending deliveries and attempts them. Blocks until ctx is canceled.
// Call this in a goroutine from the service startup (e.g., concpool group).
func (n *DeliveryWorker) RunWorker(ctx context.Context, pollInterval time.Duration) error {
	n.logger.Info("webhook delivery worker started", "poll_interval", pollInterval)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			n.logger.Info("webhook delivery worker stopped")
			return ctx.Err()
		case <-ticker.C:
			n.processBatch(ctx)
		}
	}
}

// processBatch fetches and processes a batch of pending deliveries.
func (n *DeliveryWorker) processBatch(ctx context.Context) {
	ctx, span := otel.Tracer("strait").Start(ctx, "webhook.ProcessBatch")
	defer span.End()

	deliveries, err := n.store.ListPendingWebhookRetries(ctx)
	if err != nil {
		n.logger.Error("failed to list pending webhook deliveries", "error", err)
		return
	}

	p := concpool.New().WithContext(ctx).WithMaxGoroutines(n.concurrency)
	for i := range deliveries {
		delivery := deliveries[i]
		p.Go(func(ctx context.Context) error {
			n.attemptDelivery(ctx, &delivery)
			return nil
		})
	}

	if err := p.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		n.logger.Error("webhook batch delivery failed", "error", err)
	}
}

// attemptDelivery makes one HTTP request for a delivery.
func (n *DeliveryWorker) attemptDelivery(ctx context.Context, d *domain.WebhookDelivery) {
	now := time.Now()
	d.Attempts++

	if n.circuitBreaker != nil {
		canDeliver, err := n.circuitBreaker.CanDeliver(ctx, d.WebhookURL)
		if err != nil {
			n.logger.Warn("webhook circuit breaker check failed", "delivery_id", d.ID, "url", d.WebhookURL, "error", err)
		} else if !canDeliver {
			n.recordFailure(ctx, d, now, true, "circuit breaker is open")
			return
		}
	}

	// Reconstruct payload from last_error (where we stashed it on creation)
	// or build a minimal payload from what we know.
	var body []byte
	if d.LastError != "" {
		// Try to parse as JSON — if it is, it's our stashed payload.
		var js json.RawMessage
		if json.Unmarshal([]byte(d.LastError), &js) == nil {
			body = []byte(d.LastError)
			d.LastError = "" // clear so failed attempts use the error message
		}
	}
	if len(body) == 0 {
		// Fallback: minimal payload.
		payload := map[string]any{
			"trigger_id":  d.EventTriggerID,
			"delivery_id": d.ID,
		}
		body, _ = json.Marshal(payload)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.WebhookURL, bytes.NewReader(body))
	if err != nil {
		n.recordFailure(ctx, d, now, false, fmt.Sprintf("create request: %v", err))
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if d.EventTriggerID != "" {
		req.Header.Set("X-Strait-Trigger-ID", d.EventTriggerID)
	}
	if d.RunID != "" {
		req.Header.Set("X-Run-ID", d.RunID)
	}
	if d.JobID != "" {
		req.Header.Set("X-Job-ID", d.JobID)
	}
	req.Header.Set("X-Strait-Delivery-ID", d.ID)
	req.Header.Set("X-Strait-Attempt", fmt.Sprintf("%d/%d", d.Attempts, d.MaxAttempts))

	resp, err := n.client.Do(req)
	if err != nil {
		if n.circuitBreaker != nil {
			n.circuitBreaker.RecordFailure(ctx, d.WebhookURL)
		}
		n.recordFailure(ctx, d, now, true, fmt.Sprintf("http request: %v", err))
		return
	}
	defer resp.Body.Close()

	statusCode := resp.StatusCode
	d.LastStatusCode = &statusCode

	if statusCode >= 500 {
		if n.circuitBreaker != nil {
			n.circuitBreaker.RecordFailure(ctx, d.WebhookURL)
		}
		n.recordFailure(ctx, d, now, true, fmt.Sprintf("server error: status %d", statusCode))
		return
	}
	if statusCode >= 400 {
		// 4xx: client error, not retryable — go straight to dead.
		n.recordFailure(ctx, d, now, false, fmt.Sprintf("client error: status %d", statusCode))
		return
	}

	// Success.
	if n.circuitBreaker != nil {
		n.circuitBreaker.RecordSuccess(ctx, d.WebhookURL)
	}
	d.Status = domain.WebhookStatusDelivered
	d.DeliveredAt = &now
	d.LastError = ""
	if err := n.store.UpdateWebhookDelivery(ctx, d); err != nil {
		n.logger.Error("failed to mark webhook delivered", "delivery_id", d.ID, "error", err)
		return
	}
	if d.EventTriggerID != "" {
		_ = n.store.UpdateEventTriggerNotifyStatus(ctx, d.EventTriggerID, "sent")
	}
	n.logger.Info("webhook delivered", "delivery_id", d.ID, "url", d.WebhookURL, "attempt", d.Attempts)
}

// recordFailure handles a failed delivery attempt. For retryable errors, schedules
// the next attempt with exponential backoff. For non-retryable errors or exhausted
// attempts, marks the delivery as dead (DLQ).
func (n *DeliveryWorker) recordFailure(ctx context.Context, d *domain.WebhookDelivery, now time.Time, retryable bool, errMsg string) {
	d.LastError = errMsg

	// Non-retryable or exhausted → dead letter.
	if !retryable || d.Attempts >= d.MaxAttempts {
		d.Status = domain.WebhookStatusDead
		if err := n.store.UpdateWebhookDelivery(ctx, d); err != nil {
			n.logger.Error("failed to dead-letter webhook", "delivery_id", d.ID, "error", err)
		}
		if d.EventTriggerID != "" {
			_ = n.store.UpdateEventTriggerNotifyStatus(ctx, d.EventTriggerID, "failed")
		}
		n.logger.Error("webhook delivery dead-lettered",
			"delivery_id", d.ID, "url", d.WebhookURL,
			"attempts", d.Attempts, "max_attempts", d.MaxAttempts, "error", errMsg)
		return
	}

	backoff := backoffForRetryPolicy(n.retryPolicyForDelivery(d), d.Attempts)
	nextAttempt := now.Add(backoff)
	d.NextRetryAt = &nextAttempt
	d.Status = domain.WebhookStatusPending

	if err := n.store.UpdateWebhookDelivery(ctx, d); err != nil {
		n.logger.Error("failed to schedule webhook retry", "delivery_id", d.ID, "error", err)
	}

	n.logger.Warn("webhook delivery failed, scheduled retry",
		"delivery_id", d.ID, "url", d.WebhookURL,
		"attempt", d.Attempts, "max_attempts", d.MaxAttempts,
		"next_attempt", nextAttempt, "error", errMsg)
}

// pow computes base^exp for small positive integers.
func pow(base, exp int) int {
	result := 1
	for range exp {
		result *= base
	}
	return result
}

func (n *DeliveryWorker) retryPolicyForDelivery(d *domain.WebhookDelivery) string {
	switch d.RetryPolicy {
	case domain.WebhookRetryPolicyExponential, domain.WebhookRetryPolicyLinear, domain.WebhookRetryPolicyFixed:
		return d.RetryPolicy
	default:
		return n.defaultRetryPolicy
	}
}

func backoffForRetryPolicy(policy string, attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}

	var backoff time.Duration
	switch policy {
	case domain.WebhookRetryPolicyLinear:
		backoff = time.Duration(5*attempts) * time.Second
	case domain.WebhookRetryPolicyFixed:
		backoff = 5 * time.Second
	default:
		backoff = time.Duration(pow(5, attempts)) * time.Second
	}

	if backoff > 30*time.Minute {
		return 30 * time.Minute
	}

	return backoff
}
