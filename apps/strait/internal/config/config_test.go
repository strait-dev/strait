package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	t.Setenv("DATABASE_URL", "postgres://localhost/test?sslmode=require")
	t.Setenv("REDIS_URL", "redis://localhost:6379")
	t.Setenv("SEQUIN_BASE_URL", "http://localhost:7376")
	t.Setenv("SEQUIN_CONSUMER_NAME", "strait-cdc")
	t.Setenv("SEQUIN_API_TOKEN", "sequin-api-token")
	// Relaxed deployment env so the optional SEQUIN_WEBHOOK_SECRET is not required;
	// keeps the baseline minimal (defaults tests assert the secret stays unset).
	t.Setenv("STRAIT_ENV", "development")
	t.Setenv("INTERNAL_SECRET", "test-secret-value")
	t.Setenv("JWT_SIGNING_KEY", "aaaa-test-jwt-signing-key-00000000")
}

func TestLoad_Defaults(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := Load()
	require.NoError(t, err)

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
		{"PollerInterval", cfg.PollerInterval, time.Second},
		{"RunRetentionShort", cfg.RunRetentionShort, 720 * time.Hour},
		{"RunRetentionLong", cfg.RunRetentionLong, 2160 * time.Hour},
		{"WorkflowRunRetentionDays", cfg.WorkflowRunRetentionDays, 30},
		{"DBMaxConns", cfg.DBMaxConns, int32(50)},
		{"DBMinConns", cfg.DBMinConns, int32(10)},
		{"DBMaxConnLifetime", cfg.DBMaxConnLifetime, 30 * time.Minute},
		{"DBMaxConnIdleTime", cfg.DBMaxConnIdleTime, 5 * time.Minute},
		{"DBHealthCheckPeriod", cfg.DBHealthCheckPeriod, 30 * time.Second},
		{"DBStatementTimeout", cfg.DBStatementTimeout, 30 * time.Second},
		{"DBBackpressureSampleInterval", cfg.DBBackpressureSampleInterval, 100 * time.Millisecond},
		{"DBBackpressureAcquireWaitThreshold", cfg.DBBackpressureAcquireWaitThreshold, 50 * time.Millisecond},
		{"DBBackpressureOccupancyThreshold", cfg.DBBackpressureOccupancyThreshold, 0.90},
		{"RateLimitRequests", cfg.RateLimitRequests, 100},
		{"RateLimitWindow", cfg.RateLimitWindow, time.Minute},
		{"TriggerRateLimitRequests", cfg.TriggerRateLimitRequests, 10},
		{"TriggerRateLimitWindow", cfg.TriggerRateLimitWindow, time.Minute},
		{"BackpressureEnabled", cfg.BackpressureEnabled, true},
		{"BackpressureDefaultMaxTokens", cfg.BackpressureDefaultMaxTokens, 1000},
		{"BackpressureDefaultRefillPerSec", cfg.BackpressureDefaultRefillPerSec, 500},
		{"BackpressureLocalLeaseSize", cfg.BackpressureLocalLeaseSize, 32},
		{"BackpressureSamplerInterval", cfg.BackpressureSamplerInterval, 15 * time.Second},
		{"BackpressureSamplerN", cfg.BackpressureSamplerN, 100},
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
		{"AdaptiveTimeoutEnabled", cfg.AdaptiveTimeoutEnabled, false},
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
		{"JobHealthCacheTTL", cfg.JobHealthCacheTTL, 5 * time.Minute},
		{"JobHealthStatsCacheTTL", cfg.JobHealthStatsCacheTTL, 5 * time.Minute},
		{"EndpointGuardCacheTTL", cfg.EndpointGuardCacheTTL, time.Second},
		{"EndpointHealthSuccessSampleInterval", cfg.EndpointHealthSuccessSampleInterval, time.Second},
		{"EndpointCircuitSuccessSampleInterval", cfg.EndpointCircuitSuccessSampleInterval, time.Second},
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
			require.Equal(t, tt.want, tt.got)
		})
	}
}

