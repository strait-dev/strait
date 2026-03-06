package config

import (
	"fmt"
	"time"

	"orchestrator/internal/domain"

	"github.com/spf13/viper"
)

type Config struct {
	DatabaseURL         string        `mapstructure:"DATABASE_URL"`
	RedisURL            string        `mapstructure:"REDIS_URL"`
	RedisSentinelMaster string        `mapstructure:"REDIS_SENTINEL_MASTER"`
	RedisSentinelAddrs  []string      `mapstructure:"REDIS_SENTINEL_ADDRS"`
	Mode                string        `mapstructure:"MODE"`
	Port                int           `mapstructure:"PORT"`
	WorkerConcurrency   int           `mapstructure:"WORKER_CONCURRENCY"`
	InternalSecret      string        `mapstructure:"INTERNAL_SECRET"`
	JWTSigningKey       string        `mapstructure:"JWT_SIGNING_KEY"`
	SecretEncryptionKey string        `mapstructure:"SECRET_ENCRYPTION_KEY"`
	LogLevel            string        `mapstructure:"LOG_LEVEL"`
	HeartbeatInterval   time.Duration `mapstructure:"HEARTBEAT_INTERVAL"`
	ReaperInterval      time.Duration `mapstructure:"REAPER_INTERVAL"`
	StaleThreshold      time.Duration `mapstructure:"STALE_THRESHOLD"`
	PollerInterval      time.Duration `mapstructure:"POLLER_INTERVAL"`
	RunRetentionShort   time.Duration `mapstructure:"RUN_RETENTION_SHORT"`
	RunRetentionLong    time.Duration `mapstructure:"RUN_RETENTION_LONG"`
	OTELEndpoint        string        `mapstructure:"OTEL_EXPORTER_OTLP_ENDPOINT"`

	// Database connection pool tuning
	DBMaxConns        int32         `mapstructure:"DB_MAX_CONNS"`
	DBMinConns        int32         `mapstructure:"DB_MIN_CONNS"`
	DBMaxConnLifetime time.Duration `mapstructure:"DB_MAX_CONN_LIFETIME"`
	DBMaxConnIdleTime time.Duration `mapstructure:"DB_MAX_CONN_IDLE_TIME"`

	RateLimitRequests int           `mapstructure:"RATE_LIMIT_REQUESTS"`
	RateLimitWindow   time.Duration `mapstructure:"RATE_LIMIT_WINDOW"`

	TriggerRateLimitRequests int           `mapstructure:"TRIGGER_RATE_LIMIT_REQUESTS"`
	TriggerRateLimitWindow   time.Duration `mapstructure:"TRIGGER_RATE_LIMIT_WINDOW"`

	// Sequin CDC settings
	SequinBaseURL      string `mapstructure:"SEQUIN_BASE_URL"`
	SequinConsumerName string `mapstructure:"SEQUIN_CONSUMER_NAME"`
	SequinAPIToken     string `mapstructure:"SEQUIN_API_TOKEN"`
	SequinBatchSize    int    `mapstructure:"SEQUIN_BATCH_SIZE"`
	SequinWaitTimeMs   int    `mapstructure:"SEQUIN_WAIT_TIME_MS"`

	// CORS settings
	CORSAllowedOrigins   []string `mapstructure:"CORS_ALLOWED_ORIGINS"`
	CORSAllowCredentials bool     `mapstructure:"CORS_ALLOW_CREDENTIALS"`

	FFConcurrencyLimits bool `mapstructure:"FF_CONCURRENCY_LIMITS"`
	FFProjectQuotas     bool `mapstructure:"FF_PROJECT_QUOTAS"`
	FFExecutionWindows  bool `mapstructure:"FF_EXECUTION_WINDOWS"`
	FFQueuePartitioning bool `mapstructure:"FF_QUEUE_PARTITIONING"`

	WorkerPartitions       []string `mapstructure:"WORKER_PARTITIONS"`
	WorkerPartitionWeights string   `mapstructure:"WORKER_PARTITION_WEIGHTS"`

	FFProgressStreaming bool `mapstructure:"FF_PROGRESS_STREAMING"`
	FFCheckpoints       bool `mapstructure:"FF_CHECKPOINTS"`
	FFRunContinuation   bool `mapstructure:"FF_RUN_CONTINUATION"`
	FFUsageTracking     bool `mapstructure:"FF_USAGE_TRACKING"`
	FFCostBudgets       bool `mapstructure:"FF_COST_BUDGETS"`

	FFErrorClassification bool `mapstructure:"FF_ERROR_CLASSIFICATION"`
	FFSmartRetry          bool `mapstructure:"FF_SMART_RETRY"`
	FFCircuitBreaker      bool `mapstructure:"FF_CIRCUIT_BREAKER"`
	FFBulkheads           bool `mapstructure:"FF_BULKHEADS"`
	FFRunDLQ              bool `mapstructure:"FF_RUN_DLQ"`

	FFPayloadValidation bool `mapstructure:"FF_PAYLOAD_VALIDATION"`
	FFJobTags           bool `mapstructure:"FF_JOB_TAGS"`
	FFRunAnnotations    bool `mapstructure:"FF_RUN_ANNOTATIONS"`
	FFSecretInjection   bool `mapstructure:"FF_SECRET_INJECTION"`
	FFRunReplay         bool `mapstructure:"FF_RUN_REPLAY"`
	FFDryRun            bool `mapstructure:"FF_DRY_RUN"`

	FFRunRetention     bool `mapstructure:"FF_RUN_RETENTION"`
	FFExecutionTracing bool `mapstructure:"FF_EXECUTION_TRACING"`
	FFDebugBundle      bool `mapstructure:"FF_DEBUG_BUNDLE"`
	FFBatchJobOps      bool `mapstructure:"FF_BATCH_JOB_OPS"`
	FFEnvironments     bool `mapstructure:"FF_ENVIRONMENTS"`
	FFJobGroups        bool `mapstructure:"FF_JOB_GROUPS"`
	FFJobDependencies  bool `mapstructure:"FF_JOB_DEPENDENCIES"`
	FFJobHealthScoring bool `mapstructure:"FF_JOB_HEALTH_SCORING"`
}

