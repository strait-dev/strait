package telemetry

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
)

const maxBreadcrumbMessageBytes = 512

type sentryBreadcrumbStartKey struct{}
type sentryBreadcrumbSQLKey struct{}

// AddSentryBreadcrumb records a breadcrumb on the request hub when present.
func AddSentryBreadcrumb(ctx context.Context, category, message string, data map[string]any) {
	bc := &sentry.Breadcrumb{
		Type:      "default",
		Category:  category,
		Message:   message,
		Timestamp: time.Now(),
		Data:      data,
	}
	bc = sanitizeSentryBreadcrumb(bc)
	if bc == nil {
		return
	}
	if hub := sentry.GetHubFromContext(ctx); hub != nil {
		hub.AddBreadcrumb(bc, nil)
		return
	}
}

type SentryPGXTracer struct{}

func (SentryPGXTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	ctx = context.WithValue(ctx, sentryBreadcrumbStartKey{}, time.Now())
	return context.WithValue(ctx, sentryBreadcrumbSQLKey{}, data.SQL)
}

func (SentryPGXTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	sql, _ := ctx.Value(sentryBreadcrumbSQLKey{}).(string)
	if sql == "" {
		return
	}
	bcData := map[string]any{
		"sql":     normalizeSQL(sql),
		"command": data.CommandTag.String(),
	}
	if started, ok := ctx.Value(sentryBreadcrumbStartKey{}).(time.Time); ok {
		bcData["duration_ms"] = time.Since(started).Milliseconds()
	}
	if data.Err != nil {
		bcData["error"] = data.Err.Error()
	}
	AddSentryBreadcrumb(ctx, "db.sql", "postgres query", bcData)
}

type RedisBreadcrumbHook struct{}

func (RedisBreadcrumbHook) DialHook(next redis.DialHook) redis.DialHook {
	return next
}

func (RedisBreadcrumbHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		started := time.Now()
		err := next(ctx, cmd)
		data := map[string]any{
			"cmd":         strings.ToLower(cmd.Name()),
			"duration_ms": time.Since(started).Milliseconds(),
		}
		if err != nil && !errors.Is(err, redis.Nil) {
			data["error"] = err.Error()
		}
		AddSentryBreadcrumb(ctx, "cache.redis", "redis command", data)
		return err
	}
}

func (RedisBreadcrumbHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		started := time.Now()
		err := next(ctx, cmds)
		data := map[string]any{
			"cmd_count":   len(cmds),
			"duration_ms": time.Since(started).Milliseconds(),
			"first_cmd":   firstRedisCommandName(cmds),
		}
		if err != nil && !errors.Is(err, redis.Nil) {
			data["error"] = err.Error()
		}
		AddSentryBreadcrumb(ctx, "cache.redis", "redis pipeline", data)
		return err
	}
}

func sanitizeBreadcrumbData(data map[string]any) map[string]any {
	if len(data) == 0 {
		return nil
	}
	out := make(map[string]any, len(data))
	for key, value := range data {
		if shouldDropBreadcrumbDataKey(key) {
			continue
		}
		out[key] = sanitizeSentryBreadcrumbValue(key, value, 0)
	}
	return out
}

func sanitizeSentryBreadcrumb(breadcrumb *sentry.Breadcrumb) *sentry.Breadcrumb {
	if breadcrumb == nil {
		return nil
	}
	breadcrumb.Message = truncateBreadcrumbValue(ScrubSecrets(breadcrumb.Message), maxBreadcrumbMessageBytes)
	breadcrumb.Data = sanitizeBreadcrumbData(breadcrumb.Data)
	if breadcrumb.Message == "" && breadcrumb.Category == "" && len(breadcrumb.Data) == 0 {
		return nil
	}
	return breadcrumb
}

func sanitizeSentryBreadcrumbValue(key string, value any, depth int) any {
	switch v := sanitizeSentryValue(key, value, depth).(type) {
	case string:
		return truncateBreadcrumbValue(v, maxBreadcrumbMessageBytes)
	case map[string]any:
		return truncateBreadcrumbMap(v)
	case []any:
		out := make([]any, 0, len(v))
		for _, item := range v {
			out = append(out, sanitizeSentryBreadcrumbValue(key, item, depth+1))
		}
		return out
	default:
		return v
	}
}

func truncateBreadcrumbMap(values map[string]any) map[string]any {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]any, len(values))
	for key, value := range values {
		out[key] = sanitizeSentryBreadcrumbValue(key, value, 0)
	}
	return out
}

func truncateBreadcrumbValue(value string, maxBytes int) string {
	if len(value) <= maxBytes {
		return value
	}
	return value[:maxBytes]
}

func normalizeSQL(sql string) string {
	return truncateBreadcrumbValue(ScrubSecrets(strings.Join(strings.Fields(sql), " ")), maxBreadcrumbMessageBytes)
}

func firstRedisCommandName(cmds []redis.Cmder) string {
	if len(cmds) == 0 || cmds[0] == nil {
		return ""
	}
	return strings.ToLower(cmds[0].Name())
}
