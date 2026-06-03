package scheduler

import (
	"encoding/json"
	"testing"
	"time"

	"strait/internal/domain"
)

func TestNewDebounceRun_MapsPendingAndJobFields(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	ttl := 90
	pending := domain.DebouncePending{
		ID:             "pending-1",
		JobID:          "job-1",
		ProjectID:      "project-1",
		Payload:        json.RawMessage(`{"task":"sync"}`),
		Tags:           json.RawMessage(`{"tenant":"acme"}`),
		Priority:       7,
		ConcurrencyKey: "customer:42",
		CreatedBy:      "user-1",
		TTLSecs:        &ttl,
	}
	job := &domain.Job{
		Version:       4,
		VersionID:     "version-4",
		ExecutionMode: domain.ExecutionModeWorker,
		Queue:         "priority",
		TimeoutSecs:   300,
	}

	run := newDebounceRun(pending, job, now)

	if run.ID != pending.ID {
		t.Fatalf("ID = %q, want pending ID %q", run.ID, pending.ID)
	}
	if run.IdempotencyKey != "debounce:pending-1" {
		t.Fatalf("IdempotencyKey = %q, want debounce:pending-1", run.IdempotencyKey)
	}
	if run.JobID != pending.JobID || run.ProjectID != pending.ProjectID {
		t.Fatalf("job/project mismatch: got %s/%s", run.JobID, run.ProjectID)
	}
	if string(run.Payload) != string(pending.Payload) {
		t.Fatalf("Payload = %s, want %s", run.Payload, pending.Payload)
	}
	if run.Tags["tenant"] != "acme" {
		t.Fatalf("Tags = %v, want tenant=acme", run.Tags)
	}
	if run.Status != domain.StatusQueued || run.Attempt != 1 || run.TriggeredBy != domain.TriggerDebounce {
		t.Fatalf("run state = status %s attempt %d trigger %s", run.Status, run.Attempt, run.TriggeredBy)
	}
	if run.Priority != pending.Priority || run.ConcurrencyKey != pending.ConcurrencyKey || run.CreatedBy != pending.CreatedBy {
		t.Fatalf("pending fields not preserved: %+v", run)
	}
	if run.JobVersion != job.Version || run.JobVersionID != job.VersionID {
		t.Fatalf("job version = %d/%s, want %d/%s", run.JobVersion, run.JobVersionID, job.Version, job.VersionID)
	}
	if run.ExecutionMode != job.ExecutionMode || run.QueueName != job.Queue {
		t.Fatalf("dispatch fields = %s/%s, want %s/%s", run.ExecutionMode, run.QueueName, job.ExecutionMode, job.Queue)
	}
	wantExpiresAt := now.Add(90 * time.Second)
	if run.ExpiresAt == nil || !run.ExpiresAt.Equal(wantExpiresAt) {
		t.Fatalf("ExpiresAt = %v, want %s", run.ExpiresAt, wantExpiresAt)
	}
}

func TestDebounceRunExpiresAt_Precedence(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	pendingTTL := 30
	tests := []struct {
		name    string
		pending domain.DebouncePending
		job     *domain.Job
		want    time.Time
	}{
		{
			name:    "pending TTL wins",
			pending: domain.DebouncePending{TTLSecs: &pendingTTL},
			job:     &domain.Job{RunTTLSecs: 120, TimeoutSecs: 300},
			want:    now.Add(30 * time.Second),
		},
		{
			name:    "job run TTL wins over timeout fallback",
			pending: domain.DebouncePending{},
			job:     &domain.Job{RunTTLSecs: 120, TimeoutSecs: 300},
			want:    now.Add(120 * time.Second),
		},
		{
			name:    "timeout fallback includes grace minute",
			pending: domain.DebouncePending{},
			job:     &domain.Job{TimeoutSecs: 300},
			want:    now.Add(360 * time.Second),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := debounceRunExpiresAt(tc.pending, tc.job, now)
			if !got.Equal(tc.want) {
				t.Fatalf("expiresAt = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestDebounceRunTags_InvalidJSONReturnsNil(t *testing.T) {
	t.Parallel()

	tags := debounceRunTags(domain.DebouncePending{Tags: json.RawMessage(`{"broken"`)})
	if tags != nil {
		t.Fatalf("tags = %v, want nil for invalid JSON", tags)
	}
}

func TestDebounceRunID_GeneratesWhenPendingIDEmpty(t *testing.T) {
	t.Parallel()

	id := debounceRunID("")
	if id == "" {
		t.Fatal("expected generated debounce run ID")
	}
	if id == debounceRunID("") {
		t.Fatal("expected distinct generated debounce run IDs")
	}
}
