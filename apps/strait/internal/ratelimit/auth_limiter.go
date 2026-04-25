package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const authFailKeyPrefix = "auth:fail:"

// AuthLimiterThreshold defines a lockout level triggered at a given failure count.
type AuthLimiterThreshold struct {
	Failures int
	Lockout  time.Duration
}

func authFailWindow() time.Duration {
	return 15 * time.Minute
}

func defaultAuthThresholds() []AuthLimiterThreshold {
	return []AuthLimiterThreshold{
		{Failures: 50, Lockout: 15 * time.Minute},
		{Failures: 25, Lockout: 5 * time.Minute},
		{Failures: 10, Lockout: 1 * time.Minute},
	}
}

// DefaultAuthThresholds are the progressive lockout levels.
var DefaultAuthThresholds = defaultAuthThresholds()

// AuthLimiter tracks failed authentication attempts per IP in Redis
// and enforces progressive lockout.
type AuthLimiter struct {
	client     *redis.Client
	enabled    bool
	thresholds []AuthLimiterThreshold
}

// NewAuthLimiter creates an AuthLimiter. If client is nil or enabled is false,
// all operations are no-ops (fail open).
func NewAuthLimiter(client *redis.Client, enabled bool) *AuthLimiter {
	return &AuthLimiter{
		client:     client,
		enabled:    enabled,
		thresholds: DefaultAuthThresholds,
	}
}

// RecordFailure increments the failure count for the given IP.
func (a *AuthLimiter) RecordFailure(ctx context.Context, ip string) {
	if !a.isActive() {
		return
	}
	key := authFailKeyPrefix + ip
	pipe := a.client.Pipeline()
	pipe.Incr(ctx, key)
	pipe.PExpire(ctx, key, authFailWindow())
	_, _ = pipe.Exec(ctx) // best-effort; fail open
}

// IsBlocked checks whether the IP is currently locked out due to excessive
// failed auth attempts. Returns the lockout duration if blocked.
func (a *AuthLimiter) IsBlocked(ctx context.Context, ip string) (bool, time.Duration) {
	if !a.isActive() {
		return false, 0
	}
	key := authFailKeyPrefix + ip
	count, err := a.client.Get(ctx, key).Int()
	if err != nil {
		return false, 0 // fail open: key missing or Redis error
	}

	for _, t := range a.thresholds {
		if count >= t.Failures {
			return true, t.Lockout
		}
	}
	return false, 0
}

// Reset clears the failure count for the given IP. Useful after successful auth
// or for testing.
func (a *AuthLimiter) Reset(ctx context.Context, ip string) {
	if !a.isActive() {
		return
	}
	a.client.Del(ctx, authFailKeyPrefix+ip)
}

func (a *AuthLimiter) isActive() bool {
	return a != nil && a.enabled && a.client != nil
}

// BlockedError returns a formatted error message for locked-out IPs.
func BlockedError(retryAfter time.Duration) string {
	return fmt.Sprintf("too many failed authentication attempts; retry after %s", retryAfter.Truncate(time.Second))
}
