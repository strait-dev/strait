package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
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
	require.NoError(t,
		srv.validateTriggerJobInput(input,
			&req))
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
	require.False(t, got["team"] !=
		"platform" ||
		got["env"] != "staging" ||
		got["request"] !=

			"manual")
	require.Equal(t, "prod",
		base["env"])
}

func TestMergedRunTagsFastPathsEmptyAndClonesSingleInput(t *testing.T) {
	t.Parallel()

	require.Nil(t, mergedRunTags(nil, nil))
	require.Nil(t, mergedRunTags(map[string]string{}, map[string]string{}))

	overlay := map[string]string{"request": "manual"}
	got := mergedRunTags(nil, overlay)
	overlay["request"] = "changed"

	require.Equal(t, "manual", got["request"])
}

func TestMergeRunMetadataDefaultsDoNotOverrideRequestMetadata(t *testing.T) {
	t.Parallel()

	metadata := map[string]string{"tenant": "acme"}
	defaults := map[string]string{"tenant": "default", "region": "eu"}

	got := mergeRunMetadata(metadata, defaults)
	require.False(t, got["tenant"] !=
		"acme" ||
		got["region"] !=
			"eu",
	)
	require.Equal(t, "acme",
		metadata["tenant"])
}

func TestMergeRunMetadataReturnsNilForEmptyInputs(t *testing.T) {
	t.Parallel()
	require.Nil(t, mergeRunMetadata(nil,
		nil))
	require.Nil(t, mergeRunMetadata(map[string]string{},
		map[string]string{}))
}

func TestMergeRunMetadataClonesSingleInput(t *testing.T) {
	t.Parallel()

	metadata := map[string]string{"tenant": "acme"}
	got := mergeRunMetadata(metadata, nil)
	metadata["tenant"] = "changed"

	require.Equal(t, "acme", got["tenant"])
}

func TestApplyDefaultRunMetadataMutatesFreshMetadata(t *testing.T) {
	t.Parallel()

	metadata := map[string]string{"tenant": "acme"}
	got := applyDefaultRunMetadata(metadata, map[string]string{"tenant": "default", "region": "eu"})
	got["added"] = "true"

	require.Equal(t, "acme", got["tenant"])
	require.Equal(t, "eu", got["region"])
	require.Equal(t, "true", metadata["added"])
}

func TestApplyDefaultRunMetadataClonesDefaultsWhenMetadataEmpty(t *testing.T) {
	t.Parallel()

	defaults := map[string]string{"region": "eu"}
	got := applyDefaultRunMetadata(nil, defaults)
	defaults["region"] = "us"

	require.Equal(t, "eu", got["region"])
}

func TestEnsureJobTriggerableRejectsDisabledJob(t *testing.T) {
	t.Parallel()

	err := ensureJobTriggerable(&domain.Job{Enabled: false})
	require.Error(t, err)
	require.Contains(t, err.Error(), "job is disabled")
}

func TestEnsureJobTriggerableRejectsPausedJob(t *testing.T) {
	t.Parallel()

	err := ensureJobTriggerable(&domain.Job{Enabled: true, Paused: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "job is paused")
}