func TestLoad_DefaultBooleans(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := Load()
	require.NoError(t, err)

	falseFields := []struct {
		name string
		got  bool
	}{
		{"OIDCEnabled", cfg.OIDCEnabled},
		{"CORSAllowCredentials", cfg.CORSAllowCredentials},
		{"DBPgBouncerMode", cfg.DBPgBouncerMode},
		{"DBPgBouncerPrepared", cfg.DBPgBouncerPrepared},
		{"DBTraceStatements", cfg.DBTraceStatements},
		{"DBBackpressureDisabled", cfg.DBBackpressureDisabled},
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
			require.False(t, tt.got)
		})
	}
	require.True(t, cfg.ProfilingAPIEnabled)
	require.Equal(t, "127.0.0.1",
		cfg.ProfilingManagementBindAddr,
	)
	require.Equal(t, 18080, cfg.
		ProfilingManagementPort,
	)
	require.Equal(t, 100, cfg.ProfilingMutexFraction)
	require.Equal(t, 100000, cfg.
		ProfilingBlockRate,
	)
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

				t.Setenv("DATABASE_URL", "postgres://localhost/test?sslmode=require")
				t.Setenv("JWT_SIGNING_KEY", "aaaa-test-jwt-signing-key-00000000")
			},
			errorSub: "INTERNAL_SECRET",
		},
		{
			name: "jwt signing key too short",
			setEnv: func(t *testing.T) {
				t.Helper()

				t.Setenv("DATABASE_URL", "postgres://localhost/test?sslmode=require")
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
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.errorSub)
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
	t.Setenv("BACKPRESSURE_DEFAULT_MAX_TOKENS", "4000")
	t.Setenv("BACKPRESSURE_DEFAULT_REFILL_PER_SEC", "750")
	t.Setenv("BACKPRESSURE_LOCAL_LEASE_SIZE", "64")
	t.Setenv("DB_BACKPRESSURE_DISABLED", "true")
	t.Setenv("DB_BACKPRESSURE_SAMPLE_INTERVAL", "250ms")
	t.Setenv("DB_BACKPRESSURE_ACQUIRE_WAIT_THRESHOLD", "200ms")
	t.Setenv("DB_BACKPRESSURE_OCCUPANCY_THRESHOLD", "0.98")
	t.Setenv("ENDPOINT_GUARD_CACHE_TTL", "3s")
	t.Setenv("ENDPOINT_HEALTH_SUCCESS_SAMPLE_INTERVAL", "5s")
	t.Setenv("ENDPOINT_CIRCUIT_SUCCESS_SAMPLE_INTERVAL", "7s")

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, 9090, cfg.Port)
	require.Equal(t, 20, cfg.WorkerConcurrency)
	require.Equal(t, "worker", cfg.
		Mode)
	require.Equal(t, "debug", cfg.
		LogLevel)
	require.Equal(t, 12*time.Hour,
		cfg.IndexMaintenanceInterval,
	)
	require.Equal(t, 3, cfg.AdaptiveConcurrencyMin)
	require.Equal(t, 30, cfg.AdaptiveConcurrencyMax)
	require.Equal(t, 4000, cfg.BackpressureDefaultMaxTokens)
	require.Equal(t, 750, cfg.BackpressureDefaultRefillPerSec)
	require.Equal(t, 64, cfg.BackpressureLocalLeaseSize)
	require.True(t, cfg.DBBackpressureDisabled)
	require.Equal(t, 250*time.Millisecond, cfg.DBBackpressureSampleInterval)
	require.Equal(t, 200*time.Millisecond, cfg.DBBackpressureAcquireWaitThreshold)
	require.InDelta(t, 0.98, cfg.DBBackpressureOccupancyThreshold, 0.0001)
	require.Equal(t, 3*time.Second, cfg.EndpointGuardCacheTTL)
	require.Equal(t, 5*time.Second, cfg.EndpointHealthSuccessSampleInterval)
	require.Equal(t, 7*time.Second, cfg.EndpointCircuitSuccessSampleInterval)
}

func TestLoad_BackpressureDisabledAllowsZeroBucket(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("BACKPRESSURE_ENABLED", "false")
	t.Setenv("BACKPRESSURE_DEFAULT_MAX_TOKENS", "0")
	t.Setenv("BACKPRESSURE_DEFAULT_REFILL_PER_SEC", "0")

	cfg, err := Load()
	require.NoError(t, err)
	require.False(t, cfg.BackpressureEnabled)
	require.Zero(t, cfg.BackpressureDefaultMaxTokens)
	require.Zero(t, cfg.BackpressureDefaultRefillPerSec)
}

