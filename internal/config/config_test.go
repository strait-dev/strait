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
	// Flags that default to false (opt-in / experimental)
	optInFalse := map[string]bool{
		"FFConcurrencyLimits":          cfg.FFConcurrencyLimits,
		"FFProjectQuotas":              cfg.FFProjectQuotas,
		"FFExecutionWindows":           cfg.FFExecutionWindows,
		"FFQueuePartitioning":          cfg.FFQueuePartitioning,
		"FFProgressStreaming":          cfg.FFProgressStreaming,
		"FFCheckpoints":                cfg.FFCheckpoints,
		"FFRunContinuation":            cfg.FFRunContinuation,
		"FFUsageTracking":              cfg.FFUsageTracking,
		"FFErrorClassification":        cfg.FFErrorClassification,
		"FFSecretInjection":            cfg.FFSecretInjection,
		"FFRunReplay":                  cfg.FFRunReplay,
		"FFDryRun":                     cfg.FFDryRun,
		"FFRunRetention":               cfg.FFRunRetention,
		"FFDebugBundle":                cfg.FFDebugBundle,
		"FFBatchJobOps":                cfg.FFBatchJobOps,
		"FFJobHealthScoring":           cfg.FFJobHealthScoring,
		"FFAdaptiveTimeout":            cfg.FFAdaptiveTimeout,
		"FFConcurrencyEnforcement":     cfg.FFConcurrencyEnforcement,
		"FFProjectFairQueue":           cfg.FFProjectFairQueue,
		"FFWebhookDeliveryWorker":      cfg.FFWebhookDeliveryWorker,
		"FFWebhookTimestampSignatures": cfg.FFWebhookTimestampSignatures,
		"FFFallbackEndpoint":           cfg.FFFallbackEndpoint,
		"FFRunEventsBuffered":          cfg.FFRunEventsBuffered,
		"FFRedisRequired":              cfg.FFRedisRequired,
	}
	for name, val := range optInFalse {
		if val {
			t.Fatalf("%s = true, want false", name)
		}
	}

	// Flags that default to true (production defaults)
	prodTrue := map[string]bool{
		"FFSmartRetry":            cfg.FFSmartRetry,
		"FFCircuitBreaker":        cfg.FFCircuitBreaker,
		"FFBulkheads":             cfg.FFBulkheads,
		"FFRunDLQ":                cfg.FFRunDLQ,
		"FFPayloadValidation":     cfg.FFPayloadValidation,
		"FFJobTags":               cfg.FFJobTags,
		"FFRunAnnotations":        cfg.FFRunAnnotations,
		"FFExecutionTracing":      cfg.FFExecutionTracing,
		"FFEnvironments":          cfg.FFEnvironments,
		"FFJobGroups":             cfg.FFJobGroups,
		"FFJobDependencies":       cfg.FFJobDependencies,
		"FFAdaptiveConcurrency":   cfg.FFAdaptiveConcurrency,
		"FFEventTriggers":         cfg.FFEventTriggers,
		"FFListenNotify":          cfg.FFListenNotify,
		"FFRateLimitEnforcement":  cfg.FFRateLimitEnforcement,
		"FFPriorityAging":         cfg.FFPriorityAging,
		"FFCostBudgets":           cfg.FFCostBudgets,
		"FFWebhookCircuitBreaker": cfg.FFWebhookCircuitBreaker,
		"FFWebhookSubscriptions":  cfg.FFWebhookSubscriptions,
		"FFQueryCacheWarming":     cfg.FFQueryCacheWarming,
		"FFAuditLog":              cfg.FFAuditLog,
	}
	for name, val := range prodTrue {
		if !val {
			t.Fatalf("%s = false, want true", name)
		}
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
		"FF_CONCURRENCY_LIMITS", "FF_PROJECT_QUOTAS", "FF_PROGRESS_STREAMING",
		"FF_PAYLOAD_VALIDATION", "FF_EXECUTION_TRACING", "FF_JOB_GROUPS", "FF_JOB_DEPENDENCIES",
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
	t.Setenv("FF_PAYLOAD_VALIDATION", "false")
	t.Setenv("FF_EXECUTION_TRACING", "false")
	t.Setenv("FF_JOB_GROUPS", "false")
	t.Setenv("FF_JOB_DEPENDENCIES", "false")
	t.Setenv("FF_ADAPTIVE_CONCURRENCY", "false")
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
	if cfg.FFPayloadValidation {
		t.Fatal("FFPayloadValidation = true, want false (overridden)")
	}
	if cfg.FFExecutionTracing {
		t.Fatal("FFExecutionTracing = true, want false (overridden)")
	}
	if cfg.FFJobGroups {
		t.Fatal("FFJobGroups = true, want false (overridden)")
	}
	if cfg.FFJobDependencies {
		t.Fatal("FFJobDependencies = true, want false (overridden)")
	}
	if cfg.FFAdaptiveConcurrency {
		t.Fatal("FFAdaptiveConcurrency = true, want false (overridden)")
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
