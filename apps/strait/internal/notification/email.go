package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"html"

	"strait/internal/domain"

	"github.com/resend/resend-go/v2"
)

// ResendEmailAPI is the subset of the Resend client used by EmailSender.
type ResendEmailAPI interface {
	SendWithContext(ctx context.Context, params *resend.SendEmailRequest) (*resend.SendEmailResponse, error)
}

// EmailSender sends notifications via Resend email.
type EmailSender struct {
	client    ResendEmailAPI
	fromEmail string
}

// NewEmailSender creates a new EmailSender. Returns an error if apiKey is empty.
func NewEmailSender(apiKey, fromEmail string) (*EmailSender, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("resend API key is required")
	}
	if fromEmail == "" {
		fromEmail = "noreply@strait.dev"
	}
	client := resend.NewClient(apiKey)
	return &EmailSender{
		client:    client.Emails,
		fromEmail: fromEmail,
	}, nil
}

// NewEmailSenderWithClient creates an EmailSender with a custom ResendEmailAPI.
// Useful for testing.
func NewEmailSenderWithClient(client ResendEmailAPI, fromEmail string) *EmailSender {
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
	var cfg emailConfig
	if err := json.Unmarshal(channel.Config, &cfg); err != nil {
		return fmt.Errorf("parse email config: %w", err)
	}
	if cfg.To == "" {
		return fmt.Errorf("email recipient address is empty")
	}

	subject := subjectForEvent(delivery.EventType, delivery.Payload)
	body := htmlBodyForEvent(delivery.EventType, delivery.Payload)

	req := &resend.SendEmailRequest{
		From:    e.fromEmail,
		To:      []string{cfg.To},
		Subject: subject,
		Html:    body,
	}

	_, err := e.client.SendWithContext(ctx, req)
	if err != nil {
		return fmt.Errorf("send email via resend: %w", err)
	}

	return nil
}

// subjectForEvent returns a human-readable email subject for the event type.
func subjectForEvent(eventType string, payload json.RawMessage) string {
	switch eventType {
	case domain.NotificationEventSpendingLimitWarning:
		return "Spending limit warning - 80% reached"
	case domain.NotificationEventSpendingLimitReached:
		return "Spending limit reached - 100%"
	case domain.NotificationEventCostAnomaly:
		return "Cost anomaly detected - unusual spending spike"
	case domain.NotificationEventBudgetThreshold:
		return "Compute budget threshold reached"
	default:
		return fmt.Sprintf("Strait notification: %s", eventType)
	}
}

// htmlBodyForEvent generates an HTML email body for a billing alert.
func htmlBodyForEvent(eventType string, payload json.RawMessage) string {
	var data map[string]any
	if err := json.Unmarshal(payload, &data); err != nil {
		return fmt.Sprintf("<p>Event: %s</p><pre>%s</pre>", html.EscapeString(eventType), html.EscapeString(string(payload)))
	}

	switch eventType {
	case domain.NotificationEventSpendingLimitWarning:
		return buildSpendingWarningHTML(data)
	case domain.NotificationEventSpendingLimitReached:
		return buildSpendingReachedHTML(data)
	case domain.NotificationEventCostAnomaly:
		return buildAnomalyHTML(data)
	case domain.NotificationEventBudgetThreshold:
		return buildBudgetThresholdHTML(data)
	default:
		return fmt.Sprintf("<p>Event: %s</p><pre>%s</pre>", html.EscapeString(eventType), html.EscapeString(string(payload)))
	}
}

func buildSpendingWarningHTML(data map[string]any) string {
	orgID := safeStr(data, "org_id")
	pct := safeFloat(data, "overage_pct")
	limitUsd := safeFloat(data, "spending_limit_usd")
	currentUsd := safeFloat(data, "current_spend_usd")

	return fmt.Sprintf(`<div style="font-family:sans-serif;max-width:600px;margin:0 auto">
<h2>Spending Limit Warning</h2>
<p>Your organization <strong>%s</strong> has reached <strong>%.0f%%</strong> of its monthly spending limit.</p>
<table style="border-collapse:collapse;width:100%%">
<tr><td style="padding:8px;border:1px solid #ddd">Current spend</td><td style="padding:8px;border:1px solid #ddd">$%.2f</td></tr>
<tr><td style="padding:8px;border:1px solid #ddd">Spending limit</td><td style="padding:8px;border:1px solid #ddd">$%.2f</td></tr>
</table>
<p>Consider adjusting your spending limit or reviewing resource usage.</p>
</div>`,
		html.EscapeString(orgID), pct, currentUsd, limitUsd)
}

