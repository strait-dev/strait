package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/danielgtaylor/huma/v2"
)

func validateNotificationChannelConfig(channelType string, config json.RawMessage) error {
	var parsed map[string]any
	if err := json.Unmarshal(config, &parsed); err != nil {
		return fmt.Errorf("invalid config JSON: %w", err)
	}
	var urlField string
	switch channelType {
	case domain.ChannelTypeSlack, domain.ChannelTypeDiscord:
		urlField = "webhook_url"
	case domain.ChannelTypeWebhook:
		urlField = "url"
	default:
		return fmt.Errorf("unsupported channel type: %s", channelType)
	}
	rawURL, ok := parsed[urlField]
	if !ok {
		return fmt.Errorf("%s is required in config", urlField)
	}
	urlStr, ok := rawURL.(string)
	if !ok || urlStr == "" {
		return fmt.Errorf("%s must be a non-empty string", urlField)
	}
	return validateURL(urlStr)
}

type CreateNotificationChannelRequest struct {
	ChannelType string          `json:"channel_type" validate:"required,oneof=slack discord webhook"`
	Name        string          `json:"name" validate:"required,max=255"`
	Config      json.RawMessage `json:"config" validate:"required"`
	Enabled     *bool           `json:"enabled,omitempty"`
}
type UpdateNotificationChannelRequest struct {
	Name        *string          `json:"name,omitempty"`
	ChannelType *string          `json:"channel_type,omitempty"`
	Config      *json.RawMessage `json:"config,omitempty"`
	Enabled     *bool            `json:"enabled,omitempty"`
}

type CreateNotificationChannelInput struct {
	Body CreateNotificationChannelRequest
}
type CreateNotificationChannelOutput struct{ Body *domain.NotificationChannel }

// redactNotificationChannel returns a shallow copy with sensitive fields in
// Config redacted. webhook_url, url, and token-bearing keys typically grant
// post privileges to Slack/Discord/Webhook endpoints; returning them in API
// responses is equivalent to leaking the secret. Keys are preserved so the
// caller can see which fields were configured.
func redactNotificationChannel(c *domain.NotificationChannel) *domain.NotificationChannel {
	if c == nil {
		return nil
	}
	out := *c
	if len(c.Config) == 0 {
		return &out
	}
	var parsed map[string]any
	if err := json.Unmarshal(c.Config, &parsed); err != nil {
		out.Config = json.RawMessage(`{}`)
		return &out
	}
	for k := range parsed {
		parsed[k] = "***"
	}
	redacted, err := json.Marshal(parsed)
	if err != nil {
		out.Config = json.RawMessage(`{}`)
		return &out
	}
	out.Config = redacted
	return &out
}

func redactNotificationChannelList(in []domain.NotificationChannel) []domain.NotificationChannel {
	if len(in) == 0 {
		return in
	}
	out := make([]domain.NotificationChannel, len(in))
	for i := range in {
		out[i] = *redactNotificationChannel(&in[i])
	}
	return out
}

func (s *Server) handleCreateNotificationChannel(ctx context.Context, input *CreateNotificationChannelInput) (*CreateNotificationChannelOutput, error) {
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	if err := validateNotificationChannelConfig(req.ChannelType, req.Config); err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	maxChannels, displayName, err := s.resolveNotificationChannelCreateLimit(ctx, projectID)
	if err != nil {
		return nil, err
	}
	ch := &domain.NotificationChannel{ProjectID: projectID, ChannelType: req.ChannelType, Name: req.Name, Config: req.Config, Enabled: enabled}
	var createErr error
	if creator, ok := s.store.(notificationChannelProjectLimitCreator); ok {
		createErr = creator.CreateNotificationChannelWithProjectLimit(ctx, ch, maxChannels)
	} else {
		if err := s.checkNotificationChannelLimit(ctx, projectID); err != nil {
			return nil, err
		}
		createErr = s.store.CreateNotificationChannel(ctx, ch)
	}
	if createErr != nil {
		if errors.Is(createErr, store.ErrNotificationChannelLimitExceeded) {
			return nil, huma.Error400BadRequest(
				fmt.Sprintf("Your %s plan allows %d notification channels per project. Upgrade at /settings/billing", displayName, maxChannels),
			)
		}
		return nil, huma.Error500InternalServerError("failed to create notification channel")
	}
	s.emitAuditEvent(ctx, domain.AuditActionNotificationChannelCreated, "notification_channel", ch.ID, map[string]any{
		"name":         ch.Name,
		"channel_type": ch.ChannelType,
		"enabled":      ch.Enabled,
	})
	return &CreateNotificationChannelOutput{Body: redactNotificationChannel(ch)}, nil
}

type ListNotificationChannelsInput struct{}
type ListNotificationChannelsOutput struct{ Body []domain.NotificationChannel }

func (s *Server) handleListNotificationChannels(ctx context.Context, _ *ListNotificationChannelsInput) (*ListNotificationChannelsOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	channels, err := s.store.ListNotificationChannels(ctx, projectID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list notification channels")
	}
	return &ListNotificationChannelsOutput{Body: redactNotificationChannelList(channels)}, nil
}

