package worker

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/getsentry/sentry-go"

	"strait/internal/domain"
)

type dispatchRoundTripFunc func(*http.Request) (*http.Response, error)

func (f dispatchRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestHTTPDispatch_InjectsTraceparentHeader(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var capturedHeaders http.Header

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		capturedHeaders = r.Header.Clone()
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()

	e := &Executor{httpClient: srv.Client()}

	run := &domain.JobRun{
		ID:      "run-1",
		JobID:   "job-1",
		Attempt: 1,
		Metadata: map[string]string{
			"_trace_parent": "00-abcdef1234567890abcdef1234567890-fedcba0987654321-01",
		},
	}

	_, err := e.dispatchToEndpoint(t.Context(), srv.URL, run, nil)
	if err != nil {
		t.Fatalf("dispatchToEndpoint returned error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	got := capturedHeaders.Get("Traceparent")
	if got != "00-abcdef1234567890abcdef1234567890-fedcba0987654321-01" {
		t.Errorf("Traceparent header = %q, want %q", got, "00-abcdef1234567890abcdef1234567890-fedcba0987654321-01")
	}
}

func TestHTTPDispatch_InjectsTracestateHeader(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var capturedHeaders http.Header

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		capturedHeaders = r.Header.Clone()
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()

	e := &Executor{httpClient: srv.Client()}

	run := &domain.JobRun{
		ID:      "run-1",
		JobID:   "job-1",
		Attempt: 1,
		Metadata: map[string]string{
			"_trace_parent": "00-abcdef1234567890abcdef1234567890-fedcba0987654321-01",
			"_trace_state":  "congo=t61rcWkgMzE",
		},
	}

	_, err := e.dispatchToEndpoint(t.Context(), srv.URL, run, nil)
	if err != nil {
		t.Fatalf("dispatchToEndpoint returned error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if tp := capturedHeaders.Get("Traceparent"); tp != "00-abcdef1234567890abcdef1234567890-fedcba0987654321-01" {
		t.Errorf("Traceparent header = %q, want traceparent value", tp)
	}
	if ts := capturedHeaders.Get("Tracestate"); ts != "congo=t61rcWkgMzE" {
		t.Errorf("Tracestate header = %q, want %q", ts, "congo=t61rcWkgMzE")
	}
}

func TestHTTPDispatch_InjectsSentryTraceHeaders(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var capturedHeaders http.Header

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		capturedHeaders = r.Header.Clone()
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()

	e := &Executor{httpClient: srv.Client()}

	run := &domain.JobRun{
		ID:      "run-1",
		JobID:   "job-1",
		Attempt: 1,
		Metadata: map[string]string{
			domain.RunMetadataSentryTrace:   "0123456789abcdef0123456789abcdef-0123456789abcdef-1",
			domain.RunMetadataSentryBaggage: "sentry-release=test-release,sentry-public_key=public",
		},
	}

	_, err := e.dispatchToEndpoint(t.Context(), srv.URL, run, nil)
	if err != nil {
		t.Fatalf("dispatchToEndpoint returned error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if got := capturedHeaders.Get(sentry.SentryTraceHeader); got != "0123456789abcdef0123456789abcdef-0123456789abcdef-1" {
		t.Fatalf("sentry-trace header = %q, want Sentry trace metadata", got)
	}
	if got := capturedHeaders.Get(sentry.SentryBaggageHeader); got != "sentry-release=test-release,sentry-public_key=public" {
		t.Fatalf("baggage header = %q, want Sentry baggage metadata", got)
	}
}

func TestHTTPDispatch_NoTraceMetadata(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var capturedHeaders http.Header

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		capturedHeaders = r.Header.Clone()
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()

	e := &Executor{httpClient: srv.Client()}

	run := &domain.JobRun{
		ID:       "run-1",
		JobID:    "job-1",
		Attempt:  1,
		Metadata: map[string]string{"some_key": "some_value"},
	}

	_, err := e.dispatchToEndpoint(t.Context(), srv.URL, run, nil)
	if err != nil {
		t.Fatalf("dispatchToEndpoint returned error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if tp := capturedHeaders.Get("Traceparent"); tp != "" {
		t.Errorf("expected no Traceparent header, got %q", tp)
	}
	if ts := capturedHeaders.Get("Tracestate"); ts != "" {
		t.Errorf("expected no Tracestate header, got %q", ts)
	}
	if st := capturedHeaders.Get(sentry.SentryTraceHeader); st != "" {
		t.Errorf("expected no Sentry trace header, got %q", st)
	}
	if baggage := capturedHeaders.Get(sentry.SentryBaggageHeader); baggage != "" {
		t.Errorf("expected no Sentry baggage header, got %q", baggage)
	}
}

func TestHTTPDispatch_EmptyTraceParent(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var capturedHeaders http.Header

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		capturedHeaders = r.Header.Clone()
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()

	e := &Executor{httpClient: srv.Client()}

	run := &domain.JobRun{
		ID:      "run-1",
		JobID:   "job-1",
		Attempt: 1,
		Metadata: map[string]string{
			"_trace_parent": "",
		},
	}

	_, err := e.dispatchToEndpoint(t.Context(), srv.URL, run, nil)
	if err != nil {
		t.Fatalf("dispatchToEndpoint returned error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if tp := capturedHeaders.Get("Traceparent"); tp != "" {
		t.Errorf("expected no Traceparent header when _trace_parent is empty, got %q", tp)
	}
}

func TestHTTPDispatch_NilMetadata(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var capturedHeaders http.Header

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		capturedHeaders = r.Header.Clone()
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()

	e := &Executor{httpClient: srv.Client()}

	run := &domain.JobRun{
		ID:       "run-1",
		JobID:    "job-1",
		Attempt:  1,
		Metadata: nil,
	}

	_, err := e.dispatchToEndpoint(t.Context(), srv.URL, run, nil)
	if err != nil {
		t.Fatalf("dispatchToEndpoint returned error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if tp := capturedHeaders.Get("Traceparent"); tp != "" {
		t.Errorf("expected no Traceparent header when metadata is nil, got %q", tp)
	}
	if ts := capturedHeaders.Get("Tracestate"); ts != "" {
		t.Errorf("expected no Tracestate header when metadata is nil, got %q", ts)
	}
}

func TestHTTPDispatch_TraceHeadersCoexistWithExtraHeaders(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var capturedHeaders http.Header

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		capturedHeaders = r.Header.Clone()
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()

	e := &Executor{httpClient: srv.Client()}

	run := &domain.JobRun{
		ID:      "run-1",
		JobID:   "job-1",
		Attempt: 1,
		Metadata: map[string]string{
			"_trace_parent": "00-abcdef1234567890abcdef1234567890-fedcba0987654321-01",
		},
	}

	extraHeaders := map[string]string{
		"X-Custom-Header": "custom-value",
	}

	_, err := e.dispatchToEndpoint(t.Context(), srv.URL, run, extraHeaders)
	if err != nil {
		t.Fatalf("dispatchToEndpoint returned error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if tp := capturedHeaders.Get("Traceparent"); tp != "00-abcdef1234567890abcdef1234567890-fedcba0987654321-01" {
		t.Errorf("Traceparent header = %q, want traceparent value", tp)
	}
	if ch := capturedHeaders.Get("X-Custom-Header"); ch != "custom-value" {
		t.Errorf("X-Custom-Header = %q, want %q", ch, "custom-value")
	}
}

func TestHTTPDispatch_NonTraceMetadataNotLeaked(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var capturedHeaders http.Header

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		capturedHeaders = r.Header.Clone()
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()

	e := &Executor{httpClient: srv.Client()}

	run := &domain.JobRun{
		ID:      "run-1",
		JobID:   "job-1",
		Attempt: 1,
		Metadata: map[string]string{
			"secret":        "super-secret-value",
			"_trace_parent": "00-abcdef1234567890abcdef1234567890-fedcba0987654321-01",
		},
	}

	_, err := e.dispatchToEndpoint(t.Context(), srv.URL, run, nil)
	if err != nil {
		t.Fatalf("dispatchToEndpoint returned error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if v := capturedHeaders.Get("Secret"); v != "" {
		t.Errorf("non-trace metadata 'secret' leaked as header: %q", v)
	}
	if _, ok := capturedHeaders["Secret"]; ok {
		t.Error("non-trace metadata 'secret' should not appear as a request header")
	}
	if tp := capturedHeaders.Get("Traceparent"); tp != "00-abcdef1234567890abcdef1234567890-fedcba0987654321-01" {
		t.Errorf("Traceparent header = %q, want traceparent value", tp)
	}
}

func TestHTTPDispatch_RedactsEndpointURLFromClientErrors(t *testing.T) {
	t.Parallel()

	rawURL := "https://user:pass@hooks.example.com/private/path?token=secret#frag"
	rootErr := context.DeadlineExceeded
	client := &http.Client{
		Transport: dispatchRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			return nil, &url.Error{Op: req.Method, URL: rawURL, Err: rootErr}
		}),
	}
	e := &Executor{httpClient: client}

	_, err := e.dispatchToEndpoint(t.Context(), rawURL, &domain.JobRun{ID: "run-1", JobID: "job-1", Attempt: 1}, nil)
	if err == nil {
		t.Fatal("dispatchToEndpoint returned nil error")
	}
	if !errors.Is(err, rootErr) {
		t.Fatalf("dispatchToEndpoint error does not unwrap deadline: %v", err)
	}
	got := err.Error()
	for _, leaked := range []string{"hooks.example.com", "user:pass", "/private/path", "token=secret", "#frag"} {
		if strings.Contains(got, leaked) {
			t.Fatalf("dispatchToEndpoint leaked endpoint data %q in error %q", leaked, got)
		}
	}
	if !strings.Contains(got, "http dispatch:") || !strings.Contains(got, "context deadline exceeded") {
		t.Fatalf("dispatchToEndpoint error = %q, want sanitized dispatch context and root error", got)
	}
}
