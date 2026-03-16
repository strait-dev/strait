package operations

import "context"

// BatchOperationsService provides batch operation management.
type BatchOperationsService struct{ r Requester }

func NewBatchOperationsService(r Requester) *BatchOperationsService {
	return &BatchOperationsService{r: r}
}

func (s *BatchOperationsService) List(ctx context.Context, query map[string]string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", "/v1/batch-operations", query, nil, nil, &result)
	return result, err
}

func (s *BatchOperationsService) Get(ctx context.Context, batchID string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", PathParams("/v1/batch-operations/{batchID}", map[string]string{"batchID": batchID}), nil, nil, nil, &result)
	return result, err
}
