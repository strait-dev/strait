package queue

import (
	"context"
	"fmt"

	"strait/internal/dbscan"
	"strait/internal/domain"
	"strait/internal/store"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

type dequeueSpec struct {
	spanName            string
	candidatesSQL       string
	extraArgs           []any
	skipConcurrencyCTEs bool
	postScanFn          func(runs []domain.JobRun) error
}

func withStatementTimeout(ctx context.Context, q *PostgresQueue, spanName string) (store.DBTX, pgx.Tx, error) {
	if q.statementTimeout <= 0 {
		return q.db, nil, nil
	}
	beginner, ok := q.db.(store.TxBeginner)
	if !ok {
		return q.db, nil, nil
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
	return tx, tx, nil
}

func executeDequeue(ctx context.Context, q *PostgresQueue, n int, spec dequeueSpec) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, spec.spanName)
	defer span.End()

	if n <= 0 {
		return nil, nil
	}

	db, tx, err := withStatementTimeout(ctx, q, spec.spanName)
	if err != nil {
		return nil, err
	}
	if tx != nil {
		defer tx.Rollback(ctx) //nolint:errcheck
	}

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
	if tx != nil {
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("%s commit: %w", spec.spanName, err)
		}
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

	db, tx, err := withStatementTimeout(ctx, q, spec.spanName)
	if err != nil {
		return nil, err
	}
	if tx != nil {
		defer tx.Rollback(ctx) //nolint:errcheck
	}

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
	if tx != nil {
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("%s commit: %w", spec.spanName, err)
		}
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

// executeDequeueTwoPhase claims IDs in a thin scan (RETURNING id only),
// then batch-fetches the full rows by PK in a second query. Both run
// in the same transaction. The B-tree scan in phase 1 never deserializes
// the 38-column fat rows; the PK fetch in phase 2 is a guaranteed buffer-
// cache hit because the UPDATE just touched those pages.
func executeDequeueTwoPhase(ctx context.Context, q *PostgresQueue, n int, spec dequeueSpec) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, spec.spanName)
	defer span.End()

	if n <= 0 {
		return nil, nil
	}

	// Must use a transaction: both phases share the same snapshot.
	beginner, ok := q.db.(store.TxBeginner)
	if !ok {
		// Fallback to single-phase if we can't begin a transaction.
		return executeDequeue(ctx, q, n, spec)
	}
	tx, err := beginner.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: begin tx: %w", spec.spanName, err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if q.statementTimeout > 0 {
		ms := int(q.statementTimeout.Milliseconds())
		if _, err := tx.Exec(ctx, fmt.Sprintf("SET LOCAL statement_timeout = %d", ms)); err != nil {
			return nil, fmt.Errorf("%s: set statement timeout: %w", spec.spanName, err)
		}
	}

	// Phase 1: thin claim -- UPDATE RETURNING id only.
	claimSQL := fmt.Sprintf(`
		WITH claimed AS (%s)
		UPDATE job_runs
		SET status = '%s', started_at = NOW()
		WHERE id IN (SELECT id FROM claimed)
		RETURNING id`, spec.candidatesSQL, domain.StatusDequeued)

	args := append([]any{n}, spec.extraArgs...)
	rows, err := tx.Query(ctx, claimSQL, args...)
	if err != nil {
		return nil, fmt.Errorf("%s claim: %w", spec.spanName, err)
	}

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, fmt.Errorf("%s claim scan: %w", spec.spanName, err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s claim rows: %w", spec.spanName, err)
	}

	if len(ids) == 0 {
		_ = tx.Commit(ctx)
		if spec.postScanFn != nil {
			_ = spec.postScanFn(nil)
		}
		return nil, nil
	}

	// Phase 2: fat fetch by PK. The rows were just UPDATEd so they
	// are hot in the buffer cache -- no disk I/O.
	fetchSQL := fmt.Sprintf(`SELECT %s FROM job_runs WHERE id = ANY($1) ORDER BY created_at ASC`, dequeueColumns)
	fetchRows, err := tx.Query(ctx, fetchSQL, ids)
	if err != nil {
		return nil, fmt.Errorf("%s fetch: %w", spec.spanName, err)
	}
	defer fetchRows.Close()

	runs := make([]domain.JobRun, 0, len(ids))
	for fetchRows.Next() {
		run, err := dbscan.ScanRun(fetchRows)
		if err != nil {
			return nil, fmt.Errorf("%s fetch scan: %w", spec.spanName, err)
		}
		runs = append(runs, *run)
	}
	if err := fetchRows.Err(); err != nil {
		return nil, fmt.Errorf("%s fetch rows: %w", spec.spanName, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("%s commit: %w", spec.spanName, err)
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
