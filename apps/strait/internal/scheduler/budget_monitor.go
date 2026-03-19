package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

// BudgetMonitorStore defines the store operations needed by BudgetMonitor.
type BudgetMonitorStore interface {
	ListProjectsWithComputeLimit(ctx context.Context) ([]store.ProjectComputeQuota, error)
	SumDailyComputeCost(ctx context.Context, projectID, timezone string) (int64, error)
}

// BudgetMonitorWebhookEnqueuer enqueues webhook deliveries for budget alerts.
type BudgetMonitorWebhookEnqueuer interface {
	EnqueueBudgetAlert(ctx context.Context, projectID string, payload json.RawMessage) error
}

// BudgetMonitor periodically checks compute budget thresholds and fires alerts.
type BudgetMonitor struct {
	store    BudgetMonitorStore
	enqueuer BudgetMonitorWebhookEnqueuer
	interval time.Duration
	logger   *slog.Logger

	// alerted tracks which projects have already been alerted today.
	// Key format: "projectID:YYYY-MM-DD"
	alertedMu sync.Mutex
	alerted   map[string]bool
}

// NewBudgetMonitor creates a new budget monitor.
func NewBudgetMonitor(s BudgetMonitorStore, enqueuer BudgetMonitorWebhookEnqueuer, interval time.Duration) *BudgetMonitor {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	return &BudgetMonitor{
		store:    s,
		enqueuer: enqueuer,
		interval: interval,
		logger:   slog.Default(),
		alerted:  make(map[string]bool),
	}
}

// Run starts the budget monitoring loop. Blocks until ctx is canceled.
func (bm *BudgetMonitor) Run(ctx context.Context) {
	ticker := time.NewTicker(bm.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			bm.check(ctx)
		}
	}
}

func (bm *BudgetMonitor) check(ctx context.Context) {
	projects, err := bm.store.ListProjectsWithComputeLimit(ctx)
	if err != nil {
		bm.logger.Warn("budget monitor: failed to list projects", "error", err)
		return
	}

	today := time.Now().UTC().Format("2006-01-02")

	// Cleanup old entries in a single pass under the lock.
	bm.alertedMu.Lock()
	for k := range bm.alerted {
		if len(k) > 10 {
			dateStr := k[len(k)-10:]
			if dateStr != today {
				delete(bm.alerted, k)
			}
		}
	}
	bm.alertedMu.Unlock()

	for _, pq := range projects {
		alertKey := pq.ProjectID + ":" + today

		// Optimistic lock: set alerted before the expensive query, revert on failure.
		bm.alertedMu.Lock()
		if bm.alerted[alertKey] {
			bm.alertedMu.Unlock()
			continue
		}
		bm.alerted[alertKey] = true
		bm.alertedMu.Unlock()

		revert := func() {
			bm.alertedMu.Lock()
			delete(bm.alerted, alertKey)
			bm.alertedMu.Unlock()
		}

		dailyCost, costErr := bm.store.SumDailyComputeCost(ctx, pq.ProjectID, pq.Timezone)
		if costErr != nil {
			bm.logger.Warn("budget monitor: failed to sum daily cost",
				"project_id", pq.ProjectID, "error", costErr)
			revert()
			continue
		}

		threshold := pq.ComputeDailyCostLimitMicrousd * int64(domain.ComputeBudgetAlertThresholdPct) / 100
		if dailyCost < threshold {
			revert()
			continue
		}

		bm.logger.Warn("compute budget threshold reached",
			"project_id", pq.ProjectID,
			"daily_cost_microusd", dailyCost,
			"limit_microusd", pq.ComputeDailyCostLimitMicrousd,
			"threshold_pct", domain.ComputeBudgetAlertThresholdPct,
		)

		if bm.enqueuer != nil {
			payload, _ := json.Marshal(map[string]any{
				"event":               domain.WebhookEventComputeBudgetWarning,
				"project_id":          pq.ProjectID,
				"daily_cost_microusd": dailyCost,
				"limit_microusd":      pq.ComputeDailyCostLimitMicrousd,
				"threshold_pct":       domain.ComputeBudgetAlertThresholdPct,
				"timestamp":           time.Now().UTC(),
			})
			if enqErr := bm.enqueuer.EnqueueBudgetAlert(ctx, pq.ProjectID, payload); enqErr != nil {
				bm.logger.Warn("budget monitor: failed to enqueue alert",
					"project_id", pq.ProjectID, "error", enqErr)
				revert()
				continue
			}
		}

		bm.sendBudgetNotification(ctx, pq.ProjectID, dailyCost, pq.ComputeDailyCostLimitMicrousd)
		// Alert succeeded — keep alertKey set (optimistic lock confirmed).
	}
}

// sendBudgetNotification sends budget threshold alerts via notification channels.
func (bm *BudgetMonitor) sendBudgetNotification(ctx context.Context, projectID string, dailyCost, limitMicrousd int64) {
	ns, ok := bm.store.(ApprovalNotifierStore)
	if !ok {
		return
	}

	channels, err := ns.ListEnabledNotificationChannels(ctx, projectID)
	if err != nil {
		bm.logger.Warn("budget monitor: failed to list notification channels", "project_id", projectID, "error", err)
		return
	}

	payload, _ := json.Marshal(map[string]any{
		"project_id":          projectID,
		"daily_cost_microusd": dailyCost,
		"limit_microusd":      limitMicrousd,
		"threshold_pct":       domain.ComputeBudgetAlertThresholdPct,
		"timestamp":           time.Now().UTC(),
	})

	for _, ch := range channels {
		d := &domain.NotificationDelivery{
			ChannelID:   ch.ID,
			ProjectID:   projectID,
			EventType:   domain.NotificationEventBudgetThreshold,
			Payload:     payload,
			Status:      "pending",
			MaxAttempts: 3,
		}
		if err := ns.CreateNotificationDelivery(ctx, d); err != nil {
			bm.logger.Warn("budget monitor: failed to create notification delivery",
				"channel_id", ch.ID, "project_id", projectID, "error", err)
		}
	}
}

// FormatBudgetAlertKey returns the dedup key for a project on a given date.
func FormatBudgetAlertKey(projectID string, date time.Time) string {
	return fmt.Sprintf("%s:%s", projectID, date.Format("2006-01-02"))
}
