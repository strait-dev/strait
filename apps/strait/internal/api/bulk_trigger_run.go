package api

import (
	"context"
	"encoding/json"
	"time"

	"strait/internal/domain"

	"github.com/google/uuid"
)

type bulkTriggerRunRequest struct {
	job         *domain.Job
	item        BulkTriggerItem
	payload     json.RawMessage
	batchID     string
	now         time.Time
	scheduledAt *time.Time
}

func newBulkTriggerRun(ctx context.Context, request bulkTriggerRunRequest) *domain.JobRun {
	status := domain.StatusQueued
	if request.scheduledAt != nil && request.scheduledAt.After(request.now) {
		status = domain.StatusDelayed
	}

	expiresAt := bulkTriggerExpiresAt(request.job, request.item, request.now)
	run := &domain.JobRun{
		ID:             uuid.Must(uuid.NewV7()).String(),
		JobID:          request.job.ID,
		ProjectID:      request.job.ProjectID,
		Tags:           mergedRunTags(request.job.Tags, request.item.Tags),
		Status:         status,
		Attempt:        1,
		Payload:        request.payload,
		TriggeredBy:    domain.TriggerManual,
		ScheduledAt:    request.scheduledAt,
		Priority:       request.item.Priority,
		IdempotencyKey: request.item.IdempotencyKey,
		JobVersion:     request.job.Version,
		JobVersionID:   request.job.VersionID,
		CreatedBy:      actorFromContext(ctx),
		BatchID:        request.batchID,
		ExpiresAt:      &expiresAt,
		ExecutionMode:  request.job.ExecutionMode,
		QueueName:      request.job.Queue,
		ConcurrencyKey: request.item.ConcurrencyKey,
	}
	stampRunJobConfig(run, request.job)
	run.Metadata = applyDefaultRunMetadata(run.Metadata, request.job.DefaultRunMetadata)
	return run
}

func bulkTriggerExpiresAt(job *domain.Job, item BulkTriggerItem, now time.Time) time.Time {
	if item.TTLSecs != nil && *item.TTLSecs > 0 {
		return now.Add(time.Duration(*item.TTLSecs) * time.Second)
	}
	if job.RunTTLSecs > 0 {
		return now.Add(time.Duration(job.RunTTLSecs) * time.Second)
	}
	return now.Add(time.Duration(job.TimeoutSecs)*time.Second + 60*time.Second)
}
