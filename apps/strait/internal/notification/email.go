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

	template, props := emailTemplateForEvent(delivery.EventType, delivery.Payload)
	err := e.client.Send(ctx, transactional.Request{
		Template:       template,
		To:             []string{cfg.To},
		From:           e.fromEmail,
		IdempotencyKey: fmt.Sprintf("notification:%s:%s", delivery.ID, delivery.EventType),
		Props:          props,
	})
	if err != nil {
		return fmt.Errorf("send notification email through transactional endpoint: %w", err)
	}

	return nil
}

func emailTemplateForEvent(eventType string, payload json.RawMessage) (string, map[string]any) {
	data, ok := payloadMap(payload)
	if !ok {
		return genericEmailTemplate(eventType, payload)
	}

	switch eventType {
	case domain.NotificationEventSpendingLimitWarning:
		return "notification.spending_limit_warning", map[string]any{
			"orgId":          safeStr(data, "org_id"),
			"overagePercent": fmt.Sprintf("%.0f%%", safeFloat(data, "overage_pct")),
			"spendingLimit":  fmt.Sprintf("$%.2f", safeFloat(data, "spending_limit_usd")),
			"currentSpend":   fmt.Sprintf("$%.2f", safeFloat(data, "current_spend_usd")),
		}
	case domain.NotificationEventSpendingLimitReached:
		return "notification.spending_limit_reached", map[string]any{
			"orgId":         safeStr(data, "org_id"),
			"spendingLimit": fmt.Sprintf("$%.2f", safeFloat(data, "spending_limit_usd")),
			"currentSpend":  fmt.Sprintf("$%.2f", safeFloat(data, "current_spend_usd")),
		}
	case domain.NotificationEventCostAnomaly:
		return "notification.cost_anomaly", map[string]any{
			"orgId":           safeStr(data, "org_id"),
			"severity":        safeStr(data, "severity"),
			"spikeRatio":      fmt.Sprintf("%.1fx", safeFloat(data, "spike_ratio")),
			"todaySpend":      fmt.Sprintf("%d micro-USD", int64(safeFloat(data, "today_spend"))),
			"sevenDayAverage": fmt.Sprintf("%d micro-USD", int64(safeFloat(data, "avg_7d_spend"))),
			"topContributor":  safeStr(data, "top_contributor"),
		}
	case domain.NotificationEventBudgetThreshold:
		return "notification.budget_threshold", map[string]any{
			"projectId":        safeStr(data, "project_id"),
			"thresholdPercent": fmt.Sprintf("%.0f%%", safeFloat(data, "threshold_pct")),
			"dailyCost":        fmt.Sprintf("%d micro-USD", int64(safeFloat(data, "daily_cost_microusd"))),
			"budgetLimit":      fmt.Sprintf("%d micro-USD", int64(safeFloat(data, "limit_microusd"))),
		}
	case notificationEventUsageForecastWarning:
		return "notification.usage_forecast_warning", map[string]any{
			"orgId":           safeStr(data, "org_id"),
			"daysUntilLimit":  int(safeFloat(data, "days_until_limit")),
			"recommendedPlan": safeStr(data, "recommended_plan"),
			"projectedRuns":   int64(safeFloat(data, "projected_runs")),
		}
	default:
		return genericEmailTemplate(eventType, payload)
	}
}

func genericEmailTemplate(eventType string, payload json.RawMessage) (string, map[string]any) {
	return "notification.generic", map[string]any{
		"eventType": eventType,
		"payload":   string(payload),
	}
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
