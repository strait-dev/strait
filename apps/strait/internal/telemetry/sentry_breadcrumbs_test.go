package telemetry

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/getsentry/sentry-go"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/redis/go-redis/v9"
)

func TestAddSentryBreadcrumbSanitizesData(t *testing.T) {
	t.Parallel()

	ctx, hub := contextWithSentryHub()
	AddSentryBreadcrumb(ctx, "worker.dispatch", strings.Repeat("a", maxBreadcrumbMessageBytes+20), map[string]any{
		"authorization": "Bearer should-be-dropped",
		"message":       "failed with Bearer secret-token",
		"dsn":           "postgres://user:pass@localhost/strait",
		"count":         3,
	})

	breadcrumbs := sentryBreadcrumbsFromHub(t, hub)
	if len(breadcrumbs) != 1 {
		t.Fatalf("breadcrumbs = %d, want 1", len(breadcrumbs))
	}
	bc := breadcrumbs[0]
	if bc.Category != "worker.dispatch" {
		t.Fatalf("category = %q, want worker.dispatch", bc.Category)
	}
	if len(bc.Message) != maxBreadcrumbMessageBytes {
		t.Fatalf("message length = %d, want %d", len(bc.Message), maxBreadcrumbMessageBytes)
	}
	if _, ok := bc.Data["authorization"]; ok {
		t.Fatal("authorization data was not dropped")
	}
	if got := bc.Data["message"]; got != "failed with [REDACTED]" {
		t.Fatalf("message data = %v, want redacted bearer token", got)
	}
	if got := bc.Data["dsn"]; got != "[REDACTED]" {
		t.Fatalf("dsn data = %v, want redacted", got)
	}
	if got := bc.Data["count"]; got != 3 {
		t.Fatalf("count data = %v, want 3", got)
	}
}

func TestBeforeBreadcrumbSanitizesSDKBreadcrumbs(t *testing.T) {
	t.Parallel()

	breadcrumb := BeforeBreadcrumb(&sentry.Breadcrumb{
		Category: "http.client",
		Message:  "POST https://user:pass@example.com/private",
		Data: map[string]any{
			"authorization": "Bearer secret-token",
			"url":           "https://user:pass@example.com/private?token=secret",
			"status_code":   503,
		},
	}, nil)
	if breadcrumb == nil {
		t.Fatal("expected breadcrumb")
		return
	}
	if strings.Contains(breadcrumb.Message, "user:pass") {
		t.Fatalf("message leaked credentials: %q", breadcrumb.Message)
	}
	if _, ok := breadcrumb.Data["authorization"]; ok {
		t.Fatal("authorization data was not dropped")
	}
	if got := breadcrumb.Data["url"]; got != "[REDACTED]" {
		t.Fatalf("url data = %v, want redacted", got)
	}
	if got := breadcrumb.Data["status_code"]; got != 503 {
		t.Fatalf("status_code = %v, want 503", got)
	}
}

func TestSentryPGXTracerAddsSQLBreadcrumbWithoutArgs(t *testing.T) {
	t.Parallel()

	ctx, hub := contextWithSentryHub()
	tracer := SentryPGXTracer{}
	ctx = tracer.TraceQueryStart(ctx, nil, pgx.TraceQueryStartData{
		SQL:  "SELECT *   FROM jobs WHERE token = $1 AND dsn = 'postgres://user:pass@host/db'",
		Args: []any{"secret-arg"},
	})
	tracer.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{
		CommandTag: pgconn.NewCommandTag("SELECT 1"),
	})

	breadcrumbs := sentryBreadcrumbsFromHub(t, hub)
	if len(breadcrumbs) != 1 {
		t.Fatalf("breadcrumbs = %d, want 1", len(breadcrumbs))
	}
	bc := breadcrumbs[0]
	if bc.Category != "db.sql" {
		t.Fatalf("category = %q, want db.sql", bc.Category)
	}
	if got := bc.Data["sql"].(string); !strings.Contains(got, "SELECT * FROM jobs") {
		t.Fatalf("sql = %q, want normalized query", got)
	}
	if got := bc.Data["sql"].(string); strings.Contains(got, "secret-arg") || strings.Contains(got, "user:pass") {
		t.Fatalf("sql leaked secret data: %q", got)
	}
	if got := bc.Data["command"]; got != "SELECT 1" {
		t.Fatalf("command = %v, want SELECT 1", got)
	}
}

func TestRedisBreadcrumbHookAddsCommandAndPipelineBreadcrumbs(t *testing.T) {
	t.Parallel()

	ctx, hub := contextWithSentryHub()
	hook := RedisBreadcrumbHook{}
	process := hook.ProcessHook(func(context.Context, redis.Cmder) error {
		return nil
	})
	cmd := redis.NewStringCmd(ctx, "GET", "cache:key")
	if err := process(ctx, cmd); err != nil {
		t.Fatalf("process hook returned error: %v", err)
	}

	pipeline := hook.ProcessPipelineHook(func(context.Context, []redis.Cmder) error {
		return errors.New("redis://user:pass@localhost:6379 failed")
	})
	if err := pipeline(ctx, []redis.Cmder{redis.NewStringCmd(ctx, "SET", "cache:key", "value")}); err == nil {
		t.Fatal("pipeline hook returned nil error")
	}

	breadcrumbs := sentryBreadcrumbsFromHub(t, hub)
	if len(breadcrumbs) != 2 {
		t.Fatalf("breadcrumbs = %d, want 2", len(breadcrumbs))
	}
	if got := breadcrumbs[0].Data["cmd"]; got != "get" {
		t.Fatalf("command breadcrumb cmd = %v, want get", got)
	}
	if got := breadcrumbs[1].Data["first_cmd"]; got != "set" {
		t.Fatalf("pipeline breadcrumb first_cmd = %v, want set", got)
	}
	if got := breadcrumbs[1].Data["error"].(string); strings.Contains(got, "user:pass") {
		t.Fatalf("pipeline breadcrumb leaked redis credentials: %q", got)
	}
}

func contextWithSentryHub() (context.Context, *sentry.Hub) {
	hub := sentry.NewHub(nil, sentry.NewScope())
	return sentry.SetHubOnContext(context.Background(), hub), hub
}

func sentryBreadcrumbsFromHub(t *testing.T, hub *sentry.Hub) []*sentry.Breadcrumb {
	t.Helper()
	event := hub.Scope().ApplyToEvent(&sentry.Event{}, nil, nil)
	if event == nil {
		t.Fatal("expected event")
		return nil
	}
	return event.Breadcrumbs
}
