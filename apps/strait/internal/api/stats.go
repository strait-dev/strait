package api

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
)

type QueueStatsResponse struct {
	Queued    int `json:"queued"`
	Executing int `json:"executing"`
	Delayed   int `json:"delayed"`
}

type StatsInput struct{}
type StatsOutput struct{ Body any }

func (s *Server) handleStats(ctx context.Context, _ *StatsInput) (*StatsOutput, error) {
	stats, err := s.store.QueueStats(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get stats")
	}
	return &StatsOutput{Body: stats}, nil
}
