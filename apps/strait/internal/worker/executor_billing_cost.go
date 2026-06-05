package worker

import (
	"context"
	"time"

	"strait/internal/billing"
	"strait/internal/domain"
)

func (e *Executor) recordHTTPRunCost(ctx context.Context, job *domain.Job, run *domain.JobRun) {
	if job.ExecutionMode != domain.ExecutionModeHTTP && job.ExecutionMode != "" {
		return
	}
	billing.RecordHTTPModeRunCompleted(ctx)
	e.recordTerminalRunBilling(ctx, job, run)
}

func (e *Executor) recordWorkerModeCost(ctx context.Context, run *domain.JobRun, job *domain.Job) {
	e.recordTerminalRunBilling(ctx, job, run)
}

func (e *Executor) recordTerminalRunBilling(ctx context.Context, job *domain.Job, run *domain.JobRun) {
	if job == nil || run == nil {
		return
	}
	costMicroUSD := billing.HTTPCostPerRunMicrousd
	recordCost := func(costCtx context.Context, orgID string) error {
		if e.runCostRecorder == nil {
			return nil
		}
		return e.runCostRecorder.RecordHTTPRunCost(costCtx, orgID, job.ProjectID, run.ID)
	}
	if job.ExecutionMode == domain.ExecutionModeWorker {
		costMicroUSD = billing.WorkerCostPerRunMicrousd
		recordCost = func(costCtx context.Context, orgID string) error {
			if e.runCostRecorder == nil {
				return nil
			}
			return e.runCostRecorder.RecordWorkerRunCost(costCtx, orgID, job.ProjectID, run.ID)
		}
	}

	e.ingestStripeUsageEvent(ctx, job.ProjectID, run.ID, costMicroUSD)

	if e.runCostRecorder == nil || e.billingEnforcer == nil {
		return
	}
	orgID, err := e.billingEnforcer.GetProjectOrgID(ctx, job.ProjectID)
	if err != nil || orgID == "" {
		return
	}
	costCtx := context.WithoutCancel(ctx)
	e.stripeUsageWG.Go(func() {
		if err := recordCost(costCtx, orgID); err != nil {
			e.logger.Warn("failed to record run cost",
				"run_id", run.ID,
				"org_id", orgID,
				"execution_mode", job.ExecutionMode,
				"error", err,
			)
		}
	})
}

// ingestStripeUsageEvent sends a usage event to Stripe for metered billing.
// Runs asynchronously to avoid blocking the run completion path.
// Silently skips if no Stripe usage reporter is configured (self-hosted / dev).
// costMicroUSD is the per-run cost in micro-USD; HTTP and worker modes pass
// distinct constants today, but they currently coincide at 20 micro-USD.
//
//nolint:unparam // HTTP/worker pass distinct named constants that may diverge
func (e *Executor) ingestStripeUsageEvent(ctx context.Context, projectID, runID string, costMicroUSD int64) {
	if e.stripeUsageReporter == nil || e.billingEnforcer == nil || costMicroUSD <= 0 {
		return
	}
	if !e.billingEnforcer.IsRunOverage(ctx, runID) {
		return
	}

	// Look up the org's Stripe customer ID via the billing enforcer's store.
	orgID, err := e.billingEnforcer.GetProjectOrgID(ctx, projectID)
	if err != nil || orgID == "" {
		return
	}

	stripeCustomerID, err := e.billingEnforcer.GetStripeCustomerID(ctx, orgID)
	if err != nil || stripeCustomerID == "" {
		return
	}

	// Fire-and-forget: don't block the run on Stripe API latency.
	// Uses Background() intentionally - the parent request context may be canceled
	// before the Stripe API call completes, and we still want to record the usage.
	// Tracked via stripeUsageWG for graceful shutdown.
	e.stripeUsageWG.Go(func() {
		ingestCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := e.stripeUsageReporter.IngestRunOverage(ingestCtx, stripeCustomerID, runID); err != nil {
			e.logger.Warn("failed to ingest stripe usage event",
				"run_id", runID,
				"error", err,
			)
		}
	})
}
