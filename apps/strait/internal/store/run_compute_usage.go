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

// CreateRunComputeUsage records compute usage for a managed run.
func (q *Queries) CreateRunComputeUsage(ctx context.Context, usage *domain.RunComputeUsage) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateRunComputeUsage")
	defer span.End()

	if usage.ID == "" {
		usage.ID = uuid.Must(uuid.NewV7()).String()
	}

	query := `
		INSERT INTO run_compute_usage (id, run_id, project_id, job_id, machine_preset, machine_id, duration_secs, cost_microusd, started_at, finished_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING created_at`

	err := q.db.QueryRow(ctx, query,
		usage.ID,
		usage.RunID,
		usage.ProjectID,
		usage.JobID,
		usage.MachinePreset,
		usage.MachineID,
		usage.DurationSecs,
		usage.CostMicrousd,
		usage.StartedAt,
		usage.FinishedAt,
	).Scan(&usage.CreatedAt)
	if err != nil {
		return fmt.Errorf("create run compute usage: %w", err)
	}

	return nil
}

// GetRunComputeUsage retrieves compute usage by ID.
func (q *Queries) GetRunComputeUsage(ctx context.Context, id string) (*domain.RunComputeUsage, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetRunComputeUsage")
	defer span.End()

	query := `
		SELECT id, run_id, project_id, job_id, machine_preset, machine_id, duration_secs, cost_microusd, started_at, finished_at, created_at
		FROM run_compute_usage
		WHERE id = $1`

	var u domain.RunComputeUsage
	err := q.db.QueryRow(ctx, query, id).Scan(
		&u.ID, &u.RunID, &u.ProjectID, &u.JobID, &u.MachinePreset, &u.MachineID,
		&u.DurationSecs, &u.CostMicrousd, &u.StartedAt, &u.FinishedAt, &u.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get run compute usage: %w", err)
	}

	return &u, nil
}

// SumDailyComputeCost returns the total compute cost in micro-USD for a project today.
func (q *Queries) SumDailyComputeCost(ctx context.Context, projectID, timezone string) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.SumDailyComputeCost")
	defer span.End()

	if timezone == "" {
		timezone = "UTC"
	}

	query := `
		SELECT COALESCE(SUM(cost_microusd), 0)
		FROM run_compute_usage
		WHERE project_id = $1
		  AND created_at >= date_trunc('day', NOW() AT TIME ZONE $2) AT TIME ZONE $2`

	var total int64
	err := q.db.QueryRow(ctx, query, projectID, timezone).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("sum daily compute cost: %w", err)
	}

	return total, nil
}

// ListRunComputeUsageByProject returns compute usage records for a project.
func (q *Queries) ListRunComputeUsageByProject(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.RunComputeUsage, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListRunComputeUsageByProject")
	defer span.End()

	base := `
		SELECT id, run_id, project_id, job_id, machine_preset, machine_id, duration_secs, cost_microusd, started_at, finished_at, created_at
		FROM run_compute_usage
		WHERE project_id = $1`

	args := []any{projectID}
	param := 2

	if cursor != nil {
		base += fmt.Sprintf(" AND created_at < $%d", param)
		args = append(args, *cursor)
		param++
	}

	base += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", param)
	args = append(args, limit)

	rows, err := q.db.Query(ctx, base, args...)
	if err != nil {
		return nil, fmt.Errorf("list run compute usage: %w", err)
	}
	defer rows.Close()

	usages := make([]domain.RunComputeUsage, 0, limit)
	for rows.Next() {
		var u domain.RunComputeUsage
		if err := rows.Scan(
			&u.ID, &u.RunID, &u.ProjectID, &u.JobID, &u.MachinePreset, &u.MachineID,
			&u.DurationSecs, &u.CostMicrousd, &u.StartedAt, &u.FinishedAt, &u.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan run compute usage: %w", err)
		}
		usages = append(usages, u)
	}

	return usages, rows.Err()
}

// RunComputeUsageStore defines compute usage operations.
type RunComputeUsageStore interface {
	CreateRunComputeUsage(ctx context.Context, usage *domain.RunComputeUsage) error
	GetRunComputeUsage(ctx context.Context, id string) (*domain.RunComputeUsage, error)
	SumDailyComputeCost(ctx context.Context, projectID, timezone string) (int64, error)
	ListRunComputeUsageByProject(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.RunComputeUsage, error)
}

var _ RunComputeUsageStore = (*Queries)(nil)
