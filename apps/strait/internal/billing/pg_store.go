package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// unmarshalJSON is a thin wrapper so callers don't need to import encoding/json.
func unmarshalJSON(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

var ErrSubscriptionNotFound = errors.New("organization subscription not found")

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
	_, err := s.pool.Exec(ctx,
		`INSERT INTO organization_subscriptions (id, org_id, plan_tier, status)
		 VALUES (gen_random_uuid()::text, $1, 'free', 'active')
		 ON CONFLICT (org_id) DO NOTHING`,
		orgID)
	if err != nil {
		return fmt.Errorf("ensure org subscription: %w", err)
	}
	return nil
}

func (s *PgStore) GetOrgSubscription(ctx context.Context, orgID string) (*OrgSubscription, error) {
	var sub OrgSubscription
	var addOnsJSON []byte
	err := s.pool.QueryRow(ctx, `
		SELECT id, org_id, plan_tier, stripe_subscription_id, stripe_customer_id,
			status, current_period_start, current_period_end,
			spending_limit_microusd, limit_action, pending_plan_tier, canceled_at,
			COALESCE(anomaly_threshold_warning, 3.0),
			COALESCE(anomaly_threshold_critical, 10.0),
			grace_period_end, COALESCE(payment_status, 'ok'),
			override_daily_run_limit, override_concurrent_run_limit,
			COALESCE(enforcement_mode, 'enforce'),
			COALESCE(monthly_usage_email, false),
			COALESCE(add_ons, '{}'::jsonb),
			created_at, updated_at
		FROM organization_subscriptions
		WHERE org_id = $1
	`, orgID).Scan(
		&sub.ID, &sub.OrgID, &sub.PlanTier,
		&sub.StripeSubscriptionID, &sub.StripeCustomerID,
		&sub.Status, &sub.CurrentPeriodStart, &sub.CurrentPeriodEnd,
		&sub.SpendingLimitMicrousd, &sub.LimitAction, &sub.PendingPlanTier, &sub.CanceledAt,
		&sub.AnomalyThresholdWarning, &sub.AnomalyThresholdCritical,
		&sub.GracePeriodEnd, &sub.PaymentStatus,
		&sub.OverrideDailyRunLimit, &sub.OverrideConcurrentRunLimit,
		&sub.EnforcementMode,
		&sub.MonthlyUsageEmail,
		&addOnsJSON,
		&sub.CreatedAt, &sub.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSubscriptionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting org subscription: %w", err)
	}
	if len(addOnsJSON) > 0 {
		if jsonErr := unmarshalJSON(addOnsJSON, &sub.AddOns); jsonErr != nil {
			return nil, fmt.Errorf("unmarshalling add_ons for org %s: %w", orgID, jsonErr)
		}
	}
	return &sub, nil
}

func (s *PgStore) UpsertOrgSubscription(ctx context.Context, sub *OrgSubscription) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO organization_subscriptions (
			id, org_id, plan_tier, stripe_subscription_id, stripe_customer_id,
			status, current_period_start, current_period_end,
			spending_limit_microusd, limit_action, canceled_at,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (org_id) DO UPDATE SET
			plan_tier = EXCLUDED.plan_tier,
			stripe_subscription_id = EXCLUDED.stripe_subscription_id,
			stripe_customer_id = EXCLUDED.stripe_customer_id,
			status = EXCLUDED.status,
			current_period_start = EXCLUDED.current_period_start,
			current_period_end = EXCLUDED.current_period_end,
			spending_limit_microusd = organization_subscriptions.spending_limit_microusd,
			limit_action = organization_subscriptions.limit_action,
			pending_plan_tier = COALESCE(organization_subscriptions.pending_plan_tier, NULL),
			canceled_at = EXCLUDED.canceled_at,
			updated_at = NOW()
	`, sub.ID, sub.OrgID, sub.PlanTier,
		sub.StripeSubscriptionID, sub.StripeCustomerID,
		sub.Status, sub.CurrentPeriodStart, sub.CurrentPeriodEnd,
		sub.SpendingLimitMicrousd, sub.LimitAction, sub.CanceledAt,
		sub.CreatedAt, sub.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("upserting org subscription: %w", err)
	}
	return nil
}

func (s *PgStore) UpdateOrgSubscriptionPlan(ctx context.Context, orgID, planTier, status string) error {
	tag, err := s.pool.Exec(ctx, `
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
	return nil
}

