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

// BudgetMonitorStore is the store interface required by BudgetMonitor.
// The compute-budget monitoring path was removed when run_compute_usage was dropped.
type BudgetMonitorStore = any

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
	ListEnabledNotificationChannelsByProjectIDs(ctx context.Context, projectIDs []string) (map[string][]domain.NotificationChannel, error)
	CreateNotificationDelivery(ctx context.Context, d *domain.NotificationDelivery) error
}

// RunLimitStore defines the store operations for monthly run allowance notifications.
type RunLimitStore interface {
	ListAllSubscribedOrgIDs(ctx context.Context) ([]string, error)
	ListProjectsByOrg(ctx context.Context, orgID string) ([]string, error)
	ListEnabledNotificationChannels(ctx context.Context, projectID string) ([]domain.NotificationChannel, error)
	ListEnabledNotificationChannelsByProjectIDs(ctx context.Context, projectIDs []string) (map[string][]domain.NotificationChannel, error)
	CreateNotificationDelivery(ctx context.Context, d *domain.NotificationDelivery) error
}

// BudgetMonitor periodically checks spending and run-allowance thresholds and fires alerts.
type BudgetMonitor struct {
	store         BudgetMonitorStore
	spendingStore SpendingLimitStore
	runLimitStore RunLimitStore
	enforcer      *billing.Enforcer
	enqueuer      BudgetMonitorWebhookEnqueuer
	interval      time.Duration
	logger        *slog.Logger

	// alerted tracks which thresholds have already been alerted for the active
	// daily or monthly period.
	alertedMu sync.Mutex
	alerted   map[string]bool
	// alertedDate records the UTC day represented by alerted. Cleanup is only
	// needed when this changes; scanning the whole map every tick scales poorly
	// for large installations with many daily alert keys.
	alertedDate string
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

// WithRunLimitNotifications enables proactive 80% monthly run allowance notifications.
func (bm *BudgetMonitor) WithRunLimitNotifications(s RunLimitStore, enforcer *billing.Enforcer) *BudgetMonitor {
	bm.runLimitStore = s
	bm.enforcer = enforcer
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
			runSchedulerCycleCheckIn(ctx, bm.interval, func() {
				bm.check(context.WithoutCancel(ctx))
			})
		}
	}
}

func (bm *BudgetMonitor) check(ctx context.Context) {
	now := time.Now().UTC()
	today := now.Format("2006-01-02")
	currentMonth := now.Format("2006-01")

	bm.pruneAlertedForPeriods(today, currentMonth)

	// Check org-level spending limits if the store is configured.
	if bm.spendingStore != nil {
		bm.checkSpendingLimits(ctx, today)
	}

	// Check 80% monthly run allowance and fire proactive notifications.
	if bm.runLimitStore != nil && bm.enforcer != nil {
		bm.checkRunLimitWarnings(ctx, currentMonth)
	}
}

func (bm *BudgetMonitor) pruneAlertedForPeriods(today, currentMonth string) {
	bm.alertedMu.Lock()
	defer bm.alertedMu.Unlock()

	if bm.alertedDate == today {
		return
	}
	if len(bm.alerted) == 0 {
		bm.alertedDate = today
		return
	}

	currentPeriodKeys := make(map[string]bool, len(bm.alerted))
	for k, alerted := range bm.alerted {
		if hasPeriodSuffix(k, today) || hasPeriodSuffix(k, currentMonth) {
			currentPeriodKeys[k] = alerted
		}
	}
	bm.alerted = currentPeriodKeys
	bm.alertedDate = today
}

func hasPeriodSuffix(key, period string) bool {
	return len(key) >= len(period) && key[len(key)-len(period):] == period
}

// checkSpendingLimits checks org-level spending limits and fires notifications.
// NOTE: The alerted map check has a small TOCTOU window — two concurrent callers
// could both pass the !alerted check before either sets the key. This is acceptable
// because (1) the scheduler runs as a single instance with advisory locks, and
// (2) worst case is 2 identical notifications on the same day, which is harmless.
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

		// Included compute credit is zero in orchestration-only mode; all spend is overage.
		var includedCredit int64

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
			if bm.isAlerted(alertKey) {
				continue
			}

			if bm.sendSpendingNotification(ctx, orgID, overagePct, domain.NotificationEventSpendingLimitReached, alertKey) {
				bm.markAlerted(alertKey)
			}
		} else if overagePct >= 80 {
			alertKey := fmt.Sprintf("spending:%s:80:%s", orgID, today)
			if bm.isAlerted(alertKey) {
				continue
			}

			if bm.sendSpendingNotification(ctx, orgID, overagePct, domain.NotificationEventSpendingLimitWarning, alertKey) {
				bm.markAlerted(alertKey)
			}
		}
	}
}

