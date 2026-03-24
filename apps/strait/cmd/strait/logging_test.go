package main

import (
	"context"
	"log/slog"
	"testing"
)

func TestSetupLoggingJSONDefault(t *testing.T) {
	setupLogging("info", "json")

	handler := slog.Default().Handler()
	if handler == nil {
		t.Fatal("expected non-nil handler after setupLogging with json format")
	}

	// JSON handler should be enabled at info level.
	if !handler.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("expected info level to be enabled")
	}
	if handler.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("expected debug level to be disabled for info level logger")
	}
}

func TestSetupLoggingTextFormat(t *testing.T) {
	setupLogging("debug", "text")

	handler := slog.Default().Handler()
	if handler == nil {
		t.Fatal("expected non-nil handler after setupLogging with text format")
	}

	// Debug level should be enabled when level is "debug".
	if !handler.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("expected debug level to be enabled")
	}
}

func TestSetupLoggingLevels(t *testing.T) {
	tests := []struct {
		level         string
		enabledAt     slog.Level
		disabledBelow slog.Level
	}{
		{"debug", slog.LevelDebug, slog.Level(-8)},
		{"info", slog.LevelInfo, slog.LevelDebug},
		{"warn", slog.LevelWarn, slog.LevelInfo},
		{"error", slog.LevelError, slog.LevelWarn},
		{"unknown", slog.LevelInfo, slog.LevelDebug}, // defaults to info
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			setupLogging(tt.level, "json")
			handler := slog.Default().Handler()

			if !handler.Enabled(context.Background(), tt.enabledAt) {
				t.Errorf("expected level %v to be enabled for %q", tt.enabledAt, tt.level)
			}
			if handler.Enabled(context.Background(), tt.disabledBelow) {
				t.Errorf("expected level %v to be disabled for %q", tt.disabledBelow, tt.level)
			}
		})
	}
}
