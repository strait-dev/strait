package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"strait/internal/dbscan"
	"strait/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel"
)

const agentColumns = `id, project_id, job_id, name, slug, description, model, model_fallbacks, config, provider_secrets_encrypted, created_by, updated_by, created_at, updated_at`

const agentDeploymentColumns = `id, agent_id, version, status, provider, config_snapshot, provider_metadata, created_by, created_at, updated_at, deployed_at`

func scanAgent(scanner interface {
	Scan(dest ...any) error
}) (*domain.Agent, error) {
	var agent domain.Agent
	var createdBy *string
	var updatedBy *string
	var providerSecretsEncrypted *string
	if err := scanner.Scan(
		&agent.ID,
		&agent.ProjectID,
		&agent.JobID,
		&agent.Name,
		&agent.Slug,
		&agent.Description,
		&agent.Model,
		&agent.ModelFallbacks,
		&agent.Config,
		&providerSecretsEncrypted,
		&createdBy,
		&updatedBy,
		&agent.CreatedAt,
		&agent.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if providerSecretsEncrypted != nil {
		agent.ProviderSecretsEncrypted = *providerSecretsEncrypted
	}
	if createdBy != nil {
		agent.CreatedBy = *createdBy
	}
	if updatedBy != nil {
		agent.UpdatedBy = *updatedBy
	}
	return &agent, nil
}

func scanAgentDeployment(scanner interface {
	Scan(dest ...any) error
}) (*domain.AgentDeployment, error) {
	var deployment domain.AgentDeployment
	var status string
	var createdBy *string
	if err := scanner.Scan(
		&deployment.ID,
		&deployment.AgentID,
		&deployment.Version,
		&status,
		&deployment.Provider,
		&deployment.ConfigSnapshot,
		&deployment.ProviderMetadata,
		&createdBy,
		&deployment.CreatedAt,
		&deployment.UpdatedAt,
		&deployment.DeployedAt,
	); err != nil {
		return nil, err
	}
	deployment.Status = domain.AgentDeploymentStatus(status)
	if createdBy != nil {
		deployment.CreatedBy = *createdBy
	}
	return &deployment, nil
}

func (q *Queries) CreateAgent(ctx context.Context, agent *domain.Agent) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateAgent")
	defer span.End()

	if agent.ID == "" {
		agent.ID = uuid.Must(uuid.NewV7()).String()
	}

	err := q.db.QueryRow(ctx, `
		INSERT INTO agents (id, project_id, job_id, name, slug, description, model, model_fallbacks, config, provider_secrets_encrypted, created_by, updated_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING created_at, updated_at
	`,
		agent.ID,
		agent.ProjectID,
		agent.JobID,
		agent.Name,
		agent.Slug,
		agent.Description,
		agent.Model,
		agent.ModelFallbacks,
		dbscan.NilIfEmptyRawMessage(agent.Config),
		dbscan.NilIfEmptyString(agent.ProviderSecretsEncrypted),
		dbscan.NilIfEmptyString(agent.CreatedBy),
		dbscan.NilIfEmptyString(agent.UpdatedBy),
	).Scan(&agent.CreatedAt, &agent.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return fmt.Errorf("agent with slug %q already exists: %w", agent.Slug, ErrAgentSlugConflict)
		}
		return fmt.Errorf("create agent: %w", err)
	}

	return nil
}

func (q *Queries) GetAgent(ctx context.Context, id string) (*domain.Agent, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetAgent")
	defer span.End()

	agent, err := scanAgent(q.db.QueryRow(ctx, `SELECT `+agentColumns+` FROM agents WHERE id = $1`, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAgentNotFound
		}
		return nil, fmt.Errorf("get agent: %w", err)
	}
	return agent, nil
}

func (q *Queries) GetAgentBySlug(ctx context.Context, projectID, slug string) (*domain.Agent, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetAgentBySlug")
	defer span.End()

	agent, err := scanAgent(q.db.QueryRow(ctx, `SELECT `+agentColumns+` FROM agents WHERE project_id = $1 AND slug = $2`, projectID, slug))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAgentNotFound
		}
		return nil, fmt.Errorf("get agent by slug: %w", err)
	}
	return agent, nil
}

