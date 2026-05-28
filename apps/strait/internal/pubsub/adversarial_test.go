package pubsub

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/sourcegraph/conc"
)

// Helpers

// countingPublisher tracks call counts and allows configurable failures.
type countingPublisher struct {
	publishCount   atomic.Int64
	subscribeCount atomic.Int64
	closeCount     atomic.Int64

	publishErr   error
	subscribeErr error
	closeErr     error
}

func (c *countingPublisher) Publish(_ context.Context, _ string, _ []byte) error {
	c.publishCount.Add(1)
	return c.publishErr
}

func (c *countingPublisher) PublishBatch(ctx context.Context, messages []PubSubMessage) error {
	for _, msg := range messages {
		if err := c.Publish(ctx, msg.Channel, msg.Data); err != nil {
			return err
		}
	}
	return nil
}

func (c *countingPublisher) Subscribe(_ context.Context, _ string) (*Subscription, error) {
	c.subscribeCount.Add(1)
	if c.subscribeErr != nil {
		return nil, c.subscribeErr
	}
	ch := make(chan []byte)
	return NewSubscription(ch, func() {}), nil
}

func (c *countingPublisher) Close() error {
	c.closeCount.Add(1)
	return c.closeErr
}

func (c *countingPublisher) Ping(_ context.Context) error {
	return nil
}

// 1. Message ordering violations (via Subscription channel semantics)

func TestSubscription_MessageOrdering_FIFO(t *testing.T) {
	t.Parallel()

	ch := make(chan []byte, 100)
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	sub := NewSubscription(ch, cancel)

	// Push messages in order.
	const count = 100
	for i := range count {
		ch <- fmt.Appendf(nil, "msg-%03d", i)
	}

	// Read them back and verify ordering.
	for i := range count {
		msg := <-sub.Ch
		expected := fmt.Sprintf("msg-%03d", i)
		if string(msg) != expected {
			t.Fatalf("message %d: got %q, want %q", i, msg, expected)
		}
	}
}

func TestSubscription_MessageOrdering_UnderConcurrentWrites(t *testing.T) {
	t.Parallel()

	ch := make(chan []byte, 1000)
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	sub := NewSubscription(ch, cancel)

	// Multiple producers write to the channel concurrently.
	const producers = 10
	const perProducer = 100
	var wg conc.WaitGroup

	for p := range producers {
		wg.Go(func() {
			for i := range perProducer {
				ch <- fmt.Appendf(nil, "p%d-msg-%03d", p, i)
			}
		})
	}

	// Read all messages concurrently.
	received := make(map[string]bool)
	var mu sync.Mutex
	var readerWg conc.WaitGroup
	readerWg.Go(func() {
		for range producers * perProducer {
			msg := <-sub.Ch
			mu.Lock()
			received[string(msg)] = true
			mu.Unlock()
		}
	})

	wg.Wait()
	readerWg.Wait()

	mu.Lock()
	defer mu.Unlock()

	// Verify all messages arrived (no duplicates or losses).
	if len(received) != producers*perProducer {
		t.Errorf("received %d unique messages, want %d", len(received), producers*perProducer)
	}
}

// 2. Duplicate message delivery (ResilientPublisher deduplication semantics)

func TestResilientPublisher_NoDuplicatePublishOnSuccess(t *testing.T) {
	t.Parallel()

	cp := &countingPublisher{}
	rp := NewResilientPublisher(cp, slog.Default(), 3)

	const iterations = 100
	for i := range iterations {
		if err := rp.Publish(t.Context(), "events", fmt.Appendf(nil, "msg-%d", i)); err != nil {
			t.Fatalf("Publish failed: %v", err)
		}
	}

	if cp.publishCount.Load() != iterations {
		t.Errorf("publish count = %d, want %d (no duplicates)", cp.publishCount.Load(), iterations)
	}
}

func TestResilientPublisher_PublishBatch_NoDuplicateOnFailure(t *testing.T) {
	t.Parallel()

	// When the underlying batch fails, resilient publisher should call it
	// exactly once (no implicit retry causing duplicates).
	var calls atomic.Int64
	mock := &mockPublisher{
		publishFunc: func(_ context.Context, _ string, _ []byte) error {
			calls.Add(1)
			return errors.New("redis down")
		},
	}

	rp := NewResilientPublisher(mock, slog.Default(), 3)
	msgs := []PubSubMessage{
		{Channel: "ch1", Data: []byte("a")},
		{Channel: "ch2", Data: []byte("b")},
	}

	_ = rp.PublishBatch(context.Background(), msgs)

	// The mock's PublishBatch iterates over each message, so it should call
	// publishFunc once for ch1 and then fail (stopping the batch).
	if calls.Load() != 1 {
		t.Errorf("publish calls = %d, want 1 (fail fast, no duplication)", calls.Load())
	}
}

