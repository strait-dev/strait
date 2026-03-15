package operations

import "context"

// WorkflowsService provides workflow management operations.
type WorkflowsService struct {
	r Requester
}

// NewWorkflowsService creates a new WorkflowsService.
func NewWorkflowsService(r Requester) *WorkflowsService {
	return &WorkflowsService{r: r}
}

// List lists all workflows.
func (s *WorkflowsService) List(ctx context.Context, query map[string]string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", "/v1/workflows", query, nil, nil, &result)
	return result, err
}

// Create creates a new workflow.
func (s *WorkflowsService) Create(ctx context.Context, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", "/v1/workflows", nil, nil, body, &result)
	return result, err
}

// Get gets a workflow by ID.
func (s *WorkflowsService) Get(ctx context.Context, workflowID string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", PathParams("/v1/workflows/{workflowID}", map[string]string{"workflowID": workflowID}), nil, nil, nil, &result)
	return result, err
}

// Update updates a workflow.
func (s *WorkflowsService) Update(ctx context.Context, workflowID string, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "PATCH", PathParams("/v1/workflows/{workflowID}", map[string]string{"workflowID": workflowID}), nil, nil, body, &result)
	return result, err
}

// Delete deletes a workflow.
func (s *WorkflowsService) Delete(ctx context.Context, workflowID string) error {
	return s.r.DoRequest(ctx, "DELETE", PathParams("/v1/workflows/{workflowID}", map[string]string{"workflowID": workflowID}), nil, nil, nil, nil)
}

// Clone clones a workflow.
func (s *WorkflowsService) Clone(ctx context.Context, workflowID string, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", PathParams("/v1/workflows/{workflowID}/clone", map[string]string{"workflowID": workflowID}), nil, nil, body, &result)
	return result, err
}

// DryRun validates a workflow DAG structure.
func (s *WorkflowsService) DryRun(ctx context.Context, workflowID string, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", PathParams("/v1/workflows/{workflowID}/dry-run", map[string]string{"workflowID": workflowID}), nil, nil, body, &result)
	return result, err
}

// Plan builds a topological workflow plan preview.
func (s *WorkflowsService) Plan(ctx context.Context, workflowID string, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", PathParams("/v1/workflows/{workflowID}/plan", map[string]string{"workflowID": workflowID}), nil, nil, body, &result)
	return result, err
}

// Simulate simulates workflow execution.
func (s *WorkflowsService) Simulate(ctx context.Context, workflowID string, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", PathParams("/v1/workflows/{workflowID}/simulate", map[string]string{"workflowID": workflowID}), nil, nil, body, &result)
	return result, err
}

// Trigger triggers a workflow run.
func (s *WorkflowsService) Trigger(ctx context.Context, workflowID string, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", PathParams("/v1/workflows/{workflowID}/trigger", map[string]string{"workflowID": workflowID}), nil, nil, body, &result)
	return result, err
}

// GetGraph gets the DAG visualization for a workflow run.
func (s *WorkflowsService) GetGraph(ctx context.Context, workflowRunID string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", PathParams("/v1/workflow-runs/{workflowRunID}/graph", map[string]string{"workflowRunID": workflowRunID}), nil, nil, nil, &result)
	return result, err
}

// GetGraphByWorkflowID gets the DAG visualization for a workflow.
func (s *WorkflowsService) GetGraphByWorkflowID(ctx context.Context, workflowID string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", PathParams("/v1/workflows/{workflowID}/graph", map[string]string{"workflowID": workflowID}), nil, nil, nil, &result)
	return result, err
}

// ListRuns lists runs for a workflow.
func (s *WorkflowsService) ListRuns(ctx context.Context, workflowID string, query map[string]string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", PathParams("/v1/workflows/{workflowID}/runs", map[string]string{"workflowID": workflowID}), query, nil, nil, &result)
	return result, err
}

// ListVersions lists workflow versions.
func (s *WorkflowsService) ListVersions(ctx context.Context, workflowID string, query map[string]string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", PathParams("/v1/workflows/{workflowID}/versions", map[string]string{"workflowID": workflowID}), query, nil, nil, &result)
	return result, err
}

// GetVersion gets a specific workflow version.
func (s *WorkflowsService) GetVersion(ctx context.Context, workflowID, versionID string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", PathParams("/v1/workflows/{workflowID}/versions/{versionID}", map[string]string{"workflowID": workflowID, "versionID": versionID}), nil, nil, nil, &result)
	return result, err
}

// GetDiff diffs two workflow versions.
func (s *WorkflowsService) GetDiff(ctx context.Context, workflowID, fromVersionID, toVersionID string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", PathParams("/v1/workflows/{workflowID}/versions/{fromVersionID}/diff/{toVersionID}", map[string]string{"workflowID": workflowID, "fromVersionID": fromVersionID, "toVersionID": toVersionID}), nil, nil, nil, &result)
	return result, err
}

// GetPolicy gets the workflow policy.
func (s *WorkflowsService) GetPolicy(ctx context.Context, projectID string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", PathParams("/v1/workflow-policies/{projectID}", map[string]string{"projectID": projectID}), nil, nil, nil, &result)
	return result, err
}

// UpsertPolicy upserts the workflow policy.
func (s *WorkflowsService) UpsertPolicy(ctx context.Context, projectID string, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "PUT", PathParams("/v1/workflow-policies/{projectID}", map[string]string{"projectID": projectID}), nil, nil, body, &result)
	return result, err
}

// GetExplain lists workflow step decisions.
func (s *WorkflowsService) GetExplain(ctx context.Context, workflowRunID string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", PathParams("/v1/workflow-runs/{workflowRunID}/explain", map[string]string{"workflowRunID": workflowRunID}), nil, nil, nil, &result)
	return result, err
}

// ListLabels gets workflow run labels.
func (s *WorkflowsService) ListLabels(ctx context.Context, workflowRunID string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", PathParams("/v1/workflow-runs/{workflowRunID}/labels", map[string]string{"workflowRunID": workflowRunID}), nil, nil, nil, &result)
	return result, err
}

// GetImpact gets workflow version impact.
func (s *WorkflowsService) GetImpact(ctx context.Context, workflowID, versionID string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", PathParams("/v1/workflows/{workflowID}/versions/{versionID}/impact", map[string]string{"workflowID": workflowID, "versionID": versionID}), nil, nil, nil, &result)
	return result, err
}

// ListStepsByVersion lists workflow version steps.
func (s *WorkflowsService) ListStepsByVersion(ctx context.Context, workflowID, versionID string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", PathParams("/v1/workflows/{workflowID}/versions/{versionID}/steps", map[string]string{"workflowID": workflowID, "versionID": versionID}), nil, nil, nil, &result)
	return result, err
}
