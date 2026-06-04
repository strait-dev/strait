package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
)

type BulkTriggerRequest struct {
	Items []BulkTriggerItem `json:"items" validate:"required,min=1"`
}

type BulkTriggerItem struct {
	Payload        json.RawMessage   `json:"payload,omitempty"`
	ScheduledAt    *time.Time        `json:"scheduled_at,omitempty"`
	Priority       int               `json:"priority,omitempty"`
	IdempotencyKey string            `json:"idempotency_key,omitempty"`
	Tags           map[string]string `json:"tags,omitempty"`
	TTLSecs        *int              `json:"ttl_secs,omitempty"`
	ConcurrencyKey string            `json:"concurrency_key,omitempty"`
}

type BulkTriggerResult struct {
	ID             string `json:"id"`
	Status         string `json:"status"`
	IdempotencyHit bool   `json:"idempotency_hit"`
}

type BulkTriggerResponse struct {
	BatchID string              `json:"batch_id"`
	Results []BulkTriggerResult `json:"results"`
	Total   int                 `json:"total"`
	Created int                 `json:"created"`
}

type BulkTriggerJobInput struct {
	JobID string `path:"jobID"`
	Body  BulkTriggerRequest
}

type BulkTriggerJobOutput struct {
	Body BulkTriggerResponse
}

func (s *Server) handleBulkTriggerJob(ctx context.Context, input *BulkTriggerJobInput) (*BulkTriggerJobOutput, error) {
	job, err := s.loadBulkTriggerJob(ctx, input.JobID)
	if err != nil {
		return nil, err
	}

	req := input.Body
	if err := s.validateBulkTriggerRequest(req); err != nil {
		return nil, err
	}

	batchID := uuid.Must(uuid.NewV7()).String()
	if err := s.store.CreateBatchOperation(ctx, &domain.BatchOperation{
		ID:        batchID,
		ProjectID: job.ProjectID,
		JobID:     job.ID,
		ItemCount: len(req.Items),
		CreatedBy: actorFromContext(ctx),
	}); err != nil {
		slog.Error("failed to create batch operation", "error", err)
	}

	now := time.Now()

	projectQuota, err := s.quotaCache.Get(ctx, job.ProjectID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to load project quota")
	}

	state := newBulkTriggerState(s, job, projectQuota, batchID, req.Items, now)
	if err := s.withTriggerLimitGuard(ctx, job, projectQuota, func(guardCtx context.Context, tx store.DBTX) error {
		state.ctx = guardCtx
		state.tx = tx
		if err := state.loadAdmissionCounters(); err != nil {
			return err
		}

		for _, item := range req.Items {
			if err := state.processItem(item); err != nil {
				return err
			}
		}
		return state.enqueuePendingRuns()
	}); err != nil {
		return nil, triggerLimitAPIError(err, "failed to enqueue bulk trigger")
	}

	if err := s.store.FinalizeBatchOperation(ctx, batchID, state.created); err != nil {
		slog.Error("failed to finalize batch operation", "batch_id", batchID, "error", err)
	}

	s.emitAuditEventAsync(ctx, domain.AuditActionJobBulkTriggered, "job", job.ID, map[string]any{
		"batch_id": batchID,
		"total":    len(req.Items),
		"created":  state.created,
	})

	return &BulkTriggerJobOutput{
		Body: BulkTriggerResponse{
			BatchID: batchID,
			Results: state.results,
			Total:   len(req.Items),
			Created: state.created,
		},
	}, nil
}

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

func (s *Server) requireRunEnvironmentMatch(ctx context.Context, run *domain.JobRun) error {
	if environmentIDFromContext(ctx) == "" || run == nil {
		return nil
	}
	job, err := s.store.GetJob(ctx, run.JobID)
	if err != nil {
		return err
	}
	if err := requireProjectMatch(ctx, job.ProjectID); err != nil {
		return err
	}
	return requireEnvironmentMatch(ctx, job.EnvironmentID)
}
