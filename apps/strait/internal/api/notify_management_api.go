package api

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"strait/internal/domain"
	notifycore "strait/internal/notify"
	"strait/internal/store"

	"github.com/danielgtaylor/huma/v2"
)

// Topics API.
type CreateNotifyTopicRequest struct {
	TopicKey    string          `json:"topic_key" validate:"required"`
	Name        string          `json:"name" validate:"required"`
	Description string          `json:"description,omitempty"`
	Attributes  json.RawMessage `json:"attributes,omitempty"`
}

type CreateNotifyTopicInput struct {
	Body CreateNotifyTopicRequest
}

type CreateNotifyTopicOutput struct {
	Body *domain.NotifyTopic
}

func (s *Server) handleCreateNotifyTopic(ctx context.Context, input *CreateNotifyTopicInput) (*CreateNotifyTopicOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}
	ns, err := s.requireNotifyStore()
	if err != nil {
		return nil, err
	}
	topic := &domain.NotifyTopic{
		ProjectID:   projectID,
		TopicKey:    req.TopicKey,
		Name:        req.Name,
		Description: req.Description,
		Attributes:  req.Attributes,
	}
	if err := ns.CreateNotifyTopic(ctx, topic); err != nil {
		return nil, huma.Error500InternalServerError("failed to create topic")
	}
	return &CreateNotifyTopicOutput{Body: topic}, nil
}

type ListNotifyTopicsInput struct{}

type ListNotifyTopicsOutput struct {
	Body []domain.NotifyTopic
}

func (s *Server) handleListNotifyTopics(ctx context.Context, _ *ListNotifyTopicsInput) (*ListNotifyTopicsOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	ns, err := s.requireNotifyStore()
	if err != nil {
		return nil, err
	}
	topics, err := ns.ListNotifyTopics(ctx, projectID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list topics")
	}
	return &ListNotifyTopicsOutput{Body: topics}, nil
}

type AddNotifyTopicSubscriberRequest struct {
	SubscriberID string `json:"subscriber_id" validate:"required"`
}

type AddNotifyTopicSubscriberInput struct {
	TopicKey string `path:"topicKey"`
	Body     AddNotifyTopicSubscriberRequest
}

func (s *Server) handleAddNotifyTopicSubscriber(ctx context.Context, input *AddNotifyTopicSubscriberInput) (*struct{}, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	ns, err := s.requireNotifyStore()
	if err != nil {
		return nil, err
	}
	topic, err := ns.GetNotifyTopicByKey(ctx, projectID, input.TopicKey)
	if err != nil {
		if errors.Is(err, store.ErrNotifyTopicNotFound) {
			return nil, huma.Error404NotFound("topic not found")
		}
		return nil, huma.Error500InternalServerError("failed to get topic")
	}
	if err := ns.AddNotifyTopicSubscriber(ctx, topic.ID, input.Body.SubscriberID); err != nil {
		return nil, huma.Error500InternalServerError("failed to add topic member")
	}
	return nil, nil
}

type RemoveNotifyTopicSubscriberInput struct {
	TopicKey     string `path:"topicKey"`
	SubscriberID string `path:"subscriberID"`
}

func (s *Server) handleRemoveNotifyTopicSubscriber(ctx context.Context, input *RemoveNotifyTopicSubscriberInput) (*struct{}, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	ns, err := s.requireNotifyStore()
	if err != nil {
		return nil, err
	}
	topic, err := ns.GetNotifyTopicByKey(ctx, projectID, input.TopicKey)
	if err != nil {
		if errors.Is(err, store.ErrNotifyTopicNotFound) {
			return nil, huma.Error404NotFound("topic not found")
		}
		return nil, huma.Error500InternalServerError("failed to get topic")
	}
	if err := ns.RemoveNotifyTopicSubscriber(ctx, topic.ID, input.SubscriberID); err != nil {
		return nil, huma.Error500InternalServerError("failed to remove topic member")
	}
	return nil, nil
}

// Templates API.
type CreateNotificationTemplateRequest struct {
	TemplateKey     string          `json:"template_key" validate:"required"`
	Name            string          `json:"name" validate:"required"`
	Description     string          `json:"description,omitempty"`
	Channels        json.RawMessage `json:"channels" validate:"required"`
	Variables       json.RawMessage `json:"variables,omitempty"`
	LocaleTemplates json.RawMessage `json:"locale_templates,omitempty"`
	DefaultLocale   string          `json:"default_locale,omitempty"`
	Status          string          `json:"status,omitempty"`
}

type CreateNotificationTemplateInput struct {
	Body CreateNotificationTemplateRequest
}

type CreateNotificationTemplateOutput struct {
	Body *domain.NotificationTemplate
}

