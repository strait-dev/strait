package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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

	notifyProviderCallbackMaxPayloadBytes = 256 * 1024
	notifyProviderCallbackReceiptTTL      = 30 * 24 * time.Hour
)

type notifyCallbackHTTPError struct {
	status  int
	message string
}

func (e *notifyCallbackHTTPError) Error() string {
	return e.message
}

type notifyResendCallbackEnvelope struct {
	notifyStore         notifyStore
	projectID           string
	providerID          string
	callbackID          string
	normalizedEventType string
	messageID           string
	payloadHash         string
}

func (s *Server) handleNotifyResendProviderCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	envelope, err := s.prepareNotifyResendCallback(ctx, r)
	if err != nil {
		s.writeNotifyCallbackError(w, err)
		return
	}

	recordedReceipt, err := envelope.notifyStore.RecordNotifyProviderCallbackReceipt(
		ctx,
		envelope.projectID,
		envelope.providerID,
		"resend",
		envelope.callbackID,
		envelope.normalizedEventType,
		envelope.messageID,
		envelope.payloadHash,
		time.Now().UTC().Add(notifyProviderCallbackReceiptTTL),
	)
	if err != nil {
		http.Error(w, "failed to record callback receipt", http.StatusInternalServerError)
		return
	}
	if !recordedReceipt {
		s.completeNotifyProviderCallback(ctx, w, envelope.normalizedEventType, notifyProviderCallbackOutcomeIgnored, "duplicate_replay", envelope.messageID, envelope.callbackID)
		return
	}

	processed := false
	defer func() {
		if processed {
			return
		}
		cleanupErr := envelope.notifyStore.DeleteNotifyProviderCallbackReceipt(ctx, envelope.projectID, envelope.providerID, envelope.callbackID)
		if cleanupErr != nil {
			slog.Warn("notify provider callback receipt cleanup failed",
				"provider", "resend",
				"project_id", envelope.projectID,
				"provider_id", envelope.providerID,
				"callback_id", envelope.callbackID,
				"error", cleanupErr,
			)
		}
	}()

	outcome, reason, err := s.applyNotifyResendProviderCallback(ctx, envelope)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	processed = true
	s.completeNotifyProviderCallback(ctx, w, envelope.normalizedEventType, outcome, reason, envelope.messageID, envelope.callbackID)
}

func (s *Server) prepareNotifyResendCallback(ctx context.Context, r *http.Request) (*notifyResendCallbackEnvelope, error) {
	projectID := strings.TrimSpace(chi.URLParam(r, "projectID"))
	providerID := strings.TrimSpace(chi.URLParam(r, "providerID"))
	if projectID == "" || providerID == "" {
		return nil, &notifyCallbackHTTPError{status: http.StatusBadRequest, message: "projectID and providerID are required"}
	}

	ns, err := s.requireNotifyStore()
	if err != nil {
		return nil, &notifyCallbackHTTPError{status: http.StatusServiceUnavailable, message: "notify store unavailable"}
	}

	provider, err := ns.GetNotificationProvider(ctx, providerID, projectID)
	if err != nil {
		if errors.Is(err, store.ErrNotificationProviderNotFound) {
			return nil, &notifyCallbackHTTPError{status: http.StatusNotFound, message: "provider not found"}
		}
		return nil, &notifyCallbackHTTPError{status: http.StatusInternalServerError, message: "failed to resolve provider"}
	}
	if strings.ToLower(provider.Provider) != "resend" {
		return nil, &notifyCallbackHTTPError{status: http.StatusBadRequest, message: "provider is not resend"}
	}

	cfg := resendProviderConfig{}
	if err := json.Unmarshal(provider.ConfigEnc, &cfg); err != nil {
		return nil, &notifyCallbackHTTPError{status: http.StatusBadRequest, message: "invalid provider config"}
	}
	if strings.TrimSpace(cfg.WebhookSecret) == "" {
		return nil, &notifyCallbackHTTPError{status: http.StatusBadRequest, message: "resend webhook secret is not configured"}
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, notifyProviderCallbackMaxPayloadBytes+1))
	if err != nil {
		return nil, &notifyCallbackHTTPError{status: http.StatusBadRequest, message: "failed to read body"}
	}
	defer r.Body.Close()
	if len(body) > notifyProviderCallbackMaxPayloadBytes {
		return nil, &notifyCallbackHTTPError{status: http.StatusRequestEntityTooLarge, message: "payload too large"}
	}

	callbackID := strings.TrimSpace(r.Header.Get("svix-id"))
	if callbackID == "" {
		return nil, &notifyCallbackHTTPError{status: http.StatusBadRequest, message: "missing callback id"}
	}

	client := resend.NewClient("")
	verifyErr := client.Webhooks.Verify(&resend.VerifyWebhookOptions{
		Payload: string(body),
		Headers: resend.WebhookHeaders{
			Id:        callbackID,
			Timestamp: r.Header.Get("svix-timestamp"),
			Signature: r.Header.Get("svix-signature"),
		},
		WebhookSecret: cfg.WebhookSecret,
	})
	if verifyErr != nil {
		return nil, &notifyCallbackHTTPError{status: http.StatusUnauthorized, message: "invalid webhook signature"}
	}

	payload := map[string]any{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, &notifyCallbackHTTPError{status: http.StatusBadRequest, message: "invalid payload"}
	}

	eventType, _ := payload["type"].(string)
	return &notifyResendCallbackEnvelope{
		notifyStore:         ns,
		projectID:           projectID,
		providerID:          providerID,
		callbackID:          callbackID,
		normalizedEventType: normalizeNotifyProviderEventType(eventType),
		messageID:           extractNotifyResendMessageID(payload),
		payloadHash:         hashNotifyProviderCallbackPayload(body),
	}, nil
}

