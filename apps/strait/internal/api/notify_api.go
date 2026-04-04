package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"strconv"
	"strings"
	"time"

	"strait/internal/domain"
	notifycore "strait/internal/notify"
	"strait/internal/store"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/redis/go-redis/v9"
	"github.com/resend/resend-go/v2"
)

const (
	notifyDefaultRateLimitPerHour = 20
)

type notifyStore interface {
	store.NotifyStore
}

func (s *Server) requireNotifyStore() (notifyStore, error) {
	ns, ok := s.store.(notifyStore)
	if !ok {
		return nil, huma.Error503ServiceUnavailable("notify store unavailable")
	}
	return ns, nil
}

type NotifyRecipientInput struct {
	Type string `json:"type" validate:"required,oneof=subscriber topic dashboard_user"`
	ID   string `json:"id,omitempty"`
	Key  string `json:"key,omitempty"`
}

type NotifyDedupInput struct {
	Key    string `json:"key"`
	Window string `json:"window,omitempty"`
}

type NotifyScheduleInput struct {
	Delay string `json:"delay,omitempty"`
	At    string `json:"at,omitempty"`
}

type NotifyTriggerRequest struct {
	To          NotifyRecipientInput `json:"to" validate:"required"`
	TenantID    string               `json:"tenant_id,omitempty"`
	TemplateKey string               `json:"template_key" validate:"required"`
	Payload     json.RawMessage      `json:"payload,omitempty"`
	Channels    []string             `json:"channels,omitempty"`
	CategoryKey string               `json:"category_key,omitempty"`
	Dedup       *NotifyDedupInput    `json:"dedup,omitempty"`
	Schedule    *NotifyScheduleInput `json:"schedule,omitempty"`
}

type NotifyTriggerInput struct {
	Body NotifyTriggerRequest
}

type NotifyTriggerResult struct {
	RecipientID string   `json:"recipient_id"`
	MessageIDs  []string `json:"message_ids"`
}

type NotifyTriggerResponse struct {
	Results []NotifyTriggerResult `json:"results"`
}

type NotifyTriggerOutput struct {
	Body *NotifyTriggerResponse
}

