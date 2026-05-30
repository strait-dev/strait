package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"strait/internal/dbscan"
	"strait/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

func (q *Queries) CreateWorkflow(ctx context.Context, w *domain.Workflow) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateWorkflow")
	defer span.End()

	if err := q.requireCurrentProjectContext(ctx, w.ProjectID); err != nil {
		return fmt.Errorf("create workflow: %w", err)
	}
	if w.ID == "" {
		w.ID = uuid.Must(uuid.NewV7()).String()
	}
	w.Version = 1
	if w.VersionID == "" {
		w.VersionID = domain.NewVersionID()
	}
	if w.VersionPolicy == "" {
		w.VersionPolicy = domain.VersionPolicyPin
	}

	tagsJSON, err := marshalTags(w.Tags)
	if err != nil {
		return fmt.Errorf("create workflow: %w", err)
	}

	query := `
		INSERT INTO workflows (
			id, project_id, name, slug, description, enabled, version, timeout_secs, max_concurrent_runs,
			max_parallel_steps, cron, cron_timezone, skip_if_running,
			tags, version_id, version_policy, backwards_compatible, created_by, updated_by,
			singleton_key_expr, singleton_on_conflict, singleton_max_queue_depth, singleton_preempt_higher_priority
		)
		VALUES ($1, $2, $3, $4, $5, $6, 1, $7, $8, $9, $10, $11, $12,
			$13::jsonb, $14, $15, $16, $17, $18, $19, $20, $21, $22)
		RETURNING created_at, updated_at, version`

	err = q.db.QueryRow(
		ctx,
		query,
		w.ID,
		w.ProjectID,
		w.Name,
		w.Slug,
		dbscan.NilIfEmptyString(w.Description),
		w.Enabled,
		w.TimeoutSecs,
		w.MaxConcurrentRuns,
		w.MaxParallelSteps,
		dbscan.NilIfEmptyString(w.Cron),
		dbscan.NilIfEmptyString(w.CronTimezone),
		w.SkipIfRunning,
		tagsJSON,
		w.VersionID,
		string(w.VersionPolicy),
		w.BackwardsCompatible,
		dbscan.NilIfEmptyString(w.CreatedBy),
		dbscan.NilIfEmptyString(w.UpdatedBy),
		dbscan.NilIfEmptyRawMessage(w.SingletonKeyExpr),
		dbscan.NilIfEmptyString(string(w.SingletonOnConflict)),
		w.SingletonMaxQueueDepth,
		w.SingletonPreemptHigher,
	).Scan(&w.CreatedAt, &w.UpdatedAt, &w.Version)
	if err != nil {
		return fmt.Errorf("create workflow: %w", err)
	}

	return nil
}

func (q *Queries) requireCurrentProjectContext(ctx context.Context, projectID string) error {
	var currentProjectID string
	if err := q.db.QueryRow(ctx, `SELECT COALESCE(current_setting('app.current_project_id', true), '')`).Scan(&currentProjectID); err != nil {
		return fmt.Errorf("read project context: %w", err)
	}
	if currentProjectID != "" && currentProjectID != projectID {
		return ErrProjectContextMismatch
	}
	return nil
}

func (q *Queries) GetWorkflow(ctx context.Context, id string) (*domain.Workflow, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetWorkflow")
	defer span.End()

	query := `
		SELECT id, project_id, name, slug, description, enabled, version, timeout_secs, max_concurrent_runs,
		       max_parallel_steps, cron, cron_timezone, skip_if_running, tags, version_id, version_policy, backwards_compatible, created_by, updated_by, created_at, updated_at,
		       singleton_key_expr, singleton_on_conflict, singleton_max_queue_depth, singleton_preempt_higher_priority
		FROM workflows
		WHERE id = $1`

	w, err := scanWorkflow(q.db.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrWorkflowNotFound
		}
		return nil, fmt.Errorf("get workflow: %w", err)
	}

	return w, nil
}

