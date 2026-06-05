package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// unmarshalJSON is a thin wrapper so callers don't need to import encoding/json.
func unmarshalJSON(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

var ErrSubscriptionNotFound = errors.New("organization subscription not found")

// querier abstracts the subset of pgxpool.Pool and pgx.Tx methods used by
// billing reads/writes so a single helper can serve both transactional and
// non-transactional callers. Mutators run inside WithBillingTx so the
// primary UPDATE and the entitlements refresh land in one transaction;
// pure readers pass the pool directly.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// PgStore implements Store with PostgreSQL via pgx.
type PgStore struct {
	pool *pgxpool.Pool
}

// NewPgStore creates a new PostgreSQL billing store.
func NewPgStore(pool *pgxpool.Pool) *PgStore {
	return &PgStore{pool: pool}
}

// EnsureOrgSubscription creates a free-tier subscription row for an org if one
// does not already exist. Used for lazy initialization when a project is first
// created under an org that has no Stripe subscription yet.
func (s *PgStore) EnsureOrgSubscription(ctx context.Context, orgID string) error {
	return WithBillingTx(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`INSERT INTO organization_subscriptions (id, org_id, plan_tier, status, overage_disabled)
			 VALUES (gen_random_uuid()::text, $1, 'free', 'active', true)
			 ON CONFLICT (org_id) DO NOTHING`,
			orgID); err != nil {
			return fmt.Errorf("ensure org subscription: %w", err)
		}
		return s.refreshEntitlements(ctx, tx, orgID)
	})
}

func (s *PgStore) GetOrgSubscription(ctx context.Context, orgID string) (*OrgSubscription, error) {
	return s.getOrgSubscriptionWhere(ctx, s.pool, "org_id = $1", orgID)
}

func (s *PgStore) GetOrgSubscriptionByStripeSubscriptionID(ctx context.Context, stripeSubscriptionID string) (*OrgSubscription, error) {
	if stripeSubscriptionID == "" {
		return nil, ErrSubscriptionNotFound
	}
	return s.getOrgSubscriptionWhere(ctx, s.pool, "stripe_subscription_id = $1", stripeSubscriptionID)
}

func (s *PgStore) GetOrgSubscriptionByStripeCustomerID(ctx context.Context, stripeCustomerID string) (*OrgSubscription, error) {
	if stripeCustomerID == "" {
		return nil, ErrSubscriptionNotFound
	}
	return s.getOrgSubscriptionWhere(ctx, s.pool, "stripe_customer_id = $1", stripeCustomerID)
}

func (s *PgStore) getOrgSubscriptionWhere(ctx context.Context, q querier, where string, arg string) (*OrgSubscription, error) {
	var sub OrgSubscription
	var addOnsJSON []byte
	query := `
		SELECT id, org_id, plan_tier, stripe_subscription_id, stripe_customer_id,
			stripe_lookup_key,
			status, current_period_start, current_period_end,
			spending_limit_microusd, limit_action,
			COALESCE(overage_disabled, plan_tier = 'free'),
			pending_plan_tier, canceled_at,
			COALESCE(anomaly_threshold_warning, 3.0),
			COALESCE(anomaly_threshold_critical, 10.0),
			grace_period_end, COALESCE(payment_status, 'ok'),
			override_daily_run_limit, override_concurrent_run_limit,
			COALESCE(enforcement_mode, 'enforce'),
			COALESCE(monthly_usage_email, false),
			COALESCE(add_ons, '{}'::jsonb),
			COALESCE(entitlements, '{}'::jsonb),
			created_at, updated_at, cache_version
		FROM organization_subscriptions
		WHERE ` + where
	err := q.QueryRow(ctx, query, arg).Scan(
		&sub.ID, &sub.OrgID, &sub.PlanTier,
		&sub.StripeSubscriptionID, &sub.StripeCustomerID,
		&sub.StripeLookupKey,
		&sub.Status, &sub.CurrentPeriodStart, &sub.CurrentPeriodEnd,
		&sub.SpendingLimitMicrousd, &sub.LimitAction,
		&sub.OverageDisabled,
		&sub.PendingPlanTier, &sub.CanceledAt,
		&sub.AnomalyThresholdWarning, &sub.AnomalyThresholdCritical,
		&sub.GracePeriodEnd, &sub.PaymentStatus,
		&sub.OverrideDailyRunLimit, &sub.OverrideConcurrentRunLimit,
		&sub.EnforcementMode,
		&sub.MonthlyUsageEmail,
		&addOnsJSON,
		&sub.Entitlements,
		&sub.CreatedAt, &sub.UpdatedAt, &sub.CacheVersion,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSubscriptionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting org subscription: %w", err)
	}
	if len(addOnsJSON) > 0 {
		if jsonErr := unmarshalJSON(addOnsJSON, &sub.AddOns); jsonErr != nil {
			return nil, fmt.Errorf("unmarshalling add_ons for org %s: %w", sub.OrgID, jsonErr)
		}
	}
	return &sub, nil
}

func (s *PgStore) UpsertOrgSubscription(ctx context.Context, sub *OrgSubscription) error {
	return WithBillingTx(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO organization_subscriptions (
				id, org_id, plan_tier, stripe_subscription_id, stripe_customer_id,
				stripe_lookup_key,
				status, current_period_start, current_period_end,
				spending_limit_microusd, limit_action, overage_disabled, canceled_at,
				created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
			ON CONFLICT (org_id) DO UPDATE SET
				plan_tier = EXCLUDED.plan_tier,
				stripe_subscription_id = EXCLUDED.stripe_subscription_id,
				stripe_customer_id = EXCLUDED.stripe_customer_id,
				stripe_lookup_key = COALESCE(EXCLUDED.stripe_lookup_key, organization_subscriptions.stripe_lookup_key),
				status = EXCLUDED.status,
				current_period_start = EXCLUDED.current_period_start,
				current_period_end = EXCLUDED.current_period_end,
				spending_limit_microusd = organization_subscriptions.spending_limit_microusd,
				limit_action = organization_subscriptions.limit_action,
				overage_disabled = organization_subscriptions.overage_disabled,
				pending_plan_tier = COALESCE(organization_subscriptions.pending_plan_tier, NULL),
				canceled_at = EXCLUDED.canceled_at,
				cap_warning_dispatched_at = CASE
					WHEN organization_subscriptions.current_period_start IS DISTINCT FROM EXCLUDED.current_period_start THEN NULL
					ELSE organization_subscriptions.cap_warning_dispatched_at
				END,
				cap_reached_dispatched_at = CASE
					WHEN organization_subscriptions.current_period_start IS DISTINCT FROM EXCLUDED.current_period_start THEN NULL
					ELSE organization_subscriptions.cap_reached_dispatched_at
				END,
				cap_disabled_dispatched_at = CASE
					WHEN organization_subscriptions.current_period_start IS DISTINCT FROM EXCLUDED.current_period_start THEN NULL
					ELSE organization_subscriptions.cap_disabled_dispatched_at
				END,
				overage_disabled_dispatched_at = CASE
					WHEN organization_subscriptions.current_period_start IS DISTINCT FROM EXCLUDED.current_period_start THEN NULL
					ELSE organization_subscriptions.overage_disabled_dispatched_at
				END,
				updated_at = NOW()
		`, sub.ID, sub.OrgID, sub.PlanTier,
			sub.StripeSubscriptionID, sub.StripeCustomerID,
			sub.StripeLookupKey,
			sub.Status, sub.CurrentPeriodStart, sub.CurrentPeriodEnd,
			sub.SpendingLimitMicrousd, sub.LimitAction, sub.PlanTier == string(domain.PlanFree), sub.CanceledAt,
			sub.CreatedAt, sub.UpdatedAt,
		); err != nil {
			return fmt.Errorf("upserting org subscription: %w", err)
		}
		return s.refreshEntitlements(ctx, tx, sub.OrgID)
	})
}

func (s *PgStore) UpdateOrgSubscriptionPlan(ctx context.Context, orgID, planTier, status string) error {
	return WithBillingTx(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE organization_subscriptions
			SET plan_tier = $2, status = $3, updated_at = NOW()
			WHERE org_id = $1
		`, orgID, planTier, status)
		if err != nil {
			return fmt.Errorf("updating org subscription plan: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return ErrSubscriptionNotFound
		}
		return s.refreshEntitlements(ctx, tx, orgID)
	})
}

// refreshEntitlements recomputes the entitlements snapshot from the current
// subscription row plus active addons and writes it back. Mutators call this
// inside their WithBillingTx so the primary UPDATE and the snapshot refresh
// land in one transaction — concurrent mutators can't write entitlements in
// inverse order against the same orgID.
//
// Unknown orgs (no subscription row yet) return nil silently. Listing addons
// or writing entitlements failing surfaces an error so the caller's tx rolls
// back together with its primary write.
func (s *PgStore) refreshEntitlements(ctx context.Context, q querier, orgID string) error {
	sub, err := s.getOrgSubscriptionWhere(ctx, q, "org_id = $1", orgID)
	if errors.Is(err, ErrSubscriptionNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("refresh entitlements load sub for org %s: %w", orgID, err)
	}
	addons, err := s.listActiveAddons(ctx, q, orgID)
	if err != nil {
		return fmt.Errorf("refresh entitlements load addons for org %s: %w", orgID, err)
	}
	entitlements := ComputeEntitlements(sub, addons)
	return s.updateEntitlements(ctx, q, orgID, entitlements)
}

// UpdateEntitlements writes the resolved entitlements snapshot to the
// organization_subscriptions row so subsequent quota reads can hit a single
// JSONB column instead of recomputing through the catalog/addons pipeline.
//
// Callers are expected to have produced `entitlements` via
// ComputeEntitlements; this method does not validate composition. A no-op
// for unknown orgs (zero rows affected, no error) — webhook idempotency
// retries land here for orgs that never persisted, and surfacing an error
// would defeat the retry.
func (s *PgStore) UpdateEntitlements(ctx context.Context, orgID string, entitlements OrgPlanLimits) error {
	return s.updateEntitlements(ctx, s.pool, orgID, entitlements)
}

