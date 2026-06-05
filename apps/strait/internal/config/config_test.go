package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"
)

// skipIfCommunity skips the test when running in a community build.
// Tests that set STRAIT_EDITION=cloud require the cloud build tag.
func skipIfCommunity(t *testing.T) {
	t.Helper()
	if domain.BuildEdition() != domain.EditionCloud {
		t.Skip("requires cloud build tag")
	}
}

// setRequiredEnv sets the minimum required env vars for a valid config.
func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("REDIS_URL", "redis://localhost:6379")
	t.Setenv("SEQUIN_BASE_URL", "http://localhost:7376")
	t.Setenv("SEQUIN_CONSUMER_NAME", "strait-cdc")
	t.Setenv("SEQUIN_API_TOKEN", "sequin-api-token")
	t.Setenv("INTERNAL_SECRET", "test-secret-value")
	t.Setenv("JWT_SIGNING_KEY", "aaaa-test-jwt-signing-key-00000000")
}

func TestLoad_Defaults(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tests := []struct {
		name string
		got  any
		want any
	}{
		{"Mode", cfg.Mode, "all"},
		{"Port", cfg.Port, 8080},
		{"WorkerConcurrency", cfg.WorkerConcurrency, 25},
		{"LogLevel", cfg.LogLevel, "info"},
		{"HeartbeatInterval", cfg.HeartbeatInterval, 10 * time.Second},
		{"ReaperInterval", cfg.ReaperInterval, 30 * time.Second},
		{"StaleThreshold", cfg.StaleThreshold, 60 * time.Second},
		{"PollerInterval", cfg.PollerInterval, 5 * time.Second},
		{"RunRetentionShort", cfg.RunRetentionShort, 720 * time.Hour},
		{"RunRetentionLong", cfg.RunRetentionLong, 2160 * time.Hour},
		{"WorkflowRunRetentionDays", cfg.WorkflowRunRetentionDays, 30},
		{"DBMaxConns", cfg.DBMaxConns, int32(50)},
		{"DBMinConns", cfg.DBMinConns, int32(10)},
		{"DBMaxConnLifetime", cfg.DBMaxConnLifetime, 30 * time.Minute},
		{"DBMaxConnIdleTime", cfg.DBMaxConnIdleTime, 5 * time.Minute},
		{"DBHealthCheckPeriod", cfg.DBHealthCheckPeriod, 30 * time.Second},
		{"DBStatementTimeout", cfg.DBStatementTimeout, 30 * time.Second},
		{"RateLimitRequests", cfg.RateLimitRequests, 100},
		{"RateLimitWindow", cfg.RateLimitWindow, time.Minute},
		{"TriggerRateLimitRequests", cfg.TriggerRateLimitRequests, 10},
		{"TriggerRateLimitWindow", cfg.TriggerRateLimitWindow, time.Minute},
		{"RequestTimeout", cfg.RequestTimeout, 30 * time.Second},
		{"MaxRequestBodySize", cfg.MaxRequestBodySize, int64(1 << 20)},
		{"MaxBulkTriggerItems", cfg.MaxBulkTriggerItems, 500},
		{"AdaptiveConcurrencyMin", cfg.AdaptiveConcurrencyMin, 5},
		{"AdaptiveConcurrencyMax", cfg.AdaptiveConcurrencyMax, 100},
		{"IndexMaintenanceInterval", cfg.IndexMaintenanceInterval, 24 * time.Hour},
		{"WebhookMaxPayloadBytes", cfg.WebhookMaxPayloadBytes, int64(1 << 20)},
		{"WebhookConcurrency", cfg.WebhookConcurrency, 50},
		{"WebhookTimeout", cfg.WebhookTimeout, 10 * time.Second},
		{"WebhookIdleConnTimeout", cfg.WebhookIdleConnTimeout, time.Minute},
		{"ExecutorHTTPTimeout", cfg.ExecutorHTTPTimeout, 5 * time.Minute},
		{"ExecutorIdleConnTimeout", cfg.ExecutorIdleConnTimeout, 90 * time.Second},
		{"ExecutionTraceMode", cfg.ExecutionTraceMode, "off"},
		{"WebhookDispatchTimeout", cfg.WebhookDispatchTimeout, 15 * time.Second},
		{"WebhookMaxAttempts", cfg.WebhookMaxAttempts, 3},
		{"DefaultJobMaxAttempts", cfg.DefaultJobMaxAttempts, 3},
		{"DefaultJobTimeoutSecs", cfg.DefaultJobTimeoutSecs, 300},
		{"WorkflowRetention", cfg.WorkflowRetention, 720 * time.Hour},
		{"ReaperDeleteBatchSize", cfg.ReaperDeleteBatchSize, 5000},
		{"StalledWorkflowThreshold", cfg.StalledWorkflowThreshold, 15 * time.Minute},
		{"StalledWorkflowAction", cfg.StalledWorkflowAction, "reconcile"},
		{"DependencyStatusCacheTTL", cfg.DependencyStatusCacheTTL, 5 * time.Second},
		{"MaxWorkflowNestingDepth", cfg.MaxWorkflowNestingDepth, 10},
		{"CDCBatchSize", cfg.CDCBatchSize, 200},
		{"CDCWaitTimeMs", cfg.CDCWaitTimeMs, 5000},
		{"SSEKeepaliveInterval", cfg.SSEKeepaliveInterval, 15 * time.Second},
		{"WorkerDrainTimeout", cfg.WorkerDrainTimeout, 30 * time.Second},
		{"PermissionCacheTTL", cfg.PermissionCacheTTL, 5 * time.Minute},
		{"LogDrainWorkerInterval", cfg.LogDrainWorkerInterval, time.Minute},
		{"JobCacheTTL", cfg.JobCacheTTL, 5 * time.Minute},
		{"VersionCacheTTL", cfg.VersionCacheTTL, 30 * time.Minute},
		{"RunVersionCacheTTL", cfg.RunVersionCacheTTL, 10 * time.Minute},
		{"APIKeyCacheTTL", cfg.APIKeyCacheTTL, time.Minute},
		{"JobHealthCacheTTL", cfg.JobHealthCacheTTL, 2 * time.Second},
		{"JobDepsCacheTTL", cfg.JobDepsCacheTTL, 5 * time.Minute},
		{"StatusReadModelTTL", cfg.StatusReadModelTTL, 5 * time.Minute},
		{"SharedDedupeTTL", cfg.SharedDedupeTTL, 10 * time.Minute},
		{"MaxResultSize", cfg.MaxResultSize, int64(1 << 20)},
		{"MigrationMode", cfg.MigrationMode, "auto"},
		{"MigrationLockTimeout", cfg.MigrationLockTimeout, 30 * time.Second},
		{"MaxSnoozeCount", cfg.MaxSnoozeCount, 50},
		{"DebouncePollerInterval", cfg.DebouncePollerInterval, time.Second},
		{"BatchFlushInterval", cfg.BatchFlushInterval, time.Second},
		{"DefaultRegion", cfg.DefaultRegion, "iad"},
		{"ClickHouseDatabase", cfg.ClickHouseDatabase, "strait"},
		{"ClickHouseBatchSize", cfg.ClickHouseBatchSize, 1000},
		{"ClickHouseFlushInterval", cfg.ClickHouseFlushInterval, 5 * time.Second},
		{"ResendFromEmail", cfg.ResendFromEmail, "noreply@strait.dev"},
		{"SentryEnvironment", cfg.SentryEnvironment, "development"},
		{"Edition", cfg.Edition, "community"},
		{"SequinBatchSize", cfg.SequinBatchSize, 200},
		{"SequinWaitTimeMs", cfg.SequinWaitTimeMs, 5000},
		{"SequinWebhookSecret", cfg.SequinWebhookSecret, ""},
		{"GRPCBindAddr", cfg.GRPCBindAddr, "127.0.0.1"},
		{"GRPCPort", cfg.GRPCPort, 50051},
		{"RedisPoolSize", cfg.RedisPoolSize, 30},
		{"RedisMinIdleConns", cfg.RedisMinIdleConns, 5},
		{"RedisReadTimeout", cfg.RedisReadTimeout, 3 * time.Second},
		{"RedisWriteTimeout", cfg.RedisWriteTimeout, 3 * time.Second},
		{"RedisConnMaxLifetime", cfg.RedisConnMaxLifetime, 30 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("%s = %v (%T), want %v (%T)", tt.name, tt.got, tt.got, tt.want, tt.want)
			}
		})
	}
}