func (s *PgStore) UpdateOrgSubscriptionFull(ctx context.Context, orgID, planTier, status string, periodStart, periodEnd *time.Time) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE organization_subscriptions
		SET plan_tier = $2, status = $3,
			current_period_start = COALESCE($4, current_period_start),
			current_period_end = COALESCE($5, current_period_end),
			updated_at = NOW()
		WHERE org_id = $1
	`, orgID, planTier, status, periodStart, periodEnd)
	if err != nil {
		return fmt.Errorf("updating org subscription full: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrSubscriptionNotFound
	}
	return nil
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
	tag, err := s.pool.Exec(ctx, `
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
	return nil
}

func (s *PgStore) ListOrgsWithPendingDowngrade(ctx context.Context) ([]OrgSubscription, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, org_id, plan_tier, stripe_subscription_id, stripe_customer_id,
			status, current_period_start, current_period_end,
			spending_limit_microusd, limit_action, pending_plan_tier, canceled_at,
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
			&sub.SpendingLimitMicrousd, &sub.LimitAction, &sub.PendingPlanTier, &sub.CanceledAt,
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
		FROM job_runs jr
		JOIN projects p ON p.id = jr.project_id
		WHERE p.org_id = $1
		  AND jr.status = 'executing'
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
		SELECT p.org_id, COUNT(jr.id)::int
		FROM job_runs jr
		JOIN projects p ON p.id = jr.project_id
		WHERE p.org_id = ANY($1)
		  AND jr.status = 'executing'
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

func (s *PgStore) CountAIModelCallsByOrg(ctx context.Context, orgID string, from, to time.Time) (int64, error) {
	var count int64
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)::bigint
		FROM run_usage ru
		JOIN job_runs jr ON jr.id = ru.run_id
		JOIN projects p ON p.id = jr.project_id
		WHERE p.org_id = $1
		  AND ru.created_at >= $2
		  AND ru.created_at < $3
	`, orgID, from, to).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting ai model calls by org: %w", err)
	}
	return count, nil
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
	_, err := s.pool.Exec(ctx, `
		INSERT INTO usage_records (
			id, org_id, project_id, period_date,
			runs_count, compute_cost_microusd, ai_tokens_total, ai_cost_microusd,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (org_id, project_id, period_date) DO UPDATE SET
			runs_count = usage_records.runs_count + EXCLUDED.runs_count,
			compute_cost_microusd = usage_records.compute_cost_microusd + EXCLUDED.compute_cost_microusd,
			ai_tokens_total = usage_records.ai_tokens_total + EXCLUDED.ai_tokens_total,
			ai_cost_microusd = usage_records.ai_cost_microusd + EXCLUDED.ai_cost_microusd,
			updated_at = NOW()
	`, rec.ID, rec.OrgID, rec.ProjectID, rec.PeriodDate,
		rec.RunsCount, rec.ComputeCostMicro, rec.AITokensTotal, rec.AICostMicro,
		rec.CreatedAt, rec.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("upserting usage record: %w", err)
	}
	return nil
}

func (s *PgStore) GetOrgUsageForPeriod(ctx context.Context, orgID string, from, to time.Time) ([]UsageRecord, error) {
	endExclusive := to.AddDate(0, 0, 1)

	rows, err := s.pool.Query(ctx, `
		WITH run_counts AS (
			SELECT p.org_id,
				jr.project_id,
				DATE(jr.created_at) AS period_date,
				COUNT(*)::bigint AS runs_count,
				0::bigint AS compute_cost_microusd,
				0::bigint AS ai_tokens_total,
				0::bigint AS ai_cost_microusd,
				MIN(jr.created_at) AS created_at,
				MAX(jr.created_at) AS updated_at
			FROM job_runs jr
			JOIN projects p ON p.id = jr.project_id
			WHERE p.org_id = $1
			  AND jr.created_at >= $2
			  AND jr.created_at < $3
			GROUP BY p.org_id, jr.project_id, DATE(jr.created_at)
		), ai_usage AS (
			SELECT p.org_id,
				jr.project_id,
				DATE(ru.created_at) AS period_date,
				0::bigint AS runs_count,
				0::bigint AS compute_cost_microusd,
				COALESCE(SUM(ru.total_tokens), 0)::bigint AS ai_tokens_total,
				COALESCE(SUM(ru.cost_microusd), 0)::bigint AS ai_cost_microusd,
				MIN(ru.created_at) AS created_at,
				MAX(ru.created_at) AS updated_at
			FROM run_usage ru
			JOIN job_runs jr ON jr.id = ru.run_id
			JOIN projects p ON p.id = jr.project_id
			WHERE p.org_id = $1
			  AND ru.created_at >= $2
			  AND ru.created_at < $3
			GROUP BY p.org_id, jr.project_id, DATE(ru.created_at)
		)
		SELECT '' AS id,
			org_id,
			project_id,
			period_date,
			SUM(runs_count) AS runs_count,
			SUM(compute_cost_microusd) AS compute_cost_microusd,
			SUM(ai_tokens_total) AS ai_tokens_total,
			SUM(ai_cost_microusd) AS ai_cost_microusd,
			MIN(created_at) AS created_at,
			MAX(updated_at) AS updated_at
		FROM (
			SELECT * FROM run_counts
			UNION ALL
			SELECT * FROM ai_usage
		) usage
		GROUP BY org_id, project_id, period_date
		ORDER BY period_date ASC
	`, orgID, from, endExclusive)
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
				0::bigint AS ai_tokens_total,
				0::bigint AS ai_cost_microusd,
				MIN(jr.created_at) AS created_at,
				MAX(jr.created_at) AS updated_at
			FROM job_runs jr
			JOIN projects p ON p.id = jr.project_id
			WHERE jr.project_id = $1
			  AND jr.created_at >= $2
			  AND jr.created_at < $3
			GROUP BY p.org_id, jr.project_id, DATE(jr.created_at)
		), ai_usage AS (
			SELECT p.org_id,
				jr.project_id,
				DATE(ru.created_at) AS period_date,
				0::bigint AS runs_count,
				0::bigint AS compute_cost_microusd,
				COALESCE(SUM(ru.total_tokens), 0)::bigint AS ai_tokens_total,
				COALESCE(SUM(ru.cost_microusd), 0)::bigint AS ai_cost_microusd,
				MIN(ru.created_at) AS created_at,
				MAX(ru.created_at) AS updated_at
			FROM run_usage ru
			JOIN job_runs jr ON jr.id = ru.run_id
			JOIN projects p ON p.id = jr.project_id
			WHERE jr.project_id = $1
			  AND ru.created_at >= $2
			  AND ru.created_at < $3
			GROUP BY p.org_id, jr.project_id, DATE(ru.created_at)
		)
		SELECT '' AS id,
			org_id,
			project_id,
			period_date,
			SUM(runs_count) AS runs_count,
			SUM(compute_cost_microusd) AS compute_cost_microusd,
			SUM(ai_tokens_total) AS ai_tokens_total,
			SUM(ai_cost_microusd) AS ai_cost_microusd,
			MIN(created_at) AS created_at,
			MAX(updated_at) AS updated_at
		FROM (
			SELECT * FROM run_counts
			UNION ALL
			SELECT * FROM ai_usage
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
				0::bigint AS ai_tokens_total,
				0::bigint AS ai_cost_microusd,
				MIN(jr.created_at) AS created_at,
				MAX(jr.created_at) AS updated_at
			FROM job_runs jr
			JOIN projects p ON p.id = jr.project_id
			WHERE p.org_id = $1
			  AND DATE(jr.created_at) = $2
			GROUP BY p.org_id, jr.project_id, DATE(jr.created_at)
		), ai_usage AS (
			SELECT p.org_id,
				jr.project_id,
				DATE(ru.created_at) AS period_date,
				0::bigint AS runs_count,
				0::bigint AS compute_cost_microusd,
				COALESCE(SUM(ru.total_tokens), 0)::bigint AS ai_tokens_total,
				COALESCE(SUM(ru.cost_microusd), 0)::bigint AS ai_cost_microusd,
				MIN(ru.created_at) AS created_at,
				MAX(ru.created_at) AS updated_at
			FROM run_usage ru
			JOIN job_runs jr ON jr.id = ru.run_id
			JOIN projects p ON p.id = jr.project_id
			WHERE p.org_id = $1
			  AND DATE(ru.created_at) = $2
			GROUP BY p.org_id, jr.project_id, DATE(ru.created_at)
		)
		SELECT '' AS id,
			org_id,
			project_id,
			period_date,
			SUM(runs_count) AS runs_count,
			SUM(compute_cost_microusd) AS compute_cost_microusd,
			SUM(ai_tokens_total) AS ai_tokens_total,
			SUM(ai_cost_microusd) AS ai_cost_microusd,
			MIN(created_at) AS created_at,
			MAX(updated_at) AS updated_at
		FROM (
			SELECT * FROM run_counts
			UNION ALL
			SELECT * FROM ai_usage
		) usage
		GROUP BY org_id, project_id, period_date
	`, orgID, date)
	if err != nil {
		return nil, fmt.Errorf("getting org daily usage: %w", err)
	}
	defer rows.Close()
	return scanUsageRecords(rows)
}

