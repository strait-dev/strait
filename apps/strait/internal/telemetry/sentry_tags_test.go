package telemetry

import (
	"testing"

	"github.com/getsentry/sentry-go"
)

func TestRequiredSentryTags_NormalizesRequiredValues(t *testing.T) {
	t.Parallel()

	tags := SentryTagStrings(RequiredSentryTags("Cloud", "API", "ALL", "IAD", "V1.2.3"))
	want := map[string]string{
		"edition":   "cloud",
		"subsystem": "api",
		"mode":      "all",
		"region":    "iad",
		"version":   "V1.2.3",
	}
	for key, expected := range want {
		if got := tags[key]; got != expected {
			t.Fatalf("tag %s = %q, want %q", key, got, expected)
		}
	}
}

func TestSentryTagFromString_KnownAndAliases(t *testing.T) {
	t.Parallel()

	if tag, ok := SentryTagFromString("run_id"); !ok || tag != TagRunID {
		t.Fatalf("run_id tag = %q, %v; want %q, true", tag, ok, TagRunID)
	}
	if tag, ok := SentryTagFromString("workflow_run_id"); !ok || tag != TagWorkflowID {
		t.Fatalf("workflow_run_id tag = %q, %v; want %q, true", tag, ok, TagWorkflowID)
	}
	if _, ok := SentryTagFromString("free_form_customer_name"); ok {
		t.Fatal("unexpected free-form tag key")
	}
}

func TestNormalizeSubsystem_KnownBilling(t *testing.T) {
	t.Parallel()

	if got := NormalizeSubsystem("billing"); got != SubsystemBilling {
		t.Fatalf("NormalizeSubsystem(billing) = %q, want %q", got, SubsystemBilling)
	}
}

func TestSetSentryTag_SkipsEmptyAndNormalizes(t *testing.T) {
	t.Parallel()

	scope := sentry.NewScope()
	SetSentryTag(scope, TagSubsystem, " Worker ")
	SetSentryTag(scope, TagProjectID, "")
	event := scope.ApplyToEvent(&sentry.Event{}, nil, nil)
	if event == nil {
		t.Fatal("expected event")
		return
	}
	if got := event.Tags["subsystem"]; got != "worker" {
		t.Fatalf("subsystem tag = %q, want worker", got)
	}
	if _, ok := event.Tags["project_id"]; ok {
		t.Fatal("empty project_id tag should not be set")
	}
}

func TestApplySentryRuntimeScopeSetsRequiredTags(t *testing.T) {
	t.Parallel()

	scope := sentry.NewScope()
	ApplySentryRuntimeScope(scope, SentryRuntime{
		Edition:   "Cloud",
		Subsystem: "Scheduler",
		Mode:      "ALL",
		Region:    "IAD",
		Version:   "2026.05.07",
	})
	event := scope.ApplyToEvent(&sentry.Event{}, nil, nil)
	if event == nil {
		t.Fatal("expected event")
		return
	}

	want := map[string]string{
		"edition":   "cloud",
		"subsystem": "scheduler",
		"mode":      "all",
		"region":    "iad",
		"version":   "2026.05.07",
	}
	for key, expected := range want {
		if got := event.Tags[key]; got != expected {
			t.Fatalf("tag %s = %q, want %q", key, got, expected)
		}
	}
}

func TestNormalizeHTTPStatusClass(t *testing.T) {
	t.Parallel()

	cases := map[int]string{
		200: "2xx",
		404: "4xx",
		503: "5xx",
		99:  "",
		600: "",
	}
	for status, want := range cases {
		if got := NormalizeHTTPStatusClass(status); got != want {
			t.Fatalf("NormalizeHTTPStatusClass(%d) = %q, want %q", status, got, want)
		}
	}
}
