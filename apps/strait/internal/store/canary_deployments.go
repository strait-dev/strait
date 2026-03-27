package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"strait/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel"
)

// ErrCanaryNotFound is returned when no active canary deployment exists for a workflow.
var ErrCanaryNotFound = errors.New("no active canary deployment found")

// ErrCanaryAlreadyActive is returned when attempting to create a canary for a workflow that already has one.
var ErrCanaryAlreadyActive = errors.New("an active canary deployment already exists for this workflow")

func (q *Queries) CreateCanaryDeployment(ctx context.Context, canary *domain.CanaryDeployment) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateCanaryDeployment")
	defer span.End()

	if canary.ID == "" {
		canary.ID = uuid.Must(uuid.NewV7()).String()
	}

	query := `
		INSERT INTO canary_deployments (
			id, workflow_id, project_id, source_version, target_version,
			traffic_pct, status, auto_promote_config
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at, updated_at`

	err := q.db.QueryRow(ctx, query,
		canary.ID,
		canary.WorkflowID,
		canary.ProjectID,
		canary.SourceVersion,
		canary.TargetVersion,
		canary.TrafficPct,
		canary.Status,
		canary.AutoPromote,
	).Scan(&canary.CreatedAt, &canary.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrCanaryAlreadyActive
		}
		return fmt.Errorf("create canary deployment: %w", err)
	}

	return nil
}

func (q *Queries) GetActiveCanaryDeployment(ctx context.Context, workflowID string) (*domain.CanaryDeployment, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetActiveCanaryDeployment")
	defer span.End()

	query := `
		SELECT id, workflow_id, project_id, source_version, target_version,
		       traffic_pct, status, auto_promote_config, created_at, updated_at, completed_at
		FROM canary_deployments
		WHERE workflow_id = $1 AND status = 'active'
		LIMIT 1`

	var canary domain.CanaryDeployment
	err := q.db.QueryRow(ctx, query, workflowID).Scan(
		&canary.ID,
		&canary.WorkflowID,
		&canary.ProjectID,
		&canary.SourceVersion,
		&canary.TargetVersion,
		&canary.TrafficPct,
		&canary.Status,
		&canary.AutoPromote,
		&canary.CreatedAt,
		&canary.UpdatedAt,
		&canary.CompletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCanaryNotFound
		}
		return nil, fmt.Errorf("get active canary deployment: %w", err)
	}

	return &canary, nil
}

func (q *Queries) UpdateCanaryDeploymentTraffic(ctx context.Context, workflowID string, trafficPct int) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpdateCanaryDeploymentTraffic")
	defer span.End()

	query := `
		UPDATE canary_deployments
		SET traffic_pct = $1, updated_at = NOW()
		WHERE workflow_id = $2 AND status = 'active'`

	tag, err := q.db.Exec(ctx, query, trafficPct, workflowID)
	if err != nil {
		return fmt.Errorf("update canary traffic: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrCanaryNotFound
	}

	return nil
}

func (q *Queries) CompleteCanaryDeployment(ctx context.Context, workflowID, status string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CompleteCanaryDeployment")
	defer span.End()

	now := time.Now()
	query := `
		UPDATE canary_deployments
		SET status = $1, completed_at = $2, updated_at = NOW()
		WHERE workflow_id = $3 AND status = 'active'`

	tag, err := q.db.Exec(ctx, query, status, now, workflowID)
	if err != nil {
		return fmt.Errorf("complete canary deployment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrCanaryNotFound
	}

	return nil
}
