package worker

import (
	"context"
	"sync"
)

type dispatchCacheKey struct{}

// dispatchCache caches expensive query results within a single dispatch cycle.
type dispatchCache struct {
	mu     sync.Mutex
	values map[string]any
}

func withDispatchCache(ctx context.Context) context.Context {
	return context.WithValue(ctx, dispatchCacheKey{}, &dispatchCache{
		values: make(map[string]any),
	})
}

func dispatchCacheGet[T any](ctx context.Context, key string) (T, bool) {
	var zero T
	c, ok := ctx.Value(dispatchCacheKey{}).(*dispatchCache)
	if !ok || c == nil {
		return zero, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	v, found := c.values[key]
	if !found {
		return zero, false
	}
	typed, ok := v.(T)
	if !ok {
		return zero, false
	}
	return typed, true
}

func dispatchCacheSet(ctx context.Context, key string, value any) {
	c, ok := ctx.Value(dispatchCacheKey{}).(*dispatchCache)
	if !ok || c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.values[key] = value
}
