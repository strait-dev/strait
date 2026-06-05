package api

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
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
	require.NotEmpty(t, run.ID)
	require.False(t, run.JobID !=
		"job-1" ||
		run.ProjectID !=
			"project-1")
	require.False(t, run.Status !=
		domain.StatusDelayed ||
		run.Attempt != 1)
	require.Equal(t, domain.TriggerManual,
		run.
			TriggeredBy,
	)
	require.Equal(t, "apikey:trigger",
		run.CreatedBy,
	)
	require.False(t, run.Priority !=
		8 || run.
		ConcurrencyKey !=
		"customer-1" ||
		run.
			IdempotencyKey != "idem-1")
	require.False(t, run.JobVersion !=
		7 || run.
		JobVersionID !=
		"version-7")
	require.False(t, run.ExecutionMode !=
		domain.
			ExecutionModeWorker ||
		run.QueueName !=
			"critical")
	require.False(t, run.ScheduledAt ==
		nil ||
		!run.ScheduledAt.
			Equal(scheduledAt),
	)
	require.False(t, run.ExpiresAt ==
		nil ||
		!run.ExpiresAt.
			Equal(expiresAt))
	require.JSONEq(t, `{"dependency_key":"payload-dep","ok":true}`,

		string(run.Payload))
	require.False(t, run.Tags["team"] != "platform" ||

		run.Tags["region"] != "eu",
	)
	require.Equal(t, "payload-dep",
		run.Metadata["dependency_key"])
	require.Equal(t, "short", run.
		Metadata["retention"])
	require.Equal(t, triggerJobRoute,
		run.Metadata[domain.
			RunMetadataSentryRoute])
	require.Equal(t, input.Traceparent,
		run.Metadata[domain.
			RunMetadataTraceParent],
	)
	require.Equal(t, input.Baggage,
		run.Metadata[domain.
			RunMetadataSentryBaggage])
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
			require.Equal(t, tt.want, extractDependencyKey(tt.
				payload))
		})
	}
}
