package operations

import "context"

// EnvironmentsService provides environment management operations.
type EnvironmentsService struct{ r Requester }

// NewEnvironmentsService creates a new EnvironmentsService.
func NewEnvironmentsService(r Requester) *EnvironmentsService {
	return &EnvironmentsService{r: r}
}

func (s *EnvironmentsService) List(ctx context.Context, query map[string]string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", "/v1/environments", query, nil, nil, &result)
	return result, err
}

func (s *EnvironmentsService) Create(ctx context.Context, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", "/v1/environments", nil, nil, body, &result)
	return result, err
}

func (s *EnvironmentsService) Get(ctx context.Context, envID string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", PathParams("/v1/environments/{envID}", map[string]string{"envID": envID}), nil, nil, nil, &result)
	return result, err
}

func (s *EnvironmentsService) Update(ctx context.Context, envID string, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "PATCH", PathParams("/v1/environments/{envID}", map[string]string{"envID": envID}), nil, nil, body, &result)
	return result, err
}

func (s *EnvironmentsService) Delete(ctx context.Context, envID string) error {
	return s.r.DoRequest(ctx, "DELETE", PathParams("/v1/environments/{envID}", map[string]string{"envID": envID}), nil, nil, nil, nil)
}

func (s *EnvironmentsService) ListVariables(ctx context.Context, envID string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", PathParams("/v1/environments/{envID}/variables", map[string]string{"envID": envID}), nil, nil, nil, &result)
	return result, err
}
