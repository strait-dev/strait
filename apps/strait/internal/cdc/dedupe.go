package cdc

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const defaultSharedDedupeTTL = 10 * time.Minute

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
		order: make([]string, 0, limit),
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
	d.order = append(d.order, key)
	for len(d.order) > d.limit {
		evicted := d.order[0]
		delete(d.seen, evicted)
		copy(d.order, d.order[1:])
		d.order = d.order[:len(d.order)-1]
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
	for i, existing := range d.order {
		if existing == key {
			copy(d.order[i:], d.order[i+1:])
			d.order = d.order[:len(d.order)-1]
			break
		}
	}
	d.mu.Unlock()
	if shared != nil {
		shared.Release(context.Background(), key)
	}
}
