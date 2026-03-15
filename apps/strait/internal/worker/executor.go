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

	"strait/internal/domain"
	"strait/internal/pubsub"
	"strait/internal/queue"
	"strait/internal/store"
	"strait/internal/telemetry"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
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
	GetRunErrorClass(ctx context.Context, runID string) (string, error)
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

type cachedJob struct {
	job       *domain.Job
	expiresAt time.Time
}

// Executor polls the queue and executes job runs via HTTP dispatch.
type Executor struct {
	pool                     *Pool
	concurrencyLimit         ConcurrencyLimitProvider
	queue                    queue.Queue
	wake                     <-chan struct{}
	store                    ExecutorStore
	httpClient               *http.Client
	pollInterval             time.Duration
	heartbeat                *HeartbeatSender
	publisher                pubsub.Publisher
	metrics                  *telemetry.Metrics
	workflowCallback         WorkflowCallback
	partitionCycle           []string
	nextPartition            int
	jobActiveRuns            map[string]int
	jobActiveRunsMu          sync.Mutex
	circuitThreshold         int
	circuitOpenFor           time.Duration
	logger                   *slog.Logger
	webhookClient            *http.Client
	webhookMaxRetry          int
	webhookDispatchTimeout   time.Duration
	maxDequeueBatchSize      int
	defaultJobMaxConcurrency int
	jobCache                 sync.Map
	jobCacheTTL              time.Duration
	memoryPressureThreshold  float64
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
	WebhookTimeout             time.Duration
	WebhookIdleConnTimeout     time.Duration
	WebhookDispatchTimeout     time.Duration
	WebhookMaxAttempts         int
	MaxDequeueBatchSize        int
	DefaultJobMaxConcurrency   int
	MemoryPressureThresholdPct float64
	JobCacheTTL                time.Duration
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

	whTimeout := cfg.WebhookTimeout
	if whTimeout <= 0 {
		whTimeout = webhookTimeout
	}
	whIdleTimeout := cfg.WebhookIdleConnTimeout
	if whIdleTimeout <= 0 {
		whIdleTimeout = webhookIdleConnTimeout
	}
	whClient := &http.Client{
		Timeout: whTimeout,
		Transport: otelhttp.NewTransport(&http.Transport{
			MaxIdleConns:        webhookMaxIdleConns,
			MaxIdleConnsPerHost: webhookMaxIdlePerHost,
			IdleConnTimeout:     whIdleTimeout,
		}),
	}
	whMaxAttempts := cfg.WebhookMaxAttempts
	if whMaxAttempts <= 0 {
		whMaxAttempts = 3
	}
	whDispatchTimeout := cfg.WebhookDispatchTimeout
	if whDispatchTimeout <= 0 {
		whDispatchTimeout = 15 * time.Second
	}

	return &Executor{
		pool:                     cfg.Pool,
		concurrencyLimit:         cfg.ConcurrencyLimit,
		queue:                    cfg.Queue,
		wake:                     cfg.Wake,
		store:                    cfg.Store,
		httpClient:               httpClient,
		pollInterval:             cfg.PollInterval,
		heartbeat:                NewHeartbeatSender(cfg.Store, cfg.HeartbeatInterval),
		publisher:                cfg.Publisher,
		metrics:                  cfg.Metrics,
		workflowCallback:         cfg.WorkflowCallback,
		partitionCycle:           buildPartitionCycle(cfg.Partitions, cfg.PartitionWeights),
		jobActiveRuns:            make(map[string]int),
		circuitThreshold:         defaultCircuitFailureThreshold,
		circuitOpenFor:           defaultCircuitOpenDuration,
		logger:                   slog.Default(),
		webhookClient:            whClient,
		webhookMaxRetry:          whMaxAttempts,
		webhookDispatchTimeout:   whDispatchTimeout,
		maxDequeueBatchSize:      cfg.MaxDequeueBatchSize,
		defaultJobMaxConcurrency: cfg.DefaultJobMaxConcurrency,
		jobCacheTTL:              cfg.JobCacheTTL,
		memoryPressureThreshold:  cfg.MemoryPressureThresholdPct,
		stop:                     make(chan struct{}),
		done:                     make(chan struct{}),
	}
}

func (e *Executor) tryAcquireBulkheadSlot(jobID string, maxConcurrency int) bool {
	if maxConcurrency <= 0 {
		maxConcurrency = e.defaultJobMaxConcurrency
	}
	if maxConcurrency <= 0 {
		return true
	}

	e.jobActiveRunsMu.Lock()
	defer e.jobActiveRunsMu.Unlock()

	if e.jobActiveRuns[jobID] >= maxConcurrency {
		return false
	}

	e.jobActiveRuns[jobID]++
	return true
}

func (e *Executor) releaseBulkheadSlot(jobID string, maxConcurrency int) {
	if maxConcurrency <= 0 {
		maxConcurrency = e.defaultJobMaxConcurrency
	}
	if maxConcurrency <= 0 {
		return
	}

	e.jobActiveRunsMu.Lock()
	defer e.jobActiveRunsMu.Unlock()

	active := e.jobActiveRuns[jobID]
	if active <= 1 {
		delete(e.jobActiveRuns, jobID)
		return
	}

	e.jobActiveRuns[jobID] = active - 1
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

// Run starts the heartbeat manager and polling loop. Blocks until ctx is canceled.
func (e *Executor) Run(ctx context.Context) {
	e.runStarted.Store(true)
	defer close(e.done)

	e.logger.Info("executor started", "poll_interval", e.pollInterval)

	go e.heartbeat.Run(ctx)

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

	return nil
}
