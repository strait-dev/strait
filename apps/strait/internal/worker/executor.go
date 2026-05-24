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
	"strait/internal/compute"
	"strait/internal/domain"
	"strait/internal/pubsub"
	"strait/internal/queue"
	"strait/internal/store"
	"strait/internal/telemetry"

	"strait/internal/cache/otterstore"

	"github.com/eko/gocache/lib/v4/cache"
	"github.com/getsentry/sentry-go"
	"github.com/sourcegraph/conc"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"golang.org/x/sync/semaphore"
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
	UpdateHeartbeat(ctx context.Context, id string) error
	CanDispatchEndpoint(ctx context.Context, endpointURL string, now time.Time) (bool, *time.Time, error)
	RecordEndpointCircuitFailure(ctx context.Context, endpointURL string, now time.Time, threshold int, openDuration time.Duration) error
	RecordEndpointCircuitSuccess(ctx context.Context, endpointURL string) error
	GetJobHealthStats(ctx context.Context, jobID string, since time.Time) (*store.JobHealthStats, error)
	GetResolvedEnvironmentVariables(ctx context.Context, id string) (map[string]string, error)
	GetLatestCheckpoint(ctx context.Context, runID string) (*domain.RunCheckpoint, error)
	GetRun(ctx context.Context, id string) (*domain.JobRun, error)
	GetProjectQuota(ctx context.Context, projectID string) (*store.ProjectQuota, error)
	SumDailyComputeCost(ctx context.Context, projectID, timezone string) (int64, error)
	CreateRunComputeUsage(ctx context.Context, usage *domain.RunComputeUsage) error
	InsertEvent(ctx context.Context, event *domain.RunEvent) error
	SetRunMachineID(ctx context.Context, runID, machineID string) error
	RecordOOMEvent(ctx context.Context, jobID, preset string) error
	GetPresetRecommendation(ctx context.Context, jobID string) (*store.PresetRecommendation, error)
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
	partitionCycle           []string
	nextPartition            int
	bulkhead                 *ShardedBulkhead
	circuitThreshold         int
	circuitOpenFor           time.Duration
	healthScorer             *HealthScorer
	onCompleteTrigger        *OnCompleteTrigger
	logger                   *slog.Logger
	webhookMaxRetry          int
	middlewares              []ExecutionMiddleware
	subscribers              []RunEventSubscriber
	eventCh                  chan runEventEnvelope
	maxDequeueBatchSize      int
	defaultJobMaxConcurrency int
	jobCache                 *cache.Cache[*domain.Job]
	jobCacheStore            *otterstore.OtterStore
	memoryPressureThreshold  float64
	maxSnoozeCount           int
	dequeueStrategy          string
	jwtSigningKey            string
	containerRuntime         compute.ContainerRuntime
	managedSemaphore         *semaphore.Weighted
	machinePool              *compute.MachinePool
	disableMachinePoolReuse  bool
	externalAPIURL           string
	defaultRegion            string
	billingEnforcer          *billing.Enforcer
	stripeUsageReporter      *billing.StripeUsageReporter
	stripeUsageWG            conc.WaitGroup // tracks in-flight Stripe usage event goroutines
	stop                     chan struct{}
	done                     chan struct{}
	stopOnce                 sync.Once
	pollWG                   sync.WaitGroup
	callbackWG               sync.WaitGroup
	pollInFlight             atomic.Int64
	runStarted               atomic.Bool
	claimCursor              *queue.ClaimCursor
	degradedPollInterval     time.Duration
	degraded                 queue.DegradedNotifier
	useDenormalizedDequeue   bool
	dbCircuit                *queue.DBCircuit
	eventChannelSize         int
	saturationWarnMu         sync.Mutex
	saturationLastWarn       map[string]time.Time
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
	WebhookMaxAttempts         int
	MaxDequeueBatchSize        int
	DefaultJobMaxConcurrency   int
	MemoryPressureThresholdPct float64
	JobCacheTTL                time.Duration
	MaxSnoozeCount             int
	JWTSigningKey              string
	DequeueStrategy            string
	ContainerRuntime           compute.ContainerRuntime
	ExternalAPIURL             string
	MaxConcurrentMachines      int
	DefaultRegion              string
	WarmPoolEnabled            bool
	WarmPoolMaxPerJob          int
	DisableMachinePoolReuse    bool
	WorkflowLookup             WorkflowLookup
	WorkflowTriggerer          WorkflowTriggerer
	JobLookup                  JobLookup
	JobEnqueuer                JobEnqueuer
	BillingEnforcer            *billing.Enforcer            // Optional: org-level billing enforcement (cloud only).
	StripeUsageReporter        *billing.StripeUsageReporter // Optional: Stripe usage event reporting (cloud only).
	// DegradedPollInterval is the shortened poll interval used when the
	// queue notifier enters degraded mode (LISTEN disconnected for too long).
	// Zero/negative falls back to 1 second.
	DegradedPollInterval time.Duration
	// Degraded provides a channel that closes when the queue notifier enters
	// degraded mode. The executor re-invokes Degraded() on each recovery to
	// obtain the fresh channel, avoiding stale-channel re-arm. Nil means no
	// degraded-mode support.
	Degraded queue.DegradedNotifier
	// UseDenormalizedDequeue opts into the fully denormalized dequeue path
	// backed by job_runs fan-out columns and job_active_counts. Defaults to
	// false so existing deployments are unaffected.
	UseDenormalizedDequeue bool
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
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     execIdleTimeout,
				TLSHandshakeTimeout: 10 * time.Second,
			},
		}
	}

	whMaxAttempts := cfg.WebhookMaxAttempts
	if whMaxAttempts <= 0 {
		whMaxAttempts = 3
	}

	var managedSem *semaphore.Weighted
	if cfg.ContainerRuntime != nil {
		maxMachines := cfg.MaxConcurrentMachines
		if maxMachines <= 0 {
			maxMachines = 10
		}
		managedSem = semaphore.NewWeighted(int64(maxMachines))
	}

	var machinePool *compute.MachinePool
	if cfg.WarmPoolEnabled {
		machinePool = compute.NewMachinePool(cfg.WarmPoolMaxPerJob)
		if cfg.ContainerRuntime != nil {
			rt := cfg.ContainerRuntime
			machinePool.SetOnEvict(func(machineID string) {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				_ = rt.Destroy(ctx, machineID)
			})
		}
	}

	var jobCache *cache.Cache[*domain.Job]
	var jobCacheStore *otterstore.OtterStore
	if cfg.JobCacheTTL > 0 {
		jobCacheStore = otterstore.New(otterstore.Config{
			DefaultTTL:  cfg.JobCacheTTL,
			MaxCapacity: 10_000,
		})
		jobCache = cache.New[*domain.Job](jobCacheStore)
	}

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
		eventCh:                  make(chan runEventEnvelope, resolveEventChannelSize(cfg.EventChannelSize)),
		eventChannelSize:         resolveEventChannelSize(cfg.EventChannelSize),
		saturationLastWarn:       make(map[string]time.Time),
		maxDequeueBatchSize:      cfg.MaxDequeueBatchSize,
		defaultJobMaxConcurrency: cfg.DefaultJobMaxConcurrency,
		jobCache:                 jobCache,
		jobCacheStore:            jobCacheStore,
		memoryPressureThreshold:  cfg.MemoryPressureThresholdPct,
		maxSnoozeCount:           cfg.MaxSnoozeCount,
		dequeueStrategy:          cfg.DequeueStrategy,
		jwtSigningKey:            cfg.JWTSigningKey,
		containerRuntime:         cfg.ContainerRuntime,
		managedSemaphore:         managedSem,
		machinePool:              machinePool,
		disableMachinePoolReuse:  cfg.DisableMachinePoolReuse,
		externalAPIURL:           cfg.ExternalAPIURL,
		defaultRegion:            cfg.DefaultRegion,
		billingEnforcer:          cfg.BillingEnforcer,
		stripeUsageReporter:      cfg.StripeUsageReporter,
		healthScorer:             NewHealthScorer(cfg.Store),
		onCompleteTrigger:        NewOnCompleteTrigger(cfg.WorkflowLookup, cfg.WorkflowTriggerer, cfg.JobLookup, cfg.JobEnqueuer, slog.Default()),
		stop:                     make(chan struct{}),
		done:                     make(chan struct{}),
		claimCursor:              queue.NewClaimCursor(60 * time.Second),
		degradedPollInterval:     resolveDegradedPollInterval(cfg.DegradedPollInterval),
		degraded:                 cfg.Degraded,
		useDenormalizedDequeue:   cfg.UseDenormalizedDequeue,
		dbCircuit:                queue.NewDBCircuit(cfg.DBCircuitConfig),
		queueMetrics:             resolveQueueMetrics(),
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

