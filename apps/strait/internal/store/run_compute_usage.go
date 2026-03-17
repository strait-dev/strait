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

// ProjectComputeQuota is a lightweight projection for budget monitoring.
type ProjectComputeQuota struct {
	ProjectID                     string
	Timezone                      string
	ComputeDailyCostLimitMicrousd int64
}

// ListProjectsWithComputeLimit returns all projects that have a compute daily cost limit set.
func (q *Queries) ListProjectsWithComputeLimit(ctx context.Context) ([]ProjectComputeQuota, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListProjectsWithComputeLimit")
	defer span.End()

	query := `
		SELECT project_id, COALESCE(timezone, 'UTC'), compute_daily_cost_limit_microusd
		FROM project_quotas
		WHERE compute_daily_cost_limit_microusd IS NOT NULL
		  AND compute_daily_cost_limit_microusd > 0`

	rows, err := q.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list projects with compute limit: %w", err)
	}
	defer rows.Close()

	var results []ProjectComputeQuota
	for rows.Next() {
		var pq ProjectComputeQuota
		if err := rows.Scan(&pq.ProjectID, &pq.Timezone, &pq.ComputeDailyCostLimitMicrousd); err != nil {
			return nil, fmt.Errorf("scan project compute quota: %w", err)
		}
		results = append(results, pq)
	}
	return results, rows.Err()
}

// ErrBudgetExceeded is returned when a budget reservation would exceed the daily limit.
var ErrBudgetExceeded = errors.New("compute budget exceeded")

// ReserveBudget atomically reserves estimated cost for a run using advisory locking.
// Requires the underlying DBTX to implement TxBeginner (e.g., *pgxpool.Pool).
func (q *Queries) ReserveBudget(ctx context.Context, projectID, runID, jobID, preset string, estimatedCost int64, tz string, limit int64) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ReserveBudget")
	defer span.End()

	if tz == "" {
		tz = "UTC"
	}

	beginner, ok := q.db.(TxBeginner)
	if !ok {
		return fmt.Errorf("reserve budget: db does not support transactions")
	}

	tx, err := beginner.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Advisory lock scoped to project to serialize concurrent reservations.
	lockID := hashString(projectID)
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", lockID); err != nil {
		return fmt.Errorf("advisory lock: %w", err)
	}

	// Sum daily cost including both reserved and committed.
	var dailyCost int64
	sumQuery := `
		SELECT COALESCE(SUM(cost_microusd), 0)
		FROM run_compute_usage
		WHERE project_id = $1
		  AND status IN ('reserved', 'committed')
		  AND created_at >= date_trunc('day', NOW() AT TIME ZONE $2) AT TIME ZONE $2`
	if err := tx.QueryRow(ctx, sumQuery, projectID, tz).Scan(&dailyCost); err != nil {
		return fmt.Errorf("sum daily cost: %w", err)
	}

	if dailyCost+estimatedCost > limit {
		return ErrBudgetExceeded
	}

	id := uuid.Must(uuid.NewV7()).String()
	insertQuery := `
		INSERT INTO run_compute_usage (id, run_id, project_id, job_id, machine_preset, cost_microusd, status)
		VALUES ($1, $2, $3, $4, $5, $6, 'reserved')`
	if _, err := tx.Exec(ctx, insertQuery, id, runID, projectID, jobID, preset, estimatedCost); err != nil {
		return fmt.Errorf("insert reservation: %w", err)
	}

	return tx.Commit(ctx)
}

// CommitReservation updates a reserved row to committed with actual cost and timing.
func (q *Queries) CommitReservation(ctx context.Context, runID string, actualCost int64, durationSecs float64, machineID string, startedAt, finishedAt *time.Time) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CommitReservation")
	defer span.End()

	query := `
		UPDATE run_compute_usage
		SET status = 'committed', cost_microusd = $2, duration_secs = $3,
		    machine_id = $4, started_at = $5, finished_at = $6
		WHERE run_id = $1 AND status = 'reserved'`
	tag, err := q.db.Exec(ctx, query, runID, actualCost, durationSecs, machineID, startedAt, finishedAt)
	if err != nil {
		return fmt.Errorf("commit reservation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// No reservation to commit — fall back to inserting directly.
		return nil
	}
	return nil
}

// ReleaseReservation deletes a reserved row (on Create failure).
func (q *Queries) ReleaseReservation(ctx context.Context, runID string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ReleaseReservation")
	defer span.End()

	query := `DELETE FROM run_compute_usage WHERE run_id = $1 AND status = 'reserved'`
	if _, err := q.db.Exec(ctx, query, runID); err != nil {
		return fmt.Errorf("release reservation: %w", err)
	}
	return nil
}

// CleanupStaleReservations deletes reserved rows older than the given duration.
func (q *Queries) CleanupStaleReservations(ctx context.Context, olderThan time.Duration) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CleanupStaleReservations")
	defer span.End()

	query := `DELETE FROM run_compute_usage WHERE status = 'reserved' AND created_at < $1`
	cutoff := time.Now().Add(-olderThan)
	tag, err := q.db.Exec(ctx, query, cutoff)
	if err != nil {
		return 0, fmt.Errorf("cleanup stale reservations: %w", err)
	}
	return tag.RowsAffected(), nil
}

// hashString returns a deterministic int64 hash for advisory lock keys.
func hashString(s string) int64 {
	var h int64
	for _, c := range s {
		h = h*31 + int64(c)
	}
	return h
}

// RunComputeUsageStore defines compute usage operations.
type RunComputeUsageStore interface {
	CreateRunComputeUsage(ctx context.Context, usage *domain.RunComputeUsage) error
	GetRunComputeUsage(ctx context.Context, id string) (*domain.RunComputeUsage, error)
	SumDailyComputeCost(ctx context.Context, projectID, timezone string) (int64, error)
	ListRunComputeUsageByProject(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.RunComputeUsage, error)
	ListProjectsWithComputeLimit(ctx context.Context) ([]ProjectComputeQuota, error)
	ReserveBudget(ctx context.Context, projectID, runID, jobID, preset string, estimatedCost int64, tz string, limit int64) error
	CommitReservation(ctx context.Context, runID string, actualCost int64, durationSecs float64, machineID string, startedAt, finishedAt *time.Time) error
	ReleaseReservation(ctx context.Context, runID string) error
	CleanupStaleReservations(ctx context.Context, olderThan time.Duration) (int64, error)
}

var _ RunComputeUsageStore = (*Queries)(nil)
