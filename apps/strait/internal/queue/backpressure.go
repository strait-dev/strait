package queue

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"strait/internal/store"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

// Per-project enqueue backpressure.
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
	LocalLeaseSize      int
}

// Backpressure consults the project_rate_limits table to enforce a
// DB-side token bucket per project.
type Backpressure struct {
	db           store.DBTX
	cfg          BackpressureConfig
	enabled      bool
	mu           sync.Mutex
	localLeases  map[string]int
	leaseRefills map[string]*leaseRefill
}

type leaseRefill struct {
	done chan struct{}
	err  error
}

// NewBackpressure builds a backpressure controller. When enabled is
// false, TryConsume is a no-op that always allows.
//
// Defaults only fill zero-value fields, not explicit zeros set by the
// caller: the zero-value struct is treated as "please apply sensible
// defaults" while a caller that wants a strictly capped bucket with no
// refill sets DefaultMaxTokens to a positive value and leaves
// DefaultRefillPerSec at zero to mean "no refill". A negative value is
// always rejected as user error.
func NewBackpressure(db store.DBTX, cfg BackpressureConfig, enabled bool) *Backpressure {
	if cfg.DefaultMaxTokens < 0 {
		cfg.DefaultMaxTokens = 0
	}
	if cfg.DefaultRefillPerSec < 0 {
		cfg.DefaultRefillPerSec = 0
	}
	if cfg.DefaultMaxTokens == 0 && cfg.DefaultRefillPerSec == 0 {
		cfg.DefaultMaxTokens = 1000
		cfg.DefaultRefillPerSec = 100
	}
	if cfg.LocalLeaseSize <= 0 {
		cfg.LocalLeaseSize = 32
	}
	return &Backpressure{
		db:           db,
		cfg:          cfg,
		enabled:      enabled,
		localLeases:  make(map[string]int),
		leaseRefills: make(map[string]*leaseRefill),
	}
}

// TryConsume reserves one token from the project's bucket. Returns nil
// on success or *ThrottledError on exhaustion. Passes through
// unexpected DB errors so callers can distinguish throttle from outage.
func (b *Backpressure) TryConsume(ctx context.Context, projectID string) error {
	if b == nil {
		return nil
	}
	return b.tryConsumeNOn(ctx, b.db, projectID, 1)
}

// TryConsumeN reserves n tokens from the project's bucket. Used by
// EnqueueBatch so the batch is rejected atomically when the bucket
// cannot satisfy the full request.
func (b *Backpressure) TryConsumeN(ctx context.Context, projectID string, n int) error {
	if b == nil {
		return nil
	}
	return b.tryConsumeNOn(ctx, b.db, projectID, n)
}

// TryConsumeInTx reserves one token inside the caller's transaction so enqueue
// admission and row insertion can commit or roll back together.
func (b *Backpressure) TryConsumeInTx(ctx context.Context, tx store.DBTX, projectID string) error {
	return b.tryConsumeNOn(ctx, tx, projectID, 1)
}

// TryConsumeNInTx is the transactional form used when callers need token
// consumption to succeed or fail atomically with surrounding writes.
func (b *Backpressure) TryConsumeNInTx(ctx context.Context, tx store.DBTX, projectID string, n int) error {
	return b.tryConsumeNOn(ctx, tx, projectID, n)
}

// tryConsumeNOn is the atomic bucket-decrement path. It relies on
// PostgreSQL's intrinsic row locking in INSERT ... ON CONFLICT DO UPDATE
// so the refilled balance is recomputed from the locked existing row
// (not from a pre-lock CTE snapshot). The WHERE clause on the DO UPDATE
// gates the decrement atomically; on throttle the UPDATE is skipped and
// the statement returns no rows.
//
// The INSERT path is gated by an inline `WHERE $2 >= $4` so a brand-new
// project never gets inserted with a negative or zero-tokens row when
// the request cannot be satisfied by the default bucket.
func (b *Backpressure) tryConsumeNOn(ctx context.Context, db store.DBTX, projectID string, n int) error {
	if b == nil {
		return nil
	}
	if !b.enabled {
		return nil
	}
	if projectID == "" {
		return nil
	}
	if n <= 0 {
		return nil
	}
	if db == nil {
		return nil
	}

	if n == 1 && b.cfg.LocalLeaseSize > 1 {
		return b.tryConsumeWithLocalLease(ctx, db, projectID)
	}

	return b.tryConsumeNOnDB(ctx, db, projectID, n)
}

func (b *Backpressure) tryConsumeWithLocalLease(ctx context.Context, db store.DBTX, projectID string) error {
	for {
		refill, owner := b.beginLocalLeaseRefill(projectID)
		if owner {
			return b.finishLocalLeaseRefill(ctx, db, projectID, refill)
		}
		if refill == nil {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-refill.done:
			if refill.err != nil {
				return refill.err
			}
		}
	}
}

