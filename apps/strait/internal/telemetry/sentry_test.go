package telemetry

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/getsentry/sentry-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSentryHandler_ReturnsNonNil(t *testing.T) {
	t.Parallel()
	inner := slog.NewTextHandler(&bytes.Buffer{}, nil)
	h := NewSentryHandler(inner)
	require.NotNil(t,
		h)

}

func TestNewSentryHandler_WrapsInner(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	h := NewSentryHandler(inner)

	// Log through the handler and verify it reaches the inner handler.
	logger := slog.New(h)
	logger.Info("test message", "key", "value")
	assert.True(t, strings.Contains(buf.String(), "test message"))

}

func TestSentryHandler_Enabled_RespectsInnerLevel(t *testing.T) {
	t.Parallel()
	inner := slog.NewTextHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelWarn})
	h := NewSentryHandler(inner)
	ctx := context.Background()
	assert.False(t, h.
		Enabled(ctx,
			slog.LevelDebug,
		))
	assert.False(t, h.
		Enabled(ctx,
			slog.LevelInfo,
		))
	assert.True(t, h.
		Enabled(ctx,
			slog.LevelWarn,
		))
	assert.True(t, h.
		Enabled(ctx,
			slog.LevelError,
		))

}

func TestSentryHandler_WithAttrs_ReturnsNewHandler(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	h := NewSentryHandler(inner)

	attrs := []slog.Attr{slog.String("run_id", "run-123")}
	newH := h.WithAttrs(attrs)
	require.NotNil(t,
		newH)

	// The new handler should be a SentryHandler.
	sh, ok := newH.(*SentryHandler)
	require.True(t, ok)
	assert.NotEqual(t,
		h, sh)

	// Verify the attr is included in log output.
	logger := slog.New(newH)
	logger.Info("with attrs test")
	assert.True(t, strings.Contains(buf.String(), "run_id"))

}

func TestSentryHandler_WithAttrs_Empty(t *testing.T) {
	t.Parallel()
	inner := slog.NewTextHandler(&bytes.Buffer{}, nil)
	h := NewSentryHandler(inner)

	newH := h.WithAttrs(nil)
	require.NotNil(t,
		newH)

}

func TestSentryHandler_WithGroup_ReturnsNewHandler(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	h := NewSentryHandler(inner)

	newH := h.WithGroup("request")
	require.NotNil(t,
		newH)

	sh, ok := newH.(*SentryHandler)
	require.True(t, ok)
	assert.NotEqual(t,
		h, sh)

	// Verify the group prefix appears in log output.
	logger := slog.New(newH)
	logger.Info("with group test", "method", "GET")
	assert.True(t, strings.Contains(buf.String(), "request.method"))

}

func TestSentryHandler_Handle_DelegatesToInner(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	h := NewSentryHandler(inner)

	logger := slog.New(h)
	logger.Info("info message", "key", "value")
	assert.True(t, strings.Contains(buf.String(), "info message"))

}

func TestSentryHandler_Handle_InfoDoesNotPanic(t *testing.T) {
	t.Parallel()
	inner := slog.NewTextHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelInfo})
	h := NewSentryHandler(inner)

	// Info-level should delegate to inner but skip Sentry (no panic, no error).
	logger := slog.New(h)
	logger.Info("below error level")
	logger.Warn("still below error level")
}

func TestSentryHandler_Handle_ErrorLevel(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelError})
	h := NewSentryHandler(inner)

	// Error-level should be handled without panic. Sentry may not be
	// initialized in tests, but Handle should not return an error from
	// the inner handler delegation.
	logger := slog.New(h)
	logger.Error("something failed", "error", errors.New("test error"), "run_id", "run-001")
	assert.True(t, strings.Contains(buf.String(), "something failed"))

}

func TestSentryHandler_Handle_ErrorWithTagKeys(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelError})
	h := NewSentryHandler(inner)

	// Verify that known tag keys (run_id, job_id, etc.) are handled.
	logger := slog.New(h)
	logger.Error("tag test",
		"run_id", "run-001",
		"job_id", "job-002",
		"project_id", "proj-003",
		"operation", "dispatch",
		"status_code", "500",
	)

	output := buf.String()
	assert.True(t, strings.Contains(output,
		"tag test",
	))

}

func TestSentryHandler_Handle_ErrorWithSensitiveAttrs(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelError})
	h := NewSentryHandler(inner)

	// Attrs with sensitive keys are passed through to the inner handler as-is,
	// but SanitizeValue is called inside Handle for Sentry scope.
	// The inner handler gets the original values.
	logger := slog.New(h)
	logger.Error("sensitive test",
		"password", "secret123",
		"token", "tok_abc",
	)

	output := buf.String()
	assert.True(t, strings.Contains(output,
		"sensitive test",
	))

}

