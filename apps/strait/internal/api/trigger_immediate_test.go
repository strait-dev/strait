package api

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"strait/internal/domain"
)

func TestNewImmediateTriggerRunBuildsRunEnvelope(t *testing.T) {
	t.Parallel()

	scheduledAt := time.Date(2026, 6, 4, 13, 0, 0, 0, time.UTC)
	expiresAt := scheduledAt.Add(5 * time.Minute)
	ctx := context.WithValue(context.Background(), ctxActorIDKey, "apikey:trigger")
	srv := &Server{}
	state := &triggerRequestState{
		job: &domain.Job{
			ID:                 "job-1",
			ProjectID:          "project-1",
			Tags:               map[string]string{"team": "platform", "region": "default"},
			DefaultRunMetadata: map[string]string{"dependency_key": "default-dep", "retention": "short"},
			Version:            7,
			VersionID:          "version-7",
			ExecutionMode:      domain.ExecutionModeWorker,
			Queue:              "critical",
		},
		req: TriggerRequest{
			Tags:           map[string]string{"region": "eu"},
			Priority:       8,
			ConcurrencyKey: "customer-1",
		},
		payload:        json.RawMessage(`{"dependency_key":"payload-dep","ok":true}`),
		idempotencyKey: "idem-1",
	}
	input := &TriggerJobInput{
		Traceparent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00",
		Tracestate:  "vendor=value",
		SentryTrace: "trace",
		Baggage:     "tenant=project-1",
	}

	run := srv.newImmediateTriggerRun(ctx, input, state, immediateTriggerRunConfig{
		scheduledAt: &scheduledAt,
		expiresAt:   expiresAt,
		status:      domain.StatusDelayed,
	})

	if run.ID == "" {
		t.Fatal("run ID must be assigned")
	}
	if run.JobID != "job-1" || run.ProjectID != "project-1" {
		t.Fatalf("job/project = (%q, %q), want (job-1, project-1)", run.JobID, run.ProjectID)
	}
	if run.Status != domain.StatusDelayed || run.Attempt != 1 {
		t.Fatalf("status/attempt = (%s, %d), want (delayed, 1)", run.Status, run.Attempt)
	}
	if run.TriggeredBy != domain.TriggerManual {
		t.Fatalf("triggered_by = %q, want manual", run.TriggeredBy)
	}
	if run.CreatedBy != "apikey:trigger" {
		t.Fatalf("created_by = %q, want apikey:trigger", run.CreatedBy)
	}
	if run.Priority != 8 || run.ConcurrencyKey != "customer-1" || run.IdempotencyKey != "idem-1" {
		t.Fatalf("priority/concurrency/idempotency = (%d, %q, %q)", run.Priority, run.ConcurrencyKey, run.IdempotencyKey)
	}
	if run.JobVersion != 7 || run.JobVersionID != "version-7" {
		t.Fatalf("version = (%d, %q), want (7, version-7)", run.JobVersion, run.JobVersionID)
	}
	if run.ExecutionMode != domain.ExecutionModeWorker || run.QueueName != "critical" {
		t.Fatalf("execution/queue = (%s, %q), want (worker, critical)", run.ExecutionMode, run.QueueName)
	}
	if run.ScheduledAt == nil || !run.ScheduledAt.Equal(scheduledAt) {
		t.Fatalf("scheduled_at = %v, want %s", run.ScheduledAt, scheduledAt)
	}
	if run.ExpiresAt == nil || !run.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("expires_at = %v, want %s", run.ExpiresAt, expiresAt)
	}
	if string(run.Payload) != `{"dependency_key":"payload-dep","ok":true}` {
		t.Fatalf("payload = %s", run.Payload)
	}
	if run.Tags["team"] != "platform" || run.Tags["region"] != "eu" {
		t.Fatalf("tags = %+v, want base plus request override", run.Tags)
	}
	if run.Metadata["dependency_key"] != "payload-dep" {
		t.Fatalf("dependency_key metadata = %q, want payload-dep", run.Metadata["dependency_key"])
	}
	if run.Metadata["retention"] != "short" {
		t.Fatalf("retention metadata = %q, want short", run.Metadata["retention"])
	}
	if run.Metadata[domain.RunMetadataSentryRoute] != triggerJobRoute {
		t.Fatalf("route metadata = %q, want %q", run.Metadata[domain.RunMetadataSentryRoute], triggerJobRoute)
	}
	if run.Metadata[domain.RunMetadataTraceParent] != input.Traceparent {
		t.Fatalf("traceparent metadata = %q, want %q", run.Metadata[domain.RunMetadataTraceParent], input.Traceparent)
	}
	if run.Metadata[domain.RunMetadataSentryBaggage] != input.Baggage {
		t.Fatalf("baggage metadata = %q, want %q", run.Metadata[domain.RunMetadataSentryBaggage], input.Baggage)
	}
}

func TestExtractDependencyKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		payload json.RawMessage
		want    string
	}{
		{name: "empty", payload: nil, want: ""},
		{name: "invalid", payload: json.RawMessage(`{`), want: ""},
		{name: "missing", payload: json.RawMessage(`{"ok":true}`), want: ""},
		{name: "non-string", payload: json.RawMessage(`{"dependency_key":42}`), want: ""},
		{name: "string", payload: json.RawMessage(`{"dependency_key":"dep-1"}`), want: "dep-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := extractDependencyKey(tt.payload); got != tt.want {
				t.Fatalf("extractDependencyKey() = %q, want %q", got, tt.want)
			}
		})
	}
}
