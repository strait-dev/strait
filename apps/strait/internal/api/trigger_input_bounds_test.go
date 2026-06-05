package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

func newTriggerBoundsTestServer(t *testing.T) *Server {
	t.Helper()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID:          id,
				ProjectID:   "proj-1",
				Enabled:     true,
				TimeoutSecs: 60,
				MaxAttempts: 3,
			}, nil
		},
		AreJobDependenciesSatisfiedFunc: func(_ context.Context, _ *domain.JobRun) (bool, error) {
			return true, nil
		},
		CreateRunFunc: func(_ context.Context, _ *domain.JobRun) error { return nil },
	}
	return newTestServer(t, ms, &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error { return nil }}, nil)
}

func TestTrigger_TTLSecsRejectsOverflow(t *testing.T) {
	t.Parallel()
	srv := newTriggerBoundsTestServer(t)

	w := httptest.NewRecorder()
	body := `{"ttl_secs":10000000000}`
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/jobs/job-1/trigger", body, "proj-1"))
	require.False(t, w.Code !=
		http.StatusBadRequest &&
		w.Code !=
			http.StatusUnprocessableEntity,
	)
}

func TestTrigger_TTLSecsAcceptsBoundary(t *testing.T) {
	t.Parallel()
	srv := newTriggerBoundsTestServer(t)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{"ttl_secs":2592000}`, "proj-1"))
	require.Equal(t, http.StatusCreated,
		w.Code)

	w2 := httptest.NewRecorder()
	srv.ServeHTTP(w2, authedProjectRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{"ttl_secs":2592001}`, "proj-1"))
	require.False(t, w2.Code !=
		http.StatusBadRequest &&
		w2.Code !=
			http.
				StatusUnprocessableEntity)
}

func TestTrigger_KeysRejectOversize(t *testing.T) {
	t.Parallel()
	srv := newTriggerBoundsTestServer(t)

	oversize := strings.Repeat("a", 257)
	cases := map[string]string{
		"concurrency_key": `{"concurrency_key":"` + oversize + `"}`,
		"debounce_key":    `{"debounce_key":"` + oversize + `"}`,
		"batch_key":       `{"batch_key":"` + oversize + `"}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/jobs/job-1/trigger", body, "proj-1"))
			require.False(t, w.Code !=
				http.StatusBadRequest &&
				w.Code !=
					http.StatusUnprocessableEntity,
			)
		})
	}
}

func TestTrigger_KeysAcceptBoundary(t *testing.T) {
	t.Parallel()
	srv := newTriggerBoundsTestServer(t)

	atSize := strings.Repeat("a", 256)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{"concurrency_key":"`+atSize+`"}`, "proj-1"))
	require.Equal(t, http.StatusCreated,
		w.Code)
}

func TestTrigger_TraceparentHeaderRejectsOversize(t *testing.T) {
	t.Parallel()
	srv := newTriggerBoundsTestServer(t)

	r := authedProjectRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`, "proj-1")
	r.Header.Set("Traceparent", strings.Repeat("a", 257))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusBadRequest,
		w.Code,
	)
	require.Contains(
		t, w.Body.String(), "traceparent")
}

func TestTrigger_TracestateHeaderRejectsOversize(t *testing.T) {
	t.Parallel()
	srv := newTriggerBoundsTestServer(t)

	r := authedProjectRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`, "proj-1")
	r.Header.Set("Tracestate", strings.Repeat("a", 8193))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusBadRequest,
		w.Code,
	)
	require.Contains(
		t, w.Body.String(), "tracestate")
}

func TestTrigger_SentryTraceAndBaggageHeadersRejectOversize(t *testing.T) {
	t.Parallel()
	srv := newTriggerBoundsTestServer(t)

	cases := []struct {
		name, header, want string
		size               int
	}{
		{"sentry-trace", "Sentry-Trace", "sentry-trace", 8193},
		{"baggage", "Baggage", "baggage", 8193},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := authedProjectRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`, "proj-1")
			r.Header.Set(tc.header, strings.Repeat("a", tc.size))
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, r)
			require.Equal(t, http.StatusBadRequest,
				w.Code,
			)
			require.Contains(
				t, w.Body.String(), tc.want)
		})
	}
}

func TestTrigger_TraceHeadersAcceptBoundary(t *testing.T) {
	t.Parallel()
	srv := newTriggerBoundsTestServer(t)

	r := authedProjectRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`, "proj-1")
	r.Header.Set("Traceparent", strings.Repeat("a", 256))
	r.Header.Set("Tracestate", strings.Repeat("a", 8192))
	r.Header.Set("Sentry-Trace", strings.Repeat("a", 8192))
	r.Header.Set("Baggage", strings.Repeat("a", 8192))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusCreated,
		w.Code)
}

func TestApplyRunTraceHeaderMetadata_TruncatesOversizeValues(t *testing.T) {
	t.Parallel()

	tooLongTraceparent := strings.Repeat("t", 300)
	tooLongOther := strings.Repeat("x", 9000)

	got := applyRunTraceHeaderMetadata(nil, tooLongTraceparent, tooLongOther, tooLongOther, tooLongOther)
	require.Len(t, got[domain.
		RunMetadataTraceParent], maxTraceparentLen)
	require.Len(t, got[domain.
		RunMetadataTraceState], maxTraceHeaderLen)
	require.Len(t, got[domain.
		RunMetadataSentryTrace], maxTraceHeaderLen)
	require.Len(t, got[domain.
		RunMetadataSentryBaggage], maxTraceHeaderLen)
}
