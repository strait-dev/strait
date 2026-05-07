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
)

// --- NewSentryHandler tests.

func TestNewSentryHandler_ReturnsNonNil(t *testing.T) {
	t.Parallel()
	inner := slog.NewTextHandler(&bytes.Buffer{}, nil)
	h := NewSentryHandler(inner)
	if h == nil {
		t.Fatal("NewSentryHandler returned nil")
	}
}

func TestNewSentryHandler_WrapsInner(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	h := NewSentryHandler(inner)

	// Log through the handler and verify it reaches the inner handler.
	logger := slog.New(h)
	logger.Info("test message", "key", "value")

	if !strings.Contains(buf.String(), "test message") {
		t.Errorf("inner handler did not receive log: %s", buf.String())
	}
}

// --- Enabled tests.

func TestSentryHandler_Enabled_RespectsInnerLevel(t *testing.T) {
	t.Parallel()
	inner := slog.NewTextHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelWarn})
	h := NewSentryHandler(inner)
	ctx := context.Background()

	if h.Enabled(ctx, slog.LevelDebug) {
		t.Error("expected Enabled(Debug) = false for Warn-level inner handler")
	}
	if h.Enabled(ctx, slog.LevelInfo) {
		t.Error("expected Enabled(Info) = false for Warn-level inner handler")
	}
	if !h.Enabled(ctx, slog.LevelWarn) {
		t.Error("expected Enabled(Warn) = true for Warn-level inner handler")
	}
	if !h.Enabled(ctx, slog.LevelError) {
		t.Error("expected Enabled(Error) = true for Warn-level inner handler")
	}
}

// --- WithAttrs tests.

func TestSentryHandler_WithAttrs_ReturnsNewHandler(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	h := NewSentryHandler(inner)

	attrs := []slog.Attr{slog.String("run_id", "run-123")}
	newH := h.WithAttrs(attrs)

	if newH == nil {
		t.Fatal("WithAttrs returned nil")
	}

	// The new handler should be a SentryHandler.
	sh, ok := newH.(*SentryHandler)
	if !ok {
		t.Fatalf("WithAttrs returned %T, want *SentryHandler", newH)
	}
	if sh == h {
		t.Error("WithAttrs returned the same handler, want a new one")
	}

	// Verify the attr is included in log output.
	logger := slog.New(newH)
	logger.Info("with attrs test")

	if !strings.Contains(buf.String(), "run_id") {
		t.Errorf("expected run_id in output: %s", buf.String())
	}
}

func TestSentryHandler_WithAttrs_Empty(t *testing.T) {
	t.Parallel()
	inner := slog.NewTextHandler(&bytes.Buffer{}, nil)
	h := NewSentryHandler(inner)

	newH := h.WithAttrs(nil)
	if newH == nil {
		t.Fatal("WithAttrs(nil) returned nil")
	}
}

// --- WithGroup tests.

func TestSentryHandler_WithGroup_ReturnsNewHandler(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	h := NewSentryHandler(inner)

	newH := h.WithGroup("request")

	if newH == nil {
		t.Fatal("WithGroup returned nil")
	}

	sh, ok := newH.(*SentryHandler)
	if !ok {
		t.Fatalf("WithGroup returned %T, want *SentryHandler", newH)
	}
	if sh == h {
		t.Error("WithGroup returned the same handler, want a new one")
	}

	// Verify the group prefix appears in log output.
	logger := slog.New(newH)
	logger.Info("with group test", "method", "GET")

	if !strings.Contains(buf.String(), "request.method") {
		t.Errorf("expected grouped key in output: %s", buf.String())
	}
}

// --- Handle tests.

func TestSentryHandler_Handle_DelegatesToInner(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	h := NewSentryHandler(inner)

	logger := slog.New(h)
	logger.Info("info message", "key", "value")

	if !strings.Contains(buf.String(), "info message") {
		t.Errorf("inner handler missing log: %s", buf.String())
	}
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

	if !strings.Contains(buf.String(), "something failed") {
		t.Errorf("inner handler missing error log: %s", buf.String())
	}
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
	if !strings.Contains(output, "tag test") {
		t.Errorf("inner handler missing log: %s", output)
	}
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
	if !strings.Contains(output, "sensitive test") {
		t.Errorf("inner handler missing log: %s", output)
	}
}

func TestSentryHandler_Handle_ErrorWithMessageOnly(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelError})
	h := NewSentryHandler(inner)

	// Error level without an "error" attr should use CaptureMessage path.
	logger := slog.New(h)
	logger.Error("plain error message")

	if !strings.Contains(buf.String(), "plain error message") {
		t.Errorf("inner handler missing log: %s", buf.String())
	}
}

// --- SanitizeValue tests.

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
			if got != tt.want {
				t.Errorf("SanitizeValue(%q, %q) = %q, want %q", tt.key, tt.val, got, tt.want)
			}
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
			if got != tt.want {
				t.Errorf("SanitizeValue(%q, %q) = %q, want %q", tt.key, tt.val, got, tt.want)
			}
		})
	}
}

