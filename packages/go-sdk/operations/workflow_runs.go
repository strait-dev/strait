package operations

import "context"

// WorkflowRunsService provides workflow run management operations.
type WorkflowRunsService struct {
	r Requester
}

// NewWorkflowRunsService creates a new WorkflowRunsService.
func NewWorkflowRunsService(r Requester) *WorkflowRunsService {
	return &WorkflowRunsService{r: r}
}

// List lists all workflow runs.
func (s *WorkflowRunsService) List(ctx context.Context, query map[string]string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", "/v1/workflow-runs", query, nil, nil, &result)
	return result, err
}

// Get gets a workflow run by ID.
func (s *WorkflowRunsService) Get(ctx context.Context, workflowRunID string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", PathParams("/v1/workflow-runs/{workflowRunID}", map[string]string{"workflowRunID": workflowRunID}), nil, nil, nil, &result)
	return result, err
}

// Delete cancels a workflow run.
func (s *WorkflowRunsService) Delete(ctx context.Context, workflowRunID string) error {
	return s.r.DoRequest(ctx, "DELETE", PathParams("/v1/workflow-runs/{workflowRunID}", map[string]string{"workflowRunID": workflowRunID}), nil, nil, nil, nil)
}

// Pause pauses a workflow run.
func (s *WorkflowRunsService) Pause(ctx context.Context, workflowRunID string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", PathParams("/v1/workflow-runs/{workflowRunID}/pause", map[string]string{"workflowRunID": workflowRunID}), nil, nil, nil, &result)
	return result, err
}

// Resume resumes a workflow run.
func (s *WorkflowRunsService) Resume(ctx context.Context, workflowRunID string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", PathParams("/v1/workflow-runs/{workflowRunID}/resume", map[string]string{"workflowRunID": workflowRunID}), nil, nil, nil, &result)
	return result, err
}

// Retry retries from the first failed step.
func (s *WorkflowRunsService) Retry(ctx context.Context, workflowRunID string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", PathParams("/v1/workflow-runs/{workflowRunID}/retry", map[string]string{"workflowRunID": workflowRunID}), nil, nil, nil, &result)
	return result, err
}

// ListSteps lists step runs.
func (s *WorkflowRunsService) ListSteps(ctx context.Context, workflowRunID string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", PathParams("/v1/workflow-runs/{workflowRunID}/steps", map[string]string{"workflowRunID": workflowRunID}), nil, nil, nil, &result)
	return result, err
}

// ApproveStep approves an approval step.
func (s *WorkflowRunsService) ApproveStep(ctx context.Context, workflowRunID, stepRef string, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", PathParams("/v1/workflow-runs/{workflowRunID}/steps/{stepRef}/approve", map[string]string{"workflowRunID": workflowRunID, "stepRef": stepRef}), nil, nil, body, &result)
	return result, err
}

// RetryStep retries a single workflow step.
func (s *WorkflowRunsService) RetryStep(ctx context.Context, workflowRunID, stepRef string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", PathParams("/v1/workflow-runs/{workflowRunID}/steps/{stepRef}/retry", map[string]string{"workflowRunID": workflowRunID, "stepRef": stepRef}), nil, nil, nil, &result)
	return result, err
}

// SkipStep skips a step.
func (s *WorkflowRunsService) SkipStep(ctx context.Context, workflowRunID, stepRef string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", PathParams("/v1/workflow-runs/{workflowRunID}/steps/{stepRef}/skip", map[string]string{"workflowRunID": workflowRunID, "stepRef": stepRef}), nil, nil, nil, &result)
	return result, err
}

// ForceCompleteStep force-completes a step.
func (s *WorkflowRunsService) ForceCompleteStep(ctx context.Context, workflowRunID, stepRef string, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", PathParams("/v1/workflow-runs/{workflowRunID}/steps/{stepRef}/force-complete", map[string]string{"workflowRunID": workflowRunID, "stepRef": stepRef}), nil, nil, body, &result)
	return result, err
}

// ReplaySubtreeStep replays a step subtree.
func (s *WorkflowRunsService) ReplaySubtreeStep(ctx context.Context, workflowRunID, stepRef string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", PathParams("/v1/workflow-runs/{workflowRunID}/steps/{stepRef}/replay-subtree", map[string]string{"workflowRunID": workflowRunID, "stepRef": stepRef}), nil, nil, nil, &result)
	return result, err
}

// BulkCancel bulk cancels workflow runs.
func (s *WorkflowRunsService) BulkCancel(ctx context.Context, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", "/v1/workflow-runs/bulk-cancel", nil, nil, body, &result)
	return result, err
}

// BulkReplay bulk replays workflow runs.
func (s *WorkflowRunsService) BulkReplay(ctx context.Context, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", "/v1/workflow-runs/bulk-replay", nil, nil, body, &result)
	return result, err
}
