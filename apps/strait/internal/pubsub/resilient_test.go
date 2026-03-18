package pubsub

import (
	"context"
	"errors"
	"log/slog"
	"testing"
)

type mockPublisher struct {
	publishFunc   func(ctx context.Context, channel string, data []byte) error
	subscribeFunc func(ctx context.Context, channel string) (*Subscription, error)
	closeFunc     func() error
	pingFunc      func(ctx context.Context) error
}

func (m *mockPublisher) Publish(ctx context.Context, channel string, data []byte) error {
	if m.publishFunc == nil {
		return nil
	}
	return m.publishFunc(ctx, channel, data)
}

func (m *mockPublisher) Subscribe(ctx context.Context, channel string) (*Subscription, error) {
	if m.subscribeFunc == nil {
		ch := make(chan []byte)
		return NewSubscription(ch, func() {}), nil
	}
	return m.subscribeFunc(ctx, channel)
}

func (m *mockPublisher) Close() error {
	if m.closeFunc == nil {
		return nil
	}
	return m.closeFunc()
}

func (m *mockPublisher) PublishBatch(ctx context.Context, messages []PubSubMessage) error {
	for _, msg := range messages {
		if err := m.Publish(ctx, msg.Channel, msg.Data); err != nil {
			return err
		}
	}
	return nil
}

func (m *mockPublisher) Ping(ctx context.Context) error {
	if m.pingFunc == nil {
		return nil
	}
	return m.pingFunc(ctx)
}

type mockPublisherNoPing struct {
	publishFunc   func(ctx context.Context, channel string, data []byte) error
	subscribeFunc func(ctx context.Context, channel string) (*Subscription, error)
	closeFunc     func() error
}

func (m *mockPublisherNoPing) Publish(ctx context.Context, channel string, data []byte) error {
	if m.publishFunc == nil {
		return nil
	}
	return m.publishFunc(ctx, channel, data)
}

func (m *mockPublisherNoPing) Subscribe(ctx context.Context, channel string) (*Subscription, error) {
	if m.subscribeFunc == nil {
		ch := make(chan []byte)
		return NewSubscription(ch, func() {}), nil
	}
	return m.subscribeFunc(ctx, channel)
}

func (m *mockPublisherNoPing) PublishBatch(ctx context.Context, messages []PubSubMessage) error {
	for _, msg := range messages {
		if err := m.Publish(ctx, msg.Channel, msg.Data); err != nil {
			return err
		}
	}
	return nil
}

func (m *mockPublisherNoPing) Close() error {
	if m.closeFunc == nil {
		return nil
	}
	return m.closeFunc()
}

func TestResilientPublisher_PublishFailOpenAndDegrade(t *testing.T) {
	t.Parallel()

	rp := NewResilientPublisher(
		&mockPublisher{publishFunc: func(context.Context, string, []byte) error {
			return errors.New("redis down")
		}},
		slog.Default(),
		2,
	)

	if err := rp.Publish(t.Context(), "events", []byte("payload")); err != nil {
		t.Fatalf("Publish() error = %v, want nil", err)
	}
	if !rp.IsHealthy() {
		t.Fatal("publisher unhealthy after first failure, want healthy")
	}

	if err := rp.Publish(t.Context(), "events", []byte("payload")); err != nil {
		t.Fatalf("Publish() error = %v, want nil", err)
	}
	if rp.IsHealthy() {
		t.Fatal("publisher healthy after threshold failures, want degraded")
	}
}

func TestResilientPublisher_SubscribeFailOpenReturnsClosedChannel(t *testing.T) {
	t.Parallel()

	rp := NewResilientPublisher(
		&mockPublisher{subscribeFunc: func(context.Context, string) (*Subscription, error) {
			return nil, errors.New("redis unavailable")
		}},
		slog.Default(),
		1,
	)

	sub, err := rp.Subscribe(t.Context(), "events")
	if err != nil {
		t.Fatalf("Subscribe() error = %v, want nil", err)
	}
	if sub == nil {
		t.Fatal("Subscribe() returned nil subscription")
	}

	select {
	case _, ok := <-sub.Ch:
		if ok {
			t.Fatal("subscription channel open, want closed")
		}
	default:
		t.Fatal("subscription channel read blocked, want closed channel")
	}

	if rp.IsHealthy() {
		t.Fatal("publisher healthy after failure at threshold=1, want degraded")
	}
}

func TestResilientPublisher_RecoveryAfterSuccessResetsFailures(t *testing.T) {
	t.Parallel()

	var calls int
	rp := NewResilientPublisher(
		&mockPublisher{publishFunc: func(context.Context, string, []byte) error {
			calls++
			switch calls {
			case 1, 2, 4:
				return errors.New("redis down")
			default:
				return nil
			}
		}},
		slog.Default(),
		2,
	)

	_ = rp.Publish(t.Context(), "events", []byte("first"))
	_ = rp.Publish(t.Context(), "events", []byte("second"))
	if rp.IsHealthy() {
		t.Fatal("publisher healthy after two failures, want degraded")
	}

	_ = rp.Publish(t.Context(), "events", []byte("recover"))
	if !rp.IsHealthy() {
		t.Fatal("publisher degraded after successful publish, want healthy")
	}

	_ = rp.Publish(t.Context(), "events", []byte("post-recovery"))
	if !rp.IsHealthy() {
		t.Fatal("publisher degraded after one failure post-recovery, want healthy")
	}
}

func TestResilientPublisher_PingDelegatesAndReturnsSentinel(t *testing.T) {
	t.Parallel()

	innerErr := errors.New("dial tcp timeout")
	rp := NewResilientPublisher(
		&mockPublisher{pingFunc: func(context.Context) error {
			return innerErr
		}},
		slog.Default(),
		3,
	)

	err := rp.Ping(t.Context())
	if err == nil {
		t.Fatal("Ping() error = nil, want error")
	}
	if !errors.Is(err, ErrRedisUnavailable) {
		t.Fatalf("Ping() error %v does not match ErrRedisUnavailable", err)
	}
	if !errors.Is(err, innerErr) {
		t.Fatalf("Ping() error %v does not include underlying error", err)
	}
}

func TestResilientPublisher_PingWithoutUnderlyingSupport(t *testing.T) {
	t.Parallel()

	rp := NewResilientPublisher(&mockPublisherNoPing{}, slog.Default(), 3)

	err := rp.Ping(t.Context())
	if !errors.Is(err, ErrRedisUnavailable) {
		t.Fatalf("Ping() error = %v, want ErrRedisUnavailable", err)
	}
}
