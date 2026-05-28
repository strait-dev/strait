package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const redisRateLimitScript = `
local current = redis.call("INCR", KEYS[1])
redis.call("PEXPIRE", KEYS[1], ARGV[1], "NX")
local limit = tonumber(ARGV[2])
if current > limit then
  return {0, 0}
end
return {1, limit - current}
`

const effectivelyUnlimitedRequests = 1_000_000

// RateLimitResult contains the outcome of a rate limit check.
type RateLimitResult struct {
	Allowed   bool
	Remaining int
}

type RedisRateLimiter struct {
	client  *redis.Client
	enabled bool
}

func NewRedisRateLimiter(client *redis.Client, enabled bool) *RedisRateLimiter {
	return &RedisRateLimiter{client: client, enabled: enabled}
}

// Allow checks whether the request is within the rate limit. Returns the result
// with remaining quota. Fails open: returns allowed=true when Redis is unavailable.
func (r *RedisRateLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (RateLimitResult, error) {
	if !r.enabled || r.client == nil {
		return RateLimitResult{Allowed: true, Remaining: limit}, nil
	}
	if limit <= 0 || window <= 0 {
		return RateLimitResult{Allowed: true, Remaining: limit}, nil
	}
	if isEffectivelyUnlimited(limit, window) {
		return RateLimitResult{Allowed: true, Remaining: limit}, nil
	}

	vals, err := r.client.Eval(ctx, redisRateLimitScript, []string{key}, window.Milliseconds(), limit).Int64Slice()
	if err != nil {
		return RateLimitResult{Allowed: true, Remaining: limit}, nil //nolint:nilerr // fail open: Redis rate limiting is advisory on this path
	}

	if len(vals) < 2 {
		return RateLimitResult{Allowed: true, Remaining: limit}, nil
	}

	return RateLimitResult{
		Allowed:   vals[0] == 1,
		Remaining: int(vals[1]),
	}, nil
}

func isEffectivelyUnlimited(limit int, window time.Duration) bool {
	return limit >= effectivelyUnlimitedRequests && window <= time.Hour
}

// AllowStrict checks whether the request is within the rate limit. Unlike
// Allow, it fails closed: if Redis is unavailable, an error is returned so
// the caller can deny the request rather than silently pass it through.
// Use this for security-sensitive rate limits (e.g. audit log export) where
// a compromised or down Redis must not open the floodgates.
//
// When the limiter is disabled or has no client, AllowStrict returns
// allowed=true (no rate limit). Callers that require rate limiting even
// in degraded deployments must guard against a nil/disabled limiter
// separately.
func (r *RedisRateLimiter) AllowStrict(ctx context.Context, key string, limit int, window time.Duration) (RateLimitResult, error) {
	if !r.enabled || r.client == nil {
		return RateLimitResult{Allowed: true, Remaining: limit}, nil
	}
	if limit <= 0 || window <= 0 {
		return RateLimitResult{Allowed: true, Remaining: limit}, nil
	}

	vals, err := r.client.Eval(ctx, redisRateLimitScript, []string{key}, window.Milliseconds(), limit).Int64Slice()
	if err != nil {
		return RateLimitResult{}, err
	}

	if len(vals) < 2 {
		return RateLimitResult{}, fmt.Errorf("rate limit: unexpected script response length %d", len(vals))
	}

	return RateLimitResult{
		Allowed:   vals[0] == 1,
		Remaining: int(vals[1]),
	}, nil
}