func TestLoad_BackpressureValidation(t *testing.T) {
	tests := []struct {
		name     string
		env      map[string]string
		errorSub string
	}{
		{
			name: "enabled requires positive max tokens",
			env: map[string]string{
				"BACKPRESSURE_DEFAULT_MAX_TOKENS": "0",
			},
			errorSub: "BACKPRESSURE_DEFAULT_MAX_TOKENS must be > 0",
		},
		{
			name: "rejects negative max tokens",
			env: map[string]string{
				"BACKPRESSURE_DEFAULT_MAX_TOKENS": "-1",
			},
			errorSub: "BACKPRESSURE_DEFAULT_MAX_TOKENS must be >= 0",
		},
		{
			name: "rejects negative refill",
			env: map[string]string{
				"BACKPRESSURE_DEFAULT_REFILL_PER_SEC": "-1",
			},
			errorSub: "BACKPRESSURE_DEFAULT_REFILL_PER_SEC must be >= 0",
		},
		{
			name: "rejects zero local lease",
			env: map[string]string{
				"BACKPRESSURE_LOCAL_LEASE_SIZE": "0",
			},
			errorSub: "BACKPRESSURE_LOCAL_LEASE_SIZE must be >= 1",
		},
		{
			name: "rejects negative db acquire wait threshold",
			env: map[string]string{
				"DB_BACKPRESSURE_ACQUIRE_WAIT_THRESHOLD": "-1ms",
			},
			errorSub: "DB_BACKPRESSURE_ACQUIRE_WAIT_THRESHOLD must be >= 0",
		},
		{
			name: "rejects zero db occupancy threshold",
			env: map[string]string{
				"DB_BACKPRESSURE_OCCUPANCY_THRESHOLD": "0",
			},
			errorSub: "DB_BACKPRESSURE_OCCUPANCY_THRESHOLD must be > 0 and <= 1",
		},
		{
			name: "rejects high db occupancy threshold",
			env: map[string]string{
				"DB_BACKPRESSURE_OCCUPANCY_THRESHOLD": "1.1",
			},
			errorSub: "DB_BACKPRESSURE_OCCUPANCY_THRESHOLD must be > 0 and <= 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setRequiredEnv(t)
			for key, value := range tt.env {
				t.Setenv(key, value)
			}

			_, err := Load()
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.errorSub)
		})
	}
}

func TestLoad_EncryptionKeyRotationConfig(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("ENCRYPTION_KEY", "primary-key")
	t.Setenv("ENCRYPTION_KEY_OLD", "old-key-1, old-key-2 , ,old-key-3")

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, "primary-key",
		cfg.EncryptionKey,
	)
	require.Equal(t, "primary-key",
		cfg.SecretEncryptionKey,
	)
	require.Len(t, cfg.EncryptionKeyOld,
		3)
	require.False(t, cfg.EncryptionKeyOld[0] != "old-key-1" ||

		cfg.EncryptionKeyOld[1] !=
			"old-key-2" ||
		cfg.EncryptionKeyOld[2] != "old-key-3")
}

func TestLoad_EncryptionKeyMirroring(t *testing.T) {
	t.Run("encryption key fills secret encryption key", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("ENCRYPTION_KEY", "my-key")

		cfg, err := Load()
		require.NoError(t, err)
		require.Equal(t, "my-key", cfg.
			SecretEncryptionKey,
		)
	})

	t.Run("secret encryption key fills encryption key", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("SECRET_ENCRYPTION_KEY", "my-secret-key")

		cfg, err := Load()
		require.NoError(t, err)
		require.Equal(t, "my-secret-key",
			cfg.EncryptionKey,
		)
	})

	t.Run("both set independently", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("ENCRYPTION_KEY", "enc-key")
		t.Setenv("SECRET_ENCRYPTION_KEY", "secret-key")

		cfg, err := Load()
		require.NoError(t, err)
		require.Equal(t, "enc-key",
			cfg.EncryptionKey,
		)
		require.Equal(t, "secret-key",
			cfg.SecretEncryptionKey,
		)
	})
}

func TestLoad_InvalidSequinBaseURL(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SEQUIN_BASE_URL", "not-a-url")

	_, err := Load()
	require.Error(t, err)
	require.Contains(t, err.Error(), "SEQUIN_BASE_URL")
}

func TestLoad_ValidConfig(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("REDIS_URL", "redis://localhost:6379")
	t.Setenv("SEQUIN_BASE_URL", "https://sequin.example.com")

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, "redis://localhost:6379",

		cfg.RedisURL,
	)
}

