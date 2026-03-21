package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
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

// AnomalyCooldown manages per-org cooldown state for anomaly alerts.
type AnomalyCooldown interface {
	// InCooldown returns true if the org is within its cooldown window.
	InCooldown(ctx context.Context, orgID string) (bool, error)
	// SetCooldown marks the org as alerted, starting a cooldown window.
	SetCooldown(ctx context.Context, orgID string) error
}

// AnomalyMonitor periodically runs anomaly detection and fires notifications.
type AnomalyMonitor struct {
	store    AnomalyMonitorStore
	cooldown AnomalyCooldown
	interval time.Duration
	logger   *slog.Logger
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
	}
}

// WithCooldown sets the cooldown provider for deduplicating alerts.
func (am *AnomalyMonitor) WithCooldown(c AnomalyCooldown) *AnomalyMonitor {
	am.cooldown = c
	return am
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
	orgIDs, err := am.store.ListAllSubscribedOrgIDs(ctx)
	if err != nil {
		am.logger.Warn("anomaly monitor: failed to list subscribed orgs", "error", err)
		return
	}

	if len(orgIDs) == 0 {
		return
	}

	for _, orgID := range orgIDs {
		// Check cooldown before doing expensive anomaly detection.
		if am.cooldown != nil {
			cooled, cdErr := am.cooldown.InCooldown(ctx, orgID)
			if cdErr != nil {
				am.logger.Warn("anomaly monitor: cooldown check failed",
					"org_id", orgID, "error", cdErr)
				continue
			}
			if cooled {
				continue
			}
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
			continue
		}

		if len(alerts) == 0 {
			continue
		}

		// Set cooldown after successful detection.
		if am.cooldown != nil {
			if cdErr := am.cooldown.SetCooldown(ctx, orgID); cdErr != nil {
				am.logger.Warn("anomaly monitor: failed to set cooldown",
					"org_id", orgID, "error", cdErr)
			}
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

// RedisCooldown implements AnomalyCooldown using Redis SET with TTL.
type RedisCooldown struct {
	client RedisCooldownClient
	ttl    time.Duration
}

// RedisCooldownClient is the minimal Redis interface needed by RedisCooldown.
type RedisCooldownClient interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value any, ttl time.Duration) error
}

// NewRedisCooldown creates a Redis-backed anomaly cooldown with the given TTL.
func NewRedisCooldown(client RedisCooldownClient, ttl time.Duration) *RedisCooldown {
	if ttl <= 0 {
		ttl = 4 * time.Hour
	}
	return &RedisCooldown{client: client, ttl: ttl}
}

func cooldownKey(orgID string) string {
	return fmt.Sprintf("strait:anomaly_cooldown:%s", orgID)
}

func (r *RedisCooldown) InCooldown(ctx context.Context, orgID string) (bool, error) {
	val, err := r.client.Get(ctx, cooldownKey(orgID))
	if err != nil {
		// Treat "key not found" as not in cooldown.
		return false, nil
	}
	return val != "", nil
}

func (r *RedisCooldown) SetCooldown(ctx context.Context, orgID string) error {
	return r.client.Set(ctx, cooldownKey(orgID), "1", r.ttl)
}
