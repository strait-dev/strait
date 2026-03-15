package operations

import "context"

// WebhooksService provides webhook management operations.
type WebhooksService struct{ r Requester }

func NewWebhooksService(r Requester) *WebhooksService { return &WebhooksService{r: r} }

func (s *WebhooksService) ListSubscriptions(ctx context.Context, query map[string]string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", "/v1/webhooks/subscriptions", query, nil, nil, &result)
	return result, err
}

func (s *WebhooksService) CreateSubscription(ctx context.Context, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", "/v1/webhooks/subscriptions", nil, nil, body, &result)
	return result, err
}

func (s *WebhooksService) DeleteSubscription(ctx context.Context, id string) error {
	return s.r.DoRequest(ctx, "DELETE", PathParams("/v1/webhooks/subscriptions/{id}", map[string]string{"id": id}), nil, nil, nil, nil)
}

func (s *WebhooksService) ListDeliveries(ctx context.Context, query map[string]string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", "/v1/webhooks/deliveries", query, nil, nil, &result)
	return result, err
}

func (s *WebhooksService) GetDelivery(ctx context.Context, id string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", PathParams("/v1/webhooks/deliveries/{id}", map[string]string{"id": id}), nil, nil, nil, &result)
	return result, err
}

func (s *WebhooksService) RetryDelivery(ctx context.Context, id string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", PathParams("/v1/webhooks/deliveries/{id}/retry", map[string]string{"id": id}), nil, nil, nil, &result)
	return result, err
}
