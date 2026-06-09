package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

var ErrStaleVersion = errors.New("cache loader returned stale version")

type Versioned[V any] struct {
	Value   V
	Version int64
}

type VersionedLoadFunc[K comparable, V any] func(context.Context, K) (Versioned[V], error)

type VersionBarrier struct {
	Version int64
}

func (t *Tier[K, V]) GetConsistentVersioned(
	ctx context.Context,
	key K,
	minVersion int64,
	loader VersionedLoadFunc[K, V],
) (Versioned[V], error) {
	if t == nil {
		var zero Versioned[V]
		return zero, fmt.Errorf("cache tier is nil")
	}
	if t.l1Available() {
		if entry, ok := t.l1.GetIfPresent(key); ok && entry.Version >= minVersion {
			return Versioned[V]{Value: t.clone(entry.Value), Version: entry.Version}, nil
		}
	}

	entry, err, _ := t.loadGroup.Do(tierSingleflightKey(t.name, key, minVersion, true), func() (any, error) {
		return t.loadVersionedThroughL2(ctx, key, minVersion, loader)
	})
	if err != nil {
		var zero Versioned[V]
		return zero, err
	}
	cacheEntry := entry.(cacheEntry[V])
	if cacheEntry.Negative {
		var zero V
		return Versioned[V]{Value: zero, Version: cacheEntry.Version}, nil
	}
	return Versioned[V]{Value: t.clone(cacheEntry.Value), Version: cacheEntry.Version}, nil
}

func (t *Tier[K, V]) WriteThrough(
	ctx context.Context,
	key K,
	value V,
	version int64,
	bus *Bus,
	namespace string,
	busKey string,
) (bool, error) {
	if t == nil {
		return false, fmt.Errorf("cache tier is nil")
	}
	ok, err := t.CompareAndSet(ctx, key, value, version)
	if err != nil || !ok {
		return ok, err
	}
	if cacheBusPublishConfigured(bus, namespace, busKey) {
		entry := cacheEntry[V]{Version: version, Value: t.sanitize(value)}
		if t.negEnabled && t.isNegative(value) {
			entry.Negative = true
		}
		payload, marshalErr := json.Marshal(entry)
		if marshalErr != nil {
			return ok, fmt.Errorf("marshal cachebus update payload: %w", marshalErr)
		}
		if publishErr := bus.PublishUpdate(ctx, namespace, busKey, version, payload); publishErr != nil {
			return ok, publishErr
		}
	}
	return ok, nil
}

func (t *Tier[K, V]) InvalidateThrough(
	ctx context.Context,
	key K,
	bus *Bus,
	namespace string,
	busKey string,
	version int64,
) error {
	if t == nil {
		return nil
	}
	t.applyBarrier(ctx, key, version)
	if cacheBusPublishConfigured(bus, namespace, busKey) {
		return bus.PublishInvalidate(ctx, namespace, busKey, version)
	}
	return nil
}

func cacheBusPublishConfigured(bus *Bus, namespace, busKey string) bool {
	if bus == nil {
		return false
	}
	if namespace == "" {
		return false
	}
	return busKey != ""
}

func (t *Tier[K, V]) StrongWriteThrough(
	ctx context.Context,
	policy StrongNamespacePolicy,
	key K,
	busKey string,
	value V,
	version int64,
	bus *Bus,
) (bool, error) {
	if t == nil {
		return false, fmt.Errorf("cache tier is nil")
	}
	if policy.Namespace == "" {
		policy.Namespace = t.name
	}
	return t.WriteThrough(ctx, key, value, version, bus, policy.Namespace, busKey)
}

func (t *Tier[K, V]) StrongInvalidate(
	ctx context.Context,
	policy StrongNamespacePolicy,
	key K,
	busKey string,
	barrier VersionBarrier,
	bus *Bus,
) error {
	if t == nil {
		return nil
	}
	if policy.Namespace == "" {
		policy.Namespace = t.name
	}
	return t.InvalidateThrough(ctx, key, bus, policy.Namespace, busKey, barrier.Version)
}

//nolint:gocyclo,cyclop // Keep the consistency decisions in one ordered path.
func (t *Tier[K, V]) loadVersionedThroughL2(
	ctx context.Context,
	key K,
	minVersion int64,
	loader VersionedLoadFunc[K, V],
) (cacheEntry[V], error) {
	if !t.disableL2 && t.l2 != nil {
		entry, err := t.l2.Get(ctx, key)
		switch {
		case err == nil && entry.Barrier:
			recordCacheOperation(ctx, t.name, "barrier")
			if minVersion < entry.Version {
				minVersion = entry.Version
			}
			if t.cfg.OnL2Miss != nil {
				t.cfg.OnL2Miss()
			}
		case err == nil && entry.Version >= minVersion:
			recordCacheOperation(ctx, t.name, "hit")
			if t.l1Available() {
				t.l1.Set(key, entry)
			}
			if t.cfg.OnL2Hit != nil {
				t.cfg.OnL2Hit()
			}
			return entry, nil
		case err == nil:
			recordCacheOperation(ctx, t.name, "stale")
			if t.cfg.OnL2Miss != nil {
				t.cfg.OnL2Miss()
			}
		case errors.Is(err, ErrCacheMiss):
			recordCacheOperation(ctx, t.name, "miss")
			if t.cfg.OnL2Miss != nil {
				t.cfg.OnL2Miss()
			}
		default:
			t.failOpen(ctx, "get", err)
		}
	}
	if loader == nil {
		return cacheEntry[V]{}, ErrCacheMiss
	}
	loaded, err := loader(ctx, key)
	if err != nil {
		return cacheEntry[V]{}, err
	}
	if t.negEnabled && t.isNegative(loaded.Value) && loaded.Version <= 0 && minVersion > 0 {
		loaded.Version = minVersion
	}
	if loaded.Version < minVersion {
		return cacheEntry[V]{}, fmt.Errorf("%w: got %d want at least %d", ErrStaleVersion, loaded.Version, minVersion)
	}
	entry := cacheEntry[V]{Version: loaded.Version, Value: t.sanitize(loaded.Value)}
	if t.negEnabled && t.isNegative(loaded.Value) {
		entry.Negative = true
	}
	if !t.disableL2 && t.l2 != nil {
		ok, casErr := t.l2.CompareAndSet(ctx, key, entry, t.ttl)
		if casErr != nil {
			t.failOpen(ctx, "cas_fill", casErr)
		} else if !ok {
			recordCacheCASReject(ctx, t.name)
			if t.cfg.OnCASRejected != nil {
				t.cfg.OnCASRejected()
			}
			newer, getErr := t.l2.Get(ctx, key)
			if getErr == nil && newer.Version >= minVersion {
				if t.l1Available() {
					t.l1.Set(key, newer)
				}
				return newer, nil
			}
		}
	}
	if t.l1Available() {
		t.l1.Set(key, entry)
	}
	return entry, nil
}
