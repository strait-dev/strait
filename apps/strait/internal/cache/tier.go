package cache

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	otter "github.com/maypok86/otter/v2"
	"github.com/maypok86/otter/v2/stats"
	"golang.org/x/sync/singleflight"
)

type ConsistencyLevel int

const (
	BoundedStaleness ConsistencyLevel = iota
	Strong
	Immutable
)

type LoadFunc[K comparable, V any] func(context.Context, K) (V, error)

type TierConfig[K comparable, V any] struct {
	Name             string
	L2               L2[K, V]
	Consistency      ConsistencyLevel
	MaximumSize      int
	MaximumWeight    uint64
	Weigher          func(K, V) uint32
	TTL              time.Duration
	RefreshAfter     time.Duration
	TTLJitter        float64
	DisableL1        bool
	DisableL2        bool
	EnableNegative   bool
	IsNegative       func(V) bool
	Clone            func(V) V
	Sanitize         func(V) V
	OnDelete         func(key K)
	OnFailOpen       func(operation string, err error)
	OnCASRejected    func()
	OnL2Hit          func()
	OnL2Miss         func()
	OnL2Set          func()
	OtterExecutor    func(func())
	OtterStats       *stats.Counter
	ForceSynchronous bool
}

type Tier[K comparable, V any] struct {
	name       string
	l1         *otter.Cache[K, cacheEntry[V]]
	l2         L2[K, V]
	cfg        TierConfig[K, V]
	stats      *stats.Counter
	loadGroup  singleflight.Group
	ttl        time.Duration
	disableL1  bool
	disableL2  bool
	negEnabled bool
}

func NewTier[K comparable, V any](cfg TierConfig[K, V]) *Tier[K, V] {
	counter := cfg.OtterStats
	if counter == nil {
		counter = stats.NewCounter()
	}
	t := &Tier[K, V]{
		name:       cfg.Name,
		l2:         cfg.L2,
		cfg:        cfg,
		stats:      counter,
		ttl:        JitterTTL(cfg.TTL, cfg.TTLJitter),
		disableL1:  cfg.DisableL1,
		disableL2:  cfg.DisableL2,
		negEnabled: cfg.EnableNegative,
	}
	if !cfg.DisableL1 {
		opts := &otter.Options[K, cacheEntry[V]]{
			StatsRecorder: counter,
			OnDeletion: func(e otter.DeletionEvent[K, cacheEntry[V]]) {
				if cfg.OnDelete != nil {
					cfg.OnDelete(e.Key)
				}
			},
		}
		if cfg.OtterExecutor != nil {
			opts.Executor = cfg.OtterExecutor
		}
		if cfg.MaximumWeight > 0 && cfg.Weigher != nil {
			opts.MaximumWeight = cfg.MaximumWeight
			opts.Weigher = func(k K, e cacheEntry[V]) uint32 {
				return cfg.Weigher(k, e.Value)
			}
		} else {
			opts.MaximumSize = cfg.MaximumSize
			if opts.MaximumSize <= 0 {
				opts.MaximumSize = 10_000
			}
		}
		if cfg.TTL > 0 {
			opts.ExpiryCalculator = otter.ExpiryWriting[K, cacheEntry[V]](cfg.TTL)
		}
		if cfg.RefreshAfter > 0 {
			opts.RefreshCalculator = otter.RefreshWriting[K, cacheEntry[V]](cfg.RefreshAfter)
		}
		t.l1 = otter.Must(opts)
	}
	return t
}

func (t *Tier[K, V]) Stop() {
	if t == nil || t.l1 == nil {
		return
	}
	t.l1.StopAllGoroutines()
}

func (t *Tier[K, V]) Close() {
	t.Stop()
}

func (t *Tier[K, V]) l1Available() bool {
	if t == nil {
		return false
	}
	if t.disableL1 {
		return false
	}
	return t.l1 != nil
}

func (t *Tier[K, V]) Get(ctx context.Context, key K, loader LoadFunc[K, V]) (V, error) {
	if t == nil {
		var zero V
		return zero, fmt.Errorf("cache tier is nil")
	}
	if !t.l1Available() {
		entry, err := t.loadThroughL2(ctx, key, 0, loader)
		return t.valueFromEntry(entry, err)
	}
	l1Loader := otter.LoaderFunc[K, cacheEntry[V]](func(
		loadCtx context.Context,
		loadKey K,
	) (cacheEntry[V], error) {
		return t.loadThroughL2(loadCtx, loadKey, 0, loader)
	})
	entry, err := t.l1.Get(ctx, key, l1Loader)
	return t.valueFromEntry(entry, err)
}

