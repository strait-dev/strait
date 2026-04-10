package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
)

// Inbox APIs (subscriber auth).
type ListInboxInput struct {
	Limit  string `query:"limit"`
	Cursor string `query:"cursor"`
	State  string `query:"state"`
}

type ListInboxOutput struct {
	Body []domain.InboxItem
}

func (s *Server) handleListInbox(ctx context.Context, input *ListInboxInput) (*ListInboxOutput, error) {
	ns, err := s.requireNotifyStore()
	if err != nil {
		return nil, err
	}
	recipientType := notifyRecipientTypeFromContext(ctx)
	recipientID := notifyRecipientIDFromContext(ctx)
	if recipientType == "" || recipientID == "" {
		return nil, huma.Error401Unauthorized("missing subscriber context")
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
	var state *string
	if input.State != "" {
		state = &input.State
	}
	items, err := ns.ListInboxItems(ctx, recipientType, recipientID, state, limit, cursor)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list inbox items")
	}
	return &ListInboxOutput{Body: items}, nil
}

type InboxUnreadCountOutput struct {
	Body map[string]any
}

func (s *Server) handleInboxUnreadCount(ctx context.Context, _ *struct{}) (*InboxUnreadCountOutput, error) {
	ns, err := s.requireNotifyStore()
	if err != nil {
		return nil, err
	}
	recipientType := notifyRecipientTypeFromContext(ctx)
	recipientID := notifyRecipientIDFromContext(ctx)
	count, err := ns.CountInboxUnread(ctx, recipientType, recipientID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to count unread")
	}
	return &InboxUnreadCountOutput{Body: map[string]any{"count": count}}, nil
}

type UpdateInboxItemRequest struct {
	State string `json:"state" validate:"required,oneof=read archived"`
}

type UpdateInboxItemInput struct {
	ItemID string `path:"itemID"`
	Body   UpdateInboxItemRequest
}

type UpdateInboxItemOutput struct {
	Body *domain.InboxItem
}

func (s *Server) handleUpdateInboxItem(ctx context.Context, input *UpdateInboxItemInput) (*UpdateInboxItemOutput, error) {
	ns, err := s.requireNotifyStore()
	if err != nil {
		return nil, err
	}
	recipientType := notifyRecipientTypeFromContext(ctx)
	recipientID := notifyRecipientIDFromContext(ctx)
	fields := map[string]any{}
	now := time.Now().UTC()
	if input.Body.State == domain.NotifyInboxStateRead {
		fields["read_at"] = now
	}
	if input.Body.State == domain.NotifyInboxStateArchived {
		fields["archived_at"] = now
	}
	if err := ns.UpdateInboxItemState(ctx, input.ItemID, recipientType, recipientID, input.Body.State, fields); err != nil {
		if errors.Is(err, store.ErrInboxItemNotFound) {
			return nil, huma.Error404NotFound("inbox item not found")
		}
		return nil, huma.Error500InternalServerError("failed to update inbox item")
	}
	item, err := ns.GetInboxItem(ctx, input.ItemID, recipientType, recipientID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get inbox item")
	}
	s.publishInboxEvent(ctx, item.ProjectID, recipientType, recipientID, "item_updated", item)
	if unread, countErr := ns.CountInboxUnread(ctx, recipientType, recipientID); countErr == nil {
		s.publishInboxEvent(ctx, item.ProjectID, recipientType, recipientID, "unread_count", map[string]any{"count": unread})
	}
	return &UpdateInboxItemOutput{Body: item}, nil
}

type InboxActionRequest struct {
	ActionIndex int `json:"action_index"`
}

type InboxActionInput struct {
	ItemID string `path:"itemID"`
	Body   InboxActionRequest
}

type InboxActionOutput struct {
	Body map[string]any
}

