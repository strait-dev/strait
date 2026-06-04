package api

import (
	"context"
	"encoding/json"
	"time"

	"strait/internal/domain"

	"github.com/danielgtaylor/huma/v2"
)

func (s *Server) triggerDedupOutput(ctx context.Context, state *triggerRequestState) (*TriggerJobOutput, error) {
	existingRun, err := s.findRecentDeduplicatedRun(ctx, state.job, state.payload)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to evaluate payload deduplication")
	}
	if existingRun == nil {
		return nil, nil
	}
	return triggerRunOutput(existingRun, state.payloadHash, false), nil
}

func (s *Server) findRecentDeduplicatedRun(ctx context.Context, job *domain.Job, payload json.RawMessage) (*domain.JobRun, error) {
	if job == nil || job.DedupWindowSecs <= 0 {
		return nil, nil
	}
	since := time.Now().Add(-time.Duration(job.DedupWindowSecs) * time.Second)
	return s.store.FindRecentRunByPayload(ctx, job.ID, payload, since)
}
