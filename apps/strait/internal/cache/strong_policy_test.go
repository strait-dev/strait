package cache

import "testing"

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
		if policy.CacheKey == "" {
			t.Fatalf("%s missing cache key contract", policy.Namespace)
		}
		if policy.VersionSource == "" {
			t.Fatalf("%s missing version source", policy.Namespace)
		}
		if len(policy.MutationPaths) == 0 {
			t.Fatalf("%s missing mutation paths", policy.Namespace)
		}
		if policy.WriteThroughPath == "" {
			t.Fatalf("%s missing write-through path", policy.Namespace)
		}
		if policy.BusPath == "" {
			t.Fatalf("%s missing cachebus path", policy.Namespace)
		}
		if policy.CDCRepairPath == "" {
			t.Fatalf("%s missing CDC repair path", policy.Namespace)
		}
		if len(policy.CDCTables) == 0 {
			t.Fatalf("%s missing CDC table contracts", policy.Namespace)
		}
		if policy.FailureMode == "" {
			t.Fatalf("%s missing failure mode", policy.Namespace)
		}
		if policy.TestMarker == "" {
			t.Fatalf("%s missing test marker", policy.Namespace)
		}
	}

	for namespace, found := range required {
		if !found {
			t.Fatalf("strong namespace %s missing from policy registry", namespace)
		}
	}
}
