package otterstore

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	lib_store "github.com/eko/gocache/lib/v4/store"
	"github.com/maypok86/otter"
)

func newTestStore(t *testing.T, cfg Config) *OtterStore {
	t.Helper()
	if cfg.MaxCapacity == 0 {
		cfg.MaxCapacity = 1000
	}
	if cfg.DefaultTTL == 0 {
		cfg.DefaultTTL = 5 * time.Minute
	}
	s := New(cfg)
	t.Cleanup(s.Close)
	return s
}

func TestOtterStore_SetAndGet(t *testing.T) {
	t.Parallel()
	s := newTestStore(t, Config{})
	ctx := context.Background()

	err := s.Set(ctx, "key1", "value1")
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	val, err := s.Get(ctx, "key1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if val != "value1" {
		t.Fatalf("got %v, want value1", val)
	}
}

func TestOtterStore_GetMissing(t *testing.T) {
	t.Parallel()
	s := newTestStore(t, Config{})
	ctx := context.Background()

	_, err := s.Get(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing key, got nil")
	}
	if !errors.Is(err, &lib_store.NotFound{}) {
		t.Fatalf("expected NotFound error, got %T: %v", err, err)
	}
}

func TestOtterStore_SetOverwrite(t *testing.T) {
	t.Parallel()
	s := newTestStore(t, Config{})
	ctx := context.Background()

	_ = s.Set(ctx, "key1", "first")
	_ = s.Set(ctx, "key1", "second")

	val, err := s.Get(ctx, "key1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if val != "second" {
		t.Fatalf("got %v, want second", val)
	}
}

func TestOtterStore_Delete(t *testing.T) {
	t.Parallel()
	s := newTestStore(t, Config{})
	ctx := context.Background()

	_ = s.Set(ctx, "key1", "value1")
	err := s.Delete(ctx, "key1")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = s.Get(ctx, "key1")
	if err == nil {
		t.Fatal("expected NotFound after delete")
	}
}

func TestOtterStore_DeleteMissing(t *testing.T) {
	t.Parallel()
	s := newTestStore(t, Config{})
	ctx := context.Background()

	err := s.Delete(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("Delete on missing key should not error, got: %v", err)
	}
}

func TestOtterStore_Clear(t *testing.T) {
	t.Parallel()
	s := newTestStore(t, Config{})
	ctx := context.Background()

	for i := range 10 {
		_ = s.Set(ctx, fmt.Sprintf("key%d", i), i)
	}

	err := s.Clear(ctx)
	if err != nil {
		t.Fatalf("Clear failed: %v", err)
	}

	for i := range 10 {
		_, err := s.Get(ctx, fmt.Sprintf("key%d", i))
		if err == nil {
			t.Fatalf("expected NotFound for key%d after Clear", i)
		}
	}
}

func TestOtterStore_TTLExpiry(t *testing.T) {
	t.Parallel()

	var evicted atomic.Int64
	s := newTestStore(t, Config{
		DefaultTTL: 1 * time.Second,
		OnEviction: func(_ string, _ any, _ otter.DeletionCause) {
			evicted.Add(1)
		},
	})
	ctx := context.Background()

	_ = s.Set(ctx, "expire-me", "val")

	val, err := s.Get(ctx, "expire-me")
	if err != nil {
		t.Fatalf("Get immediately after Set should succeed: %v", err)
	}
	if val != "val" {
		t.Fatalf("got %v, want val", val)
	}

	// Otter uses a timer wheel with ~1s granularity for expiration.
	// Poll until the entry expires rather than using a fixed sleep.
	deadline := time.After(10 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for TTL expiry")
		case <-ticker.C:
			_, err = s.Get(ctx, "expire-me")
			if err != nil {
				return // Entry expired as expected.
			}
		}
	}
}

