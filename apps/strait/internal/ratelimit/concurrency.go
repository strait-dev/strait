package ratelimit

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const redisReleaseSlotScript = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
end
return 0
`

type RedisConcurrencyLimiter struct {
	client  *redis.Client
	enabled bool
}

func NewRedisConcurrencyLimiter(client *redis.Client, enabled bool) *RedisConcurrencyLimiter {
	return &RedisConcurrencyLimiter{client: client, enabled: enabled}
}

func (r *RedisConcurrencyLimiter) Acquire(ctx context.Context, key string, maxConcurrent int, ttl time.Duration) (string, bool, error) {
	if !r.enabled || r.client == nil {
		return "", true, nil
	}
	if maxConcurrent <= 0 {
		return "", false, errors.New("maxConcurrent must be positive")
	}
	if ttl <= 0 {
		return "", false, errors.New("ttl must be positive")
	}

	for slot := range maxConcurrent {
		id := uuid.NewString()
		slotKey := redisConcurrencySlotKey(key, slot)
		err := r.client.SetArgs(ctx, slotKey, id, redis.SetArgs{
			Mode: "NX",
			TTL:  ttl,
		}).Err()
		if errors.Is(err, redis.Nil) {
			continue
		}
		if err != nil {
			return "", true, nil //nolint:nilerr // fail open: Redis concurrency limits are advisory on this path
		}
		return fmt.Sprintf("%d:%s", slot, id), true, nil
	}

	return "", false, nil
}

func (r *RedisConcurrencyLimiter) Release(ctx context.Context, key string, token string) error {
	if !r.enabled || r.client == nil {
		return nil
	}

	slot, id, err := parseRedisConcurrencyToken(token)
	if err != nil {
		return err
	}

	slotKey := redisConcurrencySlotKey(key, slot)
	if _, err := r.client.Eval(ctx, redisReleaseSlotScript, []string{slotKey}, id).Int(); err != nil {
		return nil //nolint:nilerr // fail open: release is best-effort when Redis is unavailable
	}

	return nil
}

func redisConcurrencySlotKey(key string, slot int) string {
	var slotBuf [20]byte
	slotBytes := strconv.AppendInt(slotBuf[:0], int64(slot), 10)
	var b strings.Builder
	b.Grow(len("concurrency::") + len(key) + len(slotBytes))
	b.WriteString("concurrency:")
	b.WriteString(key)
	b.WriteByte(':')
	b.Write(slotBytes)
	return b.String()
}

func parseRedisConcurrencyToken(token string) (int, string, error) {
	slotText, id, ok := strings.Cut(token, ":")
	if !ok || id == "" {
		return 0, "", errors.New("invalid concurrency token")
	}

	slot, err := strconv.Atoi(slotText)
	if err != nil {
		return 0, "", fmt.Errorf("parse token slot: %w", err)
	}

	return slot, id, nil
}
