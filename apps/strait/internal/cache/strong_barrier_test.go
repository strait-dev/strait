package cache

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, tier.StrongInvalidate(t.
		Context(),
		StrongNamespacePolicy{Namespace: "barrier_reject"}, "k", "k", VersionBarrier{Version: 10}, nil))

	_, err := tier.GetConsistentVersioned(t.Context(), "k", 0, func(context.Context, string) (Versioned[string], error) {
		return Versioned[string]{Value: "stale", Version: 9}, nil
	})
	require.ErrorIs(t,
		err, ErrStaleVersion)

	l2.mu.Lock()
	stored := l2.values["k"]
	l2.mu.Unlock()
	require.False(t,
		!stored.Barrier ||
			stored.
				Version !=
				10)
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
	require.NoError(t, tier.StrongInvalidate(t.
		Context(),
		StrongNamespacePolicy{Namespace: "barrier_replace"}, "k", "k", VersionBarrier{
			Version: 10}, nil))

	ok, err := tier.StrongWriteThrough(
		t.Context(),
		StrongNamespacePolicy{Namespace: "barrier_replace"},
		"k",
		"k",
		"fresh",
		10,
		nil,
	)
	require.NoError(t, err)
	require.True(t,
		ok)

	loader := func(context.Context, string) (Versioned[string], error) {
		require.Fail(t, "loader must not run after equal-version replacement")
		return Versioned[string]{}, nil
	}
	got, err := tier.GetConsistentVersioned(t.Context(), "k", 10, loader)
	require.NoError(t, err)
	require.False(t,
		got.Value != "fresh" ||
			got.
				Version !=
				10)
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
	require.NoError(t, err)

	reg.Handle(t.Context(), payload)
	reg.Handle(t.Context(), payload)

	l2.mu.Lock()
	stored := l2.values["k"]
	l2.mu.Unlock()
	require.False(t,
		!stored.Barrier ||
			stored.
				Version !=
				10)
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

	ok, err := l2.CompareAndSet(t.Context(), "k", cacheEntry[string]{Version: 10, Barrier: true}, time.Minute)
	require.False(t,
		err != nil || !ok)

	ok, err = l2.CompareAndSet(t.Context(), "k", cacheEntry[string]{Version: 10, Value: "fresh"}, time.Minute)
	require.False(t,
		err != nil || !ok)

	entry, err := l2.Get(t.Context(), "k")
	require.NoError(t, err)
	require.False(t,
		entry.Barrier ||
			entry.Value !=
				"fresh" ||
			entry.
				Version != 10)
}
