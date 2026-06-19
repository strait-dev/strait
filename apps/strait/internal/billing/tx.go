package billing

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"strait/internal/domain"
)

type billingTxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

type billingTxExec interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// WithBillingTx executes fn within a database transaction. If fn returns an
// error, the transaction is rolled back. Otherwise it is committed.
func WithBillingTx(ctx context.Context, pool *pgxpool.Pool, fn func(tx pgx.Tx) error) error {
	return withBillingTx(ctx, pool, fn)
}

func withBillingTx(ctx context.Context, beginner billingTxBeginner, fn func(tx pgx.Tx) error) error {
	tx, err := beginner.Begin(ctx)
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
		return restrictOrgInTx(ctx, tx, orgID, graceEnd)
	})
}

func restrictOrgInTx(ctx context.Context, q billingTxExec, orgID string, graceEnd *time.Time) error {
	tag, err := q.Exec(ctx, `
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

	tag, err = q.Exec(ctx, `
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

	// Collapse the entitlements snapshot to Free-tier in the same
	// transaction so a reader landing between the plan downgrade and
	// the next mutator never sees paid limits on a restricted org.
	freeEntitlements, err := json.Marshal(GetPlanLimits(domain.PlanFree))
	if err != nil {
		return fmt.Errorf("marshalling free entitlements: %w", err)
	}
	if _, err := q.Exec(ctx, `
		UPDATE organization_subscriptions
		SET entitlements = $2::jsonb, updated_at = NOW()
		WHERE org_id = $1
	`, orgID, freeEntitlements); err != nil {
		return fmt.Errorf("resetting entitlements to free: %w", err)
	}

	return nil
}