func (q *Queries) GetWorkflowBySlug(ctx context.Context, projectID, slug string) (*domain.Workflow, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetWorkflowBySlug")
	defer span.End()

	query := `
		SELECT id, project_id, name, slug, description, enabled, version, timeout_secs, max_concurrent_runs,
		       max_parallel_steps, cron, cron_timezone, skip_if_running, tags, version_id, version_policy, backwards_compatible, created_by, updated_by, created_at, updated_at,
		       singleton_key_expr, singleton_on_conflict, singleton_max_queue_depth, singleton_preempt_higher_priority
		FROM workflows
		WHERE project_id = $1 AND slug = $2`

	w, err := scanWorkflow(q.db.QueryRow(ctx, query, projectID, slug))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrWorkflowNotFound
		}
		return nil, fmt.Errorf("get workflow by slug: %w", err)
	}

	return w, nil
}

// GetWorkflowsByIDs fetches multiple workflows in a single query, keyed by
// workflow id. Ids with no matching row are absent from the result map (no
// error), so callers can treat a missing key the same as a not-found
// GetWorkflow. The empty input is a no-op.
func (q *Queries) GetWorkflowsByIDs(ctx context.Context, ids []string) (map[string]*domain.Workflow, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetWorkflowsByIDs")
	defer span.End()

	result := make(map[string]*domain.Workflow, len(ids))
	if len(ids) == 0 {
		return result, nil
	}

	query := `
		SELECT id, project_id, name, slug, description, enabled, version, timeout_secs, max_concurrent_runs,
		       max_parallel_steps, cron, cron_timezone, skip_if_running, tags, version_id, version_policy, backwards_compatible, created_by, updated_by, created_at, updated_at,
		       singleton_key_expr, singleton_on_conflict, singleton_max_queue_depth, singleton_preempt_higher_priority
		FROM workflows
		WHERE id = ANY($1)`

	rows, err := q.db.Query(ctx, query, ids)
	if err != nil {
		return nil, fmt.Errorf("get workflows by ids: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		w, scanErr := scanWorkflow(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("get workflows by ids scan: %w", scanErr)
		}
		result[w.ID] = w
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get workflows by ids rows: %w", err)
	}

	return result, nil
}

func (q *Queries) ListWorkflows(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.Workflow, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListWorkflows")
	defer span.End()

	query := `
		SELECT id, project_id, name, slug, description, enabled, version, timeout_secs, max_concurrent_runs,
		       max_parallel_steps, cron, cron_timezone, skip_if_running, tags, version_id, version_policy, backwards_compatible, created_by, updated_by, created_at, updated_at,
		       singleton_key_expr, singleton_on_conflict, singleton_max_queue_depth, singleton_preempt_higher_priority
		FROM workflows
		WHERE project_id = $1`

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
		return nil, fmt.Errorf("list workflows: %w", err)
	}
	defer rows.Close()

	workflows := make([]domain.Workflow, 0, 16)
	for rows.Next() {
		workflow, scanErr := scanWorkflow(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list workflows scan: %w", scanErr)
		}
		workflows = append(workflows, *workflow)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list workflows rows: %w", err)
	}

	return workflows, nil
}

