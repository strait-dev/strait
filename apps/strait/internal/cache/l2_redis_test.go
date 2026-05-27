package cache

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
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
	if err != nil {
		t.Fatalf("CompareAndSet(new) error = %v", err)
	}
	if !ok {
		t.Fatal("CompareAndSet(new) = false, want true")
	}
	ok, err = l2.CompareAndSet(t.Context(), "k", cacheEntry[string]{Version: 9, Value: "old"}, time.Minute)
	if err != nil {
		t.Fatalf("CompareAndSet(old) error = %v", err)
	}
	if ok {
		t.Fatal("CompareAndSet(old) = true, want false")
	}
	entry, err := l2.Get(t.Context(), "k")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if entry.Version != 10 || entry.Value != "new" {
		t.Fatalf("entry = %+v, want version 10 value new", entry)
	}
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
	if err == nil {
		t.Fatal("Set() error = nil, want value-size error")
	}
}