func (s *Server) writeNotifyCallbackError(w http.ResponseWriter, err error) {
	var httpErr *notifyCallbackHTTPError
	if errors.As(err, &httpErr) {
		http.Error(w, httpErr.message, httpErr.status)
		return
	}
	http.Error(w, "failed to process callback", http.StatusInternalServerError)
}

func (s *Server) applyNotifyResendProviderCallback(ctx context.Context, envelope *notifyResendCallbackEnvelope) (string, string, error) {
	ns := envelope.notifyStore
	if envelope.messageID == "" {
		return notifyProviderCallbackOutcomeIgnored, "missing_message_id", nil
	}

	msg, err := ns.GetNotificationMessage(ctx, envelope.messageID, envelope.projectID)
	if err != nil {
		if errors.Is(err, store.ErrNotificationMessageNotFound) {
			return notifyProviderCallbackOutcomeIgnored, "message_not_found", nil
		}
		return "", "", errors.New("failed to resolve message")
	}

	if msg.ProjectID != envelope.projectID {
		return notifyProviderCallbackOutcomeIgnored, "project_mismatch", nil
	}
	if msg.ProviderID != "" && msg.ProviderID != envelope.providerID {
		return notifyProviderCallbackOutcomeIgnored, "provider_mismatch", nil
	}

	status, fields, suppressEmail := resolveNotifyResendCallbackOutcome(envelope.normalizedEventType, time.Now().UTC())
	classifiedOutcome := classifyNotifyResendCallbackOutcome(envelope.normalizedEventType)
	if status == "" {
		return notifyProviderCallbackOutcomeIgnored, "unsupported_event", nil
	}
	if !shouldApplyNotifyProviderCallbackTransition(msg.Status, status) {
		s.applyNotifySuppressionIfNeeded(ctx, ns, msg, suppressEmail)
		return notifyProviderCallbackOutcomeIgnored, "duplicate_or_terminal", nil
	}

	if err := ns.UpdateNotificationMessageStatus(ctx, msg.ID, msg.ProjectID, msg.Status, status, fields); err != nil {
		if errors.Is(err, store.ErrNotificationMessageStatusConflict) {
			latest, latestErr := ns.GetNotificationMessage(ctx, msg.ID, msg.ProjectID)
			if latestErr == nil && !shouldApplyNotifyProviderCallbackTransition(latest.Status, status) {
				s.applyNotifySuppressionIfNeeded(ctx, ns, msg, suppressEmail)
				return notifyProviderCallbackOutcomeIgnored, "stale_event", nil
			}
		}
		return "", "", errors.New("failed to update message")
	}

	s.applyNotifySuppressionIfNeeded(ctx, ns, msg, suppressEmail)
	return classifiedOutcome, "updated", nil
}

func normalizeNotifyProviderEventType(eventType string) string {
	return strings.ToLower(strings.TrimSpace(eventType))
}

func hashNotifyProviderCallbackPayload(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
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

func (s *Server) completeNotifyProviderCallback(ctx context.Context, w http.ResponseWriter, eventType, outcome, reason, messageID, callbackID string) {
	s.recordNotifyProviderCallbackOutcome(ctx, "resend", eventType, outcome, reason)
	slog.Info("notify provider callback processed",
		"provider", "resend",
		"event_type", eventType,
		"outcome", outcome,
		"reason", reason,
		"message_id", messageID,
		"callback_id", callbackID,
	)
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) recordNotifyProviderCallbackOutcome(ctx context.Context, provider, eventType, outcome, reason string) {
	if s.metrics == nil {
		return
	}

	s.metrics.NotifyProviderCallbacksTotal.Add(ctx, 1, metric.WithAttributes(
		attribute.String("provider", provider),
		attribute.String("event_type", eventType),
		attribute.String("outcome", outcome),
		attribute.String("reason", reason),
	))
}

func (s *Server) suppressNotifyRecipientEmail(ctx context.Context, ns notifyStore, msg *domain.NotificationMessage) error {
	if msg == nil || msg.RecipientType != domain.NotifyRecipientTypeSubscriber || msg.RecipientID == "" {
		return nil
	}

	return ns.DisableNotificationChannelPreference(ctx, domain.NotifyRecipientTypeSubscriber, msg.RecipientID, "global", "email")
}
