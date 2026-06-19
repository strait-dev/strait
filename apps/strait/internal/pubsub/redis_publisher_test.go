package pubsub

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedisPublisher_PublishBatchSingleMessage(t *testing.T) {
	t.Parallel()

	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})

	publisher := NewRedisPublisher(client)
	sub, err := publisher.Subscribe(t.Context(), "test:single-batch")
	require.NoError(t, err)
	defer sub.Close()

	require.NoError(t, publisher.PublishBatch(t.Context(), []PubSubMessage{
		{Channel: "test:single-batch", Data: []byte("one")},
	}))

	select {
	case got := <-sub.Ch:
		assert.Equal(t, "one", string(got))
	case <-time.After(time.Second):
		require.FailNow(t, "timed out waiting for single batch message")
	}
}

func TestRedisPublisher_SubscribeReceiveFailureClosesSubscription(t *testing.T) {
	t.Parallel()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := listener.Addr().String()
	require.NoError(t, listener.Close())

	client := redis.NewClient(&redis.Options{
		Addr:         addr,
		DialTimeout:  10 * time.Millisecond,
		ReadTimeout:  10 * time.Millisecond,
		WriteTimeout: 10 * time.Millisecond,
		MaxRetries:   -1,
	})
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})

	ctx, cancel := context.WithTimeout(t.Context(), 250*time.Millisecond)
	defer cancel()

	sub, err := NewRedisPublisher(client).Subscribe(ctx, "test:unreachable")
	require.Error(t, err)
	assert.Nil(t, sub)
}