func (s *Server) handleCreateNotificationTemplate(ctx context.Context, input *CreateNotificationTemplateInput) (*CreateNotificationTemplateOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}
	ns, err := s.requireNotifyStore()
	if err != nil {
		return nil, err
	}
	tmpl := &domain.NotificationTemplate{
		ProjectID:       projectID,
		TemplateKey:     req.TemplateKey,
		Name:            req.Name,
		Description:     req.Description,
		Version:         1,
		Channels:        req.Channels,
		Variables:       req.Variables,
		LocaleTemplates: req.LocaleTemplates,
		DefaultLocale:   req.DefaultLocale,
		Status:          req.Status,
	}
	if err := ns.CreateNotificationTemplate(ctx, tmpl); err != nil {
		return nil, huma.Error500InternalServerError("failed to create template")
	}
	return &CreateNotificationTemplateOutput{Body: tmpl}, nil
}

type ListNotificationTemplatesInput struct {
	Status string `query:"status"`
	Limit  string `query:"limit"`
	Cursor string `query:"cursor"`
}

type ListNotificationTemplatesOutput struct {
	Body []domain.NotificationTemplate
}

func (s *Server) handleListNotificationTemplates(ctx context.Context, input *ListNotificationTemplatesInput) (*ListNotificationTemplatesOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	ns, err := s.requireNotifyStore()
	if err != nil {
		return nil, err
	}
	limit := defaultPageLimit
	if input.Limit != "" {
		if parsed, parseErr := strconv.Atoi(input.Limit); parseErr == nil && parsed > 0 && parsed <= maxPageLimit {
			limit = parsed
		}
	}
	var status *string
	if input.Status != "" {
		status = &input.Status
	}
	var cursor *time.Time
	if input.Cursor != "" {
		if ts, parseErr := time.Parse(time.RFC3339Nano, input.Cursor); parseErr == nil {
			cursor = &ts
		}
	}
	templates, err := ns.ListNotificationTemplates(ctx, projectID, status, limit, cursor)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list templates")
	}
	return &ListNotificationTemplatesOutput{Body: templates}, nil
}

type GetNotificationTemplateInput struct {
	TemplateKey string `path:"templateKey"`
}

type GetNotificationTemplateOutput struct {
	Body *domain.NotificationTemplate
}

func (s *Server) handleGetNotificationTemplate(ctx context.Context, input *GetNotificationTemplateInput) (*GetNotificationTemplateOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	ns, err := s.requireNotifyStore()
	if err != nil {
		return nil, err
	}
	tmpl, err := ns.GetLatestNotificationTemplateByKey(ctx, projectID, input.TemplateKey)
	if err != nil {
		if errors.Is(err, store.ErrNotificationTemplateNotFound) {
			return nil, huma.Error404NotFound("template not found")
		}
		return nil, huma.Error500InternalServerError("failed to get template")
	}
	return &GetNotificationTemplateOutput{Body: tmpl}, nil
}

type UpdateNotificationTemplateInput struct {
	TemplateKey string `path:"templateKey"`
	Body        CreateNotificationTemplateRequest
}

type UpdateNotificationTemplateOutput struct {
	Body *domain.NotificationTemplate
}

func (s *Server) handleUpdateNotificationTemplate(ctx context.Context, input *UpdateNotificationTemplateInput) (*UpdateNotificationTemplateOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	ns, err := s.requireNotifyStore()
	if err != nil {
		return nil, err
	}
	latest, err := ns.GetLatestNotificationTemplateByKey(ctx, projectID, input.TemplateKey)
	if err != nil {
		if errors.Is(err, store.ErrNotificationTemplateNotFound) {
			return nil, huma.Error404NotFound("template not found")
		}
		return nil, huma.Error500InternalServerError("failed to get template")
	}
	req := input.Body
	next := &domain.NotificationTemplate{
		ProjectID:       projectID,
		TemplateKey:     latest.TemplateKey,
		Name:            coalesceString(req.Name, latest.Name),
		Description:     coalesceString(req.Description, latest.Description),
		Version:         latest.Version + 1,
		Channels:        coalesceRaw(req.Channels, latest.Channels),
		Variables:       coalesceRaw(req.Variables, latest.Variables),
		LocaleTemplates: coalesceRaw(req.LocaleTemplates, latest.LocaleTemplates),
		DefaultLocale:   coalesceString(req.DefaultLocale, latest.DefaultLocale),
		Status:          coalesceString(req.Status, latest.Status),
	}
	if err := ns.CreateNotificationTemplate(ctx, next); err != nil {
		return nil, huma.Error500InternalServerError("failed to update template")
	}
	return &UpdateNotificationTemplateOutput{Body: next}, nil
}

func coalesceString(v, fallback string) string {
	if v != "" {
		return v
	}
	return fallback
}

func coalesceRaw(v, fallback json.RawMessage) json.RawMessage {
	if len(v) > 0 {
		return v
	}
	return fallback
}

