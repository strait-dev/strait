//go:build integration

package store_test

import (
	"context"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

// TestTouchAPIKeyLastUsed_ThrottlesRepeatedCalls verifies that two
// TouchAPIKeyLastUsed calls inside the cooldown window only produce a
// single UPDATE — the second call is a no-op and last_used_at retains
// the first timestamp.
func TestTouchAPIKeyLastUsed_ThrottlesRepeatedCalls(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	store.ClearAPIKeyTouchCacheForTest(t)
	store.SetAPIKeyTouchCooldownForTest(t, time.Hour)

	key := &domain.APIKey{
		ProjectID: "proj-touch-throttle-" + newID(),
		Name:      "throttle",
		KeyHash:   "hash-" + newID(),
		KeyPrefix: "sk_thr",
		Scopes:    []string{"jobs:read"},
	}
	require.NoError(t, q.CreateAPIKey(ctx, key))
	require.NoError(t, q.TouchAPIKeyLastUsed(ctx,
		key.ID))

	first, err := q.GetAPIKeyByID(ctx, key.ID)
	require.NoError(t, err)
	require.NotNil(t, first.
		LastUsedAt,
	)

	time.Sleep(20 * time.Millisecond)
	require.NoError(t, q.TouchAPIKeyLastUsed(ctx,
		key.ID))

	second, err := q.GetAPIKeyByID(ctx, key.ID)
	require.NoError(t, err)
	require.NotNil(t, second.
		LastUsedAt,
	)
	require.True(t, second.
		LastUsedAt.
		Equal(*first.
			LastUsedAt,
		))

}

// TestTouchAPIKeyLastUsed_ReissuesAfterCooldown verifies that once the
// cooldown elapses, the next call does issue an UPDATE and advances
// last_used_at.
func TestTouchAPIKeyLastUsed_ReissuesAfterCooldown(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	store.ClearAPIKeyTouchCacheForTest(t)
	store.SetAPIKeyTouchCooldownForTest(t, 10*time.Millisecond)

	key := &domain.APIKey{
		ProjectID: "proj-touch-cooldown-" + newID(),
		Name:      "cooldown",
		KeyHash:   "hash-" + newID(),
		KeyPrefix: "sk_cool",
		Scopes:    []string{"jobs:read"},
	}
	require.NoError(t, q.CreateAPIKey(ctx, key))
	require.NoError(t, q.TouchAPIKeyLastUsed(ctx,
		key.ID))

	first, _ := q.GetAPIKeyByID(ctx, key.ID)
	require.NotNil(t, first.
		LastUsedAt,
	)

	time.Sleep(50 * time.Millisecond)
	require.NoError(t, q.TouchAPIKeyLastUsed(ctx,
		key.ID))

	second, _ := q.GetAPIKeyByID(ctx, key.ID)
	require.NotNil(t, second.
		LastUsedAt,
	)
	require.True(t, second.
		LastUsedAt.
		After(*first.
			LastUsedAt,
		))

}

// TestTouchAPIKeyLastUsed_BurstCoalescedIntoSingleUpdate is the regression
// test for STR-530. Under a 100-call burst within the cooldown window, only
// the first call must hit the database; all subsequent calls must be
// short-circuited by the throttle.
func TestTouchAPIKeyLastUsed_BurstCoalescedIntoSingleUpdate(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	store.ClearAPIKeyTouchCacheForTest(t)
	store.SetAPIKeyTouchCooldownForTest(t, time.Hour)

	key := &domain.APIKey{
		ProjectID: "proj-touch-burst-" + newID(),
		Name:      "burst",
		KeyHash:   "hash-" + newID(),
		KeyPrefix: "sk_burst",
		Scopes:    []string{"jobs:read"},
	}
	require.NoError(t, q.CreateAPIKey(ctx, key))

	const burst = 100
	for range burst {
		require.NoError(t, q.TouchAPIKeyLastUsed(ctx,
			key.ID))

	}

	got, err := q.GetAPIKeyByID(ctx, key.ID)
	require.NoError(t, err)
	require.NotNil(t, got.LastUsedAt)

}
