package cache

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestStrict_RedisCompareAndSetRejectsOutOfOrderWrites(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	l2 := NewRedisL2[string, string](RedisL2Config[string, string]{
		Client:    rdb,
		Namespace: "cas_test",
	})

	ok, err := l2.CompareAndSet(t.Context(), "k", cacheEntry[string]{Version: 10, Value: "new"}, time.Minute)
	require.NoError(t, err)
	require.True(t,
		ok)

	ok, err = l2.CompareAndSet(t.Context(), "k", cacheEntry[string]{Version: 9, Value: "old"}, time.Minute)
	require.NoError(t, err)
	require.False(t,
		ok)

	entry, err := l2.Get(t.Context(), "k")
	require.NoError(t, err)
	require.False(t,
		entry.
			Version !=
			10 || entry.Value != "new",
	)

}

func TestNewCacheCore_RedisValueSizeCap(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	l2 := NewRedisL2[string, string](RedisL2Config[string, string]{
		Client:        rdb,
		Namespace:     "cap_test",
		MaxValueBytes: 16,
	})

	err := l2.Set(t.Context(), "k", cacheEntry[string]{Value: "this payload is too large"}, time.Minute)
	require.Error(t,
		err)

}
