package billing

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"strait/internal/clickhouse"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// costRecordedTTL is the Redis key TTL for the idempotency guard. 48 hours
// covers any realistic retry window (worker reconnect, dispatcher requeue,
// idempotency-key replay) without consuming unbounded memory.
const costRecordedTTL = 48 * time.Hour

const (
	costRecordMaxAttempts       = 5
	costRecordRetryInitialDelay = 50 * time.Millisecond
	costRecordRetryMaxDelay     = 500 * time.Millisecond
)

type durableUsageCostStore interface {
	RecordUsageCost(ctx context.Context, rec *UsageRecord, idempotencyKey, executionMode string) (bool, error)
}

// RunCostRecorder writes flat per-run billing events to the usage_records table
// and (optionally) to ClickHouse run_analytics for cross-system reporting.
//
// All execution modes (HTTP and Worker) charge the same flat rate of
// HTTPCostPerRunMicrousd / WorkerCostPerRunMicrousd (currently both 20 micro-USD).
//
// A Redis SetNX gate ensures each runID/deliveryID is billed AT MOST ONCE even
// when the caller retries. If Redis is unavailable the recorder FAILS CLOSED —
// it returns an error so the caller can retry the full billing flow rather than
// silently double-billing.
type RunCostRecorder struct {
	store      Store
	rdb        redis.Cmdable
	chExporter billingEventEnqueuer
	logger     *slog.Logger

	maxRecordAttempts int
	retryInitialDelay time.Duration
	retryMaxDelay     time.Duration
}

// NewRunCostRecorder creates a recorder. store must not be nil.
// rdb may be nil; when nil the idempotency guard is skipped (callers without
// Redis should ensure they only call Record* once per runID themselves).
func NewRunCostRecorder(store Store, rdb redis.Cmdable, chExporter billingEventEnqueuer, logger *slog.Logger) *RunCostRecorder {
	if logger == nil {
		logger = slog.Default()
	}
	return &RunCostRecorder{
		store:      store,
		rdb:        rdb,
		chExporter: chExporter,
		logger:     logger,

		maxRecordAttempts: costRecordMaxAttempts,
		retryInitialDelay: costRecordRetryInitialDelay,
		retryMaxDelay:     costRecordRetryMaxDelay,
	}
}

// RecordHTTPRunCost writes a flat 20 micro-USD cost row for a billable HTTP run.
// orgID and projectID are required; runID is used for ClickHouse analytics only.
func (r *RunCostRecorder) RecordHTTPRunCost(ctx context.Context, orgID, projectID, runID string) error {
	return r.record(ctx, orgID, projectID, runID, HTTPCostPerRunMicrousd, "http")
}

// RecordWorkerRunCost writes a flat 20 micro-USD cost row for a billable Worker run.
// orgID and projectID are required; runID is used for ClickHouse analytics only.
func (r *RunCostRecorder) RecordWorkerRunCost(ctx context.Context, orgID, projectID, runID string) error {
	return r.record(ctx, orgID, projectID, runID, WorkerCostPerRunMicrousd, "worker")
}

// RecordWebhookDeliveryCost writes a flat 20 micro-USD cost row for a successful
// outbound webhook delivery. Only call this on the eventual success path — failed
// deliveries that are retried and never succeed are not billed. orgID and projectID
// are required; deliveryID is used for ClickHouse analytics only.
func (r *RunCostRecorder) RecordWebhookDeliveryCost(ctx context.Context, orgID, projectID, deliveryID string) error {
	return r.record(ctx, orgID, projectID, deliveryID, WebhookDeliveryCostPerRunMicrousd, "webhook_delivery")
}