func (s *PgStore) updateEntitlements(ctx context.Context, q querier, orgID string, entitlements OrgPlanLimits) error {
	payload, err := json.Marshal(entitlements)
	if err != nil {
		return fmt.Errorf("marshalling entitlements for org %s: %w", orgID, err)
	}
	if _, err = q.Exec(ctx, `
		UPDATE organization_subscriptions
		SET entitlements = $2::jsonb, updated_at = NOW()
		WHERE org_id = $1
	`, orgID, payload); err != nil {
		return fmt.Errorf("updating entitlements for org %s: %w", orgID, err)
	}
	return nil
}

func (s *PgStore) UpdateOrgSubscriptionStatus(ctx context.Context, orgID, status string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE organization_subscriptions
		SET status = $2, updated_at = NOW()
		WHERE org_id = $1
	`, orgID, status)
	if err != nil {
		return fmt.Errorf("updating org subscription status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrSubscriptionNotFound
	}
	return nil
}

func (s *PgStore) UpdateOrgSubscriptionFull(ctx context.Context, orgID, planTier, status string, periodStart, periodEnd *time.Time) error {
	return WithBillingTx(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE organization_subscriptions
			SET plan_tier = $2, status = $3,
				current_period_start = COALESCE($4, current_period_start),
				current_period_end = COALESCE($5, current_period_end),
				payment_status = CASE WHEN $3 = 'active' THEN 'ok' ELSE payment_status END,
				grace_period_end = CASE WHEN $3 = 'active' THEN NULL ELSE grace_period_end END,
				updated_at = NOW()
			WHERE org_id = $1
		`, orgID, planTier, status, periodStart, periodEnd)
		if err != nil {
			return fmt.Errorf("updating org subscription full: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return ErrSubscriptionNotFound
		}
		return s.refreshEntitlements(ctx, tx, orgID)
	})
}

func (s *PgStore) SetPendingPlanTier(ctx context.Context, orgID, tier string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE organization_subscriptions
		SET pending_plan_tier = $2, updated_at = NOW()
		WHERE org_id = $1
	`, orgID, tier)
	if err != nil {
		return fmt.Errorf("setting pending plan tier: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrSubscriptionNotFound
	}
	return nil
}

// SetPendingDowngrade atomically sets pending_plan_tier and updates period dates
// in a single UPDATE. This replaces the two-step SetPendingPlanTier + UpdateOrgSubscriptionFull
// pattern in the webhook handler to prevent partial state on failure.
func (s *PgStore) SetPendingDowngrade(ctx context.Context, orgID, pendingTier string, periodStart, periodEnd *time.Time) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE organization_subscriptions
		SET pending_plan_tier = $2,
		    current_period_start = $3,
		    current_period_end = $4,
		    updated_at = NOW()
		WHERE org_id = $1
	`, orgID, pendingTier, periodStart, periodEnd)
	if err != nil {
		return fmt.Errorf("setting pending downgrade: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrSubscriptionNotFound
	}
	return nil
}

// Pool returns the underlying connection pool for transactional operations.
func (s *PgStore) Pool() *pgxpool.Pool { return s.pool }

func (s *PgStore) ClearPendingPlanTier(ctx context.Context, orgID string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE organization_subscriptions
		SET pending_plan_tier = NULL, updated_at = NOW()
		WHERE org_id = $1
	`, orgID)
	if err != nil {
		return fmt.Errorf("clearing pending plan tier: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrSubscriptionNotFound
	}
	return nil
}

func (s *PgStore) ApplyPendingDowngrade(ctx context.Context, orgID string) error {
	return WithBillingTx(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE organization_subscriptions
			SET plan_tier = pending_plan_tier,
				pending_plan_tier = NULL,
				updated_at = NOW()
			WHERE org_id = $1 AND pending_plan_tier IS NOT NULL
		`, orgID)
		if err != nil {
			return fmt.Errorf("applying pending downgrade: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return ErrSubscriptionNotFound
		}
		return s.refreshEntitlements(ctx, tx, orgID)
	})
}

func (s *PgStore) ApplyPendingDowngradeIfTier(ctx context.Context, orgID, pendingTier string) (bool, error) {
	var applied bool
	if err := WithBillingTx(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE organization_subscriptions
			SET plan_tier = pending_plan_tier,
				pending_plan_tier = NULL,
				updated_at = NOW()
			WHERE org_id = $1
			  AND pending_plan_tier = $2
		`, orgID, pendingTier)
		if err != nil {
			return fmt.Errorf("applying pending downgrade conditionally: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return nil
		}
		applied = true
		return s.refreshEntitlements(ctx, tx, orgID)
	}); err != nil {
		return false, err
	}
	return applied, nil
}

func (s *PgStore) ApplyPendingDowngradeTierIfPending(ctx context.Context, orgID, pendingTier string) (bool, error) {
	var applied bool
	if err := WithBillingTx(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE organization_subscriptions
			SET plan_tier = pending_plan_tier,
				updated_at = NOW()
			WHERE org_id = $1
			  AND pending_plan_tier = $2
		`, orgID, pendingTier)
		if err != nil {
			return fmt.Errorf("applying pending downgrade tier: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return nil
		}
		applied = true
		return s.refreshEntitlements(ctx, tx, orgID)
	}); err != nil {
		return false, err
	}
	return applied, nil
}

func (s *PgStore) ClearPendingPlanTierIfTier(ctx context.Context, orgID, pendingTier string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE organization_subscriptions
		SET pending_plan_tier = NULL,
			updated_at = NOW()
		WHERE org_id = $1
		  AND pending_plan_tier = $2
	`, orgID, pendingTier)
	if err != nil {
		return false, fmt.Errorf("clearing pending plan tier conditionally: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func (s *PgStore) ListOrgsWithPendingDowngrade(ctx context.Context) ([]OrgSubscription, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, org_id, plan_tier, stripe_subscription_id, stripe_customer_id,
			status, current_period_start, current_period_end,
			spending_limit_microusd, limit_action,
			COALESCE(overage_disabled, plan_tier = 'free'),
			pending_plan_tier, canceled_at,
			COALESCE(anomaly_threshold_warning, 3.0),
			COALESCE(anomaly_threshold_critical, 10.0),
			grace_period_end, COALESCE(payment_status, 'ok'),
			override_daily_run_limit, override_concurrent_run_limit,
			COALESCE(enforcement_mode, 'enforce'),
			COALESCE(monthly_usage_email, false),
			created_at, updated_at
		FROM organization_subscriptions
		WHERE pending_plan_tier IS NOT NULL
		  AND current_period_end IS NOT NULL
		  AND current_period_end < NOW()
		LIMIT 10000
	`)
	if err != nil {
		return nil, fmt.Errorf("listing orgs with pending downgrade: %w", err)
	}
	defer rows.Close()

	var subs []OrgSubscription
	for rows.Next() {
		var sub OrgSubscription
		if err := rows.Scan(
			&sub.ID, &sub.OrgID, &sub.PlanTier,
			&sub.StripeSubscriptionID, &sub.StripeCustomerID,
			&sub.Status, &sub.CurrentPeriodStart, &sub.CurrentPeriodEnd,
			&sub.SpendingLimitMicrousd, &sub.LimitAction,
			&sub.OverageDisabled,
			&sub.PendingPlanTier, &sub.CanceledAt,
			&sub.AnomalyThresholdWarning, &sub.AnomalyThresholdCritical,
			&sub.GracePeriodEnd, &sub.PaymentStatus,
			&sub.OverrideDailyRunLimit, &sub.OverrideConcurrentRunLimit,
			&sub.EnforcementMode,
			&sub.MonthlyUsageEmail,
			&sub.CreatedAt, &sub.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning pending downgrade: %w", err)
		}
		subs = append(subs, sub)
	}
	return subs, rows.Err()
}

func (s *PgStore) UpdateSpendingLimit(ctx context.Context, orgID string, limitMicrousd int64, action string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE organization_subscriptions
		SET spending_limit_microusd = $2, limit_action = $3, updated_at = NOW()
		WHERE org_id = $1
	`, orgID, limitMicrousd, action)
	if err != nil {
		return fmt.Errorf("updating spending limit: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrSubscriptionNotFound
	}
	return nil
}

func (s *PgStore) UpdateOverageDisabled(ctx context.Context, orgID string, disabled bool) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE organization_subscriptions
		SET overage_disabled = $2, updated_at = NOW()
		WHERE org_id = $1
	`, orgID, disabled)
	if err != nil {
		return fmt.Errorf("updating overage disabled flag: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrSubscriptionNotFound
	}
	return nil
}

// TryMarkBillingCapEvent stamps the per-event dedup column to NOW() iff it
// is currently NULL, returning true when this caller was the first to mark.
// Subsequent calls in the same billing period return false. The column is
// reset on period rollover by UpsertOrgSubscription.
func (s *PgStore) TryMarkBillingCapEvent(ctx context.Context, orgID string, ev BillingCapEvent) (bool, error) {
	col := ev.Column()
	if col == "" {
		return false, fmt.Errorf("unknown billing cap event: %d", ev)
	}
	// Column name is whitelisted by BillingCapEvent.Column; safe to interpolate.
	query := fmt.Sprintf(`
		UPDATE organization_subscriptions
		SET %s = NOW(), updated_at = NOW()
		WHERE org_id = $1 AND %s IS NULL
	`, col, col)
	tag, err := s.pool.Exec(ctx, query, orgID)
	if err != nil {
		return false, fmt.Errorf("marking billing cap event %s: %w", col, err)
	}
	return tag.RowsAffected() == 1, nil
}

