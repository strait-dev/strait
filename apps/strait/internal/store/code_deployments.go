package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"strait/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

// ErrCodeDeploymentNotFound is returned when a deployment does not exist or
// belongs to a different project (treated identically for security).
var ErrCodeDeploymentNotFound = errors.New("code deployment not found")

// CreateCodeDeployment inserts a new code deployment record with status "pending".
// The caller must have already generated a presigned upload URL via the object store.
func (q *Queries) CreateCodeDeployment(ctx context.Context, d *domain.CodeDeployment) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateCodeDeployment")
	defer span.End()

	if d.ID == "" {
		d.ID = uuid.Must(uuid.NewV7()).String()
	}
	if d.Status == "" {
		d.Status = domain.DeploymentStatusPending
	}

	// Derive the version number: max(version) + 1 for this job.
	versionQuery := `
		SELECT COALESCE(MAX(version), 0) + 1
		FROM code_deployments
		WHERE job_id = $1`
	if err := q.db.QueryRow(ctx, versionQuery, d.JobID).Scan(&d.Version); err != nil {
		return fmt.Errorf("create code deployment: get next version: %w", err)
	}

	var createdBy *string
	if d.CreatedBy != "" {
		createdBy = &d.CreatedBy
	}

	query := `
		INSERT INTO code_deployments (
			id, job_id, project_id, version, status, runtime,
			source_hash, source_size_bytes, source_uri, created_by
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING created_at, updated_at`

	err := q.db.QueryRow(ctx, query,
		d.ID,
		d.JobID,
		d.ProjectID,
		d.Version,
		string(d.Status),
		string(d.Runtime),
		d.SourceHash,
		d.SourceSizeBytes,
		d.SourceURI,
		createdBy,
	).Scan(&d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create code deployment: %w", err)
	}
	return nil
}

// GetCodeDeployment fetches a deployment by ID, scoped to the given project.
// Returns ErrCodeDeploymentNotFound if the deployment does not exist or belongs
// to a different project.
func (q *Queries) GetCodeDeployment(ctx context.Context, id, projectID string) (*domain.CodeDeployment, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetCodeDeployment")
	defer span.End()

	query := `
		SELECT id, job_id, project_id, version, status, runtime,
		       source_hash, source_size_bytes, source_uri,
		       built_image_uri, built_image_digest, build_logs, error_message,
		       created_by, created_at, updated_at, finished_at
		FROM code_deployments
		WHERE id = $1 AND project_id = $2`

	var d domain.CodeDeployment
	var status, runtime string
	var builtImageURI, builtImageDigest, buildLogs, errorMessage, createdBy *string

	err := q.db.QueryRow(ctx, query, id, projectID).Scan(
		&d.ID,
		&d.JobID,
		&d.ProjectID,
		&d.Version,
		&status,
		&runtime,
		&d.SourceHash,
		&d.SourceSizeBytes,
		&d.SourceURI,
		&builtImageURI,
		&builtImageDigest,
		&buildLogs,
		&errorMessage,
		&createdBy,
		&d.CreatedAt,
		&d.UpdatedAt,
		&d.FinishedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrCodeDeploymentNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get code deployment: %w", err)
	}

	d.Status = domain.DeploymentBuildStatus(status)
	d.Runtime = domain.Runtime(runtime)
	if builtImageURI != nil {
		d.BuiltImageURI = *builtImageURI
	}
	if builtImageDigest != nil {
		d.BuiltImageDigest = *builtImageDigest
	}
	if buildLogs != nil {
		d.BuildLogs = *buildLogs
	}
	if errorMessage != nil {
		d.ErrorMessage = *errorMessage
	}
	if createdBy != nil {
		d.CreatedBy = *createdBy
	}
	return &d, nil
}

