package billing

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// StartDunning enters the org into dunning at step 1 if it is not already in
// an active cycle. Idempotent on replays: a second invoice.payment_failed
// during an active cycle is a no-op so the original dunning_entered_at is
// preserved.
func (s *PgStore) StartDunning(ctx context.Context, orgID string, now time.Time) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE organization_subscriptions
		SET dunning_step = 1,
		    dunning_entered_at = $2,
		    dunning_resolved_at = NULL,
		    dunning_last_tick_at = NULL,
		    updated_at = NOW()
		WHERE org_id = $1
		  AND (dunning_entered_at IS NULL OR dunning_resolved_at IS NOT NULL)
	`, orgID, now.UTC())
	if err != nil {
		return fmt.Errorf("start dunning: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// Already in an active cycle, or org row missing. Both are safe.
		// Verify org_id exists so a typo doesn't silently no-op.
		var exists bool
		if err := s.pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM organization_subscriptions WHERE org_id = $1)`,
			orgID).Scan(&exists); err != nil {
			return fmt.Errorf("check org subscription existence: %w", err)
		}
		if !exists {
			return ErrSubscriptionNotFound
		}
	}
	return nil
}

// ResolveDunning clears the active dunning cycle. Sets dunning_resolved_at to
// NOW(), and resets dunning_step / dunning_entered_at / dunning_last_tick_at
// so the next failed payment starts a fresh cycle at step 1.
func (s *PgStore) ResolveDunning(ctx context.Context, orgID string, now time.Time) error {
	if _, err := s.pool.Exec(ctx, `
		UPDATE organization_subscriptions
		SET dunning_step = 0,
		    dunning_entered_at = NULL,
		    dunning_last_tick_at = NULL,
		    dunning_resolved_at = $2,
		    updated_at = NOW()
		WHERE org_id = $1
		  AND dunning_entered_at IS NOT NULL
		  AND dunning_resolved_at IS NULL
	`, orgID, now.UTC()); err != nil {
		return fmt.Errorf("resolve dunning: %w", err)
	}
	return nil
}

// ProcessDueDunningRows iterates up to `limit` active dunning rows whose
// dunning_last_tick_at is NULL or older than `now - cooldown`, taking a
// FOR UPDATE SKIP LOCKED row lock for each. Each row is decided and
// committed in its own transaction so a poison row cannot block the batch
// and so concurrent Dunner replicas claim disjoint work.
func (s *PgStore) ProcessDueDunningRows(
	ctx context.Context,
	now time.Time,
	cooldown time.Duration,
	limit int,
	fn func(ctx context.Context, row DunningRow) (DunningTransition, error),
) (int, error) {
	if fn == nil {
		return 0, errors.New("dunning: nil decision fn")
	}
	if limit <= 0 {
		limit = 256
	}

	// Read claim candidates first; per-row work happens in its own tx so we
	// never hold a long-lived snapshot across decide() calls.
	cutoff := now.Add(-cooldown).UTC()
	rows, err := s.pool.Query(ctx, `
		SELECT org_id
		FROM organization_subscriptions
		WHERE dunning_entered_at IS NOT NULL
		  AND dunning_resolved_at IS NULL
		  AND (dunning_last_tick_at IS NULL OR dunning_last_tick_at <= $1)
		ORDER BY dunning_last_tick_at NULLS FIRST, dunning_entered_at
		LIMIT $2
	`, cutoff, limit)
	if err != nil {
		return 0, fmt.Errorf("list due dunning rows: %w", err)
	}
	orgIDs := make([]string, 0, limit)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan dunning org_id: %w", err)
		}
		orgIDs = append(orgIDs, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("dunning rows iter: %w", err)
	}

	processed := 0
	for _, orgID := range orgIDs {
		ok, err := s.processOneDunningRow(ctx, orgID, cutoff, fn)
		if err != nil {
			return processed, err
		}
		if ok {
			processed++
		}
	}
	return processed, nil
}

func (s *PgStore) processOneDunningRow(
	ctx context.Context,
	orgID string,
	cutoff time.Time,
	fn func(ctx context.Context, row DunningRow) (DunningTransition, error),
) (bool, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, fmt.Errorf("begin dunning tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var row DunningRow
	err = tx.QueryRow(ctx, `
		SELECT org_id, plan_tier, payment_status,
		       dunning_step, dunning_entered_at
		FROM organization_subscriptions
		WHERE org_id = $1
		  AND dunning_entered_at IS NOT NULL
		  AND dunning_resolved_at IS NULL
		  AND (dunning_last_tick_at IS NULL OR dunning_last_tick_at <= $2)
		FOR UPDATE SKIP LOCKED
	`, orgID, cutoff).Scan(&row.OrgID, &row.PlanTier, &row.PaymentStatus, &row.DunningStep, &row.DunningEnteredAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Row was resolved, suspended, or claimed by a peer between the
			// candidate scan and the FOR UPDATE — skip silently.
			return false, nil
		}
		return false, fmt.Errorf("lock dunning row: %w", err)
	}

	transition, err := fn(ctx, row)
	if err != nil {
		return false, fmt.Errorf("decide dunning transition for %s: %w", orgID, err)
	}

	if transition.PaymentStatus != "" {
		if _, err := tx.Exec(ctx, `
			UPDATE organization_subscriptions
			SET dunning_step = $2,
			    dunning_last_tick_at = $3,
			    payment_status = $4,
			    updated_at = NOW()
			WHERE org_id = $1
		`, orgID, transition.NewStep, transition.TickAt.UTC(), transition.PaymentStatus); err != nil {
			return false, fmt.Errorf("apply dunning transition: %w", err)
		}
	} else {
		if _, err := tx.Exec(ctx, `
			UPDATE organization_subscriptions
			SET dunning_step = $2,
			    dunning_last_tick_at = $3,
			    updated_at = NOW()
			WHERE org_id = $1
		`, orgID, transition.NewStep, transition.TickAt.UTC()); err != nil {
			return false, fmt.Errorf("apply dunning transition: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit dunning tx: %w", err)
	}
	return true, nil
}
