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

		// Suspend excess projects if the new plan has a lower project limit.
		if sub.PendingPlanTier != nil {
			newLimits := billing.GetPlanLimits(domain.PlanTier(*sub.PendingPlanTier))
			if newLimits.MaxProjectsPerOrg != -1 {
				suspended, suspendErr := d.store.SuspendExcessProjects(ctx, sub.OrgID, newLimits.MaxProjectsPerOrg)
				if suspendErr != nil {
					slog.Warn("failed to suspend excess projects after downgrade",
						"org_id", sub.OrgID,
						"max_projects", newLimits.MaxProjectsPerOrg,
						"error", suspendErr,
					)
				} else if suspended > 0 {
					slog.Info("suspended excess projects after plan downgrade",
						"org_id", sub.OrgID,
						"suspended_count", suspended,
						"max_projects", newLimits.MaxProjectsPerOrg,
					)
				}
			}
		}

		slog.Info("applied pending downgrade",
			"org_id", sub.OrgID,
			"pending_tier", sub.PendingPlanTier,
		)
	}
}