// Template preview.
type NotifyPreviewRequest struct {
	TemplateKey   string          `json:"template_key" validate:"required"`
	Payload       json.RawMessage `json:"payload,omitempty"`
	SubscriberID  string          `json:"subscriber_id,omitempty"`
	Locale        string          `json:"locale,omitempty"`
	Channels      []string        `json:"channels,omitempty"`
	CategoryKey   string          `json:"category_key,omitempty"`
	UnsubscribeTo string          `json:"unsubscribe_scope,omitempty"`
}

type NotifyPreviewInput struct {
	Body NotifyPreviewRequest
}

type NotifyPreviewOutput struct {
	Body map[string]any
}

func (s *Server) handleNotifyPreview(ctx context.Context, input *NotifyPreviewInput) (*NotifyPreviewOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}
	ns, err := s.requireNotifyStore()
	if err != nil {
		return nil, err
	}
	tmpl, err := ns.GetLatestNotificationTemplateByKey(ctx, projectID, req.TemplateKey)
	if err != nil {
		if errors.Is(err, store.ErrNotificationTemplateNotFound) {
			return nil, huma.Error404NotFound("template not found")
		}
		return nil, huma.Error500InternalServerError("failed to resolve template")
	}

	payload := map[string]any{}
	if len(req.Payload) > 0 {
		if err := json.Unmarshal(req.Payload, &payload); err != nil {
			return nil, huma.Error400BadRequest("payload must be valid JSON")
		}
	}

	sub := domain.NotifySubscriber{Locale: req.Locale}
	if req.SubscriberID != "" {
		resolved, err := ns.GetNotifySubscriber(ctx, req.SubscriberID, projectID)
		if err != nil {
			if errors.Is(err, store.ErrNotifySubscriberNotFound) {
				return nil, huma.Error404NotFound("subscriber not found")
			}
			return nil, huma.Error500InternalServerError("failed to resolve subscriber")
		}
		sub = *resolved
	}
	system := map[string]any{
		"preferences_url": s.absoluteNotifyURL("/v1/preferences"),
		"unsubscribe_url": s.absoluteNotifyURL("/v1/unsubscribe/preview"),
	}
	ctxMap := buildNotifyRenderContext(payload, &sub, system)
	rendered, err := notifycore.RenderTemplate(tmpl, req.Locale, ctxMap)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to render template")
	}

	if len(req.Channels) == 0 {
		return &NotifyPreviewOutput{Body: rendered.Channels}, nil
	}
	filtered := make(map[string]any, len(req.Channels))
	for _, ch := range req.Channels {
		if v, ok := rendered.Channels[ch]; ok {
			filtered[ch] = v
		}
	}
	return &NotifyPreviewOutput{Body: filtered}, nil
}

// Categories API.
type CreateNotificationCategoryRequest struct {
	CategoryKey string `json:"category_key" validate:"required"`
	Name        string `json:"name" validate:"required"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type,omitempty"`
}

type CreateNotificationCategoryInput struct {
	Body CreateNotificationCategoryRequest
}

type CreateNotificationCategoryOutput struct {
	Body *domain.NotificationCategory
}

func (s *Server) handleCreateNotificationCategory(ctx context.Context, input *CreateNotificationCategoryInput) (*CreateNotificationCategoryOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}
	ns, err := s.requireNotifyStore()
	if err != nil {
		return nil, err
	}
	cat := &domain.NotificationCategory{
		ProjectID:   projectID,
		CategoryKey: req.CategoryKey,
		Name:        req.Name,
		Description: req.Description,
		Type:        req.Type,
	}
	if err := ns.CreateNotificationCategory(ctx, cat); err != nil {
		return nil, huma.Error500InternalServerError("failed to create category")
	}
	return &CreateNotificationCategoryOutput{Body: cat}, nil
}

type ListNotificationCategoriesInput struct{}

type ListNotificationCategoriesOutput struct {
	Body []domain.NotificationCategory
}

func (s *Server) handleListNotificationCategories(ctx context.Context, _ *ListNotificationCategoriesInput) (*ListNotificationCategoriesOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	ns, err := s.requireNotifyStore()
	if err != nil {
		return nil, err
	}
	categories, err := ns.ListNotificationCategories(ctx, projectID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list categories")
	}
	return &ListNotificationCategoriesOutput{Body: categories}, nil
}

type CreateNotifyPolicyOverrideRequest struct {
	ScopeType                 string `json:"scope_type" validate:"required,oneof=project category workflow_step"`
	ScopeKey                  string `json:"scope_key" validate:"required"`
	Channel                   string `json:"channel,omitempty"`
	DigestPolicy              string `json:"digest_policy,omitempty" validate:"omitempty,oneof=instant hourly daily"`
	RetryMaxAttempts          *int   `json:"retry_max_attempts,omitempty"`
	RetryBaseDelaySecs        *int   `json:"retry_base_delay_secs,omitempty"`
	RetryMaxDelaySecs         *int   `json:"retry_max_delay_secs,omitempty"`
	EscalationTiers           *int   `json:"escalation_tiers,omitempty"`
	EscalationMinIntervalSecs *int   `json:"escalation_min_interval_secs,omitempty"`
	Enabled                   *bool  `json:"enabled,omitempty"`
}