func (s *PgStore) GetProjectOrgID(ctx context.Context, projectID string) (string, error) {
	var orgID *string
	err := s.pool.QueryRow(ctx, `
		SELECT org_id FROM projects WHERE id = $1
	`, projectID).Scan(&orgID)
	if err != nil {
		return "", fmt.Errorf("getting project org_id: %w", err)
	}
	if orgID == nil {
		return "", nil
	}
	return *orgID, nil
}

func (s *PgStore) GetActiveProjectOrgID(ctx context.Context, projectID string) (string, error) {
	var orgID *string
	err := s.pool.QueryRow(ctx, `
		SELECT org_id
		FROM projects
		WHERE id = $1
		  AND deleted_at IS NULL
	`, projectID).Scan(&orgID)
	if err != nil {
		return "", fmt.Errorf("getting active project org_id: %w", err)
	}
	if orgID == nil {
		return "", nil
	}
	return *orgID, nil
}

func (s *PgStore) ListProjectsByOrg(ctx context.Context, orgID string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id
		FROM projects
		WHERE org_id = $1
		  AND deleted_at IS NULL
	`, orgID)
	if err != nil {
		return nil, fmt.Errorf("listing projects by org: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning project id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *PgStore) CountProjectsByOrg(ctx context.Context, orgID string) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM projects
		WHERE org_id = $1
		  AND deleted_at IS NULL
	`, orgID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting projects by org: %w", err)
	}
	return count, nil
}

func (s *PgStore) CountMembersByOrg(ctx context.Context, orgID string) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(DISTINCT pmr.user_id)
		FROM project_member_roles pmr
		JOIN projects p ON p.id = pmr.project_id
		WHERE p.org_id = $1
	`, orgID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting members by org: %w", err)
	}
	return count, nil
}

func (s *PgStore) CountOrgsByUser(ctx context.Context, userID string) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(DISTINCT p.org_id)
		FROM project_member_roles pmr
		JOIN projects p ON p.id = pmr.project_id
		WHERE pmr.user_id = $1
		  AND p.org_id IS NOT NULL
	`, userID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting orgs by user: %w", err)
	}
	return count, nil
}

func (s *PgStore) CountExecutingRunsByOrg(ctx context.Context, orgID string) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM job_run_read_state rs
		JOIN projects p ON p.id = rs.project_id
		WHERE p.org_id = $1
		  AND rs.status = 'executing'
	`, orgID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting executing runs by org: %w", err)
	}
	return count, nil
}

// BulkCountExecutingRunsByOrg counts executing runs for multiple orgs in a single
// query, returning a map of orgID -> count. Orgs with zero executing runs are
// included with count 0 if they appear in the input slice.
func (s *PgStore) BulkCountExecutingRunsByOrg(ctx context.Context, orgIDs []string) (map[string]int, error) {
	if len(orgIDs) == 0 {
		return map[string]int{}, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT p.org_id, COUNT(rs.run_id)::int
		FROM job_run_read_state rs
		JOIN projects p ON p.id = rs.project_id
		WHERE p.org_id = ANY($1)
		  AND rs.status = 'executing'
		GROUP BY p.org_id
	`, orgIDs)
	if err != nil {
		return nil, fmt.Errorf("bulk counting executing runs by org: %w", err)
	}
	defer rows.Close()

	result := make(map[string]int, len(orgIDs))
	for rows.Next() {
		var orgID string
		var count int
		if err := rows.Scan(&orgID, &count); err != nil {
			return nil, fmt.Errorf("scanning bulk executing run count: %w", err)
		}
		result[orgID] = count
	}
	return result, rows.Err()
}

func (s *PgStore) SetProjectOrgID(ctx context.Context, projectID, orgID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE projects SET org_id = $2 WHERE id = $1
	`, projectID, orgID)
	if err != nil {
		return fmt.Errorf("setting project org_id: %w", err)
	}
	return nil
}

func (s *PgStore) UpsertUsageRecord(ctx context.Context, rec *UsageRecord) error {
	return upsertUsageRecord(ctx, s.pool, rec)
}

func (s *PgStore) RecordUsageCost(ctx context.Context, rec *UsageRecord, idempotencyKey, executionMode string) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin record usage cost tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `
		INSERT INTO billing_cost_events (
			idempotency_key, org_id, project_id, period_date, execution_mode,
			compute_cost_microusd, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (idempotency_key) DO NOTHING
	`, idempotencyKey, rec.OrgID, rec.ProjectID, rec.PeriodDate, executionMode, rec.ComputeCostMicro, rec.CreatedAt)
	if err != nil {
		return false, fmt.Errorf("recording billing cost event: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}

	if err := upsertUsageRecord(ctx, tx, rec); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit record usage cost tx: %w", err)
	}
	return true, nil
}

func (s *PgStore) ReconcileFlatUsageCosts(ctx context.Context, orgID string, date time.Time) error {
	if err := s.reconcileCompletedRunCosts(ctx, orgID, date); err != nil {
		return err
	}
	return s.reconcileDeliveredWebhookCosts(ctx, orgID, date)
}

func (s *PgStore) reconcileCompletedRunCosts(ctx context.Context, orgID string, date time.Time) error {
	_, err := s.pool.Exec(ctx, `
		WITH missing AS (
			SELECT
				'strait:cost_recorded:' || jr.id AS idempotency_key,
				p.org_id,
				jr.project_id,
				DATE(COALESCE(rs.finished_at, jr.created_at)) AS period_date,
				COALESCE(NULLIF(rs.execution_mode, ''), 'http') AS execution_mode,
				CASE
					WHEN COALESCE(NULLIF(rs.execution_mode, ''), 'http') = 'worker' THEN $3::bigint
					ELSE $4::bigint
				END AS compute_cost_microusd,
				COALESCE(rs.finished_at, jr.created_at) AS created_at
			FROM job_runs jr
			JOIN job_run_read_state rs ON rs.run_id = jr.id
			JOIN projects p ON p.id = jr.project_id
			LEFT JOIN billing_cost_events b ON b.idempotency_key = 'strait:cost_recorded:' || jr.id
			WHERE p.org_id = $1
			  AND rs.status = $5
			  AND DATE(COALESCE(rs.finished_at, jr.created_at)) = $2
			  AND b.idempotency_key IS NULL
		), inserted AS (
			INSERT INTO billing_cost_events (
				idempotency_key, org_id, project_id, period_date, execution_mode,
				compute_cost_microusd, created_at
			)
			SELECT idempotency_key, org_id, project_id, period_date, execution_mode,
				compute_cost_microusd, created_at
			FROM missing
			ON CONFLICT (idempotency_key) DO NOTHING
			RETURNING org_id, project_id, period_date, compute_cost_microusd, created_at
		), aggregated AS (
			SELECT
				org_id,
				project_id,
				period_date,
				COUNT(*)::bigint AS runs_count,
				SUM(compute_cost_microusd)::bigint AS compute_cost_microusd,
				MIN(created_at) AS created_at,
				MAX(created_at) AS updated_at
			FROM inserted
			GROUP BY org_id, project_id, period_date
		)
		INSERT INTO usage_records (
			id, org_id, project_id, period_date,
			runs_count, compute_cost_microusd, usage_tokens_total, usage_cost_microusd,
			created_at, updated_at
		)
		SELECT gen_random_uuid()::text, org_id, project_id, period_date,
			runs_count, compute_cost_microusd, 0, 0, created_at, updated_at
		FROM aggregated
		ON CONFLICT (org_id, project_id, period_date) DO UPDATE SET
			runs_count = usage_records.runs_count + EXCLUDED.runs_count,
			compute_cost_microusd = usage_records.compute_cost_microusd + EXCLUDED.compute_cost_microusd,
			updated_at = NOW()
	`, orgID, date, WorkerCostPerRunMicrousd, HTTPCostPerRunMicrousd, domain.StatusCompleted)
	if err != nil {
		return fmt.Errorf("reconciling completed run usage costs: %w", err)
	}
	return nil
}

func (s *PgStore) reconcileDeliveredWebhookCosts(ctx context.Context, orgID string, date time.Time) error {
	_, err := s.pool.Exec(ctx, `
		WITH missing AS (
			SELECT
				'strait:cost_recorded:' || wd.id AS idempotency_key,
				p.org_id,
				wd.project_id,
				DATE(COALESCE(wd.delivered_at, wd.updated_at, wd.created_at)) AS period_date,
				'webhook_delivery' AS execution_mode,
				$3::bigint AS compute_cost_microusd,
				COALESCE(wd.delivered_at, wd.updated_at, wd.created_at) AS created_at
			FROM webhook_deliveries wd
			JOIN projects p ON p.id = wd.project_id
			LEFT JOIN billing_cost_events b ON b.idempotency_key = 'strait:cost_recorded:' || wd.id
			WHERE p.org_id = $1
			  AND wd.status = $4
			  AND DATE(COALESCE(wd.delivered_at, wd.updated_at, wd.created_at)) = $2
			  AND b.idempotency_key IS NULL
		), inserted AS (
			INSERT INTO billing_cost_events (
				idempotency_key, org_id, project_id, period_date, execution_mode,
				compute_cost_microusd, created_at
			)
			SELECT idempotency_key, org_id, project_id, period_date, execution_mode,
				compute_cost_microusd, created_at
			FROM missing
			ON CONFLICT (idempotency_key) DO NOTHING
			RETURNING org_id, project_id, period_date, compute_cost_microusd, created_at
		), aggregated AS (
			SELECT
				org_id,
				project_id,
				period_date,
				COUNT(*)::bigint AS runs_count,
				SUM(compute_cost_microusd)::bigint AS compute_cost_microusd,
				MIN(created_at) AS created_at,
				MAX(created_at) AS updated_at
			FROM inserted
			GROUP BY org_id, project_id, period_date
		)
		INSERT INTO usage_records (
			id, org_id, project_id, period_date,
			runs_count, compute_cost_microusd, usage_tokens_total, usage_cost_microusd,
			created_at, updated_at
		)
		SELECT gen_random_uuid()::text, org_id, project_id, period_date,
			runs_count, compute_cost_microusd, 0, 0, created_at, updated_at
		FROM aggregated
		ON CONFLICT (org_id, project_id, period_date) DO UPDATE SET
			runs_count = usage_records.runs_count + EXCLUDED.runs_count,
			compute_cost_microusd = usage_records.compute_cost_microusd + EXCLUDED.compute_cost_microusd,
			updated_at = NOW()
	`, orgID, date, WebhookDeliveryCostPerRunMicrousd, domain.WebhookStatusDelivered)
	if err != nil {
		return fmt.Errorf("reconciling delivered webhook usage costs: %w", err)
	}
	return nil
}

func upsertUsageRecord(ctx context.Context, q querier, rec *UsageRecord) error {
	_, err := q.Exec(ctx, `
		INSERT INTO usage_records (
			id, org_id, project_id, period_date,
			runs_count, compute_cost_microusd, usage_tokens_total, usage_cost_microusd,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (org_id, project_id, period_date) DO UPDATE SET
			runs_count = usage_records.runs_count + EXCLUDED.runs_count,
			compute_cost_microusd = usage_records.compute_cost_microusd + EXCLUDED.compute_cost_microusd,
			usage_tokens_total = usage_records.usage_tokens_total + EXCLUDED.usage_tokens_total,
			usage_cost_microusd = usage_records.usage_cost_microusd + EXCLUDED.usage_cost_microusd,
			updated_at = NOW()
	`, rec.ID, rec.OrgID, rec.ProjectID, rec.PeriodDate,
		rec.RunsCount, rec.ComputeCostMicro, rec.UsageTokensTotal, rec.UsageCostMicro,
		rec.CreatedAt, rec.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("upserting usage record: %w", err)
	}
	return nil
}

func (s *PgStore) ReplaceUsageRecord(ctx context.Context, rec *UsageRecord) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO usage_records (
			id, org_id, project_id, period_date,
			runs_count, compute_cost_microusd, usage_tokens_total, usage_cost_microusd,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (org_id, project_id, period_date) DO UPDATE SET
			runs_count = EXCLUDED.runs_count,
			compute_cost_microusd = EXCLUDED.compute_cost_microusd,
			usage_tokens_total = EXCLUDED.usage_tokens_total,
			usage_cost_microusd = EXCLUDED.usage_cost_microusd,
			updated_at = NOW()
	`, rec.ID, rec.OrgID, rec.ProjectID, rec.PeriodDate,
		rec.RunsCount, rec.ComputeCostMicro, rec.UsageTokensTotal, rec.UsageCostMicro,
		rec.CreatedAt, rec.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("replacing usage record: %w", err)
	}
	return nil
}

func (s *PgStore) GetOrgUsageForPeriod(ctx context.Context, orgID string, from, to time.Time) ([]UsageRecord, error) {
	return s.getOrgUsageForPeriod(ctx, orgID, from, to, 0)
}

func (s *PgStore) GetOrgUsageForPeriodLimited(ctx context.Context, orgID string, from, to time.Time, limit int) ([]UsageRecord, error) {
	if limit <= 0 {
		return nil, errors.New("usage period limit must be positive")
	}
	return s.getOrgUsageForPeriod(ctx, orgID, from, to, limit)
}

func (s *PgStore) getOrgUsageForPeriod(ctx context.Context, orgID string, from, to time.Time, limit int) ([]UsageRecord, error) {
	endExclusive := to.AddDate(0, 0, 1)
	query := `
		WITH run_counts AS (
			SELECT p.org_id,
				jr.project_id,
				DATE(jr.created_at) AS period_date,
				COUNT(*)::bigint AS runs_count,
				0::bigint AS compute_cost_microusd,
				0::bigint AS usage_tokens_total,
				0::bigint AS usage_cost_microusd,
				MIN(jr.created_at) AS created_at,
				MAX(jr.created_at) AS updated_at
			FROM job_runs jr
			JOIN projects p ON p.id = jr.project_id
			WHERE p.org_id = $1
			  AND jr.created_at >= $2
			  AND jr.created_at < $3
			GROUP BY p.org_id, jr.project_id, DATE(jr.created_at)
			), recorded_compute AS (
				SELECT org_id,
					project_id,
					period_date,
					0::bigint AS runs_count,
					COALESCE(SUM(compute_cost_microusd), 0)::bigint AS compute_cost_microusd,
					0::bigint AS usage_tokens_total,
					0::bigint AS usage_cost_microusd,
					MIN(created_at) AS created_at,
					MAX(updated_at) AS updated_at
				FROM usage_records
				WHERE org_id = $1
				  AND period_date >= $2::date
				  AND period_date < $3::date
				GROUP BY org_id, project_id, period_date
			)
			SELECT '' AS id,
				org_id,
			project_id,
			period_date,
			SUM(runs_count) AS runs_count,
			SUM(compute_cost_microusd) AS compute_cost_microusd,
			SUM(usage_tokens_total) AS usage_tokens_total,
			SUM(usage_cost_microusd) AS usage_cost_microusd,
			MIN(created_at) AS created_at,
			MAX(updated_at) AS updated_at
			FROM (
				SELECT * FROM run_counts
				UNION ALL
				SELECT * FROM recorded_compute
			) usage
		GROUP BY org_id, project_id, period_date
		ORDER BY period_date ASC`
	args := []any{orgID, from, endExclusive}
	if limit > 0 {
		query += `
		LIMIT $4`
		args = append(args, limit)
	}

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("getting org usage for period: %w", err)
	}
	defer rows.Close()
	return scanUsageRecords(rows)
}

