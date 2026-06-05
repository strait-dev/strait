//go:build integration

package pubsub_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"strait/internal/pubsub"
)

func TestRedisPublisher_SinglePublishSubscribeRoundTrip(t *testing.T) {
	pub := newPublisher(t)
	ctx := context.Background()

	sub, err := pub.Subscribe(ctx, "test:roundtrip")
	require.NoError(t, err)

	defer sub.Close()

	want := []byte(`{"action":"ping"}`)
	require.NoError(t, pub.Publish(ctx,
		"test:roundtrip",

		want))

	select {
	case got := <-sub.Ch:
		assert.Equal(t, string(want), string(got))
	case <-time.After(5 * time.Second):
		require.FailNow(t, "timed out waiting for message")
	}
}

func TestRedisPublisher_PublishBatch_MultipleMessages(t *testing.T) {
	pub := newPublisher(t)
	ctx := context.Background()

	sub, err := pub.Subscribe(ctx, "test:batch")
	require.NoError(t, err)

	defer sub.Close()

	messages := []pubsub.PubSubMessage{
		{Channel: "test:batch", Data: []byte("batch-1")},
		{Channel: "test:batch", Data: []byte("batch-2")},
		{Channel: "test:batch", Data: []byte("batch-3")},
	}
	require.NoError(t, pub.PublishBatch(ctx, messages))

	for i, want := range messages {
		select {
		case got := <-sub.Ch:
			assert.Equal(t, string(want.Data), string(got), "message %d", i)
		case <-time.After(5 * time.Second):
			require.FailNowf(t, "timed out waiting for batch message", "%d", i)
		}
	}
}

func TestRedisPublisher_PublishBatch_SingleMessage(t *testing.T) {
	pub := newPublisher(t)
	ctx := context.Background()

	sub, err := pub.Subscribe(ctx, "test:batch-single")
	require.NoError(t, err)

	defer sub.Close()

	// Single message batch uses the non-pipeline Publish path.
	messages := []pubsub.PubSubMessage{
		{Channel: "test:batch-single", Data: []byte("only-one")},
	}
	require.NoError(t, pub.PublishBatch(ctx, messages))

	select {
	case got := <-sub.Ch:
		assert.Equal(t, "only-one", string(got))
	case <-time.After(5 * time.Second):
		require.FailNow(t, "timed out waiting for single batch message")
	}
}

func TestRedisPublisher_PublishBatch_Empty(t *testing.T) {
	pub := newPublisher(t)
	ctx := context.Background()
	require.NoError(t, pub.PublishBatch(ctx, nil))
	require.NoError(t, pub.PublishBatch(ctx, []pubsub.
		PubSubMessage{}))

	// Empty batch should be a no-op.

}

func TestRedisPublisher_PublishBatch_MultipleChannels(t *testing.T) {
	pub := newPublisher(t)
	ctx := context.Background()

	subA, err := pub.Subscribe(ctx, "test:batch-chan-a")
	require.NoError(t, err)

	defer subA.Close()

	subB, err := pub.Subscribe(ctx, "test:batch-chan-b")
	require.NoError(t, err)

	defer subB.Close()

	messages := []pubsub.PubSubMessage{
		{Channel: "test:batch-chan-a", Data: []byte("for-a")},
		{Channel: "test:batch-chan-b", Data: []byte("for-b")},
	}
	require.NoError(t, pub.PublishBatch(ctx, messages))

	select {
	case got := <-subA.Ch:
		assert.Equal(t, "for-a", string(got))
	case <-time.After(5 * time.Second):
		require.FailNow(t, "subA timed out")
	}

	select {
	case got := <-subB.Ch:
		assert.Equal(t, "for-b", string(got))
	case <-time.After(5 * time.Second):
		require.FailNow(t, "subB timed out")
	}
}

func TestRedisPublisher_CloseWhileSubscribed(t *testing.T) {
	client := redis.NewClient(testRedis.Options())
	pub := pubsub.NewRedisPublisher(client)

	ctx := context.Background()

	sub, err := pub.Subscribe(ctx, "test:close-while-sub")
	require.NoError(t, err)
	require.NoError(t, pub.Close())

	// Close the publisher -- the subscription goroutine should detect
	// the closed connection and close the channel.

	// The subscription channel should eventually close.
	select {
	case _, ok := <-sub.Ch:
		if ok {
			// May receive buffered data; try again.
			select {
			case _, ok2 := <-sub.Ch:
				assert.False(t, ok2)
			case <-time.After(5 * time.Second):
				require.FailNow(t, "channel did not close after publisher Close()")
			}
		}
	case <-time.After(5 * time.Second):
		require.FailNow(t, "channel did not close after publisher Close()")
	}

	sub.Close()
}

