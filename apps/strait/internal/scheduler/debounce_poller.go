package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"strait/internal/domain"
	"strait/internal/queue"
	"strait/internal/store"
)

const (
	debounceAdvisoryLockID        int64 = 900001
	debounceAdmissionRetryBackoff       = 5 * time.Minute
)

// DebouncePoller polls for due debounce_pending entries and creates runs.
type DebouncePoller struct {
	store    store.DebounceStore
	queue    queue.Queue
	interval time.Duration
}

type debounceAdmissionTransactioner interface {
	WithTx(ctx context.Context, fn func(context.Context, store.DBTX) error) error
}

func NewDebouncePoller(s store.DebounceStore, q queue.Queue, interval time.Duration) *DebouncePoller {
	if interval <= 0 {
		interval = time.Second
	}
	return &DebouncePoller{store: s, queue: q, interval: interval}
}

func (p *DebouncePoller) Run(ctx context.Context) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runSchedulerCycleCheckIn(ctx, p.interval, func() {
				p.poll(ctx)
			})
		}
	}
}

func (p *DebouncePoller) poll(ctx context.Context) {
	_, err := runWithOptionalAdvisoryLock(ctx, p.store, debounceAdvisoryLockID, p.pollLocked)
	if err != nil {
		slog.Warn("debounce poller: advisory lock cycle failed", "error", err)
		return
	}
}

func (p *DebouncePoller) pollLocked(ctx context.Context) error {
	items, err := p.store.ListDueDebouncePending(ctx)
	if err != nil {
		slog.Error("debounce poller: list due", "error", err)
		return nil
	}

	for _, item := range items {
		claimed, ok, err := p.store.ClaimDueDebouncePending(ctx, item.ID)
		if err != nil {
			slog.Error("debounce poller: claim", "id", item.ID, "job_id", item.JobID, "error", err)
			continue
		}
		if !ok {
			continue
		}
		if err := p.fireDebounce(ctx, *claimed); err != nil {
			slog.Error("debounce poller: fire", "id", item.ID, "job_id", item.JobID, "error", err)
			if errors.Is(err, queue.ErrEnqueueThrottled) {
				p.rescheduleThrottledPending(ctx, *claimed)
			}
			continue
		}
		completed, err := p.store.CompleteDebouncePending(ctx, claimed.ID, claimed.FireAt)
		if err != nil {
			slog.Error("debounce poller: complete claimed pending", "id", claimed.ID, "job_id", claimed.JobID, "error", err)
			continue
		}
		if !completed {
			slog.Info("debounce poller: claimed pending superseded before completion", "id", claimed.ID, "job_id", claimed.JobID)
		}
	}
	return nil
}

func (p *DebouncePoller) rescheduleThrottledPending(ctx context.Context, d domain.DebouncePending) {
	nextFireAt := time.Now().UTC().Add(debounceAdmissionRetryBackoff)
	rescheduled, err := p.store.RescheduleDebouncePending(ctx, d.ID, d.FireAt, nextFireAt)
	if err != nil {
		slog.Error("debounce poller: reschedule throttled pending", "id", d.ID, "job_id", d.JobID, "error", err)
		return
	}
	if !rescheduled {
		slog.Info("debounce poller: throttled pending superseded before reschedule", "id", d.ID, "job_id", d.JobID)
	}
}

func (p *DebouncePoller) fireDebounce(ctx context.Context, d domain.DebouncePending) error {
	job, err := p.store.GetJob(ctx, d.JobID)
	if err != nil {
		return err
	}
	if !job.Enabled {
		return nil
	}
	if job.Paused {
		return nil
	}
	run := newDebounceRun(d, job, time.Now())

	if err := p.withDebounceAdmissionGuard(ctx, job, func(enqueueCtx context.Context, tx store.DBTX) error {
		if tx != nil {
			return p.queue.EnqueueInTx(enqueueCtx, tx, run)
		}
		return queue.EnqueueWithRetry(enqueueCtx, p.queue, run, queue.DefaultInternalEnqueueRetryConfig())
	}); err != nil {
		existing, getErr := p.store.GetRun(ctx, run.ID)
		if getErr == nil && existing != nil {
			return nil
		}
		if getErr != nil && !errors.Is(getErr, store.ErrRunNotFound) {
			slog.Warn("debounce poller: failed to verify existing debounce run", "id", d.ID, "run_id", run.ID, "error", getErr)
		}
		return err
	}
	return nil
}