func TestSentryHandler_Handle_ErrorWithMessageOnly(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelError})
	h := NewSentryHandler(inner)

	// Error level without an "error" attr should use CaptureMessage path.
	logger := slog.New(h)
	logger.Error("plain error message")
	assert.True(t, strings.Contains(buf.String(), "plain error message"))

}

func TestSanitizeValue_SensitiveKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		key  string
		val  string
		want string
	}{
		{"password", "secret123", "[REDACTED]"},
		{"db_password", "p4ss", "[REDACTED]"},
		{"secret", "s3cr3t", "[REDACTED]"},
		{"api_token", "tok_abc", "[REDACTED]"},
		{"dsn", "postgres://user:pass@host/db", "[REDACTED]"},
		{"api_key", "key_123", "[REDACTED]"},
		{"authorization", "Bearer xyz", "[REDACTED]"},
		{"credential", "cred_abc", "[REDACTED]"},
		{"auth_config", `{"token":"x"}`, "[REDACTED]"},
		{"PASSWORD", "upper", "[REDACTED]"},
		{"Auth_Token", "mixed", "[REDACTED]"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			t.Parallel()
			got := SanitizeValue(tt.key, tt.val)
			assert.Equal(t, tt.
				want, got,
			)

		})
	}
}

func TestSanitizeValue_NonSensitiveKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		key  string
		val  string
		want string
	}{
		{"run_id", "run-001", "run-001"},
		{"status", "completed", "completed"},
		{"message", "hello world", "hello world"},
		{"count", "42", "42"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			t.Parallel()
			got := SanitizeValue(tt.key, tt.val)
			assert.Equal(t, tt.
				want, got,
			)

		})
	}
}

func TestSanitizeValue_NonSensitiveKeyWithSecretPattern(t *testing.T) {
	t.Parallel()

	// A non-sensitive key but value contains a connection string.
	got := SanitizeValue("error_message", "failed: postgres://user:pass@host:5432/db")
	assert.True(t, strings.Contains(got, "[REDACTED]"))
	assert.False(t, strings.Contains(got,
		"postgres://",
	))

}

func TestSanitizeSentryEvent_RedactsNestedContext(t *testing.T) {
	t.Parallel()

	event := &sentry.Event{
		Contexts: map[string]sentry.Context{
			"nested": {
				"request": map[string]any{
					"token": "secret-token",
					"error": "postgres://user:pass@host:5432/db",
					"items": []any{
						map[string]any{"authorization": "Bearer abc123"},
						"redis://default:pass@redis:6379/0",
					},
				},
			},
		},
	}

	sanitizeSentryEvent(event)

	request := event.Contexts["nested"]["request"].(map[string]any)
	require.NotContains(t, request, "token")
	require.NotContains(t, request["error"].(string), "postgres://")
	items := request["items"].([]any)
	item := items[0].(map[string]any)
	require.NotContains(t, item, "authorization")
	require.NotContains(t, items[1].(string), "redis://")
}

func TestSanitizeBreadcrumbData_RedactsNestedValues(t *testing.T) {
	t.Parallel()

	got := sanitizeBreadcrumbData(map[string]any{
		"headers": "Authorization: Bearer abc",
		"details": map[string]any{
			"secret": "value",
			"error":  "redis://default:pass@redis:6379/0",
		},
	})

	require.NotContains(t, got, "headers")
	details := got["details"].(map[string]any)
	require.NotContains(t, details, "secret")
	require.NotContains(t, details["error"].(string), "redis://")
}

func TestScrubSecrets_PostgresURL(t *testing.T) {
	t.Parallel()
	input := "connection failed: postgres://admin:password@db.host.com:5432/mydb"
	got := ScrubSecrets(input)
	assert.False(t, strings.Contains(got,
		"postgres://",
	))
	assert.True(t, strings.Contains(got, "[REDACTED]"))

}

func TestScrubSecrets_RedisURL(t *testing.T) {
	t.Parallel()
	input := "redis error: redis://default:pass@redis.host:6379/0"
	got := ScrubSecrets(input)
	assert.False(t, strings.Contains(got,
		"redis://"))

}

func TestScrubSecrets_ClickHouseURL(t *testing.T) {
	t.Parallel()
	input := "insert failed: clickhouse://user:pass@ch.host:9000/analytics"
	got := ScrubSecrets(input)
	assert.False(t, strings.Contains(got,
		"clickhouse://",
	))

}

