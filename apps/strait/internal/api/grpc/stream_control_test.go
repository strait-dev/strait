package grpc

import (
	"context"
	"errors"
	"testing"

	"strait/internal/pubsub"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type controlChannelPublisher struct {
	subscribe func(context.Context, string) (*pubsub.Subscription, error)
}

func (p controlChannelPublisher) Publish(context.Context, string, []byte) error {
	return nil
}

func (p controlChannelPublisher) PublishBatch(context.Context, []pubsub.PubSubMessage) error {
	return nil
}

func (p controlChannelPublisher) Subscribe(ctx context.Context, channel string) (*pubsub.Subscription, error) {
	return p.subscribe(ctx, channel)
}

func (p controlChannelPublisher) Close() error {
	return nil
}

func TestSubscribeRequiredWorkerControlChannel(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		ch := make(chan []byte)
		sub, err := subscribeRequiredWorkerControlChannel(t.Context(), controlChannelPublisher{
			subscribe: func(context.Context, string) (*pubsub.Subscription, error) {
				return pubsub.NewSubscription(ch, func() { close(ch) }), nil
			},
		}, "worker:control", "control")
		require.NoError(t, err)
		require.NotNil(t, sub)
		sub.Close()
	})

	t.Run("missing publisher", func(t *testing.T) {
		t.Parallel()

		sub, err := subscribeRequiredWorkerControlChannel(t.Context(), nil, "worker:control", "control")
		require.Nil(t, sub)
		require.Equal(t, codes.Unavailable, status.Code(err))
	})

	t.Run("subscribe error", func(t *testing.T) {
		t.Parallel()

		sub, err := subscribeRequiredWorkerControlChannel(t.Context(), controlChannelPublisher{
			subscribe: func(context.Context, string) (*pubsub.Subscription, error) {
				return nil, errors.New("redis unavailable")
			},
		}, "worker:control", "control")
		require.Nil(t, sub)
		require.Equal(t, codes.Unavailable, status.Code(err))
		require.Contains(t, err.Error(), "redis unavailable")
	})

	t.Run("nil subscription", func(t *testing.T) {
		t.Parallel()

		sub, err := subscribeRequiredWorkerControlChannel(t.Context(), controlChannelPublisher{
			subscribe: func(context.Context, string) (*pubsub.Subscription, error) {
				return nil, nil
			},
		}, "worker:control", "control")
		require.Nil(t, sub)
		require.Equal(t, codes.Unavailable, status.Code(err))
		require.Contains(t, err.Error(), "nil subscription")
	})
}
