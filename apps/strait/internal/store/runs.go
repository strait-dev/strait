package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"strait/internal/dbscan"
	"strait/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/samber/lo"
	"go.opentelemetry.io/otel"
)

func (q *Queries) CreateRun(ctx context.Context, run *domain.JobRun) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateRun")
	defer span.End()

	if run.ID == "" {
		run.ID = uuid.Must(uuid.NewV7()).String()
	}

	if run.Attempt == 0 {
		run.Attempt = 1
	}

	if run.TriggeredBy == "" {
		run.TriggeredBy = domain.TriggerManual
	}

	if run.Status == "" || run.Status == domain.StatusQueued {
		run.Status = domain.StatusQueued
		if run.ScheduledAt != nil && run.ScheduledAt.After(time.Now()) {
			run.Status = domain.StatusDelayed
		}
	}

	query := `
		WITH idempotency_check AS (
			SELECT 1 FROM job_runs
			WHERE job_id = $2
			  AND idempotency_key = $18
			  AND idempotency_key IS NOT NULL
			  AND status IN ('delayed', 'queued', 'dequeued', 'executing', 'waiting')
			LIMIT 1
		)
		INSERT INTO job_runs (
			id, job_id, project_id, status, attempt, payload, result, error,
			triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
			next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, workflow_step_run_id,
			debug_mode, continuation_of, lineage_depth, tags, job_version_id, created_by, concurrency_key, batch_id,
			execution_mode, machine_id, deployment_id, pinned_image_uri, pinned_image_digest, is_rollback
		)
		SELECT
			$1, $2, $3, $4, $5, $6, $7, $8,
			$9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20,
			$21, $22, $23, $24::jsonb, $25, $26, $27, $28,
			$29, $30, $31, $32, $33
		WHERE NOT EXISTS (SELECT 1 FROM idempotency_check)
		RETURNING created_at`

	execMode := run.ExecutionMode
	if execMode == "" {
		execMode = domain.ExecutionModeHTTP
	}

	err := q.db.QueryRow(
		ctx,
		query,
		run.ID,
		run.JobID,
		run.ProjectID,
		run.Status,
		run.Attempt,
		dbscan.NilIfEmptyRawMessage(run.Payload),
		dbscan.NilIfEmptyRawMessage(run.Result),
		dbscan.NilIfEmptyString(run.Error),
		run.TriggeredBy,
		run.ScheduledAt,
		run.StartedAt,
		run.FinishedAt,
		run.HeartbeatAt,
		run.NextRetryAt,
		run.ExpiresAt,
		dbscan.NilIfEmptyString(run.ParentRunID),
		run.Priority,
		dbscan.NilIfEmptyString(run.IdempotencyKey),
		run.JobVersion,
		dbscan.NilIfEmptyString(run.WorkflowStepRunID),
		run.DebugMode,
		dbscan.NilIfEmptyString(run.ContinuationOf),
		run.LineageDepth,
		marshalTagsForRun(run.Tags),
		dbscan.NilIfEmptyString(run.JobVersionID),
		dbscan.NilIfEmptyString(run.CreatedBy),
		dbscan.NilIfEmptyString(run.ConcurrencyKey),
		dbscan.NilIfEmptyString(run.BatchID),
		string(execMode),
		dbscan.NilIfEmptyString(run.MachineID),
		dbscan.NilIfEmptyString(run.DeploymentID),
		dbscan.NilIfEmptyString(run.PinnedImageURI),
		dbscan.NilIfEmptyString(run.PinnedImageDigest),
	).Scan(&run.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) && run.IdempotencyKey != "" {
			return domain.ErrIdempotencyConflict
		}
		return fmt.Errorf("create run: %w", err)
	}

	return nil
}

func (q *Queries) GetRun(ctx context.Context, id string) (*domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetRun")
	defer span.End()

	query := `
		SELECT id, job_id, project_id, status, attempt, payload, result, metadata, error, error_class,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id, execution_trace, debug_mode, continuation_of, lineage_depth, tags, job_version_id, created_by, batch_id, concurrency_key, execution_mode, machine_id, deployment_id, pinned_image_uri, pinned_image_digest, is_rollback
		FROM job_runs
		WHERE id = $1`

	run, err := dbscan.ScanRun(q.db.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRunNotFound
		}
		return nil, fmt.Errorf("get run: %w", err)
	}

	return run, nil
}

func (q *Queries) GetRunByIdempotencyKey(ctx context.Context, jobID, idempotencyKey string) (*domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetRunByIdempotencyKey")
	defer span.End()

	// Only return runs in non-terminal statuses to match the DB partial
	// unique index (idx_runs_idempotency). This allows idempotency key
	// reuse after a run reaches a terminal state.
	query := `
		SELECT id, job_id, project_id, status, attempt, payload, result, metadata, error, error_class,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id, execution_trace, debug_mode, continuation_of, lineage_depth, tags, job_version_id, created_by, batch_id, concurrency_key, execution_mode, machine_id, deployment_id, pinned_image_uri, pinned_image_digest, is_rollback
		FROM job_runs
		WHERE job_id = $1
		  AND idempotency_key = $2
		  AND (
		    status IN ('delayed', 'queued', 'dequeued', 'executing', 'waiting')
		    OR (status IN ('completed', 'failed', 'timed_out', 'crashed', 'system_failed', 'canceled') AND finished_at > NOW() - INTERVAL '24 hours')
		  )
		ORDER BY created_at DESC
		LIMIT 1`

	run, err := dbscan.ScanRun(q.db.QueryRow(ctx, query, jobID, idempotencyKey))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get run by idempotency key: %w", err)
	}

	return run, nil
}

func (q *Queries) FindRecentRunByPayload(ctx context.Context, jobID string, payload json.RawMessage, since time.Time) (*domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.FindRecentRunByPayload")
	defer span.End()

	query := `
		SELECT id, job_id, project_id, status, attempt, payload, result, metadata, error, error_class,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id, execution_trace, debug_mode, continuation_of, lineage_depth, tags, job_version_id, created_by, batch_id, concurrency_key, execution_mode, machine_id, deployment_id, pinned_image_uri, pinned_image_digest, is_rollback
		FROM job_runs
		WHERE job_id = $1
		  AND payload = $2::jsonb
		  AND created_at >= $3
		ORDER BY created_at DESC
		LIMIT 1`

	run, err := dbscan.ScanRun(q.db.QueryRow(ctx, query, jobID, dbscan.NilIfEmptyRawMessage(payload), since))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("find recent run by payload: %w", err)
	}

	return run, nil
}

func (q *Queries) CountRunsForJobSince(ctx context.Context, jobID string, since time.Time) (int, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CountRunsForJobSince")
	defer span.End()

	query := `
		SELECT COUNT(*)
		FROM job_runs
		WHERE job_id = $1
		  AND created_at >= $2`

	var count int
	if err := q.db.QueryRow(ctx, query, jobID, since).Scan(&count); err != nil {
		return 0, fmt.Errorf("count runs for job since: %w", err)
	}

	return count, nil
}

// GetJobHealthStats returns aggregated health metrics for a job's runs over a given window.
func (q *Queries) GetJobHealthStats(ctx context.Context, jobID string, since time.Time) (*JobHealthStats, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetJobHealthStats")
	defer span.End()

	query := `
		SELECT
			COUNT(*) AS total_runs,
			COUNT(*) FILTER (WHERE status = 'completed') AS completed_runs,
			COUNT(*) FILTER (WHERE status = 'failed') AS failed_runs,
			COUNT(*) FILTER (WHERE status = 'timed_out') AS timed_out_runs,
			COUNT(*) FILTER (WHERE status IN ('crashed', 'system_failed')) AS crashed_runs,
			COUNT(*) FILTER (WHERE status = 'canceled') AS canceled_runs,
			COUNT(*) FILTER (WHERE status = 'expired') AS expired_runs,
			COALESCE(
				AVG(EXTRACT(EPOCH FROM (finished_at - started_at))) FILTER (WHERE finished_at IS NOT NULL AND started_at IS NOT NULL),
				0
			) AS avg_duration_secs,
			COALESCE(
				PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (finished_at - started_at))) FILTER (WHERE finished_at IS NOT NULL AND started_at IS NOT NULL),
				0
			) AS p95_duration_secs,
			COALESCE(
				PERCENTILE_CONT(0.99) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (finished_at - started_at))) FILTER (WHERE finished_at IS NOT NULL AND started_at IS NOT NULL),
				0
			) AS p99_duration_secs
		FROM job_runs
		WHERE job_id = $1
			AND created_at >= $2
			AND status IN ('completed', 'failed', 'timed_out', 'crashed', 'system_failed', 'canceled', 'expired')`

	stats := &JobHealthStats{}
	err := q.db.QueryRow(ctx, query, jobID, since).Scan(
		&stats.TotalRuns,
		&stats.CompletedRuns,
		&stats.FailedRuns,
		&stats.TimedOutRuns,
		&stats.CrashedRuns,
		&stats.CanceledRuns,
		&stats.ExpiredRuns,
		&stats.AvgDurationSecs,
		&stats.P95DurationSecs,
		&stats.P99DurationSecs,
	)
	if err != nil {
		return nil, fmt.Errorf("get job health stats: %w", err)
	}

	if stats.TotalRuns > 0 {
		stats.SuccessRate = float64(stats.CompletedRuns) / float64(stats.TotalRuns) * 100

		// Health score: 70% success rate + 30% duration stability (0-100).
		successComponent := stats.SuccessRate * 0.7
		stabilityComponent := 0.0 // default: no duration data = no stability credit
		if stats.AvgDurationSecs > 0 {
			stabilityComponent = 30.0 // full credit for stable durations
			if stats.P95DurationSecs > 2*stats.AvgDurationSecs {
				// Penalize high variance: ratio > 2 means unstable.
				ratio := stats.P95DurationSecs / stats.AvgDurationSecs
				penalty := min((ratio-2)*15, 30) // max 30 point penalty
				stabilityComponent = max(0, 30-penalty)
			}
		}
		stats.HealthScore = min(100, successComponent+stabilityComponent)
	} else {
		stats.HealthScore = -1 // unknown
	}

	return stats, nil
}

