package cache

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeL2[K comparable, V any] struct {
	mu      sync.Mutex
	values  map[K]cacheEntry[V]
	getErr  error
	setErr  error
	delErr  error
	casErr  error
	gets    int
	sets    int
	deletes int
	cas     int
}

func newFakeL2[K comparable, V any]() *fakeL2[K, V] {
	return &fakeL2[K, V]{values: make(map[K]cacheEntry[V])}
}

func (f *fakeL2[K, V]) Get(_ context.Context, key K) (cacheEntry[V], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gets++
	if f.getErr != nil {
		return cacheEntry[V]{}, f.getErr
	}
	entry, ok := f.values[key]
	if !ok {
		return cacheEntry[V]{}, ErrCacheMiss
	}
	return entry, nil
}

func (f *fakeL2[K, V]) Set(_ context.Context, key K, entry cacheEntry[V], _ time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sets++
	if f.setErr != nil {
		return f.setErr
	}
	f.values[key] = entry
	return nil
}

func (f *fakeL2[K, V]) CompareAndSet(_ context.Context, key K, entry cacheEntry[V], _ time.Duration) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cas++
	if f.casErr != nil {
		return false, f.casErr
	}
	if current, ok := f.values[key]; ok {
		if entry.Version < current.Version {
			return false, nil
		}
		if entry.Version == current.Version && (!current.Barrier || entry.Barrier) {
			return false, nil
		}
	}
	f.values[key] = entry
	return true, nil
}

func (f *fakeL2[K, V]) Delete(_ context.Context, key K) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deletes++
	if f.delErr != nil {
		return f.delErr
	}
	delete(f.values, key)
	return nil
}

func TestTier_CloseIsNilSafeAndIdempotent(t *testing.T) {
	t.Parallel()

	var nilTier *Tier[string, string]
	nilTier.Close()

	disabledL1 := NewTier[string, string](TierConfig[string, string]{
		Name:      "test_close_disabled_l1",
		DisableL1: true,
	})
	disabledL1.Close()
	disabledL1.Close()

	tier := NewTier[string, string](TierConfig[string, string]{
		Name:        "test_close",
		MaximumSize: 10,
		TTL:         time.Minute,
	})
	tier.Close()
	tier.Close()
}

func TestNewCacheCore_L1HitAvoidsL2AndLoader(t *testing.T) {
	t.Parallel()

	l2 := newFakeL2[string, string]()
	tier := NewTier[string, string](TierConfig[string, string]{
		Name:        "test_l1_hit",
		L2:          l2,
		MaximumSize: 10,
		TTL:         time.Minute,
	})
	if err := tier.Set(t.Context(), "k", "cached", 1); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	l2.mu.Lock()
	l2.gets = 0
	l2.sets = 0
	l2.mu.Unlock()

	got, err := tier.Get(t.Context(), "k", func(context.Context, string) (string, error) {
		t.Fatal("loader should not be called on L1 hit")
		return "", nil
	})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != "cached" {
		t.Fatalf("Get() = %q, want cached", got)
	}
	if l2.gets != 0 {
		t.Fatalf("L2 gets = %d, want 0", l2.gets)
	}
}

func TestNewCacheCore_L2HitBackfillsL1(t *testing.T) {
	t.Parallel()

	l2 := newFakeL2[string, string]()
	l2.values["k"] = cacheEntry[string]{Version: 7, Value: "from-l2"}
	tier := NewTier[string, string](TierConfig[string, string]{
		Name:        "test_l2_hit",
		L2:          l2,
		MaximumSize: 10,
		TTL:         time.Minute,
	})

	got, err := tier.Get(t.Context(), "k", func(context.Context, string) (string, error) {
		t.Fatal("loader should not be called on L2 hit")
		return "", nil
	})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != "from-l2" {
		t.Fatalf("Get() = %q, want from-l2", got)
	}
	l2.mu.Lock()
	l2.gets = 0
	l2.mu.Unlock()
	got, err = tier.Get(t.Context(), "k", nil)
	if err != nil {
		t.Fatalf("Get() second error = %v", err)
	}
	if got != "from-l2" {
		t.Fatalf("Get() second = %q, want from-l2", got)
	}
	if l2.gets != 0 {
		t.Fatalf("L2 gets after L1 backfill = %d, want 0", l2.gets)
	}
}

func TestNewCacheCore_FullMissLoadsAndNegativeCaches(t *testing.T) {
	t.Parallel()

	l2 := newFakeL2[string, *int]()
	var loads atomic.Int64
	tier := NewTier[string, *int](TierConfig[string, *int]{
		Name:           "test_negative",
		L2:             l2,
		MaximumSize:    10,
		TTL:            time.Minute,
		EnableNegative: true,
	})

	got, err := tier.Get(t.Context(), "missing", func(context.Context, string) (*int, error) {
		loads.Add(1)
		return nil, nil
	})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != nil {
		t.Fatalf("Get() = %v, want nil", got)
	}
	got, err = tier.Get(t.Context(), "missing", func(context.Context, string) (*int, error) {
		loads.Add(1)
		return new(int), nil
	})
	if err != nil {
		t.Fatalf("Get() second error = %v", err)
	}
	if got != nil {
		t.Fatalf("Get() second = %v, want negative cached nil", got)
	}
	if loads.Load() != 1 {
		t.Fatalf("loader calls = %d, want 1", loads.Load())
	}
}

