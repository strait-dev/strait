package queue

import (
	"context"
	"errors"
	"fmt"
	"time"

	"strait/internal/store"

	"go.opentelemetry.io/otel"
)

// R3 Phase 7: per-project enqueue backpressure.
//
// Simple token bucket implemented in Postgres. The row is refilled on
// every consume: new_tokens = min(max, old_tokens + floor((now - last_refill) * refill_per_sec)).
// If the resulting balance is > 0, we decrement by 1 and allow the
// enqueue; otherwise we return ErrEnqueueThrottled with a RetryAfter
// estimate derived from refill_per_sec.

// ErrEnqueueThrottled is returned when a project exhausts its token
// bucket. Callers can check RetryAfter via AsThrottled.
var ErrEnqueueThrottled = errors.New("enqueue throttled: project rate limit exceeded")

// ThrottledError wraps ErrEnqueueThrottled with the suggested retry delay.
type ThrottledError struct {
	ProjectID  string
	RetryAfter time.Duration
}

func (e *ThrottledError) Error() string {
	return fmt.Sprintf("enqueue throttled: project %s retry after %v", e.ProjectID, e.RetryAfter)
}

func (e *ThrottledError) Unwrap() error { return ErrEnqueueThrottled }

// AsThrottled returns (*ThrottledError, true) if err wraps ErrEnqueueThrottled.
func AsThrottled(err error) (*ThrottledError, bool) {
	var t *ThrottledError
	if errors.As(err, &t) {
		return t, true
	}
	return nil, false
}

// BackpressureConfig controls the bucket parameters used when a project
// has no explicit project_rate_limits row yet.
type BackpressureConfig struct {
	DefaultMaxTokens    int
	DefaultRefillPerSec int
}

// Backpressure consults the project_rate_limits table to enforce a
// DB-side token bucket per project.
type Backpressure struct {
	db      store.DBTX
	cfg     BackpressureConfig
	enabled bool
}

// NewBackpressure builds a backpressure controller. When enabled is
// false, TryConsume is a no-op that always allows.
func NewBackpressure(db store.DBTX, cfg BackpressureConfig, enabled bool) *Backpressure {
	if cfg.DefaultMaxTokens <= 0 {
		cfg.DefaultMaxTokens = 1000
	}
	if cfg.DefaultRefillPerSec <= 0 {
		cfg.DefaultRefillPerSec = 100
	}
	return &Backpressure{db: db, cfg: cfg, enabled: enabled}
}

// TryConsume reserves one token from the project's bucket. Returns nil
// on success or *ThrottledError on exhaustion. Passes through
// unexpected DB errors so callers can distinguish throttle from outage.
func (b *Backpressure) TryConsume(ctx context.Context, projectID string) error {
	if b == nil || !b.enabled || projectID == "" {
		return nil
	}
	ctx, span := otel.Tracer("strait").Start(ctx, "queue.Backpressure.TryConsume")
	defer span.End()

	// Upsert-then-decrement in a single statement so concurrent enqueues
	// see an atomic view of the bucket. The WITH clause computes the
	// refilled balance; the UPDATE decrements only when positive.
	const sql = `
		INSERT INTO project_rate_limits (project_id, tokens, max_tokens, refill_per_sec, last_refill_at, updated_at)
		VALUES ($1, $2 - 1, $2, $3, NOW(), NOW())
		ON CONFLICT (project_id) DO UPDATE SET
		  tokens = CASE
		    WHEN LEAST(
		        project_rate_limits.max_tokens,
		        project_rate_limits.tokens +
		          GREATEST(0, FLOOR(EXTRACT(EPOCH FROM (NOW() - project_rate_limits.last_refill_at)) * project_rate_limits.refill_per_sec))::int
		    ) > 0
		    THEN LEAST(
		        project_rate_limits.max_tokens,
		        project_rate_limits.tokens +
		          GREATEST(0, FLOOR(EXTRACT(EPOCH FROM (NOW() - project_rate_limits.last_refill_at)) * project_rate_limits.refill_per_sec))::int
		    ) - 1
		    ELSE project_rate_limits.tokens
		  END,
		  last_refill_at = NOW(),
		  updated_at = NOW()
		RETURNING tokens, max_tokens, refill_per_sec`

	var tokens, maxTokens, refill int
	err := b.db.QueryRow(ctx, sql, projectID, b.cfg.DefaultMaxTokens, b.cfg.DefaultRefillPerSec).
		Scan(&tokens, &maxTokens, &refill)
	if err != nil {
		return fmt.Errorf("backpressure consume: %w", err)
	}
	// If tokens ended at 0 and the UPDATE kept it at 0 (couldn't
	// decrement), the bucket was empty.
	if tokens <= 0 {
		retryAfter := time.Second
		if refill > 0 {
			retryAfter = time.Duration(float64(time.Second) / float64(refill))
		}
		return &ThrottledError{ProjectID: projectID, RetryAfter: retryAfter}
	}
	return nil
}