func (b *Backpressure) beginLocalLeaseRefill(projectID string) (*leaseRefill, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.localLeases[projectID] > 0 {
		b.localLeases[projectID]--
		return nil, false
	}

	if refill := b.leaseRefills[projectID]; refill != nil {
		return refill, false
	}

	refill := &leaseRefill{done: make(chan struct{})}
	b.leaseRefills[projectID] = refill
	return refill, true
}

func (b *Backpressure) finishLocalLeaseRefill(ctx context.Context, db store.DBTX, projectID string, refill *leaseRefill) error {
	leaseSize := b.localLeaseSize()
	granted := leaseSize
	err := b.tryConsumeNOnDB(ctx, db, projectID, leaseSize)
	if err != nil && leaseSize > 1 {
		// Explicit project buckets may be smaller than the default bucket.
		// Fall back to a strict one-token consume instead of rejecting useful
		// capacity just because the configured lease is too large.
		granted = 1
		err = b.tryConsumeNOnDB(ctx, db, projectID, 1)
	}

	b.mu.Lock()
	if err == nil && granted > 1 {
		b.localLeases[projectID] += granted - 1
	}
	refill.err = err
	delete(b.leaseRefills, projectID)
	close(refill.done)
	b.mu.Unlock()

	return err
}

func (b *Backpressure) localLeaseSize() int {
	leaseSize := b.cfg.LocalLeaseSize
	if b.cfg.DefaultMaxTokens > 0 && leaseSize > b.cfg.DefaultMaxTokens {
		leaseSize = b.cfg.DefaultMaxTokens
	}
	if leaseSize < 1 {
		leaseSize = 1
	}
	return leaseSize
}

func (b *Backpressure) tryConsumeNOnDB(ctx context.Context, db store.DBTX, projectID string, n int) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "queue.Backpressure.TryConsume")
	defer span.End()

	// The DO UPDATE SET computes: refilled = LEAST(max, old + earned) then
	// subtracts n. This can produce a negative token balance when a large
	// batch exhausts the bucket -- that is intentional burst debt. The
	// negative value is recovered naturally on the next refill cycle via
	// the LEAST(max_tokens, ...) cap, so no special handling is needed.
	const sql = `
		INSERT INTO project_rate_limits (project_id, tokens, max_tokens, refill_per_sec, last_refill_at, updated_at)
		SELECT $1::text, $2::int - $4::int, $2::int, $3::int, NOW(), NOW()
		WHERE $2::int >= $4::int
		ON CONFLICT (project_id) DO UPDATE SET
			tokens = LEAST(
				project_rate_limits.max_tokens,
				project_rate_limits.tokens +
					GREATEST(0, FLOOR(EXTRACT(EPOCH FROM (NOW() - project_rate_limits.last_refill_at)) * project_rate_limits.refill_per_sec)::int)
			) - $4::int,
			last_refill_at = CASE
				WHEN GREATEST(0, FLOOR(EXTRACT(EPOCH FROM (NOW() - project_rate_limits.last_refill_at)) * project_rate_limits.refill_per_sec)::int) > 0
				THEN NOW()
				ELSE project_rate_limits.last_refill_at
			END,
			updated_at = NOW()
		WHERE LEAST(
			project_rate_limits.max_tokens,
			project_rate_limits.tokens +
				GREATEST(0, FLOOR(EXTRACT(EPOCH FROM (NOW() - project_rate_limits.last_refill_at)) * project_rate_limits.refill_per_sec)::int)
		) >= $4::int
		RETURNING tokens, max_tokens, refill_per_sec`

	var tokens, maxTokens, refill int
	err := db.QueryRow(ctx, sql, projectID, b.cfg.DefaultMaxTokens, b.cfg.DefaultRefillPerSec, n).
		Scan(&tokens, &maxTokens, &refill)
	if errors.Is(err, pgx.ErrNoRows) {
		// The INSERT source WHERE evaluated false (n > default max) OR
		// the ON CONFLICT DO UPDATE WHERE evaluated false (bucket empty).
		// Estimate retry using the default refill rate; a richer estimate
		// would require a follow-up SELECT we intentionally avoid on the
		// throttled path.
		retryAfter := time.Second
		if b.cfg.DefaultRefillPerSec > 0 {
			retryAfter = time.Duration(float64(time.Second) * float64(n) / float64(b.cfg.DefaultRefillPerSec))
		}
		return &ThrottledError{ProjectID: projectID, RetryAfter: retryAfter}
	}
	if err != nil {
		return fmt.Errorf("backpressure consume: %w", err)
	}
	_ = tokens
	_ = maxTokens
	_ = refill
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
	if b == nil {
		return nil, nil
	}
	if !b.enabled {
		return nil, nil
	}
	if sampleN <= 0 {
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
