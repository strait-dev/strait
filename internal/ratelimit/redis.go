package ratelimit

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

const redisRateLimitScript = `
local current = redis.call("INCR", KEYS[1])
if current == 1 then
  redis.call("PEXPIRE", KEYS[1], ARGV[1])
end
if current > tonumber(ARGV[2]) then
  return 0
end
return 1
`

type RedisRateLimiter struct {
	client  *redis.Client
	enabled bool
}

func NewRedisRateLimiter(client *redis.Client, enabled bool) *RedisRateLimiter {
	return &RedisRateLimiter{client: client, enabled: enabled}
}

func (r *RedisRateLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	if !r.enabled || r.client == nil {
		return true, nil
	}
	if limit <= 0 || window <= 0 {
		return true, nil
	}

	allowed, err := r.client.Eval(ctx, redisRateLimitScript, []string{key}, window.Milliseconds(), limit).Int()
	if err != nil {
		return true, nil
	}

	return allowed == 1, nil
}
