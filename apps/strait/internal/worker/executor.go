package worker

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"strait/internal/billing"
	"strait/internal/domain"
	"strait/internal/pubsub"
	"strait/internal/queue"
	"strait/internal/store"
	"strait/internal/telemetry"

	"github.com/sourcegraph/conc"
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
	billingEnforcer          *billing.Enforcer
	stripeUsageReporter      *billing.StripeUsageReporter
	stripeUsageWG            conc.WaitGroup // tracks in-flight Stripe usage event goroutines
	runCostRecorder          *billing.RunCostRecorder
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
	saturationLastWarn       map[eventChannelKind]time.Time
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
