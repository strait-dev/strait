package scheduler

import (
	"context"
	"log/slog"
	"sync"
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
	UnpauseJobsByPauseReasonBefore(ctx context.Context, orgID, reason string, pausedBefore time.Time) (int64, error)
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
	resumedMu      sync.Mutex
	resumedPeriods map[string]string
}

// NewQuotaResumeEnforcer creates a new quota resume enforcer.
func NewQuotaResumeEnforcer(store QuotaResumeEnforcerStore, enforcer *billing.Enforcer, interval time.Duration) *QuotaResumeEnforcer {
	return &QuotaResumeEnforcer{
		store:          store,
		enforcer:       enforcer,
		interval:       interval,
		resumedPeriods: make(map[string]string),
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
	_, err := runWithOptionalAdvisoryLock(ctx, q.advisoryLocker, quotaResumeEnforcerLockID, q.enforceLocked)
	if err != nil {
		slog.Warn("quota resume enforcer: advisory lock cycle failed", "error", err)
		return
	}
}

func (q *QuotaResumeEnforcer) enforceLocked(ctx context.Context) error {
	orgIDs, err := q.store.ListAllSubscribedOrgIDs(ctx)
	if err != nil {
		slog.Warn("quota resume enforcer: failed to list subscribed orgs", "error", err)
		return nil
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
		periodKey, boundary, ok := q.resumePeriodKey(now, sub)
		if !ok {
			continue
		}
		if q.alreadyResumed(sub.OrgID, periodKey) {
			continue
		}

		resumed, err := q.store.UnpauseJobsByPauseReasonBefore(ctx, orgID, "quota_exceeded", boundary)
		if err != nil {
			slog.Warn("quota resume enforcer: failed to unpause jobs",
				"org_id", orgID, "error", err)
			continue
		}
		q.markResumed(sub.OrgID, periodKey)
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
	return nil
}

func (q *QuotaResumeEnforcer) resumePeriodKey(now time.Time, sub *billing.OrgSubscription) (string, time.Time, bool) {
	if sub == nil {
		return "", time.Time{}, false
	}
	if sub.CurrentPeriodEnd != nil {
		// New billing period: current time is past the stored period end.
		if !now.After(*sub.CurrentPeriodEnd) {
			return "", time.Time{}, false
		}
		boundary := sub.CurrentPeriodEnd.UTC()
		return boundary.Format(time.RFC3339Nano), boundary, true
	}
	// Free tier: period resets at the start of each calendar month.
	// We treat "now is the 1st of any month" as the reset boundary.
	if now.Day() != 1 {
		return "", time.Time{}, false
	}
	boundary := time.Date(now.UTC().Year(), now.UTC().Month(), 1, 0, 0, 0, 0, time.UTC)
	return boundary.Format("2006-01"), boundary, true
}

func (q *QuotaResumeEnforcer) alreadyResumed(orgID, periodKey string) bool {
	q.resumedMu.Lock()
	defer q.resumedMu.Unlock()
	return q.resumedPeriods[orgID] == periodKey
}

func (q *QuotaResumeEnforcer) markResumed(orgID, periodKey string) {
	q.resumedMu.Lock()
	defer q.resumedMu.Unlock()
	q.resumedPeriods[orgID] = periodKey
}