func (s *Server) handleInboxAction(ctx context.Context, input *InboxActionInput) (*InboxActionOutput, error) {
	ns, err := s.requireNotifyStore()
	if err != nil {
		return nil, err
	}
	recipientType := notifyRecipientTypeFromContext(ctx)
	recipientID := notifyRecipientIDFromContext(ctx)
	item, err := ns.GetInboxItem(ctx, input.ItemID, recipientType, recipientID)
	if err != nil {
		if errors.Is(err, store.ErrInboxItemNotFound) {
			return nil, huma.Error404NotFound("inbox item not found")
		}
		return nil, huma.Error500InternalServerError("failed to get inbox item")
	}

	var actions []map[string]any
	if len(item.Actions) > 0 {
		_ = json.Unmarshal(item.Actions, &actions)
	}
	if input.Body.ActionIndex < 0 || input.Body.ActionIndex >= len(actions) {
		return nil, huma.Error400BadRequest("invalid action_index")
	}
	selected := actions[input.Body.ActionIndex]
	result, _ := json.Marshal(map[string]any{
		"action_index": input.Body.ActionIndex,
		"action":       selected,
		"outcome":      "success",
		"processed_at": time.Now().UTC(),
	})
	now := time.Now().UTC()
	if err := ns.UpdateInboxItemState(ctx, item.ID, recipientType, recipientID, domain.NotifyInboxStateActioned, map[string]any{
		"actioned_at":   now,
		"action_result": json.RawMessage(result),
	}); err != nil {
		return nil, huma.Error500InternalServerError("failed to execute inbox action")
	}
	updated, err := ns.GetInboxItem(ctx, item.ID, recipientType, recipientID)
	if err == nil {
		s.publishInboxEvent(ctx, item.ProjectID, recipientType, recipientID, "item_updated", updated)
	}
	s.dispatchNotifyWebhookEvent(ctx, item.ProjectID, "notification.action_taken", map[string]any{
		"item_id":      item.ID,
		"recipient_id": recipientID,
		"action":       selected,
	})

	return &InboxActionOutput{Body: map[string]any{"item": updated}}, nil
}

func (s *Server) handleInboxMarkAllRead(ctx context.Context, _ *struct{}) (*InboxUnreadCountOutput, error) {
	ns, err := s.requireNotifyStore()
	if err != nil {
		return nil, err
	}
	recipientType := notifyRecipientTypeFromContext(ctx)
	recipientID := notifyRecipientIDFromContext(ctx)
	_, err = ns.MarkAllInboxItemsRead(ctx, recipientType, recipientID, time.Now().UTC())
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to mark all read")
	}
	count, _ := ns.CountInboxUnread(ctx, recipientType, recipientID)
	projectID := projectIDFromContext(ctx)
	s.publishInboxEvent(ctx, projectID, recipientType, recipientID, "unread_count", map[string]any{"count": count})
	return &InboxUnreadCountOutput{Body: map[string]any{"count": count}}, nil
}

func (s *Server) handleInboxFeed(w http.ResponseWriter, r *http.Request) {
	ns, err := s.requireNotifyStore()
	if err != nil {
		respondError(w, r, http.StatusServiceUnavailable, "notify store unavailable")
		return
	}
	recipientType := notifyRecipientTypeFromContext(r.Context())
	recipientID := notifyRecipientIDFromContext(r.Context())
	projectID := projectIDFromContext(r.Context())
	if recipientType == "" || recipientID == "" || projectID == "" {
		respondError(w, r, http.StatusUnauthorized, "missing subscriber context")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		respondError(w, r, http.StatusInternalServerError, "streaming not supported")
		return
	}
	if s.pubsub == nil {
		respondError(w, r, http.StatusServiceUnavailable, "streaming not available")
		return
	}

	channel := fmt.Sprintf("notify:sse:%s:%s:%s", projectID, recipientType, recipientID)
	sub, err := s.pubsub.Subscribe(r.Context(), channel)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to subscribe")
		return
	}
	defer sub.Close()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	if unread, countErr := ns.CountInboxUnread(r.Context(), recipientType, recipientID); countErr == nil {
		event := fmt.Sprintf("event: unread_count\ndata: {\"count\":%d}\n\n", unread)
		_, _ = w.Write([]byte(event))
		flusher.Flush()
	}

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case msg, ok := <-sub.Ch:
			if !ok {
				return
			}
			var envelope struct {
				Event string          `json:"event"`
				Data  json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(msg, &envelope); err != nil {
				continue
			}
			if envelope.Event == "" {
				envelope.Event = "message"
			}
			_, _ = w.Write([]byte("event: " + envelope.Event + "\n"))
			_, _ = w.Write([]byte("data: " + string(envelope.Data) + "\n\n"))
			flusher.Flush()
		case <-ticker.C:
			_, _ = w.Write([]byte("event: heartbeat\ndata: {}\n\n"))
			flusher.Flush()
		}
	}
}

// Preferences API.
type GetNotifyPreferencesOutput struct {
	Body []domain.NotificationPreference
}

func (s *Server) handleGetNotifyPreferences(ctx context.Context, _ *struct{}) (*GetNotifyPreferencesOutput, error) {
	ns, err := s.requireNotifyStore()
	if err != nil {
		return nil, err
	}
	recipientType := notifyRecipientTypeFromContext(ctx)
	recipientID := notifyRecipientIDFromContext(ctx)
	prefs, err := ns.ListNotificationPreferences(ctx, recipientType, recipientID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list preferences")
	}
	return &GetNotifyPreferencesOutput{Body: prefs}, nil
}