func TestLoad_DefaultBooleans(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	falseFields := []struct {
		name string
		got  bool
	}{
		{"OIDCEnabled", cfg.OIDCEnabled},
		{"CORSAllowCredentials", cfg.CORSAllowCredentials},
		{"DBPgBouncerMode", cfg.DBPgBouncerMode},
		{"DBPgBouncerPrepared", cfg.DBPgBouncerPrepared},
		{"DBTraceStatements", cfg.DBTraceStatements},
		{"WebhookRequireTLS", cfg.WebhookRequireTLS},
		{"AllowPrivateEndpoints", cfg.AllowPrivateEndpoints},
		{"GRPCAllowPlaintext", cfg.GRPCAllowPlaintext},
		{"ClickHouseEnabled", cfg.ClickHouseEnabled},
		{"ClickHouseExportEnabled", cfg.ClickHouseExportEnabled},
		{"OTLPMetricEnabled", cfg.OTLPMetricEnabled},
		{"BillingEnforcementEnabled", cfg.BillingEnforcementEnabled},
		{"ProfilingEnabled", cfg.ProfilingEnabled},
		{"ProfilingManagementEnabled", cfg.ProfilingManagementEnabled},
	}

	for _, tt := range falseFields {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got {
				t.Fatalf("%s = true, want false", tt.name)
			}
		})
	}

	if !cfg.ProfilingAPIEnabled {
		t.Fatal("ProfilingAPIEnabled = false, want true")
	}
	if cfg.ProfilingManagementBindAddr != "127.0.0.1" {
		t.Fatalf("ProfilingManagementBindAddr = %q, want 127.0.0.1", cfg.ProfilingManagementBindAddr)
	}
	if cfg.ProfilingManagementPort != 18080 {
		t.Fatalf("ProfilingManagementPort = %d, want 18080", cfg.ProfilingManagementPort)
	}
	if cfg.ProfilingMutexFraction != 100 {
		t.Fatalf("ProfilingMutexFraction = %d, want 100", cfg.ProfilingMutexFraction)
	}
	if cfg.ProfilingBlockRate != 100000 {
		t.Fatalf("ProfilingBlockRate = %d, want 100000", cfg.ProfilingBlockRate)
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
				t.Helper()

				t.Setenv("INTERNAL_SECRET", "test-secret-value")
				t.Setenv("JWT_SIGNING_KEY", "aaaa-test-jwt-signing-key-00000000")
			},
			errorSub: "DATABASE_URL",
		},
		{
			name: "missing internal secret",
			setEnv: func(t *testing.T) {
				t.Helper()

				t.Setenv("DATABASE_URL", "postgres://localhost/test")
				t.Setenv("JWT_SIGNING_KEY", "aaaa-test-jwt-signing-key-00000000")
			},
			errorSub: "INTERNAL_SECRET",
		},
		{
			name: "jwt signing key too short",
			setEnv: func(t *testing.T) {
				t.Helper()

				t.Setenv("DATABASE_URL", "postgres://localhost/test")
				t.Setenv("INTERNAL_SECRET", "test-secret-value")
				t.Setenv("JWT_SIGNING_KEY", "too-short")
			},
			errorSub: "JWT_SIGNING_KEY",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
	setRequiredEnv(t)
	t.Setenv("PORT", "9090")
	t.Setenv("WORKER_CONCURRENCY", "20")
	t.Setenv("MODE", "worker")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("INDEX_MAINTENANCE_INTERVAL", "12h")
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
	if cfg.AdaptiveConcurrencyMin != 3 {
		t.Fatalf("AdaptiveConcurrencyMin = %d, want %d", cfg.AdaptiveConcurrencyMin, 3)
	}
	if cfg.AdaptiveConcurrencyMax != 30 {
		t.Fatalf("AdaptiveConcurrencyMax = %d, want %d", cfg.AdaptiveConcurrencyMax, 30)
	}
}

