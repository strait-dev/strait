package pubsub

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResilientPublisher_Publish_NilPublisher(t *testing.T) {
	t.Parallel()

	rp := NewResilientPublisher(nil, slog.Default(), 2)
	require.NoError(
		t, rp.Publish(t.Context(), "events",

			[]byte("payload")))
	require.NoError(
		t, rp.Publish(t.Context(), "events",

			[]byte("payload")))
	require.False(t,
		rp.IsHealthy())

	// Publish with nil publisher should fail open (no error returned).

	// Should accumulate failures.
}

func TestResilientPublisher_Publish_Success(t *testing.T) {
	t.Parallel()

	rp := NewResilientPublisher(
		&mockPublisher{publishFunc: func(context.Context, string, []byte) error {
			return nil
		}},
		slog.Default(),
		2,
	)
	require.NoError(
		t, rp.Publish(t.Context(), "events",

			[]byte("payload")))
	require.True(t,
		rp.IsHealthy())
}

func TestResilientPublisher_Close_NilPublisher(t *testing.T) {
	t.Parallel()

	rp := NewResilientPublisher(nil, slog.Default(), 1)
	require.NoError(
		t, rp.Close())
	require.False(t,
		rp.IsHealthy())
}

func TestResilientPublisher_Close_Success(t *testing.T) {
	t.Parallel()

	rp := NewResilientPublisher(
		&mockPublisher{closeFunc: func() error { return nil }},
		slog.Default(),
		3,
	)
	require.NoError(
		t, rp.Close())
	require.True(t,
		rp.IsHealthy())
}

func TestResilientPublisher_Close_Error(t *testing.T) {
	t.Parallel()

	rp := NewResilientPublisher(
		&mockPublisher{closeFunc: func() error { return errors.New("close failed") }},
		slog.Default(),
		1,
	)
	require.NoError(
		t, rp.Close())
	require.False(t,
		rp.IsHealthy())
}

func TestResilientPublisher_Ping_Success(t *testing.T) {
	t.Parallel()

	rp := NewResilientPublisher(
		&mockPublisher{pingFunc: func(context.Context) error { return nil }},
		slog.Default(),
		3,
	)
	require.NoError(
		t, rp.Ping(t.Context()))
	require.True(t,
		rp.IsHealthy())
}

func TestResilientPublisher_Subscribe_NilPublisher(t *testing.T) {
	t.Parallel()

	rp := NewResilientPublisher(nil, slog.Default(), 1)

	sub, err := rp.Subscribe(t.Context(), "events")
	require.NoError(
		t, err)
	require.NotNil(t,
		sub)

	// Channel should be closed.
	select {
	case _, ok := <-sub.Ch:
		require.False(t, ok)
	default:
		require.FailNow(t, "subscription channel read blocked, want closed channel")
	}
}

func TestResilientPublisher_Subscribe_Success(t *testing.T) {
	t.Parallel()

	ch := make(chan []byte, 1)
	rp := NewResilientPublisher(
		&mockPublisher{subscribeFunc: func(context.Context, string) (*Subscription, error) {
			return NewSubscription(ch, func() {}), nil
		}},
		slog.Default(),
		3,
	)

	sub, err := rp.Subscribe(t.Context(), "events")
	require.NoError(
		t, err)
	require.NotNil(t,
		sub)
	require.True(t,
		rp.IsHealthy())
}

func TestResilientPublisher_DefaultThreshold_ZeroFallback(t *testing.T) {
	t.Parallel()

	// Zero threshold should default to 3.
	rp := NewResilientPublisher(
		&mockPublisher{publishFunc: func(context.Context, string, []byte) error {
			return errors.New("fail")
		}},
		nil, // nil logger should default to slog.Default()
		0,
	)

	// First two failures should not degrade.
	_ = rp.Publish(t.Context(), "ch", []byte("a"))
	require.True(t,
		rp.IsHealthy())

	_ = rp.Publish(t.Context(), "ch", []byte("b"))
	require.True(t,
		rp.IsHealthy())

	// Third failure should degrade.
	_ = rp.Publish(t.Context(), "ch", []byte("c"))
	require.False(t,
		rp.IsHealthy())
}

func TestResilientPublisher_NegativeThreshold(t *testing.T) {
	t.Parallel()

	// Negative threshold should also default to 3.
	rp := NewResilientPublisher(
		&mockPublisher{publishFunc: func(context.Context, string, []byte) error {
			return errors.New("fail")
		}},
		slog.Default(),
		-5,
	)

	_ = rp.Publish(t.Context(), "ch", []byte("a"))
	_ = rp.Publish(t.Context(), "ch", []byte("b"))
	require.True(t,
		rp.IsHealthy())

	_ = rp.Publish(t.Context(), "ch", []byte("c"))
	require.False(t,
		rp.IsHealthy())
}

func TestResilientPublisher_PublishBatch_EmptySlice(t *testing.T) {
	t.Parallel()

	var published bool
	rp := NewResilientPublisher(
		&mockPublisher{publishFunc: func(context.Context, string, []byte) error {
			published = true
			return nil
		}},
		slog.Default(),
		3,
	)
	require.NoError(
		t, rp.PublishBatch(t.Context(), nil),
	)
	assert.False(t,
		published)
}

func TestResilientPublisher_PublishBatch_NilPublisherDegrades(t *testing.T) {
	t.Parallel()

	rp := NewResilientPublisher(nil, slog.Default(), 1)

	err := rp.PublishBatch(t.Context(), []PubSubMessage{
		{Channel: "ch", Data: []byte("data")},
	})
	require.NoError(
		t, err)
	require.False(t,
		rp.IsHealthy())
}

func TestResilientPublisher_PublishBatch_RecoveryAfterFailure(t *testing.T) {
	t.Parallel()

	var calls int
	rp := NewResilientPublisher(
		&mockPublisher{
			publishFunc: func(context.Context, string, []byte) error {
				calls++
				if calls <= 2 {
					return errors.New("fail")
				}
				return nil
			},
		},
		slog.Default(),
		1,
	)

	// First batch fails.
	_ = rp.PublishBatch(t.Context(), []PubSubMessage{
		{Channel: "ch", Data: []byte("a")},
	})
	require.False(t,
		rp.IsHealthy())

	// Second batch succeeds (calls > 2).
	calls = 3
	_ = rp.PublishBatch(t.Context(), []PubSubMessage{
		{Channel: "ch", Data: []byte("b")},
	})
	require.True(t,
		rp.IsHealthy())
}