func TestNewCacheCore_SingleflightCoalescesMisses(t *testing.T) {
	t.Parallel()

	tier := NewTier[string, string](TierConfig[string, string]{
		Name:        "test_singleflight",
		L2:          newFakeL2[string, string](),
		MaximumSize: 10,
		TTL:         time.Minute,
	})
	var loads atomic.Int64
	start := make(chan struct{})
	const callers = 32
	var wg sync.WaitGroup
	errs := make(chan error, callers)
	for range callers {
		wg.Go(func() {
			<-start
			got, err := tier.Get(t.Context(), "k", func(context.Context, string) (string, error) {
				loads.Add(1)
				time.Sleep(10 * time.Millisecond)
				return "loaded", nil
			})
			if err != nil {
				errs <- err
				return
			}
			if got != "loaded" {
				errs <- fmt.Errorf("got %q, want loaded", got)
			}
		})
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	if loads.Load() != 1 {
		t.Fatalf("loader calls = %d, want 1", loads.Load())
	}
}

func TestNewCacheCore_FailOpenFallsThroughToLoader(t *testing.T) {
	t.Parallel()

	l2 := newFakeL2[string, string]()
	l2.getErr = errors.New("redis down")
	var failOp string
	tier := NewTier[string, string](TierConfig[string, string]{
		Name:        "test_fail_open",
		L2:          l2,
		MaximumSize: 10,
		TTL:         time.Minute,
		OnFailOpen: func(operation string, _ error) {
			failOp = operation
		},
	})

	got, err := tier.Get(t.Context(), "k", func(context.Context, string) (string, error) {
		return "db", nil
	})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != "db" {
		t.Fatalf("Get() = %q, want db", got)
	}
	if failOp != "get" {
		t.Fatalf("fail-open operation = %q, want get", failOp)
	}
}

func TestNewCacheCore_CloneAndSanitizeBoundaries(t *testing.T) {
	t.Parallel()

	type authDTO struct {
		Scopes []string
		Secret string
	}
	tier := NewTier[string, authDTO](TierConfig[string, authDTO]{
		Name:        "test_clone_sanitize",
		L2:          newFakeL2[string, authDTO](),
		MaximumSize: 10,
		TTL:         time.Minute,
		Sanitize: func(v authDTO) authDTO {
			v.Secret = ""
			return v
		},
		Clone: func(v authDTO) authDTO {
			v.Scopes = append([]string(nil), v.Scopes...)
			return v
		},
	})

	got, err := tier.Get(t.Context(), "k", func(context.Context, string) (authDTO, error) {
		return authDTO{Scopes: []string{"runs:read"}, Secret: "plaintext"}, nil
	})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Secret != "" {
		t.Fatalf("cached secret = %q, want empty", got.Secret)
	}
	got.Scopes[0] = "mutated"
	again, err := tier.Get(t.Context(), "k", nil)
	if err != nil {
		t.Fatalf("Get() again error = %v", err)
	}
	if again.Scopes[0] != "runs:read" {
		t.Fatalf("cached scopes mutated to %q", again.Scopes[0])
	}
}

func TestStrict_CASRejectsEqualAndLowerVersions(t *testing.T) {
	t.Parallel()

	l2 := newFakeL2[string, string]()
	tier := NewTier[string, string](TierConfig[string, string]{
		Name:        "test_cas",
		L2:          l2,
		MaximumSize: 10,
		TTL:         time.Minute,
	})

	if ok, err := tier.CompareAndSet(t.Context(), "k", "v2", 2); err != nil || !ok {
		t.Fatalf("CompareAndSet(v2) = %v, %v; want true nil", ok, err)
	}
	if ok, err := tier.CompareAndSet(t.Context(), "k", "v1", 1); err != nil || ok {
		t.Fatalf("CompareAndSet(v1) = %v, %v; want false nil", ok, err)
	}
	if ok, err := tier.CompareAndSet(t.Context(), "k", "v2-equal", 2); err != nil || ok {
		t.Fatalf("CompareAndSet(equal) = %v, %v; want false nil", ok, err)
	}
	got, err := tier.GetConsistent(t.Context(), "k", 2, nil)
	if err != nil {
		t.Fatalf("GetConsistent() error = %v", err)
	}
	if got != "v2" {
		t.Fatalf("GetConsistent() = %q, want v2", got)
	}
}

func TestStrict_GetConsistentIgnoresStaleL1AndL2(t *testing.T) {
	t.Parallel()

	l2 := newFakeL2[string, string]()
	l2.values["k"] = cacheEntry[string]{Version: 1, Value: "stale-l2"}
	tier := NewTier[string, string](TierConfig[string, string]{
		Name:        "test_consistent",
		L2:          l2,
		MaximumSize: 10,
		TTL:         time.Minute,
	})
	if err := tier.Set(t.Context(), "k", "stale-l1", 1); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	got, err := tier.GetConsistent(t.Context(), "k", 5, func(context.Context, string) (string, error) {
		return "fresh", nil
	})
	if err != nil {
		t.Fatalf("GetConsistent() error = %v", err)
	}
	if got != "fresh" {
		t.Fatalf("GetConsistent() = %q, want fresh", got)
	}
	l2.mu.Lock()
	defer l2.mu.Unlock()
	if l2.values["k"].Version != 5 {
		t.Fatalf("L2 version = %d, want 5", l2.values["k"].Version)
	}
}

func TestNewCacheCore_TTLJitterBounds(t *testing.T) {
	t.Parallel()

	base := 100 * time.Millisecond
	for range 100 {
		got := JitterTTL(base, 0.25)
		if got < base || got >= base+25*time.Millisecond {
			t.Fatalf("JitterTTL() = %s outside [%s,%s)", got, base, base+25*time.Millisecond)
		}
	}
}

func FuzzCacheEnvelopeJSON(f *testing.F) {
	f.Add([]byte(`{"version":1,"value":"ok"}`))
	f.Add([]byte(`{"version":-1,"negative":true}`))
	f.Add([]byte(`not-json`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		var entry cacheEntry[string]
		_ = JSONCodec[cacheEntry[string]]{}.Unmarshal(raw, &entry)
	})
}
