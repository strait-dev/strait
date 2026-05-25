package grpc

import (
	"context"
	"errors"
	"time"

	straitcache "strait/internal/cache"
	"strait/internal/domain"
	"strait/internal/store"

	"github.com/redis/go-redis/v9"
)

const grpcAPIKeyAuthCacheNamespace = "api_key_auth"

type cachedAPIKeyResolver struct {
	tier     *straitcache.Tier[string, *domain.APIKey]
	fallback apiKeyResolver
}

func newCachedAPIKeyResolver(client redis.Cmdable, ttl time.Duration, fallback apiKeyResolver) apiKeyResolver {
	if client == nil || ttl <= 0 || fallback == nil {
		return fallback
	}
	l2 := straitcache.NewRedisL2[string, *domain.APIKey](straitcache.RedisL2Config[string, *domain.APIKey]{
		Client:    client,
		Namespace: grpcAPIKeyAuthCacheNamespace,
	})
	refreshAfter := ttl / 3
	if refreshAfter <= 0 {
		refreshAfter = ttl
	}
	return &cachedAPIKeyResolver{
		fallback: fallback,
		tier: straitcache.NewTier[string, *domain.APIKey](straitcache.TierConfig[string, *domain.APIKey]{
			Name:           grpcAPIKeyAuthCacheNamespace,
			L2:             l2,
			Consistency:    straitcache.Strong,
			MaximumSize:    50_000,
			TTL:            ttl,
			TTLJitter:      0.1,
			RefreshAfter:   refreshAfter,
			EnableNegative: true,
			DisableL1:      true,
			Clone:          cloneAPIKeyForGRPCAuthCache,
			Sanitize:       sanitizeAPIKeyForGRPCAuthCache,
		}),
	}
}

func (r *cachedAPIKeyResolver) LookupAPIKeyByHash(ctx context.Context, keyHash string) (*domain.APIKey, error) {
	if r == nil || r.tier == nil {
		if r == nil || r.fallback == nil {
			return nil, store.ErrAPIKeyNotFound
		}
		return r.fallback.LookupAPIKeyByHash(ctx, keyHash)
	}
	got, err := r.tier.GetConsistentVersioned(ctx, keyHash, 0, func(loadCtx context.Context, hash string) (straitcache.Versioned[*domain.APIKey], error) {
		key, err := r.fallback.LookupAPIKeyByHash(loadCtx, hash)
		if errors.Is(err, store.ErrAPIKeyNotFound) {
			return straitcache.Versioned[*domain.APIKey]{Value: nil, Version: 0}, nil
		}
		if err != nil {
			return straitcache.Versioned[*domain.APIKey]{}, err
		}
		return straitcache.Versioned[*domain.APIKey]{Value: key, Version: grpcAPIKeyCacheVersion(key)}, nil
	})
	if err != nil {
		return nil, err
	}
	return got.Value, nil
}

func grpcAPIKeyCacheVersion(key *domain.APIKey) int64 {
	if key == nil || key.CacheVersion <= 0 {
		return 1
	}
	return key.CacheVersion
}

func sanitizeAPIKeyForGRPCAuthCache(key *domain.APIKey) *domain.APIKey {
	if key == nil {
		return nil
	}
	cp := cloneAPIKeyForGRPCAuthCache(key)
	cp.RotationWebhookSecret = nil
	return cp
}

func cloneAPIKeyForGRPCAuthCache(key *domain.APIKey) *domain.APIKey {
	if key == nil {
		return nil
	}
	cp := *key
	cp.Scopes = append([]string(nil), key.Scopes...)
	cp.RotationWebhookSecret = append([]byte(nil), key.RotationWebhookSecret...)
	return &cp
}
