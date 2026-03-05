package config

import (
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
)

func bindEnvKeys(t *testing.T, keys ...string) {
	t.Helper()
	for _, key := range keys {
		if err := viper.BindEnv(key); err != nil {
			t.Fatalf("BindEnv(%q) failed: %v", key, err)
		}
	}
}

func TestLoad_Defaults(t *testing.T) {
	viper.Reset()
	bindEnvKeys(t, "DATABASE_URL", "INTERNAL_SECRET", "JWT_SIGNING_KEY")
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("INTERNAL_SECRET", "test-secret")
	t.Setenv("JWT_SIGNING_KEY", "01234567890123456789012345678901")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Mode != "all" {
		t.Fatalf("Mode = %q, want %q", cfg.Mode, "all")
	}
	if cfg.Port != 8080 {
		t.Fatalf("Port = %d, want %d", cfg.Port, 8080)
	}
	if cfg.WorkerConcurrency != 10 {
		t.Fatalf("WorkerConcurrency = %d, want %d", cfg.WorkerConcurrency, 10)
	}
	if cfg.LogLevel != "info" {
		t.Fatalf("LogLevel = %q, want %q", cfg.LogLevel, "info")
	}
	if cfg.HeartbeatInterval != 10*time.Second {
		t.Fatalf("HeartbeatInterval = %v, want %v", cfg.HeartbeatInterval, 10*time.Second)
	}
	if cfg.ReaperInterval != 30*time.Second {
		t.Fatalf("ReaperInterval = %v, want %v", cfg.ReaperInterval, 30*time.Second)
	}
	if cfg.StaleThreshold != 60*time.Second {
		t.Fatalf("StaleThreshold = %v, want %v", cfg.StaleThreshold, 60*time.Second)
	}
	if cfg.PollerInterval != 5*time.Second {
		t.Fatalf("PollerInterval = %v, want %v", cfg.PollerInterval, 5*time.Second)
	}
	if cfg.DBMaxConns != 25 {
		t.Fatalf("DBMaxConns = %d, want %d", cfg.DBMaxConns, 25)
	}
	if cfg.DBMinConns != 5 {
		t.Fatalf("DBMinConns = %d, want %d", cfg.DBMinConns, 5)
	}
	if cfg.DBMaxConnLifetime != 30*time.Minute {
		t.Fatalf("DBMaxConnLifetime = %v, want %v", cfg.DBMaxConnLifetime, 30*time.Minute)
	}
	if cfg.DBMaxConnIdleTime != 5*time.Minute {
		t.Fatalf("DBMaxConnIdleTime = %v, want %v", cfg.DBMaxConnIdleTime, 5*time.Minute)
	}
	if cfg.RateLimitRequests != 100 {
		t.Fatalf("RateLimitRequests = %d, want %d", cfg.RateLimitRequests, 100)
	}
	if cfg.RateLimitWindow != time.Minute {
		t.Fatalf("RateLimitWindow = %v, want %v", cfg.RateLimitWindow, time.Minute)
	}
	if cfg.TriggerRateLimitRequests != 10 {
		t.Fatalf("TriggerRateLimitRequests = %d, want %d", cfg.TriggerRateLimitRequests, 10)
	}
	if cfg.TriggerRateLimitWindow != time.Minute {
		t.Fatalf("TriggerRateLimitWindow = %v, want %v", cfg.TriggerRateLimitWindow, time.Minute)
	}
}

func TestLoad_RequiredFields(t *testing.T) {
	tests := []struct {
		name     string
		setEnv   func(t *testing.T)
		errorSub string
	}{
		{
			name: "missing database url",
			setEnv: func(t *testing.T) {
				t.Setenv("INTERNAL_SECRET", "test-secret")
				t.Setenv("JWT_SIGNING_KEY", "01234567890123456789012345678901")
			},
			errorSub: "DATABASE_URL",
		},
		{
			name: "missing internal secret",
			setEnv: func(t *testing.T) {
				t.Setenv("DATABASE_URL", "postgres://localhost/test")
				t.Setenv("JWT_SIGNING_KEY", "01234567890123456789012345678901")
			},
			errorSub: "INTERNAL_SECRET",
		},
		{
			name: "jwt signing key too short",
			setEnv: func(t *testing.T) {
				t.Setenv("DATABASE_URL", "postgres://localhost/test")
				t.Setenv("INTERNAL_SECRET", "test-secret")
				t.Setenv("JWT_SIGNING_KEY", "too-short")
			},
			errorSub: "JWT_SIGNING_KEY",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viper.Reset()
			bindEnvKeys(t, "DATABASE_URL", "INTERNAL_SECRET", "JWT_SIGNING_KEY")
			tt.setEnv(t)

			_, err := Load()
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.errorSub)
			}
			if !strings.Contains(err.Error(), tt.errorSub) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tt.errorSub)
			}
		})
	}
}

func TestLoad_OverrideDefaults(t *testing.T) {
	viper.Reset()
	bindEnvKeys(t, "DATABASE_URL", "INTERNAL_SECRET", "JWT_SIGNING_KEY", "PORT", "WORKER_CONCURRENCY", "MODE", "LOG_LEVEL")
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("INTERNAL_SECRET", "test-secret")
	t.Setenv("JWT_SIGNING_KEY", "01234567890123456789012345678901")
	t.Setenv("PORT", "9090")
	t.Setenv("WORKER_CONCURRENCY", "20")
	t.Setenv("MODE", "worker")
	t.Setenv("LOG_LEVEL", "debug")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Port != 9090 {
		t.Fatalf("Port = %d, want %d", cfg.Port, 9090)
	}
	if cfg.WorkerConcurrency != 20 {
		t.Fatalf("WorkerConcurrency = %d, want %d", cfg.WorkerConcurrency, 20)
	}
	if cfg.Mode != "worker" {
		t.Fatalf("Mode = %q, want %q", cfg.Mode, "worker")
	}
	if cfg.LogLevel != "debug" {
		t.Fatalf("LogLevel = %q, want %q", cfg.LogLevel, "debug")
	}
}
