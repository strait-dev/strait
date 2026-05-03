package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

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
	domain.WebhookEventSLOBudgetWarning: true,
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
	if err := requireProjectMatch(ctx, req.ProjectID); err != nil {
		return nil, huma.Error403Forbidden("project_id does not match authenticated project")
	}
	if err := s.checkWebhookEndpointLimit(ctx, req.ProjectID); err != nil {
		return nil, err
	}
	for _, et := range req.EventTypes {
		if !validWebhookEventTypes[et] {
			return nil, huma.Error400BadRequest(fmt.Sprintf("invalid event type: %q", et))
		}
	}
	if err := s.checkWebhookEventTypes(ctx, req.ProjectID, req.EventTypes); err != nil {
		return nil, err
	}
	if err := validateURL(req.WebhookURL); err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	active := true
	if req.Active != nil {
		active = *req.Active
	}
	secret := req.Secret
	if s.encryptor != nil {
		enc, encErr := s.encryptor.Encrypt([]byte(secret))
		if encErr != nil {
			return nil, huma.Error500InternalServerError("failed to encrypt webhook secret")
		}
		secret = string(enc)
	}
	sub := &domain.WebhookSubscription{
		ProjectID:  req.ProjectID,
		WebhookURL: req.WebhookURL,
		EventTypes: req.EventTypes,
		Secret:     secret,
		Active:     active,
	}
	if err := s.store.CreateWebhookSubscription(ctx, sub); err != nil {
		return nil, huma.Error500InternalServerError("failed to create webhook subscription")
	}
	s.emitAuditEvent(ctx, domain.AuditActionWebhookSubscriptionCreated, "webhook_subscription", sub.ID, map[string]any{
		"url_host":    urlHost(req.WebhookURL),
		"event_types": req.EventTypes,
		"active":      active,
	})
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
	s.emitAuditEvent(ctx, domain.AuditActionWebhookSubscriptionDeleted, "webhook_subscription", input.ID, map[string]any{
		"url_host":    urlHost(sub.WebhookURL),
		"event_types": sub.EventTypes,
	})
	return nil, nil
}

type RotateWebhookSecretRequest struct {
	GracePeriodMinutes int `json:"grace_period_minutes,omitempty"`
}

type RotateWebhookSecretInput struct {
	ID   string `path:"id"`
	Body RotateWebhookSecretRequest
}

type RotateWebhookSecretOutput struct {
	Body any
}

func (s *Server) handleRotateWebhookSecret(ctx context.Context, input *RotateWebhookSecretInput) (*RotateWebhookSecretOutput, error) {
	graceMins := input.Body.GracePeriodMinutes
	if graceMins <= 0 {
		graceMins = 60
	}
	if graceMins > 10080 { // 7 days
		return nil, huma.Error400BadRequest("grace_period_minutes must be <= 10080")
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

	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, huma.Error500InternalServerError("failed to generate secret")
	}
	newSecret := "whsec_" + hex.EncodeToString(b)

	secretToStore := newSecret
	if s.encryptor != nil {
		enc, encErr := s.encryptor.Encrypt([]byte(newSecret))
		if encErr != nil {
			return nil, huma.Error500InternalServerError("failed to encrypt webhook secret")
		}
		secretToStore = string(enc)
	}

	graceExpiresAt := time.Now().Add(time.Duration(graceMins) * time.Minute)
	if err := s.store.RotateWebhookSecret(ctx, input.ID, secretToStore, graceExpiresAt); err != nil {
		if errors.Is(err, store.ErrWebhookSubscriptionNotFound) {
			return nil, huma.Error404NotFound("webhook subscription not found")
		}
		return nil, huma.Error500InternalServerError("failed to rotate webhook secret")
	}

	s.emitAuditEvent(ctx, domain.AuditActionWebhookSubscriptionRotateSecret, "webhook_subscription", input.ID, map[string]any{
		"grace_expires_at":     graceExpiresAt,
		"grace_period_minutes": graceMins,
	})

	return &RotateWebhookSecretOutput{Body: map[string]any{
		"subscription_id":      input.ID,
		"new_secret":           newSecret,
		"grace_expires_at":     graceExpiresAt,
		"grace_period_minutes": graceMins,
	}}, nil
}