//nolint:gocognit,gocyclo,cyclop,funlen
func (s *Server) handleNotifyTrigger(ctx context.Context, input *NotifyTriggerInput) (*NotifyTriggerOutput, error) {
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

	categoryType := domain.NotifyCategoryTypeProduct
	if req.CategoryKey != "" {
		category, catErr := ns.GetNotificationCategoryByKey(ctx, projectID, req.CategoryKey)
		if catErr != nil {
			if errors.Is(catErr, store.ErrNotificationCategoryNotFound) {
				return nil, huma.Error400BadRequest("invalid category_key")
			}
			return nil, huma.Error500InternalServerError("failed to resolve category")
		}
		categoryType = category.Type
	}

	recipients, err := s.resolveNotifyRecipients(ctx, ns, projectID, req.To, req.TenantID)
	if err != nil {
		return nil, err
	}
	if len(recipients) == 0 {
		return nil, huma.Error404NotFound("no recipients found")
	}

	scheduledAt, err := resolveNotifySchedule(req.Schedule)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	payloadMap := map[string]any{}
	if len(req.Payload) > 0 {
		if err := json.Unmarshal(req.Payload, &payloadMap); err != nil {
			return nil, huma.Error400BadRequest("payload must be valid JSON")
		}
	}

	results := make([]NotifyTriggerResult, 0, len(recipients))
	for _, recipient := range recipients {
		if recipient.Status != domain.NotifySubscriberStatusActive {
			continue
		}

		if req.Dedup != nil && req.Dedup.Key != "" && categoryType != domain.NotifyCategoryTypeCritical {
			window := 10 * time.Minute
			if req.Dedup.Window != "" {
				if parsed, parseErr := time.ParseDuration(req.Dedup.Window); parseErr == nil && parsed > 0 {
					window = parsed
				}
			}
			allowed, dedupErr := s.tryNotifyDedup(ctx, ns, projectID, req.Dedup.Key, window)
			if dedupErr != nil {
				return nil, huma.Error500InternalServerError("failed to evaluate dedup")
			}
			if !allowed {
				continue
			}
		}

		systemVars := map[string]any{}
		if recipient.ID != "" {
			systemVars["preferences_url"] = s.absoluteNotifyURL("/v1/preferences")
			if unsubURL, tokenErr := s.createUnsubscribeURL(ctx, ns, projectID, recipient.ID, req.CategoryKey); tokenErr == nil {
				systemVars["unsubscribe_url"] = unsubURL
			}
		}

		renderContext := buildNotifyRenderContext(payloadMap, &recipient, systemVars)
		rendered, renderErr := notifycore.RenderTemplate(tmpl, recipient.Locale, renderContext)
		if renderErr != nil {
			return nil, huma.Error500InternalServerError("failed to render template")
		}

		channels := req.Channels
		if len(channels) == 0 {
			channels = mapKeys(rendered.Channels)
		}

		result := NotifyTriggerResult{RecipientID: recipient.ID, MessageIDs: make([]string, 0, len(channels))}
		for _, channel := range channels {
			channel = strings.TrimSpace(strings.ToLower(channel))
			if channel == "" {
				continue
			}

			if categoryType != domain.NotifyCategoryTypeCritical {
				allowed, prefErr := s.isNotifyChannelAllowed(ctx, ns, recipient, channel)
				if prefErr != nil {
					return nil, huma.Error500InternalServerError("failed to evaluate preferences")
				}
				if !allowed {
					continue
				}

				rateAllowed := s.allowNotifyRate(ctx, ns, recipient, channel)
				if !rateAllowed {
					continue
				}
			}

			rawChannelPayload, ok := rendered.Channels[channel]
			if !ok {
				continue
			}
			channelPayload, marshalErr := json.Marshal(rawChannelPayload)
			if marshalErr != nil {
				return nil, huma.Error500InternalServerError("failed to encode rendered channel")
			}

			message := &domain.NotificationMessage{
				ProjectID:       projectID,
				RecipientType:   domain.NotifyRecipientTypeSubscriber,
				RecipientID:     recipient.ID,
				TenantID:        recipient.TenantID,
				TemplateID:      tmpl.ID,
				CategoryKey:     req.CategoryKey,
				Channel:         channel,
				RenderedContent: channelPayload,
				Status:          domain.NotifyMessageStatusPending,
				ScheduledAt:     scheduledAt,
			}
			if scheduledAt != nil {
				message.Status = domain.NotifyMessageStatusScheduled
			}

			if createErr := ns.CreateNotificationMessage(ctx, message); createErr != nil {
				var pgErr *pgconn.PgError
				if errors.As(createErr, &pgErr) && pgErr.Code == "23505" {
					continue
				}
				return nil, huma.Error500InternalServerError("failed to create notification message")
			}
			result.MessageIDs = append(result.MessageIDs, message.ID)

			if scheduledAt != nil {
				continue
			}

			if err := ns.UpdateNotificationMessageStatus(ctx, message.ID, projectID, domain.NotifyMessageStatusPending, domain.NotifyMessageStatusProcessing, nil); err != nil {
				return nil, huma.Error500InternalServerError("failed to update message status")
			}

			sendErr := s.deliverNotifyChannel(ctx, ns, &recipient, message, rawChannelPayload)
			if sendErr != nil {
				_ = ns.UpdateNotificationMessageStatus(ctx, message.ID, projectID, domain.NotifyMessageStatusProcessing, domain.NotifyMessageStatusFailed, map[string]any{
					"suppression_reason": sendErr.Error(),
				})
				s.dispatchNotifyWebhookEvent(ctx, projectID, "notification.failed", map[string]any{
					"message_id":    message.ID,
					"subscriber_id": recipient.ID,
					"channel":       channel,
					"error":         sendErr.Error(),
				})
				continue
			}

			now := time.Now().UTC()
			_ = ns.UpdateNotificationMessageStatus(ctx, message.ID, projectID, domain.NotifyMessageStatusProcessing, domain.NotifyMessageStatusDelivered, map[string]any{
				"delivered_at": now,
			})
			s.dispatchNotifyWebhookEvent(ctx, projectID, "notification.delivered", map[string]any{
				"message_id":    message.ID,
				"subscriber_id": recipient.ID,
				"channel":       channel,
				"delivered_at":  now,
			})
		}

		if len(result.MessageIDs) > 0 {
			results = append(results, result)
		}
	}

	return &NotifyTriggerOutput{Body: &NotifyTriggerResponse{Results: results}}, nil
}

