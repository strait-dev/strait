package notification

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"strait/internal/domain"

	"github.com/resend/resend-go/v2"
)

// mockResendClient implements ResendEmailAPI for testing.
type mockResendClient struct {
	sendFn func(ctx context.Context, params *resend.SendEmailRequest) (*resend.SendEmailResponse, error)
	calls  []*resend.SendEmailRequest
}

func (m *mockResendClient) SendWithContext(ctx context.Context, params *resend.SendEmailRequest) (*resend.SendEmailResponse, error) {
	m.calls = append(m.calls, params)
	if m.sendFn != nil {
		return m.sendFn(ctx, params)
	}
	return &resend.SendEmailResponse{Id: "msg-123"}, nil
}

func TestEmailSender_SendSuccess(t *testing.T) {
	t.Parallel()

	mock := &mockResendClient{}
	sender := NewEmailSenderWithClient(mock, "alerts@strait.dev")

	channel := &domain.NotificationChannel{
		ID:        "ch-1",
		ProjectID: "proj-1",
		Config:    json.RawMessage(`{"to":"user@example.com"}`),
	}
	delivery := &domain.NotificationDelivery{
		ID:        "d-1",
		EventType: domain.NotificationEventBudgetThreshold,
		Payload:   json.RawMessage(`{"project_id":"proj-1","daily_cost_microusd":85000}`),
	}

	err := sender.Send(context.Background(), channel, delivery)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(mock.calls) != 1 {
		t.Fatalf("expected 1 send call, got %d", len(mock.calls))
	}
	req := mock.calls[0]
	if len(req.To) != 1 || req.To[0] != "user@example.com" {
		t.Errorf("expected recipient user@example.com, got %v", req.To)
	}
	if req.From != "alerts@strait.dev" {
		t.Errorf("expected from alerts@strait.dev, got %s", req.From)
	}
}

func TestEmailSender_SendFailure_ReturnsError(t *testing.T) {
	t.Parallel()

	mock := &mockResendClient{
		sendFn: func(_ context.Context, _ *resend.SendEmailRequest) (*resend.SendEmailResponse, error) {
			return nil, errors.New("resend API error")
		},
	}
	sender := NewEmailSenderWithClient(mock, "alerts@strait.dev")

	channel := &domain.NotificationChannel{
		Config: json.RawMessage(`{"to":"user@example.com"}`),
	}
	delivery := &domain.NotificationDelivery{
		EventType: domain.NotificationEventBudgetThreshold,
		Payload:   json.RawMessage(`{}`),
	}

	err := sender.Send(context.Background(), channel, delivery)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, err) {
		t.Errorf("unexpected error type: %v", err)
	}
}

func TestEmailSender_MissingAPIKey_Fails(t *testing.T) {
	t.Parallel()

	_, err := NewEmailSender("", "noreply@strait.dev")
	if err == nil {
		t.Fatal("expected error when API key is empty, got nil")
	}
}

func TestEmailSender_FormatsBillingAlertBody(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		eventType     string
		payload       map[string]any
		wantInSubject string
		wantInBody    string
	}{
		{
			name:      "spending_limit_warning",
			eventType: domain.NotificationEventSpendingLimitWarning,
			payload: map[string]any{
				"org_id":             "org-1",
				"overage_pct":        80.0,
				"spending_limit_usd": 100.0,
				"current_spend_usd":  80.0,
			},
			wantInSubject: "80%",
			wantInBody:    "Spending Limit Warning",
		},
		{
			name:      "spending_limit_reached",
			eventType: domain.NotificationEventSpendingLimitReached,
			payload: map[string]any{
				"org_id":             "org-1",
				"spending_limit_usd": 100.0,
				"current_spend_usd":  100.0,
			},
			wantInSubject: "100%",
			wantInBody:    "Spending Limit Reached",
		},
		{
			name:      "anomaly_spike",
			eventType: domain.NotificationEventCostAnomaly,
			payload: map[string]any{
				"org_id":          "org-1",
				"spike_ratio":     5.0,
				"severity":        "high",
				"today_spend":     5000.0,
				"avg_7d_spend":    1000.0,
				"top_contributor": "proj-main",
			},
			wantInSubject: "anomaly",
			wantInBody:    "Cost Anomaly Detected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mock := &mockResendClient{}
			sender := NewEmailSenderWithClient(mock, "noreply@strait.dev")

			payloadBytes, _ := json.Marshal(tt.payload)
			channel := &domain.NotificationChannel{
				Config: json.RawMessage(`{"to":"user@example.com"}`),
			}
			delivery := &domain.NotificationDelivery{
				EventType: tt.eventType,
				Payload:   payloadBytes,
			}

			err := sender.Send(context.Background(), channel, delivery)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(mock.calls) != 1 {
				t.Fatalf("expected 1 call, got %d", len(mock.calls))
			}

			req := mock.calls[0]
			if !containsStr(req.Subject, tt.wantInSubject) {
				t.Errorf("subject %q does not contain %q", req.Subject, tt.wantInSubject)
			}
			if !containsStr(req.Html, tt.wantInBody) {
				t.Errorf("body does not contain %q", tt.wantInBody)
			}
		})
	}
}