func (bm *BudgetMonitor) sendSpendingNotification(ctx context.Context, orgID string, overagePct float64, eventType string, alertKey string) bool {
	projectIDs, err := bm.spendingStore.ListProjectsByOrg(ctx, orgID)
	if err != nil {
		bm.logger.Warn("budget monitor: failed to list org projects",
			"org_id", orgID, "error", err)
		return false
	}

	payload := spendingNotificationPayload(eventType, overagePct)

	isLimitReached := eventType == domain.NotificationEventSpendingLimitReached

	// Bulk-fetch all channels for all projects in one query (eliminates N+1).
	channelsByProject, chBulkErr := bm.spendingStore.ListEnabledNotificationChannelsByProjectIDs(ctx, projectIDs)
	if chBulkErr != nil {
		bm.logger.Warn("budget monitor: failed to bulk-list notification channels",
			"org_id", orgID, "error", chBulkErr)
		return false
	}

	delivered := false
	for _, projectID := range projectIDs {
		for _, ch := range channelsByProject[projectID] {
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
				DedupeKey:   fmt.Sprintf("budget:%s:%s:%s", alertKey, projectID, ch.ID),
			}
			if err := bm.spendingStore.CreateNotificationDelivery(ctx, d); err != nil {
				bm.logger.Warn("budget monitor: failed to create spending notification delivery",
					"channel_id", ch.ID, "project_id", projectID, "error", err)
				continue
			}
			delivered = true
		}
	}
	return delivered
}

// checkRunLimitWarnings checks if any org has hit 80% of its monthly run allowance
// and sends a proactive notification so users aren't surprised by a hard block.
func (bm *BudgetMonitor) checkRunLimitWarnings(ctx context.Context, currentMonth string) {
	orgIDs, err := bm.runLimitStore.ListAllSubscribedOrgIDs(ctx)
	if err != nil {
		bm.logger.Warn("budget monitor: failed to list orgs for run limit check", "error", err)
		return
	}

	for _, orgID := range orgIDs {
		alertKey := fmt.Sprintf("runlimit:%s:80:%s", orgID, currentMonth)

		if bm.isAlerted(alertKey) {
			continue
		}

		warning, warnErr := bm.enforcer.Check80PercentMonthlyWarning(ctx, orgID)
		if warnErr != nil {
			continue
		}
		if !warning {
			continue
		}

		// Send notifications via org's project channels.
		projectIDs, projErr := bm.runLimitStore.ListProjectsByOrg(ctx, orgID)
		if projErr != nil {
			continue
		}

		payload := runLimitNotificationPayload()

		// Bulk-fetch all channels for all projects in one query (eliminates N+1).
		channelsByProject, chBulkErr := bm.runLimitStore.ListEnabledNotificationChannelsByProjectIDs(ctx, projectIDs)
		if chBulkErr != nil {
			bm.logger.Warn("budget monitor: failed to bulk-list notification channels",
				"org_id", orgID, "error", chBulkErr)
			continue
		}

		delivered := false
		for _, projectID := range projectIDs {
			for _, ch := range channelsByProject[projectID] {
				d := &domain.NotificationDelivery{
					ChannelID:   ch.ID,
					ProjectID:   projectID,
					EventType:   domain.NotificationEventRunLimitApproaching,
					Payload:     payload,
					Status:      "pending",
					MaxAttempts: 3,
					DedupeKey:   fmt.Sprintf("budget:%s:%s:%s", alertKey, projectID, ch.ID),
				}
				if err := bm.runLimitStore.CreateNotificationDelivery(ctx, d); err != nil {
					bm.logger.Warn("budget monitor: failed to create run limit notification",
						"channel_id", ch.ID, "project_id", projectID, "error", err)
					continue
				}
				delivered = true
			}
		}
		if delivered {
			bm.markAlerted(alertKey)
		}
	}
}

func (bm *BudgetMonitor) isAlerted(alertKey string) bool {
	bm.alertedMu.Lock()
	defer bm.alertedMu.Unlock()
	return bm.alerted[alertKey]
}

func (bm *BudgetMonitor) markAlerted(alertKey string) {
	bm.alertedMu.Lock()
	defer bm.alertedMu.Unlock()
	bm.alerted[alertKey] = true
}

func spendingNotificationPayload(eventType string, overagePct float64) json.RawMessage {
	threshold := 80
	if overagePct >= 100 {
		threshold = 100
	}
	payload, _ := json.Marshal(map[string]any{
		"event":         eventType,
		"threshold_pct": threshold,
		"timestamp":     time.Now().UTC(),
	})
	return payload
}

func runLimitNotificationPayload() json.RawMessage {
	payload, _ := json.Marshal(map[string]any{
		"event":         domain.NotificationEventRunLimitApproaching,
		"threshold_pct": 80,
		"timestamp":     time.Now().UTC(),
	})
	return payload
}

// FormatBudgetAlertKey returns the dedup key for a project on a given date.
func FormatBudgetAlertKey(projectID string, date time.Time) string {
	return fmt.Sprintf("%s:%s", projectID, date.Format("2006-01-02"))
}
