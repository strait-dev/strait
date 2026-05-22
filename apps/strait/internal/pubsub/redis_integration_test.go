//go:build integration

package pubsub_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"strait/internal/pubsub"
)

func TestRedisPublisher_SinglePublishSubscribeRoundTrip(t *testing.T) {
	pub := newPublisher(t)
	ctx := context.Background()

	sub, err := pub.Subscribe(ctx, "test:roundtrip")
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer sub.Close()

	want := []byte(`{"action":"ping"}`)
	if err := pub.Publish(ctx, "test:roundtrip", want); err != nil {
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

func TestRedisPublisher_PublishBatch_MultipleMessages(t *testing.T) {
	pub := newPublisher(t)
	ctx := context.Background()

	sub, err := pub.Subscribe(ctx, "test:batch")
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer sub.Close()

	messages := []pubsub.PubSubMessage{
		{Channel: "test:batch", Data: []byte("batch-1")},
		{Channel: "test:batch", Data: []byte("batch-2")},
		{Channel: "test:batch", Data: []byte("batch-3")},
	}

	if err := pub.PublishBatch(ctx, messages); err != nil {
		t.Fatalf("PublishBatch() error = %v", err)
	}

	for i, want := range messages {
		select {
		case got := <-sub.Ch:
			if string(got) != string(want.Data) {
				t.Errorf("message[%d] = %q, want %q", i, got, want.Data)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for batch message %d", i)
		}
	}
}

func TestRedisPublisher_PublishBatch_SingleMessage(t *testing.T) {
	pub := newPublisher(t)
	ctx := context.Background()

	sub, err := pub.Subscribe(ctx, "test:batch-single")
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer sub.Close()

	// Single message batch uses the non-pipeline Publish path.
	messages := []pubsub.PubSubMessage{
		{Channel: "test:batch-single", Data: []byte("only-one")},
	}

	if err := pub.PublishBatch(ctx, messages); err != nil {
		t.Fatalf("PublishBatch() error = %v", err)
	}

	select {
	case got := <-sub.Ch:
		if string(got) != "only-one" {
			t.Errorf("received %q, want %q", got, "only-one")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for single batch message")
	}
}

func TestRedisPublisher_PublishBatch_Empty(t *testing.T) {
	pub := newPublisher(t)
	ctx := context.Background()

	// Empty batch should be a no-op.
	if err := pub.PublishBatch(ctx, nil); err != nil {
		t.Fatalf("PublishBatch(nil) error = %v", err)
	}
	if err := pub.PublishBatch(ctx, []pubsub.PubSubMessage{}); err != nil {
		t.Fatalf("PublishBatch([]) error = %v", err)
	}
}

func TestRedisPublisher_PublishBatch_MultipleChannels(t *testing.T) {
	pub := newPublisher(t)
	ctx := context.Background()

	subA, err := pub.Subscribe(ctx, "test:batch-chan-a")
	if err != nil {
		t.Fatalf("Subscribe(chan-a) error = %v", err)
	}
	defer subA.Close()

	subB, err := pub.Subscribe(ctx, "test:batch-chan-b")
	if err != nil {
		t.Fatalf("Subscribe(chan-b) error = %v", err)
	}
	defer subB.Close()

	messages := []pubsub.PubSubMessage{
		{Channel: "test:batch-chan-a", Data: []byte("for-a")},
		{Channel: "test:batch-chan-b", Data: []byte("for-b")},
	}

	if err := pub.PublishBatch(ctx, messages); err != nil {
		t.Fatalf("PublishBatch() error = %v", err)
	}

	select {
	case got := <-subA.Ch:
		if string(got) != "for-a" {
			t.Errorf("subA received %q, want %q", got, "for-a")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("subA timed out")
	}

	select {
	case got := <-subB.Ch:
		if string(got) != "for-b" {
			t.Errorf("subB received %q, want %q", got, "for-b")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("subB timed out")
	}
}

func TestRedisPublisher_CloseWhileSubscribed(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: testRedis.Addr})
	pub := pubsub.NewRedisPublisher(client)

	ctx := context.Background()

	sub, err := pub.Subscribe(ctx, "test:close-while-sub")
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}

	// Close the publisher -- the subscription goroutine should detect
	// the closed connection and close the channel.
	if err := pub.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// The subscription channel should eventually close.
	select {
	case _, ok := <-sub.Ch:
		if ok {
			// May receive buffered data; try again.
			select {
			case _, ok2 := <-sub.Ch:
				if ok2 {
					t.Error("channel still open after second read")
				}
			case <-time.After(5 * time.Second):
				t.Fatal("channel did not close after publisher Close()")
			}
		}
	case <-time.After(5 * time.Second):
		t.Fatal("channel did not close after publisher Close()")
	}

	sub.Close()
}

func TestRedisPublisher_PublishToClosedPublisher(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: testRedis.Addr})
	pub := pubsub.NewRedisPublisher(client)

	if err := pub.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// Publishing after Close should return an error.
	err := pub.Publish(context.Background(), "test:closed", []byte("hello"))
	if err == nil {
		t.Error("Publish() after Close() returned nil, want error")
	}
}

func TestRedisPublisher_PublishBatchToClosedPublisher(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: testRedis.Addr})
	pub := pubsub.NewRedisPublisher(client)

	if err := pub.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// PublishBatch after Close should return an error.
	err := pub.PublishBatch(context.Background(), []pubsub.PubSubMessage{
		{Channel: "test:closed", Data: []byte("hello")},
		{Channel: "test:closed", Data: []byte("world")},
	})
	if err == nil {
		t.Error("PublishBatch() after Close() returned nil, want error")
	}
}