func TestSanitizeValue_NonSensitiveKeyWithSecretPattern(t *testing.T) {
	t.Parallel()

	// A non-sensitive key but value contains a connection string.
	got := SanitizeValue("error_message", "failed: postgres://user:pass@host:5432/db")
	if !strings.Contains(got, "[REDACTED]") {
		t.Errorf("expected connection string scrubbed in value: %q", got)
	}
	if strings.Contains(got, "postgres://") {
		t.Errorf("connection string not scrubbed: %q", got)
	}
}

// --- ScrubSecrets tests.

func TestScrubSecrets_PostgresURL(t *testing.T) {
	t.Parallel()
	input := "connection failed: postgres://admin:password@db.host.com:5432/mydb"
	got := ScrubSecrets(input)
	if strings.Contains(got, "postgres://") {
		t.Errorf("postgres URL not scrubbed: %q", got)
	}
	if !strings.Contains(got, "[REDACTED]") {
		t.Errorf("expected [REDACTED] in output: %q", got)
	}
}

func TestScrubSecrets_RedisURL(t *testing.T) {
	t.Parallel()
	input := "redis error: redis://default:pass@redis.host:6379/0"
	got := ScrubSecrets(input)
	if strings.Contains(got, "redis://") {
		t.Errorf("redis URL not scrubbed: %q", got)
	}
}

func TestScrubSecrets_ClickHouseURL(t *testing.T) {
	t.Parallel()
	input := "insert failed: clickhouse://user:pass@ch.host:9000/analytics"
	got := ScrubSecrets(input)
	if strings.Contains(got, "clickhouse://") {
		t.Errorf("clickhouse URL not scrubbed: %q", got)
	}
}

func TestScrubSecrets_BearerToken(t *testing.T) {
	t.Parallel()
	input := "auth header: Bearer eyJhbGciOiJIUzI1NiJ9.payload.signature"
	got := ScrubSecrets(input)
	if strings.Contains(got, "Bearer eyJ") {
		t.Errorf("bearer token not scrubbed: %q", got)
	}
}

func TestScrubSecrets_APIKey(t *testing.T) {
	t.Parallel()
	input := "key used: strait_abcdefghij1234567890"
	got := ScrubSecrets(input)
	if strings.Contains(got, "strait_abcdefghij") {
		t.Errorf("API key not scrubbed: %q", got)
	}
}

func TestScrubSecrets_SentryDSN(t *testing.T) {
	t.Parallel()
	input := "sentry dsn: https://sentry.io/12345"
	got := ScrubSecrets(input)
	if strings.Contains(got, "sentry.io/12345") {
		t.Errorf("sentry DSN not scrubbed: %q", got)
	}
}

func TestScrubSecrets_HTTPWithCredentials(t *testing.T) {
	t.Parallel()
	input := "url: https://user:password@api.example.com/v1/data"
	got := ScrubSecrets(input)
	if strings.Contains(got, "user:password@") {
		t.Errorf("HTTP credentials not scrubbed: %q", got)
	}
}

func TestScrubSecrets_MultiplePatterns(t *testing.T) {
	t.Parallel()
	input := "db=postgres://u:p@h/d redis=redis://x:y@r:6379 token=Bearer abc123"
	got := ScrubSecrets(input)
	count := strings.Count(got, "[REDACTED]")
	if count < 3 {
		t.Errorf("expected at least 3 redactions, got %d in: %q", count, got)
	}
}

func TestScrubSecrets_NoSecrets(t *testing.T) {
	t.Parallel()
	input := "everything is fine, no secrets here"
	got := ScrubSecrets(input)
	if got != input {
		t.Errorf("ScrubSecrets(%q) = %q, want original string", input, got)
	}
}

func TestScrubSecrets_EmptyString(t *testing.T) {
	t.Parallel()
	got := ScrubSecrets("")
	if got != "" {
		t.Errorf("ScrubSecrets(\"\") = %q, want empty", got)
	}
}

// --- SanitizeQueryString additional tests.

func TestSanitizeQueryString_InvalidInput(t *testing.T) {
	t.Parallel()
	got := SanitizeQueryString("%ZZ%YY%invalid")
	if got != "" {
		t.Errorf("SanitizeQueryString(invalid) = %q, want empty", got)
	}
}

// --- Sentry tag taxonomy tests.

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
		if _, ok := SentryTagFromString(key); !ok {
			t.Errorf("SentryTagFromString missing %q", key)
		}
	}
}

func TestSentryTagKeys_DoesNotContain_NonTagKeys(t *testing.T) {
	t.Parallel()
	nonTagKeys := []string{"error", "message", "level", "timestamp", "random_key"}
	for _, key := range nonTagKeys {
		if _, ok := SentryTagFromString(key); ok {
			t.Errorf("SentryTagFromString should not contain %q", key)
		}
	}
}