func (t *Tier[K, V]) GetConsistent(
	ctx context.Context,
	key K,
	minVersion int64,
	loader LoadFunc[K, V],
) (V, error) {
	if t == nil {
		var zero V
		return zero, fmt.Errorf("cache tier is nil")
	}
	if t.l1Available() {
		if entry, ok := t.l1.GetIfPresent(key); ok && entry.Version >= minVersion {
			return t.valueFromEntry(entry, nil)
		}
	}
	entry, err, _ := t.loadGroup.Do(tierSingleflightKey(t.name, key, minVersion, false), func() (any, error) {
		return t.loadThroughL2(ctx, key, minVersion, loader)
	})
	if err != nil {
		var zero V
		return zero, err
	}
	return t.valueFromEntry(entry.(cacheEntry[V]), nil)
}

func (t *Tier[K, V]) Set(ctx context.Context, key K, value V, version int64) error {
	entry := cacheEntry[V]{Version: version, Value: t.sanitize(value)}
	if t.negEnabled && t.isNegative(value) {
		entry.Negative = true
	}
	if t.l1Available() {
		t.l1.Set(key, entry)
	}
	if t.disableL2 || t.l2 == nil {
		return nil
	}
	if err := t.l2.Set(ctx, key, entry, t.ttl); err != nil {
		t.failOpen(ctx, "set", err)
		return nil
	}
	if t.cfg.OnL2Set != nil {
		t.cfg.OnL2Set()
	}
	return nil
}

func (t *Tier[K, V]) CompareAndSet(ctx context.Context, key K, value V, version int64) (bool, error) {
	entry := cacheEntry[V]{Version: version, Value: t.sanitize(value)}
	if t.negEnabled && t.isNegative(value) {
		entry.Negative = true
	}
	if t.disableL2 || t.l2 == nil {
		if t.l1Available() {
			t.l1.Set(key, entry)
		}
		return true, nil
	}
	ok, err := t.l2.CompareAndSet(ctx, key, entry, t.ttl)
	if err != nil {
		t.failOpen(ctx, "cas", err)
		return false, nil
	}
	if !ok {
		recordCacheCASReject(ctx, t.name)
		if t.cfg.OnCASRejected != nil {
			t.cfg.OnCASRejected()
		}
		return false, nil
	}
	if t.l1Available() {
		t.l1.Set(key, entry)
	}
	if t.cfg.OnL2Set != nil {
		t.cfg.OnL2Set()
	}
	return true, nil
}

func (t *Tier[K, V]) Invalidate(ctx context.Context, key K) {
	if t == nil {
		return
	}
	if t.l1Available() {
		t.l1.Invalidate(key)
	}
	if !t.disableL2 && t.l2 != nil {
		if err := t.l2.Delete(ctx, key); err != nil {
			t.failOpen(ctx, "delete", err)
		}
	}
}

func (t *Tier[K, V]) applyUpdate(ctx context.Context, key K, entry cacheEntry[V]) {
	if t == nil {
		return
	}
	if entry.Barrier {
		t.applyBarrier(ctx, key, entry.Version)
		return
	}
	if t.l1Available() {
		if current, ok := t.l1.GetIfPresent(key); ok && current.Version > entry.Version {
			recordCacheCASReject(ctx, t.name)
			if t.cfg.OnCASRejected != nil {
				t.cfg.OnCASRejected()
			}
			return
		}
	}
	if !t.disableL2 && t.l2 != nil {
		ok, err := t.l2.CompareAndSet(ctx, key, entry, t.ttl)
		if err != nil {
			t.failOpen(ctx, "bus_update_cas", err)
			return
		}
		if !ok {
			recordCacheCASReject(ctx, t.name)
			if t.cfg.OnCASRejected != nil {
				t.cfg.OnCASRejected()
			}
			newer, getErr := t.l2.Get(ctx, key)
			if getErr != nil {
				t.failOpen(ctx, "bus_update_get", getErr)
				return
			}
			if newer.Version > entry.Version {
				entry = newer
			}
		}
	}
	if t.l1Available() {
		t.l1.Set(key, entry)
	}
}

func (t *Tier[K, V]) GetIfPresent(key K) (V, bool) {
	if !t.l1Available() {
		var zero V
		return zero, false
	}
	entry, ok := t.l1.GetIfPresent(key)
	if !ok || entry.Negative || entry.Barrier {
		var zero V
		return zero, ok
	}
	return t.clone(entry.Value), true
}

