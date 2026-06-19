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
	maxSentrySanitizeDepth       = 8
	maxSentrySanitizeItems       = 50
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
	DSN                     string
	Environment             string
	Release                 string
	TracesSampleRate        float64
	Debug                   bool
	MaxBreadcrumbs          int
	MaxSpans                int
	MaxErrorDepth           int
	StrictTraceContinuation bool
}

const sentryFlushTimeout time.Duration = 2_000_000_000

// InitSentry initializes the process-wide Sentry SDK and returns a shutdown
// function. An empty DSN leaves Sentry disabled and returns a no-op shutdown.
func InitSentry(cfg SentryConfig) (func(), error) {
	if cfg.DSN == "" {
		return func() {}, nil
	}
	tracesSampleRate := normalizeSentrySampleRate(cfg.TracesSampleRate)
	if err := sentry.Init(SentryClientOptions(cfg, tracesSampleRate)); err != nil {
		return nil, fmt.Errorf("init sentry: %w", err)
	}
	return func() { sentry.Flush(sentryFlushTimeout) }, nil
}

// EnsureSentryHub returns ctx with an isolated Sentry hub attached.
func EnsureSentryHub(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if sentry.GetHubFromContext(ctx) != nil {
		return ctx
	}
	return sentry.SetHubOnContext(ctx, sentry.CurrentHub().Clone())
}

// SentryClientOptions returns the SDK options used by InitSentry. Tests use it
// with a fake transport so classifier behavior is exercised through the SDK.
func SentryClientOptions(cfg SentryConfig, tracesSampleRate float64) sentry.ClientOptions {
	tracesSampleRate = normalizeSentrySampleRate(tracesSampleRate)
	opts := sentry.ClientOptions{
		Dsn:                     cfg.DSN,
		Environment:             cfg.Environment,
		Release:                 cfg.Release,
		Debug:                   cfg.Debug,
		AttachStacktrace:        true,
		SampleRate:              1.0,
		EnableTracing:           tracesSampleRate > 0,
		BeforeSend:              BeforeSend,
		BeforeSendTransaction:   BeforeSendTransaction,
		BeforeBreadcrumb:        BeforeBreadcrumb,
		MaxBreadcrumbs:          cfg.MaxBreadcrumbs,
		MaxSpans:                cfg.MaxSpans,
		MaxErrorDepth:           cfg.MaxErrorDepth,
		PropagateTraceparent:    true,
		StrictTraceContinuation: cfg.StrictTraceContinuation,
	}
	if tracesSampleRate > 0 {
		opts.TracesSampler = SentryTracesSampler(tracesSampleRate)
	}
	return opts
}

// SentryTracesSampler drops known high-volume transactions before they are
// sent and otherwise applies the configured global sample rate. It deliberately
// ignores the upstream parent sampling decision: HTTP/gRPC trace headers are
// client-controlled at Strait's edge, so letting ParentSampled force 0% or
// 100% would allow callers to override our local telemetry budget.
func SentryTracesSampler(sampleRate float64) sentry.TracesSampler {
	sampleRate = normalizeSentrySampleRate(sampleRate)
	return func(ctx sentry.SamplingContext) float64 {
		if ctx.Span != nil && isHeavyTransactionName(ctx.Span.Name) &&
			stableModulo(ctx.Span.Name, sentryHeavyTransactionModulo) != 0 {
			return 0
		}
		return sampleRate
	}
}

func normalizeSentrySampleRate(rate float64) float64 {
	if rate < 0 {
		return 0
	}
	if rate > 1 {
		return 1
	}
	return rate
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

// BeforeSendTransaction is the final transaction safety filter. TracesSampler
// handles primary sampling; this hook preserves deterministic heavy-route
// filtering and redaction for transactions that still reach send time.
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

// BeforeBreadcrumb applies the same redaction policy to every breadcrumb,
// including breadcrumbs emitted by SDK integrations.
func BeforeBreadcrumb(breadcrumb *sentry.Breadcrumb, _ *sentry.BreadcrumbHint) *sentry.Breadcrumb {
	return sanitizeSentryBreadcrumb(breadcrumb)
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

	if hub := sentry.GetHubFromContext(ctx); hub != nil {
		hub.WithScope(func(scope *sentry.Scope) {
			captureErr := configureSentryLogScope(scope, r)
			if captureErr != nil {
				hub.CaptureException(captureErr)
			} else {
				hub.CaptureMessage(r.Message)
			}
		})
		return err
	}

	sentry.WithScope(func(scope *sentry.Scope) {
		captureErr := configureSentryLogScope(scope, r)
		if captureErr != nil {
			sentry.CaptureException(captureErr)
		} else {
			sentry.CaptureMessage(r.Message)
		}
	})

	return err
}

func configureSentryLogScope(scope *sentry.Scope, r slog.Record) error {
	scope.SetLevel(sentry.LevelError)

	var captureErr error
	extra := sentry.Context{}
	r.Attrs(func(a slog.Attr) bool {
		key := a.Key
		val := fmt.Sprintf("%v", a.Value.Any())

		if key == "error" {
			if e, ok := a.Value.Any().(error); ok {
				captureErr = e
			}
		}

		val = SanitizeValue(key, val)

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
	return captureErr
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

// SanitizeQueryString redacts credential-bearing query parameters.
func SanitizeQueryString(qs string) string {
	params, err := url.ParseQuery(qs)
	if err != nil {
		return ""
	}
	for k := range params {
		if isCredentialQueryKey(k) {
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
		event.Breadcrumbs[i] = sanitizeSentryBreadcrumb(event.Breadcrumbs[i])
	}
	for name, ctx := range event.Contexts {
		event.Contexts[name] = sanitizeSentryContext(ctx, 0)
	}
}

func sanitizeSentryContext(ctx sentry.Context, depth int) sentry.Context {
	if len(ctx) == 0 {
		return nil
	}
	out := make(sentry.Context, len(ctx))
	for key, value := range ctx {
		if shouldDropBreadcrumbDataKey(key) {
			continue
		}
		out[key] = sanitizeSentryValue(key, value, depth+1)
	}
	return out
}

func sanitizeSentryMap(values map[string]any, depth int) map[string]any {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]any, len(values))
	for key, value := range values {
		if shouldDropBreadcrumbDataKey(key) {
			continue
		}
		out[key] = sanitizeSentryValue(key, value, depth+1)
	}
	return out
}

func sanitizeSentrySlice(key string, values []any, depth int) []any {
	if len(values) == 0 {
		return nil
	}
	limit := min(len(values), maxSentrySanitizeItems)
	out := make([]any, 0, limit)
	for i := range limit {
		out = append(out, sanitizeSentryValue(key, values[i], depth+1))
	}
	return out
}

func sanitizeSentryValue(key string, value any, depth int) any {
	if depth > maxSentrySanitizeDepth {
		return "[TRUNCATED]"
	}
	switch v := value.(type) {
	case string:
		return SanitizeValue(key, v)
	case map[string]any:
		return sanitizeSentryMap(v, depth+1)
	case []any:
		return sanitizeSentrySlice(key, v, depth+1)
	case []string:
		values := make([]any, 0, len(v))
		for _, item := range v {
			values = append(values, item)
		}
		return sanitizeSentrySlice(key, values, depth+1)
	default:
		return value
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
	name := event.Transaction
	if name == "" && event.Request != nil {
		name = event.Request.URL
	}
	return isHeavyTransactionName(name)
}

func isHeavyTransactionName(name string) bool {
	name = strings.ToLower(name)
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
