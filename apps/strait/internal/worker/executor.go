package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"strait/internal/billing"
	straitcache "strait/internal/cache"
	"strait/internal/domain"
	"strait/internal/httputil"
	"strait/internal/pubsub"
	"strait/internal/queue"
	"strait/internal/store"
	"strait/internal/telemetry"

	"github.com/getsentry/sentry-go"
	"github.com/redis/go-redis/v9"
	"github.com/sourcegraph/conc"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// ExecutorStore is the subset of store operations needed by Executor.
type ExecutorStore interface {
	GetJob(ctx context.Context, id string) (*domain.Job, error)
	GetJobAtVersion(ctx context.Context, jobID string, version int) (*domain.Job, error)
	GetWorkflowStepRun(ctx context.Context, id string) (*domain.WorkflowStepRun, error)
	GetWorkflowRun(ctx context.Context, id string) (*domain.WorkflowRun, error)
	ListStepsByWorkflowVersion(ctx context.Context, workflowID string, version int) ([]domain.WorkflowStep, error)
	ListJobSecretsByJob(ctx context.Context, jobID, environment string) ([]domain.JobSecret, error)
	UpdateRunStatus(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error
	SnoozeRunWithLock(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error
	UpdateHeartbeat(ctx context.Context, id string) error
	ScheduleRetry(ctx context.Context, runID string, at time.Time, attempt int) error
	ClearRetry(ctx context.Context, runID string) error
	CanDispatchEndpoint(ctx context.Context, endpointURL string, now time.Time) (bool, *time.Time, error)
	RecordEndpointCircuitFailure(
		ctx context.Context,
		endpointURL string,
		now time.Time,
		threshold int,
		openDuration time.Duration,
	) error
	RecordEndpointCircuitSuccess(ctx context.Context, endpointURL string) error
	GetJobHealthStats(ctx context.Context, jobID string, since time.Time) (*store.JobHealthStats, error)
	GetResolvedEnvironmentVariables(ctx context.Context, id string) (map[string]string, error)
	GetLatestCheckpoint(ctx context.Context, runID string) (*domain.RunCheckpoint, error)
	GetRun(ctx context.Context, id string) (*domain.JobRun, error)
	GetProjectQuota(ctx context.Context, projectID string) (*store.ProjectQuota, error)
	InsertEvent(ctx context.Context, event *domain.RunEvent) error
	GetEndpointHealthScore(ctx context.Context, endpointURL string) (*domain.EndpointHealthScore, error)
	UpsertEndpointHealthScore(ctx context.Context, score *domain.EndpointHealthScore) error
	AtomicRecordHealthResult(
		ctx context.Context,
		endpointURL string,
		successVal, timeoutVal, latencyVal, alpha float64,
		weightSuccess, weightTimeout, weightLatency float64,
		lastLatencyMs float64,
	) (*domain.EndpointHealthScore, error)
	CountExecutingRunsByOrg(ctx context.Context, orgID string) (int, error)
}

type existingRunEnqueuer interface {
	EnqueueExisting(ctx context.Context, run *domain.JobRun) error
}

type executionPolicy struct {
	maxAttempts      int
	timeoutSecs      int
	retryBackoff     domain.RetryBackoffPolicy
	retryInitialSecs int
	retryMaxSecs     int
}

type jobVersionKey struct {
	JobID   string
	Version int
}

type workflowStepsVersionKey struct {
	WorkflowID string
	Version    int
}

type workflowRunVersion struct {
	WorkflowID string
	Version    int
}

type jobHealthKey struct {
	JobID  string
	Bucket int64
}

type workerCacheDeps struct {
	Redis    redis.Cmdable
	Bus      *straitcache.Bus
	Registry *straitcache.Registry
}

type executorJobCache interface {
	Get(context.Context, string) (*domain.Job, error)
	Load(context.Context, string, straitcache.LoadFunc[string, *domain.Job]) (*domain.Job, error)
	Set(context.Context, string, *domain.Job) error
	Delete(context.Context, string) error
}

type executorVersionedJobCache interface {
	Load(context.Context, jobVersionKey, straitcache.LoadFunc[jobVersionKey, *domain.Job]) (*domain.Job, error)
}

type tierJobCache struct {
	tier *straitcache.Tier[string, *domain.Job]
	bus  *straitcache.Bus
}

const (
	workerJobCacheNamespace                = "worker_job"
	workerJobVersionCacheNamespace         = "worker_job_version"
	workerWorkflowRunVersionCacheNamespace = "worker_workflow_run_version"
	workerWorkflowStepsCacheNamespace      = "worker_workflow_steps_version"
	workerJobHealthCacheNamespace          = "worker_job_health"
)

func newTierJobCache(ttl time.Duration, depsOpt ...workerCacheDeps) *tierJobCache {
	if ttl <= 0 {
		return nil
	}
	var deps workerCacheDeps
	if len(depsOpt) > 0 {
		deps = depsOpt[0]
	}
	var l2 straitcache.L2[string, *domain.Job]
	if deps.Redis != nil {
		l2 = straitcache.NewRedisL2[string, *domain.Job](straitcache.RedisL2Config[string, *domain.Job]{
			Client:    deps.Redis,
			Namespace: workerJobCacheNamespace,
		})
	}
	c := &tierJobCache{bus: deps.Bus}
	c.tier = straitcache.NewTier[string, *domain.Job](straitcache.TierConfig[string, *domain.Job]{
		Name:        workerJobCacheNamespace,
		L2:          l2,
		Consistency: straitcache.Strong,
		MaximumSize: 10_000,
		TTL:         ttl,
		TTLJitter:   0.1,
		DisableL1:   l2 != nil,
		DisableL2:   l2 == nil,
		Clone:       cloneJob,
	})
	if deps.Registry != nil {
		deps.Registry.Register(workerJobCacheNamespace, straitcache.UpdatingStringTierHandler[*domain.Job]{Tier: c.tier})
	}
	return c
}

func (c *tierJobCache) Get(ctx context.Context, key string) (*domain.Job, error) {
	if c == nil || c.tier == nil {
		return nil, straitcache.ErrCacheMiss
	}
	return c.tier.Get(ctx, key, nil)
}

func (c *tierJobCache) Load(
	ctx context.Context,
	key string,
	loader straitcache.LoadFunc[string, *domain.Job],
) (*domain.Job, error) {
	if c == nil || c.tier == nil {
		return loader(ctx, key)
	}
	versionedLoader := func(loadCtx context.Context, loadKey string) (straitcache.Versioned[*domain.Job], error) {
		job, err := loader(loadCtx, loadKey)
		if err != nil {
			return straitcache.Versioned[*domain.Job]{}, err
		}
		return straitcache.Versioned[*domain.Job]{Value: job, Version: jobCacheVersion(job)}, nil
	}
	got, err := c.tier.GetConsistentVersioned(ctx, key, 0, versionedLoader)
	if err != nil {
		return nil, err
	}
	return got.Value, nil
}

func (c *tierJobCache) Set(ctx context.Context, key string, job *domain.Job) error {
	if c == nil || c.tier == nil {
		return nil
	}
	_, err := c.tier.StrongWriteThrough(
		ctx,
		workerCachePolicy(workerJobCacheNamespace),
		key,
		key,
		job,
		jobCacheVersion(job),
		c.bus,
	)
	return err
}

func (c *tierJobCache) Delete(ctx context.Context, key string) error {
	if c == nil || c.tier == nil {
		return nil
	}
	return c.tier.StrongInvalidate(
		ctx,
		workerCachePolicy(workerJobCacheNamespace),
		key,
		key,
		workerCacheBarrier(time.Now().UnixNano()),
		c.bus,
	)
}

func workerCachePolicy(namespace string) straitcache.StrongNamespacePolicy {
	return straitcache.StrongNamespacePolicy{Namespace: namespace}
}

func workerCacheBarrier(version int64) straitcache.VersionBarrier {
	return straitcache.VersionBarrier{Version: version}
}

func jobCacheVersion(job *domain.Job) int64 {
	if job == nil {
		return 0
	}
	if job.CacheVersion > 0 {
		return job.CacheVersion
	}
	if !job.UpdatedAt.IsZero() {
		return job.UpdatedAt.UnixNano()
	}
	if job.Version > 0 {
		return int64(job.Version)
	}
	return 1
}

type tierVersionedJobCache struct {
	tier *straitcache.Tier[jobVersionKey, *domain.Job]
}

func newTierVersionedJobCache(ttl time.Duration, depsOpt ...workerCacheDeps) *tierVersionedJobCache {
	if ttl <= 0 {
		return nil
	}
	var deps workerCacheDeps
	if len(depsOpt) > 0 {
		deps = depsOpt[0]
	}
	l2 := newWorkerJobVersionL2(deps.Redis)
	tier := straitcache.NewTier[jobVersionKey, *domain.Job](straitcache.TierConfig[jobVersionKey, *domain.Job]{
		Name:        workerJobVersionCacheNamespace,
		L2:          l2,
		Consistency: straitcache.Immutable,
		MaximumSize: 10_000,
		TTL:         ttl,
		TTLJitter:   0.1,
		DisableL2:   l2 == nil,
		Clone:       cloneJob,
	})
	return &tierVersionedJobCache{tier: tier}
}

func newWorkerJobVersionL2(redis redis.Cmdable) straitcache.L2[jobVersionKey, *domain.Job] {
	if redis == nil {
		return nil
	}
	return straitcache.NewRedisL2[jobVersionKey, *domain.Job](
		straitcache.RedisL2Config[jobVersionKey, *domain.Job]{
			Client:    redis,
			Namespace: workerJobVersionCacheNamespace,
			Key:       workerJobVersionKeyString,
		},
	)
}

func workerJobVersionKeyString(key jobVersionKey) string {
	return fmt.Sprintf("%s\x00%d", key.JobID, key.Version)
}

func (c *tierVersionedJobCache) Load(
	ctx context.Context,
	key jobVersionKey,
	loader straitcache.LoadFunc[jobVersionKey, *domain.Job],
) (*domain.Job, error) {
	if c == nil || c.tier == nil {
		return loader(ctx, key)
	}
	return c.tier.Get(ctx, key, loader)
}

type tierWorkflowRunVersionCache struct {
	tier *straitcache.Tier[string, workflowRunVersion]
}

func newTierWorkflowRunVersionCache(ttl time.Duration, depsOpt ...workerCacheDeps) *tierWorkflowRunVersionCache {
	if ttl <= 0 {
		return nil
	}
	var deps workerCacheDeps
	if len(depsOpt) > 0 {
		deps = depsOpt[0]
	}
	var l2 straitcache.L2[string, workflowRunVersion]
	if deps.Redis != nil {
		l2 = straitcache.NewRedisL2[string, workflowRunVersion](straitcache.RedisL2Config[string, workflowRunVersion]{
			Client:    deps.Redis,
			Namespace: workerWorkflowRunVersionCacheNamespace,
		})
	}
	tier := straitcache.NewTier[string, workflowRunVersion](straitcache.TierConfig[string, workflowRunVersion]{
		Name:        workerWorkflowRunVersionCacheNamespace,
		L2:          l2,
		Consistency: straitcache.Immutable,
		MaximumSize: 100_000,
		TTL:         ttl,
		TTLJitter:   0.1,
		DisableL2:   l2 == nil,
	})
	return &tierWorkflowRunVersionCache{tier: tier}
}

func (c *tierWorkflowRunVersionCache) Load(
	ctx context.Context,
	key string,
	loader straitcache.LoadFunc[string, workflowRunVersion],
) (workflowRunVersion, error) {
	if c == nil || c.tier == nil {
		return loader(ctx, key)
	}
	return c.tier.Get(ctx, key, loader)
}

type tierWorkflowStepsVersionCache struct {
	tier *straitcache.Tier[workflowStepsVersionKey, []domain.WorkflowStep]
}

func newTierWorkflowStepsVersionCache(ttl time.Duration, depsOpt ...workerCacheDeps) *tierWorkflowStepsVersionCache {
	if ttl <= 0 {
		return nil
	}
	var deps workerCacheDeps
	if len(depsOpt) > 0 {
		deps = depsOpt[0]
	}
	l2 := newWorkerWorkflowStepsL2(deps.Redis)
	tierConfig := workerWorkflowStepsTierConfig(ttl, l2)
	tier := straitcache.NewTier[workflowStepsVersionKey, []domain.WorkflowStep](tierConfig)
	return &tierWorkflowStepsVersionCache{tier: tier}
}

func workerWorkflowStepsTierConfig(
	ttl time.Duration,
	l2 straitcache.L2[workflowStepsVersionKey, []domain.WorkflowStep],
) straitcache.TierConfig[workflowStepsVersionKey, []domain.WorkflowStep] {
	return straitcache.TierConfig[workflowStepsVersionKey, []domain.WorkflowStep]{
		Name:          workerWorkflowStepsCacheNamespace,
		L2:            l2,
		Consistency:   straitcache.Immutable,
		MaximumWeight: 100_000,
		Weigher: func(_ workflowStepsVersionKey, steps []domain.WorkflowStep) uint32 {
			if len(steps) == 0 {
				return 1
			}
			if len(steps) > 100_000 {
				return 100_000
			}
			return uint32(len(steps)) // #nosec G115 -- bounded above before conversion.
		},
		TTL:       ttl,
		TTLJitter: 0.1,
		DisableL2: l2 == nil,
		Clone:     domain.CloneWorkflowSteps,
	}
}

func newWorkerWorkflowStepsL2(redis redis.Cmdable) straitcache.L2[workflowStepsVersionKey, []domain.WorkflowStep] {
	if redis == nil {
		return nil
	}
	return straitcache.NewRedisL2[workflowStepsVersionKey, []domain.WorkflowStep](
		straitcache.RedisL2Config[workflowStepsVersionKey, []domain.WorkflowStep]{
			Client:    redis,
			Namespace: workerWorkflowStepsCacheNamespace,
			Key:       workerWorkflowStepsKeyString,
		},
	)
}

func workerWorkflowStepsKeyString(key workflowStepsVersionKey) string {
	return fmt.Sprintf("%s\x00%d", key.WorkflowID, key.Version)
}

func (c *tierWorkflowStepsVersionCache) Load(
	ctx context.Context,
	key workflowStepsVersionKey,
	loader straitcache.LoadFunc[workflowStepsVersionKey, []domain.WorkflowStep],
) ([]domain.WorkflowStep, error) {
	if c == nil || c.tier == nil {
		return loader(ctx, key)
	}
	return c.tier.Get(ctx, key, loader)
}

type tierJobHealthCache struct {
	tier *straitcache.Tier[jobHealthKey, *store.JobHealthStats]
	ttl  time.Duration
}

func newTierJobHealthCache(ttl time.Duration, depsOpt ...workerCacheDeps) *tierJobHealthCache {
	if ttl <= 0 {
		return nil
	}
	var deps workerCacheDeps
	if len(depsOpt) > 0 {
		deps = depsOpt[0]
	}
	l2 := newWorkerJobHealthL2(deps.Redis)
	tierConfig := workerJobHealthTierConfig(ttl, l2)
	return &tierJobHealthCache{
		ttl:  ttl,
		tier: straitcache.NewTier[jobHealthKey, *store.JobHealthStats](tierConfig),
	}
}

func workerJobHealthTierConfig(
	ttl time.Duration,
	l2 straitcache.L2[jobHealthKey, *store.JobHealthStats],
) straitcache.TierConfig[jobHealthKey, *store.JobHealthStats] {
	return straitcache.TierConfig[jobHealthKey, *store.JobHealthStats]{
		Name:        workerJobHealthCacheNamespace,
		L2:          l2,
		Consistency: straitcache.BoundedStaleness,
		MaximumSize: 20_000,
		TTL:         ttl,
		TTLJitter:   0.05,
		DisableL2:   l2 == nil,
		Clone:       cloneJobHealthStats,
	}
}

func cloneJobHealthStats(v *store.JobHealthStats) *store.JobHealthStats {
	if v == nil {
		return nil
	}
	cp := *v
	return &cp
}

func newWorkerJobHealthL2(redis redis.Cmdable) straitcache.L2[jobHealthKey, *store.JobHealthStats] {
	if redis == nil {
		return nil
	}
	return straitcache.NewRedisL2[jobHealthKey, *store.JobHealthStats](
		straitcache.RedisL2Config[jobHealthKey, *store.JobHealthStats]{
			Client:    redis,
			Namespace: workerJobHealthCacheNamespace,
			Key:       workerJobHealthKeyString,
		},
	)
}

func workerJobHealthKeyString(key jobHealthKey) string {
	return fmt.Sprintf("%s\x00%d", key.JobID, key.Bucket)
}

func (c *tierJobHealthCache) Key(jobID string, now time.Time) jobHealthKey {
	bucketSecs := int64(c.ttl.Seconds())
	if bucketSecs <= 0 {
		bucketSecs = 1
	}
	return jobHealthKey{JobID: jobID, Bucket: now.Unix() / bucketSecs}
}

func (c *tierJobHealthCache) Load(
	ctx context.Context,
	key jobHealthKey,
	loader straitcache.LoadFunc[jobHealthKey, *store.JobHealthStats],
) (*store.JobHealthStats, error) {
	if c == nil || c.tier == nil {
		return loader(ctx, key)
	}
	return c.tier.Get(ctx, key, loader)
}

// WorkflowCallback is called after a job run reaches a terminal state.
// Nil-safe: if nil, no callback is invoked.
type WorkflowCallback interface {
	OnJobRunTerminal(ctx context.Context, run *domain.JobRun) error
}

// WorkerRunDispatcher is implemented by grpc.WorkerDispatcher to avoid a
// circular import between internal/worker and internal/api/grpc. Injected
// via ExecutorConfig.WorkerDispatcher.
//
// The return value is an opaque result container; callers cast it to extract
// the status and error message fields they need. Defined as interface{} to
// keep the worker package free of grpc proto imports — the actual type is
// *workerv1.TaskResult.
type WorkerRunDispatcher interface {
	WorkerDispatch(ctx context.Context, run *domain.JobRun, job *domain.Job) (any, error)
	// ResultStatus extracts the status string ("success", "failed", or "")
	// from an opaque TaskResult. Returns "" for nil or wrong type.
	ResultStatus(opaque any) string
	// ResultError extracts the error message from a failed TaskResult.
	// Returns "" for nil, wrong type, or empty error_message.
	ResultError(opaque any) string
	// ResultOutput extracts output_json from a successful TaskResult.
	// Returns nil for nil, wrong type, or empty output_json.
	ResultOutput(opaque any) json.RawMessage
}

type workerTaskCompletionDispatcher interface {
	CompleteWorkerTask(ctx context.Context, opaque any, status domain.WorkerTaskStatus) error
}

type SecretDecryptor interface {
	Decrypt([]byte) ([]byte, error)
}

// Executor polls the queue and executes job runs via HTTP dispatch.
type Executor struct {
	pool                     *Pool
	concurrencyLimit         ConcurrencyLimitProvider
	queue                    queue.Queue
	wake                     <-chan struct{}
	store                    ExecutorStore
	txPool                   store.TxBeginner
	httpClient               *http.Client
	pollInterval             time.Duration
	heartbeat                *HeartbeatSender
	publisher                pubsub.Publisher
	metrics                  *telemetry.Metrics
	workflowCallback         WorkflowCallback
	workerDispatcher         WorkerRunDispatcher
	partitionCycle           []string
	nextPartition            int
	bulkhead                 *ShardedBulkhead
	circuitThreshold         int
	circuitOpenFor           time.Duration
	healthScorer             *HealthScorer
	onCompleteTrigger        *OnCompleteTrigger
	logger                   *slog.Logger
	webhookMaxRetry          int
	executionTraceMode       executionTraceMode
	middlewares              []ExecutionMiddleware
	subscribers              []RunEventSubscriber
	eventCh                  chan runEventEnvelope
	maxDequeueBatchSize      int
	defaultJobMaxConcurrency int
	jobCache                 executorJobCache
	jobVersionCache          executorVersionedJobCache
	runVersionCache          *tierWorkflowRunVersionCache
	stepsVersionCache        *tierWorkflowStepsVersionCache
	jobHealthCache           *tierJobHealthCache
	memoryPressureThreshold  float64
	maxSnoozeCount           int
	jwtSigningKey            string
	externalAPIURL           string
	defaultRegion            string
	mode                     string
	version                  string
	edition                  domain.Edition
	billingEnforcer          *billing.Enforcer
	stripeUsageReporter      *billing.StripeUsageReporter
	stripeUsageWG            conc.WaitGroup // tracks in-flight Stripe usage event goroutines
	runCostRecorder          *billing.RunCostRecorder
	dlqCapEnforcer           *DLQCapEnforcer
	secretDecryptor          SecretDecryptor
	stop                     chan struct{}
	done                     chan struct{}
	stopOnce                 sync.Once
	pollWG                   sync.WaitGroup
	bgWG                     conc.WaitGroup
	callbackWG               conc.WaitGroup
	pollInFlight             atomic.Int64
	runStarted               atomic.Bool
	degradedPollInterval     time.Duration
	degraded                 queue.DegradedNotifier
	dbCircuit                *queue.DBCircuit
	eventChannelSize         int
	saturationWarnMu         sync.Mutex
	saturationLastWarn       map[string]time.Time
	// queueSnapshotter returns the set of queue names with active workers on
	// this replica. When non-nil, poll performs a second dequeue pass for
	// worker-mode runs filtered to those queues. Injected from the gRPC
	// ConnectionRegistry via QueueSnapshotter interface (no circular import).
	queueSnapshotter QueueSnapshotter
	// queueMetrics caches the process-wide queue metrics handle so the
	// hot-path lifecycle emit/drop paths avoid a sync.Once + error-check
	// lookup per event. Resolved once in NewExecutor; may be nil if the
	// metrics subsystem failed to initialise (tests without OTEL).
	queueMetrics *queue.QueueMetrics
	// instanceID is a stable identifier for this process used as the
	// "instance" attribute on multi-instance gauges (notably
	// EventChannelSaturationRatio) so multi-pod deployments can tell
	// which replica is saturated. Resolved lazily once via
	// resolveInstanceID and cached for the life of the executor.
	instanceIDOnce sync.Once
	instanceID     string
}

type ConcurrencyLimitProvider interface {
	CurrentLimit() int
}

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
	Edition                    domain.Edition
	WorkflowLookup             WorkflowLookup
	WorkflowTriggerer          WorkflowTriggerer
	JobLookup                  JobLookup
	JobEnqueuer                JobEnqueuer
	BillingEnforcer            *billing.Enforcer            // Optional: org-level billing enforcement (cloud only).
	StripeUsageReporter        *billing.StripeUsageReporter // Optional: Stripe usage event reporting (cloud only).
	RunCostRecorder            *billing.RunCostRecorder     // Optional: flat per-run cost recording (cloud only).
	DLQCapEnforcer             *DLQCapEnforcer              // Optional: enforces DLQ caps before terminal dead-letter transitions.
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
	defaultEventChannelSize        = 1024
	minEventChannelSize            = 16
	eventChannelSaturationRatio    = 0.8
	eventChannelWarnInterval       = 30 * time.Second
	defaultDegradedPollInterval    = time.Second
)

func resolveDegradedPollInterval(d time.Duration) time.Duration {
	if d <= 0 {
		return defaultDegradedPollInterval
	}
	return d
}

func NewExecutor(cfg ExecutorConfig) *Executor {
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		execTimeout := cfg.ExecutorHTTPTimeout
		if execTimeout <= 0 {
			execTimeout = 5 * time.Minute
		}
		execIdleTimeout := cfg.ExecutorIdleConnTimeout
		if execIdleTimeout <= 0 {
			execIdleTimeout = 90 * time.Second
		}
		httpClient = &http.Client{
			Timeout: execTimeout,
			Transport: func() *http.Transport {
				transport := httputil.NewExternalTransport(cfg.AllowPrivateEndpoints)
				transport.MaxIdleConns = 100
				transport.MaxIdleConnsPerHost = 10
				transport.IdleConnTimeout = execIdleTimeout
				transport.TLSHandshakeTimeout = 10 * time.Second
				return transport
			}(),
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}

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
		saturationLastWarn:       make(map[string]time.Time),
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
		edition:                  cfg.Edition,
		billingEnforcer:          cfg.BillingEnforcer,
		stripeUsageReporter:      cfg.StripeUsageReporter,
		runCostRecorder:          cfg.RunCostRecorder,
		dlqCapEnforcer:           cfg.DLQCapEnforcer,
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

func (e *Executor) CloseCache() {}

func (e *Executor) tryAcquireBulkheadSlot(jobID string, maxConcurrency int) bool {
	return e.bulkhead.TryAcquire(jobID, maxConcurrency)
}

func (e *Executor) releaseBulkheadSlot(jobID string, maxConcurrency int) {
	e.bulkhead.Release(jobID, maxConcurrency)
}

// Use adds execution middleware to the chain. Must be called before Run().
func (e *Executor) Use(mw ExecutionMiddleware) {
	e.middlewares = append(e.middlewares, mw)
}

// Subscribe registers a run lifecycle event subscriber. Must be called before Run().
func (e *Executor) Subscribe(sub RunEventSubscriber) {
	e.subscribers = append(e.subscribers, sub)
}

// emit sends a lifecycle event to all subscribers via the buffered channel.
// Non-blocking: drops the event with a warning if the channel is full or closed.
func (e *Executor) emit(ctx context.Context, event RunLifecycleEvent) {
	if len(e.subscribers) == 0 {
		return
	}

	// Recover from send-on-closed-channel if the executor is shutting down
	// and a pool goroutine emits after eventCh is closed.
	defer func() {
		if r := recover(); r != nil {
			sentry.CurrentHub().Recover(r)
			sentry.Flush(2 * time.Second)
			e.logger.Warn("event channel closed, dropping event",
				"type", event.Type,
				"run_id", event.Run.ID,
			)
			e.recordEventChannelDrop(ctx, "closed")
		}
	}()

	select {
	case e.eventCh <- runEventEnvelope{ctx: ctx, event: event}:
		e.sampleEventChannelSaturation(ctx, string(event.Type))
	default:
		if e.shouldLogSaturation(string(event.Type)) {
			e.logger.Warn("event channel full, dropping event",
				"type", event.Type,
				"run_id", event.Run.ID,
			)
		}
		e.recordEventChannelDrop(ctx, string(event.Type))
	}
}

// resolveEventChannelSize applies defaults and a lower bound to the configured
// event channel capacity.
func resolveEventChannelSize(configured int) int {
	if configured <= 0 {
		return defaultEventChannelSize
	}
	if configured < minEventChannelSize {
		return minEventChannelSize
	}
	return configured
}

// sampleEventChannelSaturation records the current channel fill ratio and emits
// a rate-limited warning log plus the saturation gauge whenever it exceeds the
// threshold. Per-kind throttling prevents log floods under sustained pressure.
func (e *Executor) sampleEventChannelSaturation(ctx context.Context, kind string) {
	if e.eventChannelSize <= 0 {
		return
	}
	ratio := float64(len(e.eventCh)) / float64(e.eventChannelSize)
	if qm := e.queueMetrics; qm != nil && qm.EventChannelSaturationRatio != nil {
		qm.EventChannelSaturationRatio.Record(ctx, ratio,
			metric.WithAttributes(attribute.String("instance", e.resolveInstanceID())))
	}
	if ratio > eventChannelSaturationRatio && e.shouldLogSaturation(kind) {
		e.logger.Warn("event channel saturated",
			"kind", kind,
			"ratio", ratio,
			"depth", len(e.eventCh),
			"capacity", e.eventChannelSize,
		)
	}
}

// resolveInstanceID returns a stable per-process identifier suitable
// for use as a metric attribute. It prefers the OS hostname, which commonly
// matches the container or instance identity, and falls back to a process-scoped
// UUID if Hostname errors or returns empty. Resolution happens at most once per
// Executor; subsequent calls return the cached value.
func (e *Executor) resolveInstanceID() string {
	e.instanceIDOnce.Do(func() {
		host, err := os.Hostname()
		if err == nil && host != "" {
			e.instanceID = host
			return
		}
		e.instanceID = uuid.NewString()
	})
	return e.instanceID
}

// shouldLogSaturation returns true at most once per eventChannelWarnInterval
// per event kind, so the warn log survives sustained backpressure without
// spamming.
func (e *Executor) shouldLogSaturation(kind string) bool {
	e.saturationWarnMu.Lock()
	defer e.saturationWarnMu.Unlock()
	if e.saturationLastWarn == nil {
		e.saturationLastWarn = make(map[string]time.Time)
	}
	now := time.Now()
	if last, ok := e.saturationLastWarn[kind]; ok && now.Sub(last) < eventChannelWarnInterval {
		return false
	}
	e.saturationLastWarn[kind] = now
	return true
}

// recordEventChannelDrop increments the drop counter labelled by event kind.
// No-op when queue metrics have not been initialised. Uses the cached
// Executor queueMetrics handle to avoid a sync.Once + error-check
// lookup on every drop in the lifecycle hot path.
func (e *Executor) recordEventChannelDrop(ctx context.Context, kind string) {
	qm := e.queueMetrics
	if qm == nil || qm.EventChannelDropped == nil {
		return
	}
	qm.EventChannelDropped.Add(ctx, 1, metric.WithAttributes(attribute.String("kind", kind)))
}

// runEventLoop drains the event channel and fans out to all subscribers.
// Exits when eventCh is closed (during shutdown or when Run exits).
func (e *Executor) runEventLoop() {
	for env := range e.eventCh {
		for _, sub := range e.subscribers {
			func() {
				defer func() {
					if r := recover(); r != nil {
						e.logger.Error("event subscriber panicked", "panic", r)
					}
				}()
				sub(env.ctx, env.event)
			}()
		}
	}
}

func (e *Executor) notifyWorkflowCallback(ctx context.Context, run *domain.JobRun) {
	if e.workflowCallback == nil {
		return
	}

	e.callbackWG.Go(func() {
		if err := e.workflowCallback.OnJobRunTerminal(ctx, run); err != nil {
			e.logger.Error("workflow callback failed", "run_id", run.ID, "error", err)
		}
	})
}

func (e *Executor) publishEvent(ctx context.Context, run *domain.JobRun, data map[string]any) {
	if e.publisher == nil {
		return
	}

	event := map[string]any{
		"type":       "status_change",
		"run_id":     run.ID,
		"job_id":     run.JobID,
		"project_id": run.ProjectID,
		"timestamp":  time.Now().UTC(),
	}
	maps.Copy(event, data)

	payload, err := json.Marshal(event)
	if err != nil {
		e.logger.Error("failed to marshal event", "error", err)
		return
	}

	channel := fmt.Sprintf("run:%s", run.ID)
	if err := e.publisher.Publish(ctx, channel, payload); err != nil {
		e.logger.Error("failed to publish event", "run_id", run.ID, "error", err)
	}
}

// doPoll wraps a single poll cycle with proper WaitGroup tracking via
// defer so that pollWG.Done is always called even if poll panics.
func (e *Executor) doPoll(ctx context.Context) {
	e.pollWG.Add(1)
	e.pollInFlight.Add(1)
	defer e.pollWG.Done()
	defer e.pollInFlight.Add(-1)
	e.poll(ctx)
}

// Run starts the heartbeat manager, event loop, and polling loop. Blocks until ctx is canceled.
func (e *Executor) Run(ctx context.Context) {
	e.runStarted.Store(true)

	// Create a child context that cancels when either the parent context
	// is canceled or Shutdown closes e.stop, so all background goroutines
	// (heartbeat, pool pruner) exit promptly in both cases.
	// runCancel must fire before pollWG.Wait or shutdown can deadlock.
	runCtx, runCancel := context.WithCancel(ctx) //nolint:gosec,nolintlint

	defer func() {
		runCancel() // Cancel context first so heartbeat and other goroutines exit.
		close(e.done)
		// Wait for in-flight polls and tracked goroutines to finish
		// emitting events, then close the event channel so the event
		// loop goroutine exits cleanly.
		e.pollWG.Wait()
		close(e.eventCh)
	}()

	e.logger.Info("executor started", "poll_interval", e.pollInterval)

	e.bgWG.Go(func() {
		e.heartbeat.Run(runCtx)
	})
	e.bgWG.Go(e.runEventLoop)

	ticker := time.NewTicker(e.pollInterval)
	defer ticker.Stop()

	var degradedCh <-chan struct{}
	if e.degraded != nil {
		degradedCh = e.degraded.Degraded()
	}
	inDegradedMode := false

	for {
		select {
		case <-ctx.Done():
			e.logger.Info("executor stopping")
			return
		case <-e.stop:
			e.logger.Info("executor stopping")
			return
		case _, ok := <-e.wake:
			if !ok {
				e.wake = nil
				continue
			}
			if inDegradedMode {
				ticker.Reset(e.pollInterval)
				inDegradedMode = false
				if e.degraded != nil {
					degradedCh = e.degraded.Degraded()
				}
				e.logger.Info("executor restored normal poll interval after wake reconnect")
			}
			e.doPoll(ctx)
		case <-degradedCh:
			ticker.Reset(e.degradedPollInterval)
			inDegradedMode = true
			degradedCh = nil
			e.logger.Warn("executor entering degraded mode: fast polling engaged",
				"degraded_poll_interval", e.degradedPollInterval,
			)
			e.doPoll(ctx)
		case <-ticker.C:
			e.doPoll(ctx)
		}
	}
}

func (e *Executor) Shutdown(ctx context.Context) error {
	e.stopOnce.Do(func() {
		close(e.stop)
	})

	if !e.runStarted.Load() {
		return nil
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-e.done:
	}

	e.pollWG.Wait()

	callbackDone := make(chan struct{})
	var callbackWaitWG conc.WaitGroup
	callbackWaitWG.Go(func() {
		e.callbackWG.Wait()
		close(callbackDone)
	})

	callbackTimeout := time.NewTimer(10 * time.Second)
	defer callbackTimeout.Stop()
	select {
	case <-callbackDone:
	case <-callbackTimeout.C:
		e.logger.Warn("timed out waiting for in-flight workflow callbacks")
	case <-ctx.Done():
		return ctx.Err()
	}

	// Wait for any in-flight Stripe usage event goroutines.
	stripeDone := make(chan struct{})
	var stripeWaitWG conc.WaitGroup
	stripeWaitWG.Go(func() {
		e.stripeUsageWG.Wait()
		close(stripeDone)
	})

	stripeTimeout := time.NewTimer(10 * time.Second)
	defer stripeTimeout.Stop()
	select {
	case <-stripeDone:
	case <-stripeTimeout.C:
		e.logger.Warn("timed out waiting for in-flight stripe usage events")
	case <-ctx.Done():
		return ctx.Err()
	}

	bgDone := make(chan struct{})
	var bgWaitWG conc.WaitGroup
	bgWaitWG.Go(func() {
		e.bgWG.Wait()
		close(bgDone)
	})

	bgTimeout := time.NewTimer(10 * time.Second)
	defer bgTimeout.Stop()
	select {
	case <-bgDone:
	case <-bgTimeout.C:
		e.logger.Warn("timed out waiting for executor background goroutines")
	case <-ctx.Done():
		return ctx.Err()
	}

	return nil
}
