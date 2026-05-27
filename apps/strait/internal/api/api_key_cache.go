package api

import (
	"context"
	"errors"
	"time"

	straitcache "strait/internal/cache"
	"strait/internal/domain"
	"strait/internal/store"

	"github.com/redis/go-redis/v9"
)

const apiKeyAuthCacheNamespace = "authn_keys" // #nosec G101 -- cache namespace, not a credential.

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
	refreshAfter := ttl / 3
	if refreshAfter <= 0 {
		refreshAfter = ttl
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
		RefreshAfter:   refreshAfter,
		EnableNegative: true,
		DisableL1:      l2 != nil,
		DisableL2:      l2 == nil,
		Clone:          cloneAPIKeyForAuthCache,
		Sanitize:       sanitizeAPIKeyForAuthCache,
	})
	if dep.Registry != nil {
		dep.Registry.Register(apiKeyAuthCacheNamespace, straitcache.UpdatingStringTierHandler[*domain.APIKey]{Tier: c.tier})
	}
	return c
}

func (c *apiKeyCache) Get(
	ctx context.Context,
	keyHash string,
	loader func(context.Context, string) (*domain.APIKey, error),
) (*domain.APIKey, error) {
	if c == nil || c.tier == nil {
		return loader(ctx, keyHash)
	}
	versionedLoader := func(loadCtx context.Context, hash string) (straitcache.Versioned[*domain.APIKey], error) {
		key, err := loader(loadCtx, hash)
		if errors.Is(err, store.ErrAPIKeyNotFound) {
			return straitcache.Versioned[*domain.APIKey]{Value: nil, Version: 0}, nil
		}
		if err != nil {
			return straitcache.Versioned[*domain.APIKey]{}, err
		}
		return straitcache.Versioned[*domain.APIKey]{Value: key, Version: apiKeyCacheVersion(key)}, nil
	}
	got, err := c.tier.GetConsistentVersioned(ctx, keyHash, 0, versionedLoader)
	if err != nil {
		return nil, err
	}
	return got.Value, nil
}

func (c *apiKeyCache) Set(ctx context.Context, key *domain.APIKey) {
	if c == nil || c.tier == nil || key == nil || key.KeyHash == "" {
		return
	}
	_, _ = c.tier.StrongWriteThrough(
		ctx,
		strongCachePolicy(apiKeyAuthCacheNamespace),
		key.KeyHash,
		key.KeyHash,
		key,
		apiKeyCacheVersion(key),
		c.bus,
	)
}

func (c *apiKeyCache) Invalidate(ctx context.Context, keyHash string) {
	c.InvalidateWithVersion(ctx, keyHash, time.Now().UnixNano())
}

func (c *apiKeyCache) InvalidateWithVersion(ctx context.Context, keyHash string, version int64) {
	if c == nil || c.tier == nil || keyHash == "" {
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

func sanitizeAPIKeyForAuthCache(key *domain.APIKey) *domain.APIKey {
	if key == nil {
		return nil
	}
	cp := cloneAPIKeyForAuthCache(key)
	cp.RotationWebhookSecret = nil
	return cp
}

func cloneAPIKeyForAuthCache(key *domain.APIKey) *domain.APIKey {
	if key == nil {
		return nil
	}
	cp := *key
	cp.Scopes = append([]string(nil), key.Scopes...)
	cp.RotationWebhookSecret = append([]byte(nil), key.RotationWebhookSecret...)
	return &cp
}

func apiKeyCacheVersion(key *domain.APIKey) int64 {
	if key == nil || key.CacheVersion <= 0 {
		return 1
	}
	return key.CacheVersion
}
