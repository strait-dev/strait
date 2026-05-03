package billing

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"strait/internal/clickhouse"
)

// RunCostRecorder writes flat per-run billing events to the usage_records table
// and (optionally) to ClickHouse run_analytics for cross-system reporting.
//
// All execution modes (HTTP and Worker) charge the same flat rate of
// HTTPCostPerRunMicrousd / WorkerCostPerRunMicrousd (currently both 20 micro-USD).
type RunCostRecorder struct {
	store      Store
	chExporter billingEventEnqueuer
	logger     *slog.Logger
}

// NewRunCostRecorder creates a recorder. store must not be nil.
func NewRunCostRecorder(store Store, chExporter billingEventEnqueuer, logger *slog.Logger) *RunCostRecorder {
	if logger == nil {
		logger = slog.Default()
	}
	return &RunCostRecorder{
		store:      store,
		chExporter: chExporter,
		logger:     logger,
	}
}

// RecordHTTPRunCost writes a flat 20 micro-USD cost row for a completed HTTP dispatch.
// orgID and projectID are required; runID is used for ClickHouse analytics only.
func (r *RunCostRecorder) RecordHTTPRunCost(ctx context.Context, orgID, projectID, runID string) error {
	return r.record(ctx, orgID, projectID, runID, HTTPCostPerRunMicrousd, "http")
}

// RecordWorkerRunCost writes a flat 20 micro-USD cost row for a completed Worker dispatch.
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
		return nil
	}

	today := time.Now().UTC().Truncate(24 * time.Hour)
	rec := &UsageRecord{
		OrgID:            orgID,
		ProjectID:        projectID,
		PeriodDate:       today,
		RunsCount:        1,
		ComputeCostMicro: costMicroUSD,
	}

	if err := r.store.UpsertUsageRecord(ctx, rec); err != nil {
		return fmt.Errorf("recording %s run cost (org=%s project=%s run=%s): %w",
			executionMode, orgID, projectID, runID, err)
	}

	r.emitClickHouse(orgID, projectID, runID, costMicroUSD, executionMode)
	return nil
}

// emitClickHouse sends a billing event to ClickHouse run_analytics if the exporter
// is configured. Errors are logged but not returned — ClickHouse is non-critical.
func (r *RunCostRecorder) emitClickHouse(orgID, projectID, runID string, costMicroUSD int64, executionMode string) {
	if r.chExporter == nil {
		return
	}
	r.chExporter.Enqueue(clickhouse.BillingEventRecord{
		Timestamp: time.Now(),
		OrgID:     orgID,
		EventType: executionMode + "_run_completed",
		PlanTier:  "", // caller does not need to know the tier for cost recording
	})
	_ = projectID
	_ = runID
	_ = costMicroUSD
}