// --- Handler interface compliance.

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
	if !strings.Contains(output, "http.method") {
		t.Errorf("expected grouped attr in output: %s", output)
	}
	if !strings.Contains(output, "http.path") {
		t.Errorf("expected grouped key in output: %s", output)
	}
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
	if err != nil {
		t.Fatalf("sentry.Init error = %v", err)
	}
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

	if collector.len() != 1 {
		t.Fatalf("expected 1 sentry event, got %d", collector.len())
	}
}

func TestSentryHandler_WarnLevel_NoCaptureCall(t *testing.T) {
	collector := &sentryEventCollector{}
	initTestSentry(t, collector)

	var buf bytes.Buffer
	inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	h := NewSentryHandler(inner)
	logger := slog.New(h)

	logger.Warn("test warning event")

	if collector.len() != 0 {
		t.Fatalf("expected 0 sentry events for warn level, got %d", collector.len())
	}
}

func TestSentryHandler_ErrorWithErrorAttr_CapturesException(t *testing.T) {
	collector := &sentryEventCollector{}
	initTestSentry(t, collector)

	var buf bytes.Buffer
	inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	h := NewSentryHandler(inner)
	logger := slog.New(h)

	logger.Error("failed operation", "error", errors.New("db connection lost"))

	if collector.len() != 1 {
		t.Fatalf("expected 1 sentry event, got %d", collector.len())
	}
	event := collector.get(0)
	if len(event.Exception) == 0 {
		t.Fatal("expected exception in sentry event, got message-only")
	}
}

func TestSentryHandler_ErrorWithoutErrorAttr_CapturesMessage(t *testing.T) {
	collector := &sentryEventCollector{}
	initTestSentry(t, collector)

	var buf bytes.Buffer
	inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	h := NewSentryHandler(inner)
	logger := slog.New(h)

	logger.Error("plain error without error attr", "some_key", "some_value")

	if collector.len() != 1 {
		t.Fatalf("expected 1 sentry event, got %d", collector.len())
	}
	event := collector.get(0)
	if event.Message != "plain error without error attr" {
		t.Errorf("expected message='plain error without error attr', got %q", event.Message)
	}
}

func TestSentryHandler_TagKeys_SetAsTag(t *testing.T) {
	collector := &sentryEventCollector{}
	initTestSentry(t, collector)

	var buf bytes.Buffer
	inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	h := NewSentryHandler(inner)
	logger := slog.New(h)

	logger.Error("tag test", "run_id", "run-123", "project_id", "proj-456")

	if collector.len() != 1 {
		t.Fatalf("expected 1 sentry event, got %d", collector.len())
	}
	event := collector.get(0)
	if event.Tags["run_id"] != "run-123" {
		t.Errorf("expected tag run_id=run-123, got %q", event.Tags["run_id"])
	}
	if event.Tags["project_id"] != "proj-456" {
		t.Errorf("expected tag project_id=proj-456, got %q", event.Tags["project_id"])
	}
}

func TestSentryHandler_UsesContextHubScope(t *testing.T) {
	collector := &sentryEventCollector{}
	initTestSentry(t, collector)

	ctx := EnsureSentryHub(context.Background())
	hub := sentry.GetHubFromContext(ctx)
	if hub == nil {
		t.Fatal("expected context hub")
	}
	hub.ConfigureScope(func(scope *sentry.Scope) {
		SetSentryTag(scope, TagRequestID, "req-123")
		scope.SetContext("http.request", sentry.Context{"route": "/v1/jobs"})
	})

	var buf bytes.Buffer
	inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(NewSentryHandler(inner))

	logger.ErrorContext(ctx, "request failed", "error", errors.New("boom"))

	if collector.len() != 1 {
		t.Fatalf("expected 1 sentry event, got %d", collector.len())
	}
	event := collector.get(0)
	if got := event.Tags["request_id"]; got != "req-123" {
		t.Fatalf("request_id tag = %q, want req-123", got)
	}
	if event.Contexts["http.request"]["route"] != "/v1/jobs" {
		t.Fatalf("http.request context = %v, want route", event.Contexts["http.request"])
	}
}

func TestSentryHandler_NonTagKeys_SetAsExtra(t *testing.T) {
	collector := &sentryEventCollector{}
	initTestSentry(t, collector)

	var buf bytes.Buffer
	inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	h := NewSentryHandler(inner)
	logger := slog.New(h)

	logger.Error("extra test", "custom_field", "custom_value")

	if collector.len() != 1 {
		t.Fatalf("expected 1 sentry event, got %d", collector.len())
	}
	event := collector.get(0)
	if event.Contexts["extra"]["custom_field"] != "custom_value" {
		t.Errorf("expected extra custom_field=custom_value, got %v", event.Contexts["extra"]["custom_field"])
	}
	if _, isTag := event.Tags["custom_field"]; isTag {
		t.Error("custom_field should not be a tag")
	}
}
