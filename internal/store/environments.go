package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"

	"orchestrator/internal/dbscan"
	"orchestrator/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

func (q *Queries) CreateEnvironment(ctx context.Context, env *domain.Environment) error {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.CreateEnvironment")
	defer span.End()

	if env.ID == "" {
		env.ID = uuid.Must(uuid.NewV7()).String()
	}

	variablesJSON, err := marshalEnvironmentVariables(env.Variables)
	if err != nil {
		return err
	}

	query := `
		INSERT INTO environments (id, project_id, name, slug, parent_id, variables)
		VALUES ($1, $2, $3, $4, $5, $6)
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
	).Scan(&env.CreatedAt, &env.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create environment: %w", err)
	}

	return nil
}

func (q *Queries) GetEnvironment(ctx context.Context, id string) (*domain.Environment, error) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.GetEnvironment")
	defer span.End()

	query := `
		SELECT id, project_id, name, slug, parent_id, variables, created_at, updated_at
		FROM environments
		WHERE id = $1`

	env, err := scanEnvironment(q.db.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrEnvironmentNotFound
		}
		return nil, fmt.Errorf("get environment: %w", err)
	}

	return env, nil
}

func (q *Queries) ListEnvironments(ctx context.Context, projectID string) ([]domain.Environment, error) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.ListEnvironments")
	defer span.End()

	query := `
		SELECT id, project_id, name, slug, parent_id, variables, created_at, updated_at
		FROM environments
		WHERE project_id = $1
		ORDER BY created_at DESC`

	rows, err := q.db.Query(ctx, query, projectID)
	if err != nil {
		return nil, fmt.Errorf("list environments: %w", err)
	}
	defer rows.Close()

	envs := make([]domain.Environment, 0)
	for rows.Next() {
		env, scanErr := scanEnvironment(rows)
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
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.UpdateEnvironment")
	defer span.End()

	variablesJSON, err := marshalEnvironmentVariables(env.Variables)
	if err != nil {
		return err
	}

	query := `
		UPDATE environments
		SET name = $1,
		    slug = $2,
		    parent_id = $3,
		    variables = $4,
		    updated_at = NOW()
		WHERE id = $5
		RETURNING updated_at`

	err = q.db.QueryRow(
		ctx,
		query,
		env.Name,
		env.Slug,
		dbscan.NilIfEmptyString(env.ParentID),
		variablesJSON,
		env.ID,
	).Scan(&env.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrEnvironmentNotFound
		}
		return fmt.Errorf("update environment: %w", err)
	}

	return nil
}

func (q *Queries) DeleteEnvironment(ctx context.Context, id string) error {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.DeleteEnvironment")
	defer span.End()

	query := `DELETE FROM environments WHERE id = $1`
	tag, err := q.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete environment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrEnvironmentNotFound
	}

	return nil
}

func (q *Queries) GetResolvedEnvironmentVariables(ctx context.Context, id string) (map[string]string, error) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.GetResolvedEnvironmentVariables")
	defer span.End()

	const maxDepth = 10

	chain := make([]*domain.Environment, 0, maxDepth)
	currentID := id
	for range maxDepth {
		env, err := q.GetEnvironment(ctx, currentID)
		if err != nil {
			return nil, err
		}
		chain = append(chain, env)

		if env.ParentID == "" {
			break
		}

		currentID = env.ParentID
	}

	if len(chain) == maxDepth && chain[len(chain)-1].ParentID != "" {
		return nil, fmt.Errorf("resolve environment variables: exceeded max inheritance depth %d", maxDepth)
	}

	resolved := make(map[string]string)
	for i := len(chain) - 1; i >= 0; i-- {
		maps.Copy(resolved, chain[i].Variables)
	}

	return resolved, nil
}

func scanEnvironment(scanner scanTarget) (*domain.Environment, error) {
	var env domain.Environment
	var parentID *string
	var variablesRaw []byte

	err := scanner.Scan(
		&env.ID,
		&env.ProjectID,
		&env.Name,
		&env.Slug,
		&parentID,
		&variablesRaw,
		&env.CreatedAt,
		&env.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if parentID != nil {
		env.ParentID = *parentID
	}

	variables, err := unmarshalEnvironmentVariables(variablesRaw)
	if err != nil {
		return nil, err
	}
	env.Variables = variables

	return &env, nil
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
