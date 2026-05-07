package telemetry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/jackc/pgx/v5"

	"strait/internal/queue"
	"strait/internal/store"
)

const (
	sentryHeavyTransactionModulo = 100
)

var (
	// ErrExpectedNotFound marks store misses that are expected control flow.
	// Plain pgx.ErrNoRows is not dropped globally because unexpected missing
	// rows often indicate a real consistency bug.
	ErrExpectedNotFound = errors.New("expected not found")

	// ErrRetryableResolved marks transient failures that already recovered
	// before the event would be sent.
	ErrRetryableResolved = errors.New("retryable transient resolved")
)

// SentryConfig contains the runtime values needed to initialize Sentry.
type SentryConfig struct {
	DSN              string
	Environment      string
	Release          string
	TracesSampleRate float64
}

// InitSentry initializes the process-wide Sentry SDK and returns a shutdown
// function. An empty DSN leaves Sentry disabled and returns a no-op shutdown.
func InitSentry(cfg SentryConfig) (func(), error) {
	if cfg.DSN == "" {
		return func() {}, nil
	}
	tracesSampleRate := cfg.TracesSampleRate
	if tracesSampleRate < 0 {
		tracesSampleRate = 0
	}
	if tracesSampleRate > 1 {
		tracesSampleRate = 1
	}
	if err := sentry.Init(SentryClientOptions(cfg, tracesSampleRate)); err != nil {
		return nil, fmt.Errorf("init sentry: %w", err)
	}
	return func() { sentry.Flush(2 * time.Second) }, nil
}

// SentryClientOptions returns the SDK options used by InitSentry. Tests use it
// with a fake transport so classifier behavior is exercised through the SDK.
func SentryClientOptions(cfg SentryConfig, tracesSampleRate float64) sentry.ClientOptions {
	return sentry.ClientOptions{
		Dsn:                   cfg.DSN,
		Environment:           cfg.Environment,
		Release:               cfg.Release,
		AttachStacktrace:      true,
		SampleRate:            1.0,
		TracesSampleRate:      tracesSampleRate,
		BeforeSend:            BeforeSend,
		BeforeSendTransaction: BeforeSendTransaction,
	}
}

// BeforeSend applies Strait's event filter and redaction policy.
func BeforeSend(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
	if event == nil {
		return nil
	}
	if shouldDropSentryEvent(event, hint) {
		return nil
	}
	ApplySentryFingerprint(event, hint)
	sanitizeSentryEvent(event)
	return event
}

// BeforeSendTransaction drops high-volume streaming transactions except for a
// deterministic 1% sample. The SDK's global trace sample rate still applies.
func BeforeSendTransaction(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
	if event == nil {
		return nil
	}
	if isHeavyTransaction(event) && stableModulo(event.Transaction, sentryHeavyTransactionModulo) != 0 {
		return nil
	}
	sanitizeSentryEvent(event)
	return event
}

// MarkExpectedNotFound wraps err so BeforeSend can drop it as expected control
// flow. Use this for API/store paths that already map to 404 or benign misses.
func MarkExpectedNotFound(err error) error {
	if err == nil {
		return nil
	}
	return errors.Join(ErrExpectedNotFound, err)
}

// MarkRetryableResolved wraps a transient err that was already retried
// successfully and should not page Sentry.
func MarkRetryableResolved(err error) error {
	if err == nil {
		return nil
	}
	return errors.Join(ErrRetryableResolved, err)
}

// SentryHandler wraps an slog.Handler and sends Error-level records to Sentry.
type SentryHandler struct {
	inner slog.Handler
}

// NewSentryHandler creates a new SentryHandler wrapping the given handler.
func NewSentryHandler(inner slog.Handler) *SentryHandler {
	return &SentryHandler{inner: inner}
}

func (h *SentryHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *SentryHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &SentryHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h *SentryHandler) WithGroup(name string) slog.Handler {
	return &SentryHandler{inner: h.inner.WithGroup(name)}
}

func (h *SentryHandler) Handle(ctx context.Context, r slog.Record) error {
	// Always delegate to the inner handler first (stdout logging).
	err := h.inner.Handle(ctx, r)

	// Only send Error-level and above to Sentry.
	if r.Level < slog.LevelError {
		return err
	}

	sentry.WithScope(func(scope *sentry.Scope) {
		scope.SetLevel(sentry.LevelError)

		var captureErr error
		extra := sentry.Context{}
		r.Attrs(func(a slog.Attr) bool {
			key := a.Key
			val := fmt.Sprintf("%v", a.Value.Any())

			// If the attr is an error, capture it as an exception.
			if key == "error" {
				if e, ok := a.Value.Any().(error); ok {
					captureErr = e
				}
			}

			// Sanitize values that might contain secrets.
			val = SanitizeValue(key, val)

			// Promote documented identifiers to tags, rest to extra context.
			if tag, ok := SentryTagFromString(key); ok {
				SetSentryTag(scope, tag, val)
			} else {
				extra[key] = val
			}
			return true
		})
		if len(extra) > 0 {
			scope.SetContext("extra", extra)
		}

		if captureErr != nil {
			sentry.CaptureException(captureErr)
		} else {
			sentry.CaptureMessage(r.Message)
		}
	})

	return err
}