func (s *PgStore) GetProjectUsageForPeriod(ctx context.Context, projectID string, from, to time.Time) ([]UsageRecord, error) {
	endExclusive := to.AddDate(0, 0, 1)

	rows, err := s.pool.Query(ctx, `
		WITH run_counts AS (
			SELECT p.org_id,
				jr.project_id,
				DATE(jr.created_at) AS period_date,
				COUNT(*)::bigint AS runs_count,
				0::bigint AS compute_cost_microusd,
				0::bigint AS usage_tokens_total,
				0::bigint AS usage_cost_microusd,
				MIN(jr.created_at) AS created_at,
				MAX(jr.created_at) AS updated_at
			FROM job_runs jr
			JOIN projects p ON p.id = jr.project_id
			WHERE jr.project_id = $1
			  AND jr.created_at >= $2
			  AND jr.created_at < $3
			GROUP BY p.org_id, jr.project_id, DATE(jr.created_at)
			), recorded_compute AS (
				SELECT org_id,
					project_id,
					period_date,
					0::bigint AS runs_count,
					COALESCE(SUM(compute_cost_microusd), 0)::bigint AS compute_cost_microusd,
					0::bigint AS usage_tokens_total,
					0::bigint AS usage_cost_microusd,
					MIN(created_at) AS created_at,
					MAX(updated_at) AS updated_at
				FROM usage_records
				WHERE project_id = $1
				  AND period_date >= $2::date
				  AND period_date < $3::date
				GROUP BY org_id, project_id, period_date
			)
			SELECT '' AS id,
				org_id,
			project_id,
			period_date,
			SUM(runs_count) AS runs_count,
			SUM(compute_cost_microusd) AS compute_cost_microusd,
			SUM(usage_tokens_total) AS usage_tokens_total,
			SUM(usage_cost_microusd) AS usage_cost_microusd,
			MIN(created_at) AS created_at,
			MAX(updated_at) AS updated_at
			FROM (
				SELECT * FROM run_counts
				UNION ALL
				SELECT * FROM recorded_compute
			) usage
		GROUP BY org_id, project_id, period_date
		ORDER BY period_date ASC
	`, projectID, from, endExclusive)
	if err != nil {
		return nil, fmt.Errorf("getting project usage for period: %w", err)
	}
	defer rows.Close()
	return scanUsageRecords(rows)
}

func (s *PgStore) GetOrgDailyUsage(ctx context.Context, orgID string, date time.Time) ([]UsageRecord, error) {
	rows, err := s.pool.Query(ctx, `
		WITH run_counts AS (
			SELECT p.org_id,
				jr.project_id,
				DATE(jr.created_at) AS period_date,
				COUNT(*)::bigint AS runs_count,
				0::bigint AS compute_cost_microusd,
				0::bigint AS usage_tokens_total,
				0::bigint AS usage_cost_microusd,
				MIN(jr.created_at) AS created_at,
				MAX(jr.created_at) AS updated_at
			FROM job_runs jr
			JOIN projects p ON p.id = jr.project_id
			WHERE p.org_id = $1
			  AND DATE(jr.created_at) = $2
			GROUP BY p.org_id, jr.project_id, DATE(jr.created_at)
			), recorded_compute AS (
				SELECT org_id,
					project_id,
					period_date,
					0::bigint AS runs_count,
					COALESCE(SUM(compute_cost_microusd), 0)::bigint AS compute_cost_microusd,
					0::bigint AS usage_tokens_total,
					0::bigint AS usage_cost_microusd,
					MIN(created_at) AS created_at,
					MAX(updated_at) AS updated_at
				FROM usage_records
				WHERE org_id = $1
				  AND period_date = $2
				GROUP BY org_id, project_id, period_date
			)
			SELECT '' AS id,
				org_id,
			project_id,
			period_date,
			SUM(runs_count) AS runs_count,
			SUM(compute_cost_microusd) AS compute_cost_microusd,
			SUM(usage_tokens_total) AS usage_tokens_total,
			SUM(usage_cost_microusd) AS usage_cost_microusd,
			MIN(created_at) AS created_at,
			MAX(updated_at) AS updated_at
			FROM (
				SELECT * FROM run_counts
				UNION ALL
				SELECT * FROM recorded_compute
			) usage
		GROUP BY org_id, project_id, period_date
	`, orgID, date)
	if err != nil {
		return nil, fmt.Errorf("getting org daily usage: %w", err)
	}
	defer rows.Close()
	return scanUsageRecords(rows)
}