func TestScrubSecrets_BearerToken(t *testing.T) {
	t.Parallel()
	input := "auth header: Bearer eyJhbGciOiJIUzI1NiJ9.payload.signature"
	got := ScrubSecrets(input)
	assert.False(t, strings.Contains(got,
		"Bearer eyJ",
	))

}

func TestScrubSecrets_APIKey(t *testing.T) {
	t.Parallel()
	input := "key used: strait_abcdefghij1234567890"
	got := ScrubSecrets(input)
	assert.False(t, strings.Contains(got,
		"strait_abcdefghij",
	))

}

func TestScrubSecrets_SentryDSN(t *testing.T) {
	t.Parallel()
	input := "sentry dsn: https://sentry.io/12345"
	got := ScrubSecrets(input)
	assert.False(t, strings.Contains(got,
		"sentry.io/12345",
	))

}

func TestScrubSecrets_HTTPWithCredentials(t *testing.T) {
	t.Parallel()
	input := "url: https://user:password@api.example.com/v1/data"
	got := ScrubSecrets(input)
	assert.False(t, strings.Contains(got,
		"user:password@",
	))

}

func TestScrubSecrets_MultiplePatterns(t *testing.T) {
	t.Parallel()
	input := "db=postgres://u:p@h/d redis=redis://x:y@r:6379 token=Bearer abc123"
	got := ScrubSecrets(input)
	count := strings.Count(got, "[REDACTED]")
	assert.GreaterOrEqual(t, count,
		3)

}

func TestScrubSecrets_NoSecrets(t *testing.T) {
	t.Parallel()
	input := "everything is fine, no secrets here"
	got := ScrubSecrets(input)
	assert.Equal(t, input,
		got)

}

func TestScrubSecrets_EmptyString(t *testing.T) {
	t.Parallel()
	got := ScrubSecrets("")
	assert.Equal(t, "",
		got)

}

func TestSanitizeQueryString_InvalidInput(t *testing.T) {
	t.Parallel()
	got := SanitizeQueryString("%ZZ%YY%invalid")
	assert.Equal(t, "",
		got)

}

func TestSanitizeQueryString_RedactsCredentialAliases(t *testing.T) {
	t.Parallel()

	got := SanitizeQueryString("access_token=a&client_secret=b&x-api-key=c&signature=d&tenant=prod")
	for _, secret := range []string{"=a", "=b", "=c", "=d"} {
		require.False(t,
			strings.Contains(got,
				secret))

	}
	for _, want := range []string{
		"access_token=%5BREDACTED%5D",
		"client_secret=%5BREDACTED%5D",
		"signature=%5BREDACTED%5D",
		"tenant=prod",
		"x-api-key=%5BREDACTED%5D",
	} {
		require.True(t, strings.Contains(got,
			want))

	}
}

func TestSentryTagKeys_Contains_KnownKeys(t *testing.T) {
	t.Parallel()
	knownKeys := []string{
		"run_id", "job_id", "project_id", "workflow_run_id",
		"delivery_id", "trigger_id", "step_run_id",
		"table", "batch_key", "error_class", "attempt",
		"status_code", "operation", "consumer", "approval_id",
		"key_id", "subscription_id", "source_id", "drain_id", "batch_id",
	}

	for _, key := range knownKeys {
		_, ok := SentryTagFromString(key)
		assert.True(t, ok)
	}
}

func TestSentryTagKeys_DoesNotContain_NonTagKeys(t *testing.T) {
	t.Parallel()
	nonTagKeys := []string{"error", "message", "level", "timestamp", "random_key"}
	for _, key := range nonTagKeys {
		_, ok := SentryTagFromString(key)
		assert.False(t, ok)
	}
}

func TestSentryHandler_ImplementsHandlerInterface(t *testing.T) {
	t.Parallel()
	inner := slog.NewTextHandler(&bytes.Buffer{}, nil)
	var h slog.Handler = NewSentryHandler(inner)
	_ = h
}

func TestSentryHandler_ChainedWithAttrsAndGroup(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	h := NewSentryHandler(inner)

	// Chain WithGroup and WithAttrs.
	chained := h.WithGroup("http").WithAttrs([]slog.Attr{slog.String("method", "POST")})
	logger := slog.New(chained)
	logger.Info("chained test", "path", "/api/v1")

	output := buf.String()
	assert.True(t, strings.Contains(output,
		"http.method",
	))
	assert.True(t, strings.Contains(output,
		"http.path",
	))

}

