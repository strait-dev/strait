package api

import (
	"context"
	"encoding/json"
	"time"

	"strait/internal/domain"
)

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
