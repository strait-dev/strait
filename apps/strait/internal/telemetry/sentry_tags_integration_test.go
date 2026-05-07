//go:build integration

package telemetry

import "testing"

func TestIntegrationSentryRequiredTagsCoverBillingSubsystem(t *testing.T) {
	t.Parallel()

	tags := SentryTagStrings(RequiredSentryTags("Cloud", SubsystemBilling, "ALL", "IAD", "V1.2.3"))
	want := map[string]string{
		"edition":   "cloud",
		"subsystem": "billing",
		"mode":      "all",
		"region":    "iad",
		"version":   "V1.2.3",
	}
	for key, value := range want {
		if got := tags[key]; got != value {
			t.Fatalf("tag %s = %q, want %q", key, got, value)
		}
	}
}