type UpdateNotifyPreferencesRequest struct {
	ChannelPrefs      json.RawMessage `json:"channel_prefs,omitempty"`
	QuietHours        json.RawMessage `json:"quiet_hours,omitempty"`
	Phone             string          `json:"phone,omitempty"`
	Timezone          string          `json:"timezone,omitempty"`
	DigestPolicy      string          `json:"digest_policy,omitempty"`
	CriticalOverride  *bool           `json:"critical_override,omitempty"`
	RateLimitOverride *int            `json:"rate_limit_override,omitempty"`
}

type UpdateNotifyPreferencesInput struct {
	Body UpdateNotifyPreferencesRequest
}

func (s *Server) handleUpdateNotifyPreferences(ctx context.Context, input *UpdateNotifyPreferencesInput) (*GetNotifyPreferencesOutput, error) {
	return s.upsertNotifyPreferenceScope(ctx, "global", input.Body)
}

type UpdateNotifyPreferencesScopeInput struct {
	Scope string `path:"scope"`
	Body  UpdateNotifyPreferencesRequest
}

func (s *Server) handleUpdateNotifyPreferencesScope(ctx context.Context, input *UpdateNotifyPreferencesScopeInput) (*GetNotifyPreferencesOutput, error) {
	return s.upsertNotifyPreferenceScope(ctx, input.Scope, input.Body)
}

func (s *Server) upsertNotifyPreferenceScope(ctx context.Context, scope string, req UpdateNotifyPreferencesRequest) (*GetNotifyPreferencesOutput, error) {
	ns, err := s.requireNotifyStore()
	if err != nil {
		return nil, err
	}
	projectID := projectIDFromContext(ctx)
	recipientType := notifyRecipientTypeFromContext(ctx)
	recipientID := notifyRecipientIDFromContext(ctx)

	existing, err := ns.GetNotificationPreference(ctx, recipientType, recipientID, scope)
	if err != nil && !errors.Is(err, store.ErrNotificationPreferenceNotFound) {
		return nil, huma.Error500InternalServerError("failed to read existing preference")
	}
	if existing == nil {
		existing = &domain.NotificationPreference{
			RecipientType:    recipientType,
			RecipientID:      recipientID,
			Scope:            scope,
			CriticalOverride: true,
		}
	}
	if len(req.ChannelPrefs) > 0 {
		if notifyChannelPrefExplicitEnableEmail(req.ChannelPrefs) {
			if err := s.enforceNotifyUnsuppressPolicy(
				ctx,
				ns,
				projectID,
				recipientID,
				"email",
				false,
				true,
			); err != nil {
				return nil, err
			}
		}
		existing.ChannelPrefs = req.ChannelPrefs
	} else {
		existing.ChannelPrefs = nil
	}
	if len(req.QuietHours) > 0 {
		existing.QuietHours = req.QuietHours
	}
	if req.Phone != "" {
		existing.Phone = req.Phone
	}
	if req.Timezone != "" {
		existing.Timezone = req.Timezone
	}
	if req.DigestPolicy != "" {
		existing.DigestPolicy = req.DigestPolicy
	}
	if req.CriticalOverride != nil {
		existing.CriticalOverride = *req.CriticalOverride
	}
	if req.RateLimitOverride != nil {
		existing.RateLimitOverride = req.RateLimitOverride
	}

	if err := ns.UpsertNotificationPreference(ctx, existing); err != nil {
		return nil, huma.Error500InternalServerError("failed to update preference")
	}

	prefs, err := ns.ListNotificationPreferences(ctx, recipientType, recipientID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list preferences")
	}
	return &GetNotifyPreferencesOutput{Body: prefs}, nil
}

// Unsubscribe endpoints.
type GetUnsubscribeInput struct {
	Token string `path:"token"`
}

type GetUnsubscribeOutput struct {
	Body map[string]any
}

func (s *Server) handleGetUnsubscribe(ctx context.Context, input *GetUnsubscribeInput) (*GetUnsubscribeOutput, error) {
	ns, err := s.requireNotifyStore()
	if err != nil {
		return nil, err
	}
	tok, err := ns.GetUnsubscribeToken(ctx, input.Token)
	if err != nil {
		if errors.Is(err, store.ErrUnsubscribeTokenNotFound) {
			return nil, huma.Error404NotFound("unsubscribe token not found")
		}
		return nil, huma.Error500InternalServerError("failed to resolve unsubscribe token")
	}
	return &GetUnsubscribeOutput{Body: map[string]any{
		"token":         input.Token,
		"scope":         tok.Scope,
		"subscriber_id": tok.SubscriberID,
		"expires_at":    tok.ExpiresAt,
	}}, nil
}

type UnsubscribeRequest struct {
	Scope string `json:"scope,omitempty"`
}

