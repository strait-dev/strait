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

const agentColumns = `id, project_id, job_id, name, slug, description, model, model_fallbacks, config, provider_secrets_encrypted, created_by, updated_by, created_at, updated_at, enabled, dismissed_recommendations`

const agentDeploymentColumns = `id, agent_id, environment_id, version, status, provider, config_snapshot, provider_metadata, created_by, created_at, updated_at, deployed_at`

func scanAgent(scanner interface {
	Scan(dest ...any) error
}) (*domain.Agent, error) {
	var agent domain.Agent
	var createdBy *string
	var updatedBy *string
	var providerSecretsEncrypted *string
	var dismissedRecsJSON []byte
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
		&agent.Enabled,
		&dismissedRecsJSON,
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
	if len(dismissedRecsJSON) > 0 {
		_ = json.Unmarshal(dismissedRecsJSON, &agent.DismissedRecommendations)
	}
	return &agent, nil
}

func scanAgentDeployment(scanner interface {
	Scan(dest ...any) error
}) (*domain.AgentDeployment, error) {
	var deployment domain.AgentDeployment
	var status string
	var createdBy *string
	var environmentID *string
	if err := scanner.Scan(
		&deployment.ID,
		&deployment.AgentID,
		&environmentID,
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
	if environmentID != nil {
		deployment.EnvironmentID = *environmentID
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

func (q *Queries) GetAgentByJobID(ctx context.Context, jobID string) (*domain.Agent, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetAgentByJobID")
	defer span.End()

	agent, err := scanAgent(q.db.QueryRow(ctx, `SELECT `+agentColumns+` FROM agents WHERE job_id = $1`, jobID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAgentNotFound
		}
		return nil, fmt.Errorf("get agent by job id: %w", err)
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

func (q *Queries) SetAgentEnabled(ctx context.Context, agentID string, enabled bool) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.SetAgentEnabled")
	defer span.End()
	_, err := q.db.Exec(ctx, `UPDATE agents SET enabled = $2, updated_at = NOW() WHERE id = $1`, agentID, enabled)
	if err != nil {
		return fmt.Errorf("set agent enabled: %w", err)
	}
	return nil
}

func (q *Queries) DismissRecommendation(ctx context.Context, agentID, recID, actor string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DismissRecommendation")
	defer span.End()
	rec := domain.DismissedRecommendation{
		RecommendationID: recID,
		DismissedAt:      time.Now(),
		DismissedBy:      actor,
	}
	recJSON, err := json.Marshal([]domain.DismissedRecommendation{rec})
	if err != nil {
		return fmt.Errorf("marshal dismissed recommendation: %w", err)
	}
	_, err = q.db.Exec(ctx,
		`UPDATE agents SET dismissed_recommendations = dismissed_recommendations || $2::jsonb, updated_at = NOW() WHERE id = $1`,
		agentID, recJSON)
	if err != nil {
		return fmt.Errorf("dismiss recommendation: %w", err)
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
		INSERT INTO agent_deployments (id, agent_id, environment_id, version, status, provider, config_snapshot, provider_metadata, created_by, deployed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING created_at, updated_at
	`,
		deployment.ID,
		deployment.AgentID,
		dbscan.NilIfEmptyString(deployment.EnvironmentID),
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

// GetAgentHealthStats returns aggregated health metrics for an agent over a time window.
// Queries the agent's backing job runs and computes a composite health score
// factoring success rate, error class distribution, and cost efficiency.
// GetAgentHealthStats returns aggregate health metrics for an agent
// over the window `[since, now()]`. Collapsed to a single CTE query
// (plus one agent-lookup up front) so the `job_runs` partition scan
// happens exactly once per call. Pre-F2 this function issued four
// separate round-trips (agent, run stats, error classes, cost) and
// re-filtered `job_runs` by `(job_id, created_at)` three times.
func (q *Queries) GetAgentHealthStats(ctx context.Context, agentID string, since time.Time) (*AgentHealthStats, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetAgentHealthStats")
	defer span.End()

	agent, err := scanAgent(q.db.QueryRow(ctx, `SELECT `+agentColumns+` FROM agents WHERE id = $1`, agentID))
	if err != nil {
		return nil, fmt.Errorf("get agent for health: %w", err)
	}

	// One CTE materializes the matching runs; three sibling CTEs
	// aggregate over it without re-scanning job_runs. The final
	// SELECT cross-joins the three single-row aggregate CTEs so
	// the result is a single row of columns.
	query := `
		WITH recent_runs AS (
			SELECT id, status, finished_at, started_at
			FROM job_runs
			WHERE job_id = $1
			  AND created_at >= $2
			  AND status IN ('completed','failed','system_failed','timed_out','crashed','canceled')
		),
		run_stats AS (
			SELECT
				COUNT(*) AS total_runs,
				COUNT(*) FILTER (WHERE status = 'completed') AS completed_runs,
				COUNT(*) FILTER (WHERE status IN ('failed','system_failed')) AS failed_runs,
				COALESCE(
					AVG(EXTRACT(EPOCH FROM (finished_at - started_at)))
						FILTER (WHERE finished_at IS NOT NULL AND started_at IS NOT NULL),
					0
				) AS avg_duration_secs
			FROM recent_runs
		),
		err_stats AS (
			SELECT
				COUNT(*) FILTER (WHERE data::jsonb->>'error_class' = 'oom') AS oom_runs,
				COUNT(*) FILTER (WHERE data::jsonb->>'error_class' = 'timeout') AS timeout_runs,
				COUNT(*) FILTER (WHERE data::jsonb->>'error_class' = 'rate_limited') AS rate_limited_runs
			FROM run_events
			WHERE run_id IN (SELECT id FROM recent_runs)
			  AND type = 'error'
		),
		cost_stats AS (
			SELECT COALESCE(AVG(cost_microusd), 0) AS avg_cost_microusd
			FROM run_usage
			WHERE run_id IN (SELECT id FROM recent_runs)
		)
		SELECT
			run_stats.total_runs,
			run_stats.completed_runs,
			run_stats.failed_runs,
			run_stats.avg_duration_secs,
			err_stats.oom_runs,
			err_stats.timeout_runs,
			err_stats.rate_limited_runs,
			cost_stats.avg_cost_microusd
		FROM run_stats, err_stats, cost_stats`

	stats := &AgentHealthStats{}
	if err := q.db.QueryRow(ctx, query, agent.JobID, since).Scan(
		&stats.TotalRuns,
		&stats.CompletedRuns,
		&stats.FailedRuns,
		&stats.AvgDurationSecs,
		&stats.OOMRuns,
		&stats.TimeoutRuns,
		&stats.RateLimitedRuns,
		&stats.AvgCostMicrousd,
	); err != nil {
		return nil, fmt.Errorf("get agent health stats: %w", err)
	}

	// Compute health score.
	stats.HealthScore, stats.HealthLevel = ComputeAgentHealthScore(stats)
	if stats.TotalRuns > 0 {
		stats.SuccessRate = float64(stats.CompletedRuns) / float64(stats.TotalRuns) * 100
	}

	return stats, nil
}

// ComputeAgentHealthScore calculates a 0-100 health score for an agent.
// 50% success rate + 25% error class severity + 25% stability.
func ComputeAgentHealthScore(stats *AgentHealthStats) (float64, string) {
	if stats.TotalRuns == 0 {
		return 0, "unknown"
	}

	successRate := float64(stats.CompletedRuns) / float64(stats.TotalRuns)
	failureRate := 1 - successRate

	// Success component: 60% weight.
	successComponent := successRate * 60

	// Error class component: 20% weight. Scaled by success -- no credit if everything fails.
	// OOM/timeout are penalized extra on top of the failure rate.
	errorComponent := successRate * 20
	if stats.TotalRuns > 0 {
		severeErrorRate := float64(stats.OOMRuns+stats.TimeoutRuns) / float64(stats.TotalRuns)
		errorComponent = max(0, errorComponent-severeErrorRate*40)
	}

	// Stability component: 20% weight. Penalize high failure rates and long durations.
	stabilityComponent := (1 - failureRate) * 20
	if stats.AvgDurationSecs > 120 {
		penalty := min(stabilityComponent, (stats.AvgDurationSecs-120)/60*5)
		stabilityComponent = max(0, stabilityComponent-penalty)
	}

	score := min(100, successComponent+errorComponent+stabilityComponent)

	level := "healthy"
	switch {
	case score < 30:
		level = "unhealthy"
	case score <= 60:
		level = "degraded"
	}

	return score, level
}

// GetAgentDeploymentByID returns a single deployment by its ID.
func (q *Queries) GetAgentDeploymentByID(ctx context.Context, deploymentID string) (*domain.AgentDeployment, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetAgentDeploymentByID")
	defer span.End()

	d, err := scanAgentDeployment(q.db.QueryRow(ctx, `SELECT `+agentDeploymentColumns+` FROM agent_deployments WHERE id = $1`, deploymentID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAgentNotFound
		}
		return nil, fmt.Errorf("get agent deployment by id: %w", err)
	}
	return d, nil
}

// GetLatestAgentDeploymentByEnvironment returns the most recent deployed
// deployment for an agent in the given environment. Used by RunAgent to pick
// the target deployment when the caller specifies an environment_id, and by
// the canary router to determine the current primary in an env.
func (q *Queries) GetLatestAgentDeploymentByEnvironment(ctx context.Context, agentID, environmentID string) (*domain.AgentDeployment, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetLatestAgentDeploymentByEnvironment")
	defer span.End()

	deployment, err := scanAgentDeployment(q.db.QueryRow(ctx, `
		SELECT `+agentDeploymentColumns+`
		FROM agent_deployments
		WHERE agent_id = $1 AND environment_id = $2
		ORDER BY version DESC
		LIMIT 1
	`, agentID, environmentID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAgentDeploymentNotFound
		}
		return nil, fmt.Errorf("get latest agent deployment by env: %w", err)
	}
	return deployment, nil
}

// RollbackAgentDeployment creates a new deployment using the config_snapshot from
// a target deployment. This effectively "rolls back" to a previous version.
func (q *Queries) RollbackAgentDeployment(ctx context.Context, agentID, targetDeploymentID, actor string) (*domain.AgentDeployment, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.RollbackAgentDeployment")
	defer span.End()

	// Get target deployment's config.
	target, err := q.GetAgentDeploymentByID(ctx, targetDeploymentID)
	if err != nil {
		return nil, fmt.Errorf("get rollback target: %w", err)
	}
	if target.AgentID != agentID {
		return nil, fmt.Errorf("deployment %s does not belong to agent %s", targetDeploymentID, agentID)
	}

	// Get next version number.
	nextVersion, err := q.NextAgentDeploymentVersion(ctx, agentID)
	if err != nil {
		return nil, fmt.Errorf("get next version: %w", err)
	}

	// Create new deployment from target's config snapshot.
	newDeploy := &domain.AgentDeployment{
		ID:             uuid.Must(uuid.NewV7()).String(),
		AgentID:        agentID,
		Version:        nextVersion,
		Status:         domain.AgentDeploymentStatusDeployed,
		Provider:       target.Provider,
		ConfigSnapshot: target.ConfigSnapshot,
		CreatedBy:      actor,
	}
	if err := q.CreateAgentDeployment(ctx, newDeploy); err != nil {
		return nil, fmt.Errorf("create rollback deployment: %w", err)
	}

	return newDeploy, nil
}