func (s *PgStore) SumOrgPeriodSpend(ctx context.Context, orgID string, from time.Time) (int64, error) {
	var sum int64
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(compute_cost_microusd), 0)
		FROM usage_records
		WHERE org_id = $1 AND period_date >= $2
	`, orgID, from).Scan(&sum)
	if err != nil {
		return 0, fmt.Errorf("summing org period spend: %w", err)
	}
	return sum, nil
}

func (s *PgStore) GetProjectBudget(ctx context.Context, projectID string) (int64, string, error) {
	var budget int64
	var action string
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(monthly_budget_microusd, -1), COALESCE(budget_action, 'notify')
		FROM project_quotas
		WHERE project_id = $1
	`, projectID).Scan(&budget, &action)
	if errors.Is(err, pgx.ErrNoRows) {
		return -1, "notify", nil
	}
	if err != nil {
		return -1, "notify", fmt.Errorf("getting project budget: %w", err)
	}
	return budget, action, nil
}

func (s *PgStore) SetProjectBudget(ctx context.Context, projectID string, budgetMicro int64, action string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO project_quotas (project_id, monthly_budget_microusd, budget_action)
		VALUES ($1, $2, $3)
		ON CONFLICT (project_id) DO UPDATE
		SET monthly_budget_microusd = EXCLUDED.monthly_budget_microusd,
		    budget_action = EXCLUDED.budget_action
	`, projectID, budgetMicro, action)
	if err != nil {
		return fmt.Errorf("setting project budget: %w", err)
	}
	return nil
}

func (s *PgStore) GetProjectPeriodSpend(ctx context.Context, projectID string, from time.Time) (int64, error) {
	var sum int64
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(compute_cost_microusd), 0)
		FROM usage_records
		WHERE project_id = $1 AND period_date >= $2
	`, projectID, from).Scan(&sum)
	if err != nil {
		return 0, fmt.Errorf("summing project period spend: %w", err)
	}
	return sum, nil
}

func (s *PgStore) UpdateAnomalyThresholds(ctx context.Context, orgID string, warning, critical float64) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE organization_subscriptions
		SET anomaly_threshold_warning = $2, anomaly_threshold_critical = $3, updated_at = NOW()
		WHERE org_id = $1
	`, orgID, warning, critical)
	if err != nil {
		return fmt.Errorf("updating anomaly thresholds: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrSubscriptionNotFound
	}
	return nil
}

func (s *PgStore) ListAllSubscribedOrgIDs(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx, listAllSubscribedOrgIDsSQL())
	if err != nil {
		return nil, fmt.Errorf("listing subscribed org IDs: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning subscribed org ID: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func listAllSubscribedOrgIDsSQL() string {
	return `
		SELECT org_id
		FROM organization_subscriptions
		WHERE status = 'active'
		ORDER BY org_id
	`
}

func (s *PgStore) UpdatePaymentStatus(ctx context.Context, orgID string, status string, graceEnd *time.Time) error {
	return WithBillingTx(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE organization_subscriptions
			SET payment_status = $2, grace_period_end = $3, updated_at = NOW()
			WHERE org_id = $1
		`, orgID, status, graceEnd)
		if err != nil {
			return fmt.Errorf("updating payment status: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return ErrSubscriptionNotFound
		}
		return s.refreshEntitlements(ctx, tx, orgID)
	})
}

// RestrictExpiredContractIfCurrent restricts an org only if the contract row
// still matches the expired, non-renewing state observed by the scheduler.
func (s *PgStore) RestrictExpiredContractIfCurrent(ctx context.Context, orgID string, contractEndDate time.Time) (bool, error) {
	entitlements, err := json.Marshal(GetPlanLimits(domain.PlanFree))
	if err != nil {
		return false, fmt.Errorf("marshalling expired contract entitlements: %w", err)
	}
	var restricted bool
	err = WithBillingTx(ctx, s.pool, func(tx pgx.Tx) error {
		tag, execErr := tx.Exec(ctx, `
			UPDATE organization_subscriptions os
			SET payment_status = 'restricted',
			    entitlements = $3::jsonb,
			    updated_at = NOW()
			WHERE os.org_id = $1
			  AND EXISTS (
			      SELECT 1
			      FROM enterprise_contracts ec
			      WHERE ec.org_id = $1
			        AND ec.contract_end_date = $2
			        AND ec.contract_end_date <= NOW()
			        AND ec.auto_renew = false
			  )
		`, orgID, contractEndDate, entitlements)
		if execErr != nil {
			return fmt.Errorf("restricting expired contract if current: %w", execErr)
		}
		restricted = tag.RowsAffected() > 0
		return nil
	})
	if err != nil {
		return false, err
	}
	return restricted, nil
}

// RestrictExpiredGracePeriod atomically moves an org from expired payment grace
// to restricted/free. The conditional WHERE clause makes concurrent recovery
// webhooks win without leaving payment_status and plan_tier half-updated.
func (s *PgStore) RestrictExpiredGracePeriod(ctx context.Context, orgID string, graceEnd *time.Time) (bool, error) {
	if graceEnd == nil {
		return false, nil
	}
	entitlements, err := json.Marshal(GetPlanLimits(domain.PlanFree))
	if err != nil {
		return false, fmt.Errorf("marshalling restricted entitlements: %w", err)
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE organization_subscriptions
		SET plan_tier = 'free',
			status = 'restricted',
			payment_status = 'restricted',
			entitlements = $3::jsonb,
			updated_at = NOW()
		WHERE org_id = $1
		  AND payment_status = 'grace'
		  AND grace_period_end = $2
		  AND grace_period_end < NOW()
	`, orgID, graceEnd, entitlements)
	if err != nil {
		return false, fmt.Errorf("restricting expired grace period: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func (s *PgStore) ListOrgsInGracePeriod(ctx context.Context) ([]OrgSubscription, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, org_id, plan_tier, stripe_subscription_id, stripe_customer_id,
			status, current_period_start, current_period_end,
			spending_limit_microusd, limit_action,
			COALESCE(overage_disabled, plan_tier = 'free'),
			pending_plan_tier, canceled_at,
			COALESCE(anomaly_threshold_warning, 3.0),
			COALESCE(anomaly_threshold_critical, 10.0),
			grace_period_end, COALESCE(payment_status, 'ok'),
			override_daily_run_limit, override_concurrent_run_limit,
			COALESCE(enforcement_mode, 'enforce'),
			COALESCE(monthly_usage_email, false),
			created_at, updated_at
		FROM organization_subscriptions
		WHERE payment_status = 'grace'
		  AND grace_period_end < NOW()
		LIMIT 10000
	`)
	if err != nil {
		return nil, fmt.Errorf("listing orgs in grace period: %w", err)
	}
	defer rows.Close()

	var subs []OrgSubscription
	for rows.Next() {
		var sub OrgSubscription
		if err := rows.Scan(
			&sub.ID, &sub.OrgID, &sub.PlanTier,
			&sub.StripeSubscriptionID, &sub.StripeCustomerID,
			&sub.Status, &sub.CurrentPeriodStart, &sub.CurrentPeriodEnd,
			&sub.SpendingLimitMicrousd, &sub.LimitAction,
			&sub.OverageDisabled,
			&sub.PendingPlanTier, &sub.CanceledAt,
			&sub.AnomalyThresholdWarning, &sub.AnomalyThresholdCritical,
			&sub.GracePeriodEnd, &sub.PaymentStatus,
			&sub.OverrideDailyRunLimit, &sub.OverrideConcurrentRunLimit,
			&sub.EnforcementMode,
			&sub.MonthlyUsageEmail,
			&sub.CreatedAt, &sub.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning grace period org: %w", err)
		}
		subs = append(subs, sub)
	}
	return subs, rows.Err()
}

// ListStaleSubscriptions returns subscriptions that are marked active but whose
// current_period_end has passed by more than 1 day without a pending downgrade.
// These may indicate missed cancellation webhooks from Stripe.
func (s *PgStore) ListStaleSubscriptions(ctx context.Context) ([]OrgSubscription, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, org_id, plan_tier, stripe_subscription_id, stripe_customer_id,
			status, current_period_start, current_period_end,
			spending_limit_microusd, limit_action,
			COALESCE(overage_disabled, plan_tier = 'free'),
			pending_plan_tier, canceled_at,
			COALESCE(anomaly_threshold_warning, 3.0),
			COALESCE(anomaly_threshold_critical, 10.0),
			grace_period_end, COALESCE(payment_status, 'ok'),
			override_daily_run_limit, override_concurrent_run_limit,
			COALESCE(enforcement_mode, 'enforce'),
			COALESCE(monthly_usage_email, false),
			created_at, updated_at
		FROM organization_subscriptions
		WHERE status = 'active'
		  AND current_period_end IS NOT NULL
		  AND current_period_end < NOW() - INTERVAL '1 day'
		  AND pending_plan_tier IS NULL
		LIMIT 10000
	`)
	if err != nil {
		return nil, fmt.Errorf("listing stale subscriptions: %w", err)
	}
	defer rows.Close()

	var subs []OrgSubscription
	for rows.Next() {
		var sub OrgSubscription
		if err := rows.Scan(
			&sub.ID, &sub.OrgID, &sub.PlanTier,
			&sub.StripeSubscriptionID, &sub.StripeCustomerID,
			&sub.Status, &sub.CurrentPeriodStart, &sub.CurrentPeriodEnd,
			&sub.SpendingLimitMicrousd, &sub.LimitAction,
			&sub.OverageDisabled,
			&sub.PendingPlanTier, &sub.CanceledAt,
			&sub.AnomalyThresholdWarning, &sub.AnomalyThresholdCritical,
			&sub.GracePeriodEnd, &sub.PaymentStatus,
			&sub.OverrideDailyRunLimit, &sub.OverrideConcurrentRunLimit,
			&sub.EnforcementMode,
			&sub.MonthlyUsageEmail,
			&sub.CreatedAt, &sub.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning stale subscription: %w", err)
		}
		subs = append(subs, sub)
	}
	return subs, rows.Err()
}

// IsProjectSuspended checks whether a project is suspended due to plan downgrade.
func (s *PgStore) IsProjectSuspended(ctx context.Context, projectID string) (bool, error) {
	var suspended bool
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(suspended, false)
		FROM projects
		WHERE id = $1
	`, projectID).Scan(&suspended)
	if err != nil {
		return false, fmt.Errorf("checking project suspended status: %w", err)
	}
	return suspended, nil
}

