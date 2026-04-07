package config

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	"strait/internal/domain"

	"github.com/cristalhq/aconfig"
)

type Config struct {
	DatabaseURL                   string        `env:"DATABASE_URL"`
	RedisURL                      string        `env:"REDIS_URL"`
	RedisSentinelMaster           string        `env:"REDIS_SENTINEL_MASTER"`
	RedisSentinelAddrs            []string      `env:"REDIS_SENTINEL_ADDRS"`
	Mode                          string        `env:"MODE" default:"all"`
	Port                          int           `env:"PORT" default:"8080"`
	WorkerConcurrency             int           `env:"WORKER_CONCURRENCY" default:"25"`
	InternalSecret                string        `env:"INTERNAL_SECRET"`
	JWTSigningKey                 string        `env:"JWT_SIGNING_KEY"`
	NotifySubscriberTokenIssuer   string        `env:"NOTIFY_SUBSCRIBER_TOKEN_ISSUER" default:"strait-notify"`
	NotifySubscriberTokenAudience string        `env:"NOTIFY_SUBSCRIBER_TOKEN_AUDIENCE" default:"strait-notify-subscriber"`
	NotifyEmailProvider           string        `env:"NOTIFY_EMAIL_PROVIDER" default:"ses"`
	NotifyEmailAllowLegacyResend  bool          `env:"NOTIFY_EMAIL_ALLOW_LEGACY_RESEND" default:"false"`
	NotifyEmailNormalizeEnabled   bool          `env:"NOTIFY_EMAIL_NORMALIZE_ENABLED" default:"true"`
	NotifyEmailVerifyEnabled      bool          `env:"NOTIFY_EMAIL_VERIFY_ENABLED" default:"true"`
	NotifyEmailVerifyMX           bool          `env:"NOTIFY_EMAIL_VERIFY_MX" default:"true"`
	SecretEncryptionKey           string        `env:"SECRET_ENCRYPTION_KEY"`
	EncryptionKey                 string        `env:"ENCRYPTION_KEY"`
	EncryptionKeyOld              []string      `env:"ENCRYPTION_KEY_OLD"`
	OIDCEnabled                   bool          `env:"OIDC_ENABLED" default:"false"`
	OIDCIssuer                    string        `env:"OIDC_ISSUER"`
	OIDCAudience                  string        `env:"OIDC_AUDIENCE"`
	OIDCPublicKeyPEM              string        `env:"OIDC_PUBLIC_KEY_PEM"`
	LogLevel                      string        `env:"LOG_LEVEL" default:"info"`
	LogFormat                     string        `env:"LOG_FORMAT" default:"json"`
	HeartbeatInterval             time.Duration `env:"HEARTBEAT_INTERVAL" default:"10s"`
	ReaperInterval                time.Duration `env:"REAPER_INTERVAL" default:"30s"`
	StaleThreshold                time.Duration `env:"STALE_THRESHOLD" default:"1m"`
	PollerInterval                time.Duration `env:"POLLER_INTERVAL" default:"5s"`
	RunRetentionShort             time.Duration `env:"RUN_RETENTION_SHORT" default:"720h"`
	RunRetentionLong              time.Duration `env:"RUN_RETENTION_LONG" default:"2160h"`
	OTELEndpoint                  string        `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	WorkflowRunRetentionDays      int           `env:"WORKFLOW_RUN_RETENTION_DAYS" default:"30"`
	EventTriggerRetentionDays     int           `env:"EVENT_TRIGGER_RETENTION_DAYS"`

	// Database connection pool tuning
	DBMaxConns         int32         `env:"DB_MAX_CONNS" default:"50"`
	DBMinConns         int32         `env:"DB_MIN_CONNS" default:"10"`
	DBMaxConnLifetime  time.Duration `env:"DB_MAX_CONN_LIFETIME" default:"30m"`
	DBMaxConnIdleTime  time.Duration `env:"DB_MAX_CONN_IDLE_TIME" default:"5m"`
	DBStatementTimeout time.Duration `env:"DB_STATEMENT_TIMEOUT" default:"30s"`

	RateLimitRequests int           `env:"RATE_LIMIT_REQUESTS" default:"100"`
	RateLimitWindow   time.Duration `env:"RATE_LIMIT_WINDOW" default:"1m"`

	DefaultAPIKeyRateLimit      int `env:"DEFAULT_API_KEY_RATE_LIMIT" default:"1000"`
	DefaultAPIKeyRateWindowSecs int `env:"DEFAULT_API_KEY_RATE_WINDOW_SECS" default:"60"`

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
	CORSAllowedOrigins   []string `env:"CORS_ALLOWED_ORIGINS"`
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
	WebhookTimeout             time.Duration `env:"WEBHOOK_TIMEOUT" default:"10s"`
	WebhookIdleConnTimeout     time.Duration `env:"WEBHOOK_IDLE_CONN_TIMEOUT" default:"1m"`
	ExecutorHTTPTimeout        time.Duration `env:"EXECUTOR_HTTP_TIMEOUT" default:"5m"`
	ExecutorIdleConnTimeout    time.Duration `env:"EXECUTOR_IDLE_CONN_TIMEOUT" default:"1m30s"`
	WebhookDispatchTimeout     time.Duration `env:"WEBHOOK_DISPATCH_TIMEOUT" default:"15s"`
	WebhookMaxPayloadBytes     int64         `env:"WEBHOOK_MAX_PAYLOAD_BYTES" default:"1048576"`
	WebhookConcurrency         int           `env:"WEBHOOK_CONCURRENCY" default:"50"`
	WebhookMaxIdleConns        int           `env:"WEBHOOK_MAX_IDLE_CONNS" default:"100"`
	WebhookMaxIdleConnsPerHost int           `env:"WEBHOOK_MAX_IDLE_CONNS_PER_HOST" default:"50"`
	WebhookBatchEnabled        bool          `env:"WEBHOOK_BATCH_ENABLED" default:"false"`
	WebhookMaxBatchSize        int           `env:"WEBHOOK_MAX_BATCH_SIZE" default:"50"`

	// Worker settings
	WebhookMaxAttempts       int `env:"WEBHOOK_MAX_ATTEMPTS" default:"3"`
	DefaultJobMaxAttempts    int `env:"DEFAULT_JOB_MAX_ATTEMPTS" default:"3"`
	DefaultJobTimeoutSecs    int `env:"DEFAULT_JOB_TIMEOUT_SECS" default:"300"`
	DefaultJobMaxConcurrency int `env:"DEFAULT_JOB_MAX_CONCURRENCY" default:"0"`
	WorkerQueueSize          int `env:"WORKER_QUEUE_SIZE" default:"0"`
	MaxDequeueBatchSize      int `env:"MAX_DEQUEUE_BATCH_SIZE" default:"0"`

	// Scheduler settings
	WorkflowRetention                         time.Duration `env:"WORKFLOW_RETENTION" default:"720h"`
	EventTriggerRetention                     time.Duration `env:"EVENT_TRIGGER_RETENTION"`
	IndexMaintenanceInterval                  time.Duration `env:"INDEX_MAINTENANCE_INTERVAL" default:"24h"`
	ReaperDeleteBatchSize                     int           `env:"REAPER_DELETE_BATCH_SIZE" default:"100"`
	StalledWorkflowThreshold                  time.Duration `env:"WF_STALL_THRESHOLD" default:"15m"`
	StalledWorkflowAction                     string        `env:"WF_STALL_ACTION" default:"log_only"`
	WfMaxStepCap                              int           `env:"WF_MAX_STEP_CAP" default:"100"`
	WfStepConcurrencyLimit                    int           `env:"WF_STEP_CONCURRENCY_LIMIT" default:"0"`
	DependencyStatusCacheTTL                  time.Duration `env:"DEPENDENCY_STATUS_CACHE_TTL" default:"5s"`
	NotifyDeliveryMaxAttempts                 int           `env:"NOTIFY_DELIVERY_MAX_ATTEMPTS" default:"5"`
	NotifyRetryBaseDelay                      time.Duration `env:"NOTIFY_RETRY_BASE_DELAY" default:"30s"`
	NotifyRetryMaxDelay                       time.Duration `env:"NOTIFY_RETRY_MAX_DELAY" default:"15m"`
	NotifyDigestMaxItems                      int           `env:"NOTIFY_DIGEST_MAX_ITEMS" default:"50"`
	NotifyDigestMaxTitleChars                 int           `env:"NOTIFY_DIGEST_MAX_TITLE_CHARS" default:"120"`
	NotifyEscalationTiers                     int           `env:"NOTIFY_ESCALATION_TIERS" default:"3"`
	NotifyEscalationMinInterval               time.Duration `env:"NOTIFY_ESCALATION_MIN_INTERVAL" default:"10m"`
	NotifySESFeedbackSQSURL                   string        `env:"NOTIFY_SES_FEEDBACK_SQS_URL"`
	NotifySESFeedbackPollInterval             time.Duration `env:"NOTIFY_SES_FEEDBACK_POLL_INTERVAL" default:"5s"`
	NotifySESFeedbackWaitTimeSeconds          int32         `env:"NOTIFY_SES_FEEDBACK_WAIT_TIME_SECONDS" default:"10"`
	NotifySESFeedbackMaxMessages              int32         `env:"NOTIFY_SES_FEEDBACK_MAX_MESSAGES" default:"10"`
	NotifySESFeedbackVisibilityTimeoutSeconds int32         `env:"NOTIFY_SES_FEEDBACK_VISIBILITY_TIMEOUT_SECONDS" default:"120"`

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
	AllowedImageRegistries  []string      `env:"ALLOWED_IMAGE_REGISTRIES" envSeparator:"," envDefault:""`
	RequireImageDigest      bool          `env:"REQUIRE_IMAGE_DIGEST" envDefault:"false"`
	ComputeRuntime          string        `env:"COMPUTE_RUNTIME" default:"k8s"`
	ComputeFallbackProvider string        `env:"COMPUTE_FALLBACK_PROVIDER"`
	DefaultRegion           string        `env:"DEFAULT_REGION" default:"iad"`
	ExternalAPIURL          string        `env:"EXTERNAL_API_URL"`
	MaxConcurrentMachines   int           `env:"MAX_CONCURRENT_MACHINES" default:"10"`
	WarmPoolEnabled         bool          `env:"WARM_POOL_ENABLED" default:"false"`
	WarmPoolMaxPerJob       int           `env:"WARM_POOL_MAX_PER_JOB" default:"0"`
	WarmPoolTTL             time.Duration `env:"WARM_POOL_TTL"`
	DisableMachinePoolReuse bool          `env:"DISABLE_MACHINE_POOL_REUSE" default:"true"`

	// Kubernetes runtime
	K8sKubeconfig    string        `env:"K8S_KUBECONFIG"`
	K8sNamespace     string        `env:"K8S_NAMESPACE" default:"default"`
	K8sPriorityClass string        `env:"K8S_PRIORITY_CLASS" default:"strait-job"`
	K8sGCEnabled     bool          `env:"K8S_GC_ENABLED" default:"true"`
	K8sGCMaxAge      time.Duration `env:"K8S_GC_MAX_AGE" default:"30m"`
	K8sGCInterval    time.Duration `env:"K8S_GC_INTERVAL" default:"5m"`

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

	// Stripe billing integration
	StripeSecretKey                      string `env:"STRIPE_SECRET_KEY"`
	StripeWebhookSecret                  string `env:"STRIPE_WEBHOOK_SECRET"`
	StripeStarterMonthlyPriceID          string `env:"STRIPE_STARTER_MONTHLY_PRICE_ID"`
	StripeStarterYearlyPriceID           string `env:"STRIPE_STARTER_YEARLY_PRICE_ID"`
	StripeProMonthlyPriceID              string `env:"STRIPE_PRO_MONTHLY_PRICE_ID"`
	StripeProYearlyPriceID               string `env:"STRIPE_PRO_YEARLY_PRICE_ID"`
	StripeScaleMonthlyPriceID            string `env:"STRIPE_SCALE_MONTHLY_PRICE_ID"`
	StripeScaleYearlyPriceID             string `env:"STRIPE_SCALE_YEARLY_PRICE_ID"`
	StripeEnterpriseStarterYearlyPriceID string `env:"STRIPE_ENTERPRISE_STARTER_YEARLY_PRICE_ID"`
	StripeEnterpriseGrowthYearlyPriceID  string `env:"STRIPE_ENTERPRISE_GROWTH_YEARLY_PRICE_ID"`
	StripeEnterpriseLargeYearlyPriceID   string `env:"STRIPE_ENTERPRISE_LARGE_YEARLY_PRICE_ID"`
	StripeAddonConcurrentRunsID          string `env:"STRIPE_ADDON_CONCURRENT_RUNS_PRICE_ID"`
	StripeAddonMembersID                 string `env:"STRIPE_ADDON_MEMBERS_PRICE_ID"`
	StripeAddonCronSchedulesID           string `env:"STRIPE_ADDON_CRON_SCHEDULES_PRICE_ID"`
	StripeAddonDataRetentionID           string `env:"STRIPE_ADDON_DATA_RETENTION_PRICE_ID"`
	StripeAddonWebhookEndpointsID        string `env:"STRIPE_ADDON_WEBHOOK_ENDPOINTS_PRICE_ID"`
	StripeMeterID                        string `env:"STRIPE_METER_ID"`
	BillingEnforcementEnabled            bool   `env:"BILLING_ENFORCEMENT_ENABLED" default:"false"`

	// Notify email provider integration
	SESRegion           string `env:"SES_REGION" default:"us-east-1"`
	SESFromEmail        string `env:"SES_FROM_EMAIL" default:"noreply@strait.dev"`
	SESConfigurationSet string `env:"SES_CONFIGURATION_SET"`
	SESAccessKeyID      string `env:"SES_ACCESS_KEY_ID"`
	SESSecretAccessKey  string `env:"SES_SECRET_ACCESS_KEY"`
	SESSessionToken     string `env:"SES_SESSION_TOKEN"`

	// Resend integration (legacy notify + billing emails)
	ResendAPIKey    string `env:"RESEND_API_KEY"`
	ResendFromEmail string `env:"RESEND_FROM_EMAIL" default:"noreply@strait.dev"`

	// PostHog product analytics (server-side revenue events)
	PostHogAPIKey string `env:"POSTHOG_API_KEY"`
	PostHogHost   string `env:"POSTHOG_HOST" default:"https://us.i.posthog.com"`

	// Sentry error tracking
	SentryDSN         string `env:"SENTRY_DSN"`
	SentryEnvironment string `env:"SENTRY_ENVIRONMENT" default:"development"`

	// Pyroscope continuous profiling
	PyroscopeEndpoint  string `env:"PYROSCOPE_ENDPOINT"`
	PyroscopeAuthToken string `env:"PYROSCOPE_AUTH_TOKEN"`

	// Debug tools
	DebugStatsviz bool `env:"DEBUG_STATSVIZ" default:"false"`

	// Edition controls feature gating (community vs cloud)
	Edition string `env:"STRAIT_EDITION" default:"community"`
}

// Load reads configuration from environment variables.
//
//nolint:gocyclo,cyclop,gocognit
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
	if len(cfg.InternalSecret) < 16 {
		return nil, &domain.ConfigError{Field: "INTERNAL_SECRET", Message: "must be at least 16 characters"}
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
		if cfg.SentryEnvironment != "development" && cfg.SentryEnvironment != "test" {
			return nil, &domain.ConfigError{Field: "DATABASE_URL", Message: "sslmode=disable is not allowed in non-development environments"}
		}
		slog.Warn("DATABASE_URL has sslmode=disable; connections are not encrypted")
	}

	if cfg.SequinBaseURL != "" {
		if u, err := url.Parse(cfg.SequinBaseURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") {
			return nil, &domain.ConfigError{Field: "SEQUIN_BASE_URL", Message: "must be a valid HTTP(S) URL"}
		}
	}

	switch cfg.ComputeRuntime {
	case "none", "docker", "k8s", "":
		// valid
	default:
		return nil, &domain.ConfigError{Field: "COMPUTE_RUNTIME", Message: "must be none, docker, or k8s"}
	}
	if cfg.ComputeRuntime == "k8s" || cfg.ComputeFallbackProvider == "k8s" {
		if cfg.K8sNamespace == "" {
			return nil, &domain.ConfigError{Field: "K8S_NAMESPACE", Message: "is required when using k8s compute runtime"}
		}
	}
	if cfg.ComputeFallbackProvider != "" {
		switch cfg.ComputeFallbackProvider {
		case "docker", "k8s":
			// valid
		default:
			return nil, &domain.ConfigError{Field: "COMPUTE_FALLBACK_PROVIDER", Message: "must be docker or k8s"}
		}
		if cfg.ComputeFallbackProvider == cfg.ComputeRuntime {
			return nil, &domain.ConfigError{Field: "COMPUTE_FALLBACK_PROVIDER", Message: "must differ from COMPUTE_RUNTIME"}
		}
		if cfg.ComputeRuntime == "none" || cfg.ComputeRuntime == "" {
			return nil, &domain.ConfigError{Field: "COMPUTE_FALLBACK_PROVIDER", Message: "requires a primary COMPUTE_RUNTIME"}
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
	if err := validateNotifyConfig(&cfg); err != nil {
		return nil, err
	}

	if cfg.EncryptionKey == "" && cfg.SecretEncryptionKey == "" {
		slog.Warn("neither ENCRYPTION_KEY nor SECRET_ENCRYPTION_KEY is set; secret encryption will be unavailable")
	}

	for _, origin := range cfg.CORSAllowedOrigins {
		if origin == "*" && cfg.CORSAllowCredentials {
			return nil, &domain.ConfigError{
				Field:   "CORS_ALLOWED_ORIGINS",
				Message: "wildcard origin (*) is not allowed when CORS_ALLOW_CREDENTIALS is true",
			}
		}
		if origin == "*" {
			if cfg.SentryEnvironment != "development" && cfg.SentryEnvironment != "test" {
				return nil, &domain.ConfigError{
					Field:   "CORS_ALLOWED_ORIGINS",
					Message: "wildcard origin (*) is not allowed in non-development environments",
				}
			}
			slog.Warn("CORS_ALLOWED_ORIGINS is set to wildcard (*); consider restricting to specific origins in production")
		}
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

func validateNotifyConfig(cfg *Config) error {
	if cfg == nil {
		return nil
	}
	if strings.TrimSpace(cfg.NotifySubscriberTokenIssuer) == "" {
		return &domain.ConfigError{Field: "NOTIFY_SUBSCRIBER_TOKEN_ISSUER", Message: "must not be empty"}
	}
	if strings.TrimSpace(cfg.NotifySubscriberTokenAudience) == "" {
		return &domain.ConfigError{Field: "NOTIFY_SUBSCRIBER_TOKEN_AUDIENCE", Message: "must not be empty"}
	}
	switch strings.ToLower(strings.TrimSpace(cfg.NotifyEmailProvider)) {
	case "", "ses", "resend":
		// valid
	default:
		return &domain.ConfigError{Field: "NOTIFY_EMAIL_PROVIDER", Message: "must be ses or resend"}
	}
	if strings.EqualFold(cfg.NotifyEmailProvider, "ses") {
		if strings.TrimSpace(cfg.SESRegion) == "" {
			return &domain.ConfigError{Field: "SES_REGION", Message: "is required when NOTIFY_EMAIL_PROVIDER=ses"}
		}
		if strings.TrimSpace(cfg.SESFromEmail) == "" {
			return &domain.ConfigError{Field: "SES_FROM_EMAIL", Message: "is required when NOTIFY_EMAIL_PROVIDER=ses"}
		}
	}
	if strings.EqualFold(cfg.NotifyEmailProvider, "resend") {
		if !cfg.NotifyEmailAllowLegacyResend {
			return &domain.ConfigError{Field: "NOTIFY_EMAIL_ALLOW_LEGACY_RESEND", Message: "must be true when NOTIFY_EMAIL_PROVIDER=resend"}
		}
		if strings.TrimSpace(cfg.ResendFromEmail) == "" {
			return &domain.ConfigError{Field: "RESEND_FROM_EMAIL", Message: "is required when NOTIFY_EMAIL_PROVIDER=resend"}
		}
		slog.Warn("NOTIFY_EMAIL_PROVIDER=resend is legacy mode and should only be used for temporary rollback")
	}
	if cfg.NotifyDeliveryMaxAttempts < 1 {
		return &domain.ConfigError{Field: "NOTIFY_DELIVERY_MAX_ATTEMPTS", Message: "must be >= 1"}
	}
	if cfg.NotifyRetryBaseDelay <= 0 {
		return &domain.ConfigError{Field: "NOTIFY_RETRY_BASE_DELAY", Message: "must be > 0"}
	}
	if cfg.NotifyRetryMaxDelay < cfg.NotifyRetryBaseDelay {
		return &domain.ConfigError{Field: "NOTIFY_RETRY_MAX_DELAY", Message: "must be >= NOTIFY_RETRY_BASE_DELAY"}
	}
	if cfg.NotifyDigestMaxItems < 1 {
		return &domain.ConfigError{Field: "NOTIFY_DIGEST_MAX_ITEMS", Message: "must be >= 1"}
	}
	if cfg.NotifyDigestMaxTitleChars < 16 {
		return &domain.ConfigError{Field: "NOTIFY_DIGEST_MAX_TITLE_CHARS", Message: "must be >= 16"}
	}
	if cfg.NotifyEscalationTiers < 1 {
		return &domain.ConfigError{Field: "NOTIFY_ESCALATION_TIERS", Message: "must be >= 1"}
	}
	if cfg.NotifyEscalationMinInterval <= 0 {
		return &domain.ConfigError{Field: "NOTIFY_ESCALATION_MIN_INTERVAL", Message: "must be > 0"}
	}
	if cfg.NotifySESFeedbackWaitTimeSeconds < 0 || cfg.NotifySESFeedbackWaitTimeSeconds > 20 {
		return &domain.ConfigError{Field: "NOTIFY_SES_FEEDBACK_WAIT_TIME_SECONDS", Message: "must be between 0 and 20"}
	}
	if cfg.NotifySESFeedbackMaxMessages < 1 || cfg.NotifySESFeedbackMaxMessages > 10 {
		return &domain.ConfigError{Field: "NOTIFY_SES_FEEDBACK_MAX_MESSAGES", Message: "must be between 1 and 10"}
	}
	if cfg.NotifySESFeedbackVisibilityTimeoutSeconds < 1 || cfg.NotifySESFeedbackVisibilityTimeoutSeconds > 43200 {
		return &domain.ConfigError{Field: "NOTIFY_SES_FEEDBACK_VISIBILITY_TIMEOUT_SECONDS", Message: "must be between 1 and 43200"}
	}
	if strings.EqualFold(cfg.NotifyEmailProvider, "ses") && strings.TrimSpace(cfg.NotifySESFeedbackSQSURL) == "" {
		slog.Warn("NOTIFY_SES_FEEDBACK_SQS_URL is not set; SES bounce/complaint feedback ingestion is disabled")
	}
	return nil
}

// Redacted returns a map of config field names to values with secrets masked.
// Includes all operationally useful fields while hiding credentials and keys.
func (c *Config) Redacted() map[string]any {
	return map[string]any{
		"Mode":                   c.Mode,
		"Port":                   c.Port,
		"Edition":                c.Edition,
		"WorkerConcurrency":      c.WorkerConcurrency,
		"PollerInterval":         c.PollerInterval.String(),
		"DBMaxConns":             c.DBMaxConns,
		"ComputeRuntime":         c.ComputeRuntime,
		"SentryEnvironment":      c.SentryEnvironment,
		"DefaultAPIKeyRateLimit": c.DefaultAPIKeyRateLimit,
		"DatabaseURL":            "[REDACTED]",
		"RedisURL":               "[REDACTED]",
		"InternalSecret":         "[REDACTED]",
		"JWTSigningKey":          "[REDACTED]",
		"EncryptionKey":          "[REDACTED]",
		"StripeSecretKey":        "[REDACTED]",
		"StripeWebhookSecret":    "[REDACTED]",
		"SESAccessKeyID":         "[REDACTED]",
		"SESSecretAccessKey":     "[REDACTED]",
		"SESSessionToken":        "[REDACTED]",
		"ResendAPIKey":           "[REDACTED]",
		"PostHogAPIKey":          "[REDACTED]",
		"SentryDSN":              "[REDACTED]",
	}
}

// String returns a redacted string representation of the config for logging.
// Keys are sorted for deterministic output.
func (c *Config) String() string {
	r := c.Redacted()
	keys := make([]string, 0, len(r))
	for k := range r {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	var sb strings.Builder
	for _, k := range keys {
		if sb.Len() > 0 {
			sb.WriteString(" ")
		}
		fmt.Fprintf(&sb, "%s=%v", k, r[k])
	}
	return sb.String()
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
