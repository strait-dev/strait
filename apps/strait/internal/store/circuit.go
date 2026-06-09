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

	return q.getEndpointCircuitState(ctx, endpointURL)
}

func (q *Queries) getEndpointCircuitState(ctx context.Context, endpointURL string) (*domain.EndpointCircuitState, error) {
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

	state, err := q.getEndpointCircuitState(ctx, endpointURL)
	if err != nil {
		return false, nil, err
	}
	if state == nil || state.State == domain.CircuitStateClosed {
		return true, nil, nil
	}
	if state.State == domain.CircuitStateOpen && state.HalfOpenUntil != nil && state.HalfOpenUntil.After(now) {
		return false, state.HalfOpenUntil, nil
	}

	return q.canDispatchEndpointLocked(ctx, endpointURL, now)
}

func (q *Queries) canDispatchEndpointLocked(ctx context.Context, endpointURL string, now time.Time) (bool, *time.Time, error) {
	// Slow path: serialize only when the circuit is open/half-open and may need
	// a reset transition. The healthy closed path above is a plain read.
	const sql = `
		WITH locked AS (
			SELECT endpoint_url, state, consecutive_failures, opened_at, half_open_until, updated_at, created_at
			FROM endpoint_circuit_state
			WHERE endpoint_url = $1
			FOR UPDATE
		),
		maybe_reset AS (
			UPDATE endpoint_circuit_state ecs
			SET state = 'closed',
				consecutive_failures = 0,
				opened_at = NULL,
				half_open_until = NULL,
				updated_at = NOW()
			FROM locked l
			WHERE ecs.endpoint_url = l.endpoint_url
			  AND l.state IN ('open', 'half_open')
			  AND (l.half_open_until IS NULL OR l.half_open_until <= $2)
			RETURNING ecs.endpoint_url, ecs.state, ecs.consecutive_failures, ecs.opened_at, ecs.half_open_until, ecs.updated_at, ecs.created_at
		)
		SELECT endpoint_url, state, consecutive_failures, opened_at, half_open_until, updated_at, created_at
		FROM maybe_reset
		UNION ALL
		SELECT endpoint_url, state, consecutive_failures, opened_at, half_open_until, updated_at, created_at
		FROM locked
		WHERE NOT EXISTS (SELECT 1 FROM maybe_reset)
	`

	var state domain.EndpointCircuitState
	err := q.db.QueryRow(ctx, sql, endpointURL, now).Scan(
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
			// The row disappeared between the fast read and locked slow path.
			// No circuit state is equivalent to a closed circuit.
			return true, nil, nil
		}
		return false, nil, fmt.Errorf("can dispatch endpoint circuit check: %w", err)
	}

	// If the circuit is still open (half_open_until is in the future), reject.
	if state.State == domain.CircuitStateOpen && state.HalfOpenUntil != nil && state.HalfOpenUntil.After(now) {
		return false, state.HalfOpenUntil, nil
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
