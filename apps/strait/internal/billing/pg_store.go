package billing

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrSubscriptionNotFound = errors.New("organization subscription not found")

// PgStore implements Store with PostgreSQL via pgx.
type PgStore struct {
	pool *pgxpool.Pool
}

// NewPgStore creates a new PostgreSQL billing store.
func NewPgStore(pool *pgxpool.Pool) *PgStore {
	return &PgStore{pool: pool}
}

func (s *PgStore) GetOrgSubscription(ctx context.Context, orgID string) (*OrgSubscription, error) {
	var sub OrgSubscription
	err := s.pool.QueryRow(ctx, `
		SELECT id, org_id, plan_tier, polar_subscription_id, polar_customer_id,
			status, current_period_start, current_period_end,
			spending_limit_microusd, limit_action, pending_plan_tier, canceled_at,
			COALESCE(anomaly_threshold_warning, 3.0),
			COALESCE(anomaly_threshold_critical, 10.0),
			created_at, updated_at
		FROM organization_subscriptions
		WHERE org_id = $1
	`, orgID).Scan(
		&sub.ID, &sub.OrgID, &sub.PlanTier,
		&sub.PolarSubscriptionID, &sub.PolarCustomerID,
		&sub.Status, &sub.CurrentPeriodStart, &sub.CurrentPeriodEnd,
		&sub.SpendingLimitMicrousd, &sub.LimitAction, &sub.PendingPlanTier, &sub.CanceledAt,
		&sub.AnomalyThresholdWarning, &sub.AnomalyThresholdCritical,
		&sub.CreatedAt, &sub.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSubscriptionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting org subscription: %w", err)
	}
	return &sub, nil
}

func (s *PgStore) UpsertOrgSubscription(ctx context.Context, sub *OrgSubscription) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO organization_subscriptions (
			id, org_id, plan_tier, polar_subscription_id, polar_customer_id,
			status, current_period_start, current_period_end,
			spending_limit_microusd, limit_action, canceled_at,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (org_id) DO UPDATE SET
			plan_tier = EXCLUDED.plan_tier,
			polar_subscription_id = EXCLUDED.polar_subscription_id,
			polar_customer_id = EXCLUDED.polar_customer_id,
			status = EXCLUDED.status,
			current_period_start = EXCLUDED.current_period_start,
			current_period_end = EXCLUDED.current_period_end,
			spending_limit_microusd = organization_subscriptions.spending_limit_microusd,
			limit_action = organization_subscriptions.limit_action,
			pending_plan_tier = NULL,
			canceled_at = EXCLUDED.canceled_at,
			updated_at = NOW()
	`, sub.ID, sub.OrgID, sub.PlanTier,
		sub.PolarSubscriptionID, sub.PolarCustomerID,
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
		SELECT id, org_id, plan_tier, polar_subscription_id, polar_customer_id,
			status, current_period_start, current_period_end,
			spending_limit_microusd, limit_action, pending_plan_tier, canceled_at,
			COALESCE(anomaly_threshold_warning, 3.0),
			COALESCE(anomaly_threshold_critical, 10.0),
			created_at, updated_at
		FROM organization_subscriptions
		WHERE pending_plan_tier IS NOT NULL
		  AND current_period_end IS NOT NULL
		  AND current_period_end < NOW()
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
			&sub.PolarSubscriptionID, &sub.PolarCustomerID,
			&sub.Status, &sub.CurrentPeriodStart, &sub.CurrentPeriodEnd,
			&sub.SpendingLimitMicrousd, &sub.LimitAction, &sub.PendingPlanTier, &sub.CanceledAt,
			&sub.AnomalyThresholdWarning, &sub.AnomalyThresholdCritical,
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
		), compute_usage AS (
			SELECT p.org_id,
				rcu.project_id,
				DATE(rcu.created_at) AS period_date,
				0::bigint AS runs_count,
				COALESCE(SUM(rcu.cost_microusd), 0)::bigint AS compute_cost_microusd,
				0::bigint AS ai_tokens_total,
				0::bigint AS ai_cost_microusd,
				MIN(rcu.created_at) AS created_at,
				MAX(rcu.created_at) AS updated_at
			FROM run_compute_usage rcu
			JOIN projects p ON p.id = rcu.project_id
			WHERE p.org_id = $1
			  AND rcu.created_at >= $2
			  AND rcu.created_at < $3
			  AND rcu.status = 'committed'
			GROUP BY p.org_id, rcu.project_id, DATE(rcu.created_at)
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
			SELECT * FROM compute_usage
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
		), compute_usage AS (
			SELECT p.org_id,
				rcu.project_id,
				DATE(rcu.created_at) AS period_date,
				0::bigint AS runs_count,
				COALESCE(SUM(rcu.cost_microusd), 0)::bigint AS compute_cost_microusd,
				0::bigint AS ai_tokens_total,
				0::bigint AS ai_cost_microusd,
				MIN(rcu.created_at) AS created_at,
				MAX(rcu.created_at) AS updated_at
			FROM run_compute_usage rcu
			JOIN projects p ON p.id = rcu.project_id
			WHERE rcu.project_id = $1
			  AND rcu.created_at >= $2
			  AND rcu.created_at < $3
			  AND rcu.status = 'committed'
			GROUP BY p.org_id, rcu.project_id, DATE(rcu.created_at)
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
			SELECT * FROM compute_usage
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
		), compute_usage AS (
			SELECT p.org_id,
				rcu.project_id,
				DATE(rcu.created_at) AS period_date,
				0::bigint AS runs_count,
				COALESCE(SUM(rcu.cost_microusd), 0)::bigint AS compute_cost_microusd,
				0::bigint AS ai_tokens_total,
				0::bigint AS ai_cost_microusd,
				MIN(rcu.created_at) AS created_at,
				MAX(rcu.created_at) AS updated_at
			FROM run_compute_usage rcu
			JOIN projects p ON p.id = rcu.project_id
			WHERE p.org_id = $1
			  AND DATE(rcu.created_at) = $2
			  AND rcu.status = 'committed'
			GROUP BY p.org_id, rcu.project_id, DATE(rcu.created_at)
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
			SELECT * FROM compute_usage
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

func (s *PgStore) SumOrgPeriodSpend(ctx context.Context, orgID string, from time.Time) (int64, error) {
	var total int64
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(rcu.cost_microusd), 0)
		FROM run_compute_usage rcu
		JOIN projects p ON p.id = rcu.project_id
		WHERE p.org_id = $1
		  AND rcu.created_at >= $2
		  AND rcu.status = 'committed'
	`, orgID, from).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("summing org period spend: %w", err)
	}
	return total, nil
}

