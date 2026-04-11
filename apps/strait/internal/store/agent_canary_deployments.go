package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"strait/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

// CreateAgentCanaryDeployment inserts a new canary deployment for an agent.
// Only one active canary per agent is allowed (enforced by unique partial index).
func (q *Queries) CreateAgentCanaryDeployment(ctx context.Context, canary *domain.AgentCanaryDeployment) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateAgentCanaryDeployment")
	defer span.End()

	if canary.ID == "" {
		canary.ID = uuid.Must(uuid.NewV7()).String()
	}

	err := q.db.QueryRow(ctx, `
		INSERT INTO agent_canary_deployments (
			id, agent_id, project_id, source_deployment_id, target_deployment_id,
			traffic_pct, status
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING created_at, updated_at
	`,
		canary.ID, canary.AgentID, canary.ProjectID,
		canary.SourceDeploymentID, canary.TargetDeploymentID,
		canary.TrafficPct, canary.Status,
	).Scan(&canary.CreatedAt, &canary.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create agent canary deployment: %w", err)
	}
	return nil
}

// GetActiveAgentCanary returns the active canary deployment for an agent, or nil if none.
func (q *Queries) GetActiveAgentCanary(ctx context.Context, agentID string) (*domain.AgentCanaryDeployment, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetActiveAgentCanary")
	defer span.End()

	var c domain.AgentCanaryDeployment
	err := q.db.QueryRow(ctx, `
		SELECT id, agent_id, project_id, source_deployment_id, target_deployment_id,
			traffic_pct, status, created_at, updated_at, completed_at
		FROM agent_canary_deployments
		WHERE agent_id = $1 AND status = 'active'
		LIMIT 1
	`, agentID).Scan(
		&c.ID, &c.AgentID, &c.ProjectID,
		&c.SourceDeploymentID, &c.TargetDeploymentID,
		&c.TrafficPct, &c.Status,
		&c.CreatedAt, &c.UpdatedAt, &c.CompletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get active agent canary: %w", err)
	}
	return &c, nil
}

// GetActiveAgentCanaryWithTarget returns the active canary deployment
// for an agent joined with its target AgentDeployment row in a single
// query. This collapses the per-RunAgent N+1 where the service used to
// call GetActiveAgentCanary followed by GetAgentDeploymentByID. Returns
// (nil, nil, nil) when no active canary exists for the agent.
//
// The join is INNER: a canary row references a real deployment via FK,
// so a missing target is a schema bug, not a no-rows case.
func (q *Queries) GetActiveAgentCanaryWithTarget(ctx context.Context, agentID string) (*domain.AgentCanaryDeployment, *domain.AgentDeployment, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetActiveAgentCanaryWithTarget")
	defer span.End()

	var c domain.AgentCanaryDeployment
	var d domain.AgentDeployment
	var status string
	var createdBy *string
	var environmentID *string
	err := q.db.QueryRow(ctx, `
		SELECT
			c.id, c.agent_id, c.project_id, c.source_deployment_id, c.target_deployment_id,
			c.traffic_pct, c.status, c.created_at, c.updated_at, c.completed_at,
			d.id, d.agent_id, d.environment_id, d.version, d.status, d.provider,
			d.config_snapshot, d.provider_metadata, d.created_by,
			d.created_at, d.updated_at, d.deployed_at
		FROM agent_canary_deployments c
		JOIN agent_deployments d ON d.id = c.target_deployment_id
		WHERE c.agent_id = $1 AND c.status = 'active'
		LIMIT 1
	`, agentID).Scan(
		&c.ID, &c.AgentID, &c.ProjectID,
		&c.SourceDeploymentID, &c.TargetDeploymentID,
		&c.TrafficPct, &c.Status,
		&c.CreatedAt, &c.UpdatedAt, &c.CompletedAt,
		&d.ID, &d.AgentID, &environmentID, &d.Version, &status, &d.Provider,
		&d.ConfigSnapshot, &d.ProviderMetadata, &createdBy,
		&d.CreatedAt, &d.UpdatedAt, &d.DeployedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("get active agent canary with target: %w", err)
	}
	d.Status = domain.AgentDeploymentStatus(status)
	if createdBy != nil {
		d.CreatedBy = *createdBy
	}
	if environmentID != nil {
		d.EnvironmentID = *environmentID
	}
	return &c, &d, nil
}

// UpdateAgentCanaryTraffic sets the traffic percentage for the active canary.
func (q *Queries) UpdateAgentCanaryTraffic(ctx context.Context, agentID string, trafficPct int) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpdateAgentCanaryTraffic")
	defer span.End()

	_, err := q.db.Exec(ctx, `
		UPDATE agent_canary_deployments
		SET traffic_pct = $2, updated_at = NOW()
		WHERE agent_id = $1 AND status = 'active'
	`, agentID, trafficPct)
	if err != nil {
		return fmt.Errorf("update agent canary traffic: %w", err)
	}
	return nil
}

// CompleteAgentCanary marks the active canary as completed or rolled back.
func (q *Queries) CompleteAgentCanary(ctx context.Context, agentID, status string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CompleteAgentCanary")
	defer span.End()

	now := time.Now()
	_, err := q.db.Exec(ctx, `
		UPDATE agent_canary_deployments
		SET status = $2, completed_at = $3, updated_at = NOW()
		WHERE agent_id = $1 AND status = 'active'
	`, agentID, status, now)
	if err != nil {
		return fmt.Errorf("complete agent canary: %w", err)
	}
	return nil
}