func buildSpendingReachedHTML(data map[string]any) string {
	orgID := safeStr(data, "org_id")
	limitUsd := safeFloat(data, "spending_limit_usd")
	currentUsd := safeFloat(data, "current_spend_usd")

	return fmt.Sprintf(`<div style="font-family:sans-serif;max-width:600px;margin:0 auto">
<h2>Spending Limit Reached</h2>
<p>Your organization <strong>%s</strong> has reached its monthly spending limit of <strong>$%.2f</strong>.</p>
<table style="border-collapse:collapse;width:100%%">
<tr><td style="padding:8px;border:1px solid #ddd">Current spend</td><td style="padding:8px;border:1px solid #ddd">$%.2f</td></tr>
<tr><td style="padding:8px;border:1px solid #ddd">Spending limit</td><td style="padding:8px;border:1px solid #ddd">$%.2f</td></tr>
</table>
<p>New runs may be rejected until the next billing period. Increase your spending limit to continue.</p>
</div>`,
		html.EscapeString(orgID), limitUsd, currentUsd, limitUsd)
}

func buildAnomalyHTML(data map[string]any) string {
	orgID := safeStr(data, "org_id")
	severity := safeStr(data, "severity")
	spikeRatio := safeFloat(data, "spike_ratio")
	todaySpend := safeFloat(data, "today_spend")
	avg7d := safeFloat(data, "avg_7d_spend")
	topContrib := safeStr(data, "top_contributor")

	return fmt.Sprintf(`<div style="font-family:sans-serif;max-width:600px;margin:0 auto">
<h2>Cost Anomaly Detected</h2>
<p>A <strong>%s</strong>-severity spending spike of <strong>%.1fx</strong> was detected for organization <strong>%s</strong>.</p>
<table style="border-collapse:collapse;width:100%%">
<tr><td style="padding:8px;border:1px solid #ddd">Today's spend</td><td style="padding:8px;border:1px solid #ddd">%d micro-USD</td></tr>
<tr><td style="padding:8px;border:1px solid #ddd">7-day average</td><td style="padding:8px;border:1px solid #ddd">%d micro-USD</td></tr>
<tr><td style="padding:8px;border:1px solid #ddd">Spike ratio</td><td style="padding:8px;border:1px solid #ddd">%.1fx</td></tr>
<tr><td style="padding:8px;border:1px solid #ddd">Top contributor</td><td style="padding:8px;border:1px solid #ddd">%s</td></tr>
</table>
<p>Review your usage to ensure this activity is expected.</p>
</div>`,
		html.EscapeString(severity), spikeRatio,
		html.EscapeString(orgID),
		int64(todaySpend), int64(avg7d), spikeRatio,
		html.EscapeString(topContrib))
}

func buildBudgetThresholdHTML(data map[string]any) string {
	projectID := safeStr(data, "project_id")
	dailyCost := safeFloat(data, "daily_cost_microusd")
	limit := safeFloat(data, "limit_microusd")
	thresholdPct := safeFloat(data, "threshold_pct")

	return fmt.Sprintf(`<div style="font-family:sans-serif;max-width:600px;margin:0 auto">
<h2>Compute Budget Threshold Reached</h2>
<p>Project <strong>%s</strong> has exceeded <strong>%.0f%%</strong> of its daily compute budget.</p>
<table style="border-collapse:collapse;width:100%%">
<tr><td style="padding:8px;border:1px solid #ddd">Daily cost</td><td style="padding:8px;border:1px solid #ddd">%d micro-USD</td></tr>
<tr><td style="padding:8px;border:1px solid #ddd">Budget limit</td><td style="padding:8px;border:1px solid #ddd">%d micro-USD</td></tr>
</table>
</div>`,
		html.EscapeString(projectID), thresholdPct, int64(dailyCost), int64(limit))
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
	f, ok := v.(float64)
	if !ok {
		return 0
	}
	return f
}
