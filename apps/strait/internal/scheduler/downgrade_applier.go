package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"strait/internal/billing"
	"strait/internal/domain"
)

// DowngradeApplierStore defines the store operations needed by the downgrade applier.
type DowngradeApplierStore interface {
	planResourceLimitStore
	ListOrgsWithPendingDowngrade(ctx context.Context) ([]billing.OrgSubscription, error)
	ApplyPendingDowngradeTierIfPending(ctx context.Context, orgID, pendingTier string) (bool, error)
	ClearPendingPlanTierIfTier(ctx context.Context, orgID, pendingTier string) (bool, error)
}

type planResourceLimitStore interface {
	SuspendExcessProjects(ctx context.Context, orgID string, maxProjects int) (int, error)
	DeactivateExcessCronJobs(ctx context.Context, orgID string, maxSchedules int) ([]string, error)
	DeactivateExcessWebhookSubscriptions(ctx context.Context, orgID string, maxEndpoints int) (int64, error)
	DeactivateExcessEnvironments(ctx context.Context, orgID string, maxEnvironments int) (int64, error)
	DeactivateExcessLogDrains(ctx context.Context, orgID string, maxDrains int) (int64, error)
	DeactivateExcessNotificationChannelsByProject(ctx context.Context, projectID string, maxChannels int) (int64, error)
	ListProjectsByOrg(ctx context.Context, orgID string) ([]string, error)
	PauseHTTPJobsByOrg(ctx context.Context, orgID, reason string) ([]string, error)
	CountMembersByOrg(ctx context.Context, orgID string) (int, error)
}

// Advisory lock ID for the downgrade applier (arbitrary unique constant).
const downgradeApplierLockID int64 = 900_100_004

// DowngradeApplier periodically applies pending plan downgrades whose billing
// period has ended.
type DowngradeApplier struct {
	store             DowngradeApplierStore
	enforcer          *billing.Enforcer
	advisoryLocker    AdvisoryLocker
	billingDispatcher billing.BillingEventDispatcher
	interval          time.Duration
}

// NewDowngradeApplier creates a new downgrade applier.
func NewDowngradeApplier(store DowngradeApplierStore, enforcer *billing.Enforcer, interval time.Duration) *DowngradeApplier {
	return &DowngradeApplier{
		store:    store,
		enforcer: enforcer,
		interval: interval,
	}
}

// WithAdvisoryLocker enables distributed single-leader downgrade application.
func (d *DowngradeApplier) WithAdvisoryLocker(locker AdvisoryLocker) *DowngradeApplier {
	d.advisoryLocker = locker
	return d
}

// WithBillingDispatcher enables schedule.suspended webhook dispatches when
// jobs are auto-paused or have their cron cleared as part of a downgrade.
func (d *DowngradeApplier) WithBillingDispatcher(dispatcher billing.BillingEventDispatcher) *DowngradeApplier {
	d.billingDispatcher = dispatcher
	return d
}

// Run starts the periodic downgrade application loop.
func (d *DowngradeApplier) Run(ctx context.Context) {
	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runSchedulerCycleCheckIn(ctx, d.interval, func() {
				d.apply(context.WithoutCancel(ctx))
			})
		}
	}
}

func (d *DowngradeApplier) apply(ctx context.Context) {
	_, err := runWithOptionalAdvisoryLock(ctx, d.advisoryLocker, downgradeApplierLockID, d.applyLocked)
	if err != nil {
		slog.Warn("downgrade applier: advisory lock cycle failed", "error", err)
		return
	}
}