// CloseCache shuts down the otter cache if one was created.
func (e *Executor) CloseCache() {
	if e.jobCacheStore != nil {
		e.jobCacheStore.Close()
	}
}

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
// for use as a metric attribute. Prefers the OS hostname (matches K8s
// pod name in standard deployments); falls back to a process-scoped
// UUID if Hostname errors or returns empty. Resolution happens at most
// once per Executor; subsequent calls return the cached value.
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
			sub(env.ctx, env.event)
		}
	}
}

func (e *Executor) notifyWorkflowCallback(ctx context.Context, run *domain.JobRun) {
	if e.workflowCallback == nil {
		return
	}

	e.callbackWG.Add(1)
	defer e.callbackWG.Done()

	if err := e.workflowCallback.OnJobRunTerminal(ctx, run); err != nil {
		e.logger.Error("workflow callback failed", "run_id", run.ID, "error", err)
	}
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
	runCtx, runCancel := context.WithCancel(ctx) //nolint:gosec,nolintlint // G118: runCancel is called inside the defer below; must be before pollWG.Wait to avoid deadlock

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

	e.pollWG.Go(func() {
		e.heartbeat.Run(runCtx)
	})
	go e.runEventLoop()

	if e.machinePool != nil {
		go e.runPoolPruner(runCtx)
	}

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

func (e *Executor) runPoolPruner(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-e.stop:
			return
		case <-ticker.C:
			pruned := e.machinePool.Prune(10*time.Minute, func(mid string) error {
				dCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				return e.containerRuntime.Destroy(dCtx, mid)
			})
			if pruned > 0 {
				e.logger.Info("pool pruner cleaned machines", "count", pruned)
			}
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

	if e.machinePool != nil && e.containerRuntime != nil {
		drained := e.machinePool.Prune(0, func(mid string) error {
			dCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			return e.containerRuntime.Destroy(dCtx, mid)
		})
		if drained > 0 {
			e.logger.Info("shutdown: drained warm pool", "count", drained)
		}
	}

	callbackDone := make(chan struct{})
	go func() {
		e.callbackWG.Wait()
		close(callbackDone)
	}()

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
	go func() {
		e.stripeUsageWG.Wait()
		close(stripeDone)
	}()

	stripeTimeout := time.NewTimer(10 * time.Second)
	defer stripeTimeout.Stop()
	select {
	case <-stripeDone:
	case <-stripeTimeout.C:
		e.logger.Warn("timed out waiting for in-flight stripe usage events")
	case <-ctx.Done():
		return ctx.Err()
	}

	return nil
}
