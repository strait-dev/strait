package worker

import (
	"sync"
	"time"

	"strait/internal/domain"
)

type endpointGuardCache struct {
	ttl     time.Duration
	mu      sync.Mutex
	entries map[string]endpointGuardCacheEntry
}

type endpointGuardCacheEntry struct {
	expiresAt      time.Time
	circuitAllowed bool
	circuitRetryAt *time.Time
	healthScore    *domain.EndpointHealthScore
	healthAllowed  bool
}

func newEndpointGuardCache(ttl time.Duration) *endpointGuardCache {
	if ttl <= 0 {
		return nil
	}
	return &endpointGuardCache{
		ttl:     ttl,
		entries: make(map[string]endpointGuardCacheEntry),
	}
}

func (c *endpointGuardCache) get(endpointKey string, now time.Time) (dispatchPrefetch, bool) {
	if c == nil || endpointKey == "" {
		return dispatchPrefetch{}, false
	}
	c.mu.Lock()
	entry, ok := c.entries[endpointKey]
	if !ok {
		c.mu.Unlock()
		return dispatchPrefetch{}, false
	}
	if !now.Before(entry.expiresAt) {
		delete(c.entries, endpointKey)
		c.mu.Unlock()
		return dispatchPrefetch{}, false
	}
	c.mu.Unlock()

	return dispatchPrefetch{
		circuitAllowed: entry.circuitAllowed,
		circuitRetryAt: cloneTimePtr(entry.circuitRetryAt),
		healthScore:    cloneEndpointHealthScore(entry.healthScore),
		healthAllowed:  entry.healthAllowed,
	}, true
}

func (c *endpointGuardCache) setAllowed(endpointKey string, now time.Time, prefetch dispatchPrefetch) {
	if c == nil || endpointKey == "" {
		return
	}
	if prefetch.circuitErr != nil || prefetch.healthErr != nil {
		return
	}
	if !prefetch.circuitAllowed || !prefetch.healthAllowed {
		return
	}
	c.mu.Lock()
	c.entries[endpointKey] = endpointGuardCacheEntry{
		expiresAt:      now.Add(c.ttl),
		circuitAllowed: prefetch.circuitAllowed,
		circuitRetryAt: cloneTimePtr(prefetch.circuitRetryAt),
		healthScore:    cloneEndpointHealthScore(prefetch.healthScore),
		healthAllowed:  prefetch.healthAllowed,
	}
	c.mu.Unlock()
}

func (c *endpointGuardCache) invalidate(endpointKey string) {
	if c == nil || endpointKey == "" {
		return
	}
	c.mu.Lock()
	delete(c.entries, endpointKey)
	c.mu.Unlock()
}

func cloneEndpointHealthScore(score *domain.EndpointHealthScore) *domain.EndpointHealthScore {
	if score == nil {
		return nil
	}
	cp := *score
	return &cp
}

func cloneTimePtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	cp := *t
	return &cp
}