func Load() (*Config, error) {
	viper.SetDefault("MODE", "all")
	viper.SetDefault("PORT", 8080)
	viper.SetDefault("WORKER_CONCURRENCY", 10)
	viper.SetDefault("LOG_LEVEL", "info")
	viper.SetDefault("HEARTBEAT_INTERVAL", 10*time.Second)
	viper.SetDefault("REAPER_INTERVAL", 30*time.Second)
	viper.SetDefault("STALE_THRESHOLD", 60*time.Second)
	viper.SetDefault("POLLER_INTERVAL", 5*time.Second)
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
	viper.SetDefault("SEQUIN_BATCH_SIZE", 10)
	viper.SetDefault("SEQUIN_WAIT_TIME_MS", 5000)
	viper.SetDefault("CORS_ALLOWED_ORIGINS", []string{"*"})
	viper.SetDefault("CORS_ALLOW_CREDENTIALS", false)
	viper.SetDefault("FF_CONCURRENCY_LIMITS", false)
	viper.SetDefault("FF_PROJECT_QUOTAS", false)
	viper.SetDefault("FF_EXECUTION_WINDOWS", false)
	viper.SetDefault("FF_QUEUE_PARTITIONING", false)
	viper.SetDefault("WORKER_PARTITIONS", []string{})
	viper.SetDefault("WORKER_PARTITION_WEIGHTS", "")
	viper.SetDefault("FF_PROGRESS_STREAMING", false)
	viper.SetDefault("FF_CHECKPOINTS", false)
	viper.SetDefault("FF_RUN_CONTINUATION", false)
	viper.SetDefault("FF_USAGE_TRACKING", false)
	viper.SetDefault("FF_COST_BUDGETS", false)
	viper.SetDefault("FF_ERROR_CLASSIFICATION", false)
	viper.SetDefault("FF_SMART_RETRY", false)
	viper.SetDefault("FF_CIRCUIT_BREAKER", false)
	viper.SetDefault("FF_BULKHEADS", false)
	viper.SetDefault("FF_RUN_DLQ", false)
	viper.SetDefault("FF_PAYLOAD_VALIDATION", false)
	viper.SetDefault("FF_JOB_TAGS", false)
	viper.SetDefault("FF_RUN_ANNOTATIONS", false)
	viper.SetDefault("FF_SECRET_INJECTION", false)
	viper.SetDefault("FF_RUN_REPLAY", false)
	viper.SetDefault("FF_DRY_RUN", false)
	viper.SetDefault("FF_RUN_RETENTION", false)
	viper.SetDefault("FF_EXECUTION_TRACING", false)
	viper.SetDefault("FF_DEBUG_BUNDLE", false)
	viper.SetDefault("FF_BATCH_JOB_OPS", false)
	viper.SetDefault("FF_ENVIRONMENTS", false)
	viper.SetDefault("FF_JOB_GROUPS", false)
	viper.SetDefault("FF_JOB_DEPENDENCIES", false)
	viper.SetDefault("FF_JOB_HEALTH_SCORING", false)
	viper.SetDefault("SECRET_ENCRYPTION_KEY", "")

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
	cfg.CORSAllowedOrigins = viper.GetStringSlice("CORS_ALLOWED_ORIGINS")
	cfg.CORSAllowCredentials = viper.GetBool("CORS_ALLOW_CREDENTIALS")
	cfg.RedisSentinelAddrs = viper.GetStringSlice("REDIS_SENTINEL_ADDRS")
	cfg.WorkerPartitions = viper.GetStringSlice("WORKER_PARTITIONS")
	cfg.WorkerPartitionWeights = viper.GetString("WORKER_PARTITION_WEIGHTS")

	if cfg.DatabaseURL == "" {
		return nil, &domain.ConfigError{Field: "DATABASE_URL", Message: "is required"}
	}
	if cfg.InternalSecret == "" {
		return nil, &domain.ConfigError{Field: "INTERNAL_SECRET", Message: "is required"}
	}
	if len(cfg.JWTSigningKey) < 32 {
		return nil, &domain.ConfigError{Field: "JWT_SIGNING_KEY", Message: "must be at least 32 characters"}
	}
	if cfg.FFSecretInjection && cfg.SecretEncryptionKey == "" {
		return nil, &domain.ConfigError{Field: "SECRET_ENCRYPTION_KEY", Message: "is required when FF_SECRET_INJECTION is enabled"}
	}

	return &cfg, nil
}
