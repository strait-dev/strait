package worker

import (
	"hash/fnv"
	"sync"
	"sync/atomic"
)

const numShards = 64

// ShardedBulkhead is a concurrency limiter that shards locks by job ID to reduce
// contention. Instead of a single mutex protecting all job counters, each job ID
// is hashed to one of 64 shards, each with its own mutex.
type ShardedBulkhead struct {
	shards       [numShards]bulkheadShard
	defaultLimit int
}

type bulkheadShard struct {
	mu       sync.Mutex
	counters map[string]*atomic.Int32
}

// NewShardedBulkhead creates a ShardedBulkhead with the given default concurrency limit.
// A defaultLimit of 0 means unlimited (TryAcquire always returns true when maxConcurrency is also 0).
func NewShardedBulkhead(defaultLimit int) *ShardedBulkhead {
	b := &ShardedBulkhead{defaultLimit: defaultLimit}
	for i := range b.shards {
		b.shards[i].counters = make(map[string]*atomic.Int32)
	}
	return b
}

func (b *ShardedBulkhead) shard(jobID string) *bulkheadShard {
	h := fnv.New32a()
	_, _ = h.Write([]byte(jobID))
	return &b.shards[h.Sum32()%numShards]
}

// TryAcquire attempts to acquire a slot for the given jobID. Returns true if the
// slot was acquired, false if the concurrency limit has been reached.
func (b *ShardedBulkhead) TryAcquire(jobID string, maxConcurrency int) bool {
	if maxConcurrency <= 0 {
		maxConcurrency = b.defaultLimit
	}
	if maxConcurrency <= 0 {
		return true
	}

	s := b.shard(jobID)
	s.mu.Lock()
	defer s.mu.Unlock()

	counter, ok := s.counters[jobID]
	if !ok {
		counter = new(atomic.Int32)
		s.counters[jobID] = counter
	}

	cur := counter.Load()
	if int(cur) >= maxConcurrency {
		return false
	}
	counter.Add(1)
	return true
}

// Release releases a slot for the given jobID. If the count reaches 0, the
// counter is removed from the shard map to prevent unbounded growth.
func (b *ShardedBulkhead) Release(jobID string, maxConcurrency int) {
	if maxConcurrency <= 0 {
		maxConcurrency = b.defaultLimit
	}
	if maxConcurrency <= 0 {
		return
	}

	s := b.shard(jobID)
	s.mu.Lock()
	defer s.mu.Unlock()

	counter, ok := s.counters[jobID]
	if !ok {
		return
	}

	newVal := counter.Add(-1)
	if newVal <= 0 {
		delete(s.counters, jobID)
	}
}

// ActiveCount returns the current active count for a jobID. Intended for testing.
func (b *ShardedBulkhead) ActiveCount(jobID string) int {
	s := b.shard(jobID)
	s.mu.Lock()
	defer s.mu.Unlock()

	counter, ok := s.counters[jobID]
	if !ok {
		return 0
	}
	return int(counter.Load())
}