func (q *Queries) CreateRunCheckpoint(ctx context.Context, checkpoint *domain.RunCheckpoint) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateRunCheckpoint")
	defer span.End()

	if checkpoint.ID == "" {
		checkpoint.ID = uuid.Must(uuid.NewV7()).String()
	}
	if checkpoint.Source == "" {
		checkpoint.Source = "sdk"
	}

	query := `
		WITH next_seq AS (
			SELECT COALESCE(MAX(sequence), 0) + 1 AS seq
			FROM run_checkpoints
			WHERE run_id = $1
		)
		INSERT INTO run_checkpoints (id, run_id, sequence, source, state)
		VALUES ($2, $1, (SELECT seq FROM next_seq), $3, $4)
		RETURNING sequence, created_at`

	err := q.db.QueryRow(
		ctx,
		query,
		checkpoint.RunID,
		checkpoint.ID,
		checkpoint.Source,
		dbscan.NilIfEmptyRawMessage(checkpoint.State),
	).Scan(&checkpoint.Sequence, &checkpoint.CreatedAt)
	if err != nil {
		return fmt.Errorf("create run checkpoint: %w", err)
	}

	return nil
}

func (q *Queries) ListRunCheckpoints(ctx context.Context, runID string, limit int, cursor *time.Time) ([]domain.RunCheckpoint, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListRunCheckpoints")
	defer span.End()

	if limit <= 0 {
		limit = 50
	}

	query := `
		SELECT id, run_id, sequence, source, state, created_at
		FROM run_checkpoints
		WHERE run_id = $1`

	args := []any{runID}
	param := 2

	if cursor != nil {
		query += fmt.Sprintf(" AND created_at < $%d", param)
		args = append(args, *cursor)
		param++
	}

	query += fmt.Sprintf(" ORDER BY sequence DESC LIMIT $%d", param)
	args = append(args, limit)

	rows, err := q.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list run checkpoints: %w", err)
	}
	defer rows.Close()

	checkpoints := make([]domain.RunCheckpoint, 0)
	for rows.Next() {
		cp, scanErr := scanRunCheckpoint(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list run checkpoints scan: %w", scanErr)
		}
		checkpoints = append(checkpoints, *cp)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list run checkpoints rows: %w", err)
	}

	return checkpoints, nil
}

func (q *Queries) GetLatestCheckpoint(ctx context.Context, runID string) (*domain.RunCheckpoint, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetLatestCheckpoint")
	defer span.End()

	query := `
		SELECT id, run_id, sequence, source, state, created_at
		FROM run_checkpoints
		WHERE run_id = $1
		ORDER BY sequence DESC
		LIMIT 1`

	row := q.db.QueryRow(ctx, query, runID)
	cp, err := scanRunCheckpoint(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get latest checkpoint: %w", err)
	}

	return cp, nil
}

func (q *Queries) CreateRunUsage(ctx context.Context, usage *domain.RunUsage) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateRunUsage")
	defer span.End()

	if usage.ID == "" {
		usage.ID = uuid.Must(uuid.NewV7()).String()
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	if usage.CostMicrousd == 0 {
		inputCost, outputCost, err := q.lookupPricing(ctx, usage.Provider, usage.Model)
		if err != nil {
			return err
		}
		if inputCost > 0 || outputCost > 0 {
			usage.CostMicrousd = int64(usage.PromptTokens)*inputCost + int64(usage.CompletionTokens)*outputCost
		}
	}

	query := `
		INSERT INTO run_usage (id, run_id, provider, model, prompt_tokens, completion_tokens, total_tokens, cost_microusd)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at`

	err := q.db.QueryRow(
		ctx,
		query,
		usage.ID,
		usage.RunID,
		usage.Provider,
		usage.Model,
		usage.PromptTokens,
		usage.CompletionTokens,
		usage.TotalTokens,
		usage.CostMicrousd,
	).Scan(&usage.CreatedAt)
	if err != nil {
		return fmt.Errorf("create run usage: %w", err)
	}

	return nil
}

func (q *Queries) ListRunUsage(ctx context.Context, runID string, limit int, cursor *time.Time) ([]domain.RunUsage, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListRunUsage")
	defer span.End()

	if limit <= 0 {
		limit = 100
	}

	query := `
		SELECT id, run_id, provider, model, prompt_tokens, completion_tokens, total_tokens, cost_microusd, created_at
		FROM run_usage
		WHERE run_id = $1`

	args := []any{runID}
	param := 2

	if cursor != nil {
		query += fmt.Sprintf(" AND created_at < $%d", param)
		args = append(args, *cursor)
		param++
	}

	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", param)
	args = append(args, limit)

	rows, err := q.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list run usage: %w", err)
	}
	defer rows.Close()

	usages := make([]domain.RunUsage, 0)
	for rows.Next() {
		u, scanErr := scanRunUsage(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list run usage scan: %w", scanErr)
		}
		usages = append(usages, *u)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list run usage rows: %w", err)
	}

	return usages, nil
}

func (q *Queries) CreateRunToolCall(ctx context.Context, call *domain.RunToolCall) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateRunToolCall")
	defer span.End()

	if call.ID == "" {
		call.ID = uuid.Must(uuid.NewV7()).String()
	}
	if call.Status == "" {
		call.Status = "completed"
	}

	query := `
		INSERT INTO run_tool_calls (id, run_id, tool_name, input, output, duration_ms, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING created_at`

	err := q.db.QueryRow(
		ctx,
		query,
		call.ID,
		call.RunID,
		call.ToolName,
		dbscan.NilIfEmptyRawMessage(call.Input),
		dbscan.NilIfEmptyRawMessage(call.Output),
		dbscan.NilIfZeroInt(call.DurationMs),
		call.Status,
	).Scan(&call.CreatedAt)
	if err != nil {
		return fmt.Errorf("create run tool call: %w", err)
	}

	return nil
}

func (q *Queries) ListRunToolCalls(ctx context.Context, runID string, limit int, cursor *time.Time) ([]domain.RunToolCall, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListRunToolCalls")
	defer span.End()

	if limit <= 0 {
		limit = 100
	}

	query := `
		SELECT id, run_id, tool_name, input, output, duration_ms, status, created_at
		FROM run_tool_calls
		WHERE run_id = $1`

	args := []any{runID}
	param := 2

	if cursor != nil {
		query += fmt.Sprintf(" AND created_at < $%d", param)
		args = append(args, *cursor)
		param++
	}

	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", param)
	args = append(args, limit)

	rows, err := q.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list run tool calls: %w", err)
	}
	defer rows.Close()

	calls := make([]domain.RunToolCall, 0)
	for rows.Next() {
		c, scanErr := scanRunToolCall(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list run tool calls scan: %w", scanErr)
		}
		calls = append(calls, *c)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list run tool calls rows: %w", err)
	}

	return calls, nil
}

func (q *Queries) UpsertRunOutput(ctx context.Context, output *domain.RunOutput) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpsertRunOutput")
	defer span.End()

	if output.ID == "" {
		output.ID = uuid.Must(uuid.NewV7()).String()
	}

	query := `
		INSERT INTO run_outputs (id, run_id, output_key, schema, value)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (run_id, output_key)
		DO UPDATE SET schema = EXCLUDED.schema, value = EXCLUDED.value, created_at = NOW()
		RETURNING created_at`

	err := q.db.QueryRow(
		ctx,
		query,
		output.ID,
		output.RunID,
		output.OutputKey,
		dbscan.NilIfEmptyRawMessage(output.Schema),
		dbscan.NilIfEmptyRawMessage(output.Value),
	).Scan(&output.CreatedAt)
	if err != nil {
		return fmt.Errorf("upsert run output: %w", err)
	}

	return nil
}

