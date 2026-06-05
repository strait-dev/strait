package api

import (
	"context"
	"errors"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/danielgtaylor/huma/v2"
)

type BulkCancelAllRequest struct {
	JobID       string           `json:"job_id,omitempty"`
	BatchID     string           `json:"batch_id,omitempty"`
	TriggeredBy string           `json:"triggered_by,omitempty"`
	Status      domain.RunStatus `json:"status,omitempty"`
}

type BulkCancelAllInput struct {
	Body BulkCancelAllRequest
}

type BulkCancelAllOutput struct {
	Body map[string]any
}

func (s *Server) handleBulkCancelAll(ctx context.Context, input *BulkCancelAllInput) (*BulkCancelAllOutput, error) {
	req := input.Body
	projectID := projectIDFromContext(ctx)

	if req.JobID == "" && req.BatchID == "" && req.TriggeredBy == "" && req.Status == "" {
		return nil, huma.Error400BadRequest("at least one filter is required")
	}
	if environmentIDFromContext(ctx) != "" {
		if req.JobID == "" {
			return nil, huma.Error403Forbidden("environment-scoped bulk cancellation requires a job_id filter")
		}
		job, err := s.store.GetJob(ctx, req.JobID)
		if err != nil {
			if errors.Is(err, store.ErrJobNotFound) {
				return nil, huma.Error404NotFound("job not found")
			}
			return nil, huma.Error500InternalServerError("failed to get job")
		}
		if err := requireProjectMatch(ctx, job.ProjectID); err != nil {
			return nil, huma.Error404NotFound("job not found")
		}
		if err := requireEnvironmentMatch(ctx, job.EnvironmentID); err != nil {
			return nil, huma.Error404NotFound("job not found")
		}
	}

	now := time.Now()
	ids, err := s.store.BulkCancelByFilter(ctx, projectID, store.BulkCancelFilter{
		JobID: req.JobID, BatchID: req.BatchID, TriggeredBy: req.TriggeredBy, Status: req.Status,
	}, now, "canceled by user (bulk filter)")
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to cancel runs")
	}

	s.emitAuditEvent(ctx, domain.AuditActionRunBulkCancelledAll, "run", "", map[string]any{
		"project_id":   projectID,
		"canceled":     len(ids),
		"job_id":       req.JobID,
		"batch_id":     req.BatchID,
		"triggered_by": req.TriggeredBy,
		"status":       string(req.Status),
	})

	return &BulkCancelAllOutput{Body: map[string]any{"canceled": len(ids), "run_ids": ids}}, nil
}