// SuspendExcessProjects suspends projects that exceed the plan limit for an org.
// It keeps the oldest maxProjects active (by created_at) and suspends the rest.
// Returns the number of projects that were suspended.
func (s *PgStore) SuspendExcessProjects(ctx context.Context, orgID string, maxProjects int) (int, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE projects SET suspended = true
		WHERE org_id = $1 AND suspended = false AND deleted_at IS NULL AND id NOT IN (
			SELECT id FROM projects
			WHERE org_id = $1 AND deleted_at IS NULL
			ORDER BY created_at ASC LIMIT $2
		)
	`, orgID, maxProjects)
	if err != nil {
		return 0, fmt.Errorf("suspending excess projects: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

func scanUsageRecords(rows pgx.Rows) ([]UsageRecord, error) {
	var records []UsageRecord
	for rows.Next() {
		var r UsageRecord
		if err := rows.Scan(
			&r.ID, &r.OrgID, &r.ProjectID, &r.PeriodDate,
			&r.RunsCount, &r.ComputeCostMicro, &r.UsageTokensTotal, &r.UsageCostMicro,
			&r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning usage record: %w", err)
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

// ListOrgAdminEmails returns email addresses for org admins.
// Joins through project_roles to filter by the 'admin' system role (which has wildcard permissions).
// Schema: project_member_roles.role_id -> project_roles.id; project_roles.name = 'admin'.
func (s *PgStore) ListOrgAdminEmails(ctx context.Context, orgID string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT u.email
		FROM project_member_roles pmr
		JOIN projects p ON p.id = pmr.project_id
		JOIN project_roles pr ON pr.id = pmr.role_id
		JOIN users u ON u.id = pmr.user_id
		WHERE p.org_id = $1
		  AND pr.name = 'admin'
		  AND u.email IS NOT NULL
		  AND u.email != ''
		  AND COALESCE(u.email_verified, false) = true
		ORDER BY u.email
	`, orgID)
	if err != nil {
		return nil, fmt.Errorf("listing org admin emails: %w", err)
	}
	defer rows.Close()

	var emails []string
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return nil, fmt.Errorf("scanning org admin email: %w", err)
		}
		emails = append(emails, email)
	}
	return emails, rows.Err()
}

// HasSentUsageReport checks if a usage report was already sent for this org and period.
func (s *PgStore) HasSentUsageReport(ctx context.Context, orgID string, periodEnd time.Time) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM sent_usage_reports
			WHERE org_id = $1 AND period_end = $2
			  AND send_status = 'sent'
		)
	`, orgID, periodEnd.Truncate(24*time.Hour)).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking sent usage report: %w", err)
	}
	return exists, nil
}

// RecordSentUsageReport records that a usage report email was sent for deduplication.
func (s *PgStore) RecordSentUsageReport(ctx context.Context, orgID string, periodEnd time.Time) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO sent_usage_reports (org_id, period_end, send_status, sent_at, claimed_at)
		VALUES ($1, $2, 'sent', NOW(), NULL)
		ON CONFLICT (org_id, period_end) DO UPDATE
		SET send_status = 'sent',
		    sent_at = NOW(),
		    claimed_at = NULL
	`, orgID, periodEnd.Truncate(24*time.Hour))
	if err != nil {
		return fmt.Errorf("recording sent usage report: %w", err)
	}
	return nil
}

// ClaimUsageReportSend atomically claims an org/period report before the email
// side effect. A false return means another scheduler already claimed or sent it.
func (s *PgStore) ClaimUsageReportSend(ctx context.Context, orgID string, periodEnd time.Time) (bool, error) {
	var claimed int
	err := s.pool.QueryRow(ctx, `
		INSERT INTO sent_usage_reports (org_id, period_end, send_status, claimed_at, sent_at)
		VALUES ($1, $2, 'claimed', NOW(), NULL)
		ON CONFLICT (org_id, period_end) DO UPDATE
		SET send_status = 'claimed',
		    claimed_at = NOW(),
		    sent_at = NULL
		WHERE sent_usage_reports.send_status = 'claimed'
		  AND sent_usage_reports.claimed_at < NOW() - INTERVAL '1 hour'
		RETURNING 1
	`, orgID, periodEnd.Truncate(24*time.Hour)).Scan(&claimed)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("claiming usage report send: %w", err)
	}
	return claimed == 1, nil
}

// FinalizeUsageReportSend marks a pre-send report claim as sent after the
// email side effect succeeds.
func (s *PgStore) FinalizeUsageReportSend(ctx context.Context, orgID string, periodEnd time.Time) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO sent_usage_reports (org_id, period_end, send_status, sent_at, claimed_at)
		VALUES ($1, $2, 'sent', NOW(), NULL)
		ON CONFLICT (org_id, period_end) DO UPDATE
		SET send_status = 'sent',
		    sent_at = NOW(),
		    claimed_at = NULL
	`, orgID, periodEnd.Truncate(24*time.Hour))
	if err != nil {
		return fmt.Errorf("finalizing usage report send: %w", err)
	}
	return nil
}

// ReleaseUsageReportSendClaim releases a pre-send claim after a definite send
// failure so a later scheduler tick can retry the report.
func (s *PgStore) ReleaseUsageReportSendClaim(ctx context.Context, orgID string, periodEnd time.Time) error {
	_, err := s.pool.Exec(ctx, `
		DELETE FROM sent_usage_reports
		WHERE org_id = $1 AND period_end = $2
		  AND send_status = 'claimed'
	`, orgID, periodEnd.Truncate(24*time.Hour))
	if err != nil {
		return fmt.Errorf("releasing usage report send claim: %w", err)
	}
	return nil
}

func (s *PgStore) ClaimContractReminderSend(ctx context.Context, orgID string, contractEndDate time.Time, reminderWindowDays int) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO contract_reminder_sends (org_id, contract_end_date, reminder_window_days)
		VALUES ($1, $2, $3)
		ON CONFLICT (org_id, contract_end_date, reminder_window_days) DO NOTHING
	`, orgID, contractEndDate.UTC().Truncate(24*time.Hour), reminderWindowDays)
	if err != nil {
		return false, fmt.Errorf("claiming contract reminder send: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// UpdateMonthlyUsageEmail updates the email preference for an org.
func (s *PgStore) UpdateMonthlyUsageEmail(ctx context.Context, orgID string, enabled bool) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE organization_subscriptions
		SET monthly_usage_email = $2, updated_at = NOW()
		WHERE org_id = $1
	`, orgID, enabled)
	if err != nil {
		return fmt.Errorf("updating monthly usage email preference: %w", err)
	}
	return nil
}

// DeactivateExcessCronJobs disables cron jobs and workflows beyond the given
// shared schedule limit for an org. Keeps the most recently updated schedules;
// clears cron on the oldest excess. Returns the IDs whose cron was cleared so
// callers can dispatch per-schedule suspension events.
func (s *PgStore) DeactivateExcessCronJobs(ctx context.Context, orgID string, maxSchedules int) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		WITH active_projects AS (
			SELECT id FROM projects WHERE org_id = $1 AND deleted_at IS NULL
		),
		ranked_schedules AS (
			SELECT 'job' AS kind, j.id, j.updated_at
			FROM jobs j
			WHERE j.project_id IN (SELECT id FROM active_projects)
			  AND j.cron IS NOT NULL AND j.cron != ''
			UNION ALL
			SELECT 'workflow' AS kind, w.id, w.updated_at
			FROM workflows w
			WHERE w.project_id IN (SELECT id FROM active_projects)
			  AND w.cron IS NOT NULL AND w.cron != ''
		),
		excess_schedules AS (
			SELECT kind, id
			FROM ranked_schedules
			ORDER BY updated_at DESC, id DESC
			OFFSET $2
		),
		disabled_jobs AS (
			UPDATE jobs SET cron = '', updated_at = NOW()
			WHERE id IN (SELECT id FROM excess_schedules WHERE kind = 'job')
			RETURNING id
		),
		disabled_workflows AS (
			UPDATE workflows SET cron = '', updated_at = NOW()
			WHERE id IN (SELECT id FROM excess_schedules WHERE kind = 'workflow')
			RETURNING id
		)
		SELECT id FROM disabled_jobs
		UNION ALL
		SELECT id FROM disabled_workflows
	`, orgID, maxSchedules)
	if err != nil {
		return nil, fmt.Errorf("deactivate excess cron schedules: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if scanErr := rows.Scan(&id); scanErr != nil {
			return nil, fmt.Errorf("scan deactivated cron schedule id: %w", scanErr)
		}
		ids = append(ids, id)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, fmt.Errorf("iterating deactivated cron schedules: %w", rowsErr)
	}
	return ids, nil
}

// DeactivateExcessEnvironments marks excess environments as deleted for an org.
// Keeps the most recently created environments; deactivates the oldest excess.
func (s *PgStore) DeactivateExcessEnvironments(ctx context.Context, orgID string, maxEnvironments int) (int64, error) {
	result, err := s.pool.Exec(ctx, `
		DELETE FROM environments
		WHERE id IN (
			SELECT e.id FROM environments e
			WHERE e.project_id IN (SELECT id FROM projects WHERE org_id = $1 AND deleted_at IS NULL)
			  AND e.is_standard = false
			ORDER BY e.created_at DESC
			OFFSET $2
		)
	`, orgID, maxEnvironments)
	if err != nil {
		return 0, fmt.Errorf("deactivate excess environments: %w", err)
	}
	return result.RowsAffected(), nil
}