func (q *Queries) ListRunOutputs(ctx context.Context, runID string, limit int, cursor *time.Time) ([]domain.RunOutput, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListRunOutputs")
	defer span.End()

	query := `
		SELECT id, run_id, output_key, schema, value, created_at
		FROM run_outputs
		WHERE run_id = $1`

	args := []any{runID}
	param := 2

	if cursor != nil {
		query += fmt.Sprintf(" AND created_at < $%d", param)
		args = append(args, *cursor)
		param++
	}

	query += fmt.Sprintf(" ORDER BY output_key ASC LIMIT $%d", param)
	args = append(args, limit)

	rows, err := q.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list run outputs: %w", err)
	}
	defer rows.Close()

	outputs := make([]domain.RunOutput, 0)
	for rows.Next() {
		o, scanErr := scanRunOutput(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list run outputs scan: %w", scanErr)
		}
		outputs = append(outputs, *o)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list run outputs rows: %w", err)
	}

	return outputs, nil
}

func (q *Queries) AreAllDescendantsTerminal(ctx context.Context, parentRunID string) (bool, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.AreAllDescendantsTerminal")
	defer span.End()

	query := `
		WITH RECURSIVE descendants AS (
			SELECT id, status, 1 AS depth
			FROM job_runs
			WHERE parent_run_id = $1
			UNION ALL
			SELECT jr.id, jr.status, d.depth + 1
			FROM job_runs jr
			JOIN descendants d ON jr.parent_run_id = d.id
			WHERE d.depth < 100
		)
		SELECT COUNT(*)
		FROM descendants
		WHERE status NOT IN ('completed', 'failed', 'timed_out', 'crashed', 'system_failed', 'canceled', 'expired')`

	var nonTerminalCount int
	if err := q.db.QueryRow(ctx, query, parentRunID).Scan(&nonTerminalCount); err != nil {
		return false, fmt.Errorf("check descendants terminal: %w", err)
	}

	return nonTerminalCount == 0, nil
}

func (q *Queries) lookupPricing(ctx context.Context, provider, model string) (int64, int64, error) {
	query := `
		SELECT input_cost_microusd, output_cost_microusd
		FROM pricing_catalog
		WHERE provider = $1 AND model = $2 AND active = TRUE
		ORDER BY effective_from DESC
		LIMIT 1`

	var inputCost int64
	var outputCost int64
	err := q.db.QueryRow(ctx, query, provider, model).Scan(&inputCost, &outputCost)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, 0, nil
		}
		return 0, 0, fmt.Errorf("lookup pricing: %w", err)
	}

	return inputCost, outputCost, nil
}

// SumRunCostMicrousd returns the total cost of all usage records for a single run.
func (q *Queries) SumRunCostMicrousd(ctx context.Context, runID string) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.SumRunCostMicrousd")
	defer span.End()

	query := `SELECT COALESCE(SUM(cost_microusd), 0) FROM run_usage WHERE run_id = $1`
	var total int64
	if err := q.db.QueryRow(ctx, query, runID).Scan(&total); err != nil {
		return 0, fmt.Errorf("sum run cost: %w", err)
	}
	return total, nil
}

// SumProjectDailyCostMicrousd returns the total cost of all usage records for a project today.
func (q *Queries) SumProjectDailyCostMicrousd(ctx context.Context, projectID string, timezone string) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.SumProjectDailyCostMicrousd")
	defer span.End()

	tz := timezone
	if tz == "" {
		tz = "UTC"
	}

	query := `
		SELECT COALESCE(SUM(u.cost_microusd), 0)
		FROM run_usage u
		JOIN job_runs r ON u.run_id = r.id
		WHERE r.project_id = $1
		  AND u.created_at >= date_trunc('day', NOW() AT TIME ZONE $2)
	`
	var total int64
	if err := q.db.QueryRow(ctx, query, projectID, tz).Scan(&total); err != nil {
		return 0, fmt.Errorf("sum project daily cost: %w", err)
	}
	return total, nil
}

func scanRunCheckpoint(scanner scanTarget) (*domain.RunCheckpoint, error) {
	var cp domain.RunCheckpoint
	var state []byte
	err := scanner.Scan(&cp.ID, &cp.RunID, &cp.Sequence, &cp.Source, &state, &cp.CreatedAt)
	if err != nil {
		return nil, err
	}
	if state != nil {
		cp.State = json.RawMessage(state)
	}
	return &cp, nil
}

func scanRunUsage(scanner scanTarget) (*domain.RunUsage, error) {
	var usage domain.RunUsage
	err := scanner.Scan(
		&usage.ID,
		&usage.RunID,
		&usage.Provider,
		&usage.Model,
		&usage.PromptTokens,
		&usage.CompletionTokens,
		&usage.TotalTokens,
		&usage.CostMicrousd,
		&usage.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &usage, nil
}

func scanRunToolCall(scanner scanTarget) (*domain.RunToolCall, error) {
	var call domain.RunToolCall
	var input []byte
	var output []byte
	var durationMs *int
	err := scanner.Scan(&call.ID, &call.RunID, &call.ToolName, &input, &output, &durationMs, &call.Status, &call.CreatedAt)
	if err != nil {
		return nil, err
	}
	if input != nil {
		call.Input = json.RawMessage(input)
	}
	if output != nil {
		call.Output = json.RawMessage(output)
	}
	if durationMs != nil {
		call.DurationMs = *durationMs
	}
	return &call, nil
}

func scanRunOutput(scanner scanTarget) (*domain.RunOutput, error) {
	var output domain.RunOutput
	var schema []byte
	var value []byte
	err := scanner.Scan(&output.ID, &output.RunID, &output.OutputKey, &schema, &value, &output.CreatedAt)
	if err != nil {
		return nil, err
	}
	if schema != nil {
		output.Schema = json.RawMessage(schema)
	}
	if value != nil {
		output.Value = json.RawMessage(value)
	}
	return &output, nil
}

func (q *Queries) ListRunsByJob(ctx context.Context, jobID string, limit, offset int) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListRunsByJob")
	defer span.End()

	query := `
		SELECT id, job_id, project_id, status, attempt, payload, result, metadata, error, error_class,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id, execution_trace, debug_mode, continuation_of, lineage_depth, tags, job_version_id, created_by, batch_id, concurrency_key, execution_mode, machine_id, deployment_id, pinned_image_uri, pinned_image_digest, is_rollback
		FROM job_runs
		WHERE job_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`

	rows, err := q.db.Query(ctx, query, jobID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list runs by job: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.JobRun, 0, limit)
	for rows.Next() {
		run, err := dbscan.ScanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("list runs by job scan: %w", err)
		}
		runs = append(runs, *run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list runs by job rows: %w", err)
	}

	return runs, nil
}

