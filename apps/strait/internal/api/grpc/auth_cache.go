package grpc

import (
	"context"
	"time"

	"strait/internal/apikeycache"
	straitcache "strait/internal/cache"
	"strait/internal/domain"
	"strait/internal/store"

	"github.com/redis/go-redis/v9"
)

const grpcAPIKeyAuthCacheNamespace = apikeycache.Namespace

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
	return &cachedAPIKeyResolver{
		fallback: fallback,
		tier: straitcache.NewTier[string, *domain.APIKey](straitcache.TierConfig[string, *domain.APIKey]{
			Name:           grpcAPIKeyAuthCacheNamespace,
			L2:             l2,
			Consistency:    straitcache.Strong,
			MaximumSize:    50_000,
			TTL:            ttl,
			TTLJitter:      0.1,
			RefreshAfter:   apikeycache.RefreshAfter(ttl),
			EnableNegative: true,
			DisableL1:      true,
			Clone:          apikeycache.Clone,
			Sanitize:       apikeycache.Sanitize,
		}),
	}
}

func (r *cachedAPIKeyResolver) cacheEnabled() bool {
	return r != nil && r.tier != nil
}

func (r *cachedAPIKeyResolver) LookupAPIKeyByHash(ctx context.Context, keyHash string) (*domain.APIKey, error) {
	if !r.cacheEnabled() {
		if r == nil || r.fallback == nil {
			return nil, store.ErrAPIKeyNotFound
		}
		return r.fallback.LookupAPIKeyByHash(ctx, keyHash)
	}
	loader := apikeycache.VersionedLoader(
		r.fallback.LookupAPIKeyByHash,
		store.ErrAPIKeyNotFound,
	)
	got, err := r.tier.GetConsistentVersioned(ctx, keyHash, 0, loader)
	if err != nil {
		return nil, err
	}
	return got.Value, nil
}
