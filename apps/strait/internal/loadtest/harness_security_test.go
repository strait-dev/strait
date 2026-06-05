//go:build loadtest

package loadtest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStandardLoadTestJobConfigsIncludeEnduranceAndErrorJobs(t *testing.T) {
	configs := standardLoadTestJobConfigs("project-1", "http://127.0.0.1:9000", "secret")
	bySlug := make(map[string]JobConfig, len(configs))
	for _, cfg := range configs {
		bySlug[cfg.Slug] = cfg
	}

	for _, slug := range []string{"loadtest-fast-echo", "loadtest-slow-process", "loadtest-errors"} {
		require.Contains(t, bySlug, slug)
	}
	require.Equal(t, "http://127.0.0.1:9000/error-scenario",

		bySlug["loadtest-errors"].
			EndpointURL,
	)

	require.NotContains(t, bySlug, "loadtest-slow-cpu")
}

func TestHarnessResolveJobIDUsesSetupIDs(t *testing.T) {
	h := &Harness{
		jobIDs: map[string]string{
			"loadtest-fast-echo": "job-uuid",
		},
	}
	require.Equal(t, "job-uuid", h.ResolveJobID("loadtest-fast-echo"))
	require.Equal(t, "ad-hoc", h.ResolveJobID("ad-hoc"))
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
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "status 500"))

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
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "stats disabled"))

}

func TestHarnessGetQueueStats_DecodesValidStats(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "project-1", r.Header.Get("X-Project-Id"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"queued":2,"executing":1,"delayed":3,"failed":4,"completed":5}`))
	}))
	defer srv.Close()

	h := NewHarness(HarnessConfig{StraitURL: srv.URL})
	stats, err := h.GetQueueStats(context.Background(), "project-1")
	require.NoError(t,

		err)
	require.False(t, stats.
		QueueDepth() !=
		5 || stats.Executing !=
		1 || stats.
		Failed !=
		4 || stats.
		Completed !=
		5)

}
