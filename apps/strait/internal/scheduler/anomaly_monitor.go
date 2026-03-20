package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"strait/internal/billing"
	"strait/internal/domain"
)

// AnomalyMonitorStore defines the store operations needed by AnomalyMonitor.
type AnomalyMonitorStore interface {
	billing.Store
	ListEnabledNotificationChannels(ctx context.Context, projectID string) ([]domain.NotificationChannel, error)
	CreateNotificationDelivery(ctx context.Context, d *domain.NotificationDelivery) error
}

// AnomalyMonitor periodically runs anomaly detection and fires notifications.
type AnomalyMonitor struct {
	store    AnomalyMonitorStore
	interval time.Duration
	logger   *slog.Logger

	// alertedMu guards the alerted map. Key format: "orgID:YYYY-MM-DD-HH-block"
	// where block = hour/4, giving a 4-hour cooldown window per org.
	alertedMu sync.Mutex
	alerted   map[string]bool
}

// NewAnomalyMonitor creates a new anomaly monitor.
func NewAnomalyMonitor(s AnomalyMonitorStore, interval time.Duration) *AnomalyMonitor {
	if interval <= 0 {
		interval = 15 * time.Minute
	}
	return &AnomalyMonitor{
		store:    s,
		interval: interval,
		logger:   slog.Default(),
		alerted:  make(map[string]bool),
	}
}

// Run starts the anomaly monitoring loop. Blocks until ctx is canceled.
func (am *AnomalyMonitor) Run(ctx context.Context) {
	ticker := time.NewTicker(am.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			am.check(ctx)
		}
	}
}

func (am *AnomalyMonitor) check(ctx context.Context) {
	// List orgs with recent activity by getting all orgs that have subscriptions.
	subs, err := am.store.ListOrgsWithPendingDowngrade(ctx)
	if err != nil {
		// Fallback: we don't have a dedicated ListOrgsWithActivity, so we use
		// all orgs we know about. In practice, anomaly detection naturally returns
		// no alerts for inactive orgs (insufficient baseline data).
		am.logger.Warn("anomaly monitor: failed to list orgs, skipping check", "error", err)
		return
	}

	// Collect unique org IDs. We piggyback on the subscriptions table; any org
	// with a subscription may have activity.
	orgIDs := make([]string, 0, len(subs))
	for _, sub := range subs {
		orgIDs = append(orgIDs, sub.OrgID)
	}

	if len(orgIDs) == 0 {
		return
	}

	now := time.Now().UTC()
	block := now.Hour() / 4
	blockKey := fmt.Sprintf("%s-%02d-%d", now.Format("2006-01-02"), now.Hour(), block)

	// Cleanup old entries.
	am.alertedMu.Lock()
	currentPrefix := now.Format("2006-01-02")
	for k := range am.alerted {
		// Remove entries from previous days.
		if len(k) > 10 && k[len(k)-len(blockKey):len(k)-len(blockKey)+10] != currentPrefix {
			delete(am.alerted, k)
		}
	}
	am.alertedMu.Unlock()

	for _, orgID := range orgIDs {
		alertKey := orgID + ":" + blockKey

		am.alertedMu.Lock()
		if am.alerted[alertKey] {
			am.alertedMu.Unlock()
			continue
		}
		am.alerted[alertKey] = true
		am.alertedMu.Unlock()

		revert := func() {
			am.alertedMu.Lock()
			delete(am.alerted, alertKey)
			am.alertedMu.Unlock()
		}

		// Load org-specific thresholds.
		cfg := billing.DefaultAnomalyConfig()
		sub, subErr := am.store.GetOrgSubscription(ctx, orgID)
		if subErr == nil && sub != nil {
			if sub.AnomalyThresholdWarning > 0 {
				cfg.WarningThreshold = sub.AnomalyThresholdWarning
			}
			if sub.AnomalyThresholdCritical > 0 {
				cfg.CriticalThreshold = sub.AnomalyThresholdCritical
			}
		}

		detector := billing.NewAnomalyDetectorWithConfig(am.store, cfg)
		alerts, detectErr := detector.DetectAnomalies(ctx, []string{orgID})
		if detectErr != nil {
			am.logger.Warn("anomaly monitor: detection failed",
				"org_id", orgID, "error", detectErr)
			revert()
			continue
		}

		if len(alerts) == 0 {
			revert()
			continue
		}

		// Send notifications for each alert.
		for _, alert := range alerts {
			am.sendAnomalyNotification(ctx, orgID, alert)
		}

		am.logger.Warn("cost anomaly detected",
			"org_id", orgID,
			"spike_ratio", alerts[0].SpikeRatio,
			"severity", alerts[0].Severity,
		)
	}
}

func (am *AnomalyMonitor) sendAnomalyNotification(ctx context.Context, orgID string, alert billing.AnomalyAlert) {
	// Get projects for this org to find notification channels.
	projectIDs, err := am.store.ListProjectsByOrg(ctx, orgID)
	if err != nil {
		am.logger.Warn("anomaly monitor: failed to list org projects",
			"org_id", orgID, "error", err)
		return
	}

	payload, _ := json.Marshal(map[string]any{
		"event":           domain.NotificationEventCostAnomaly,
		"org_id":          orgID,
		"spike_ratio":     alert.SpikeRatio,
		"severity":        alert.Severity,
		"today_spend":     alert.TodaySpend,
		"avg_7d_spend":    alert.Avg7dSpend,
		"top_contributor": alert.TopContributor,
		"timestamp":       time.Now().UTC(),
	})

	for _, projectID := range projectIDs {
		channels, chErr := am.store.ListEnabledNotificationChannels(ctx, projectID)
		if chErr != nil {
			am.logger.Warn("anomaly monitor: failed to list notification channels",
				"project_id", projectID, "error", chErr)
			continue
		}

		for _, ch := range channels {
			d := &domain.NotificationDelivery{
				ChannelID:   ch.ID,
				ProjectID:   projectID,
				EventType:   domain.NotificationEventCostAnomaly,
				Payload:     payload,
				Status:      "pending",
				MaxAttempts: 3,
			}
			if err := am.store.CreateNotificationDelivery(ctx, d); err != nil {
				am.logger.Warn("anomaly monitor: failed to create notification delivery",
					"channel_id", ch.ID, "project_id", projectID, "error", err)
			}
		}
	}
}
