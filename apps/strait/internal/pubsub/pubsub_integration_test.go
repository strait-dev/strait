//go:build integration

package pubsub_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"strait/internal/pubsub"
	"strait/internal/testutil"
)

var testRedis *testutil.TestRedis

func TestMain(m *testing.M) {
	ctx := context.Background()
	var err error
	testRedis, err = testutil.SetupSharedTestRedis(ctx, "pubsub")
	if err != nil {
		fmt.Fprintf(os.Stderr, "setup redis: %v\n", err)
		os.Exit(1)
	}
	code := m.Run()
	testRedis.Cleanup(ctx)
	os.Exit(code)
}

// newPublisher creates a RedisPublisher with a fresh client for test isolation.
func newPublisher(t *testing.T) *pubsub.RedisPublisher {
	t.Helper()
	client := redis.NewClient(testRedis.Options())
	t.Cleanup(func() { _ = client.Close() })
	return pubsub.NewRedisPublisher(client)
}

func TestPublishSubscribe(t *testing.T) {
	pub := newPublisher(t)
	ctx := context.Background()

	sub, err := pub.Subscribe(ctx, "test:basic")
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer sub.Close()

	want := []byte(`{"event":"created","run_id":"run-001"}`)
	if err := pub.Publish(ctx, "test:basic", want); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	select {
	case got := <-sub.Ch:
		if string(got) != string(want) {
			t.Errorf("received %q, want %q", got, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for message")
	}
}

func TestPublishSubscribe_MultipleMessages(t *testing.T) {
	pub := newPublisher(t)
	ctx := context.Background()

	sub, err := pub.Subscribe(ctx, "test:multi")
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer sub.Close()

	messages := []string{"msg-1", "msg-2", "msg-3", "msg-4", "msg-5"}
	for _, msg := range messages {
		if err := pub.Publish(ctx, "test:multi", []byte(msg)); err != nil {
			t.Fatalf("Publish(%q) error = %v", msg, err)
		}
	}

	for i, want := range messages {
		select {
		case got := <-sub.Ch:
			if string(got) != want {
				t.Errorf("message[%d] = %q, want %q", i, got, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for message %d (%q)", i, want)
		}
	}
}

func TestSubscribe_ChannelIsolation(t *testing.T) {
	pub := newPublisher(t)
	ctx := context.Background()

	subA, err := pub.Subscribe(ctx, "test:chan-a")
	if err != nil {
		t.Fatalf("Subscribe(chan-a) error = %v", err)
	}
	defer subA.Close()

	subB, err := pub.Subscribe(ctx, "test:chan-b")
	if err != nil {
		t.Fatalf("Subscribe(chan-b) error = %v", err)
	}
	defer subB.Close()

	// Publish only to channel A.
	if err := pub.Publish(ctx, "test:chan-a", []byte("for-a-only")); err != nil {
		t.Fatalf("Publish(chan-a) error = %v", err)
	}

	// subA should receive the message.
	select {
	case got := <-subA.Ch:
		if string(got) != "for-a-only" {
			t.Errorf("subA received %q, want %q", got, "for-a-only")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("subA timed out")
	}

	// subB should NOT receive anything.
	select {
	case got := <-subB.Ch:
		t.Errorf("subB received unexpected message: %q", got)
	case <-time.After(300 * time.Millisecond):
		// Expected — no cross-channel leakage.
	}
}

func TestPublish_NoSubscribers(t *testing.T) {
	pub := newPublisher(t)
	ctx := context.Background()

	// Publishing with zero subscribers must not return an error.
	if err := pub.Publish(ctx, "test:nobody-listening", []byte("echo")); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
}

func TestSubscription_CloseStopsReceiving(t *testing.T) {
	pub := newPublisher(t)
	ctx := context.Background()

	sub, err := pub.Subscribe(ctx, "test:close-stop")
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}

	// Close the subscription — the internal goroutine should exit
	// and close the channel.
	sub.Close()

	select {
	case _, ok := <-sub.Ch:
		if ok {
			// May receive a buffered message, try again for close signal.
			select {
			case _, ok2 := <-sub.Ch:
				if ok2 {
					t.Error("channel still open after second read")
				}
			case <-time.After(5 * time.Second):
				t.Fatal("channel did not close after subscription Close()")
			}
		}
		// ok == false means channel is closed — correct.
	case <-time.After(5 * time.Second):
		t.Fatal("channel did not close after subscription Close()")
	}
}

func TestSubscribe_SlowConsumer(t *testing.T) {
	pub := newPublisher(t)
	ctx := context.Background()

	sub, err := pub.Subscribe(ctx, "test:slow-consumer")
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer sub.Close()

	// Publish many more messages than the internal buffer (64) while
	// the consumer is blocked, forcing drops.
	const total = 500

	// Block the consumer so the buffer fills and drops occur.
	time.Sleep(50 * time.Millisecond)
	for i := range total {
		msg := fmt.Sprintf("msg-%03d", i)
		if err := pub.Publish(ctx, "test:slow-consumer", []byte(msg)); err != nil {
			t.Fatalf("Publish(%q) error = %v", msg, err)
		}
	}

	// Drain the channel — we should receive fewer than total because
	// the non-blocking send drops messages when the buffer is full.
	var received int
drain:
	for {
		select {
		case _, ok := <-sub.Ch:
			if !ok {
				break drain
			}
			received++
		case <-time.After(300 * time.Millisecond):
			break drain
		}
	}

	if received >= total {
		t.Errorf("received all %d messages, expected some to be dropped", total)
	}
	if received == 0 {
		t.Error("received 0 messages, want at least 1")
	}
	t.Logf("received %d/%d messages (buffer=64)", received, total)
}

func TestRedisPublisher_Close(t *testing.T) {
	client := redis.NewClient(testRedis.Options())
	pub := pubsub.NewRedisPublisher(client)

	if err := pub.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// Underlying client should be closed — operations should fail.
	if err := client.Ping(context.Background()).Err(); err == nil {
		t.Error("Ping succeeded after Close(), want error")
	}
}
