package api

import (
	"context"
	"time"

	"github.com/eko/gocache/lib/v4/cache"
	"github.com/maypok86/otter"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"

	"strait/internal/cache/otterstore"
)

var metricsCtx = context.Background()

// permissionCache is a short-lived, concurrency-safe cache for user permissions.
// Avoids hitting the database on every request for the same user+project pair.
// Backed by otter (W-TinyLFU) for high hit rates and low GC overhead.
type permissionCache struct {
	inner    *cache.Cache[[]string]
	ttl      time.Duration
	disabled bool // when true (zero TTL), all Gets return miss

	hits      metric.Int64Counter
	misses    metric.Int64Counter
	evictions metric.Int64Counter
	entriesUp metric.Int64UpDownCounter
}

func newPermissionCache(ttl time.Duration) *permissionCache {
	meter := otel.Meter("strait")
	hits, _ := meter.Int64Counter("strait_permission_cache_hits_total")
	misses, _ := meter.Int64Counter("strait_permission_cache_misses_total")
	evictions, _ := meter.Int64Counter("strait_permission_cache_evictions_total")
	entriesUp, _ := meter.Int64UpDownCounter("strait_permission_cache_entries")

	c := &permissionCache{
		ttl:       ttl,
		disabled:  ttl <= 0,
		hits:      hits,
		misses:    misses,
		evictions: evictions,
		entriesUp: entriesUp,
	}

	cacheTTL := ttl
	if cacheTTL <= 0 {
		cacheTTL = time.Second // minimum for otter's timer wheel
	}

	store := otterstore.New(otterstore.Config{
		DefaultTTL:  cacheTTL,
		MaxCapacity: 10_000,
		TTLJitter:   0.1,
		OnEviction: func(_ string, _ any, _ otter.DeletionCause) {
			c.evictions.Add(metricsCtx, 1)
			c.entriesUp.Add(metricsCtx, -1)
		},
	})

	c.inner = cache.New[[]string](store)
	return c
}

// Stop is a no-op retained for API compatibility.
func (c *permissionCache) Stop() {}

func (c *permissionCache) key(projectID, userID string) string {
	// Use \x00 as separator -- cannot appear in UUIDs or user IDs,
	// preventing collisions like ("a:", "b") vs ("a", ":b").
	return projectID + "\x00" + userID
}

// Get returns cached permissions if they exist and haven't expired.
// Returns (permissions, true) on hit, (nil, false) on miss.
func (c *permissionCache) Get(projectID, userID string) ([]string, bool) {
	if c.disabled {
		c.misses.Add(metricsCtx, 1)
		return nil, false
	}

	k := c.key(projectID, userID)

	perms, err := c.inner.Get(metricsCtx, k)
	if err != nil {
		c.misses.Add(metricsCtx, 1)
		return nil, false
	}

	c.hits.Add(metricsCtx, 1)
	return perms, true
}

// Set stores permissions in the cache.
func (c *permissionCache) Set(projectID, userID string, permissions []string) {
	k := c.key(projectID, userID)

	// Check if this is a new entry for the entries gauge.
	_, err := c.inner.Get(metricsCtx, k)
	isNew := err != nil

	_ = c.inner.Set(metricsCtx, k, permissions)

	if isNew {
		c.entriesUp.Add(metricsCtx, 1)
	}
}

// Invalidate removes a specific user's cached permissions.
func (c *permissionCache) Invalidate(projectID, userID string) {
	k := c.key(projectID, userID)
	// Delete triggers OnEvicted which handles metrics.
	_ = c.inner.Delete(metricsCtx, k)
}
