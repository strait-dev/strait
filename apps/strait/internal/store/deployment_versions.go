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

// deploymentVersionColumns is the shared column list used by all deployment version queries.
const deploymentVersionColumns = `id, project_id, environment, runtime, artifact_uri, manifest, checksum, status,
			strategy, canary_percent, EXTRACT(EPOCH FROM canary_duration) * 1000000,
			finalized_at, promoted_at, rollback_from_deployment_id, created_by, updated_by, created_at, updated_at`

func scanDeploymentVersion(scanner dbscan.Scanner) (*domain.DeploymentVersion, error) {
	var deployment domain.DeploymentVersion
	var manifest []byte
	var checksum *string
	var strategy string
	var canaryDurationUs *float64
	var rollbackFromDeploymentID *string
	var createdBy *string
	var updatedBy *string

	err := scanner.Scan(
		&deployment.ID,
		&deployment.ProjectID,
		&deployment.Environment,
		&deployment.Runtime,
		&deployment.ArtifactURI,
		&manifest,
		&checksum,
		&deployment.Status,
		&strategy,
		&deployment.CanaryPercent,
		&canaryDurationUs,
		&deployment.FinalizedAt,
		&deployment.PromotedAt,
		&rollbackFromDeploymentID,
		&createdBy,
		&updatedBy,
		&deployment.CreatedAt,
		&deployment.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	deployment.Strategy = domain.DeploymentStrategy(strategy)
	if canaryDurationUs != nil {
		d := time.Duration(*canaryDurationUs * float64(time.Microsecond))
		deployment.CanaryDuration = &d
	}
	if len(manifest) > 0 {
		deployment.Manifest = json.RawMessage(manifest)
	}
	if checksum != nil {
		deployment.Checksum = *checksum
	}
	if rollbackFromDeploymentID != nil {
		deployment.RollbackFromDeployment = *rollbackFromDeploymentID
	}
	if createdBy != nil {
		deployment.CreatedBy = *createdBy
	}
	if updatedBy != nil {
		deployment.UpdatedBy = *updatedBy
	}

	return &deployment, nil
}

func (q *Queries) CreateDeploymentVersion(ctx context.Context, deployment *domain.DeploymentVersion) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateDeploymentVersion")
	defer span.End()

	if deployment.ID == "" {
		deployment.ID = uuid.Must(uuid.NewV7()).String()
	}
	if deployment.Status == "" {
		deployment.Status = domain.DeploymentVersionStatusDraft
	}
	if !deployment.Status.IsValid() {
		return fmt.Errorf("create deployment version: invalid status %q", deployment.Status)
	}
	if deployment.Strategy == "" {
		deployment.Strategy = domain.DeploymentStrategyDirect
	}

	query := `
		INSERT INTO deployment_versions (
			id,
			project_id,
			environment,
			runtime,
			artifact_uri,
			manifest,
			checksum,
			status,
			strategy,
			canary_percent,
			canary_duration,
			created_by,
			updated_by
		)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8, $9, $10, make_interval(secs => $11), $12, $13)
		RETURNING created_at, updated_at`

	manifest := deployment.Manifest
	if len(manifest) == 0 {
		manifest = json.RawMessage(`{}`)
	}

	var canaryDurationSecs *float64
	if deployment.CanaryDuration != nil {
		secs := deployment.CanaryDuration.Seconds()
		canaryDurationSecs = &secs
	}

	err := q.db.QueryRow(
		ctx,
		query,
		deployment.ID,
		deployment.ProjectID,
		deployment.Environment,
		deployment.Runtime,
		deployment.ArtifactURI,
		manifest,
		deployment.Checksum,
		string(deployment.Status),
		string(deployment.Strategy),
		deployment.CanaryPercent,
		canaryDurationSecs,
		deployment.CreatedBy,
		deployment.UpdatedBy,
	).Scan(&deployment.CreatedAt, &deployment.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create deployment version: %w", err)
	}

	return nil
}

func (q *Queries) GetDeploymentVersion(ctx context.Context, deploymentID, projectID string) (*domain.DeploymentVersion, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetDeploymentVersion")
	defer span.End()

	query := `
		SELECT ` + deploymentVersionColumns + `
		FROM deployment_versions
		WHERE id = $1 AND project_id = $2`

	deployment, err := scanDeploymentVersion(q.db.QueryRow(ctx, query, deploymentID, projectID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrDeploymentVersionNotFound
		}
		return nil, fmt.Errorf("get deployment version: %w", err)
	}

	return deployment, nil
}

