package otterstore

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	lib_store "github.com/eko/gocache/lib/v4/store"
	"github.com/maypok86/otter"
)

const (
	// OtterType represents the storage type as a string value.
	OtterType = "otter"
	// OtterTagPattern represents the tag pattern to be used as a key in specified storage.
	OtterTagPattern = "otter_tag_%s"
)

// Config holds configuration for the otter cache store.
type Config struct {
	// DefaultTTL is the default time-to-live for cache entries.
	DefaultTTL time.Duration
	// MaxCapacity is the maximum number of entries the cache can hold.
	MaxCapacity int
	// OnEviction is called when an entry is evicted from the cache.
	// It receives the key, value, and the reason for eviction.
	OnEviction func(key string, value any, reason otter.DeletionCause)
}

// OtterStore is a gocache store backed by maypok86/otter.
type OtterStore struct {
	mu      sync.RWMutex
	client  otter.CacheWithVariableTTL[string, any]
	options *lib_store.Options
}

// New creates a new otter-backed gocache store.
func New(cfg Config) *OtterStore {
	if cfg.MaxCapacity <= 0 {
		cfg.MaxCapacity = 10_000
	}

	builder := otter.MustBuilder[string, any](cfg.MaxCapacity).WithVariableTTL()

	if cfg.OnEviction != nil {
		builder = builder.DeletionListener(func(key string, value any, cause otter.DeletionCause) {
			cfg.OnEviction(key, value, cause)
		})
	}

	cache, err := builder.Build()
	if err != nil {
		panic(fmt.Sprintf("otterstore: build cache: %v", err))
	}

	opts := &lib_store.Options{}
	if cfg.DefaultTTL > 0 {
		opts.Expiration = cfg.DefaultTTL
	}

	return &OtterStore{
		client:  cache,
		options: opts,
	}
}

// Get returns data stored from a given key.
func (s *OtterStore) Get(_ context.Context, key any) (any, error) {
	keyStr, ok := key.(string)
	if !ok {
		return nil, fmt.Errorf("otterstore: key must be string, got %T", key)
	}
	value, found := s.client.Get(keyStr)
	if !found {
		return nil, lib_store.NotFoundWithCause(errors.New("value not found in otter store"))
	}
	return value, nil
}

// GetWithTTL returns data stored from a given key and its corresponding TTL.
// Otter does not expose per-item TTL, so we return -1 as the remaining duration.
func (s *OtterStore) GetWithTTL(_ context.Context, key any) (any, time.Duration, error) {
	keyStr, ok := key.(string)
	if !ok {
		return nil, 0, fmt.Errorf("otterstore: key must be string, got %T", key)
	}
	value, found := s.client.Get(keyStr)
	if !found {
		return nil, 0, lib_store.NotFoundWithCause(errors.New("value not found in otter store"))
	}
	return value, -1, nil
}

// Set defines data in the otter cache for a given key identifier.
func (s *OtterStore) Set(ctx context.Context, key any, value any, options ...lib_store.Option) error {
	opts := lib_store.ApplyOptions(options...)
	if opts == nil {
		opts = s.options
	}

	ttl := opts.Expiration
	if ttl <= 0 {
		ttl = s.options.Expiration
	}
	if ttl <= 0 {
		ttl = time.Hour
	}

	keyStr, ok := key.(string)
	if !ok {
		return fmt.Errorf("otterstore: key must be string, got %T", key)
	}

	s.client.Set(keyStr, value, ttl)

	if tags := opts.Tags; len(tags) > 0 {
		s.setTags(ctx, key, tags)
	}

	return nil
}

// setTags associates a cache key with one or more tags for bulk invalidation.
// NOTE: Tag bookkeeping is not fully concurrency-safe -- the read-modify-write
// on the tag map can lose updates under concurrent writers. This is acceptable
// because no current cache site uses tags. If tags are needed in the future,
// protect the whole read-modify-write with s.mu or use a sync.Map per tag.
func (s *OtterStore) setTags(ctx context.Context, key any, tags []string) {
	for _, tag := range tags {
		tagKey := fmt.Sprintf(OtterTagPattern, tag)
		var cacheKeys map[string]struct{}

		if result, err := s.Get(ctx, tagKey); err == nil {
			if m, ok := result.(map[string]struct{}); ok {
				cacheKeys = m
			}
		}

		keyStr, ok := key.(string)
		if !ok {
			continue
		}

		s.mu.RLock()
		if _, exists := cacheKeys[keyStr]; exists {
			s.mu.RUnlock()
			continue
		}
		s.mu.RUnlock()

		if cacheKeys == nil {
			cacheKeys = make(map[string]struct{})
		}

		s.mu.Lock()
		cacheKeys[keyStr] = struct{}{}
		s.mu.Unlock()

		s.client.Set(tagKey, cacheKeys, 720*time.Hour)
	}
}

// Delete removes data in the otter cache for a given key identifier.
func (s *OtterStore) Delete(_ context.Context, key any) error {
	keyStr, ok := key.(string)
	if !ok {
		return fmt.Errorf("otterstore: key must be string, got %T", key)
	}
	s.client.Delete(keyStr)
	return nil
}

// Invalidate invalidates cache data for given options.
func (s *OtterStore) Invalidate(ctx context.Context, options ...lib_store.InvalidateOption) error {
	opts := lib_store.ApplyInvalidateOptions(options...)

	if tags := opts.Tags; len(tags) > 0 {
		for _, tag := range tags {
			tagKey := fmt.Sprintf(OtterTagPattern, tag)
			result, err := s.Get(ctx, tagKey)
			if err != nil {
				continue
			}

			var cacheKeys map[string]struct{}
			if m, ok := result.(map[string]struct{}); ok {
				cacheKeys = m
			}

			s.mu.RLock()
			for cacheKey := range cacheKeys {
				_ = s.Delete(ctx, cacheKey)
			}
			s.mu.RUnlock()
		}
	}

	return nil
}

// GetType returns the store type.
func (s *OtterStore) GetType() string {
	return OtterType
}

// Clear resets all data in the store.
func (s *OtterStore) Clear(_ context.Context) error {
	s.client.Clear()
	return nil
}

// Close gracefully shuts down the otter cache.
func (s *OtterStore) Close() {
	s.client.Close()
}
