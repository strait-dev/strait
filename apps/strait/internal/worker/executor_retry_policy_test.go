package worker

import (
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

// TestRetryAndFallbackPolicyMatchesDomain guards against the worker re-deriving
// the retry/fallback policy and diverging from the canonical domain enum.
func TestRetryAndFallbackPolicyMatchesDomain(t *testing.T) {
	t.Parallel()
	classes := []string{
		domain.ErrorClassClient, domain.ErrorClassAuth, domain.ErrorClassBudget,
		domain.ErrorClassOOM, domain.ErrorClassTransient, domain.ErrorClassRateLimited,
		domain.ErrorClassConnection, domain.ErrorClassTimeout, domain.ErrorClassServer, "unknown",
	}
	for _, c := range classes {
		require.Equalf(t, domain.ErrorClassEnum(c).IsRetryable(), shouldRetryForClass(c), "retry %q", c)
		require.Equalf(t, domain.ErrorClassEnum(c).IsTransient(), shouldUseFallbackForClass(c), "fallback %q", c)
	}
}
