package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/go-chi/chi/v5"
	"github.com/resend/resend-go/v2"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const (
	notifyProviderCallbackOutcomeDelivered = "delivered"
	notifyProviderCallbackOutcomeBounced   = "bounced"
	notifyProviderCallbackOutcomeComplaint = "complaint"
	notifyProviderCallbackOutcomeIgnored   = "ignored"
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
	normalizedEventType := normalizeNotifyProviderEventType(eventType)
	messageID := extractNotifyResendMessageID(payload)
	if messageID == "" {
		s.completeNotifyProviderCallback(ctx, w, normalizedEventType, notifyProviderCallbackOutcomeIgnored, "missing_message_id", "")
		return
	}

	msg, err := ns.GetNotificationMessage(ctx, messageID, projectID)
	if err != nil {
		if errors.Is(err, store.ErrNotificationMessageNotFound) {
			s.completeNotifyProviderCallback(ctx, w, normalizedEventType, notifyProviderCallbackOutcomeIgnored, "message_not_found", messageID)
			return
		}
		http.Error(w, "failed to resolve message", http.StatusInternalServerError)
		return
	}

	if msg.ProjectID != projectID {
		s.completeNotifyProviderCallback(ctx, w, normalizedEventType, notifyProviderCallbackOutcomeIgnored, "project_mismatch", messageID)
		return
	}
	if msg.ProviderID != "" && msg.ProviderID != providerID {
		s.completeNotifyProviderCallback(ctx, w, normalizedEventType, notifyProviderCallbackOutcomeIgnored, "provider_mismatch", messageID)
		return
	}

	status, fields, suppressEmail := resolveNotifyResendCallbackOutcome(normalizedEventType, time.Now().UTC())
	classifiedOutcome := classifyNotifyResendCallbackOutcome(normalizedEventType)
	if status == "" {
		s.completeNotifyProviderCallback(ctx, w, normalizedEventType, notifyProviderCallbackOutcomeIgnored, "unsupported_event", messageID)
		return
	}
	if !shouldApplyNotifyProviderCallbackTransition(msg.Status, status) {
		s.applyNotifySuppressionIfNeeded(ctx, ns, msg, suppressEmail)
		s.completeNotifyProviderCallback(ctx, w, normalizedEventType, notifyProviderCallbackOutcomeIgnored, "duplicate_or_terminal", messageID)
		return
	}

	if err := ns.UpdateNotificationMessageStatus(ctx, msg.ID, msg.ProjectID, msg.Status, status, fields); err != nil {
		if errors.Is(err, store.ErrNotificationMessageStatusConflict) {
			latest, latestErr := ns.GetNotificationMessage(ctx, msg.ID, msg.ProjectID)
			if latestErr == nil && !shouldApplyNotifyProviderCallbackTransition(latest.Status, status) {
				s.applyNotifySuppressionIfNeeded(ctx, ns, msg, suppressEmail)
				s.completeNotifyProviderCallback(ctx, w, normalizedEventType, notifyProviderCallbackOutcomeIgnored, "stale_event", messageID)
				return
			}
		}
		http.Error(w, "failed to update message", http.StatusInternalServerError)
		return
	}

	s.applyNotifySuppressionIfNeeded(ctx, ns, msg, suppressEmail)
	s.completeNotifyProviderCallback(ctx, w, normalizedEventType, classifiedOutcome, "updated", messageID)
}

func normalizeNotifyProviderEventType(eventType string) string {
	return strings.ToLower(strings.TrimSpace(eventType))
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

func classifyNotifyResendCallbackOutcome(eventType string) string {
	normalized := normalizeNotifyProviderEventType(eventType)

	switch {
	case strings.Contains(normalized, "bounce"):
		return notifyProviderCallbackOutcomeBounced
	case strings.Contains(normalized, "complain"):
		return notifyProviderCallbackOutcomeComplaint
	case strings.Contains(normalized, "deliver"):
		return notifyProviderCallbackOutcomeDelivered
	default:
		return notifyProviderCallbackOutcomeIgnored
	}
}

func resolveNotifyResendCallbackOutcome(eventType string, now time.Time) (string, map[string]any, bool) {
	normalized := normalizeNotifyProviderEventType(eventType)
	reason := "provider_callback:" + normalized

	switch classifyNotifyResendCallbackOutcome(normalized) {
	case notifyProviderCallbackOutcomeBounced:
		return domain.NotifyMessageStatusBounced, map[string]any{
			"bounced_at":         now,
			"suppression_reason": reason,
		}, true
	case notifyProviderCallbackOutcomeComplaint:
		return domain.NotifyMessageStatusFailed, map[string]any{
			"suppression_reason": reason,
		}, true
	case notifyProviderCallbackOutcomeDelivered:
		return domain.NotifyMessageStatusDelivered, map[string]any{
			"delivered_at": now,
		}, false
	default:
		return "", nil, false
	}
}

func shouldApplyNotifyProviderCallbackTransition(currentStatus, nextStatus string) bool {
	if nextStatus == "" {
		return false
	}
	if strings.EqualFold(currentStatus, nextStatus) {
		return false
	}
	if isTerminalNotifyMessageStatus(currentStatus) {
		return false
	}
	return true
}

func isTerminalNotifyMessageStatus(status string) bool {
	switch normalizeNotifyProviderEventType(status) {
	case domain.NotifyMessageStatusDelivered,
		domain.NotifyMessageStatusFailed,
		domain.NotifyMessageStatusBounced,
		domain.NotifyMessageStatusCancelled:
		return true
	default:
		return false
	}
}

func (s *Server) applyNotifySuppressionIfNeeded(ctx context.Context, ns notifyStore, msg *domain.NotificationMessage, suppressEmail bool) {
	if !suppressEmail {
		return
	}
	if err := s.suppressNotifyRecipientEmail(ctx, ns, msg); err != nil {
		slog.Warn("notify provider callback suppression failed",
			"message_id", msg.ID,
			"project_id", msg.ProjectID,
			"recipient_id", msg.RecipientID,
			"error", err,
		)
	}
}

func (s *Server) completeNotifyProviderCallback(ctx context.Context, w http.ResponseWriter, eventType, outcome, reason, messageID string) {
	s.recordNotifyProviderCallbackOutcome(ctx, "resend", eventType, outcome)
	slog.Info("notify provider callback processed",
		"provider", "resend",
		"event_type", eventType,
		"outcome", outcome,
		"reason", reason,
		"message_id", messageID,
	)
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) recordNotifyProviderCallbackOutcome(ctx context.Context, provider, eventType, outcome string) {
	if s.metrics == nil {
		return
	}

	s.metrics.NotifyProviderCallbacksTotal.Add(ctx, 1, metric.WithAttributes(
		attribute.String("provider", provider),
		attribute.String("event_type", eventType),
		attribute.String("outcome", outcome),
	))
}

func (s *Server) suppressNotifyRecipientEmail(ctx context.Context, ns notifyStore, msg *domain.NotificationMessage) error {
	if msg == nil || msg.RecipientType != domain.NotifyRecipientTypeSubscriber || msg.RecipientID == "" {
		return nil
	}

	return ns.DisableNotificationChannelPreference(ctx, domain.NotifyRecipientTypeSubscriber, msg.RecipientID, "global", "email")
}
