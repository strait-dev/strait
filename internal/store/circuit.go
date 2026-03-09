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

func (q *Queries) GetEndpointCircuitState(ctx context.Context, endpointURL string) (*domain.EndpointCircuitState, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetEndpointCircuitState")
	defer span.End()

	const sql = `
		SELECT endpoint_url, state, consecutive_failures, opened_at, half_open_until, updated_at, created_at
		FROM endpoint_circuit_state
		WHERE endpoint_url = $1
	`

	var state domain.EndpointCircuitState
	err := q.db.QueryRow(ctx, sql, endpointURL).Scan(
		&state.EndpointURL,
		&state.State,
		&state.ConsecutiveFailures,
		&state.OpenedAt,
		&state.HalfOpenUntil,
		&state.UpdatedAt,
		&state.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("query endpoint circuit state: %w", err)
	}

	return &state, nil
}

func (q *Queries) CanDispatchEndpoint(ctx context.Context, endpointURL string, now time.Time) (bool, *time.Time, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CanDispatchEndpoint")
	defer span.End()

	if _, err := q.db.Exec(ctx, `
		INSERT INTO endpoint_circuit_state (endpoint_url)
		VALUES ($1)
		ON CONFLICT (endpoint_url) DO NOTHING
	`, endpointURL); err != nil {
		return false, nil, fmt.Errorf("upsert endpoint circuit row: %w", err)
	}

	state, err := q.GetEndpointCircuitState(ctx, endpointURL)
	if err != nil {
		return false, nil, err
	}
	if state == nil {
		return true, nil, nil
	}

	if state.State == domain.CircuitStateOpen && state.HalfOpenUntil != nil && state.HalfOpenUntil.After(now) {
		return false, state.HalfOpenUntil, nil
	}

	if state.State == domain.CircuitStateOpen || state.State == domain.CircuitStateHalfOpen {
		if _, err := q.db.Exec(ctx, `
			UPDATE endpoint_circuit_state
			SET state = 'closed',
				consecutive_failures = 0,
				opened_at = NULL,
				half_open_until = NULL,
				updated_at = NOW()
			WHERE endpoint_url = $1
		`, endpointURL); err != nil {
			return false, nil, fmt.Errorf("reset endpoint circuit state: %w", err)
		}
	}

	return true, nil, nil
}

func (q *Queries) RecordEndpointCircuitFailure(ctx context.Context, endpointURL string, now time.Time, threshold int, openDuration time.Duration) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.RecordEndpointCircuitFailure")
	defer span.End()

	if threshold < 1 {
		threshold = 1
	}

	if _, err := q.db.Exec(ctx, `
		INSERT INTO endpoint_circuit_state (endpoint_url)
		VALUES ($1)
		ON CONFLICT (endpoint_url) DO NOTHING
	`, endpointURL); err != nil {
		return fmt.Errorf("upsert endpoint circuit row: %w", err)
	}

	halfOpenUntil := now.Add(openDuration)
	if _, err := q.db.Exec(ctx, `
		UPDATE endpoint_circuit_state
		SET consecutive_failures = CASE
				WHEN state = 'open' AND half_open_until IS NOT NULL AND half_open_until > $2 THEN consecutive_failures
				ELSE consecutive_failures + 1
			END,
			state = CASE
				WHEN state = 'open' AND half_open_until IS NOT NULL AND half_open_until > $2 THEN 'open'
				WHEN consecutive_failures + 1 >= $3 THEN 'open'
				ELSE 'closed'
			END,
			opened_at = CASE
				WHEN state = 'open' AND half_open_until IS NOT NULL AND half_open_until > $2 THEN opened_at
				WHEN consecutive_failures + 1 >= $3 THEN $2
				ELSE NULL
			END,
			half_open_until = CASE
				WHEN state = 'open' AND half_open_until IS NOT NULL AND half_open_until > $2 THEN half_open_until
				WHEN consecutive_failures + 1 >= $3 THEN $4
				ELSE NULL
			END,
			updated_at = NOW()
		WHERE endpoint_url = $1
	`, endpointURL, now.UTC(), threshold, halfOpenUntil.UTC()); err != nil {
		return fmt.Errorf("update endpoint circuit failure state: %w", err)
	}

	return nil
}

func (q *Queries) RecordEndpointCircuitSuccess(ctx context.Context, endpointURL string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.RecordEndpointCircuitSuccess")
	defer span.End()

	if _, err := q.db.Exec(ctx, `
		INSERT INTO endpoint_circuit_state (endpoint_url, state, consecutive_failures, opened_at, half_open_until)
		VALUES ($1, 'closed', 0, NULL, NULL)
		ON CONFLICT (endpoint_url) DO UPDATE
		SET state = 'closed',
			consecutive_failures = 0,
			opened_at = NULL,
			half_open_until = NULL,
			updated_at = NOW()
	`, endpointURL); err != nil {
		return fmt.Errorf("record endpoint circuit success: %w", err)
	}

	return nil
}
