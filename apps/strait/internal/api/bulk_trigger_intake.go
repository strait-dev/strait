package api

import (
	"context"
	"fmt"

	"strait/internal/domain"

	"github.com/danielgtaylor/huma/v2"
)

func (s *Server) loadBulkTriggerJob(ctx context.Context, jobID string) (*domain.Job, error) {
	job, err := s.loadRunCreationJob(ctx, jobID, "bulk_trigger_job.project_match", "handleBulkTriggerJob")
	if err != nil {
		return nil, err
	}
	if err := s.checkHTTPModeAllowed(ctx, job.ExecutionMode, job.ProjectID); err != nil {
		return nil, err
	}
	if err := ensureJobTriggerable(job); err != nil {
		return nil, err
	}
	return job, nil
}

func (s *Server) validateBulkTriggerRequest(req BulkTriggerRequest) error {
	if err := s.validate.Struct(&req); err != nil {
		return newValidationError(err)
	}
	if len(req.Items) > s.config.MaxBulkTriggerItems {
		return huma.Error400BadRequest(fmt.Sprintf("maximum %d items per bulk trigger request", s.config.MaxBulkTriggerItems))
	}
	return nil
}

func (s *Server) validateBulkTriggerItem(job *domain.Job, item BulkTriggerItem, itemIdx int) error {
	if err := validateTriggerScheduledAt(item.ScheduledAt); err != nil {
		return huma.Error400BadRequest(fmt.Sprintf("scheduled_at validation failed for item %d: %v", itemIdx, err))
	}
	if err := validateTriggerTTLSecs(item.TTLSecs); err != nil {
		return huma.Error400BadRequest(fmt.Sprintf("ttl_secs validation failed for item %d: %v", itemIdx, err))
	}
	if len(item.Tags) > 0 {
		if err := validateTags(item.Tags); err != nil {
			return huma.Error400BadRequest(fmt.Sprintf("invalid tags for item %d: %v", itemIdx, err))
		}
	}
	if err := validatePayloadAgainstSchema(item.Payload, job.PayloadSchema); err != nil {
		return huma.Error400BadRequest(fmt.Sprintf("payload validation failed for item %d: %v", itemIdx, err))
	}
	return nil
}

func bulkHasIdempotencyKey(items []BulkTriggerItem) bool {
	for _, item := range items {
		if item.IdempotencyKey != "" {
			return true
		}
	}
	return false
}
