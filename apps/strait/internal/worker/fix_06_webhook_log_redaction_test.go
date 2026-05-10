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
		if err := json.Unmarshal(line, &m); err != nil {
			t.Fatalf("invalid slog line %q: %v", string(line), err)
		}
		entries = append(entries, m)
	}
	return entries
}

// TestFix_06_LogsEmitOnlySchemeAndHost asserts that the success-path
// "webhook delivered" log carries a redacted URL (scheme://host) and not
// the path/query that may carry secret tokens.
func TestFix_06_LogsEmitOnlySchemeAndHost(t *testing.T) {
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
		if !result.Delivered {
			t.Fatalf("expected delivery to succeed for redaction test, got: %s", result.Error)
		}
	})

	want := httputil.RedactURLForLog(job.WebhookURL)
	for _, e := range entries {
		raw, ok := e["url"].(string)
		if !ok {
			continue
		}
		if strings.Contains(raw, "super-secret-token-abc123") || strings.Contains(raw, "leak-me") {
			t.Fatalf("log entry leaked URL secret: %v", e)
		}
		if raw != want {
			t.Fatalf("log url = %q, want %q (scheme://host only)", raw, want)
		}
	}
}

// TestFix_06_OTelAttributeRedacted checks the webhook.url span attribute
// is recorded as scheme://host -- before the fix the raw user-supplied
// URL was attached, leaking path/query tokens to the trace backend.
func TestFix_06_OTelAttributeRedacted(t *testing.T) {
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
			if strings.Contains(got, "super-secret-token-abc123") || strings.Contains(got, "leak-me") {
				t.Fatalf("span webhook.url leaked secret: %s=%q (span=%q)", attr.Key, got, span.Name)
			}
			if got != want {
				t.Fatalf("span webhook.url = %q, want %q", got, want)
			}
		}
	}
	if !seen {
		t.Fatal("expected at least one span with webhook.url attribute")
	}
}

// TestFix_06_ErrorStringStripsURL pins SanitizeHTTPClientError on the
// delivery-error path: a *url.Error containing a token in its URL must
// not surface verbatim in WebhookResult.Error.
func TestFix_06_ErrorStringStripsURL(t *testing.T) {
	t.Parallel()

	urlErr := &url.Error{
		Op:  "Post",
		URL: "https://example.com/cb/super-secret-token-abc123?sig=leak-me",
		Err: errors.New("connection reset by peer"),
	}
	got := httputil.SanitizeHTTPClientError(urlErr)
	if strings.Contains(got, "super-secret-token-abc123") || strings.Contains(got, "leak-me") {
		t.Fatalf("SanitizeHTTPClientError leaked URL secret: %q", got)
	}

	// Wire the same expectation through the worker delivery path: when
	// the package-level helper sanitizes the http.Client error, the
	// resulting WebhookResult.Error must be free of URL fragments.
	job := &domain.Job{ID: "job-1", WebhookURL: urlErr.URL}
	run := webhookTestRun()

	// Build a client that always returns the secret-bearing url.Error.
	leakyClient := &http.Client{Transport: roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
		return nil, urlErr
	})}
	result := SendWebhookWithClient(context.Background(), leakyClient, job, run, 1)
	if result.Delivered {
		t.Fatal("expected delivered=false; mock transport always errors")
	}
	if strings.Contains(result.Error, "super-secret-token-abc123") || strings.Contains(result.Error, "leak-me") {
		t.Fatalf("WebhookResult.Error leaked URL secret: %q", result.Error)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// FuzzFix_06_RedactURLForLogNeverLeaksQuery feeds arbitrary raw URLs and
// asserts the helper's output never carries a query or fragment marker.
// Provides the worker package its own fuzz coverage of the logging
// invariant -- httputil has its own dedicated fuzz seeds elsewhere.
func FuzzFix_06_RedactURLForLogNeverLeaksQuery(f *testing.F) {
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
		if strings.ContainsAny(got, "?#") {
			t.Fatalf("RedactURLForLog(%q) = %q -- must not contain ? or #", raw, got)
		}
	})
}
