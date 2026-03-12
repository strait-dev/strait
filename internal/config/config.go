package config

import (
	"fmt"
	"strings"
	"time"

	"strait/internal/domain"

	"github.com/spf13/viper"
)

type Config struct {
	DatabaseURL               string        `mapstructure:"DATABASE_URL"`
	RedisURL                  string        `mapstructure:"REDIS_URL"`
	RedisSentinelMaster       string        `mapstructure:"REDIS_SENTINEL_MASTER"`
	RedisSentinelAddrs        []string      `mapstructure:"REDIS_SENTINEL_ADDRS"`
	Mode                      string        `mapstructure:"MODE"`
	Port                      int           `mapstructure:"PORT"`
	WorkerConcurrency         int           `mapstructure:"WORKER_CONCURRENCY"`
	InternalSecret            string        `mapstructure:"INTERNAL_SECRET"`
	JWTSigningKey             string        `mapstructure:"JWT_SIGNING_KEY"`
	SecretEncryptionKey       string        `mapstructure:"SECRET_ENCRYPTION_KEY"`
	EncryptionKey             string        `mapstructure:"ENCRYPTION_KEY"`
	EncryptionKeyOld          []string      `mapstructure:"ENCRYPTION_KEY_OLD"`
	OIDCEnabled               bool          `mapstructure:"OIDC_ENABLED"`
	OIDCIssuer                string        `mapstructure:"OIDC_ISSUER"`
	OIDCAudience              string        `mapstructure:"OIDC_AUDIENCE"`
	OIDCPublicKeyPEM          string        `mapstructure:"OIDC_PUBLIC_KEY_PEM"`
	LogLevel                  string        `mapstructure:"LOG_LEVEL"`
	HeartbeatInterval         time.Duration `mapstructure:"HEARTBEAT_INTERVAL"`
	ReaperInterval            time.Duration `mapstructure:"REAPER_INTERVAL"`
	StaleThreshold            time.Duration `mapstructure:"STALE_THRESHOLD"`
	PollerInterval            time.Duration `mapstructure:"POLLER_INTERVAL"`
	RunRetentionShort         time.Duration `mapstructure:"RUN_RETENTION_SHORT"`
	RunRetentionLong          time.Duration `mapstructure:"RUN_RETENTION_LONG"`
	OTELEndpoint              string        `mapstructure:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	WorkflowRunRetentionDays  int           `mapstructure:"WORKFLOW_RUN_RETENTION_DAYS"`
	EventTriggerRetentionDays int           `mapstructure:"EVENT_TRIGGER_RETENTION_DAYS"`

	// Database connection pool tuning
	DBMaxConns        int32         `mapstructure:"DB_MAX_CONNS"`
	DBMinConns        int32         `mapstructure:"DB_MIN_CONNS"`
	DBMaxConnLifetime time.Duration `mapstructure:"DB_MAX_CONN_LIFETIME"`
	DBMaxConnIdleTime time.Duration `mapstructure:"DB_MAX_CONN_IDLE_TIME"`

	RateLimitRequests int           `mapstructure:"RATE_LIMIT_REQUESTS"`
	RateLimitWindow   time.Duration `mapstructure:"RATE_LIMIT_WINDOW"`

	TriggerRateLimitRequests int           `mapstructure:"TRIGGER_RATE_LIMIT_REQUESTS"`
	TriggerRateLimitWindow   time.Duration `mapstructure:"TRIGGER_RATE_LIMIT_WINDOW"`

	RequestTimeout     time.Duration `mapstructure:"REQUEST_TIMEOUT"`
	MaxRequestBodySize int64         `mapstructure:"MAX_REQUEST_BODY_SIZE"`

	// Sequin CDC settings
	SequinBaseURL      string `mapstructure:"SEQUIN_BASE_URL"`
	SequinConsumerName string `mapstructure:"SEQUIN_CONSUMER_NAME"`
	SequinAPIToken     string `mapstructure:"SEQUIN_API_TOKEN"`
	SequinBatchSize    int    `mapstructure:"SEQUIN_BATCH_SIZE"`
	SequinWaitTimeMs   int    `mapstructure:"SEQUIN_WAIT_TIME_MS"`

	// CORS settings
	CORSAllowedOrigins   []string `mapstructure:"CORS_ALLOWED_ORIGINS"`
	CORSAllowCredentials bool     `mapstructure:"CORS_ALLOW_CREDENTIALS"`

	WorkerPartitions       []string `mapstructure:"WORKER_PARTITIONS"`
	WorkerPartitionWeights string   `mapstructure:"WORKER_PARTITION_WEIGHTS"`
	AdaptiveConcurrencyMin int      `mapstructure:"ADAPTIVE_CONCURRENCY_MIN"`
	AdaptiveConcurrencyMax int      `mapstructure:"ADAPTIVE_CONCURRENCY_MAX"`
	DBPgBouncerMode        bool     `mapstructure:"DB_PGBOUNCER_MODE"`

	WorkerDrainTimeout time.Duration `mapstructure:"WORKER_DRAIN_TIMEOUT"`

	// RBAC permission cache
	PermissionCacheTTL time.Duration `mapstructure:"PERMISSION_CACHE_TTL"`

	// Worker/Executor timeouts
	WebhookTimeout          time.Duration `mapstructure:"WEBHOOK_TIMEOUT"`
	WebhookIdleConnTimeout  time.Duration `mapstructure:"WEBHOOK_IDLE_CONN_TIMEOUT"`
	ExecutorHTTPTimeout     time.Duration `mapstructure:"EXECUTOR_HTTP_TIMEOUT"`
	ExecutorIdleConnTimeout time.Duration `mapstructure:"EXECUTOR_IDLE_CONN_TIMEOUT"`
	WebhookDispatchTimeout  time.Duration `mapstructure:"WEBHOOK_DISPATCH_TIMEOUT"`
	WebhookMaxPayloadBytes  int64         `mapstructure:"WEBHOOK_MAX_PAYLOAD_BYTES"`

	// Worker settings
	WebhookMaxAttempts    int `mapstructure:"WEBHOOK_MAX_ATTEMPTS"`
	DefaultJobMaxAttempts int `mapstructure:"DEFAULT_JOB_MAX_ATTEMPTS"`
	DefaultJobTimeoutSecs int `mapstructure:"DEFAULT_JOB_TIMEOUT_SECS"`
	WorkerQueueSize       int `mapstructure:"WORKER_QUEUE_SIZE"`

	// Scheduler settings
	WorkflowRetention        time.Duration `mapstructure:"WORKFLOW_RETENTION"`
	EventTriggerRetention    time.Duration `mapstructure:"EVENT_TRIGGER_RETENTION"`
	IndexMaintenanceInterval time.Duration `mapstructure:"INDEX_MAINTENANCE_INTERVAL"`
	ReaperDeleteBatchSize    int           `mapstructure:"REAPER_DELETE_BATCH_SIZE"`
	StalledWorkflowThreshold time.Duration `mapstructure:"WF_STALL_THRESHOLD"`
	StalledWorkflowAction    string        `mapstructure:"WF_STALL_ACTION"`
	WfMaxStepCap             int           `mapstructure:"WF_MAX_STEP_CAP"`
	WfStepConcurrencyLimit   int           `mapstructure:"WF_STEP_CONCURRENCY_LIMIT"`
	DependencyStatusCacheTTL time.Duration `mapstructure:"DEPENDENCY_STATUS_CACHE_TTL"`

	// Workflow settings
	MaxWorkflowNestingDepth int `mapstructure:"MAX_WORKFLOW_NESTING_DEPTH"`

	CDCBatchSize  int `mapstructure:"CDC_BATCH_SIZE"`
	CDCWaitTimeMs int `mapstructure:"CDC_WAIT_TIME_MS"`

	// SSE settings
	SSEKeepaliveInterval time.Duration `mapstructure:"SSE_KEEPALIVE_INTERVAL"`
}

func setDefaults() {
	viper.SetDefault("MODE", "all")
	viper.SetDefault("PORT", 8080)
	viper.SetDefault("WORKER_CONCURRENCY", 10)
	viper.SetDefault("LOG_LEVEL", "info")
	viper.SetDefault("HEARTBEAT_INTERVAL", 10*time.Second)
	viper.SetDefault("REAPER_INTERVAL", 30*time.Second)
	viper.SetDefault("STALE_THRESHOLD", 60*time.Second)
	viper.SetDefault("POLLER_INTERVAL", 5*time.Second)
	viper.SetDefault("WORKFLOW_RUN_RETENTION_DAYS", 30)
	viper.SetDefault("RUN_RETENTION_SHORT", 30*24*time.Hour)
	viper.SetDefault("RUN_RETENTION_LONG", 90*24*time.Hour)
	viper.SetDefault("DB_MAX_CONNS", 25)
	viper.SetDefault("DB_MIN_CONNS", 5)
	viper.SetDefault("DB_MAX_CONN_LIFETIME", 30*time.Minute)
	viper.SetDefault("DB_MAX_CONN_IDLE_TIME", 5*time.Minute)
	viper.SetDefault("RATE_LIMIT_REQUESTS", 100)
	viper.SetDefault("RATE_LIMIT_WINDOW", time.Minute)
	viper.SetDefault("TRIGGER_RATE_LIMIT_REQUESTS", 10)
	viper.SetDefault("TRIGGER_RATE_LIMIT_WINDOW", time.Minute)
	viper.SetDefault("REQUEST_TIMEOUT", 30*time.Second)
	viper.SetDefault("MAX_REQUEST_BODY_SIZE", int64(1<<20))
	viper.SetDefault("OIDC_ENABLED", false)
	viper.SetDefault("OIDC_ISSUER", "")
	viper.SetDefault("OIDC_AUDIENCE", "")
	viper.SetDefault("OIDC_PUBLIC_KEY_PEM", "")
	viper.SetDefault("SEQUIN_BATCH_SIZE", 10)
	viper.SetDefault("SEQUIN_WAIT_TIME_MS", 5000)
	viper.SetDefault("CORS_ALLOWED_ORIGINS", []string{"*"})
	viper.SetDefault("CORS_ALLOW_CREDENTIALS", false)
	viper.SetDefault("WORKER_PARTITIONS", []string{})
	viper.SetDefault("WORKER_PARTITION_WEIGHTS", "")
	viper.SetDefault("ADAPTIVE_CONCURRENCY_MIN", 5)
	viper.SetDefault("ADAPTIVE_CONCURRENCY_MAX", 100)
	viper.SetDefault("DB_PGBOUNCER_MODE", false)
	viper.SetDefault("WORKER_DRAIN_TIMEOUT", 30*time.Second)
	viper.SetDefault("PERMISSION_CACHE_TTL", 5*time.Minute)
	viper.SetDefault("SECRET_ENCRYPTION_KEY", "")
	viper.SetDefault("ENCRYPTION_KEY", "")
	viper.SetDefault("ENCRYPTION_KEY_OLD", []string{})
	viper.SetDefault("WEBHOOK_TIMEOUT", 10*time.Second)
	viper.SetDefault("WEBHOOK_IDLE_CONN_TIMEOUT", 60*time.Second)
	viper.SetDefault("EXECUTOR_HTTP_TIMEOUT", 5*time.Minute)
	viper.SetDefault("EXECUTOR_IDLE_CONN_TIMEOUT", 90*time.Second)
	viper.SetDefault("WEBHOOK_DISPATCH_TIMEOUT", 15*time.Second)
	viper.SetDefault("WEBHOOK_MAX_PAYLOAD_BYTES", int64(1<<20))
	viper.SetDefault("WEBHOOK_MAX_ATTEMPTS", 3)
	viper.SetDefault("DEFAULT_JOB_MAX_ATTEMPTS", 3)
	viper.SetDefault("DEFAULT_JOB_TIMEOUT_SECS", 300)
	viper.SetDefault("WORKER_QUEUE_SIZE", 0)
	viper.SetDefault("WORKFLOW_RETENTION", 30*24*time.Hour)
	viper.SetDefault("INDEX_MAINTENANCE_INTERVAL", 24*time.Hour)
	viper.SetDefault("REAPER_DELETE_BATCH_SIZE", 100)
	viper.SetDefault("WF_STALL_THRESHOLD", 15*time.Minute)
	viper.SetDefault("WF_STALL_ACTION", "log_only")
	viper.SetDefault("WF_MAX_STEP_CAP", 0)
	viper.SetDefault("WF_STEP_CONCURRENCY_LIMIT", 0)
	viper.SetDefault("DEPENDENCY_STATUS_CACHE_TTL", 5*time.Second)
	viper.SetDefault("MAX_WORKFLOW_NESTING_DEPTH", 10)
	viper.SetDefault("CDC_BATCH_SIZE", 10)
	viper.SetDefault("CDC_WAIT_TIME_MS", 5000)
	viper.SetDefault("SSE_KEEPALIVE_INTERVAL", 15*time.Second)
}

func BindEnv() error {
	keys := []string{
		"DATABASE_URL", "REDIS_URL", "REDIS_SENTINEL_MASTER", "REDIS_SENTINEL_ADDRS",
		"MODE", "PORT", "WORKER_CONCURRENCY", "INTERNAL_SECRET", "JWT_SIGNING_KEY",
		"SECRET_ENCRYPTION_KEY", "ENCRYPTION_KEY", "ENCRYPTION_KEY_OLD", "LOG_LEVEL", "HEARTBEAT_INTERVAL", "REAPER_INTERVAL",
		"STALE_THRESHOLD", "POLLER_INTERVAL", "RUN_RETENTION_SHORT", "RUN_RETENTION_LONG",
		"OTEL_EXPORTER_OTLP_ENDPOINT", "WORKFLOW_RUN_RETENTION_DAYS", "DB_MAX_CONNS",
		"DB_MIN_CONNS", "DB_MAX_CONN_LIFETIME", "DB_MAX_CONN_IDLE_TIME", "RATE_LIMIT_REQUESTS",
		"RATE_LIMIT_WINDOW", "TRIGGER_RATE_LIMIT_REQUESTS", "TRIGGER_RATE_LIMIT_WINDOW",
		"REQUEST_TIMEOUT", "MAX_REQUEST_BODY_SIZE", "SEQUIN_BASE_URL", "SEQUIN_CONSUMER_NAME",
		"SEQUIN_API_TOKEN", "SEQUIN_BATCH_SIZE", "SEQUIN_WAIT_TIME_MS", "CORS_ALLOWED_ORIGINS",
		"CORS_ALLOW_CREDENTIALS", "WORKER_PARTITIONS", "WORKER_PARTITION_WEIGHTS",
		"ADAPTIVE_CONCURRENCY_MIN", "ADAPTIVE_CONCURRENCY_MAX", "DB_PGBOUNCER_MODE",
		"WORKER_DRAIN_TIMEOUT",
		"WEBHOOK_TIMEOUT", "WEBHOOK_IDLE_CONN_TIMEOUT", "EXECUTOR_HTTP_TIMEOUT",
		"EXECUTOR_IDLE_CONN_TIMEOUT", "WEBHOOK_DISPATCH_TIMEOUT", "WEBHOOK_MAX_PAYLOAD_BYTES", "WEBHOOK_MAX_ATTEMPTS",
		"DEFAULT_JOB_MAX_ATTEMPTS", "DEFAULT_JOB_TIMEOUT_SECS", "WORKER_QUEUE_SIZE",
		"WORKFLOW_RETENTION", "INDEX_MAINTENANCE_INTERVAL", "REAPER_DELETE_BATCH_SIZE",
		"WF_STALL_THRESHOLD", "WF_STALL_ACTION", "WF_MAX_STEP_CAP", "WF_STEP_CONCURRENCY_LIMIT",
		"DEPENDENCY_STATUS_CACHE_TTL", "MAX_WORKFLOW_NESTING_DEPTH", "CDC_BATCH_SIZE",
		"CDC_WAIT_TIME_MS", "SSE_KEEPALIVE_INTERVAL",
		"PERMISSION_CACHE_TTL",
		"EVENT_TRIGGER_RETENTION", "EVENT_TRIGGER_RETENTION_DAYS",
	}

	for _, key := range keys {
		if err := viper.BindEnv(key); err != nil {
			return fmt.Errorf("bind env %s: %w", key, err)
		}
	}

	return nil
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	setDefaults()
	if err := BindEnv(); err != nil {
		return nil, err
	}

	viper.AutomaticEnv()

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	cfg.HeartbeatInterval = viper.GetDuration("HEARTBEAT_INTERVAL")
	cfg.ReaperInterval = viper.GetDuration("REAPER_INTERVAL")
	cfg.StaleThreshold = viper.GetDuration("STALE_THRESHOLD")
	cfg.PollerInterval = viper.GetDuration("POLLER_INTERVAL")
	cfg.RunRetentionShort = viper.GetDuration("RUN_RETENTION_SHORT")
	cfg.RunRetentionLong = viper.GetDuration("RUN_RETENTION_LONG")
	cfg.DBMaxConnLifetime = viper.GetDuration("DB_MAX_CONN_LIFETIME")
	cfg.DBMaxConnIdleTime = viper.GetDuration("DB_MAX_CONN_IDLE_TIME")
	cfg.DBMaxConns = viper.GetInt32("DB_MAX_CONNS")
	cfg.DBMinConns = viper.GetInt32("DB_MIN_CONNS")
	cfg.RateLimitWindow = viper.GetDuration("RATE_LIMIT_WINDOW")
	cfg.TriggerRateLimitWindow = viper.GetDuration("TRIGGER_RATE_LIMIT_WINDOW")
	cfg.RequestTimeout = viper.GetDuration("REQUEST_TIMEOUT")
	cfg.MaxRequestBodySize = viper.GetInt64("MAX_REQUEST_BODY_SIZE")
	cfg.OIDCEnabled = viper.GetBool("OIDC_ENABLED")
	cfg.OIDCIssuer = viper.GetString("OIDC_ISSUER")
	cfg.OIDCAudience = viper.GetString("OIDC_AUDIENCE")
	cfg.OIDCPublicKeyPEM = viper.GetString("OIDC_PUBLIC_KEY_PEM")
	cfg.CORSAllowedOrigins = viper.GetStringSlice("CORS_ALLOWED_ORIGINS")
	cfg.CORSAllowCredentials = viper.GetBool("CORS_ALLOW_CREDENTIALS")
	cfg.RedisSentinelAddrs = viper.GetStringSlice("REDIS_SENTINEL_ADDRS")
	cfg.EncryptionKeyOld = parseCSVEnv("ENCRYPTION_KEY_OLD")
	cfg.WorkflowRunRetentionDays = viper.GetInt("WORKFLOW_RUN_RETENTION_DAYS")
	cfg.WorkerPartitions = viper.GetStringSlice("WORKER_PARTITIONS")
	cfg.WorkerPartitionWeights = viper.GetString("WORKER_PARTITION_WEIGHTS")
	cfg.WebhookTimeout = viper.GetDuration("WEBHOOK_TIMEOUT")
	cfg.WebhookIdleConnTimeout = viper.GetDuration("WEBHOOK_IDLE_CONN_TIMEOUT")
	cfg.ExecutorHTTPTimeout = viper.GetDuration("EXECUTOR_HTTP_TIMEOUT")
	cfg.ExecutorIdleConnTimeout = viper.GetDuration("EXECUTOR_IDLE_CONN_TIMEOUT")
	cfg.WebhookDispatchTimeout = viper.GetDuration("WEBHOOK_DISPATCH_TIMEOUT")
	cfg.WebhookMaxPayloadBytes = viper.GetInt64("WEBHOOK_MAX_PAYLOAD_BYTES")
	cfg.WorkflowRetention = viper.GetDuration("WORKFLOW_RETENTION")
	cfg.EventTriggerRetention = viper.GetDuration("EVENT_TRIGGER_RETENTION")
	cfg.IndexMaintenanceInterval = viper.GetDuration("INDEX_MAINTENANCE_INTERVAL")
	cfg.StalledWorkflowThreshold = viper.GetDuration("WF_STALL_THRESHOLD")
	cfg.StalledWorkflowAction = viper.GetString("WF_STALL_ACTION")
	cfg.WfMaxStepCap = viper.GetInt("WF_MAX_STEP_CAP")
	cfg.WfStepConcurrencyLimit = viper.GetInt("WF_STEP_CONCURRENCY_LIMIT")
	cfg.DependencyStatusCacheTTL = viper.GetDuration("DEPENDENCY_STATUS_CACHE_TTL")
	// Legacy: support EVENT_TRIGGER_RETENTION_DAYS as days → duration.
	if cfg.EventTriggerRetention == 0 && cfg.EventTriggerRetentionDays > 0 {
		cfg.EventTriggerRetention = time.Duration(cfg.EventTriggerRetentionDays) * 24 * time.Hour
	}
	cfg.CDCBatchSize = viper.GetInt("CDC_BATCH_SIZE")
	cfg.CDCWaitTimeMs = viper.GetInt("CDC_WAIT_TIME_MS")
	cfg.SSEKeepaliveInterval = viper.GetDuration("SSE_KEEPALIVE_INTERVAL")
	cfg.WorkerDrainTimeout = viper.GetDuration("WORKER_DRAIN_TIMEOUT")

	if !viper.IsSet("CDC_BATCH_SIZE") && viper.IsSet("SEQUIN_BATCH_SIZE") {
		cfg.CDCBatchSize = viper.GetInt("SEQUIN_BATCH_SIZE")
	}
	if !viper.IsSet("CDC_WAIT_TIME_MS") && viper.IsSet("SEQUIN_WAIT_TIME_MS") {
		cfg.CDCWaitTimeMs = viper.GetInt("SEQUIN_WAIT_TIME_MS")
	}

	if cfg.EncryptionKey == "" {
		cfg.EncryptionKey = cfg.SecretEncryptionKey
	}
	if cfg.SecretEncryptionKey == "" {
		cfg.SecretEncryptionKey = cfg.EncryptionKey
	}

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

	return &cfg, nil
}

func parseCSVEnv(key string) []string {
	raw := strings.TrimSpace(viper.GetString(key))
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