func TestEmailSender_SetsCorrectFromAddress(t *testing.T) {
	t.Parallel()

	mock := &mockResendClient{}
	sender := NewEmailSenderWithClient(mock, "custom@example.com")

	channel := &domain.NotificationChannel{
		Config: json.RawMessage(`{"to":"user@example.com"}`),
	}
	delivery := &domain.NotificationDelivery{
		EventType: domain.NotificationEventBudgetThreshold,
		Payload:   json.RawMessage(`{"project_id":"proj-1"}`),
	}

	if err := sender.Send(context.Background(), channel, delivery); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.calls[0].From != "custom@example.com" {
		t.Errorf("expected from custom@example.com, got %s", mock.calls[0].From)
	}
}

func TestEmailSender_SetsCorrectSubjectLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		eventType string
		expected  string
	}{
		{domain.NotificationEventSpendingLimitWarning, "Spending limit warning - 80% reached"},
		{domain.NotificationEventSpendingLimitReached, "Spending limit reached - 100%"},
		{domain.NotificationEventCostAnomaly, "Cost anomaly detected - unusual spending spike"},
		{domain.NotificationEventBudgetThreshold, "Compute budget threshold reached"},
		{"unknown.event", "Strait notification: unknown.event"},
	}

	for _, tt := range tests {
		t.Run(tt.eventType, func(t *testing.T) {
			t.Parallel()

			mock := &mockResendClient{}
			sender := NewEmailSenderWithClient(mock, "noreply@strait.dev")

			channel := &domain.NotificationChannel{
				Config: json.RawMessage(`{"to":"user@example.com"}`),
			}
			delivery := &domain.NotificationDelivery{
				EventType: tt.eventType,
				Payload:   json.RawMessage(`{}`),
			}

			if err := sender.Send(context.Background(), channel, delivery); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if mock.calls[0].Subject != tt.expected {
				t.Errorf("expected subject %q, got %q", tt.expected, mock.calls[0].Subject)
			}
		})
	}
}

func TestEmailSender_DefaultFromAddress(t *testing.T) {
	t.Parallel()

	mock := &mockResendClient{}
	sender := NewEmailSenderWithClient(mock, "")

	channel := &domain.NotificationChannel{
		Config: json.RawMessage(`{"to":"user@example.com"}`),
	}
	delivery := &domain.NotificationDelivery{
		EventType: domain.NotificationEventBudgetThreshold,
		Payload:   json.RawMessage(`{}`),
	}

	if err := sender.Send(context.Background(), channel, delivery); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.calls[0].From != "noreply@strait.dev" {
		t.Errorf("expected default from noreply@strait.dev, got %s", mock.calls[0].From)
	}
}

func TestEmailSender_EmptyRecipient_Fails(t *testing.T) {
	t.Parallel()

	mock := &mockResendClient{}
	sender := NewEmailSenderWithClient(mock, "noreply@strait.dev")

	channel := &domain.NotificationChannel{
		Config: json.RawMessage(`{"to":""}`),
	}
	delivery := &domain.NotificationDelivery{
		EventType: domain.NotificationEventBudgetThreshold,
		Payload:   json.RawMessage(`{}`),
	}

	err := sender.Send(context.Background(), channel, delivery)
	if err == nil {
		t.Fatal("expected error for empty recipient, got nil")
	}
}

func TestEmailSender_InvalidConfig_Fails(t *testing.T) {
	t.Parallel()

	mock := &mockResendClient{}
	sender := NewEmailSenderWithClient(mock, "noreply@strait.dev")

	channel := &domain.NotificationChannel{
		Config: json.RawMessage(`not-json`),
	}
	delivery := &domain.NotificationDelivery{
		EventType: domain.NotificationEventBudgetThreshold,
		Payload:   json.RawMessage(`{}`),
	}

	err := sender.Send(context.Background(), channel, delivery)
	if err == nil {
		t.Fatal("expected error for invalid config, got nil")
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (substr == "" || indexOf(s, substr) >= 0)
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
