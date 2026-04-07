package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"strait/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

	var createdBy *string
	if d.CreatedBy != "" {
		createdBy = &d.CreatedBy
	}

	// Derive version inline in the INSERT to avoid a TOCTOU gap between a
	// separate SELECT MAX and the INSERT. The UNIQUE(job_id, version) constraint
	// catches the rare case where two concurrent inserts compute the same MAX.
	// Retry once on unique violation — collisions require simultaneous requests.
	insertQuery := `
		INSERT INTO code_deployments (
			id, job_id, project_id, version, status, runtime,
			source_hash, source_size_bytes, source_uri, created_by
		)
		SELECT $1, $2, $3,
		       COALESCE((SELECT MAX(version) FROM code_deployments WHERE job_id = $2), 0) + 1,
		       $4, $5, $6, $7, $8, $9
		RETURNING version, created_at, updated_at`

	for attempt := range 2 {
		err := q.db.QueryRow(ctx, insertQuery,
			d.ID,
			d.JobID,
			d.ProjectID,
			string(d.Status),
			string(d.Runtime),
			d.SourceHash,
			d.SourceSizeBytes,
			d.SourceURI,
			createdBy,
		).Scan(&d.Version, &d.CreatedAt, &d.UpdatedAt)
		if err == nil {
			return nil
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && attempt == 0 {
			// Unique violation on (job_id, version) — retry once.
			continue
		}
		return fmt.Errorf("create code deployment: %w", err)
	}
	return fmt.Errorf("create code deployment: version conflict after retry")
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

// ConfirmCodeDeployment atomically transitions a deployment from "pending" to
// "building" using a single UPDATE WHERE id=$1 AND status='pending'. Returns
// ErrCodeDeploymentNotFound if the deployment does not exist or is already in a
// non-pending state, making concurrent confirm calls idempotent-safe.
func (q *Queries) ConfirmCodeDeployment(ctx context.Context, id string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ConfirmCodeDeployment")
	defer span.End()

	tag, err := q.db.Exec(ctx,
		`UPDATE code_deployments SET status = 'building', updated_at = NOW() WHERE id = $1 AND status = 'pending'`,
		id,
	)
	if err != nil {
		return fmt.Errorf("confirm code deployment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrCodeDeploymentNotFound
	}
	return nil
}

// UpdateCodeDeploymentStatus transitions a deployment to a new status and
// writes optional build output fields (image URI, digest, logs, error).
// finished_at is set automatically when transitioning to a terminal status.
func (q *Queries) UpdateCodeDeploymentStatus(ctx context.Context, id string, status domain.DeploymentBuildStatus, fields map[string]any) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpdateCodeDeploymentStatus")
	defer span.End()

	terminal := status == domain.DeploymentStatusReady ||
		status == domain.DeploymentStatusFailed ||
		status == domain.DeploymentStatusTimedOut
	var finishedAt *time.Time
	if terminal {
		now := time.Now().UTC()
		finishedAt = &now
	}

	// On terminal status, release the build_node claim so the row is clearly
	// no longer in-flight. This also ensures stale-claim recovery skips rows
	// that have already been finalized.
	query := `
		UPDATE code_deployments
		SET status                = $2,
		    updated_at            = NOW(),
		    finished_at           = COALESCE($3, finished_at),
		    built_image_uri       = COALESCE($4, built_image_uri),
		    built_image_digest    = COALESCE($5, built_image_digest),
		    build_logs            = COALESCE($6, build_logs),
		    error_message         = COALESCE($7, error_message),
		    build_node_id         = NULL,
		    build_node_claimed_at = NULL
		WHERE id = $1`

	tag, err := q.db.Exec(ctx, query,
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
	if tag.RowsAffected() == 0 {
		return ErrCodeDeploymentNotFound
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
// The check and update are merged into a single query to eliminate a TOCTOU window
// where the deployment status could change between the SELECT and the UPDATE.
func (q *Queries) RollbackToDeployment(ctx context.Context, jobID, deploymentID, projectID string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.RollbackToDeployment")
	defer span.End()

	// Single atomic UPDATE: set active deployment only if the target is ready and
	// belongs to the correct job+project. Eliminates the TOCTOU race from the
	// previous SELECT-then-UPDATE pattern.
	query := `
		UPDATE jobs
		SET active_deployment_id = $2,
		    source_type          = 'code',
		    updated_at           = NOW()
		WHERE id = $1
		  AND project_id = $3
		  AND EXISTS (
		      SELECT 1 FROM code_deployments
		      WHERE id = $2 AND job_id = $1 AND project_id = $3 AND status = 'ready'
		  )`
	tag, err := q.db.Exec(ctx, query, jobID, deploymentID, projectID)
	if err != nil {
		return fmt.Errorf("rollback to deployment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrCodeDeploymentNotFound
	}
	return nil
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

// ClaimBuildingDeployment atomically selects one unclaimed "building" deployment
// and marks it as owned by workerID. Uses SELECT … FOR UPDATE SKIP LOCKED so
// concurrent orchestrator replicas each claim a different deployment with no
// coordination beyond the database. Returns nil, nil when no work is available.
func (q *Queries) ClaimBuildingDeployment(ctx context.Context, workerID string) (*domain.CodeDeployment, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ClaimBuildingDeployment")
	defer span.End()

	query := `
		WITH candidate AS (
			SELECT id FROM code_deployments
			WHERE status = 'building' AND build_node_id IS NULL
			ORDER BY created_at ASC
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE code_deployments
		SET build_node_id         = $1,
		    build_node_claimed_at = NOW()
		FROM candidate
		WHERE code_deployments.id = candidate.id
		RETURNING id, job_id, project_id, version, status, runtime,
		          source_hash, source_size_bytes, source_uri,
		          built_image_uri, built_image_digest, build_logs, error_message,
		          created_by, created_at, updated_at, finished_at`

	var d domain.CodeDeployment
	var status, runtime string
	var builtImageURI, builtImageDigest, buildLogs, errorMessage, createdBy *string

	err := q.db.QueryRow(ctx, query, workerID).Scan(
		&d.ID, &d.JobID, &d.ProjectID, &d.Version, &status, &runtime,
		&d.SourceHash, &d.SourceSizeBytes, &d.SourceURI,
		&builtImageURI, &builtImageDigest, &buildLogs, &errorMessage,
		&createdBy, &d.CreatedAt, &d.UpdatedAt, &d.FinishedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil // nothing to claim
	}
	if err != nil {
		return nil, fmt.Errorf("claim building deployment: %w", err)
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

// ReleaseStaleClaimedDeployments resets the build_node claim on any "building"
// deployments whose claim is older than olderThan. This recovers from crashed
// orchestrator workers that claimed a deployment but never finished it.
// Returns the number of deployments released.
func (q *Queries) ReleaseStaleClaimedDeployments(ctx context.Context, olderThan time.Duration) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ReleaseStaleClaimedDeployments")
	defer span.End()

	tag, err := q.db.Exec(ctx, `
		UPDATE code_deployments
		SET build_node_id         = NULL,
		    build_node_claimed_at = NULL,
		    updated_at            = NOW()
		WHERE status = 'building'
		  AND build_node_claimed_at IS NOT NULL
		  AND build_node_claimed_at < NOW() - $1::interval`,
		olderThan.String(),
	)
	if err != nil {
		return 0, fmt.Errorf("release stale claimed deployments: %w", err)
	}
	return tag.RowsAffected(), nil
}

// DeleteExpiredDeployments removes stale deployments that are no longer actionable:
//   - pending deployments created before pendingBefore (presign TTL expired, never uploaded)
//   - failed or timed_out deployments finished before failedBefore
//
// The active deployment for any job is never deleted, even if it would otherwise qualify.
// Returns the number of rows deleted.
func (q *Queries) DeleteExpiredDeployments(ctx context.Context, pendingBefore, failedBefore time.Time) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteExpiredDeployments")
	defer span.End()

	tag, err := q.db.Exec(ctx, `
		DELETE FROM code_deployments
		WHERE (
		    (status = 'pending' AND created_at < $1)
		 OR (status IN ('failed', 'timed_out') AND finished_at < $2)
		)
		AND id NOT IN (
		    SELECT active_deployment_id
		    FROM jobs
		    WHERE active_deployment_id IS NOT NULL
		)`,
		pendingBefore,
		failedBefore,
	)
	if err != nil {
		return 0, fmt.Errorf("delete expired deployments: %w", err)
	}
	return tag.RowsAffected(), nil
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
