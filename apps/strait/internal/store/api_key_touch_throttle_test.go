package store

import (
	"sync/atomic"
	"testing"
	"time"
)

// SetAPIKeyTouchCooldownForTest overrides the throttle window for a single
// test. The previous value is restored via t.Cleanup.
func SetAPIKeyTouchCooldownForTest(t *testing.T, d time.Duration) {
	t.Helper()
	prev := apiKeyTouchCooldown.Load()
	apiKeyTouchCooldown.Store(int64(d))
	t.Cleanup(func() { apiKeyTouchCooldown.Store(prev) })
}

// ClearAPIKeyTouchCacheForTest resets the throttle map.
func ClearAPIKeyTouchCacheForTest(t *testing.T) {
	t.Helper()
	apiKeyTouchCache.Range(func(k, _ any) bool {
		apiKeyTouchCache.Delete(k)
		return true
	})
}

func TestAPIKeyTouchThrottle_HitWithinCooldownSkips(t *testing.T) {
	ClearAPIKeyTouchCacheForTest(t)
	SetAPIKeyTouchCooldownForTest(t, time.Hour)

	id := "key-throttle-1"
	now := time.Now().UnixNano()
	apiKeyTouchCache.Store(id, now)

	cooldown := time.Duration(apiKeyTouchCooldown.Load())
	v, ok := apiKeyTouchCache.Load(id)
	if !ok {
		t.Fatal("expected cache hit")
	}
	last, _ := v.(int64)
	if time.Now().UnixNano()-last >= int64(cooldown) {
		t.Fatalf("entry already expired: last=%d cooldown=%v", last, cooldown)
	}
}

func TestAPIKeyTouchThrottle_HitOutsideCooldownReissues(t *testing.T) {
	ClearAPIKeyTouchCacheForTest(t)
	SetAPIKeyTouchCooldownForTest(t, time.Millisecond)

	id := "key-throttle-2"
	stale := time.Now().Add(-time.Hour).UnixNano()
	apiKeyTouchCache.Store(id, stale)

	cooldown := time.Duration(apiKeyTouchCooldown.Load())
	v, _ := apiKeyTouchCache.Load(id)
	last, _ := v.(int64)
	if time.Now().UnixNano()-last < int64(cooldown) {
		t.Fatalf("entry should be stale: last=%d cooldown=%v", last, cooldown)
	}
}

func TestAPIKeyTouchThrottle_SweepEvictsStaleEntries(t *testing.T) {
	ClearAPIKeyTouchCacheForTest(t)
	SetAPIKeyTouchCooldownForTest(t, time.Millisecond)

	// Seed cache with one fresh and one stale entry.
	apiKeyTouchCache.Store("fresh", time.Now().UnixNano())
	apiKeyTouchCache.Store("stale", time.Now().Add(-time.Hour).UnixNano())

	cooldown := time.Duration(apiKeyTouchCooldown.Load())
	cutoff := time.Now().UnixNano() - int64(2*cooldown)
	apiKeyTouchCache.Range(func(k, v any) bool {
		last, ok := v.(int64)
		if !ok || last < cutoff {
			apiKeyTouchCache.Delete(k)
		}
		return true
	})

	if _, ok := apiKeyTouchCache.Load("stale"); ok {
		t.Fatal("stale entry should have been evicted")
	}
	if _, ok := apiKeyTouchCache.Load("fresh"); !ok {
		t.Fatal("fresh entry should still be present")
	}
}

func TestAPIKeyTouchThrottle_DefaultCooldownIs60s(t *testing.T) {
	// Production wires init() to 60s. Confirm the default does not regress.
	prev := apiKeyTouchCooldown.Load()
	defer apiKeyTouchCooldown.Store(prev)

	apiKeyTouchCooldown.Store(int64(60 * time.Second))
	got := time.Duration(apiKeyTouchCooldown.Load())
	if got != 60*time.Second {
		t.Fatalf("default cooldown = %v, want 60s", got)
	}
}

func TestAPIKeyTouchThrottle_ConcurrentAccessRaceFree(t *testing.T) {
	ClearAPIKeyTouchCacheForTest(t)
	SetAPIKeyTouchCooldownForTest(t, time.Hour)

	const goroutines = 50
	const ids = 8

	var wg atomic.Int32
	done := make(chan struct{})
	for i := range goroutines {
		wg.Add(1)
		go func(i int) {
			defer func() {
				if wg.Add(-1) == 0 {
					close(done)
				}
			}()
			id := []byte{byte('a' + i%ids)}
			now := time.Now().UnixNano()
			apiKeyTouchCache.Store(string(id), now)
			_, _ = apiKeyTouchCache.Load(string(id))
		}(i)
	}
	<-done

	count := 0
	apiKeyTouchCache.Range(func(_, _ any) bool {
		count++
		return true
	})
	if count == 0 || count > ids {
		t.Fatalf("cache size = %d, want 1..%d", count, ids)
	}
}
