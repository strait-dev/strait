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
	"go.opentelemetry.io/otel"
)

func (q *Queries) CreateWorkflowRun(ctx context.Context, run *domain.WorkflowRun) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateWorkflowRun")
	defer span.End()

	if run.ID == "" {
		run.ID = uuid.Must(uuid.NewV7()).String()
	}
	if run.Status == "" {
		run.Status = domain.WfStatusPending
	}
	if run.TriggeredBy == "" {
		run.TriggeredBy = domain.TriggerManual
	}
	if run.WorkflowVersion == 0 {
		run.WorkflowVersion = 1
	}
	if run.WorkflowSnapshotID != "" {
		var snapshotExists bool
		if err := q.db.QueryRow(
			ctx,
			`SELECT EXISTS (SELECT 1 FROM workflow_snapshots WHERE id = $1)`,
			run.WorkflowSnapshotID,
		).Scan(&snapshotExists); err != nil {
			return fmt.Errorf("create workflow run: verify workflow snapshot %q: %w", run.WorkflowSnapshotID, err)
		}
		if !snapshotExists {
			return fmt.Errorf("create workflow run: workflow snapshot %q not found", run.WorkflowSnapshotID)
		}
	}

	tagsJSON := []byte("{}")
	if len(run.Tags) > 0 {
		var marshalErr error
		tagsJSON, marshalErr = json.Marshal(run.Tags)
		if marshalErr != nil {
			return fmt.Errorf("create workflow run: marshal tags: %w", marshalErr)
		}
	}

	var traceContextJSON []byte
	if len(run.TraceContext) > 0 {
		var marshalErr error
		traceContextJSON, marshalErr = json.Marshal(run.TraceContext)
		if marshalErr != nil {
			return fmt.Errorf("create workflow run: marshal trace_context: %w", marshalErr)
		}
	}

	query := `
		INSERT INTO workflow_runs (
			id, workflow_id, project_id, status, triggered_by, payload,
			workflow_version, max_parallel_steps, error, started_at, finished_at, expires_at,
			retry_of_run_id, parent_workflow_run_id, parent_step_run_id,
			tags, workflow_version_id, created_by, trace_context, workflow_snapshot_id,
			expected_completion_at, continued_from_workflow_run_id, lineage_depth
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15,
			$16::jsonb, $17, $18, $19::jsonb, $20, $21, $22, $23)
		RETURNING created_at`

	err := q.db.QueryRow(
		ctx,
		query,
		run.ID,
		run.WorkflowID,
		run.ProjectID,
		run.Status,
		run.TriggeredBy,
		dbscan.NilIfEmptyRawMessage(run.Payload),
		run.WorkflowVersion,
		run.MaxParallelSteps,
		dbscan.NilIfEmptyString(run.Error),
		run.StartedAt,
		run.FinishedAt,
		run.ExpiresAt,
		dbscan.NilIfEmptyString(run.RetryOfRunID),
		dbscan.NilIfEmptyString(run.ParentWorkflowRunID),
		dbscan.NilIfEmptyString(run.ParentStepRunID),
		tagsJSON,
		dbscan.NilIfEmptyString(run.WorkflowVersionID),
		dbscan.NilIfEmptyString(run.CreatedBy),
		traceContextJSON,
		dbscan.NilIfEmptyString(run.WorkflowSnapshotID),
		run.ExpectedCompletionAt,
		dbscan.NilIfEmptyString(run.ContinuedFromWorkflowRunID),
		run.LineageDepth,
	).Scan(&run.CreatedAt)
	if err != nil {
		return fmt.Errorf("create workflow run snapshot_id=%q: %w", run.WorkflowSnapshotID, err)
	}

	return nil
}

