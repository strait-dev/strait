package api

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"strait/internal/domain"

	"github.com/google/uuid"
)

const triggerJobRoute = "POST /v1/jobs/{jobID}/trigger"

type batchFlushRunRequest struct {
	input *TriggerJobInput
	job   *domain.Job
	req   TriggerRequest
	items []domain.BatchBufferItem
	now   time.Time
}

func newBatchFlushRun(ctx context.Context, request batchFlushRunRequest) *domain.JobRun {
	batchPayload, _ := batchFlushPayload(request.items)
	expiresAt := request.now.Add(time.Duration(request.job.TimeoutSecs)*time.Second + 60*time.Second)
	metadata := sentryRunMetadata(ctx, triggerJobRoute, nil)
	metadata = applyRunTraceHeaderMetadata(
		metadata,
		request.input.Traceparent,
		request.input.Tracestate,
		request.input.SentryTrace,
		request.input.Baggage,
	)

	return &domain.JobRun{
		ID:            uuid.Must(uuid.NewV7()).String(),
		JobID:         request.job.ID,
		ProjectID:     request.job.ProjectID,
		Status:        domain.StatusQueued,
		Attempt:       1,
		Payload:       batchPayload,
		TriggeredBy:   "batch",
		Priority:      request.req.Priority,
		JobVersion:    request.job.Version,
		JobVersionID:  request.job.VersionID,
		ExpiresAt:     &expiresAt,
		CreatedBy:     actorFromContext(ctx),
		ExecutionMode: request.job.ExecutionMode,
		QueueName:     request.job.Queue,
		IsRollback:    false,
		Metadata:      metadata,
	}
}

func batchFlushPayload(items []domain.BatchBufferItem) (json.RawMessage, error) {
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
		return nil, fmt.Errorf("batch flush payload item: invalid JSON")
	}
	return json.RawMessage(out), nil
}
