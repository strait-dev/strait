package api

import (
	"context"
	"encoding/json"
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
