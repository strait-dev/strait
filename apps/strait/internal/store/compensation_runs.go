package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"strait/internal/dbscan"
	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

func (q *Queries) CreateCompensationRun(ctx context.Context, run *domain.CompensationRun) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateCompensationRun")
	defer span.End()

	if run == nil {
		return fmt.Errorf("compensation run is nil")
	}
	if run.Status == "" {
		run.Status = domain.CompensationPending
	}
	_, err := q.db.Exec(ctx, `
		INSERT INTO compensation_runs (
			id, workflow_run_id, step_run_id, step_ref, compensation_job_id,
			job_run_id, status, input, output, error, started_at, finished_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, run.ID, run.WorkflowRunID, run.StepRunID, run.StepRef, run.CompensationJobID,
		dbscan.NilIfEmptyString(run.JobRunID), run.Status, dbscan.NilIfEmptyRawMessage(run.Input),
		dbscan.NilIfEmptyRawMessage(run.Output), dbscan.NilIfEmptyString(run.Error), run.StartedAt, run.FinishedAt)
	if err != nil {
		return fmt.Errorf("create compensation run: %w", err)
	}
	return nil
}

func (q *Queries) MarkCompensationRunStarted(ctx context.Context, id, jobRunID string, startedAt time.Time) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.MarkCompensationRunStarted")
	defer span.End()

	tag, err := q.db.Exec(ctx, `
		UPDATE compensation_runs
		SET status = $3, job_run_id = $2, started_at = COALESCE(started_at, $4)
		WHERE id = $1
		  AND status IN ($5, $3)
	`, id, jobRunID, domain.CompensationRunning, startedAt, domain.CompensationPending)
	if err != nil {
		return fmt.Errorf("mark compensation run started: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("compensation run not found or already terminal: %s", id)
	}
	return nil
}

func (q *Queries) MarkCompensationRunTerminalByJobRunID(
	ctx context.Context,
	jobRunID string,
	status string,
	output json.RawMessage,
	errMsg string,
	finishedAt time.Time,
) (*domain.CompensationRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.MarkCompensationRunTerminalByJobRunID")
	defer span.End()

	row := q.db.QueryRow(ctx, `
		UPDATE compensation_runs
		SET status = $2,
		    output = $3,
		    error = $4,
		    finished_at = COALESCE(finished_at, $5)
		WHERE job_run_id = $1
		  AND status IN ($6, $7, $2)
		RETURNING id, workflow_run_id, step_run_id, step_ref, compensation_job_id,
		          job_run_id, status, input, output, error, started_at, finished_at, created_at
	`, jobRunID, status, dbscan.NilIfEmptyRawMessage(output), dbscan.NilIfEmptyString(errMsg),
		finishedAt, domain.CompensationPending, domain.CompensationRunning)
	run, err := scanCompensationRun(row)
	if err != nil {
		return nil, fmt.Errorf("mark compensation run terminal: %w", err)
	}
	return run, nil
}

func (q *Queries) CountIncompleteCompensationRuns(ctx context.Context, workflowRunID string) (int, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CountIncompleteCompensationRuns")
	defer span.End()

	var count int
	if err := q.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM compensation_runs
		WHERE workflow_run_id = $1
		  AND status <> $2
	`, workflowRunID, domain.CompensationCompleted).Scan(&count); err != nil {
		return 0, fmt.Errorf("count incomplete compensation runs: %w", err)
	}
	return count, nil
}

func scanCompensationRun(row pgx.Row) (*domain.CompensationRun, error) {
	var run domain.CompensationRun
	var jobRunID *string
	var input, output []byte
	var errMsg *string
	if err := row.Scan(
		&run.ID,
		&run.WorkflowRunID,
		&run.StepRunID,
		&run.StepRef,
		&run.CompensationJobID,
		&jobRunID,
		&run.Status,
		&input,
		&output,
		&errMsg,
		&run.StartedAt,
		&run.FinishedAt,
		&run.CreatedAt,
	); err != nil {
		return nil, err
	}
	if jobRunID != nil {
		run.JobRunID = *jobRunID
	}
	if len(input) > 0 {
		run.Input = json.RawMessage(input)
	}
	if len(output) > 0 {
		run.Output = json.RawMessage(output)
	}
	if errMsg != nil {
		run.Error = *errMsg
	}
	return &run, nil
}