func (p *DebouncePoller) withDebounceAdmissionGuard(ctx context.Context, job *domain.Job, fn func(context.Context, store.DBTX) error) error {
	if txer, ok := p.store.(debounceAdmissionTransactioner); ok {
		return txer.WithTx(ctx, func(txCtx context.Context, tx store.DBTX) error {
			if _, err := tx.Exec(txCtx, "SELECT pg_advisory_xact_lock($1)", cronAdmissionLockID(job.ProjectID)); err != nil {
				return fmt.Errorf("acquire debounce admission lock: %w", err)
			}
			if err := p.checkFireLimits(txCtx, job); err != nil {
				return err
			}
			return fn(txCtx, tx)
		})
	}
	if err := p.checkFireLimits(ctx, job); err != nil {
		return err
	}
	return fn(ctx, nil)
}

func (p *DebouncePoller) checkFireLimits(ctx context.Context, job *domain.Job) error {
	quota, err := p.store.GetProjectQuota(ctx, job.ProjectID)
	if err != nil {
		return err
	}
	if err := p.checkProjectFireQuota(ctx, job, quota); err != nil {
		return err
	}
	return p.checkJobFireRateLimit(ctx, job)
}

func (p *DebouncePoller) checkProjectFireQuota(ctx context.Context, job *domain.Job, quota *store.ProjectQuota) error {
	if quota == nil {
		return nil
	}
	if err := p.checkProjectQueuedQuota(ctx, job.ProjectID, quota.MaxQueuedRuns); err != nil {
		return err
	}
	if err := p.checkProjectExecutingQuota(ctx, job.ProjectID, quota.MaxExecutingRuns); err != nil {
		return err
	}
	return p.checkProjectDailyCostQuota(ctx, job.ProjectID, quota)
}

func (p *DebouncePoller) checkProjectQueuedQuota(ctx context.Context, projectID string, limit int) error {
	if limit <= 0 {
		return nil
	}
	queued, err := p.store.CountProjectQueuedRuns(ctx, projectID)
	if err != nil {
		return err
	}
	if queued >= limit {
		return queue.ErrEnqueueThrottled
	}
	return nil
}

func (p *DebouncePoller) checkProjectExecutingQuota(ctx context.Context, projectID string, limit int) error {
	if limit <= 0 {
		return nil
	}
	active, err := p.store.CountProjectActiveRuns(ctx, projectID)
	if err != nil {
		return err
	}
	if active >= limit {
		return queue.ErrEnqueueThrottled
	}
	return nil
}

func (p *DebouncePoller) checkProjectDailyCostQuota(ctx context.Context, projectID string, quota *store.ProjectQuota) error {
	if quota.MaxDailyCostMicrousd <= 0 {
		return nil
	}
	tz := quota.Timezone
	if tz == "" {
		tz = "UTC"
	}
	cost, err := p.store.SumProjectDailyCostMicrousd(ctx, projectID, tz)
	if err != nil {
		return err
	}
	if cost >= quota.MaxDailyCostMicrousd {
		return queue.ErrEnqueueThrottled
	}
	return nil
}

func (p *DebouncePoller) checkJobFireRateLimit(ctx context.Context, job *domain.Job) error {
	if job.RateLimitMax > 0 && job.RateLimitWindowSecs > 0 {
		since := time.Now().Add(-time.Duration(job.RateLimitWindowSecs) * time.Second)
		count, countErr := p.store.CountRunsForJobSince(ctx, job.ID, since)
		if countErr != nil {
			return countErr
		}
		if count >= job.RateLimitMax {
			return queue.ErrEnqueueThrottled
		}
	}
	return nil
}