func (s *PgStore) SumOrgPeriodSpend(_ context.Context, orgID string, from time.Time) (int64, error) {
	// run_compute_usage was dropped in migration 000227. Compute spend is now
	// tracked via usage_records (flat per-run cost). Return 0 until the new
	// usage_records aggregation is wired in.
	_ = orgID
	_ = from
	return 0, nil
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

func (s *PgStore) GetProjectPeriodSpend(_ context.Context, projectID string, from time.Time) (int64, error) {
	// run_compute_usage was dropped in migration 000227. Return 0 until the new
	// per-run cost aggregation is wired in.
	_ = projectID
	_ = from
	return 0, nil
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
	rows, err := s.pool.Query(ctx, `
		SELECT org_id
		FROM organization_subscriptions
		WHERE status = 'active'
		ORDER BY org_id
		LIMIT 50000
	`)
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

func (s *PgStore) UpdatePaymentStatus(ctx context.Context, orgID string, status string, graceEnd *time.Time) error {
	tag, err := s.pool.Exec(ctx, `
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
	return nil
}

func (s *PgStore) ListOrgsInGracePeriod(ctx context.Context) ([]OrgSubscription, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, org_id, plan_tier, stripe_subscription_id, stripe_customer_id,
			status, current_period_start, current_period_end,
			spending_limit_microusd, limit_action, pending_plan_tier, canceled_at,
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
			&sub.SpendingLimitMicrousd, &sub.LimitAction, &sub.PendingPlanTier, &sub.CanceledAt,
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
			spending_limit_microusd, limit_action, pending_plan_tier, canceled_at,
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
			&sub.SpendingLimitMicrousd, &sub.LimitAction, &sub.PendingPlanTier, &sub.CanceledAt,
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
			&r.RunsCount, &r.ComputeCostMicro, &r.AITokensTotal, &r.AICostMicro,
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
		INSERT INTO sent_usage_reports (org_id, period_end)
		VALUES ($1, $2)
		ON CONFLICT (org_id, period_end) DO NOTHING
	`, orgID, periodEnd.Truncate(24*time.Hour))
	if err != nil {
		return fmt.Errorf("recording sent usage report: %w", err)
	}
	return nil
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

// DeactivateExcessCronJobs disables cron jobs beyond the given limit for an org.
// Keeps the most recently updated jobs; clears cron on the oldest excess.
func (s *PgStore) DeactivateExcessCronJobs(ctx context.Context, orgID string, maxSchedules int) (int64, error) {
	result, err := s.pool.Exec(ctx, `
		UPDATE jobs SET cron = '', updated_at = NOW()
		WHERE id IN (
			SELECT j.id FROM jobs j
			WHERE j.project_id IN (SELECT id FROM projects WHERE org_id = $1 AND deleted_at IS NULL)
			  AND j.cron IS NOT NULL AND j.cron != ''
			ORDER BY j.updated_at DESC
			OFFSET $2
		)
	`, orgID, maxSchedules)
	if err != nil {
		return 0, fmt.Errorf("deactivate excess cron jobs: %w", err)
	}
	return result.RowsAffected(), nil
}

// DeactivateExcessEnvironments marks excess environments as deleted for an org.
// Keeps the most recently created environments; deactivates the oldest excess.
func (s *PgStore) DeactivateExcessEnvironments(ctx context.Context, orgID string, maxEnvironments int) (int64, error) {
	result, err := s.pool.Exec(ctx, `
		DELETE FROM environments
		WHERE id IN (
			SELECT e.id FROM environments e
			WHERE e.project_id IN (SELECT id FROM projects WHERE org_id = $1 AND deleted_at IS NULL)
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

// ListActiveAddons returns all active add-ons for an organization.
func (s *PgStore) ListActiveAddons(ctx context.Context, orgID string) ([]Addon, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, org_id, addon_type, quantity, stripe_subscription_id, active, expires_at, created_at, updated_at
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
		if err := rows.Scan(&a.ID, &a.OrgID, &a.AddonType, &a.Quantity, &a.StripeSubscriptionID, &a.Active, &a.ExpiresAt, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning addon row: %w", err)
		}
		addons = append(addons, a)
	}
	return addons, rows.Err()
}

// CreateAddon inserts a new add-on record.
func (s *PgStore) CreateAddon(ctx context.Context, addon *Addon) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO organization_addons (id, org_id, addon_type, quantity, stripe_subscription_id, active, expires_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())
	`, addon.ID, addon.OrgID, addon.AddonType, addon.Quantity, addon.StripeSubscriptionID, addon.Active, addon.ExpiresAt)
	if err != nil {
		return fmt.Errorf("creating addon: %w", err)
	}
	return nil
}

// DeactivateAddon sets an add-on to inactive.
func (s *PgStore) DeactivateAddon(ctx context.Context, addonID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE organization_addons SET active = false, updated_at = NOW() WHERE id = $1
	`, addonID)
	if err != nil {
		return fmt.Errorf("deactivating addon: %w", err)
	}
	return nil
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
			included_credit_microusd, compute_discount_pct,
			contract_start_date, contract_end_date,
			auto_renew, billing_cadence, stripe_subscription_id,
			notes, created_at, updated_at
		FROM enterprise_contracts
		WHERE org_id = $1
	`, orgID).Scan(
		&c.ID, &c.OrgID, &c.EnterpriseTier,
		&c.AnnualCommitmentCents, &c.IncludedCreditMicrousd, &c.ComputeDiscountPct,
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
			included_credit_microusd, compute_discount_pct,
			contract_start_date, contract_end_date,
			auto_renew, billing_cadence, stripe_subscription_id,
			notes, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT (org_id) DO UPDATE SET
			enterprise_tier = EXCLUDED.enterprise_tier,
			annual_commitment_cents = EXCLUDED.annual_commitment_cents,
			included_credit_microusd = EXCLUDED.included_credit_microusd,
			compute_discount_pct = EXCLUDED.compute_discount_pct,
			contract_start_date = EXCLUDED.contract_start_date,
			contract_end_date = EXCLUDED.contract_end_date,
			auto_renew = EXCLUDED.auto_renew,
			billing_cadence = EXCLUDED.billing_cadence,
			stripe_subscription_id = EXCLUDED.stripe_subscription_id,
			notes = EXCLUDED.notes,
			updated_at = NOW()
	`, c.ID, c.OrgID, c.EnterpriseTier,
		c.AnnualCommitmentCents, c.IncludedCreditMicrousd, c.ComputeDiscountPct,
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
			included_credit_microusd, compute_discount_pct,
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
			&c.AnnualCommitmentCents, &c.IncludedCreditMicrousd, &c.ComputeDiscountPct,
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

// PauseHTTPJobsByOrg pauses all active HTTP-mode jobs for an org.
// Sets the pause reason so they can be selectively unpaused on upgrade.
// Returns the number of jobs paused.
func (s *PgStore) PauseHTTPJobsByOrg(ctx context.Context, orgID, reason string) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE jobs SET
			paused = true,
			paused_at = NOW(),
			pause_reason = $2,
			updated_at = NOW()
		WHERE project_id IN (SELECT id FROM projects WHERE org_id = $1 AND deleted_at IS NULL)
		  AND execution_mode = 'http'
		  AND paused = false
		  AND enabled = true
	`, orgID, reason)
	if err != nil {
		return 0, fmt.Errorf("pausing HTTP jobs for org: %w", err)
	}
	return tag.RowsAffected(), nil
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