// ListCodeDeployments returns deployments for a job in descending creation order.
func (q *Queries) ListCodeDeployments(ctx context.Context, jobID, projectID string, limit int, cursor *time.Time) ([]domain.CodeDeployment, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListCodeDeployments")
	defer span.End()

	query := `
		SELECT id, job_id, project_id, version, status, runtime,
		       source_hash, source_size_bytes, source_uri,
		       built_image_uri, built_image_digest, build_logs, error_message,
		       created_by, created_at, updated_at, finished_at
		FROM code_deployments
		WHERE job_id = $1 AND project_id = $2
		  AND ($3::TIMESTAMPTZ IS NULL OR created_at < $3)
		ORDER BY created_at DESC
		LIMIT $4`

	rows, err := q.db.Query(ctx, query, jobID, projectID, cursor, limit)
	if err != nil {
		return nil, fmt.Errorf("list code deployments: %w", err)
	}
	defer rows.Close()

	var result []domain.CodeDeployment
	for rows.Next() {
		var d domain.CodeDeployment
		var status, runtime string
		var builtImageURI, builtImageDigest, buildLogs, errorMessage, createdBy *string

		if err := rows.Scan(
			&d.ID,
			&d.JobID,
			&d.ProjectID,
			&d.Version,
			&status,
			&runtime,
			&d.SourceHash,
			&d.SourceSizeBytes,
			&d.SourceURI,
			&builtImageURI,
			&builtImageDigest,
			&buildLogs,
			&errorMessage,
			&createdBy,
			&d.CreatedAt,
			&d.UpdatedAt,
			&d.FinishedAt,
		); err != nil {
			return nil, fmt.Errorf("list code deployments: scan: %w", err)
		}

		d.Status = domain.DeploymentBuildStatus(status)
		d.Runtime = domain.Runtime(runtime)
		if builtImageURI != nil {
			d.BuiltImageURI = *builtImageURI
		}
		if builtImageDigest != nil {
			d.BuiltImageDigest = *builtImageDigest
		}
		if buildLogs != nil {
			d.BuildLogs = *buildLogs
		}
		if errorMessage != nil {
			d.ErrorMessage = *errorMessage
		}
		if createdBy != nil {
			d.CreatedBy = *createdBy
		}
		result = append(result, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list code deployments: rows: %w", err)
	}
	return result, nil
}

// UpdateCodeDeploymentStatus transitions a deployment to a new status and
// writes optional build output fields (image URI, digest, logs, error).
// finished_at is set automatically when transitioning to a terminal status.
func (q *Queries) UpdateCodeDeploymentStatus(ctx context.Context, id string, status domain.DeploymentBuildStatus, fields map[string]any) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpdateCodeDeploymentStatus")
	defer span.End()

	terminal := status == domain.DeploymentStatusReady || status == domain.DeploymentStatusFailed
	var finishedAt *time.Time
	if terminal {
		now := time.Now().UTC()
		finishedAt = &now
	}

	query := `
		UPDATE code_deployments
		SET status             = $2,
		    updated_at         = NOW(),
		    finished_at        = COALESCE($3, finished_at),
		    built_image_uri    = COALESCE($4, built_image_uri),
		    built_image_digest = COALESCE($5, built_image_digest),
		    build_logs         = COALESCE($6, build_logs),
		    error_message      = COALESCE($7, error_message)
		WHERE id = $1`

	_, err := q.db.Exec(ctx, query,
		id,
		string(status),
		finishedAt,
		nilStringFromMap(fields, "built_image_uri"),
		nilStringFromMap(fields, "built_image_digest"),
		nilStringFromMap(fields, "build_logs"),
		nilStringFromMap(fields, "error_message"),
	)
	if err != nil {
		return fmt.Errorf("update code deployment status: %w", err)
	}
	return nil
}

