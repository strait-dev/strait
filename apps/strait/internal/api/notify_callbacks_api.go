package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/go-chi/chi/v5"
	"github.com/resend/resend-go/v2"
)

func (s *Server) handleNotifyResendProviderCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projectID := strings.TrimSpace(chi.URLParam(r, "projectID"))
	providerID := strings.TrimSpace(chi.URLParam(r, "providerID"))
	if projectID == "" || providerID == "" {
		http.Error(w, "projectID and providerID are required", http.StatusBadRequest)
		return
	}

	ns, err := s.requireNotifyStore()
	if err != nil {
		http.Error(w, "notify store unavailable", http.StatusServiceUnavailable)
		return
	}

	provider, err := ns.GetNotificationProvider(ctx, providerID, projectID)
	if err != nil {
		if errors.Is(err, store.ErrNotificationProviderNotFound) {
			http.Error(w, "provider not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to resolve provider", http.StatusInternalServerError)
		return
	}
	if strings.ToLower(provider.Provider) != "resend" {
		http.Error(w, "provider is not resend", http.StatusBadRequest)
		return
	}

	cfg := resendProviderConfig{}
	if err := json.Unmarshal(provider.ConfigEnc, &cfg); err != nil {
		http.Error(w, "invalid provider config", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(cfg.WebhookSecret) == "" {
		http.Error(w, "resend webhook secret is not configured", http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	client := resend.NewClient("")
	verifyErr := client.Webhooks.Verify(&resend.VerifyWebhookOptions{
		Payload: string(body),
		Headers: resend.WebhookHeaders{
			Id:        r.Header.Get("svix-id"),
			Timestamp: r.Header.Get("svix-timestamp"),
			Signature: r.Header.Get("svix-signature"),
		},
		WebhookSecret: cfg.WebhookSecret,
	})
	if verifyErr != nil {
		http.Error(w, "invalid webhook signature", http.StatusUnauthorized)
		return
	}

	payload := map[string]any{}
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	eventType, _ := payload["type"].(string)
	messageID := extractNotifyResendMessageID(payload)
	if messageID == "" {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	msg, err := ns.GetNotificationMessage(ctx, messageID, projectID)
	if err != nil {
		if errors.Is(err, store.ErrNotificationMessageNotFound) {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		http.Error(w, "failed to resolve message", http.StatusInternalServerError)
		return
	}

	status, fields, suppressEmail := resolveNotifyResendCallbackOutcome(eventType, time.Now().UTC())
	if status == "" {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	if err := ns.UpdateNotificationMessageStatus(ctx, msg.ID, msg.ProjectID, "", status, fields); err != nil {
		http.Error(w, "failed to update message", http.StatusInternalServerError)
		return
	}

	if suppressEmail {
		_ = s.suppressNotifyRecipientEmail(ctx, ns, msg)
	}

	w.WriteHeader(http.StatusAccepted)
}

func extractNotifyResendMessageID(payload map[string]any) string {
	if payload == nil {
		return ""
	}
	if messageID, ok := payload["message_id"].(string); ok && messageID != "" {
		return messageID
	}

	data, _ := payload["data"].(map[string]any)
	if messageID, ok := data["message_id"].(string); ok && messageID != "" {
		return messageID
	}

	tags, _ := data["tags"].([]any)
	for _, rawTag := range tags {
		tag, _ := rawTag.(map[string]any)
		name, _ := tag["name"].(string)
		value, _ := tag["value"].(string)
		if strings.EqualFold(name, "strait_message_id") && value != "" {
			return value
		}
	}

	return ""
}

func resolveNotifyResendCallbackOutcome(eventType string, now time.Time) (string, map[string]any, bool) {
	normalized := strings.ToLower(strings.TrimSpace(eventType))
	reason := "provider_callback:" + normalized

	switch {
	case strings.Contains(normalized, "bounce"):
		return domain.NotifyMessageStatusBounced, map[string]any{
			"bounced_at":         now,
			"suppression_reason": reason,
		}, true
	case strings.Contains(normalized, "complain"):
		return domain.NotifyMessageStatusFailed, map[string]any{
			"suppression_reason": reason,
		}, true
	case strings.Contains(normalized, "deliver"):
		return domain.NotifyMessageStatusDelivered, map[string]any{
			"delivered_at": now,
		}, false
	default:
		return "", nil, false
	}
}

func (s *Server) suppressNotifyRecipientEmail(ctx context.Context, ns notifyStore, msg *domain.NotificationMessage) error {
	if msg == nil || msg.RecipientType != domain.NotifyRecipientTypeSubscriber || msg.RecipientID == "" {
		return nil
	}

	pref, err := ns.GetNotificationPreference(ctx, domain.NotifyRecipientTypeSubscriber, msg.RecipientID, "global")
	if err != nil && !errors.Is(err, store.ErrNotificationPreferenceNotFound) {
		return err
	}
	if pref == nil {
		pref = &domain.NotificationPreference{
			RecipientType:    domain.NotifyRecipientTypeSubscriber,
			RecipientID:      msg.RecipientID,
			Scope:            "global",
			CriticalOverride: true,
		}
	}

	channelPrefs := map[string]bool{}
	if len(pref.ChannelPrefs) > 0 {
		_ = json.Unmarshal(pref.ChannelPrefs, &channelPrefs)
	}
	channelPrefs["email"] = false

	encoded, err := json.Marshal(channelPrefs)
	if err != nil {
		return err
	}
	pref.ChannelPrefs = encoded

	return ns.UpsertNotificationPreference(ctx, pref)
}