func (q *Queries) ListRunsByProject(ctx context.Context, projectID string, status *domain.RunStatus, metadataKey, metadataValue, triggeredBy, batchID *string, payloadContains json.RawMessage, executionMode *domain.ExecutionMode, errorClass *string, limit int, cursor *time.Time) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListRunsByProject")
	defer span.End()

	baseQuery := `
		SELECT id, job_id, project_id, status, attempt, payload, result, metadata, error, error_class,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id, execution_trace, debug_mode, continuation_of, lineage_depth, tags, job_version_id, created_by, batch_id, concurrency_key, execution_mode, machine_id, deployment_id, pinned_image_uri, pinned_image_digest, is_rollback
		FROM job_runs
		WHERE project_id = $1`

	args := []any{projectID}
	param := 2

	if status != nil {
		baseQuery += fmt.Sprintf(" AND status = $%d", param)
		args = append(args, *status)
		param++
	}

	if metadataKey != nil {
		if metadataValue == nil {
			baseQuery += fmt.Sprintf(" AND metadata ? $%d", param)
			args = append(args, *metadataKey)
			param++
		} else {
			baseQuery += fmt.Sprintf(" AND metadata ->> $%d = $%d", param, param+1)
			args = append(args, *metadataKey, *metadataValue)
			param += 2
		}
	}

	if triggeredBy != nil {
		baseQuery += fmt.Sprintf(" AND triggered_by = $%d", param)
		args = append(args, *triggeredBy)
		param++
	}

	if batchID != nil {
		baseQuery += fmt.Sprintf(" AND batch_id = $%d", param)
		args = append(args, *batchID)
		param++
	}

	if len(payloadContains) > 0 {
		baseQuery += fmt.Sprintf(" AND payload @> $%d::jsonb", param)
		args = append(args, payloadContains)
		param++
	}

	if executionMode != nil {
		baseQuery += fmt.Sprintf(" AND execution_mode = $%d", param)
		args = append(args, string(*executionMode))
		param++
	}

	if errorClass != nil {
		baseQuery += fmt.Sprintf(" AND error_class = $%d", param)
		args = append(args, *errorClass)
		param++
	}

	if cursor != nil {
		baseQuery += fmt.Sprintf(" AND created_at < $%d", param)
		args = append(args, *cursor)
		param++
	}

	baseQuery += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", param)
	args = append(args, limit)

	rows, err := q.db.Query(ctx, baseQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("list runs by project: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.JobRun, 0, limit)
	for rows.Next() {
		run, err := dbscan.ScanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("list runs by project scan: %w", err)
		}
		runs = append(runs, *run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list runs by project rows: %w", err)
	}

	return runs, nil
}

func (q *Queries) ListFinishedRunsSince(ctx context.Context, projectID string, since time.Time, sinceRunID string, limit int) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListFinishedRunsSince")
	defer span.End()

	if limit <= 0 {
		limit = 100
	}

	query := `
		SELECT id, job_id, project_id, status, attempt, payload, result, metadata, error, error_class,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id, execution_trace, debug_mode, continuation_of, lineage_depth, tags, job_version_id, created_by, batch_id, concurrency_key, execution_mode, machine_id, deployment_id, pinned_image_uri, pinned_image_digest, is_rollback
		FROM job_runs
		WHERE project_id = $1
		  AND status IN ('completed', 'failed', 'timed_out', 'crashed', 'system_failed', 'canceled', 'expired')
		  AND (finished_at > $2 OR (finished_at = $2 AND id > $3))
		ORDER BY finished_at ASC, id ASC
		LIMIT $4
	`

	rows, err := q.db.Query(ctx, query, projectID, since, sinceRunID, limit)
	if err != nil {
		return nil, fmt.Errorf("list finished runs since: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.JobRun, 0, limit)
	for rows.Next() {
		run, scanErr := dbscan.ScanRun(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list finished runs since scan: %w", scanErr)
		}
		runs = append(runs, *run)
	}
	return runs, rows.Err()
}

func (q *Queries) ListDeadLetterRuns(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListDeadLetterRuns")
	defer span.End()

	if limit <= 0 {
		limit = 50
	}

	query := `
		SELECT id, job_id, project_id, status, attempt, payload, result, metadata, error, error_class,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id, execution_trace, debug_mode, continuation_of, lineage_depth, tags, job_version_id, created_by, batch_id, concurrency_key, execution_mode, machine_id, deployment_id, pinned_image_uri, pinned_image_digest, is_rollback
		FROM job_runs
		WHERE project_id = $1 AND status = 'dead_letter'`

	args := []any{projectID}
	param := 2

	if cursor != nil {
		query += fmt.Sprintf(" AND created_at < $%d", param)
		args = append(args, *cursor)
		param++
	}

	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", param)
	args = append(args, limit)

	rows, err := q.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list dead letter runs: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.JobRun, 0)
	for rows.Next() {
		run, err := dbscan.ScanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("list dead letter runs scan: %w", err)
		}
		runs = append(runs, *run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list dead letter runs rows: %w", err)
	}

	return runs, nil
}

func (q *Queries) ReplayDeadLetterRun(ctx context.Context, runID string) (*domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ReplayDeadLetterRun")
	defer span.End()

	// Verify the run is currently in dead_letter status before attempting the
	// CAS transition. This prevents the idempotent path in UpdateRunStatus
	// from masking invalid replay attempts (e.g. replaying a queued run returns
	// nil because queued == queued target).
	run, err := q.GetRun(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("replay dead letter run: %w", err)
	}
	if run.Status != domain.StatusDeadLetter {
		return nil, fmt.Errorf("replay dead letter run: %w: run %s has status %s, expected dead_letter", ErrRunConflict, runID, run.Status)
	}

	// CAS transition to prevent concurrent replays from both succeeding.
	err = q.UpdateRunStatus(ctx, runID, domain.StatusDeadLetter, domain.StatusQueued, map[string]any{
		"attempt":       1,
		"error":         "",
		"started_at":    nil,
		"finished_at":   nil,
		"heartbeat_at":  nil,
		"next_retry_at": nil,
	})
	if err != nil {
		return nil, fmt.Errorf("replay dead letter run: %w", err)
	}

	updatedRun, err := q.GetRun(ctx, runID)
	if err != nil {
		return nil, err
	}

	return updatedRun, nil
}

func (q *Queries) UpdateRunStatus(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpdateRunStatus")
	defer span.End()

	if err := domain.ValidateTransition(from, to); err != nil {
		return fmt.Errorf("invalid status transition: %w", err)
	}

	allowedColumns := map[string]struct{}{
		"attempt":              {},
		"payload":              {},
		"result":               {},
		"error":                {},
		"error_class":          {},
		"triggered_by":         {},
		"scheduled_at":         {},
		"started_at":           {},
		"finished_at":          {},
		"heartbeat_at":         {},
		"next_retry_at":        {},
		"expires_at":           {},
		"execution_trace":      {},
		"workflow_step_run_id": {},
		"debug_mode":           {},
		"continuation_of":      {},
		"lineage_depth":        {},
		"priority":             {},
		"metadata":             {},
		"machine_id":           {},
	}

	setClauses := []string{"status = $1"}
	args := []any{to, id, from}
	param := 4

	keys := lo.Keys(fields)
	sort.Strings(keys)

	for _, key := range keys {
		if _, ok := allowedColumns[key]; !ok {
			return &domain.FieldError{Field: key}
		}

		value := fields[key]
		if raw, ok := value.(json.RawMessage); ok {
			value = dbscan.NilIfEmptyRawMessage(raw)
		}
		if key == "metadata" {
			if m, ok := value.(map[string]string); ok {
				encoded, err := json.Marshal(m)
				if err != nil {
					return fmt.Errorf("marshal metadata: %w", err)
				}
				setClauses = append(setClauses, fmt.Sprintf("metadata = COALESCE(metadata, '{}'::jsonb) || $%d::jsonb", param))
				args = append(args, encoded)
				param++
				continue
			}
		}
		if key == "execution_trace" {
			switch trace := value.(type) {
			case *domain.ExecutionTrace:
				if trace == nil {
					value = nil
				} else {
					encoded, err := json.Marshal(trace)
					if err != nil {
						return fmt.Errorf("marshal execution trace: %w", err)
					}
					value = dbscan.NilIfEmptyRawMessage(encoded)
				}
			case domain.ExecutionTrace:
				encoded, err := json.Marshal(trace)
				if err != nil {
					return fmt.Errorf("marshal execution trace: %w", err)
				}
				value = dbscan.NilIfEmptyRawMessage(encoded)
			}
		}
		if key == "error" || key == "workflow_step_run_id" {
			if text, ok := value.(string); ok {
				value = dbscan.NilIfEmptyString(text)
			}
		}

		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", key, param))
		args = append(args, value)
		param++
	}

	query := fmt.Sprintf(
		"UPDATE job_runs SET %s WHERE id = $2 AND status = $3",
		strings.Join(setClauses, ", "),
	)

	tag, err := q.db.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update run status: %w", err)
	}

	if tag.RowsAffected() == 0 {
		var currentStatus domain.RunStatus
		err := q.db.QueryRow(ctx, "SELECT status FROM job_runs WHERE id = $1", id).Scan(&currentStatus)
		if err != nil {
			return fmt.Errorf("checking current status: %w", err)
		}
		if currentStatus == to {
			return nil // idempotent: already in target state
		}
		return fmt.Errorf("%w: id %s from %s", ErrRunConflict, id, from)
	}

	return nil
}

func (q *Queries) UpdateRunMetadata(ctx context.Context, id string, annotations map[string]string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpdateRunMetadata")
	defer span.End()

	encoded, err := json.Marshal(annotations)
	if err != nil {
		return fmt.Errorf("marshal annotations: %w", err)
	}

	query := `
		UPDATE job_runs
		SET metadata = COALESCE(metadata, '{}'::jsonb) || $1::jsonb
		WHERE id = $2`

	tag, err := q.db.Exec(ctx, query, encoded, id)
	if err != nil {
		return fmt.Errorf("update run metadata: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", ErrRunNotFound, id)
	}

	return nil
}

func (q *Queries) UpdateHeartbeat(ctx context.Context, id string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpdateHeartbeat")
	defer span.End()

	query := `UPDATE job_runs SET heartbeat_at = NOW() WHERE id = $1`

	tag, err := q.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("update heartbeat: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", ErrRunNotFound, id)
	}

	return nil
}

func (q *Queries) ListStaleRuns(ctx context.Context, threshold time.Duration) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListStaleRuns")
	defer span.End()

	query := fmt.Sprintf(`
		SELECT id, job_id, project_id, status, attempt, payload, result, metadata, error, error_class,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id, execution_trace, debug_mode, continuation_of, lineage_depth, tags, job_version_id, created_by, batch_id, concurrency_key, execution_mode, machine_id, deployment_id, pinned_image_uri, pinned_image_digest, is_rollback
		FROM job_runs
		WHERE status = '%s' AND heartbeat_at < NOW() - $1::interval
		ORDER BY heartbeat_at ASC
		LIMIT 1000`, domain.StatusExecuting)

	rows, err := q.db.Query(ctx, query, threshold.String())
	if err != nil {
		return nil, fmt.Errorf("list stale runs: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.JobRun, 0, 16)
	for rows.Next() {
		run, err := dbscan.ScanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("list stale runs scan: %w", err)
		}
		runs = append(runs, *run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list stale runs rows: %w", err)
	}

	return runs, nil
}

func (q *Queries) ListDueRuns(ctx context.Context) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListDueRuns")
	defer span.End()

	query := fmt.Sprintf(`
		SELECT id, job_id, project_id, status, attempt, payload, result, metadata, error, error_class,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id, execution_trace, debug_mode, continuation_of, lineage_depth, tags, job_version_id, created_by, batch_id, concurrency_key, execution_mode, machine_id, deployment_id, pinned_image_uri, pinned_image_digest, is_rollback
		FROM job_runs
		WHERE status = '%s' AND scheduled_at <= NOW()
		ORDER BY scheduled_at ASC
		LIMIT 1000`, domain.StatusDelayed)

	rows, err := q.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list due runs: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.JobRun, 0, 16)
	for rows.Next() {
		run, err := dbscan.ScanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("list due runs scan: %w", err)
		}
		runs = append(runs, *run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list due runs rows: %w", err)
	}

	return runs, nil
}

