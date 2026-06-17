package cache

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type scriptedL2[K comparable, V any] struct {
	getEntries []cacheEntry[V]
	getErrs    []error
	casOK      bool
	casErr     error
	casCalls   int
}

func (s *scriptedL2[K, V]) Get(_ context.Context, _ K) (cacheEntry[V], error) {
	if len(s.getEntries) == 0 {
		return cacheEntry[V]{}, ErrCacheMiss
	}
	entry := s.getEntries[0]
	s.getEntries = s.getEntries[1:]
	var err error
	if len(s.getErrs) > 0 {
		err = s.getErrs[0]
		s.getErrs = s.getErrs[1:]
	}
	return entry, err
}

func (s *scriptedL2[K, V]) Set(context.Context, K, cacheEntry[V], time.Duration) error {
	return nil
}

func (s *scriptedL2[K, V]) CompareAndSet(context.Context, K, cacheEntry[V], time.Duration) (bool, error) {
	s.casCalls++
	return s.casOK, s.casErr
}

func (s *scriptedL2[K, V]) Delete(context.Context, K) error {
	return nil
}

func TestBusMutationEdges(t *testing.T) {
	var nilBus *Bus
	require.Empty(t, nilBus.Origin())

	tests := []struct {
		name      string
		input     string
		wantExtra int
		wantJSON  string
	}{
		{
			name:      "empty",
			input:     "",
			wantExtra: 0,
			wantJSON:  `""`,
		},
		{
			name:      "plain",
			input:     "plain",
			wantExtra: 0,
			wantJSON:  `"plain"`,
		},
		{
			name:      "quote",
			input:     `a"b`,
			wantExtra: 1,
			wantJSON:  `"a\"b"`,
		},
		{
			name:      "backslash",
			input:     `a\b`,
			wantExtra: 1,
			wantJSON:  `"a\\b"`,
		},
		{
			name:      "backspace",
			input:     "a\bb",
			wantExtra: 5,
			wantJSON:  `"a\bb"`,
		},
		{
			name:      "formfeed",
			input:     "a\fb",
			wantExtra: 5,
			wantJSON:  `"a\fb"`,
		},
		{
			name:      "newline",
			input:     "a\nb",
			wantExtra: 5,
			wantJSON:  `"a\nb"`,
		},
		{
			name:      "carriage return",
			input:     "a\rb",
			wantExtra: 5,
			wantJSON:  `"a\rb"`,
		},
		{
			name:      "tab",
			input:     "a\tb",
			wantExtra: 5,
			wantJSON:  `"a\tb"`,
		},
		{
			name:      "generic control",
			input:     "a\x1fb",
			wantExtra: 5,
			wantJSON:  `"a\u001fb"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.wantExtra, jsonStringExtraBytes(tt.input))
			got := string(appendBusJSONString(nil, tt.input))
			require.Equal(t, tt.wantJSON, got)
			var decoded string
			require.NoError(t, json.Unmarshal([]byte(got), &decoded))
			require.Equal(t, tt.input, decoded)
		})
	}
}

func TestReadModelDeleteMutationEdges(t *testing.T) {
	var nilModel *ReadModel[string]
	require.NoError(t, nilModel.Delete(t.Context(), "missing"))
	ok, err := nilModel.DeleteVersion(t.Context(), "missing", 1)
	require.NoError(t, err)
	require.False(t, ok)

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	model := NewReadModel[string](ReadModelConfig[string]{
		Client:    rdb,
		Namespace: "delete_edges",
		TTL:       time.Minute,
	})

	ok, err = model.CompareAndSet(t.Context(), "delete", "cached", 1)
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, model.Delete(t.Context(), "delete"))
	_, err = model.Get(t.Context(), "delete")
	require.ErrorIs(t, err, ErrCacheMiss)

	ok, err = model.CompareAndSet(t.Context(), "versioned", "cached", 5)
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = model.DeleteVersion(t.Context(), "versioned", 0)
	require.Error(t, err)
	require.False(t, ok)
	ok, err = model.DeleteVersion(t.Context(), "versioned", 4)
	require.NoError(t, err)
	require.False(t, ok)
	ok, err = model.DeleteVersion(t.Context(), "versioned", 6)
	require.NoError(t, err)
	require.True(t, ok)
	_, err = model.Get(t.Context(), "versioned")
	require.ErrorIs(t, err, ErrCacheMiss)
}

func TestRegistryMutationEdges(t *testing.T) {
	NamespaceHandlerFuncs{}.InvalidateCacheKey(t.Context(), "k", 1)
	NamespaceHandlerFuncs{}.ApplyCacheUpdate(t.Context(), "k", 1, nil)

	var invalidated atomic.Int64
	var updated atomic.Int64
	handler := NamespaceHandlerFuncs{
		Invalidate: func(context.Context, string, int64) { invalidated.Add(1) },
		Update:     func(context.Context, string, int64, json.RawMessage) { updated.Add(1) },
	}
	handler.InvalidateCacheKey(t.Context(), "k", 1)
	handler.ApplyCacheUpdate(t.Context(), "k", 1, json.RawMessage(`{"version":1}`))
	require.Equal(t, int64(1), invalidated.Load())
	require.Equal(t, int64(1), updated.Load())

	var nilRegistry *Registry
	nilRegistry.Unregister("ns")
	registry := NewRegistry(RegistryConfig{Origin: "node-a"})
	registry.Register("ns", handler)
	registry.Unregister("")
	require.Equal(t, []string{"ns"}, registry.RegisteredNamespaces())
	registry.Unregister("ns")
	require.Empty(t, registry.RegisteredNamespaces())

	tier := NewTier[int, string](TierConfig[int, string]{
		Name:        "registry_edges",
		L2:          newFakeL2[int, string](),
		MaximumSize: 10,
		TTL:         time.Minute,
	})
	defer tier.Close()

	nilTierHandler := TierHandler[int, string]{}
	nilTierHandler.InvalidateCacheKey(t.Context(), "1", 1)
	nilTierHandler.ApplyCacheUpdate(t.Context(), "1", 1, json.RawMessage(`{"version":1}`))

	parseFalse := TierHandler[int, string]{
		Tier:  tier,
		Parse: func(string) (int, bool) { return 0, false },
	}
	parseFalse.InvalidateCacheKey(t.Context(), "1", 1)
	parseFalse.ApplyCacheUpdate(t.Context(), "1", 1, json.RawMessage(`{"version":1,"value":"ignored"}`))
	_, ok := tier.GetIfPresent(0)
	require.False(t, ok)

	require.False(t, TierHandler[int, string]{}.canApplyUpdate(json.RawMessage(`{"version":1}`)))
	require.False(t, TierHandler[int, string]{Tier: tier}.canApplyUpdate(json.RawMessage(`{"version":1}`)))
	require.False(t, TierHandler[int, string]{
		Tier:  tier,
		Parse: func(string) (int, bool) { return 0, true },
	}.canApplyUpdate(nil))
	require.True(t, TierHandler[int, string]{
		Tier:  tier,
		Parse: func(string) (int, bool) { return 1, true },
	}.canApplyUpdate(json.RawMessage(`{"version":1}`)))

	parseTrue := TierHandler[int, string]{
		Tier:  tier,
		Parse: func(string) (int, bool) { return 1, true },
	}
	parseTrue.ApplyCacheUpdate(t.Context(), "1", 1, json.RawMessage(`{`))

	UpdatingStringTierHandler[string]{}.ApplyCacheUpdate(t.Context(), "k", 1, json.RawMessage(`{"version":1}`))
	UpdatingStringTierHandler[string]{Tier: NewTier[string, string](TierConfig[string, string]{
		Name:        "updating_handler_edges",
		L2:          newFakeL2[string, string](),
		MaximumSize: 10,
		TTL:         time.Minute,
	})}.ApplyCacheUpdate(t.Context(), "k", 1, json.RawMessage(`{`))
}

func TestTierMutationEdges(t *testing.T) {
	deleteErr := errors.New("delete failed")
	l2 := newFakeL2[string, string]()
	l2.delErr = deleteErr
	var failOpenOp string
	tier := NewTier[string, string](TierConfig[string, string]{
		Name:        "tier_delete_edges",
		L2:          l2,
		MaximumSize: 10,
		TTL:         time.Minute,
		OnFailOpen: func(operation string, err error) {
			require.ErrorIs(t, err, deleteErr)
			failOpenOp = operation
		},
	})
	defer tier.Close()
	tier.Invalidate(t.Context(), "k")
	require.Equal(t, "delete", failOpenOp)

	var nilTier *Tier[string, string]
	require.Zero(t, nilTier.Stats())
	require.Zero(t, (&Tier[string, string]{}).Stats())
}

func TestTierApplyUpdateMutationEdges(t *testing.T) {
	var l1Rejects atomic.Int64
	tier := NewTier[string, string](TierConfig[string, string]{
		Name:        "apply_update_l1_reject",
		L2:          newFakeL2[string, string](),
		MaximumSize: 10,
		TTL:         time.Minute,
		OnCASRejected: func() {
			l1Rejects.Add(1)
		},
	})
	defer tier.Close()
	require.NoError(t, tier.Set(t.Context(), "k", "current", 10))
	tier.applyUpdate(t.Context(), "k", cacheEntry[string]{Version: 9, Value: "stale"})
	got, ok := tier.GetIfPresent("k")
	require.True(t, ok)
	require.Equal(t, "current", got)
	require.Equal(t, int64(1), l1Rejects.Load())

	getErr := errors.New("redis unavailable")
	l2 := newFakeL2[string, string]()
	l2.values["k"] = cacheEntry[string]{Version: 10, Value: "newer"}
	l2.getErr = getErr
	var failOpenOp string
	l2RejectGetErr := NewTier[string, string](TierConfig[string, string]{
		Name:      "apply_update_l2_get_err",
		L2:        l2,
		DisableL1: true,
		TTL:       time.Minute,
		OnFailOpen: func(operation string, err error) {
			require.ErrorIs(t, err, getErr)
			failOpenOp = operation
		},
	})
	l2RejectGetErr.applyUpdate(t.Context(), "k", cacheEntry[string]{Version: 9, Value: "stale"})
	require.Equal(t, "bus_update_get", failOpenOp)

	l2 = newFakeL2[string, string]()
	l2.values["k"] = cacheEntry[string]{Version: 10, Value: "newer"}
	l2RejectNewer := NewTier[string, string](TierConfig[string, string]{
		Name:        "apply_update_l2_newer",
		L2:          l2,
		MaximumSize: 10,
		TTL:         time.Minute,
	})
	defer l2RejectNewer.Close()
	l2RejectNewer.applyUpdate(t.Context(), "k", cacheEntry[string]{Version: 9, Value: "stale"})
	got, ok = l2RejectNewer.GetIfPresent("k")
	require.True(t, ok)
	require.Equal(t, "newer", got)

	l2 = newFakeL2[string, string]()
	l2.values["k"] = cacheEntry[string]{Version: 9, Value: "same-version-current"}
	l2RejectEqual := NewTier[string, string](TierConfig[string, string]{
		Name:        "apply_update_l2_equal",
		L2:          l2,
		MaximumSize: 10,
		TTL:         time.Minute,
	})
	defer l2RejectEqual.Close()
	l2RejectEqual.applyUpdate(t.Context(), "k", cacheEntry[string]{Version: 9, Value: "incoming"})
	got, ok = l2RejectEqual.GetIfPresent("k")
	require.True(t, ok)
	require.Equal(t, "incoming", got)
}

func TestTierLoadThroughL2MutationEdges(t *testing.T) {
	var misses atomic.Int64
	l2 := newFakeL2[string, string]()
	l2.values["barrier"] = cacheEntry[string]{Version: 7, Barrier: true}
	l2.values["stale"] = cacheEntry[string]{Version: 2, Value: "stale"}
	tier := NewTier[string, string](TierConfig[string, string]{
		Name:        "load_l2_edges",
		L2:          l2,
		MaximumSize: 10,
		TTL:         time.Minute,
		OnL2Miss: func() {
			misses.Add(1)
		},
	})
	defer tier.Close()

	got, err := tier.GetConsistent(t.Context(), "barrier", 3, func(context.Context, string) (string, error) {
		return "loaded-after-barrier", nil
	})
	require.NoError(t, err)
	require.Equal(t, "loaded-after-barrier", got)
	require.Equal(t, int64(1), misses.Load())
	require.Equal(t, int64(7), l2.values["barrier"].Version)

	got, err = tier.GetConsistent(t.Context(), "stale", 5, func(context.Context, string) (string, error) {
		return "loaded-after-stale", nil
	})
	require.NoError(t, err)
	require.Equal(t, "loaded-after-stale", got)
	require.Equal(t, int64(2), misses.Load())

	var hits atomic.Int64
	l2Hit := newFakeL2[string, string]()
	l2Hit.values["k"] = cacheEntry[string]{Version: 5, Value: "from-l2"}
	hitTier := NewTier[string, string](TierConfig[string, string]{
		Name:      "load_l2_hit",
		L2:        l2Hit,
		DisableL1: true,
		TTL:       time.Minute,
		OnL2Hit: func() {
			hits.Add(1)
		},
	})
	got, err = hitTier.GetConsistent(t.Context(), "k", 5, func(context.Context, string) (string, error) {
		return "unexpected-loader", nil
	})
	require.NoError(t, err)
	require.Equal(t, "from-l2", got)
	require.Equal(t, int64(1), hits.Load())
}

func TestTierLoadThroughL2CASRejectMutationEdges(t *testing.T) {
	var rejects atomic.Int64
	l2 := &scriptedL2[string, string]{
		getEntries: []cacheEntry[string]{
			{},
			{Version: 11, Value: "writer"},
		},
		getErrs: []error{ErrCacheMiss, nil},
		casOK:   false,
	}
	tier := NewTier[string, string](TierConfig[string, string]{
		Name:      "load_l2_cas_reject",
		L2:        l2,
		DisableL1: true,
		TTL:       time.Minute,
		OnCASRejected: func() {
			rejects.Add(1)
		},
	})
	got, err := tier.GetConsistent(t.Context(), "k", 10, func(context.Context, string) (string, error) {
		return "loader", nil
	})
	require.NoError(t, err)
	require.Equal(t, "writer", got)
	require.Equal(t, int64(1), rejects.Load())
	require.Equal(t, 1, l2.casCalls)

	barrierL2 := &scriptedL2[string, string]{
		getEntries: []cacheEntry[string]{
			{},
			{Version: 12, Barrier: true},
		},
		getErrs: []error{ErrCacheMiss, nil},
		casOK:   false,
	}
	barrierTier := NewTier[string, string](TierConfig[string, string]{
		Name:      "load_l2_cas_reject_barrier",
		L2:        barrierL2,
		DisableL1: true,
		TTL:       time.Minute,
	})
	_, err = barrierTier.GetConsistent(t.Context(), "k", 10, func(context.Context, string) (string, error) {
		return "loader", nil
	})
	require.ErrorIs(t, err, ErrStaleVersion)
}

func TestTierVersionedLoadMutationEdges(t *testing.T) {
	var misses atomic.Int64
	var hits atomic.Int64
	l2 := newFakeL2[string, string]()
	l2.values["barrier"] = cacheEntry[string]{Version: 7, Barrier: true}
	l2.values["hit"] = cacheEntry[string]{Version: 9, Value: "from-l2"}
	tier := NewTier[string, string](TierConfig[string, string]{
		Name:      "versioned_l2_edges",
		L2:        l2,
		DisableL1: true,
		TTL:       time.Minute,
		OnL2Miss: func() {
			misses.Add(1)
		},
		OnL2Hit: func() {
			hits.Add(1)
		},
	})

	got, err := tier.GetConsistentVersioned(t.Context(), "barrier", 3, versionedStringLoader("loaded", 7))
	require.NoError(t, err)
	require.Equal(t, Versioned[string]{Value: "loaded", Version: 7}, got)
	require.Equal(t, int64(1), misses.Load())

	got, err = tier.GetConsistentVersioned(t.Context(), "hit", 9, versionedStringLoader("unexpected", 10))
	require.NoError(t, err)
	require.Equal(t, Versioned[string]{Value: "from-l2", Version: 9}, got)
	require.Equal(t, int64(1), hits.Load())

	l2.values["stale"] = cacheEntry[string]{Version: 2, Value: "stale"}
	got, err = tier.GetConsistentVersioned(t.Context(), "stale", 5, versionedStringLoader("loaded", 5))
	require.NoError(t, err)
	require.Equal(t, Versioned[string]{Value: "loaded", Version: 5}, got)
	require.Equal(t, int64(2), misses.Load())
}

func TestTierVersionedCASRejectMutationEdges(t *testing.T) {
	l2 := &scriptedL2[string, string]{
		getEntries: []cacheEntry[string]{
			{},
			{Version: 11, Value: "writer"},
		},
		getErrs: []error{ErrCacheMiss, nil},
		casOK:   false,
	}
	tier := NewTier[string, string](TierConfig[string, string]{
		Name:      "versioned_l2_cas_reject",
		L2:        l2,
		DisableL1: true,
		TTL:       time.Minute,
	})
	got, err := tier.GetConsistentVersioned(t.Context(), "k", 10, versionedStringLoader("loader", 10))
	require.NoError(t, err)
	require.Equal(t, Versioned[string]{Value: "writer", Version: 11}, got)

	lowVersionL2 := &scriptedL2[string, string]{
		getEntries: []cacheEntry[string]{
			{},
			{Version: 9, Value: "too-old"},
		},
		getErrs: []error{ErrCacheMiss, nil},
		casOK:   false,
	}
	lowVersionTier := NewTier[string, string](TierConfig[string, string]{
		Name:      "versioned_l2_cas_reject_low_version",
		L2:        lowVersionL2,
		DisableL1: true,
		TTL:       time.Minute,
	})
	got, err = lowVersionTier.GetConsistentVersioned(t.Context(), "k", 10, versionedStringLoader("loader", 10))
	require.NoError(t, err)
	require.Equal(t, Versioned[string]{Value: "loader", Version: 10}, got)
}

func TestRedisL2MutationEdges(t *testing.T) {
	var nilL2 *redisL2[string, string]
	require.NoError(t, nilL2.Set(t.Context(), "k", cacheEntry[string]{Version: 1, Value: "ignored"}, time.Minute))
	require.NoError(t, nilL2.Delete(t.Context(), "k"))

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{
		Addr:         mr.Addr(),
		MaxRetries:   -1,
		DialTimeout:  25 * time.Millisecond,
		ReadTimeout:  25 * time.Millisecond,
		WriteTimeout: 25 * time.Millisecond,
	})
	t.Cleanup(func() { _ = rdb.Close() })
	l2 := NewRedisL2[string, string](RedisL2Config[string, string]{
		Client:    rdb,
		Namespace: "redis_edges",
	}).(*redisL2[string, string])
	mr.Close()

	err := l2.Set(t.Context(), "k", cacheEntry[string]{Version: 1, Value: "value"}, time.Minute)
	require.Error(t, err)
	require.Contains(t, err.Error(), "redis cache set")
	err = l2.Delete(t.Context(), "k")
	require.Error(t, err)
	require.Contains(t, err.Error(), "redis cache delete")
}
