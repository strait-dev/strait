package operations

import "context"

// HealthService provides health check operations.
type HealthService struct {
	r Requester
}

// NewHealthService creates a new HealthService.
func NewHealthService(r Requester) *HealthService {
	return &HealthService{r: r}
}

// List performs a liveness check.
func (s *HealthService) List(ctx context.Context) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", "/health", nil, nil, nil, &result)
	return result, err
}

// GetReady performs a readiness check.
func (s *HealthService) GetReady(ctx context.Context) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", "/health/ready", nil, nil, nil, &result)
	return result, err
}

// ListMetrics returns Prometheus metrics.
func (s *HealthService) ListMetrics(ctx context.Context) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", "/metrics", nil, nil, nil, &result)
	return result, err
}
