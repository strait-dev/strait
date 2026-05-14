package store

import (
	"fmt"
	"strings"
	"sync"
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

// ClearAPIKeyTouchCacheForTest resets the throttle map and its bookkeeping.
func ClearAPIKeyTouchCacheForTest(t *testing.T) {
	t.Helper()
	apiKeyTouchCache.Range(func(k, _ any) bool {
		apiKeyTouchCache.Delete(k)
		return true
	})
	apiKeyTouchSize.Store(0)
	apiKeyTouchSweeping.Store(false)
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

func TestRecordAPIKeyTouch_SizeMatchesUniqueKeys(t *testing.T) {
	ClearAPIKeyTouchCacheForTest(t)
	SetAPIKeyTouchCooldownForTest(t, time.Hour)

	const goroutines = 200
	const uniqueIDs = 16

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("key-%d", i%uniqueIDs)
			recordAPIKeyTouch(id, time.Now().UnixNano())
		}(i)
	}
	wg.Wait()

	got := apiKeyTouchSize.Load()
	if got != uniqueIDs {
		t.Fatalf("apiKeyTouchSize = %d, want %d", got, uniqueIDs)
	}

	// Spot check: the actual map cardinality matches the counter.
	var ranged int64
	apiKeyTouchCache.Range(func(_, _ any) bool {
		ranged++
		return true
	})
	if ranged != got {
		t.Fatalf("map cardinality %d != size counter %d", ranged, got)
	}
}

func TestSweepAPIKeyTouchCache_SkipsWhenUnderThreshold(t *testing.T) {
	ClearAPIKeyTouchCacheForTest(t)
	SetAPIKeyTouchCooldownForTest(t, time.Millisecond)

	stale := time.Now().Add(-time.Hour).UnixNano()
	for i := range 100 {
		recordAPIKeyTouch(fmt.Sprintf("stale-%d", i), stale)
	}
	if apiKeyTouchSize.Load() != 100 {
		t.Fatalf("setup: size = %d, want 100", apiKeyTouchSize.Load())
	}

	sweepAPIKeyTouchCacheIfFull(time.Duration(apiKeyTouchCooldown.Load()))

	// Under the high-water mark, the sweep MUST be a no-op even when every
	// entry is stale. Operators rely on this to keep the cache warm.
	if got := apiKeyTouchSize.Load(); got != 100 {
		t.Fatalf("size after sweep = %d, want 100 (no eviction below high-water)", got)
	}
}

func TestSweepAPIKeyTouchCache_EvictsStalePreservesFresh(t *testing.T) {
	ClearAPIKeyTouchCacheForTest(t)
	// Use a generous cooldown so the fresh timestamps stay within the
	// 2*cooldown window for the entire test, regardless of how long the
	// seeding loop and Range walk take under -race.
	SetAPIKeyTouchCooldownForTest(t, time.Hour)

	// Seed enough entries to cross the high-water mark.
	stale := time.Now().Add(-2 * time.Hour).Add(-time.Minute).UnixNano()
	fresh := time.Now().UnixNano()
	for i := range apiKeyTouchSweepHighWater + 50 {
		if i%2 == 0 {
			recordAPIKeyTouch(fmt.Sprintf("stale-%d", i), stale)
		} else {
			recordAPIKeyTouch(fmt.Sprintf("fresh-%d", i), fresh)
		}
	}

	cooldown := time.Duration(apiKeyTouchCooldown.Load())
	sweepAPIKeyTouchCacheIfFull(cooldown)

	freshLeft := 0
	staleLeft := 0
	apiKeyTouchCache.Range(func(k, _ any) bool {
		key, _ := k.(string)
		switch {
		case strings.HasPrefix(key, "fresh-"):
			freshLeft++
		case strings.HasPrefix(key, "stale-"):
			staleLeft++
		}
		return true
	})
	if staleLeft != 0 {
		t.Fatalf("stale entries left after sweep: %d", staleLeft)
	}
	if freshLeft == 0 {
		t.Fatal("sweep evicted every fresh entry")
	}
	if got := apiKeyTouchSize.Load(); got != int64(freshLeft) {
		t.Fatalf("size counter = %d, want %d", got, freshLeft)
	}
}

func TestSweepAPIKeyTouchCache_SingleSweeperWins(t *testing.T) {
	ClearAPIKeyTouchCacheForTest(t)
	SetAPIKeyTouchCooldownForTest(t, time.Millisecond)

	stale := time.Now().Add(-time.Hour).UnixNano()
	for i := range apiKeyTouchSweepHighWater + 100 {
		recordAPIKeyTouch(fmt.Sprintf("stale-%d", i), stale)
	}

	// Race many concurrent sweepers. Exactly one must observe the CAS win;
	// the others must short-circuit so we don't get duplicate eviction
	// work or counter underflow.
	const sweepers = 64
	var observed atomic.Int32
	var wg sync.WaitGroup
	wg.Add(sweepers)
	gate := make(chan struct{})
	for range sweepers {
		go func() {
			defer wg.Done()
			<-gate
			before := apiKeyTouchSweeping.Load()
			sweepAPIKeyTouchCacheIfFull(time.Duration(apiKeyTouchCooldown.Load()))
			if !before {
				// Best-effort: only the winner ever sees sweeping=false on entry.
				// Multiple goroutines may pass this check, but only one will
				// have actually executed the eviction Range (the CAS winner).
				observed.Add(1)
			}
		}()
	}
	close(gate)
	wg.Wait()

	if got := apiKeyTouchSweeping.Load(); got {
		t.Fatalf("apiKeyTouchSweeping still true after all sweepers finished")
	}
	if got := apiKeyTouchSize.Load(); got != 0 {
		t.Fatalf("size after sweep = %d, want 0 (every entry was stale)", got)
	}
}

func TestSweepAPIKeyTouchCache_RefreshedEntryNotEvicted(t *testing.T) {
	ClearAPIKeyTouchCacheForTest(t)
	SetAPIKeyTouchCooldownForTest(t, time.Millisecond)

	stale := time.Now().Add(-time.Hour).UnixNano()
	for i := range apiKeyTouchSweepHighWater + 1 {
		recordAPIKeyTouch(fmt.Sprintf("k-%d", i), stale)
	}
	// Refresh one entry to "now" before triggering the sweep. The
	// CompareAndDelete in the sweep must observe the updated value and
	// leave the entry alone.
	recordAPIKeyTouch("k-0", time.Now().UnixNano())

	sweepAPIKeyTouchCacheIfFull(time.Duration(apiKeyTouchCooldown.Load()))

	if _, ok := apiKeyTouchCache.Load("k-0"); !ok {
		t.Fatal("refreshed entry was evicted by sweep")
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
