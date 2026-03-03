package config

import (
	"fmt"
	"time"

	"orchestrator/internal/domain"

	"github.com/spf13/viper"
)

type Config struct {
	DatabaseURL       string        `mapstructure:"DATABASE_URL"`
	RedisURL          string        `mapstructure:"REDIS_URL"`
	Mode              string        `mapstructure:"MODE"`
	Port              int           `mapstructure:"PORT"`
	WorkerConcurrency int           `mapstructure:"WORKER_CONCURRENCY"`
	InternalSecret    string        `mapstructure:"INTERNAL_SECRET"`
	JWTSigningKey     string        `mapstructure:"JWT_SIGNING_KEY"`
	LogLevel          string        `mapstructure:"LOG_LEVEL"`
	HeartbeatInterval time.Duration `mapstructure:"HEARTBEAT_INTERVAL"`
	ReaperInterval    time.Duration `mapstructure:"REAPER_INTERVAL"`
	StaleThreshold    time.Duration `mapstructure:"STALE_THRESHOLD"`
	PollerInterval    time.Duration `mapstructure:"POLLER_INTERVAL"`
	OTELEndpoint      string        `mapstructure:"OTEL_EXPORTER_OTLP_ENDPOINT"`
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

	viper.AutomaticEnv()

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	cfg.HeartbeatInterval = viper.GetDuration("HEARTBEAT_INTERVAL")
	cfg.ReaperInterval = viper.GetDuration("REAPER_INTERVAL")
	cfg.StaleThreshold = viper.GetDuration("STALE_THRESHOLD")
	cfg.PollerInterval = viper.GetDuration("POLLER_INTERVAL")

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
