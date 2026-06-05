package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"strait/internal/domain"
	"strait/internal/httputil"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// withCapturedSlog redirects slog default output to an in-memory JSON
// handler for the duration of fn so attribute values can be inspected.
func withCapturedSlog(t *testing.T, fn func(t *testing.T)) []map[string]any {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	fn(t)

	var entries []map[string]any
	for line := range bytes.SplitSeq(buf.Bytes(), []byte{'\n'}) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var m map[string]any
		require.NoError(t, json.Unmarshal(line, &m))

		entries = append(entries, m)
	}
	return entries
}

// TestLogsEmitOnlySchemeAndHost asserts that the success-path
// "webhook delivered" log carries a redacted URL (scheme://host) and not
// the path/query that may carry secret tokens.
func TestLogsEmitOnlySchemeAndHost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	secretPath := "/deliver/super-secret-token-abc123"
	job := &domain.Job{ID: "job-1", WebhookURL: srv.URL + secretPath + "?token=leak-me"}
	run := webhookTestRun()

	entries := withCapturedSlog(t, func(t *testing.T) {
		t.Helper()
		result := SendWebhookWithRetry(context.Background(), job, run, 1)
		require.True(t,
			result.Delivered,
		)

	})

	want := httputil.RedactURLForLog(job.WebhookURL)
	for _, e := range entries {
		raw, ok := e["url"].(string)
		if !ok {
			continue
		}
		require.False(
			t, strings.Contains(raw, "super-secret-token-abc123") || strings.Contains(raw, "leak-me"))
		require.Equal(
			t, want, raw,
		)

	}
}

// TestOTelAttributeRedacted checks the webhook.url span attribute
// is recorded as scheme://host -- before the fix the raw user-supplied
// URL was attached, leaking path/query tokens to the trace backend.
func TestOTelAttributeRedacted(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	origTP := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(origTP) })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	job := &domain.Job{
		ID:         "job-1",
		WebhookURL: srv.URL + "/cb/super-secret-token-abc123?sig=leak-me",
	}
	run := webhookTestRun()

	_ = SendWebhookWithRetry(context.Background(), job, run, 1)

	tp.ForceFlush(context.Background())

	want := httputil.RedactURLForLog(job.WebhookURL)
	var seen bool
	for _, span := range exporter.GetSpans() {
		for _, attr := range span.Attributes {
			if string(attr.Key) != "webhook.url" {
				continue
			}
			seen = true
			got := attr.Value.AsString()
			require.False(
				t, strings.Contains(got, "super-secret-token-abc123") || strings.Contains(got, "leak-me"))
			require.Equal(
				t, want, got,
			)

		}
	}
	require.True(t,
		seen)

}

// TestErrorStringStripsURL pins SanitizeHTTPClientError on the
// delivery-error path: a *url.Error containing a token in its URL must
// not surface verbatim in WebhookResult.Error.
func TestErrorStringStripsURL(t *testing.T) {
	t.Parallel()

	urlErr := &url.Error{
		Op:  "Post",
		URL: "https://example.com/cb/super-secret-token-abc123?sig=leak-me",
		Err: errors.New("connection reset by peer"),
	}
	got := httputil.SanitizeHTTPClientError(urlErr)
	require.False(
		t, strings.Contains(got, "super-secret-token-abc123") || strings.Contains(got, "leak-me"))

	// Wire the same expectation through the worker delivery path: when
	// the package-level helper sanitizes the http.Client error, the
	// resulting WebhookResult.Error must be free of URL fragments.
	job := &domain.Job{ID: "job-1", WebhookURL: urlErr.URL}
	run := webhookTestRun()

	// Build a client that always returns the secret-bearing url.Error.
	leakyClient := &http.Client{Transport: roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
		return nil, urlErr
	})}
	result := sendWebhookWithClientForTest(context.Background(), leakyClient, job, run, 1)
	require.False(
		t, result.Delivered,
	)
	require.False(
		t, strings.Contains(result.Error,
			"super-secret-token-abc123",
		) ||
			strings.Contains(
				result.Error,
				"leak-me",
			))

}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// FuzzRedactURLForLogNeverLeaksQuery feeds arbitrary raw URLs and
// asserts the helper's output never carries a query or fragment marker.
// Provides the worker package its own fuzz coverage of the logging
// invariant -- httputil has its own dedicated fuzz seeds elsewhere.
func FuzzRedactURLForLogNeverLeaksQuery(f *testing.F) {
	seeds := []string{
		"",
		"https://example.com",
		"https://example.com/cb?token=abc",
		"https://example.com/cb#frag",
		"http://user:pass@example.com/cb?x=1",
		"://malformed",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		got := httputil.RedactURLForLog(raw)
		require.False(
			t, strings.ContainsAny(got,
				"?#"))

	})
}