type CreateNotifyPolicyOverrideInput struct {
	Body CreateNotifyPolicyOverrideRequest
}

type CreateNotifyPolicyOverrideOutput struct {
	Body *domain.NotifyPolicyOverride
}

func (s *Server) handleCreateNotifyPolicyOverride(ctx context.Context, input *CreateNotifyPolicyOverrideInput) (*CreateNotifyPolicyOverrideOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}

	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	ns, err := s.requireNotifyStore()
	if err != nil {
		return nil, err
	}

	policy := &domain.NotifyPolicyOverride{
		ProjectID:                 projectID,
		ScopeType:                 req.ScopeType,
		ScopeKey:                  req.ScopeKey,
		Channel:                   strings.TrimSpace(strings.ToLower(req.Channel)),
		DigestPolicy:              strings.TrimSpace(strings.ToLower(req.DigestPolicy)),
		RetryMaxAttempts:          req.RetryMaxAttempts,
		RetryBaseDelaySecs:        req.RetryBaseDelaySecs,
		RetryMaxDelaySecs:         req.RetryMaxDelaySecs,
		EscalationTiers:           req.EscalationTiers,
		EscalationMinIntervalSecs: req.EscalationMinIntervalSecs,
		Enabled:                   enabled,
	}

	if err := ns.UpsertNotifyPolicyOverride(ctx, policy); err != nil {
		return nil, huma.Error500InternalServerError("failed to create notify policy override")
	}

	return &CreateNotifyPolicyOverrideOutput{Body: policy}, nil
}

type UpdateNotifyPolicyOverrideRequest struct {
	DigestPolicy              *string `json:"digest_policy,omitempty" validate:"omitempty,oneof=instant hourly daily"`
	RetryMaxAttempts          *int    `json:"retry_max_attempts,omitempty"`
	RetryBaseDelaySecs        *int    `json:"retry_base_delay_secs,omitempty"`
	RetryMaxDelaySecs         *int    `json:"retry_max_delay_secs,omitempty"`
	EscalationTiers           *int    `json:"escalation_tiers,omitempty"`
	EscalationMinIntervalSecs *int    `json:"escalation_min_interval_secs,omitempty"`
	Enabled                   *bool   `json:"enabled,omitempty"`
}

type UpdateNotifyPolicyOverrideInput struct {
	PolicyID string `path:"policyID"`
	Body     UpdateNotifyPolicyOverrideRequest
}

type UpdateNotifyPolicyOverrideOutput struct {
	Body *domain.NotifyPolicyOverride
}

func (s *Server) handleUpdateNotifyPolicyOverride(ctx context.Context, input *UpdateNotifyPolicyOverrideInput) (*UpdateNotifyPolicyOverrideOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}

	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}

	ns, err := s.requireNotifyStore()
	if err != nil {
		return nil, err
	}

	policy, err := ns.GetNotifyPolicyOverride(ctx, input.PolicyID, projectID)
	if err != nil {
		if errors.Is(err, store.ErrNotifyPolicyNotFound) {
			return nil, huma.Error404NotFound("notify policy override not found")
		}
		return nil, huma.Error500InternalServerError("failed to get notify policy override")
	}

	if req.DigestPolicy != nil {
		policy.DigestPolicy = strings.TrimSpace(strings.ToLower(*req.DigestPolicy))
	}
	if req.RetryMaxAttempts != nil {
		policy.RetryMaxAttempts = req.RetryMaxAttempts
	}
	if req.RetryBaseDelaySecs != nil {
		policy.RetryBaseDelaySecs = req.RetryBaseDelaySecs
	}
	if req.RetryMaxDelaySecs != nil {
		policy.RetryMaxDelaySecs = req.RetryMaxDelaySecs
	}
	if req.EscalationTiers != nil {
		policy.EscalationTiers = req.EscalationTiers
	}
	if req.EscalationMinIntervalSecs != nil {
		policy.EscalationMinIntervalSecs = req.EscalationMinIntervalSecs
	}
	if req.Enabled != nil {
		policy.Enabled = *req.Enabled
	}

	if err := ns.UpsertNotifyPolicyOverride(ctx, policy); err != nil {
		return nil, huma.Error500InternalServerError("failed to update notify policy override")
	}

	return &UpdateNotifyPolicyOverrideOutput{Body: policy}, nil
}

type ListNotifyPolicyOverridesInput struct {
	ScopeType string `query:"scope_type" validate:"omitempty,oneof=project category workflow_step"`
}

type ListNotifyPolicyOverridesOutput struct {
	Body []domain.NotifyPolicyOverride
}