func TestLoad_EncryptionKeyRotationConfig(t *testing.T) {
	setRequiredEnv(t)
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

func TestLoad_EncryptionKeyMirroring(t *testing.T) {
	t.Run("encryption key fills secret encryption key", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("ENCRYPTION_KEY", "my-key")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.SecretEncryptionKey != "my-key" {
			t.Fatalf("SecretEncryptionKey = %q, want %q", cfg.SecretEncryptionKey, "my-key")
		}
	})

	t.Run("secret encryption key fills encryption key", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("SECRET_ENCRYPTION_KEY", "my-secret-key")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.EncryptionKey != "my-secret-key" {
			t.Fatalf("EncryptionKey = %q, want %q", cfg.EncryptionKey, "my-secret-key")
		}
	})

	t.Run("both set independently", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("ENCRYPTION_KEY", "enc-key")
		t.Setenv("SECRET_ENCRYPTION_KEY", "secret-key")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.EncryptionKey != "enc-key" {
			t.Fatalf("EncryptionKey = %q, want %q", cfg.EncryptionKey, "enc-key")
		}
		if cfg.SecretEncryptionKey != "secret-key" {
			t.Fatalf("SecretEncryptionKey = %q, want %q", cfg.SecretEncryptionKey, "secret-key")
		}
	})
}

func TestLoad_InvalidSequinBaseURL(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SEQUIN_BASE_URL", "not-a-url")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid SEQUIN_BASE_URL, got nil")
	}
	if !strings.Contains(err.Error(), "SEQUIN_BASE_URL") {
		t.Fatalf("error = %q, want substring SEQUIN_BASE_URL", err.Error())
	}
}

func TestLoad_ValidConfig(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("REDIS_URL", "redis://localhost:6379")
	t.Setenv("SEQUIN_BASE_URL", "https://sequin.example.com")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.RedisURL != "redis://localhost:6379" {
		t.Fatalf("RedisURL = %q, want redis://localhost:6379", cfg.RedisURL)
	}
}

func TestLoad_WebhookConcurrencyDefault(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.WebhookConcurrency != 50 {
		t.Errorf("WebhookConcurrency = %d, want 50", cfg.WebhookConcurrency)
	}
}

func TestLoad_WebhookConcurrencyFromEnv(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("WEBHOOK_CONCURRENCY", "100")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.WebhookConcurrency != 100 {
		t.Errorf("WebhookConcurrency = %d, want 100", cfg.WebhookConcurrency)
	}
}

func TestLoad_MaxBulkTriggerItemsDefault(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.MaxBulkTriggerItems != 500 {
		t.Errorf("MaxBulkTriggerItems = %d, want 500", cfg.MaxBulkTriggerItems)
	}
}

