package api

import (
	"context"
	"encoding/json"
	"maps"
	"testing"
	"time"

	"strait/internal/domain"
)

func TestNewBulkTriggerRun_BuildsDelayedRun(t *testing.T) {
	t.Parallel()

	ttlSecs := 120
	now := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	scheduledAt := now.Add(10 * time.Minute)
	ctx := context.WithValue(context.Background(), ctxActorIDKey, "user-bulk")
	job := &domain.Job{
		ID:                 "job-bulk",
		ProjectID:          "project-1",
		Tags:               map[string]string{"team": "platform", "tier": "base"},
		DefaultRunMetadata: map[string]string{"source": "job-default"},
		TimeoutSecs:        30,
		RunTTLSecs:         600,
		Version:            3,
		VersionID:          "version-3",
		ExecutionMode:      domain.ExecutionModeWorker,
		Queue:              "priority",
	}
	item := BulkTriggerItem{
		Tags:           map[string]string{"tier": "override", "region": "eu"},
		ScheduledAt:    &scheduledAt,
		Priority:       7,
		IdempotencyKey: "idem-1",
		TTLSecs:        &ttlSecs,
		ConcurrencyKey: "customer-1",
	}

	run := newBulkTriggerRun(ctx, bulkTriggerRunRequest{
		job:         job,
		item:        item,
		payload:     json.RawMessage(`{"n":1}`),
		batchID:     "batch-1",
		now:         now,
		scheduledAt: &scheduledAt,
	})

	if run.ID == "" {
		t.Fatal("run ID must be set")
	}
	if run.JobID != job.ID || run.ProjectID != job.ProjectID {
		t.Fatalf("job/project = (%q, %q), want (%q, %q)", run.JobID, run.ProjectID, job.ID, job.ProjectID)
	}
	if !maps.Equal(run.Tags, map[string]string{"team": "platform", "tier": "override", "region": "eu"}) {
		t.Fatalf("tags = %#v", run.Tags)
	}
	if run.Status != domain.StatusDelayed {
		t.Fatalf("status = %q, want delayed", run.Status)
	}
	if run.Attempt != 1 {
		t.Fatalf("attempt = %d, want 1", run.Attempt)
	}
	if string(run.Payload) != `{"n":1}` {
		t.Fatalf("payload = %s", run.Payload)
	}
	if run.TriggeredBy != domain.TriggerManual {
		t.Fatalf("triggered_by = %q, want manual", run.TriggeredBy)
	}
	if run.ScheduledAt == nil || !run.ScheduledAt.Equal(scheduledAt) {
		t.Fatalf("scheduled_at = %v, want %v", run.ScheduledAt, scheduledAt)
	}
	if run.Priority != item.Priority {
		t.Fatalf("priority = %d, want %d", run.Priority, item.Priority)
	}
	if run.IdempotencyKey != item.IdempotencyKey {
		t.Fatalf("idempotency key = %q, want %q", run.IdempotencyKey, item.IdempotencyKey)
	}
	if run.JobVersion != job.Version || run.JobVersionID != job.VersionID {
		t.Fatalf("version = (%d, %q), want (%d, %q)", run.JobVersion, run.JobVersionID, job.Version, job.VersionID)
	}
	if run.CreatedBy != "user-bulk" {
		t.Fatalf("created_by = %q, want user-bulk", run.CreatedBy)
	}
	if run.BatchID != "batch-1" {
		t.Fatalf("batch_id = %q, want batch-1", run.BatchID)
	}
	if run.ExpiresAt == nil || !run.ExpiresAt.Equal(now.Add(120*time.Second)) {
		t.Fatalf("expires_at = %v, want %v", run.ExpiresAt, now.Add(120*time.Second))
	}
	if run.ExecutionMode != domain.ExecutionModeWorker || run.QueueName != "priority" {
		t.Fatalf("execution = (%q, %q), want worker/priority", run.ExecutionMode, run.QueueName)
	}
	if run.ConcurrencyKey != item.ConcurrencyKey {
		t.Fatalf("concurrency key = %q, want %q", run.ConcurrencyKey, item.ConcurrencyKey)
	}
	if !maps.Equal(run.Metadata, job.DefaultRunMetadata) {
		t.Fatalf("metadata = %#v, want %#v", run.Metadata, job.DefaultRunMetadata)
	}
}

func TestNewBulkTriggerRun_BuildsQueuedRun(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	run := newBulkTriggerRun(context.Background(), bulkTriggerRunRequest{
		job: &domain.Job{
			ID:          "job-bulk",
			ProjectID:   "project-1",
			TimeoutSecs: 30,
		},
		item:    BulkTriggerItem{},
		payload: json.RawMessage(`{}`),
		batchID: "batch-1",
		now:     now,
	})

	if run.Status != domain.StatusQueued {
		t.Fatalf("status = %q, want queued", run.Status)
	}
	if run.ScheduledAt != nil {
		t.Fatalf("scheduled_at = %v, want nil", run.ScheduledAt)
	}
}

func TestBulkTriggerExpiresAt_Precedence(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	job := &domain.Job{TimeoutSecs: 30, RunTTLSecs: 600}
	ttlSecs := 120

	if got := bulkTriggerExpiresAt(job, BulkTriggerItem{TTLSecs: &ttlSecs}, now); !got.Equal(now.Add(120 * time.Second)) {
		t.Fatalf("item ttl expires_at = %v, want %v", got, now.Add(120*time.Second))
	}

	zeroTTL := 0
	if got := bulkTriggerExpiresAt(job, BulkTriggerItem{TTLSecs: &zeroTTL}, now); !got.Equal(now.Add(600 * time.Second)) {
		t.Fatalf("job ttl expires_at = %v, want %v", got, now.Add(600*time.Second))
	}

	job.RunTTLSecs = 0
	if got := bulkTriggerExpiresAt(job, BulkTriggerItem{}, now); !got.Equal(now.Add(90 * time.Second)) {
		t.Fatalf("timeout expires_at = %v, want %v", got, now.Add(90*time.Second))
	}
}
