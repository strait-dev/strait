package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"time"

	"strait/internal/dbscan"
	"strait/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

func (q *Queries) CreateEnvironment(ctx context.Context, env *domain.Environment) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateEnvironment")
	defer span.End()

	if env.ID == "" {
		env.ID = uuid.Must(uuid.NewV7()).String()
	}

	variablesJSON, variablesEncrypted, err := q.prepareEnvironmentVariables(env.ID, env.Variables)
	if err != nil {
		return err
	}

	query := `
		INSERT INTO environments (id, project_id, name, slug, parent_id, variables, variables_encrypted, is_standard)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at, updated_at`

	err = q.db.QueryRow(
		ctx,
		query,
		env.ID,
		env.ProjectID,
		env.Name,
		env.Slug,
		dbscan.NilIfEmptyString(env.ParentID),
		variablesJSON,
		variablesEncrypted,
		env.IsStandard,
	).Scan(&env.CreatedAt, &env.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create environment: %w", err)
	}

	return nil
}

// CreateEnvironmentWithOrgLimit serializes org-wide environment quota
// enforcement with row creation.
func (q *Queries) CreateEnvironmentWithOrgLimit(ctx context.Context, env *domain.Environment, orgID string, maxEnvironments int) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateEnvironmentWithOrgLimit")
	defer span.End()

	if maxEnvironments < 0 || orgID == "" {
		return q.CreateEnvironment(ctx, env)
	}

	if _, ok := TxFromContext(ctx); ok {
		return q.createEnvironmentWithOrgLimitLocked(ctx, env, orgID, maxEnvironments)
	}
	if _, ok := q.db.(pgx.Tx); ok {
		return q.createEnvironmentWithOrgLimitLocked(ctx, env, orgID, maxEnvironments)
	}
	if _, ok := q.db.(TxBeginner); !ok {
		return q.createEnvironmentWithOrgLimitLocked(ctx, env, orgID, maxEnvironments)
	}

	return q.withTx(ctx, func(txq *Queries) error {
		return txq.createEnvironmentWithOrgLimitLocked(ctx, env, orgID, maxEnvironments)
	})
}

func (q *Queries) createEnvironmentWithOrgLimitLocked(ctx context.Context, env *domain.Environment, orgID string, maxEnvironments int) error {
	if err := q.acquirePlanLimitLock(ctx, "environment_limit:"+orgID); err != nil {
		return err
	}

	count, err := q.CountEnvironmentsByOrg(ctx, orgID)
	if err != nil {
		return fmt.Errorf("count environments before create: %w", err)
	}
	if count >= maxEnvironments {
		return ErrEnvironmentLimitExceeded
	}

	return q.CreateEnvironment(ctx, env)
}

func (q *Queries) GetEnvironment(ctx context.Context, id, projectID string) (*domain.Environment, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetEnvironment")
	defer span.End()

	query := `
		SELECT id, project_id, name, slug, parent_id, variables, variables_encrypted, is_standard, created_at, updated_at
		FROM environments
		WHERE id = $1 AND project_id = $2`

	env, err := q.scanEnvironment(q.db.QueryRow(ctx, query, id, projectID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrEnvironmentNotFound
		}
		return nil, fmt.Errorf("get environment: %w", err)
	}

	return env, nil
}