func (t *Tier[K, V]) Stats() stats.Stats {
	if t == nil || t.stats == nil {
		return stats.Stats{}
	}
	return t.stats.Snapshot()
}

func (t *Tier[K, V]) loadThroughL2(
	ctx context.Context,
	key K,
	minVersion int64,
	loader LoadFunc[K, V],
) (cacheEntry[V], error) {
	if !t.disableL2 && t.l2 != nil {
		entry, err := t.l2.Get(ctx, key)
		if err == nil && entry.Barrier {
			recordCacheOperation(ctx, t.name, "barrier")
			if minVersion < entry.Version {
				minVersion = entry.Version
			}
			if t.cfg.OnL2Miss != nil {
				t.cfg.OnL2Miss()
			}
		} else if err == nil && entry.Version >= minVersion {
			recordCacheOperation(ctx, t.name, "hit")
			if t.l1Available() {
				t.l1.Set(key, entry)
			}
			if t.cfg.OnL2Hit != nil {
				t.cfg.OnL2Hit()
			}
			return entry, nil
		} else if err == nil {
			recordCacheOperation(ctx, t.name, "stale")
			if t.cfg.OnL2Miss != nil {
				t.cfg.OnL2Miss()
			}
		} else if errors.Is(err, ErrCacheMiss) {
			recordCacheOperation(ctx, t.name, "miss")
			if t.cfg.OnL2Miss != nil {
				t.cfg.OnL2Miss()
			}
		} else {
			t.failOpen(ctx, "get", err)
		}
	}
	if loader == nil {
		return cacheEntry[V]{}, ErrCacheMiss
	}
	value, err := loader(ctx, key)
	if err != nil {
		return cacheEntry[V]{}, err
	}
	entry := cacheEntry[V]{Value: t.sanitize(value)}
	if t.negEnabled && t.isNegative(value) {
		entry.Negative = true
	}
	if !t.disableL2 && t.l2 != nil {
		entry.Version = minVersion
		if ok, err := t.l2.CompareAndSet(ctx, key, entry, t.ttl); err != nil {
			t.failOpen(ctx, "cas_fill", err)
		} else if !ok {
			recordCacheCASReject(ctx, t.name)
			if t.cfg.OnCASRejected != nil {
				t.cfg.OnCASRejected()
			}
			newer, getErr := t.l2.Get(ctx, key)
			if getErr == nil {
				if newer.Barrier {
					return cacheEntry[V]{}, fmt.Errorf("%w: blocked by version barrier %d", ErrStaleVersion, newer.Version)
				}
				return newer, nil
			}
		}
	}
	return entry, nil
}

func (t *Tier[K, V]) valueFromEntry(entry cacheEntry[V], err error) (V, error) {
	if err != nil {
		var zero V
		return zero, err
	}
	if entry.Negative || entry.Barrier {
		var zero V
		return zero, nil
	}
	return t.clone(entry.Value), nil
}

func (t *Tier[K, V]) applyBarrier(ctx context.Context, key K, version int64) {
	if t == nil {
		return
	}
	if version <= 0 {
		version = time.Now().UnixNano()
	}
	entry := cacheEntry[V]{Version: version, Barrier: true}
	if t.l1Available() {
		t.l1.Invalidate(key)
	}
	if t.disableL2 || t.l2 == nil {
		return
	}
	ok, err := t.l2.CompareAndSet(ctx, key, entry, t.ttl)
	if err != nil {
		t.failOpen(ctx, "barrier", err)
		return
	}
	if !ok {
		recordCacheCASReject(ctx, t.name)
		if t.cfg.OnCASRejected != nil {
			t.cfg.OnCASRejected()
		}
	}
}

func (t *Tier[K, V]) clone(v V) V {
	if t != nil && t.cfg.Clone != nil {
		return t.cfg.Clone(v)
	}
	return v
}

func (t *Tier[K, V]) sanitize(v V) V {
	if t != nil && t.cfg.Sanitize != nil {
		return t.cfg.Sanitize(v)
	}
	return v
}

func (t *Tier[K, V]) isNegative(v V) bool {
	if t != nil && t.cfg.IsNegative != nil {
		return t.cfg.IsNegative(v)
	}
	rv := reflect.ValueOf(v)
	if !rv.IsValid() {
		return true
	}
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return rv.IsZero()
	}
}

func (t *Tier[K, V]) failOpen(ctx context.Context, operation string, err error) {
	if err != nil {
		recordCacheFailOpen(ctx, t.name, operation)
	}
	if t != nil && t.cfg.OnFailOpen != nil && err != nil {
		t.cfg.OnFailOpen(operation, err)
	}
}
