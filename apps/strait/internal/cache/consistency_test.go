package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func versionedStringLoader(value string, version int64) VersionedLoadFunc[string, string] {
	return func(context.Context, string) (Versioned[string], error) {
		return Versioned[string]{Value: value, Version: version}, nil
	}
}

func TestStrictConsistency_VersionedLoaderPreservesDatabaseVersion(t *testing.T) {
	t.Parallel()

	l2 := newFakeL2[string, string]()
	tier := NewTier[string, string](TierConfig[string, string]{
		Name:        "versioned_loader",
		L2:          l2,
		MaximumSize: 10,
		TTL:         time.Minute,
	})

	got, err := tier.GetConsistentVersioned(t.Context(), "k", 10, versionedStringLoader("db", 12))
	require.NoError(t, err)
	require.False(t,
		got.Value != "db" ||
			got.
				Version !=
				12)

	l2.mu.Lock()
	stored := l2.values["k"]
	l2.mu.Unlock()
	require.Equal(t, int64(12), stored.Version)
}

func TestStrictConsistency_RacingStaleReaderCannotOverwriteNewerWriter(t *testing.T) {
	t.Parallel()

	l2 := newFakeL2[string, string]()
	tier := NewTier[string, string](TierConfig[string, string]{
		Name:        "stale_reader",
		L2:          l2,
		MaximumSize: 10,
		TTL:         time.Minute,
	})
	var rejects atomic.Int64
	tier.cfg.OnCASRejected = func() { rejects.Add(1) }
	loader := func(context.Context, string) (Versioned[string], error) {
		l2.mu.Lock()
		l2.values["k"] = cacheEntry[string]{Version: 20, Value: "writer"}
		l2.mu.Unlock()
		return Versioned[string]{Value: "stale-reader", Version: 19}, nil
	}
	got, err := tier.GetConsistentVersioned(t.Context(), "k", 10, loader)
	require.NoError(t, err)
	require.False(t,
		got.Value != "writer" ||
			got.Version !=
				20)

	l2.mu.Lock()
	stored := l2.values["k"]
	l2.mu.Unlock()
	require.False(t,
		stored.Value !=
			"writer" ||
			stored.
				Version !=
				20)
	require.Equal(t, int64(1), rejects.Load())
}

func TestStrictConsistency_GetConsistentVersionedRejectsLoaderBelowMinVersion(t *testing.T) {
	t.Parallel()

	tier := NewTier[string, string](TierConfig[string, string]{
		Name:        "loader_below_min",
		L2:          newFakeL2[string, string](),
		MaximumSize: 10,
		TTL:         time.Minute,
	})

	_, err := tier.GetConsistentVersioned(t.Context(), "k", 20, versionedStringLoader("db", 19))
	require.ErrorIs(t,
		err, ErrStaleVersion)
}

func TestStrictConsistency_WriteThroughCASAndPublishesUpdate(t *testing.T) {
	t.Parallel()

	publisher := newMemoryBusPublisher()
	bus := NewBus(publisher, BusConfig{Origin: "node-a"})
	l2 := newFakeL2[string, string]()
	tier := NewTier[string, string](TierConfig[string, string]{
		Name:        "write_through",
		L2:          l2,
		MaximumSize: 10,
		TTL:         time.Minute,
	})
	sub, err := publisher.Subscribe(t.Context(), bus.Channel())
	require.NoError(t, err)

	defer sub.Close()

	ok, err := tier.WriteThrough(t.Context(), "k", "value", 4, bus, "ns", "k")
	require.NoError(t, err)
	require.True(t,
		ok)

	select {
	case data := <-sub.Ch:
		var msg BusMessage
		require.NoError(t, json.Unmarshal(data, &msg))
		require.Equal(t, BusActionUpdate, msg.Action)
		require.Equal(t, "ns", msg.Namespace)
		require.Equal(t, "k", msg.Key)
		require.Equal(t, int64(4), msg.Version)
		var entry cacheEntry[string]
		require.NoError(t, json.Unmarshal(msg.Payload, &entry))
		require.Equal(t, "value", entry.Value)
		require.Equal(t, int64(4), entry.Version)
	case <-time.After(time.Second):
		require.FailNow(t, "timed out waiting for bus update")
	}
}

func TestStrictConsistency_ConcurrentVersionedLoadsCoalesce(t *testing.T) {
	t.Parallel()

	tier := NewTier[string, string](TierConfig[string, string]{
		Name:        "versioned_singleflight",
		L2:          newFakeL2[string, string](),
		MaximumSize: 10,
		TTL:         time.Minute,
	})
	var loads atomic.Int64
	start := make(chan struct{})
	const callers = 24
	var wg sync.WaitGroup
	errs := make(chan error, callers)
	for range callers {
		wg.Go(func() {
			<-start
			loader := func(context.Context, string) (Versioned[string], error) {
				loads.Add(1)
				time.Sleep(10 * time.Millisecond)
				return Versioned[string]{Value: "db", Version: 3}, nil
			}
			got, err := tier.GetConsistentVersioned(t.Context(), "k", 2, loader)
			if err != nil {
				errs <- err
				return
			}
			if got.Value != "db" || got.Version != 3 {
				errs <- errUnexpectedVersioned(got, "db", 3)
			}
		})
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	require.Equal(t, int64(1), loads.Load())
}

func errUnexpectedVersioned[V comparable](got Versioned[V], wantValue V, wantVersion int64) error {
	return fmt.Errorf("got %+v, want %v@%d", got, wantValue, wantVersion)
}