func (d *DowngradeApplier) applyLocked(ctx context.Context) error {
	subs, err := d.store.ListOrgsWithPendingDowngrade(ctx)
	if err != nil {
		slog.Warn("failed to list orgs with pending downgrade", "error", err)
		return nil
	}

	for _, sub := range subs {
		if sub.PendingPlanTier == nil {
			slog.Warn("skipping pending downgrade without pending tier", "org_id", sub.OrgID)
			continue
		}

		applied, err := d.store.ApplyPendingDowngradeTierIfPending(ctx, sub.OrgID, *sub.PendingPlanTier)
		if err != nil {
			slog.Warn("failed to apply pending downgrade",
				"org_id", sub.OrgID,
				"pending_tier", sub.PendingPlanTier,
				"error", err,
			)
			continue
		}
		if !applied {
			slog.Warn("pending downgrade changed before apply",
				"org_id", sub.OrgID,
				"pending_tier", sub.PendingPlanTier,
			)
			continue
		}

		if d.enforcer != nil {
			d.enforcer.InvalidateOrgCache(sub.OrgID)
		}

		if err := d.enforceDowngradeLimits(ctx, sub.OrgID, *sub.PendingPlanTier); err != nil {
			slog.Warn("failed to enforce pending downgrade limits after tier transition",
				"org_id", sub.OrgID,
				"pending_tier", sub.PendingPlanTier,
				"error", err,
			)
			continue
		}

		cleared, err := d.store.ClearPendingPlanTierIfTier(ctx, sub.OrgID, *sub.PendingPlanTier)
		if err != nil {
			slog.Warn("failed to clear enforced pending downgrade",
				"org_id", sub.OrgID,
				"pending_tier", sub.PendingPlanTier,
				"error", err,
			)
			continue
		}
		if !cleared {
			slog.Warn("pending downgrade changed before clear",
				"org_id", sub.OrgID,
				"pending_tier", sub.PendingPlanTier,
			)
			continue
		}

		if d.enforcer != nil {
			d.enforcer.InvalidateOrgCache(sub.OrgID)
		}

		slog.Info("applied pending downgrade",
			"org_id", sub.OrgID,
			"pending_tier", sub.PendingPlanTier,
		)
	}
	return nil
}

// enforceDowngradeLimits deactivates excess resources that exceed the new plan's limits.
func (d *DowngradeApplier) enforceDowngradeLimits(ctx context.Context, orgID, pendingTier string) error {
	return enforcePlanResourceLimits(ctx, d.store, d.enforcer, d.billingDispatcher, orgID, pendingTier)
}

func enforcePlanResourceLimits(
	ctx context.Context,
	limitStore planResourceLimitStore,
	enforcer *billing.Enforcer,
	billingDispatcher billing.BillingEventDispatcher,
	orgID string,
	pendingTier string,
) error {
	newLimits := billing.GetPlanLimits(domain.PlanTier(pendingTier))
	return enforceResolvedPlanResourceLimits(ctx, limitStore, enforcer, billingDispatcher, orgID, pendingTier, newLimits)
}

