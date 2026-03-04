package pubsub

import (
	"context"
	"testing"
)

func TestSubscription_Close(t *testing.T) {
	var called bool
	ch := make(chan []byte)
	sub := &Subscription{
		Ch:     ch,
		cancel: func() { called = true },
	}

	sub.Close()
	if !called {
		t.Error("Close() did not invoke cancel function")
	}
}

func TestSubscription_Close_Idempotent(t *testing.T) {
	// Use a real context.CancelFunc which is safe to call multiple times.
	ctx, cancel := context.WithCancel(context.Background())
	_ = ctx

	ch := make(chan []byte)
	sub := &Subscription{
		Ch:     ch,
		cancel: cancel,
	}

	// Should not panic when called multiple times.
	sub.Close()
	sub.Close()
	sub.Close()
}

func TestSubscription_ChannelType(t *testing.T) {
	ch := make(chan []byte, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = ctx

	sub := &Subscription{
		Ch:     ch,
		cancel: cancel,
	}

	// Write to the underlying channel and read via the Subscription.
	ch <- []byte("test-message")

	got := <-sub.Ch
	if string(got) != "test-message" {
		t.Errorf("received %q, want %q", got, "test-message")
	}
}
