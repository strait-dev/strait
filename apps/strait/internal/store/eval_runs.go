package store

import (
	"context"
	"fmt"
	"time"

	"strait/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

// CreateEvalRun inserts a new eval run result.
func (q *Queries) CreateEvalRun(ctx context.Context, run *domain.EvalRun) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateEvalRun")
	defer span.End()

	if run.ID == "" {
		run.ID = uuid.Must(uuid.NewV7()).String()
	}

	err := q.db.QueryRow(ctx, `
		INSERT INTO eval_runs (
			id, agent_id, deployment_id, project_id, suite_name,
			results_json, total_cases, passed_cases, failed_cases, duration_ms, status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING created_at
	`,
		run.ID, run.AgentID, run.DeploymentID, run.ProjectID, run.SuiteName,
		run.ResultsJSON, run.TotalCases, run.PassedCases, run.FailedCases,
		run.DurationMs, run.Status,
	).Scan(&run.CreatedAt)
	if err != nil {
		return fmt.Errorf("create eval run: %w", err)
	}
	return nil
}

// GetEvalRun returns a single eval run by ID.
func (q *Queries) GetEvalRun(ctx context.Context, id string) (*domain.EvalRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetEvalRun")
	defer span.End()

	var r domain.EvalRun
	err := q.db.QueryRow(ctx, `
		SELECT id, agent_id, deployment_id, project_id, suite_name,
			results_json, total_cases, passed_cases, failed_cases, duration_ms, status, created_at
		FROM eval_runs WHERE id = $1
	`, id).Scan(
		&r.ID, &r.AgentID, &r.DeploymentID, &r.ProjectID, &r.SuiteName,
		&r.ResultsJSON, &r.TotalCases, &r.PassedCases, &r.FailedCases,
		&r.DurationMs, &r.Status, &r.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get eval run: %w", err)
	}
	return &r, nil
}

// ListEvalRuns returns eval runs for an agent, newest first.
func (q *Queries) ListEvalRuns(ctx context.Context, agentID string, limit int) ([]domain.EvalRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListEvalRuns")
	defer span.End()

	if limit <= 0 || limit > 100 {
		limit = 20
	}

	rows, err := q.db.Query(ctx, `
		SELECT id, agent_id, deployment_id, project_id, suite_name,
			results_json, total_cases, passed_cases, failed_cases, duration_ms, status, created_at
		FROM eval_runs WHERE agent_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`, agentID, limit)
	if err != nil {
		return nil, fmt.Errorf("list eval runs: %w", err)
	}
	defer rows.Close()

	var runs []domain.EvalRun
	for rows.Next() {
		var r domain.EvalRun
		if err := rows.Scan(
			&r.ID, &r.AgentID, &r.DeploymentID, &r.ProjectID, &r.SuiteName,
			&r.ResultsJSON, &r.TotalCases, &r.PassedCases, &r.FailedCases,
			&r.DurationMs, &r.Status, &r.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan eval run: %w", err)
		}
		runs = append(runs, r)
	}
	return runs, nil
}

// CountEvalRunsByAgent returns the total number of eval runs for an agent.
func (q *Queries) CountEvalRunsByAgent(ctx context.Context, agentID string) (int, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CountEvalRunsByAgent")
	defer span.End()

	var count int
	err := q.db.QueryRow(ctx, `SELECT COUNT(*) FROM eval_runs WHERE agent_id = $1`, agentID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count eval runs: %w", err)
	}
	return count, nil
}

// EvalRunsSummary returns aggregate stats for an agent's eval runs.
type EvalRunsSummary struct {
	TotalRuns   int       `json:"total_runs"`
	TotalCases  int       `json:"total_cases"`
	PassedCases int       `json:"passed_cases"`
	FailedCases int       `json:"failed_cases"`
	PassRate    float64   `json:"pass_rate"`
	LastRunAt   time.Time `json:"last_run_at"`
}