func TestLoad_DurationOverrides(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("HEARTBEAT_INTERVAL", "10s")
	t.Setenv("REAPER_INTERVAL", "1m")
	t.Setenv("STALE_THRESHOLD", "2m")
	t.Setenv("POLLER_INTERVAL", "2s")
	t.Setenv("DB_MAX_CONN_LIFETIME", "1h")
	t.Setenv("DB_MAX_CONN_IDLE_TIME", "10m")
	t.Setenv("WEBHOOK_TIMEOUT", "30s")
	t.Setenv("EXECUTOR_HTTP_TIMEOUT", "10m")
	t.Setenv("WORKFLOW_RETENTION", "2160h")
	t.Setenv("SSE_KEEPALIVE_INTERVAL", "30s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tests := []struct {
		name string
		got  time.Duration
		want time.Duration
	}{
		{"HeartbeatInterval", cfg.HeartbeatInterval, 10 * time.Second},
		{"ReaperInterval", cfg.ReaperInterval, time.Minute},
		{"StaleThreshold", cfg.StaleThreshold, 2 * time.Minute},
		{"PollerInterval", cfg.PollerInterval, 2 * time.Second},
		{"DBMaxConnLifetime", cfg.DBMaxConnLifetime, time.Hour},
		{"DBMaxConnIdleTime", cfg.DBMaxConnIdleTime, 10 * time.Minute},
		{"WebhookTimeout", cfg.WebhookTimeout, 30 * time.Second},
		{"ExecutorHTTPTimeout", cfg.ExecutorHTTPTimeout, 10 * time.Minute},
		{"WorkflowRetention", cfg.WorkflowRetention, 2160 * time.Hour},
		{"SSEKeepaliveInterval", cfg.SSEKeepaliveInterval, 30 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("%s = %v, want %v", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestLoad_Int32AndInt64Overrides(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("DB_MAX_CONNS", "100")
	t.Setenv("DB_MIN_CONNS", "20")
	t.Setenv("MAX_REQUEST_BODY_SIZE", "2097152")
	t.Setenv("WEBHOOK_MAX_PAYLOAD_BYTES", "5242880")
	t.Setenv("MAX_RESULT_SIZE", "4194304")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.DBMaxConns != 100 {
		t.Fatalf("DBMaxConns = %d, want 100", cfg.DBMaxConns)
	}
	if cfg.DBMinConns != 20 {
		t.Fatalf("DBMinConns = %d, want 20", cfg.DBMinConns)
	}
	if cfg.MaxRequestBodySize != 2097152 {
		t.Fatalf("MaxRequestBodySize = %d, want 2097152", cfg.MaxRequestBodySize)
	}
	if cfg.WebhookMaxPayloadBytes != 5242880 {
		t.Fatalf("WebhookMaxPayloadBytes = %d, want 5242880", cfg.WebhookMaxPayloadBytes)
	}
	if cfg.MaxResultSize != 4194304 {
		t.Fatalf("MaxResultSize = %d, want 4194304", cfg.MaxResultSize)
	}
}

func TestLoad_BoolOverrides(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("DB_PGBOUNCER_MODE", "true")
	t.Setenv("DB_PGBOUNCER_PREPARED_STATEMENTS", "true")
	t.Setenv("DB_TRACE_STATEMENTS", "true")
	t.Setenv("WEBHOOK_REQUIRE_TLS", "true")
	t.Setenv("ALLOW_PRIVATE_ENDPOINTS", "true")
	t.Setenv("GRPC_ALLOW_PLAINTEXT", "true")
	t.Setenv("OTLP_METRIC_ENABLED", "true")
	t.Setenv("CORS_ALLOW_CREDENTIALS", "true")
	t.Setenv("STRAIT_PROFILING_ENABLED", "true")
	t.Setenv("STRAIT_PROFILING_API_ENABLED", "false")
	t.Setenv("STRAIT_PROFILING_MANAGEMENT_ENABLED", "true")
	t.Setenv("STRAIT_PROFILING_MANAGEMENT_BIND_ADDR", "127.0.0.2")
	t.Setenv("STRAIT_PROFILING_MANAGEMENT_PORT", "28080")
	t.Setenv("STRAIT_PROFILING_MUTEX_FRACTION", "50")
	t.Setenv("STRAIT_PROFILING_BLOCK_RATE", "250000")
	t.Setenv("STRAIT_PROFILING_SECRET", "pprof-secret-value")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !cfg.DBPgBouncerMode {
		t.Fatal("DBPgBouncerMode = false, want true")
	}
	if !cfg.DBPgBouncerPrepared {
		t.Fatal("DBPgBouncerPrepared = false, want true")
	}
	if !cfg.DBTraceStatements {
		t.Fatal("DBTraceStatements = false, want true")
	}
	if !cfg.WebhookRequireTLS {
		t.Fatal("WebhookRequireTLS = false, want true")
	}
	if !cfg.AllowPrivateEndpoints {
		t.Fatal("AllowPrivateEndpoints = false, want true")
	}
	if !cfg.GRPCAllowPlaintext {
		t.Fatal("GRPCAllowPlaintext = false, want true")
	}
	if !cfg.OTLPMetricEnabled {
		t.Fatal("OTLPMetricEnabled = false, want true")
	}
	if !cfg.CORSAllowCredentials {
		t.Fatal("CORSAllowCredentials = false, want true")
	}
	if !cfg.ProfilingEnabled {
		t.Fatal("ProfilingEnabled = false, want true")
	}
	if cfg.ProfilingAPIEnabled {
		t.Fatal("ProfilingAPIEnabled = true, want false")
	}
	if !cfg.ProfilingManagementEnabled {
		t.Fatal("ProfilingManagementEnabled = false, want true")
	}
	if cfg.ProfilingManagementBindAddr != "127.0.0.2" {
		t.Fatalf("ProfilingManagementBindAddr = %q, want 127.0.0.2", cfg.ProfilingManagementBindAddr)
	}
	if cfg.ProfilingManagementPort != 28080 {
		t.Fatalf("ProfilingManagementPort = %d, want 28080", cfg.ProfilingManagementPort)
	}
	if cfg.ProfilingMutexFraction != 50 {
		t.Fatalf("ProfilingMutexFraction = %d, want 50", cfg.ProfilingMutexFraction)
	}
	if cfg.ProfilingBlockRate != 250000 {
		t.Fatalf("ProfilingBlockRate = %d, want 250000", cfg.ProfilingBlockRate)
	}
	if cfg.ProfilingSecret != "pprof-secret-value" {
		t.Fatalf("ProfilingSecret = %q, want pprof-secret-value", cfg.ProfilingSecret)
	}
}

func TestLoad_Float64Override(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("MEMORY_PRESSURE_THRESHOLD_PCT", "85.5")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.MemoryPressureThresholdPct != 85.5 {
		t.Fatalf("MemoryPressureThresholdPct = %f, want 85.5", cfg.MemoryPressureThresholdPct)
	}
}

func TestLoad_SliceFields(t *testing.T) {
	t.Run("CORS allowed origins from env", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("CORS_ALLOWED_ORIGINS", "https://app.example.com,https://admin.example.com")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(cfg.CORSAllowedOrigins) != 2 {
			t.Fatalf("len(CORSAllowedOrigins) = %d, want 2", len(cfg.CORSAllowedOrigins))
		}
		if cfg.CORSAllowedOrigins[0] != "https://app.example.com" {
			t.Fatalf("CORSAllowedOrigins[0] = %q, want https://app.example.com", cfg.CORSAllowedOrigins[0])
		}
	})

	t.Run("redis sentinel addrs", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("REDIS_SENTINEL_ADDRS", "host1:26379,host2:26379,host3:26379")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(cfg.RedisSentinelAddrs) != 3 {
			t.Fatalf("len(RedisSentinelAddrs) = %d, want 3", len(cfg.RedisSentinelAddrs))
		}
	})

	t.Run("worker partitions", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("WORKER_PARTITIONS", "critical,default,bulk")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(cfg.WorkerPartitions) != 3 {
			t.Fatalf("len(WorkerPartitions) = %d, want 3", len(cfg.WorkerPartitions))
		}
		if cfg.WorkerPartitions[0] != "critical" {
			t.Fatalf("WorkerPartitions[0] = %q, want critical", cfg.WorkerPartitions[0])
		}
	})

	t.Run("profiling allowed CIDRs", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("STRAIT_PROFILING_ALLOWED_CIDRS", "127.0.0.1/32,10.0.0.0/8")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(cfg.ProfilingAllowedCIDRs) != 2 {
			t.Fatalf("len(ProfilingAllowedCIDRs) = %d, want 2", len(cfg.ProfilingAllowedCIDRs))
		}
		if cfg.ProfilingAllowedCIDRs[0] != "127.0.0.1/32" {
			t.Fatalf("ProfilingAllowedCIDRs[0] = %q, want 127.0.0.1/32", cfg.ProfilingAllowedCIDRs[0])
		}
	})
}