func (q *Queries) ListExpiredRuns(ctx context.Context) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListExpiredRuns")
	defer span.End()

	query := fmt.Sprintf(`
		SELECT id, job_id, project_id, status, attempt, payload, result, metadata, error, error_class,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id, execution_trace, debug_mode, continuation_of, lineage_depth, tags, job_version_id, created_by, batch_id, concurrency_key, execution_mode, machine_id, deployment_id, pinned_image_uri, pinned_image_digest, is_rollback
		FROM job_runs
		WHERE status IN ('%s', '%s')
		  AND expires_at IS NOT NULL
		  AND expires_at <= NOW()
		ORDER BY expires_at ASC
		LIMIT 1000`, domain.StatusDelayed, domain.StatusQueued)

	rows, err := q.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list expired runs: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.JobRun, 0, 16)
	for rows.Next() {
		run, err := dbscan.ScanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("list expired runs scan: %w", err)
		}
		runs = append(runs, *run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list expired runs rows: %w", err)
	}

	return runs, nil
}

func (q *Queries) ListChildRuns(ctx context.Context, parentRunID string, limit int, cursor *time.Time) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListChildRuns")
	defer span.End()

	query := `
		SELECT id, job_id, project_id, status, attempt, payload, result, metadata, error, error_class,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id, execution_trace, debug_mode, continuation_of, lineage_depth, tags, job_version_id, created_by, batch_id, concurrency_key, execution_mode, machine_id, deployment_id, pinned_image_uri, pinned_image_digest, is_rollback
		FROM job_runs
		WHERE parent_run_id = $1`

	args := []any{parentRunID}
	param := 2

	if cursor != nil {
		query += fmt.Sprintf(" AND created_at < $%d", param)
		args = append(args, *cursor)
		param++
	}

	query += fmt.Sprintf(" ORDER BY created_at ASC LIMIT $%d", param)
	args = append(args, limit)

	rows, err := q.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list child runs: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.JobRun, 0, 16)
	for rows.Next() {
		run, err := dbscan.ScanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("list child runs scan: %w", err)
		}
		runs = append(runs, *run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list child runs rows: %w", err)
	}

	return runs, nil
}

func (q *Queries) ListStaleDequeued(ctx context.Context, threshold time.Duration) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListStaleDequeued")
	defer span.End()

	query := fmt.Sprintf(`
		SELECT id, job_id, project_id, status, attempt, payload, result, metadata, error, error_class,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id, execution_trace, debug_mode, continuation_of, lineage_depth, tags, job_version_id, created_by, batch_id, concurrency_key, execution_mode, machine_id, deployment_id, pinned_image_uri, pinned_image_digest, is_rollback
		FROM job_runs
		WHERE status = '%s' AND started_at < NOW() - $1::interval
		ORDER BY started_at ASC
		LIMIT 1000`, domain.StatusDequeued)

	rows, err := q.db.Query(ctx, query, threshold.String())
	if err != nil {
		return nil, fmt.Errorf("list stale dequeued runs: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.JobRun, 0, 16)
	for rows.Next() {
		run, err := dbscan.ScanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("list stale dequeued runs scan: %w", err)
		}
		runs = append(runs, *run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list stale dequeued runs rows: %w", err)
	}

	return runs, nil
}

func (q *Queries) DeleteTerminalRunsPastRetention(ctx context.Context, shortRetention, longRetention time.Duration) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteTerminalRunsPastRetention")
	defer span.End()

	shortCutoff := time.Now().Add(-shortRetention)
	longCutoff := time.Now().Add(-longRetention)

	query := `
		WITH to_delete AS (
			SELECT id FROM job_runs
			WHERE finished_at IS NOT NULL
			  AND (
				(status IN ('completed', 'failed', 'canceled', 'expired') AND finished_at <= $1)
				OR
				(status IN ('timed_out', 'crashed', 'system_failed') AND finished_at <= $2)
			  )
			LIMIT 5000
			FOR UPDATE SKIP LOCKED
		)
		DELETE FROM job_runs WHERE id IN (SELECT id FROM to_delete)`

	tag, err := q.db.Exec(ctx, query, shortCutoff, longCutoff)
	if err != nil {
		return 0, fmt.Errorf("delete terminal runs past retention: %w", err)
	}

	return tag.RowsAffected(), nil
}

// DLQJobDepth represents the dead-letter queue depth for a single job.
type DLQJobDepth struct {
	JobID             string
	WebhookURL        string
	DLQCount          int
	DLQAlertThreshold int
}

func (q *Queries) ListDLQDepthByJob(ctx context.Context) ([]DLQJobDepth, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListDLQDepthByJob")
	defer span.End()

	query := `
		SELECT jr.job_id, COALESCE(j.webhook_url, ''), COUNT(*) AS dlq_count, j.dlq_alert_threshold
		FROM job_runs jr
		JOIN jobs j ON j.id = jr.job_id
		WHERE jr.status = 'dead_letter'
		  AND j.dlq_alert_threshold IS NOT NULL
		GROUP BY jr.job_id, j.webhook_url, j.dlq_alert_threshold
		HAVING COUNT(*) >= j.dlq_alert_threshold`

	rows, err := q.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list dlq depth by job: %w", err)
	}
	defer rows.Close()

	var results []DLQJobDepth
	for rows.Next() {
		var d DLQJobDepth
		if err := rows.Scan(&d.JobID, &d.WebhookURL, &d.DLQCount, &d.DLQAlertThreshold); err != nil {
			return nil, fmt.Errorf("scan dlq depth: %w", err)
		}
		results = append(results, d)
	}
	return results, rows.Err()
}

// QueueJobDepth represents the queue depth for a single job.
type QueueJobDepth struct {
	JobID                    string
	QueuedCount              int
	QueueDepthAlertThreshold int
}

func (q *Queries) ListQueueDepthByJob(ctx context.Context) ([]QueueJobDepth, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListQueueDepthByJob")
	defer span.End()

	query := `
		SELECT jr.job_id, COUNT(*) AS queued_count, j.queue_depth_alert_threshold
		FROM job_runs jr
		JOIN jobs j ON j.id = jr.job_id
		WHERE jr.status = 'queued'
		  AND j.queue_depth_alert_threshold IS NOT NULL
		GROUP BY jr.job_id, j.queue_depth_alert_threshold
		HAVING COUNT(*) >= j.queue_depth_alert_threshold`

	rows, err := q.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list queue depth by job: %w", err)
	}
	defer rows.Close()

	var results []QueueJobDepth
	for rows.Next() {
		var d QueueJobDepth
		if err := rows.Scan(&d.JobID, &d.QueuedCount, &d.QueueDepthAlertThreshold); err != nil {
			return nil, fmt.Errorf("scan queue depth: %w", err)
		}
		results = append(results, d)
	}
	return results, rows.Err()
}

func (q *Queries) GetDebugBundle(ctx context.Context, runID string) (*domain.DebugBundle, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetDebugBundle")
	defer span.End()

	run, err := q.GetRun(ctx, runID)
	if err != nil {
		return nil, err
	}

	events, err := q.ListEvents(ctx, runID, 10000, nil)
	if err != nil {
		return nil, fmt.Errorf("get debug bundle events: %w", err)
	}

	checkpoints, err := q.ListRunCheckpoints(ctx, runID, 1000, nil)
	if err != nil {
		return nil, fmt.Errorf("get debug bundle checkpoints: %w", err)
	}

	usage, err := q.ListRunUsage(ctx, runID, 1000, nil)
	if err != nil {
		return nil, fmt.Errorf("get debug bundle usage: %w", err)
	}

	toolCalls, err := q.ListRunToolCalls(ctx, runID, 1000, nil)
	if err != nil {
		return nil, fmt.Errorf("get debug bundle tool calls: %w", err)
	}

	outputs, err := q.ListRunOutputs(ctx, runID, 10000, nil)
	if err != nil {
		return nil, fmt.Errorf("get debug bundle outputs: %w", err)
	}

	resourceSnapshots, err := q.ListRunResourceSnapshots(ctx, runID, nil, nil, 1000)
	if err != nil {
		return nil, fmt.Errorf("get debug bundle resource snapshots: %w", err)
	}

	return &domain.DebugBundle{
		Run:               run,
		Events:            events,
		Checkpoints:       checkpoints,
		Usage:             usage,
		ToolCalls:         toolCalls,
		Outputs:           outputs,
		ResourceSnapshots: resourceSnapshots,
	}, nil
}

// SetRunMachineID stores the container machine ID on a run.
func (q *Queries) SetRunMachineID(ctx context.Context, runID, machineID string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.SetRunMachineID")
	defer span.End()

	query := `UPDATE job_runs SET machine_id = $1 WHERE id = $2`
	tag, err := q.db.Exec(ctx, query, machineID, runID)
	if err != nil {
		return fmt.Errorf("set run machine_id: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", ErrRunNotFound, runID)
	}
	return nil
}

