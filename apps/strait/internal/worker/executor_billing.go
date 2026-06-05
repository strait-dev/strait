package worker

import (
	"context"
	"time"

	"strait/internal/billing"
	"strait/internal/domain"
)

func (e *Executor) enforceDispatchBilling(
	ctx context.Context,
	run *domain.JobRun,
	job *domain.Job,
) (func(), bool) {
	if e.billingEnforcer == nil {
		return nil, true
	}
	if err := e.billingEnforcer.CheckProjectSuspended(ctx, job.ProjectID); err != nil {
		e.logger.Warn("project suspended", "run_id", run.ID, "project_id", job.ProjectID, "error", err)
		e.handleSystemFailureWithJob(ctx, run, job, err.Error())
		return nil, false
	}

	orgID, err := e.billingEnforcer.GetProjectOrgID(ctx, job.ProjectID)
	if err != nil {
		e.logger.Warn("failed to resolve org for billing check", "run_id", run.ID, "error", err, "fail_open", true)
	}
	if orgID == "" {
		return nil, true
	}
	if !e.checkDispatchBillingLimits(ctx, run, job, orgID) {
		return nil, false
	}
	releaseCtx := context.WithoutCancel(ctx)
	return func() {
		e.billingEnforcer.DecrConcurrentRunCount(releaseCtx, orgID)
	}, true
}

func (e *Executor) checkDispatchBillingLimits(
	ctx context.Context,
	run *domain.JobRun,
	job *domain.Job,
	orgID string,
) bool {
	if err := e.billingEnforcer.CheckSpendingLimit(ctx, orgID); err != nil {
		e.logger.Warn("org spending limit exceeded", "run_id", run.ID, "org_id", orgID, "error", err)
		e.handleSystemFailureWithJob(ctx, run, job, err.Error())
		return false
	}
	if err := e.billingEnforcer.CheckProjectBudgetLimit(ctx, job.ProjectID); err != nil {
		e.logger.Warn("project budget limit exceeded", "run_id", run.ID, "project_id", job.ProjectID, "error", err)
		e.handleSystemFailureWithJob(ctx, run, job, err.Error())
		return false
	}
	if err := e.billingEnforcer.CheckDailyRunLimit(ctx, orgID); err != nil {
		e.logger.Warn("org daily run limit exceeded", "run_id", run.ID, "org_id", orgID, "error", err)
		e.handleSystemFailureWithJob(ctx, run, job, err.Error())
		return false
	}
	if err := e.billingEnforcer.CheckMonthlyRunLimit(ctx, orgID); err != nil {
		e.logger.Warn("org monthly run limit exceeded", "run_id", run.ID, "org_id", orgID, "error", err)
		e.billingEnforcer.DecrDailyRunCount(ctx, orgID)
		e.handleSystemFailureWithJob(ctx, run, job, err.Error())
		return false
	}
	if err := e.billingEnforcer.CheckConcurrentRunLimit(ctx, orgID); err != nil {
		e.logger.Warn("org concurrent run limit exceeded", "run_id", run.ID, "org_id", orgID, "error", err)
		e.billingEnforcer.DecrDailyRunCount(ctx, orgID)
		e.billingEnforcer.DecrMonthlyRunCount(ctx, orgID)
		e.handleSystemFailureWithJob(ctx, run, job, err.Error())
		return false
	}
	return e.checkDispatchHTTPModeAllowed(ctx, run, job, orgID)
}

func (e *Executor) checkDispatchHTTPModeAllowed(
	ctx context.Context,
	run *domain.JobRun,
	job *domain.Job,
	orgID string,
) bool {
	if job.ExecutionMode != domain.ExecutionModeHTTP && job.ExecutionMode != "" {
		return true
	}
	limits, err := e.billingEnforcer.GetOrgPlanLimits(ctx, orgID)
	if err != nil || limits.AllowsHTTPMode {
		return true
	}
	billing.RecordHTTPModeGateRejected(ctx, string(limits.PlanTier), "dispatch")
	// CheckConcurrentRunLimit already INCR'd the per-org concurrent counter on
	// the under-limit path; this early return happens before enforceDispatchBilling
	// installs the deferred DecrConcurrentRunCount, so balance it here to avoid
	// leaking the counter on every HTTP-mode-gate rejection.
	e.billingEnforcer.DecrConcurrentRunCount(ctx, orgID)
	e.billingEnforcer.DecrDailyRunCount(ctx, orgID)
	e.billingEnforcer.DecrMonthlyRunCount(ctx, orgID)
	e.handleSystemFailureWithJob(ctx, run, job, "HTTP execution mode requires the Pro plan. Upgrade at /settings/billing")
	return false
}

func (e *Executor) recordHTTPRunCost(ctx context.Context, job *domain.Job, run *domain.JobRun) {
	if job.ExecutionMode != domain.ExecutionModeHTTP && job.ExecutionMode != "" {
		return
	}
	billing.RecordHTTPModeRunCompleted(ctx)
	e.ingestStripeUsageEvent(ctx, job.ProjectID, run.ID, billing.HTTPCostPerRunMicrousd)
	e.recordRunCostRow(ctx, job.ProjectID, run.ID, "failed to record HTTP run cost", func(costCtx context.Context, orgID, projectID, runID string) error {
		return e.runCostRecorder.RecordHTTPRunCost(costCtx, orgID, projectID, runID)
	})
}

func (e *Executor) recordRunCostRow(
	ctx context.Context,
	projectID string,
	runID string,
	logMessage string,
	record func(context.Context, string, string, string) error,
) {
	if e.runCostRecorder == nil || e.billingEnforcer == nil {
		return
	}
	orgID, err := e.billingEnforcer.GetProjectOrgID(ctx, projectID)
	if err != nil || orgID == "" {
		return
	}
	// Tracked on stripeUsageWG so graceful shutdown waits for the billing row.
	costCtx := context.WithoutCancel(ctx)
	e.stripeUsageWG.Go(func() {
		if err := record(costCtx, orgID, projectID, runID); err != nil {
			e.logger.Warn(logMessage,
				"run_id", runID,
				"org_id", orgID,
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
		if err := e.stripeUsageReporter.IngestComputeUsage(ingestCtx, stripeCustomerID, runID, costMicroUSD); err != nil {
			e.logger.Warn("failed to ingest stripe usage event",
				"run_id", runID,
				"cost_microusd", costMicroUSD,
				"error", err,
			)
		}
	})
}