func TestLoad_ProfilingSecurityValidation(t *testing.T) {
	t.Run("short profiling secret rejected", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("STRAIT_PROFILING_SECRET", "short")

		_, err := Load()
		if err == nil || !strings.Contains(err.Error(), "STRAIT_PROFILING_SECRET") {
			t.Fatalf("error = %v, want STRAIT_PROFILING_SECRET", err)
		}
	})

	t.Run("invalid profiling CIDR rejected", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("STRAIT_PROFILING_ALLOWED_CIDRS", "127.0.0.1/32,not-a-cidr")

		_, err := Load()
		if err == nil || !strings.Contains(err.Error(), "STRAIT_PROFILING_ALLOWED_CIDRS") {
			t.Fatalf("error = %v, want STRAIT_PROFILING_ALLOWED_CIDRS", err)
		}
	})

	t.Run("enabled profiling requires listener", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("STRAIT_PROFILING_ENABLED", "true")
		t.Setenv("STRAIT_PROFILING_API_ENABLED", "false")
		t.Setenv("STRAIT_PROFILING_MANAGEMENT_ENABLED", "false")

		_, err := Load()
		if err == nil || !strings.Contains(err.Error(), "STRAIT_PROFILING_ENABLED") {
			t.Fatalf("error = %v, want STRAIT_PROFILING_ENABLED", err)
		}
	})

	t.Run("invalid management port rejected", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("STRAIT_PROFILING_MANAGEMENT_ENABLED", "true")
		t.Setenv("STRAIT_PROFILING_MANAGEMENT_PORT", "70000")

		_, err := Load()
		if err == nil || !strings.Contains(err.Error(), "STRAIT_PROFILING_MANAGEMENT_PORT") {
			t.Fatalf("error = %v, want STRAIT_PROFILING_MANAGEMENT_PORT", err)
		}
	})

	t.Run("negative mutex profiling fraction rejected", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("STRAIT_PROFILING_MUTEX_FRACTION", "-1")

		_, err := Load()
		if err == nil || !strings.Contains(err.Error(), "STRAIT_PROFILING_MUTEX_FRACTION") {
			t.Fatalf("error = %v, want STRAIT_PROFILING_MUTEX_FRACTION", err)
		}
	})

	t.Run("negative block profiling rate rejected", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("STRAIT_PROFILING_BLOCK_RATE", "-1")

		_, err := Load()
		if err == nil || !strings.Contains(err.Error(), "STRAIT_PROFILING_BLOCK_RATE") {
			t.Fatalf("error = %v, want STRAIT_PROFILING_BLOCK_RATE", err)
		}
	})
}