func TestOtterStore_CustomTTLOverride(t *testing.T) {
	t.Parallel()
	s := newTestStore(t, Config{
		DefaultTTL: 10 * time.Minute,
	})
	ctx := context.Background()

	_ = s.Set(ctx, "short-ttl", "val", lib_store.WithExpiration(1*time.Second))

	val, err := s.Get(ctx, "short-ttl")
	if err != nil {
		t.Fatalf("Get immediately should succeed: %v", err)
	}
	if val != "val" {
		t.Fatalf("got %v, want val", val)
	}

	// Otter uses a timer wheel with ~1s granularity.
	// Poll until the entry expires rather than using a fixed sleep.
	deadline := time.After(10 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for custom TTL expiry")
		case <-ticker.C:
			_, err = s.Get(ctx, "short-ttl")
			if err != nil {
				return // Entry expired as expected.
			}
		}
	}
}

func TestOtterStore_EvictionCallback(t *testing.T) {
	t.Parallel()

	var evictedKeys sync.Map
	s := newTestStore(t, Config{
		MaxCapacity: 10,
		DefaultTTL:  time.Hour,
		OnEviction: func(key string, _ any, _ otter.DeletionCause) {
			evictedKeys.Store(key, true)
		},
	})
	ctx := context.Background()

	for i := range 100 {
		_ = s.Set(ctx, fmt.Sprintf("k%d", i), i)
	}

	// Poll until at least one eviction callback fires.
	deadline := time.After(5 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for eviction callback to fire when exceeding capacity")
		case <-ticker.C:
			count := 0
			evictedKeys.Range(func(_, _ any) bool {
				count++
				return true
			})
			if count > 0 {
				return
			}
		}
	}
}

func TestOtterStore_EvictionCallbackNil(t *testing.T) {
	t.Parallel()
	s := newTestStore(t, Config{
		MaxCapacity: 5,
		OnEviction:  nil,
	})
	ctx := context.Background()

	// Should not panic even without callback.
	for i := range 20 {
		_ = s.Set(ctx, fmt.Sprintf("k%d", i), i)
	}
}

func TestOtterStore_ConcurrentReadWrite(t *testing.T) {
	t.Parallel()
	s := newTestStore(t, Config{})
	ctx := context.Background()

	var wg sync.WaitGroup
	const goroutines = 50

	// Writers.
	for i := range goroutines {
		id := i
		wg.Go(func() {
			for j := range 100 {
				key := fmt.Sprintf("key%d", j%20)
				_ = s.Set(ctx, key, id*100+j)
			}
		})
	}

	// Readers.
	for range goroutines {
		wg.Go(func() {
			for j := range 100 {
				key := fmt.Sprintf("key%d", j%20)
				_, _ = s.Get(ctx, key)
			}
		})
	}

	wg.Wait()
}

func TestOtterStore_ConcurrentSetDelete(t *testing.T) {
	t.Parallel()
	s := newTestStore(t, Config{})
	ctx := context.Background()

	var wg sync.WaitGroup
	const goroutines = 50

	for range goroutines {
		wg.Go(func() {
			for j := range 100 {
				_ = s.Set(ctx, "contested", j)
			}
		})
	}

	for range goroutines {
		wg.Go(func() {
			for range 100 {
				_ = s.Delete(ctx, "contested")
			}
		})
	}

	wg.Wait()
}

func TestOtterStore_GetType(t *testing.T) {
	t.Parallel()
	s := newTestStore(t, Config{})
	if got := s.GetType(); got != "otter" {
		t.Fatalf("got %q, want %q", got, "otter")
	}
}

func TestOtterStore_SetEmptyStringKey(t *testing.T) {
	t.Parallel()
	s := newTestStore(t, Config{})
	ctx := context.Background()

	_ = s.Set(ctx, "", "empty-key-value")
	val, err := s.Get(ctx, "")
	if err != nil {
		t.Fatalf("Get with empty key failed: %v", err)
	}
	if val != "empty-key-value" {
		t.Fatalf("got %v, want empty-key-value", val)
	}
}