func (s *Server) resolveNotifyRecipients(ctx context.Context, ns notifyStore, projectID string, to NotifyRecipientInput, tenantID string) ([]domain.NotifySubscriber, error) {
	switch to.Type {
	case "subscriber":
		if to.ID == "" {
			return nil, huma.Error400BadRequest("to.id is required for subscriber recipient")
		}
		sub, err := ns.GetNotifySubscriber(ctx, to.ID, projectID)
		if err != nil {
			if errors.Is(err, store.ErrNotifySubscriberNotFound) {
				return nil, huma.Error404NotFound("subscriber not found")
			}
			return nil, huma.Error500InternalServerError("failed to resolve subscriber")
		}
		if tenantID != "" && sub.TenantID != tenantID {
			return nil, nil
		}
		return []domain.NotifySubscriber{*sub}, nil
	case "topic":
		if to.Key == "" {
			return nil, huma.Error400BadRequest("to.key is required for topic recipient")
		}
		var tenant *string
		if tenantID != "" {
			tenant = &tenantID
		}
		subs, err := ns.ListNotifySubscribersByTopicKey(ctx, projectID, to.Key, tenant, 10000)
		if err != nil {
			return nil, huma.Error500InternalServerError("failed to resolve topic subscribers")
		}
		return subs, nil
	default:
		return nil, huma.Error400BadRequest("unsupported recipient type")
	}
}

func resolveNotifySchedule(schedule *NotifyScheduleInput) (*time.Time, error) {
	if schedule == nil {
		return nil, nil
	}
	if schedule.Delay != "" {
		d, err := time.ParseDuration(schedule.Delay)
		if err != nil {
			return nil, fmt.Errorf("invalid schedule.delay")
		}
		t := time.Now().UTC().Add(d)
		return &t, nil
	}
	if schedule.At != "" {
		t, err := time.Parse(time.RFC3339, schedule.At)
		if err != nil {
			return nil, fmt.Errorf("invalid schedule.at")
		}
		utc := t.UTC()
		return &utc, nil
	}
	return nil, nil
}

func buildNotifyRenderContext(payload map[string]any, sub *domain.NotifySubscriber, systemVars map[string]any) map[string]any {
	ctx := map[string]any{}
	maps.Copy(ctx, payload)
	ctx["payload"] = payload

	subscriberMap := map[string]any{
		"id":          sub.ID,
		"external_id": sub.ExternalID,
		"email":       sub.Email,
		"phone":       sub.Phone,
		"locale":      sub.Locale,
		"timezone":    sub.Timezone,
		"tenant_id":   sub.TenantID,
	}
	if len(sub.Attributes) > 0 {
		attrs := map[string]any{}
		if err := json.Unmarshal(sub.Attributes, &attrs); err == nil {
			maps.Copy(subscriberMap, attrs)
		}
	}
	ctx["subscriber"] = subscriberMap

	maps.Copy(ctx, systemVars)

	return ctx
}

func mapKeys(in map[string]any) []string {
	keys := make([]string, 0, len(in))
	for key := range in {
		keys = append(keys, key)
	}
	return keys
}