// ReferralStore implementation on PgStore.

func (s *PgStore) CreateReferral(ctx context.Context, referral *Referral) error {
	err := s.pool.QueryRow(ctx, `
		INSERT INTO referrals (referrer_org_id, referral_code, status, credit_microusd)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at
	`, referral.ReferrerOrgID, referral.ReferralCode, referral.Status, referral.CreditMicrousd,
	).Scan(&referral.ID, &referral.CreatedAt)
	if err != nil {
		return fmt.Errorf("creating referral: %w", err)
	}
	return nil
}

func (s *PgStore) GetReferralByCode(ctx context.Context, code string) (*Referral, error) {
	var r Referral
	var referredOrgID, referredEmail *string
	err := s.pool.QueryRow(ctx, `
		SELECT id, referrer_org_id, referral_code, referred_email, referred_org_id,
			status, credit_microusd, activated_at, created_at
		FROM referrals
		WHERE referral_code = $1
	`, code).Scan(
		&r.ID, &r.ReferrerOrgID, &r.ReferralCode, &referredEmail, &referredOrgID,
		&r.Status, &r.CreditMicrousd, &r.ActivatedAt, &r.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("getting referral by code: %w", err)
	}
	if referredOrgID != nil {
		r.ReferredOrgID = *referredOrgID
	}
	if referredEmail != nil {
		r.ReferredEmail = *referredEmail
	}
	return &r, nil
}

func (s *PgStore) ListReferralsByOrg(ctx context.Context, orgID string) ([]Referral, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, referrer_org_id, referral_code, referred_email, referred_org_id,
			status, credit_microusd, activated_at, created_at
		FROM referrals
		WHERE referrer_org_id = $1
		ORDER BY created_at DESC
	`, orgID)
	if err != nil {
		return nil, fmt.Errorf("listing referrals by org: %w", err)
	}
	defer rows.Close()

	var referrals []Referral
	for rows.Next() {
		var r Referral
		var referredOrgID, referredEmail *string
		if err := rows.Scan(
			&r.ID, &r.ReferrerOrgID, &r.ReferralCode, &referredEmail, &referredOrgID,
			&r.Status, &r.CreditMicrousd, &r.ActivatedAt, &r.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning referral: %w", err)
		}
		if referredOrgID != nil {
			r.ReferredOrgID = *referredOrgID
		}
		if referredEmail != nil {
			r.ReferredEmail = *referredEmail
		}
		referrals = append(referrals, r)
	}
	return referrals, rows.Err()
}

func (s *PgStore) ActivateReferral(ctx context.Context, code string, referredOrgID string) error {
	now := time.Now()
	tag, err := s.pool.Exec(ctx, `
		UPDATE referrals
		SET status = 'activated', referred_org_id = $2, activated_at = $3
		WHERE referral_code = $1 AND status = 'pending'
	`, code, referredOrgID, now)
	if err != nil {
		return fmt.Errorf("activating referral: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("referral not found or already activated")
	}
	return nil
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
		UPDATE project_quotas
		SET monthly_budget_microusd = $2, budget_action = $3
		WHERE project_id = $1
	`, projectID, budgetMicro, action)
	if err != nil {
		return fmt.Errorf("setting project budget: %w", err)
	}
	return nil
}

func (s *PgStore) GetProjectPeriodSpend(ctx context.Context, projectID string, from time.Time) (int64, error) {
	var total int64
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(rcu.cost_microusd), 0)
		FROM run_compute_usage rcu
		WHERE rcu.project_id = $1
		  AND rcu.created_at >= $2
		  AND rcu.status = 'committed'
	`, projectID, from).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("summing project period spend: %w", err)
	}
	return total, nil
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