func (q *Queries) UpdateRunDebugMode(ctx context.Context, runID string, debugMode bool) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpdateRunDebugMode")
	defer span.End()

	query := `UPDATE job_runs SET debug_mode = $1 WHERE id = $2`

	tag, err := q.db.Exec(ctx, query, debugMode, runID)
	if err != nil {
		return fmt.Errorf("update run debug mode: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", ErrRunNotFound, runID)
	}

	return nil
}

func (q *Queries) ListRunLineage(ctx context.Context, runID string, limit int, _ *time.Time) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListRunLineage")
	defer span.End()

	// Walk backward to find the root of the lineage chain.
	rootID := runID
	for range 20 { // safety bound
		var continuationOf *string
		err := q.db.QueryRow(ctx, "SELECT continuation_of FROM job_runs WHERE id = $1", rootID).Scan(&continuationOf)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				break
			}
			return nil, fmt.Errorf("list run lineage walk: %w", err)
		}
		if continuationOf == nil || *continuationOf == "" {
			break
		}
		rootID = *continuationOf
	}

	// Walk forward from root via recursive CTE.
	query := `
		WITH RECURSIVE lineage AS (
			SELECT id, job_id, project_id, status, attempt, payload, result, metadata, error, error_class,
			       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
			       next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id, execution_trace, debug_mode, continuation_of, lineage_depth, tags, job_version_id, created_by, batch_id, concurrency_key, execution_mode, machine_id, deployment_id, pinned_image_uri, pinned_image_digest, is_rollback
			FROM job_runs
			WHERE id = $1
			UNION ALL
			SELECT jr.id, jr.job_id, jr.project_id, jr.status, jr.attempt, jr.payload, jr.result, jr.metadata, jr.error, jr.error_class,
			       jr.triggered_by, jr.scheduled_at, jr.started_at, jr.finished_at, jr.heartbeat_at,
			       jr.next_retry_at, jr.expires_at, jr.parent_run_id, jr.priority, jr.idempotency_key, jr.job_version, jr.created_at, jr.workflow_step_run_id, jr.execution_trace, jr.debug_mode, jr.continuation_of, jr.lineage_depth, jr.tags, jr.job_version_id, jr.created_by, jr.batch_id, jr.concurrency_key, jr.execution_mode, jr.machine_id, jr.deployment_id, jr.pinned_image_uri, jr.pinned_image_digest, jr.is_rollback
			FROM job_runs jr
			JOIN lineage l ON jr.continuation_of = l.id
		)
		SELECT id, job_id, project_id, status, attempt, payload, result, metadata, error, error_class,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id, execution_trace, debug_mode, continuation_of, lineage_depth, tags, job_version_id, created_by, batch_id, concurrency_key, execution_mode, machine_id, deployment_id, pinned_image_uri, pinned_image_digest, is_rollback
		FROM lineage
		ORDER BY lineage_depth ASC
		LIMIT $2`

	rows, err := q.db.Query(ctx, query, rootID, limit)
	if err != nil {
		return nil, fmt.Errorf("list run lineage: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.JobRun, 0)
	for rows.Next() {
		run, err := dbscan.ScanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("list run lineage scan: %w", err)
		}
		runs = append(runs, *run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list run lineage rows: %w", err)
	}

	return runs, nil
}

func (q *Queries) ListRunsByTag(ctx context.Context, projectID, tagKey, tagValue string, limit int, cursor *time.Time) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListRunsByTag")
	defer span.End()

	base := `
		SELECT id, job_id, project_id, status, attempt, payload, result, metadata, error, error_class,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id, execution_trace, debug_mode, continuation_of, lineage_depth, tags, job_version_id, created_by, batch_id, concurrency_key, execution_mode, machine_id, deployment_id, pinned_image_uri, pinned_image_digest, is_rollback
		FROM job_runs
		WHERE project_id = $1`

	args := []any{projectID, tagKey}
	param := 3
	if tagValue == "" {
		base += ` AND tags ? $2`
	} else {
		base += ` AND tags ->> $2 = $3`
		args = append(args, tagValue)
		param++
	}

	if cursor != nil {
		base += fmt.Sprintf(" AND created_at < $%d", param)
		args = append(args, *cursor)
		param++
	}

	base += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", param)
	args = append(args, limit)

	rows, err := q.db.Query(ctx, base, args...)
	if err != nil {
		return nil, fmt.Errorf("list runs by tag: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.JobRun, 0)
	for rows.Next() {
		run, scanErr := dbscan.ScanRun(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list runs by tag scan: %w", scanErr)
		}
		runs = append(runs, *run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list runs by tag rows: %w", err)
	}

	return runs, nil
}

func marshalTagsForRun(tags map[string]string) []byte {
	if len(tags) == 0 {
		return []byte("{}")
	}
	b, _ := json.Marshal(tags)
	return b
}

// CancelJobRunsByWorkflowRun bulk-cancels all non-terminal job runs linked
// to step runs of the given workflow run.
func (q *Queries) CancelJobRunsByWorkflowRun(ctx context.Context, workflowRunID string, finishedAt time.Time, reason string) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CancelJobRunsByWorkflowRun")
	defer span.End()

	query := `
		UPDATE job_runs r
		SET status = 'canceled',
		    finished_at = $2,
		    error = NULLIF($3, '')
		FROM workflow_step_runs wsr
		WHERE wsr.job_run_id = r.id
		  AND wsr.workflow_run_id = $1
		  AND r.status NOT IN ('completed', 'failed', 'canceled')`

	tag, err := q.db.Exec(ctx, query, workflowRunID, finishedAt, reason)
	if err != nil {
		return 0, fmt.Errorf("cancel job runs by workflow run: %w", err)
	}
	return tag.RowsAffected(), nil
}

// ListManagedMachineIDsByWorkflowRun returns machine IDs for managed runs linked
// to a workflow run that have a non-empty machine_id (for stopping on cancel).
func (q *Queries) ListManagedMachineIDsByWorkflowRun(ctx context.Context, workflowRunID string) ([]string, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListManagedMachineIDsByWorkflowRun")
	defer span.End()

	query := `
		SELECT r.machine_id
		FROM job_runs r
		JOIN workflow_step_runs wsr ON wsr.job_run_id = r.id
		WHERE wsr.workflow_run_id = $1
		  AND r.execution_mode = 'managed'
		  AND r.machine_id IS NOT NULL
		  AND r.machine_id != ''`

	rows, err := q.db.Query(ctx, query, workflowRunID)
	if err != nil {
		return nil, fmt.Errorf("list managed machine ids: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan machine id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// MarkJobRunsPausedByWorkflowRun transitions executing managed job runs linked
// to this workflow run to paused status, storing the machine_id in metadata
// so resume knows to re-dispatch them.
func (q *Queries) MarkJobRunsPausedByWorkflowRun(ctx context.Context, workflowRunID string) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.MarkJobRunsPausedByWorkflowRun")
	defer span.End()

	query := `
		UPDATE job_runs r
		SET status = 'paused',
		    metadata = COALESCE(metadata, '{}'::jsonb) || jsonb_build_object('_paused_machine_id', r.machine_id)
		FROM workflow_step_runs wsr
		WHERE wsr.job_run_id = r.id
		  AND wsr.workflow_run_id = $1
		  AND r.status = 'executing'
		  AND r.execution_mode = 'managed'
		  AND r.machine_id IS NOT NULL
		  AND r.machine_id != ''`

	tag, err := q.db.Exec(ctx, query, workflowRunID)
	if err != nil {
		return 0, fmt.Errorf("mark job runs paused by workflow run: %w", err)
	}
	return tag.RowsAffected(), nil
}

// RequeuePausedJobRuns transitions paused job runs linked to a workflow run
// back to queued status, clearing pause metadata and resetting timing fields.
func (q *Queries) RequeuePausedJobRuns(ctx context.Context, workflowRunID string) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.RequeuePausedJobRuns")
	defer span.End()

	query := `
		UPDATE job_runs r
		SET status = 'queued',
		    started_at = NULL,
		    finished_at = NULL,
		    metadata = metadata - '_paused_machine_id'
		FROM workflow_step_runs wsr
		WHERE wsr.job_run_id = r.id
		  AND wsr.workflow_run_id = $1
		  AND r.status = 'paused'`

	tag, err := q.db.Exec(ctx, query, workflowRunID)
	if err != nil {
		return 0, fmt.Errorf("requeue paused job runs: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (q *Queries) ActivateDueRuns(ctx context.Context, limit int) (int64, error) {
	tag, err := q.db.Exec(ctx,
		`UPDATE job_runs SET status = 'queued'
		 WHERE id IN (
		     SELECT id FROM job_runs
		     WHERE status = 'delayed'
		     AND scheduled_at <= NOW()
		     ORDER BY scheduled_at ASC
		     LIMIT $1
		     FOR UPDATE SKIP LOCKED
		 ) AND status = 'delayed'`,
		limit)
	if err != nil {
		return 0, fmt.Errorf("activate due runs: %w", err)
	}
	return tag.RowsAffected(), nil
}

// BulkCancelResult holds the per-run outcome of a bulk cancel.
type BulkCancelResult struct {
	ID             string
	PreviousStatus domain.RunStatus
	Canceled       bool
	Error          string
}

func (q *Queries) GetRunsByIDs(ctx context.Context, ids []string) (map[string]*domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetRunsByIDs")
	defer span.End()
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := q.db.Query(ctx,
		`SELECT id, job_id, project_id, status, attempt, payload, result, metadata, error, error_class,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id, execution_trace, debug_mode, continuation_of, lineage_depth, tags, job_version_id, created_by, batch_id, concurrency_key, execution_mode, machine_id, deployment_id, pinned_image_uri, pinned_image_digest, is_rollback
		 FROM job_runs WHERE id = ANY($1)`, ids)
	if err != nil {
		return nil, fmt.Errorf("get runs by ids: %w", err)
	}
	defer rows.Close()
	result := make(map[string]*domain.JobRun, len(ids))
	for rows.Next() {
		run, err := dbscan.ScanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("scan run: %w", err)
		}
		result[run.ID] = run
	}
	return result, rows.Err()
}

func (q *Queries) BulkCancelRuns(ctx context.Context, ids []string, finishedAt time.Time, reason string) ([]BulkCancelResult, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.BulkCancelRuns")
	defer span.End()
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := q.db.Query(ctx,
		`UPDATE job_runs
		 SET status = 'canceled', finished_at = $2, error = $3
		 WHERE id = ANY($1)
		   AND status IN ('delayed', 'queued', 'dequeued', 'executing', 'waiting')
		 RETURNING id`, ids, finishedAt, reason)
	if err != nil {
		return nil, fmt.Errorf("bulk cancel runs: %w", err)
	}
	defer rows.Close()
	canceledSet := make(map[string]struct{})
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan canceled id: %w", err)
		}
		canceledSet[id] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("bulk cancel rows: %w", err)
	}
	results := make([]BulkCancelResult, 0, len(ids))
	for _, id := range ids {
		if _, ok := canceledSet[id]; ok {
			results = append(results, BulkCancelResult{ID: id, Canceled: true})
		}
	}
	return results, nil
}

func (q *Queries) CancelChildRunsByParentIDs(ctx context.Context, parentIDs []string, finishedAt time.Time, reason string) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CancelChildRunsByParentIDs")
	defer span.End()
	if len(parentIDs) == 0 {
		return 0, nil
	}
	tag, err := q.db.Exec(ctx,
		`UPDATE job_runs
		 SET status = 'canceled', finished_at = $2, error = $3
		 WHERE parent_run_id = ANY($1)
		   AND status IN ('delayed', 'queued', 'dequeued', 'executing', 'waiting')`,
		parentIDs, finishedAt, reason)
	if err != nil {
		return 0, fmt.Errorf("cancel child runs: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (q *Queries) BulkReplayDeadLetterRuns(ctx context.Context, runIDs []string, projectID string, limit int) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.BulkReplayDeadLetterRuns")
	defer span.End()

	if len(runIDs) == 0 && projectID == "" {
		return nil, fmt.Errorf("at least one run id or project_id is required")
	}
	if len(runIDs) > 0 && projectID != "" {
		return nil, fmt.Errorf("provide either run_ids or project_id, not both")
	}

	idsToReplay := runIDs
	if len(idsToReplay) == 0 {
		if limit <= 0 {
			limit = 100
		}
		query := `
			SELECT id
			FROM job_runs
			WHERE project_id = $1 AND status = 'dead_letter'
			ORDER BY created_at ASC
			LIMIT $2`
		rows, err := q.db.Query(ctx, query, projectID, limit)
		if err != nil {
			return nil, fmt.Errorf("select dead letter runs for bulk replay: %w", err)
		}
		defer rows.Close()

		idsToReplay = make([]string, 0, limit)
		for rows.Next() {
			var runID string
			if scanErr := rows.Scan(&runID); scanErr != nil {
				return nil, fmt.Errorf("scan dead letter run id for bulk replay: %w", scanErr)
			}
			idsToReplay = append(idsToReplay, runID)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("iterate dead letter run ids for bulk replay: %w", err)
		}
	}

	replayed := make([]domain.JobRun, 0, len(idsToReplay))
	replayRuns := func(runQ *Queries) error {
		for _, runID := range idsToReplay {
			run, err := runQ.GetRun(ctx, runID)
			if err != nil {
				if errors.Is(err, ErrRunNotFound) {
					continue
				}
				return fmt.Errorf("get run %s for bulk replay: %w", runID, err)
			}
			if run.Status != domain.StatusDeadLetter {
				continue
			}

			if err := runQ.UpdateRunStatus(ctx, runID, domain.StatusDeadLetter, domain.StatusReplayStaged, nil); err != nil {
				return fmt.Errorf("stage run %s for replay: %w", runID, err)
			}

			if err := runQ.UpdateRunStatus(ctx, runID, domain.StatusReplayStaged, domain.StatusQueued, map[string]any{
				"attempt":       1,
				"error":         "",
				"started_at":    nil,
				"finished_at":   nil,
				"heartbeat_at":  nil,
				"next_retry_at": nil,
			}); err != nil {
				return fmt.Errorf("enqueue staged run %s: %w", runID, err)
			}

			updatedRun, err := runQ.GetRun(ctx, runID)
			if err != nil {
				return fmt.Errorf("get replayed run %s: %w", runID, err)
			}
			replayed = append(replayed, *updatedRun)
		}

		return nil
	}

	if beginner, ok := q.db.(TxBeginner); ok {
		if err := WithTx(ctx, beginner, replayRuns); err != nil {
			return nil, fmt.Errorf("bulk replay dead letter runs transaction: %w", err)
		}
	} else {
		if err := replayRuns(q); err != nil {
			return nil, err
		}
	}

	if len(replayed) == 0 {
		return nil, fmt.Errorf("no dead_letter runs available for replay")
	}

	return replayed, nil
}

func (q *Queries) UpdateRunStatusReturningOld(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) (domain.RunStatus, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpdateRunStatus")
	defer span.End()

	if err := domain.ValidateTransition(from, to); err != nil {
		return "", fmt.Errorf("invalid status transition: %w", err)
	}

	allowedColumns := map[string]struct{}{
		"attempt":              {},
		"payload":              {},
		"result":               {},
		"error":                {},
		"error_class":          {},
		"triggered_by":         {},
		"scheduled_at":         {},
		"started_at":           {},
		"finished_at":          {},
		"heartbeat_at":         {},
		"next_retry_at":        {},
		"expires_at":           {},
		"execution_trace":      {},
		"workflow_step_run_id": {},
		"debug_mode":           {},
		"continuation_of":      {},
		"lineage_depth":        {},
		"priority":             {},
		"metadata":             {},
		"machine_id":           {},
	}

	setClauses := []string{"status = $1"}
	args := []any{to, id, from}
	param := 4

	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		if _, ok := allowedColumns[key]; !ok {
			return "", &domain.FieldError{Field: key}
		}

		value := fields[key]
		if raw, ok := value.(json.RawMessage); ok {
			value = dbscan.NilIfEmptyRawMessage(raw)
		}
		if key == "metadata" {
			if m, ok := value.(map[string]string); ok {
				encoded, err := json.Marshal(m)
				if err != nil {
					return "", fmt.Errorf("marshal metadata: %w", err)
				}
				setClauses = append(setClauses, fmt.Sprintf("metadata = COALESCE(metadata, '{}'::jsonb) || $%d::jsonb", param))
				args = append(args, encoded)
				param++
				continue
			}
		}
		if key == "execution_trace" {
			switch trace := value.(type) {
			case *domain.ExecutionTrace:
				if trace == nil {
					value = nil
				} else {
					encoded, err := json.Marshal(trace)
					if err != nil {
						return "", fmt.Errorf("marshal execution trace: %w", err)
					}
					value = dbscan.NilIfEmptyRawMessage(encoded)
				}
			case domain.ExecutionTrace:
				encoded, err := json.Marshal(trace)
				if err != nil {
					return "", fmt.Errorf("marshal execution trace: %w", err)
				}
				value = dbscan.NilIfEmptyRawMessage(encoded)
			}
		}
		if key == "error" || key == "workflow_step_run_id" {
			if text, ok := value.(string); ok {
				value = dbscan.NilIfEmptyString(text)
			}
		}

		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", key, param))
		args = append(args, value)
		param++
	}

	query := fmt.Sprintf(
		"WITH old AS (SELECT status FROM job_runs WHERE id = $2 AND status = $3) UPDATE job_runs SET %s WHERE id = $2 AND status = $3 RETURNING (SELECT status FROM old) AS old_status",
		strings.Join(setClauses, ", "),
	)

	var oldStatus domain.RunStatus
	err := q.db.QueryRow(ctx, query, args...).Scan(&oldStatus)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			var currentStatus domain.RunStatus
			rerr := q.db.QueryRow(ctx, "SELECT status FROM job_runs WHERE id = $1", id).Scan(&currentStatus)
			if rerr != nil {
				return "", fmt.Errorf("checking current status: %w", rerr)
			}
			if currentStatus == to {
				return from, nil // idempotent: already in target state
			}
			return "", fmt.Errorf("%w: id %s from %s", ErrRunConflict, id, from)
		}
		return "", fmt.Errorf("update run status: %w", err)
	}

	return oldStatus, nil
}

