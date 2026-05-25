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

const apiKeyAuthCacheNamespace = "api_key_auth"

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

func (c *apiKeyCache) Get(ctx context.Context, keyHash string, loader func(context.Context, string) (*domain.APIKey, error)) (*domain.APIKey, error) {
	if c == nil || c.tier == nil {
		return loader(ctx, keyHash)
	}
	return c.tier.Get(ctx, keyHash, func(loadCtx context.Context, hash string) (*domain.APIKey, error) {
		key, err := loader(loadCtx, hash)
		if errors.Is(err, store.ErrAPIKeyNotFound) {
			return nil, nil
		}
		return key, err
	})
}

func (c *apiKeyCache) Set(ctx context.Context, key *domain.APIKey) {
	if c == nil || c.tier == nil || key == nil || key.KeyHash == "" {
		return
	}
	_, _ = c.tier.WriteThrough(ctx, key.KeyHash, key, time.Now().UnixNano(), c.bus, apiKeyAuthCacheNamespace, key.KeyHash)
}

func (c *apiKeyCache) Invalidate(ctx context.Context, keyHash string) {
	if c == nil || c.tier == nil || keyHash == "" {
		return
	}
	_ = c.tier.InvalidateThrough(ctx, keyHash, c.bus, apiKeyAuthCacheNamespace, keyHash, time.Now().UnixNano())
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