func TestLoad_CDCSequinFallback(t *testing.T) {
	t.Run("CDC uses Sequin batch size when CDC not set", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("SEQUIN_BATCH_SIZE", "50")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.CDCBatchSize != 50 {
			t.Fatalf("CDCBatchSize = %d, want 50 (from SEQUIN_BATCH_SIZE)", cfg.CDCBatchSize)
		}
	})

	t.Run("CDC uses Sequin wait time when CDC not set", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("SEQUIN_WAIT_TIME_MS", "10000")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.CDCWaitTimeMs != 10000 {
			t.Fatalf("CDCWaitTimeMs = %d, want 10000 (from SEQUIN_WAIT_TIME_MS)", cfg.CDCWaitTimeMs)
		}
	})

	t.Run("explicit CDC overrides Sequin", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("CDC_BATCH_SIZE", "25")
		t.Setenv("SEQUIN_BATCH_SIZE", "50")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.CDCBatchSize != 25 {
			t.Fatalf("CDCBatchSize = %d, want 25 (explicit CDC_BATCH_SIZE)", cfg.CDCBatchSize)
		}
	})
}

func TestLoad_EventTriggerRetentionDaysLegacy(t *testing.T) {
	t.Run("days converted to duration", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("EVENT_TRIGGER_RETENTION_DAYS", "14")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := 14 * 24 * time.Hour
		if cfg.EventTriggerRetention != want {
			t.Fatalf("EventTriggerRetention = %v, want %v", cfg.EventTriggerRetention, want)
		}
	})

	t.Run("explicit duration takes precedence over days", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("EVENT_TRIGGER_RETENTION", "168h")
		t.Setenv("EVENT_TRIGGER_RETENTION_DAYS", "14")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := 168 * time.Hour
		if cfg.EventTriggerRetention != want {
			t.Fatalf("EventTriggerRetention = %v, want %v (explicit duration should win)", cfg.EventTriggerRetention, want)
		}
	})
}

func TestLoad_OIDCValidation(t *testing.T) {
	tests := []struct {
		name     string
		setEnv   func(t *testing.T)
		errorSub string
	}{
		{
			name: "OIDC enabled missing issuer",
			setEnv: func(t *testing.T) {
				t.Helper()

				t.Setenv("OIDC_ENABLED", "true")
				t.Setenv("OIDC_AUDIENCE", "my-audience")
				t.Setenv("OIDC_PUBLIC_KEY_PEM", "my-key")
			},
			errorSub: "OIDC_ISSUER",
		},
		{
			name: "OIDC enabled missing audience",
			setEnv: func(t *testing.T) {
				t.Helper()

				t.Setenv("OIDC_ENABLED", "true")
				t.Setenv("OIDC_ISSUER", "https://issuer.example.com")
				t.Setenv("OIDC_PUBLIC_KEY_PEM", "my-key")
			},
			errorSub: "OIDC_AUDIENCE",
		},
		{
			name: "OIDC enabled missing public key",
			setEnv: func(t *testing.T) {
				t.Helper()

				t.Setenv("OIDC_ENABLED", "true")
				t.Setenv("OIDC_ISSUER", "https://issuer.example.com")
				t.Setenv("OIDC_AUDIENCE", "my-audience")
			},
			errorSub: "OIDC_PUBLIC_KEY_PEM",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setRequiredEnv(t)
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

	t.Run("OIDC disabled skips validation", func(t *testing.T) {
		setRequiredEnv(t)
		// OIDC_ENABLED defaults to false, so no OIDC fields needed.
		_, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("OIDC enabled with all fields", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("OIDC_ENABLED", "true")
		t.Setenv("OIDC_ISSUER", "https://issuer.example.com")
		t.Setenv("OIDC_AUDIENCE", "my-audience")
		t.Setenv("OIDC_PUBLIC_KEY_PEM", "my-key-pem")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !cfg.OIDCEnabled {
			t.Fatal("OIDCEnabled = false, want true")
		}
	})
}

func TestLoad_MigrationModeValidation(t *testing.T) {
	validModes := []string{"auto", "manual", "validate"}
	for _, mode := range validModes {
		t.Run("valid_"+mode, func(t *testing.T) {
			setRequiredEnv(t)
			t.Setenv("MIGRATION_MODE", mode)

			cfg, err := Load()
			if err != nil {
				t.Fatalf("unexpected error for mode %q: %v", mode, err)
			}
			if cfg.MigrationMode != mode {
				t.Fatalf("MigrationMode = %q, want %q", cfg.MigrationMode, mode)
			}
		})
	}

	t.Run("invalid mode", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("MIGRATION_MODE", "invalid")

		_, err := Load()
		if err == nil {
			t.Fatal("expected error for invalid MIGRATION_MODE, got nil")
		}
		if !strings.Contains(err.Error(), "MIGRATION_MODE") {
			t.Fatalf("error = %q, want substring MIGRATION_MODE", err.Error())
		}
	})
}

func TestLoad_ClickHouseValidation(t *testing.T) {
	t.Run("enabled without URL fails", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("CLICKHOUSE_ENABLED", "true")

		_, err := Load()
		if err == nil {
			t.Fatal("expected error for CLICKHOUSE_ENABLED without URL, got nil")
		}
		if !strings.Contains(err.Error(), "CLICKHOUSE_URL") {
			t.Fatalf("error = %q, want substring CLICKHOUSE_URL", err.Error())
		}
	})

	t.Run("enabled with URL succeeds", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("CLICKHOUSE_ENABLED", "true")
		t.Setenv("CLICKHOUSE_URL", "clickhouse://localhost:9000")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !cfg.ClickHouseEnabled {
			t.Fatal("ClickHouseEnabled = false, want true")
		}
		if cfg.ClickHouseURL != "clickhouse://localhost:9000" {
			t.Fatalf("ClickHouseURL = %q, want clickhouse://localhost:9000", cfg.ClickHouseURL)
		}
	})
}

