package pubsub

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
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
	require.NoError(
		t, rp.Publish(t.Context(), "events",

			[]byte("payload")))
	require.True(t,
		rp.IsHealthy())
	require.NoError(
		t, rp.Publish(t.Context(), "events",

			[]byte("payload")))
	require.False(t,
		rp.IsHealthy())

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
	require.NoError(
		t, err)
	require.NotNil(t,
		sub)

	select {
	case _, ok := <-sub.Ch:
		require.False(t, ok)
	default:
		require.FailNow(t, "subscription channel read blocked, want closed channel")
	}
	require.False(t,
		rp.IsHealthy())

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
	require.False(t,
		rp.IsHealthy())

	_ = rp.Publish(t.Context(), "events", []byte("recover"))
	require.True(t,
		rp.IsHealthy())

	_ = rp.Publish(t.Context(), "events", []byte("post-recovery"))
	require.True(t,
		rp.IsHealthy())

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
	require.Error(t,
		err)
	require.True(t,
		errors.Is(
			err, ErrRedisUnavailable,
		),
	)
	require.True(t,
		errors.Is(
			err, innerErr,
		))

}

func TestResilientPublisher_PingWithoutUnderlyingSupport(t *testing.T) {
	t.Parallel()

	rp := NewResilientPublisher(&mockPublisherNoPing{}, slog.Default(), 3)

	err := rp.Ping(t.Context())
	require.True(t,
		errors.Is(
			err, ErrRedisUnavailable,
		),
	)

}
