package scheduler

import (
	"context"
	"log/slog"
	"time"

	"strait/internal/billing"
)

// Ensure *billing.PgStore satisfies QuotaResumeEnforcerStore.
var _ QuotaResumeEnforcerStore = (*billing.PgStore)(nil)

// QuotaResumeEnforcerStore defines the store operations needed by the quota resume enforcer.
type QuotaResumeEnforcerStore interface {
	ListAllSubscribedOrgIDs(ctx context.Context) ([]string, error)
	GetOrgSubscription(ctx context.Context, orgID string) (*billing.OrgSubscription, error)
	UnpauseJobsByPauseReason(ctx context.Context, orgID, reason string) (int64, error)
}

// Advisory lock ID for the quota resume enforcer (arbitrary unique constant).
const quotaResumeEnforcerLockID int64 = 900_100_007

// QuotaResumeEnforcer periodically checks whether orgs that had their jobs
// paused due to quota exhaustion have entered a new billing period. When the
// billing period boundary is crossed, the enforcer resumes (unpauses) the
// jobs that were paused with reason "quota_exceeded".
//
// This enforcer is additive-only: it never pauses jobs. Pausing is performed
// by billing.Enforcer.PauseJobsForQuotaExceeded on the hot dispatch path.
type QuotaResumeEnforcer struct {
	store          QuotaResumeEnforcerStore
	enforcer       *billing.Enforcer
	advisoryLocker AdvisoryLocker
	interval       time.Duration
}

// NewQuotaResumeEnforcer creates a new quota resume enforcer.
func NewQuotaResumeEnforcer(store QuotaResumeEnforcerStore, enforcer *billing.Enforcer, interval time.Duration) *QuotaResumeEnforcer {
	return &QuotaResumeEnforcer{
		store:    store,
		enforcer: enforcer,
		interval: interval,
	}
}

// WithAdvisoryLocker enables distributed single-leader enforcement.
func (q *QuotaResumeEnforcer) WithAdvisoryLocker(locker AdvisoryLocker) *QuotaResumeEnforcer {
	q.advisoryLocker = locker
	return q
}

// Run starts the periodic quota resume enforcement loop.
func (q *QuotaResumeEnforcer) Run(ctx context.Context) {
	ticker := time.NewTicker(q.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			q.enforce(context.WithoutCancel(ctx))
		}
	}
}

func (q *QuotaResumeEnforcer) enforce(ctx context.Context) {
	if q.advisoryLocker != nil {
		acquired, err := q.advisoryLocker.TryAdvisoryLock(ctx, quotaResumeEnforcerLockID)
		if err != nil {
			slog.Warn("quota resume enforcer: failed to acquire advisory lock", "error", err)
			return
		}
		if !acquired {
			return
		}
		defer func() {
			if relErr := q.advisoryLocker.ReleaseAdvisoryLock(ctx, quotaResumeEnforcerLockID); relErr != nil {
				slog.Warn("quota resume enforcer: failed to release advisory lock", "error", relErr)
			}
		}()
	}

	orgIDs, err := q.store.ListAllSubscribedOrgIDs(ctx)
	if err != nil {
		slog.Warn("quota resume enforcer: failed to list subscribed orgs", "error", err)
		return
	}

	now := time.Now().UTC()

	for _, orgID := range orgIDs {
		sub, err := q.store.GetOrgSubscription(ctx, orgID)
		if err != nil {
			slog.Warn("quota resume enforcer: failed to get subscription",
				"org_id", orgID, "error", err)
			continue
		}

		// Only attempt to resume when a billing period boundary has been
		// crossed. For free-tier orgs the period resets on the calendar month;
		// for paid plans the Stripe period anchors are used.
		if !q.isNewBillingPeriod(now, sub) {
			continue
		}

		resumed, err := q.store.UnpauseJobsByPauseReason(ctx, orgID, "quota_exceeded")
		if err != nil {
			slog.Warn("quota resume enforcer: failed to unpause jobs",
				"org_id", orgID, "error", err)
			continue
		}
		if resumed == 0 {
			continue
		}

		// Invalidate the enforcer cache so the next run check picks up the
		// refreshed plan limits for the new period.
		if q.enforcer != nil {
			q.enforcer.InvalidateOrgCache(orgID)
		}

		slog.Info("quota resume enforcer: resumed paused jobs at billing period boundary",
			"org_id", orgID,
			"jobs_resumed", resumed,
		)
	}
}

// isNewBillingPeriod returns true when the current time is at or past the
// subscription's billing period boundary. For free-tier orgs (no period end
// set) it resets on the first day of each calendar month; for paid plans it
// uses the current_period_end anchor from Stripe.
func (q *QuotaResumeEnforcer) isNewBillingPeriod(now time.Time, sub *billing.OrgSubscription) bool {
	if sub.CurrentPeriodEnd != nil {
		// New billing period: current time is past the stored period end.
		return now.After(*sub.CurrentPeriodEnd)
	}
	// Free tier: period resets at the start of each calendar month.
	// We treat "now is the 1st of any month" as the reset boundary.
	return now.Day() == 1
}
