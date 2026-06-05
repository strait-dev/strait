package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
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
	domain.WebhookEventRunCompleted:      true,
	domain.WebhookEventRunFailed:         true,
	domain.WebhookEventRunTimedOut:       true,
	domain.WebhookEventRunCanceled:       true,
	domain.WebhookEventWorkflowCompleted: true,
	domain.WebhookEventWorkflowFailed:    true,
	domain.WebhookEventSLOBudgetWarning:  true,

	domain.WebhookEventBillingCapWarning:            true,
	domain.WebhookEventBillingCapReached:            true,
	domain.WebhookEventBillingCapDisabled:           true,
	domain.WebhookEventBillingOverageDisabled:       true,
	domain.WebhookEventBillingSuspended:             true,
	domain.WebhookEventBillingDelinquent:            true,
	domain.WebhookEventBillingPaymentSucceeded:      true,
	domain.WebhookEventScheduleSuspended:            true,
	domain.WebhookEventWorkflowRegistrationRejected: true,
	domain.WebhookEventSLACreditIssued:              true,
}

type CreateWebhookSubscriptionRequest struct {
	ProjectID  string   `json:"project_id" validate:"required"`
	WebhookURL string   `json:"webhook_url" validate:"required"`
	EventTypes []string `json:"event_types" validate:"required,min=1"`
	Active     *bool    `json:"active,omitempty"`
}

type CreateWebhookSubscriptionInput struct {
	Body CreateWebhookSubscriptionRequest
}

// CreateWebhookSubscriptionResponse wraps the created subscription and returns
// the server-generated signing secret exactly once. Capture signing_secret
// from this response — it is encrypted at rest and never re-served.
type CreateWebhookSubscriptionResponse struct {
	Subscription  *domain.WebhookSubscription `json:"subscription"`
	SigningSecret string                      `json:"signing_secret"`
}

type CreateWebhookSubscriptionOutput struct {
	Body *CreateWebhookSubscriptionResponse
}

type webhookSubscriptionLimitCreator interface {
	CreateWebhookSubscriptionWithOrgLimit(ctx context.Context, sub *domain.WebhookSubscription, orgID string, maxEndpoints int) error
}

type webhookSubscriptionLimitsCreator interface {
	CreateWebhookSubscriptionWithLimits(ctx context.Context, sub *domain.WebhookSubscription, orgID string, maxEndpoints, maxProjectSubscriptions int) error
}

func (s *Server) handleCreateWebhookSubscription(ctx context.Context, input *CreateWebhookSubscriptionInput) (*CreateWebhookSubscriptionOutput, error) {
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}
	if err := requireProjectMatch(ctx, req.ProjectID); err != nil {
		return nil, huma.Error403Forbidden("project_id does not match authenticated project")
	}
	if err := requireProjectWideWebhookAccess(ctx); err != nil {
		return nil, err
	}
	s.emitInternalSecretBypassAuditIfProjectless(ctx, "create_webhook_subscription.project_match", "handleCreateWebhookSubscription", "project", req.ProjectID)
	orgID, maxEndpoints, _, err := s.resolveWebhookEndpointCreateLimit(ctx, req.ProjectID)
	if err != nil {
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
	requireTLS := s.config != nil && s.config.WebhookRequireTLS
	if err := validateURLWithTLS(req.WebhookURL, requireTLS); err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	active := true
	if req.Active != nil {
		active = *req.Active
	}

	// Server-generates the signing secret. Client-supplied values used to be
	// accepted here but were prone to weak inputs (no min length); rotation
	// already produced server-side secrets, so create now matches that path.
	b := make([]byte, 32)
	if _, randErr := rand.Read(b); randErr != nil {
		return nil, huma.Error500InternalServerError("failed to generate signing secret")
	}
	plaintextSecret := "whsec_" + hex.EncodeToString(b)

	if s.encryptor == nil {
		return nil, huma.Error500InternalServerError("webhook secret encryption is not configured")
	}
	enc, encErr := s.encryptor.Encrypt([]byte(plaintextSecret))
	if encErr != nil {
		return nil, huma.Error500InternalServerError("failed to encrypt webhook secret")
	}
	storedSecret := base64.StdEncoding.EncodeToString(enc)
	sub := &domain.WebhookSubscription{
		ProjectID:  req.ProjectID,
		WebhookURL: req.WebhookURL,
		EventTypes: req.EventTypes,
		Secret:     storedSecret,
		Active:     active,
	}
	if err := s.createWebhookSubscriptionWithLimit(ctx, sub, req.ProjectID, orgID, maxEndpoints); err != nil {
		return nil, err
	}
	s.emitAuditEvent(ctx, domain.AuditActionWebhookSubscriptionCreated, "webhook_subscription", sub.ID, map[string]any{
		"url_host":    urlHost(req.WebhookURL),
		"event_types": req.EventTypes,
		"active":      active,
	})
	return &CreateWebhookSubscriptionOutput{Body: &CreateWebhookSubscriptionResponse{
		Subscription:  sub,
		SigningSecret: plaintextSecret,
	}}, nil
}

