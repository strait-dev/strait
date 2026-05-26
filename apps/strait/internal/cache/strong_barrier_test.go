package cache

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestStrongBarrierRejectsStaleFill(t *testing.T) {
	t.Parallel()

	l2 := newFakeL2[string, string]()
	tier := NewTier[string, string](TierConfig[string, string]{
		Name:        "barrier_reject",
		L2:          l2,
		MaximumSize: 10,
		TTL:         time.Minute,
	})

	if err := tier.StrongInvalidate(t.Context(), StrongNamespacePolicy{Namespace: "barrier_reject"}, "k", "k", VersionBarrier{Version: 10}, nil); err != nil {
		t.Fatalf("StrongInvalidate() error = %v", err)
	}
	_, err := tier.GetConsistentVersioned(t.Context(), "k", 0, func(context.Context, string) (Versioned[string], error) {
		return Versioned[string]{Value: "stale", Version: 9}, nil
	})
	if !errors.Is(err, ErrStaleVersion) {
		t.Fatalf("GetConsistentVersioned() error = %v, want ErrStaleVersion", err)
	}

	l2.mu.Lock()
	stored := l2.values["k"]
	l2.mu.Unlock()
	if !stored.Barrier || stored.Version != 10 {
		t.Fatalf("stored entry = %+v, want barrier@10", stored)
	}
}

func TestStrongBarrierAllowsEqualVersionValueReplacement(t *testing.T) {
	t.Parallel()

	l2 := newFakeL2[string, string]()
	tier := NewTier[string, string](TierConfig[string, string]{
		Name:        "barrier_replace",
		L2:          l2,
		MaximumSize: 10,
		TTL:         time.Minute,
	})

	if err := tier.StrongInvalidate(t.Context(), StrongNamespacePolicy{Namespace: "barrier_replace"}, "k", "k", VersionBarrier{Version: 10}, nil); err != nil {
		t.Fatalf("StrongInvalidate() error = %v", err)
	}
	ok, err := tier.StrongWriteThrough(t.Context(), StrongNamespacePolicy{Namespace: "barrier_replace"}, "k", "k", "fresh", 10, nil)
	if err != nil {
		t.Fatalf("StrongWriteThrough() error = %v", err)
	}
	if !ok {
		t.Fatal("StrongWriteThrough() ok = false, want true")
	}
	got, err := tier.GetConsistentVersioned(t.Context(), "k", 10, func(context.Context, string) (Versioned[string], error) {
		t.Fatal("loader must not run after equal-version replacement")
		return Versioned[string]{}, nil
	})
	if err != nil {
		t.Fatalf("GetConsistentVersioned() error = %v", err)
	}
	if got.Value != "fresh" || got.Version != 10 {
		t.Fatalf("got %+v, want fresh@10", got)
	}
}

func TestStrongBarrierBusMessageIsIdempotent(t *testing.T) {
	t.Parallel()

	l2 := newFakeL2[string, string]()
	tier := NewTier[string, string](TierConfig[string, string]{
		Name:        "barrier_bus",
		L2:          l2,
		MaximumSize: 10,
		TTL:         time.Minute,
	})
	reg := NewRegistry(RegistryConfig{Origin: "node-b"})
	reg.Register("barrier_bus", UpdatingStringTierHandler[string]{Tier: tier})
	msg := BusMessage{Action: BusActionInvalidate, Namespace: "barrier_bus", Key: "k", Version: 10, Origin: "node-a"}
	payload, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal bus message: %v", err)
	}

	reg.Handle(t.Context(), payload)
	reg.Handle(t.Context(), payload)

	l2.mu.Lock()
	stored := l2.values["k"]
	l2.mu.Unlock()
	if !stored.Barrier || stored.Version != 10 {
		t.Fatalf("stored entry = %+v, want idempotent barrier@10", stored)
	}
}

func TestStrongBarrierRedisCASAllowsEqualVersionReplacement(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	l2 := NewRedisL2[string, string](RedisL2Config[string, string]{
		Client:    rdb,
		Namespace: "barrier_redis",
	})

	ok, err := l2.CompareAndSet(context.Background(), "k", cacheEntry[string]{Version: 10, Barrier: true}, time.Minute)
	if err != nil || !ok {
		t.Fatalf("CompareAndSet(barrier) = %v, %v; want true, nil", ok, err)
	}
	ok, err = l2.CompareAndSet(context.Background(), "k", cacheEntry[string]{Version: 10, Value: "fresh"}, time.Minute)
	if err != nil || !ok {
		t.Fatalf("CompareAndSet(equal value) = %v, %v; want true, nil", ok, err)
	}
	entry, err := l2.Get(context.Background(), "k")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if entry.Barrier || entry.Value != "fresh" || entry.Version != 10 {
		t.Fatalf("entry = %+v, want fresh@10", entry)
	}
}
