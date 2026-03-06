package config

import (
	"fmt"
	"time"

	"orchestrator/internal/domain"

	"github.com/spf13/viper"
)

type Config struct {
	DatabaseURL              string        `mapstructure:"DATABASE_URL"`
	RedisURL                 string        `mapstructure:"REDIS_URL"`
	RedisSentinelMaster      string        `mapstructure:"REDIS_SENTINEL_MASTER"`
	RedisSentinelAddrs       []string      `mapstructure:"REDIS_SENTINEL_ADDRS"`
	Mode                     string        `mapstructure:"MODE"`
	Port                     int           `mapstructure:"PORT"`
	WorkerConcurrency        int           `mapstructure:"WORKER_CONCURRENCY"`
	InternalSecret           string        `mapstructure:"INTERNAL_SECRET"`
	JWTSigningKey            string        `mapstructure:"JWT_SIGNING_KEY"`
	LogLevel                 string        `mapstructure:"LOG_LEVEL"`
	HeartbeatInterval        time.Duration `mapstructure:"HEARTBEAT_INTERVAL"`
	ReaperInterval           time.Duration `mapstructure:"REAPER_INTERVAL"`
	StaleThreshold           time.Duration `mapstructure:"STALE_THRESHOLD"`
	PollerInterval           time.Duration `mapstructure:"POLLER_INTERVAL"`
	OTELEndpoint             string        `mapstructure:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	WorkflowRunRetentionDays int           `mapstructure:"WORKFLOW_RUN_RETENTION_DAYS"`

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
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	viper.SetDefault("MODE", "all")
	viper.SetDefault("PORT", 8080)
	viper.SetDefault("WORKER_CONCURRENCY", 10)
	viper.SetDefault("LOG_LEVEL", "info")
	viper.SetDefault("HEARTBEAT_INTERVAL", 10*time.Second)
	viper.SetDefault("REAPER_INTERVAL", 30*time.Second)
	viper.SetDefault("STALE_THRESHOLD", 60*time.Second)
	viper.SetDefault("POLLER_INTERVAL", 5*time.Second)
	viper.SetDefault("WORKFLOW_RUN_RETENTION_DAYS", 30)
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

	viper.AutomaticEnv()

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	cfg.HeartbeatInterval = viper.GetDuration("HEARTBEAT_INTERVAL")
	cfg.ReaperInterval = viper.GetDuration("REAPER_INTERVAL")
	cfg.StaleThreshold = viper.GetDuration("STALE_THRESHOLD")
	cfg.PollerInterval = viper.GetDuration("POLLER_INTERVAL")
	cfg.DBMaxConnLifetime = viper.GetDuration("DB_MAX_CONN_LIFETIME")
	cfg.DBMaxConnIdleTime = viper.GetDuration("DB_MAX_CONN_IDLE_TIME")
	cfg.DBMaxConns = viper.GetInt32("DB_MAX_CONNS")
	cfg.DBMinConns = viper.GetInt32("DB_MIN_CONNS")
	cfg.RateLimitWindow = viper.GetDuration("RATE_LIMIT_WINDOW")
	cfg.TriggerRateLimitWindow = viper.GetDuration("TRIGGER_RATE_LIMIT_WINDOW")
	cfg.CORSAllowedOrigins = viper.GetStringSlice("CORS_ALLOWED_ORIGINS")
	cfg.CORSAllowCredentials = viper.GetBool("CORS_ALLOW_CREDENTIALS")
	cfg.RedisSentinelAddrs = viper.GetStringSlice("REDIS_SENTINEL_ADDRS")
	cfg.WorkflowRunRetentionDays = viper.GetInt("WORKFLOW_RUN_RETENTION_DAYS")

	if cfg.DatabaseURL == "" {
		return nil, &domain.ConfigError{Field: "DATABASE_URL", Message: "is required"}
	}
	if cfg.InternalSecret == "" {
		return nil, &domain.ConfigError{Field: "INTERNAL_SECRET", Message: "is required"}
	}
	if len(cfg.JWTSigningKey) < 32 {
		return nil, &domain.ConfigError{Field: "JWT_SIGNING_KEY", Message: "must be at least 32 characters"}
	}

	return &cfg, nil
}
