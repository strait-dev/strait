package api

import (
	"context"
	"errors"
	"fmt"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/danielgtaylor/huma/v2"
)

// validWebhookEventTypes is the allowed set of event types for webhook subscriptions.
var validWebhookEventTypes = map[string]bool{
	domain.WebhookEventRunCompleted:         true,
	domain.WebhookEventRunFailed:            true,
	domain.WebhookEventRunTimedOut:          true,
	domain.WebhookEventRunCanceled:          true,
	domain.WebhookEventWorkflowCompleted:    true,
	domain.WebhookEventWorkflowFailed:       true,
	domain.WebhookEventComputeBudgetWarning: true,
	domain.WebhookEventSLOBudgetWarning:     true,
}

type CreateWebhookSubscriptionRequest struct {
	ProjectID  string   `json:"project_id" validate:"required"`
	WebhookURL string   `json:"webhook_url" validate:"required"`
	EventTypes []string `json:"event_types" validate:"required,min=1"`
	Secret     string   `json:"secret" validate:"required"`
	Active     *bool    `json:"active,omitempty"`
}

type CreateWebhookSubscriptionInput struct {
	Body CreateWebhookSubscriptionRequest
}

type CreateWebhookSubscriptionOutput struct {
	Body *domain.WebhookSubscription
}

func (s *Server) handleCreateWebhookSubscription(ctx context.Context, input *CreateWebhookSubscriptionInput) (*CreateWebhookSubscriptionOutput, error) {
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}
	for _, et := range req.EventTypes {
		if !validWebhookEventTypes[et] {
			return nil, huma.Error400BadRequest(fmt.Sprintf("invalid event type: %q", et))
		}
	}
	if err := validateURL(req.WebhookURL); err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	active := true
	if req.Active != nil {
		active = *req.Active
	}
	sub := &domain.WebhookSubscription{
		ProjectID:  req.ProjectID,
		WebhookURL: req.WebhookURL,
		EventTypes: req.EventTypes,
		Secret:     req.Secret,
		Active:     active,
	}
	if err := s.store.CreateWebhookSubscription(ctx, sub); err != nil {
		return nil, huma.Error500InternalServerError("failed to create webhook subscription")
	}
	return &CreateWebhookSubscriptionOutput{Body: sub}, nil
}

type ListWebhookSubscriptionsInput struct{}

type ListWebhookSubscriptionsOutput struct {
	Body []domain.WebhookSubscription
}

func (s *Server) handleListWebhookSubscriptions(ctx context.Context, _ *ListWebhookSubscriptionsInput) (*ListWebhookSubscriptionsOutput, error) {
	projectID := projectIDFromContext(ctx)
	subs, err := s.store.ListWebhookSubscriptions(ctx, projectID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list webhook subscriptions")
	}
	return &ListWebhookSubscriptionsOutput{Body: subs}, nil
}

type DeleteWebhookSubscriptionInput struct {
	ID string `path:"id"`
}

func (s *Server) handleDeleteWebhookSubscription(ctx context.Context, input *DeleteWebhookSubscriptionInput) (*struct{}, error) {
	if input.ID == "" {
		return nil, huma.Error400BadRequest("subscription id is required")
	}

	sub, err := s.store.GetWebhookSubscription(ctx, input.ID)
	if err != nil {
		if errors.Is(err, store.ErrWebhookSubscriptionNotFound) {
			return nil, huma.Error404NotFound("webhook subscription not found")
		}
		return nil, huma.Error500InternalServerError("failed to get webhook subscription")
	}

	if err := requireProjectMatch(ctx, sub.ProjectID); err != nil {
		return nil, huma.Error404NotFound("webhook subscription not found")
	}

	err = s.store.DeleteWebhookSubscription(ctx, input.ID)
	if err != nil {
		if errors.Is(err, store.ErrWebhookSubscriptionNotFound) {
			return nil, huma.Error404NotFound("webhook subscription not found")
		}
		return nil, huma.Error500InternalServerError("failed to delete webhook subscription")
	}
	return nil, nil
}
