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
	if cfg.IndexMaintenanceInterval != 24*time.Hour {
		t.Fatalf("IndexMaintenanceInterval = %v, want %v", cfg.IndexMaintenanceInterval, 24*time.Hour)
	}
	if cfg.WebhookMaxPayloadBytes != int64(1<<20) {
		t.Fatalf("WebhookMaxPayloadBytes = %d, want %d", cfg.WebhookMaxPayloadBytes, int64(1<<20))
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
	if cfg.FFConcurrencyLimits {
		t.Fatal("FFConcurrencyLimits = true, want false")
	}
	if cfg.FFProjectQuotas {
		t.Fatal("FFProjectQuotas = true, want false")
	}
	if cfg.FFExecutionWindows {
		t.Fatal("FFExecutionWindows = true, want false")
	}
	if cfg.FFQueuePartitioning {
		t.Fatal("FFQueuePartitioning = true, want false")
	}
	if cfg.FFProgressStreaming {
		t.Fatal("FFProgressStreaming = true, want false")
	}
	if cfg.FFCheckpoints {
		t.Fatal("FFCheckpoints = true, want false")
	}
	if cfg.FFRunContinuation {
		t.Fatal("FFRunContinuation = true, want false")
	}
	if cfg.FFUsageTracking {
		t.Fatal("FFUsageTracking = true, want false")
	}
	if cfg.FFCostBudgets {
		t.Fatal("FFCostBudgets = true, want false")
	}
	if cfg.FFErrorClassification {
		t.Fatal("FFErrorClassification = true, want false")
	}
	if cfg.FFSmartRetry {
		t.Fatal("FFSmartRetry = true, want false")
	}
	if cfg.FFCircuitBreaker {
		t.Fatal("FFCircuitBreaker = true, want false")
	}
	if cfg.FFBulkheads {
		t.Fatal("FFBulkheads = true, want false")
	}
	if cfg.FFRunDLQ {
		t.Fatal("FFRunDLQ = true, want false")
	}
	if cfg.FFPayloadValidation {
		t.Fatal("FFPayloadValidation = true, want false")
	}
	if cfg.FFJobTags {
		t.Fatal("FFJobTags = true, want false")
	}
	if cfg.FFRunAnnotations {
		t.Fatal("FFRunAnnotations = true, want false")
	}
	if cfg.FFSecretInjection {
		t.Fatal("FFSecretInjection = true, want false")
	}
	if cfg.FFRunReplay {
		t.Fatal("FFRunReplay = true, want false")
	}
	if cfg.FFRunRetention {
		t.Fatal("FFRunRetention = true, want false")
	}
	if cfg.FFExecutionTracing {
		t.Fatal("FFExecutionTracing = true, want false")
	}
	if cfg.FFDebugBundle {
		t.Fatal("FFDebugBundle = true, want false")
	}
	if cfg.FFJobGroups {
		t.Fatal("FFJobGroups = true, want false")
	}
	if cfg.FFJobDependencies {
		t.Fatal("FFJobDependencies = true, want false")
	}
	if cfg.FFJobHealthScoring {
		t.Fatal("FFJobHealthScoring = true, want false")
	}
	if cfg.FFAdaptiveTimeout {
		t.Fatal("FFAdaptiveTimeout = true, want false")
	}
	if cfg.FFAdaptiveConcurrency {
		t.Fatal("FFAdaptiveConcurrency = true, want false")
	}
	if cfg.AdaptiveConcurrencyMin != 5 {
		t.Fatalf("AdaptiveConcurrencyMin = %d, want %d", cfg.AdaptiveConcurrencyMin, 5)
	}
	if cfg.AdaptiveConcurrencyMax != 100 {
		t.Fatalf("AdaptiveConcurrencyMax = %d, want %d", cfg.AdaptiveConcurrencyMax, 100)
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
	bindEnvKeys(
		t,
		"DATABASE_URL", "INTERNAL_SECRET", "JWT_SIGNING_KEY", "PORT", "WORKER_CONCURRENCY", "MODE", "LOG_LEVEL",
		"INDEX_MAINTENANCE_INTERVAL",
		"FF_CONCURRENCY_LIMITS", "FF_PROJECT_QUOTAS", "FF_PROGRESS_STREAMING", "FF_PAYLOAD_VALIDATION", "FF_EXECUTION_TRACING", "FF_JOB_GROUPS", "FF_JOB_DEPENDENCIES",
		"FF_ADAPTIVE_CONCURRENCY", "ADAPTIVE_CONCURRENCY_MIN", "ADAPTIVE_CONCURRENCY_MAX",
	)
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("INTERNAL_SECRET", "test-secret")
	t.Setenv("JWT_SIGNING_KEY", "01234567890123456789012345678901")
	t.Setenv("PORT", "9090")
	t.Setenv("WORKER_CONCURRENCY", "20")
	t.Setenv("MODE", "worker")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("INDEX_MAINTENANCE_INTERVAL", "12h")
	t.Setenv("FF_CONCURRENCY_LIMITS", "true")
	t.Setenv("FF_PROJECT_QUOTAS", "true")
	t.Setenv("FF_PROGRESS_STREAMING", "true")
	t.Setenv("FF_PAYLOAD_VALIDATION", "true")
	t.Setenv("FF_EXECUTION_TRACING", "true")
	t.Setenv("FF_JOB_GROUPS", "true")
	t.Setenv("FF_JOB_DEPENDENCIES", "true")
	t.Setenv("FF_ADAPTIVE_CONCURRENCY", "true")
	t.Setenv("ADAPTIVE_CONCURRENCY_MIN", "3")
	t.Setenv("ADAPTIVE_CONCURRENCY_MAX", "30")

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
	if cfg.IndexMaintenanceInterval != 12*time.Hour {
		t.Fatalf("IndexMaintenanceInterval = %v, want %v", cfg.IndexMaintenanceInterval, 12*time.Hour)
	}
	if !cfg.FFConcurrencyLimits {
		t.Fatal("FFConcurrencyLimits = false, want true")
	}
	if !cfg.FFProjectQuotas {
		t.Fatal("FFProjectQuotas = false, want true")
	}
	if !cfg.FFProgressStreaming {
		t.Fatal("FFProgressStreaming = false, want true")
	}
	if !cfg.FFPayloadValidation {
		t.Fatal("FFPayloadValidation = false, want true")
	}
	if !cfg.FFExecutionTracing {
		t.Fatal("FFExecutionTracing = false, want true")
	}
	if !cfg.FFJobGroups {
		t.Fatal("FFJobGroups = false, want true")
	}
	if !cfg.FFJobDependencies {
		t.Fatal("FFJobDependencies = false, want true")
	}
	if !cfg.FFAdaptiveConcurrency {
		t.Fatal("FFAdaptiveConcurrency = false, want true")
	}
	if cfg.AdaptiveConcurrencyMin != 3 {
		t.Fatalf("AdaptiveConcurrencyMin = %d, want %d", cfg.AdaptiveConcurrencyMin, 3)
	}
	if cfg.AdaptiveConcurrencyMax != 30 {
		t.Fatalf("AdaptiveConcurrencyMax = %d, want %d", cfg.AdaptiveConcurrencyMax, 30)
	}
}

func TestLoad_EncryptionKeyRotationConfig(t *testing.T) {
	viper.Reset()
	bindEnvKeys(t,
		"DATABASE_URL",
		"INTERNAL_SECRET",
		"JWT_SIGNING_KEY",
		"ENCRYPTION_KEY",
		"ENCRYPTION_KEY_OLD",
	)

	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("INTERNAL_SECRET", "test-secret")
	t.Setenv("JWT_SIGNING_KEY", "01234567890123456789012345678901")
	t.Setenv("ENCRYPTION_KEY", "primary-key")
	t.Setenv("ENCRYPTION_KEY_OLD", "old-key-1, old-key-2 , ,old-key-3")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.EncryptionKey != "primary-key" {
		t.Fatalf("EncryptionKey = %q, want %q", cfg.EncryptionKey, "primary-key")
	}
	if cfg.SecretEncryptionKey != "primary-key" {
		t.Fatalf("SecretEncryptionKey = %q, want %q", cfg.SecretEncryptionKey, "primary-key")
	}
	if len(cfg.EncryptionKeyOld) != 3 {
		t.Fatalf("len(EncryptionKeyOld) = %d, want 3", len(cfg.EncryptionKeyOld))
	}
	if cfg.EncryptionKeyOld[0] != "old-key-1" || cfg.EncryptionKeyOld[1] != "old-key-2" || cfg.EncryptionKeyOld[2] != "old-key-3" {
		t.Fatalf("EncryptionKeyOld = %#v, want [old-key-1 old-key-2 old-key-3]", cfg.EncryptionKeyOld)
	}
}
