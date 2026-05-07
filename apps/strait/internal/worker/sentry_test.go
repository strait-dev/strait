package worker

import (
	"testing"

	"github.com/getsentry/sentry-go"

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
	if event == nil {
		t.Fatal("expected event")
	}

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
		if got := event.Tags[key]; got != want {
			t.Fatalf("tag %s = %q, want %q", key, got, want)
		}
	}
	if event.User.ID != "user-1" {
		t.Fatalf("user id = %q, want user-1", event.User.ID)
	}
	if got := event.Contexts["dispatch.request"]["request_id"]; got != "req-1" {
		t.Fatalf("dispatch request context request_id = %v, want req-1", got)
	}
}
