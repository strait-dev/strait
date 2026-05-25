package cache

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/pubsub"
)

type memoryBusPublisher struct {
	mu           sync.Mutex
	subscribers  map[string][]chan []byte
	subscribeErr error
	closed       bool
}

func newMemoryBusPublisher() *memoryBusPublisher {
	return &memoryBusPublisher{subscribers: make(map[string][]chan []byte)}
}

func (m *memoryBusPublisher) Publish(_ context.Context, channel string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, ch := range m.subscribers[channel] {
		select {
		case ch <- append([]byte(nil), data...):
		default:
		}
	}
	return nil
}

func (m *memoryBusPublisher) PublishBatch(ctx context.Context, messages []pubsub.PubSubMessage) error {
	for _, msg := range messages {
		if err := m.Publish(ctx, msg.Channel, msg.Data); err != nil {
			return err
		}
	}
	return nil
}

func (m *memoryBusPublisher) Subscribe(_ context.Context, channel string) (*pubsub.Subscription, error) {
	if m.subscribeErr != nil {
		return nil, m.subscribeErr
	}
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan []byte, 8)
	m.mu.Lock()
	m.subscribers[channel] = append(m.subscribers[channel], ch)
	m.mu.Unlock()
	go func() {
		<-ctx.Done()
		m.mu.Lock()
		defer m.mu.Unlock()
		for i, sub := range m.subscribers[channel] {
			if sub == ch {
				m.subscribers[channel] = append(m.subscribers[channel][:i], m.subscribers[channel][i+1:]...)
				break
			}
		}
		close(ch)
	}()
	return pubsub.NewSubscription(ch, cancel), nil
}

func (m *memoryBusPublisher) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func (m *memoryBusPublisher) subscriberCount(channel string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.subscribers[channel])
}

func TestBus_CrossReplicaInvalidateEvictsPeerL1(t *testing.T) {
	t.Parallel()

	publisher := newMemoryBusPublisher()
	peerTier := NewTier[string, string](TierConfig[string, string]{
		Name:        "bus_peer",
		MaximumSize: 10,
		TTL:         time.Minute,
	})
	if err := peerTier.Set(t.Context(), "job-1", "cached", 3); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	registryB := NewRegistry(RegistryConfig{Origin: "node-b"})
	registryB.Register("job", StringTierHandler[string]{Tier: peerTier})
	busB := NewBus(publisher, BusConfig{Origin: registryB.Origin()})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- busB.Run(ctx, registryB)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-errCh:
			if err != nil {
				t.Errorf("Run() error = %v", err)
			}
		case <-time.After(time.Second):
			t.Error("cachebus Run() did not stop")
		}
	})
	waitFor(t, time.Second, func() bool {
		return publisher.subscriberCount(DefaultBusChannel) == 1
	})

	busA := NewBus(publisher, BusConfig{Origin: "node-a"})
	if err := busA.PublishInvalidate(t.Context(), "job", "job-1", 4); err != nil {
		t.Fatalf("PublishInvalidate() error = %v", err)
	}

	waitFor(t, time.Second, func() bool {
		_, ok := peerTier.GetIfPresent("job-1")
		return !ok
	})
}

