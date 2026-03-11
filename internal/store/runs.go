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
	"github.com/jackc/pgx/v5/pgconn"
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
		INSERT INTO job_runs (
			id, job_id, project_id, status, attempt, payload, result, error,
			triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
			next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, workflow_step_run_id,
			debug_mode, continuation_of, lineage_depth, tags, job_version_id, created_by
		)
		VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8,
			$9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20,
			$21, $22, $23, $24::jsonb, $25, $26
		)
		RETURNING created_at`

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
	).Scan(&run.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "idx_runs_idempotency" {
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
		SELECT id, job_id, project_id, status, attempt, payload, result, metadata, error,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id, execution_trace, debug_mode, continuation_of, lineage_depth, tags, job_version_id, created_by
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
		SELECT id, job_id, project_id, status, attempt, payload, result, metadata, error,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id, execution_trace, debug_mode, continuation_of, lineage_depth, tags, job_version_id, created_by
		FROM job_runs
		WHERE job_id = $1
		  AND idempotency_key = $2
		  AND status IN ('delayed', 'queued', 'dequeued', 'executing', 'waiting')
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
		SELECT id, job_id, project_id, status, attempt, payload, result, metadata, error,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id, execution_trace, debug_mode, continuation_of, lineage_depth, tags, job_version_id, created_by
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
			) AS p95_duration_secs
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
			SELECT id, status
			FROM job_runs
			WHERE parent_run_id = $1
			UNION ALL
			SELECT jr.id, jr.status
			FROM job_runs jr
			JOIN descendants d ON jr.parent_run_id = d.id
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
		SELECT id, job_id, project_id, status, attempt, payload, result, metadata, error,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id, execution_trace, debug_mode, continuation_of, lineage_depth, tags, job_version_id, created_by
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

func (q *Queries) ListRunsByProject(ctx context.Context, projectID string, status *domain.RunStatus, metadataKey, metadataValue *string, limit int, cursor *time.Time) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListRunsByProject")
	defer span.End()

	baseQuery := `
		SELECT id, job_id, project_id, status, attempt, payload, result, metadata, error,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id, execution_trace, debug_mode, continuation_of, lineage_depth, tags, job_version_id, created_by
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

func (q *Queries) ListDeadLetterRuns(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListDeadLetterRuns")
	defer span.End()

	if limit <= 0 {
		limit = 50
	}

	query := `
		SELECT id, job_id, project_id, status, attempt, payload, result, metadata, error,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id, execution_trace, debug_mode, continuation_of, lineage_depth, tags, job_version_id, created_by
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

	run, err := q.GetRun(ctx, runID)
	if err != nil {
		return nil, err
	}

	if run.Status != domain.StatusDeadLetter {
		return nil, fmt.Errorf("run %s is not dead_letter", runID)
	}

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

func (q *Queries) ListStaleRuns(ctx context.Context, threshold time.Duration) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListStaleRuns")
	defer span.End()

	query := fmt.Sprintf(`
		SELECT id, job_id, project_id, status, attempt, payload, result, metadata, error,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id, execution_trace, debug_mode, continuation_of, lineage_depth, tags, job_version_id, created_by
		FROM job_runs
		WHERE status = '%s' AND heartbeat_at < NOW() - $1::interval
		ORDER BY heartbeat_at ASC`, domain.StatusExecuting)

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
		SELECT id, job_id, project_id, status, attempt, payload, result, metadata, error,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id, execution_trace, debug_mode, continuation_of, lineage_depth, tags, job_version_id, created_by
		FROM job_runs
		WHERE status = '%s' AND scheduled_at <= NOW()
		ORDER BY scheduled_at ASC`, domain.StatusDelayed)

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
		SELECT id, job_id, project_id, status, attempt, payload, result, metadata, error,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id, execution_trace, debug_mode, continuation_of, lineage_depth, tags, job_version_id, created_by
		FROM job_runs
		WHERE status IN ('%s', '%s')
		  AND expires_at IS NOT NULL
		  AND expires_at <= NOW()
		ORDER BY expires_at ASC`, domain.StatusDelayed, domain.StatusQueued)

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
		SELECT id, job_id, project_id, status, attempt, payload, result, metadata, error,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id, execution_trace, debug_mode, continuation_of, lineage_depth, tags, job_version_id, created_by
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
		SELECT id, job_id, project_id, status, attempt, payload, result, metadata, error,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id, execution_trace, debug_mode, continuation_of, lineage_depth, tags, job_version_id, created_by
		FROM job_runs
		WHERE status = '%s' AND started_at < NOW() - $1::interval
		ORDER BY started_at ASC`, domain.StatusDequeued)

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
		DELETE FROM job_runs
		WHERE finished_at IS NOT NULL
		  AND (
			(status IN ('completed', 'failed', 'canceled', 'expired') AND finished_at <= $1)
			OR
			(status IN ('timed_out', 'crashed', 'system_failed') AND finished_at <= $2)
		  )`

	tag, err := q.db.Exec(ctx, query, shortCutoff, longCutoff)
	if err != nil {
		return 0, fmt.Errorf("delete terminal runs past retention: %w", err)
	}

	return tag.RowsAffected(), nil
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

	return &domain.DebugBundle{
		Run:         run,
		Events:      events,
		Checkpoints: checkpoints,
		Usage:       usage,
		ToolCalls:   toolCalls,
		Outputs:     outputs,
	}, nil
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
			SELECT id, job_id, project_id, status, attempt, payload, result, metadata, error,
			       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
			       next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id, execution_trace, debug_mode, continuation_of, lineage_depth, tags, job_version_id, created_by
			FROM job_runs
			WHERE id = $1
			UNION ALL
			SELECT jr.id, jr.job_id, jr.project_id, jr.status, jr.attempt, jr.payload, jr.result, jr.metadata, jr.error,
			       jr.triggered_by, jr.scheduled_at, jr.started_at, jr.finished_at, jr.heartbeat_at,
			       jr.next_retry_at, jr.expires_at, jr.parent_run_id, jr.priority, jr.idempotency_key, jr.job_version, jr.created_at, jr.workflow_step_run_id, jr.execution_trace, jr.debug_mode, jr.continuation_of, jr.lineage_depth, jr.tags, jr.job_version_id, jr.created_by
			FROM job_runs jr
			JOIN lineage l ON jr.continuation_of = l.id
		)
		SELECT id, job_id, project_id, status, attempt, payload, result, metadata, error,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id, execution_trace, debug_mode, continuation_of, lineage_depth, tags, job_version_id, created_by
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
		SELECT id, job_id, project_id, status, attempt, payload, result, metadata, error,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id, execution_trace, debug_mode, continuation_of, lineage_depth, tags, job_version_id, created_by
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
