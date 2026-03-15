package operations

import "context"

// JobGroupsService provides job group management operations.
type JobGroupsService struct{ r Requester }

func NewJobGroupsService(r Requester) *JobGroupsService { return &JobGroupsService{r: r} }

func (s *JobGroupsService) List(ctx context.Context, query map[string]string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", "/v1/job-groups", query, nil, nil, &result)
	return result, err
}

func (s *JobGroupsService) Create(ctx context.Context, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", "/v1/job-groups", nil, nil, body, &result)
	return result, err
}

func (s *JobGroupsService) Get(ctx context.Context, groupID string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", PathParams("/v1/job-groups/{groupID}", map[string]string{"groupID": groupID}), nil, nil, nil, &result)
	return result, err
}

func (s *JobGroupsService) Update(ctx context.Context, groupID string, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "PATCH", PathParams("/v1/job-groups/{groupID}", map[string]string{"groupID": groupID}), nil, nil, body, &result)
	return result, err
}

func (s *JobGroupsService) Delete(ctx context.Context, groupID string) error {
	return s.r.DoRequest(ctx, "DELETE", PathParams("/v1/job-groups/{groupID}", map[string]string{"groupID": groupID}), nil, nil, nil, nil)
}

func (s *JobGroupsService) ListJobs(ctx context.Context, groupID string, query map[string]string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", PathParams("/v1/job-groups/{groupID}/jobs", map[string]string{"groupID": groupID}), query, nil, nil, &result)
	return result, err
}

func (s *JobGroupsService) PauseAll(ctx context.Context, groupID string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", PathParams("/v1/job-groups/{groupID}/pause-all", map[string]string{"groupID": groupID}), nil, nil, nil, &result)
	return result, err
}

func (s *JobGroupsService) ResumeAll(ctx context.Context, groupID string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", PathParams("/v1/job-groups/{groupID}/resume-all", map[string]string{"groupID": groupID}), nil, nil, nil, &result)
	return result, err
}

func (s *JobGroupsService) GetStats(ctx context.Context, groupID string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", PathParams("/v1/job-groups/{groupID}/stats", map[string]string{"groupID": groupID}), nil, nil, nil, &result)
	return result, err
}
