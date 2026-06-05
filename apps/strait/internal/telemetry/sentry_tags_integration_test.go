//go:build integration

package telemetry

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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
		require.Equal(t, value,

			tags[key])

	}
}