func (q *Queries) ListEnvironments(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.Environment, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListEnvironments")
	defer span.End()

	query := `
		SELECT id, project_id, name, slug, parent_id, variables, variables_encrypted, is_standard, created_at, updated_at
		FROM environments
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
		return nil, fmt.Errorf("list environments: %w", err)
	}
	defer rows.Close()

	envs := make([]domain.Environment, 0, limit)
	for rows.Next() {
		env, scanErr := q.scanEnvironment(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list environments scan: %w", scanErr)
		}
		envs = append(envs, *env)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list environments rows: %w", err)
	}

	return envs, nil
}

func (q *Queries) UpdateEnvironment(ctx context.Context, env *domain.Environment) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpdateEnvironment")
	defer span.End()

	variablesJSON, variablesEncrypted, err := q.prepareEnvironmentVariables(env.ID, env.Variables)
	if err != nil {
		return err
	}

	query := `
		UPDATE environments
		SET name = $1,
		    slug = $2,
		    parent_id = $3,
		    variables = $4,
		    variables_encrypted = $5,
		    updated_at = NOW()
		WHERE id = $6 AND project_id = $7
		RETURNING updated_at`

	err = q.db.QueryRow(
		ctx,
		query,
		env.Name,
		env.Slug,
		dbscan.NilIfEmptyString(env.ParentID),
		variablesJSON,
		variablesEncrypted,
		env.ID,
		env.ProjectID,
	).Scan(&env.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrEnvironmentNotFound
		}
		return fmt.Errorf("update environment: %w", err)
	}

	return nil
}

// ErrStandardEnvironment is returned when attempting to delete or rename a standard environment.
var ErrStandardEnvironment = errors.New("cannot modify standard environment")

// ErrEnvironmentVariableEncryptionRequired is returned when callers try to
// persist environment variables without configuring at-rest secret encryption.
var ErrEnvironmentVariableEncryptionRequired = errors.New("environment variable encryption key is required")

func (q *Queries) DeleteEnvironment(ctx context.Context, id, projectID string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteEnvironment")
	defer span.End()

	// Prevent deletion of standard environments.
	query := `DELETE FROM environments WHERE id = $1 AND project_id = $2 AND is_standard = FALSE`
	tag, err := q.db.Exec(ctx, query, id, projectID)
	if err != nil {
		return fmt.Errorf("delete environment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// Check if the environment exists but is standard.
		var isStandard bool
		checkErr := q.db.QueryRow(ctx, `SELECT is_standard FROM environments WHERE id = $1 AND project_id = $2`, id, projectID).Scan(&isStandard)
		if checkErr != nil {
			return ErrEnvironmentNotFound
		}
		if isStandard {
			return ErrStandardEnvironment
		}
		return ErrEnvironmentNotFound
	}

	return nil
}

// CreateStandardEnvironments creates the legacy standard environments
// (development, staging, production) for callers that explicitly request them.
// Launch project creation does not call this helper because active environments
// are plan-capped and created on demand.
func (q *Queries) CreateStandardEnvironments(ctx context.Context, projectID string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateStandardEnvironments")
	defer span.End()

	for _, slug := range domain.StandardEnvironmentSlugs {
		name := domain.StandardEnvironmentNames[slug]
		env := &domain.Environment{
			ProjectID:  projectID,
			Name:       name,
			Slug:       slug,
			IsStandard: true,
		}
		if err := q.CreateEnvironment(ctx, env); err != nil {
			return fmt.Errorf("create standard environment %s: %w", slug, err)
		}
	}

	return nil
}

func (q *Queries) GetResolvedEnvironmentVariables(ctx context.Context, projectID, id string) (map[string]string, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetResolvedEnvironmentVariables")
	defer span.End()

	const maxDepth = 10

	// Use a recursive CTE to fetch the entire parent chain in a single query.
	// The chain is returned root-first so we can overlay child variables on top.
	// The seed row is scoped by project_id (in addition to RLS) so a snapshot
	// lookup by id from one tenant cannot resolve and decrypt another tenant's
	// secret values; the recursive step already stays within the same project.
	query := `
		WITH RECURSIVE chain AS (
			SELECT id, project_id, parent_id, variables, variables_encrypted, 1 AS depth
			FROM environments
			WHERE id = $1 AND project_id = $3
			UNION ALL
			SELECT e.id, e.project_id, e.parent_id, e.variables, e.variables_encrypted, c.depth + 1
			FROM environments e
			JOIN chain c ON e.id = c.parent_id
			WHERE c.depth < $2
			  AND e.project_id = c.project_id
		)
			SELECT id, project_id, parent_id, variables, variables_encrypted, depth FROM chain
			ORDER BY depth DESC`

	rows, err := q.db.Query(ctx, query, id, maxDepth, projectID)
	if err != nil {
		return nil, fmt.Errorf("resolve environment variables: %w", err)
	}
	defer rows.Close()

	resolved := make(map[string]string)
	// Rows arrive ORDER BY depth DESC, so the first row is the deepest ancestor
	// the CTE could reach. Only that row's parent_id tells us whether the chain
	// was truncated at maxDepth — the leaf's parent_id (the last row iterated)
	// is irrelevant to truncation and used to wrongly signal "too deep" on
	// every non-orphan environment with a parent.
	var deepestAncestorParentID *string
	var rowCount int
	for rows.Next() {
		var envID string
		var projectID string
		var parentID *string
		var variablesRaw []byte
		var variablesEncrypted []byte
		var depth int
		if err := rows.Scan(&envID, &projectID, &parentID, &variablesRaw, &variablesEncrypted, &depth); err != nil {
			return nil, fmt.Errorf("resolve environment variables scan: %w", err)
		}
		variablesRaw, err = q.decryptEnvironmentVariables(envID, variablesRaw, variablesEncrypted)
		if err != nil {
			return nil, fmt.Errorf("resolve environment variables decrypt: %w", err)
		}
		if len(variablesRaw) > 0 {
			var vars map[string]string
			if err := json.Unmarshal(variablesRaw, &vars); err != nil {
				return nil, fmt.Errorf("resolve environment variables unmarshal: %w", err)
			}
			maps.Copy(resolved, vars)
		}
		if rowCount == 0 {
			deepestAncestorParentID = parentID
		}
		rowCount++
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("resolve environment variables rows: %w", err)
	}

	if rowCount == 0 {
		return nil, fmt.Errorf("resolve environment variables: %w", ErrEnvironmentNotFound)
	}

	if rowCount >= maxDepth && deepestAncestorParentID != nil && *deepestAncestorParentID != "" {
		return nil, fmt.Errorf("resolve environment variables: exceeded max inheritance depth %d", maxDepth)
	}

	return resolved, nil
}

func (q *Queries) scanEnvironment(scanner scanTarget) (*domain.Environment, error) {
	var env domain.Environment
	var parentID *string
	var variablesRaw []byte
	var variablesEncrypted []byte

	err := scanner.Scan(
		&env.ID,
		&env.ProjectID,
		&env.Name,
		&env.Slug,
		&parentID,
		&variablesRaw,
		&variablesEncrypted,
		&env.IsStandard,
		&env.CreatedAt,
		&env.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if parentID != nil {
		env.ParentID = *parentID
	}

	variablesRaw, err = q.decryptEnvironmentVariables(env.ID, variablesRaw, variablesEncrypted)
	if err != nil {
		return nil, err
	}

	variables, err := unmarshalEnvironmentVariables(variablesRaw)
	if err != nil {
		return nil, err
	}
	env.Variables = variables

	return &env, nil
}

func (q *Queries) prepareEnvironmentVariables(envID string, variables map[string]string) ([]byte, []byte, error) {
	variablesJSON, err := marshalEnvironmentVariables(variables)
	if err != nil {
		return nil, nil, err
	}
	if len(variables) == 0 {
		return variablesJSON, nil, nil
	}
	if q.secretEncryptionKey == "" {
		return nil, nil, fmt.Errorf("prepare environment variables for %s: %w", envID, ErrEnvironmentVariableEncryptionRequired)
	}
	enc, err := q.secretEncryptor()
	if err != nil {
		return nil, nil, fmt.Errorf("create environment variable encryptor for %s: %w", envID, err)
	}
	variablesEncrypted, err := enc.Encrypt(variablesJSON)
	if err != nil {
		return nil, nil, fmt.Errorf("encrypt environment variables for %s: %w", envID, err)
	}
	return []byte(`{}`), variablesEncrypted, nil
}

func (q *Queries) decryptEnvironmentVariables(envID string, variablesRaw, variablesEncrypted []byte) ([]byte, error) {
	if len(variablesEncrypted) == 0 {
		variables, err := unmarshalEnvironmentVariables(variablesRaw)
		if err != nil {
			return nil, fmt.Errorf("inspect legacy environment variables for %s: %w", envID, err)
		}
		if len(variables) > 0 {
			return nil, fmt.Errorf("decrypt environment variables for %s: legacy plaintext variables are not readable: %w", envID, ErrEnvironmentVariableEncryptionRequired)
		}
		return variablesRaw, nil
	}
	if q.secretEncryptionKey == "" {
		return nil, fmt.Errorf("decrypt environment variables for %s: %w", envID, ErrEnvironmentVariableEncryptionRequired)
	}
	enc, err := q.secretEncryptor()
	if err != nil {
		return nil, fmt.Errorf("create environment variable decryptor for %s: %w", envID, err)
	}
	decrypted, err := enc.Decrypt(variablesEncrypted)
	if err != nil {
		return nil, fmt.Errorf("decrypt environment variables for %s: %w", envID, err)
	}
	return decrypted, nil
}

func marshalEnvironmentVariables(variables map[string]string) ([]byte, error) {
	if len(variables) == 0 {
		return []byte(`{}`), nil
	}

	encoded, err := json.Marshal(variables)
	if err != nil {
		return nil, fmt.Errorf("marshal environment variables: %w", err)
	}

	return encoded, nil
}

func unmarshalEnvironmentVariables(raw []byte) (map[string]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	var variables map[string]string
	if err := json.Unmarshal(raw, &variables); err != nil {
		return nil, fmt.Errorf("unmarshal environment variables: %w", err)
	}

	if len(variables) == 0 {
		return nil, nil
	}

	return variables, nil
}
