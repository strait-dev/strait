package api

import (
	"context"
	"time"

	"github.com/eko/gocache/lib/v4/cache"
	"github.com/maypok86/otter"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	"golang.org/x/sync/singleflight"

	"strait/internal/cache/otterstore"
	"strait/internal/store"
)

// quotaCache memoizes store.GetProjectQuota responses on the hot path. Eight
// production call sites (trigger, bulk trigger, middleware, SDK telemetry /
// guardrails / memory, region validation, api keys) hit this same row on
// every authenticated request; the row itself changes rarely (admin update,
// billing plan change). Backed by otter (W-TinyLFU) with a short, jittered
// TTL and singleflight coalescing to absorb concurrent cache misses.
//
// A nil ProjectQuota — meaning "no row for this project" — is a perfectly
// valid cached value: the trigger path treats nil as "no per-project cap"
// and we want to memoize that just as eagerly as a populated row.
type quotaCache struct {
	inner    *cache.Cache[*store.ProjectQuota]
	loader   singleflight.Group
	loadFn   func(ctx context.Context, projectID string) (*store.ProjectQuota, error)
	ttl      time.Duration
	disabled bool

	hits      metric.Int64Counter
	misses    metric.Int64Counter
	evictions metric.Int64Counter
	entriesUp metric.Int64UpDownCounter
	dedupes   metric.Int64Counter
}

// newQuotaCache constructs a quotaCache. Pass ttl <= 0 to disable caching
// entirely (every Get goes straight to loadFn). loadFn is the underlying DB
// fetch — usually store.GetProjectQuota.
func newQuotaCache(
	ttl time.Duration,
	loadFn func(ctx context.Context, projectID string) (*store.ProjectQuota, error),
) *quotaCache {
	meter := otel.Meter("strait")
	hits, _ := meter.Int64Counter("strait_quota_cache_hits_total")
	misses, _ := meter.Int64Counter("strait_quota_cache_misses_total")
	evictions, _ := meter.Int64Counter("strait_quota_cache_evictions_total")
	entriesUp, _ := meter.Int64UpDownCounter("strait_quota_cache_entries")
	dedupes, _ := meter.Int64Counter("strait_quota_cache_singleflight_dedupes_total")

	c := &quotaCache{
		loadFn:    loadFn,
		ttl:       ttl,
		disabled:  ttl <= 0,
		hits:      hits,
		misses:    misses,
		evictions: evictions,
		entriesUp: entriesUp,
		dedupes:   dedupes,
	}

	cacheTTL := ttl
	if cacheTTL <= 0 {
		cacheTTL = time.Second // minimum for otter's timer wheel
	}

	backing := otterstore.New(otterstore.Config{
		DefaultTTL:  cacheTTL,
		MaxCapacity: 10_000,
		TTLJitter:   0.1,
		OnEviction: func(_ string, _ any, _ otter.DeletionCause) {
			c.evictions.Add(metricsCtx, 1)
			c.entriesUp.Add(metricsCtx, -1)
		},
	})

	c.inner = cache.New[*store.ProjectQuota](backing)
	return c
}

// Get returns the cached quota for projectID, loading it through loadFn on
// miss. Concurrent misses for the same projectID are coalesced via
// singleflight so a cold-cache fan-out issues exactly one DB query.
func (c *quotaCache) Get(ctx context.Context, projectID string) (*store.ProjectQuota, error) {
	if c.disabled {
		c.misses.Add(metricsCtx, 1)
		return c.loadFn(ctx, projectID)
	}

	if cached, err := c.inner.Get(metricsCtx, projectID); err == nil {
		c.hits.Add(metricsCtx, 1)
		return cached, nil
	}

	c.misses.Add(metricsCtx, 1)

	result, err, shared := c.loader.Do(projectID, func() (any, error) {
		if cached, gerr := c.inner.Get(metricsCtx, projectID); gerr == nil {
			return cached, nil
		}
		quota, lerr := c.loadFn(ctx, projectID)
		if lerr != nil {
			return nil, lerr
		}
		_ = c.inner.Set(metricsCtx, projectID, quota)
		c.entriesUp.Add(metricsCtx, 1)
		return quota, nil
	})
	if shared {
		c.dedupes.Add(metricsCtx, 1)
	}
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	return result.(*store.ProjectQuota), nil
}

// Invalidate drops the cached entry for projectID. Call this from quota
// write paths (admin updates, billing plan changes) so subsequent reads
// see the new value rather than waiting for TTL expiry.
func (c *quotaCache) Invalidate(projectID string) {
	if c.disabled {
		return
	}
	_ = c.inner.Delete(metricsCtx, projectID)
}
