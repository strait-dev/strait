package billing

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"strait/internal/domain"
)

// TestCheckSpendingLimit_RespectsContextCancellation guards against a blocking
// Redis lock-retry loop. The old path used time.Sleep, so
// a cancelled context could still wait up to 3 × 200ms. The select-based
// retry must short-circuit immediately when the caller cancels.
func TestCheckSpendingLimit_RespectsContextCancellation(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	// Hold the lock so the retry path always fires.
	const orgID = "org-cancel"
	require.NoError(t,
		mr.Set("strait:spend_check:"+
			orgID,
			"1"))

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			orgID: {
				OrgID:                 orgID,
				PlanTier:              string(domain.PlanStarter),
				SpendingLimitMicrousd: 100_000_000,
				LimitAction:           "reject",
			},
		},
		periodSpendByOrg: map[string]int64{orgID: 50_000_000},
	}
	e := NewEnforcer(store, rdb, slog.Default())

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before the call so the retry loop sees Done() immediately.

	start := time.Now()
	// The call may return an error from the cancelled context downstream,
	// or it may succeed if it reached the spend query before observing
	// cancellation. Either way the retry loop must not have eaten 600ms.
	_ = e.CheckSpendingLimit(ctx, orgID)
	elapsed := time.Since(start)
	require.False(t,
		elapsed >= 200*
			time.Millisecond,
	)

}
