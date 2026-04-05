package store

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"strait/internal/dbscan"
	"strait/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

func (q *Queries) UpsertNotifyPolicyOverride(ctx context.Context, policy *domain.NotifyPolicyOverride) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpsertNotifyPolicyOverride")
	defer span.End()

	if err := policy.Validate(); err != nil {
		return fmt.Errorf("validate notify policy override: %w", err)
	}
	if policy.ID == "" {
		policy.ID = uuid.Must(uuid.NewV7()).String()
	}
	query := `
		INSERT INTO notify_policy_overrides (
			id, project_id, scope_type, scope_key, channel,
			digest_policy, retry_max_attempts, retry_base_delay_secs, retry_max_delay_secs,
			escalation_tiers, escalation_min_interval_secs, enabled
		)
		VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9,
			$10, $11, $12
		)
		ON CONFLICT (project_id, scope_type, scope_key, channel)
		DO UPDATE SET
			digest_policy = EXCLUDED.digest_policy,
			retry_max_attempts = EXCLUDED.retry_max_attempts,
			retry_base_delay_secs = EXCLUDED.retry_base_delay_secs,
			retry_max_delay_secs = EXCLUDED.retry_max_delay_secs,
			escalation_tiers = EXCLUDED.escalation_tiers,
			escalation_min_interval_secs = EXCLUDED.escalation_min_interval_secs,
			enabled = EXCLUDED.enabled,
			updated_at = NOW()
		RETURNING id, created_at, updated_at`

	if err := q.db.QueryRow(ctx, query,
		policy.ID,
		policy.ProjectID,
		policy.ScopeType,
		policy.ScopeKey,
		policy.Channel,
		dbscan.NilIfEmptyString(policy.DigestPolicy),
		policy.RetryMaxAttempts,
		policy.RetryBaseDelaySecs,
		policy.RetryMaxDelaySecs,
		policy.EscalationTiers,
		policy.EscalationMinIntervalSecs,
		policy.Enabled,
	).Scan(&policy.ID, &policy.CreatedAt, &policy.UpdatedAt); err != nil {
		return fmt.Errorf("upsert notify policy override: %w", err)
	}

	return nil
}

func (q *Queries) GetNotifyPolicyOverride(ctx context.Context, id, projectID string) (*domain.NotifyPolicyOverride, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetNotifyPolicyOverride")
	defer span.End()

	query := `
		SELECT id, project_id, scope_type, scope_key, channel, digest_policy,
		       retry_max_attempts, retry_base_delay_secs, retry_max_delay_secs,
		       escalation_tiers, escalation_min_interval_secs, enabled, created_at, updated_at
		FROM notify_policy_overrides
		WHERE id = $1 AND project_id = $2`

	policy, err := scanNotifyPolicyOverride(q.db.QueryRow(ctx, query, id, projectID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotifyPolicyNotFound
		}
		return nil, fmt.Errorf("get notify policy override: %w", err)
	}

	return policy, nil
}

func (q *Queries) ListNotifyPolicyOverrides(ctx context.Context, projectID string, scopeType *string) ([]domain.NotifyPolicyOverride, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListNotifyPolicyOverrides")
	defer span.End()

	query := `
		SELECT id, project_id, scope_type, scope_key, channel, digest_policy,
		       retry_max_attempts, retry_base_delay_secs, retry_max_delay_secs,
		       escalation_tiers, escalation_min_interval_secs, enabled, created_at, updated_at
		FROM notify_policy_overrides
		WHERE project_id = $1
		  AND ($2::text IS NULL OR scope_type = $2)
		ORDER BY scope_type ASC, scope_key ASC, created_at DESC`

	var scopeValue any
	if scopeType != nil && *scopeType != "" {
		scopeValue = *scopeType
	}

	rows, err := q.db.Query(ctx, query, projectID, scopeValue)
	if err != nil {
		return nil, fmt.Errorf("list notify policy overrides: %w", err)
	}
	defer rows.Close()

	policies := make([]domain.NotifyPolicyOverride, 0, 8)
	for rows.Next() {
		policy, scanErr := scanNotifyPolicyOverride(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list notify policy overrides scan: %w", scanErr)
		}
		policies = append(policies, *policy)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list notify policy overrides rows: %w", err)
	}
	return policies, nil
}

