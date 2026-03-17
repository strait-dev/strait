package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"strait/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

// GetOrCreateWorkflowSnapshot returns an existing snapshot for the given
// (workflowID, versionID) pair, or creates a new one by serializing the
// current workflow metadata and steps.
func (q *Queries) GetOrCreateWorkflowSnapshot(
	ctx context.Context,
	wf *domain.Workflow,
	steps []domain.WorkflowStep,
) (*domain.WorkflowSnapshot, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetOrCreateWorkflowSnapshot")
	defer span.End()

	// If the workflow has a versionID, try to find an existing snapshot (dedup).
	if wf.VersionID != "" {
		existing, err := q.getWorkflowSnapshotByVersion(ctx, wf.ID, wf.VersionID)
		if err == nil && existing != nil {
			return existing, nil
		}
		// Not found — create a new one.
	}

	def := domain.WorkflowSnapshotDefinition{
		Workflow: domain.WorkflowSnapshotMeta{
			ID:                wf.ID,
			ProjectID:         wf.ProjectID,
			Name:              wf.Name,
			Slug:              wf.Slug,
			Description:       wf.Description,
			Tags:              wf.Tags,
			Version:           wf.Version,
			VersionID:         wf.VersionID,
			TimeoutSecs:       wf.TimeoutSecs,
			MaxConcurrentRuns: wf.MaxConcurrentRuns,
			MaxParallelSteps:  wf.MaxParallelSteps,
		},
		Steps: steps,
	}

	defJSON, err := json.Marshal(def)
	if err != nil {
		return nil, fmt.Errorf("marshal snapshot definition: %w", err)
	}

	snapshot := &domain.WorkflowSnapshot{
		ID:         uuid.Must(uuid.NewV7()).String(),
		WorkflowID: wf.ID,
		VersionID:  wf.VersionID,
		Version:    wf.Version,
		Definition: defJSON,
	}

	query := `
		INSERT INTO workflow_snapshots (id, workflow_id, version_id, version, definition)
		VALUES ($1, $2, $3, $4, $5::jsonb)
		ON CONFLICT (workflow_id, version_id) WHERE version_id != ''
		DO UPDATE SET id = workflow_snapshots.id
		RETURNING id, created_at`

	err = q.db.QueryRow(
		ctx, query,
		snapshot.ID,
		snapshot.WorkflowID,
		snapshot.VersionID,
		snapshot.Version,
		snapshot.Definition,
	).Scan(&snapshot.ID, &snapshot.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert workflow snapshot: %w", err)
	}

	return snapshot, nil
}

// GetWorkflowSnapshot retrieves a workflow snapshot by ID.
func (q *Queries) GetWorkflowSnapshot(ctx context.Context, id string) (*domain.WorkflowSnapshot, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetWorkflowSnapshot")
	defer span.End()

	query := `
		SELECT id, workflow_id, version_id, version, definition, created_at
		FROM workflow_snapshots
		WHERE id = $1`

	var s domain.WorkflowSnapshot
	var defBytes []byte
	err := q.db.QueryRow(ctx, query, id).Scan(
		&s.ID, &s.WorkflowID, &s.VersionID, &s.Version, &defBytes, &s.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get workflow snapshot: %w", err)
	}
	s.Definition = json.RawMessage(defBytes)
	return &s, nil
}

// getWorkflowSnapshotByVersion retrieves a snapshot by (workflowID, versionID).
func (q *Queries) getWorkflowSnapshotByVersion(ctx context.Context, workflowID, versionID string) (*domain.WorkflowSnapshot, error) {
	query := `
		SELECT id, workflow_id, version_id, version, definition, created_at
		FROM workflow_snapshots
		WHERE workflow_id = $1 AND version_id = $2`

	var s domain.WorkflowSnapshot
	var defBytes []byte
	err := q.db.QueryRow(ctx, query, workflowID, versionID).Scan(
		&s.ID, &s.WorkflowID, &s.VersionID, &s.Version, &defBytes, &s.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get workflow snapshot by version: %w", err)
	}
	s.Definition = json.RawMessage(defBytes)
	return &s, nil
}

// ParseSnapshotDefinition deserializes a snapshot's JSONB definition into steps
// and validates that the result is well-formed.
func ParseSnapshotDefinition(definition json.RawMessage) (*domain.WorkflowSnapshotDefinition, error) {
	if len(definition) == 0 {
		return nil, fmt.Errorf("snapshot definition is empty")
	}
	var def domain.WorkflowSnapshotDefinition
	if err := json.Unmarshal(definition, &def); err != nil {
		return nil, fmt.Errorf("unmarshal snapshot definition: %w", err)
	}
	// Validate: check for duplicate step refs which would silently overwrite
	// in any stepByRef map built from these steps.
	seen := make(map[string]struct{}, len(def.Steps))
	for _, step := range def.Steps {
		if _, dup := seen[step.StepRef]; dup && step.StepRef != "" {
			return nil, fmt.Errorf("duplicate step_ref %q in snapshot", step.StepRef)
		}
		seen[step.StepRef] = struct{}{}
	}
	return &def, nil
}

// ListWorkflowSnapshotsByWorkflow returns snapshots for a workflow, newest first.
func (q *Queries) ListWorkflowSnapshotsByWorkflow(ctx context.Context, workflowID string, limit int) ([]domain.WorkflowSnapshot, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListWorkflowSnapshotsByWorkflow")
	defer span.End()

	query := `
		SELECT id, workflow_id, version_id, version, definition, created_at
		FROM workflow_snapshots
		WHERE workflow_id = $1
		ORDER BY created_at DESC
		LIMIT $2`

	rows, err := q.db.Query(ctx, query, workflowID, limit)
	if err != nil {
		return nil, fmt.Errorf("list workflow snapshots: %w", err)
	}
	defer rows.Close()

	snapshots := make([]domain.WorkflowSnapshot, 0, limit)
	for rows.Next() {
		var s domain.WorkflowSnapshot
		var defBytes []byte
		if err := rows.Scan(&s.ID, &s.WorkflowID, &s.VersionID, &s.Version, &defBytes, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan workflow snapshot: %w", err)
		}
		s.Definition = json.RawMessage(defBytes)
		snapshots = append(snapshots, s)
	}

	return snapshots, rows.Err()
}

// WorkflowSnapshotStore defines snapshot operations for the Store interface.
type WorkflowSnapshotStore interface {
	GetOrCreateWorkflowSnapshot(ctx context.Context, wf *domain.Workflow, steps []domain.WorkflowStep) (*domain.WorkflowSnapshot, error)
	GetWorkflowSnapshot(ctx context.Context, id string) (*domain.WorkflowSnapshot, error)
	ListWorkflowSnapshotsByWorkflow(ctx context.Context, workflowID string, limit int) ([]domain.WorkflowSnapshot, error)
}

// Ensure Queries implements WorkflowSnapshotStore.
var _ WorkflowSnapshotStore = (*Queries)(nil)
