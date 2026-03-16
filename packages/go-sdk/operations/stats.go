package operations

import "context"

// StatsService provides queue statistics operations.
type StatsService struct{ r Requester }

func NewStatsService(r Requester) *StatsService { return &StatsService{r: r} }

func (s *StatsService) List(ctx context.Context, query map[string]string) (map[string]any, error) {
	var result map[string]any
	err := s.r.DoRequest(ctx, "GET", "/v1/stats", query, nil, nil, &result)
	return result, err
}
