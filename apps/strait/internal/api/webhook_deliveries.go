package api

import (
	"context"
	"strings"
	"time"

	"strait/internal/domain"

	"github.com/danielgtaylor/huma/v2"
)

type ListWebhookDeliveriesInput struct {
	Status string `query:"status"`
	Limit  string `query:"limit"`
	Cursor string `query:"cursor"`
}

type ListWebhookDeliveriesOutput struct {
	Body PaginatedResponse
}

func (s *Server) handleListWebhookDeliveries(ctx context.Context, input *ListWebhookDeliveriesInput) (*ListWebhookDeliveriesOutput, error) {
	projectID := projectIDFromContext(ctx)
	status := input.Status
	if status != "" {
		switch status {
		case domain.WebhookStatusPending, domain.WebhookStatusDelivered, domain.WebhookStatusFailed, domain.WebhookStatusDead:
		default:
			return nil, huma.Error400BadRequest("status is invalid")
		}
	}
	limit, cursor, err := parsePaginationFromStrings(input.Limit, input.Cursor)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	deliveries, err := s.store.ListWebhookDeliveries(ctx, projectID, status, limit+1, cursor)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list webhook deliveries")
	}
	return &ListWebhookDeliveriesOutput{Body: paginatedResult(deliveries, limit, func(d domain.WebhookDelivery) string {
		return d.CreatedAt.Format(time.RFC3339Nano)
	})}, nil
}

type GetWebhookDeliveryInput struct {
	ID string `path:"id"`
}

type GetWebhookDeliveryOutput struct {
	Body *domain.WebhookDelivery
}

func (s *Server) handleGetWebhookDelivery(ctx context.Context, input *GetWebhookDeliveryInput) (*GetWebhookDeliveryOutput, error) {
	deliveryID := input.ID
	if deliveryID == "" {
		return nil, huma.Error400BadRequest("delivery ID is required")
	}
	delivery, err := s.store.GetWebhookDelivery(ctx, deliveryID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, huma.Error404NotFound("delivery not found")
		}
		return nil, huma.Error500InternalServerError("failed to get delivery")
	}
	if delivery == nil {
		return nil, huma.Error404NotFound("delivery not found")
	}
	if err := s.verifyDeliveryProjectAccess(ctx, delivery); err != nil {
		return nil, huma.Error404NotFound("delivery not found")
	}
	return &GetWebhookDeliveryOutput{Body: delivery}, nil
}

type RetryWebhookDeliveryInput struct {
	DeliveryID string `path:"deliveryID"`
	ID         string `path:"id"`
}

type RetryWebhookDeliveryOutput struct {
	Body *domain.WebhookDelivery
}

func (s *Server) handleRetryWebhookDelivery(ctx context.Context, input *RetryWebhookDeliveryInput) (*RetryWebhookDeliveryOutput, error) {
	deliveryID := input.DeliveryID
	if deliveryID == "" {
		deliveryID = input.ID
	}
	if deliveryID == "" {
		return nil, huma.Error400BadRequest("delivery ID is required")
	}
	d, err := s.store.GetWebhookDelivery(ctx, deliveryID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get delivery")
	}
	if d == nil {
		return nil, huma.Error404NotFound("delivery not found")
	}
	if err := s.verifyDeliveryProjectAccess(ctx, d); err != nil {
		return nil, huma.Error404NotFound("delivery not found")
	}
	if d.Status != domain.WebhookStatusFailed && d.Status != domain.WebhookStatusDead {
		return nil, huma.Error409Conflict("only failed or dead deliveries can be retried")
	}
	retried, err := s.store.RetryWebhookDelivery(ctx, deliveryID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to retry delivery")
	}
	s.emitAuditEvent(ctx, domain.AuditActionWebhookDeliveryRetried, "webhook_delivery", deliveryID, map[string]any{
		"subscription_id": d.SubscriptionID,
		"previous_status": string(d.Status),
	})
	return &RetryWebhookDeliveryOutput{Body: retried}, nil
}

// verifyDeliveryProjectAccess checks that the webhook delivery belongs to the
// caller's project by looking up the associated job. Returns nil if there is no
// project in context (internal caller) or the delivery has no job (e.g. event
// trigger delivery). Returns errProjectMismatch when the job belongs to a
// different project.
func (s *Server) verifyDeliveryProjectAccess(ctx context.Context, d *domain.WebhookDelivery) error {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil // internal caller without project context
	}
	if d.JobID == "" {
		return nil // no job association to verify
	}
	job, err := s.store.GetJob(ctx, d.JobID)
	if err != nil || job == nil {
		return errProjectMismatch
	}
	return requireProjectMatch(ctx, job.ProjectID)
}
