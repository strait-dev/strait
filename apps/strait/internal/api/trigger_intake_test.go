package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"
)

func TestValidateTriggerJobInputAcceptsValidInput(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	scheduledAt := time.Now().Add(time.Minute)
	req := TriggerRequest{
		Payload:        json.RawMessage(`{"ok":true}`),
		Tags:           map[string]string{"team": "platform"},
		ScheduledAt:    &scheduledAt,
		Priority:       10,
		ConcurrencyKey: strings.Repeat("c", 256),
		DebounceKey:    strings.Repeat("d", 256),
		BatchKey:       strings.Repeat("b", 256),
	}
	input := &TriggerJobInput{
		Traceparent: strings.Repeat("t", maxTraceparentLen),
		Tracestate:  strings.Repeat("s", maxTraceHeaderLen),
		SentryTrace: strings.Repeat("r", maxTraceHeaderLen),
		Baggage:     strings.Repeat("g", maxTraceHeaderLen),
	}

	if err := srv.validateTriggerJobInput(input, &req); err != nil {
		t.Fatalf("validateTriggerJobInput() error = %v", err)
	}
}

func TestValidateTriggerJobInputRejectsOversizeTraceHeader(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	req := TriggerRequest{}
	input := &TriggerJobInput{Traceparent: strings.Repeat("t", maxTraceparentLen+1)}

	err := srv.validateTriggerJobInput(input, &req)
	assertStatusError(t, err, http.StatusBadRequest, "traceparent")
}

func TestMergedRunTagsOverlayWins(t *testing.T) {
	t.Parallel()

	base := map[string]string{"team": "platform", "env": "prod"}
	overlay := map[string]string{"env": "staging", "request": "manual"}

	got := mergedRunTags(base, overlay)

	if got["team"] != "platform" || got["env"] != "staging" || got["request"] != "manual" {
		t.Fatalf("merged tags = %#v", got)
	}
	if base["env"] != "prod" {
		t.Fatalf("base tags mutated: %#v", base)
	}
}

func TestMergeRunMetadataDefaultsDoNotOverrideRequestMetadata(t *testing.T) {
	t.Parallel()

	metadata := map[string]string{"tenant": "acme"}
	defaults := map[string]string{"tenant": "default", "region": "eu"}

	got := mergeRunMetadata(metadata, defaults)

	if got["tenant"] != "acme" || got["region"] != "eu" {
		t.Fatalf("merged metadata = %#v", got)
	}
	if metadata["tenant"] != "acme" {
		t.Fatalf("request metadata mutated: %#v", metadata)
	}
}

func TestMergeRunMetadataReturnsNilForEmptyInputs(t *testing.T) {
	t.Parallel()

	if got := mergeRunMetadata(nil, nil); got != nil {
		t.Fatalf("mergeRunMetadata(nil, nil) = %#v, want nil", got)
	}
}

func TestEnsureJobTriggerableRejectsDisabledJob(t *testing.T) {
	t.Parallel()

	err := ensureJobTriggerable(&domain.Job{Enabled: false})
	if err == nil {
		t.Fatal("expected disabled job error")
	}
	if !strings.Contains(err.Error(), "job is disabled") {
		t.Fatalf("error = %q, want disabled job message", err.Error())
	}
}

func TestEnsureJobTriggerableRejectsPausedJob(t *testing.T) {
	t.Parallel()

	err := ensureJobTriggerable(&domain.Job{Enabled: true, Paused: true})
	if err == nil {
		t.Fatal("expected paused job error")
	}
	if !strings.Contains(err.Error(), "job is paused") {
		t.Fatalf("error = %q, want paused job message", err.Error())
	}
}
