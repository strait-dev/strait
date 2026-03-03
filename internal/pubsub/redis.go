package pubsub

import (
	"context"
	"log/slog"

	"github.com/redis/go-redis/v9"
)

type RedisPublisher struct {
	client *redis.Client
}

func NewRedisPublisher(client *redis.Client) *RedisPublisher {
	return &RedisPublisher{client: client}
}

func (r *RedisPublisher) Publish(ctx context.Context, channel string, data []byte) error {
	return r.client.Publish(ctx, channel, data).Err()
}

func (r *RedisPublisher) Subscribe(ctx context.Context, channel string) (*Subscription, error) {
	sub := r.client.Subscribe(ctx, channel)
	if _, err := sub.Receive(ctx); err != nil {
		sub.Close()
		return nil, err
	}

	ctx, cancel := context.WithCancel(ctx)
	ch := make(chan []byte, 64)

	go func() {
		defer close(ch)
		defer sub.Close()
		msgCh := sub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-msgCh:
				if !ok {
					return
				}
				select {
				case ch <- []byte(msg.Payload):
				default:
					slog.Warn("pubsub: message dropped, channel full", "channel", channel)
				}
			}
		}
	}()

	return &Subscription{Ch: ch, cancel: cancel}, nil
}

func (r *RedisPublisher) Close() error {
	return r.client.Close()
}
