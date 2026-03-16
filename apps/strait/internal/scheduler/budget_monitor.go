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

	for _, pq := range projects {
		alertKey := pq.ProjectID + ":" + today

		bm.alertedMu.Lock()
		already := bm.alerted[alertKey]
		bm.alertedMu.Unlock()
		if already {
			continue
		}

		dailyCost, costErr := bm.store.SumDailyComputeCost(ctx, pq.ProjectID, pq.Timezone)
		if costErr != nil {
			bm.logger.Warn("budget monitor: failed to sum daily cost",
				"project_id", pq.ProjectID, "error", costErr)
			continue
		}

		threshold := pq.ComputeDailyCostLimitMicrousd * int64(domain.ComputeBudgetAlertThresholdPct) / 100
		if dailyCost < threshold {
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
				continue
			}
		}

		bm.alertedMu.Lock()
		bm.alerted[alertKey] = true
		// Clean old entries (anything not today).
		for k := range bm.alerted {
			if len(k) > 10 {
				dateStr := k[len(k)-10:]
				if dateStr != today {
					delete(bm.alerted, k)
				}
			}
		}
		bm.alertedMu.Unlock()
	}
}

// FormatBudgetAlertKey returns the dedup key for a project on a given date.
func FormatBudgetAlertKey(projectID string, date time.Time) string {
	return fmt.Sprintf("%s:%s", projectID, date.Format("2006-01-02"))
}
