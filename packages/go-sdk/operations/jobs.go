package operations

import "context"

// JobsService provides job management operations.
type JobsService struct {
	r Requester
}

// NewJobsService creates a new JobsService.
func NewJobsService(r Requester) *JobsService {
	return &JobsService{r: r}
}

// List lists all jobs.
func (s *JobsService) List(ctx context.Context, query map[string]string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", "/v1/jobs", query, nil, nil, &result)
	return result, err
}

// Create creates a new job.
func (s *JobsService) Create(ctx context.Context, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", "/v1/jobs", nil, nil, body, &result)
	return result, err
}

// Get gets a job by ID.
func (s *JobsService) Get(ctx context.Context, jobID string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", PathParams("/v1/jobs/{jobID}", map[string]string{"jobID": jobID}), nil, nil, nil, &result)
	return result, err
}

// Update updates a job.
func (s *JobsService) Update(ctx context.Context, jobID string, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "PATCH", PathParams("/v1/jobs/{jobID}", map[string]string{"jobID": jobID}), nil, nil, body, &result)
	return result, err
}

// Delete deletes a job.
func (s *JobsService) Delete(ctx context.Context, jobID string) error {
	return s.r.DoRequest(ctx, "DELETE", PathParams("/v1/jobs/{jobID}", map[string]string{"jobID": jobID}), nil, nil, nil, nil)
}

// Clone clones a job.
func (s *JobsService) Clone(ctx context.Context, jobID string, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", PathParams("/v1/jobs/{jobID}/clone", map[string]string{"jobID": jobID}), nil, nil, body, &result)
	return result, err
}

// GetHealth gets the health score for a job.
func (s *JobsService) GetHealth(ctx context.Context, jobID string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", PathParams("/v1/jobs/{jobID}/health", map[string]string{"jobID": jobID}), nil, nil, nil, &result)
	return result, err
}

// Trigger triggers a job run.
func (s *JobsService) Trigger(ctx context.Context, jobID string, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", PathParams("/v1/jobs/{jobID}/trigger", map[string]string{"jobID": jobID}), nil, nil, body, &result)
	return result, err
}

// BulkTrigger triggers multiple runs of a job.
func (s *JobsService) BulkTrigger(ctx context.Context, jobID string, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", PathParams("/v1/jobs/{jobID}/trigger/bulk", map[string]string{"jobID": jobID}), nil, nil, body, &result)
	return result, err
}

// ListVersions lists job versions.
func (s *JobsService) ListVersions(ctx context.Context, jobID string, query map[string]string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", PathParams("/v1/jobs/{jobID}/versions", map[string]string{"jobID": jobID}), query, nil, nil, &result)
	return result, err
}

// GetVersion gets a specific job version.
func (s *JobsService) GetVersion(ctx context.Context, jobID, versionID string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", PathParams("/v1/jobs/{jobID}/versions/{versionID}", map[string]string{"jobID": jobID, "versionID": versionID}), nil, nil, nil, &result)
	return result, err
}

// ListDependencies lists job dependencies.
func (s *JobsService) ListDependencies(ctx context.Context, jobID string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", PathParams("/v1/jobs/{jobID}/dependencies", map[string]string{"jobID": jobID}), nil, nil, nil, &result)
	return result, err
}

// CreateDependency creates a job dependency.
func (s *JobsService) CreateDependency(ctx context.Context, jobID string, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", PathParams("/v1/jobs/{jobID}/dependencies", map[string]string{"jobID": jobID}), nil, nil, body, &result)
	return result, err
}

// DeleteDependency deletes a job dependency.
func (s *JobsService) DeleteDependency(ctx context.Context, jobID, depID string) error {
	return s.r.DoRequest(ctx, "DELETE", PathParams("/v1/jobs/{jobID}/dependencies/{depID}", map[string]string{"jobID": jobID, "depID": depID}), nil, nil, nil, nil)
}

// Batch creates jobs in batch.
func (s *JobsService) Batch(ctx context.Context, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", "/v1/jobs/batch", nil, nil, body, &result)
	return result, err
}

// BatchDisable disables jobs in batch.
func (s *JobsService) BatchDisable(ctx context.Context, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", "/v1/jobs/batch-disable", nil, nil, body, &result)
	return result, err
}

// BatchEnable enables jobs in batch.
func (s *JobsService) BatchEnable(ctx context.Context, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", "/v1/jobs/batch-enable", nil, nil, body, &result)
	return result, err
}