// DeactivateExcessWebhookSubscriptions deactivates webhook subscriptions beyond the limit.
// Keeps the most recently created subscriptions; deactivates the oldest excess.
func (s *PgStore) DeactivateExcessWebhookSubscriptions(ctx context.Context, orgID string, maxEndpoints int) (int64, error) {
	result, err := s.pool.Exec(ctx, `
		UPDATE webhook_subscriptions SET active = false
		WHERE id IN (
			SELECT ws.id FROM webhook_subscriptions ws
			WHERE ws.project_id IN (SELECT id FROM projects WHERE org_id = $1 AND deleted_at IS NULL)
			  AND ws.active = true
			ORDER BY ws.created_at DESC
			OFFSET $2
		)
	`, orgID, maxEndpoints)
	if err != nil {
		return 0, fmt.Errorf("deactivate excess webhook subscriptions: %w", err)
	}
	return result.RowsAffected(), nil
}

// DeactivateExcessLogDrains disables log drains beyond the org-wide limit.
// Keeps the most recently created drains; disables the oldest excess.
func (s *PgStore) DeactivateExcessLogDrains(ctx context.Context, orgID string, maxDrains int) (int64, error) {
	result, err := s.pool.Exec(ctx, `
		UPDATE log_drains SET enabled = false, updated_at = NOW()
		WHERE id IN (
			SELECT ld.id FROM log_drains ld
			WHERE ld.project_id IN (SELECT id FROM projects WHERE org_id = $1 AND deleted_at IS NULL)
			  AND ld.enabled = true
			ORDER BY ld.created_at DESC
			OFFSET $2
		)
	`, orgID, maxDrains)
	if err != nil {
		return 0, fmt.Errorf("deactivate excess log drains: %w", err)
	}
	return result.RowsAffected(), nil
}

// DeactivateExcessNotificationChannelsByProject disables notification channels
// beyond the per-project limit. Keeps the most recently created channels.
func (s *PgStore) DeactivateExcessNotificationChannelsByProject(ctx context.Context, projectID string, maxChannels int) (int64, error) {
	result, err := s.pool.Exec(ctx, `
		UPDATE notification_channels SET enabled = false, updated_at = NOW()
		WHERE id IN (
			SELECT nc.id FROM notification_channels nc
			WHERE nc.project_id = $1
			  AND nc.enabled = true
			ORDER BY nc.created_at DESC
			OFFSET $2
		)
	`, projectID, maxChannels)
	if err != nil {
		return 0, fmt.Errorf("deactivate excess notification channels: %w", err)
	}
	return result.RowsAffected(), nil
}

// ListActiveAddons returns all active add-ons for an organization.
func (s *PgStore) ListActiveAddons(ctx context.Context, orgID string) ([]Addon, error) {
	return s.listActiveAddons(ctx, s.pool, orgID)
}

func (s *PgStore) listActiveAddons(ctx context.Context, q querier, orgID string) ([]Addon, error) {
	rows, err := q.Query(ctx, `
		SELECT id, org_id, addon_type, quantity, stripe_subscription_id, stripe_lookup_key, active, expires_at, created_at, updated_at
		FROM organization_addons
		WHERE org_id = $1 AND active = true
		ORDER BY created_at
	`, orgID)
	if err != nil {
		return nil, fmt.Errorf("listing active addons: %w", err)
	}
	defer rows.Close()

	var addons []Addon
	for rows.Next() {
		var a Addon
		if err := rows.Scan(&a.ID, &a.OrgID, &a.AddonType, &a.Quantity, &a.StripeSubscriptionID, &a.StripeLookupKey, &a.Active, &a.ExpiresAt, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning addon row: %w", err)
		}
		addons = append(addons, a)
	}
	return addons, rows.Err()
}

// CreateAddon inserts a new add-on record.
func (s *PgStore) CreateAddon(ctx context.Context, addon *Addon) error {
	return WithBillingTx(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO organization_addons (id, org_id, addon_type, quantity, stripe_subscription_id, stripe_lookup_key, active, expires_at, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
		`, addon.ID, addon.OrgID, addon.AddonType, addon.Quantity, addon.StripeSubscriptionID, addon.StripeLookupKey, addon.Active, addon.ExpiresAt); err != nil {
			return fmt.Errorf("creating addon: %w", err)
		}
		return s.refreshEntitlements(ctx, tx, addon.OrgID)
	})
}

// DeactivateAddon sets an add-on to inactive.
func (s *PgStore) DeactivateAddon(ctx context.Context, addonID string) error {
	return WithBillingTx(ctx, s.pool, func(tx pgx.Tx) error {
		var orgID string
		err := tx.QueryRow(ctx, `
			UPDATE organization_addons SET active = false, updated_at = NOW()
			WHERE id = $1
			RETURNING org_id
		`, addonID).Scan(&orgID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			return fmt.Errorf("deactivating addon: %w", err)
		}
		return s.refreshEntitlements(ctx, tx, orgID)
	})
}

// CountActiveAddonsByType returns the number of active add-ons for an org and type.
func (s *PgStore) CountActiveAddonsByType(ctx context.Context, orgID string, addonType AddonType) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM organization_addons WHERE org_id = $1 AND addon_type = $2 AND active = true",
		orgID, string(addonType)).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count active addons by type: %w", err)
	}
	return count, nil
}

// RecordProcessedWebhook records a webhook message ID as processed for idempotency.
func (s *PgStore) RecordProcessedWebhook(ctx context.Context, msgID string) error {
	_, err := s.pool.Exec(ctx,
		"INSERT INTO processed_webhook_messages (msg_id, status) VALUES ($1, 'processed') ON CONFLICT (msg_id) DO UPDATE SET status = 'processed', processed_at = NOW()",
		msgID)
	if err != nil {
		return fmt.Errorf("record processed webhook: %w", err)
	}
	return nil
}

func (s *PgStore) ClaimWebhookForProcessing(ctx context.Context, msgID string, staleAfter time.Duration) (bool, error) {
	if msgID == "" {
		return true, nil
	}
	if staleAfter <= 0 {
		staleAfter = 10 * time.Minute
	}
	cutoff := time.Now().UTC().Add(-staleAfter)
	var claimed bool
	err := s.pool.QueryRow(ctx, `
		WITH inserted AS (
			INSERT INTO processed_webhook_messages (msg_id, status, processed_at)
			VALUES ($1, 'processing', NOW())
			ON CONFLICT (msg_id) DO NOTHING
			RETURNING 1
		),
		reclaimed AS (
			UPDATE processed_webhook_messages
			SET status = 'processing', processed_at = NOW()
			WHERE msg_id = $1
			  AND status = 'processing'
			  AND processed_at < $2
			  AND NOT EXISTS (SELECT 1 FROM inserted)
			RETURNING 1
		)
		SELECT EXISTS (SELECT 1 FROM inserted UNION ALL SELECT 1 FROM reclaimed)
	`, msgID, cutoff).Scan(&claimed)
	if err != nil {
		return false, fmt.Errorf("claim webhook for processing: %w", err)
	}
	return claimed, nil
}

func (s *PgStore) MarkWebhookProcessed(ctx context.Context, msgID string) error {
	if msgID == "" {
		return nil
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE processed_webhook_messages
		SET status = 'processed', processed_at = NOW()
		WHERE msg_id = $1
	`, msgID)
	if err != nil {
		return fmt.Errorf("mark webhook processed: %w", err)
	}
	return nil
}

func (s *PgStore) ReleaseWebhookClaim(ctx context.Context, msgID string) error {
	if msgID == "" {
		return nil
	}
	_, err := s.pool.Exec(ctx, `
		DELETE FROM processed_webhook_messages
		WHERE msg_id = $1 AND status = 'processing'
	`, msgID)
	if err != nil {
		return fmt.Errorf("release webhook claim: %w", err)
	}
	return nil
}

func (s *PgStore) GetWebhookProcessingStatus(ctx context.Context, msgID string) (string, error) {
	if msgID == "" {
		return "", nil
	}
	var status string
	err := s.pool.QueryRow(ctx,
		`SELECT status FROM processed_webhook_messages WHERE msg_id = $1`,
		msgID,
	).Scan(&status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("get webhook processing status: %w", err)
	}
	return status, nil
}

// IsWebhookProcessed checks whether a webhook message ID has already been processed.
func (s *PgStore) IsWebhookProcessed(ctx context.Context, msgID string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM processed_webhook_messages WHERE msg_id = $1 AND status = 'processed')",
		msgID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check webhook processed: %w", err)
	}
	return exists, nil
}

// DeleteOldWebhookMessages removes processed webhook message records older than the given time.
func (s *PgStore) DeleteOldWebhookMessages(ctx context.Context, olderThan time.Time) (int64, error) {
	tag, err := s.pool.Exec(ctx,
		"DELETE FROM processed_webhook_messages WHERE processed_at < $1",
		olderThan)
	if err != nil {
		return 0, fmt.Errorf("delete old webhook messages: %w", err)
	}
	return tag.RowsAffected(), nil
}

// GetEnterpriseContract returns the enterprise contract for an org.
func (s *PgStore) GetEnterpriseContract(ctx context.Context, orgID string) (*EnterpriseContract, error) {
	var c EnterpriseContract
	err := s.pool.QueryRow(ctx, `
		SELECT id, org_id, enterprise_tier, annual_commitment_cents,
			overage_discount_pct,
			contract_start_date, contract_end_date,
			auto_renew, billing_cadence, stripe_subscription_id,
			notes, created_at, updated_at
		FROM enterprise_contracts
		WHERE org_id = $1
	`, orgID).Scan(
		&c.ID, &c.OrgID, &c.EnterpriseTier,
		&c.AnnualCommitmentCents, &c.OverageDiscountPct,
		&c.ContractStartDate, &c.ContractEndDate,
		&c.AutoRenew, &c.BillingCadence, &c.StripeSubscriptionID,
		&c.Notes, &c.CreatedAt, &c.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrContractNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting enterprise contract: %w", err)
	}
	return &c, nil
}