func (s *Server) deliverNotifyChannel(ctx context.Context, ns notifyStore, sub *domain.NotifySubscriber, msg *domain.NotificationMessage, payload any) error {
	switch msg.Channel {
	case "inbox":
		item, err := buildInboxItemFromRenderedPayload(sub, msg.ProjectID, msg.WorkflowRunID, msg.CategoryKey, payload)
		if err != nil {
			return err
		}
		if err := ns.CreateInboxItem(ctx, item); err != nil {
			return fmt.Errorf("create inbox item: %w", err)
		}
		s.publishInboxEvent(ctx, msg.ProjectID, domain.NotifyRecipientTypeSubscriber, sub.ID, "new_item", item)
		return nil
	case "email":
		return s.sendNotifyEmail(ctx, ns, sub, msg, payload)
	default:
		return fmt.Errorf("unsupported channel: %s", msg.Channel)
	}
}

func buildInboxItemFromRenderedPayload(sub *domain.NotifySubscriber, projectID, workflowRunID, categoryKey string, payload any) (*domain.InboxItem, error) {
	obj, ok := payload.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("inbox payload must be an object")
	}
	title, _ := obj["title"].(string)
	if title == "" {
		title = "Notification"
	}
	body, _ := obj["body"].(string)
	avatar, _ := obj["avatar"].(string)
	priority, _ := obj["priority"].(string)
	if priority == "" {
		priority = "normal"
	}
	actionsRaw := []byte("[]")
	if actions, hasActions := obj["actions"]; hasActions {
		if encoded, err := json.Marshal(actions); err == nil {
			actionsRaw = encoded
		}
	}

	return &domain.InboxItem{
		RecipientType: domain.NotifyRecipientTypeSubscriber,
		RecipientID:   sub.ID,
		ProjectID:     projectID,
		TenantID:      sub.TenantID,
		WorkflowRunID: workflowRunID,
		CategoryKey:   categoryKey,
		Title:         title,
		Body:          body,
		Avatar:        avatar,
		Priority:      priority,
		State:         domain.NotifyInboxStateUnread,
		Actions:       actionsRaw,
	}, nil
}

type resendProviderConfig struct {
	APIKey    string `json:"api_key"`
	FromEmail string `json:"from_email"`
}

func (s *Server) sendNotifyEmail(ctx context.Context, ns notifyStore, sub *domain.NotifySubscriber, msg *domain.NotificationMessage, payload any) error {
	if sub.Email == "" {
		return fmt.Errorf("subscriber email is required")
	}

	obj, ok := payload.(map[string]any)
	if !ok {
		return fmt.Errorf("email payload must be an object")
	}
	subject, _ := obj["subject"].(string)
	htmlBody, _ := obj["html_body"].(string)
	textBody, _ := obj["text_body"].(string)
	if subject == "" {
		return fmt.Errorf("email subject is required")
	}
	if htmlBody == "" && textBody == "" {
		return fmt.Errorf("email body is required")
	}

	apiKey := s.config.ResendAPIKey
	fromEmail := s.config.ResendFromEmail
	providerName := "resend"

	if msg.ProviderID != "" {
		provider, err := ns.GetNotificationProvider(ctx, msg.ProviderID, msg.ProjectID)
		if err != nil {
			return fmt.Errorf("resolve provider: %w", err)
		}
		providerName = provider.Provider
		cfg := resendProviderConfig{}
		if err := json.Unmarshal(provider.ConfigEnc, &cfg); err != nil {
			return fmt.Errorf("parse provider config: %w", err)
		}
		if cfg.APIKey != "" {
			apiKey = cfg.APIKey
		}
		if cfg.FromEmail != "" {
			fromEmail = cfg.FromEmail
		}
	}

	if strings.ToLower(providerName) != "resend" {
		return fmt.Errorf("unsupported email provider: %s", providerName)
	}
	if apiKey == "" {
		return fmt.Errorf("resend api key is required")
	}
	if fromEmail == "" {
		fromEmail = "noreply@strait.dev"
	}

	client := resend.NewClient(apiKey)
	_, err := client.Emails.SendWithContext(ctx, &resend.SendEmailRequest{
		From:    fromEmail,
		To:      []string{sub.Email},
		Subject: subject,
		Html:    htmlBody,
		Text:    textBody,
	})
	if err != nil {
		return fmt.Errorf("send email: %w", err)
	}

	return nil
}

