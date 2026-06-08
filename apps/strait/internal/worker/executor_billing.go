package worker

import (
	"context"

	"strait/internal/billing"
	"strait/internal/domain"
)

func (e *Executor) enforceDispatchBilling(
	ctx context.Context,
	run *domain.JobRun,
	job *domain.Job,
) (func(), bool) {
	if e.billingEnforcer == nil {
		if e.edition.RequiresHTTPModeGating() {
			if e.allowsUngatedCloudDevelopment() {
				return nil, true
			}
			e.logger.Warn("billing enforcer unavailable for gated dispatch", "run_id", run.ID, "project_id", job.ProjectID)
			e.handleSystemFailureWithJob(ctx, run, job, "billing enforcement unavailable")
			return nil, false
		}
		return nil, true
	}
	if err := e.billingEnforcer.CheckProjectSuspended(ctx, job.ProjectID); err != nil {
		e.logger.Warn("project suspended", "run_id", run.ID, "project_id", job.ProjectID, "error", err)
		e.handleSystemFailureWithJob(ctx, run, job, err.Error())
		return nil, false
	}

	orgID, err := e.billingEnforcer.GetProjectOrgID(ctx, job.ProjectID)
	if err != nil {
		e.logger.Warn("failed to resolve org for billing check", "run_id", run.ID, "error", err)
		e.handleSystemFailureWithJob(ctx, run, job, "billing enforcement unavailable")
		return nil, false
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
	countedMonthlyRun := shouldCountMonthlyRun(run)
	if countedMonthlyRun {
		if err := e.billingEnforcer.CheckMonthlyRunLimitForRun(ctx, orgID, run.ID); err != nil {
			e.logger.Warn("org monthly run limit exceeded", "run_id", run.ID, "org_id", orgID, "error", err)
			e.handleSystemFailureWithJob(ctx, run, job, err.Error())
			return false
		}
	}
	if err := e.billingEnforcer.CheckConcurrentRunLimit(ctx, orgID); err != nil {
		e.logger.Warn("org concurrent run limit exceeded", "run_id", run.ID, "org_id", orgID, "error", err)
		if countedMonthlyRun {
			e.billingEnforcer.DecrMonthlyRunCount(ctx, orgID)
		}
		e.handleSystemFailureWithJob(ctx, run, job, err.Error())
		return false
	}
	return e.checkDispatchHTTPModeAllowed(ctx, run, job, orgID, countedMonthlyRun)
}

func (e *Executor) allowsUngatedCloudDevelopment() bool {
	if e.billingEnforcement || e.stripeWebhookSecret != "" {
		return false
	}
	return e.sentryEnvironment == "development" || e.sentryEnvironment == "test"
}

func shouldCountMonthlyRun(run *domain.JobRun) bool {
	if run == nil {
		return false
	}
	// Manual replays create a fresh run with attempt 1 and remain billable.
	// Automatic retries reuse the same run ID with attempt >1, so they must not
	// consume another monthly run or create another Stripe overage marker.
	return run.Attempt <= 1
}

func (e *Executor) checkDispatchHTTPModeAllowed(
	ctx context.Context,
	run *domain.JobRun,
	job *domain.Job,
	orgID string,
	countedMonthlyRun bool,
) bool {
	if job.ExecutionMode != domain.ExecutionModeHTTP && job.ExecutionMode != "" {
		return true
	}
	limits, err := e.billingEnforcer.GetOrgPlanLimits(ctx, orgID)
	if err != nil {
		if countedMonthlyRun {
			e.billingEnforcer.DecrMonthlyRunCount(ctx, orgID)
		}
		e.billingEnforcer.DecrConcurrentRunCount(ctx, orgID)
		e.handleSystemFailureWithJob(ctx, run, job, "billing enforcement unavailable")
		return false
	}
	if limits.AllowsHTTPMode {
		return true
	}
	billing.RecordHTTPModeGateRejected(ctx, string(limits.PlanTier), "dispatch")
	// CheckConcurrentRunLimit already INCR'd the per-org concurrent counter on
	// the under-limit path; this early return happens before enforceDispatchBilling
	// installs the deferred DecrConcurrentRunCount, so balance it here to avoid
	// leaking the counter on every HTTP-mode-gate rejection.
	e.billingEnforcer.DecrConcurrentRunCount(ctx, orgID)
	if countedMonthlyRun {
		e.billingEnforcer.DecrMonthlyRunCount(ctx, orgID)
	}
	e.handleSystemFailureWithJob(ctx, run, job, "HTTP execution mode is unavailable for this organization. Contact support if this persists.")
	return false
}
