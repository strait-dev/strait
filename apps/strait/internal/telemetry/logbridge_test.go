package telemetry

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitLogBridge_NoEndpoint(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger, shutdown, err := InitLogBridge(ctx, "test-service", "", "test")
	require.NoError(t,
		err)
	assert.Nil(t, logger)
	assert.NoError(t,
		shutdown(ctx))

}

func TestInitLogBridge_WithEndpoint(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger, shutdown, err := InitLogBridge(ctx, "test-service", "http://localhost:4318", "dev")
	require.NoError(t,
		err)
	require.NotNil(t,
		logger)

	// Log something to verify no panic.
	logger.Info("test log line", "key", "value")
	// Shutdown may return an error because no real endpoint is listening;
	// we only verify that it does not panic.
	_ = shutdown(ctx)
}

func TestInitLogBridge_RedactsCredentialedEndpointInStartupLog(t *testing.T) {
	ctx := context.Background()
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() {
		slog.SetDefault(previous)
	})

	logger, shutdown, err := InitLogBridge(ctx, "test-service", "http://user:pass@localhost:4318/v1/logs?token=secret&tenant=prod", "dev")
	require.NoError(t,
		err)
	require.NotNil(t,
		logger)

	_ = shutdown(ctx)

	out := buf.String()
	require.True(t, strings.Contains(out, "otel log bridge enabled"))

	for _, leaked := range []string{"user", "pass", "secret"} {
		require.False(t,
			strings.Contains(out, leaked))

	}
	require.True(t, strings.Contains(out, "token=%5Bredacted%5D"))
	require.True(t, strings.Contains(out, "tenant=prod"))

}

func TestInitLogBridge_EmptyEnvironment(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger, shutdown, err := InitLogBridge(ctx, "test-service", "http://localhost:4318", "")
	require.NoError(t,
		err)
	require.NotNil(t,
		logger)

	_ = shutdown(ctx)
}

func TestTeeHandler_FansOut(t *testing.T) {
	t.Parallel()

	var buf1, buf2 bytes.Buffer
	h1 := slog.NewTextHandler(&buf1, &slog.HandlerOptions{Level: slog.LevelInfo})
	h2 := slog.NewTextHandler(&buf2, &slog.HandlerOptions{Level: slog.LevelInfo})

	tee := NewTeeHandler(h1, h2)
	logger := slog.New(tee)

	logger.Info("hello", "key", "value")
	assert.True(t, strings.Contains(buf1.String(), "hello"))
	assert.True(t, strings.Contains(buf2.String(), "hello"))

}

func TestTeeHandler_Enabled(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	h := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})
	tee := NewTeeHandler(h)
	assert.False(t, tee.
		Enabled(
			context.Background(), slog.
				LevelDebug,
		))
	assert.True(t, tee.
		Enabled(context.
			Background(), slog.
			LevelWarn,
		))

}

func TestTeeHandler_WithAttrs(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	h := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	tee := NewTeeHandler(h)

	withAttrs := tee.WithAttrs([]slog.Attr{slog.String("run_id", "run-123")})
	logger := slog.New(withAttrs)
	logger.Info("test")
	assert.True(t, strings.Contains(buf.String(), "run_id"))

}

func TestTeeHandler_WithGroup(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	h := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	tee := NewTeeHandler(h)

	withGroup := tee.WithGroup("request")
	logger := slog.New(withGroup)
	logger.Info("test", "method", "GET")
	assert.True(t, strings.Contains(buf.String(), "request.method"))

}

func TestTeeHandler_LevelFiltering(t *testing.T) {
	t.Parallel()

	var warnBuf, infoBuf bytes.Buffer
	warnHandler := slog.NewTextHandler(&warnBuf, &slog.HandlerOptions{Level: slog.LevelWarn})
	infoHandler := slog.NewTextHandler(&infoBuf, &slog.HandlerOptions{Level: slog.LevelInfo})

	tee := NewTeeHandler(warnHandler, infoHandler)
	logger := slog.New(tee)

	logger.Info("info message")
	assert.False(t, strings.Contains(warnBuf.
		String(),
		"info message",
	))
	assert.True(t, strings.Contains(infoBuf.
		String(), "info message",
	))

}

func TestNewTeeHandler_SingleHandler(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	h := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})

	tee := NewTeeHandler(h)
	logger := slog.New(tee)

	logger.Info("single handler test", "k", "v")
	assert.True(t, strings.Contains(buf.String(), "single handler test"))
	assert.True(t, strings.Contains(buf.String(), "k=v"))

}

func TestNewTeeHandler_ConcurrentLogging(t *testing.T) {
	t.Parallel()

	var buf1, buf2 bytes.Buffer
	h1 := slog.NewTextHandler(&buf1, &slog.HandlerOptions{Level: slog.LevelInfo})
	h2 := slog.NewTextHandler(&buf2, &slog.HandlerOptions{Level: slog.LevelInfo})

	tee := NewTeeHandler(h1, h2)
	logger := slog.New(tee)

	var wg conc.WaitGroup
	for i := range 100 {
		wg.Go(func() {
			logger.Info("concurrent log", "goroutine", i)
		})
	}
	wg.Wait()

	out1 := buf1.String()
	out2 := buf2.String()
	count1 := strings.Count(out1, "concurrent log")
	count2 := strings.Count(out2, "concurrent log")
	assert.EqualValues(t, 100,
		count1)
	assert.EqualValues(t, 100,
		count2)

}