func (s *Server) isNotifyChannelAllowed(ctx context.Context, ns notifyStore, sub domain.NotifySubscriber, channel string) (bool, error) {
	pref, err := ns.GetNotificationPreference(ctx, domain.NotifyRecipientTypeSubscriber, sub.ID, "global")
	if err != nil {
		if errors.Is(err, store.ErrNotificationPreferenceNotFound) {
			return true, nil
		}
		return false, err
	}

	if len(pref.ChannelPrefs) == 0 {
		return true, nil
	}

	prefs := map[string]bool{}
	_ = json.Unmarshal(pref.ChannelPrefs, &prefs)
	allowed, exists := prefs[channel]
	if !exists {
		return true, nil
	}
	return allowed, nil
}

func (s *Server) allowNotifyRate(ctx context.Context, ns notifyStore, sub domain.NotifySubscriber, channel string) bool {
	limit := notifyDefaultRateLimitPerHour
	if pref, err := ns.GetNotificationPreference(ctx, domain.NotifyRecipientTypeSubscriber, sub.ID, "global"); err == nil {
		if pref.RateLimitOverride != nil && *pref.RateLimitOverride > 0 {
			limit = *pref.RateLimitOverride
		}
	}

	if s.redisClient == nil {
		return true
	}

	key := fmt.Sprintf("notify:rate:%s:%s:%s", sub.ProjectID, sub.ID, channel)
	val, redisErr := s.redisClient.Incr(ctx, key).Result()
	if redisErr != nil {
		return true
	}
	if val == 1 {
		if _, expErr := s.redisClient.Expire(ctx, key, time.Hour).Result(); expErr != nil {
			_ = expErr
		}
	}
	return int(val) <= limit
}

func (s *Server) tryNotifyDedup(ctx context.Context, ns notifyStore, projectID, dedupKey string, ttl time.Duration) (bool, error) {
	if s.redisClient != nil {
		key := fmt.Sprintf("notify:dedup:%s:%s", projectID, dedupKey)
		res, err := s.redisClient.SetArgs(ctx, key, "1", redis.SetArgs{Mode: "NX", TTL: ttl}).Result()
		if err == nil {
			return res == "OK", nil
		}
	}

	return ns.TryNotifyDedupKey(ctx, projectID, dedupKey, ttl)
}

func (s *Server) publishInboxEvent(ctx context.Context, projectID, recipientType, recipientID, eventType string, payload any) {
	if s.pubsub == nil {
		return
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return
	}
	event := map[string]any{
		"event": eventType,
		"data":  json.RawMessage(body),
	}
	msg, err := json.Marshal(event)
	if err != nil {
		return
	}

	channel := fmt.Sprintf("notify:sse:%s:%s:%s", projectID, recipientType, recipientID)
	_ = s.pubsub.Publish(ctx, channel, msg)
}