func TestLoad_WebhookConcurrencyDefault(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, 50, cfg.WebhookConcurrency)
}

func TestLoad_WebhookConcurrencyFromEnv(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("WEBHOOK_CONCURRENCY", "100")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, 100, cfg.WebhookConcurrency)
}

func TestLoad_MaxBulkTriggerItemsDefault(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, 500, cfg.MaxBulkTriggerItems)
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
	require.NoError(t, err)

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
			require.Equal(t, tt.want, tt.
				got)
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
	require.NoError(t, err)
	require.Equal(t, int32(100), cfg.DBMaxConns)
	require.Equal(t, int32(20), cfg.DBMinConns)
	require.Equal(t, int64(2097152), cfg.MaxRequestBodySize)
	require.Equal(t, int64(5242880), cfg.WebhookMaxPayloadBytes)
	require.Equal(t, int64(4194304), cfg.MaxResultSize)
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
	require.NoError(t, err)
	require.True(t, cfg.DBPgBouncerMode)
	require.True(t, cfg.DBPgBouncerPrepared)
	require.True(t, cfg.DBTraceStatements)
	require.True(t, cfg.WebhookRequireTLS)
	require.True(t, cfg.AllowPrivateEndpoints)
	require.True(t, cfg.GRPCAllowPlaintext)
	require.True(t, cfg.OTLPMetricEnabled)
	require.True(t, cfg.CORSAllowCredentials)
	require.True(t, cfg.ProfilingEnabled)
	require.False(t, cfg.ProfilingAPIEnabled)
	require.True(t, cfg.ProfilingManagementEnabled)
	require.Equal(t, "127.0.0.2",
		cfg.ProfilingManagementBindAddr,
	)
	require.Equal(t, 28080, cfg.
		ProfilingManagementPort,
	)
	require.Equal(t, 50, cfg.ProfilingMutexFraction)
	require.Equal(t, 250000, cfg.
		ProfilingBlockRate,
	)
	require.Equal(t, "pprof-secret-value",

		cfg.ProfilingSecret,
	)
}

func TestLoad_Float64Override(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("MEMORY_PRESSURE_THRESHOLD_PCT", "85.5")

	cfg, err := Load()
	require.NoError(t, err)
	require.InDelta(t, 85.5, cfg.MemoryPressureThresholdPct, 1e-9)
}

func TestLoad_SliceFields(t *testing.T) {
	t.Run("CORS allowed origins from env", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("CORS_ALLOWED_ORIGINS", "https://app.example.com,https://admin.example.com")

		cfg, err := Load()
		require.NoError(t, err)
		require.Len(t, cfg.CORSAllowedOrigins,

			2)
		require.Equal(t, "https://app.example.com",

			cfg.CORSAllowedOrigins[0])
	})

	t.Run("redis sentinel addrs", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("REDIS_SENTINEL_ADDRS", "host1:26379,host2:26379,host3:26379")

		cfg, err := Load()
		require.NoError(t, err)
		require.Len(t, cfg.RedisSentinelAddrs,

			3)
	})

	t.Run("worker partitions", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("WORKER_PARTITIONS", "critical,default,bulk")

		cfg, err := Load()
		require.NoError(t, err)
		require.Len(t, cfg.WorkerPartitions,
			3)
		require.Equal(t, "critical",
			cfg.WorkerPartitions[0])
	})

	t.Run("profiling allowed CIDRs", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("STRAIT_PROFILING_ALLOWED_CIDRS", "127.0.0.1/32,10.0.0.0/8")

		cfg, err := Load()
		require.NoError(t, err)
		require.Len(t, cfg.ProfilingAllowedCIDRs,

			2)
		require.Equal(t, "127.0.0.1/32",
			cfg.ProfilingAllowedCIDRs[0])
	})
}

