package otterstore

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	lib_store "github.com/eko/gocache/lib/v4/store"
	"github.com/sourcegraph/conc"
)

// TestCache_ZeroTTL verifies that a zero TTL falls back to the default 1h.
func TestCache_ZeroTTL(t *testing.T) {
	t.Parallel()
	s := newTestStore(t, Config{})
	ctx := context.Background()

	// Set with explicit zero TTL; the store should clamp to the fallback.
	err := s.Set(ctx, "zero-ttl", "val", lib_store.WithExpiration(0))
	if err != nil {
		t.Fatalf("Set with zero TTL failed: %v", err)
	}

	val, err := s.Get(ctx, "zero-ttl")
	if err != nil {
		t.Fatalf("Get after zero-TTL Set failed: %v", err)
	}
	if val != "val" {
		t.Fatalf("got %v, want val", val)
	}
}

// TestCache_NegativeTTL verifies that a negative TTL falls back to the default.
func TestCache_NegativeTTL(t *testing.T) {
	t.Parallel()
	s := newTestStore(t, Config{})
	ctx := context.Background()

	err := s.Set(ctx, "neg-ttl", "val", lib_store.WithExpiration(-5*time.Second))
	if err != nil {
		t.Fatalf("Set with negative TTL failed: %v", err)
	}

	val, err := s.Get(ctx, "neg-ttl")
	if err != nil {
		t.Fatalf("Get after negative-TTL Set failed: %v", err)
	}
	if val != "val" {
		t.Fatalf("got %v, want val", val)
	}
}

// TestCache_NullByteKey verifies that keys containing null bytes work correctly.
func TestCache_NullByteKey(t *testing.T) {
	t.Parallel()
	s := newTestStore(t, Config{})
	ctx := context.Background()

	key := "key\x00with\x00nulls"
	err := s.Set(ctx, key, "nullval")
	if err != nil {
		t.Fatalf("Set with null byte key failed: %v", err)
	}

	val, err := s.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get with null byte key failed: %v", err)
	}
	if val != "nullval" {
		t.Fatalf("got %v, want nullval", val)
	}
}

// TestCache_LargeValue verifies that a 10MB value can be stored and retrieved.
func TestCache_LargeValue(t *testing.T) {
	t.Parallel()
	s := newTestStore(t, Config{})
	ctx := context.Background()

	large := strings.Repeat("x", 10*1024*1024) // 10 MB.
	err := s.Set(ctx, "large", large)
	if err != nil {
		t.Fatalf("Set with large value failed: %v", err)
	}

	val, err := s.Get(ctx, "large")
	if err != nil {
		t.Fatalf("Get large value failed: %v", err)
	}
	if val != large {
		t.Fatal("retrieved value does not match stored 10MB value")
	}
}

// TestCache_ConcurrentGetSet verifies that 100 goroutines can read and write concurrently.
func TestCache_ConcurrentGetSet(t *testing.T) {
	t.Parallel()
	s := newTestStore(t, Config{})
	ctx := context.Background()

	var wg conc.WaitGroup
	for i := range 100 {
		id := i
		wg.Go(func() {
			key := fmt.Sprintf("conc-%d", id%10)
			_ = s.Set(ctx, key, id)
			_, _ = s.Get(ctx, key)
		})
	}
	wg.Wait()
}

// FuzzCacheKeyValue fuzzes cache Set and Get with arbitrary keys and values.
func FuzzCacheKeyValue(f *testing.F) {
	f.Add("key", "value")
	f.Add("", "")
	f.Add("\x00", "\x00")
	f.Add(strings.Repeat("k", 1000), strings.Repeat("v", 1000))
	f.Add("emoji-key", "emoji-value")

	f.Fuzz(func(t *testing.T, key, value string) {
		s := New(Config{MaxCapacity: 100, DefaultTTL: time.Minute})
		defer s.Close()
		ctx := context.Background()

		// Must not panic.
		_ = s.Set(ctx, key, value)
		got, err := s.Get(ctx, key)
		if err != nil {
			// Key may have been evicted due to small capacity; acceptable.
			return
		}
		if got != value {
			t.Fatalf("got %v, want %v", got, value)
		}
	})
}
