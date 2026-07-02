package notification

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/transactional"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockNotificationEmailClient struct {
	sendFn func(ctx context.Context, req transactional.Request) error
	calls  []transactional.Request
}

func (m *mockNotificationEmailClient) Send(ctx context.Context, req transactional.Request) error {
	m.calls = append(m.calls, req)
	if m.sendFn != nil {
		return m.sendFn(ctx, req)
	}
	return nil
}

func transactionalPropsMap(t *testing.T, props any) map[string]any {
	t.Helper()
	payload, err := json.Marshal(props)
	require.NoError(t, err)
	var out map[string]any
	require.NoError(t, json.Unmarshal(payload, &out))
	return out
}

func TestEmailSender_SendSuccess(t *testing.T) {
	t.Parallel()

	mock := &mockNotificationEmailClient{}
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
	require.Len(t, mock.calls, 1)

	req := mock.calls[0]
	assert.Equal(t, []string{"user@example.com"}, req.To)
	assert.Equal(t, "alerts@strait.dev", req.From)
	assert.Equal(t, "notification.budget_threshold", string(req.Template))
	assert.Equal(t, "notification:d-1:budget.threshold_reached", req.IdempotencyKey)
}

func TestEmailSender_SendFailure_ReturnsError(t *testing.T) {
	t.Parallel()

	mock := &mockNotificationEmailClient{
		sendFn: func(_ context.Context, _ transactional.Request) error {
			return errors.New("app email API error")
		},
	}
	sender := NewEmailSenderWithClient(mock, "alerts@strait.dev")

	channel := &domain.NotificationChannel{Config: json.RawMessage(`{"to":"user@example.com"}`)}
	delivery := &domain.NotificationDelivery{
		EventType: domain.NotificationEventBudgetThreshold,
		Payload:   json.RawMessage(`{}`),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := sender.Send(ctx, channel, delivery)
	require.Error(t, err)
	require.ErrorContains(t, err, "send notification email through transactional endpoint")
}

func TestEmailSender_MissingClient_Fails(t *testing.T) {
	t.Parallel()

	_, err := NewEmailSender(nil, "noreply@strait.dev")
	require.Error(t, err)
}

func TestEmailSender_MapsEventPayloadsToTemplates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		eventType    string
		payload      map[string]any
		wantTemplate string
		wantProps    map[string]any
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
			wantTemplate: "notification.spending_limit_warning",
			wantProps: map[string]any{
				"orgId":          "org-1",
				"overagePercent": "80%",
				"spendingLimit":  "$100.00",
				"currentSpend":   "$80.00",
			},
		},
		{
			name:      "spending_limit_reached",
			eventType: domain.NotificationEventSpendingLimitReached,
			payload: map[string]any{
				"org_id":             "org-1",
				"spending_limit_usd": 100.0,
				"current_spend_usd":  100.0,
			},
			wantTemplate: "notification.spending_limit_reached",
			wantProps: map[string]any{
				"orgId":         "org-1",
				"spendingLimit": "$100.00",
				"currentSpend":  "$100.00",
			},
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
			wantTemplate: "notification.cost_anomaly",
			wantProps: map[string]any{
				"orgId":           "org-1",
				"spikeRatio":      "5.0x",
				"severity":        "high",
				"todaySpend":      "5000 micro-USD",
				"sevenDayAverage": "1000 micro-USD",
				"topContributor":  "proj-main",
			},
		},
		{
			name:      "usage forecast warning",
			eventType: notificationEventUsageForecastWarning,
			payload: map[string]any{
				"org_id":           "org-forecast",
				"days_until_limit": 2.0,
				"recommended_plan": "scale",
				"projected_runs":   1200000.0,
			},
			wantTemplate: "notification.usage_forecast_warning",
			wantProps: map[string]any{
				"orgId":           "org-forecast",
				"daysUntilLimit":  2,
				"recommendedPlan": "scale",
				"projectedRuns":   int64(1200000),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mock := &mockNotificationEmailClient{}
			sender := NewEmailSenderWithClient(mock, "noreply@strait.dev")

			payloadBytes, _ := json.Marshal(tt.payload)
			channel := &domain.NotificationChannel{Config: json.RawMessage(`{"to":"user@example.com"}`)}
			delivery := &domain.NotificationDelivery{
				ID:        "delivery-1",
				EventType: tt.eventType,
				Payload:   payloadBytes,
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			err := sender.Send(ctx, channel, delivery)
			require.NoError(t, err)
			require.Len(t, mock.calls, 1)

			req := mock.calls[0]
			assert.Equal(t, tt.wantTemplate, string(req.Template))
			props := transactionalPropsMap(t, req.Props)
			for key, want := range tt.wantProps {
				assert.EqualValues(t, want, props[key])
			}
		})
	}
}

func TestEmailSender_UnknownEventFallsBackToGenericTemplate(t *testing.T) {
	t.Parallel()

	mock := &mockNotificationEmailClient{}
	sender := NewEmailSenderWithClient(mock, "")

	channel := &domain.NotificationChannel{Config: json.RawMessage(`{"to":"user@example.com"}`)}
	delivery := &domain.NotificationDelivery{
		ID:        "d-unknown",
		EventType: "unknown.event",
		Payload:   json.RawMessage(`{"field":"value"}`),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, sender.Send(ctx, channel, delivery))
	require.Len(t, mock.calls, 1)
	assert.Equal(t, "noreply@strait.dev", mock.calls[0].From)
	assert.Equal(t, "notification.generic", string(mock.calls[0].Template))
	props := transactionalPropsMap(t, mock.calls[0].Props)
	assert.Equal(t, "unknown.event", props["eventType"])
	payload, ok := props["payload"].(string)
	require.True(t, ok)
	assert.JSONEq(t, `{"field":"value"}`, payload)
}

func TestEmailSender_EmptyRecipient_Fails(t *testing.T) {
	t.Parallel()

	mock := &mockNotificationEmailClient{}
	sender := NewEmailSenderWithClient(mock, "noreply@strait.dev")

	channel := &domain.NotificationChannel{Config: json.RawMessage(`{"to":""}`)}
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

	mock := &mockNotificationEmailClient{}
	sender := NewEmailSenderWithClient(mock, "noreply@strait.dev")

	channel := &domain.NotificationChannel{Config: json.RawMessage(`not-json`)}
	delivery := &domain.NotificationDelivery{
		EventType: domain.NotificationEventBudgetThreshold,
		Payload:   json.RawMessage(`{}`),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := sender.Send(ctx, channel, delivery)
	require.Error(t, err)
}