func (q *Queries) ListAgentsByJobIDs(ctx context.Context, projectID string, jobIDs []string) ([]domain.Agent, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListAgentsByJobIDs")
	defer span.End()

	if len(jobIDs) == 0 {
		return []domain.Agent{}, nil
	}

	rows, err := q.db.Query(ctx, `
		SELECT `+agentColumns+`
		FROM agents
		WHERE project_id = $1 AND job_id = ANY($2::text[])
		ORDER BY created_at DESC
	`, projectID, jobIDs)
	if err != nil {
		return nil, fmt.Errorf("list agents by job ids: %w", err)
	}
	defer rows.Close()

	agents := make([]domain.Agent, 0, len(jobIDs))
	for rows.Next() {
		agent, scanErr := scanAgent(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list agents by job ids scan: %w", scanErr)
		}
		agents = append(agents, *agent)
	}

	return agents, rows.Err()
}

func (q *Queries) ListAgents(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.Agent, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListAgents")
	defer span.End()

	query := `SELECT ` + agentColumns + ` FROM agents WHERE project_id = $1`
	args := []any{projectID}
	param := 2
	if cursor != nil {
		query += fmt.Sprintf(" AND created_at < $%d", param)
		args = append(args, *cursor)
		param++
	}
	if limit <= 0 {
		limit = 50
	}
	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", param)
	args = append(args, limit)

	rows, err := q.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	defer rows.Close()

	agents := make([]domain.Agent, 0, limit)
	for rows.Next() {
		agent, scanErr := scanAgent(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list agents scan: %w", scanErr)
		}
		agents = append(agents, *agent)
	}
	return agents, rows.Err()
}

func (q *Queries) UpdateAgent(ctx context.Context, agent *domain.Agent) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpdateAgent")
	defer span.End()

	err := q.db.QueryRow(ctx, `
		UPDATE agents
		SET name = $2,
		    slug = $3,
		    description = $4,
		    model = $5,
		    model_fallbacks = $6,
		    config = $7,
		    provider_secrets_encrypted = $8,
		    updated_by = $9,
		    updated_at = NOW()
		WHERE id = $1
		RETURNING updated_at
	`,
		agent.ID,
		agent.Name,
		agent.Slug,
		agent.Description,
		agent.Model,
		agent.ModelFallbacks,
		dbscan.NilIfEmptyRawMessage(agent.Config),
		dbscan.NilIfEmptyString(agent.ProviderSecretsEncrypted),
		dbscan.NilIfEmptyString(agent.UpdatedBy),
	).Scan(&agent.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return fmt.Errorf("agent with slug %q already exists: %w", agent.Slug, ErrAgentSlugConflict)
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrAgentNotFound
		}
		return fmt.Errorf("update agent: %w", err)
	}

	return nil
}

// EncryptAgentProviderSecrets encrypts a map of provider keys to a ciphertext
// string suitable for storage in the provider_secrets_encrypted column.
func (q *Queries) EncryptAgentProviderSecrets(secrets map[string]string) (string, error) {
	if len(secrets) == 0 {
		return "", nil
	}
	raw, err := json.Marshal(secrets)
	if err != nil {
		return "", fmt.Errorf("marshal provider secrets: %w", err)
	}
	key, err := q.secretKey()
	if err != nil {
		return "", err
	}
	return encryptSecret(string(raw), key)
}

// DecryptAgentProviderSecrets decrypts a ciphertext string back to a map of
// provider keys. Used exclusively by the runtime envelope builder.
func (q *Queries) DecryptAgentProviderSecrets(ciphertext string) (map[string]string, error) {
	if ciphertext == "" {
		return nil, nil
	}
	plaintext, err := q.decryptSecretWithFallback(ciphertext)
	if err != nil {
		return nil, fmt.Errorf("decrypt provider secrets: %w", err)
	}
	var secrets map[string]string
	if jsonErr := json.Unmarshal([]byte(plaintext), &secrets); jsonErr != nil {
		return nil, fmt.Errorf("unmarshal provider secrets: %w", jsonErr)
	}
	return secrets, nil
}

