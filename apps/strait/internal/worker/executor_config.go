package worker

import (
	"log/slog"
	"net/http"
	"time"

	"strait/internal/billing"
	straitcache "strait/internal/cache"
	"strait/internal/pubsub"
	"strait/internal/queue"
	"strait/internal/store"
	"strait/internal/telemetry"

	"github.com/redis/go-redis/v9"
)

// ExecutorConfig holds configuration for the Executor.
type ExecutorConfig struct {
	Pool                       *Pool
	Queue                      queue.Queue
	Wake                       <-chan struct{}
	ConcurrencyLimit           ConcurrencyLimitProvider
	Store                      ExecutorStore
	TxPool                     store.TxBeginner
	Publisher                  pubsub.Publisher
	HTTPClient                 *http.Client
	PollInterval               time.Duration
	HeartbeatInterval          time.Duration
	Metrics                    *telemetry.Metrics
	WorkflowCallback           WorkflowCallback
	Partitions                 []string
	PartitionWeights           string
	ExecutorHTTPTimeout        time.Duration
	ExecutorIdleConnTimeout    time.Duration
	AllowPrivateEndpoints      bool
	WebhookMaxAttempts         int
	ExecutionTraceMode         string
	MaxDequeueBatchSize        int
	DefaultJobMaxConcurrency   int
	MemoryPressureThresholdPct float64
	JobCacheTTL                time.Duration
	VersionCacheTTL            time.Duration
	RunVersionCacheTTL         time.Duration
	JobHealthCacheTTL          time.Duration
	RedisClient                redis.Cmdable
	CacheBus                   *straitcache.Bus
	CacheRegistry              *straitcache.Registry
	MaxSnoozeCount             int
	JWTSigningKey              string
	ExternalAPIURL             string
	DefaultRegion              string
	Mode                       string
	Version                    string
	WorkflowLookup             WorkflowLookup
	WorkflowTriggerer          WorkflowTriggerer
	JobLookup                  JobLookup
	JobEnqueuer                JobEnqueuer
	BillingEnforcer            *billing.Enforcer            // Optional: org-level billing enforcement (cloud only).
	StripeUsageReporter        *billing.StripeUsageReporter // Optional: Stripe usage event reporting (cloud only).
	RunCostRecorder            *billing.RunCostRecorder     // Optional: flat per-run cost recording (cloud only).
	SecretDecryptor            SecretDecryptor              // Optional: decrypts encrypted endpoint signing secrets.
	// QueueSnapshotter provides the set of queue names with active workers on
	// this replica. When set, the poll loop performs a second dequeue pass
	// for worker-mode runs filtered to those queues.
	// Typically injected from grpc.ConnectionRegistry via the QueueSnapshotter
	// interface to avoid a circular import.
	QueueSnapshotter QueueSnapshotter
	// WorkerDispatcher handles gRPC-based dispatch for ExecutionModeWorker runs.
	// Injected from the gRPC server to avoid a circular import.
	WorkerDispatcher WorkerRunDispatcher
	// DegradedPollInterval is the shortened poll interval used when the
	// queue notifier enters degraded mode (LISTEN disconnected for too long).
	// Zero/negative falls back to 1 second.
	DegradedPollInterval time.Duration
	// Degraded provides a channel that closes when the queue notifier enters
	// degraded mode. The executor re-invokes Degraded() on each recovery to
	// obtain the fresh channel, avoiding stale-channel re-arm. Nil means no
	// degraded-mode support.
	Degraded queue.DegradedNotifier
	// DBCircuitConfig configures the circuit breaker for the
	// dequeue hot path. Zero values fall back to defaults.
	DBCircuitConfig queue.DBCircuitConfig
	// EventChannelSize overrides the default (1024) buffered capacity of the
	// run-lifecycle event channel. Zero/negative values fall back to the
	// default. Values below 16 are clamped to 16.
	EventChannelSize int
}

const (
	defaultCircuitFailureThreshold = 5
	defaultCircuitOpenDuration     = time.Minute
	defaultDegradedPollInterval    = time.Second
)

func resolveDegradedPollInterval(d time.Duration) time.Duration {
	if d <= 0 {
		return defaultDegradedPollInterval
	}
	return d
}

