package pubsub

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/redis/go-redis/v9"
)

// NewRedisClient creates a Redis client, using Sentinel failover when configured.
func NewRedisClient(redisURL, sentinelMaster string, sentinelAddrs []string) (*redis.Client, error) {
	if sentinelMaster != "" && len(sentinelAddrs) > 0 {
		opts := &redis.FailoverOptions{
			MasterName:    sentinelMaster,
			SentinelAddrs: sentinelAddrs,
		}
		if redisURL != "" {
			parsedOpts, err := redis.ParseURL(redisURL)
			if err == nil {
				opts.Password = parsedOpts.Password
				opts.DB = parsedOpts.DB
			}
		}

		return redis.NewFailoverClient(opts), nil
	}

	if redisURL == "" {
		return nil, nil //nolint:nilnil // nil client signals Redis is disabled.
	}

	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}

	return redis.NewClient(opts), nil
}

type RedisPublisher struct {
	client *redis.Client
}

// NewRedisPublisher creates a new Redis-backed event publisher.
func NewRedisPublisher(client *redis.Client) *RedisPublisher {
	return &RedisPublisher{client: client}
}

func (r *RedisPublisher) Publish(ctx context.Context, channel string, data []byte) error {
	return r.client.Publish(ctx, channel, data).Err()
}

func (r *RedisPublisher) Subscribe(ctx context.Context, channel string) (*Subscription, error) {
	sub := r.client.Subscribe(ctx, channel)
	if _, err := sub.Receive(ctx); err != nil {
		if closeErr := sub.Close(); closeErr != nil {
			slog.Warn("failed to close subscription on error", "error", closeErr)
		}
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

// Ping checks Redis connectivity.
func (r *RedisPublisher) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

func (r *RedisPublisher) Close() error {
	return r.client.Close()
}