type UnsubscribeInput struct {
	Token string `path:"token"`
	Body  UnsubscribeRequest
}

type UnsubscribeOutput struct {
	Body map[string]any
}

func (s *Server) handleProcessUnsubscribe(ctx context.Context, input *UnsubscribeInput) (*UnsubscribeOutput, error) {
	ns, err := s.requireNotifyStore()
	if err != nil {
		return nil, err
	}
	tok, err := ns.GetUnsubscribeToken(ctx, input.Token)
	if err != nil {
		if errors.Is(err, store.ErrUnsubscribeTokenNotFound) {
			return nil, huma.Error404NotFound("unsubscribe token not found")
		}
		return nil, huma.Error500InternalServerError("failed to resolve unsubscribe token")
	}
	scope := input.Body.Scope
	if scope == "" {
		scope = tok.Scope
	}
	if scope == "global" {
		if err := ns.SoftDeleteNotifySubscriber(ctx, tok.SubscriberID, tok.ProjectID); err != nil {
			return nil, huma.Error500InternalServerError("failed to unsubscribe subscriber")
		}
	} else {
		pref := &domain.NotificationPreference{
			RecipientType:    domain.NotifyRecipientTypeSubscriber,
			RecipientID:      tok.SubscriberID,
			Scope:            scope,
			ChannelPrefs:     []byte(`{"email":false}`),
			CriticalOverride: true,
		}
		if err := ns.UpsertNotificationPreference(ctx, pref); err != nil {
			return nil, huma.Error500InternalServerError("failed to update category preference")
		}
	}
	if markErr := ns.UseUnsubscribeToken(ctx, input.Token, time.Now().UTC()); markErr != nil {
		slog.ErrorContext(ctx, "failed to mark unsubscribe token used",
			"err", markErr,
			"token_prefix", input.Token[:min(8, len(input.Token))],
		)
		return nil, huma.Error500InternalServerError("failed to record unsubscribe")
	}
	s.dispatchNotifyWebhookEvent(ctx, tok.ProjectID, "notification.unsubscribed", map[string]any{
		"subscriber_id": tok.SubscriberID,
		"scope":         scope,
	})
	return &UnsubscribeOutput{Body: map[string]any{"status": "ok", "scope": scope}}, nil
}

type UnsubscribeOneClickInput struct {
	Token string `path:"token"`
}

func (s *Server) handleUnsubscribeOneClick(ctx context.Context, input *UnsubscribeOneClickInput) (*UnsubscribeOutput, error) {
	return s.handleProcessUnsubscribe(ctx, &UnsubscribeInput{Token: input.Token, Body: UnsubscribeRequest{}})
}

// Tracking endpoints.
func (s *Server) handleNotifyOpenPixel(w http.ResponseWriter, r *http.Request) {
	messageID := chi.URLParam(r, "messageID")
	ns, err := s.requireNotifyStore()
	if err == nil {
		if msg, getErr := ns.GetNotificationMessageByID(r.Context(), messageID); getErr == nil {
			now := time.Now().UTC()
			_ = ns.UpdateNotificationMessageStatus(r.Context(), msg.ID, msg.ProjectID, "", msg.Status, map[string]any{"read_at": now})
			s.dispatchNotifyWebhookEvent(r.Context(), msg.ProjectID, "notification.read", map[string]any{"message_id": msg.ID, "subscriber_id": msg.RecipientID})
		}
	}
	pixel, _ := base64.StdEncoding.DecodeString("R0lGODlhAQABAIABAP///wAAACwAAAAAAQABAAACAkQBADs=")
	w.Header().Set("Content-Type", "image/gif")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(pixel)
}

func (s *Server) handleNotifyClickRedirect(w http.ResponseWriter, r *http.Request) {
	messageID := chi.URLParam(r, "messageID")
	target := r.URL.Query().Get("url")
	if target == "" {
		respondError(w, r, http.StatusBadRequest, "missing url")
		return
	}
	parsed, err := url.Parse(target)
	if err != nil || parsed.Scheme == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		respondError(w, r, http.StatusBadRequest, "invalid redirect url")
		return
	}
	ns, nsErr := s.requireNotifyStore()
	if nsErr == nil {
		if msg, getErr := ns.GetNotificationMessageByID(r.Context(), messageID); getErr == nil {
			now := time.Now().UTC()
			_ = ns.UpdateNotificationMessageStatus(r.Context(), msg.ID, msg.ProjectID, "", msg.Status, map[string]any{"clicked_at": now})
			s.dispatchNotifyWebhookEvent(r.Context(), msg.ProjectID, "notification.clicked", map[string]any{"message_id": msg.ID, "subscriber_id": msg.RecipientID, "url": target})
		}
	}
	http.Redirect(w, r, target, http.StatusFound)
}
