package api

import (
	"context"
	"time"

	"strait/internal/apikeycache"
	straitcache "strait/internal/cache"
	"strait/internal/domain"
	"strait/internal/store"

	"github.com/redis/go-redis/v9"
)

const apiKeyAuthCacheNamespace = apikeycache.Namespace

type apiCacheDeps struct {
	Redis    redis.Cmdable
	Bus      *straitcache.Bus
	Registry *straitcache.Registry
}

type apiKeyCache struct {
	tier *straitcache.Tier[string, *domain.APIKey]
	bus  *straitcache.Bus
}

func newAPIKeyCache(ttl time.Duration, deps ...apiCacheDeps) *apiKeyCache {
	if ttl <= 0 {
		return nil
	}
	var dep apiCacheDeps
	if len(deps) > 0 {
		dep = deps[0]
	}
	var l2 straitcache.L2[string, *domain.APIKey]
	if dep.Redis != nil {
		l2 = straitcache.NewRedisL2[string, *domain.APIKey](straitcache.RedisL2Config[string, *domain.APIKey]{
			Client:    dep.Redis,
			Namespace: apiKeyAuthCacheNamespace,
		})
	}
	c := &apiKeyCache{bus: dep.Bus}
	c.tier = straitcache.NewTier[string, *domain.APIKey](straitcache.TierConfig[string, *domain.APIKey]{
		Name:           apiKeyAuthCacheNamespace,
		L2:             l2,
		Consistency:    straitcache.Strong,
		MaximumSize:    50_000,
		TTL:            ttl,
		TTLJitter:      0.1,
		RefreshAfter:   apikeycache.RefreshAfter(ttl),
		EnableNegative: true,
		DisableL1:      l2 != nil,
		DisableL2:      l2 == nil,
		Clone:          apikeycache.Clone,
		Sanitize:       apikeycache.Sanitize,
	})
	if dep.Registry != nil {
		dep.Registry.Register(apiKeyAuthCacheNamespace, straitcache.UpdatingStringTierHandler[*domain.APIKey]{Tier: c.tier})
	}
	return c
}

func (c *apiKeyCache) Stop() {
	if c == nil || c.tier == nil {
		return
	}
	c.tier.Stop()
}

func (c *apiKeyCache) cacheEnabled() bool {
	return c != nil && c.tier != nil
}

func (c *apiKeyCache) Get(
	ctx context.Context,
	keyHash string,
	loader func(context.Context, string) (*domain.APIKey, error),
) (*domain.APIKey, error) {
	if !c.cacheEnabled() {
		return loader(ctx, keyHash)
	}
	got, err := c.tier.GetConsistentVersioned(
		ctx,
		keyHash,
		0,
		apikeycache.VersionedLoader(loader, store.ErrAPIKeyNotFound),
	)
	if err != nil {
		return nil, err
	}
	return got.Value, nil
}

func (c *apiKeyCache) Set(ctx context.Context, key *domain.APIKey) {
	if !c.cacheEnabled() || key == nil || key.KeyHash == "" {
		return
	}
	_, _ = c.tier.StrongWriteThrough(
		ctx,
		strongCachePolicy(apiKeyAuthCacheNamespace),
		key.KeyHash,
		key.KeyHash,
		key,
		apikeycache.Version(key),
		c.bus,
	)
}

func (c *apiKeyCache) Invalidate(ctx context.Context, keyHash string) {
	c.InvalidateWithVersion(ctx, keyHash, time.Now().UnixNano())
}

func (c *apiKeyCache) InvalidateWithVersion(ctx context.Context, keyHash string, version int64) {
	if !c.cacheEnabled() || keyHash == "" {
		return
	}
	_ = c.tier.StrongInvalidate(
		ctx,
		strongCachePolicy(apiKeyAuthCacheNamespace),
		keyHash,
		keyHash,
		cacheVersionBarrier(version),
		c.bus,
	)
}