func (q *Queries) ListDeploymentVersions(ctx context.Context, projectID, environment string, limit int, cursor *time.Time) ([]domain.DeploymentVersion, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListDeploymentVersions")
	defer span.End()

	if limit <= 0 {
		limit = 50
	}

	query := `
		SELECT ` + deploymentVersionColumns + `
		FROM deployment_versions
		WHERE project_id = $1
			AND ($2 = '' OR environment = $2)
			AND ($3::timestamptz IS NULL OR created_at < $3)
		ORDER BY created_at DESC
		LIMIT $4`

	rows, err := q.db.Query(ctx, query, projectID, environment, cursor, limit)
	if err != nil {
		return nil, fmt.Errorf("list deployment versions: %w", err)
	}
	defer rows.Close()

	versions := make([]domain.DeploymentVersion, 0)
	for rows.Next() {
		version, scanErr := scanDeploymentVersion(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list deployment versions: %w", scanErr)
		}
		versions = append(versions, *version)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list deployment versions: %w", err)
	}

	return versions, nil
}

func (q *Queries) FinalizeDeploymentVersion(ctx context.Context, deploymentID, projectID, updatedBy string) (*domain.DeploymentVersion, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.FinalizeDeploymentVersion")
	defer span.End()

	query := `
		UPDATE deployment_versions
		SET status = 'finalized',
			finalized_at = COALESCE(finalized_at, NOW()),
			updated_by = COALESCE(NULLIF($3, ''), updated_by),
			updated_at = NOW()
		WHERE id = $1 AND project_id = $2
		RETURNING ` + deploymentVersionColumns

	deployment, err := scanDeploymentVersion(
		q.db.QueryRow(ctx, query, deploymentID, projectID, updatedBy),
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrDeploymentVersionNotFound
		}
		return nil, fmt.Errorf("finalize deployment version: %w", err)
	}

	return deployment, nil
}

func (q *Queries) PromoteDeploymentVersion(ctx context.Context, deploymentID, projectID, environment, updatedBy string) (*domain.DeploymentVersion, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.PromoteDeploymentVersion")
	defer span.End()

	return q.promoteDeploymentVersion(ctx, deploymentID, projectID, environment, updatedBy, false)
}

func (q *Queries) RollbackDeploymentVersion(ctx context.Context, deploymentID, projectID, environment, updatedBy string) (*domain.DeploymentVersion, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.RollbackDeploymentVersion")
	defer span.End()

	return q.promoteDeploymentVersion(ctx, deploymentID, projectID, environment, updatedBy, true)
}

func (q *Queries) promoteDeploymentVersion(ctx context.Context, deploymentID, projectID, environment, updatedBy string, rollback bool) (*domain.DeploymentVersion, error) {
	txb, ok := q.db.(TxBeginner)
	if !ok {
		return nil, fmt.Errorf("promote deployment version: transactional database required")
	}

	var promoted *domain.DeploymentVersion
	err := WithTx(ctx, txb, func(txQ *Queries) error {
		var previousPromotedID *string
		row := txQ.db.QueryRow(
			ctx,
			`SELECT id FROM deployment_versions WHERE project_id = $1 AND environment = $2 AND promoted_at IS NOT NULL ORDER BY promoted_at DESC LIMIT 1 FOR UPDATE`,
			projectID,
			environment,
		)
		if scanErr := row.Scan(&previousPromotedID); scanErr != nil && !errors.Is(scanErr, pgx.ErrNoRows) {
			return fmt.Errorf("load currently promoted deployment version: %w", scanErr)
		}

		if _, execErr := txQ.db.Exec(
			ctx,
			`UPDATE deployment_versions
			 SET promoted_at = NULL,
				 status = CASE WHEN status = 'promoted' THEN 'finalized' ELSE status END,
				 updated_by = COALESCE(NULLIF($3, ''), updated_by),
				 updated_at = NOW()
			 WHERE project_id = $1 AND environment = $2 AND promoted_at IS NOT NULL`,
			projectID,
			environment,
			updatedBy,
		); execErr != nil {
			return fmt.Errorf("clear promoted deployment versions: %w", execErr)
		}

		query := `
			UPDATE deployment_versions
			SET status = 'promoted',
				promoted_at = NOW(),
				updated_by = COALESCE(NULLIF($4, ''), updated_by),
				updated_at = NOW(),
				rollback_from_deployment_id = CASE
					WHEN $5::boolean AND $6::text IS NOT NULL AND $6::text <> id THEN $6::text
					ELSE rollback_from_deployment_id
				END
			WHERE id = $1 AND project_id = $2 AND environment = $3
			RETURNING ` + deploymentVersionColumns

		rollbackFromID := ""
		if previousPromotedID != nil {
			rollbackFromID = *previousPromotedID
		}

		var promoteErr error
		promoted, promoteErr = scanDeploymentVersion(
			txQ.db.QueryRow(
				ctx,
				query,
				deploymentID,
				projectID,
				environment,
				updatedBy,
				rollback,
				rollbackFromID,
			),
		)
		if promoteErr != nil {
			if errors.Is(promoteErr, pgx.ErrNoRows) {
				return ErrDeploymentVersionNotFound
			}
			return fmt.Errorf("promote deployment version: %w", promoteErr)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return promoted, nil
}