func TestRedisPublisher_Ping(t *testing.T) {
	pub := newPublisher(t)
	ctx := context.Background()

	if err := pub.Ping(ctx); err != nil {
		t.Fatalf("Ping() error = %v", err)
	}
}

func TestRedisPublisher_PingAfterClose(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: testRedis.Addr})
	pub := pubsub.NewRedisPublisher(client)

	if err := pub.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	err := pub.Ping(context.Background())
	if err == nil {
		t.Error("Ping() after Close() returned nil, want error")
	}
}

func TestRedisPublisher_SubscribeAfterClose(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: testRedis.Addr})
	pub := pubsub.NewRedisPublisher(client)

	if err := pub.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	_, err := pub.Subscribe(context.Background(), "test:closed-sub")
	if err == nil {
		t.Error("Subscribe() after Close() returned nil, want error")
	}
}

func TestRedisPublisher_LargePayload(t *testing.T) {
	pub := newPublisher(t)
	ctx := context.Background()

	sub, err := pub.Subscribe(ctx, "test:large-payload")
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer sub.Close()

	// 64KB payload.
	payload := make([]byte, 64*1024)
	for i := range payload {
		payload[i] = byte(i % 256)
	}

	if err := pub.Publish(ctx, "test:large-payload", payload); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	select {
	case got := <-sub.Ch:
		if len(got) != len(payload) {
			t.Errorf("received %d bytes, want %d", len(got), len(payload))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for large payload")
	}
}

func TestRedisPublisher_MultipleSubscribersSameChannel(t *testing.T) {
	pub := newPublisher(t)
	ctx := context.Background()

	sub1, err := pub.Subscribe(ctx, "test:multi-sub")
	if err != nil {
		t.Fatalf("Subscribe() sub1 error = %v", err)
	}
	defer sub1.Close()

	sub2, err := pub.Subscribe(ctx, "test:multi-sub")
	if err != nil {
		t.Fatalf("Subscribe() sub2 error = %v", err)
	}
	defer sub2.Close()

	want := []byte("shared-message")
	if err := pub.Publish(ctx, "test:multi-sub", want); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	// Both subscribers should receive the message.
	for i, sub := range []*pubsub.Subscription{sub1, sub2} {
		select {
		case got := <-sub.Ch:
			if string(got) != string(want) {
				t.Errorf("sub%d received %q, want %q", i+1, got, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("sub%d timed out", i+1)
		}
	}
}

func TestRedisPublisher_PublishBatch_LargeCount(t *testing.T) {
	pub := newPublisher(t)
	ctx := context.Background()

	sub, err := pub.Subscribe(ctx, "test:batch-large")
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer sub.Close()

	const count = 50
	messages := make([]pubsub.PubSubMessage, count)
	for i := range messages {
		messages[i] = pubsub.PubSubMessage{
			Channel: "test:batch-large",
			Data:    fmt.Appendf(nil, "msg-%03d", i),
		}
	}

	if err := pub.PublishBatch(ctx, messages); err != nil {
		t.Fatalf("PublishBatch() error = %v", err)
	}

	received := 0
	for received < count {
		select {
		case <-sub.Ch:
			received++
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out: received %d/%d messages", received, count)
		}
	}
}
