package operations

import "context"

// EventSourcesService provides event source management operations.
type EventSourcesService struct{ r Requester }

func NewEventSourcesService(r Requester) *EventSourcesService {
	return &EventSourcesService{r: r}
}

func (s *EventSourcesService) List(ctx context.Context, query map[string]string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", "/v1/event-sources", query, nil, nil, &result)
	return result, err
}

func (s *EventSourcesService) Create(ctx context.Context, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", "/v1/event-sources", nil, nil, body, &result)
	return result, err
}

func (s *EventSourcesService) Get(ctx context.Context, sourceID string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", PathParams("/v1/event-sources/{sourceID}", map[string]string{"sourceID": sourceID}), nil, nil, nil, &result)
	return result, err
}

func (s *EventSourcesService) Update(ctx context.Context, sourceID string, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "PATCH", PathParams("/v1/event-sources/{sourceID}", map[string]string{"sourceID": sourceID}), nil, nil, body, &result)
	return result, err
}

func (s *EventSourcesService) Delete(ctx context.Context, sourceID string) error {
	return s.r.DoRequest(ctx, "DELETE", PathParams("/v1/event-sources/{sourceID}", map[string]string{"sourceID": sourceID}), nil, nil, nil, nil)
}

func (s *EventSourcesService) Subscribe(ctx context.Context, sourceID string, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", PathParams("/v1/event-sources/{sourceID}/subscribe", map[string]string{"sourceID": sourceID}), nil, nil, body, &result)
	return result, err
}

func (s *EventSourcesService) ListSubscriptions(ctx context.Context, sourceID string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", PathParams("/v1/event-sources/{sourceID}/subscriptions", map[string]string{"sourceID": sourceID}), nil, nil, nil, &result)
	return result, err
}

func (s *EventSourcesService) DeleteSubscription(ctx context.Context, sourceID, subID string) error {
	return s.r.DoRequest(ctx, "DELETE", PathParams("/v1/event-sources/{sourceID}/subscriptions/{subID}", map[string]string{"sourceID": sourceID, "subID": subID}), nil, nil, nil, nil)
}

func (s *EventSourcesService) DispatchEvent(ctx context.Context, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", "/v1/events/dispatch", nil, nil, body, &result)
	return result, err
}