// SetActiveDeployment atomically swaps the active_deployment_id on the job and
// sets source_type to "code". Called after a successful build.
func (q *Queries) SetActiveDeployment(ctx context.Context, jobID, deploymentID, projectID string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.SetActiveDeployment")
	defer span.End()

	query := `
		UPDATE jobs
		SET active_deployment_id = $2,
		    source_type          = 'code',
		    updated_at           = NOW()
		WHERE id = $1 AND project_id = $3`

	tag, err := q.db.Exec(ctx, query, jobID, deploymentID, projectID)
	if err != nil {
		return fmt.Errorf("set active deployment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("set active deployment: job not found or project mismatch")
	}
	return nil
}

// RollbackToDeployment sets an earlier deployment as the active one.
// The target deployment must be in "ready" status and belong to the same job + project.
func (q *Queries) RollbackToDeployment(ctx context.Context, jobID, deploymentID, projectID string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.RollbackToDeployment")
	defer span.End()

	// Verify the target deployment is ready and belongs to this job+project.
	var count int
	checkQuery := `
		SELECT COUNT(1)
		FROM code_deployments
		WHERE id = $1 AND job_id = $2 AND project_id = $3 AND status = 'ready'`
	if err := q.db.QueryRow(ctx, checkQuery, deploymentID, jobID, projectID).Scan(&count); err != nil {
		return fmt.Errorf("rollback to deployment: verify: %w", err)
	}
	if count == 0 {
		return ErrCodeDeploymentNotFound
	}
	return q.SetActiveDeployment(ctx, jobID, deploymentID, projectID)
}

// ListBuildingDeployments returns up to limit deployments currently in "building"
// status, ordered by creation time ascending (oldest first so we process in order).
// Called by the build orchestrator to find work.
func (q *Queries) ListBuildingDeployments(ctx context.Context, limit int) ([]domain.CodeDeployment, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListBuildingDeployments")
	defer span.End()

	query := `
		SELECT id, job_id, project_id, version, status, runtime,
		       source_hash, source_size_bytes, source_uri,
		       built_image_uri, built_image_digest, build_logs, error_message,
		       created_by, created_at, updated_at, finished_at
		FROM code_deployments
		WHERE status = 'building'
		ORDER BY created_at ASC
		LIMIT $1`

	rows, err := q.db.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("list building deployments: %w", err)
	}
	defer rows.Close()

	var result []domain.CodeDeployment
	for rows.Next() {
		var d domain.CodeDeployment
		var status, runtime string
		var builtImageURI, builtImageDigest, buildLogs, errorMessage, createdBy *string

		if err := rows.Scan(
			&d.ID,
			&d.JobID,
			&d.ProjectID,
			&d.Version,
			&status,
			&runtime,
			&d.SourceHash,
			&d.SourceSizeBytes,
			&d.SourceURI,
			&builtImageURI,
			&builtImageDigest,
			&buildLogs,
			&errorMessage,
			&createdBy,
			&d.CreatedAt,
			&d.UpdatedAt,
			&d.FinishedAt,
		); err != nil {
			return nil, fmt.Errorf("list building deployments: scan: %w", err)
		}

		d.Status = domain.DeploymentBuildStatus(status)
		d.Runtime = domain.Runtime(runtime)
		if builtImageURI != nil {
			d.BuiltImageURI = *builtImageURI
		}
		if builtImageDigest != nil {
			d.BuiltImageDigest = *builtImageDigest
		}
		if buildLogs != nil {
			d.BuildLogs = *buildLogs
		}
		if errorMessage != nil {
			d.ErrorMessage = *errorMessage
		}
		if createdBy != nil {
			d.CreatedBy = *createdBy
		}
		result = append(result, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list building deployments: rows: %w", err)
	}
	return result, nil
}

// nilStringFromMap extracts a *string from a map[string]any. Returns nil if the key
// is absent, maps to a non-string value, or maps to an empty string.
func nilStringFromMap(m map[string]any, key string) *string {
	if m == nil {
		return nil
	}
	v, ok := m[key]
	if !ok {
		return nil
	}
	s, ok := v.(string)
	if !ok || s == "" {
		return nil
	}
	return &s
}