func (s *Server) dispatchNotifyWebhookEvent(ctx context.Context, projectID, eventType string, data map[string]any) {
	subs, err := s.store.ListWebhookSubscriptions(ctx, projectID)
	if err != nil {
		return
	}

	payload := map[string]any{
		"event":     eventType,
		"timestamp": time.Now().UTC(),
		"data":      data,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return
	}

	for _, sub := range subs {
		if !sub.Active || !eventTypeMatches(sub.EventTypes, eventType) {
			continue
		}
		bodyCopy := append([]byte(nil), body...)
		s.bgPool.Submit(func() {
			req, reqErr := http.NewRequestWithContext(context.Background(), http.MethodPost, sub.WebhookURL, strings.NewReader(string(bodyCopy)))
			if reqErr != nil {
				return
			}
			req.Header.Set("Content-Type", "application/json")
			if sub.Secret != "" {
				mac := hmac.New(sha256.New, []byte(sub.Secret))
				_, _ = mac.Write(bodyCopy)
				req.Header.Set("X-Strait-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
			}
			client := &http.Client{Timeout: 5 * time.Second}
			resp, doErr := client.Do(req)
			if doErr != nil {
				return
			}
			_ = resp.Body.Close()
		})
	}
}

func eventTypeMatches(allowed []string, eventType string) bool {
	for _, typ := range allowed {
		if typ == "*" || typ == eventType {
			return true
		}
	}
	return false
}

func (s *Server) absoluteNotifyURL(path string) string {
	base := strings.TrimSuffix(strings.TrimSpace(s.config.ExternalAPIURL), "/")
	if base == "" {
		base = "https://notify.strait.dev"
	}
	return base + path
}

func (s *Server) createUnsubscribeURL(ctx context.Context, ns notifyStore, projectID, subscriberID, categoryKey string) (string, error) {
	tokenValue := fmt.Sprintf("unsub_%d", time.Now().UnixNano())
	scope := "global"
	if categoryKey != "" {
		scope = "category:" + categoryKey
	}
	tok := &domain.UnsubscribeToken{
		ProjectID:    projectID,
		SubscriberID: subscriberID,
		Scope:        scope,
		Token:        tokenValue,
		ExpiresAt:    time.Now().UTC().Add(30 * 24 * time.Hour),
	}
	if err := ns.CreateUnsubscribeToken(ctx, tok); err != nil {
		return "", err
	}
	return s.absoluteNotifyURL("/v1/unsubscribe/" + tok.Token), nil
}

// Subscribers API.
type UpsertNotifySubscriberRequest struct {
	ExternalID string          `json:"external_id" validate:"required"`
	Email      string          `json:"email,omitempty"`
	Phone      string          `json:"phone,omitempty"`
	Locale     string          `json:"locale,omitempty"`
	Timezone   string          `json:"timezone,omitempty"`
	TenantID   string          `json:"tenant_id,omitempty"`
	Attributes json.RawMessage `json:"attributes,omitempty"`
}

type UpsertNotifySubscriberInput struct {
	Body UpsertNotifySubscriberRequest
}

type UpsertNotifySubscriberOutput struct {
	Body *domain.NotifySubscriber
}

func (s *Server) handleUpsertNotifySubscriber(ctx context.Context, input *UpsertNotifySubscriberInput) (*UpsertNotifySubscriberOutput, error) {
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

	sub := &domain.NotifySubscriber{
		ProjectID:  projectID,
		ExternalID: req.ExternalID,
		Email:      req.Email,
		Phone:      req.Phone,
		Locale:     req.Locale,
		Timezone:   req.Timezone,
		TenantID:   req.TenantID,
		Attributes: req.Attributes,
	}
	if err := ns.UpsertNotifySubscriber(ctx, sub); err != nil {
		return nil, huma.Error500InternalServerError("failed to upsert subscriber")
	}

	return &UpsertNotifySubscriberOutput{Body: sub}, nil
}

type ListNotifySubscribersInput struct {
	Limit    string `query:"limit"`
	Cursor   string `query:"cursor"`
	TenantID string `query:"tenant_id"`
	Status   string `query:"status"`
}

type ListNotifySubscribersOutput struct {
	Body []domain.NotifySubscriber
}

func (s *Server) handleListNotifySubscribers(ctx context.Context, input *ListNotifySubscribersInput) (*ListNotifySubscribersOutput, error) {
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
	var cursor *time.Time
	if input.Cursor != "" {
		if ts, parseErr := time.Parse(time.RFC3339Nano, input.Cursor); parseErr == nil {
			cursor = &ts
		}
	}
	var tenant *string
	if input.TenantID != "" {
		tenant = &input.TenantID
	}
	var status *string
	if input.Status != "" {
		status = &input.Status
	}

	subs, err := ns.ListNotifySubscribers(ctx, projectID, tenant, status, limit, cursor)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list subscribers")
	}
	return &ListNotifySubscribersOutput{Body: subs}, nil
}