func (s *Server) handleListNotifyPolicyOverrides(ctx context.Context, input *ListNotifyPolicyOverridesInput) (*ListNotifyPolicyOverridesOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	if err := s.validate.Struct(input); err != nil {
		return nil, newValidationError(err)
	}

	ns, err := s.requireNotifyStore()
	if err != nil {
		return nil, err
	}

	var scopeType *string
	if input.ScopeType != "" {
		scope := strings.TrimSpace(strings.ToLower(input.ScopeType))
		scopeType = &scope
	}

	policies, err := ns.ListNotifyPolicyOverrides(ctx, projectID, scopeType)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list notify policy overrides")
	}

	return &ListNotifyPolicyOverridesOutput{Body: policies}, nil
}

type GetNotifyPolicyOverrideInput struct {
	PolicyID string `path:"policyID"`
}

type GetNotifyPolicyOverrideOutput struct {
	Body *domain.NotifyPolicyOverride
}

func (s *Server) handleGetNotifyPolicyOverride(ctx context.Context, input *GetNotifyPolicyOverrideInput) (*GetNotifyPolicyOverrideOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}

	ns, err := s.requireNotifyStore()
	if err != nil {
		return nil, err
	}

	policy, err := ns.GetNotifyPolicyOverride(ctx, input.PolicyID, projectID)
	if err != nil {
		if errors.Is(err, store.ErrNotifyPolicyNotFound) {
			return nil, huma.Error404NotFound("notify policy override not found")
		}
		return nil, huma.Error500InternalServerError("failed to get notify policy override")
	}

	return &GetNotifyPolicyOverrideOutput{Body: policy}, nil
}

type DeleteNotifyPolicyOverrideInput struct {
	PolicyID string `path:"policyID"`
}

func (s *Server) handleDeleteNotifyPolicyOverride(ctx context.Context, input *DeleteNotifyPolicyOverrideInput) (*struct{}, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}

	ns, err := s.requireNotifyStore()
	if err != nil {
		return nil, err
	}

	if err := ns.DeleteNotifyPolicyOverride(ctx, input.PolicyID, projectID); err != nil {
		if errors.Is(err, store.ErrNotifyPolicyNotFound) {
			return nil, huma.Error404NotFound("notify policy override not found")
		}
		return nil, huma.Error500InternalServerError("failed to delete notify policy override")
	}
	return nil, nil
}

// Providers API.
type ConfigureNotificationProviderRequest struct {
	Channel    string          `json:"channel" validate:"required"`
	Provider   string          `json:"provider" validate:"required"`
	Name       string          `json:"name" validate:"required"`
	Config     json.RawMessage `json:"config" validate:"required"`
	IsDefault  bool            `json:"is_default"`
	FallbackID string          `json:"fallback_id,omitempty"`
	RateLimit  *int            `json:"rate_limit,omitempty"`
}

type ConfigureNotificationProviderInput struct {
	Body ConfigureNotificationProviderRequest
}

type ConfigureNotificationProviderOutput struct {
	Body *domain.NotificationProvider
}

func (s *Server) handleConfigureNotificationProvider(ctx context.Context, input *ConfigureNotificationProviderInput) (*ConfigureNotificationProviderOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}
	ns, err := s.requireNotifyStore()
	if err != nil {
		return nil, err
	}
	provider := &domain.NotificationProvider{
		ProjectID:  projectID,
		Channel:    req.Channel,
		Provider:   req.Provider,
		Name:       req.Name,
		ConfigEnc:  req.Config,
		IsDefault:  req.IsDefault,
		FallbackID: req.FallbackID,
		RateLimit:  req.RateLimit,
	}
	if err := ns.CreateNotificationProvider(ctx, provider); err != nil {
		return nil, huma.Error500InternalServerError("failed to create provider")
	}
	return &ConfigureNotificationProviderOutput{Body: provider}, nil
}

type ListNotificationProvidersInput struct {
	Channel string `query:"channel"`
}

type ListNotificationProvidersOutput struct {
	Body []domain.NotificationProvider
}

func (s *Server) handleListNotificationProviders(ctx context.Context, input *ListNotificationProvidersInput) (*ListNotificationProvidersOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	ns, err := s.requireNotifyStore()
	if err != nil {
		return nil, err
	}
	providers, err := ns.ListNotificationProviders(ctx, projectID, input.Channel)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list providers")
	}
	return &ListNotificationProvidersOutput{Body: providers}, nil
}

type UpdateNotificationProviderInput struct {
	ProviderID string `path:"providerID"`
	Body       ConfigureNotificationProviderRequest
}

type UpdateNotificationProviderOutput struct {
	Body *domain.NotificationProvider
}

