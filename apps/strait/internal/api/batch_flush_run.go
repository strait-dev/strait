package api

import (
	"context"
	"encoding/json"
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
	payloads := make([]json.RawMessage, len(items))
	for i, item := range items {
		payloads[i] = item.Payload
	}
	return json.Marshal(map[string]any{"items": payloads})
}
