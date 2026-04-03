package scheduler

import (
	"context"
	"log/slog"
	"time"

	"strait/internal/billing"
	"strait/internal/domain"
)

// DowngradeApplierStore defines the store operations needed by the downgrade applier.
type DowngradeApplierStore interface {
	ListOrgsWithPendingDowngrade(ctx context.Context) ([]billing.OrgSubscription, error)
	ApplyPendingDowngrade(ctx context.Context, orgID string) error
	SuspendExcessProjects(ctx context.Context, orgID string, maxProjects int) (int, error)
	DeactivateExcessCronJobs(ctx context.Context, orgID string, maxSchedules int) (int64, error)
	DeactivateExcessWebhookSubscriptions(ctx context.Context, orgID string, maxEndpoints int) (int64, error)
	DeactivateExcessEnvironments(ctx context.Context, orgID string, maxEnvironments int) (int64, error)
	ListProjectsByOrg(ctx context.Context, orgID string) ([]string, error)
	PauseHTTPJobsByOrg(ctx context.Context, orgID, reason string) (int64, error)
}

// Advisory lock ID for the downgrade applier (arbitrary unique constant).
const downgradeApplierLockID int64 = 900_100_004

// DowngradeApplier periodically applies pending plan downgrades whose billing
// period has ended.
type DowngradeApplier struct {
	store          DowngradeApplierStore
	enforcer       *billing.Enforcer
	advisoryLocker AdvisoryLocker
	interval       time.Duration
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

// Run starts the periodic downgrade application loop.
func (d *DowngradeApplier) Run(ctx context.Context) {
	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.apply(context.WithoutCancel(ctx))
		}
	}
}

func (d *DowngradeApplier) apply(ctx context.Context) {
	if d.advisoryLocker != nil {
		acquired, err := d.advisoryLocker.TryAdvisoryLock(ctx, downgradeApplierLockID)
		if err != nil {
			slog.Warn("downgrade applier: failed to acquire advisory lock", "error", err)
			return
		}
		if !acquired {
			return
		}
		defer func() {
			if relErr := d.advisoryLocker.ReleaseAdvisoryLock(ctx, downgradeApplierLockID); relErr != nil {
				slog.Warn("downgrade applier: failed to release advisory lock", "error", relErr)
			}
		}()
	}

	subs, err := d.store.ListOrgsWithPendingDowngrade(ctx)
	if err != nil {
		slog.Warn("failed to list orgs with pending downgrade", "error", err)
		return
	}

	for _, sub := range subs {
		if err := d.store.ApplyPendingDowngrade(ctx, sub.OrgID); err != nil {
			slog.Warn("failed to apply pending downgrade",
				"org_id", sub.OrgID,
				"pending_tier", sub.PendingPlanTier,
				"error", err,
			)
			continue
		}

		if d.enforcer != nil {
			d.enforcer.InvalidateOrgCache(sub.OrgID)
		}

		// Enforce resource limits for the new plan tier.
		if sub.PendingPlanTier != nil {
			d.enforceDowngradeLimits(ctx, sub.OrgID, *sub.PendingPlanTier)
		}

		slog.Info("applied pending downgrade",
			"org_id", sub.OrgID,
			"pending_tier", sub.PendingPlanTier,
		)
	}
}

// enforceDowngradeLimits deactivates excess resources that exceed the new plan's limits.
func (d *DowngradeApplier) enforceDowngradeLimits(ctx context.Context, orgID, pendingTier string) {
	newLimits := billing.GetPlanLimits(domain.PlanTier(pendingTier))

	if newLimits.MaxProjectsPerOrg != -1 {
		if n, err := d.store.SuspendExcessProjects(ctx, orgID, newLimits.MaxProjectsPerOrg); err != nil {
			slog.Warn("failed to suspend excess projects", "org_id", orgID, "error", err)
		} else if n > 0 {
			slog.Info("suspended excess projects after downgrade", "org_id", orgID, "count", n)
		}
	}

	// Flush suspended cache so enforcement picks up the new suspension state immediately.
	if d.enforcer != nil {
		projectIDs, _ := d.store.ListProjectsByOrg(ctx, orgID)
		d.enforcer.FlushSuspendedCacheForOrg(projectIDs)
	}

	if newLimits.MaxScheduledJobs != -1 {
		if n, err := d.store.DeactivateExcessCronJobs(ctx, orgID, newLimits.MaxScheduledJobs); err != nil {
			slog.Warn("failed to deactivate excess cron jobs", "org_id", orgID, "error", err)
		} else if n > 0 {
			slog.Info("deactivated excess cron jobs after downgrade", "org_id", orgID, "count", n)
		}
	}

	if newLimits.MaxWebhookEndpoints != -1 {
		if n, err := d.store.DeactivateExcessWebhookSubscriptions(ctx, orgID, newLimits.MaxWebhookEndpoints); err != nil {
			slog.Warn("failed to deactivate excess webhooks", "org_id", orgID, "error", err)
		} else if n > 0 {
			slog.Info("deactivated excess webhooks after downgrade", "org_id", orgID, "count", n)
		}
	}

	if newLimits.MaxEnvironments > 0 {
		if n, err := d.store.DeactivateExcessEnvironments(ctx, orgID, newLimits.MaxEnvironments); err != nil {
			slog.Warn("failed to deactivate excess environments", "org_id", orgID, "error", err)
		} else if n > 0 {
			slog.Info("deactivated excess environments after downgrade", "org_id", orgID, "count", n)
		}
	}

	// Auto-pause HTTP-mode jobs when downgrading to a tier that doesn't support HTTP mode.
	if !newLimits.AllowsHTTPMode {
		if n, err := d.store.PauseHTTPJobsByOrg(ctx, orgID, "plan_downgrade"); err != nil {
			slog.Error("failed to pause HTTP jobs on downgrade", "org_id", orgID, "error", err)
		} else if n > 0 {
			slog.Info("paused HTTP jobs on downgrade", "org_id", orgID, "count", n)
		}
	}
}
