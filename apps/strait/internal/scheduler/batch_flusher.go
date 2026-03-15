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
			f.poll(ctx)
		}
	}
}

func (f *BatchFlusher) poll(ctx context.Context) {
	acquired, err := f.store.TryAdvisoryLock(ctx, batchAdvisoryLockID)
	if err != nil || !acquired {
		return
	}
	defer func() {
		if err := f.store.ReleaseAdvisoryLock(ctx, batchAdvisoryLockID); err != nil {
			slog.Warn("failed to release batch advisory lock", "error", err)
		}
	}()

	batches, err := f.store.ListFlushableBatches(ctx)
	if err != nil {
		slog.Error("batch flusher: list flushable", "error", err)
		return
	}

	for _, batch := range batches {
		if err := f.flush(ctx, batch); err != nil {
			slog.Error("batch flusher: flush", "job_id", batch.JobID, "batch_key", batch.BatchKey, "error", err)
		}
	}
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

	items, err := f.store.DrainBatchBuffer(ctx, batch.JobID, batch.BatchKey, limit)
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
		ID:           uuid.Must(uuid.NewV7()).String(),
		JobID:        batch.JobID,
		ProjectID:    batch.ProjectID,
		Tags:         job.Tags,
		Status:       domain.StatusQueued,
		Attempt:      1,
		Payload:      batchPayload,
		TriggeredBy:  "batch",
		Priority:     items[0].Priority,
		JobVersion:   job.Version,
		JobVersionID: job.VersionID,
		ExpiresAt:    &expiresAt,
		CreatedBy:    items[0].CreatedBy,
	}

	return f.queue.Enqueue(ctx, run)
}
