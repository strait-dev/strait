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
		WHERE endpoint_health_scores.health_score IS DISTINCT FROM EXCLUDED.health_score
		   OR endpoint_health_scores.success_rate IS DISTINCT FROM EXCLUDED.success_rate
		   OR endpoint_health_scores.timeout_rate IS DISTINCT FROM EXCLUDED.timeout_rate
		   OR endpoint_health_scores.latency_score IS DISTINCT FROM EXCLUDED.latency_score
		   OR endpoint_health_scores.total_requests IS DISTINCT FROM EXCLUDED.total_requests
		   OR endpoint_health_scores.last_latency_ms IS DISTINCT FROM EXCLUDED.last_latency_ms
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

// AtomicRecordHealthResult computes EWMA health scores server-side in a single
// INSERT ... ON CONFLICT DO UPDATE to prevent lost-update races under concurrent
// writes. Parameters: successVal/timeoutVal/latencyVal are the raw signal values
// (0.0 or 1.0), alpha is the EWMA smoothing factor, and the weight parameters
// control how each component contributes to the composite score.
func (q *Queries) AtomicRecordHealthResult(
	ctx context.Context,
	endpointURL string,
	successVal, timeoutVal, latencyVal, alpha float64,
	weightSuccess, weightTimeout, weightLatency float64,
	lastLatencyMs float64,
) (*domain.EndpointHealthScore, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.AtomicRecordHealthResult")
	defer span.End()

	// $1=endpoint_url $2=successVal $3=timeoutVal $4=latencyVal
	// $5=alpha $6=wSuccess $7=wTimeout $8=wLatency $9=lastLatencyMs
	//
	// All parameter references are explicitly cast to float8 to avoid
	// "operator is not unique: unknown * unknown" on PostgreSQL 18+.
	const atomicSQL = `
		INSERT INTO endpoint_health_scores AS ehs (
			endpoint_url, success_rate, timeout_rate, latency_score,
			health_score, total_requests, last_latency_ms, updated_at
		) VALUES (
			$1,
			$5::float8 * $2::float8 + (1.0 - $5::float8) * 1.0,
			$5::float8 * $3::float8 + (1.0 - $5::float8) * 0.0,
			$5::float8 * $4::float8 + (1.0 - $5::float8) * 1.0,
			LEAST(100.0, GREATEST(0.0,
				($6::float8 * ($5::float8 * $2::float8 + (1.0 - $5::float8) * 1.0) +
				 $7::float8 * (1.0 - ($5::float8 * $3::float8 + (1.0 - $5::float8) * 0.0)) +
				 $8::float8 * ($5::float8 * $4::float8 + (1.0 - $5::float8) * 1.0)) * 100.0
			)),
			1,
			$9::float8,
			NOW()
		)
		ON CONFLICT (endpoint_url) DO UPDATE SET
			success_rate  = $5::float8 * $2::float8 + (1.0 - $5::float8) * ehs.success_rate,
			timeout_rate  = $5::float8 * $3::float8 + (1.0 - $5::float8) * ehs.timeout_rate,
			latency_score = $5::float8 * $4::float8 + (1.0 - $5::float8) * ehs.latency_score,
			health_score  = LEAST(100.0, GREATEST(0.0,
				($6::float8 * ($5::float8 * $2::float8 + (1.0 - $5::float8) * ehs.success_rate) +
				 $7::float8 * (1.0 - ($5::float8 * $3::float8 + (1.0 - $5::float8) * ehs.timeout_rate)) +
				 $8::float8 * ($5::float8 * $4::float8 + (1.0 - $5::float8) * ehs.latency_score)) * 100.0
			)),
			total_requests = ehs.total_requests + 1,
			last_latency_ms = $9::float8,
			updated_at = NOW()
		RETURNING endpoint_url, health_score, success_rate, timeout_rate, latency_score,
		          total_requests, last_latency_ms, updated_at, created_at
	`

	var s domain.EndpointHealthScore
	err := q.db.QueryRow(ctx, atomicSQL,
		endpointURL,   // $1
		successVal,    // $2
		timeoutVal,    // $3
		latencyVal,    // $4
		alpha,         // $5
		weightSuccess, // $6
		weightTimeout, // $7
		weightLatency, // $8
		lastLatencyMs, // $9
	).Scan(
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
		return nil, fmt.Errorf("atomic record health result: %w", err)
	}

	return &s, nil
}
