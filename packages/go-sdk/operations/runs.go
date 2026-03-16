package operations

import "context"

// RunsService provides run management operations.
type RunsService struct {
	r Requester
}

// NewRunsService creates a new RunsService.
func NewRunsService(r Requester) *RunsService {
	return &RunsService{r: r}
}

// List lists all runs.
func (s *RunsService) List(ctx context.Context, query map[string]string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", "/v1/runs", query, nil, nil, &result)
	return result, err
}

// Get gets a run by ID.
func (s *RunsService) Get(ctx context.Context, runID string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", PathParams("/v1/runs/{runID}", map[string]string{"runID": runID}), nil, nil, nil, &result)
	return result, err
}

// Delete cancels a run.
func (s *RunsService) Delete(ctx context.Context, runID string) error {
	return s.r.DoRequest(ctx, "DELETE", PathParams("/v1/runs/{runID}", map[string]string{"runID": runID}), nil, nil, nil, nil)
}

// ListCheckpoints lists run checkpoints.
func (s *RunsService) ListCheckpoints(ctx context.Context, runID string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", PathParams("/v1/runs/{runID}/checkpoints", map[string]string{"runID": runID}), nil, nil, nil, &result)
	return result, err
}

// GetChildren lists child runs.
func (s *RunsService) GetChildren(ctx context.Context, runID string, query map[string]string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", PathParams("/v1/runs/{runID}/children", map[string]string{"runID": runID}), query, nil, nil, &result)
	return result, err
}

// Debug enables/disables debug mode.
func (s *RunsService) Debug(ctx context.Context, runID string, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", PathParams("/v1/runs/{runID}/debug", map[string]string{"runID": runID}), nil, nil, body, &result)
	return result, err
}

// GetDebugBundle gets the debug bundle.
func (s *RunsService) GetDebugBundle(ctx context.Context, runID string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", PathParams("/v1/runs/{runID}/debug-bundle", map[string]string{"runID": runID}), nil, nil, nil, &result)
	return result, err
}

// ListDependencyStatus gets dependency status for a run.
func (s *RunsService) ListDependencyStatus(ctx context.Context, runID string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", PathParams("/v1/runs/{runID}/dependency-status", map[string]string{"runID": runID}), nil, nil, nil, &result)
	return result, err
}

// DlqReplay replays a dead-lettered run.
func (s *RunsService) DlqReplay(ctx context.Context, runID string, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", PathParams("/v1/runs/{runID}/dlq-replay", map[string]string{"runID": runID}), nil, nil, body, &result)
	return result, err
}

// ListEvents lists run events.
func (s *RunsService) ListEvents(ctx context.Context, runID string, query map[string]string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", PathParams("/v1/runs/{runID}/events", map[string]string{"runID": runID}), query, nil, nil, &result)
	return result, err
}

// DeleteIdempotencyKey resets the idempotency key for a run.
func (s *RunsService) DeleteIdempotencyKey(ctx context.Context, runID string) error {
	return s.r.DoRequest(ctx, "DELETE", PathParams("/v1/runs/{runID}/idempotency-key", map[string]string{"runID": runID}), nil, nil, nil, nil)
}

// GetLineage lists run continuation lineage.
func (s *RunsService) GetLineage(ctx context.Context, runID string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", PathParams("/v1/runs/{runID}/lineage", map[string]string{"runID": runID}), nil, nil, nil, &result)
	return result, err
}

// ListOutputs lists run structured outputs.
func (s *RunsService) ListOutputs(ctx context.Context, runID string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", PathParams("/v1/runs/{runID}/outputs", map[string]string{"runID": runID}), nil, nil, nil, &result)
	return result, err
}

// Replay replays a failed run.
func (s *RunsService) Replay(ctx context.Context, runID string, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", PathParams("/v1/runs/{runID}/replay", map[string]string{"runID": runID}), nil, nil, body, &result)
	return result, err
}

// Reschedule reschedules a run.
func (s *RunsService) Reschedule(ctx context.Context, runID string, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", PathParams("/v1/runs/{runID}/reschedule", map[string]string{"runID": runID}), nil, nil, body, &result)
	return result, err
}

// ListToolCalls lists run tool calls.
func (s *RunsService) ListToolCalls(ctx context.Context, runID string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", PathParams("/v1/runs/{runID}/tool-calls", map[string]string{"runID": runID}), nil, nil, nil, &result)
	return result, err
}

// GetUsage lists run AI model usage.
func (s *RunsService) GetUsage(ctx context.Context, runID string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", PathParams("/v1/runs/{runID}/usage", map[string]string{"runID": runID}), nil, nil, nil, &result)
	return result, err
}

// BulkCancel cancels multiple runs.
func (s *RunsService) BulkCancel(ctx context.Context, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", "/v1/runs/bulk-cancel", nil, nil, body, &result)
	return result, err
}

// BulkCancelAll cancels all runs matching filters.
func (s *RunsService) BulkCancelAll(ctx context.Context, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", "/v1/runs/bulk-cancel-all", nil, nil, body, &result)
	return result, err
}

// BulkDlqReplay bulk replays dead-lettered runs.
func (s *RunsService) BulkDlqReplay(ctx context.Context, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", "/v1/runs/bulk-dlq-replay", nil, nil, body, &result)
	return result, err
}

// BulkReplay replays multiple runs by ID.
func (s *RunsService) BulkReplay(ctx context.Context, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", "/v1/runs/bulk-replay", nil, nil, body, &result)
	return result, err
}

// GetDlq lists dead-lettered runs.
func (s *RunsService) GetDlq(ctx context.Context, query map[string]string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", "/v1/runs/dlq", query, nil, nil, &result)
	return result, err
}