func (s *Server) handleUpdateNotificationProvider(ctx context.Context, input *UpdateNotificationProviderInput) (*UpdateNotificationProviderOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	ns, err := s.requireNotifyStore()
	if err != nil {
		return nil, err
	}
	provider, err := ns.GetNotificationProvider(ctx, input.ProviderID, projectID)
	if err != nil {
		if errors.Is(err, store.ErrNotificationProviderNotFound) {
			return nil, huma.Error404NotFound("provider not found")
		}
		return nil, huma.Error500InternalServerError("failed to get provider")
	}
	req := input.Body
	provider.Channel = coalesceString(req.Channel, provider.Channel)
	provider.Provider = coalesceString(req.Provider, provider.Provider)
	provider.Name = coalesceString(req.Name, provider.Name)
	if len(req.Config) > 0 {
		provider.ConfigEnc = req.Config
	}
	provider.IsDefault = req.IsDefault
	provider.FallbackID = req.FallbackID
	provider.RateLimit = req.RateLimit

	if err := ns.UpdateNotificationProvider(ctx, provider); err != nil {
		if errors.Is(err, store.ErrNotificationProviderNotFound) {
			return nil, huma.Error404NotFound("provider not found")
		}
		return nil, huma.Error500InternalServerError("failed to update provider")
	}
	return &UpdateNotificationProviderOutput{Body: provider}, nil
}

type DeleteNotificationProviderInput struct {
	ProviderID string `path:"providerID"`
}

func (s *Server) handleDeleteNotificationProvider(ctx context.Context, input *DeleteNotificationProviderInput) (*struct{}, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	ns, err := s.requireNotifyStore()
	if err != nil {
		return nil, err
	}
	if err := ns.DeleteNotificationProvider(ctx, input.ProviderID, projectID); err != nil {
		if errors.Is(err, store.ErrNotificationProviderNotFound) {
			return nil, huma.Error404NotFound("provider not found")
		}
		return nil, huma.Error500InternalServerError("failed to delete provider")
	}
	return nil, nil
}

// Deliveries API.
type ListNotifyDeliveriesInput struct {
	Status string `query:"status"`
	Limit  string `query:"limit"`
	Cursor string `query:"cursor"`
}

type ListNotifyDeliveriesOutput struct {
	Body []domain.NotificationMessage
}

func (s *Server) handleListNotifyDeliveries(ctx context.Context, input *ListNotifyDeliveriesInput) (*ListNotifyDeliveriesOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	ns, err := s.requireNotifyStore()
	if err != nil {
		return nil, err
	}
	limit := defaultPageLimit
	if input.Limit != "" {
		if parsed, parseErr := strconv.Atoi(input.Limit); parseErr == nil && parsed > 0 && parsed <= maxPageLimit {
			limit = parsed
		}
	}
	var status *string
	if input.Status != "" {
		status = &input.Status
	}
	var cursor *time.Time
	if input.Cursor != "" {
		if ts, parseErr := time.Parse(time.RFC3339Nano, input.Cursor); parseErr == nil {
			cursor = &ts
		}
	}
	messages, err := ns.ListNotificationMessagesByProject(ctx, projectID, status, limit, cursor)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list deliveries")
	}
	return &ListNotifyDeliveriesOutput{Body: messages}, nil
}

type GetNotifyEscalationByStepRunInput struct {
	StepRunID string `path:"stepRunID"`
}

type GetNotifyEscalationByStepRunOutput struct {
	Body *domain.EscalationState
}

func (s *Server) handleGetNotifyEscalationByStepRun(ctx context.Context, input *GetNotifyEscalationByStepRunInput) (*GetNotifyEscalationByStepRunOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	ns, err := s.requireNotifyStore()
	if err != nil {
		return nil, err
	}

	state, err := ns.GetActiveEscalationStateByStepRun(ctx, projectID, input.StepRunID)
	if err != nil {
		if errors.Is(err, store.ErrEscalationStateNotFound) {
			return nil, huma.Error404NotFound("escalation not found")
		}
		return nil, huma.Error500InternalServerError("failed to get escalation")
	}

	return &GetNotifyEscalationByStepRunOutput{Body: state}, nil
}

type AcknowledgeNotifyEscalationByStepRunInput struct {
	StepRunID string `path:"stepRunID"`
}

type AcknowledgeNotifyEscalationByStepRunOutput struct {
	Body map[string]any
}