type sentryEventCollector struct {
	mu     sync.Mutex
	events []*sentry.Event
}

func (c *sentryEventCollector) collect(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, event)
	return event
}

func (c *sentryEventCollector) len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.events)
}

func (c *sentryEventCollector) get(i int) *sentry.Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.events[i]
}

func initTestSentry(t *testing.T, collector *sentryEventCollector) {
	t.Helper()
	err := sentry.Init(sentry.ClientOptions{
		Dsn:        "https://examplePublicKey@o0.ingest.sentry.io/0",
		BeforeSend: collector.collect,
		Transport:  &sentry.HTTPSyncTransport{},
	})
	require.NoError(t,
		err)

	t.Cleanup(func() {
		sentry.Flush(0)
	})
}

func TestSentryHandler_ErrorLevel_CapturesCalled(t *testing.T) {
	collector := &sentryEventCollector{}
	initTestSentry(t, collector)

	var buf bytes.Buffer
	inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	h := NewSentryHandler(inner)
	logger := slog.New(h)

	logger.Error("test error event")
	require.Equal(t,
		1, collector.
			len())

}

func TestSentryHandler_WarnLevel_NoCaptureCall(t *testing.T) {
	collector := &sentryEventCollector{}
	initTestSentry(t, collector)

	var buf bytes.Buffer
	inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	h := NewSentryHandler(inner)
	logger := slog.New(h)

	logger.Warn("test warning event")
	require.Equal(t,
		0, collector.
			len())

}

func TestSentryHandler_ErrorWithErrorAttr_CapturesException(t *testing.T) {
	collector := &sentryEventCollector{}
	initTestSentry(t, collector)

	var buf bytes.Buffer
	inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	h := NewSentryHandler(inner)
	logger := slog.New(h)

	logger.Error("failed operation", "error", errors.New("db connection lost"))
	require.Equal(t,
		1, collector.
			len())

	event := collector.get(0)
	require.NotEmpty(
		t, event.Exception,
	)

}

func TestSentryHandler_ErrorWithoutErrorAttr_CapturesMessage(t *testing.T) {
	collector := &sentryEventCollector{}
	initTestSentry(t, collector)

	var buf bytes.Buffer
	inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	h := NewSentryHandler(inner)
	logger := slog.New(h)

	logger.Error("plain error without error attr", "some_key", "some_value")
	require.Equal(t,
		1, collector.
			len())

	event := collector.get(0)
	assert.Equal(t, "plain error without error attr",

		event.Message,
	)

}

func TestSentryHandler_TagKeys_SetAsTag(t *testing.T) {
	collector := &sentryEventCollector{}
	initTestSentry(t, collector)

	var buf bytes.Buffer
	inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	h := NewSentryHandler(inner)
	logger := slog.New(h)

	logger.Error("tag test", "run_id", "run-123", "project_id", "proj-456")
	require.Equal(t,
		1, collector.
			len())

	event := collector.get(0)
	assert.Equal(t, "run-123",
		event.
			Tags["run_id"])
	assert.Equal(t, "proj-456",

		event.Tags["project_id"])

}

func TestSentryHandler_UsesContextHubScope(t *testing.T) {
	collector := &sentryEventCollector{}
	initTestSentry(t, collector)

	ctx := EnsureSentryHub(context.Background())
	hub := sentry.GetHubFromContext(ctx)
	require.NotNil(t,
		hub)

	hub.ConfigureScope(func(scope *sentry.Scope) {
		SetSentryTag(scope, TagRequestID, "req-123")
		scope.SetContext("http.request", sentry.Context{"route": "/v1/jobs"})
	})

	var buf bytes.Buffer
	inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(NewSentryHandler(inner))

	logger.ErrorContext(ctx, "request failed", "error", errors.New("boom"))
	require.Equal(t,
		1, collector.
			len())

	event := collector.get(0)
	require.Equal(t,
		"req-123",
		event.Tags["request_id"])

	require.Equal(t,
		"/v1/jobs",
		event.Contexts["http.request"]["route"])

}

func TestSentryHandler_NonTagKeys_SetAsExtra(t *testing.T) {
	collector := &sentryEventCollector{}
	initTestSentry(t, collector)

	var buf bytes.Buffer
	inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	h := NewSentryHandler(inner)
	logger := slog.New(h)

	logger.Error("extra test", "custom_field", "custom_value")
	require.Equal(t,
		1, collector.
			len())

	event := collector.get(0)
	assert.Equal(t, "custom_value",

		event.
			Contexts["extra"]["custom_field"])

	assert.NotContains(t, event.Tags, "custom_field")
}
