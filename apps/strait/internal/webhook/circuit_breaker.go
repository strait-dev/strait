package webhook

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"
	"unsafe"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	defaultWebhookFailureThreshold = 5
	defaultWebhookFailureWindow    = time.Minute
	defaultWebhookOpenDuration     = time.Minute

	webhookCircuitFailuresPrefix = "webhook:circuit:failures:"
	webhookCircuitOpenPrefix     = "webhook:circuit:open:"
)

type WebhookCircuitBreaker interface {
	CanDeliver(ctx context.Context, url string) (bool, error)
	RecordSuccess(ctx context.Context, url string)
	RecordFailure(ctx context.Context, url string)
}

type RedisWebhookCircuitBreaker struct {
	client           *redis.Client
	enabled          bool
	failureThreshold int
	failureWindow    time.Duration
	openDuration     time.Duration
	now              func() time.Time
}

type RedisWebhookCircuitBreakerOption func(*RedisWebhookCircuitBreaker)

func WithWebhookCircuitBreakerThreshold(threshold int) RedisWebhookCircuitBreakerOption {
	return func(cb *RedisWebhookCircuitBreaker) {
		if threshold > 0 {
			cb.failureThreshold = threshold
		}
	}
}

func WithWebhookCircuitBreakerWindow(window time.Duration) RedisWebhookCircuitBreakerOption {
	return func(cb *RedisWebhookCircuitBreaker) {
		if window > 0 {
			cb.failureWindow = window
		}
	}
}

func WithWebhookCircuitBreakerOpenDuration(openDuration time.Duration) RedisWebhookCircuitBreakerOption {
	return func(cb *RedisWebhookCircuitBreaker) {
		if openDuration > 0 {
			cb.openDuration = openDuration
		}
	}
}

func NewRedisWebhookCircuitBreaker(client *redis.Client, enabled bool, opts ...RedisWebhookCircuitBreakerOption) *RedisWebhookCircuitBreaker {
	cb := &RedisWebhookCircuitBreaker{
		client:           client,
		enabled:          enabled,
		failureThreshold: defaultWebhookFailureThreshold,
		failureWindow:    defaultWebhookFailureWindow,
		openDuration:     defaultWebhookOpenDuration,
		now:              time.Now,
	}
	for _, opt := range opts {
		opt(cb)
	}
	return cb
}

func (cb *RedisWebhookCircuitBreaker) CanDeliver(ctx context.Context, url string) (bool, error) {
	if !cb.canUseRemoteState(url) {
		return true, nil
	}

	openKey := cb.openKey(url)
	open, err := cb.client.Exists(ctx, openKey).Result()
	if err != nil {
		return false, fmt.Errorf("circuit breaker exists: %w", err)
	}
	if open > 0 {
		return false, nil
	}

	failureKey := cb.failureKey(url)
	cutoff := strconv.FormatInt(cb.now().Add(-cb.failureWindow).UnixMilli(), 10)
	if err := cb.client.ZRemRangeByScore(ctx, failureKey, "-inf", cutoff).Err(); err != nil {
		return false, fmt.Errorf("circuit breaker cleanup: %w", err)
	}

	failures, err := cb.client.ZCard(ctx, failureKey).Result()
	if err != nil {
		return false, fmt.Errorf("circuit breaker count failures: %w", err)
	}

	if failures >= int64(cb.failureThreshold) {
		if err := cb.client.Set(ctx, openKey, "1", cb.openDuration).Err(); err != nil {
			return false, fmt.Errorf("circuit breaker set open: %w", err)
		}
		return false, nil
	}

	return true, nil
}

func (cb *RedisWebhookCircuitBreaker) RecordSuccess(ctx context.Context, url string) {
	if !cb.canUseRemoteState(url) {
		return
	}

	_ = cb.client.Del(ctx, cb.failureKey(url), cb.openKey(url)).Err()
}

func (cb *RedisWebhookCircuitBreaker) RecordFailure(ctx context.Context, url string) {
	if !cb.canUseRemoteState(url) {
		return
	}

	now := cb.now()
	failureKey := cb.failureKey(url)
	openKey := cb.openKey(url)

	_ = cb.client.ZAdd(ctx, failureKey, redis.Z{
		Score:  float64(now.UnixMilli()),
		Member: strconv.FormatInt(now.UnixNano(), 10) + ":" + uuid.Must(uuid.NewV7()).String(),
	}).Err()
	_ = cb.client.Expire(ctx, failureKey, cb.failureWindow).Err()

	// Evict entries older than the failure window before counting, mirroring
	// CanDeliver. Without this, stale failures still present in the sorted set
	// (the key TTL is reset on every failure) inflate ZCard and can trip the
	// breaker open even when recent failures are below the threshold.
	cutoff := strconv.FormatInt(now.Add(-cb.failureWindow).UnixMilli(), 10)
	_ = cb.client.ZRemRangeByScore(ctx, failureKey, "-inf", cutoff).Err()

	failures, err := cb.client.ZCard(ctx, failureKey).Result()
	if err == nil && failures >= int64(cb.failureThreshold) {
		_ = cb.client.Set(ctx, openKey, "1", cb.openDuration).Err()
	}
}

func (cb *RedisWebhookCircuitBreaker) canUseRemoteState(url string) bool {
	return cb.enabled && cb.client != nil && url != ""
}

func (cb *RedisWebhookCircuitBreaker) failureKey(url string) string {
	var out [len(webhookCircuitFailuresPrefix) + sha256.Size*2]byte
	copy(out[:], webhookCircuitFailuresPrefix)
	writeURLHashHex(out[len(webhookCircuitFailuresPrefix):], url)
	return string(out[:])
}

func (cb *RedisWebhookCircuitBreaker) openKey(url string) string {
	var out [len(webhookCircuitOpenPrefix) + sha256.Size*2]byte
	copy(out[:], webhookCircuitOpenPrefix)
	writeURLHashHex(out[len(webhookCircuitOpenPrefix):], url)
	return string(out[:])
}

func hashURL(url string) string {
	// Sum256 consumes the input synchronously and does not retain it, so this
	// avoids copying the URL string on every circuit-breaker key lookup.
	sum := sha256.Sum256(unsafe.Slice(unsafe.StringData(url), len(url)))
	var out [sha256.Size * 2]byte
	hex.Encode(out[:], sum[:])
	return string(out[:])
}

func writeURLHashHex(dst []byte, url string) {
	sum := sha256.Sum256(unsafe.Slice(unsafe.StringData(url), len(url)))
	hex.Encode(dst, sum[:])
}