type GetNotificationChannelInput struct {
	ChannelID string `path:"channelID"`
}
type GetNotificationChannelOutput struct{ Body *domain.NotificationChannel }

func (s *Server) handleGetNotificationChannel(ctx context.Context, input *GetNotificationChannelInput) (*GetNotificationChannelOutput, error) {
	if input.ChannelID == "" {
		return nil, huma.Error400BadRequest("channel id is required")
	}
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	ch, err := s.store.GetNotificationChannel(ctx, input.ChannelID, projectID)
	if err != nil {
		if errors.Is(err, store.ErrNotificationChannelNotFound) {
			return nil, huma.Error404NotFound("notification channel not found")
		}
		return nil, huma.Error500InternalServerError("failed to get notification channel")
	}
	return &GetNotificationChannelOutput{Body: redactNotificationChannel(ch)}, nil
}

type UpdateNotificationChannelInput struct {
	ChannelID string `path:"channelID"`
	Body      UpdateNotificationChannelRequest
}
type UpdateNotificationChannelOutput struct{ Body *domain.NotificationChannel }

func (s *Server) handleUpdateNotificationChannel(ctx context.Context, input *UpdateNotificationChannelInput) (*UpdateNotificationChannelOutput, error) {
	if input.ChannelID == "" {
		return nil, huma.Error400BadRequest("channel id is required")
	}
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	ch, err := s.store.GetNotificationChannel(ctx, input.ChannelID, projectID)
	if err != nil {
		if errors.Is(err, store.ErrNotificationChannelNotFound) {
			return nil, huma.Error404NotFound("notification channel not found")
		}
		return nil, huma.Error500InternalServerError("failed to get notification channel")
	}
	req := input.Body
	if req.Name != nil {
		ch.Name = *req.Name
	}
	if req.ChannelType != nil {
		ch.ChannelType = *req.ChannelType
	}
	if req.Config != nil {
		ch.Config = *req.Config
	}
	if req.Enabled != nil {
		ch.Enabled = *req.Enabled
	}
	if req.Config != nil || req.ChannelType != nil {
		if err := validateNotificationChannelConfig(ch.ChannelType, ch.Config); err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}
	}
	if err := s.store.UpdateNotificationChannel(ctx, ch); err != nil {
		if errors.Is(err, store.ErrNotificationChannelNotFound) {
			return nil, huma.Error404NotFound("notification channel not found")
		}
		return nil, huma.Error500InternalServerError("failed to update notification channel")
	}
	changedFields := make([]string, 0, 4)
	if req.Name != nil {
		changedFields = append(changedFields, "name")
	}
	if req.ChannelType != nil {
		changedFields = append(changedFields, "channel_type")
	}
	if req.Config != nil {
		changedFields = append(changedFields, "config")
	}
	if req.Enabled != nil {
		changedFields = append(changedFields, "enabled")
	}
	s.emitAuditEvent(ctx, domain.AuditActionNotificationChannelUpdated, "notification_channel", ch.ID, map[string]any{
		"name":           ch.Name,
		"channel_type":   ch.ChannelType,
		"changed_fields": changedFields,
	})
	return &UpdateNotificationChannelOutput{Body: redactNotificationChannel(ch)}, nil
}

type DeleteNotificationChannelInput struct {
	ChannelID string `path:"channelID"`
}

func (s *Server) handleDeleteNotificationChannel(ctx context.Context, input *DeleteNotificationChannelInput) (*struct{}, error) {
	if input.ChannelID == "" {
		return nil, huma.Error400BadRequest("channel id is required")
	}
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	if err := s.store.DeleteNotificationChannel(ctx, input.ChannelID, projectID); err != nil {
		if errors.Is(err, store.ErrNotificationChannelNotFound) {
			return nil, huma.Error404NotFound("notification channel not found")
		}
		return nil, huma.Error500InternalServerError("failed to delete notification channel")
	}
	s.emitAuditEvent(ctx, domain.AuditActionNotificationChannelDeleted, "notification_channel", input.ChannelID, nil)
	return nil, nil
}

type ListNotificationDeliveriesInput struct {
	Limit  string `query:"limit"`
	Cursor string `query:"cursor"`
}
type ListNotificationDeliveriesOutput struct{ Body []domain.NotificationDelivery }

func (s *Server) handleListNotificationDeliveries(ctx context.Context, input *ListNotificationDeliveriesInput) (*ListNotificationDeliveriesOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	limit := defaultPageLimit
	if input.Limit != "" {
		if parsed, err := strconv.Atoi(input.Limit); err == nil && parsed > 0 && parsed <= maxPageLimit {
			limit = parsed
		}
	}
	var cursor *time.Time
	if input.Cursor != "" {
		if t, err := time.Parse(time.RFC3339Nano, input.Cursor); err == nil {
			cursor = &t
		}
	}
	deliveries, err := s.store.ListNotificationDeliveries(ctx, projectID, limit, cursor)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list notification deliveries")
	}
	return &ListNotificationDeliveriesOutput{Body: deliveries}, nil
}
