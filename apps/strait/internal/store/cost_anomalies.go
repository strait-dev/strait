package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

// CreateCostAnomaly inserts a new cost anomaly row.
func (q *Queries) CreateCostAnomaly(ctx context.Context, anomaly *domain.CostAnomaly) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateCostAnomaly")
	defer span.End()

	query := `
		INSERT INTO agent_cost_anomalies (agent_id, project_id, daily_cost_microusd, baseline_avg_microusd, multiplier, threshold, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, detected_at`

	err := q.db.QueryRow(
		ctx,
		query,
		anomaly.AgentID,
		anomaly.ProjectID,
		anomaly.DailyCostMicrousd,
		anomaly.BaselineAvgMicrousd,
		anomaly.Multiplier,
		anomaly.Threshold,
		anomaly.Status,
	).Scan(&anomaly.ID, &anomaly.DetectedAt)
	if err != nil {
		return fmt.Errorf("create cost anomaly: %w", err)
	}
	return nil
}

// ListCostAnomalies returns anomalies for an agent, most recent first.
func (q *Queries) ListCostAnomalies(ctx context.Context, agentID string, limit int) ([]domain.CostAnomaly, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListCostAnomalies")
	defer span.End()

	if limit <= 0 {
		limit = 50
	}

	query := `
		SELECT id, agent_id, project_id, detected_at, daily_cost_microusd,
		       baseline_avg_microusd, multiplier, threshold, status,
		       resolved_at, snoozed_until
		FROM agent_cost_anomalies
		WHERE agent_id = $1
		ORDER BY detected_at DESC
		LIMIT $2`

	rows, err := q.db.Query(ctx, query, agentID, limit)
	if err != nil {
		return nil, fmt.Errorf("list cost anomalies: %w", err)
	}
	defer rows.Close()

	var result []domain.CostAnomaly
	for rows.Next() {
		var a domain.CostAnomaly
		if err := rows.Scan(
			&a.ID, &a.AgentID, &a.ProjectID, &a.DetectedAt,
			&a.DailyCostMicrousd, &a.BaselineAvgMicrousd,
			&a.Multiplier, &a.Threshold, &a.Status,
			&a.ResolvedAt, &a.SnoozedUntil,
		); err != nil {
			return nil, fmt.Errorf("scan cost anomaly: %w", err)
		}
		result = append(result, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list cost anomalies rows: %w", err)
	}
	return result, nil
}

// UpdateCostAnomalyStatus updates the status of an anomaly, setting
// resolved_at when status becomes "resolved" and snoozed_until when "snoozed".
func (q *Queries) UpdateCostAnomalyStatus(ctx context.Context, id, status string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpdateCostAnomalyStatus")
	defer span.End()

	var query string
	switch status {
	case "resolved":
		query = `UPDATE agent_cost_anomalies SET status = $1, resolved_at = NOW() WHERE id = $2`
	case "snoozed":
		query = `UPDATE agent_cost_anomalies SET status = $1, snoozed_until = NOW() + INTERVAL '24 hours' WHERE id = $2`
	default:
		query = `UPDATE agent_cost_anomalies SET status = $1 WHERE id = $2`
	}

	tag, err := q.db.Exec(ctx, query, status, id)
	if err != nil {
		return fmt.Errorf("update cost anomaly status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("cost anomaly not found: %s", id)
	}
	return nil
}

// GetOpenAnomalyForAgent returns the most recent open anomaly for an agent, if any.
func (q *Queries) GetOpenAnomalyForAgent(ctx context.Context, agentID string) (*domain.CostAnomaly, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetOpenAnomalyForAgent")
	defer span.End()

	query := `
		SELECT id, agent_id, project_id, detected_at, daily_cost_microusd,
		       baseline_avg_microusd, multiplier, threshold, status,
		       resolved_at, snoozed_until
		FROM agent_cost_anomalies
		WHERE agent_id = $1 AND status = 'open'
		ORDER BY detected_at DESC
		LIMIT 1`

	var a domain.CostAnomaly
	err := q.db.QueryRow(ctx, query, agentID).Scan(
		&a.ID, &a.AgentID, &a.ProjectID, &a.DetectedAt,
		&a.DailyCostMicrousd, &a.BaselineAvgMicrousd,
		&a.Multiplier, &a.Threshold, &a.Status,
		&a.ResolvedAt, &a.SnoozedUntil,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get open anomaly for agent: %w", err)
	}
	return &a, nil
}

// SnoozeAnomaly sets an anomaly status to snoozed with the given expiration.
func (q *Queries) SnoozeAnomaly(ctx context.Context, id string, until time.Time) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.SnoozeAnomaly")
	defer span.End()

	query := `UPDATE agent_cost_anomalies SET status = 'snoozed', snoozed_until = $1 WHERE id = $2`

	tag, err := q.db.Exec(ctx, query, until, id)
	if err != nil {
		return fmt.Errorf("snooze anomaly: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("cost anomaly not found: %s", id)
	}
	return nil
}
