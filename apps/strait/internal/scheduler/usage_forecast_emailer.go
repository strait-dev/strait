package scheduler

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"strait/internal/billing"
	"strait/internal/domain"
)

// UsageForecastEmailerStore defines the store operations needed by UsageForecastEmailer.
type UsageForecastEmailerStore interface {
	billing.Store
	ListEnabledNotificationChannels(ctx context.Context, projectID string) ([]domain.NotificationChannel, error)
	ListEnabledNotificationChannelsByProjectIDs(ctx context.Context, projectIDs []string) (map[string][]domain.NotificationChannel, error)
	CreateNotificationDelivery(ctx context.Context, d *domain.NotificationDelivery) error
}

type UsageForecaster interface {
	GetUsageForecast(ctx context.Context, orgID string) (*billing.UsageForecastResponse, error)
}

// UsageForecastEmailer sends daily emails to subscribed orgs when their
// projected usage will hit a limit within 3 days.
type UsageForecastEmailer struct {
	store    UsageForecastEmailerStore
	usage    UsageForecaster
	interval time.Duration
	logger   *slog.Logger
	// lastRunDate prevents running more than once per day.
	lastRunDate string
}

// NewUsageForecastEmailer creates a new forecast emailer.
func NewUsageForecastEmailer(store UsageForecastEmailerStore, usage UsageForecaster, interval time.Duration) *UsageForecastEmailer {
	if interval <= 0 {
		interval = time.Hour
	}
	return &UsageForecastEmailer{
		store:    store,
		usage:    usage,
		interval: interval,
		logger:   slog.Default(),
	}
}

// Run starts the forecast email loop. Blocks until ctx is canceled.
func (fe *UsageForecastEmailer) Run(ctx context.Context) {
	ticker := time.NewTicker(fe.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runSchedulerCycleCheckIn(ctx, fe.interval, func() {
				fe.send(context.WithoutCancel(ctx))
			})
		}
	}
}

func (fe *UsageForecastEmailer) send(ctx context.Context) {
	today := time.Now().UTC().Format("2006-01-02")
	if fe.lastRunDate == today {
		return
	}
	fe.lastRunDate = today

	orgIDs, err := fe.store.ListAllSubscribedOrgIDs(ctx)
	if err != nil {
		fe.logger.Warn("forecast emailer: failed to list subscribed orgs", "error", err)
		return
	}

	for _, orgID := range orgIDs {
		forecast, fErr := fe.usage.GetUsageForecast(ctx, orgID)
		if fErr != nil {
			fe.logger.Warn("forecast emailer: failed to get forecast",
				"org_id", orgID, "error", fErr)
			continue
		}

		if forecast.DaysUntilLimit > 3 || forecast.DaysUntilLimit <= 0 {
			continue
		}

		// Send email notification for approaching limit.
		projectIDs, pErr := fe.store.ListProjectsByOrg(ctx, orgID)
		if pErr != nil {
			continue
		}

		payload, _ := json.Marshal(map[string]any{
			"event":            "usage.forecast_warning",
			"org_id":           orgID,
			"days_until_limit": forecast.DaysUntilLimit,
			"recommended_plan": forecast.RecommendedPlan,
			"projected_runs":   forecast.ProjectedMonthlyRuns,
			"timestamp":        time.Now().UTC(),
		})

		// Bulk-fetch all channels for all projects in one query (eliminates N+1).
		channelsByProject, chBulkErr := fe.store.ListEnabledNotificationChannelsByProjectIDs(ctx, projectIDs)
		if chBulkErr != nil {
			continue
		}

		for _, projectID := range projectIDs {
			for _, ch := range channelsByProject[projectID] {
				if ch.ChannelType != domain.ChannelTypeEmail {
					continue
				}
				d := &domain.NotificationDelivery{
					ChannelID:   ch.ID,
					ProjectID:   projectID,
					EventType:   "usage.forecast_warning",
					Payload:     payload,
					Status:      "pending",
					MaxAttempts: 3,
				}
				if err := fe.store.CreateNotificationDelivery(ctx, d); err != nil {
					fe.logger.Warn("forecast emailer: failed to create delivery",
						"channel_id", ch.ID, "org_id", orgID, "error", err)
				}
			}
		}
	}
}