func (s *Server) createWebhookSubscriptionWithLimit(ctx context.Context, sub *domain.WebhookSubscription, projectID, orgID string, maxEndpoints int) error {
	maxProjectSubscriptions, err := s.resolveWebhookProjectCreateLimit(ctx, projectID)
	if err != nil {
		return err
	}
	if orgID == "" || maxEndpoints < 0 {
		if maxProjectSubscriptions >= 0 {
			if limitedStore, ok := s.store.(webhookSubscriptionLimitsCreator); ok {
				if err := limitedStore.CreateWebhookSubscriptionWithLimits(ctx, sub, "", -1, maxProjectSubscriptions); err != nil {
					if errors.Is(err, store.ErrWebhookProjectLimitExceeded) {
						return huma.Error400BadRequest("webhook subscription limit exceeded")
					}
					if errors.Is(err, store.ErrWebhookSubscriptionDuplicate) {
						return huma.Error409Conflict("a webhook subscription for this URL already exists in this project")
					}
					return huma.Error500InternalServerError("failed to create webhook subscription")
				}
				return nil
			}
		}
		if err := s.checkWebhookProjectLimit(ctx, projectID, maxProjectSubscriptions); err != nil {
			return err
		}
		if err := s.store.CreateWebhookSubscription(ctx, sub); err != nil {
			if errors.Is(err, store.ErrWebhookSubscriptionDuplicate) {
				return huma.Error409Conflict("a webhook subscription for this URL already exists in this project")
			}
			return huma.Error500InternalServerError("failed to create webhook subscription")
		}
		return nil
	}

	if limitedStore, ok := s.store.(webhookSubscriptionLimitsCreator); ok {
		if err := limitedStore.CreateWebhookSubscriptionWithLimits(ctx, sub, orgID, maxEndpoints, maxProjectSubscriptions); err != nil {
			if errors.Is(err, store.ErrWebhookEndpointLimitExceeded) {
				return huma.Error400BadRequest("webhook endpoint limit exceeded")
			}
			if errors.Is(err, store.ErrWebhookProjectLimitExceeded) {
				return huma.Error400BadRequest("webhook subscription limit exceeded")
			}
			if errors.Is(err, store.ErrWebhookSubscriptionDuplicate) {
				return huma.Error409Conflict("a webhook subscription for this URL already exists in this project")
			}
			return huma.Error500InternalServerError("failed to create webhook subscription")
		}
		return nil
	}

	limitedStore, ok := s.store.(webhookSubscriptionLimitCreator)
	if !ok {
		if err := s.checkWebhookProjectLimit(ctx, projectID, maxProjectSubscriptions); err != nil {
			return err
		}
		if err := s.checkWebhookEndpointLimit(ctx, projectID); err != nil {
			return err
		}
		if err := s.store.CreateWebhookSubscription(ctx, sub); err != nil {
			if errors.Is(err, store.ErrWebhookSubscriptionDuplicate) {
				return huma.Error409Conflict("a webhook subscription for this URL already exists in this project")
			}
			return huma.Error500InternalServerError("failed to create webhook subscription")
		}
		return nil
	}

	if err := s.checkWebhookProjectLimit(ctx, projectID, maxProjectSubscriptions); err != nil {
		return err
	}
	if err := limitedStore.CreateWebhookSubscriptionWithOrgLimit(ctx, sub, orgID, maxEndpoints); err != nil {
		if errors.Is(err, store.ErrWebhookEndpointLimitExceeded) {
			return huma.Error400BadRequest("webhook endpoint limit exceeded")
		}
		if errors.Is(err, store.ErrWebhookSubscriptionDuplicate) {
			return huma.Error409Conflict("a webhook subscription for this URL already exists in this project")
		}
		return huma.Error500InternalServerError("failed to create webhook subscription")
	}
	return nil
}

type ListWebhookSubscriptionsInput struct{}

type ListWebhookSubscriptionsOutput struct {
	Body []domain.WebhookSubscription
}

func (s *Server) handleListWebhookSubscriptions(ctx context.Context, _ *ListWebhookSubscriptionsInput) (*ListWebhookSubscriptionsOutput, error) {
	if err := requireProjectWideWebhookAccess(ctx); err != nil {
		return nil, err
	}
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
	if err := requireProjectWideWebhookAccess(ctx); err != nil {
		return nil, err
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
	s.emitInternalSecretBypassAuditIfProjectless(ctx, "delete_webhook_subscription.project_match", "handleDeleteWebhookSubscription", "webhook_subscription", input.ID)

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
	if err := requireProjectWideWebhookAccess(ctx); err != nil {
		return nil, err
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
	s.emitInternalSecretBypassAuditIfProjectless(ctx, "rotate_webhook_secret.project_match", "handleRotateWebhookSecret", "webhook_subscription", input.ID)

	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, huma.Error500InternalServerError("failed to generate secret")
	}
	newSecret := "whsec_" + hex.EncodeToString(b)

	if s.encryptor == nil {
		return nil, huma.Error500InternalServerError("webhook secret encryption is not configured")
	}
	enc, encErr := s.encryptor.Encrypt([]byte(newSecret))
	if encErr != nil {
		return nil, huma.Error500InternalServerError("failed to encrypt webhook secret")
	}
	secretToStore := base64.StdEncoding.EncodeToString(enc)

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

func requireProjectWideWebhookAccess(ctx context.Context) error {
	if environmentIDFromContext(ctx) != "" {
		return huma.Error403Forbidden("webhook subscriptions require a project-wide key")
	}
	return nil
}
