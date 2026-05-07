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
//
// We use TxPipelined (MULTI/EXEC) so the INCR and PExpire either both
// succeed or both fail. With a plain Pipeline a race could leave the
// key incremented but without a TTL — meaning a single failed login
// from a client at the right moment could permanently lock that IP
// out, since the counter would never expire on its own.
func (a *AuthLimiter) RecordFailure(ctx context.Context, ip string) {
	if !a.isActive() {
		return
	}
	key := authFailKeyPrefix + ip
	_, _ = a.client.TxPipelined(ctx, func(p redis.Pipeliner) error {
		p.Incr(ctx, key)
		p.PExpire(ctx, key, authFailWindow())
		return nil
	})
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
