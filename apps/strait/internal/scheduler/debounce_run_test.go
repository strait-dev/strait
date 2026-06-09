package scheduler

import (
	"encoding/json"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
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
	require.Equal(t, pending.ID, run.ID)
	require.Equal(t, "debounce:pending-1", run.IdempotencyKey)
	require.Equal(t, pending.JobID, run.JobID)
	require.Equal(t, pending.ProjectID, run.ProjectID)
	require.Equal(t, string(pending.Payload), string(run.Payload))
	require.Equal(t, "acme", run.Tags["tenant"])
	require.Equal(t, domain.StatusQueued, run.Status)
	require.Equal(t, 1, run.Attempt)
	require.Equal(t, domain.TriggerDebounce, run.TriggeredBy)
	require.Equal(t, pending.Priority, run.Priority)
	require.Equal(t, pending.ConcurrencyKey, run.ConcurrencyKey)
	require.Equal(t, pending.CreatedBy, run.CreatedBy)
	require.Equal(t, job.Version, run.JobVersion)
	require.Equal(t, job.VersionID, run.JobVersionID)
	require.Equal(t, job.ExecutionMode, run.ExecutionMode)
	require.Equal(t, job.Queue, run.QueueName)

	wantExpiresAt := now.Add(90 * time.Second)
	require.NotNil(t, run.ExpiresAt)
	require.True(t, run.ExpiresAt.Equal(wantExpiresAt))
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
			require.True(t, got.Equal(tc.want))
		})
	}
}

func TestDebounceRunTags_InvalidJSONReturnsNil(t *testing.T) {
	t.Parallel()

	tags := debounceRunTags(domain.DebouncePending{Tags: json.RawMessage(`{"broken"`)})
	require.Nil(t, tags)
}

func TestDebounceRunID_GeneratesWhenPendingIDEmpty(t *testing.T) {
	t.Parallel()

	id := debounceRunID("")
	require.NotEmpty(t, id)
	require.NotEqual(t, debounceRunID(""), id)
}