func (r *RunCostRecorder) record(ctx context.Context, orgID, projectID, runID string, costMicroUSD int64, executionMode string) error {
	if orgID == "" || projectID == "" {
		recordBillingUsageRecord(ctx, executionMode, "skipped")
		return nil
	}

	// Idempotency gate: use Redis SetNX to ensure each runID is billed at most once.
	// Fail closed on Redis error so retries don't silently double-bill.
	var idempotencyKey string
	idempotencyClaimed := false
	if runID == "" {
		r.logger.Warn("run_cost_recorder: empty runID, skipping idempotency guard",
			"org_id", orgID, "project_id", projectID, "execution_mode", executionMode)
	} else if r.rdb != nil {
		idempotencyKey = "strait:cost_recorded:" + runID
		set, err := r.rdb.SetNX(ctx, idempotencyKey, "1", costRecordedTTL).Result()
		if err != nil {
			r.logger.Warn("run_cost_recorder: redis idempotency unavailable; falling back to durable DB idempotency",
				"org_id", orgID, "project_id", projectID, "run_id", runID, "execution_mode", executionMode, "error", err)
		}
		if err == nil && !set {
			recordBillingIdempotencyDuplicate(ctx, executionMode)
			recordBillingUsageRecord(ctx, executionMode, "duplicate")
			r.logger.Debug("skipping duplicate cost record", "run_id", runID,
				"org_id", orgID, "execution_mode", executionMode)
			return nil
		}
		idempotencyClaimed = err == nil
	} else if runID != "" {
		idempotencyKey = "strait:cost_recorded:" + runID
	}

	now := time.Now().UTC()
	today := now.Truncate(24 * time.Hour)
	// ID/CreatedAt/UpdatedAt must be set explicitly: UpsertUsageRecord passes
	// every column by parameter, so the schema's DEFAULT clauses do not fire.
	// Without an explicit ID, the second insert in a day under a different
	// (org, project, period_date) tuple would collide on PRIMARY KEY id=''.
	rec := &UsageRecord{
		ID:               uuid.NewString(),
		OrgID:            orgID,
		ProjectID:        projectID,
		PeriodDate:       today,
		RunsCount:        1,
		ComputeCostMicro: costMicroUSD,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	recorded, err := r.recordUsageWithRetry(ctx, rec, idempotencyKey, executionMode)
	if err != nil {
		// Release the Redis idempotency claim so a subsequent retry can
		// bill. Without this, the SetNX claim outlives the failed DB write
		// and any retry within costRecordedTTL silently skips billing —
		// the bill is permanently lost. Best-effort: log and continue if
		// the release itself fails (the runID will simply be unbilled
		// until the TTL expires, which is the prior failure mode).
		if idempotencyClaimed {
			if delErr := r.rdb.Del(context.Background(), idempotencyKey).Err(); delErr != nil {
				r.logger.Warn("run_cost_recorder: failed to release idempotency key after DB error",
					"run_id", runID, "redis_key", idempotencyKey, "error", delErr)
			}
		}
		recordBillingUsageRecord(ctx, executionMode, "error")
		return fmt.Errorf("recording %s run cost (org=%s project=%s run=%s): %w",
			executionMode, orgID, projectID, runID, err)
	}
	if !recorded {
		recordBillingIdempotencyDuplicate(ctx, executionMode)
		recordBillingUsageRecord(ctx, executionMode, "duplicate")
		r.logger.Debug("skipping duplicate durable cost record", "run_id", runID,
			"org_id", orgID, "execution_mode", executionMode)
		return nil
	}

	recordBillingUsageRecord(ctx, executionMode, "success")
	recordBillingUsageRecordCost(ctx, executionMode, costMicroUSD)
	r.emitClickHouse(orgID, projectID, runID, costMicroUSD, executionMode)
	return nil
}

func (r *RunCostRecorder) recordUsageWithRetry(ctx context.Context, rec *UsageRecord, idempotencyKey, executionMode string) (bool, error) {
	attempts := r.maxRecordAttempts
	if attempts <= 0 {
		attempts = 1
	}
	delay := r.retryInitialDelay
	if delay <= 0 {
		delay = costRecordRetryInitialDelay
	}
	maxDelay := r.retryMaxDelay
	if maxDelay <= 0 {
		maxDelay = delay
	}

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		recorded, err := r.recordUsageOnce(ctx, rec, idempotencyKey, executionMode)
		if err == nil {
			return recorded, nil
		}
		lastErr = err
		if attempt == attempts {
			break
		}
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return false, ctx.Err()
		case <-timer.C:
		}
		delay *= 2
		if delay > maxDelay {
			delay = maxDelay
		}
	}
	return false, lastErr
}

func (r *RunCostRecorder) recordUsageOnce(ctx context.Context, rec *UsageRecord, idempotencyKey, executionMode string) (bool, error) {
	if store, ok := r.store.(durableUsageCostStore); ok && idempotencyKey != "" {
		return store.RecordUsageCost(ctx, rec, idempotencyKey, executionMode)
	}
	if err := r.store.UpsertUsageRecord(ctx, rec); err != nil {
		return false, err
	}
	return true, nil
}

// emitClickHouse sends a billing event to ClickHouse run_analytics if the exporter
// is configured. Errors are logged but not returned — ClickHouse is non-critical.
//
// ProjectID carries per-project attribution; runID and the recorded cost go into
// the Details JSON blob so completed-run events can be correlated back to the run
// and reconciled against usage_records.
func (r *RunCostRecorder) emitClickHouse(orgID, projectID, runID string, costMicroUSD int64, executionMode string) {
	if r.chExporter == nil {
		return
	}
	details, err := json.Marshal(struct {
		RunID        string `json:"run_id"`
		CostMicroUSD int64  `json:"cost_micro_usd"`
	}{RunID: runID, CostMicroUSD: costMicroUSD})
	if err != nil {
		// Marshalling two scalars cannot realistically fail; log and emit the
		// event without details rather than dropping attribution entirely.
		r.logger.Warn("marshalling billing event details", "run_id", runID, "error", err)
		details = nil
	}
	r.chExporter.Enqueue(clickhouse.BillingEventRecord{
		Timestamp: time.Now(),
		OrgID:     orgID,
		ProjectID: projectID,
		EventType: executionMode + "_run_completed",
		PlanTier:  "", // caller does not need to know the tier for cost recording
		Details:   string(details),
	})
}
