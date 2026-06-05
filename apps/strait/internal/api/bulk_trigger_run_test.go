package api

import (
	"context"
	"encoding/json"
	"maps"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
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
	require.NotEmpty(t, run.ID)
	require.False(t, run.JobID !=
		job.ID ||
		run.ProjectID !=
			job.
				ProjectID)
	require.True(
		t, maps.Equal(run.
			Tags, map[string]string{"team": "platform", "tier": "override",

			"region": "eu"}))
	require.Equal(t, domain.StatusDelayed,

		run.Status,
	)
	require.Equal(t, 1, run.Attempt)
	require.Equal(t, `{"n":1}`, string(run.
		Payload))
	require.Equal(t, domain.TriggerManual,

		run.TriggeredBy,
	)
	require.False(t, run.ScheduledAt ==
		nil ||
		!run.ScheduledAt.
			Equal(scheduledAt))
	require.Equal(t, item.Priority,
		run.Priority,
	)
	require.Equal(t, item.IdempotencyKey,

		run.IdempotencyKey,
	)
	require.False(t, run.JobVersion !=
		job.
			Version ||
		run.JobVersionID !=
			job.VersionID)
	require.Equal(t, "user-bulk",
		run.CreatedBy,
	)
	require.Equal(t, "batch-1", run.
		BatchID,
	)
	require.False(t, run.ExpiresAt ==
		nil ||
		!run.ExpiresAt.
			Equal(now.Add(120*time.Second)),
	)
	require.False(t, run.ExecutionMode !=
		domain.ExecutionModeWorker ||
		run.QueueName !=
			"priority",
	)
	require.Equal(t, item.ConcurrencyKey,

		run.ConcurrencyKey,
	)
	require.True(
		t, maps.Equal(run.
			Metadata,
			job.DefaultRunMetadata,
		))
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
	require.Equal(t, domain.StatusQueued,

		run.Status)
	require.Nil(t, run.ScheduledAt)
}

func TestBulkTriggerExpiresAt_Precedence(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	job := &domain.Job{TimeoutSecs: 30, RunTTLSecs: 600}
	ttlSecs := 120

	if got := bulkTriggerExpiresAt(job, BulkTriggerItem{TTLSecs: &ttlSecs}, now); !got.Equal(now.Add(120 * time.Second)) {
		require.Failf(t, "test failure",

			"item ttl expires_at = %v, want %v", got, now.Add(120*time.Second))
	}

	zeroTTL := 0
	if got := bulkTriggerExpiresAt(job, BulkTriggerItem{TTLSecs: &zeroTTL}, now); !got.Equal(now.Add(600 * time.Second)) {
		require.Failf(t, "test failure",

			"job ttl expires_at = %v, want %v", got, now.Add(600*time.Second))
	}

	job.RunTTLSecs = 0
	if got := bulkTriggerExpiresAt(job, BulkTriggerItem{}, now); !got.Equal(now.Add(90 * time.Second)) {
		require.Failf(t, "test failure",

			"timeout expires_at = %v, want %v", got, now.Add(90*time.Second))
	}
}
