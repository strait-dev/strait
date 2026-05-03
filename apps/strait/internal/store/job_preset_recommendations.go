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

// PresetRecommendation represents a historical OOM-based preset recommendation.
type PresetRecommendation struct {
	ID                string
	JobID             string
	CurrentPreset     string
	RecommendedPreset string
	OOMCount          int
	WindowStart       time.Time
	ExpiresAt         time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// RecordOOMEvent upserts a preset recommendation for a job that experienced an OOM.
// Increments oom_count, upgrades recommended_preset, and resets expiry to 24h.
func (q *Queries) RecordOOMEvent(ctx context.Context, jobID, preset string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.RecordOOMEvent")
	defer span.End()

	recommended := preset
	if next, ok := domain.NextPreset(preset); ok {
		recommended = next
	}

	id := uuid.Must(uuid.NewV7()).String()
	now := time.Now()
	expiresAt := now.Add(24 * time.Hour)

	query := `
		INSERT INTO job_preset_recommendations (id, job_id, current_preset, recommended_preset, oom_count, window_start, expires_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 1, $5, $6, $5, $5)
		ON CONFLICT (job_id) DO UPDATE SET
			oom_count = job_preset_recommendations.oom_count + 1,
			recommended_preset = $4,
			expires_at = $6,
			updated_at = $5`

	if _, err := q.db.Exec(ctx, query, id, jobID, preset, recommended, now, expiresAt); err != nil {
		return fmt.Errorf("record oom event: %w", err)
	}
	return nil
}

// GetPresetRecommendation returns the active (non-expired) recommendation for a job.
func (q *Queries) GetPresetRecommendation(ctx context.Context, jobID string) (*PresetRecommendation, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetPresetRecommendation")
	defer span.End()

	query := `
		SELECT id, job_id, current_preset, recommended_preset, oom_count, window_start, expires_at, created_at, updated_at
		FROM job_preset_recommendations
		WHERE job_id = $1 AND expires_at > NOW()`

	var r PresetRecommendation
	err := q.db.QueryRow(ctx, query, jobID).Scan(
		&r.ID, &r.JobID, &r.CurrentPreset, &r.RecommendedPreset,
		&r.OOMCount, &r.WindowStart, &r.ExpiresAt, &r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get preset recommendation: %w", err)
	}
	return &r, nil
}

// CleanupExpiredRecommendations deletes expired preset recommendations.
func (q *Queries) CleanupExpiredRecommendations(ctx context.Context) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CleanupExpiredRecommendations")
	defer span.End()

	tag, err := q.db.Exec(ctx, "DELETE FROM job_preset_recommendations WHERE expires_at <= NOW()")
	if err != nil {
		return 0, fmt.Errorf("cleanup expired recommendations: %w", err)
	}
	return tag.RowsAffected(), nil
}

// PresetRecommendationStore defines preset recommendation operations.
type PresetRecommendationStore interface {
	RecordOOMEvent(ctx context.Context, jobID, preset string) error
	GetPresetRecommendation(ctx context.Context, jobID string) (*PresetRecommendation, error)
	CleanupExpiredRecommendations(ctx context.Context) (int64, error)
}

var _ PresetRecommendationStore = (*Queries)(nil)
