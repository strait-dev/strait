package worker

import (
	"context"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

const (
	dispatchMetadataCacheTTL     = time.Second
	dispatchMetadataCacheMaxKeys = 10_000
)

type executorMetadataCache[T any] struct {
	ttl   time.Duration
	clone func(T) T

	mu      sync.RWMutex
	entries map[string]executorMetadataCacheEntry[T]
	group   singleflight.Group
}

type executorMetadataCacheEntry[T any] struct {
	value     T
	expiresAt time.Time
}

func newExecutorMetadataCache[T any](ttl time.Duration, clone func(T) T) *executorMetadataCache[T] {
	if ttl <= 0 {
		return nil
	}
	return &executorMetadataCache[T]{
		ttl:     ttl,
		clone:   clone,
		entries: make(map[string]executorMetadataCacheEntry[T]),
	}
}

func (c *executorMetadataCache[T]) Get(key string) (T, bool) {
	var zero T
	if c == nil {
		return zero, false
	}
	now := time.Now()
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok || now.After(entry.expiresAt) {
		return zero, false
	}
	return c.cloneValue(entry.value), true
}

func (c *executorMetadataCache[T]) Load(ctx context.Context, key string, loader func(context.Context) (T, error)) (T, error) {
	if c == nil {
		return loader(ctx)
	}
	if cached, ok := c.Get(key); ok {
		return cached, nil
	}

	value, err, _ := c.group.Do(key, func() (any, error) {
		if cached, ok := c.Get(key); ok {
			return cached, nil
		}
		loaded, loadErr := loader(ctx)
		if loadErr != nil {
			return nil, loadErr
		}
		c.Set(key, loaded)
		return c.cloneValue(loaded), nil
	})
	if err != nil {
		var zero T
		return zero, err
	}
	return value.(T), nil
}

func (c *executorMetadataCache[T]) Set(key string, value T) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= dispatchMetadataCacheMaxKeys {
		now := time.Now()
		for k, entry := range c.entries {
			if now.After(entry.expiresAt) {
				delete(c.entries, k)
			}
		}
		if len(c.entries) >= dispatchMetadataCacheMaxKeys {
			c.entries = make(map[string]executorMetadataCacheEntry[T])
		}
	}
	c.entries[key] = executorMetadataCacheEntry[T]{
		value:     c.cloneValue(value),
		expiresAt: time.Now().Add(c.ttl),
	}
}

func (c *executorMetadataCache[T]) cloneValue(value T) T {
	if c.clone == nil {
		return value
	}
	return c.clone(value)
}
