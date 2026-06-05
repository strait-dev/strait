package store

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
)

func (q *Queries) acquirePlanLimitLock(ctx context.Context, key string) error {
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()

	for {
		var locked bool
		if err := q.db.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtext($1))`, key).Scan(&locked); err != nil {
			return fmt.Errorf("acquire plan limit lock %q: %w", key, err)
		}
		if locked {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// EnforceCronScheduleLimit serializes org-wide cron schedule quota checks for
// jobs and workflows. Callers that are about to create or add a cron schedule
// must invoke this inside the same transaction as the following write.
func (q *Queries) EnforceCronScheduleLimit(ctx context.Context, orgID string, maxSchedules int) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.EnforceCronScheduleLimit")
	defer span.End()

	if orgID == "" || maxSchedules < 0 {
		return nil
	}
	if err := q.acquirePlanLimitLock(ctx, "cron_schedule_limit:"+orgID); err != nil {
		return err
	}

	count, err := q.CountCronJobsByOrg(ctx, orgID)
	if err != nil {
		return fmt.Errorf("count cron schedules before write: %w", err)
	}
	if count >= maxSchedules {
		return ErrCronScheduleLimitExceeded
	}
	return nil
}
