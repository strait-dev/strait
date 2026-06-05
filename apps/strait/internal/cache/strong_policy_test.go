package cache

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStrongNamespacePoliciesCoverRequiredNamespaces(t *testing.T) {
	required := map[string]bool{
		"authn_keys":           false,
		"permission":           false,
		"quota":                false,
		"billing_org_limits":   false,
		"worker_job":           false,
		"api_job_dependencies": false,
	}

	for _, policy := range StrongNamespacePolicies {
		if _, ok := required[policy.Namespace]; ok {
			required[policy.Namespace] = true
		}
		require.NotEmpty(t, policy.CacheKey)
		require.NotEmpty(t, policy.VersionSource)
		require.NotEmpty(t, policy.MutationPaths)
		require.NotEmpty(t, policy.WriteThroughPath)
		require.NotEmpty(t, policy.BusPath)
		require.NotEmpty(t, policy.CDCRepairPath)
		require.NotEmpty(t, policy.CDCTables)
		require.NotEmpty(t, policy.FailureMode)
		require.NotEmpty(t, policy.TestMarker)
	}

	for namespace, found := range required {
		require.True(t, found, "namespace %q", namespace)
	}
}
