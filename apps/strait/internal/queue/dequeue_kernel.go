package queue

import (
	"context"
	"fmt"

	"strait/internal/dbscan"
	"strait/internal/domain"
	"strait/internal/store"

	"go.opentelemetry.io/otel"
)

type dequeueSpec struct {
	spanName            string
	candidatesSQL       string
	extraArgs           []any
	skipConcurrencyCTEs bool
	postScanFn          func(runs []domain.JobRun) error
}

func withStatementTimeout(ctx context.Context, q *PostgresQueue, spanName string) (store.DBTX, func(), error) {
	if q.statementTimeout <= 0 {
		return q.db, func() {}, nil
	}
	beginner, ok := q.db.(store.TxBeginner)
	if !ok {
		return q.db, func() {}, nil
	}
	tx, err := beginner.Begin(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: begin tx: %w", spanName, err)
	}
	ms := int(q.statementTimeout.Milliseconds())
	if _, err := tx.Exec(ctx, fmt.Sprintf("SET LOCAL statement_timeout = %d", ms)); err != nil {
		_ = tx.Rollback(ctx)
		return nil, nil, fmt.Errorf("%s: set statement timeout: %w", spanName, err)
	}
	return tx, func() { _ = tx.Commit(ctx) }, nil
}

func executeDequeue(ctx context.Context, q *PostgresQueue, n int, spec dequeueSpec) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, spec.spanName)
	defer span.End()

	if n <= 0 {
		return nil, nil
	}

	db, cleanup, err := withStatementTimeout(ctx, q, spec.spanName)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	withClause := concurrencyCTEs + ","
	if spec.skipConcurrencyCTEs {
		withClause = ""
	}

	var query string
	if withClause != "" {
		query = fmt.Sprintf(`
			WITH %s
			claimed AS (%s),
			updated AS (
				UPDATE job_runs
				SET status = '%s', started_at = NOW()
				WHERE id IN (SELECT id FROM claimed)
				RETURNING %s
			)
			SELECT %s FROM updated ORDER BY created_at ASC`,
			withClause, spec.candidatesSQL, domain.StatusDequeued, dequeueColumns, dequeueColumns)
	} else {
		query = fmt.Sprintf(`
			WITH claimed AS (%s),
			updated AS (
				UPDATE job_runs
				SET status = '%s', started_at = NOW()
				WHERE id IN (SELECT id FROM claimed)
				RETURNING %s
			)
			SELECT %s FROM updated ORDER BY created_at ASC`,
			spec.candidatesSQL, domain.StatusDequeued, dequeueColumns, dequeueColumns)
	}

	args := append([]any{n}, spec.extraArgs...)
	rows, err := db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", spec.spanName, err)
	}
	defer rows.Close()

	runs := make([]domain.JobRun, 0, n)
	for rows.Next() {
		run, err := dbscan.ScanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("%s scan: %w", spec.spanName, err)
		}
		runs = append(runs, *run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s rows: %w", spec.spanName, err)
	}
	for i := range runs {
		q.recordClaimMetrics(ctx, &runs[i])
	}
	if spec.postScanFn != nil {
		if err := spec.postScanFn(runs); err != nil {
			return runs, err
		}
	}
	return runs, nil
}

func executeDequeueFair(ctx context.Context, q *PostgresQueue, n int, spec dequeueSpec) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, spec.spanName)
	defer span.End()

	if n <= 0 {
		return nil, nil
	}

	db, cleanup, err := withStatementTimeout(ctx, q, spec.spanName)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	query := fmt.Sprintf(`
		WITH %s,
		candidates AS (%s),
		claimed AS (
			SELECT c.id
			FROM candidates c
			JOIN job_runs jr ON jr.id = c.id
			ORDER BY %s
			FOR UPDATE OF jr SKIP LOCKED
			LIMIT $1
		),
		updated AS (
			UPDATE job_runs
			SET status = '%s', started_at = NOW()
			WHERE id IN (SELECT id FROM claimed)
			RETURNING %s
		)
		SELECT %s FROM updated ORDER BY created_at ASC`,
		concurrencyCTEs, spec.candidatesSQL, q.dequeueOrderByClause(), domain.StatusDequeued, dequeueColumns, dequeueColumns)

	args := append([]any{n}, spec.extraArgs...)
	rows, err := db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", spec.spanName, err)
	}
	defer rows.Close()

	runs := make([]domain.JobRun, 0, n)
	for rows.Next() {
		run, err := dbscan.ScanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("%s scan: %w", spec.spanName, err)
		}
		runs = append(runs, *run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s rows: %w", spec.spanName, err)
	}
	for i := range runs {
		q.recordClaimMetrics(ctx, &runs[i])
	}
	if spec.postScanFn != nil {
		if err := spec.postScanFn(runs); err != nil {
			return runs, err
		}
	}
	return runs, nil
}
