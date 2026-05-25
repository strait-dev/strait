package api

import (
	"context"
	"errors"
	"time"

	straitcache "strait/internal/cache"
	"strait/internal/domain"
	"strait/internal/store"
)

type apiKeyCache struct {
	tier *straitcache.Tier[string, *domain.APIKey]
}

func newAPIKeyCache(ttl time.Duration) *apiKeyCache {
	if ttl <= 0 {
		return nil
	}
	refreshAfter := ttl / 3
	if refreshAfter <= 0 {
		refreshAfter = ttl
	}
	return &apiKeyCache{tier: straitcache.NewTier[string, *domain.APIKey](straitcache.TierConfig[string, *domain.APIKey]{
		Name:           "api_key_auth",
		Consistency:    straitcache.Strong,
		MaximumSize:    50_000,
		TTL:            ttl,
		TTLJitter:      0.1,
		RefreshAfter:   refreshAfter,
		EnableNegative: true,
		DisableL2:      true,
		Clone:          cloneAPIKeyForAuthCache,
		Sanitize:       sanitizeAPIKeyForAuthCache,
	})}
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
	_ = c.tier.Set(ctx, key.KeyHash, key, 0)
}

func (c *apiKeyCache) Invalidate(ctx context.Context, keyHash string) {
	if c == nil || c.tier == nil || keyHash == "" {
		return
	}
	c.tier.Invalidate(ctx, keyHash)
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
