package api

import (
	"strings"
	"testing"

	"strait/internal/domain"
)

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
