package store

import (
	"context"
	"errors"
	"fmt"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

func (q *Queries) GetEndpointHealthScore(ctx context.Context, endpointURL string) (*domain.EndpointHealthScore, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetEndpointHealthScore")
	defer span.End()

	const sql = `
		SELECT endpoint_url, health_score, success_rate, timeout_rate, latency_score,
		       total_requests, last_latency_ms, updated_at, created_at
		FROM endpoint_health_scores
		WHERE endpoint_url = $1
	`

	var s domain.EndpointHealthScore
	err := q.db.QueryRow(ctx, sql, endpointURL).Scan(
		&s.EndpointURL,
		&s.HealthScore,
		&s.SuccessRate,
		&s.TimeoutRate,
		&s.LatencyScore,
		&s.TotalRequests,
		&s.LastLatencyMs,
		&s.UpdatedAt,
		&s.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("query endpoint health score: %w", err)
	}

	return &s, nil
}

func (q *Queries) UpsertEndpointHealthScore(ctx context.Context, score *domain.EndpointHealthScore) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpsertEndpointHealthScore")
	defer span.End()

	const sql = `
		INSERT INTO endpoint_health_scores (
			endpoint_url, health_score, success_rate, timeout_rate, latency_score,
			total_requests, last_latency_ms, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
		ON CONFLICT (endpoint_url) DO UPDATE SET
			health_score = EXCLUDED.health_score,
			success_rate = EXCLUDED.success_rate,
			timeout_rate = EXCLUDED.timeout_rate,
			latency_score = EXCLUDED.latency_score,
			total_requests = EXCLUDED.total_requests,
			last_latency_ms = EXCLUDED.last_latency_ms,
			updated_at = NOW()
	`

	if _, err := q.db.Exec(ctx, sql,
		score.EndpointURL,
		score.HealthScore,
		score.SuccessRate,
		score.TimeoutRate,
		score.LatencyScore,
		score.TotalRequests,
		score.LastLatencyMs,
	); err != nil {
		return fmt.Errorf("upsert endpoint health score: %w", err)
	}

	return nil
}
