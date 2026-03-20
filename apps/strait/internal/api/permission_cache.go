package api

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

var metricsCtx = context.Background()

// permissionCache is a short-lived, concurrency-safe cache for user permissions.
// Avoids hitting the database on every request for the same user+project pair.
// A background goroutine sweeps expired entries every 2×TTL to bound memory.
type permissionCache struct {
	mu      sync.RWMutex
	entries map[string]permCacheEntry
	ttl     time.Duration
	stop    chan struct{}

	hits      metric.Int64Counter
	misses    metric.Int64Counter
	evictions metric.Int64Counter
	entriesUp metric.Int64UpDownCounter
}

type permCacheEntry struct {
	permissions []string
	cachedAt    time.Time
}

func newPermissionCache(ttl time.Duration) *permissionCache {
	meter := otel.Meter("strait")
	hits, _ := meter.Int64Counter("strait.permission_cache.hits_total")
	misses, _ := meter.Int64Counter("strait.permission_cache.misses_total")
	evictions, _ := meter.Int64Counter("strait.permission_cache.evictions_total")
	entriesUp, _ := meter.Int64UpDownCounter("strait.permission_cache.entries")

	c := &permissionCache{
		entries:   make(map[string]permCacheEntry),
		ttl:       ttl,
		stop:      make(chan struct{}),
		hits:      hits,
		misses:    misses,
		evictions: evictions,
		entriesUp: entriesUp,
	}
	go c.sweepLoop()
	return c
}

// sweepLoop periodically removes expired entries to prevent unbounded growth.
func (c *permissionCache) sweepLoop() {
	interval := max(c.ttl*2, time.Second)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.sweep()
		case <-c.stop:
			return
		}
	}
}

func (c *permissionCache) sweep() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	evicted := int64(0)
	for k, entry := range c.entries {
		if now.Sub(entry.cachedAt) > c.ttl {
			delete(c.entries, k)
			evicted++
		}
	}
	if evicted > 0 {
		c.evictions.Add(metricsCtx, evicted)
		c.entriesUp.Add(metricsCtx, -evicted)
	}
}

// Stop terminates the background sweep goroutine.
func (c *permissionCache) Stop() {
	select {
	case <-c.stop:
	default:
		close(c.stop)
	}
}

func (c *permissionCache) key(projectID, userID string) string {
	// Use \x00 as separator — cannot appear in UUIDs or user IDs,
	// preventing collisions like ("a:", "b") vs ("a", ":b").
	return projectID + "\x00" + userID
}

// Get returns cached permissions if they exist and haven't expired.
// Returns (permissions, true) on hit, (nil, false) on miss.
// Uses RLock for the fast path (cache hit) to allow concurrent reads.
// Escalates to Lock only when an expired entry needs eviction.
func (c *permissionCache) Get(projectID, userID string) ([]string, bool) {
	k := c.key(projectID, userID)

	// Fast path: read lock for cache hits (most common case).
	c.mu.RLock()
	entry, exists := c.entries[k]
	if exists && time.Since(entry.cachedAt) <= c.ttl {
		c.mu.RUnlock()
		c.hits.Add(metricsCtx, 1)
		return entry.permissions, true
	}
	c.mu.RUnlock()

	// Slow path: miss or expired entry. Acquire write lock if eviction needed.
	if exists {
		// Entry was expired — evict under write lock.
		c.mu.Lock()
		// Double-check: another goroutine may have evicted or refreshed it.
		entry, exists = c.entries[k]
		if exists && time.Since(entry.cachedAt) > c.ttl {
			delete(c.entries, k)
			c.mu.Unlock()
			c.evictions.Add(metricsCtx, 1)
			c.entriesUp.Add(metricsCtx, -1)
			c.misses.Add(metricsCtx, 1)
			return nil, false
		}
		c.mu.Unlock()

		// Entry was refreshed by another goroutine between our RLock and Lock.
		if exists {
			c.hits.Add(metricsCtx, 1)
			return entry.permissions, true
		}
	}

	c.misses.Add(metricsCtx, 1)
	return nil, false
}

// Set stores permissions in the cache.
func (c *permissionCache) Set(projectID, userID string, permissions []string) {
	entriesDelta := int64(0)

	c.mu.Lock()
	k := c.key(projectID, userID)
	_, existed := c.entries[k]
	c.entries[k] = permCacheEntry{
		permissions: permissions,
		cachedAt:    time.Now(),
	}
	if !existed {
		entriesDelta = 1
	}
	c.mu.Unlock()

	if entriesDelta != 0 {
		c.entriesUp.Add(metricsCtx, entriesDelta)
	}
}

// Invalidate removes a specific user's cached permissions.
func (c *permissionCache) Invalidate(projectID, userID string) {
	evicted := int64(0)
	entriesDelta := int64(0)

	c.mu.Lock()
	k := c.key(projectID, userID)
	if _, existed := c.entries[k]; existed {
		delete(c.entries, k)
		evicted = 1
		entriesDelta = -1
	}
	c.mu.Unlock()

	if evicted > 0 {
		c.evictions.Add(metricsCtx, evicted)
	}
	if entriesDelta != 0 {
		c.entriesUp.Add(metricsCtx, entriesDelta)
	}
}