func (q *Queries) GetWorkflowRun(ctx context.Context, id string) (*domain.WorkflowRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetWorkflowRun")
	defer span.End()

	query := `
		SELECT id, workflow_id, project_id, status, triggered_by, payload,
		       workflow_version, max_parallel_steps, error, started_at, finished_at, expires_at,
		       retry_of_run_id, parent_workflow_run_id, parent_step_run_id, created_at, tags, workflow_version_id, created_by, trace_context, workflow_snapshot_id, expected_completion_at,
		       continued_from_workflow_run_id, continued_to_workflow_run_id, lineage_depth
		FROM workflow_runs
		WHERE id = $1`

	run, err := scanWorkflowRun(q.db.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrWorkflowRunNotFound
		}
		return nil, fmt.Errorf("get workflow run: %w", err)
	}

	return run, nil
}

func (q *Queries) GetWorkflowRunWithCacheVersion(ctx context.Context, id string) (*domain.WorkflowRun, int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetWorkflowRunWithCacheVersion")
	defer span.End()

	query := `
		SELECT id, workflow_id, project_id, status, triggered_by, payload,
		       workflow_version, max_parallel_steps, error, started_at, finished_at, expires_at,
		       retry_of_run_id, parent_workflow_run_id, parent_step_run_id, created_at, tags, workflow_version_id, created_by, trace_context, workflow_snapshot_id, expected_completion_at,
		       continued_from_workflow_run_id, continued_to_workflow_run_id, lineage_depth, cache_version
		FROM workflow_runs
		WHERE id = $1`

	run, err := scanWorkflowRunWithCacheVersion(q.db.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, 0, ErrWorkflowRunNotFound
		}
		return nil, 0, fmt.Errorf("get workflow run with cache version: %w", err)
	}

	return run, run.CacheVersion, nil
}