func TestLoad_ProfilingSecurityValidation(t *testing.T) {
	t.Run("short profiling secret rejected", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("STRAIT_PROFILING_SECRET", "short")

		_, err := Load()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "STRAIT_PROFILING_SECRET")
	})

	t.Run("invalid profiling CIDR rejected", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("STRAIT_PROFILING_ALLOWED_CIDRS", "127.0.0.1/32,not-a-cidr")

		_, err := Load()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "STRAIT_PROFILING_ALLOWED_CIDRS")
	})

	t.Run("enabled profiling requires listener", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("STRAIT_PROFILING_ENABLED", "true")
		t.Setenv("STRAIT_PROFILING_API_ENABLED", "false")
		t.Setenv("STRAIT_PROFILING_MANAGEMENT_ENABLED", "false")

		_, err := Load()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "STRAIT_PROFILING_ENABLED")
	})

	t.Run("invalid management port rejected", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("STRAIT_PROFILING_MANAGEMENT_ENABLED", "true")
		t.Setenv("STRAIT_PROFILING_MANAGEMENT_PORT", "70000")

		_, err := Load()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "STRAIT_PROFILING_MANAGEMENT_PORT")
	})

	t.Run("negative mutex profiling fraction rejected", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("STRAIT_PROFILING_MUTEX_FRACTION", "-1")

		_, err := Load()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "STRAIT_PROFILING_MUTEX_FRACTION")
	})

	t.Run("negative block profiling rate rejected", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("STRAIT_PROFILING_BLOCK_RATE", "-1")

		_, err := Load()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "STRAIT_PROFILING_BLOCK_RATE")
	})
}

func TestLoad_CDCSequinFallback(t *testing.T) {
	t.Run("CDC uses Sequin batch size when CDC not set", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("SEQUIN_BATCH_SIZE", "50")

		cfg, err := Load()
		require.NoError(t, err)
		require.Equal(t, 50, cfg.CDCBatchSize)
	})

	t.Run("CDC uses Sequin wait time when CDC not set", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("SEQUIN_WAIT_TIME_MS", "10000")

		cfg, err := Load()
		require.NoError(t, err)
		require.Equal(t, 10000, cfg.
			CDCWaitTimeMs,
		)
	})

	t.Run("explicit CDC overrides Sequin", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("CDC_BATCH_SIZE", "25")
		t.Setenv("SEQUIN_BATCH_SIZE", "50")

		cfg, err := Load()
		require.NoError(t, err)
		require.Equal(t, 25, cfg.CDCBatchSize)
	})
}

func TestLoad_EventTriggerRetentionDaysLegacy(t *testing.T) {
	t.Run("days converted to duration", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("EVENT_TRIGGER_RETENTION_DAYS", "14")

		cfg, err := Load()
		require.NoError(t, err)

		want := 14 * 24 * time.Hour
		require.Equal(t, want, cfg.EventTriggerRetention)
	})

	t.Run("explicit duration takes precedence over days", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("EVENT_TRIGGER_RETENTION", "168h")
		t.Setenv("EVENT_TRIGGER_RETENTION_DAYS", "14")

		cfg, err := Load()
		require.NoError(t, err)

		want := 168 * time.Hour
		require.Equal(t, want, cfg.EventTriggerRetention)
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
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.errorSub)
		})
	}

	t.Run("OIDC disabled skips validation", func(t *testing.T) {
		setRequiredEnv(t)
		// OIDC_ENABLED defaults to false, so no OIDC fields needed.
		_, err := Load()
		require.NoError(t, err)
	})

	t.Run("OIDC enabled with all fields", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("OIDC_ENABLED", "true")
		t.Setenv("OIDC_ISSUER", "https://issuer.example.com")
		t.Setenv("OIDC_AUDIENCE", "my-audience")
		t.Setenv("OIDC_PUBLIC_KEY_PEM", "my-key-pem")

		cfg, err := Load()
		require.NoError(t, err)
		require.True(t, cfg.OIDCEnabled)
	})
}

func TestLoad_MigrationModeValidation(t *testing.T) {
	validModes := []string{"auto", "manual", "validate"}
	for _, mode := range validModes {
		t.Run("valid_"+mode, func(t *testing.T) {
			setRequiredEnv(t)
			t.Setenv("MIGRATION_MODE", mode)

			cfg, err := Load()
			require.NoError(t, err)
			require.Equal(t, mode, cfg.MigrationMode)
		})
	}

	t.Run("invalid mode", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("MIGRATION_MODE", "invalid")

		_, err := Load()
		require.Error(t, err)
		require.Contains(t, err.Error(), "MIGRATION_MODE")
	})
}

