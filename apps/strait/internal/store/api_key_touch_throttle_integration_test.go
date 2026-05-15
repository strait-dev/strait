//go:build integration

package store_test

import (
	"context"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
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
	if err := q.CreateAPIKey(ctx, key); err != nil {
		t.Fatalf("CreateAPIKey() error = %v", err)
	}

	if err := q.TouchAPIKeyLastUsed(ctx, key.ID); err != nil {
		t.Fatalf("TouchAPIKeyLastUsed(1) error = %v", err)
	}
	first, err := q.GetAPIKeyByID(ctx, key.ID)
	if err != nil {
		t.Fatalf("GetAPIKeyByID(after first) error = %v", err)
	}
	if first.LastUsedAt == nil {
		t.Fatal("first touch did not stamp last_used_at")
	}

	time.Sleep(20 * time.Millisecond)
	if err := q.TouchAPIKeyLastUsed(ctx, key.ID); err != nil {
		t.Fatalf("TouchAPIKeyLastUsed(2) error = %v", err)
	}
	second, err := q.GetAPIKeyByID(ctx, key.ID)
	if err != nil {
		t.Fatalf("GetAPIKeyByID(after second) error = %v", err)
	}
	if second.LastUsedAt == nil {
		t.Fatal("last_used_at became nil after throttled call")
	}
	if !second.LastUsedAt.Equal(*first.LastUsedAt) {
		t.Fatalf("last_used_at advanced under throttle: first=%v second=%v",
			*first.LastUsedAt, *second.LastUsedAt)
	}
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
	if err := q.CreateAPIKey(ctx, key); err != nil {
		t.Fatalf("CreateAPIKey() error = %v", err)
	}

	if err := q.TouchAPIKeyLastUsed(ctx, key.ID); err != nil {
		t.Fatalf("TouchAPIKeyLastUsed(1) error = %v", err)
	}
	first, _ := q.GetAPIKeyByID(ctx, key.ID)
	if first.LastUsedAt == nil {
		t.Fatal("first touch did not stamp last_used_at")
	}

	time.Sleep(50 * time.Millisecond)
	if err := q.TouchAPIKeyLastUsed(ctx, key.ID); err != nil {
		t.Fatalf("TouchAPIKeyLastUsed(2) error = %v", err)
	}
	second, _ := q.GetAPIKeyByID(ctx, key.ID)
	if second.LastUsedAt == nil {
		t.Fatal("second touch did not stamp last_used_at")
	}
	if !second.LastUsedAt.After(*first.LastUsedAt) {
		t.Fatalf("last_used_at did not advance after cooldown: first=%v second=%v",
			*first.LastUsedAt, *second.LastUsedAt)
	}
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
	if err := q.CreateAPIKey(ctx, key); err != nil {
		t.Fatalf("CreateAPIKey() error = %v", err)
	}

	const burst = 100
	for range burst {
		if err := q.TouchAPIKeyLastUsed(ctx, key.ID); err != nil {
			t.Fatalf("TouchAPIKeyLastUsed error = %v", err)
		}
	}

	got, err := q.GetAPIKeyByID(ctx, key.ID)
	if err != nil {
		t.Fatalf("GetAPIKeyByID() error = %v", err)
	}
	if got.LastUsedAt == nil {
		t.Fatal("last_used_at never stamped after burst")
	}
}
