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
	// see an atomic view of the bucket. The CTE computes the refilled
	// balance and returns both the new token count and a boolean
	// indicating whether the consume succeeded. A success with the
	// bucket dropping to zero is still a success.
	const sql = `
		WITH refilled AS (
			SELECT
				$1::text AS project_id,
				LEAST(
					COALESCE(rl.max_tokens, $2),
					COALESCE(rl.tokens, $2) +
						GREATEST(0, FLOOR(EXTRACT(EPOCH FROM (NOW() - COALESCE(rl.last_refill_at, NOW()))) * COALESCE(rl.refill_per_sec, $3))::int)
				) AS available,
				COALESCE(rl.max_tokens, $2) AS max_tokens,
				COALESCE(rl.refill_per_sec, $3) AS refill_per_sec
			FROM (SELECT 1) AS dummy
			LEFT JOIN project_rate_limits rl ON rl.project_id = $1
		),
		upsert AS (
			INSERT INTO project_rate_limits (project_id, tokens, max_tokens, refill_per_sec, last_refill_at, updated_at)
			SELECT project_id,
				CASE WHEN available > 0 THEN available - 1 ELSE 0 END,
				max_tokens, refill_per_sec, NOW(), NOW()
			FROM refilled
			ON CONFLICT (project_id) DO UPDATE SET
				tokens = EXCLUDED.tokens,
				last_refill_at = NOW(),
				updated_at = NOW()
			RETURNING tokens, max_tokens, refill_per_sec
		)
		SELECT u.tokens, u.max_tokens, u.refill_per_sec, r.available
		FROM upsert u, refilled r`

	var tokens, maxTokens, refill, available int
	err := b.db.QueryRow(ctx, sql, projectID, b.cfg.DefaultMaxTokens, b.cfg.DefaultRefillPerSec).
		Scan(&tokens, &maxTokens, &refill, &available)
	if err != nil {
		return fmt.Errorf("backpressure consume: %w", err)
	}
	// available is the pre-decrement balance. If it was zero the bucket
	// was empty and this consume did not succeed.
	if available <= 0 {
		retryAfter := time.Second
		if refill > 0 {
			retryAfter = time.Duration(float64(time.Second) / float64(refill))
		}
		return &ThrottledError{ProjectID: projectID, RetryAfter: retryAfter}
	}
	return nil
}

// TryConsumeN reserves n tokens from the project's bucket. Used by
// EnqueueBatch so the batch is rejected atomically when the bucket
// cannot satisfy the full request.
func (b *Backpressure) TryConsumeN(ctx context.Context, projectID string, n int) error {
	if b == nil || !b.enabled || projectID == "" || n <= 0 {
		return nil
	}
	ctx, span := otel.Tracer("strait").Start(ctx, "queue.Backpressure.TryConsumeN")
	defer span.End()

	const sql = `
		WITH refilled AS (
			SELECT
				$1::text AS project_id,
				LEAST(
					COALESCE(rl.max_tokens, $2),
					COALESCE(rl.tokens, $2) +
						GREATEST(0, FLOOR(EXTRACT(EPOCH FROM (NOW() - COALESCE(rl.last_refill_at, NOW()))) * COALESCE(rl.refill_per_sec, $3))::int)
				) AS available,
				COALESCE(rl.max_tokens, $2) AS max_tokens,
				COALESCE(rl.refill_per_sec, $3) AS refill_per_sec
			FROM (SELECT 1) AS dummy
			LEFT JOIN project_rate_limits rl ON rl.project_id = $1
		),
		upsert AS (
			INSERT INTO project_rate_limits (project_id, tokens, max_tokens, refill_per_sec, last_refill_at, updated_at)
			SELECT project_id,
				CASE WHEN available >= $4 THEN available - $4 ELSE 0 END,
				max_tokens, refill_per_sec, NOW(), NOW()
			FROM refilled
			ON CONFLICT (project_id) DO UPDATE SET
				tokens = EXCLUDED.tokens,
				last_refill_at = NOW(),
				updated_at = NOW()
			RETURNING tokens, max_tokens, refill_per_sec
		)
		SELECT u.tokens, u.max_tokens, u.refill_per_sec, r.available
		FROM upsert u, refilled r`

	var tokens, maxTokens, refill, available int
	err := b.db.QueryRow(ctx, sql, projectID, b.cfg.DefaultMaxTokens, b.cfg.DefaultRefillPerSec, n).
		Scan(&tokens, &maxTokens, &refill, &available)
	if err != nil {
		return fmt.Errorf("backpressure consume N: %w", err)
	}
	if available < n {
		retryAfter := time.Second
		if refill > 0 {
			retryAfter = time.Duration(float64(time.Second) * float64(n-available) / float64(refill))
		}
		return &ThrottledError{ProjectID: projectID, RetryAfter: retryAfter}
	}
	return nil
}

// TokenSample is a point-in-time read of a project's bucket.
type TokenSample struct {
	ProjectID string
	Tokens    int64
}

// SampleAvailableTokens reads up to sampleN project buckets ordered by
// most-recently-updated. Used by a scheduler sampler to populate the
// backpressure_tokens_available gauge. Read-only and index-friendly.
func (b *Backpressure) SampleAvailableTokens(ctx context.Context, sampleN int) ([]TokenSample, error) {
	if b == nil || !b.enabled || sampleN <= 0 {
		return nil, nil
	}
	rows, err := b.db.Query(ctx, `
		SELECT project_id, tokens
		FROM project_rate_limits
		ORDER BY updated_at DESC NULLS LAST
		LIMIT $1
	`, sampleN)
	if err != nil {
		return nil, fmt.Errorf("sample backpressure tokens: %w", err)
	}
	defer rows.Close()
	out := make([]TokenSample, 0, sampleN)
	for rows.Next() {
		var s TokenSample
		if err := rows.Scan(&s.ProjectID, &s.Tokens); err != nil {
			return nil, fmt.Errorf("scan sample: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
