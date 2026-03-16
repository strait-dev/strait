package operations

import "context"

// AnalyticsService provides performance analytics operations.
type AnalyticsService struct{ r Requester }

func NewAnalyticsService(r Requester) *AnalyticsService { return &AnalyticsService{r: r} }

func (s *AnalyticsService) GetPerformance(ctx context.Context, query map[string]string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", "/v1/analytics/performance", query, nil, nil, &result)
	return result, err
}