func TestLoad_SequinBaseURLValidation(t *testing.T) {
	t.Run("valid HTTPS URL", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("SEQUIN_BASE_URL", "https://sequin.example.com")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.SequinBaseURL != "https://sequin.example.com" {
			t.Fatalf("SequinBaseURL = %q, want https://sequin.example.com", cfg.SequinBaseURL)
		}
	})

	t.Run("valid HTTP URL", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("SEQUIN_BASE_URL", "http://localhost:8080")

		_, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("invalid URL rejected", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("SEQUIN_BASE_URL", "not-a-url")

		_, err := Load()
		if err == nil {
			t.Fatal("expected error for invalid SEQUIN_BASE_URL, got nil")
		}
	})

	t.Run("empty URL is rejected", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("SEQUIN_BASE_URL", "")

		_, err := Load()
		if err == nil {
			t.Fatal("expected error for empty SEQUIN_BASE_URL, got nil")
		}
	})
}

func TestLoad_RequiredRuntimeDependencies(t *testing.T) {
	t.Run("missing Redis URL", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("REDIS_URL", "")

		_, err := Load()
		if err == nil || !strings.Contains(err.Error(), "REDIS_URL") {
			t.Fatalf("error = %v, want REDIS_URL requirement", err)
		}
	})

	t.Run("missing Sequin consumer", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("SEQUIN_CONSUMER_NAME", "")

		_, err := Load()
		if err == nil || !strings.Contains(err.Error(), "SEQUIN_CONSUMER_NAME") {
			t.Fatalf("error = %v, want SEQUIN_CONSUMER_NAME requirement", err)
		}
	})

	t.Run("missing Sequin API token", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("SEQUIN_API_TOKEN", "")

		_, err := Load()
		if err == nil || !strings.Contains(err.Error(), "SEQUIN_API_TOKEN") {
			t.Fatalf("error = %v, want SEQUIN_API_TOKEN requirement", err)
		}
	})
}

func TestDeepSecLoad_SequinWebhookSecret(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SEQUIN_WEBHOOK_SECRET", "cdc-webhook-secret-32-bytes-min")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SequinWebhookSecret != "cdc-webhook-secret-32-bytes-min" {
		t.Fatalf("SequinWebhookSecret = %q", cfg.SequinWebhookSecret)
	}
}

func TestLoad_StringOverrides(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SENTRY_DSN", "https://sentry.io/123")
	t.Setenv("SENTRY_ENVIRONMENT", "production")
	t.Setenv("SENTRY_TRACES_SAMPLE_RATE", "0.25")
	t.Setenv("SENTRY_RELEASE", "2026.05.07-sha")
	t.Setenv("SENTRY_DEBUG", "true")
	t.Setenv("SENTRY_MAX_BREADCRUMBS", "64")
	t.Setenv("SENTRY_MAX_SPANS", "256")
	t.Setenv("SENTRY_MAX_ERROR_DEPTH", "16")
	t.Setenv("SENTRY_STRICT_TRACE_CONTINUATION", "true")
	t.Setenv("SENTRY_SCHEDULER_CHECKINS", "true")
	t.Setenv("SENTRY_SCHEDULER_CHECKIN_PREFIX", "custom-scheduler")
	t.Setenv("RESEND_API_KEY", "re_123")
	t.Setenv("RESEND_FROM_EMAIL", "support@strait.dev")
	t.Setenv("GRPC_BIND_ADDR", "0.0.0.0")
	skipIfCommunity(t)
	t.Setenv("STRAIT_EDITION", "cloud")
	t.Setenv("WORKER_PARTITION_WEIGHTS", "critical:3,default:1")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.SentryDSN != "https://sentry.io/123" {
		t.Fatalf("SentryDSN = %q, want https://sentry.io/123", cfg.SentryDSN)
	}
	if cfg.SentryEnvironment != "production" {
		t.Fatalf("SentryEnvironment = %q, want production", cfg.SentryEnvironment)
	}
	if cfg.SentryTracesSampleRate != 0.25 {
		t.Fatalf("SentryTracesSampleRate = %v, want 0.25", cfg.SentryTracesSampleRate)
	}
	if cfg.SentryRelease != "2026.05.07-sha" {
		t.Fatalf("SentryRelease = %q, want 2026.05.07-sha", cfg.SentryRelease)
	}
	if !cfg.SentryDebug {
		t.Fatal("SentryDebug = false, want true")
	}
	if cfg.SentryMaxBreadcrumbs != 64 {
		t.Fatalf("SentryMaxBreadcrumbs = %d, want 64", cfg.SentryMaxBreadcrumbs)
	}
	if cfg.SentryMaxSpans != 256 {
		t.Fatalf("SentryMaxSpans = %d, want 256", cfg.SentryMaxSpans)
	}
	if cfg.SentryMaxErrorDepth != 16 {
		t.Fatalf("SentryMaxErrorDepth = %d, want 16", cfg.SentryMaxErrorDepth)
	}
	if !cfg.SentryStrictTraceContinuation {
		t.Fatal("SentryStrictTraceContinuation = false, want true")
	}
	if !cfg.SentrySchedulerCheckIns {
		t.Fatal("SentrySchedulerCheckIns = false, want true")
	}
	if cfg.SentrySchedulerCheckInPrefix != "custom-scheduler" {
		t.Fatalf("SentrySchedulerCheckInPrefix = %q, want custom-scheduler", cfg.SentrySchedulerCheckInPrefix)
	}
	if cfg.ResendAPIKey != "re_123" {
		t.Fatalf("ResendAPIKey = %q, want re_123", cfg.ResendAPIKey)
	}
	if cfg.ResendFromEmail != "support@strait.dev" {
		t.Fatalf("ResendFromEmail = %q, want support@strait.dev", cfg.ResendFromEmail)
	}
	if cfg.GRPCBindAddr != "0.0.0.0" {
		t.Fatalf("GRPCBindAddr = %q, want 0.0.0.0", cfg.GRPCBindAddr)
	}
	if cfg.Edition != "cloud" {
		t.Fatalf("Edition = %q, want cloud", cfg.Edition)
	}
	if cfg.WorkerPartitionWeights != "critical:3,default:1" {
		t.Fatalf("WorkerPartitionWeights = %q, want critical:3,default:1", cfg.WorkerPartitionWeights)
	}
}

