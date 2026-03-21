package telemetry

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestInitLogBridge_NoEndpoint(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger, shutdown, err := InitLogBridge(ctx, "test-service", "", "test")
	if err != nil {
		t.Fatalf("InitLogBridge() error = %v", err)
	}
	if logger != nil {
		t.Error("expected nil logger when endpoint is empty")
	}
	if err := shutdown(ctx); err != nil {
		t.Errorf("shutdown() error = %v", err)
	}
}

func TestInitLogBridge_WithEndpoint(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger, shutdown, err := InitLogBridge(ctx, "test-service", "http://localhost:4318", "dev")
	if err != nil {
		t.Fatalf("InitLogBridge() error = %v", err)
	}
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
	// Log something to verify no panic.
	logger.Info("test log line", "key", "value")
	// Shutdown may return an error because no real endpoint is listening;
	// we only verify that it does not panic.
	_ = shutdown(ctx)
}

func TestInitLogBridge_EmptyEnvironment(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger, shutdown, err := InitLogBridge(ctx, "test-service", "http://localhost:4318", "")
	if err != nil {
		t.Fatalf("InitLogBridge() error = %v", err)
	}
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
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

	if !strings.Contains(buf1.String(), "hello") {
		t.Errorf("handler 1 did not receive log: %s", buf1.String())
	}
	if !strings.Contains(buf2.String(), "hello") {
		t.Errorf("handler 2 did not receive log: %s", buf2.String())
	}
}

func TestTeeHandler_Enabled(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	h := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})
	tee := NewTeeHandler(h)

	if tee.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("should not be enabled for debug level")
	}
	if !tee.Enabled(context.Background(), slog.LevelWarn) {
		t.Error("should be enabled for warn level")
	}
}

func TestTeeHandler_WithAttrs(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	h := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	tee := NewTeeHandler(h)

	withAttrs := tee.WithAttrs([]slog.Attr{slog.String("run_id", "run-123")})
	logger := slog.New(withAttrs)
	logger.Info("test")

	if !strings.Contains(buf.String(), "run_id") {
		t.Errorf("expected run_id in output: %s", buf.String())
	}
}

func TestTeeHandler_WithGroup(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	h := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	tee := NewTeeHandler(h)

	withGroup := tee.WithGroup("request")
	logger := slog.New(withGroup)
	logger.Info("test", "method", "GET")

	if !strings.Contains(buf.String(), "request.method") {
		t.Errorf("expected grouped key in output: %s", buf.String())
	}
}

func TestTeeHandler_LevelFiltering(t *testing.T) {
	t.Parallel()

	var warnBuf, infoBuf bytes.Buffer
	warnHandler := slog.NewTextHandler(&warnBuf, &slog.HandlerOptions{Level: slog.LevelWarn})
	infoHandler := slog.NewTextHandler(&infoBuf, &slog.HandlerOptions{Level: slog.LevelInfo})

	tee := NewTeeHandler(warnHandler, infoHandler)
	logger := slog.New(tee)

	logger.Info("info message")

	if strings.Contains(warnBuf.String(), "info message") {
		t.Error("warn handler should not receive info-level message")
	}
	if !strings.Contains(infoBuf.String(), "info message") {
		t.Error("info handler should receive info-level message")
	}
}
