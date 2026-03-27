package pubsub

import (
	"context"
	"errors"
	"log/slog"
	"testing"
)

func TestResilientPublisher_Publish_NilPublisher(t *testing.T) {
	t.Parallel()

	rp := NewResilientPublisher(nil, slog.Default(), 2)

	// Publish with nil publisher should fail open (no error returned).
	if err := rp.Publish(t.Context(), "events", []byte("payload")); err != nil {
		t.Fatalf("Publish() error = %v, want nil", err)
	}

	// Should accumulate failures.
	if err := rp.Publish(t.Context(), "events", []byte("payload")); err != nil {
		t.Fatalf("Publish() error = %v, want nil", err)
	}
	if rp.IsHealthy() {
		t.Fatal("publisher healthy after threshold failures with nil publisher, want degraded")
	}
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

	if err := rp.Publish(t.Context(), "events", []byte("payload")); err != nil {
		t.Fatalf("Publish() error = %v, want nil", err)
	}
	if !rp.IsHealthy() {
		t.Fatal("publisher unhealthy after successful publish")
	}
}

func TestResilientPublisher_Close_NilPublisher(t *testing.T) {
	t.Parallel()

	rp := NewResilientPublisher(nil, slog.Default(), 1)

	if err := rp.Close(); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}
	if rp.IsHealthy() {
		t.Fatal("publisher healthy after close failure with nil publisher, want degraded")
	}
}

func TestResilientPublisher_Close_Success(t *testing.T) {
	t.Parallel()

	rp := NewResilientPublisher(
		&mockPublisher{closeFunc: func() error { return nil }},
		slog.Default(),
		3,
	)

	if err := rp.Close(); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}
	if !rp.IsHealthy() {
		t.Fatal("publisher unhealthy after successful close")
	}
}

func TestResilientPublisher_Close_Error(t *testing.T) {
	t.Parallel()

	rp := NewResilientPublisher(
		&mockPublisher{closeFunc: func() error { return errors.New("close failed") }},
		slog.Default(),
		1,
	)

	if err := rp.Close(); err != nil {
		t.Fatalf("Close() error = %v, want nil (fail-open)", err)
	}
	if rp.IsHealthy() {
		t.Fatal("publisher healthy after close failure, want degraded")
	}
}

func TestResilientPublisher_Ping_Success(t *testing.T) {
	t.Parallel()

	rp := NewResilientPublisher(
		&mockPublisher{pingFunc: func(context.Context) error { return nil }},
		slog.Default(),
		3,
	)

	if err := rp.Ping(t.Context()); err != nil {
		t.Fatalf("Ping() error = %v, want nil", err)
	}
	if !rp.IsHealthy() {
		t.Fatal("publisher unhealthy after successful ping")
	}
}

func TestResilientPublisher_Subscribe_NilPublisher(t *testing.T) {
	t.Parallel()

	rp := NewResilientPublisher(nil, slog.Default(), 1)

	sub, err := rp.Subscribe(t.Context(), "events")
	if err != nil {
		t.Fatalf("Subscribe() error = %v, want nil", err)
	}
	if sub == nil {
		t.Fatal("Subscribe() returned nil subscription")
	}

	// Channel should be closed.
	select {
	case _, ok := <-sub.Ch:
		if ok {
			t.Fatal("subscription channel open, want closed")
		}
	default:
		t.Fatal("subscription channel read blocked, want closed channel")
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
	if err != nil {
		t.Fatalf("Subscribe() error = %v, want nil", err)
	}
	if sub == nil {
		t.Fatal("Subscribe() returned nil subscription")
	}
	if !rp.IsHealthy() {
		t.Fatal("publisher unhealthy after successful subscribe")
	}
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
	if !rp.IsHealthy() {
		t.Fatal("degraded after 1 failure, want healthy (threshold=3)")
	}
	_ = rp.Publish(t.Context(), "ch", []byte("b"))
	if !rp.IsHealthy() {
		t.Fatal("degraded after 2 failures, want healthy (threshold=3)")
	}

	// Third failure should degrade.
	_ = rp.Publish(t.Context(), "ch", []byte("c"))
	if rp.IsHealthy() {
		t.Fatal("healthy after 3 failures, want degraded (threshold=3)")
	}
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
	if !rp.IsHealthy() {
		t.Fatal("degraded after 2 failures, want healthy (threshold defaults to 3)")
	}
	_ = rp.Publish(t.Context(), "ch", []byte("c"))
	if rp.IsHealthy() {
		t.Fatal("healthy after 3 failures, want degraded")
	}
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

	if err := rp.PublishBatch(t.Context(), nil); err != nil {
		t.Fatalf("PublishBatch(nil) error = %v, want nil", err)
	}
	if published {
		t.Error("underlying publisher called for empty batch")
	}
}

func TestResilientPublisher_PublishBatch_NilPublisherDegrades(t *testing.T) {
	t.Parallel()

	rp := NewResilientPublisher(nil, slog.Default(), 1)

	err := rp.PublishBatch(t.Context(), []PubSubMessage{
		{Channel: "ch", Data: []byte("data")},
	})
	if err != nil {
		t.Fatalf("PublishBatch() error = %v, want nil", err)
	}
	if rp.IsHealthy() {
		t.Fatal("publisher healthy after nil publisher batch, want degraded")
	}
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
	if rp.IsHealthy() {
		t.Fatal("expected degraded after failed batch")
	}

	// Second batch succeeds (calls > 2).
	calls = 3
	_ = rp.PublishBatch(t.Context(), []PubSubMessage{
		{Channel: "ch", Data: []byte("b")},
	})
	if !rp.IsHealthy() {
		t.Fatal("expected healthy after successful batch")
	}
}
