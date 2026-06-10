package config

import (
	"fmt"
	"log/slog"
	"net"
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
	RedisPoolSize             int           `env:"REDIS_POOL_SIZE" default:"30"`
	RedisMinIdleConns         int           `env:"REDIS_MIN_IDLE_CONNS" default:"5"`
	RedisReadTimeout          time.Duration `env:"REDIS_READ_TIMEOUT" default:"3s"`
	RedisWriteTimeout         time.Duration `env:"REDIS_WRITE_TIMEOUT" default:"3s"`
	RedisConnMaxLifetime      time.Duration `env:"REDIS_CONN_MAX_LIFETIME" default:"30m"`
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
	DeploymentEnvironment     string        `env:"STRAIT_ENV" default:"production"`
	HeartbeatInterval         time.Duration `env:"HEARTBEAT_INTERVAL" default:"10s"`
	ReaperInterval            time.Duration `env:"REAPER_INTERVAL" default:"30s"`
	StaleThreshold            time.Duration `env:"STALE_THRESHOLD" default:"1m"`
	PollerInterval            time.Duration `env:"POLLER_INTERVAL" default:"1s"`
	RunRetentionShort         time.Duration `env:"RUN_RETENTION_SHORT" default:"720h"`
	RunRetentionLong          time.Duration `env:"RUN_RETENTION_LONG" default:"2160h"`
	OTELEndpoint              string        `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	WorkflowRunRetentionDays  int           `env:"WORKFLOW_RUN_RETENTION_DAYS" default:"30"`
	EventTriggerRetentionDays int           `env:"EVENT_TRIGGER_RETENTION_DAYS"`
	AuditRetentionDefaultDays int           `env:"AUDIT_RETENTION_DEFAULT_DAYS" default:"365"`
	AuditAsyncBufferSize      int           `env:"AUDIT_ASYNC_BUFFER_SIZE" default:"4096"`
	AuditSIEMEndpoint         string        `env:"AUDIT_SIEM_ENDPOINT"`
	AuditSIEMAuthToken        string        `env:"AUDIT_SIEM_AUTH_TOKEN"`
	AuditSIEMBatchSize        int           `env:"AUDIT_SIEM_BATCH_SIZE" default:"100"`
	AuditSIEMFlushInterval    time.Duration `env:"AUDIT_SIEM_FLUSH_INTERVAL" default:"10s"`
	AuditDLQReclaimBatch      int           `env:"AUDIT_DLQ_RECLAIM_BATCH" default:"200"`
	// AuditDLQMaxAgeDays bounds how long a deadletter row may live before the
	// DLQ retention reaper drops it. 0 disables the sweep — useful for
	// installations where operators want to retain forever and rely on
	// manual triage. Default is 30 days.
	AuditDLQMaxAgeDays int `env:"AUDIT_DLQ_MAX_AGE_DAYS" default:"30"`
	// AuditDLQMaxReclaimAttempts caps how many times the reclaimer will retry
	// a single DLQ row's chain insert. After this many failures the row is
	// skipped (it stays in the DLQ for operator triage) and the
	// strait_audit_reclaimer_abandoned_total metric is bumped. 0 disables
	// the cap entirely (legacy behavior). Default is 10.
	AuditDLQMaxReclaimAttempts int   `env:"AUDIT_DLQ_MAX_RECLAIM_ATTEMPTS" default:"10"`
	AuditExportRowCapDefault   int64 `env:"AUDIT_EXPORT_ROW_CAP_DEFAULT" default:"1000000"`

	// Database connection pool tuning
	DBMaxConns          int32         `env:"DB_MAX_CONNS" default:"50"`
	DBMinConns          int32         `env:"DB_MIN_CONNS" default:"10"`
	DBMaxConnLifetime   time.Duration `env:"DB_MAX_CONN_LIFETIME" default:"30m"`
	DBMaxConnIdleTime   time.Duration `env:"DB_MAX_CONN_IDLE_TIME" default:"5m"`
	DBHealthCheckPeriod time.Duration `env:"DB_HEALTH_CHECK_PERIOD" default:"30s"`
	DBStatementTimeout  time.Duration `env:"DB_STATEMENT_TIMEOUT" default:"30s"`

	// MVCC horizon guardrails. These prevent stray long transactions
	// from pinning pg_xmin and blocking autovacuum on hot queue tables.
	DBIdleInTransactionTimeout         time.Duration `env:"DB_IDLE_IN_TRANSACTION_TIMEOUT" default:"30s"`
	DBLockTimeout                      time.Duration `env:"DB_LOCK_TIMEOUT" default:"5s"`
	DBTransactionTimeout               time.Duration `env:"DB_TRANSACTION_TIMEOUT" default:"60s"`
	DBLongTxnAlertThreshold            time.Duration `env:"DB_LONG_TXN_ALERT_THRESHOLD" default:"60s"`
	DBWatchdogInterval                 time.Duration `env:"DB_WATCHDOG_INTERVAL" default:"15s"`
	DBWatchdogEnabled                  bool          `env:"DB_WATCHDOG_ENABLED" default:"true"`
	DBBackpressureDisabled             bool          `env:"DB_BACKPRESSURE_DISABLED" default:"false"`
	DBBackpressureSampleInterval       time.Duration `env:"DB_BACKPRESSURE_SAMPLE_INTERVAL" default:"100ms"`
	DBBackpressureAcquireWaitThreshold time.Duration `env:"DB_BACKPRESSURE_ACQUIRE_WAIT_THRESHOLD" default:"50ms"`
	DBBackpressureOccupancyThreshold   float64       `env:"DB_BACKPRESSURE_OCCUPANCY_THRESHOLD" default:"0.90"`

	QueuePgQueMaintenanceInterval time.Duration `env:"QUEUE_PGQUE_MAINTENANCE_INTERVAL" default:"30s"`
	QueuePgQueRotationPeriod      time.Duration `env:"QUEUE_PGQUE_ROTATION_PERIOD" default:"5m"`

	// DLQ caps and overflow policy.
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
	SequinBaseURL       string `env:"SEQUIN_BASE_URL"`
	SequinConsumerName  string `env:"SEQUIN_CONSUMER_NAME"`
	SequinDatabaseName  string `env:"SEQUIN_DATABASE_NAME" default:"strait-db"`
	SequinAPIToken      string `env:"SEQUIN_API_TOKEN"`
	SequinWebhookSecret string `env:"SEQUIN_WEBHOOK_SECRET"`
	SequinBatchSize     int    `env:"SEQUIN_BATCH_SIZE" default:"200"`
	SequinWaitTimeMs    int    `env:"SEQUIN_WAIT_TIME_MS" default:"5000"`

	// CORS settings
	CORSAllowedOrigins   []string `env:"CORS_ALLOWED_ORIGINS"`
	CORSAllowCredentials bool     `env:"CORS_ALLOW_CREDENTIALS" default:"false"`

	// TrustedProxies is a comma-separated list of CIDR ranges (or single IPs)
	// that are allowed to set the X-Forwarded-For header. When empty (the
	// default), X-Forwarded-For is ignored entirely and the connection's
	// RemoteAddr is used for rate-limit / lockout accounting. This prevents
	// clients from spoofing their source IP by adding their own XFF entries.
	TrustedProxies []string `env:"TRUSTED_PROXIES"`

	WorkerPartitions       []string `env:"WORKER_PARTITIONS"`
	WorkerPartitionWeights string   `env:"WORKER_PARTITION_WEIGHTS"`
	AdaptiveConcurrencyMin int      `env:"ADAPTIVE_CONCURRENCY_MIN" default:"5"`
	AdaptiveConcurrencyMax int      `env:"ADAPTIVE_CONCURRENCY_MAX" default:"100"`
	DBPgBouncerMode        bool     `env:"DB_PGBOUNCER_MODE" default:"false"`
	DBPgBouncerPrepared    bool     `env:"DB_PGBOUNCER_PREPARED_STATEMENTS" default:"false"`
	DBTraceStatements      bool     `env:"DB_TRACE_STATEMENTS" default:"false"`

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

	// BackpressureEnabled gates the per-project enqueue token bucket. Keep this
	// enabled in production unless an upstream limiter is enforcing equivalent
	// tenant isolation.
	BackpressureEnabled bool `env:"BACKPRESSURE_ENABLED" default:"true"`

	// BackpressureDefaultMaxTokens controls the initial burst capacity for
	// projects without an explicit project_rate_limits row.
	BackpressureDefaultMaxTokens int `env:"BACKPRESSURE_DEFAULT_MAX_TOKENS" default:"1000"`

	// BackpressureDefaultRefillPerSec controls the steady-state accepted enqueue
	// rate per project for projects without an explicit project_rate_limits row.
	BackpressureDefaultRefillPerSec int `env:"BACKPRESSURE_DEFAULT_REFILL_PER_SEC" default:"500"`

	// BackpressureLocalLeaseSize controls how many DB-backed project tokens a
	// process reserves at once before serving single-run admissions from memory.
	// Set to 1 for strict per-trigger DB accounting.
	BackpressureLocalLeaseSize int `env:"BACKPRESSURE_LOCAL_LEASE_SIZE" default:"32"`

	// BackpressureSamplerN bounds the number of project rate-limit rows
	// the sampler reads per tick. Larger values give better gauge
	// coverage on high-tenant deployments at the cost of one extra
	// indexed scan per interval. Defaults to 100 if unset or non-positive.
	BackpressureSamplerN int `env:"BACKPRESSURE_SAMPLER_N" default:"100"`

	// RBAC permission cache
	PermissionCacheTTL time.Duration `env:"PERMISSION_CACHE_TTL" default:"5m"`

	// ProjectQuotaCacheTTL bounds how long a cached project quota row may be
	// reused before refetching. Quotas change rarely (admin updates, billing
	// plan transitions) so a short TTL is a safe trade-off against fresh
	// reads on every trigger/SDK call. Set to 0 to disable caching.
	ProjectQuotaCacheTTL time.Duration `env:"PROJECT_QUOTA_CACHE_TTL" default:"60s"`

	// Worker/Executor timeouts
	WebhookTimeout             time.Duration `env:"WEBHOOK_TIMEOUT" default:"10s"`
	WebhookIdleConnTimeout     time.Duration `env:"WEBHOOK_IDLE_CONN_TIMEOUT" default:"1m"`
	ExecutorHTTPTimeout        time.Duration `env:"EXECUTOR_HTTP_TIMEOUT" default:"5m"`
	ExecutorIdleConnTimeout    time.Duration `env:"EXECUTOR_IDLE_CONN_TIMEOUT" default:"1m30s"`
	ExecutionTraceMode         string        `env:"EXECUTION_TRACE_MODE" default:"off"`
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
	MaxDequeueBatchSize      int `env:"MAX_DEQUEUE_BATCH_SIZE" default:"50"`

	// Scheduler settings
	WorkflowRetention        time.Duration `env:"WORKFLOW_RETENTION" default:"720h"`
	EventTriggerRetention    time.Duration `env:"EVENT_TRIGGER_RETENTION"`
	IndexMaintenanceInterval time.Duration `env:"INDEX_MAINTENANCE_INTERVAL" default:"24h"`
	ReaperDeleteBatchSize    int           `env:"REAPER_DELETE_BATCH_SIZE" default:"5000"`
	TerminalArchiveEnabled   bool          `env:"TERMINAL_ARCHIVE_ENABLED" default:"true"`
	PartitionReclaimEnabled  bool          `env:"PARTITION_RECLAIM_ENABLED" default:"false"`
	PartitionReclaimInterval time.Duration `env:"PARTITION_RECLAIM_INTERVAL" default:"24h"`
	PartitionReclaimSafety   int           `env:"PARTITION_RECLAIM_SAFETY_MONTHS" default:"2"`
	StalledWorkflowThreshold time.Duration `env:"WF_STALL_THRESHOLD" default:"15m"`
	StalledWorkflowAction    string        `env:"WF_STALL_ACTION" default:"reconcile"`
	WfMaxStepCap             int           `env:"WF_MAX_STEP_CAP" default:"100"`
	WfStepConcurrencyLimit   int           `env:"WF_STEP_CONCURRENCY_LIMIT" default:"0"`
	DependencyStatusCacheTTL time.Duration `env:"DEPENDENCY_STATUS_CACHE_TTL" default:"5s"`

	// Workflow settings
	MaxWorkflowNestingDepth int `env:"MAX_WORKFLOW_NESTING_DEPTH" default:"10"`

	CDCBatchSize  int `env:"CDC_BATCH_SIZE" default:"200"`
	CDCWaitTimeMs int `env:"CDC_WAIT_TIME_MS" default:"5000"`

	// SSE settings
	SSEKeepaliveInterval  time.Duration `env:"SSE_KEEPALIVE_INTERVAL" default:"15s"`
	SSEMaxConns           int64         `env:"SSE_MAX_CONNS" default:"5000"`
	SSEMaxConnsPerProject int64         `env:"SSE_MAX_CONNS_PER_PROJECT" default:"100"`
	SSEMaxConnDuration    time.Duration `env:"SSE_MAX_CONN_DURATION" default:"30m"`

	// Idempotency settings. IdempotencyFailOpen=false (the default) makes
	// the middleware return 503 when the idempotency store is unavailable,
	// so non-idempotent operations are never executed twice during a DB
	// outage. Setting it to true degrades to "no dedupe" on store errors.
	IdempotencyFailOpen       bool          `env:"IDEMPOTENCY_FAIL_OPEN" default:"false"`
	IdempotencyCleanupTimeout time.Duration `env:"IDEMPOTENCY_CLEANUP_TIMEOUT" default:"5s"`

	// Log drain settings
	LogDrainWorkerInterval     time.Duration `env:"LOG_DRAIN_WORKER_INTERVAL" default:"1m"`
	MemoryPressureThresholdPct float64       `env:"MEMORY_PRESSURE_THRESHOLD_PCT" default:"0"`
	JobCacheTTL                time.Duration `env:"JOB_CACHE_TTL" default:"5m"`
	VersionCacheTTL            time.Duration `env:"VERSION_CACHE_TTL" default:"30m"`
	RunVersionCacheTTL         time.Duration `env:"RUN_VERSION_CACHE_TTL" default:"10m"`
	APIKeyCacheTTL             time.Duration `env:"API_KEY_CACHE_TTL" default:"60s"`
	JobHealthCacheTTL          time.Duration `env:"JOB_HEALTH_CACHE_TTL" default:"5m"`
	EndpointGuardCacheTTL      time.Duration `env:"ENDPOINT_GUARD_CACHE_TTL" default:"1s"`

	EndpointHealthSuccessSampleInterval  time.Duration `env:"ENDPOINT_HEALTH_SUCCESS_SAMPLE_INTERVAL" default:"1s"`
	EndpointCircuitSuccessSampleInterval time.Duration `env:"ENDPOINT_CIRCUIT_SUCCESS_SAMPLE_INTERVAL" default:"1s"`

	// JobHealthStatsCacheTTL is kept as a compatibility alias for the job
	// health stats cache added before the generalized worker cache tiers. The
	// executor uses JobHealthCacheTTL.
	JobHealthStatsCacheTTL time.Duration `env:"JOB_HEALTH_STATS_CACHE_TTL" default:"5m"`
	JobDepsCacheTTL        time.Duration `env:"JOB_DEPS_CACHE_TTL" default:"5m"`
	StatusReadModelTTL     time.Duration `env:"CACHE_STATUS_READMODEL_TTL" default:"5m"`
	SharedDedupeTTL        time.Duration `env:"CACHE_SHARED_DEDUPE_TTL" default:"10m"`
	DefaultRunTTLSecs      int           `env:"DEFAULT_RUN_TTL_SECS" default:"0"`
	MaxResultSize          int64         `env:"MAX_RESULT_SIZE" default:"1048576"`
	MigrationMode          string        `env:"MIGRATION_MODE" default:"auto"`
	MigrationLockTimeout   time.Duration `env:"MIGRATION_LOCK_TIMEOUT" default:"30s"`
	MaxSnoozeCount         int           `env:"MAX_SNOOZE_COUNT" default:"50"`
	DebouncePollerInterval time.Duration `env:"DEBOUNCE_POLLER_INTERVAL" default:"1s"`
	BatchFlushInterval     time.Duration `env:"BATCH_FLUSH_INTERVAL" default:"1s"`
	WebhookRequireTLS      bool          `env:"WEBHOOK_REQUIRE_TLS" default:"false"`
	EndpointRequireTLS     bool          `env:"ENDPOINT_REQUIRE_TLS" default:"false"`
	AllowPrivateEndpoints  bool          `env:"ALLOW_PRIVATE_ENDPOINTS" default:"false"`
	DefaultRegion          string        `env:"DEFAULT_REGION" default:"iad"`
	ExternalAPIURL         string        `env:"EXTERNAL_API_URL"`

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
	StripeBusinessMonthlyPriceID         string `env:"STRIPE_BUSINESS_MONTHLY_PRICE_ID"`
	StripeBusinessYearlyPriceID          string `env:"STRIPE_BUSINESS_YEARLY_PRICE_ID"`
	StripeEnterpriseStarterYearlyPriceID string `env:"STRIPE_ENTERPRISE_STARTER_YEARLY_PRICE_ID"`
	StripeEnterpriseGrowthYearlyPriceID  string `env:"STRIPE_ENTERPRISE_GROWTH_YEARLY_PRICE_ID"`
	StripeEnterpriseLargeYearlyPriceID   string `env:"STRIPE_ENTERPRISE_LARGE_YEARLY_PRICE_ID"`
	StripeMeterID                        string `env:"STRIPE_METER_ID"`
	BillingEnforcementEnabled            bool   `env:"BILLING_ENFORCEMENT_ENABLED" default:"false"`
	// BillingEntitlementsAuthoritative governs whether the Enforcer reads
	// the persisted entitlements snapshot on the hot path (true, default)
	// or always recomputes from the catalog + addons pipeline (false).
	// Operators can flip this to false as an escape hatch if a bad
	// migration/backfill writes corrupt snapshots.
	BillingEntitlementsAuthoritative bool `env:"BILLING_ENTITLEMENTS_AUTHORITATIVE" default:"true"`

	// Orchestration-only tier price IDs (new billing model).
	// Set these to the Stripe Price IDs for each plan's flat monthly subscription.
	StripeStarterPriceID    string `env:"STRIPE_STARTER_PRICE_ID"`
	StripeProPriceID        string `env:"STRIPE_PRO_PRICE_ID"`
	StripeScalePriceID      string `env:"STRIPE_SCALE_PRICE_ID"`
	StripeEnterprisePriceID string `env:"STRIPE_ENTERPRISE_PRICE_ID"`
	// Overage meter price IDs — one per paid tier, used for metered billing.
	StripeStarterOveragePriceID string `env:"STRIPE_STARTER_OVERAGE_PRICE_ID"`
	StripeProOveragePriceID     string `env:"STRIPE_PRO_OVERAGE_PRICE_ID"`
	StripeScaleOveragePriceID   string `env:"STRIPE_SCALE_OVERAGE_PRICE_ID"`
	// Prometheus uptime source for the SLA credit calculator. When
	// PrometheusQueryURL is unset the SLACalculator falls back to a
	// 100% StaticUptimeSource (no breaches), which keeps community /
	// dev deployments quiet. The default query is service-level
	// (Strait's `up` metric) — swap it in operators' env when a
	// per-tenant aggregation is plumbed in.
	PrometheusQueryURL    string `env:"PROMETHEUS_QUERY_URL"`
	PrometheusUptimeQuery string `env:"PROMETHEUS_UPTIME_QUERY" default:"avg_over_time(up{job=\"strait\"}[30d]) * 100"`

	// Resend email integration
	ResendAPIKey    string `env:"RESEND_API_KEY"`
	ResendFromEmail string `env:"RESEND_FROM_EMAIL" default:"noreply@strait.dev"`

	// PostHog product analytics (server-side revenue events)
	PostHogAPIKey string `env:"POSTHOG_API_KEY"`
	PostHogHost   string `env:"POSTHOG_HOST" default:"https://us.i.posthog.com"`

	// Sentry error tracking
	SentryDSN                     string  `env:"SENTRY_DSN"`
	SentryEnvironment             string  `env:"SENTRY_ENVIRONMENT" default:"development"`
	SentryTracesSampleRate        float64 `env:"SENTRY_TRACES_SAMPLE_RATE" default:"0.1"`
	SentryRelease                 string  `env:"SENTRY_RELEASE"`
	SentryDebug                   bool    `env:"SENTRY_DEBUG" default:"false"`
	SentryMaxBreadcrumbs          int     `env:"SENTRY_MAX_BREADCRUMBS" default:"100"`
	SentryMaxSpans                int     `env:"SENTRY_MAX_SPANS" default:"1000"`
	SentryMaxErrorDepth           int     `env:"SENTRY_MAX_ERROR_DEPTH" default:"100"`
	SentryStrictTraceContinuation bool    `env:"SENTRY_STRICT_TRACE_CONTINUATION" default:"false"`
	SentrySchedulerCheckIns       bool    `env:"SENTRY_SCHEDULER_CHECKINS" default:"false"`
	SentrySchedulerCheckInPrefix  string  `env:"SENTRY_SCHEDULER_CHECKIN_PREFIX" default:"strait-scheduler"`

	// Pyroscope continuous profiling
	PyroscopeEndpoint  string `env:"PYROSCOPE_ENDPOINT"`
	PyroscopeAuthToken string `env:"PYROSCOPE_AUTH_TOKEN"`

	// Debug tools
	ProfilingEnabled            bool     `env:"STRAIT_PROFILING_ENABLED" default:"false"`
	ProfilingAPIEnabled         bool     `env:"STRAIT_PROFILING_API_ENABLED" default:"true"`
	ProfilingManagementEnabled  bool     `env:"STRAIT_PROFILING_MANAGEMENT_ENABLED" default:"false"`
	ProfilingManagementBindAddr string   `env:"STRAIT_PROFILING_MANAGEMENT_BIND_ADDR" default:"127.0.0.1"`
	ProfilingManagementPort     int      `env:"STRAIT_PROFILING_MANAGEMENT_PORT" default:"18080"`
	ProfilingMutexFraction      int      `env:"STRAIT_PROFILING_MUTEX_FRACTION" default:"100"`
	ProfilingBlockRate          int      `env:"STRAIT_PROFILING_BLOCK_RATE" default:"100000"`
	ProfilingSecret             string   `env:"STRAIT_PROFILING_SECRET"`
	ProfilingAllowedCIDRs       []string `env:"STRAIT_PROFILING_ALLOWED_CIDRS"`
	DebugStatsviz               bool     `env:"DEBUG_STATSVIZ" default:"false"`

	// Edition is determined at compile time via build tags (community vs cloud).
	// This field exists for config logging but is ignored by domain.ParseEdition.
	Edition string `env:"STRAIT_EDITION" default:"community"`

	// gRPC server settings.
	GRPCEnabled              bool          `env:"GRPC_ENABLED" default:"true"`
	GRPCBindAddr             string        `env:"GRPC_BIND_ADDR" default:"127.0.0.1"`
	GRPCPort                 int           `env:"GRPC_PORT" default:"50051"`
	GRPCAllowPlaintext       bool          `env:"GRPC_ALLOW_PLAINTEXT" default:"false"`
	GRPCTLSCertPath          string        `env:"GRPC_TLS_CERT_PATH"`
	GRPCTLSKeyPath           string        `env:"GRPC_TLS_KEY_PATH"`
	GRPCKeepaliveTime        time.Duration `env:"GRPC_KEEPALIVE_TIME" default:"30s"`
	GRPCKeepaliveTimeout     time.Duration `env:"GRPC_KEEPALIVE_TIMEOUT" default:"10s"`
	GRPCPubsubStartupTimeout time.Duration `env:"GRPC_PUBSUB_STARTUP_TIMEOUT" default:"30s"`

	// gRPC Worker connection management.
	WorkerHeartbeatTimeout        time.Duration `env:"WORKER_HEARTBEAT_TIMEOUT" default:"30s"`
	WorkerDBSyncInterval          time.Duration `env:"WORKER_DB_SYNC_INTERVAL" default:"15s"`
	WorkerDisconnectSweepInterval time.Duration `env:"WORKER_DISCONNECT_SWEEP_INTERVAL" default:"30s"`
	WorkerDisconnectAckTimeout    time.Duration `env:"WORKER_DISCONNECT_ACK_TIMEOUT" default:"5s"`
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	var cfg Config

	loader := aconfig.LoaderFor(&cfg, aconfig.Config{
		SkipFiles: true,
		SkipFlags: true,
	})
	if err := loader.Load(); err != nil {
		return nil, fmt.Errorf("load config: %w", err)
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

	if err := validateLoaded(&cfg); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
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

// validateLoaded runs the post-load validation gauntlet on a populated
// Config: required fields, enumerated values, URL parsing, edition gating,
// CORS policy, and audit subsystem invariants. It mutates cfg in two
// well-defined cases to match the pre-refactor behavior of Load. Returns a
// *domain.ConfigError pinpointing the offending field, or nil on success.
func validateLoaded(cfg *Config) error {
	validators := []func(*Config) error{
		validateRequiredConfig,
		validateProfilingConfig,
		validateAuthConfig,
		validateRedisConfig,
		validateMigrationConfig,
		validateDatabaseConfig,
		validateSequinConfig,
		validateClickHouseConfig,
		validateEncryptionConfig,
		validateCORSConfig,
		validateAuditConfig,
		validateWorkerStreamConfig,
	}
	for _, validate := range validators {
		if err := validate(cfg); err != nil {
			return err
		}
	}
	return nil
}

func validateRequiredConfig(cfg *Config) error {
	if cfg.DatabaseURL == "" {
		return &domain.ConfigError{Field: "DATABASE_URL", Message: "is required"}
	}
	if cfg.InternalSecret == "" {
		return &domain.ConfigError{Field: "INTERNAL_SECRET", Message: "is required"}
	}
	if len(cfg.InternalSecret) < 16 {
		return &domain.ConfigError{Field: "INTERNAL_SECRET", Message: "must be at least 16 characters"}
	}
	return nil
}

func validateAuthConfig(cfg *Config) error {
	if len(cfg.JWTSigningKey) < 32 {
		return &domain.ConfigError{Field: "JWT_SIGNING_KEY", Message: "must be at least 32 characters"}
	}
	if cfg.OIDCEnabled {
		if cfg.OIDCIssuer == "" {
			return &domain.ConfigError{Field: "OIDC_ISSUER", Message: "is required when OIDC is enabled"}
		}
		if cfg.OIDCAudience == "" {
			return &domain.ConfigError{Field: "OIDC_AUDIENCE", Message: "is required when OIDC is enabled"}
		}
		if cfg.OIDCPublicKeyPEM == "" {
			return &domain.ConfigError{Field: "OIDC_PUBLIC_KEY_PEM", Message: "is required when OIDC is enabled"}
		}
	}
	return nil
}

func validateRedisConfig(cfg *Config) error {
	if cfg.RedisURL != "" {
		u, err := url.Parse(cfg.RedisURL)
		if err != nil || (u.Scheme != "redis" && u.Scheme != "rediss") {
			return &domain.ConfigError{Field: "REDIS_URL", Message: "must be a valid redis:// or rediss:// URL"}
		}
	}
	if cfg.RedisURL == "" && (cfg.RedisSentinelMaster == "" || len(cfg.RedisSentinelAddrs) == 0) {
		return &domain.ConfigError{Field: "REDIS_URL", Message: "is required unless REDIS_SENTINEL_MASTER and REDIS_SENTINEL_ADDRS are configured"}
	}
	return nil
}

func validateMigrationConfig(cfg *Config) error {
	switch cfg.MigrationMode {
	case "auto", "manual", "validate":
		return nil
	default:
		return &domain.ConfigError{Field: "MIGRATION_MODE", Message: "must be auto, manual, or validate"}
	}
}

func validateDatabaseConfig(cfg *Config) error {
	return ValidateDatabaseSSLMode(cfg.DatabaseURL, cfg.DeploymentEnvironment)
}

// insecureSSLModes are libpq sslmode values that permit (or silently fall back
// to) an unencrypted connection. "prefer" — the effective default when sslmode
// is unset — downgrades to plaintext when the server does not advertise TLS.
var insecureSSLModes = map[string]bool{
	"disable": true,
	"allow":   true,
	"prefer":  true,
}

// databaseSSLMode extracts the (lowercased) sslmode from a postgres URL or a
// keyword/value DSN. It returns "" when sslmode is not specified at all.
func databaseSSLMode(databaseURL string) string {
	if u, err := url.Parse(databaseURL); err == nil && u.Scheme != "" {
		return strings.ToLower(strings.TrimSpace(u.Query().Get("sslmode")))
	}
	for _, field := range strings.Fields(databaseURL) {
		if k, v, ok := strings.Cut(field, "="); ok && strings.EqualFold(strings.TrimSpace(k), "sslmode") {
			return strings.ToLower(strings.TrimSpace(v))
		}
	}
	return ""
}

// ValidateDatabaseSSLMode rejects database URLs that would permit an
// unencrypted connection outside development. An unset sslmode is rejected as
// well: libpq treats it as "prefer", which silently uses a plaintext
// connection when the server does not advertise TLS.
func ValidateDatabaseSSLMode(databaseURL, environment string) error {
	if IsRelaxedDeploymentEnvironment(environment) {
		if databaseSSLMode(databaseURL) == "disable" {
			slog.Warn("DATABASE_URL has sslmode=disable; connections are not encrypted")
		}
		return nil
	}
	switch sslMode := databaseSSLMode(databaseURL); {
	case sslMode == "":
		return &domain.ConfigError{Field: "DATABASE_URL", Message: "sslmode must be set to require, verify-ca, or verify-full in non-development environments (an unset sslmode defaults to 'prefer' and can use an unencrypted connection)"}
	case insecureSSLModes[sslMode]:
		return &domain.ConfigError{Field: "DATABASE_URL", Message: "sslmode=" + sslMode + " is not allowed in non-development environments; use require, verify-ca, or verify-full"}
	default:
		return nil
	}
}

func validateSequinConfig(cfg *Config) error {
	if cfg.SequinBaseURL == "" {
		return &domain.ConfigError{Field: "SEQUIN_BASE_URL", Message: "is required"}
	}
	if u, err := url.Parse(cfg.SequinBaseURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return &domain.ConfigError{Field: "SEQUIN_BASE_URL", Message: "must be a valid HTTP(S) URL"}
	}
	if cfg.SequinConsumerName == "" {
		return &domain.ConfigError{Field: "SEQUIN_CONSUMER_NAME", Message: "is required"}
	}
	if cfg.SequinAPIToken == "" {
		return &domain.ConfigError{Field: "SEQUIN_API_TOKEN", Message: "is required"}
	}
	if cfg.SequinBatchSize <= 0 {
		return &domain.ConfigError{Field: "SEQUIN_BATCH_SIZE", Message: "must be > 0"}
	}
	if cfg.SequinWaitTimeMs <= 0 {
		return &domain.ConfigError{Field: "SEQUIN_WAIT_TIME_MS", Message: "must be > 0"}
	}
	// Gate on the deployment environment (STRAIT_ENV), not SENTRY_ENVIRONMENT.
	// SENTRY_ENVIRONMENT is an observability label, not a security boundary, so
	// keying CDC webhook authentication on it let an operator silently disable
	// signature verification in production by setting SENTRY_ENVIRONMENT=development.
	if cfg.SequinWebhookSecret == "" && !IsRelaxedDeploymentEnvironment(cfg.DeploymentEnvironment) {
		return &domain.ConfigError{Field: "SEQUIN_WEBHOOK_SECRET", Message: "is required in non-development environments"}
	}
	return nil
}

func validateClickHouseConfig(cfg *Config) error {
	if cfg.ClickHouseEnabled && cfg.ClickHouseURL == "" {
		return &domain.ConfigError{Field: "CLICKHOUSE_URL", Message: "is required when CLICKHOUSE_ENABLED=true"}
	}
	return nil
}

func validateEncryptionConfig(cfg *Config) error {
	if cfg.EncryptionKey == "" && cfg.SecretEncryptionKey == "" {
		slog.Warn("neither ENCRYPTION_KEY nor SECRET_ENCRYPTION_KEY is set; secret encryption will be unavailable")
	}
	return nil
}

func validateCORSConfig(cfg *Config) error {
	for _, origin := range cfg.CORSAllowedOrigins {
		if origin == "*" && cfg.CORSAllowCredentials {
			return &domain.ConfigError{
				Field:   "CORS_ALLOWED_ORIGINS",
				Message: "wildcard origin (*) is not allowed when CORS_ALLOW_CREDENTIALS is true",
			}
		}
		if origin == "*" {
			if !IsRelaxedDeploymentEnvironment(cfg.DeploymentEnvironment) {
				return &domain.ConfigError{
					Field:   "CORS_ALLOWED_ORIGINS",
					Message: "wildcard origin (*) is not allowed in non-development environments",
				}
			}
			slog.Warn("CORS_ALLOWED_ORIGINS is set to wildcard (*); consider restricting to specific origins in production")
		}
	}
	return nil
}

func IsRelaxedDeploymentEnvironment(environment string) bool {
	switch strings.ToLower(strings.TrimSpace(environment)) {
	case "development", "dev", "test":
		return true
	default:
		return false
	}
}

func validateAuditConfig(cfg *Config) error {
	if cfg.AuditRetentionDefaultDays < 0 {
		return &domain.ConfigError{Field: "AUDIT_RETENTION_DEFAULT_DAYS", Message: "must be >= 0"}
	}
	if cfg.AuditRetentionDefaultDays > domain.MaxAuditRetentionDays {
		return &domain.ConfigError{
			Field:   "AUDIT_RETENTION_DEFAULT_DAYS",
			Message: fmt.Sprintf("must be <= %d", domain.MaxAuditRetentionDays),
		}
	}
	if cfg.AuditAsyncBufferSize < 256 {
		return &domain.ConfigError{Field: "AUDIT_ASYNC_BUFFER_SIZE", Message: "must be >= 256"}
	}
	if cfg.AuditDLQReclaimBatch <= 0 {
		return &domain.ConfigError{Field: "AUDIT_DLQ_RECLAIM_BATCH", Message: "must be > 0"}
	}
	if cfg.AuditDLQMaxAgeDays < 0 {
		return &domain.ConfigError{Field: "AUDIT_DLQ_MAX_AGE_DAYS", Message: "must be >= 0 (0 disables retention sweep)"}
	}
	if cfg.AuditDLQMaxAgeDays > domain.MaxAuditRetentionDays {
		return &domain.ConfigError{
			Field:   "AUDIT_DLQ_MAX_AGE_DAYS",
			Message: fmt.Sprintf("must be <= %d", domain.MaxAuditRetentionDays),
		}
	}
	if cfg.AuditDLQMaxReclaimAttempts < 0 {
		return &domain.ConfigError{Field: "AUDIT_DLQ_MAX_RECLAIM_ATTEMPTS", Message: "must be >= 0 (0 disables the cap)"}
	}
	if cfg.AuditSIEMEndpoint != "" && cfg.AuditSIEMAuthToken == "" {
		return &domain.ConfigError{Field: "AUDIT_SIEM_AUTH_TOKEN", Message: "is required when AUDIT_SIEM_ENDPOINT is set"}
	}
	// Reject userinfo (https://user:password@host/...) in the SIEM
	// endpoint. Sanitization strips it before logging but configuring
	// it in the first place is a footgun: the credential lives in the
	// process environment in plaintext and every operator who can read
	// the config sees it. Require the Authorization Bearer token path.
	if cfg.AuditSIEMEndpoint != "" {
		u, err := url.Parse(cfg.AuditSIEMEndpoint)
		if err != nil {
			return &domain.ConfigError{Field: "AUDIT_SIEM_ENDPOINT", Message: fmt.Sprintf("unparseable URL: %v", err)}
		}
		if u.User != nil {
			return &domain.ConfigError{
				Field:   "AUDIT_SIEM_ENDPOINT",
				Message: "must not contain userinfo (user:password@host) — use AUDIT_SIEM_AUTH_TOKEN for credentials",
			}
		}
	}
	return nil
}

func validateWorkerStreamConfig(cfg *Config) error {
	if cfg.WorkerDBSyncInterval <= cfg.HeartbeatInterval {
		return &domain.ConfigError{
			Field:   "WORKER_DB_SYNC_INTERVAL",
			Message: fmt.Sprintf("must be > HEARTBEAT_INTERVAL (%v), got %v", cfg.HeartbeatInterval, cfg.WorkerDBSyncInterval),
		}
	}
	if cfg.WorkerDBSyncInterval >= cfg.StaleThreshold {
		return &domain.ConfigError{
			Field:   "WORKER_DB_SYNC_INTERVAL",
			Message: fmt.Sprintf("must be < STALE_THRESHOLD (%v), got %v", cfg.StaleThreshold, cfg.WorkerDBSyncInterval),
		}
	}
	if cfg.WorkerDisconnectAckTimeout <= 0 {
		return &domain.ConfigError{Field: "WORKER_DISCONNECT_ACK_TIMEOUT", Message: "must be > 0"}
	}
	if cfg.GRPCPubsubStartupTimeout <= 0 {
		return &domain.ConfigError{Field: "GRPC_PUBSUB_STARTUP_TIMEOUT", Message: "must be > 0"}
	}

	return nil
}

func validateProfilingConfig(cfg *Config) error {
	if cfg.ProfilingSecret != "" && len(cfg.ProfilingSecret) < 16 {
		return &domain.ConfigError{Field: "STRAIT_PROFILING_SECRET", Message: "must be at least 16 characters"}
	}
	if bad := firstInvalidCIDREntry(cfg.ProfilingAllowedCIDRs); bad != "" {
		return &domain.ConfigError{Field: "STRAIT_PROFILING_ALLOWED_CIDRS", Message: fmt.Sprintf("contains invalid CIDR/IP entry %q", bad)}
	}
	if cfg.ProfilingEnabled && !cfg.ProfilingAPIEnabled && !cfg.ProfilingManagementEnabled {
		return &domain.ConfigError{Field: "STRAIT_PROFILING_ENABLED", Message: "requires at least one profiling listener"}
	}
	if cfg.ProfilingManagementEnabled && (cfg.ProfilingManagementPort <= 0 || cfg.ProfilingManagementPort > 65535) {
		return &domain.ConfigError{Field: "STRAIT_PROFILING_MANAGEMENT_PORT", Message: "must be between 1 and 65535"}
	}
	if cfg.ProfilingMutexFraction < 0 {
		return &domain.ConfigError{Field: "STRAIT_PROFILING_MUTEX_FRACTION", Message: "must be >= 0"}
	}
	if cfg.ProfilingBlockRate < 0 {
		return &domain.ConfigError{Field: "STRAIT_PROFILING_BLOCK_RATE", Message: "must be >= 0"}
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
		"SentryEnvironment":      c.SentryEnvironment,
		"SentryTracesSampleRate": c.SentryTracesSampleRate,
		"SentryRelease":          c.SentryRelease,
		"DefaultAPIKeyRateLimit": c.DefaultAPIKeyRateLimit,
		"DatabaseURL":            "[REDACTED]",
		"RedisURL":               "[REDACTED]",
		"InternalSecret":         "[REDACTED]",
		"ProfilingSecret":        "[REDACTED]",
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

func firstInvalidCIDREntry(entries []string) string {
	for _, raw := range entries {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}
		if _, _, err := net.ParseCIDR(entry); err == nil {
			continue
		}
		if net.ParseIP(entry) != nil {
			continue
		}
		return entry
	}
	return ""
}
