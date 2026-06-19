package pubsub

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sourcegraph/conc"
)

// RedisPoolOptions bounds the go-redis connection pool and per-op timeouts.
// Zero or negative values fall through to go-redis defaults; in practice
// callers should always pass explicit values (see config.Config).
type RedisPoolOptions struct {
	PoolSize        int
	MinIdleConns    int
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ConnMaxLifetime time.Duration
}

func (p RedisPoolOptions) applyToClient(o *redis.Options) {
	if p.PoolSize > 0 {
		o.PoolSize = p.PoolSize
	}
	if p.MinIdleConns > 0 {
		o.MinIdleConns = p.MinIdleConns
	}
	if p.ReadTimeout > 0 {
		o.ReadTimeout = p.ReadTimeout
	}
	if p.WriteTimeout > 0 {
		o.WriteTimeout = p.WriteTimeout
	}
	if p.ConnMaxLifetime > 0 {
		o.ConnMaxLifetime = p.ConnMaxLifetime
	}
}

func (p RedisPoolOptions) applyToFailover(o *redis.FailoverOptions) {
	if p.PoolSize > 0 {
		o.PoolSize = p.PoolSize
	}
	if p.MinIdleConns > 0 {
		o.MinIdleConns = p.MinIdleConns
	}
	if p.ReadTimeout > 0 {
		o.ReadTimeout = p.ReadTimeout
	}
	if p.WriteTimeout > 0 {
		o.WriteTimeout = p.WriteTimeout
	}
	if p.ConnMaxLifetime > 0 {
		o.ConnMaxLifetime = p.ConnMaxLifetime
	}
}

// NewRedisClient creates a Redis client, using Sentinel failover when
// configured. Pool sizing and per-op timeouts are sourced from pool.
func NewRedisClient(redisURL, sentinelMaster string, sentinelAddrs []string, pool RedisPoolOptions) (*redis.Client, error) {
	if sentinelMaster != "" && len(sentinelAddrs) > 0 {
		opts := &redis.FailoverOptions{
			MasterName:    sentinelMaster,
			SentinelAddrs: sentinelAddrs,
		}
		if redisURL != "" {
			parsedOpts, err := redis.ParseURL(redisURL)
			if err != nil {
				return nil, fmt.Errorf("parse redis sentinel url: %w", err)
			}
			opts.Password = parsedOpts.Password
			opts.DB = parsedOpts.DB
			opts.TLSConfig = parsedOpts.TLSConfig
		}
		pool.applyToFailover(opts)

		return redis.NewFailoverClient(opts), nil
	}

	if redisURL == "" {
		return nil, fmt.Errorf("REDIS_URL is required unless REDIS_SENTINEL_MASTER and REDIS_SENTINEL_ADDRS are configured")
	}

	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}
	pool.applyToClient(opts)

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

func (r *RedisPublisher) PublishBatch(ctx context.Context, messages []PubSubMessage) error {
	if len(messages) == 0 {
		return nil
	}
	if len(messages) == 1 {
		return r.Publish(ctx, messages[0].Channel, messages[0].Data)
	}

	pipe := r.client.Pipeline()
	for _, msg := range messages {
		pipe.Publish(ctx, msg.Channel, msg.Data)
	}
	_, err := pipe.Exec(ctx)
	return err
}

func (r *RedisPublisher) Subscribe(ctx context.Context, channel string) (*Subscription, error) {
	sub := r.client.Subscribe(ctx, channel)
	if _, err := sub.Receive(ctx); err != nil {
		_ = sub.Close()
		return nil, err
	}

	ctx, cancel := context.WithCancel(ctx)
	ch := make(chan []byte, 64)

	var relayWG conc.WaitGroup
	relayWG.Go(func() {
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
	})

	return &Subscription{Ch: ch, cancel: cancel}, nil
}

// Ping checks Redis connectivity.
func (r *RedisPublisher) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

func (r *RedisPublisher) Close() error {
	return r.client.Close()
}
