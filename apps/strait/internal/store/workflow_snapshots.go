package store

import (
	"context"
	"crypto/sha256"
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
	hash := workflowSnapshotDefinitionHash(defJSON)

	// If the workflow has a versionID, dedupe only exact definition matches.
	// Trigger-time step overrides can change the serialized step set while
	// keeping the workflow version stable, so version_id alone is insufficient.
	if wf.VersionID != "" {
		existing, err := q.getWorkflowSnapshotByVersionAndHash(ctx, wf.ProjectID, wf.ID, wf.VersionID, hash)
		if err == nil && existing != nil {
			return existing, nil
		}
		// Not found — create a new definition variant.
	}

	snapshot := &domain.WorkflowSnapshot{
		ID:         uuid.Must(uuid.NewV7()).String(),
		WorkflowID: wf.ID,
		VersionID:  wf.VersionID,
		Version:    wf.Version,
		Definition: defJSON,
	}

	query := `
		INSERT INTO workflow_snapshots (id, workflow_id, project_id, version_id, version, definition, definition_hash)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7)
		ON CONFLICT (workflow_id, version_id, definition_hash) WHERE version_id != ''
		DO UPDATE SET id = workflow_snapshots.id
		RETURNING id, created_at`

	err = q.db.QueryRow(
		ctx, query,
		snapshot.ID,
		snapshot.WorkflowID,
		wf.ProjectID,
		snapshot.VersionID,
		snapshot.Version,
		snapshot.Definition,
		hash,
	).Scan(&snapshot.ID, &snapshot.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert workflow snapshot: %w", err)
	}

	return snapshot, nil
}

func workflowSnapshotDefinitionHash(definition []byte) string {
	sum := sha256.Sum256(definition)
	return fmt.Sprintf("%x", sum[:])
}

// GetWorkflowSnapshot retrieves a workflow snapshot by ID, scoped to the
// caller's project so a snapshot id from one tenant cannot read another
// tenant's snapshot definition.
func (q *Queries) GetWorkflowSnapshot(ctx context.Context, projectID, id string) (*domain.WorkflowSnapshot, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetWorkflowSnapshot")
	defer span.End()

	query := `
		SELECT id, workflow_id, version_id, version, definition, created_at
		FROM workflow_snapshots
		WHERE id = $1 AND project_id = $2`

	var s domain.WorkflowSnapshot
	var defBytes []byte
	err := q.db.QueryRow(ctx, query, id, projectID).Scan(
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

// getWorkflowSnapshotByVersionAndHash retrieves a snapshot by exact definition
// identity within a workflow version.
func (q *Queries) getWorkflowSnapshotByVersionAndHash(ctx context.Context, projectID, workflowID, versionID, definitionHash string) (*domain.WorkflowSnapshot, error) {
	query := `
		SELECT id, workflow_id, version_id, version, definition, created_at
		FROM workflow_snapshots
		WHERE workflow_id = $1 AND version_id = $2 AND definition_hash = $3 AND project_id = $4`

	var s domain.WorkflowSnapshot
	var defBytes []byte
	err := q.db.QueryRow(ctx, query, workflowID, versionID, definitionHash, projectID).Scan(
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

// ListWorkflowSnapshotsByWorkflow returns snapshots for a workflow, newest
// first, scoped to the caller's project.
func (q *Queries) ListWorkflowSnapshotsByWorkflow(ctx context.Context, projectID, workflowID string, limit int) ([]domain.WorkflowSnapshot, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListWorkflowSnapshotsByWorkflow")
	defer span.End()

	query := `
		SELECT id, workflow_id, version_id, version, definition, created_at
		FROM workflow_snapshots
		WHERE workflow_id = $1 AND project_id = $2
		ORDER BY created_at DESC
		LIMIT $3`

	rows, err := q.db.Query(ctx, query, workflowID, projectID, limit)
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
	GetWorkflowSnapshot(ctx context.Context, projectID, id string) (*domain.WorkflowSnapshot, error)
	ListWorkflowSnapshotsByWorkflow(ctx context.Context, projectID, workflowID string, limit int) ([]domain.WorkflowSnapshot, error)
}

// Ensure Queries implements WorkflowSnapshotStore.
var _ WorkflowSnapshotStore = (*Queries)(nil)
