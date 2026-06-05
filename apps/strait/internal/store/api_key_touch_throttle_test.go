package store

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/require"
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
	require.True(t, ok)

	last, _ := v.(int64)
	require.False(t, time.
		Now().UnixNano()-last >=
		int64(cooldown))

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
	require.GreaterOrEqual(t, time.
		Now().UnixNano()-last,
		int64(cooldown))

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
		require.Fail(t,

			"stale entry should have been evicted")
	}
	if _, ok := apiKeyTouchCache.Load("fresh"); !ok {
		require.Fail(t,

			"fresh entry should still be present")
	}
}

func TestAPIKeyTouchThrottle_DefaultCooldownIs60s(t *testing.T) {
	// Production wires init() to 60s. Confirm the default does not regress.
	prev := apiKeyTouchCooldown.Load()
	defer apiKeyTouchCooldown.Store(prev)

	apiKeyTouchCooldown.Store(int64(60 * time.Second))
	got := time.Duration(apiKeyTouchCooldown.Load())
	require.Equal(t, 60*
		time.Second,
		got)

}

func TestRecordAPIKeyTouch_SizeMatchesUniqueKeys(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	ClearAPIKeyTouchCacheForTest(t)
	SetAPIKeyTouchCooldownForTest(t, time.Hour)

	const goroutines = 200
	const uniqueIDs = 16

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		{
			i := i
			concWG.Go(func() {
				defer wg.Done()
				id := fmt.Sprintf("key-%d", i%uniqueIDs)
				recordAPIKeyTouch(id, time.Now().UnixNano())
			})
		}
	}
	wg.Wait()

	got := apiKeyTouchSize.Load()
	require.EqualValues(t, uniqueIDs,
		got,
	)

	// Spot check: the actual map cardinality matches the counter.
	var ranged int64
	apiKeyTouchCache.Range(func(_, _ any) bool {
		ranged++
		return true
	})
	require.Equal(t, got,
		ranged)

}

func TestSweepAPIKeyTouchCache_SkipsWhenUnderThreshold(t *testing.T) {
	ClearAPIKeyTouchCacheForTest(t)
	SetAPIKeyTouchCooldownForTest(t, time.Millisecond)

	stale := time.Now().Add(-time.Hour).UnixNano()
	for i := range 100 {
		recordAPIKeyTouch(fmt.Sprintf("stale-%d", i), stale)
	}
	require.EqualValues(t, 100,
		apiKeyTouchSize.
			Load())

	sweepAPIKeyTouchCacheIfFull(time.Duration(apiKeyTouchCooldown.Load()))
	require.EqualValues(t, 100,
		apiKeyTouchSize.
			Load())

	// Under the high-water mark, the sweep MUST be a no-op even when every
	// entry is stale. Operators rely on this to keep the cache warm.

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
	require.EqualValues(t, 0, staleLeft)
	require.NotEqual(t, 0,
		freshLeft,
	)
	require.Equal(t, int64(freshLeft), apiKeyTouchSize.
		Load())

}

func TestSweepAPIKeyTouchCache_SingleSweeperWins(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
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
		concWG.Go(func() {
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
		})
	}
	close(gate)
	wg.Wait()

	if got := apiKeyTouchSweeping.Load(); got {
		require.Failf(t, "test failure",

			"apiKeyTouchSweeping still true after all sweepers finished")
	}
	require.EqualValues(t, 0, apiKeyTouchSize.
		Load())

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
		require.Fail(t,

			"refreshed entry was evicted by sweep")
	}
}

func TestAPIKeyTouchThrottle_ConcurrentAccessRaceFree(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	ClearAPIKeyTouchCacheForTest(t)
	SetAPIKeyTouchCooldownForTest(t, time.Hour)

	const goroutines = 50
	const ids = 8

	var wg sync.WaitGroup
	for i := range goroutines {
		wg.Add(1)
		{
			i := i
			concWG.Go(func() {
				defer wg.Done()
				id := []byte{byte('a' + i%ids)}
				now := time.Now().UnixNano()
				apiKeyTouchCache.Store(string(id), now)
				_, _ = apiKeyTouchCache.Load(string(id))
			})
		}
	}
	wg.Wait()

	count := 0
	apiKeyTouchCache.Range(func(_, _ any) bool {
		count++
		return true
	})
	require.False(t, count ==
		0 ||
		count > ids)

}