// UpsertEnterpriseContract creates or updates an enterprise contract for an org.
func (s *PgStore) UpsertEnterpriseContract(ctx context.Context, c *EnterpriseContract) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO enterprise_contracts (
			id, org_id, enterprise_tier, annual_commitment_cents,
			overage_discount_pct,
			contract_start_date, contract_end_date,
			auto_renew, billing_cadence, stripe_subscription_id,
			notes, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (org_id) DO UPDATE SET
			enterprise_tier = EXCLUDED.enterprise_tier,
			annual_commitment_cents = EXCLUDED.annual_commitment_cents,
			overage_discount_pct = EXCLUDED.overage_discount_pct,
			contract_start_date = EXCLUDED.contract_start_date,
			contract_end_date = EXCLUDED.contract_end_date,
			auto_renew = EXCLUDED.auto_renew,
			billing_cadence = EXCLUDED.billing_cadence,
			stripe_subscription_id = EXCLUDED.stripe_subscription_id,
			notes = EXCLUDED.notes,
			updated_at = NOW()
	`, c.ID, c.OrgID, c.EnterpriseTier,
		c.AnnualCommitmentCents, c.OverageDiscountPct,
		c.ContractStartDate, c.ContractEndDate,
		c.AutoRenew, c.BillingCadence, c.StripeSubscriptionID,
		c.Notes, c.CreatedAt, c.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("upserting enterprise contract: %w", err)
	}
	return nil
}

// ListExpiringContracts returns enterprise contracts expiring within the given number of days.
func (s *PgStore) ListExpiringContracts(ctx context.Context, withinDays int) ([]EnterpriseContract, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, org_id, enterprise_tier, annual_commitment_cents,
			overage_discount_pct,
			contract_start_date, contract_end_date,
			auto_renew, billing_cadence, stripe_subscription_id,
			notes, created_at, updated_at
		FROM enterprise_contracts
		WHERE contract_end_date <= NOW() + make_interval(days => $1)
		  AND contract_end_date > NOW()
		ORDER BY contract_end_date ASC
	`, withinDays)
	if err != nil {
		return nil, fmt.Errorf("listing expiring contracts: %w", err)
	}
	defer rows.Close()

	var contracts []EnterpriseContract
	for rows.Next() {
		var c EnterpriseContract
		if err := rows.Scan(
			&c.ID, &c.OrgID, &c.EnterpriseTier,
			&c.AnnualCommitmentCents, &c.OverageDiscountPct,
			&c.ContractStartDate, &c.ContractEndDate,
			&c.AutoRenew, &c.BillingCadence, &c.StripeSubscriptionID,
			&c.Notes, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning enterprise contract: %w", err)
		}
		contracts = append(contracts, c)
	}
	return contracts, rows.Err()
}

func (s *PgStore) ListExpiredContracts(ctx context.Context) ([]EnterpriseContract, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, org_id, enterprise_tier, annual_commitment_cents,
			overage_discount_pct,
			contract_start_date, contract_end_date,
			auto_renew, billing_cadence, stripe_subscription_id,
			notes, created_at, updated_at
		FROM enterprise_contracts
		WHERE contract_end_date <= NOW()
		ORDER BY contract_end_date ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("listing expired contracts: %w", err)
	}
	defer rows.Close()

	var contracts []EnterpriseContract
	for rows.Next() {
		var c EnterpriseContract
		if err := rows.Scan(
			&c.ID, &c.OrgID, &c.EnterpriseTier,
			&c.AnnualCommitmentCents, &c.OverageDiscountPct,
			&c.ContractStartDate, &c.ContractEndDate,
			&c.AutoRenew, &c.BillingCadence, &c.StripeSubscriptionID,
			&c.Notes, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning expired enterprise contract: %w", err)
		}
		contracts = append(contracts, c)
	}
	return contracts, rows.Err()
}

// ListEnterpriseContractsActiveAt returns every enterprise contract whose
// window covers the supplied instant. The SLA-credit calculator passes the
// period-end so a contract that lapsed mid-month still contributes a credit
// for the partial period it covered.
func (s *PgStore) ListEnterpriseContractsActiveAt(ctx context.Context, at time.Time) ([]EnterpriseContract, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, org_id, enterprise_tier, annual_commitment_cents,
			overage_discount_pct,
			contract_start_date, contract_end_date,
			auto_renew, billing_cadence, stripe_subscription_id,
			notes, created_at, updated_at
		FROM enterprise_contracts
		WHERE contract_start_date <= $1
		  AND contract_end_date   >  $1
		ORDER BY org_id ASC
	`, at)
	if err != nil {
		return nil, fmt.Errorf("listing active enterprise contracts: %w", err)
	}
	defer rows.Close()

	var contracts []EnterpriseContract
	for rows.Next() {
		var c EnterpriseContract
		if err := rows.Scan(
			&c.ID, &c.OrgID, &c.EnterpriseTier,
			&c.AnnualCommitmentCents, &c.OverageDiscountPct,
			&c.ContractStartDate, &c.ContractEndDate,
			&c.AutoRenew, &c.BillingCadence, &c.StripeSubscriptionID,
			&c.Notes, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning enterprise contract: %w", err)
		}
		contracts = append(contracts, c)
	}
	return contracts, rows.Err()
}

// ListEnterpriseContractsOverlappingPeriod returns every enterprise contract
// whose contract window overlaps [periodStart, periodEnd). This is used for
// SLA credits because a contract that lapsed mid-month still covered part of
// the credited period.
func (s *PgStore) ListEnterpriseContractsOverlappingPeriod(ctx context.Context, periodStart, periodEnd time.Time) ([]EnterpriseContract, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, org_id, enterprise_tier, annual_commitment_cents,
			overage_discount_pct,
			contract_start_date, contract_end_date,
			auto_renew, billing_cadence, stripe_subscription_id,
			notes, created_at, updated_at
		FROM enterprise_contracts
		WHERE contract_start_date < $2
		  AND contract_end_date   > $1
		ORDER BY org_id ASC
	`, periodStart, periodEnd)
	if err != nil {
		return nil, fmt.Errorf("listing overlapping enterprise contracts: %w", err)
	}
	defer rows.Close()

	var contracts []EnterpriseContract
	for rows.Next() {
		var c EnterpriseContract
		if err := rows.Scan(
			&c.ID, &c.OrgID, &c.EnterpriseTier,
			&c.AnnualCommitmentCents, &c.OverageDiscountPct,
			&c.ContractStartDate, &c.ContractEndDate,
			&c.AutoRenew, &c.BillingCadence, &c.StripeSubscriptionID,
			&c.Notes, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning overlapping enterprise contract: %w", err)
		}
		contracts = append(contracts, c)
	}
	return contracts, rows.Err()
}

// PauseHTTPJobsByOrg pauses all active HTTP-mode jobs for an org.
// Sets the pause reason so they can be selectively unpaused on upgrade.
// Returns the IDs of the jobs that were paused so callers can dispatch
// per-schedule suspension events.
func (s *PgStore) PauseHTTPJobsByOrg(ctx context.Context, orgID, reason string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		UPDATE jobs SET
			paused = true,
			paused_at = NOW(),
			pause_reason = $2,
			updated_at = NOW()
		WHERE project_id IN (SELECT id FROM projects WHERE org_id = $1 AND deleted_at IS NULL)
		  AND execution_mode = 'http'
		  AND paused = false
		  AND enabled = true
		RETURNING id
	`, orgID, reason)
	if err != nil {
		return nil, fmt.Errorf("pausing HTTP jobs for org: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if scanErr := rows.Scan(&id); scanErr != nil {
			return nil, fmt.Errorf("scan paused job id: %w", scanErr)
		}
		ids = append(ids, id)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, fmt.Errorf("iterating paused HTTP jobs: %w", rowsErr)
	}
	return ids, nil
}

// UnpauseJobsByPauseReason unpauses jobs that were paused for a specific reason.
// Only affects jobs matching the exact reason; manually paused jobs are preserved.
// Returns the number of jobs unpaused.
func (s *PgStore) UnpauseJobsByPauseReason(ctx context.Context, orgID, reason string) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE jobs SET
			paused = false,
			paused_at = NULL,
			pause_reason = NULL,
			updated_at = NOW()
		WHERE project_id IN (SELECT id FROM projects WHERE org_id = $1 AND deleted_at IS NULL)
		  AND pause_reason = $2
		  AND paused = true
	`, orgID, reason)
	if err != nil {
		return 0, fmt.Errorf("unpausing jobs by reason: %w", err)
	}
	return tag.RowsAffected(), nil
}

// UnpauseJobsByPauseReasonBefore unpauses only jobs paused before a billing
// period boundary so jobs paused again during the new period stay paused.
func (s *PgStore) UnpauseJobsByPauseReasonBefore(ctx context.Context, orgID, reason string, pausedBefore time.Time) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE jobs SET
			paused = false,
			paused_at = NULL,
			pause_reason = NULL,
			updated_at = NOW()
		WHERE project_id IN (SELECT id FROM projects WHERE org_id = $1 AND deleted_at IS NULL)
		  AND pause_reason = $2
		  AND paused = true
		  AND paused_at IS NOT NULL
		  AND paused_at < $3
	`, orgID, reason, pausedBefore.UTC())
	if err != nil {
		return 0, fmt.Errorf("unpausing jobs by reason before boundary: %w", err)
	}
	return tag.RowsAffected(), nil
}

// CountHTTPJobsByOrg returns the number of HTTP-mode jobs (not deleted) for an org.
func (s *PgStore) CountHTTPJobsByOrg(ctx context.Context, orgID string) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM jobs
		WHERE project_id IN (SELECT id FROM projects WHERE org_id = $1 AND deleted_at IS NULL)
		  AND execution_mode = 'http'
	`, orgID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting HTTP jobs for org: %w", err)
	}
	return count, nil
}
