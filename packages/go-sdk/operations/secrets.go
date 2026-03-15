package operations

import "context"

// SecretsService provides secret management operations.
type SecretsService struct{ r Requester }

func NewSecretsService(r Requester) *SecretsService { return &SecretsService{r: r} }

func (s *SecretsService) List(ctx context.Context, query map[string]string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", "/v1/secrets", query, nil, nil, &result)
	return result, err
}

func (s *SecretsService) Create(ctx context.Context, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", "/v1/secrets", nil, nil, body, &result)
	return result, err
}

func (s *SecretsService) Delete(ctx context.Context, secretID string) error {
	return s.r.DoRequest(ctx, "DELETE", PathParams("/v1/secrets/{secretID}", map[string]string{"secretID": secretID}), nil, nil, nil, nil)
}
