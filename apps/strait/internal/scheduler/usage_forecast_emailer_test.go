package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"strait/internal/billing"
	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockUsageForecaster struct {
	forecasts map[string]*billing.UsageForecastResponse
	err       error
}

func (m *mockUsageForecaster) GetUsageForecast(_ context.Context, orgID string) (*billing.UsageForecastResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.forecasts[orgID], nil
}

func TestNewUsageForecastEmailer_DefaultInterval(t *testing.T) {
	t.Parallel()

	emailer := NewUsageForecastEmailer(&mockReportStore{}, nil, 0)

	require.Equal(t, time.Hour, emailer.interval)
}

func TestUsageForecastEmailer_SendCreatesEmailDeliveries(t *testing.T) {
	t.Parallel()

	store := &mockReportStore{
		orgIDs: []string{"org-forecast"},
		projectIDsByOrg: map[string][]string{
			"org-forecast": {"project-1", "project-2"},
		},
		channelsByProject: map[string][]domain.NotificationChannel{
			"project-1": {
				{ID: "ch-email", ProjectID: "project-1", ChannelType: domain.ChannelTypeEmail},
				{ID: "ch-webhook", ProjectID: "project-1", ChannelType: domain.ChannelTypeWebhook},
			},
			"project-2": {
				{ID: "ch-email-2", ProjectID: "project-2", ChannelType: domain.ChannelTypeEmail},
			},
		},
	}
	forecaster := &mockUsageForecaster{
		forecasts: map[string]*billing.UsageForecastResponse{
			"org-forecast": {
				DaysUntilLimit:       2,
				RecommendedPlan:      string(domain.PlanStarter),
				ProjectedMonthlyRuns: 75_000,
			},
		},
	}
	emailer := NewUsageForecastEmailer(store, forecaster, time.Hour)

	emailer.send(context.Background())

	require.Len(t, store.deliveries, 2)
	assert.Equal(t, "ch-email", store.deliveries[0].ChannelID)
	assert.Equal(t, "project-1", store.deliveries[0].ProjectID)
	assert.Equal(t, "usage.forecast_warning", store.deliveries[0].EventType)
	assert.Equal(t, "pending", store.deliveries[0].Status)
	assert.Equal(t, 3, store.deliveries[0].MaxAttempts)
	assert.Equal(t, "ch-email-2", store.deliveries[1].ChannelID)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(store.deliveries[0].Payload, &payload))
	assert.Equal(t, "usage.forecast_warning", payload["event"])
	assert.Equal(t, "org-forecast", payload["org_id"])
	assert.InEpsilon(t, float64(2), payload["days_until_limit"], 0.001)
	assert.NotEmpty(t, payload["timestamp"])
}

func TestUsageForecastEmailer_SendSkipsOutsideWarningWindow(t *testing.T) {
	t.Parallel()

	store := &mockReportStore{
		orgIDs: []string{"org-low-usage"},
		projectIDsByOrg: map[string][]string{
			"org-low-usage": {"project-1"},
		},
		channelsByProject: map[string][]domain.NotificationChannel{
			"project-1": {{ID: "ch-email", ProjectID: "project-1", ChannelType: domain.ChannelTypeEmail}},
		},
	}
	forecaster := &mockUsageForecaster{
		forecasts: map[string]*billing.UsageForecastResponse{
			"org-low-usage": {DaysUntilLimit: 4},
		},
	}
	emailer := NewUsageForecastEmailer(store, forecaster, time.Hour)

	emailer.send(context.Background())

	require.Empty(t, store.deliveries)
}

func TestUsageForecastEmailer_SendSkipsWhenProjectOrChannelLookupFails(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		store *mockReportStore
	}{
		{
			name: "project list error",
			store: &mockReportStore{
				listProjectsErr: errors.New("projects unavailable"),
			},
		},
		{
			name: "channel bulk error",
			store: &mockReportStore{
				projectIDsByOrg: map[string][]string{"org-forecast": {"project-1"}},
				channelBulkErr:  errors.New("channels unavailable"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tt.store.orgIDs = []string{"org-forecast"}
			forecaster := &mockUsageForecaster{
				forecasts: map[string]*billing.UsageForecastResponse{
					"org-forecast": {DaysUntilLimit: 2},
				},
			}
			emailer := NewUsageForecastEmailer(tt.store, forecaster, time.Hour)

			emailer.send(context.Background())

			require.Empty(t, tt.store.deliveries)
		})
	}
}

func TestUsageForecastEmailer_SendRunsOncePerDay(t *testing.T) {
	t.Parallel()

	store := &mockReportStore{
		orgIDs: []string{"org-forecast"},
		projectIDsByOrg: map[string][]string{
			"org-forecast": {"project-1"},
		},
		channelsByProject: map[string][]domain.NotificationChannel{
			"project-1": {{ID: "ch-email", ProjectID: "project-1", ChannelType: domain.ChannelTypeEmail}},
		},
	}
	forecaster := &mockUsageForecaster{
		forecasts: map[string]*billing.UsageForecastResponse{
			"org-forecast": {DaysUntilLimit: 2},
		},
	}
	emailer := NewUsageForecastEmailer(store, forecaster, time.Hour)

	emailer.send(context.Background())
	emailer.send(context.Background())

	require.Len(t, store.deliveries, 1)
}
