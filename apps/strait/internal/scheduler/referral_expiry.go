package scheduler

import (
	"context"
	"log/slog"
	"time"
)

// ReferralExpiryStore defines the store operations needed by the referral expiry job.
type ReferralExpiryStore interface {
	ExpireOldReferrals(ctx context.Context) (int64, error)
}

// ReferralExpiry periodically marks referrals past their expires_at as expired.
type ReferralExpiry struct {
	store    ReferralExpiryStore
	interval time.Duration
}

// NewReferralExpiry creates a new referral expiry scheduler job.
func NewReferralExpiry(store ReferralExpiryStore, interval time.Duration) *ReferralExpiry {
	return &ReferralExpiry{
		store:    store,
		interval: interval,
	}
}

// Run starts the periodic referral expiry loop.
func (r *ReferralExpiry) Run(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.expire(ctx)
		}
	}
}

func (r *ReferralExpiry) expire(ctx context.Context) {
	expired, err := r.store.ExpireOldReferrals(ctx)
	if err != nil {
		slog.Warn("failed to expire old referrals", "error", err)
		return
	}
	if expired > 0 {
		slog.Info("expired old referrals", "count", expired)
	}
}