func (q *Queries) UpdateWorkflow(ctx context.Context, w *domain.Workflow) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpdateWorkflow")
	defer span.End()

	newVersionID := domain.NewVersionID()

	tagsJSON, err := marshalTags(w.Tags)
	if err != nil {
		return fmt.Errorf("update workflow: %w", err)
	}

	snapshotID := fmt.Sprintf("%s:v%d-pre", w.ID, w.Version)

	query := `
		WITH snapshot AS (
			INSERT INTO workflow_versions (
				id, workflow_id, version, project_id, name, slug, description, enabled,
				timeout_secs, max_concurrent_runs, max_parallel_steps, cron, cron_timezone, skip_if_running,
				backwards_compatible, created_by, updated_by,
				singleton_key_expr, singleton_on_conflict, singleton_max_queue_depth, singleton_preempt_higher_priority
			)
			SELECT $17, id, version, project_id, name, slug, description, enabled,
			       timeout_secs, max_concurrent_runs, max_parallel_steps, cron, cron_timezone, skip_if_running,
			       backwards_compatible, created_by, updated_by,
			       singleton_key_expr, singleton_on_conflict, singleton_max_queue_depth, singleton_preempt_higher_priority
			FROM workflows WHERE id = $11
			ON CONFLICT (workflow_id, version) DO NOTHING
		)
		UPDATE workflows
		SET name = $1,
		    slug = $2,
		    description = $3,
		    enabled = $4,
		    timeout_secs = $5,
		    max_concurrent_runs = $6,
		    max_parallel_steps = $7,
		    cron = $8,
		    cron_timezone = $9,
		    skip_if_running = $10,
		    tags = $12::jsonb,
		    version_id = $13,
		    updated_by = $14,
		    version_policy = $15,
		    backwards_compatible = $16,
		    singleton_key_expr = $18,
		    singleton_on_conflict = $19,
		    singleton_max_queue_depth = $20,
		    singleton_preempt_higher_priority = $21,
		    version = version + 1,
		    updated_at = NOW()
		WHERE id = $11
		RETURNING updated_at, version, version_id`

	err = q.db.QueryRow(
		ctx,
		query,
		w.Name,
		w.Slug,
		dbscan.NilIfEmptyString(w.Description),
		w.Enabled,
		w.TimeoutSecs,
		w.MaxConcurrentRuns,
		w.MaxParallelSteps,
		dbscan.NilIfEmptyString(w.Cron),
		dbscan.NilIfEmptyString(w.CronTimezone),
		w.SkipIfRunning,
		w.ID,
		tagsJSON,
		newVersionID,
		dbscan.NilIfEmptyString(w.UpdatedBy),
		string(w.VersionPolicy),
		w.BackwardsCompatible,
		snapshotID,
		dbscan.NilIfEmptyRawMessage(w.SingletonKeyExpr),
		dbscan.NilIfEmptyString(string(w.SingletonOnConflict)),
		w.SingletonMaxQueueDepth,
		w.SingletonPreemptHigher,
	).Scan(&w.UpdatedAt, &w.Version, &w.VersionID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrWorkflowNotFound
		}
		return fmt.Errorf("update workflow: %w", err)
	}

	return nil
}

func (q *Queries) DeleteWorkflow(ctx context.Context, id string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteWorkflow")
	defer span.End()

	if _, ok := q.db.(TxBeginner); ok {
		return q.withTx(ctx, func(tx *Queries) error {
			return tx.deleteWorkflowTx(ctx, id)
		})
	}

	return q.deleteWorkflowTx(ctx, id)
}

// ErrWorkflowHasActiveRuns is returned when deleting a workflow that still has
// non-terminal runs.
var ErrWorkflowHasActiveRuns = errors.New("workflow has active runs")

