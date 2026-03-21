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
	GetOrgSubscription(ctx context.Context, orgID string) (*billing.OrgSubscription, error)
	UpdatePaymentStatus(ctx context.Context, orgID string, status string, graceEnd *time.Time) error
	UpdateOrgSubscriptionPlan(ctx context.Context, orgID, planTier, status string) error
}

// Advisory lock ID for the grace period enforcer (arbitrary unique constant).
const gracePeriodEnforcerLockID int64 = 900_100_002

// GracePeriodEnforcer periodically checks for orgs whose payment grace period
// has expired and restricts them by downgrading to the free tier.
type GracePeriodEnforcer struct {
	store          GraceEnforcerStore
	enforcer       *billing.Enforcer
	advisoryLocker AdvisoryLocker
	interval       time.Duration
}

// NewGracePeriodEnforcer creates a new grace period enforcer.
func NewGracePeriodEnforcer(store GraceEnforcerStore, enforcer *billing.Enforcer, interval time.Duration) *GracePeriodEnforcer {
	return &GracePeriodEnforcer{
		store:    store,
		enforcer: enforcer,
		interval: interval,
	}
}

// WithAdvisoryLocker enables distributed single-leader enforcement.
func (g *GracePeriodEnforcer) WithAdvisoryLocker(locker AdvisoryLocker) *GracePeriodEnforcer {
	g.advisoryLocker = locker
	return g
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
			g.enforce(context.WithoutCancel(ctx))
		}
	}
}

func (g *GracePeriodEnforcer) enforce(ctx context.Context) {
	if g.advisoryLocker != nil {
		acquired, err := g.advisoryLocker.TryAdvisoryLock(ctx, gracePeriodEnforcerLockID)
		if err != nil {
			slog.Warn("grace period enforcer: failed to acquire advisory lock", "error", err)
			return
		}
		if !acquired {
			return
		}
		defer func() {
			if relErr := g.advisoryLocker.ReleaseAdvisoryLock(ctx, gracePeriodEnforcerLockID); relErr != nil {
				slog.Warn("grace period enforcer: failed to release advisory lock", "error", relErr)
			}
		}()
	}

	subs, err := g.store.ListOrgsInGracePeriod(ctx)
	if err != nil {
		slog.Warn("failed to list orgs past grace period", "error", err)
		return
	}

	for _, sub := range subs {
		// Skip orgs already restricted.
		if sub.PaymentStatus == "restricted" {
			slog.Debug("grace period enforcer: org already restricted, skipping", "org_id", sub.OrgID)
			continue
		}

		// Re-read subscription after lock to avoid acting on stale state.
		// This prevents a race where a webhook clears the grace period
		// between our list query and this enforcement action.
		freshSub, freshErr := g.store.GetOrgSubscription(ctx, sub.OrgID)
		if freshErr != nil {
			slog.Warn("grace period enforcer: failed to re-read subscription", "org_id", sub.OrgID, "error", freshErr)
			continue
		}
		if freshSub.PaymentStatus != "grace" {
			slog.Info("grace period enforcer: org no longer in grace (resolved concurrently)", "org_id", sub.OrgID)
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
			// Reset concurrent run counter to prevent stale high counts from
			// blocking new runs under the lower free-tier limit.
			if err := g.enforcer.ReconcileConcurrentRunCount(ctx, sub.OrgID, 0); err != nil {
				slog.Warn("failed to reset concurrent counter after downgrade",
					"org_id", sub.OrgID, "error", err)
			}
		}

		slog.Info("restricted org past grace period, downgraded to free",
			"org_id", sub.OrgID,
			"previous_tier", sub.PlanTier,
		)
	}
}