func TestBus_SelfOriginMessageDoesNotEvictOrigin(t *testing.T) {
	t.Parallel()

	tier := NewTier[string, string](TierConfig[string, string]{
		Name:        "self_origin",
		MaximumSize: 10,
		TTL:         time.Minute,
	})
	if err := tier.Set(t.Context(), "k", "cached", 1); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	var suppressed atomic.Int64
	registry := NewRegistry(RegistryConfig{
		Origin:       "node-a",
		OnSuppressed: func() { suppressed.Add(1) },
	})
	registry.Register("job", StringTierHandler[string]{Tier: tier})
	data, err := json.Marshal(BusMessage{
		Action:    BusActionInvalidate,
		Namespace: "job",
		Key:       "k",
		Version:   2,
		Origin:    "node-a",
		SentAt:    time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	registry.Handle(t.Context(), data)

	if suppressed.Load() != 1 {
		t.Fatalf("suppressed = %d, want 1", suppressed.Load())
	}
	got, ok := tier.GetIfPresent("k")
	if !ok || got != "cached" {
		t.Fatalf("origin cache entry = %q, %v; want cached,true", got, ok)
	}
}

func TestBus_DuplicateInvalidationMessagesAreIdempotent(t *testing.T) {
	t.Parallel()

	tier := NewTier[string, string](TierConfig[string, string]{
		Name:        "duplicate_invalidate",
		MaximumSize: 10,
		TTL:         time.Minute,
	})
	if err := tier.Set(t.Context(), "k", "cached", 1); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	registry := NewRegistry(RegistryConfig{Origin: "node-b"})
	registry.Register("job", StringTierHandler[string]{Tier: tier})
	data, err := json.Marshal(BusMessage{
		Action:    BusActionInvalidate,
		Namespace: "job",
		Key:       "k",
		Version:   2,
		Origin:    "node-a",
		SentAt:    time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	registry.Handle(t.Context(), data)
	registry.Handle(t.Context(), data)

	if _, ok := tier.GetIfPresent("k"); ok {
		t.Fatal("duplicate invalidations should leave the entry absent")
	}
}

func TestBus_UpdateMessageAppliesOnlyMonotonicVersion(t *testing.T) {
	t.Parallel()

	tier := NewTier[string, string](TierConfig[string, string]{
		Name:        "bus_update_monotonic",
		L2:          newFakeL2[string, string](),
		MaximumSize: 10,
		TTL:         time.Minute,
	})
	registry := NewRegistry(RegistryConfig{Origin: "node-b"})
	registry.Register("job", UpdatingStringTierHandler[string]{Tier: tier})

	registry.Handle(t.Context(), mustMarshalBusMessage(t, BusMessage{
		Action:    BusActionUpdate,
		Namespace: "job",
		Key:       "k",
		Version:   10,
		Origin:    "node-a",
		Payload:   mustMarshalRaw(t, cacheEntry[string]{Version: 10, Value: "new"}),
	}))
	got, ok := tier.GetIfPresent("k")
	if !ok || got != "new" {
		t.Fatalf("updated entry = %q, %v; want new,true", got, ok)
	}

	registry.Handle(t.Context(), mustMarshalBusMessage(t, BusMessage{
		Action:    BusActionUpdate,
		Namespace: "job",
		Key:       "k",
		Version:   9,
		Origin:    "node-a",
		Payload:   mustMarshalRaw(t, cacheEntry[string]{Version: 9, Value: "stale"}),
	}))
	got, ok = tier.GetIfPresent("k")
	if !ok || got != "new" {
		t.Fatalf("stale update moved cache backward: %q, %v", got, ok)
	}
}

func TestRegistry_BadNamespaceAndPayloadAreIgnoredAndCounted(t *testing.T) {
	t.Parallel()

	var invalid atomic.Int64
	var unknown atomic.Int64
	registry := NewRegistry(RegistryConfig{
		Origin:    "node-b",
		OnInvalid: func(string) { invalid.Add(1) },
		OnUnknown: func(string) { unknown.Add(1) },
	})

	registry.Handle(t.Context(), []byte("{"))
	registry.Handle(t.Context(), mustMarshalBusMessage(t, BusMessage{
		Action: BusActionInvalidate,
		Key:    "k",
		Origin: "node-a",
	}))
	registry.Handle(t.Context(), mustMarshalBusMessage(t, BusMessage{
		Action:    BusActionInvalidate,
		Namespace: "missing",
		Key:       "k",
		Origin:    "node-a",
	}))

	if invalid.Load() != 2 {
		t.Fatalf("invalid count = %d, want 2", invalid.Load())
	}
	if unknown.Load() != 1 {
		t.Fatalf("unknown count = %d, want 1", unknown.Load())
	}
}

func TestBus_SubscribeFailureFailsOpen(t *testing.T) {
	t.Parallel()

	publisher := newMemoryBusPublisher()
	publisher.subscribeErr = errors.New("redis down")
	bus := NewBus(publisher, BusConfig{Origin: "node-a"})

	if err := bus.Run(t.Context(), NewRegistry(RegistryConfig{Origin: "node-a"})); err != nil {
		t.Fatalf("Run() error = %v, want nil fail-open", err)
	}
}

func TestBus_ClosedSubscriptionStopsWithoutPanic(t *testing.T) {
	t.Parallel()

	ch := make(chan []byte)
	close(ch)
	publisher := &closedSubPublisher{sub: pubsub.NewSubscription(ch, func() {})}
	bus := NewBus(publisher, BusConfig{Origin: "node-a"})

	if err := bus.Run(t.Context(), NewRegistry(RegistryConfig{Origin: "node-a"})); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

type closedSubPublisher struct {
	sub *pubsub.Subscription
}

func (c *closedSubPublisher) Publish(context.Context, string, []byte) error {
	return nil
}

func (c *closedSubPublisher) PublishBatch(context.Context, []pubsub.PubSubMessage) error {
	return nil
}

func (c *closedSubPublisher) Subscribe(context.Context, string) (*pubsub.Subscription, error) {
	return c.sub, nil
}

func (c *closedSubPublisher) Close() error {
	return nil
}

func mustMarshalBusMessage(t *testing.T, msg BusMessage) []byte {
	t.Helper()
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	return data
}

func mustMarshalRaw(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	return data
}

func waitFor(t *testing.T, timeout time.Duration, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition was not met before timeout")
}
