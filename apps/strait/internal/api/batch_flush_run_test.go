package api

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"strait/internal/domain"
)

func TestNewBatchFlushRun_BuildsQueuedRun(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	ctx := context.WithValue(context.Background(), ctxActorIDKey, "apikey:batch")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxRequestIDKey, "req-batch")

	job := &domain.Job{
		ID:            "job-batch",
		ProjectID:     "project-1",
		TimeoutSecs:   30,
		Version:       7,
		VersionID:     "version-7",
		ExecutionMode: domain.ExecutionModeWorker,
		Queue:         "critical",
	}
	input := &TriggerJobInput{
		Traceparent: "00-abcdef1234567890abcdef1234567890-fedcba0987654321-01",
		Tracestate:  "congo=t61rcWkgMzE",
		SentryTrace: "0123456789abcdef0123456789abcdef-0123456789abcdef-1",
		Baggage:     "sentry-release=test-release,sentry-public_key=public",
	}

	run := newBatchFlushRun(ctx, batchFlushRunRequest{
		input: input,
		job:   job,
		req:   TriggerRequest{Priority: 8},
		items: []domain.BatchBufferItem{
			{Payload: json.RawMessage(`{"n":1}`)},
			{Payload: json.RawMessage(`{"n":2}`)},
		},
		now: now,
	})

	if run.ID == "" {
		t.Fatal("run ID must be set")
	}
	if run.JobID != job.ID || run.ProjectID != job.ProjectID {
		t.Fatalf("run job/project = (%q, %q), want (%q, %q)", run.JobID, run.ProjectID, job.ID, job.ProjectID)
	}
	if run.Status != domain.StatusQueued {
		t.Fatalf("status = %q, want queued", run.Status)
	}
	if run.TriggeredBy != "batch" {
		t.Fatalf("triggered_by = %q, want batch", run.TriggeredBy)
	}
	if run.Priority != 8 {
		t.Fatalf("priority = %d, want 8", run.Priority)
	}
	if run.Attempt != 1 {
		t.Fatalf("attempt = %d, want 1", run.Attempt)
	}
	if string(run.Payload) != `{"items":[{"n":1},{"n":2}]}` {
		t.Fatalf("payload = %s", run.Payload)
	}
	if run.JobVersion != job.Version || run.JobVersionID != job.VersionID {
		t.Fatalf("version = (%d, %q), want (%d, %q)", run.JobVersion, run.JobVersionID, job.Version, job.VersionID)
	}
	if run.ExpiresAt == nil || !run.ExpiresAt.Equal(now.Add(90*time.Second)) {
		t.Fatalf("expires_at = %v, want %v", run.ExpiresAt, now.Add(90*time.Second))
	}
	if run.CreatedBy != "apikey:batch" {
		t.Fatalf("created_by = %q, want actor", run.CreatedBy)
	}
	if run.ExecutionMode != domain.ExecutionModeWorker || run.QueueName != "critical" {
		t.Fatalf("execution = (%q, %q), want worker/critical", run.ExecutionMode, run.QueueName)
	}
	if run.IsRollback {
		t.Fatal("batch flush runs must not be rollback runs")
	}
	if run.Metadata[domain.RunMetadataSentryRoute] != triggerJobRoute {
		t.Fatalf("route metadata = %q, want %q", run.Metadata[domain.RunMetadataSentryRoute], triggerJobRoute)
	}
	if run.Metadata[domain.RunMetadataSentryActorType] != "api_key" {
		t.Fatalf("actor type metadata = %q, want api_key", run.Metadata[domain.RunMetadataSentryActorType])
	}
	if run.Metadata[domain.RunMetadataSentryRequestID] != "req-batch" {
		t.Fatalf("request id metadata = %q, want req-batch", run.Metadata[domain.RunMetadataSentryRequestID])
	}
	if run.Metadata[domain.RunMetadataTraceParent] != input.Traceparent {
		t.Fatalf("traceparent metadata = %q, want header", run.Metadata[domain.RunMetadataTraceParent])
	}
	if run.Metadata[domain.RunMetadataTraceState] != input.Tracestate {
		t.Fatalf("tracestate metadata = %q, want header", run.Metadata[domain.RunMetadataTraceState])
	}
	if run.Metadata[domain.RunMetadataSentryTrace] != input.SentryTrace {
		t.Fatalf("sentry trace metadata = %q, want header", run.Metadata[domain.RunMetadataSentryTrace])
	}
	if run.Metadata[domain.RunMetadataSentryBaggage] != input.Baggage {
		t.Fatalf("baggage metadata = %q, want header", run.Metadata[domain.RunMetadataSentryBaggage])
	}
}

func TestBatchFlushPayload(t *testing.T) {
	t.Parallel()

	payload, err := batchFlushPayload([]domain.BatchBufferItem{
		{Payload: json.RawMessage(`{"a":1}`)},
		{Payload: json.RawMessage(`["b"]`)},
	})
	if err != nil {
		t.Fatalf("batchFlushPayload: %v", err)
	}
	if string(payload) != `{"items":[{"a":1},["b"]]}` {
		t.Fatalf("payload = %s", payload)
	}
}

func TestBatchFlushPayload_InvalidItem(t *testing.T) {
	t.Parallel()

	if _, err := batchFlushPayload([]domain.BatchBufferItem{{Payload: json.RawMessage(`{`)}}); err == nil {
		t.Fatal("expected invalid batch item payload to fail")
	}
}