// 3. Subscriber crash/disconnect during message processing

func TestSubscription_CloseWhileWriting(t *testing.T) {
	t.Parallel()

	ch := make(chan []byte, 10)
	_, cancel := context.WithCancel(context.Background())
	sub := NewSubscription(ch, cancel)

	// Write some messages.
	for i := range 5 {
		ch <- fmt.Appendf(nil, "msg-%d", i)
	}

	// Close while messages are buffered -- must not panic.
	sub.Close()

	// Drain buffered messages via non-blocking reads.
	drained := 0
	for {
		select {
		case _, ok := <-sub.Ch:
			if !ok {
				// Channel was closed.
				goto done
			}
			drained++
		default:
			goto done
		}
	}
done:
	if drained > 5 {
		t.Errorf("drained %d messages, want at most 5", drained)
	}
}

func TestResilientPublisher_SubscribeFailure_ReturnClosedChannel(t *testing.T) {
	t.Parallel()

	mock := &mockPublisher{
		subscribeFunc: func(_ context.Context, _ string) (*Subscription, error) {
			return nil, errors.New("connection refused")
		},
	}

	rp := NewResilientPublisher(mock, slog.Default(), 1)

	sub, err := rp.Subscribe(t.Context(), "events")
	if err != nil {
		t.Fatalf("Subscribe() error = %v, want nil (fail-open)", err)
	}

	// The returned subscription should have a closed channel.
	select {
	case _, ok := <-sub.Ch:
		if ok {
			t.Fatal("expected closed channel on failed subscription")
		}
	default:
		t.Fatal("expected closed channel to be immediately readable")
	}
}

func TestResilientPublisher_NilPublisher_SubscribeReturnsClosedChannel(t *testing.T) {
	t.Parallel()

	rp := NewResilientPublisher(nil, slog.Default(), 3)

	sub, err := rp.Subscribe(t.Context(), "events")
	if err != nil {
		t.Fatalf("Subscribe() error = %v, want nil", err)
	}

	select {
	case _, ok := <-sub.Ch:
		if ok {
			t.Fatal("expected closed channel for nil publisher")
		}
	default:
		t.Fatal("expected closed channel to be immediately readable")
	}
}

func TestSubscription_MultipleCloseNoPanic(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	_ = ctx
	ch := make(chan []byte)
	sub := NewSubscription(ch, cancel)

	// Close from multiple goroutines concurrently.
	var wg conc.WaitGroup
	for range 50 {
		wg.Go(func() {
			sub.Close()
		})
	}
	wg.Wait()
}

// 4. Channel buffer overflow

func TestSubscription_BufferOverflow_DoesNotBlock(t *testing.T) {
	t.Parallel()

	// Create a small-buffered channel to simulate overflow.
	ch := make(chan []byte, 2)
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	sub := NewSubscription(ch, cancel)

	// Fill the buffer.
	ch <- []byte("msg-1")
	ch <- []byte("msg-2")

	// The channel is now full. Writing should block, but reading should work.
	// Verify we can read from the subscription without deadlock.
	msg := <-sub.Ch
	if string(msg) != "msg-1" {
		t.Errorf("got %q, want %q", msg, "msg-1")
	}

	msg = <-sub.Ch
	if string(msg) != "msg-2" {
		t.Errorf("got %q, want %q", msg, "msg-2")
	}
}

func TestSubscription_ZeroBufferChannel(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	ch := make(chan []byte) // Unbuffered channel.
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	sub := NewSubscription(ch, cancel)
	concWG.

		// Write and read must synchronize.
		Go(func() {
			ch <- []byte("sync-msg")
		})

	msg := <-sub.Ch
	if string(msg) != "sync-msg" {
		t.Errorf("got %q, want %q", msg, "sync-msg")
	}
}

// 5. Concurrent subscribe/unsubscribe

func TestResilientPublisher_ConcurrentSubscribeUnsubscribe(t *testing.T) {
	t.Parallel()

	mock := &mockPublisher{
		subscribeFunc: func(_ context.Context, _ string) (*Subscription, error) {
			ch := make(chan []byte, 1)
			return NewSubscription(ch, func() {}), nil
		},
	}

	rp := NewResilientPublisher(mock, slog.Default(), 3)

	var wg conc.WaitGroup
	const goroutines = 50

	for i := range goroutines {
		wg.Go(func() {
			channel := fmt.Sprintf("events:%d", i)
			sub, err := rp.Subscribe(t.Context(), channel)
			if err != nil {
				return
			}
			sub.Close()
		})
	}

	wg.Wait()

	if !rp.IsHealthy() {
		t.Error("publisher should remain healthy after concurrent subscribe/unsubscribe")
	}
}

