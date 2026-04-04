package scheduler

import (
	"context"
	"log/slog"
	"time"

	"strait/internal/billing"
)

// StaleSubscriptionStore defines the store operations needed by the stale subscription checker.
type StaleSubscriptionStore interface {
	ListStaleSubscriptions(ctx context.Context) ([]billing.OrgSubscription, error)
}

// Advisory lock ID for the stale subscription checker (arbitrary unique constant).
const staleSubscriptionCheckerLockID int64 = 900_100_005

// StaleSubscriptionChecker periodically queries for subscriptions that appear
// stale -- active status but current_period_end has passed by more than 1 day
// with no pending downgrade. These may indicate missed cancellation webhooks
// from Stripe. The checker logs warnings for manual investigation rather than
// taking automated action.
type StaleSubscriptionChecker struct {
	store          StaleSubscriptionStore
	advisoryLocker AdvisoryLocker
	interval       time.Duration
}

// NewStaleSubscriptionChecker creates a new stale subscription checker.
func NewStaleSubscriptionChecker(store StaleSubscriptionStore, interval time.Duration) *StaleSubscriptionChecker {
	return &StaleSubscriptionChecker{
		store:    store,
		interval: interval,
	}
}

// WithAdvisoryLocker enables distributed single-leader checking.
func (c *StaleSubscriptionChecker) WithAdvisoryLocker(locker AdvisoryLocker) *StaleSubscriptionChecker {
	c.advisoryLocker = locker
	return c
}

// Run starts the periodic stale subscription check loop.
func (c *StaleSubscriptionChecker) Run(ctx context.Context) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.check(context.WithoutCancel(ctx))
		}
	}
}

func (c *StaleSubscriptionChecker) check(ctx context.Context) {
	if c.advisoryLocker != nil {
		acquired, err := c.advisoryLocker.TryAdvisoryLock(ctx, staleSubscriptionCheckerLockID)
		if err != nil {
			slog.Warn("stale subscription checker: failed to acquire advisory lock", "error", err)
			return
		}
		if !acquired {
			return
		}
		defer func() {
			if relErr := c.advisoryLocker.ReleaseAdvisoryLock(ctx, staleSubscriptionCheckerLockID); relErr != nil {
				slog.Warn("stale subscription checker: failed to release advisory lock", "error", relErr)
			}
		}()
	}

	subs, err := c.store.ListStaleSubscriptions(ctx)
	if err != nil {
		slog.Warn("failed to list stale subscriptions", "error", err)
		return
	}

	for _, sub := range subs {
		daysPastEnd := time.Since(*sub.CurrentPeriodEnd).Hours() / 24
		slog.Warn("stale subscription detected: active subscription past period end",
			"org_id", sub.OrgID,
			"plan_tier", sub.PlanTier,
			"stripe_subscription_id", sub.StripeSubscriptionID,
			"current_period_end", sub.CurrentPeriodEnd,
			"days_past_end", int(daysPastEnd),
		)
	}

	if len(subs) > 0 {
		slog.Info("stale subscription check complete",
			"stale_count", len(subs),
		)
	}
}
