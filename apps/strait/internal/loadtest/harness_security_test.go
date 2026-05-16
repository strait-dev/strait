//go:build loadtest

package loadtest

import "testing"

func TestStandardLoadTestJobConfigsIncludeEnduranceAndErrorJobs(t *testing.T) {
	configs := standardLoadTestJobConfigs("project-1", "http://127.0.0.1:9000", "secret")
	bySlug := make(map[string]JobConfig, len(configs))
	for _, cfg := range configs {
		bySlug[cfg.Slug] = cfg
	}

	for _, slug := range []string{"loadtest-fast-echo", "loadtest-slow-process", "loadtest-errors"} {
		if _, ok := bySlug[slug]; !ok {
			t.Fatalf("missing standard load-test job slug %q", slug)
		}
	}
	if bySlug["loadtest-errors"].EndpointURL != "http://127.0.0.1:9000/error-scenario" {
		t.Fatalf("error scenario endpoint = %q", bySlug["loadtest-errors"].EndpointURL)
	}
	if _, ok := bySlug["loadtest-slow-cpu"]; ok {
		t.Fatal("obsolete loadtest-slow-cpu slug should not be configured")
	}
}

func TestHarnessResolveJobIDUsesSetupIDs(t *testing.T) {
	h := &Harness{
		jobIDs: map[string]string{
			"loadtest-fast-echo": "job-uuid",
		},
	}
	if got := h.ResolveJobID("loadtest-fast-echo"); got != "job-uuid" {
		t.Fatalf("ResolveJobID() = %q, want job-uuid", got)
	}
	if got := h.ResolveJobID("ad-hoc"); got != "ad-hoc" {
		t.Fatalf("ResolveJobID() fallback = %q, want ad-hoc", got)
	}
}
