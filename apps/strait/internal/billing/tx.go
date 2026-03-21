package billing

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// WithBillingTx executes fn within a database transaction. If fn returns an
// error, the transaction is rolled back. Otherwise it is committed.
func WithBillingTx(ctx context.Context, pool *pgxpool.Pool, fn func(tx pgx.Tx) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin billing tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			if rbErr := tx.Rollback(ctx); rbErr != nil {
				slog.Warn("failed to rollback billing tx", "error", rbErr)
			}
		}
	}()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit billing tx: %w", err)
	}
	committed = true
	return nil
}

// RestrictOrgTx atomically sets payment_status to "restricted" and downgrades
// plan_tier to "free" within a single transaction. This prevents the inconsistent
// state where payment is restricted but plan is still on a paid tier.
func RestrictOrgTx(ctx context.Context, pool *pgxpool.Pool, orgID string, graceEnd *time.Time) error {
	return WithBillingTx(ctx, pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE organization_subscriptions
			SET payment_status = 'restricted', grace_period_end = $2, updated_at = NOW()
			WHERE org_id = $1
		`, orgID, graceEnd)
		if err != nil {
			return fmt.Errorf("restricting org payment status: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return ErrSubscriptionNotFound
		}

		tag, err = tx.Exec(ctx, `
			UPDATE organization_subscriptions
			SET plan_tier = 'free', status = 'restricted', updated_at = NOW()
			WHERE org_id = $1
		`, orgID)
		if err != nil {
			return fmt.Errorf("downgrading org to free: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return ErrSubscriptionNotFound
		}

		return nil
	})
}
