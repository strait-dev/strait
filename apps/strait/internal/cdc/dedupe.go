package cdc

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const defaultSharedDedupeTTL time.Duration = 600_000_000_000

type SharedDedupeStore struct {
	client redis.Cmdable
	ttl    time.Duration
}

func NewSharedDedupeStore(client redis.Cmdable, ttl time.Duration) *SharedDedupeStore {
	if client == nil {
		return nil
	}
	if ttl <= 0 {
		ttl = defaultSharedDedupeTTL
	}
	return &SharedDedupeStore{client: client, ttl: ttl}
}

func (s *SharedDedupeStore) Claim(ctx context.Context, key string) (bool, error) {
	if !s.canUseSharedDedupe(key) {
		return true, nil
	}
	ok, err := s.client.SetNX(ctx, s.redisKey(key), "1", s.ttl).Result()
	if err != nil {
		return true, fmt.Errorf("redis dedupe claim: %w", err)
	}
	return ok, nil
}

func (s *SharedDedupeStore) Release(ctx context.Context, key string) {
	if !s.canUseSharedDedupe(key) {
		return
	}
	_ = s.client.Del(ctx, s.redisKey(key)).Err()
}

func (s *SharedDedupeStore) canUseSharedDedupe(key string) bool {
	return s != nil && s.client != nil && strings.TrimSpace(key) != ""
}

func (s *SharedDedupeStore) redisKey(key string) string {
	return "strait:dedupe:" + key
}

type recentDedupe struct {
	mu       sync.Mutex
	limit    int
	seen     map[string]struct{}
	order    []string
	head     int
	count    int
	shared   *SharedDedupeStore
	fallback func(error)
}

func newRecentDedupe(limit int) *recentDedupe {
	if limit <= 0 {
		limit = 1
	}
	return &recentDedupe{
		limit: limit,
		seen:  make(map[string]struct{}, limit),
		order: make([]string, limit),
	}
}

func (d *recentDedupe) WithShared(shared *SharedDedupeStore, fallback func(error)) *recentDedupe {
	if d == nil {
		return nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.shared = shared
	d.fallback = fallback
	return d
}

func (d *recentDedupe) Remember(key string) bool {
	if d == nil || key == "" {
		return true
	}
	d.mu.Lock()

	if _, ok := d.seen[key]; ok {
		d.mu.Unlock()
		return false
	}
	d.seen[key] = struct{}{}
	if d.count < d.limit {
		idx := (d.head + d.count) % d.limit
		d.order[idx] = key
		d.count++
	} else {
		evicted := d.order[d.head]
		delete(d.seen, evicted)
		d.order[d.head] = key
		d.head = (d.head + 1) % d.limit
	}
	shared := d.shared
	fallback := d.fallback
	d.mu.Unlock()
	if shared != nil {
		ok, err := shared.Claim(context.Background(), key)
		if err != nil {
			recordSharedDedupeFallback("handler")
			if fallback != nil {
				fallback(err)
			}
			return true
		}
		return ok
	}
	return true
}

func (d *recentDedupe) Forget(key string) {
	if d == nil || key == "" {
		return
	}
	d.mu.Lock()
	shared := d.shared
	_, ok := d.seen[key]
	if ok {
		delete(d.seen, key)
	}
	for offset := range d.count {
		idx := (d.head + offset) % d.limit
		if d.order[idx] == key {
			for shift := offset; shift < d.count-1; shift++ {
				to := (d.head + shift) % d.limit
				from := (d.head + shift + 1) % d.limit
				d.order[to] = d.order[from]
			}
			tail := (d.head + d.count - 1) % d.limit
			d.order[tail] = ""
			d.count--
			if d.count == 0 {
				d.head = 0
			}
			break
		}
	}
	d.mu.Unlock()
	if shared != nil {
		shared.Release(context.Background(), key)
	}
}
