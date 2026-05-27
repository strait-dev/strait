package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestStrictConsistency_VersionedLoaderPreservesDatabaseVersion(t *testing.T) {
	t.Parallel()

	l2 := newFakeL2[string, string]()
	tier := NewTier[string, string](TierConfig[string, string]{
		Name:        "versioned_loader",
		L2:          l2,
		MaximumSize: 10,
		TTL:         time.Minute,
	})

	got, err := tier.GetConsistentVersioned(t.Context(), "k", 10, func(context.Context, string) (Versioned[string], error) {
		return Versioned[string]{Value: "db", Version: 12}, nil
	})
	if err != nil {
		t.Fatalf("GetConsistentVersioned() error = %v", err)
	}
	if got.Value != "db" || got.Version != 12 {
		t.Fatalf("GetConsistentVersioned() = %+v, want db@12", got)
	}
	l2.mu.Lock()
	stored := l2.values["k"]
	l2.mu.Unlock()
	if stored.Version != 12 {
		t.Fatalf("stored version = %d, want 12", stored.Version)
	}
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
	got, err := tier.GetConsistentVersioned(t.Context(), "k", 10, func(context.Context, string) (Versioned[string], error) {
		l2.mu.Lock()
		l2.values["k"] = cacheEntry[string]{Version: 20, Value: "writer"}
		l2.mu.Unlock()
		return Versioned[string]{Value: "stale-reader", Version: 19}, nil
	})
	if err != nil {
		t.Fatalf("GetConsistentVersioned() error = %v", err)
	}
	if got.Value != "writer" || got.Version != 20 {
		t.Fatalf("GetConsistentVersioned() = %+v, want writer@20", got)
	}
	l2.mu.Lock()
	stored := l2.values["k"]
	l2.mu.Unlock()
	if stored.Value != "writer" || stored.Version != 20 {
		t.Fatalf("stored entry = %+v, want writer@20", stored)
	}
	if rejects.Load() != 1 {
		t.Fatalf("CAS rejects = %d, want 1", rejects.Load())
	}
}

func TestStrictConsistency_GetConsistentVersionedRejectsLoaderBelowMinVersion(t *testing.T) {
	t.Parallel()

	tier := NewTier[string, string](TierConfig[string, string]{
		Name:        "loader_below_min",
		L2:          newFakeL2[string, string](),
		MaximumSize: 10,
		TTL:         time.Minute,
	})

	_, err := tier.GetConsistentVersioned(t.Context(), "k", 20, func(context.Context, string) (Versioned[string], error) {
		return Versioned[string]{Value: "db", Version: 19}, nil
	})
	if !errors.Is(err, ErrStaleVersion) {
		t.Fatalf("GetConsistentVersioned() error = %v, want ErrStaleVersion", err)
	}
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
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer sub.Close()

	ok, err := tier.WriteThrough(t.Context(), "k", "value", 4, bus, "ns", "k")
	if err != nil {
		t.Fatalf("WriteThrough() error = %v", err)
	}
	if !ok {
		t.Fatal("WriteThrough() ok = false, want true")
	}

	select {
	case data := <-sub.Ch:
		var msg BusMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			t.Fatalf("unmarshal bus message: %v", err)
		}
		if msg.Action != BusActionUpdate || msg.Namespace != "ns" || msg.Key != "k" || msg.Version != 4 {
			t.Fatalf("bus message = %+v, want update ns/k@4", msg)
		}
		var entry cacheEntry[string]
		if err := json.Unmarshal(msg.Payload, &entry); err != nil {
			t.Fatalf("unmarshal update payload: %v", err)
		}
		if entry.Value != "value" || entry.Version != 4 {
			t.Fatalf("update payload = %+v, want value@4", entry)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for bus update")
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
			got, err := tier.GetConsistentVersioned(t.Context(), "k", 2, func(context.Context, string) (Versioned[string], error) {
				loads.Add(1)
				time.Sleep(10 * time.Millisecond)
				return Versioned[string]{Value: "db", Version: 3}, nil
			})
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
		t.Fatal(err)
	}
	if loads.Load() != 1 {
		t.Fatalf("loader calls = %d, want 1", loads.Load())
	}
}

func errUnexpectedVersioned[V comparable](got Versioned[V], wantValue V, wantVersion int64) error {
	return fmt.Errorf("got %+v, want %v@%d", got, wantValue, wantVersion)
}
