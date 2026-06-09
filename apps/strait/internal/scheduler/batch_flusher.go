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

const batchAdvisoryLockID int64 = 900002

// BatchFlusher polls for flushable batch groups and creates runs with array payloads.
type BatchFlusher struct {
	store    store.BatchStore
	queue    queue.Queue
	interval time.Duration
}

type transactionalBatchStore interface {
	WithTx(ctx context.Context, fn func(context.Context, store.DBTX) error) error
	DrainBatchBufferInTx(ctx context.Context, tx store.DBTX, jobID, batchKey string, limit int) ([]domain.BatchBufferItem, error)
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
	_, err := runWithOptionalAdvisoryLock(ctx, f.store, batchAdvisoryLockID, f.pollLocked)
	if err != nil {
		slog.Warn("batch flusher: advisory lock cycle failed", "error", err)
		return
	}
}

func (f *BatchFlusher) pollLocked(ctx context.Context) error {
	batches, err := f.store.ListFlushableBatches(ctx)
	if err != nil {
		slog.Error("batch flusher: list flushable", "error", err)
		return nil
	}

	jobCache := make(map[string]*domain.Job)
	for _, batch := range batches {
		if err := f.flush(ctx, batch, jobCache); err != nil {
			slog.Error("batch flusher: flush", "job_id", batch.JobID, "batch_key", batch.BatchKey, "error", err)
		}
	}
	return nil
}

func (f *BatchFlusher) flush(ctx context.Context, batch store.FlushableBatch, jobCache map[string]*domain.Job) error {
	job, ok := jobCache[batch.JobID]
	if !ok {
		var err error
		job, err = f.store.GetJob(ctx, batch.JobID)
		if err != nil {
			return err
		}
		jobCache[batch.JobID] = job
	}
	if !job.Enabled {
		return nil
	}

	limit := job.BatchMaxSize
	if limit <= 0 {
		limit = batch.ItemCount
	}

	if txStore, ok := f.store.(transactionalBatchStore); ok {
		return f.flushInTx(ctx, txStore, batch, job, limit)
	}

	items, err := f.store.ListBatchBufferItems(ctx, batch.JobID, batch.BatchKey, limit)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return nil
	}

	batchPayload, err := marshalBatchPayload(items)
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

func (f *BatchFlusher) flushInTx(
	ctx context.Context,
	txStore transactionalBatchStore,
	batch store.FlushableBatch,
	job *domain.Job,
	limit int,
) error {
	return txStore.WithTx(ctx, func(txCtx context.Context, tx store.DBTX) error {
		items, err := txStore.DrainBatchBufferInTx(txCtx, tx, batch.JobID, batch.BatchKey, limit)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			return nil
		}
		run, err := buildBatchRun(batch, job, items)
		if err != nil {
			return err
		}
		err = f.queue.EnqueueInTx(txCtx, tx, run)
		if errors.Is(err, domain.ErrIdempotencyConflict) {
			return nil
		}
		return err
	})
}

func buildBatchRun(batch store.FlushableBatch, job *domain.Job, items []domain.BatchBufferItem) (*domain.JobRun, error) {
	batchPayload, err := marshalBatchPayload(items)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	expiresAt := now.Add(time.Duration(job.TimeoutSecs)*time.Second + 60*time.Second)
	if job.RunTTLSecs > 0 {
		expiresAt = now.Add(time.Duration(job.RunTTLSecs) * time.Second)
	}

	return &domain.JobRun{
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
	}, nil
}

func marshalBatchPayload(items []domain.BatchBufferItem) (json.RawMessage, error) {
	size := len(`{"items":[]}`)
	for _, item := range items {
		if item.Payload == nil {
			size += len("null")
			continue
		}
		size += len(item.Payload)
	}
	if len(items) > 1 {
		size += len(items) - 1
	}

	out := make([]byte, 0, size)
	out = append(out, `{"items":[`...)
	for i, item := range items {
		if i > 0 {
			out = append(out, ',')
		}
		if item.Payload == nil {
			out = append(out, "null"...)
			continue
		}
		out = append(out, item.Payload...)
	}
	out = append(out, `]}`...)
	if !json.Valid(out) {
		return nil, invalidBatchPayloadItemError(items)
	}
	return json.RawMessage(out), nil
}

func invalidBatchPayloadItemError(items []domain.BatchBufferItem) error {
	for _, item := range items {
		if item.Payload != nil && !json.Valid(item.Payload) {
			return fmt.Errorf("batch payload item %s: invalid JSON", item.ID)
		}
	}
	return fmt.Errorf("batch payload: invalid JSON")
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
