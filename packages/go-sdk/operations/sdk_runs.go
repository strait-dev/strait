package operations

import "context"

// SDKRunsService provides SDK run-token operations for executor use.
type SDKRunsService struct{ r Requester }

func NewSDKRunsService(r Requester) *SDKRunsService { return &SDKRunsService{r: r} }

func (s *SDKRunsService) AnnotateRun(ctx context.Context, runID string, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", PathParams("/sdk/v1/runs/{runID}/annotate", map[string]string{"runID": runID}), nil, nil, body, &result)
	return result, err
}

func (s *SDKRunsService) CheckpointRun(ctx context.Context, runID string, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", PathParams("/sdk/v1/runs/{runID}/checkpoint", map[string]string{"runID": runID}), nil, nil, body, &result)
	return result, err
}

func (s *SDKRunsService) CompleteRun(ctx context.Context, runID string, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", PathParams("/sdk/v1/runs/{runID}/complete", map[string]string{"runID": runID}), nil, nil, body, &result)
	return result, err
}

func (s *SDKRunsService) ContinueRun(ctx context.Context, runID string, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", PathParams("/sdk/v1/runs/{runID}/continue", map[string]string{"runID": runID}), nil, nil, body, &result)
	return result, err
}

func (s *SDKRunsService) FailRun(ctx context.Context, runID string, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", PathParams("/sdk/v1/runs/{runID}/fail", map[string]string{"runID": runID}), nil, nil, body, &result)
	return result, err
}

func (s *SDKRunsService) HeartbeatRun(ctx context.Context, runID string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", PathParams("/sdk/v1/runs/{runID}/heartbeat", map[string]string{"runID": runID}), nil, nil, nil, &result)
	return result, err
}

func (s *SDKRunsService) LogRun(ctx context.Context, runID string, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", PathParams("/sdk/v1/runs/{runID}/log", map[string]string{"runID": runID}), nil, nil, body, &result)
	return result, err
}

func (s *SDKRunsService) OutputRun(ctx context.Context, runID string, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", PathParams("/sdk/v1/runs/{runID}/output", map[string]string{"runID": runID}), nil, nil, body, &result)
	return result, err
}

func (s *SDKRunsService) ProgressRun(ctx context.Context, runID string, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", PathParams("/sdk/v1/runs/{runID}/progress", map[string]string{"runID": runID}), nil, nil, body, &result)
	return result, err
}

func (s *SDKRunsService) SpawnRun(ctx context.Context, runID string, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", PathParams("/sdk/v1/runs/{runID}/spawn", map[string]string{"runID": runID}), nil, nil, body, &result)
	return result, err
}

func (s *SDKRunsService) ToolCallRun(ctx context.Context, runID string, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", PathParams("/sdk/v1/runs/{runID}/tool-call", map[string]string{"runID": runID}), nil, nil, body, &result)
	return result, err
}

func (s *SDKRunsService) UsageRun(ctx context.Context, runID string, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", PathParams("/sdk/v1/runs/{runID}/usage", map[string]string{"runID": runID}), nil, nil, body, &result)
	return result, err
}

func (s *SDKRunsService) WaitForEventRun(ctx context.Context, runID string, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", PathParams("/sdk/v1/runs/{runID}/wait-for-event", map[string]string{"runID": runID}), nil, nil, body, &result)
	return result, err
}

func (s *SDKRunsService) SetState(ctx context.Context, runID string, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", PathParams("/sdk/v1/runs/{runID}/state", map[string]string{"runID": runID}), nil, nil, body, &result)
	return result, err
}

func (s *SDKRunsService) ListState(ctx context.Context, runID string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", PathParams("/sdk/v1/runs/{runID}/state", map[string]string{"runID": runID}), nil, nil, nil, &result)
	return result, err
}

func (s *SDKRunsService) GetState(ctx context.Context, runID string, key string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", PathParams("/sdk/v1/runs/{runID}/state/{key}", map[string]string{"runID": runID, "key": key}), nil, nil, nil, &result)
	return result, err
}

func (s *SDKRunsService) DeleteState(ctx context.Context, runID string, key string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "DELETE", PathParams("/sdk/v1/runs/{runID}/state/{key}", map[string]string{"runID": runID, "key": key}), nil, nil, nil, &result)
	return result, err
}

func (s *SDKRunsService) GetPayload(ctx context.Context, runID string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", PathParams("/sdk/v1/runs/{runID}/payload", map[string]string{"runID": runID}), nil, nil, nil, &result)
	return result, err
}

func (s *SDKRunsService) ResourcesRun(ctx context.Context, runID string, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", PathParams("/sdk/v1/runs/{runID}/resources", map[string]string{"runID": runID}), nil, nil, body, &result)
	return result, err
}

func (s *SDKRunsService) StreamRun(ctx context.Context, runID string, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", PathParams("/sdk/v1/runs/{runID}/stream", map[string]string{"runID": runID}), nil, nil, body, &result)
	return result, err
}
