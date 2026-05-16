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
	RestrictExpiredGracePeriod(ctx context.Context, orgID string, graceEnd *time.Time) (bool, error)
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
			runSchedulerCycleCheckIn(ctx, g.interval, func() {
				g.enforce(context.WithoutCancel(ctx))
			})
		}
	}
}

func (g *GracePeriodEnforcer) enforce(ctx context.Context) {
	_, err := runWithOptionalAdvisoryLock(ctx, g.advisoryLocker, gracePeriodEnforcerLockID, g.enforceLocked)
	if err != nil {
		slog.Warn("grace period enforcer: advisory lock cycle failed", "error", err)
		return
	}
}

func (g *GracePeriodEnforcer) enforceLocked(ctx context.Context) error {
	subs, err := g.store.ListOrgsInGracePeriod(ctx)
	if err != nil {
		slog.Warn("failed to list orgs past grace period", "error", err)
		return nil
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

		restricted, err := g.store.RestrictExpiredGracePeriod(ctx, sub.OrgID, sub.GracePeriodEnd)
		if err != nil {
			slog.Warn("failed to atomically restrict org past grace period",
				"org_id", sub.OrgID,
				"error", err,
			)
			continue
		}
		if !restricted {
			slog.Info("grace period enforcer: org no longer eligible for restriction", "org_id", sub.OrgID)
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
	return nil
}
