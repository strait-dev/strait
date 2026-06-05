package worker

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/stretchr/testify/require"

	"strait/internal/domain"
	"strait/internal/telemetry"
)

func TestWorkerSentryScopeAttachesDispatchRequestContext(t *testing.T) {
	t.Parallel()

	exec := &Executor{mode: "worker", defaultRegion: "iad", version: "test-version"}
	run := &domain.JobRun{
		ID:          "run-1",
		JobID:       "job-1",
		ProjectID:   "proj-1",
		Attempt:     2,
		Status:      domain.StatusExecuting,
		TriggeredBy: domain.TriggerManual,
		CreatedBy:   "user-1",
		Metadata: map[string]string{
			domain.RunMetadataSentryActorType: "user",
			domain.RunMetadataSentryRequestID: "req-1",
			domain.RunMetadataSentryRoute:     "POST /v1/jobs/{jobID}/trigger",
		},
	}

	scope := sentry.NewScope()
	exec.applyWorkerSentryScope(scope, run, map[string]any{
		"execution_mode": string(domain.ExecutionModeHTTP),
	})
	event := scope.ApplyToEvent(&sentry.Event{}, nil, nil)
	require.NotNil(t,
		event)

	wantTags := map[string]string{
		string(telemetry.TagSubsystem): "worker",
		string(telemetry.TagMode):      "worker",
		string(telemetry.TagRegion):    "iad",
		string(telemetry.TagVersion):   "test-version",
		string(telemetry.TagRunID):     "run-1",
		string(telemetry.TagJobID):     "job-1",
		string(telemetry.TagProjectID): "proj-1",
		string(telemetry.TagActorID):   "user-1",
		string(telemetry.TagActorType): "user",
		string(telemetry.TagRequestID): "req-1",
		string(telemetry.TagRoute):     "POST /v1/jobs/{jobID}/trigger",
	}
	for key, want := range wantTags {
		require.Equal(t,
			want, event.
				Tags[key])
	}
	require.Equal(t,
		"user-1",
		event.User.
			ID)
	require.Equal(t,
		"req-1",
		event.Contexts["dispatch.request"]["request_id"])
}

func TestWorkerSentryCaptureContract(t *testing.T) {
	t.Parallel()

	transport := &capturingSentryTransport{}
	opts := telemetry.SentryClientOptions(telemetry.SentryConfig{
		DSN:         "https://public@example.com/1",
		Environment: "test",
		Release:     "test-release",
	}, 0)
	opts.Transport = transport
	client, err := sentry.NewClient(opts)
	require.NoError(
		t, err)

	hub := sentry.NewHub(client, sentry.NewScope())
	ctx := sentry.SetHubOnContext(context.Background(), hub)
	telemetry.AddSentryBreadcrumb(ctx, "worker.claim", "claimed Bearer secret-token", map[string]any{
		"run_id": "run-1",
	})
	telemetry.AddSentryBreadcrumb(ctx, "http.dispatch", "endpoint failed", map[string]any{
		"authorization": "Bearer secret-token",
		"status_code":   503,
	})

	exec := &Executor{mode: "worker", defaultRegion: "iad", version: "test-version"}
	run := &domain.JobRun{
		ID:          "run-1",
		JobID:       "job-1",
		ProjectID:   "proj-1",
		Attempt:     2,
		Status:      domain.StatusExecuting,
		TriggeredBy: domain.TriggerManual,
		CreatedBy:   "user-1",
		Metadata: map[string]string{
			domain.RunMetadataSentryActorType: "user",
			domain.RunMetadataSentryRequestID: "req-1",
			domain.RunMetadataSentryRoute:     "POST /v1/jobs/{jobID}/trigger",
		},
	}

	hub.WithScope(func(scope *sentry.Scope) {
		exec.applyWorkerSentryScope(scope, run, map[string]any{
			"execution_mode": string(domain.ExecutionModeHTTP),
		})
		hub.CaptureException(errors.Join(
			errors.New("dispatch failed with Bearer secret-token"),
			&domain.EndpointError{StatusCode: 503},
		))
	})

	event := transport.singleEvent(t)
	require.Equal(t,
		"test-release",
		event.
			Release,
	)

	wantTags := map[string]string{
		string(telemetry.TagSubsystem): "worker",
		string(telemetry.TagMode):      "worker",
		string(telemetry.TagRegion):    "iad",
		string(telemetry.TagVersion):   "test-version",
		string(telemetry.TagRunID):     "run-1",
		string(telemetry.TagJobID):     "job-1",
		string(telemetry.TagProjectID): "proj-1",
		string(telemetry.TagActorID):   "user-1",
		string(telemetry.TagActorType): "user",
		string(telemetry.TagRequestID): "req-1",
		string(telemetry.TagRoute):     "POST /v1/jobs/{jobID}/trigger",
	}
	for key, want := range wantTags {
		require.Equal(t,
			want, event.
				Tags[key])
	}
	require.Equal(t,
		"user-1",
		event.User.
			ID)
	require.Equal(t,
		"req-1",
		event.Contexts["dispatch.request"]["request_id"])

	if got, want := strings.Join(event.Fingerprint, "/"), "endpoint/5xx"; got != want {
		require.Failf(t, "test failure",

			"fingerprint = %q, want %q", got, want)
	}
	require.Len(t, event.
		Breadcrumbs,
		2)

	for _, breadcrumb := range event.Breadcrumbs {
		require.NotContains(t,
			breadcrumb.
				Message, "secret-token")

		if _, ok := breadcrumb.Data["authorization"]; ok {
			require.Fail(t,

				"breadcrumb authorization data was not dropped")
		}
	}
	require.NotEmpty(t, event.
		Exception)

	for _, exception := range event.Exception {
		require.NotContains(t,
			exception.
				Value, "secret-token")
	}
}

type capturingSentryTransport struct {
	mu     sync.Mutex
	events []*sentry.Event
}

func (t *capturingSentryTransport) Configure(sentry.ClientOptions) {}

func (t *capturingSentryTransport) SendEvent(event *sentry.Event) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.events = append(t.events, event)
}

func (t *capturingSentryTransport) Flush(time.Duration) bool {
	return true
}

func (t *capturingSentryTransport) FlushWithContext(context.Context) bool {
	return true
}

func (t *capturingSentryTransport) Close() {}

func (t *capturingSentryTransport) singleEvent(tb testing.TB) *sentry.Event {
	tb.Helper()

	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.events) != 1 {
		tb.Fatalf("captured events = %d, want 1", len(t.events))
	}
	return t.events[0]
}