func TestOtterStore_SetNilValue(t *testing.T) {
	t.Parallel()
	s := newTestStore(t, Config{})
	ctx := context.Background()

	_ = s.Set(ctx, "nil-val", nil)
	val, err := s.Get(ctx, "nil-val")
	if err != nil {
		t.Fatalf("Get with nil value failed: %v", err)
	}
	if val != nil {
		t.Fatalf("got %v, want nil", val)
	}
}

func TestOtterStore_ManyEntries(t *testing.T) {
	t.Parallel()
	s := newTestStore(t, Config{
		MaxCapacity: 15_000,
	})
	ctx := context.Background()

	const n = 10_000
	for i := range n {
		_ = s.Set(ctx, fmt.Sprintf("key%d", i), i)
	}

	// Verify a random sample.
	samples := []int{0, 42, 999, 5000, 7777, 9999}
	for _, idx := range samples {
		val, err := s.Get(ctx, fmt.Sprintf("key%d", idx))
		if err != nil {
			t.Fatalf("Get key%d failed: %v", idx, err)
		}
		if val != idx {
			t.Fatalf("key%d: got %v, want %d", idx, val, idx)
		}
	}
}

func TestOtterStore_GetWithTTL_Missing(t *testing.T) {
	t.Parallel()
	s := newTestStore(t, Config{})
	ctx := context.Background()

	_, _, err := s.GetWithTTL(ctx, "missing")
	if err == nil {
		t.Fatal("expected NotFound error")
	}
	if !errors.Is(err, &lib_store.NotFound{}) {
		t.Fatalf("expected NotFound, got %T", err)
	}
}

func TestOtterStore_GetWithTTL_Present(t *testing.T) {
	t.Parallel()
	s := newTestStore(t, Config{})
	ctx := context.Background()

	_ = s.Set(ctx, "key1", "val1")
	val, _, err := s.GetWithTTL(ctx, "key1")
	if err != nil {
		t.Fatalf("GetWithTTL failed: %v", err)
	}
	if val != "val1" {
		t.Fatalf("got %v, want val1", val)
	}
}

func TestOtterStore_NonStringKey_Get(t *testing.T) {
	t.Parallel()
	s := newTestStore(t, Config{})
	ctx := context.Background()

	_, err := s.Get(ctx, 12345)
	if err == nil {
		t.Fatal("expected error for non-string key, got nil")
	}
}

func TestOtterStore_NonStringKey_GetWithTTL(t *testing.T) {
	t.Parallel()
	s := newTestStore(t, Config{})
	ctx := context.Background()

	_, _, err := s.GetWithTTL(ctx, 12345)
	if err == nil {
		t.Fatal("expected error for non-string key, got nil")
	}
}

func TestOtterStore_NonStringKey_Set(t *testing.T) {
	t.Parallel()
	s := newTestStore(t, Config{})
	ctx := context.Background()

	err := s.Set(ctx, 12345, "value")
	if err == nil {
		t.Fatal("expected error for non-string key, got nil")
	}
}

func TestOtterStore_NonStringKey_Delete(t *testing.T) {
	t.Parallel()
	s := newTestStore(t, Config{})
	ctx := context.Background()

	err := s.Delete(ctx, 12345)
	if err == nil {
		t.Fatal("expected error for non-string key, got nil")
	}
}

func TestOtterStore_StringKey_Works(t *testing.T) {
	t.Parallel()
	s := newTestStore(t, Config{})
	ctx := context.Background()

	if err := s.Set(ctx, "valid-key", "val"); err != nil {
		t.Fatalf("Set with string key failed: %v", err)
	}

	val, err := s.Get(ctx, "valid-key")
	if err != nil {
		t.Fatalf("Get with string key failed: %v", err)
	}
	if val != "val" {
		t.Fatalf("got %v, want val", val)
	}

	val, _, err = s.GetWithTTL(ctx, "valid-key")
	if err != nil {
		t.Fatalf("GetWithTTL with string key failed: %v", err)
	}
	if val != "val" {
		t.Fatalf("got %v, want val", val)
	}

	if err := s.Delete(ctx, "valid-key"); err != nil {
		t.Fatalf("Delete with string key failed: %v", err)
	}
}