func TestRedisPublisher_PublishToClosedPublisher(t *testing.T) {
	client := redis.NewClient(testRedis.Options())
	pub := pubsub.NewRedisPublisher(client)
	require.NoError(t, pub.Close())

	// Publishing after Close should return an error.
	err := pub.Publish(context.Background(), "test:closed", []byte("hello"))
	assert.Error(t, err)

}

func TestRedisPublisher_PublishBatchToClosedPublisher(t *testing.T) {
	client := redis.NewClient(testRedis.Options())
	pub := pubsub.NewRedisPublisher(client)
	require.NoError(t, pub.Close())

	// PublishBatch after Close should return an error.
	err := pub.PublishBatch(context.Background(), []pubsub.PubSubMessage{
		{Channel: "test:closed", Data: []byte("hello")},
		{Channel: "test:closed", Data: []byte("world")},
	})
	assert.Error(t, err)

}

func TestRedisPublisher_Ping(t *testing.T) {
	pub := newPublisher(t)
	ctx := context.Background()
	require.NoError(t, pub.Ping(ctx))

}

func TestRedisPublisher_PingAfterClose(t *testing.T) {
	client := redis.NewClient(testRedis.Options())
	pub := pubsub.NewRedisPublisher(client)
	require.NoError(t, pub.Close())

	err := pub.Ping(context.Background())
	assert.Error(t, err)

}

func TestRedisPublisher_SubscribeAfterClose(t *testing.T) {
	client := redis.NewClient(testRedis.Options())
	pub := pubsub.NewRedisPublisher(client)
	require.NoError(t, pub.Close())

	_, err := pub.Subscribe(context.Background(), "test:closed-sub")
	assert.Error(t, err)

}

func TestRedisPublisher_LargePayload(t *testing.T) {
	pub := newPublisher(t)
	ctx := context.Background()

	sub, err := pub.Subscribe(ctx, "test:large-payload")
	require.NoError(t, err)

	defer sub.Close()

	// 64KB payload.
	payload := make([]byte, 64*1024)
	for i := range payload {
		payload[i] = byte(i % 256)
	}
	require.NoError(t, pub.Publish(ctx,
		"test:large-payload",

		payload,
	))

	select {
	case got := <-sub.Ch:
		assert.Len(t, got, len(payload))
	case <-time.After(5 * time.Second):
		require.FailNow(t, "timed out waiting for large payload")
	}
}

func TestRedisPublisher_MultipleSubscribersSameChannel(t *testing.T) {
	pub := newPublisher(t)
	ctx := context.Background()

	sub1, err := pub.Subscribe(ctx, "test:multi-sub")
	require.NoError(t, err)

	defer sub1.Close()

	sub2, err := pub.Subscribe(ctx, "test:multi-sub")
	require.NoError(t, err)

	defer sub2.Close()

	want := []byte("shared-message")
	require.NoError(t, pub.Publish(ctx,
		"test:multi-sub",

		want))

	// Both subscribers should receive the message.
	for i, sub := range []*pubsub.Subscription{sub1, sub2} {
		select {
		case got := <-sub.Ch:
			assert.Equal(t, string(want), string(got), "subscriber %d", i+1)
		case <-time.After(5 * time.Second):
			require.FailNowf(t, "subscriber timed out", "%d", i+1)
		}
	}
}

func TestRedisPublisher_PublishBatch_LargeCount(t *testing.T) {
	pub := newPublisher(t)
	ctx := context.Background()

	sub, err := pub.Subscribe(ctx, "test:batch-large")
	require.NoError(t, err)

	defer sub.Close()

	const count = 50
	messages := make([]pubsub.PubSubMessage, count)
	for i := range messages {
		messages[i] = pubsub.PubSubMessage{
			Channel: "test:batch-large",
			Data:    fmt.Appendf(nil, "msg-%03d", i),
		}
	}
	require.NoError(t, pub.PublishBatch(ctx, messages))

	received := 0
	for received < count {
		select {
		case <-sub.Ch:
			received++
		case <-time.After(5 * time.Second):
			require.FailNowf(t, "timed out waiting for messages", "received %d/%d", received, count)
		}
	}
}
