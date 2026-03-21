package config

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"time"

	"strait/internal/domain"

	"github.com/cristalhq/aconfig"
)

type Config struct {
	DatabaseURL               string        `env:"DATABASE_URL"`
	RedisURL                  string        `env:"REDIS_URL"`
	RedisSentinelMaster       string        `env:"REDIS_SENTINEL_MASTER"`
	RedisSentinelAddrs        []string      `env:"REDIS_SENTINEL_ADDRS"`
	Mode                      string        `env:"MODE" default:"all"`
	Port                      int           `env:"PORT" default:"8080"`
	WorkerConcurrency         int           `env:"WORKER_CONCURRENCY" default:"25"`
	InternalSecret            string        `env:"INTERNAL_SECRET"`
	JWTSigningKey             string        `env:"JWT_SIGNING_KEY"`
	SecretEncryptionKey       string        `env:"SECRET_ENCRYPTION_KEY"`
	EncryptionKey             string        `env:"ENCRYPTION_KEY"`
	EncryptionKeyOld          []string      `env:"ENCRYPTION_KEY_OLD"`
	OIDCEnabled               bool          `env:"OIDC_ENABLED" default:"false"`
	OIDCIssuer                string        `env:"OIDC_ISSUER"`
	OIDCAudience              string        `env:"OIDC_AUDIENCE"`
	OIDCPublicKeyPEM          string        `env:"OIDC_PUBLIC_KEY_PEM"`
	LogLevel                  string        `env:"LOG_LEVEL" default:"info"`
	HeartbeatInterval         time.Duration `env:"HEARTBEAT_INTERVAL" default:"10s"`
	ReaperInterval            time.Duration `env:"REAPER_INTERVAL" default:"30s"`
	StaleThreshold            time.Duration `env:"STALE_THRESHOLD" default:"1m"`
	PollerInterval            time.Duration `env:"POLLER_INTERVAL" default:"5s"`
	RunRetentionShort         time.Duration `env:"RUN_RETENTION_SHORT" default:"720h"`
	RunRetentionLong          time.Duration `env:"RUN_RETENTION_LONG" default:"2160h"`
	OTELEndpoint              string        `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	WorkflowRunRetentionDays  int           `env:"WORKFLOW_RUN_RETENTION_DAYS" default:"30"`
	EventTriggerRetentionDays int           `env:"EVENT_TRIGGER_RETENTION_DAYS"`

	// Database connection pool tuning
	DBMaxConns         int32         `env:"DB_MAX_CONNS" default:"50"`
	DBMinConns         int32         `env:"DB_MIN_CONNS" default:"10"`
	DBMaxConnLifetime  time.Duration `env:"DB_MAX_CONN_LIFETIME" default:"30m"`
	DBMaxConnIdleTime  time.Duration `env:"DB_MAX_CONN_IDLE_TIME" default:"5m"`
	DBStatementTimeout time.Duration `env:"DB_STATEMENT_TIMEOUT" default:"30s"`

	RateLimitRequests int           `env:"RATE_LIMIT_REQUESTS" default:"100"`
	RateLimitWindow   time.Duration `env:"RATE_LIMIT_WINDOW" default:"1m"`

	TriggerRateLimitRequests int           `env:"TRIGGER_RATE_LIMIT_REQUESTS" default:"10"`
	TriggerRateLimitWindow   time.Duration `env:"TRIGGER_RATE_LIMIT_WINDOW" default:"1m"`

	RequestTimeout      time.Duration `env:"REQUEST_TIMEOUT" default:"30s"`
	MaxRequestBodySize  int64         `env:"MAX_REQUEST_BODY_SIZE" default:"1048576"`
	MaxBulkTriggerItems int           `env:"MAX_BULK_TRIGGER_ITEMS" default:"500"`

	// Sequin CDC settings
	SequinBaseURL      string `env:"SEQUIN_BASE_URL"`
	SequinConsumerName string `env:"SEQUIN_CONSUMER_NAME"`
	SequinAPIToken     string `env:"SEQUIN_API_TOKEN"`
	SequinBatchSize    int    `env:"SEQUIN_BATCH_SIZE" default:"10"`
	SequinWaitTimeMs   int    `env:"SEQUIN_WAIT_TIME_MS" default:"5000"`

	// CORS settings
	CORSAllowedOrigins   []string `env:"CORS_ALLOWED_ORIGINS" default:"*"`
	CORSAllowCredentials bool     `env:"CORS_ALLOW_CREDENTIALS" default:"false"`

	WorkerPartitions       []string `env:"WORKER_PARTITIONS"`
	WorkerPartitionWeights string   `env:"WORKER_PARTITION_WEIGHTS"`
	AdaptiveConcurrencyMin int      `env:"ADAPTIVE_CONCURRENCY_MIN" default:"5"`
	AdaptiveConcurrencyMax int      `env:"ADAPTIVE_CONCURRENCY_MAX" default:"100"`
	DBPgBouncerMode        bool     `env:"DB_PGBOUNCER_MODE" default:"false"`

	WorkerDrainTimeout time.Duration `env:"WORKER_DRAIN_TIMEOUT" default:"30s"`

	// RBAC permission cache
	PermissionCacheTTL time.Duration `env:"PERMISSION_CACHE_TTL" default:"5m"`

	// Worker/Executor timeouts
	WebhookTimeout          time.Duration `env:"WEBHOOK_TIMEOUT" default:"10s"`
	WebhookIdleConnTimeout  time.Duration `env:"WEBHOOK_IDLE_CONN_TIMEOUT" default:"1m"`
	ExecutorHTTPTimeout     time.Duration `env:"EXECUTOR_HTTP_TIMEOUT" default:"5m"`
	ExecutorIdleConnTimeout time.Duration `env:"EXECUTOR_IDLE_CONN_TIMEOUT" default:"1m30s"`
	WebhookDispatchTimeout  time.Duration `env:"WEBHOOK_DISPATCH_TIMEOUT" default:"15s"`
	WebhookMaxPayloadBytes  int64         `env:"WEBHOOK_MAX_PAYLOAD_BYTES" default:"1048576"`
	WebhookConcurrency      int           `env:"WEBHOOK_CONCURRENCY" default:"50"`

	// Worker settings
	WebhookMaxAttempts       int `env:"WEBHOOK_MAX_ATTEMPTS" default:"3"`
	DefaultJobMaxAttempts    int `env:"DEFAULT_JOB_MAX_ATTEMPTS" default:"3"`
	DefaultJobTimeoutSecs    int `env:"DEFAULT_JOB_TIMEOUT_SECS" default:"300"`
	DefaultJobMaxConcurrency int `env:"DEFAULT_JOB_MAX_CONCURRENCY" default:"0"`
	WorkerQueueSize          int `env:"WORKER_QUEUE_SIZE" default:"0"`
	MaxDequeueBatchSize      int `env:"MAX_DEQUEUE_BATCH_SIZE" default:"0"`

	// Scheduler settings
	WorkflowRetention        time.Duration `env:"WORKFLOW_RETENTION" default:"720h"`
	EventTriggerRetention    time.Duration `env:"EVENT_TRIGGER_RETENTION"`
	IndexMaintenanceInterval time.Duration `env:"INDEX_MAINTENANCE_INTERVAL" default:"24h"`
	ReaperDeleteBatchSize    int           `env:"REAPER_DELETE_BATCH_SIZE" default:"100"`
	StalledWorkflowThreshold time.Duration `env:"WF_STALL_THRESHOLD" default:"15m"`
	StalledWorkflowAction    string        `env:"WF_STALL_ACTION" default:"log_only"`
	WfMaxStepCap             int           `env:"WF_MAX_STEP_CAP" default:"0"`
	WfStepConcurrencyLimit   int           `env:"WF_STEP_CONCURRENCY_LIMIT" default:"0"`
	DependencyStatusCacheTTL time.Duration `env:"DEPENDENCY_STATUS_CACHE_TTL" default:"5s"`

	// Workflow settings
	MaxWorkflowNestingDepth int `env:"MAX_WORKFLOW_NESTING_DEPTH" default:"10"`

	CDCBatchSize  int `env:"CDC_BATCH_SIZE" default:"10"`
	CDCWaitTimeMs int `env:"CDC_WAIT_TIME_MS" default:"5000"`

	// SSE settings
	SSEKeepaliveInterval time.Duration `env:"SSE_KEEPALIVE_INTERVAL" default:"15s"`

	// Log drain settings
	LogDrainWorkerInterval     time.Duration `env:"LOG_DRAIN_WORKER_INTERVAL" default:"1m"`
	MemoryPressureThresholdPct float64       `env:"MEMORY_PRESSURE_THRESHOLD_PCT" default:"0"`
	JobCacheTTL                time.Duration `env:"JOB_CACHE_TTL" default:"5m"`
	DefaultRunTTLSecs          int           `env:"DEFAULT_RUN_TTL_SECS" default:"0"`
	MaxResultSize              int64         `env:"MAX_RESULT_SIZE" default:"1048576"`
	MigrationMode              string        `env:"MIGRATION_MODE" default:"auto"`
	MigrationLockTimeout       time.Duration `env:"MIGRATION_LOCK_TIMEOUT" default:"30s"`
	MaxSnoozeCount             int           `env:"MAX_SNOOZE_COUNT" default:"50"`
	DebouncePollerInterval     time.Duration `env:"DEBOUNCE_POLLER_INTERVAL" default:"1s"`
	BatchFlushInterval         time.Duration `env:"BATCH_FLUSH_INTERVAL" default:"1s"`
	WebhookRequireTLS          bool          `env:"WEBHOOK_REQUIRE_TLS" default:"false"`
	AllowPrivateEndpoints      bool          `env:"ALLOW_PRIVATE_ENDPOINTS" default:"false"`
	DequeueStrategy            string        `env:"DEQUEUE_STRATEGY" default:"priority"`

	// Managed execution (container runtime)
	ComputeRuntime        string        `env:"COMPUTE_RUNTIME" default:"none"`
	FlyAPIToken           string        `env:"FLY_API_TOKEN"`
	FlyAppName            string        `env:"FLY_APP_NAME"`
	FlyRegion             string        `env:"FLY_REGION" default:"iad"`
	ExternalAPIURL        string        `env:"EXTERNAL_API_URL"`
	MaxConcurrentMachines int           `env:"MAX_CONCURRENT_MACHINES" default:"10"`
	WarmPoolEnabled       bool          `env:"WARM_POOL_ENABLED" default:"false"`
	WarmPoolMaxPerJob     int           `env:"WARM_POOL_MAX_PER_JOB" default:"0"`
	WarmPoolTTL           time.Duration `env:"WARM_POOL_TTL"`

	// Region gating
	EnforceRegionGating bool `env:"ENFORCE_REGION_GATING" default:"false"`

	// ClickHouse (optional analytics)
	ClickHouseEnabled       bool          `env:"CLICKHOUSE_ENABLED" default:"false"`
	ClickHouseURL           string        `env:"CLICKHOUSE_URL"`
	ClickHouseDatabase      string        `env:"CLICKHOUSE_DATABASE" default:"strait"`
	ClickHouseBatchSize     int           `env:"CLICKHOUSE_BATCH_SIZE" default:"1000"`
	ClickHouseFlushInterval time.Duration `env:"CLICKHOUSE_FLUSH_INTERVAL" default:"5s"`
	ClickHouseExportEnabled bool          `env:"CLICKHOUSE_EXPORT_ENABLED" default:"false"`

	// OTel metrics push
	OTLPMetricEndpoint string `env:"OTLP_METRIC_ENDPOINT"`
	OTLPMetricEnabled  bool   `env:"OTLP_METRIC_ENABLED" default:"false"`

	// Polar billing integration
	PolarAccessToken          string `env:"POLAR_ACCESS_TOKEN"`
	PolarWebhookSecret        string `env:"POLAR_WEBHOOK_SECRET"`
	PolarServer               string `env:"POLAR_SERVER"`
	PolarStarterMonthlyID     string `env:"POLAR_STARTER_MONTHLY_ID"`
	PolarStarterYearlyID      string `env:"POLAR_STARTER_YEARLY_ID"`
	PolarProMonthlyID         string `env:"POLAR_PRO_MONTHLY_ID"`
	PolarProYearlyID          string `env:"POLAR_PRO_YEARLY_ID"`
	BillingEnforcementEnabled bool   `env:"BILLING_ENFORCEMENT_ENABLED" default:"false"`

	// Resend email integration
	ResendAPIKey    string `env:"RESEND_API_KEY"`
	ResendFromEmail string `env:"RESEND_FROM_EMAIL" default:"noreply@strait.dev"`

	// Sentry error tracking
	SentryDSN         string `env:"SENTRY_DSN"`
	SentryEnvironment string `env:"SENTRY_ENVIRONMENT" default:"development"`

	// Edition controls feature gating (community vs cloud)
	Edition string `env:"STRAIT_EDITION" default:"community"`
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	var cfg Config

	loader := aconfig.LoaderFor(&cfg, aconfig.Config{
		SkipFiles: true,
		SkipFlags: true,
	})
	if err := loader.Load(); err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Post-load: parse ENCRYPTION_KEY_OLD with whitespace trimming.
	cfg.EncryptionKeyOld = parseCSVEnv("ENCRYPTION_KEY_OLD")

	// CDC fallback to Sequin settings when CDC-specific vars are not set.
	if os.Getenv("CDC_BATCH_SIZE") == "" && os.Getenv("SEQUIN_BATCH_SIZE") != "" {
		cfg.CDCBatchSize = cfg.SequinBatchSize
	}
	if os.Getenv("CDC_WAIT_TIME_MS") == "" && os.Getenv("SEQUIN_WAIT_TIME_MS") != "" {
		cfg.CDCWaitTimeMs = cfg.SequinWaitTimeMs
	}

	// Legacy: support EVENT_TRIGGER_RETENTION_DAYS as days -> duration.
	if cfg.EventTriggerRetention == 0 && cfg.EventTriggerRetentionDays > 0 {
		cfg.EventTriggerRetention = time.Duration(cfg.EventTriggerRetentionDays) * 24 * time.Hour
	}

	// Encryption key mirroring.
	if cfg.EncryptionKey == "" {
		cfg.EncryptionKey = cfg.SecretEncryptionKey
	}
	if cfg.SecretEncryptionKey == "" {
		cfg.SecretEncryptionKey = cfg.EncryptionKey
	}

	// Validation.
	if cfg.DatabaseURL == "" {
		return nil, &domain.ConfigError{Field: "DATABASE_URL", Message: "is required"}
	}
	if cfg.InternalSecret == "" {
		return nil, &domain.ConfigError{Field: "INTERNAL_SECRET", Message: "is required"}
	}
	if len(cfg.JWTSigningKey) < 32 {
		return nil, &domain.ConfigError{Field: "JWT_SIGNING_KEY", Message: "must be at least 32 characters"}
	}
	if cfg.OIDCEnabled {
		if cfg.OIDCIssuer == "" {
			return nil, &domain.ConfigError{Field: "OIDC_ISSUER", Message: "is required when OIDC is enabled"}
		}
		if cfg.OIDCAudience == "" {
			return nil, &domain.ConfigError{Field: "OIDC_AUDIENCE", Message: "is required when OIDC is enabled"}
		}
		if cfg.OIDCPublicKeyPEM == "" {
			return nil, &domain.ConfigError{Field: "OIDC_PUBLIC_KEY_PEM", Message: "is required when OIDC is enabled"}
		}
	}

	if cfg.RedisURL != "" {
		if _, err := url.Parse(cfg.RedisURL); err != nil {
			return nil, &domain.ConfigError{Field: "REDIS_URL", Message: fmt.Sprintf("invalid URL: %v", err)}
		}
	}

	switch cfg.MigrationMode {
	case "auto", "manual", "validate":
		// valid
	default:
		return nil, &domain.ConfigError{Field: "MIGRATION_MODE", Message: "must be auto, manual, or validate"}
	}

	if strings.Contains(cfg.DatabaseURL, "sslmode=disable") {
		slog.Warn("DATABASE_URL has sslmode=disable; connections are not encrypted")
	}

	if cfg.SequinBaseURL != "" {
		if u, err := url.Parse(cfg.SequinBaseURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") {
			return nil, &domain.ConfigError{Field: "SEQUIN_BASE_URL", Message: "must be a valid HTTP(S) URL"}
		}
	}

	switch cfg.ComputeRuntime {
	case "none", "fly", "docker", "":
		// valid
	default:
		return nil, &domain.ConfigError{Field: "COMPUTE_RUNTIME", Message: "must be none, fly, or docker"}
	}
	if cfg.ComputeRuntime == "fly" {
		if cfg.FlyAPIToken == "" {
			return nil, &domain.ConfigError{Field: "FLY_API_TOKEN", Message: "is required when COMPUTE_RUNTIME=fly"}
		}
		if cfg.FlyAppName == "" {
			return nil, &domain.ConfigError{Field: "FLY_APP_NAME", Message: "is required when COMPUTE_RUNTIME=fly"}
		}
	}
	if cfg.Edition == string(domain.EditionCommunity) && cfg.ComputeRuntime != "none" && cfg.ComputeRuntime != "" {
		slog.Warn("community edition does not support managed execution; overriding COMPUTE_RUNTIME to none",
			"configured", cfg.ComputeRuntime)
		cfg.ComputeRuntime = "none"
	}

	if cfg.ClickHouseEnabled && cfg.ClickHouseURL == "" {
		return nil, &domain.ConfigError{Field: "CLICKHOUSE_URL", Message: "is required when CLICKHOUSE_ENABLED=true"}
	}

	slog.Info("config loaded",
		"mode", cfg.Mode,
		"port", cfg.Port,
		"worker_concurrency", cfg.WorkerConcurrency,
		"poll_interval", cfg.PollerInterval,
		"db_max_conns", cfg.DBMaxConns,
	)

	return &cfg, nil
}

func parseCSVEnv(key string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return []string{}
	}

	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			values = append(values, trimmed)
		}
	}

	return values
}
