package store

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestEvictAPIKeyTouch_RemovesEntryAndDecrementsSize verifies the
// cleanup path called from RevokeAPIKey actually frees the cache slot
// and adjusts the size counter atomically.
func TestEvictAPIKeyTouch_RemovesEntryAndDecrementsSize(t *testing.T) {
	ClearAPIKeyTouchCacheForTest(t)

	recordAPIKeyTouch("key-A", time.Now().UnixNano())
	recordAPIKeyTouch("key-B", time.Now().UnixNano())
	require.EqualValues(t, 2, apiKeyTouchSize.
		Load())

	evictAPIKeyTouch("key-A")

	if _, ok := apiKeyTouchCache.Load("key-A"); ok {
		require.Fail(t,

			"key-A still present after eviction")
	}
	if _, ok := apiKeyTouchCache.Load("key-B"); !ok {
		require.Fail(t,

			"key-B was unexpectedly evicted")
	}
	require.EqualValues(t, 1, apiKeyTouchSize.
		Load())

}

// TestEvictAPIKeyTouch_MissIsNoOp guards against double-decrement: if
// a key was never touched (or already evicted), the size counter must
// not go negative.
func TestEvictAPIKeyTouch_MissIsNoOp(t *testing.T) {
	ClearAPIKeyTouchCacheForTest(t)

	recordAPIKeyTouch("only-key", time.Now().UnixNano())
	require.EqualValues(t, 1, apiKeyTouchSize.
		Load())

	evictAPIKeyTouch("never-touched")
	evictAPIKeyTouch("never-touched")
	require.EqualValues(t, 1, apiKeyTouchSize.
		Load())

}

// TestEvictAPIKeyTouch_DoubleEvictIsIdempotent verifies the size
// counter holds at zero when the same id is evicted twice. LoadAndDelete
// returns loaded=false on the second call so the decrement is skipped.
func TestEvictAPIKeyTouch_DoubleEvictIsIdempotent(t *testing.T) {
	ClearAPIKeyTouchCacheForTest(t)

	recordAPIKeyTouch("hot-key", time.Now().UnixNano())
	evictAPIKeyTouch("hot-key")
	evictAPIKeyTouch("hot-key")
	require.EqualValues(t, 0, apiKeyTouchSize.
		Load())

}
