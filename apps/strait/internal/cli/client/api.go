package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"strait/internal/domain"
)

func (c *Client) ListJobs(ctx context.Context, projectID string) ([]domain.Job, error) {
	query := url.Values{}
	query.Set("project_id", projectID)

	var out []domain.Job
	if err := c.doListJSON(ctx, "/v1/jobs", query, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) GetJob(ctx context.Context, id string) (*domain.Job, error) {
	var out domain.Job
	if err := c.doJSON(ctx, http.MethodGet, path.Join("/v1/jobs", id), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) CreateJob(ctx context.Context, req CreateJobRequest) (*domain.Job, error) {
	var out domain.Job
	if err := c.doJSON(ctx, http.MethodPost, "/v1/jobs", nil, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteJob(ctx context.Context, id string) error {
	return c.doJSON(ctx, http.MethodDelete, path.Join("/v1/jobs", id), nil, nil, nil)
}

func (c *Client) UpdateJob(ctx context.Context, id string, req UpdateJobRequest) (*domain.Job, error) {
	var out domain.Job
	if err := c.doJSON(ctx, http.MethodPatch, path.Join("/v1/jobs", id), nil, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) TriggerJob(ctx context.Context, jobID string, req TriggerJobRequest, idempotencyKey string) (*TriggerJobResponse, error) {
	var out TriggerJobResponse
	headers := map[string]string{}
	if strings.TrimSpace(idempotencyKey) != "" {
		headers["X-Idempotency-Key"] = strings.TrimSpace(idempotencyKey)
	}
	if err := c.doJSONWithHeaders(ctx, http.MethodPost, path.Join("/v1/jobs", jobID, "trigger"), nil, req, headers, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) BulkTriggerJob(ctx context.Context, jobID string, req BulkTriggerRequest) (*BulkTriggerResponse, error) {
	var out BulkTriggerResponse
	if err := c.doJSON(ctx, http.MethodPost, path.Join("/v1/jobs", jobID, "trigger", "bulk"), nil, req, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

func (c *Client) ListJobVersions(ctx context.Context, jobID string) ([]domain.JobVersion, error) {
	var out []domain.JobVersion
	if err := c.doListJSON(ctx, path.Join("/v1/jobs", jobID, "versions"), nil, &out); err != nil {
		return nil, err
	}

	return out, nil
}

func (c *Client) ListRuns(ctx context.Context, projectID, status string, limit int, cursor *time.Time) ([]domain.JobRun, error) {
	query := url.Values{}
	query.Set("project_id", projectID)
	if strings.TrimSpace(status) != "" {
		query.Set("status", strings.TrimSpace(status))
	}
	if limit > 0 {
		query.Set("limit", fmt.Sprintf("%d", limit))
	}
	if cursor != nil {
		query.Set("cursor", cursor.Format(time.RFC3339Nano))
	}

	var out []domain.JobRun
	if err := c.doListJSON(ctx, "/v1/runs", query, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListAllRuns fetches all runs by following cursor-based pagination.
func (c *Client) ListAllRuns(ctx context.Context, projectID, status string) ([]domain.JobRun, error) {
	const pageSize = 100
	var all []domain.JobRun
	var cursor *time.Time

	for {
		page, err := c.ListRuns(ctx, projectID, status, pageSize, cursor)
		if err != nil {
			return nil, err
		}
		all = append(all, page...)
		if len(page) < pageSize {
			break
		}
		last := page[len(page)-1].CreatedAt
		cursor = &last
	}
	return all, nil
}

func (c *Client) GetRun(ctx context.Context, runID string) (*domain.JobRun, error) {
	var out domain.JobRun
	if err := c.doJSON(ctx, http.MethodGet, path.Join("/v1/runs", runID), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) CancelRun(ctx context.Context, runID string) (*domain.JobRun, error) {
	var out domain.JobRun
	if err := c.doJSON(ctx, http.MethodDelete, path.Join("/v1/runs", runID), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) ListRunEvents(ctx context.Context, runID, level, eventType string) ([]domain.RunEvent, error) {
	query := url.Values{}
	if strings.TrimSpace(level) != "" {
		query.Set("level", strings.TrimSpace(level))
	}
	if strings.TrimSpace(eventType) != "" {
		query.Set("type", strings.TrimSpace(eventType))
	}

	var out []domain.RunEvent
	if err := c.doListAllJSON(ctx, path.Join("/v1/runs", runID, "events"), query, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) Health(ctx context.Context) (*HealthStatus, error) {
	var out HealthStatus
	if err := c.doJSON(ctx, http.MethodGet, "/health", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) HealthReady(ctx context.Context) (*HealthStatus, error) {
	var out HealthStatus
	if err := c.doJSON(ctx, http.MethodGet, "/health/ready", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) ListWorkflows(ctx context.Context, projectID string) ([]domain.Workflow, error) {
	query := url.Values{}
	query.Set("project_id", projectID)

	var out []domain.Workflow
	if err := c.doListJSON(ctx, "/v1/workflows", query, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) GetWorkflow(ctx context.Context, workflowID string) (*WorkflowResponse, error) {
	var out WorkflowResponse
	if err := c.doJSON(ctx, http.MethodGet, path.Join("/v1/workflows", workflowID), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) CreateWorkflow(ctx context.Context, req CreateWorkflowRequest) (*WorkflowResponse, error) {
	var out WorkflowResponse
	if err := c.doJSON(ctx, http.MethodPost, "/v1/workflows", nil, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateWorkflow(ctx context.Context, id string, req UpdateWorkflowRequest) (*WorkflowResponse, error) {
	var out WorkflowResponse
	if err := c.doJSON(ctx, http.MethodPatch, path.Join("/v1/workflows", id), nil, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteWorkflow(ctx context.Context, id string) error {
	return c.doJSON(ctx, http.MethodDelete, path.Join("/v1/workflows", id), nil, nil, &map[string]string{})
}

func (c *Client) TriggerWorkflow(ctx context.Context, workflowID string, req TriggerWorkflowRequest) (*domain.WorkflowRun, error) {
	var out domain.WorkflowRun
	if err := c.doJSON(ctx, http.MethodPost, path.Join("/v1/workflows", workflowID, "trigger"), nil, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) ListWorkflowRuns(ctx context.Context, workflowID string, limit, offset int) ([]domain.WorkflowRun, error) {
	query := url.Values{}
	if limit > 0 {
		query.Set("limit", fmt.Sprintf("%d", limit))
	}
	if offset > 0 {
		query.Set("offset", fmt.Sprintf("%d", offset))
	}

	var out []domain.WorkflowRun
	if err := c.doListJSON(ctx, path.Join("/v1/workflows", workflowID, "runs"), query, &out); err != nil {
		return nil, err
	}

	return out, nil
}

func (c *Client) ListWorkflowRunsByProject(ctx context.Context, projectID, status string, limit int) ([]domain.WorkflowRun, error) {
	query := url.Values{}
	query.Set("project_id", projectID)
	if strings.TrimSpace(status) != "" {
		query.Set("status", strings.TrimSpace(status))
	}
	if limit > 0 {
		query.Set("limit", fmt.Sprintf("%d", limit))
	}

	var out []domain.WorkflowRun
	if err := c.doListJSON(ctx, "/v1/workflow-runs", query, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) GetWorkflowRun(ctx context.Context, workflowRunID string) (*domain.WorkflowRun, error) {
	var out domain.WorkflowRun
	if err := c.doJSON(ctx, http.MethodGet, path.Join("/v1/workflow-runs", workflowRunID), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) CancelWorkflowRun(ctx context.Context, workflowRunID string) (*domain.WorkflowRun, error) {
	var out domain.WorkflowRun
	if err := c.doJSON(ctx, http.MethodDelete, path.Join("/v1/workflow-runs", workflowRunID), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) ListWorkflowStepRuns(ctx context.Context, workflowRunID string) ([]domain.WorkflowStepRun, error) {
	var out []domain.WorkflowStepRun
	if err := c.doListJSON(ctx, path.Join("/v1/workflow-runs", workflowRunID, "steps"), nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) CreateAPIKey(ctx context.Context, req CreateAPIKeyRequest) (*APIKeyCreateResponse, error) {
	var out APIKeyCreateResponse
	if err := c.doJSON(ctx, http.MethodPost, "/v1/api-keys", nil, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) ListAPIKeys(ctx context.Context, projectID string) ([]domain.APIKey, error) {
	query := url.Values{}
	query.Set("project_id", projectID)

	var out []domain.APIKey
	if err := c.doListJSON(ctx, "/v1/api-keys", query, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) RevokeAPIKey(ctx context.Context, keyID string) error {
	return c.doJSON(ctx, http.MethodDelete, path.Join("/v1/api-keys", keyID), nil, nil, &map[string]string{})
}

func (c *Client) RotateAPIKey(ctx context.Context, keyID string, req RotateAPIKeyRequest) (*RotateAPIKeyResponse, error) {
	var out RotateAPIKeyResponse
	if err := c.doJSON(ctx, http.MethodPost, path.Join("/v1/api-keys", keyID, "rotate"), nil, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) Stats(ctx context.Context) (*QueueStats, error) {
	var out QueueStats
	if err := c.doJSON(ctx, http.MethodGet, "/v1/stats", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) ListEventTriggers(ctx context.Context, projectID, status string) ([]domain.EventTrigger, error) {
	query := url.Values{}
	query.Set("project_id", projectID)
	if status != "" {
		query.Set("status", status)
	}

	var out []domain.EventTrigger
	if err := c.doListJSON(ctx, "/v1/events", query, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) GetEventTrigger(ctx context.Context, eventKey string) (*domain.EventTrigger, error) {
	var out domain.EventTrigger
	if err := c.doJSON(ctx, http.MethodGet, path.Join("/v1/events", eventKey), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) SendEvent(ctx context.Context, eventKey string, payload map[string]any) (*domain.EventTrigger, error) {
	body := map[string]any{}
	if payload != nil {
		body["payload"] = payload
	}
	var out domain.EventTrigger
	if err := c.doJSON(ctx, http.MethodPost, path.Join("/v1/events", eventKey, "send"), nil, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) PurgeEventTriggers(ctx context.Context, olderThanDays int, dryRun bool) (int64, error) {
	body := map[string]any{
		"older_than_days": olderThanDays,
		"dry_run":         dryRun,
	}
	var out map[string]any
	if err := c.doJSON(ctx, http.MethodPost, "/v1/events/purge", nil, body, &out); err != nil {
		return 0, err
	}
	if dryRun {
		if v, ok := out["would_delete"].(float64); ok {
			return int64(v), nil
		}
		return 0, nil
	}
	if v, ok := out["deleted"].(float64); ok {
		return int64(v), nil
	}
	return 0, nil
}

func (c *Client) ListEnvironments(ctx context.Context, projectID string) ([]domain.Environment, error) {
	query := url.Values{}
	query.Set("project_id", projectID)

	var out []domain.Environment
	if err := c.doListJSON(ctx, "/v1/environments", query, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Deployment methods.

func (c *Client) CreateDeploymentVersion(ctx context.Context, req CreateDeploymentVersionRequest) (*DeploymentVersion, error) {
	var out DeploymentVersion
	if err := c.doJSON(ctx, http.MethodPost, "/v1/deployments", nil, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) FinalizeDeployment(ctx context.Context, id string, req FinalizeDeploymentRequest) error {
	return c.doJSON(ctx, http.MethodPost, path.Join("/v1/deployments", id, "finalize"), nil, req, nil)
}

func (c *Client) PromoteDeployment(ctx context.Context, id string, req PromoteDeploymentRequest) error {
	return c.doJSON(ctx, http.MethodPost, path.Join("/v1/deployments", id, "promote"), nil, req, nil)
}

func (c *Client) RollbackDeployment(ctx context.Context, id string, req RollbackDeploymentRequest) error {
	return c.doJSON(ctx, http.MethodPost, path.Join("/v1/deployments", id, "rollback"), nil, req, nil)
}

func (c *Client) ListDeployments(ctx context.Context, projectID string, limit int) ([]DeploymentVersion, error) {
	query := url.Values{}
	query.Set("project_id", projectID)
	if limit > 0 {
		query.Set("limit", fmt.Sprintf("%d", limit))
	}

	var out []DeploymentVersion
	if err := c.doListJSON(ctx, "/v1/deployments", query, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Server-side secret methods.

func (c *Client) ListServerSecrets(ctx context.Context, projectID, environment string) ([]ServerSecret, error) {
	query := url.Values{}
	query.Set("project_id", projectID)
	if strings.TrimSpace(environment) != "" {
		query.Set("environment", strings.TrimSpace(environment))
	}

	var out []ServerSecret
	if err := c.doListJSON(ctx, "/v1/secrets", query, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) CreateServerSecret(ctx context.Context, req CreateServerSecretRequest) (*ServerSecret, error) {
	var out ServerSecret
	if err := c.doJSON(ctx, http.MethodPost, "/v1/secrets", nil, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteServerSecret(ctx context.Context, id string) error {
	return c.doJSON(ctx, http.MethodDelete, path.Join("/v1/secrets", id), nil, nil, nil)
}

// Performance analytics methods.

func (c *Client) GetPerformanceAnalytics(ctx context.Context, projectID string, periodHours int) (*PerformanceAnalytics, error) {
	query := url.Values{}
	query.Set("project_id", projectID)
	if periodHours > 0 {
		query.Set("period_hours", fmt.Sprintf("%d", periodHours))
	}

	var out PerformanceAnalytics
	if err := c.doJSON(ctx, http.MethodGet, "/v1/analytics/performance", query, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Team/RBAC methods.

func (c *Client) ListMembers(ctx context.Context, projectID string) ([]ProjectMember, error) {
	query := url.Values{}
	query.Set("project_id", projectID)

	var out []ProjectMember
	if err := c.doListJSON(ctx, "/v1/members", query, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) AddMember(ctx context.Context, req AssignMemberRequest) (*ProjectMember, error) {
	var out ProjectMember
	if err := c.doJSON(ctx, http.MethodPost, "/v1/members", nil, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) RemoveMember(ctx context.Context, userID string) error {
	return c.doJSON(ctx, http.MethodDelete, path.Join("/v1/members", userID), nil, nil, nil)
}

func (c *Client) ListRoles(ctx context.Context, projectID string) ([]ProjectRole, error) {
	query := url.Values{}
	query.Set("project_id", projectID)

	var out []ProjectRole
	if err := c.doListJSON(ctx, "/v1/roles", query, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) ListAuditEvents(ctx context.Context, params ListAuditEventsParams) ([]AuditEvent, error) {
	query := url.Values{}
	query.Set("project_id", params.ProjectID)
	if strings.TrimSpace(params.ActorID) != "" {
		query.Set("actor_id", strings.TrimSpace(params.ActorID))
	}
	if strings.TrimSpace(params.ResourceType) != "" {
		query.Set("resource_type", strings.TrimSpace(params.ResourceType))
	}
	if strings.TrimSpace(params.ResourceID) != "" {
		query.Set("resource_id", strings.TrimSpace(params.ResourceID))
	}
	if params.Limit > 0 {
		query.Set("limit", fmt.Sprintf("%d", params.Limit))
	}
	if params.From != nil {
		query.Set("from", params.From.UTC().Format(time.RFC3339Nano))
	}
	if params.To != nil {
		query.Set("to", params.To.UTC().Format(time.RFC3339Nano))
	}
	if strings.TrimSpace(params.Order) != "" {
		query.Set("order", strings.TrimSpace(params.Order))
	}

	var out []AuditEvent
	if err := c.doListJSON(ctx, "/v1/audit-events", query, &out); err != nil {
		return nil, err
	}
	return out, nil
}
