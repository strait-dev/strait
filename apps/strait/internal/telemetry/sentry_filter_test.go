package telemetry

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/getsentry/sentry-go"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

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
	require.Nil(t, BeforeSend(event, hint))
}

func TestBeforeSend_KeepsBackgroundCancellation(t *testing.T) {
	t.Parallel()

	event := &sentry.Event{}
	hint := &sentry.EventHint{OriginalException: context.Canceled}
	require.NotNil(t,
		BeforeSend(event,
			hint))
}

func TestBeforeSend_DropsValidationAnd4xx(t *testing.T) {
	t.Parallel()

	event := &sentry.Event{}
	hint := &sentry.EventHint{OriginalException: testStatusError{status: http.StatusUnprocessableEntity}}
	require.Nil(t, BeforeSend(event, hint))
}

func TestBeforeSend_KeepsGenuine5xx(t *testing.T) {
	t.Parallel()

	event := &sentry.Event{}
	hint := &sentry.EventHint{OriginalException: testStatusError{status: http.StatusInternalServerError}}
	require.NotNil(t,
		BeforeSend(event,
			hint))
}

func TestBeforeSend_DoesNotDropPlainPgxNoRows(t *testing.T) {
	t.Parallel()

	event := &sentry.Event{}
	hint := &sentry.EventHint{OriginalException: pgx.ErrNoRows}
	require.NotNil(t,
		BeforeSend(event,
			hint))
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
			require.Nil(t, BeforeSend(tc.event,
				&sentry.EventHint{OriginalException: tc.err}))
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
			require.Nil(t, BeforeSend(&sentry.
				Event{}, &sentry.
				EventHint{OriginalException: tc.err}))
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
	require.NotNil(t, got)

	require.False(t, got.
		Request.
		Headers != nil ||
		got.Request.
			Cookies !=
			"" || got.Request.Data != "",
	)
	require.Equal(t, "ok=1&token=%5BREDACTED%5D",

		got.Request.
			QueryString,
	)
	require.NotContains(t, got.Message, "postgres://")
	require.Equal(t, "[REDACTED]",

		got.Exception[0].Value,
	)
	require.Equal(t, "[REDACTED]",

		got.Contexts["extra"]["api_key"],
	)

	require.NotContains(t, got.Breadcrumbs[0].Data, "authorization")
	require.Equal(t, "[REDACTED]",

		got.Breadcrumbs[0].Data["message"])
}

func TestSanitizeQueryString_RedactsCommonCredentialParameters(t *testing.T) {
	t.Parallel()

	got := SanitizeQueryString("sig=signed&code=oauth-code&jwt=header.payload&session_id=cookievalue&sid=short&samlresponse=assertion&ticket=tgt&ok=1&tenant=prod")
	for _, leaked := range []string{"signed", "oauth-code", "header.payload", "cookievalue", "short", "assertion", "tgt"} {
		require.NotContains(t, got, leaked)
	}
	for _, key := range []string{"sig", "code", "jwt", "session_id", "sid", "samlresponse", "ticket"} {
		require.Contains(t, got, key+
			"=%5BREDACTED%5D")
	}
	for _, preserved := range []string{"ok=1", "tenant=prod"} {
		require.Contains(t, got, preserved)
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
	require.True(t, dropped)
	require.NotNil(t,
		BeforeSendTransaction(&sentry.
			Event{Transaction: "GET /v1/jobs"}, nil))
}

func TestSentryTracesSamplerDropsHeavyTransactionsEarly(t *testing.T) {
	t.Parallel()

	sampler := SentryTracesSampler(0.5)
	heavy := sampler(sentry.SamplingContext{Span: &sentry.Span{Name: "GET /v1/runs/stream"}})
	require.InDelta(t, 0,
		heavy, 1e-9)

	normal := sampler(sentry.SamplingContext{Span: &sentry.Span{Name: "GET /v1/jobs"}})
	require.InDelta(t, 0.5,
		normal, 1e-9)

	parentTrue := sampler(sentry.SamplingContext{
		Span:          &sentry.Span{Name: "GET /v1/jobs"},
		ParentSampled: sentry.SampledTrue,
	})
	require.InDelta(t, 0.5,
		parentTrue, 1e-9,
	)

	parentFalse := sampler(sentry.SamplingContext{
		Span:          &sentry.Span{Name: "GET /v1/jobs"},
		ParentSampled: sentry.SampledFalse,
	})
	require.InDelta(t, 0.5,
		parentFalse, 1e-9,
	)
}

func TestInitSentry_NoDSNNoop(t *testing.T) {
	t.Parallel()

	shutdown, err := InitSentry(SentryConfig{Environment: "test"})
	require.NoError(t,
		err)
	require.NotNil(t, shutdown)

	shutdown()
}
