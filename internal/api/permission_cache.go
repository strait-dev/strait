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
	return projectID + ":" + userID
}

// Get returns cached permissions if they exist and haven't expired.
// Returns (permissions, true) on hit, (nil, false) on miss.
func (c *permissionCache) Get(projectID, userID string) ([]string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[c.key(projectID, userID)]
	if !ok {
		return nil, false
	}
	if time.Since(entry.cachedAt) > c.ttl {
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
