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
	LogFormat                 string        `env:"LOG_FORMAT" default:"json"`
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
	DBMaxConns          int32         `env:"DB_MAX_CONNS" default:"50"`
	DBMinConns          int32         `env:"DB_MIN_CONNS" default:"10"`
	DBMaxConnLifetime   time.Duration `env:"DB_MAX_CONN_LIFETIME" default:"30m"`
	DBMaxConnIdleTime   time.Duration `env:"DB_MAX_CONN_IDLE_TIME" default:"5m"`
	DBHealthCheckPeriod time.Duration `env:"DB_HEALTH_CHECK_PERIOD" default:"30s"`
	DBStatementTimeout  time.Duration `env:"DB_STATEMENT_TIMEOUT" default:"30s"`

	// MVCC horizon guardrails (Phase 1). These prevent stray long transactions
	// from pinning pg_xmin and blocking autovacuum on hot queue tables.
	DBIdleInTransactionTimeout time.Duration `env:"DB_IDLE_IN_TRANSACTION_TIMEOUT" default:"30s"`
	DBLockTimeout              time.Duration `env:"DB_LOCK_TIMEOUT" default:"5s"`
	DBTransactionTimeout       time.Duration `env:"DB_TRANSACTION_TIMEOUT" default:"0"`
	DBLongTxnAlertThreshold    time.Duration `env:"DB_LONG_TXN_ALERT_THRESHOLD" default:"60s"`
	DBWatchdogInterval         time.Duration `env:"DB_WATCHDOG_INTERVAL" default:"15s"`
	DBWatchdogEnabled          bool          `env:"DB_WATCHDOG_ENABLED" default:"true"`

	// Phase 6: toggle the denormalized dequeue path (uses job_active_counts
	// lookup table instead of COUNT-over-active-rows CTE).
	QueueUseDenormalizedDequeue bool `env:"QUEUE_USE_DENORMALIZED_DEQUEUE" default:"false"`

	// Phase 9: DLQ caps and overflow policy.
	DLQMaxPerProject  int    `env:"DLQ_MAX_PER_PROJECT" default:"10000"`
	DLQMaxPerJob      int    `env:"DLQ_MAX_PER_JOB" default:"1000"`
	DLQOverflowPolicy string `env:"DLQ_OVERFLOW_POLICY" default:"drop_oldest"`

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

	// WorkerEventChannelSize configures the buffered capacity of the executor's
	// internal run-lifecycle event channel. Increasing this value trades memory
	// for tolerance of bursty subscriber latency before events start dropping.
	WorkerEventChannelSize int `env:"WORKER_EVENT_CHANNEL_SIZE" default:"1024"`

	// SchedulerComponentShutdownTimeout bounds how long Scheduler.Stop will wait
	// for any single background component to exit before emitting a warning and
	// moving on. A stuck component past this deadline increments
	// strait.scheduler.shutdown_timeouts_total and is reported in logs.
	SchedulerComponentShutdownTimeout time.Duration `env:"SCHEDULER_COMPONENT_SHUTDOWN_TIMEOUT" default:"15s"`

	// BackpressureSamplerInterval controls how often the scheduler samples
	// per-project backpressure token buckets to populate the
	// strait.queue.backpressure_tokens_available gauge. Set to 0 to disable
	// the sampler; the gauge will simply report no points.
	BackpressureSamplerInterval time.Duration `env:"BACKPRESSURE_SAMPLER_INTERVAL" default:"15s"`

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
	WorkflowRetention        time.Duration `env:"WORKFLOW_RETENTION" default:"720h"`
	EventTriggerRetention    time.Duration `env:"EVENT_TRIGGER_RETENTION"`
	IndexMaintenanceInterval time.Duration `env:"INDEX_MAINTENANCE_INTERVAL" default:"24h"`
	ReaperDeleteBatchSize    int           `env:"REAPER_DELETE_BATCH_SIZE" default:"100"`
	StalledWorkflowThreshold time.Duration `env:"WF_STALL_THRESHOLD" default:"15m"`
	StalledWorkflowAction    string        `env:"WF_STALL_ACTION" default:"log_only"`
	WfMaxStepCap             int           `env:"WF_MAX_STEP_CAP" default:"100"`
	WfStepConcurrencyLimit   int           `env:"WF_STEP_CONCURRENCY_LIMIT" default:"0"`
	DependencyStatusCacheTTL time.Duration `env:"DEPENDENCY_STATUS_CACHE_TTL" default:"5s"`

	// Workflow settings
	MaxWorkflowNestingDepth int `env:"MAX_WORKFLOW_NESTING_DEPTH" default:"10"`

	CDCBatchSize  int `env:"CDC_BATCH_SIZE" default:"10"`
	CDCWaitTimeMs int `env:"CDC_WAIT_TIME_MS" default:"5000"`

	// SSE settings
	SSEKeepaliveInterval  time.Duration `env:"SSE_KEEPALIVE_INTERVAL" default:"15s"`
	SSEMaxConns           int64         `env:"SSE_MAX_CONNS" default:"5000"`
	SSEMaxConnsPerProject int64         `env:"SSE_MAX_CONNS_PER_PROJECT" default:"100"`
	SSEMaxConnDuration    time.Duration `env:"SSE_MAX_CONN_DURATION" default:"30m"`

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
	// K8sRuntimeClass sets the RuntimeClassName on all job pods.
	// Set to "gvisor" to enable gVisor kernel isolation on worker nodes that have
	// the RuntimeClass installed. Leave empty to use the node default runtime.
	K8sRuntimeClass string `env:"K8S_RUNTIME_CLASS" default:""`

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

	// Resend email integration
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

	// Edition is determined at compile time via build tags (community vs cloud).
	// This field exists for config logging but is ignored by domain.ParseEdition.
	Edition string `env:"STRAIT_EDITION" default:"community"`

	// Code-first build pipeline (STR-385).

	// BuildKit daemon address. Used by the build orchestrator to submit builds.
	BuildKitAddress string `env:"BUILDKIT_ADDRESS" default:"tcp://buildkitd.strait-build.svc.cluster.local:1234"`
	// BuildKitAddresses is an optional comma-separated list of BuildKit daemon
	// addresses used for multi-node round-robin dispatch. When non-empty it
	// overrides BuildKitAddress.
	BuildKitAddresses string `env:"BUILDKIT_ADDRESSES" default:""`
	BuildKitNamespace string `env:"BUILDKIT_NAMESPACE" default:"strait-build"`

	// Deployment GC removes stale pending and old failed/timed_out deployments.
	DeploymentGCEnabled    bool          `env:"DEPLOYMENT_GC_ENABLED" default:"true"`
	DeploymentGCInterval   time.Duration `env:"DEPLOYMENT_GC_INTERVAL" default:"1h"`
	DeploymentGCPendingTTL time.Duration `env:"DEPLOYMENT_GC_PENDING_TTL" default:"15m"`
	DeploymentGCFailedAge  time.Duration `env:"DEPLOYMENT_GC_FAILED_AGE" default:"168h"` // 7 days

	BuildKitCacheEnabled bool          `env:"BUILDKIT_CACHE_ENABLED" default:"true"`
	BuildMaxTarballMB    int           `env:"BUILD_MAX_TARBALL_MB" default:"256"`
	BuildTimeout         time.Duration `env:"BUILD_TIMEOUT" default:"10m"`

	// Object store for deployment tarballs.
	// Type selects the implementation: "s3" (default, works for R2 and MinIO).
	ObjectStoreType           string `env:"OBJECT_STORE_TYPE" default:"s3"`
	ObjectStoreBucket         string `env:"OBJECT_STORE_BUCKET"`
	ObjectStoreEndpoint       string `env:"OBJECT_STORE_ENDPOINT"` // e.g. "https://{account}.r2.cloudflarestorage.com" or "http://minio:9000"
	ObjectStoreRegion         string `env:"OBJECT_STORE_REGION" default:"auto"`
	ObjectStoreAccessKey      string `env:"OBJECT_STORE_ACCESS_KEY"`
	ObjectStoreSecretKey      string `env:"OBJECT_STORE_SECRET_KEY"`
	ObjectStoreForcePathStyle bool   `env:"OBJECT_STORE_FORCE_PATH_STYLE" default:"false"` // set true for MinIO

	// Container registry for built images.
	// Type selects the implementation: "ecr" or "generic" (Docker Registry API v2).
	ContainerRegistryType   string `env:"CONTAINER_REGISTRY_TYPE" default:"ecr"`
	ContainerRegistryURL    string `env:"CONTAINER_REGISTRY_URL"`  // for generic
	ContainerRegistryUser   string `env:"CONTAINER_REGISTRY_USER"` // for generic
	ContainerRegistryPass   string `env:"CONTAINER_REGISTRY_PASS"` // for generic
	ContainerRegistryPrefix string `env:"CONTAINER_REGISTRY_PREFIX" default:"strait-jobs"`
	ECRRegion               string `env:"ECR_REGION" default:"us-east-1"`
	ECRRegistryID           string `env:"ECR_REGISTRY_ID"` // AWS account ID; defaults to caller account
	ECRRoleARN              string `env:"ECR_ROLE_ARN"`    // optional IAM role for cross-account access
	// BuildExtraRegistryAuths is a JSON object mapping registry hostnames to
	// bearer tokens used for authenticating private base images at build time.
	// Example: {"private.registry.io": "base64token", "ghcr.io": "ghp_token"}
	BuildExtraRegistryAuths string `env:"BUILD_EXTRA_REGISTRY_AUTHS" default:"{}"`

	// Dispatcher mode: multi-cluster job routing (--mode dispatcher).
	// DispatcherClusterRegistryConfigMap is the name of the K8s ConfigMap that
	// contains the cluster-registry.yaml manifest listing all Strait clusters.
	// Defaults to the name deployed by the infra repo.
	DispatcherClusterRegistryConfigMap string `env:"DISPATCHER_CLUSTER_REGISTRY_CONFIGMAP" default:"cluster-registry"`
	// DispatcherClusterRegistryNamespace is the K8s namespace that contains the
	// cluster-registry ConfigMap.
	DispatcherClusterRegistryNamespace string `env:"DISPATCHER_CLUSTER_REGISTRY_NAMESPACE" default:"strait"`
	// DispatcherRefreshInterval controls how often the dispatcher re-reads cluster
	// queue depths. Shorter intervals improve routing accuracy at the cost of more
	// Prometheus queries.
	DispatcherRefreshInterval time.Duration `env:"DISPATCHER_REFRESH_INTERVAL" default:"5s"`

	// Performance: image pull policy and lazy loading.
	// ImagePullPolicy matches the Kubernetes imagePullPolicy values.
	ImagePullPolicy string `env:"IMAGE_PULL_POLICY" default:"IfNotPresent"`
	// PrePullEnabled deploys a DaemonSet that pre-pulls base runtime images to
	// all nodes, eliminating cold-start latency on the first run.
	PrePullEnabled bool `env:"PRE_PULL_ENABLED" default:"false"`
	// SOCIEnabled enables Seekable OCI (SOCI) lazy image loading via the
	// AWS SOCI snapshotter. Requires the SOCI snapshotter to be installed on nodes.
	// When enabled, container startup begins before the full image is pulled.
	SOCIEnabled bool `env:"SOCI_ENABLED" default:"false"`
	// SOCIMinImageBytes is the minimum compressed image size (in bytes) below which
	// SOCI index generation is skipped — the overhead of lazy loading is not worth it
	// for small images. Defaults to 10 MiB.
	// NOTE: size-based skipping is not yet implemented; this config is reserved for
	// a future optimisation once post-build image size querying is available.
	SOCIMinImageBytes int64 `env:"SOCI_MIN_IMAGE_SIZE_BYTES" default:"10485760"`
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
	if domain.ParseEdition(cfg.Edition) == domain.EditionCommunity && cfg.ComputeRuntime != "none" && cfg.ComputeRuntime != "" {
		slog.Warn("community edition does not support managed execution; overriding COMPUTE_RUNTIME to none",
			"configured", cfg.ComputeRuntime)
		cfg.ComputeRuntime = "none"
	}

	if cfg.ClickHouseEnabled && cfg.ClickHouseURL == "" {
		return nil, &domain.ConfigError{Field: "CLICKHOUSE_URL", Message: "is required when CLICKHOUSE_ENABLED=true"}
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
