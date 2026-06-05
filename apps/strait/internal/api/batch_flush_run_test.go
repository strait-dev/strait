package api

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
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
	require.NotEqual(t, "", run.ID)
	require.False(t, run.JobID !=
		job.ID || run.
		ProjectID !=
		job.ProjectID)
	require.Equal(t, domain.StatusQueued,
		run.
			Status)
	require.Equal(t, "batch", run.
		TriggeredBy,
	)
	require.EqualValues(t, 8, run.Priority)
	require.EqualValues(t, 1, run.Attempt)
	require.Equal(t, `{"items":[{"n":1},{"n":2}]}`,

		string(run.Payload))
	require.False(t, run.JobVersion !=
		job.Version ||

		run.JobVersionID != job.VersionID,
	)
	require.False(t, run.ExpiresAt ==
		nil ||
		!run.ExpiresAt.
			Equal(now.Add(90*time.
				Second,
			)))
	require.Equal(t, "apikey:batch",
		run.CreatedBy,
	)
	require.False(t, run.ExecutionMode !=
		domain.
			ExecutionModeWorker ||
		run.QueueName !=
			"critical")
	require.False(t, run.IsRollback)
	require.Equal(t, triggerJobRoute,
		run.Metadata[domain.
			RunMetadataSentryRoute])
	require.Equal(t, "api_key", run.
		Metadata[domain.RunMetadataSentryActorType])
	require.Equal(t, "req-batch",
		run.Metadata[domain.
			RunMetadataSentryRequestID])
	require.Equal(t, input.Traceparent,
		run.Metadata[domain.
			RunMetadataTraceParent],
	)
	require.Equal(t, input.Tracestate,
		run.Metadata[domain.
			RunMetadataTraceState])
	require.Equal(t, input.SentryTrace,
		run.Metadata[domain.
			RunMetadataSentryTrace],
	)
	require.Equal(t, input.Baggage,
		run.Metadata[domain.
			RunMetadataSentryBaggage])

}

func TestBatchFlushPayload(t *testing.T) {
	t.Parallel()

	payload, err := batchFlushPayload([]domain.BatchBufferItem{
		{Payload: json.RawMessage(`{"a":1}`)},
		{Payload: json.RawMessage(`["b"]`)},
	})
	require.NoError(t, err)
	require.Equal(t, `{"items":[{"a":1},["b"]]}`,

		string(payload))

}

func TestBatchFlushPayload_InvalidItem(t *testing.T) {
	t.Parallel()

	if _, err := batchFlushPayload([]domain.BatchBufferItem{{Payload: json.RawMessage(`{`)}}); err == nil {
		require.Fail(t,

			"expected invalid batch item payload to fail")
	}
}