func (q *Queries) BatchUpdateHeartbeat(ctx context.Context, ids []string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.BatchUpdateHeartbeat")
	defer span.End()

	if len(ids) == 0 {
		return nil
	}

	query := `UPDATE job_runs SET heartbeat_at = NOW() WHERE id = ANY($1)`

	if _, err := q.db.Exec(ctx, query, ids); err != nil {
		return fmt.Errorf("batch update heartbeat: %w", err)
	}

	return nil
}

func (q *Queries) ResetRunIdempotencyKey(ctx context.Context, runID string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ResetRunIdempotencyKey")
	defer span.End()

	txb, ok := q.db.(TxBeginner)
	if !ok {
		return fmt.Errorf("reset idempotency key requires transaction support")
	}

	return WithTx(ctx, txb, func(txQ *Queries) error {
		// Fetch run details needed for idempotency cleanup.
		var idempotencyKey *string
		var jobID string
		var createdAt time.Time
		err := txQ.db.QueryRow(ctx, `
			SELECT idempotency_key, job_id, created_at
			FROM job_runs
			WHERE id = $1
			  AND status NOT IN ('dequeued', 'executing', 'waiting')`,
			runID,
		).Scan(&idempotencyKey, &jobID, &createdAt)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrRunNotFound
			}
			return fmt.Errorf("reset idempotency key fetch: %w", err)
		}

		if idempotencyKey == nil || *idempotencyKey == "" {
			return nil
		}

		// Clear idempotency_key on the run. Use created_at for partition pruning.
		_, err = txQ.db.Exec(ctx, `
			UPDATE job_runs
			SET idempotency_key = NULL
			WHERE id = $1 AND created_at = $2`,
			runID, createdAt,
		)
		if err != nil {
			return fmt.Errorf("reset idempotency key update: %w", err)
		}

		// Remove from global dedup table.
		_, err = txQ.db.Exec(ctx, `
			DELETE FROM job_run_idempotency
			WHERE job_id = $1 AND idempotency_key = $2`,
			jobID, *idempotencyKey,
		)
		if err != nil {
			return fmt.Errorf("reset idempotency key cleanup: %w", err)
		}

		return nil
	})
}