func (s *Server) handleAcknowledgeNotifyEscalationByStepRun(ctx context.Context, input *AcknowledgeNotifyEscalationByStepRunInput) (*AcknowledgeNotifyEscalationByStepRunOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	ns, err := s.requireNotifyStore()
	if err != nil {
		return nil, err
	}

	state, err := ns.GetActiveEscalationStateByStepRun(ctx, projectID, input.StepRunID)
	if err != nil {
		if errors.Is(err, store.ErrEscalationStateNotFound) {
			return nil, huma.Error404NotFound("escalation not found")
		}
		return nil, huma.Error500InternalServerError("failed to get escalation")
	}

	acknowledgedBy := actorFromContext(ctx)
	if acknowledgedBy == "" {
		acknowledgedBy = "system"
	}
	acknowledgedAt := time.Now().UTC()
	if err := ns.AcknowledgeActiveEscalationStateByStepRun(ctx, input.StepRunID, acknowledgedBy, acknowledgedAt); err != nil {
		return nil, huma.Error500InternalServerError("failed to acknowledge escalation")
	}

	return &AcknowledgeNotifyEscalationByStepRunOutput{Body: map[string]any{
		"id":              state.ID,
		"step_run_id":     input.StepRunID,
		"status":          domain.NotifyEscalationStatusAcknowledged,
		"acknowledged_by": acknowledgedBy,
		"acknowledged_at": acknowledgedAt,
	}}, nil
}

type CompleteNotifyEscalationByStepRunRequest struct {
	Status string `json:"status,omitempty" validate:"omitempty,oneof=completed failed"`
}

type CompleteNotifyEscalationByStepRunInput struct {
	StepRunID string `path:"stepRunID"`
	Body      CompleteNotifyEscalationByStepRunRequest
}

type CompleteNotifyEscalationByStepRunOutput struct {
	Body map[string]any
}

func (s *Server) handleCompleteNotifyEscalationByStepRun(ctx context.Context, input *CompleteNotifyEscalationByStepRunInput) (*CompleteNotifyEscalationByStepRunOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	ns, err := s.requireNotifyStore()
	if err != nil {
		return nil, err
	}
	if err := s.validate.Struct(input.Body); err != nil {
		return nil, newValidationError(err)
	}

	state, err := ns.GetActiveEscalationStateByStepRun(ctx, projectID, input.StepRunID)
	if err != nil {
		if errors.Is(err, store.ErrEscalationStateNotFound) {
			return nil, huma.Error404NotFound("escalation not found")
		}
		return nil, huma.Error500InternalServerError("failed to get escalation")
	}

	status := input.Body.Status
	if status == "" {
		status = domain.NotifyEscalationStatusCompleted
	}

	if err := ns.CompleteActiveEscalationStateByStepRun(ctx, input.StepRunID, status); err != nil {
		return nil, huma.Error500InternalServerError("failed to complete escalation")
	}

	return &CompleteNotifyEscalationByStepRunOutput{Body: map[string]any{
		"id":           state.ID,
		"step_run_id":  input.StepRunID,
		"status":       status,
		"completed_at": time.Now().UTC(),
	}}, nil
}

// Subscriber token generation.
type CreateNotifySubscriberTokenRequest struct {
	ExpiresIn string `json:"expires_in,omitempty"`
	TenantID  string `json:"tenant_id,omitempty"`
}

type CreateNotifySubscriberTokenInput struct {
	SubscriberID string `path:"subscriberID"`
	Body         CreateNotifySubscriberTokenRequest
}

type CreateNotifySubscriberTokenOutput struct {
	Body map[string]any
}

func (s *Server) handleCreateNotifySubscriberToken(ctx context.Context, input *CreateNotifySubscriberTokenInput) (*CreateNotifySubscriberTokenOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	ns, err := s.requireNotifyStore()
	if err != nil {
		return nil, err
	}
	sub, err := ns.GetNotifySubscriber(ctx, input.SubscriberID, projectID)
	if err != nil {
		if errors.Is(err, store.ErrNotifySubscriberNotFound) {
			return nil, huma.Error404NotFound("subscriber not found")
		}
		return nil, huma.Error500InternalServerError("failed to resolve subscriber")
	}
	expires := 24 * time.Hour
	if input.Body.ExpiresIn != "" {
		parsed, parseErr := time.ParseDuration(input.Body.ExpiresIn)
		if parseErr != nil {
			return nil, huma.Error400BadRequest("invalid expires_in")
		}
		expires = parsed
	}
	tenantID := input.Body.TenantID
	if tenantID == "" {
		tenantID = sub.TenantID
	}
	tok, err := s.createNotifySubscriberToken(sub.ID, projectID, tenantID, expires)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to create subscriber token")
	}
	return &CreateNotifySubscriberTokenOutput{Body: map[string]any{"token": tok}}, nil
}

type CreateNotifyUnsuppressRequest struct {
	Channel string `json:"channel" validate:"required"`
	Reason  string `json:"reason,omitempty"`
	Scope   string `json:"scope,omitempty"`
	Force   bool   `json:"force,omitempty"`
}

type CreateNotifyUnsuppressInput struct {
	SubscriberID string `path:"subscriberID"`
	Body         CreateNotifyUnsuppressRequest
}

type CreateNotifyUnsuppressOutput struct {
	Body map[string]any
}