func (q *Queries) DeleteAgent(ctx context.Context, id string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteAgent")
	defer span.End()

	result, err := q.db.Exec(ctx, `DELETE FROM agents WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete agent: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrAgentNotFound
	}
	return nil
}

func (q *Queries) NextAgentDeploymentVersion(ctx context.Context, agentID string) (int, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.NextAgentDeploymentVersion")
	defer span.End()

	var next int
	if err := q.db.QueryRow(ctx, `SELECT COALESCE(MAX(version), 0) + 1 FROM agent_deployments WHERE agent_id = $1`, agentID).Scan(&next); err != nil {
		return 0, fmt.Errorf("next agent deployment version: %w", err)
	}
	return next, nil
}

func (q *Queries) CreateAgentDeployment(ctx context.Context, deployment *domain.AgentDeployment) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateAgentDeployment")
	defer span.End()

	if deployment.ID == "" {
		deployment.ID = uuid.Must(uuid.NewV7()).String()
	}
	if deployment.Status == "" {
		deployment.Status = domain.AgentDeploymentStatusPending
	}
	if len(deployment.ConfigSnapshot) == 0 {
		deployment.ConfigSnapshot = json.RawMessage(`{}`)
	}
	if len(deployment.ProviderMetadata) == 0 {
		deployment.ProviderMetadata = json.RawMessage(`{}`)
	}

	err := q.db.QueryRow(ctx, `
		INSERT INTO agent_deployments (id, agent_id, version, status, provider, config_snapshot, provider_metadata, created_by, deployed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING created_at, updated_at
	`,
		deployment.ID,
		deployment.AgentID,
		deployment.Version,
		string(deployment.Status),
		deployment.Provider,
		deployment.ConfigSnapshot,
		deployment.ProviderMetadata,
		dbscan.NilIfEmptyString(deployment.CreatedBy),
		deployment.DeployedAt,
	).Scan(&deployment.CreatedAt, &deployment.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create agent deployment: %w", err)
	}
	return nil
}

func (q *Queries) GetLatestAgentDeployment(ctx context.Context, agentID string) (*domain.AgentDeployment, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetLatestAgentDeployment")
	defer span.End()

	deployment, err := scanAgentDeployment(q.db.QueryRow(ctx, `
		SELECT `+agentDeploymentColumns+`
		FROM agent_deployments
		WHERE agent_id = $1
		ORDER BY version DESC
		LIMIT 1
	`, agentID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAgentDeploymentNotFound
		}
		return nil, fmt.Errorf("get latest agent deployment: %w", err)
	}
	return deployment, nil
}

func (q *Queries) ListAgentDeployments(ctx context.Context, agentID string, limit int, cursor *time.Time) ([]domain.AgentDeployment, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListAgentDeployments")
	defer span.End()

	query := `SELECT ` + agentDeploymentColumns + ` FROM agent_deployments WHERE agent_id = $1`
	args := []any{agentID}
	param := 2
	if cursor != nil {
		query += fmt.Sprintf(" AND created_at < $%d", param)
		args = append(args, *cursor)
		param++
	}
	if limit <= 0 {
		limit = 20
	}
	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", param)
	args = append(args, limit)

	rows, err := q.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list agent deployments: %w", err)
	}
	defer rows.Close()

	deployments := make([]domain.AgentDeployment, 0, limit)
	for rows.Next() {
		deployment, scanErr := scanAgentDeployment(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list agent deployments scan: %w", scanErr)
		}
		deployments = append(deployments, *deployment)
	}
	return deployments, rows.Err()
}

func (q *Queries) UpdateAgentDeployment(ctx context.Context, id string, patch map[string]any) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpdateAgentDeployment")
	defer span.End()

	allowedColumns := map[string]struct{}{
		"status":            {},
		"provider_metadata": {},
		"deployed_at":       {},
		"updated_at":        {},
	}

	patch["updated_at"] = time.Now()
	setClauses := make([]string, 0, len(patch))
	args := make([]any, 0, 1+len(patch))
	args = append(args, id)
	param := 2
	for k, v := range patch {
		if _, ok := allowedColumns[k]; !ok {
			return &domain.FieldError{Field: k}
		}
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", k, param))
		args = append(args, v)
		param++
	}

	query := fmt.Sprintf(`UPDATE agent_deployments SET %s WHERE id = $1`, strings.Join(setClauses, ", "))
	result, err := q.db.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update agent deployment: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrAgentDeploymentNotFound
	}
	return nil
}