// SanitizeValue redacts values for keys that commonly hold secrets,
// and scrubs patterns in error messages that look like connection strings or tokens.
func SanitizeValue(key, val string) string {
	sensitiveKeys := []string{
		"password", "secret", "token", "dsn", "key", "authorization",
		"credential", "auth_config",
	}
	lowerKey := strings.ToLower(key)
	for _, sk := range sensitiveKeys {
		if strings.Contains(lowerKey, sk) {
			return "[REDACTED]"
		}
	}

	return ScrubSecrets(val)
}

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`postgres://[^\s"]+`),
	regexp.MustCompile(`redis://[^\s"]+`),
	regexp.MustCompile(`clickhouse://[^\s"]+`),
	regexp.MustCompile(`https?://[^@\s"]*:[^@\s"]*@[^\s"]+`),
	regexp.MustCompile(`Bearer\s+\S+`),
	regexp.MustCompile(`strait_[a-zA-Z0-9_]{20,}`),
	regexp.MustCompile(`(?i)sentry\.io/\S+`),
}

// ScrubSecrets removes connection strings, bearer tokens, API keys from a string.
func ScrubSecrets(s string) string {
	for _, p := range secretPatterns {
		s = p.ReplaceAllString(s, "[REDACTED]")
	}
	return s
}

// SanitizeQueryString redacts token/key/secret query parameters.
func SanitizeQueryString(qs string) string {
	params, err := url.ParseQuery(qs)
	if err != nil {
		return ""
	}
	sensitive := []string{"token", "api_key", "secret", "key", "password", "auth"}
	for _, k := range sensitive {
		if _, ok := params[k]; ok {
			params.Set(k, "[REDACTED]")
		}
	}
	return params.Encode()
}

func shouldDropSentryEvent(event *sentry.Event, hint *sentry.EventHint) bool {
	err := eventError(event, hint)
	if err != nil {
		if isRequestCancellation(err, event, hint) {
			return true
		}
		if isValidationOr4xx(err) {
			return true
		}
		if isExpectedNotFound(err, event) {
			return true
		}
		if errors.Is(err, ErrRetryableResolved) {
			return true
		}
		if errors.Is(err, queue.ErrCircuitOpen) {
			return true
		}
	}
	if statusClass, ok := event.Tags["status_class"]; ok && statusClass != "" && statusClass != "5xx" {
		return true
	}
	return false
}

func eventError(event *sentry.Event, hint *sentry.EventHint) error {
	if hint != nil && hint.OriginalException != nil {
		return hint.OriginalException
	}
	if event == nil || len(event.Exception) == 0 {
		return nil
	}
	return errors.New(event.Exception[0].Value)
}

func isRequestCancellation(err error, event *sentry.Event, hint *sentry.EventHint) bool {
	if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	return (event != nil && event.Request != nil) || (hint != nil && (hint.Request != nil || hint.Context != nil))
}

type statusError interface {
	GetStatus() int
}

func isValidationOr4xx(err error) bool {
	var se statusError
	if errors.As(err, &se) {
		status := se.GetStatus()
		return status >= 400 && status < 500
	}
	return false
}

func isExpectedNotFound(err error, event *sentry.Event) bool {
	if errors.Is(err, ErrExpectedNotFound) {
		return true
	}
	if isKnownNotFound(err) {
		return event != nil && event.Tags["expected_not_found"] == "true"
	}
	return false
}

func isKnownNotFound(err error) bool {
	return errors.Is(err, pgx.ErrNoRows) ||
		errors.Is(err, store.ErrRunNotFound) ||
		errors.Is(err, store.ErrJobNotFound) ||
		errors.Is(err, store.ErrProjectNotFound) ||
		errors.Is(err, store.ErrWorkflowNotFound) ||
		errors.Is(err, store.ErrWorkflowRunNotFound) ||
		errors.Is(err, store.ErrWorkflowStepRunNotFound) ||
		errors.Is(err, store.ErrWebhookSubscriptionNotFound) ||
		errors.Is(err, store.ErrOutboxRowNotFound)
}

func sanitizeSentryEvent(event *sentry.Event) {
	if event.Request != nil {
		event.Request.Headers = nil
		event.Request.Cookies = ""
		event.Request.Data = ""
		if event.Request.QueryString != "" {
			event.Request.QueryString = SanitizeQueryString(event.Request.QueryString)
		}
	}
	for i := range event.Exception {
		event.Exception[i].Value = ScrubSecrets(event.Exception[i].Value)
	}
	event.Message = ScrubSecrets(event.Message)
	for i := range event.Breadcrumbs {
		if event.Breadcrumbs[i].Data != nil {
			for key, value := range event.Breadcrumbs[i].Data {
				if shouldDropBreadcrumbDataKey(key) {
					delete(event.Breadcrumbs[i].Data, key)
					continue
				}
				if s, ok := value.(string); ok {
					event.Breadcrumbs[i].Data[key] = SanitizeValue(key, s)
				}
			}
		}
	}
	for name, ctx := range event.Contexts {
		for k, v := range ctx {
			if s, ok := v.(string); ok {
				event.Contexts[name][k] = SanitizeValue(k, s)
			}
		}
	}
}

func shouldDropBreadcrumbDataKey(key string) bool {
	switch strings.ToLower(key) {
	case "request_body", "response_body", "headers", "authorization", "token", "secret":
		return true
	default:
		return false
	}
}

func isHeavyTransaction(event *sentry.Event) bool {
	name := strings.ToLower(event.Transaction)
	if name == "" && event.Request != nil {
		name = strings.ToLower(event.Request.URL)
	}
	return strings.Contains(name, "sse") ||
		strings.Contains(name, "stream") ||
		strings.Contains(name, "/logs") ||
		strings.Contains(name, "log stream")
}

func stableModulo(s string, modulo uint32) uint32 {
	if modulo == 0 {
		return 0
	}
	var h uint32 = 2166136261
	for i := range len(s) {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h % modulo
}
