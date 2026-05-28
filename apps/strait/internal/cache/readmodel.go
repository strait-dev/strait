package cache

import (
	"context"
	"errors"
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
	namespace string
	l2        L2[string, V]
	ttl       time.Duration
	clone     func(V) V
	sanitize  func(V) V
}

func NewReadModel[V any](cfg ReadModelConfig[V]) *ReadModel[V] {
	if cfg.Client == nil {
		return nil
	}
	l2 := NewRedisL2[string, V](RedisL2Config[string, V]{
		Client:        cfg.Client,
		Namespace:     cfg.Namespace,
		MaxValueBytes: cfg.MaxValueBytes,
	})
	if l2 == nil {
		return nil
	}
	return &ReadModel[V]{
		namespace: cfg.Namespace,
		l2:        l2,
		ttl:       cfg.TTL,
		clone:     cfg.Clone,
		sanitize:  cfg.Sanitize,
	}
}

func (r *ReadModel[V]) Get(ctx context.Context, key string) (Versioned[V], error) {
	if r == nil || r.l2 == nil {
		var zero Versioned[V]
		return zero, ErrCacheMiss
	}
	entry, err := r.l2.Get(ctx, key)
	if err != nil {
		if errors.Is(err, ErrCacheMiss) {
			recordCacheOperation(ctx, r.namespace, "miss")
		} else {
			recordCacheFailOpen(ctx, r.namespace, "read_model_get")
		}
		var zero Versioned[V]
		return zero, err
	}
	recordCacheOperation(ctx, r.namespace, "hit")
	if entry.Barrier {
		var zero Versioned[V]
		return zero, ErrCacheMiss
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
	entry := cacheEntry[V]{
		Version: version,
		Value:   r.sanitizeValue(value),
	}
	ok, err := r.l2.CompareAndSet(ctx, key, entry, r.ttl)
	if err != nil {
		recordCacheFailOpen(ctx, r.namespace, "read_model_cas")
		return false, err
	}
	if !ok {
		recordCacheCASReject(ctx, r.namespace)
	}
	return ok, nil
}

func (r *ReadModel[V]) SetIfCold(ctx context.Context, key string, value V) error {
	return r.SetIfColdVersion(ctx, key, value, 1)
}

func (r *ReadModel[V]) SetIfColdVersion(ctx context.Context, key string, value V, version int64) error {
	if r == nil || r.l2 == nil {
		return nil
	}
	if version <= 0 {
		return fmt.Errorf("read model cold fill version must be positive")
	}
	entry := cacheEntry[V]{
		Version: version,
		Value:   r.sanitizeValue(value),
	}
	_, err := r.l2.CompareAndSet(ctx, key, entry, r.ttl)
	return err
}

func (r *ReadModel[V]) Delete(ctx context.Context, key string) error {
	if r == nil || r.l2 == nil {
		return nil
	}
	return r.l2.Delete(ctx, key)
}

func (r *ReadModel[V]) DeleteVersion(ctx context.Context, key string, version int64) (bool, error) {
	if r == nil || r.l2 == nil {
		return false, nil
	}
	if version <= 0 {
		return false, fmt.Errorf("read model delete version must be positive")
	}
	ok, err := r.l2.CompareAndSet(ctx, key, cacheEntry[V]{Version: version, Barrier: true}, r.ttl)
	if err != nil {
		recordCacheFailOpen(ctx, r.namespace, "read_model_delete")
		return false, err
	}
	if !ok {
		recordCacheCASReject(ctx, r.namespace)
	}
	return ok, nil
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
