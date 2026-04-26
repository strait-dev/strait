package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"strings"

	"github.com/getsentry/sentry-go"
)

// Known identifier keys that should become Sentry tags (for filtering/searching).
var sentryTagKeys = map[string]bool{
	"run_id": true, "job_id": true, "project_id": true,
	"workflow_run_id": true, "delivery_id": true, "trigger_id": true,
	"step_run_id": true, "machine_id": true, "table": true,
	"batch_key": true, "error_class": true, "attempt": true,
	"status_code": true, "operation": true, "consumer": true,
	"approval_id": true, "key_id": true, "subscription_id": true,
	"source_id": true, "drain_id": true, "batch_id": true,
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

			// Promote known identifiers to tags, rest to extra context.
			if sentryTagKeys[key] {
				scope.SetTag(key, val)
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
