package worker

import (
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
	drain                    *drainController
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
