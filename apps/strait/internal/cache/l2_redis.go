package cache

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const defaultMaxRedisValueBytes = 1 << 20

type RedisKeyFunc[K comparable] func(K) string

type RedisL2Config[K comparable, V any] struct {
	Client        redis.Cmdable
	Namespace     string
	Key           RedisKeyFunc[K]
	Codec         Codec[cacheEntry[V]]
	MaxValueBytes int
}

type redisL2[K comparable, V any] struct {
	client        redis.Cmdable
	namespace     string
	key           RedisKeyFunc[K]
	codec         Codec[cacheEntry[V]]
	maxValueBytes int
}

func NewRedisL2[K comparable, V any](cfg RedisL2Config[K, V]) L2[K, V] {
	if !redisClientReady(cfg.Client) {
		return nil
	}
	codec := cfg.Codec
	if codec == nil {
		codec = JSONCodec[cacheEntry[V]]{}
	}
	keyFn := cfg.Key
	if keyFn == nil {
		keyFn = func(k K) string { return fmt.Sprint(k) }
	}
	maxBytes := cfg.MaxValueBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxRedisValueBytes
	}
	namespace := strings.TrimSpace(cfg.Namespace)
	if namespace == "" {
		namespace = "default"
	}
	return &redisL2[K, V]{
		client:        cfg.Client,
		namespace:     namespace,
		key:           keyFn,
		codec:         codec,
		maxValueBytes: maxBytes,
	}
}

func (r *redisL2[K, V]) Get(ctx context.Context, key K) (cacheEntry[V], error) {
	if r == nil || !redisClientReady(r.client) {
		var zero cacheEntry[V]
		return zero, ErrCacheMiss
	}
	raw, err := r.client.Get(ctx, r.redisKey(key)).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			var zero cacheEntry[V]
			return zero, ErrCacheMiss
		}
		var zero cacheEntry[V]
		return zero, fmt.Errorf("redis cache get: %w", err)
	}
	if r.maxValueBytes > 0 && len(raw) > r.maxValueBytes {
		var zero cacheEntry[V]
		return zero, fmt.Errorf(
			"redis cache value exceeds cap: %d > %d",
			len(raw),
			r.maxValueBytes,
		)
	}
	var entry cacheEntry[V]
	if err := r.codec.Unmarshal(raw, &entry); err != nil {
		return cacheEntry[V]{}, err
	}
	return entry, nil
}

func (r *redisL2[K, V]) Set(ctx context.Context, key K, entry cacheEntry[V], ttl time.Duration) error {
	if r == nil || !redisClientReady(r.client) {
		return nil
	}
	raw, err := r.codec.Marshal(entry)
	if err != nil {
		return err
	}
	if r.maxValueBytes > 0 && len(raw) > r.maxValueBytes {
		return fmt.Errorf(
			"redis cache value exceeds cap: %d > %d",
			len(raw),
			r.maxValueBytes,
		)
	}
	if err := r.client.Set(ctx, r.redisKey(key), raw, ttl).Err(); err != nil {
		return fmt.Errorf("redis cache set: %w", err)
	}
	return nil
}

func (r *redisL2[K, V]) CompareAndSet(
	ctx context.Context,
	key K,
	entry cacheEntry[V],
	ttl time.Duration,
) (bool, error) {
	if r == nil || !redisClientReady(r.client) {
		return false, nil
	}
	raw, err := r.codec.Marshal(entry)
	if err != nil {
		return false, err
	}
	if r.maxValueBytes > 0 && len(raw) > r.maxValueBytes {
		return false, fmt.Errorf(
			"redis cache value exceeds cap: %d > %d",
			len(raw),
			r.maxValueBytes,
		)
	}
	ttlMillis := int64(ttl / time.Millisecond)
	if ttl > 0 && ttlMillis <= 0 {
		ttlMillis = 1
	}
	res, err := redisCASScript.Run(ctx, r.client, []string{r.redisKey(key)}, entry.Version, raw, ttlMillis).Int()
	if err != nil {
		return false, fmt.Errorf("redis cache cas: %w", err)
	}
	return res == 1, nil
}

func (r *redisL2[K, V]) Delete(ctx context.Context, key K) error {
	if r == nil || !redisClientReady(r.client) {
		return nil
	}
	if err := r.client.Del(ctx, r.redisKey(key)).Err(); err != nil {
		return fmt.Errorf("redis cache delete: %w", err)
	}
	return nil
}

func (r *redisL2[K, V]) redisKey(key K) string {
	return "strait:cache:" + r.namespace + ":" + r.key(key)
}

func redisClientReady(client redis.Cmdable) bool {
	if client == nil {
		return false
	}
	value := reflect.ValueOf(client)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return !value.IsNil()
	default:
		return true
	}
}

//nolint:dupword // Lua control flow naturally closes nested blocks with adjacent end tokens.
var redisCASScript = redis.NewScript(`
local existing = redis.call("GET", KEYS[1])
if existing then
  local ok, decoded = pcall(cjson.decode, existing)
  if ok and decoded and decoded["version"] ~= nil then
    local current = tonumber(decoded["version"])
    local incoming = tonumber(ARGV[1])
    local existing_barrier = decoded["barrier"] == true
    local incoming_ok, incoming_decoded = pcall(cjson.decode, ARGV[2])
    local incoming_barrier = incoming_ok and incoming_decoded and incoming_decoded["barrier"] == true
    if current ~= nil and incoming ~= nil and incoming < current then
      return 0
    end
    if current ~= nil and incoming ~= nil and incoming == current and
      not (existing_barrier and not incoming_barrier) then
      return 0
    end
  end
end
if tonumber(ARGV[3]) ~= nil and tonumber(ARGV[3]) > 0 then
  redis.call("PSETEX", KEYS[1], ARGV[3], ARGV[2])
else
  redis.call("SET", KEYS[1], ARGV[2])
end
return 1
`)
