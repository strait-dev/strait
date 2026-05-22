package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const authFailKeyPrefix = "auth:fail:"
const authBlockKeyPrefix = "auth:block:"

const (
	AuthScopeAPIKey         = "api_key"
	AuthScopeOIDC           = "oidc"
	AuthScopeInternalSecret = "internal_secret"
	AuthScopeGRPCWorker     = "grpc_worker"
)

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
	a.RecordFailureScoped(ctx, ip, AuthScopeAPIKey)
}

func (a *AuthLimiter) RecordFailureScoped(ctx context.Context, ip, scope string) {
	if !a.isActive() {
		return
	}
	failKey := authFailKey(scope, ip)
	var incr *redis.IntCmd
	_, _ = a.client.TxPipelined(ctx, func(p redis.Pipeliner) error {
		incr = p.Incr(ctx, failKey)
		p.PExpire(ctx, failKey, authFailWindow())
		return nil
	})
	if incr == nil || incr.Err() != nil {
		return
	}
	if threshold, ok := a.thresholdForCount(int(incr.Val())); ok {
		_ = a.client.Set(ctx, authBlockKey(scope, ip), "1", threshold.Lockout).Err()
	}
}

// IsBlocked checks whether the IP is currently locked out due to excessive
// failed auth attempts. Returns the lockout duration if blocked.
func (a *AuthLimiter) IsBlocked(ctx context.Context, ip string) (bool, time.Duration) {
	return a.IsBlockedScoped(ctx, ip, AuthScopeAPIKey)
}

func (a *AuthLimiter) IsBlockedScoped(ctx context.Context, ip, scope string) (bool, time.Duration) {
	if !a.isActive() {
		return false, 0
	}
	ttl, err := a.client.PTTL(ctx, authBlockKey(scope, ip)).Result()
	if err != nil {
		return false, 0 // fail open: key missing or Redis error
	}
	if ttl > 0 {
		return true, ttl
	}
	return false, 0
}

// Reset clears the failure count for the given IP. Useful after successful auth
// or for testing.
func (a *AuthLimiter) Reset(ctx context.Context, ip string) {
	a.ResetScoped(ctx, ip, AuthScopeAPIKey)
}

func (a *AuthLimiter) ResetScoped(ctx context.Context, ip, scope string) {
	if !a.isActive() {
		return
	}
	a.client.Del(ctx, authFailKey(scope, ip), authBlockKey(scope, ip))
}

func (a *AuthLimiter) isActive() bool {
	return a != nil && a.enabled && a.client != nil
}

func (a *AuthLimiter) thresholdForCount(count int) (AuthLimiterThreshold, bool) {
	for _, t := range a.thresholds {
		if count >= t.Failures {
			return t, true
		}
	}
	return AuthLimiterThreshold{}, false
}

func authFailKey(scope, ip string) string {
	return authFailKeyPrefix + normalizeAuthScope(scope) + ":" + ip
}

func authBlockKey(scope, ip string) string {
	return authBlockKeyPrefix + normalizeAuthScope(scope) + ":" + ip
}

func normalizeAuthScope(scope string) string {
	if scope == "" {
		return AuthScopeAPIKey
	}
	return scope
}

// BlockedError returns a formatted error message for locked-out IPs.
func BlockedError(retryAfter time.Duration) string {
	return fmt.Sprintf("too many failed authentication attempts; retry after %s", retryAfter.Truncate(time.Second))
}
