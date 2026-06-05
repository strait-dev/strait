package grpc

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestCachedAPIKeyResolver_UsesRedisL2AndSanitizesSecrets(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	var lookups atomic.Int64
	fallback := apiKeyResolverFunc(func(context.Context, string) (*domain.APIKey, error) {
		lookups.Add(1)
		return &domain.APIKey{
			ID:                    "key-1",
			ProjectID:             "project-1",
			Scopes:                []string{domain.ScopeWorkersConnect},
			RotationWebhookSecret: []byte("encrypted-secret"),
			CacheVersion:          8,
		}, nil
	})
	resolver := newCachedAPIKeyResolver(rdb, time.Minute, fallback)

	first, err := resolver.LookupAPIKeyByHash(t.Context(), "hash-1")
	require.NoError(t, err)

	first.Scopes[0] = domain.ScopeRunsRead
	second, err := resolver.LookupAPIKeyByHash(t.Context(), "hash-1")
	require.NoError(t, err)
	require.EqualValues(t, 1, lookups.Load())
	require.Equal(
		t, domain.ScopeWorkersConnect,
		second.
			Scopes[0])
	require.Empty(t,
		second.RotationWebhookSecret,
	)

	redisKey := "strait:cache:" + grpcAPIKeyAuthCacheNamespace + ":hash-1"
	raw, err := rdb.Get(t.Context(), redisKey).Bytes()
	require.NoError(t, err)

	var envelope struct {
		Version int64 `json:"version"`
	}
	require.NoError(t, json.Unmarshal(raw, &envelope))
	require.EqualValues(t, 8, envelope.Version)
}
