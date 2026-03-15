package operations

import "context"

// APIKeysService provides API key management operations.
type APIKeysService struct{ r Requester }

func NewAPIKeysService(r Requester) *APIKeysService { return &APIKeysService{r: r} }

func (s *APIKeysService) List(ctx context.Context, query map[string]string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", "/v1/api-keys", query, nil, nil, &result)
	return result, err
}

func (s *APIKeysService) Create(ctx context.Context, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", "/v1/api-keys", nil, nil, body, &result)
	return result, err
}

func (s *APIKeysService) Delete(ctx context.Context, keyID string) error {
	return s.r.DoRequest(ctx, "DELETE", PathParams("/v1/api-keys/{keyID}", map[string]string{"keyID": keyID}), nil, nil, nil, nil)
}

func (s *APIKeysService) Rotate(ctx context.Context, keyID string, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", PathParams("/v1/api-keys/{keyID}/rotate", map[string]string{"keyID": keyID}), nil, nil, body, &result)
	return result, err
}