func TestResilientPublisher_ConcurrentPublishAndSubscribe(t *testing.T) {
	t.Parallel()

	mock := &mockPublisher{
		subscribeFunc: func(_ context.Context, _ string) (*Subscription, error) {
			ch := make(chan []byte, 1)
			return NewSubscription(ch, func() {}), nil
		},
	}

	rp := NewResilientPublisher(mock, slog.Default(), 3)

	var wg conc.WaitGroup
	const goroutines = 50

	// Half publish, half subscribe -- all concurrently.
	for i := range goroutines {
		if i%2 == 0 {
			wg.Go(func() {
				_ = rp.Publish(t.Context(), fmt.Sprintf("ch:%d", i), []byte("data"))
			})
		} else {
			wg.Go(func() {
				sub, err := rp.Subscribe(t.Context(), fmt.Sprintf("ch:%d", i))
				if err == nil {
					sub.Close()
				}
			})
		}
	}

	wg.Wait()

	if !rp.IsHealthy() {
		t.Error("publisher should remain healthy after mixed concurrent operations")
	}
}

// 6. Malformed messages (nil, empty, oversized)

func TestResilientPublisher_NilData(t *testing.T) {
	t.Parallel()

	cp := &countingPublisher{}
	rp := NewResilientPublisher(cp, slog.Default(), 3)

	err := rp.Publish(t.Context(), "events", nil)
	if err != nil {
		t.Fatalf("Publish(nil data) = %v, want nil", err)
	}

	if cp.publishCount.Load() != 1 {
		t.Errorf("publish count = %d, want 1", cp.publishCount.Load())
	}
}

func TestResilientPublisher_EmptyChannel(t *testing.T) {
	t.Parallel()

	cp := &countingPublisher{}
	rp := NewResilientPublisher(cp, slog.Default(), 3)

	err := rp.Publish(t.Context(), "", []byte("data"))
	if err != nil {
		t.Fatalf("Publish(empty channel) = %v, want nil", err)
	}
}

func TestResilientPublisher_OversizedMessage(t *testing.T) {
	t.Parallel()

	cp := &countingPublisher{}
	rp := NewResilientPublisher(cp, slog.Default(), 3)

	// 10MB message.
	data := []byte(strings.Repeat("x", 10*1024*1024))
	err := rp.Publish(t.Context(), "events:large", data)
	if err != nil {
		t.Fatalf("Publish(10MB) = %v, want nil", err)
	}
}

func TestResilientPublisher_PublishBatch_AllNilData(t *testing.T) {
	t.Parallel()

	cp := &countingPublisher{}
	rp := NewResilientPublisher(cp, slog.Default(), 3)

	msgs := []PubSubMessage{
		{Channel: "ch1", Data: nil},
		{Channel: "ch2", Data: nil},
		{Channel: "ch3", Data: nil},
	}

	err := rp.PublishBatch(context.Background(), msgs)
	if err != nil {
		t.Fatalf("PublishBatch(all nil data) = %v, want nil", err)
	}
}

func TestResilientPublisher_PublishBatch_EmptyMessages(t *testing.T) {
	t.Parallel()

	cp := &countingPublisher{}
	rp := NewResilientPublisher(cp, slog.Default(), 3)

	err := rp.PublishBatch(context.Background(), nil)
	if err != nil {
		t.Fatalf("PublishBatch(nil) = %v, want nil", err)
	}

	err = rp.PublishBatch(context.Background(), []PubSubMessage{})
	if err != nil {
		t.Fatalf("PublishBatch(empty) = %v, want nil", err)
	}
}

func TestResilientPublisher_PublishBatch_MixedChannelNames(t *testing.T) {
	t.Parallel()

	cp := &countingPublisher{}
	rp := NewResilientPublisher(cp, slog.Default(), 3)

	msgs := []PubSubMessage{
		{Channel: "", Data: []byte("a")},
		{Channel: strings.Repeat("x", 100000), Data: []byte("b")},
		{Channel: "normal", Data: []byte("c")},
		{Channel: "with\x00null", Data: []byte("d")},
		{Channel: "with spaces", Data: []byte("e")},
	}

	err := rp.PublishBatch(context.Background(), msgs)
	if err != nil {
		t.Fatalf("PublishBatch(mixed channels) = %v, want nil", err)
	}
}

