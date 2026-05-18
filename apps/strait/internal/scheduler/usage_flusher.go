package scheduler

import (
	"context"
	"log/slog"
	"time"

	"strait/internal/billing"

	"github.com/google/uuid"
)

// UsageFlusherStore defines the store operations needed by UsageFlusher.
type UsageFlusherStore interface {
	ListAllSubscribedOrgIDs(ctx context.Context) ([]string, error)
	GetOrgDailyUsage(ctx context.Context, orgID string, date time.Time) ([]billing.UsageRecord, error)
	ReplaceUsageRecord(ctx context.Context, rec *billing.UsageRecord) error
}

type flatUsageCostReconciler interface {
	ReconcileFlatUsageCosts(ctx context.Context, orgID string, date time.Time) error
}

// UsageFlusher periodically queries current-day usage for all subscribed orgs
// and upserts it into the usage_records table.
type UsageFlusher struct {
	store          UsageFlusherStore
	advisoryLocker AdvisoryLocker
	interval       time.Duration
	logger         *slog.Logger
}

// Advisory lock ID for the usage flusher (arbitrary unique constant).
const usageFlusherLockID int64 = 900_100_001
const usageFlusherReconcileLookbackDays = 35

// NewUsageFlusher creates a new usage flusher.
func NewUsageFlusher(store UsageFlusherStore, interval time.Duration) *UsageFlusher {
	if interval <= 0 {
		interval = 60 * time.Second
	}
	return &UsageFlusher{
		store:    store,
		interval: interval,
		logger:   slog.Default(),
	}
}

// WithAdvisoryLocker enables distributed single-leader flushing.
func (uf *UsageFlusher) WithAdvisoryLocker(locker AdvisoryLocker) *UsageFlusher {
	uf.advisoryLocker = locker
	return uf
}

// Run starts the usage flushing loop. Blocks until ctx is canceled.
func (uf *UsageFlusher) Run(ctx context.Context) {
	ticker := time.NewTicker(uf.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runSchedulerCycleCheckIn(ctx, uf.interval, func() {
				uf.flush(context.WithoutCancel(ctx))
			})
		}
	}
}

func (uf *UsageFlusher) flush(ctx context.Context) {
	_, err := runWithOptionalAdvisoryLock(ctx, uf.advisoryLocker, usageFlusherLockID, uf.flushLocked)
	if err != nil {
		uf.logger.Warn("usage flusher: advisory lock cycle failed", "error", err)
		return
	}
}

func (uf *UsageFlusher) flushLocked(ctx context.Context) error {
	orgIDs, err := uf.store.ListAllSubscribedOrgIDs(ctx)
	if err != nil {
		uf.logger.Warn("usage flusher: failed to list subscribed orgs", "error", err)
		return nil
	}

	if len(orgIDs) == 0 {
		return nil
	}

	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	for _, orgID := range uniqueNonEmptyStrings(orgIDs) {
		uf.reconcileFlatUsageCosts(ctx, orgID, today)

		records, dailyErr := uf.store.GetOrgDailyUsage(ctx, orgID, today)
		if dailyErr != nil {
			uf.logger.Warn("usage flusher: failed to get daily usage",
				"org_id", orgID, "error", dailyErr)
			continue
		}

		for i := range records {
			normalizeUsageSnapshot(&records[i], today)
			if err := uf.store.ReplaceUsageRecord(ctx, &records[i]); err != nil {
				uf.logger.Warn("usage flusher: failed to upsert usage record",
					"org_id", orgID,
					"project_id", records[i].ProjectID,
					"error", err)
			}
		}
	}
	return nil
}

func (uf *UsageFlusher) reconcileFlatUsageCosts(ctx context.Context, orgID string, today time.Time) {
	reconciler, ok := uf.store.(flatUsageCostReconciler)
	if !ok {
		return
	}
	for offset := usageFlusherReconcileLookbackDays - 1; offset >= 0; offset-- {
		periodDate := today.AddDate(0, 0, -offset)
		if reconcileErr := reconciler.ReconcileFlatUsageCosts(ctx, orgID, periodDate); reconcileErr != nil {
			uf.logger.Warn("usage flusher: failed to reconcile flat usage costs",
				"org_id", orgID, "period_date", periodDate, "error", reconcileErr)
		}
	}
}

func normalizeUsageSnapshot(rec *billing.UsageRecord, periodDate time.Time) {
	if rec.ID == "" {
		rec.ID = uuid.Must(uuid.NewV7()).String()
	}
	if rec.PeriodDate.IsZero() {
		rec.PeriodDate = periodDate
	}
	now := time.Now().UTC()
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = now
	}
	if rec.UpdatedAt.IsZero() {
		rec.UpdatedAt = now
	}
}

func uniqueNonEmptyStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
