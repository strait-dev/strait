package telemetry

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/getsentry/sentry-go"
	"github.com/jackc/pgx/v5"

	"strait/internal/queue"
	"strait/internal/store"
)

type testStatusError struct {
	status int
}

func (e testStatusError) Error() string {
	return http.StatusText(e.status)
}

func (e testStatusError) GetStatus() int {
	return e.status
}

func TestBeforeSend_DropsRequestCancellation(t *testing.T) {
	t.Parallel()

	event := &sentry.Event{Request: &sentry.Request{URL: "https://api.example.test/v1/runs"}}
	hint := &sentry.EventHint{OriginalException: context.Canceled}

	if got := BeforeSend(event, hint); got != nil {
		t.Fatal("expected request cancellation to be dropped")
	}
}

func TestBeforeSend_KeepsBackgroundCancellation(t *testing.T) {
	t.Parallel()

	event := &sentry.Event{}
	hint := &sentry.EventHint{OriginalException: context.Canceled}

	if got := BeforeSend(event, hint); got == nil {
		t.Fatal("expected background cancellation to be kept")
		return
	}
}

func TestBeforeSend_DropsValidationAnd4xx(t *testing.T) {
	t.Parallel()

	event := &sentry.Event{}
	hint := &sentry.EventHint{OriginalException: testStatusError{status: http.StatusUnprocessableEntity}}

	if got := BeforeSend(event, hint); got != nil {
		t.Fatal("expected 4xx status error to be dropped")
	}
}

func TestBeforeSend_KeepsGenuine5xx(t *testing.T) {
	t.Parallel()

	event := &sentry.Event{}
	hint := &sentry.EventHint{OriginalException: testStatusError{status: http.StatusInternalServerError}}

	if got := BeforeSend(event, hint); got == nil {
		t.Fatal("expected 5xx status error to be kept")
		return
	}
}

func TestBeforeSend_DoesNotDropPlainPgxNoRows(t *testing.T) {
	t.Parallel()

	event := &sentry.Event{}
	hint := &sentry.EventHint{OriginalException: pgx.ErrNoRows}

	if got := BeforeSend(event, hint); got == nil {
		t.Fatal("plain pgx.ErrNoRows should be kept unless marked expected")
		return
	}
}

func TestBeforeSend_DropsExpectedNotFound(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		err   error
		event *sentry.Event
	}{
		{
			name:  "wrapped expected pgx no rows",
			err:   MarkExpectedNotFound(pgx.ErrNoRows),
			event: &sentry.Event{},
		},
		{
			name: "tagged known store not found",
			err:  store.ErrRunNotFound,
			event: &sentry.Event{
				Tags: map[string]string{"expected_not_found": "true"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := BeforeSend(tc.event, &sentry.EventHint{OriginalException: tc.err}); got != nil {
				t.Fatal("expected marked not-found error to be dropped")
			}
		})
	}
}

func TestBeforeSend_DropsResolvedTransientAndCircuitOpen(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
	}{
		{name: "resolved transient", err: MarkRetryableResolved(errors.New("connection reset by peer"))},
		{name: "queue circuit open", err: queue.ErrCircuitOpen},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := BeforeSend(&sentry.Event{}, &sentry.EventHint{OriginalException: tc.err}); got != nil {
				t.Fatal("expected noise event to be dropped")
			}
		})
	}
}

func TestBeforeSend_SanitizesEvent(t *testing.T) {
	t.Parallel()

	event := &sentry.Event{
		Message: "failed postgres://user:pass@example.test/db",
		Request: &sentry.Request{
			Headers:     map[string]string{"Authorization": "Bearer secret"},
			Cookies:     "session=secret",
			Data:        "token=secret",
			QueryString: "token=secret&ok=1",
		},
		Exception: []sentry.Exception{{Value: "Bearer abc123"}},
		Contexts: map[string]sentry.Context{
			"extra": {"api_key": "strait_abcdefghijklmnopqrstuvwxyz"},
		},
		Breadcrumbs: []*sentry.Breadcrumb{{
			Data: map[string]any{
				"authorization": "Bearer abc",
				"message":       "redis://:pass@example.test:6379",
			},
		}},
	}

	got := BeforeSend(event, &sentry.EventHint{OriginalException: errors.New("boom")})
	if got == nil {
		t.Fatal("expected event to be kept")
		return
	}
	if got.Request.Headers != nil || got.Request.Cookies != "" || got.Request.Data != "" {
		t.Fatal("expected request headers, cookies, and data to be stripped")
	}
	if got.Request.QueryString != "ok=1&token=%5BREDACTED%5D" {
		t.Fatalf("query string = %q, want redacted token", got.Request.QueryString)
	}
	if strings.Contains(got.Message, "postgres://") {
		t.Fatalf("message was not sanitized: %q", got.Message)
	}
	if got.Exception[0].Value != "[REDACTED]" {
		t.Fatalf("exception value = %q, want redacted", got.Exception[0].Value)
	}
	if got.Contexts["extra"]["api_key"] != "[REDACTED]" {
		t.Fatalf("context api_key = %v, want redacted", got.Contexts["extra"]["api_key"])
	}
	if _, ok := got.Breadcrumbs[0].Data["authorization"]; ok {
		t.Fatal("authorization breadcrumb key was not dropped")
	}
	if got.Breadcrumbs[0].Data["message"] != "[REDACTED]" {
		t.Fatalf("breadcrumb message = %v, want redacted", got.Breadcrumbs[0].Data["message"])
	}
}

