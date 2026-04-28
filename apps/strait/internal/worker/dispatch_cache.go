package worker

import (
	"context"
	"sync"
)

// dispatchCacheKey is the context key for per-dispatch cached data.
type dispatchCacheKey struct{}

// dispatchCache stores expensive query results that are reused
// multiple times during a single run's dispatch cycle. Prevents
// duplicate DB calls for secrets, checkpoints, and health stats.
//
//nolint:unused // wired but consumers not yet migrated
type dispatchCache struct {
	mu     sync.Mutex
	values map[string]any
}

// withDispatchCache creates a new dispatch cache in the context.
func withDispatchCache(ctx context.Context) context.Context {
	return context.WithValue(ctx, dispatchCacheKey{}, &dispatchCache{
		values: make(map[string]any),
	})
}

// dispatchCacheGet retrieves a cached value. Returns nil, false on miss.
//
//nolint:unused // consumers not yet migrated
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

// dispatchCacheSet stores a value for later retrieval.
//
//nolint:unused // consumers not yet migrated
func dispatchCacheSet(ctx context.Context, key string, value any) {
	c, ok := ctx.Value(dispatchCacheKey{}).(*dispatchCache)
	if !ok || c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.values[key] = value
}
