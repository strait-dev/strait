package scheduler

import (
	"encoding/json"
	"time"

	"strait/internal/domain"

	"github.com/google/uuid"
)

func newDebounceRun(d domain.DebouncePending, job *domain.Job, now time.Time) *domain.JobRun {
	expiresAt := debounceRunExpiresAt(d, job, now)
	return &domain.JobRun{
		ID:             debounceRunID(d.ID),
		JobID:          d.JobID,
		ProjectID:      d.ProjectID,
		Tags:           debounceRunTags(d),
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
}

func debounceRunTags(d domain.DebouncePending) map[string]string {
	if len(d.Tags) == 0 {
		return nil
	}
	var tags map[string]string
	_ = json.Unmarshal(d.Tags, &tags)
	return tags
}

func debounceRunExpiresAt(d domain.DebouncePending, job *domain.Job, now time.Time) time.Time {
	if d.TTLSecs != nil && *d.TTLSecs > 0 {
		return now.Add(time.Duration(*d.TTLSecs) * time.Second)
	}
	if job.RunTTLSecs > 0 {
		return now.Add(time.Duration(job.RunTTLSecs) * time.Second)
	}
	return now.Add(time.Duration(job.TimeoutSecs)*time.Second + 60*time.Second)
}

func debounceRunID(pendingID string) string {
	if pendingID != "" {
		return pendingID
	}
	return uuid.Must(uuid.NewV7()).String()
}
