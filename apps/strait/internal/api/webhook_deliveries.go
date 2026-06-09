package api

import (
	"context"
	"errors"
	"strings"
	"time"

	"strait/internal/domain"
	"strait/internal/httputil"

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
	var deliveries []domain.WebhookDelivery
	if environmentIDFromContext(ctx) != "" {
		deliveries, err = s.listWebhookDeliveriesForEnvironment(ctx, projectID, status, limit+1, cursor)
	} else {
		deliveries, err = s.store.ListWebhookDeliveries(ctx, projectID, status, limit+1, cursor)
	}
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list webhook deliveries")
	}
	deliveries = sanitizeWebhookDeliveryResponses(deliveries)
	return &ListWebhookDeliveriesOutput{Body: paginatedResult(deliveries, limit, func(d domain.WebhookDelivery) string {
		return d.CreatedAt.Format(time.RFC3339Nano)
	})}, nil
}

func (s *Server) listWebhookDeliveriesForEnvironment(ctx context.Context, projectID, status string, limit int, cursor *time.Time) ([]domain.WebhookDelivery, error) {
	if limit <= 0 {
		return nil, nil
	}

	deliveries := make([]domain.WebhookDelivery, 0, limit)
	fetchLimit := max(limit, 25)
	nextCursor := cursor
	for len(deliveries) < limit {
		batch, err := s.store.ListWebhookDeliveries(ctx, projectID, status, fetchLimit, nextCursor)
		if err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			break
		}
		for i := range batch {
			delivery := batch[i]
			if accessErr := s.verifyDeliveryProjectAccess(ctx, &delivery); accessErr != nil {
				if errors.Is(accessErr, errProjectMismatch) || errors.Is(accessErr, errEnvironmentMismatch) {
					continue
				}
				return nil, accessErr
			}
			deliveries = append(deliveries, delivery)
			if len(deliveries) >= limit {
				break
			}
		}
		last := batch[len(batch)-1].CreatedAt
		nextCursor = &last
		if len(batch) < fetchLimit {
			break
		}
	}
	return deliveries, nil
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
	delivery, err := s.store.GetWebhookDelivery(ctx, projectIDFromContext(ctx), deliveryID)
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
	return &GetWebhookDeliveryOutput{Body: sanitizeWebhookDeliveryResponsePtr(delivery)}, nil
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
	d, err := s.store.GetWebhookDelivery(ctx, projectIDFromContext(ctx), deliveryID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, huma.Error404NotFound("delivery not found")
		}
		return nil, huma.Error500InternalServerError("failed to get delivery")
	}
	if d == nil {
		return nil, huma.Error404NotFound("delivery not found")
	}
	if err := s.verifyDeliveryProjectAccess(ctx, d); err != nil {
		return nil, huma.Error404NotFound("delivery not found")
	}
	if !isRetriableWebhookDeliveryStatus(d.Status) {
		return nil, huma.Error409Conflict("only failed or dead deliveries can be retried")
	}
	retried, err := s.store.RetryWebhookDelivery(ctx, deliveryID)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "not found") {
			return nil, huma.Error404NotFound("delivery not found")
		}
		if strings.Contains(errMsg, "not retriable") {
			return nil, huma.Error409Conflict("only failed or dead deliveries can be retried")
		}
		return nil, huma.Error500InternalServerError("failed to retry delivery")
	}
	s.emitAuditEvent(ctx, domain.AuditActionWebhookDeliveryRetried, "webhook_delivery", deliveryID, map[string]any{
		"subscription_id": d.SubscriptionID,
		"previous_status": d.Status,
	})
	return &RetryWebhookDeliveryOutput{Body: sanitizeWebhookDeliveryResponsePtr(retried)}, nil
}

func sanitizeWebhookDeliveryResponses(deliveries []domain.WebhookDelivery) []domain.WebhookDelivery {
	for i := range deliveries {
		deliveries[i] = sanitizeWebhookDeliveryResponse(deliveries[i])
	}
	return deliveries
}

func sanitizeWebhookDeliveryResponsePtr(delivery *domain.WebhookDelivery) *domain.WebhookDelivery {
	if delivery == nil {
		return nil
	}
	sanitized := sanitizeWebhookDeliveryResponse(*delivery)
	return &sanitized
}

func sanitizeWebhookDeliveryResponse(delivery domain.WebhookDelivery) domain.WebhookDelivery {
	delivery.WebhookURL = httputil.RedactURLForLog(delivery.WebhookURL)
	return delivery
}

func isRetriableWebhookDeliveryStatus(status string) bool {
	switch status {
	case domain.WebhookStatusFailed, domain.WebhookStatusDead:
		return true
	default:
		return false
	}
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
		if environmentIDFromContext(ctx) != "" {
			return errEnvironmentMismatch
		}
		if d.ProjectID != "" && d.ProjectID != projectID {
			return errProjectMismatch
		}
		return nil // no job association to verify
	}
	job, err := s.store.GetJob(ctx, d.JobID)
	if err != nil || job == nil {
		return errProjectMismatch
	}
	if err := requireProjectMatch(ctx, job.ProjectID); err != nil {
		return err
	}
	return requireEnvironmentMatch(ctx, job.EnvironmentID)
}
