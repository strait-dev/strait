//go:build loadtest

package loadtest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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

func TestHarnessGetQueueStats_RejectsHTTPErrorPayload(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"redis unavailable"}`))
	}))
	defer srv.Close()

	h := NewHarness(HarnessConfig{StraitURL: srv.URL})
	_, err := h.GetQueueStats(context.Background(), "project-1")
	if err == nil {
		t.Fatal("expected stats request to fail on non-2xx response")
	}
	if !strings.Contains(err.Error(), "status 500") {
		t.Fatalf("error = %v, want status 500 context", err)
	}
}

func TestHarnessGetQueueStats_RejectsJSONErrorEnvelope(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"error":"stats disabled"}`))
	}))
	defer srv.Close()

	h := NewHarness(HarnessConfig{StraitURL: srv.URL})
	_, err := h.GetQueueStats(context.Background(), "project-1")
	if err == nil {
		t.Fatal("expected stats request to fail on JSON error envelope")
	}
	if !strings.Contains(err.Error(), "stats disabled") {
		t.Fatalf("error = %v, want envelope message", err)
	}
}

func TestHarnessGetQueueStats_DecodesValidStats(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Project-Id"); got != "project-1" {
			t.Fatalf("X-Project-Id = %q, want project-1", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"queued":2,"executing":1,"delayed":3,"failed":4,"completed":5}`))
	}))
	defer srv.Close()

	h := NewHarness(HarnessConfig{StraitURL: srv.URL})
	stats, err := h.GetQueueStats(context.Background(), "project-1")
	if err != nil {
		t.Fatalf("GetQueueStats() error = %v", err)
	}
	if stats.QueueDepth() != 5 || stats.Executing != 1 || stats.Failed != 4 || stats.Completed != 5 {
		t.Fatalf("stats = %+v, want decoded counters", *stats)
	}
}
