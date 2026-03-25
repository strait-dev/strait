package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

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
	defaultFlyRegion         string
	billingEnforcer          *billing.Enforcer
	polarIngester            *billing.PolarEventIngester
	polarWG                  sync.WaitGroup // tracks in-flight Polar event ingestion goroutines
	stop                     chan struct{}
	done                     chan struct{}
	stopOnce                 sync.Once
	pollWG                   sync.WaitGroup
	callbackWG               sync.WaitGroup
	pollInFlight             atomic.Int64
	runStarted               atomic.Bool
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
	DefaultFlyRegion           string
	WarmPoolEnabled            bool
	WarmPoolMaxPerJob          int
	DisableMachinePoolReuse    bool
	WorkflowLookup             WorkflowLookup
	WorkflowTriggerer          WorkflowTriggerer
	BillingEnforcer            *billing.Enforcer           // Optional: org-level billing enforcement (cloud only).
	PolarIngester              *billing.PolarEventIngester // Optional: Polar usage event ingestion (cloud only).
}

const (
	defaultCircuitFailureThreshold = 5
	defaultCircuitOpenDuration     = time.Minute
)

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
		eventCh:                  make(chan runEventEnvelope, 256),
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
		defaultFlyRegion:         cfg.DefaultFlyRegion,
		billingEnforcer:          cfg.BillingEnforcer,
		polarIngester:            cfg.PolarIngester,
		healthScorer:             NewHealthScorer(cfg.Store),
		onCompleteTrigger:        NewOnCompleteTrigger(cfg.WorkflowLookup, cfg.WorkflowTriggerer, slog.Default()),
		stop:                     make(chan struct{}),
		done:                     make(chan struct{}),
	}
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
		}
	}()

	select {
	case e.eventCh <- runEventEnvelope{ctx: ctx, event: event}:
	default:
		e.logger.Warn("event channel full, dropping event",
			"type", event.Type,
			"run_id", event.Run.ID,
		)
	}
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

// Run starts the heartbeat manager, event loop, and polling loop. Blocks until ctx is canceled.
func (e *Executor) Run(ctx context.Context) {
	e.runStarted.Store(true)
	defer func() {
		close(e.done)
		// Wait for in-flight polls to finish emitting events, then close the
		// event channel so the event loop goroutine exits cleanly.
		e.pollWG.Wait()
		close(e.eventCh)
	}()

	e.logger.Info("executor started", "poll_interval", e.pollInterval)

	go e.heartbeat.Run(ctx)
	go e.runEventLoop()

	if e.machinePool != nil {
		go e.runPoolPruner(ctx)
	}

	ticker := time.NewTicker(e.pollInterval)
	defer ticker.Stop()

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
			e.pollWG.Add(1)
			e.pollInFlight.Add(1)
			e.poll(ctx)
			e.pollInFlight.Add(-1)
			e.pollWG.Done()
		case <-ticker.C:
			e.pollWG.Add(1)
			e.pollInFlight.Add(1)
			e.poll(ctx)
			e.pollInFlight.Add(-1)
			e.pollWG.Done()
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

	// Wait for any in-flight Polar billing event ingestion goroutines.
	polarDone := make(chan struct{})
	go func() {
		e.polarWG.Wait()
		close(polarDone)
	}()

	polarTimeout := time.NewTimer(10 * time.Second)
	defer polarTimeout.Stop()
	select {
	case <-polarDone:
	case <-polarTimeout.C:
		e.logger.Warn("timed out waiting for in-flight polar billing events")
	case <-ctx.Done():
		return ctx.Err()
	}

	return nil
}
