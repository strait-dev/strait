package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"strait/internal/domain"
	"strait/internal/queue"
	"strait/internal/store"

	"github.com/google/uuid"
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
	var tags map[string]string
	if len(d.Tags) > 0 {
		_ = json.Unmarshal(d.Tags, &tags)
	}

	now := time.Now()
	var expiresAt time.Time
	if d.TTLSecs != nil && *d.TTLSecs > 0 {
		expiresAt = now.Add(time.Duration(*d.TTLSecs) * time.Second)
	} else if job.RunTTLSecs > 0 {
		expiresAt = now.Add(time.Duration(job.RunTTLSecs) * time.Second)
	} else {
		expiresAt = now.Add(time.Duration(job.TimeoutSecs)*time.Second + 60*time.Second)
	}

	run := &domain.JobRun{
		ID:             debounceRunID(d.ID),
		JobID:          d.JobID,
		ProjectID:      d.ProjectID,
		Tags:           tags,
		Status:         domain.StatusQueued,
		Attempt:        1,
		Payload:        d.Payload,
		TriggeredBy:    domain.TriggerDebounce,
		Priority:       d.Priority,
		ConcurrencyKey: d.ConcurrencyKey,
		JobVersion:     job.Version,
		JobVersionID:   job.VersionID,
		CreatedBy:      d.CreatedBy,
		ExpiresAt:      &expiresAt,
		ExecutionMode:  job.ExecutionMode,
		QueueName:      job.Queue,
		IdempotencyKey: "debounce:" + d.ID,
	}

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
	//nolint:nestif
	if quota != nil {
		if quota.MaxQueuedRuns > 0 {
			queued, countErr := p.store.CountProjectQueuedRuns(ctx, job.ProjectID)
			if countErr != nil {
				return countErr
			}
			if queued >= quota.MaxQueuedRuns {
				return queue.ErrEnqueueThrottled
			}
		}
		if quota.MaxExecutingRuns > 0 {
			active, countErr := p.store.CountProjectActiveRuns(ctx, job.ProjectID)
			if countErr != nil {
				return countErr
			}
			if active >= quota.MaxExecutingRuns {
				return queue.ErrEnqueueThrottled
			}
		}
		if quota.MaxDailyCostMicrousd > 0 {
			tz := quota.Timezone
			if tz == "" {
				tz = "UTC"
			}
			cost, costErr := p.store.SumProjectDailyCostMicrousd(ctx, job.ProjectID, tz)
			if costErr != nil {
				return costErr
			}
			if cost >= quota.MaxDailyCostMicrousd {
				return queue.ErrEnqueueThrottled
			}
		}
	}
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

func debounceRunID(pendingID string) string {
	if pendingID != "" {
		return pendingID
	}
	return uuid.Must(uuid.NewV7()).String()
}