func (q *Queries) deleteWorkflowTx(ctx context.Context, id string) error {
	var exists bool
	if err := q.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM workflows WHERE id = $1 FOR UPDATE)`, id).Scan(&exists); err != nil {
		return fmt.Errorf("delete workflow lock: %w", err)
	}
	if !exists {
		return ErrWorkflowNotFound
	}

	var activeCount int
	err := q.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM workflow_runs
		WHERE workflow_id = $1
		  AND status NOT IN ('completed', 'failed', 'timed_out', 'canceled', 'compensated', 'compensation_failed')`,
		id,
	).Scan(&activeCount)
	if err != nil {
		return fmt.Errorf("delete workflow check active runs: %w", err)
	}
	if activeCount > 0 {
		return ErrWorkflowHasActiveRuns
	}

	if _, err := q.db.Exec(ctx, `
		DELETE FROM event_triggers
		WHERE workflow_run_id IN (SELECT id FROM workflow_runs WHERE workflow_id = $1)`,
		id,
	); err != nil {
		return fmt.Errorf("delete workflow event triggers: %w", err)
	}
	if _, err := q.db.Exec(ctx, `DELETE FROM workflow_runs WHERE workflow_id = $1`, id); err != nil {
		return fmt.Errorf("delete workflow runs: %w", err)
	}

	tag, err := q.db.Exec(ctx, `DELETE FROM workflows WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete workflow: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrWorkflowNotFound
	}

	return nil
}

func scanWorkflow(scanner scanTarget) (*domain.Workflow, error) {
	var workflow domain.Workflow
	var description *string
	var cron *string
	var cronTimezone *string
	var tagsJSON []byte
	var versionID *string
	var versionPolicy *string
	var createdBy *string
	var updatedBy *string
	var singletonKeyExpr []byte
	var singletonOnConflict *string
	var singletonMaxQueueDepth *int
	var singletonPreemptHigher bool

	err := scanner.Scan(
		&workflow.ID,
		&workflow.ProjectID,
		&workflow.Name,
		&workflow.Slug,
		&description,
		&workflow.Enabled,
		&workflow.Version,
		&workflow.TimeoutSecs,
		&workflow.MaxConcurrentRuns,
		&workflow.MaxParallelSteps,
		&cron,
		&cronTimezone,
		&workflow.SkipIfRunning,
		&tagsJSON,
		&versionID,
		&versionPolicy,
		&workflow.BackwardsCompatible,
		&createdBy,
		&updatedBy,
		&workflow.CreatedAt,
		&workflow.UpdatedAt,
		&singletonKeyExpr,
		&singletonOnConflict,
		&singletonMaxQueueDepth,
		&singletonPreemptHigher,
	)
	if err != nil {
		return nil, err
	}

	if description != nil {
		workflow.Description = *description
	}
	if singletonKeyExpr != nil {
		workflow.SingletonKeyExpr = json.RawMessage(singletonKeyExpr)
	}
	if singletonOnConflict != nil {
		workflow.SingletonOnConflict = domain.SingletonOnConflict(*singletonOnConflict)
	}
	if singletonMaxQueueDepth != nil {
		workflow.SingletonMaxQueueDepth = singletonMaxQueueDepth
	}
	workflow.SingletonPreemptHigher = singletonPreemptHigher
	if cron != nil {
		workflow.Cron = *cron
	}
	if cronTimezone != nil {
		workflow.CronTimezone = *cronTimezone
	}
	if len(tagsJSON) > 0 {
		tags, tagErr := unmarshalTags(tagsJSON)
		if tagErr != nil {
			return nil, tagErr
		}
		workflow.Tags = tags
	}
	if versionID != nil {
		workflow.VersionID = *versionID
	}
	if versionPolicy != nil {
		workflow.VersionPolicy = domain.VersionPolicy(*versionPolicy)
	}
	if createdBy != nil {
		workflow.CreatedBy = *createdBy
	}
	if updatedBy != nil {
		workflow.UpdatedBy = *updatedBy
	}

	return &workflow, nil
}

func (q *Queries) ListCronWorkflows(ctx context.Context) ([]domain.Workflow, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListCronWorkflows")
	defer span.End()

	query := `
		SELECT w.id, w.project_id, w.name, w.slug, w.description, w.enabled, w.version, w.timeout_secs, w.max_concurrent_runs,
		       w.max_parallel_steps, w.cron, w.cron_timezone, w.skip_if_running, w.tags, w.version_id, w.version_policy, w.backwards_compatible, w.created_by, w.updated_by, w.created_at, w.updated_at,
		       w.singleton_key_expr, w.singleton_on_conflict, w.singleton_max_queue_depth, w.singleton_preempt_higher_priority
		FROM workflows w
		JOIN projects p ON p.id = w.project_id
		WHERE w.enabled = TRUE
		  AND w.cron IS NOT NULL
		  AND w.cron <> ''
		  AND p.deleted_at IS NULL
		  AND COALESCE(p.suspended, false) = false
		ORDER BY w.created_at DESC`

	rows, err := q.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list cron workflows: %w", err)
	}
	defer rows.Close()

	workflows := make([]domain.Workflow, 0, 8)
	for rows.Next() {
		workflow, scanErr := scanWorkflow(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list cron workflows scan: %w", scanErr)
		}
		workflows = append(workflows, *workflow)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list cron workflows rows: %w", err)
	}

	return workflows, nil
}

func (q *Queries) ListWorkflowsByTag(ctx context.Context, projectID, tagKey, tagValue string, limit int, cursor *time.Time) ([]domain.Workflow, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListWorkflowsByTag")
	defer span.End()

	base := `
		SELECT id, project_id, name, slug, description, enabled, version, timeout_secs, max_concurrent_runs,
		       max_parallel_steps, cron, cron_timezone, skip_if_running, tags, version_id, version_policy, backwards_compatible, created_by, updated_by, created_at, updated_at,
		       singleton_key_expr, singleton_on_conflict, singleton_max_queue_depth, singleton_preempt_higher_priority
		FROM workflows
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
		return nil, fmt.Errorf("list workflows by tag: %w", err)
	}
	defer rows.Close()

	workflows := make([]domain.Workflow, 0, limit)
	for rows.Next() {
		wf, scanErr := scanWorkflow(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list workflows by tag scan: %w", scanErr)
		}
		workflows = append(workflows, *wf)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list workflows by tag rows: %w", err)
	}

	return workflows, nil
}
