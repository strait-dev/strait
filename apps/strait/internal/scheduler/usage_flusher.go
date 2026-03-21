package scheduler

import (
	"context"
	"log/slog"
	"time"

	"strait/internal/billing"
)

// UsageFlusherStore defines the store operations needed by UsageFlusher.
type UsageFlusherStore interface {
	ListAllSubscribedOrgIDs(ctx context.Context) ([]string, error)
	GetOrgDailyUsage(ctx context.Context, orgID string, date time.Time) ([]billing.UsageRecord, error)
	UpsertUsageRecord(ctx context.Context, rec *billing.UsageRecord) error
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
			uf.flush(context.WithoutCancel(ctx))
		}
	}
}

func (uf *UsageFlusher) flush(ctx context.Context) {
	// Acquire advisory lock to prevent concurrent flushes across replicas.
	if uf.advisoryLocker != nil {
		acquired, err := uf.advisoryLocker.TryAdvisoryLock(ctx, usageFlusherLockID)
		if err != nil {
			uf.logger.Warn("usage flusher: failed to acquire advisory lock", "error", err)
			return
		}
		if !acquired {
			return
		}
		defer func() {
			if relErr := uf.advisoryLocker.ReleaseAdvisoryLock(ctx, usageFlusherLockID); relErr != nil {
				uf.logger.Warn("usage flusher: failed to release advisory lock", "error", relErr)
			}
		}()
	}

	orgIDs, err := uf.store.ListAllSubscribedOrgIDs(ctx)
	if err != nil {
		uf.logger.Warn("usage flusher: failed to list subscribed orgs", "error", err)
		return
	}

	if len(orgIDs) == 0 {
		return
	}

	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	for _, orgID := range orgIDs {
		records, dailyErr := uf.store.GetOrgDailyUsage(ctx, orgID, today)
		if dailyErr != nil {
			uf.logger.Warn("usage flusher: failed to get daily usage",
				"org_id", orgID, "error", dailyErr)
			continue
		}

		for i := range records {
			if err := uf.store.UpsertUsageRecord(ctx, &records[i]); err != nil {
				uf.logger.Warn("usage flusher: failed to upsert usage record",
					"org_id", orgID,
					"project_id", records[i].ProjectID,
					"error", err)
			}
		}
	}
}