func (q *Queries) ListWorkflowRuns(ctx context.Context, workflowID string, limit int, cursor *time.Time) ([]domain.WorkflowRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListWorkflowRuns")
	defer span.End()

	var rows pgx.Rows
	var err error

	if cursor != nil {
		query := `
			SELECT id, workflow_id, project_id, status, triggered_by, payload,
			       workflow_version, max_parallel_steps, error, started_at, finished_at, expires_at,
			       retry_of_run_id, parent_workflow_run_id, parent_step_run_id, created_at, tags, workflow_version_id, created_by, trace_context, workflow_snapshot_id, expected_completion_at,
			       continued_from_workflow_run_id, continued_to_workflow_run_id, lineage_depth
			FROM workflow_runs
			WHERE workflow_id = $1 AND created_at < $3
			ORDER BY created_at DESC
			LIMIT $2`
		rows, err = q.db.Query(ctx, query, workflowID, limit, *cursor)
	} else {
		query := `
			SELECT id, workflow_id, project_id, status, triggered_by, payload,
			       workflow_version, max_parallel_steps, error, started_at, finished_at, expires_at,
			       retry_of_run_id, parent_workflow_run_id, parent_step_run_id, created_at, tags, workflow_version_id, created_by, trace_context, workflow_snapshot_id, expected_completion_at,
			       continued_from_workflow_run_id, continued_to_workflow_run_id, lineage_depth
			FROM workflow_runs
			WHERE workflow_id = $1
			ORDER BY created_at DESC
			LIMIT $2`
		rows, err = q.db.Query(ctx, query, workflowID, limit)
	}

	if err != nil {
		return nil, fmt.Errorf("list workflow runs: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.WorkflowRun, 0, limit)
	for rows.Next() {
		run, scanErr := scanWorkflowRun(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list workflow runs scan: %w", scanErr)
		}
		runs = append(runs, *run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list workflow runs rows: %w", err)
	}

	return runs, nil
}

func (q *Queries) ListWorkflowRunsByProject(ctx context.Context, projectID string, status *domain.WorkflowRunStatus, limit int, cursor *time.Time) ([]domain.WorkflowRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListWorkflowRunsByProject")
	defer span.End()

	baseQuery := `
		SELECT id, workflow_id, project_id, status, triggered_by, payload,
		       workflow_version, max_parallel_steps, error, started_at, finished_at, expires_at,
		       retry_of_run_id, parent_workflow_run_id, parent_step_run_id, created_at, tags, workflow_version_id, created_by, trace_context, workflow_snapshot_id, expected_completion_at,
		       continued_from_workflow_run_id, continued_to_workflow_run_id, lineage_depth
		FROM workflow_runs
		WHERE project_id = $1`

	args := []any{projectID}
	param := 2

	if status != nil {
		baseQuery += fmt.Sprintf(" AND status = $%d", param)
		args = append(args, *status)
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
		return nil, fmt.Errorf("list workflow runs by project: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.WorkflowRun, 0, 16)
	for rows.Next() {
		run, scanErr := scanWorkflowRun(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list workflow runs by project scan: %w", scanErr)
		}
		runs = append(runs, *run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list workflow runs by project rows: %w", err)
	}

	return runs, nil
}

func (q *Queries) DeleteWorkflowRunsFinishedBefore(ctx context.Context, before time.Time, limit int) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteWorkflowRunsFinishedBefore")
	defer span.End()

	if limit <= 0 {
		limit = 100
	}

	query := `
		WITH doomed AS (
			SELECT id
			FROM workflow_runs
			WHERE status IN ('completed', 'failed', 'timed_out', 'canceled', 'compensated', 'compensation_failed', 'continued')
			  AND finished_at IS NOT NULL
			  AND finished_at < $1
			ORDER BY finished_at ASC
			LIMIT $2
		)
		DELETE FROM workflow_runs wr
		USING doomed
		WHERE wr.id = doomed.id`

	tag, err := q.db.Exec(ctx, query, before, limit)
	if err != nil {
		return 0, fmt.Errorf("delete old workflow runs: %w", err)
	}

	return tag.RowsAffected(), nil
}

func (q *Queries) UpdateWorkflowRunStatus(ctx context.Context, id string, from, to domain.WorkflowRunStatus, fields map[string]any) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpdateWorkflowRunStatus")
	defer span.End()

	if err := domain.ValidateWorkflowTransition(from, to); err != nil {
		return fmt.Errorf("invalid workflow status transition: %w", err)
	}

	allowedColumns := map[string]struct{}{
		"triggered_by":                 {},
		"payload":                      {},
		"error":                        {},
		"started_at":                   {},
		"finished_at":                  {},
		"expires_at":                   {},
		"expected_completion_at":       {},
		"continued_to_workflow_run_id": {},
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
			return &domain.FieldError{Field: key}
		}

		value := fields[key]
		if raw, ok := value.(json.RawMessage); ok {
			value = dbscan.NilIfEmptyRawMessage(raw)
		}
		if key == "error" {
			if text, ok := value.(string); ok {
				value = dbscan.NilIfEmptyString(text)
			}
		}

		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", key, param))
		args = append(args, value)
		param++
	}

	query := fmt.Sprintf(
		"UPDATE workflow_runs SET %s WHERE id = $2 AND status = $3",
		strings.Join(setClauses, ", "),
	)

	tag, err := q.db.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update workflow run status: %w", err)
	}

	if tag.RowsAffected() == 0 {
		var currentStatus domain.WorkflowRunStatus
		err := q.db.QueryRow(ctx, "SELECT status FROM workflow_runs WHERE id = $1", id).Scan(&currentStatus)
		if err != nil {
			return fmt.Errorf("checking current workflow status: %w", err)
		}
		if currentStatus == to {
			return nil // idempotent: already in target state
		}
		return fmt.Errorf("update workflow run status conflict: id %s from %s", id, from)
	}

	return nil
}

// GetWorkflowRunsByParent returns all child workflow runs for a given parent workflow run.
func (q *Queries) GetWorkflowRunsByParent(ctx context.Context, parentWorkflowRunID string) ([]domain.WorkflowRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetWorkflowRunsByParent")
	defer span.End()

	query := `
		SELECT id, workflow_id, project_id, status, triggered_by, payload,
		       workflow_version, max_parallel_steps, error, started_at, finished_at, expires_at,
		       retry_of_run_id, parent_workflow_run_id, parent_step_run_id, created_at, tags, workflow_version_id, created_by, trace_context, workflow_snapshot_id, expected_completion_at,
		       continued_from_workflow_run_id, continued_to_workflow_run_id, lineage_depth
		FROM workflow_runs
		WHERE parent_workflow_run_id = $1
		ORDER BY created_at ASC
		LIMIT 10000`

	rows, err := q.db.Query(ctx, query, parentWorkflowRunID)
	if err != nil {
		return nil, fmt.Errorf("get workflow runs by parent: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.WorkflowRun, 0, 8)
	for rows.Next() {
		run, scanErr := scanWorkflowRun(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("get workflow runs by parent scan: %w", scanErr)
		}
		runs = append(runs, *run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get workflow runs by parent rows: %w", err)
	}

	return runs, nil
}

func scanWorkflowRun(scanner scanTarget) (*domain.WorkflowRun, error) {
	return scanWorkflowRunFields(scanner, false)
}

func scanWorkflowRunWithCacheVersion(scanner scanTarget) (*domain.WorkflowRun, error) {
	return scanWorkflowRunFields(scanner, true)
}

func scanWorkflowRunFields(scanner scanTarget, includeCacheVersion bool) (*domain.WorkflowRun, error) {
	var run domain.WorkflowRun
	var payload []byte
	var tagsJSON []byte
	var runError *string
	var startedAt *time.Time
	var finishedAt *time.Time
	var expiresAt *time.Time
	var retryOfRunID *string
	var parentWorkflowRunID *string
	var parentStepRunID *string
	var workflowVersionID *string
	var createdBy *string
	var traceContextJSON []byte
	var workflowSnapshotID *string
	var expectedCompletionAt *time.Time
	var continuedFromWorkflowRunID *string
	var continuedToWorkflowRunID *string

	dest := []any{
		&run.ID,
		&run.WorkflowID,
		&run.ProjectID,
		&run.Status,
		&run.TriggeredBy,
		&payload,
		&run.WorkflowVersion,
		&run.MaxParallelSteps,
		&runError,
		&startedAt,
		&finishedAt,
		&expiresAt,
		&retryOfRunID,
		&parentWorkflowRunID,
		&parentStepRunID,
		&run.CreatedAt,
		&tagsJSON,
		&workflowVersionID,
		&createdBy,
		&traceContextJSON,
		&workflowSnapshotID,
		&expectedCompletionAt,
		&continuedFromWorkflowRunID,
		&continuedToWorkflowRunID,
		&run.LineageDepth,
	}
	if includeCacheVersion {
		dest = append(dest, &run.CacheVersion)
	}

	err := scanner.Scan(dest...)
	if err != nil {
		return nil, err
	}

	if payload != nil {
		run.Payload = json.RawMessage(payload)
	}
	if runError != nil {
		run.Error = *runError
	}
	if retryOfRunID != nil {
		run.RetryOfRunID = *retryOfRunID
	}
	if parentWorkflowRunID != nil {
		run.ParentWorkflowRunID = *parentWorkflowRunID
	}
	if parentStepRunID != nil {
		run.ParentStepRunID = *parentStepRunID
	}
	run.StartedAt = startedAt
	run.FinishedAt = finishedAt
	run.ExpiresAt = expiresAt
	if len(tagsJSON) > 0 && string(tagsJSON) != "{}" {
		if err := json.Unmarshal(tagsJSON, &run.Tags); err != nil {
			return nil, err
		}
	}
	if workflowVersionID != nil {
		run.WorkflowVersionID = *workflowVersionID
	}
	if createdBy != nil {
		run.CreatedBy = *createdBy
	}
	if len(traceContextJSON) > 0 && string(traceContextJSON) != "{}" {
		if err := json.Unmarshal(traceContextJSON, &run.TraceContext); err != nil {
			return nil, err
		}
	}
	if workflowSnapshotID != nil {
		run.WorkflowSnapshotID = *workflowSnapshotID
	}
	run.ExpectedCompletionAt = expectedCompletionAt
	if continuedFromWorkflowRunID != nil {
		run.ContinuedFromWorkflowRunID = *continuedFromWorkflowRunID
	}
	if continuedToWorkflowRunID != nil {
		run.ContinuedToWorkflowRunID = *continuedToWorkflowRunID
	}

	return &run, nil
}

func (q *Queries) CreateWorkflowRunBootstrap(ctx context.Context, run *domain.WorkflowRun, stepRuns []domain.WorkflowStepRun, startedAt time.Time) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateWorkflowRunBootstrap")
	defer span.End()

	_, ok := q.db.(TxBeginner)
	if !ok {
		if err := q.CreateWorkflowRun(ctx, run); err != nil {
			return err
		}
		if err := q.UpdateWorkflowRunStatus(ctx, run.ID, domain.WfStatusPending, domain.WfStatusRunning, map[string]any{"started_at": startedAt}); err != nil {
			return err
		}
		return q.CreateWorkflowStepRuns(ctx, stepRuns)
	}

	return q.withTx(ctx, func(txQ *Queries) error {
		if err := txQ.CreateWorkflowRun(ctx, run); err != nil {
			return fmt.Errorf("create workflow run bootstrap: %w", err)
		}
		if err := txQ.UpdateWorkflowRunStatus(ctx, run.ID, domain.WfStatusPending, domain.WfStatusRunning, map[string]any{"started_at": startedAt}); err != nil {
			return fmt.Errorf("mark workflow running bootstrap: %w", err)
		}
		if err := txQ.CreateWorkflowStepRuns(ctx, stepRuns); err != nil {
			return fmt.Errorf("create workflow step runs bootstrap: %w", err)
		}
		return nil
	})
}

// ContinueWorkflowRunBootstrap atomically completes a predecessor workflow run
// via continue-as-new and starts its successor. Within a single transaction it:
//
//  1. Claims the predecessor with a guarded status transition to continued
//     (setting finished_at), so only one of two concurrent continuations wins.
//     This guard runs first, before any successor work, so the losing racer
//     bails out immediately without inserting a successor or step runs.
//  2. Inserts the successor (whose continued_from_workflow_run_id and
//     lineage_depth are already set on run), flips it pending -> running, and
//     inserts its initial step runs.
//  3. Links the predecessor forward to the now-existing successor via
//     continued_to_workflow_run_id. This is a separate update because that
//     column is a foreign key onto the successor, which must exist first.
//  4. Tears down the predecessor's in-flight step runs, job runs, and event
//     triggers, mirroring a cancel.
//
// If the predecessor is no longer in fromStatus, the guard matches no row and
// ErrWorkflowRunContinueConflict is returned before any successor is created;
// any later failure rolls the whole transaction back, so no orphan successor
// or step runs are ever left behind.
func (q *Queries) ContinueWorkflowRunBootstrap(ctx context.Context, predecessorID string, fromStatus domain.WorkflowRunStatus, successor *domain.WorkflowRun, stepRuns []domain.WorkflowStepRun, now time.Time) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ContinueWorkflowRunBootstrap")
	defer span.End()

	if err := domain.ValidateWorkflowTransition(fromStatus, domain.WfStatusContinued); err != nil {
		return fmt.Errorf("continue workflow run bootstrap: %w", err)
	}

	bootstrap := func(txQ *Queries) error {
		// 1. Claim the predecessor first: a guarded status transition so only
		//    one concurrent continuation wins and the loser bails out before
		//    doing any successor work. continued_to is set in step 3 once its
		//    foreign-key target (the successor row) exists.
		tag, err := txQ.db.Exec(ctx, `
			UPDATE workflow_runs
			SET status = $1, finished_at = $2
			WHERE id = $3 AND status = $4`,
			domain.WfStatusContinued, now, predecessorID, fromStatus,
		)
		if err != nil {
			return fmt.Errorf("mark predecessor continued: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return ErrWorkflowRunContinueConflict
		}

		// 2. Insert the successor and bring it to running with its step runs.
		if err := txQ.CreateWorkflowRun(ctx, successor); err != nil {
			return fmt.Errorf("create successor workflow run: %w", err)
		}
		if err := txQ.UpdateWorkflowRunStatus(ctx, successor.ID, domain.WfStatusPending, domain.WfStatusRunning, map[string]any{"started_at": now}); err != nil {
			return fmt.Errorf("mark successor running: %w", err)
		}
		if err := txQ.CreateWorkflowStepRuns(ctx, stepRuns); err != nil {
			return fmt.Errorf("create successor step runs: %w", err)
		}

		// 3. Link the predecessor forward to the now-existing successor.
		if _, err := txQ.db.Exec(ctx, `
			UPDATE workflow_runs
			SET continued_to_workflow_run_id = $1
			WHERE id = $2`,
			successor.ID, predecessorID,
		); err != nil {
			return fmt.Errorf("link predecessor to successor: %w", err)
		}

		// 4. Tear down the predecessor's in-flight work, mirroring a cancel.
		reason := "workflow continued as new"
		if _, err := txQ.CancelNonTerminalStepRuns(ctx, predecessorID, now, reason); err != nil {
			return fmt.Errorf("cancel predecessor step runs: %w", err)
		}
		if _, err := txQ.CancelJobRunsByWorkflowRun(ctx, predecessorID, now, reason); err != nil {
			return fmt.Errorf("cancel predecessor job runs: %w", err)
		}
		if _, err := txQ.CancelEventTriggersByWorkflowRun(ctx, predecessorID); err != nil {
			return fmt.Errorf("cancel predecessor event triggers: %w", err)
		}
		return nil
	}

	if _, ok := q.db.(TxBeginner); !ok {
		return bootstrap(q)
	}
	return q.withTx(ctx, bootstrap)
}

// GetWorkflowRunChain returns one page of the continue-as-new lineage that
// anyRunID belongs to, ordered from the chain root (the run with no predecessor)
// to the latest successor. It returns a lightweight projection
// (domain.WorkflowRunChainEntry) rather than full runs, and is cursor-paginated
// so a long chain is never materialized in full: callers fetch full detail for a
// specific run on demand via GetWorkflowRun.
//
// Paging is anchored on run ids:
//   - cursor == "" requests the first page. The chain root is located by walking
//     up continued_from_workflow_run_id from anyRunID, then the page is read
//     forward from the root. Root discovery is the only depth-proportional work
//     and it walks an indexed single-column FK; everything else is O(limit).
//   - cursor != "" requests the page that follows the run whose id is cursor.
//     The forward walk seeds directly at that run's successor
//     (continued_from_workflow_run_id = cursor, served by
//     idx_workflow_runs_continued_from), so subsequent pages cost O(limit) with
//     no root re-discovery.
//
// limit bounds the number of rows returned; callers pass page-size+1 to detect
// whether more pages remain. An empty first page means anyRunID does not exist
// in projectID (ErrWorkflowRunNotFound); an empty subsequent page simply means
// the chain ends at the cursor.
//
// The walk is scoped to projectID so that a caller-supplied cursor (untrusted
// input) can never pull runs from another tenant's chain: a continue-as-new
// chain never crosses projects, so this scoping is both correct and a hard
// tenant-isolation boundary.
func (q *Queries) GetWorkflowRunChain(ctx context.Context, anyRunID, projectID string, limit int, cursor string) ([]domain.WorkflowRunChainEntry, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetWorkflowRunChain")
	defer span.End()

	if limit <= 0 {
		limit = 1
	}

	// The recursive chain CTE carries only (id, continued_to, pos) so the walk
	// stays cheap; the projection columns are joined onto the final page only.
	const projection = `
		SELECT wr.id, wr.lineage_depth, wr.status, wr.triggered_by,
		       wr.started_at, wr.finished_at, wr.created_at
		FROM chain c
		JOIN workflow_runs wr ON wr.id = c.id
		ORDER BY c.pos ASC
		LIMIT $2`

	var query string
	seed := anyRunID
	if cursor == "" {
		query = `
			WITH RECURSIVE ancestors AS (
				SELECT id, continued_from_workflow_run_id, 0 AS up_depth
				FROM workflow_runs
				WHERE id = $1 AND project_id = $3
				UNION ALL
				SELECT wr.id, wr.continued_from_workflow_run_id, a.up_depth + 1
				FROM workflow_runs wr
				JOIN ancestors a ON wr.id = a.continued_from_workflow_run_id
				WHERE a.up_depth < 1000000 AND wr.project_id = $3
			),
			root AS (
				SELECT id
				FROM ancestors
				WHERE continued_from_workflow_run_id IS NULL
				ORDER BY up_depth DESC
				LIMIT 1
			),
			chain AS (
				SELECT wr.id, wr.continued_to_workflow_run_id, 0 AS pos
				FROM workflow_runs wr
				JOIN root ON wr.id = root.id
				UNION ALL
				SELECT wr.id, wr.continued_to_workflow_run_id, c.pos + 1
				FROM workflow_runs wr
				JOIN chain c ON wr.id = c.continued_to_workflow_run_id
				WHERE c.pos < $2 - 1 AND wr.project_id = $3
			)` + projection
	} else {
		seed = cursor
		query = `
			WITH RECURSIVE chain AS (
				SELECT wr.id, wr.continued_to_workflow_run_id, 0 AS pos
				FROM workflow_runs wr
				WHERE wr.continued_from_workflow_run_id = $1 AND wr.project_id = $3
				UNION ALL
				SELECT wr.id, wr.continued_to_workflow_run_id, c.pos + 1
				FROM workflow_runs wr
				JOIN chain c ON wr.id = c.continued_to_workflow_run_id
				WHERE c.pos < $2 - 1 AND wr.project_id = $3
			)` + projection
	}

	rows, err := q.db.Query(ctx, query, seed, limit, projectID)
	if err != nil {
		return nil, fmt.Errorf("get workflow run chain: %w", err)
	}
	defer rows.Close()

	entries := make([]domain.WorkflowRunChainEntry, 0, limit)
	for rows.Next() {
		var entry domain.WorkflowRunChainEntry
		var startedAt, finishedAt *time.Time
		if scanErr := rows.Scan(
			&entry.ID,
			&entry.LineageDepth,
			&entry.Status,
			&entry.TriggeredBy,
			&startedAt,
			&finishedAt,
			&entry.CreatedAt,
		); scanErr != nil {
			return nil, fmt.Errorf("get workflow run chain scan: %w", scanErr)
		}
		entry.StartedAt = startedAt
		entry.FinishedAt = finishedAt
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get workflow run chain rows: %w", err)
	}
	if cursor == "" && len(entries) == 0 {
		return nil, ErrWorkflowRunNotFound
	}
	return entries, nil
}

func (q *Queries) ListStalledWorkflowRuns(ctx context.Context, threshold time.Duration) ([]domain.WorkflowRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListStalledWorkflowRuns")
	defer span.End()

	query := `
		SELECT id, workflow_id, project_id, status, triggered_by, payload,
		       workflow_version, max_parallel_steps, error, started_at, finished_at, expires_at,
		       retry_of_run_id, parent_workflow_run_id, parent_step_run_id, created_at, tags, workflow_version_id, created_by, trace_context, workflow_snapshot_id, expected_completion_at,
		       continued_from_workflow_run_id, continued_to_workflow_run_id, lineage_depth
		FROM workflow_runs wr
		WHERE wr.status = 'running'
		  AND wr.started_at IS NOT NULL
		  AND wr.started_at < NOW() - ($1::interval)
		  AND NOT EXISTS (
			SELECT 1 FROM workflow_step_runs sr
			WHERE sr.workflow_run_id = wr.id
			  AND sr.status = 'running'
		  )
		ORDER BY wr.started_at ASC
		LIMIT 200`

	interval := fmt.Sprintf("%f seconds", threshold.Seconds())
	rows, err := q.db.Query(ctx, query, interval)
	if err != nil {
		return nil, fmt.Errorf("list stalled workflow runs: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.WorkflowRun, 0, 16)
	for rows.Next() {
		run, scanErr := scanWorkflowRun(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list stalled workflow runs scan: %w", scanErr)
		}
		runs = append(runs, *run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list stalled workflow runs rows: %w", err)
	}
	return runs, nil
}

func (q *Queries) ListWorkflowRunsByTag(ctx context.Context, projectID, tagKey, tagValue string, limit int, cursor *time.Time) ([]domain.WorkflowRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListWorkflowRunsByTag")
	defer span.End()

	base := `
		SELECT id, workflow_id, project_id, status, triggered_by, payload,
		       workflow_version, max_parallel_steps, error, started_at, finished_at, expires_at,
		       retry_of_run_id, parent_workflow_run_id, parent_step_run_id, created_at, tags, workflow_version_id, created_by, trace_context, workflow_snapshot_id, expected_completion_at,
		       continued_from_workflow_run_id, continued_to_workflow_run_id, lineage_depth
		FROM workflow_runs
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
		return nil, fmt.Errorf("list workflow runs by tag: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.WorkflowRun, 0, limit)
	for rows.Next() {
		run, scanErr := scanWorkflowRun(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list workflow runs by tag scan: %w", scanErr)
		}
		runs = append(runs, *run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list workflow runs by tag rows: %w", err)
	}

	return runs, nil
}

// CountActiveWorkflowRunsByVersion returns the number of non-terminal workflow runs for a specific version.
func (q *Queries) CountActiveWorkflowRunsByVersion(ctx context.Context, workflowID, versionID string) (int, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CountActiveWorkflowRunsByVersion")
	defer span.End()

	var count int
	err := q.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM workflow_runs
		WHERE workflow_id = $1
		  AND workflow_version_id = $2
		  AND status IN ('pending', 'running', 'paused')
	`, workflowID, versionID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count active workflow runs by version: %w", err)
	}
	return count, nil
}

// ActiveVersion represents a workflow version with active run counts.
type ActiveVersion struct {
	VersionID string `json:"version_id"`
	Version   int    `json:"version"`
	Pending   int    `json:"pending"`
	Running   int    `json:"running"`
	Paused    int    `json:"paused"`
	Total     int    `json:"total"`
}

// ListActiveWorkflowVersions groups active workflow runs by version with status counts.
func (q *Queries) ListActiveWorkflowVersions(ctx context.Context, workflowID string) ([]ActiveVersion, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListActiveWorkflowVersions")
	defer span.End()

	rows, err := q.db.Query(ctx, `
		SELECT
			COALESCE(workflow_version_id, ''),
			workflow_version,
			COUNT(*) FILTER (WHERE status = 'pending') AS pending,
			COUNT(*) FILTER (WHERE status = 'running') AS running,
			COUNT(*) FILTER (WHERE status = 'paused') AS paused,
			COUNT(*) AS total
		FROM workflow_runs
		WHERE workflow_id = $1
		  AND status IN ('pending', 'running', 'paused')
		GROUP BY workflow_version_id, workflow_version
		ORDER BY workflow_version DESC
	`, workflowID)
	if err != nil {
		return nil, fmt.Errorf("list active workflow versions: %w", err)
	}
	defer rows.Close()

	var versions []ActiveVersion
	for rows.Next() {
		var v ActiveVersion
		if err := rows.Scan(&v.VersionID, &v.Version, &v.Pending, &v.Running, &v.Paused, &v.Total); err != nil {
			return nil, fmt.Errorf("list active workflow versions scan: %w", err)
		}
		versions = append(versions, v)
	}
	return versions, rows.Err()
}

func (q *Queries) BulkCancelWorkflowRuns(ctx context.Context, projectID string, ids []string, now time.Time) ([]string, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.BulkCancelWorkflowRuns")
	defer span.End()

	rows, err := q.db.Query(ctx, `
		UPDATE workflow_runs
		SET status = 'canceled', finished_at = $2, error = 'canceled by user (bulk)'
		WHERE id = ANY($1) AND project_id = $3
		  AND status NOT IN ('completed', 'failed', 'timed_out', 'canceled', 'compensated', 'compensation_failed', 'continued')
		RETURNING id
	`, ids, now, projectID)
	if err != nil {
		return nil, fmt.Errorf("bulk cancel workflow runs: %w", err)
	}
	defer rows.Close()

	var canceled []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("bulk cancel workflow runs scan: %w", err)
		}
		canceled = append(canceled, id)
	}
	return canceled, rows.Err()
}
