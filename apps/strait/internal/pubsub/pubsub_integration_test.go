//go:build integration

package pubsub_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	require.NoError(t, err)

	defer sub.Close()

	want := []byte(`{"event":"created","run_id":"run-001"}`)
	require.NoError(t, pub.Publish(ctx,
		"test:basic",
		want,
	))

	select {
	case got := <-sub.Ch:
		assert.Equal(t, string(want), string(got))
	case <-time.After(5 * time.Second):
		require.FailNow(t, "timed out waiting for message")
	}
}

func TestPublishSubscribe_MultipleMessages(t *testing.T) {
	pub := newPublisher(t)
	ctx := context.Background()

	sub, err := pub.Subscribe(ctx, "test:multi")
	require.NoError(t, err)

	defer sub.Close()

	messages := []string{"msg-1", "msg-2", "msg-3", "msg-4", "msg-5"}
	for _, msg := range messages {
		require.NoError(t, pub.Publish(ctx,
			"test:multi",
			[]byte(msg)))

	}

	for i, want := range messages {
		select {
		case got := <-sub.Ch:
			assert.Equal(t, want, string(got), "message %d", i)
		case <-time.After(5 * time.Second):
			require.FailNowf(t, "timed out waiting for message", "%d (%q)", i, want)
		}
	}
}

func TestSubscribe_ChannelIsolation(t *testing.T) {
	pub := newPublisher(t)
	ctx := context.Background()

	subA, err := pub.Subscribe(ctx, "test:chan-a")
	require.NoError(t, err)

	defer subA.Close()

	subB, err := pub.Subscribe(ctx, "test:chan-b")
	require.NoError(t, err)

	defer subB.Close()
	require.NoError(t, pub.Publish(ctx,
		"test:chan-a",
		[]byte(
			"for-a-only",
		)))

	// Publish only to channel A.

	// subA should receive the message.
	select {
	case got := <-subA.Ch:
		assert.Equal(t, "for-a-only", string(got))
	case <-time.After(5 * time.Second):
		require.FailNow(t, "subA timed out")
	}

	// subB should NOT receive anything.
	select {
	case got := <-subB.Ch:
		assert.Failf(t, "subB received unexpected message", "%q", got)
	case <-time.After(300 * time.Millisecond):
		// Expected — no cross-channel leakage.
	}
}

func TestPublish_NoSubscribers(t *testing.T) {
	pub := newPublisher(t)
	ctx := context.Background()
	require.NoError(t, pub.Publish(ctx,
		"test:nobody-listening",

		[]byte("echo")))

	// Publishing with zero subscribers must not return an error.

}

func TestSubscription_CloseStopsReceiving(t *testing.T) {
	pub := newPublisher(t)
	ctx := context.Background()

	sub, err := pub.Subscribe(ctx, "test:close-stop")
	require.NoError(t, err)

	// Close the subscription — the internal goroutine should exit
	// and close the channel.
	sub.Close()

	select {
	case _, ok := <-sub.Ch:
		if ok {
			// May receive a buffered message, try again for close signal.
			select {
			case _, ok2 := <-sub.Ch:
				assert.False(t, ok2)
			case <-time.After(5 * time.Second):
				require.FailNow(t, "channel did not close after subscription Close()")
			}
		}
		// ok == false means channel is closed — correct.
	case <-time.After(5 * time.Second):
		require.FailNow(t, "channel did not close after subscription Close()")
	}
}

func TestSubscribe_SlowConsumer(t *testing.T) {
	pub := newPublisher(t)
	ctx := context.Background()

	sub, err := pub.Subscribe(ctx, "test:slow-consumer")
	require.NoError(t, err)

	defer sub.Close()

	// Publish many more messages than the internal buffer (64) while
	// the consumer is blocked, forcing drops.
	const total = 500

	// Block the consumer so the buffer fills and drops occur.
	time.Sleep(50 * time.Millisecond)
	for i := range total {
		msg := fmt.Sprintf("msg-%03d", i)
		require.NoError(t, pub.Publish(ctx,
			"test:slow-consumer",

			[]byte(msg),
		))

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
	assert.False(t, received >=
		total)
	assert.NotEqual(t, 0, received)

	t.Logf("received %d/%d messages (buffer=64)", received, total)
}

func TestRedisPublisher_Close(t *testing.T) {
	client := redis.NewClient(testRedis.Options())
	pub := pubsub.NewRedisPublisher(client)
	require.NoError(t, pub.Close())
	assert.Error(t, client.Ping(context.
		Background()).Err())

	// Underlying client should be closed — operations should fail.

}