func TestSanitizeQueryString_RedactsCommonCredentialParameters(t *testing.T) {
	t.Parallel()

	got := SanitizeQueryString("sig=signed&code=oauth-code&jwt=header.payload&session_id=cookievalue&sid=short&samlresponse=assertion&ticket=tgt&ok=1&tenant=prod")
	for _, leaked := range []string{"signed", "oauth-code", "header.payload", "cookievalue", "short", "assertion", "tgt"} {
		if strings.Contains(got, leaked) {
			t.Fatalf("sanitized query leaked %q: %s", leaked, got)
		}
	}
	for _, key := range []string{"sig", "code", "jwt", "session_id", "sid", "samlresponse", "ticket"} {
		if !strings.Contains(got, key+"=%5BREDACTED%5D") {
			t.Fatalf("sanitized query missing redaction for %s: %s", key, got)
		}
	}
	for _, preserved := range []string{"ok=1", "tenant=prod"} {
		if !strings.Contains(got, preserved) {
			t.Fatalf("sanitized query should preserve %s: %s", preserved, got)
		}
	}
}

func TestBeforeSendTransaction_SamplesHeavyTransactions(t *testing.T) {
	t.Parallel()

	var dropped bool
	for i := range 200 {
		name := "GET /v1/runs/stream/" + string(rune('a'+(i%26)))
		if stableModulo(name, sentryHeavyTransactionModulo) != 0 {
			dropped = BeforeSendTransaction(&sentry.Event{Transaction: name}, nil) == nil
			break
		}
	}
	if !dropped {
		t.Fatal("expected a heavy transaction outside the 1% sample to be dropped")
	}

	if got := BeforeSendTransaction(&sentry.Event{Transaction: "GET /v1/jobs"}, nil); got == nil {
		t.Fatal("expected non-heavy transaction to be kept")
	}
}

func TestSentryTracesSamplerDropsHeavyTransactionsEarly(t *testing.T) {
	t.Parallel()

	sampler := SentryTracesSampler(0.5)
	heavy := sampler(sentry.SamplingContext{Span: &sentry.Span{Name: "GET /v1/runs/stream"}})
	if heavy != 0 {
		t.Fatalf("heavy transaction sample rate = %v, want 0", heavy)
	}
	normal := sampler(sentry.SamplingContext{Span: &sentry.Span{Name: "GET /v1/jobs"}})
	if normal != 0.5 {
		t.Fatalf("normal transaction sample rate = %v, want 0.5", normal)
	}
	parentTrue := sampler(sentry.SamplingContext{
		Span:          &sentry.Span{Name: "GET /v1/jobs"},
		ParentSampled: sentry.SampledTrue,
	})
	if parentTrue != 0.5 {
		t.Fatalf("sampled parent sample rate = %v, want 0.5", parentTrue)
	}
	parentFalse := sampler(sentry.SamplingContext{
		Span:          &sentry.Span{Name: "GET /v1/jobs"},
		ParentSampled: sentry.SampledFalse,
	})
	if parentFalse != 0.5 {
		t.Fatalf("unsampled parent sample rate = %v, want 0.5", parentFalse)
	}
}

func TestInitSentry_NoDSNNoop(t *testing.T) {
	t.Parallel()

	shutdown, err := InitSentry(SentryConfig{Environment: "test"})
	if err != nil {
		t.Fatalf("InitSentry error = %v", err)
	}
	if shutdown == nil {
		t.Fatal("expected no-op shutdown function")
		return
	}
	shutdown()
}