func (s *Server) handleCreateNotifyUnsuppress(ctx context.Context, input *CreateNotifyUnsuppressInput) (*CreateNotifyUnsuppressOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	if err := s.validate.Struct(input.Body); err != nil {
		return nil, newValidationError(err)
	}

	ns, err := s.requireNotifyStore()
	if err != nil {
		return nil, err
	}

	sub, err := ns.GetNotifySubscriber(ctx, input.SubscriberID, projectID)
	if err != nil {
		if errors.Is(err, store.ErrNotifySubscriberNotFound) {
			return nil, huma.Error404NotFound("subscriber not found")
		}
		return nil, huma.Error500InternalServerError("failed to resolve subscriber")
	}

	channel := strings.ToLower(strings.TrimSpace(input.Body.Channel))
	scope := strings.TrimSpace(input.Body.Scope)
	if scope == "" {
		scope = "global"
	}
	reason := strings.TrimSpace(input.Body.Reason)
	if reason == "" {
		reason = "manual_unsuppress"
	}

	if policyErr := s.enforceNotifyUnsuppressPolicy(ctx, ns, projectID, sub.ID, channel, input.Body.Force, false); policyErr != nil {
		return nil, policyErr
	}

	if err := ns.EnableNotificationChannelPreference(ctx, domain.NotifyRecipientTypeSubscriber, sub.ID, scope, channel); err != nil {
		return nil, huma.Error500InternalServerError("failed to enable channel preference")
	}

	metadata, err := json.Marshal(map[string]any{
		"actor": actorFromContext(ctx),
		"path":  "api.unsuppress",
		"force": input.Body.Force,
	})
	if err == nil {
		if eventErr := ns.CreateNotifySuppressionEvent(ctx, &domain.NotifySuppressionEvent{
			ProjectID:     projectID,
			RecipientType: domain.NotifyRecipientTypeSubscriber,
			RecipientID:   sub.ID,
			Scope:         scope,
			Channel:       channel,
			Action:        domain.NotifySuppressionActionUnsuppressed,
			Reason:        reason,
			Source:        domain.NotifySuppressionSourceAdminAPI,
			Metadata:      metadata,
		}); eventErr != nil {
			return nil, huma.Error500InternalServerError("failed to record unsuppression event")
		}
	}

	return &CreateNotifyUnsuppressOutput{Body: map[string]any{
		"status":        "ok",
		"subscriber_id": sub.ID,
		"channel":       channel,
		"scope":         scope,
		"reason":        reason,
		"force":         input.Body.Force,
	}}, nil
}

type ListNotifySuppressionEventsInput struct {
	SubscriberID string `path:"subscriberID"`
	Limit        string `query:"limit"`
	Cursor       string `query:"cursor"`
}

type ListNotifySuppressionEventsOutput struct {
	Body []domain.NotifySuppressionEvent
}

func (s *Server) handleListNotifySuppressionEvents(ctx context.Context, input *ListNotifySuppressionEventsInput) (*ListNotifySuppressionEventsOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}

	ns, err := s.requireNotifyStore()
	if err != nil {
		return nil, err
	}
	if _, err := ns.GetNotifySubscriber(ctx, input.SubscriberID, projectID); err != nil {
		if errors.Is(err, store.ErrNotifySubscriberNotFound) {
			return nil, huma.Error404NotFound("subscriber not found")
		}
		return nil, huma.Error500InternalServerError("failed to resolve subscriber")
	}

	limit := defaultPageLimit
	if input.Limit != "" {
		if parsed, parseErr := strconv.Atoi(input.Limit); parseErr == nil && parsed > 0 && parsed <= maxPageLimit {
			limit = parsed
		}
	}
	var cursor *time.Time
	if input.Cursor != "" {
		if ts, parseErr := time.Parse(time.RFC3339Nano, input.Cursor); parseErr == nil {
			cursor = &ts
		}
	}

	events, err := ns.ListNotifySuppressionEvents(ctx, projectID, domain.NotifyRecipientTypeSubscriber, input.SubscriberID, limit, cursor)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list suppression events")
	}
	return &ListNotifySuppressionEventsOutput{Body: events}, nil
}

// Test send uses the same trigger pipeline but marks payload as test.
type NotifyTestInput struct {
	Body NotifyTriggerRequest
}

type NotifyTestOutput struct {
	Body *NotifyTriggerResponse
}

func (s *Server) handleNotifyTest(ctx context.Context, input *NotifyTestInput) (*NotifyTestOutput, error) {
	payload := map[string]any{}
	if len(input.Body.Payload) > 0 {
		if err := json.Unmarshal(input.Body.Payload, &payload); err != nil {
			return nil, huma.Error400BadRequest("payload must be valid JSON")
		}
	}
	payload["test"] = true
	encoded, _ := json.Marshal(payload)
	req := NotifyTriggerInput{Body: input.Body}
	req.Body.Payload = encoded
	out, err := s.handleNotifyTrigger(ctx, &req)
	if err != nil {
		return nil, err
	}
	return &NotifyTestOutput{Body: out.Body}, nil
}
