package store

import "sync"

type auditSigningKeyCacheKey struct {
	projectID string
	epoch     int
}

type auditSigningKeyCache struct {
	mu   sync.RWMutex
	keys map[auditSigningKeyCacheKey][]byte
}

func newAuditSigningKeyCache() *auditSigningKeyCache {
	return &auditSigningKeyCache{
		keys: make(map[auditSigningKeyCacheKey][]byte),
	}
}

func (c *auditSigningKeyCache) Get(projectID string, epoch int) ([]byte, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.RLock()
	key, ok := c.keys[auditSigningKeyCacheKey{projectID: projectID, epoch: epoch}]
	c.mu.RUnlock()
	if !ok {
		return nil, false
	}
	return append([]byte(nil), key...), true
}

func (c *auditSigningKeyCache) Set(projectID string, epoch int, key []byte) {
	if c == nil || key == nil {
		return
	}
	copied := append([]byte(nil), key...)
	c.mu.Lock()
	c.keys[auditSigningKeyCacheKey{projectID: projectID, epoch: epoch}] = copied
	c.mu.Unlock()
}

func (c *auditSigningKeyCache) Clear() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.keys = make(map[auditSigningKeyCacheKey][]byte)
	c.mu.Unlock()
}
