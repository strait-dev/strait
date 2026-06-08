package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

func (s *Server) handleDebounceTrigger(ctx context.Context, state *triggerRequestState) (*TriggerJobOutput, bool, error) {
	job := state.job
	req := state.req
	if job.DebounceWindowSecs <= 0 {
		return nil, false, nil
	}

	pending := newDebouncePending(ctx, debouncePendingRequest{
		job:     job,
		req:     req,
		payload: state.payload,
		now:     time.Now(),
	})
	if err := s.withTriggerLimitGuard(ctx, job, state.projectQuota, func(guardCtx context.Context, _ store.DBTX) error {
		return s.store.UpsertDebouncePending(guardCtx, pending)
	}); err != nil {
		return nil, true, triggerLimitAPIError(err, "failed to upsert debounce pending")
	}
	s.emitAuditEventAsync(auditContextWithProject(ctx, job.ProjectID), domain.AuditActionJobTriggered, "job", job.ID, map[string]any{
		"debounced":         true,
		"fire_at":           pending.FireAt,
		"priority":          req.Priority,
		"debounce_key_hash": hashIdempotencyKey(req.DebounceKey),
		"tag_keys":          tagKeys(req.Tags),
		"triggered_by":      domain.TriggerDebounce,
	})
	return &TriggerJobOutput{Body: map[string]any{
		"debounced": true,
		"fire_at":   pending.FireAt,
	}}, true, nil
}

func (s *Server) handleBatchTrigger(ctx context.Context, input *TriggerJobInput, state *triggerRequestState) (*TriggerJobOutput, bool, error) {
	job := state.job
	req := state.req
	if job.BatchWindowSecs <= 0 {
		return nil, false, nil
	}

	item := newBatchBufferItem(ctx, batchBufferItemRequest{
		job:     job,
		req:     req,
		payload: state.payload,
	})
	var batchOutput *TriggerJobOutput
	var batchRunID string
	if err := s.withTriggerLimitGuard(ctx, job, state.projectQuota, func(guardCtx context.Context, tx store.DBTX) error {
		if err := s.store.InsertBatchBufferItem(guardCtx, item); err != nil {
			return fmt.Errorf("insert batch buffer item: %w", err)
		}

		if job.BatchMaxSize <= 0 {
			return nil
		}
		count, countErr := s.store.CountBatchBufferItems(guardCtx, job.ID, req.BatchKey)
		if countErr != nil || count < job.BatchMaxSize {
			return countErr
		}
		items, drainErr := s.store.DrainBatchBuffer(guardCtx, job.ID, req.BatchKey, job.BatchMaxSize)
		if drainErr != nil || len(items) == 0 {
			return drainErr
		}
		batchRun := newBatchFlushRun(ctx, batchFlushRunRequest{
			input: input,
			job:   job,
			req:   req,
			items: items,
			now:   time.Now(),
		})
		if enqErr := s.enqueueTriggerRun(guardCtx, tx, batchRun); enqErr != nil {
			slog.Error("batch immediate flush enqueue failed", "job_id", job.ID, "error", enqErr)
			return enqErr
		}
		batchRunID = batchRun.ID
		batchOutput = &TriggerJobOutput{Body: map[string]any{
			"id":     batchRun.ID,
			"status": batchRun.Status,
			"batch":  true,
		}}
		return nil
	}); err != nil {
		if apiErr := enqueueAPIError(err); apiErr != nil {
			return nil, true, apiErr
		}
		return nil, true, triggerLimitAPIError(err, "failed to insert batch buffer item")
	}
	if batchOutput != nil {
		s.emitAuditEventAsync(auditContextWithProject(ctx, job.ProjectID), domain.AuditActionJobTriggered, "job", job.ID, map[string]any{
			"run_id":           batchRunID,
			"batch":            true,
			"priority":         req.Priority,
			"batch_key_hash":   hashIdempotencyKey(req.BatchKey),
			"tag_keys":         tagKeys(req.Tags),
			"triggered_by":     "batch",
			"batch_max_size":   job.BatchMaxSize,
			"batch_window_sec": job.BatchWindowSecs,
		})
		return batchOutput, true, nil
	}

	s.emitAuditEventAsync(auditContextWithProject(ctx, job.ProjectID), domain.AuditActionJobTriggered, "job", job.ID, map[string]any{
		"buffered":         true,
		"priority":         req.Priority,
		"batch_key_hash":   hashIdempotencyKey(req.BatchKey),
		"tag_keys":         tagKeys(req.Tags),
		"triggered_by":     "batch_buffer",
		"batch_window_sec": job.BatchWindowSecs,
	})
	return &TriggerJobOutput{Body: map[string]any{
		"buffered": true,
	}}, true, nil
}

type debouncePendingRequest struct {
	job     *domain.Job
	req     TriggerRequest
	payload json.RawMessage
	now     time.Time
}

func newDebouncePending(ctx context.Context, request debouncePendingRequest) *domain.DebouncePending {
	tagsJSON, _ := json.Marshal(request.req.Tags)
	return &domain.DebouncePending{
		JobID:          request.job.ID,
		ProjectID:      request.job.ProjectID,
		DebounceKey:    request.req.DebounceKey,
		Payload:        request.payload,
		Tags:           tagsJSON,
		Priority:       request.req.Priority,
		ConcurrencyKey: request.req.ConcurrencyKey,
		TTLSecs:        request.req.TTLSecs,
		TriggeredBy:    domain.TriggerDebounce,
		CreatedBy:      actorFromContext(ctx),
		FireAt:         request.now.Add(time.Duration(request.job.DebounceWindowSecs) * time.Second),
	}
}

type batchBufferItemRequest struct {
	job     *domain.Job
	req     TriggerRequest
	payload json.RawMessage
}

func newBatchBufferItem(ctx context.Context, request batchBufferItemRequest) *domain.BatchBufferItem {
	tagsJSON, _ := json.Marshal(request.req.Tags)
	return &domain.BatchBufferItem{
		JobID:       request.job.ID,
		ProjectID:   request.job.ProjectID,
		BatchKey:    request.req.BatchKey,
		Payload:     request.payload,
		Tags:        tagsJSON,
		Priority:    request.req.Priority,
		TriggeredBy: domain.TriggerManual,
		CreatedBy:   actorFromContext(ctx),
	}
}
