package notification

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/resend/resend-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := sender.Send(ctx, channel, delivery)
	require.NoError(t, err)
	require.Len(t, mock.calls,
		1)

	req := mock.calls[0]
	assert.False(t, len(req.
		To) != 1 || req.
		To[0] !=
		"user@example.com",
	)
	assert.Equal(t, "alerts@strait.dev",
		req.
			From)

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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := sender.Send(ctx, channel, delivery)
	require.Error(t, err)
	assert.True(t, errors.Is(err, err))

}

func TestEmailSender_MissingAPIKey_Fails(t *testing.T) {
	t.Parallel()

	_, err := NewEmailSender("", "noreply@strait.dev")
	require.Error(t, err)

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

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			err := sender.Send(ctx, channel, delivery)
			require.NoError(t, err)
			require.Len(t, mock.calls,
				1)

			req := mock.calls[0]
			assert.True(t, containsStr(req.Subject,
				tt.wantInSubject,
			))
			assert.True(t, containsStr(req.Html, tt.
				wantInBody,
			))

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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, sender.
		Send(ctx, channel,
			delivery,
		))
	assert.Equal(t, "custom@example.com",
		mock.
			calls[0].From)

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

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			require.NoError(t, sender.
				Send(ctx, channel,
					delivery,
				))
			assert.Equal(t, tt.expected,
				mock.calls[0].Subject,
			)

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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, sender.
		Send(ctx, channel,
			delivery,
		))
	assert.Equal(t, "noreply@strait.dev",
		mock.
			calls[0].From)

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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := sender.Send(ctx, channel, delivery)
	require.Error(t, err)

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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := sender.Send(ctx, channel, delivery)
	require.Error(t, err)

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
