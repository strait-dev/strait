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
)

func TestCachedAPIKeyResolver_UsesRedisL2AndSanitizesSecrets(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	var lookups atomic.Int64
	resolver := newCachedAPIKeyResolver(rdb, time.Minute, apiKeyResolverFunc(func(context.Context, string) (*domain.APIKey, error) {
		lookups.Add(1)
		return &domain.APIKey{
			ID:                    "key-1",
			ProjectID:             "project-1",
			Scopes:                []string{domain.ScopeWorkersConnect},
			RotationWebhookSecret: []byte("encrypted-secret"),
			CacheVersion:          8,
		}, nil
	}))

	first, err := resolver.LookupAPIKeyByHash(context.Background(), "hash-1")
	if err != nil {
		t.Fatalf("LookupAPIKeyByHash(first) error = %v", err)
	}
	first.Scopes[0] = domain.ScopeRunsRead
	second, err := resolver.LookupAPIKeyByHash(context.Background(), "hash-1")
	if err != nil {
		t.Fatalf("LookupAPIKeyByHash(second) error = %v", err)
	}
	if lookups.Load() != 1 {
		t.Fatalf("fallback lookups = %d, want 1", lookups.Load())
	}
	if second.Scopes[0] != domain.ScopeWorkersConnect {
		t.Fatalf("cached scopes were mutated: %+v", second.Scopes)
	}
	if len(second.RotationWebhookSecret) != 0 {
		t.Fatalf("cached key includes rotation webhook secret: %q", second.RotationWebhookSecret)
	}

	raw, err := rdb.Get(context.Background(), "strait:cache:"+grpcAPIKeyAuthCacheNamespace+":hash-1").Bytes()
	if err != nil {
		t.Fatalf("read redis entry: %v", err)
	}
	var envelope struct {
		Version int64 `json:"version"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("decode redis entry: %v", err)
	}
	if envelope.Version != 8 {
		t.Fatalf("redis version = %d, want 8", envelope.Version)
	}
}