func TestLoad_ClickHouseValidation(t *testing.T) {
	t.Run("enabled without URL fails", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("CLICKHOUSE_ENABLED", "true")

		_, err := Load()
		require.Error(t, err)
		require.Contains(t, err.Error(), "CLICKHOUSE_URL")
	})

	t.Run("enabled with URL succeeds", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("CLICKHOUSE_ENABLED", "true")
		t.Setenv("CLICKHOUSE_URL", "clickhouse://localhost:9000")

		cfg, err := Load()
		require.NoError(t, err)
		require.True(t, cfg.ClickHouseEnabled)
		require.Equal(t, "clickhouse://localhost:9000",

			cfg.
				ClickHouseURL,
		)
	})
}

func TestLoad_SequinBaseURLValidation(t *testing.T) {
	t.Run("valid HTTPS URL", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("SEQUIN_BASE_URL", "https://sequin.example.com")

		cfg, err := Load()
		require.NoError(t, err)
		require.Equal(t, "https://sequin.example.com",

			cfg.
				SequinBaseURL,
		)
	})

	t.Run("valid HTTP URL", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("SEQUIN_BASE_URL", "http://localhost:8080")

		_, err := Load()
		require.NoError(t, err)
	})

	t.Run("invalid URL rejected", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("SEQUIN_BASE_URL", "not-a-url")

		_, err := Load()
		require.Error(t, err)
	})

	t.Run("empty URL is rejected", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("SEQUIN_BASE_URL", "")

		_, err := Load()
		require.Error(t, err)
	})
}

func TestLoad_RequiredRuntimeDependencies(t *testing.T) {
	t.Run("missing Redis URL", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("REDIS_URL", "")

		_, err := Load()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "REDIS_URL")
	})

	t.Run("missing Sequin consumer", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("SEQUIN_CONSUMER_NAME", "")

		_, err := Load()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "SEQUIN_CONSUMER_NAME")
	})

	t.Run("missing Sequin API token", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("SEQUIN_API_TOKEN", "")

		_, err := Load()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "SEQUIN_API_TOKEN")
	})
}

func TestDeepSecLoad_SequinWebhookSecret(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SEQUIN_WEBHOOK_SECRET", "cdc-webhook-secret-32-bytes-min")

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, "cdc-webhook-secret-32-bytes-min",

		cfg.SequinWebhookSecret,
	)
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
	require.NoError(t, err)
	require.Equal(t, "https://sentry.io/123",

		cfg.SentryDSN,
	)
	require.Equal(t, "production",
		cfg.SentryEnvironment,
	)
	require.InDelta(t, 0.25, cfg.SentryTracesSampleRate, 1e-9)
	require.Equal(t, "2026.05.07-sha",
		cfg.
			SentryRelease,
	)
	require.True(t, cfg.SentryDebug)
	require.Equal(t, 64, cfg.SentryMaxBreadcrumbs)
	require.Equal(t, 256, cfg.SentryMaxSpans)
	require.Equal(t, 16, cfg.SentryMaxErrorDepth)
	require.True(t, cfg.SentryStrictTraceContinuation)
	require.True(t, cfg.SentrySchedulerCheckIns)
	require.Equal(t, "custom-scheduler",
		cfg.
			SentrySchedulerCheckInPrefix,
	)
	require.Equal(t, "re_123", cfg.
		ResendAPIKey,
	)
	require.Equal(t, "support@strait.dev",

		cfg.ResendFromEmail,
	)
	require.Equal(t, "0.0.0.0",
		cfg.GRPCBindAddr,
	)
	require.Equal(t, "cloud", cfg.
		Edition)
	require.Equal(t, "critical:3,default:1",

		cfg.WorkerPartitionWeights,
	)
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
	require.NoError(t, err)
	require.Equal(t, "sk_test_123",
		cfg.StripeSecretKey,
	)
	require.Equal(t, "whsec_123",
		cfg.StripeWebhookSecret,
	)
	require.True(t, cfg.BillingEnforcementEnabled)
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
				require.Empty(t, got)

				return
			}
			require.Len(t, got, len(tt.want))

			for i := range got {
				require.Equal(t, tt.want[i],
					got[i])
			}
		})
	}
}

func TestLoad_InvalidRedisURL(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("REDIS_URL", "redis://loc%zz")

	_, err := Load()
	require.Error(t, err)
	require.Contains(t, err.Error(), "REDIS_URL")
}

func TestLoad_WfMaxStepCapDefault(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, 100, cfg.WfMaxStepCap)
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
	require.NoError(t, err)

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
	require.NoError(t, err)

	content := string(data)
	for _, key := range required {
		assert.Contains(t, content, key+"=")
	}
}
