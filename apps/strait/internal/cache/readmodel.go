package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type ReadModelConfig[V any] struct {
	Client        redis.Cmdable
	Namespace     string
	TTL           time.Duration
	MaxValueBytes int
	Clone         func(V) V
	Sanitize      func(V) V
}

type ReadModel[V any] struct {
	l2       *RedisL2[string, V]
	ttl      time.Duration
	clone    func(V) V
	sanitize func(V) V
}

func NewReadModel[V any](cfg ReadModelConfig[V]) *ReadModel[V] {
	if cfg.Client == nil {
		return nil
	}
	return &ReadModel[V]{
		l2: NewRedisL2[string, V](RedisL2Config[string, V]{
			Client:        cfg.Client,
			Namespace:     cfg.Namespace,
			MaxValueBytes: cfg.MaxValueBytes,
		}),
		ttl:      cfg.TTL,
		clone:    cfg.Clone,
		sanitize: cfg.Sanitize,
	}
}

func (r *ReadModel[V]) Get(ctx context.Context, key string) (Versioned[V], error) {
	if r == nil || r.l2 == nil {
		var zero Versioned[V]
		return zero, ErrCacheMiss
	}
	entry, err := r.l2.Get(ctx, key)
	if err != nil {
		var zero Versioned[V]
		return zero, err
	}
	if entry.Negative {
		var zero V
		return Versioned[V]{Value: zero, Version: entry.Version}, nil
	}
	return Versioned[V]{Value: r.cloneValue(entry.Value), Version: entry.Version}, nil
}

func (r *ReadModel[V]) CompareAndSet(ctx context.Context, key string, value V, version int64) (bool, error) {
	if r == nil || r.l2 == nil {
		return false, nil
	}
	if version <= 0 {
		return false, fmt.Errorf("read model version must be positive")
	}
	return r.l2.CompareAndSet(ctx, key, cacheEntry[V]{Version: version, Value: r.sanitizeValue(value)}, r.ttl)
}

func (r *ReadModel[V]) SetIfCold(ctx context.Context, key string, value V) error {
	if r == nil || r.l2 == nil {
		return nil
	}
	_, err := r.l2.CompareAndSet(ctx, key, cacheEntry[V]{Version: 1, Value: r.sanitizeValue(value)}, r.ttl)
	return err
}

func (r *ReadModel[V]) Delete(ctx context.Context, key string) error {
	if r == nil || r.l2 == nil {
		return nil
	}
	return r.l2.Delete(ctx, key)
}

func (r *ReadModel[V]) cloneValue(v V) V {
	if r != nil && r.clone != nil {
		return r.clone(v)
	}
	return v
}

func (r *ReadModel[V]) sanitizeValue(v V) V {
	if r != nil && r.sanitize != nil {
		return r.sanitize(v)
	}
	return v
}