func TestLoad_StripeBillingFields(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("STRIPE_SECRET_KEY", "sk_test_123")
	t.Setenv("STRIPE_WEBHOOK_SECRET", "whsec_123")
	t.Setenv("STRIPE_STARTER_MONTHLY_PRICE_ID", "price_starter_m")
	t.Setenv("STRIPE_STARTER_YEARLY_PRICE_ID", "price_starter_y")
	t.Setenv("STRIPE_PRO_MONTHLY_PRICE_ID", "price_pro_m")
	t.Setenv("STRIPE_PRO_YEARLY_PRICE_ID", "price_pro_y")
	t.Setenv("BILLING_ENFORCEMENT_ENABLED", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.StripeSecretKey != "sk_test_123" {
		t.Fatalf("StripeSecretKey = %q, want sk_test_123", cfg.StripeSecretKey)
	}
	if cfg.StripeWebhookSecret != "whsec_123" {
		t.Fatalf("StripeWebhookSecret = %q, want whsec_123", cfg.StripeWebhookSecret)
	}
	if !cfg.BillingEnforcementEnabled {
		t.Fatal("BillingEnforcementEnabled = false, want true")
	}
}

func TestParseCSVEnv(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  []string
	}{
		{"empty", "", nil},
		{"single", "key1", []string{"key1"}},
		{"multiple", "key1,key2,key3", []string{"key1", "key2", "key3"}},
		{"with spaces", "key1, key2 , key3", []string{"key1", "key2", "key3"}},
		{"with empty entries", "key1,,key2, ,key3", []string{"key1", "key2", "key3"}},
		{"leading trailing spaces", "  key1, key2  ", []string{"key1", "key2"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envKey := "TEST_CSV_" + strings.ToUpper(tt.name)
			if tt.value != "" {
				t.Setenv(envKey, tt.value)
			}
			got := parseCSVEnv(envKey)
			if tt.value == "" {
				if len(got) != 0 {
					t.Fatalf("parseCSVEnv(%q) = %v, want empty", envKey, got)
				}
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("parseCSVEnv(%q) len = %d, want %d; got %v", envKey, len(got), len(tt.want), got)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("parseCSVEnv(%q)[%d] = %q, want %q", envKey, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestLoad_InvalidRedisURL(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("REDIS_URL", "redis://loc%zz")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid REDIS_URL, got nil")
	}
	if !strings.Contains(err.Error(), "REDIS_URL") {
		t.Fatalf("error = %q, want substring REDIS_URL", err.Error())
	}
}

func TestLoad_WfMaxStepCapDefault(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.WfMaxStepCap != 100 {
		t.Errorf("WfMaxStepCap = %d, want 100", cfg.WfMaxStepCap)
	}
}

// TestEnvExample_ListsAllAuditVars asserts the repository .env.example file
// enumerates every AUDIT_* config field. If a new field is added to config.go
// without updating .env.example, this test fails. When run outside a repo
// checkout (e.g. from a packaged tarball) the file may not be findable —
// in that case the test is skipped with a diagnostic listing the search path.
func TestEnvExample_ListsAllAuditVars(t *testing.T) {
	required := []string{
		"AUDIT_RETENTION_DEFAULT_DAYS",
		"AUDIT_ASYNC_BUFFER_SIZE",
		"AUDIT_SIEM_ENDPOINT",
		"AUDIT_SIEM_AUTH_TOKEN",
		"AUDIT_SIEM_BATCH_SIZE",
		"AUDIT_SIEM_FLUSH_INTERVAL",
		"AUDIT_EXPORT_ROW_CAP_DEFAULT",
		"AUDIT_DLQ_RECLAIM_BATCH",
	}

	// Walk up from the test file's cwd looking for .env.example at the
	// repository root. Test cwd is the package dir.
	start, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	var found string
	var tried []string
	dir := start
	for range 10 {
		candidate := filepath.Join(dir, ".env.example")
		tried = append(tried, candidate)
		if _, err := os.Stat(candidate); err == nil {
			found = candidate
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	if found == "" {
		t.Skipf(".env.example not found; searched: %v", tried)
	}

	data, err := os.ReadFile(found)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", found, err)
	}
	content := string(data)
	for _, key := range required {
		if !strings.Contains(content, key+"=") {
			t.Errorf("%s: missing %q= entry", found, key)
		}
	}
}
