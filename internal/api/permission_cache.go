package api

import (
	"sync"
	"time"
)

// permissionCache is a short-lived, concurrency-safe cache for user permissions.
// Avoids hitting the database on every request for the same user+project pair.
type permissionCache struct {
	mu      sync.RWMutex
	entries map[string]permCacheEntry
	ttl     time.Duration
}

type permCacheEntry struct {
	permissions []string
	cachedAt    time.Time
}

func newPermissionCache(ttl time.Duration) *permissionCache {
	return &permissionCache{
		entries: make(map[string]permCacheEntry),
		ttl:     ttl,
	}
}

func (c *permissionCache) key(projectID, userID string) string {
	// Use \x00 as separator — cannot appear in UUIDs or user IDs,
	// preventing collisions like ("a:", "b") vs ("a", ":b").
	return projectID + "\x00" + userID
}

// Get returns cached permissions if they exist and haven't expired.
// Returns (permissions, true) on hit, (nil, false) on miss.
// Expired entries are evicted on access to prevent unbounded growth.
func (c *permissionCache) Get(projectID, userID string) ([]string, bool) {
	k := c.key(projectID, userID)

	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[k]
	if !ok {
		return nil, false
	}
	if time.Since(entry.cachedAt) > c.ttl {
		delete(c.entries, k)
		return nil, false
	}
	return entry.permissions, true
}

// Set stores permissions in the cache.
func (c *permissionCache) Set(projectID, userID string, permissions []string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[c.key(projectID, userID)] = permCacheEntry{
		permissions: permissions,
		cachedAt:    time.Now(),
	}
}

// Invalidate removes a specific user's cached permissions.
func (c *permissionCache) Invalidate(projectID, userID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.entries, c.key(projectID, userID))
}