func enforceResolvedPlanResourceLimits(
	ctx context.Context,
	limitStore planResourceLimitStore,
	enforcer *billing.Enforcer,
	billingDispatcher billing.BillingEventDispatcher,
	orgID string,
	pendingTier string,
	newLimits billing.OrgPlanLimits,
) error {
	var errs []error

	if newLimits.MaxProjectsPerOrg != -1 {
		if n, err := limitStore.SuspendExcessProjects(ctx, orgID, newLimits.MaxProjectsPerOrg); err != nil {
			slog.Warn("failed to suspend excess projects", "org_id", orgID, "error", err)
			errs = append(errs, fmt.Errorf("suspend excess projects: %w", err))
		} else if n > 0 {
			slog.Info("suspended excess projects after downgrade", "org_id", orgID, "count", n)
		}
	}

	// Flush suspended cache so enforcement picks up the new suspension state immediately.
	if enforcer != nil {
		projectIDs, _ := limitStore.ListProjectsByOrg(ctx, orgID)
		enforcer.FlushSuspendedCacheForOrg(projectIDs)
	}

	if newLimits.MaxScheduledJobs != -1 {
		if ids, err := limitStore.DeactivateExcessCronJobs(ctx, orgID, newLimits.MaxScheduledJobs); err != nil {
			slog.Warn("failed to deactivate excess cron jobs", "org_id", orgID, "error", err)
			errs = append(errs, fmt.Errorf("deactivate excess cron jobs: %w", err))
		} else if len(ids) > 0 {
			slog.Info("deactivated excess cron jobs after downgrade", "org_id", orgID, "count", len(ids))
			dispatchScheduleSuspended(ctx, billingDispatcher, orgID, pendingTier, ids, "plan_downgrade_cron_limit")
		}
	}

	if newLimits.MaxWebhookEndpoints != -1 {
		if n, err := limitStore.DeactivateExcessWebhookSubscriptions(ctx, orgID, newLimits.MaxWebhookEndpoints); err != nil {
			slog.Warn("failed to deactivate excess webhooks", "org_id", orgID, "error", err)
			errs = append(errs, fmt.Errorf("deactivate excess webhooks: %w", err))
		} else if n > 0 {
			slog.Info("deactivated excess webhooks after downgrade", "org_id", orgID, "count", n)
		}
	}

	if newLimits.MaxEnvironments > 0 {
		if n, err := limitStore.DeactivateExcessEnvironments(ctx, orgID, newLimits.MaxEnvironments); err != nil {
			slog.Warn("failed to deactivate excess environments", "org_id", orgID, "error", err)
			errs = append(errs, fmt.Errorf("deactivate excess environments: %w", err))
		} else if n > 0 {
			slog.Info("deactivated excess environments after downgrade", "org_id", orgID, "count", n)
		}
	}

	// Auto-pause HTTP-mode jobs when downgrading to a tier that doesn't support HTTP mode.
	if !newLimits.AllowsHTTPMode {
		if ids, err := limitStore.PauseHTTPJobsByOrg(ctx, orgID, "plan_downgrade"); err != nil {
			slog.Error("failed to pause HTTP jobs on downgrade", "org_id", orgID, "error", err)
			errs = append(errs, fmt.Errorf("pause http jobs: %w", err))
		} else if len(ids) > 0 {
			slog.Info("paused HTTP jobs on downgrade", "org_id", orgID, "count", len(ids))
			dispatchScheduleSuspended(ctx, billingDispatcher, orgID, pendingTier, ids, "plan_downgrade_http_mode")
		}
	}

	if newLimits.MaxLogDrainsPerOrg != -1 {
		if n, err := limitStore.DeactivateExcessLogDrains(ctx, orgID, newLimits.MaxLogDrainsPerOrg); err != nil {
			slog.Warn("failed to deactivate excess log drains", "org_id", orgID, "error", err)
			errs = append(errs, fmt.Errorf("deactivate excess log drains: %w", err))
		} else if n > 0 {
			slog.Info("deactivated excess log drains after downgrade", "org_id", orgID, "count", n)
		}
	}

	// Notification channels are capped per project, so iterate once per project.
	if newLimits.MaxNotificationChannels != -1 {
		projectIDs, err := limitStore.ListProjectsByOrg(ctx, orgID)
		if err != nil {
			slog.Warn("failed to list projects for notification channel cleanup", "org_id", orgID, "error", err)
		} else {
			for _, projectID := range projectIDs {
				if n, err := limitStore.DeactivateExcessNotificationChannelsByProject(ctx, projectID, newLimits.MaxNotificationChannels); err != nil {
					slog.Warn("failed to deactivate excess notification channels", "project_id", projectID, "error", err)
					errs = append(errs, fmt.Errorf("deactivate excess notification channels: %w", err))
				} else if n > 0 {
					slog.Info("deactivated excess notification channels after downgrade", "project_id", projectID, "count", n)
				}
			}
		}
	}

	// Members: do not auto-deactivate; emit a billing event so the dashboard
	// can surface the overage. New invites are blocked at the API per the
	// plan's "block new, leave existing" policy.
	if enforcer != nil && newLimits.MaxMembersPerOrg != -1 {
		count, err := limitStore.CountMembersByOrg(ctx, orgID)
		if err != nil {
			slog.Warn("failed to count members for overage signal", "org_id", orgID, "error", err)
		} else if count > newLimits.MaxMembersPerOrg {
			enforcer.EmitBillingEvent(orgID, "org_member_overage", pendingTier)
			slog.Info("emitted member overage signal after downgrade",
				"org_id", orgID,
				"member_count", count,
				"new_cap", newLimits.MaxMembersPerOrg,
			)
		}
	}
	return errors.Join(errs...)
}

func dispatchScheduleSuspended(ctx context.Context, dispatcher billing.BillingEventDispatcher, orgID, planTier string, jobIDs []string, reason string) {
	if dispatcher == nil || len(jobIDs) == 0 {
		return
	}
	tier := domain.PlanTier(planTier)
	for _, jobID := range jobIDs {
		detail := map[string]any{
			"schedule_id": jobID,
			"reason":      reason,
		}
		if err := billing.DispatchBillingWebhook(ctx, dispatcher, orgID, tier, domain.WebhookEventScheduleSuspended, detail); err != nil {
			slog.Warn("dispatch schedule.suspended failed",
				"org_id", orgID,
				"job_id", jobID,
				"reason", reason,
				"error", err,
			)
		}
	}
}
