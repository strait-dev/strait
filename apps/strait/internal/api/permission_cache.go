package api

import (
	"context"
	"time"

	"github.com/eko/gocache/lib/v4/cache"
	gocachestore "github.com/eko/gocache/store/go_cache/v4"
	gocache "github.com/patrickmn/go-cache"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

var metricsCtx = context.Background()

// permissionCache is a short-lived, concurrency-safe cache for user permissions.
// Avoids hitting the database on every request for the same user+project pair.
// Backed by go-cache with a background janitor for automatic expiration.
type permissionCache struct {
	inner *cache.Cache[[]string]
	ttl   time.Duration

	hits      metric.Int64Counter
	misses    metric.Int64Counter
	evictions metric.Int64Counter
	entriesUp metric.Int64UpDownCounter
}

func newPermissionCache(ttl time.Duration) *permissionCache {
	meter := otel.Meter("strait")
	hits, _ := meter.Int64Counter("strait.permission_cache.hits_total")
	misses, _ := meter.Int64Counter("strait.permission_cache.misses_total")
	evictions, _ := meter.Int64Counter("strait.permission_cache.evictions_total")
	entriesUp, _ := meter.Int64UpDownCounter("strait.permission_cache.entries")

	// go-cache treats 0 as "no expiration", so use 1ns as the minimum
	// to ensure items expire immediately when a zero TTL is configured.
	gcTTL := ttl
	if gcTTL <= 0 {
		gcTTL = time.Nanosecond
	}
	gc := gocache.New(gcTTL, max(gcTTL*2, time.Second))

	c := &permissionCache{
		inner:     cache.New[[]string](gocachestore.NewGoCache(gc)),
		ttl:       ttl,
		hits:      hits,
		misses:    misses,
		evictions: evictions,
		entriesUp: entriesUp,
	}

	gc.OnEvicted(func(_ string, _ any) {
		c.evictions.Add(metricsCtx, 1)
		c.entriesUp.Add(metricsCtx, -1)
	})

	return c
}

// Stop is a no-op retained for API compatibility.
// go-cache's janitor goroutine stops when the cache is garbage collected.
func (c *permissionCache) Stop() {}

func (c *permissionCache) key(projectID, userID string) string {
	// Use \x00 as separator -- cannot appear in UUIDs or user IDs,
	// preventing collisions like ("a:", "b") vs ("a", ":b").
	return projectID + "\x00" + userID
}

// Get returns cached permissions if they exist and haven't expired.
// Returns (permissions, true) on hit, (nil, false) on miss.
func (c *permissionCache) Get(projectID, userID string) ([]string, bool) {
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
