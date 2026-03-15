package operations

import "context"

// EventTriggersService provides event trigger management operations.
type EventTriggersService struct{ r Requester }

func NewEventTriggersService(r Requester) *EventTriggersService {
	return &EventTriggersService{r: r}
}

func (s *EventTriggersService) ListEvents(ctx context.Context, query map[string]string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", "/v1/events", query, nil, nil, &result)
	return result, err
}

func (s *EventTriggersService) GetEvent(ctx context.Context, eventKey string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", PathParams("/v1/events/{eventKey}", map[string]string{"eventKey": eventKey}), nil, nil, nil, &result)
	return result, err
}

func (s *EventTriggersService) DeleteEvent(ctx context.Context, eventKey string) error {
	return s.r.DoRequest(ctx, "DELETE", PathParams("/v1/events/{eventKey}", map[string]string{"eventKey": eventKey}), nil, nil, nil, nil)
}

func (s *EventTriggersService) SendEvent(ctx context.Context, eventKey string, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", PathParams("/v1/events/{eventKey}/send", map[string]string{"eventKey": eventKey}), nil, nil, body, &result)
	return result, err
}

func (s *EventTriggersService) SendPrefix(ctx context.Context, prefix string, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", PathParams("/v1/events/prefix/{prefix}/send", map[string]string{"prefix": prefix}), nil, nil, body, &result)
	return result, err
}

func (s *EventTriggersService) PurgeEvent(ctx context.Context, body any) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "POST", "/v1/events/purge", nil, nil, body, &result)
	return result, err
}

func (s *EventTriggersService) GetStat(ctx context.Context) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", "/v1/events/stats", nil, nil, nil, &result)
	return result, err
}