func (q *Queries) ResolveNotifyPolicyOverride(ctx context.Context, projectID, stepRunID, categoryKey, channel string) (*domain.NotifyPolicyOverride, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ResolveNotifyPolicyOverride")
	defer span.End()

	candidates, err := q.ListNotifyPolicyOverrides(ctx, projectID, nil)
	if err != nil {
		return nil, fmt.Errorf("resolve notify policy override candidates: %w", err)
	}

	matched := filterMatchingNotifyPolicies(candidates, stepRunID, categoryKey, channel)
	if len(matched) == 0 {
		return nil, ErrNotifyPolicyNotFound
	}
	sort.SliceStable(matched, func(i, j int) bool {
		return notifyPolicyRank(matched[i], channel) < notifyPolicyRank(matched[j], channel)
	})
	selected := matched[0]
	return &selected, nil
}

func (q *Queries) DeleteNotifyPolicyOverride(ctx context.Context, id, projectID string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteNotifyPolicyOverride")
	defer span.End()

	tag, err := q.db.Exec(ctx, `DELETE FROM notify_policy_overrides WHERE id = $1 AND project_id = $2`, id, projectID)
	if err != nil {
		return fmt.Errorf("delete notify policy override: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotifyPolicyNotFound
	}
	return nil
}

func filterMatchingNotifyPolicies(candidates []domain.NotifyPolicyOverride, stepRunID, categoryKey, channel string) []domain.NotifyPolicyOverride {
	matched := make([]domain.NotifyPolicyOverride, 0, len(candidates))
	for _, candidate := range candidates {
		if !candidate.Enabled {
			continue
		}
		if candidate.Channel != "" && candidate.Channel != channel {
			continue
		}
		switch candidate.ScopeType {
		case domain.NotifyPolicyScopeWorkflowStep:
			if stepRunID == "" || candidate.ScopeKey != stepRunID {
				continue
			}
		case domain.NotifyPolicyScopeCategory:
			if categoryKey == "" || candidate.ScopeKey != categoryKey {
				continue
			}
		case domain.NotifyPolicyScopeProject:
			if candidate.ScopeKey != "*" {
				continue
			}
		default:
			continue
		}
		matched = append(matched, candidate)
	}
	return matched
}

func notifyPolicyRank(policy domain.NotifyPolicyOverride, requestedChannel string) int {
	rank := 100
	switch policy.ScopeType {
	case domain.NotifyPolicyScopeWorkflowStep:
		rank = 10
	case domain.NotifyPolicyScopeCategory:
		rank = 20
	case domain.NotifyPolicyScopeProject:
		rank = 30
	}
	if requestedChannel != "" {
		switch policy.Channel {
		case requestedChannel:
			rank -= 3
		case "":
			rank += 2
		}
	}
	return rank
}

func scanNotifyPolicyOverride(scanner scanTarget) (*domain.NotifyPolicyOverride, error) {
	var policy domain.NotifyPolicyOverride
	var channel *string
	var digestPolicy *string

	err := scanner.Scan(
		&policy.ID,
		&policy.ProjectID,
		&policy.ScopeType,
		&policy.ScopeKey,
		&channel,
		&digestPolicy,
		&policy.RetryMaxAttempts,
		&policy.RetryBaseDelaySecs,
		&policy.RetryMaxDelaySecs,
		&policy.EscalationTiers,
		&policy.EscalationMinIntervalSecs,
		&policy.Enabled,
		&policy.CreatedAt,
		&policy.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if channel != nil {
		policy.Channel = *channel
	}
	if digestPolicy != nil {
		policy.DigestPolicy = *digestPolicy
	}
	return &policy, nil
}