func (q *Queries) RescheduleRun(ctx context.Context, runID string, scheduledAt time.Time, payload json.RawMessage) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.RescheduleRun")
	defer span.End()

	result, err := q.db.Exec(ctx, `
		UPDATE job_runs
		SET scheduled_at = $2,
		    status = CASE WHEN $2 <= NOW() THEN 'queued' ELSE 'delayed' END,
		    payload = COALESCE($3, payload)
		WHERE id = $1
		  AND status IN ('delayed', 'queued')
		  AND started_at IS NULL
	`, runID, scheduledAt, dbscan.NilIfEmptyRawMessage(payload))
	if err != nil {
		return fmt.Errorf("reschedule run: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrRunNotFound
	}
	return nil
}

// BulkCancelFilter defines optional filters for bulk-canceling runs by criteria.
type BulkCancelFilter struct {
	JobID       string
	BatchID     string
	TriggeredBy string
	Status      domain.RunStatus
}

func (q *Queries) BulkCancelByFilter(ctx context.Context, projectID string, f BulkCancelFilter, now time.Time, reason string) ([]string, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.BulkCancelByFilter")
	defer span.End()

	// Build the filter conditions for the subquery that selects candidate rows.
	// A LIMIT 10000 cap prevents locking millions of rows in a single UPDATE.
	filterQuery := `SELECT id FROM job_runs
		WHERE project_id = $3
		  AND status IN ('delayed', 'queued')
		  AND started_at IS NULL`

	args := []any{now, reason, projectID}
	param := 4

	if f.JobID != "" {
		filterQuery += fmt.Sprintf(" AND job_id = $%d", param)
		args = append(args, f.JobID)
		param++
	}
	if f.BatchID != "" {
		filterQuery += fmt.Sprintf(" AND batch_id = $%d", param)
		args = append(args, f.BatchID)
		param++
	}
	if f.TriggeredBy != "" {
		filterQuery += fmt.Sprintf(" AND triggered_by = $%d", param)
		args = append(args, f.TriggeredBy)
		param++
	}
	if f.Status != "" {
		filterQuery += fmt.Sprintf(" AND status = $%d", param)
		args = append(args, f.Status)
	}

	filterQuery += " LIMIT 10000"

	baseQuery := `
		UPDATE job_runs
		SET status = 'canceled', finished_at = $1, error = $2
		WHERE id IN (` + filterQuery + `)
		RETURNING id`

	rows, err := q.db.Query(ctx, baseQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("bulk cancel by filter: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("bulk cancel by filter scan: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (q *Queries) GetRunErrorClass(ctx context.Context, runID string) (string, error) {
	var errorClass string
	err := q.db.QueryRow(ctx, "SELECT COALESCE(error_class, '') FROM job_runs WHERE id = $1", runID).Scan(&errorClass)
	return errorClass, err
}

func (q *Queries) CountActiveRunsForJob(ctx context.Context, jobID string) (int, error) {
	query := `SELECT COUNT(*) FROM job_runs WHERE job_id = $1 AND status IN ('queued','dequeued','executing','waiting','delayed')`
	var count int
	err := q.db.QueryRow(ctx, query, jobID).Scan(&count)
	return count, err
}

// CanceledRun holds metadata about a run that was canceled, allowing callers
// to perform side effects like stopping managed containers.
type CanceledRun struct {
	ID            string
	MachineID     string
	ExecutionMode domain.ExecutionMode
}

// CancelActiveRunsForJob cancels all non-terminal runs for a job and returns
// details of each canceled run. Used by the cron overlap policy cancel_running.
func (q *Queries) CancelActiveRunsForJob(ctx context.Context, jobID string, reason string) ([]CanceledRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CancelActiveRunsForJob")
	defer span.End()

	query := `UPDATE job_runs
		SET status = 'canceled', finished_at = NOW(), error = $2
		WHERE job_id = $1
		  AND status IN ('queued', 'dequeued', 'executing', 'waiting', 'delayed')
		RETURNING id, COALESCE(machine_id, ''), COALESCE(execution_mode, 'http')`
	rows, err := q.db.Query(ctx, query, jobID, reason)
	if err != nil {
		return nil, fmt.Errorf("cancel active runs for job: %w", err)
	}
	defer rows.Close()

	var result []CanceledRun
	for rows.Next() {
		var cr CanceledRun
		var execMode string
		if err := rows.Scan(&cr.ID, &cr.MachineID, &execMode); err != nil {
			return nil, fmt.Errorf("scan canceled run: %w", err)
		}
		cr.ExecutionMode = domain.ExecutionMode(execMode)
		result = append(result, cr)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cancel active runs rows: %w", err)
	}
	return result, nil
}

// SumRunTotalTokens returns the total tokens used by a single run.
func (q *Queries) SumRunTotalTokens(ctx context.Context, runID string) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.SumRunTotalTokens")
	defer span.End()

	query := `SELECT COALESCE(SUM(total_tokens), 0) FROM run_usage WHERE run_id = $1`
	var total int64
	if err := q.db.QueryRow(ctx, query, runID).Scan(&total); err != nil {
		return 0, fmt.Errorf("sum run total tokens: %w", err)
	}
	return total, nil
}

// CountRunToolCalls returns the number of tool calls recorded for a run.
func (q *Queries) CountRunToolCalls(ctx context.Context, runID string) (int, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CountRunToolCalls")
	defer span.End()

	query := `SELECT COUNT(*) FROM run_tool_calls WHERE run_id = $1`
	var count int
	if err := q.db.QueryRow(ctx, query, runID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count run tool calls: %w", err)
	}
	return count, nil
}

// CountRunIterations returns the number of iterations recorded for a run.
func (q *Queries) CountRunIterations(ctx context.Context, runID string) (int, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CountRunIterations")
	defer span.End()

	query := `SELECT COUNT(*) FROM run_iterations WHERE run_id = $1`
	var count int
	if err := q.db.QueryRow(ctx, query, runID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count run iterations: %w", err)
	}
	return count, nil
}

// CreateRunIteration inserts a new run iteration record.
func (q *Queries) CreateRunIteration(ctx context.Context, iter *domain.RunIteration) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateRunIteration")
	defer span.End()

	if iter.ID == "" {
		iter.ID = uuid.Must(uuid.NewV7()).String()
	}

	query := `
		INSERT INTO run_iterations (id, run_id, iteration, description)
		VALUES ($1, $2, $3, $4)
		RETURNING created_at`

	err := q.db.QueryRow(ctx, query, iter.ID, iter.RunID, iter.Iteration, iter.Description).Scan(&iter.CreatedAt)
	if err != nil {
		return fmt.Errorf("create run iteration: %w", err)
	}
	return nil
}