// ResilientPublisher health transitions under adversarial conditions

func TestResilientPublisher_RapidFailureRecoveryCycles(t *testing.T) {
	t.Parallel()

	var callCount atomic.Int64
	mock := &mockPublisher{
		publishFunc: func(_ context.Context, _ string, _ []byte) error {
			n := callCount.Add(1)
			// Alternate: 2 failures, 1 success, repeat.
			if n%3 != 0 {
				return errors.New("intermittent failure")
			}
			return nil
		},
	}

	rp := NewResilientPublisher(mock, slog.Default(), 2)

	for range 30 {
		_ = rp.Publish(t.Context(), "events", []byte("data"))
	}

	// The publisher should not be stuck in a degraded state permanently
	// since successes reset the counter. The final state depends on the
	// last few operations but it must not panic.
}

func TestResilientPublisher_ConcurrentPublishDegradation(t *testing.T) {
	t.Parallel()

	mock := &mockPublisher{
		publishFunc: func(_ context.Context, _ string, _ []byte) error {
			return errors.New("always fail")
		},
	}

	rp := NewResilientPublisher(mock, slog.Default(), 5)

	var wg conc.WaitGroup
	for range 100 {
		wg.Go(func() {
			_ = rp.Publish(t.Context(), "events", []byte("data"))
		})
	}
	wg.Wait()

	// After 100 failures with threshold=5, the publisher should be degraded.
	if rp.IsHealthy() {
		t.Error("publisher should be degraded after 100 consecutive failures")
	}
}

func TestResilientPublisher_CloseNilPublisher(t *testing.T) {
	t.Parallel()

	rp := NewResilientPublisher(nil, slog.Default(), 3)
	err := rp.Close()
	if err != nil {
		t.Fatalf("Close(nil publisher) = %v, want nil", err)
	}
}

func TestResilientPublisher_CloseFailure(t *testing.T) {
	t.Parallel()

	cp := &countingPublisher{closeErr: errors.New("close failed")}
	rp := NewResilientPublisher(cp, slog.Default(), 3)

	err := rp.Close()
	// Resilient publisher swallows close errors.
	if err != nil {
		t.Fatalf("Close() = %v, want nil (fail-open)", err)
	}
}

func TestResilientPublisher_DefaultThreshold(t *testing.T) {
	t.Parallel()

	// Passing threshold <= 0 should use the default (3).
	cp := &countingPublisher{publishErr: errors.New("fail")}
	rp := NewResilientPublisher(cp, slog.Default(), 0)

	// After 2 failures, should still be healthy (default threshold is 3).
	_ = rp.Publish(t.Context(), "ch", []byte("a"))
	_ = rp.Publish(t.Context(), "ch", []byte("b"))
	if !rp.IsHealthy() {
		t.Error("should still be healthy after 2 failures with default threshold=3")
	}

	// Third failure should degrade.
	_ = rp.Publish(t.Context(), "ch", []byte("c"))
	if rp.IsHealthy() {
		t.Error("should be degraded after 3 failures with default threshold=3")
	}
}

func TestResilientPublisher_DefaultLogger(t *testing.T) {
	t.Parallel()

	// Passing nil logger should not cause a panic.
	cp := &countingPublisher{publishErr: errors.New("fail")}
	rp := NewResilientPublisher(cp, nil, 3)

	_ = rp.Publish(t.Context(), "ch", []byte("data"))
	// No panic means success.
}

// RedisPublisher unit tests (without actual Redis)

func TestRedisPublisher_PublishBatch_NilMessages(t *testing.T) {
	t.Parallel()

	rp := &RedisPublisher{client: nil}
	err := rp.PublishBatch(context.Background(), nil)
	if err != nil {
		t.Fatalf("PublishBatch(nil) = %v, want nil", err)
	}
}

func TestNewSubscription_NilCancel(t *testing.T) {
	t.Parallel()

	// NewSubscription with a cancel func that does nothing should be safe.
	ch := make(chan []byte)
	sub := NewSubscription(ch, func() {})

	if sub.Ch != ch {
		t.Error("subscription channel mismatch")
	}

	// Close should not panic.
	sub.Close()
}

func TestClosedSubscription_DrainImmediately(t *testing.T) {
	t.Parallel()

	sub := closedSubscription()

	// Reading from a closed subscription should return immediately.
	msg, ok := <-sub.Ch
	if ok {
		t.Fatalf("expected closed channel, got message: %q", msg)
	}

	// Close should be idempotent and safe.
	sub.Close()
	sub.Close()
}
