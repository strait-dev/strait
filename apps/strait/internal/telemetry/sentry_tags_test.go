package telemetry

import (
	"testing"

	"github.com/getsentry/sentry-go"
	"github.com/stretchr/testify/require"
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
		require.Equal(t, expected,

			tags[key])

	}
}

func TestSentryTagFromString_KnownAndAliases(t *testing.T) {
	t.Parallel()

	tag, ok := SentryTagFromString("run_id")
	require.True(t, ok)
	require.Equal(t, TagRunID, tag)
	tag, ok = SentryTagFromString("workflow_run_id")
	require.True(t, ok)
	require.Equal(t, TagWorkflowID, tag)
	_, ok = SentryTagFromString("free_form_customer_name")
	require.False(t, ok)
}

func TestNormalizeSubsystem_KnownBilling(t *testing.T) {
	t.Parallel()
	require.Equal(t, SubsystemBilling,

		NormalizeSubsystem(
			"billing",
		))

}

func TestSetSentryTag_SkipsEmptyAndNormalizes(t *testing.T) {
	t.Parallel()

	scope := sentry.NewScope()
	SetSentryTag(scope, TagSubsystem, " Worker ")
	SetSentryTag(scope, TagProjectID, "")
	event := scope.ApplyToEvent(&sentry.Event{}, nil, nil)
	require.NotNil(t, event)

	require.Equal(t, "worker",

		event.Tags["subsystem"])

	require.NotContains(t, event.Tags, "project_id")
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
	require.NotNil(t, event)

	want := map[string]string{
		"edition":   "cloud",
		"subsystem": "scheduler",
		"mode":      "all",
		"region":    "iad",
		"version":   "2026.05.07",
	}
	for key, expected := range want {
		require.Equal(t, expected,

			event.Tags[key])

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
		require.Equal(t, want,

			NormalizeHTTPStatusClass(status))

	}
}
