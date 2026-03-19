package scheduler

import (
	"context"
	"log/slog"
	"time"

	"strait/internal/billing"
)

// DowngradeApplierStore defines the store operations needed by the downgrade applier.
type DowngradeApplierStore interface {
	ListOrgsWithPendingDowngrade(ctx context.Context) ([]billing.OrgSubscription, error)
	ApplyPendingDowngrade(ctx context.Context, orgID string) error
}

// DowngradeApplier periodically applies pending plan downgrades whose billing
// period has ended.
type DowngradeApplier struct {
	store    DowngradeApplierStore
	enforcer *billing.Enforcer
	interval time.Duration
}

// NewDowngradeApplier creates a new downgrade applier.
func NewDowngradeApplier(store DowngradeApplierStore, enforcer *billing.Enforcer, interval time.Duration) *DowngradeApplier {
	return &DowngradeApplier{
		store:    store,
		enforcer: enforcer,
		interval: interval,
	}
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
			d.apply(ctx)
		}
	}
}

func (d *DowngradeApplier) apply(ctx context.Context) {
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

		slog.Info("applied pending downgrade",
			"org_id", sub.OrgID,
			"pending_tier", sub.PendingPlanTier,
		)
	}
}
