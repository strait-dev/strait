package queue

import (
	"sync"
	"time"
)

// ClaimCursor is the per-worker Brandur-style hint that lets the dequeue
// query skip past dead tuples at the left edge of the job_runs partial
// index. A worker tracks the maximum (created_at, id) it has successfully
// claimed and feeds that pair back into subsequent dequeue calls so
// Postgres can position the B-tree scan past those rows without visiting
// them.
//
// The cursor must be reset periodically (typically every 60s) so that
// re-queued runs, retries with next_retry_at in the past, or runs with
// backdated created_at remain reachable.
type ClaimCursor struct {
	mu        sync.Mutex
	createdAt time.Time
	id        string
	resetAt   time.Time
	interval  time.Duration
}

// NewClaimCursor builds a cursor with the given reset interval. Zero or
// negative values default to 60s.
func NewClaimCursor(resetInterval time.Duration) *ClaimCursor {
	if resetInterval <= 0 {
		resetInterval = 60 * time.Second
	}
	return &ClaimCursor{
		interval: resetInterval,
		resetAt:  time.Now().Add(resetInterval),
	}
}

// Snapshot returns the current cursor values or zero values if no cursor
// is active (either unset or past its reset time).
func (c *ClaimCursor) Snapshot() (time.Time, string, bool) {
	if c == nil {
		return time.Time{}, "", false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Now().After(c.resetAt) || c.id == "" {
		return time.Time{}, "", false
	}
	return c.createdAt, c.id, true
}

// Advance updates the cursor if the provided (createdAt, id) is strictly
// greater than the current value. Safe to call from multiple goroutines.
func (c *ClaimCursor) Advance(createdAt time.Time, id string) {
	if c == nil || id == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.shouldAdvanceLocked(createdAt, id) {
		c.createdAt = createdAt
		c.id = id
	}
}

func (c *ClaimCursor) shouldAdvanceLocked(createdAt time.Time, id string) bool {
	if c.id == "" {
		return true
	}
	if createdAt.After(c.createdAt) {
		return true
	}
	if createdAt.Equal(c.createdAt) && id > c.id {
		return true
	}
	return false
}

// Reset clears the cursor and schedules the next reset. Workers call this
// on empty dequeue results and on claim errors so progress does not leave
// older rows unreachable.
func (c *ClaimCursor) Reset() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.createdAt = time.Time{}
	c.id = ""
	c.resetAt = time.Now().Add(c.interval)
}

// ForceExpire moves the reset deadline to the current instant so the next
// Snapshot returns no cursor. Used by tests.
func (c *ClaimCursor) ForceExpire() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.resetAt = time.Now().Add(-time.Second)
}
