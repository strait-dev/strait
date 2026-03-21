package scheduler

import (
	"context"
	"log/slog"
	"time"

	"strait/internal/billing"
)

// GraceEnforcerStore defines the store operations needed by the grace period enforcer.
type GraceEnforcerStore interface {
	ListOrgsInGracePeriod(ctx context.Context) ([]billing.OrgSubscription, error)
	UpdatePaymentStatus(ctx context.Context, orgID string, status string, graceEnd *time.Time) error
	UpdateOrgSubscriptionPlan(ctx context.Context, orgID, planTier, status string) error
}

// GracePeriodEnforcer periodically checks for orgs whose payment grace period
// has expired and restricts them by downgrading to the free tier.
type GracePeriodEnforcer struct {
	store    GraceEnforcerStore
	enforcer *billing.Enforcer
	interval time.Duration
}

// NewGracePeriodEnforcer creates a new grace period enforcer.
func NewGracePeriodEnforcer(store GraceEnforcerStore, enforcer *billing.Enforcer, interval time.Duration) *GracePeriodEnforcer {
	return &GracePeriodEnforcer{
		store:    store,
		enforcer: enforcer,
		interval: interval,
	}
}

// Run starts the periodic grace period enforcement loop.
func (g *GracePeriodEnforcer) Run(ctx context.Context) {
	ticker := time.NewTicker(g.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			g.enforce(ctx)
		}
	}
}

func (g *GracePeriodEnforcer) enforce(ctx context.Context) {
	subs, err := g.store.ListOrgsInGracePeriod(ctx)
	if err != nil {
		slog.Warn("failed to list orgs past grace period", "error", err)
		return
	}

	for _, sub := range subs {
		// Skip orgs already restricted.
		if sub.PaymentStatus == "restricted" {
			continue
		}

		// Set payment status to restricted.
		if err := g.store.UpdatePaymentStatus(ctx, sub.OrgID, "restricted", sub.GracePeriodEnd); err != nil {
			slog.Warn("failed to restrict org past grace period",
				"org_id", sub.OrgID,
				"error", err,
			)
			continue
		}

		// Downgrade to free tier.
		if err := g.store.UpdateOrgSubscriptionPlan(ctx, sub.OrgID, "free", "restricted"); err != nil {
			slog.Warn("failed to downgrade org to free after grace expiry",
				"org_id", sub.OrgID,
				"error", err,
			)
			continue
		}

		if g.enforcer != nil {
			g.enforcer.InvalidateOrgCache(sub.OrgID)
		}

		slog.Info("restricted org past grace period, downgraded to free",
			"org_id", sub.OrgID,
			"previous_tier", sub.PlanTier,
		)
	}
}
