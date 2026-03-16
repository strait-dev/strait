package operations

import "context"

// LogDrainsService provides log drain management operations.
type LogDrainsService struct{ r Requester }

func NewLogDrainsService(r Requester) *LogDrainsService { return &LogDrainsService{r: r} }

func (s *LogDrainsService) List(ctx context.Context, query map[string]string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", "/v1/log-drains", query, nil, nil, &result)
	return result, err
}

func (s *LogDrainsService) Create(ctx context.Context, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", "/v1/log-drains", nil, nil, body, &result)
	return result, err
}

func (s *LogDrainsService) Get(ctx context.Context, drainID string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", PathParams("/v1/log-drains/{drainID}", map[string]string{"drainID": drainID}), nil, nil, nil, &result)
	return result, err
}

func (s *LogDrainsService) Update(ctx context.Context, drainID string, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "PATCH", PathParams("/v1/log-drains/{drainID}", map[string]string{"drainID": drainID}), nil, nil, body, &result)
	return result, err
}

func (s *LogDrainsService) Delete(ctx context.Context, drainID string) error {
	return s.r.DoRequest(ctx, "DELETE", PathParams("/v1/log-drains/{drainID}", map[string]string{"drainID": drainID}), nil, nil, nil, nil)
}
