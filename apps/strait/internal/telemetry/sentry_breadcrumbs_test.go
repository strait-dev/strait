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
	"github.com/stretchr/testify/require"
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
	require.Len(t, breadcrumbs,
		1,
	)

	bc := breadcrumbs[0]
	require.Equal(t, "worker.dispatch",

		bc.Category,
	)
	require.Len(t, bc.Message,
		maxBreadcrumbMessageBytes,
	)

	require.NotContains(t, bc.Data, "authorization")
	require.Equal(t, "failed with [REDACTED]",

		bc.Data["message"])
	require.Equal(t, "[REDACTED]",

		bc.Data["dsn"])
	require.EqualValues(t, 3,
		bc.Data["count"])

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
	require.NotNil(t, breadcrumb)

	require.False(t, strings.Contains(breadcrumb.
		Message,

		"user:pass"))

	require.NotContains(t, breadcrumb.Data, "authorization")
	require.Equal(t, "[REDACTED]",

		breadcrumb.
			Data["url"])
	require.EqualValues(t, 503,
		breadcrumb.
			Data["status_code"],
	)

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
	require.Len(t, breadcrumbs,
		1,
	)

	bc := breadcrumbs[0]
	require.Equal(t, "db.sql",
		bc.
			Category)

	gotSQL := bc.Data["sql"].(string)
	require.Contains(t, gotSQL, "SELECT * FROM jobs")
	require.NotContains(t, gotSQL, "secret-arg")
	require.NotContains(t, gotSQL, "user:pass")
	require.Equal(t, "SELECT 1",

		bc.Data["command"])

}

func TestRedisBreadcrumbHookAddsCommandAndPipelineBreadcrumbs(t *testing.T) {
	t.Parallel()

	ctx, hub := contextWithSentryHub()
	hook := RedisBreadcrumbHook{}
	process := hook.ProcessHook(func(context.Context, redis.Cmder) error {
		return nil
	})
	cmd := redis.NewStringCmd(ctx, "GET", "cache:key")
	require.NoError(t,
		process(ctx,
			cmd))

	pipeline := hook.ProcessPipelineHook(func(context.Context, []redis.Cmder) error {
		return errors.New("redis://user:pass@localhost:6379 failed")
	})
	require.Error(t, pipeline(ctx,
		[]redis.Cmder{redis.
			NewStringCmd(ctx, "SET", "cache:key",
				"value")}))

	breadcrumbs := sentryBreadcrumbsFromHub(t, hub)
	require.Len(t, breadcrumbs,
		2,
	)
	require.Equal(t, "get",
		breadcrumbs[0].Data["cmd"])
	require.Equal(t, "set",
		breadcrumbs[1].Data["first_cmd"])

	require.NotContains(t, breadcrumbs[1].Data["error"].(string), "user:pass")
}

func contextWithSentryHub() (context.Context, *sentry.Hub) {
	hub := sentry.NewHub(nil, sentry.NewScope())
	return sentry.SetHubOnContext(context.Background(), hub), hub
}

func sentryBreadcrumbsFromHub(t *testing.T, hub *sentry.Hub) []*sentry.Breadcrumb {
	t.Helper()
	event := hub.Scope().ApplyToEvent(&sentry.Event{}, nil, nil)
	require.NotNil(t, event)

	return event.Breadcrumbs
}
