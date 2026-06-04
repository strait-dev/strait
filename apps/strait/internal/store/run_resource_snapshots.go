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

// CreateRunResourceSnapshot records a point-in-time resource utilization snapshot for a run.
func (q *Queries) CreateRunResourceSnapshot(ctx context.Context, snapshot *domain.RunResourceSnapshot) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateRunResourceSnapshot")
	defer span.End()

	if snapshot.ID == "" {
		snapshot.ID = uuid.Must(uuid.NewV7()).String()
	}

	query := `
		INSERT INTO run_resource_snapshots (id, run_id, cpu_percent, memory_mb, memory_limit_mb, network_rx_bytes, network_tx_bytes)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING created_at`

	err := q.db.QueryRow(ctx, query,
		snapshot.ID,
		snapshot.RunID,
		snapshot.CPUPercent,
		snapshot.MemoryMB,
		snapshot.MemoryLimitMB,
		snapshot.NetworkRxBytes,
		snapshot.NetworkTxBytes,
	).Scan(&snapshot.CreatedAt)
	if err != nil {
		return fmt.Errorf("create run resource snapshot: %w", err)
	}

	return nil
}

func (q *Queries) CreateRunResourceSnapshotForActiveRun(ctx context.Context, snapshot *domain.RunResourceSnapshot, attempt int) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateRunResourceSnapshotForActiveRun")
	defer span.End()

	if snapshot.ID == "" {
		snapshot.ID = uuid.Must(uuid.NewV7()).String()
	}

	query := `
		WITH active_run AS (
			SELECT jr.id
			FROM job_runs jr
			LEFT JOIN job_run_read_state s ON s.run_id = jr.id
			WHERE jr.id = $2
			  AND COALESCE(s.attempt, jr.attempt) = $8
			  AND COALESCE(s.status, jr.status) IN ('executing', 'waiting')
			FOR UPDATE OF jr
		)
		INSERT INTO run_resource_snapshots (id, run_id, cpu_percent, memory_mb, memory_limit_mb, network_rx_bytes, network_tx_bytes)
		SELECT $1, id, $3, $4, $5, $6, $7
		FROM active_run
		RETURNING created_at`

	err := q.db.QueryRow(ctx, query,
		snapshot.ID,
		snapshot.RunID,
		snapshot.CPUPercent,
		snapshot.MemoryMB,
		snapshot.MemoryLimitMB,
		snapshot.NetworkRxBytes,
		snapshot.NetworkTxBytes,
		attempt,
	).Scan(&snapshot.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: run %s is not active for attempt %d", ErrRunConflict, snapshot.RunID, attempt)
		}
		return fmt.Errorf("create active run resource snapshot: %w", err)
	}

	return nil
}

// ListRunResourceSnapshots returns resource snapshots for a run, optionally filtered by time range.
func (q *Queries) ListRunResourceSnapshots(ctx context.Context, runID string, from, to *time.Time, limit int) ([]domain.RunResourceSnapshot, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListRunResourceSnapshots")
	defer span.End()

	base := `
		SELECT id, run_id, cpu_percent, memory_mb, memory_limit_mb, network_rx_bytes, network_tx_bytes, created_at
		FROM run_resource_snapshots
		WHERE run_id = $1`

	args := []any{runID}
	param := 2

	if from != nil {
		base += fmt.Sprintf(" AND created_at >= $%d", param)
		args = append(args, *from)
		param++
	}

	if to != nil {
		base += fmt.Sprintf(" AND created_at <= $%d", param)
		args = append(args, *to)
		param++
	}

	base += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", param)
	args = append(args, limit)

	rows, err := q.db.Query(ctx, base, args...)
	if err != nil {
		return nil, fmt.Errorf("list run resource snapshots: %w", err)
	}
	defer rows.Close()

	snapshots := make([]domain.RunResourceSnapshot, 0, limit)
	for rows.Next() {
		var s domain.RunResourceSnapshot
		if err := rows.Scan(
			&s.ID, &s.RunID, &s.CPUPercent, &s.MemoryMB, &s.MemoryLimitMB,
			&s.NetworkRxBytes, &s.NetworkTxBytes, &s.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan run resource snapshot: %w", err)
		}
		snapshots = append(snapshots, s)
	}

	return snapshots, rows.Err()
}
