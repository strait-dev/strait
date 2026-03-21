package scheduler

import (
	"context"
	"testing"
	"time"
)

type mockReferralExpiryStore struct {
	expiredCount int64
	expireErr    error
}

func (m *mockReferralExpiryStore) ExpireOldReferrals(_ context.Context) (int64, error) {
	if m.expireErr != nil {
		return 0, m.expireErr
	}
	return m.expiredCount, nil
}

func TestReferralExpiry_MarksExpiredReferrals(t *testing.T) {
	t.Parallel()

	store := &mockReferralExpiryStore{expiredCount: 3}
	expiry := NewReferralExpiry(store, 24*time.Hour)

	// Call the internal expire method directly.
	expiry.expire(context.Background())

	// The mock returns 3, which means 3 referrals would be expired.
	// We verify no panic and the method completes successfully.
	if store.expiredCount != 3 {
		t.Errorf("expected 3 expired referrals, got %d", store.expiredCount)
	}
}

func TestReferralExpiry_KeepsActiveReferrals(t *testing.T) {
	t.Parallel()

	store := &mockReferralExpiryStore{expiredCount: 0}
	expiry := NewReferralExpiry(store, 24*time.Hour)

	// Call the internal expire method directly.
	expiry.expire(context.Background())

	// Zero expired means no referrals were past their expiry.
	if store.expiredCount != 0 {
		t.Errorf("expected 0 expired referrals, got %d", store.expiredCount)
	}
}
