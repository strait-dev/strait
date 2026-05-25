package api

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

func TestAPIKeyCache_ServesValidKeyAndSanitizesSecrets(t *testing.T) {
	t.Parallel()

	cache := newAPIKeyCache(time.Minute)
	var loads atomic.Int64
	key := &domain.APIKey{
		ID:                    "key-1",
		ProjectID:             "proj-1",
		KeyHash:               "hash-1",
		KeyPrefix:             "strait_abcde",
		Scopes:                []string{domain.ScopeRunsRead},
		RotationWebhookSecret: []byte("secret-ciphertext"),
	}
	loader := func(_ context.Context, _ string) (*domain.APIKey, error) {
		loads.Add(1)
		return key, nil
	}

	first, err := cache.Get(context.Background(), "hash-1", loader)
	if err != nil {
		t.Fatalf("Get() first error = %v", err)
	}
	first.Scopes[0] = domain.ScopeRunsWrite

	second, err := cache.Get(context.Background(), "hash-1", loader)
	if err != nil {
		t.Fatalf("Get() second error = %v", err)
	}
	if loads.Load() != 1 {
		t.Fatalf("loader calls = %d, want 1", loads.Load())
	}
	if second.Scopes[0] != domain.ScopeRunsRead {
		t.Fatalf("cached scopes were mutated: %+v", second.Scopes)
	}
	if len(second.RotationWebhookSecret) != 0 {
		t.Fatalf("cached key includes rotation webhook secret: %q", second.RotationWebhookSecret)
	}
}

func TestAPIKeyCache_NegativeCachesInvalidKey(t *testing.T) {
	t.Parallel()

	cache := newAPIKeyCache(time.Minute)
	var loads atomic.Int64
	loader := func(_ context.Context, _ string) (*domain.APIKey, error) {
		loads.Add(1)
		return nil, store.ErrAPIKeyNotFound
	}

	for range 2 {
		key, err := cache.Get(context.Background(), "missing-hash", loader)
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if key != nil {
			t.Fatalf("Get() key = %+v, want nil", key)
		}
	}
	if loads.Load() != 1 {
		t.Fatalf("loader calls = %d, want 1", loads.Load())
	}
}

func TestAPIKeyCache_InvalidateForcesReload(t *testing.T) {
	t.Parallel()

	cache := newAPIKeyCache(time.Minute)
	var loads atomic.Int64
	loader := func(_ context.Context, _ string) (*domain.APIKey, error) {
		loads.Add(1)
		return &domain.APIKey{ID: "key", KeyHash: "hash-1"}, nil
	}

	if _, err := cache.Get(context.Background(), "hash-1", loader); err != nil {
		t.Fatalf("Get() first error = %v", err)
	}
	cache.Invalidate(context.Background(), "hash-1")
	if _, err := cache.Get(context.Background(), "hash-1", loader); err != nil {
		t.Fatalf("Get() second error = %v", err)
	}
	if loads.Load() != 2 {
		t.Fatalf("loader calls = %d, want 2", loads.Load())
	}
}
