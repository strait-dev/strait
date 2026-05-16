package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"strait/internal/domain"
	"strait/internal/queue"
	"strait/internal/store"

	"github.com/google/uuid"
)

const batchAdvisoryLockID int64 = 900002

// BatchFlusher polls for flushable batch groups and creates runs with array payloads.
type BatchFlusher struct {
	store    store.BatchStore
	queue    queue.Queue
	interval time.Duration
}

func NewBatchFlusher(s store.BatchStore, q queue.Queue, interval time.Duration) *BatchFlusher {
	if interval <= 0 {
		interval = time.Second
	}
	return &BatchFlusher{store: s, queue: q, interval: interval}
}

func (f *BatchFlusher) Run(ctx context.Context) {
	ticker := time.NewTicker(f.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runSchedulerCycleCheckIn(ctx, f.interval, func() {
				f.poll(ctx)
			})
		}
	}
}

func (f *BatchFlusher) poll(ctx context.Context) {
	acquired, err := runWithOptionalAdvisoryLock(ctx, f.store, batchAdvisoryLockID, f.pollLocked)
	if err != nil {
		slog.Warn("batch flusher: advisory lock cycle failed", "error", err)
		return
	}
	if !acquired {
		return
	}
}

func (f *BatchFlusher) pollLocked(ctx context.Context) error {
	batches, err := f.store.ListFlushableBatches(ctx)
	if err != nil {
		slog.Error("batch flusher: list flushable", "error", err)
		return nil
	}

	for _, batch := range batches {
		if err := f.flush(ctx, batch); err != nil {
			slog.Error("batch flusher: flush", "job_id", batch.JobID, "batch_key", batch.BatchKey, "error", err)
		}
	}
	return nil
}

func (f *BatchFlusher) flush(ctx context.Context, batch store.FlushableBatch) error {
	job, err := f.store.GetJob(ctx, batch.JobID)
	if err != nil {
		return err
	}
	if !job.Enabled {
		return nil
	}

	limit := job.BatchMaxSize
	if limit <= 0 {
		limit = batch.ItemCount
	}

	items, err := f.store.ListBatchBufferItems(ctx, batch.JobID, batch.BatchKey, limit)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return nil
	}

	payloads := make([]json.RawMessage, len(items))
	for i, item := range items {
		payloads[i] = item.Payload
	}
	batchPayload, err := json.Marshal(map[string]any{"items": payloads})
	if err != nil {
		return err
	}

	now := time.Now()
	expiresAt := now.Add(time.Duration(job.TimeoutSecs)*time.Second + 60*time.Second)
	if job.RunTTLSecs > 0 {
		expiresAt = now.Add(time.Duration(job.RunTTLSecs) * time.Second)
	}

	run := &domain.JobRun{
		ID:             uuid.Must(uuid.NewV7()).String(),
		JobID:          batch.JobID,
		ProjectID:      batch.ProjectID,
		Tags:           job.Tags,
		Status:         domain.StatusQueued,
		Attempt:        1,
		Payload:        batchPayload,
		TriggeredBy:    "batch",
		Priority:       items[0].Priority,
		JobVersion:     job.Version,
		JobVersionID:   job.VersionID,
		ExpiresAt:      &expiresAt,
		CreatedBy:      items[0].CreatedBy,
		ExecutionMode:  job.ExecutionMode,
		QueueName:      job.Queue,
		IdempotencyKey: batchIdempotencyKey(batch, items),
	}

	err = queue.EnqueueWithRetry(ctx, f.queue, run, queue.DefaultInternalEnqueueRetryConfig())
	if err != nil && !errors.Is(err, domain.ErrIdempotencyConflict) {
		return err
	}
	return f.store.DeleteBatchBufferItems(ctx, batchItemIDs(items))
}

func batchItemIDs(items []domain.BatchBufferItem) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}

func batchIdempotencyKey(batch store.FlushableBatch, items []domain.BatchBufferItem) string {
	if len(items) == 0 {
		return ""
	}
	return "batch:" + batch.JobID + ":" + batch.BatchKey + ":" + items[0].ID + ":" + items[len(items)-1].ID
}