type GetNotifySubscriberInput struct {
	SubscriberID string `path:"subscriberID"`
}

type GetNotifySubscriberOutput struct {
	Body *domain.NotifySubscriber
}

func (s *Server) handleGetNotifySubscriber(ctx context.Context, input *GetNotifySubscriberInput) (*GetNotifySubscriberOutput, error) {
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
		return nil, huma.Error500InternalServerError("failed to get subscriber")
	}
	return &GetNotifySubscriberOutput{Body: sub}, nil
}

type UpdateNotifySubscriberInput struct {
	SubscriberID string `path:"subscriberID"`
	Body         UpsertNotifySubscriberRequest
}

type UpdateNotifySubscriberOutput struct {
	Body *domain.NotifySubscriber
}

func (s *Server) handleUpdateNotifySubscriber(ctx context.Context, input *UpdateNotifySubscriberInput) (*UpdateNotifySubscriberOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}

	ns, err := s.requireNotifyStore()
	if err != nil {
		return nil, err
	}

	existing, err := ns.GetNotifySubscriber(ctx, input.SubscriberID, projectID)
	if err != nil {
		if errors.Is(err, store.ErrNotifySubscriberNotFound) {
			return nil, huma.Error404NotFound("subscriber not found")
		}
		return nil, huma.Error500InternalServerError("failed to get subscriber")
	}

	req := input.Body
	if req.ExternalID != "" {
		existing.ExternalID = req.ExternalID
	}
	existing.Email = req.Email
	existing.Phone = req.Phone
	if req.Locale != "" {
		existing.Locale = req.Locale
	}
	if req.Timezone != "" {
		existing.Timezone = req.Timezone
	}
	existing.TenantID = req.TenantID
	if len(req.Attributes) > 0 {
		existing.Attributes = req.Attributes
	}

	if err := ns.UpdateNotifySubscriber(ctx, existing); err != nil {
		if errors.Is(err, store.ErrNotifySubscriberNotFound) {
			return nil, huma.Error404NotFound("subscriber not found")
		}
		return nil, huma.Error500InternalServerError("failed to update subscriber")
	}

	return &UpdateNotifySubscriberOutput{Body: existing}, nil
}

type DeleteNotifySubscriberInput struct {
	SubscriberID string `path:"subscriberID"`
	Purge        string `query:"purge"`
}

func (s *Server) handleDeleteNotifySubscriber(ctx context.Context, input *DeleteNotifySubscriberInput) (*struct{}, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}

	ns, err := s.requireNotifyStore()
	if err != nil {
		return nil, err
	}

	purge := strings.EqualFold(strings.TrimSpace(input.Purge), "true")
	if purge {
		if err := ns.PurgeNotifySubscriber(ctx, input.SubscriberID, projectID); err != nil {
			if errors.Is(err, store.ErrNotifySubscriberNotFound) {
				return nil, huma.Error404NotFound("subscriber not found")
			}
			return nil, huma.Error500InternalServerError("failed to purge subscriber")
		}
		return nil, nil
	}

	if err := ns.SoftDeleteNotifySubscriber(ctx, input.SubscriberID, projectID); err != nil {
		if errors.Is(err, store.ErrNotifySubscriberNotFound) {
			return nil, huma.Error404NotFound("subscriber not found")
		}
		return nil, huma.Error500InternalServerError("failed to delete subscriber")
	}

	return nil, nil
}
