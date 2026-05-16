package scheduler

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"strait/internal/domain"
	"strait/internal/queue"
	"strait/internal/store"

	"github.com/google/uuid"
)

const debounceAdvisoryLockID int64 = 900001

// DebouncePoller polls for due debounce_pending entries and creates runs.
type DebouncePoller struct {
	store    store.DebounceStore
	queue    queue.Queue
	interval time.Duration
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
	acquired, err := runWithOptionalAdvisoryLock(ctx, p.store, debounceAdvisoryLockID, p.pollLocked)
	if err != nil {
		slog.Warn("debounce poller: advisory lock cycle failed", "error", err)
		return
	}
	if !acquired {
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
		if err := p.fireDebounce(ctx, item); err != nil {
			slog.Error("debounce poller: fire", "id", item.ID, "job_id", item.JobID, "error", err)
			continue
		}
		if err := p.store.DeleteDebouncePending(ctx, item.ID); err != nil {
			slog.Error("debounce poller: delete", "id", item.ID, "error", err)
		}
	}
	return nil
}

func (p *DebouncePoller) fireDebounce(ctx context.Context, d domain.DebouncePending) error {
	job, err := p.store.GetJob(ctx, d.JobID)
	if err != nil {
		return err
	}
	if !job.Enabled {
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
		ID:             uuid.Must(uuid.NewV7()).String(),
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
	}

	return queue.EnqueueWithRetry(ctx, p.queue, run, queue.DefaultInternalEnqueueRetryConfig())
}
