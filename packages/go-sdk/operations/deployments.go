package operations

import "context"

// DeploymentsService provides deployment management operations.
type DeploymentsService struct {
	r Requester
}

// NewDeploymentsService creates a new DeploymentsService.
func NewDeploymentsService(r Requester) *DeploymentsService {
	return &DeploymentsService{r: r}
}

// List lists deployment versions.
func (s *DeploymentsService) List(ctx context.Context, query map[string]string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", "/v1/deployments", query, nil, nil, &result)
	return result, err
}

// Create creates a deployment version.
func (s *DeploymentsService) Create(ctx context.Context, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", "/v1/deployments", nil, nil, body, &result)
	return result, err
}

// Finalize finalizes a deployment version.
func (s *DeploymentsService) Finalize(ctx context.Context, deploymentID string, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", PathParams("/v1/deployments/{deploymentID}/finalize", map[string]string{"deploymentID": deploymentID}), nil, nil, body, &result)
	return result, err
}

// Promote promotes a deployment version.
func (s *DeploymentsService) Promote(ctx context.Context, deploymentID string, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", PathParams("/v1/deployments/{deploymentID}/promote", map[string]string{"deploymentID": deploymentID}), nil, nil, body, &result)
	return result, err
}

// Rollback rolls back to a previous deployment version.
func (s *DeploymentsService) Rollback(ctx context.Context, deploymentID string, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", PathParams("/v1/deployments/{deploymentID}/rollback", map[string]string{"deploymentID": deploymentID}), nil, nil, body, &result)
	return result, err
}
