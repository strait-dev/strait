package notification

import (
	"context"
	"encoding/json"
	"fmt"

	"strait/internal/domain"
	"strait/internal/transactional"
)

const notificationEventUsageForecastWarning = "usage.forecast_warning"

// TransactionalEmailClient is the narrow app-email surface used by notifications.
type TransactionalEmailClient interface {
	Send(ctx context.Context, req transactional.Request) error
}

// EmailSender sends notification email intents through apps/app.
type EmailSender struct {
	client    TransactionalEmailClient
	fromEmail string
}

// NewEmailSender creates a new EmailSender. Returns an error if the
// transactional client is not configured.
func NewEmailSender(client TransactionalEmailClient, fromEmail string) (*EmailSender, error) {
	if client == nil {
		return nil, fmt.Errorf("transactional email client is required")
	}
	return NewEmailSenderWithClient(client, fromEmail), nil
}

// NewEmailSenderWithClient creates an EmailSender with a custom transactional
// email client. Useful for testing.
func NewEmailSenderWithClient(client TransactionalEmailClient, fromEmail string) *EmailSender {
	if fromEmail == "" {
		fromEmail = "noreply@strait.dev"
	}
	return &EmailSender{
		client:    client,
		fromEmail: fromEmail,
	}
}

type emailConfig struct {
	To string `json:"to"`
}

func (e *EmailSender) Send(ctx context.Context, channel *domain.NotificationChannel, delivery *domain.NotificationDelivery) error {
	if e == nil || e.client == nil {
		return fmt.Errorf("email sender is not configured")
	}
	var cfg emailConfig
	if err := json.Unmarshal(channel.Config, &cfg); err != nil {
		return fmt.Errorf("parse email config: %w", err)
	}
	if cfg.To == "" {
		return fmt.Errorf("email recipient address is empty")
	}

	req := emailRequestForEvent([]string{cfg.To}, e.fromEmail, delivery.ID, delivery.EventType, delivery.Payload)
	err := e.client.Send(ctx, req)
	if err != nil {
		return fmt.Errorf("send notification email through transactional endpoint: %w", err)
	}

	return nil
}

func emailRequestForEvent(to []string, from, deliveryID, eventType string, payload json.RawMessage) transactional.Request {
	data, ok := payloadMap(payload)
	if !ok {
		return genericEmailTemplate(to, from, deliveryID, eventType, payload)
	}

	switch eventType {
	case domain.NotificationEventSpendingLimitWarning:
		return transactional.NotificationSpendingLimitWarningRequest(
			to,
			from,
			deliveryID,
			eventType,
			safeStr(data, "org_id"),
			fmt.Sprintf("%.0f%%", safeFloat(data, "overage_pct")),
			fmt.Sprintf("$%.2f", safeFloat(data, "spending_limit_usd")),
			fmt.Sprintf("$%.2f", safeFloat(data, "current_spend_usd")),
		)
	case domain.NotificationEventSpendingLimitReached:
		return transactional.NotificationSpendingLimitReachedRequest(
			to,
			from,
			deliveryID,
			eventType,
			safeStr(data, "org_id"),
			fmt.Sprintf("$%.2f", safeFloat(data, "spending_limit_usd")),
			fmt.Sprintf("$%.2f", safeFloat(data, "current_spend_usd")),
		)
	case domain.NotificationEventCostAnomaly:
		return transactional.NotificationCostAnomalyRequest(
			to,
			from,
			deliveryID,
			eventType,
			safeStr(data, "org_id"),
			safeStr(data, "severity"),
			fmt.Sprintf("%.1fx", safeFloat(data, "spike_ratio")),
			fmt.Sprintf("%d micro-USD", int64(safeFloat(data, "today_spend"))),
			fmt.Sprintf("%d micro-USD", int64(safeFloat(data, "avg_7d_spend"))),
			safeStr(data, "top_contributor"),
		)
	case domain.NotificationEventBudgetThreshold:
		return transactional.NotificationBudgetThresholdRequest(
			to,
			from,
			deliveryID,
			eventType,
			safeStr(data, "project_id"),
			fmt.Sprintf("%.0f%%", safeFloat(data, "threshold_pct")),
			fmt.Sprintf("%d micro-USD", int64(safeFloat(data, "daily_cost_microusd"))),
			fmt.Sprintf("%d micro-USD", int64(safeFloat(data, "limit_microusd"))),
		)
	case notificationEventUsageForecastWarning:
		return transactional.NotificationUsageForecastWarningRequest(
			to,
			from,
			deliveryID,
			eventType,
			safeStr(data, "org_id"),
			int64(safeFloat(data, "days_until_limit")),
			safeStr(data, "recommended_plan"),
			int64(safeFloat(data, "projected_runs")),
		)
	default:
		return genericEmailTemplate(to, from, deliveryID, eventType, payload)
	}
}

func genericEmailTemplate(to []string, from, deliveryID, eventType string, payload json.RawMessage) transactional.Request {
	return transactional.NotificationGenericRequest(to, from, deliveryID, eventType, string(payload))
}

func payloadMap(payload json.RawMessage) (map[string]any, bool) {
	var data map[string]any
	if err := json.Unmarshal(payload, &data); err != nil {
		return nil, false
	}
	return data, true
}

func safeStr(data map[string]any, key string) string {
	v, ok := data[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return fmt.Sprintf("%v", v)
	}
	return s
}

func safeFloat(data map[string]any, key string) float64 {
	v, ok := data[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	default:
		return 0
	}
}
