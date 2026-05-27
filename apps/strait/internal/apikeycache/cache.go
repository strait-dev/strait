package apikeycache

import (
	"context"
	"errors"
	"time"

	straitcache "strait/internal/cache"
	"strait/internal/domain"
)

const Namespace = "authn_keys" // #nosec G101 -- cache namespace, not a credential.

type Loader func(context.Context, string) (*domain.APIKey, error)

// RefreshAfter keeps refresh timing consistent across REST and worker-plane auth caches.
func RefreshAfter(ttl time.Duration) time.Duration {
	refreshAfter := ttl / 3
	if refreshAfter <= 0 {
		return ttl
	}
	return refreshAfter
}

// VersionedLoader normalizes missing keys into negative cache entries.
func VersionedLoader(
	loader Loader,
	notFound error,
) straitcache.VersionedLoadFunc[string, *domain.APIKey] {
	return func(ctx context.Context, keyHash string) (straitcache.Versioned[*domain.APIKey], error) {
		key, err := loader(ctx, keyHash)
		if notFound != nil && errors.Is(err, notFound) {
			return straitcache.Versioned[*domain.APIKey]{Value: nil, Version: 0}, nil
		}
		if err != nil {
			return straitcache.Versioned[*domain.APIKey]{}, err
		}
		return straitcache.Versioned[*domain.APIKey]{
			Value:   key,
			Version: Version(key),
		}, nil
	}
}

// Sanitize removes fields that should never be persisted to shared cache storage.
func Sanitize(key *domain.APIKey) *domain.APIKey {
	if key == nil {
		return nil
	}
	cp := Clone(key)
	cp.RotationWebhookSecret = nil
	return cp
}

// Clone copies mutable fields so cache readers cannot mutate stored entries.
func Clone(key *domain.APIKey) *domain.APIKey {
	if key == nil {
		return nil
	}
	cp := *key
	cp.Scopes = append([]string(nil), key.Scopes...)
	cp.RotationWebhookSecret = append([]byte(nil), key.RotationWebhookSecret...)
	return &cp
}

// Version returns the storage version used to reject stale cache writes.
func Version(key *domain.APIKey) int64 {
	if key == nil || key.CacheVersion <= 0 {
		return 1
	}
	return key.CacheVersion
}
