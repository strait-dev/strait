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

// SpendingLimitStore defines the store operations needed for org-level spending limit checks.
type SpendingLimitStore interface {
	ListAllSubscribedOrgIDs(ctx context.Context) ([]string, error)
	GetOrgSubscription(ctx context.Context, orgID string) (*billing.OrgSubscription, error)
	SumOrgPeriodSpend(ctx context.Context, orgID string, from time.Time) (int64, error)
	ListProjectsByOrg(ctx context.Context, orgID string) ([]string, error)
	ListEnabledNotificationChannels(ctx context.Context, projectID string) ([]domain.NotificationChannel, error)
	CreateNotificationDelivery(ctx context.Context, d *domain.NotificationDelivery) error
}

// BudgetMonitor periodically checks compute budget thresholds and fires alerts.
type BudgetMonitor struct {
	store         BudgetMonitorStore
	spendingStore SpendingLimitStore
	enqueuer      BudgetMonitorWebhookEnqueuer
	interval      time.Duration
	logger        *slog.Logger

	// alerted tracks which projects have already been alerted today.
	// Key format: "projectID:YYYY-MM-DD" or "spending:orgID:80:YYYY-MM-DD"
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

// WithSpendingLimitStore sets the store for org-level spending limit checks.
func (bm *BudgetMonitor) WithSpendingLimitStore(s SpendingLimitStore) *BudgetMonitor {
	bm.spendingStore = s
	return bm
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

	// Check org-level spending limits if the store is configured.
	if bm.spendingStore != nil {
		bm.checkSpendingLimits(ctx, today)
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

func (bm *BudgetMonitor) checkSpendingLimits(ctx context.Context, today string) {
	orgIDs, err := bm.spendingStore.ListAllSubscribedOrgIDs(ctx)
	if err != nil {
		bm.logger.Warn("budget monitor: failed to list subscribed orgs", "error", err)
		return
	}

	for _, orgID := range orgIDs {
		sub, subErr := bm.spendingStore.GetOrgSubscription(ctx, orgID)
		if subErr != nil || sub == nil {
			continue
		}

		// -1 means no spending limit configured; 0 means hard cap (free tier).
		if sub.SpendingLimitMicrousd == -1 {
			continue
		}
		if sub.SpendingLimitMicrousd == 0 {
			continue
		}

		limits := billing.GetPlanLimits(domain.PlanTier(sub.PlanTier))
		includedCredit := limits.ComputeCreditMicrousd

		periodStart := sub.CurrentPeriodStart
		if periodStart == nil {
			now := time.Now()
			periodStart = &now
		}

		periodSpend, spendErr := bm.spendingStore.SumOrgPeriodSpend(ctx, orgID, *periodStart)
		if spendErr != nil {
			bm.logger.Warn("budget monitor: failed to sum org period spend",
				"org_id", orgID, "error", spendErr)
			continue
		}

		overageSpend := max(periodSpend-includedCredit, 0)
		overagePct := float64(overageSpend) / float64(sub.SpendingLimitMicrousd) * 100

		// Check 100% first, then 80%.
		if overagePct >= 100 {
			alertKey := fmt.Sprintf("spending:%s:100:%s", orgID, today)
			bm.alertedMu.Lock()
			if bm.alerted[alertKey] {
				bm.alertedMu.Unlock()
				continue
			}
			bm.alerted[alertKey] = true
			bm.alertedMu.Unlock()

			bm.sendSpendingNotification(ctx, orgID, sub, overageSpend, overagePct, domain.NotificationEventSpendingLimitReached)
		} else if overagePct >= 80 {
			alertKey := fmt.Sprintf("spending:%s:80:%s", orgID, today)
			bm.alertedMu.Lock()
			if bm.alerted[alertKey] {
				bm.alertedMu.Unlock()
				continue
			}
			bm.alerted[alertKey] = true
			bm.alertedMu.Unlock()

			bm.sendSpendingNotification(ctx, orgID, sub, overageSpend, overagePct, domain.NotificationEventSpendingLimitWarning)
		}
	}
}

func (bm *BudgetMonitor) sendSpendingNotification(ctx context.Context, orgID string, sub *billing.OrgSubscription, overageSpend int64, overagePct float64, eventType string) {
	projectIDs, err := bm.spendingStore.ListProjectsByOrg(ctx, orgID)
	if err != nil {
		bm.logger.Warn("budget monitor: failed to list org projects",
			"org_id", orgID, "error", err)
		return
	}

	payload, _ := json.Marshal(map[string]any{
		"event":              eventType,
		"org_id":             orgID,
		"overage_pct":        overagePct,
		"spending_limit_usd": float64(sub.SpendingLimitMicrousd) / 1_000_000,
		"current_spend_usd":  float64(overageSpend) / 1_000_000,
		"timestamp":          time.Now().UTC(),
	})

	isLimitReached := eventType == domain.NotificationEventSpendingLimitReached

	for _, projectID := range projectIDs {
		channels, chErr := bm.spendingStore.ListEnabledNotificationChannels(ctx, projectID)
		if chErr != nil {
			bm.logger.Warn("budget monitor: failed to list notification channels",
				"project_id", projectID, "error", chErr)
			continue
		}

		for _, ch := range channels {
			// At 80%: only webhook channels. At 100%: webhook + email channels.
			if !isLimitReached && ch.ChannelType == domain.ChannelTypeEmail {
				continue
			}

			d := &domain.NotificationDelivery{
				ChannelID:   ch.ID,
				ProjectID:   projectID,
				EventType:   eventType,
				Payload:     payload,
				Status:      "pending",
				MaxAttempts: 3,
			}
			if err := bm.spendingStore.CreateNotificationDelivery(ctx, d); err != nil {
				bm.logger.Warn("budget monitor: failed to create spending notification delivery",
					"channel_id", ch.ID, "project_id", projectID, "error", err)
			}
		}
	}
}

// FormatBudgetAlertKey returns the dedup key for a project on a given date.
func FormatBudgetAlertKey(projectID string, date time.Time) string {
	return fmt.Sprintf("%s:%s", projectID, date.Format("2006-01-02"))
}