func NewExecutor(cfg ExecutorConfig) *Executor {
	httpClient := resolveExecutorHTTPClient(cfg)

	whMaxAttempts := cfg.WebhookMaxAttempts
	if whMaxAttempts <= 0 {
		whMaxAttempts = 3
	}

	cacheDeps := workerCacheDeps{
		Redis:    cfg.RedisClient,
		Bus:      cfg.CacheBus,
		Registry: cfg.CacheRegistry,
	}
	jobCache := newTierJobCache(cfg.JobCacheTTL, cacheDeps)
	jobVersionCache := newTierVersionedJobCache(cfg.VersionCacheTTL, cacheDeps)
	runVersionCache := newTierWorkflowRunVersionCache(cfg.RunVersionCacheTTL, cacheDeps)
	stepsVersionCache := newTierWorkflowStepsVersionCache(cfg.VersionCacheTTL, cacheDeps)
	jobHealthCache := newTierJobHealthCache(cfg.JobHealthCacheTTL, cacheDeps)

	return &Executor{
		pool:                     cfg.Pool,
		concurrencyLimit:         cfg.ConcurrencyLimit,
		queue:                    cfg.Queue,
		wake:                     cfg.Wake,
		store:                    cfg.Store,
		txPool:                   cfg.TxPool,
		httpClient:               httpClient,
		pollInterval:             cfg.PollInterval,
		heartbeat:                NewHeartbeatSender(cfg.Store, cfg.HeartbeatInterval),
		publisher:                cfg.Publisher,
		metrics:                  cfg.Metrics,
		workflowCallback:         cfg.WorkflowCallback,
		partitionCycle:           buildPartitionCycle(cfg.Partitions, cfg.PartitionWeights),
		bulkhead:                 NewShardedBulkhead(cfg.DefaultJobMaxConcurrency),
		circuitThreshold:         defaultCircuitFailureThreshold,
		circuitOpenFor:           defaultCircuitOpenDuration,
		logger:                   slog.Default(),
		webhookMaxRetry:          whMaxAttempts,
		executionTraceMode:       normalizeExecutionTraceMode(cfg.ExecutionTraceMode),
		eventCh:                  make(chan runEventEnvelope, resolveEventChannelSize(cfg.EventChannelSize)),
		eventChannelSize:         resolveEventChannelSize(cfg.EventChannelSize),
		saturationLastWarn:       make(map[eventChannelKind]time.Time),
		maxDequeueBatchSize:      cfg.MaxDequeueBatchSize,
		defaultJobMaxConcurrency: cfg.DefaultJobMaxConcurrency,
		jobCache:                 jobCache,
		jobVersionCache:          jobVersionCache,
		runVersionCache:          runVersionCache,
		stepsVersionCache:        stepsVersionCache,
		jobHealthCache:           jobHealthCache,
		memoryPressureThreshold:  cfg.MemoryPressureThresholdPct,
		maxSnoozeCount:           cfg.MaxSnoozeCount,
		jwtSigningKey:            cfg.JWTSigningKey,
		externalAPIURL:           cfg.ExternalAPIURL,
		defaultRegion:            cfg.DefaultRegion,
		mode:                     cfg.Mode,
		version:                  cfg.Version,
		billingEnforcer:          cfg.BillingEnforcer,
		stripeUsageReporter:      cfg.StripeUsageReporter,
		runCostRecorder:          cfg.RunCostRecorder,
		secretDecryptor:          cfg.SecretDecryptor,
		healthScorer:             NewHealthScorer(cfg.Store),
		onCompleteTrigger: NewOnCompleteTrigger(
			cfg.WorkflowLookup,
			cfg.WorkflowTriggerer,
			cfg.JobLookup,
			cfg.JobEnqueuer,
			slog.Default(),
		),
		stop:                 make(chan struct{}),
		done:                 make(chan struct{}),
		degradedPollInterval: resolveDegradedPollInterval(cfg.DegradedPollInterval),
		degraded:             cfg.Degraded,
		dbCircuit:            queue.NewDBCircuit(cfg.DBCircuitConfig),
		queueSnapshotter:     cfg.QueueSnapshotter,
		workerDispatcher:     cfg.WorkerDispatcher,
		queueMetrics:         resolveQueueMetrics(),
	}
}

// resolveQueueMetrics fetches the singleton queue metrics handle once at
// executor construction. Returns nil when the metrics subsystem failed
// to initialise so hot-path callers can nil-guard cheaply instead of
// re-entering the sync.Once on every emit.
func resolveQueueMetrics() *queue.QueueMetrics {
	qm, err := queue.Metrics()
	if err != nil {
		return nil
	}
	return qm
}
