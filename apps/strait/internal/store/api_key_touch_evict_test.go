package store

import (
	"testing"
	"time"
)

// TestEvictAPIKeyTouch_RemovesEntryAndDecrementsSize verifies the
// cleanup path called from RevokeAPIKey actually frees the cache slot
// and adjusts the size counter atomically.
func TestEvictAPIKeyTouch_RemovesEntryAndDecrementsSize(t *testing.T) {
	ClearAPIKeyTouchCacheForTest(t)

	recordAPIKeyTouch("key-A", time.Now().UnixNano())
	recordAPIKeyTouch("key-B", time.Now().UnixNano())
	if got := apiKeyTouchSize.Load(); got != 2 {
		t.Fatalf("size after seed = %d, want 2", got)
	}

	evictAPIKeyTouch("key-A")

	if _, ok := apiKeyTouchCache.Load("key-A"); ok {
		t.Fatal("key-A still present after eviction")
	}
	if _, ok := apiKeyTouchCache.Load("key-B"); !ok {
		t.Fatal("key-B was unexpectedly evicted")
	}
	if got := apiKeyTouchSize.Load(); got != 1 {
		t.Fatalf("size after eviction = %d, want 1", got)
	}
}

// TestEvictAPIKeyTouch_MissIsNoOp guards against double-decrement: if
// a key was never touched (or already evicted), the size counter must
// not go negative.
func TestEvictAPIKeyTouch_MissIsNoOp(t *testing.T) {
	ClearAPIKeyTouchCacheForTest(t)

	recordAPIKeyTouch("only-key", time.Now().UnixNano())
	if got := apiKeyTouchSize.Load(); got != 1 {
		t.Fatalf("size after seed = %d, want 1", got)
	}

	evictAPIKeyTouch("never-touched")
	evictAPIKeyTouch("never-touched")

	if got := apiKeyTouchSize.Load(); got != 1 {
		t.Fatalf("size after evicting unseen keys = %d, want 1 (no double-decrement)", got)
	}
}

// TestEvictAPIKeyTouch_DoubleEvictIsIdempotent verifies the size
// counter holds at zero when the same id is evicted twice. LoadAndDelete
// returns loaded=false on the second call so the decrement is skipped.
func TestEvictAPIKeyTouch_DoubleEvictIsIdempotent(t *testing.T) {
	ClearAPIKeyTouchCacheForTest(t)

	recordAPIKeyTouch("hot-key", time.Now().UnixNano())
	evictAPIKeyTouch("hot-key")
	evictAPIKeyTouch("hot-key")

	if got := apiKeyTouchSize.Load(); got != 0 {
		t.Fatalf("size after double evict = %d, want 0", got)
	}
}
