package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net"
	"net/http"
	"net/http/httptrace"
	"strconv"
	"strings"
	"sync"
	"time"

	"orchestrator/internal/domain"
	"orchestrator/internal/pubsub"
	"orchestrator/internal/queue"
	"orchestrator/internal/store"
	"orchestrator/internal/telemetry"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// ExecutorStore is the subset of store operations needed by Executor.
type ExecutorStore interface {
	GetJob(ctx context.Context, id string) (*domain.Job, error)
	ListJobSecretsByJob(ctx context.Context, jobID, environment string) ([]domain.JobSecret, error)
	UpdateRunStatus(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error
	UpdateHeartbeat(ctx context.Context, id string) error
	CanDispatchEndpoint(ctx context.Context, endpointURL string, now time.Time) (bool, *time.Time, error)
	RecordEndpointCircuitFailure(ctx context.Context, endpointURL string, now time.Time, threshold int, openDuration time.Duration) error
	RecordEndpointCircuitSuccess(ctx context.Context, endpointURL string) error
	GetJobHealthStats(ctx context.Context, jobID string, since time.Time) (*store.JobHealthStats, error)
}

// WorkflowCallback is called after a job run reaches a terminal state.
// Nil-safe: if nil, no callback is invoked.
type WorkflowCallback interface {
	OnJobRunTerminal(ctx context.Context, run *domain.JobRun) error
}

// Executor polls the queue and executes job runs via HTTP dispatch.
type Executor struct {
	pool             *Pool
	queue            queue.Queue
	store            ExecutorStore
	httpClient       *http.Client
	pollInterval     time.Duration
	heartbeat        *HeartbeatSender
	publisher        pubsub.Publisher
	metrics          *telemetry.Metrics
	workflowCallback WorkflowCallback
	partitionCycle   []string
	nextPartition    int
	circuitBreaker   bool
	secretInjection  bool
	smartRetry       bool
	bulkheads        bool
	executionTracing bool
	adaptiveTimeout  bool
	dlqEnabled       bool
	jobActiveRuns    map[string]int
	jobActiveRunsMu  sync.Mutex
	circuitThreshold int
	circuitOpenFor   time.Duration
	logger           *slog.Logger
}

// ExecutorConfig holds configuration for the Executor.
type ExecutorConfig struct {
	Pool              *Pool
	Queue             queue.Queue
	Store             ExecutorStore
	Publisher         pubsub.Publisher
	HTTPClient        *http.Client
	PollInterval      time.Duration
	HeartbeatInterval time.Duration
	Metrics           *telemetry.Metrics
	WorkflowCallback  WorkflowCallback
	Partitions        []string
	PartitionWeights  string
	CircuitBreaker    bool
	SecretInjection   bool
	SmartRetry        bool
	Bulkheads         bool
	ExecutionTracing  bool
	AdaptiveTimeout   bool
	DLQEnabled        bool
}

const (
	defaultCircuitFailureThreshold = 5
	defaultCircuitOpenDuration     = time.Minute
)

func NewExecutor(cfg ExecutorConfig) *Executor {
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 5 * time.Minute,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
				TLSHandshakeTimeout: 10 * time.Second,
			},
		}
	}

	return &Executor{
		pool:             cfg.Pool,
		queue:            cfg.Queue,
		store:            cfg.Store,
		httpClient:       httpClient,
		pollInterval:     cfg.PollInterval,
		heartbeat:        NewHeartbeatSender(cfg.Store, cfg.HeartbeatInterval),
		publisher:        cfg.Publisher,
		metrics:          cfg.Metrics,
		workflowCallback: cfg.WorkflowCallback,
		partitionCycle:   buildPartitionCycle(cfg.Partitions, cfg.PartitionWeights),
		circuitBreaker:   cfg.CircuitBreaker,
		secretInjection:  cfg.SecretInjection,
		smartRetry:       cfg.SmartRetry,
		bulkheads:        cfg.Bulkheads,
		executionTracing: cfg.ExecutionTracing,
		adaptiveTimeout:  cfg.AdaptiveTimeout,
		dlqEnabled:       cfg.DLQEnabled,
		jobActiveRuns:    make(map[string]int),
		circuitThreshold: defaultCircuitFailureThreshold,
		circuitOpenFor:   defaultCircuitOpenDuration,
		logger:           slog.Default(),
	}
}

func (e *Executor) tryAcquireBulkheadSlot(jobID string, maxConcurrency int) bool {
	if !e.bulkheads || maxConcurrency <= 0 {
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
	if !e.bulkheads || maxConcurrency <= 0 {
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

// Run starts the polling loop. Blocks until ctx is canceled.
func (e *Executor) Run(ctx context.Context) {
	e.logger.Info("executor started", "poll_interval", e.pollInterval)
	ticker := time.NewTicker(e.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			e.logger.Info("executor stopping")
			return
		case <-ticker.C:
			e.poll(ctx)
		}
	}
}

func (e *Executor) poll(ctx context.Context) {
	start := time.Now()
	available := e.pool.Available()
	if available <= 0 {
		return
	}

	var runs []domain.JobRun
	var err error
	if len(e.partitionCycle) == 0 {
		runs, err = e.queue.DequeueN(ctx, available)
	} else {
		runs, err = e.dequeueAcrossPartitions(ctx, available)
	}
	if e.metrics != nil {
		e.metrics.DequeueDuration.Record(ctx, time.Since(start).Seconds())
	}
	if err != nil {
		e.logger.Error("dequeue failed", "error", err)
		return
	}
	if len(runs) == 0 {
		return
	}

	e.logger.Info("dequeued runs", "count", len(runs))

	for i := range runs {
		run := runs[i]
		e.logger.Info(
			"dequeued run",
			"run_id", run.ID,
			"job_id", run.JobID,
			"project_id", run.ProjectID,
			"attempt", run.Attempt,
			"priority", run.Priority,
		)

		execCtx := context.WithoutCancel(ctx)
		e.pool.Submit(ctx, func() {
			e.execute(execCtx, &run)
		})
	}
}

func (e *Executor) dequeueAcrossPartitions(ctx context.Context, capacity int) ([]domain.JobRun, error) {
	out := make([]domain.JobRun, 0, capacity)
	if capacity <= 0 || len(e.partitionCycle) == 0 {
		return out, nil
	}

	remaining := capacity
	iterations := len(e.partitionCycle)
	for i := 0; i < iterations && remaining > 0; i++ {
		partition := e.partitionCycle[e.nextPartition%len(e.partitionCycle)]
		e.nextPartition = (e.nextPartition + 1) % len(e.partitionCycle)

		claimed, err := e.queue.DequeueNByProject(ctx, remaining, partition)
		if err != nil {
			return nil, err
		}
		if len(claimed) == 0 {
			continue
		}

		out = append(out, claimed...)
		remaining -= len(claimed)
	}

	return out, nil
}

func buildPartitionCycle(partitions []string, weightsRaw string) []string {
	if len(partitions) == 0 {
		return nil
	}

	weights := make(map[string]int)
	if weightsRaw != "" {
		for _, token := range strings.FieldsFunc(weightsRaw, func(r rune) bool { return r == ',' }) {
			token = strings.TrimSpace(token)
			if token == "" {
				continue
			}
			parts := strings.SplitN(token, ":", 2)
			if len(parts) != 2 {
				continue
			}
			key := strings.TrimSpace(parts[0])
			weight, err := strconv.Atoi(strings.TrimSpace(parts[1]))
			if err != nil || weight <= 0 {
				continue
			}
			weights[key] = weight
		}
	}

	cycle := make([]string, 0, len(partitions))
	for _, partition := range partitions {
		w := weights[partition]
		if w <= 0 {
			w = 1
		}
		for i := 0; i < w; i++ {
			cycle = append(cycle, partition)
		}
	}

	return cycle
}

func (e *Executor) execute(ctx context.Context, run *domain.JobRun) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "executor.Execute")
	defer span.End()

	executeStart := time.Now()

	job, err := e.store.GetJob(ctx, run.JobID)
	if err != nil || job == nil {
		e.logger.Error(
			"job lookup failed",
			"run_id", run.ID,
			"job_id", run.JobID,
			"error", err,
		)
		e.handleSystemFailure(ctx, run, "job not found")
		return
	}

	if e.circuitBreaker {
		allowed, retryAt, circuitErr := e.store.CanDispatchEndpoint(ctx, job.EndpointURL, time.Now().UTC())
		if circuitErr != nil {
			e.logger.Error(
				"circuit breaker check failed",
				"run_id", run.ID,
				"job_id", run.JobID,
				"endpoint", job.EndpointURL,
				"error", circuitErr,
			)
			e.handleSystemFailure(ctx, run, "circuit breaker unavailable")
			return
		}

		if !allowed {
			fields := map[string]any{
				"next_retry_at": retryAt,
				"error":         "endpoint circuit breaker open",
				"error_class":   "transient",
				"started_at":    nil,
				"finished_at":   nil,
			}
			if err := e.store.UpdateRunStatus(ctx, run.ID, domain.StatusDequeued, domain.StatusQueued, fields); err != nil {
				e.logger.Error(
					"failed to requeue run while circuit open",
					"run_id", run.ID,
					"job_id", run.JobID,
					"error", err,
				)
				return
			}

			e.recordRunTransition(ctx, domain.StatusDequeued, domain.StatusQueued)
			e.publishEvent(ctx, run, map[string]any{"from": "dequeued", "to": "queued", "reason": "circuit_open"})
			return
		}
	}

	acquired := e.tryAcquireBulkheadSlot(job.ID, job.MaxConcurrency)
	if !acquired {
		retryAt := NextRetryAt(run.Attempt)
		fields := map[string]any{
			"next_retry_at": retryAt,
			"error":         "job bulkhead at capacity",
			"error_class":   "transient",
			"started_at":    nil,
			"finished_at":   nil,
		}
		if err := e.store.UpdateRunStatus(ctx, run.ID, domain.StatusDequeued, domain.StatusQueued, fields); err != nil {
			e.logger.Error(
				"failed to requeue run while bulkhead at capacity",
				"run_id", run.ID,
				"job_id", run.JobID,
				"error", err,
			)
			return
		}

		e.recordRunTransition(ctx, domain.StatusDequeued, domain.StatusQueued)
		e.publishEvent(ctx, run, map[string]any{"from": "dequeued", "to": "queued", "reason": "bulkhead_capacity"})
		return
	}
	defer e.releaseBulkheadSlot(job.ID, job.MaxConcurrency)

	err = e.store.UpdateRunStatus(ctx, run.ID, domain.StatusDequeued, domain.StatusExecuting, map[string]any{
		"started_at": time.Now(),
	})
	if err != nil {
		e.logger.Error(
			"failed to transition to executing",
			"run_id", run.ID,
			"job_id", run.JobID,
			"error", err,
		)
		return
	}
	run.Status = domain.StatusExecuting
	e.publishEvent(ctx, run, map[string]any{"from": "dequeued", "to": "executing"})

	// Start heartbeat
	hbCtx, hbCancel := context.WithCancel(ctx)
	defer hbCancel()
	go e.heartbeat.Run(hbCtx, run.ID)

	timeout := time.Duration(job.TimeoutSecs) * time.Second
	if e.adaptiveTimeout && job.TimeoutSecs > 0 {
		stats, err := e.store.GetJobHealthStats(ctx, job.ID, time.Now().Add(-24*time.Hour))
		if err == nil && stats.P95DurationSecs > 0 {
			adaptiveTimeout := time.Duration(stats.P95DurationSecs * 1.5 * float64(time.Second))
			if adaptiveTimeout > timeout {
				timeout = adaptiveTimeout
				e.logger.Debug("using adaptive timeout", "job_id", job.ID, "p95_secs", stats.P95DurationSecs, "timeout", timeout)
			}
		}
	}
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result, execTrace, err := e.tracedDispatch(execCtx, job, run)
	if execTrace != nil {
		execTrace.TotalMs = durationMillisecondsAtLeastOne(time.Since(executeStart))
		queueWait := max(time.Duration(0), executeStart.Sub(run.CreatedAt))
		execTrace.QueueWaitMs = durationMillisecondsAtLeastOne(queueWait)
		if run.StartedAt != nil {
			dequeue := max(time.Duration(0), executeStart.Sub(*run.StartedAt))
			execTrace.DequeueMs = durationMillisecondsAtLeastOne(dequeue)
		}
	}
	if e.metrics != nil && execTrace != nil {
		e.metrics.ExecutionTraceDispatch.Record(ctx, float64(execTrace.DispatchMs))
		e.metrics.ExecutionTraceQueueWait.Record(ctx, float64(execTrace.QueueWaitMs))
	}
	if err != nil {
		if job.FallbackEndpointURL != "" {
			errClass := classifyError(err)
			if shouldUseFallbackForClass(errClass) {
				fallbackResult, fallbackErr := e.dispatchToEndpoint(execCtx, job.FallbackEndpointURL, run, nil)
				if fallbackErr == nil {
					e.handleSuccess(ctx, run, job, fallbackResult, execTrace)
					return
				}
				err = errors.Join(
					fmt.Errorf("primary dispatch failed: %w", err),
					fmt.Errorf("fallback dispatch failed: %w", fallbackErr),
				)
			}
		}

		if execCtx.Err() == context.DeadlineExceeded {
			e.handleTimeout(ctx, run, job, execTrace)
		} else {
			e.handleFailure(ctx, run, job, err, execTrace)
		}
		return
	}

	e.handleSuccess(ctx, run, job, result, execTrace)
}

func (e *Executor) tracedDispatch(ctx context.Context, job *domain.Job, run *domain.JobRun) (json.RawMessage, *domain.ExecutionTrace, error) {
	if !e.executionTracing {
		result, err := e.dispatch(ctx, job, run)
		return result, nil, err
	}

	dispatchStart := time.Now()
	var connectStart time.Time
	var connectDone time.Time
	var gotFirstByte time.Time

	trace := &httptrace.ClientTrace{
		ConnectStart:         func(string, string) { connectStart = time.Now() },
		ConnectDone:          func(string, string, error) { connectDone = time.Now() },
		GotFirstResponseByte: func() { gotFirstByte = time.Now() },
	}

	tracedCtx := httptrace.WithClientTrace(ctx, trace)

	var extraHeaders map[string]string
	if e.secretInjection {
		secrets, err := e.store.ListJobSecretsByJob(tracedCtx, job.ID, "production")
		if err != nil {
			return nil, nil, fmt.Errorf("failed to load secrets for job %s: %w", job.ID, err)
		}
		if len(secrets) > 0 {
			extraHeaders = make(map[string]string, len(secrets))
			for _, secret := range secrets {
				extraHeaders[fmt.Sprintf("X-Secret-%s", secret.SecretKey)] = secret.EncryptedValue
			}
		}
	}

	result, err := e.dispatchToEndpoint(tracedCtx, job.EndpointURL, run, extraHeaders)
	gotLastByte := time.Now()

	execTrace := &domain.ExecutionTrace{}
	if !connectStart.IsZero() && !connectDone.IsZero() {
		execTrace.ConnectMs = durationMillisecondsAtLeastOne(connectDone.Sub(connectStart))
	}
	if !gotFirstByte.IsZero() {
		base := dispatchStart
		if !connectDone.IsZero() {
			base = connectDone
		}
		execTrace.TtfbMs = durationMillisecondsAtLeastOne(gotFirstByte.Sub(base))
	}
	if !gotFirstByte.IsZero() {
		execTrace.TransferMs = durationMillisecondsAtLeastOne(gotLastByte.Sub(gotFirstByte))
	}
	execTrace.DispatchMs = execTrace.ConnectMs + execTrace.TtfbMs + execTrace.TransferMs

	return result, execTrace, err
}

func (e *Executor) dispatch(ctx context.Context, job *domain.Job, run *domain.JobRun) (json.RawMessage, error) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "executor.Dispatch")
	defer span.End()
	start := time.Now()
	defer func() {
		if e.metrics != nil {
			e.metrics.DispatchDuration.Record(ctx, time.Since(start).Seconds())
		}
	}()

	var extraHeaders map[string]string
	if e.secretInjection {
		secrets, err := e.store.ListJobSecretsByJob(ctx, job.ID, "production")
		if err != nil {
			return nil, fmt.Errorf("failed to load secrets for job %s: %w", job.ID, err)
		}
		if len(secrets) > 0 {
			extraHeaders = make(map[string]string, len(secrets))
			for _, secret := range secrets {
				extraHeaders[fmt.Sprintf("X-Secret-%s", secret.SecretKey)] = secret.EncryptedValue
			}
		}
	}

	return e.dispatchToEndpoint(ctx, job.EndpointURL, run, extraHeaders)
}

func (e *Executor) dispatchToEndpoint(ctx context.Context, endpointURL string, run *domain.JobRun, extraHeaders map[string]string) (json.RawMessage, error) {

	var body io.Reader
	if len(run.Payload) > 0 {
		body = bytes.NewReader(run.Payload)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL, body)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Run-ID", run.ID)
	req.Header.Set("X-Job-ID", run.JobID)
	req.Header.Set("X-Attempt", fmt.Sprintf("%d", run.Attempt))
	for key, value := range extraHeaders {
		req.Header.Set(key, value)
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		if e.metrics != nil {
			e.metrics.DispatchErrors.Add(ctx, 1)
		}
		return nil, fmt.Errorf("http dispatch: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, &domain.EndpointError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}

	if len(respBody) > 0 {
		return json.RawMessage(respBody), nil
	}

	return nil, nil
}

func (e *Executor) handleSuccess(ctx context.Context, run *domain.JobRun, job *domain.Job, result json.RawMessage, execTrace *domain.ExecutionTrace) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "executor.HandleSuccess")
	defer span.End()

	now := time.Now()
	fields := map[string]any{
		"finished_at": now,
	}
	if len(result) > 0 {
		fields["result"] = result
	}
	if execTrace != nil {
		fields["execution_trace"] = execTrace
	}

	err := e.store.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusCompleted, fields)
	if err != nil {
		e.logger.Error(
			"failed to mark run completed",
			"run_id", run.ID,
			"job_id", run.JobID,
			"error", err,
		)
		return
	}
	e.recordRunTransition(ctx, domain.StatusExecuting, domain.StatusCompleted)
	if e.circuitBreaker {
		if err := e.store.RecordEndpointCircuitSuccess(ctx, job.EndpointURL); err != nil {
			e.logger.Warn("failed to record circuit breaker success", "endpoint", job.EndpointURL, "error", err)
		}
	}

	e.logger.Info(
		"run completed",
		"run_id", run.ID,
		"job_id", run.JobID,
		"attempt", run.Attempt,
	)
	e.publishEvent(ctx, run, map[string]any{"from": "executing", "to": "completed"})
	run.Status = domain.StatusCompleted
	e.notifyWorkflowCallback(ctx, run)
	e.submitWebhook(ctx, job, run)
}

func classifyError(err error) string {
	if err == nil {
		return "unknown"
	}

	var endpointErr *domain.EndpointError
	if errors.As(err, &endpointErr) {
		switch {
		case endpointErr.StatusCode == http.StatusTooManyRequests:
			return "rate_limited"
		case endpointErr.StatusCode == http.StatusUnauthorized || endpointErr.StatusCode == http.StatusForbidden:
			return "auth"
		case endpointErr.StatusCode >= http.StatusBadRequest && endpointErr.StatusCode < http.StatusInternalServerError:
			return "client"
		case endpointErr.StatusCode >= http.StatusInternalServerError:
			return "server"
		}
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return "transient"
	}

	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return "transient"
	}

	return "unknown"
}

func shouldRetryForClass(errClass string) bool {
	switch errClass {
	case "client", "auth":
		return false
	default:
		return true
	}
}

func shouldUseFallbackForClass(errClass string) bool {
	switch errClass {
	case "transient", "rate_limited":
		return true
	default:
		return false
	}
}

func (e *Executor) handleFailure(ctx context.Context, run *domain.JobRun, job *domain.Job, err error, execTrace *domain.ExecutionTrace) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "executor.HandleFailure")
	defer span.End()

	errMsg := err.Error()
	errClass := classifyError(err)
	if e.circuitBreaker {
		if recordErr := e.store.RecordEndpointCircuitFailure(ctx, job.EndpointURL, time.Now().UTC(), e.circuitThreshold, e.circuitOpenFor); recordErr != nil {
			e.logger.Warn("failed to record circuit breaker failure", "endpoint", job.EndpointURL, "error", recordErr)
		}
	}

	e.logger.Warn(
		"run failed",
		"run_id", run.ID,
		"job_id", run.JobID,
		"attempt", run.Attempt,
		"max_attempts", job.MaxAttempts,
		"error", errMsg,
		"error_class", errClass,
	)

	shouldRetry := run.Attempt < job.MaxAttempts
	if shouldRetry && e.smartRetry && !shouldRetryForClass(errClass) {
		shouldRetry = false
	}

	if shouldRetry {
		retryAt := NextRetryAtWithStrategy(run.Attempt, job.RetryStrategy, job.RetryDelaysSecs)
		err := e.store.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusQueued, map[string]any{
			"attempt":       run.Attempt + 1,
			"next_retry_at": retryAt,
			"error":         errMsg,
			"error_class":   errClass,
			"started_at":    nil,
			"finished_at":   nil,
		})
		if err != nil {
			e.logger.Error(
				"failed to re-enqueue run",
				"run_id", run.ID,
				"job_id", run.JobID,
				"error", err,
			)
		} else {
			e.recordRunTransition(ctx, domain.StatusExecuting, domain.StatusQueued)
			e.logger.Info(
				"run re-enqueued for retry",
				"run_id", run.ID,
				"job_id", run.JobID,
				"attempt", run.Attempt+1,
				"next_retry_at", retryAt,
			)
			e.publishEvent(ctx, run, map[string]any{"from": "executing", "to": "queued", "attempt": run.Attempt + 1})
		}
		return
	}

	now := time.Now()
	fields := map[string]any{
		"finished_at": now,
		"error":       errMsg,
		"error_class": errClass,
	}
	if execTrace != nil {
		fields["execution_trace"] = execTrace
	}
	targetStatus := domain.StatusFailed
	if e.dlqEnabled {
		targetStatus = domain.StatusDeadLetter
	}

	updateErr := e.store.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, targetStatus, fields)
	if updateErr != nil {
		e.logger.Error(
			"failed to mark run terminal",
			"run_id", run.ID,
			"job_id", run.JobID,
			"error", updateErr,
		)
		return
	}
	e.recordRunTransition(ctx, domain.StatusExecuting, targetStatus)
	e.publishEvent(ctx, run, map[string]any{"from": "executing", "to": string(targetStatus), "error": errMsg})
	run.Status = targetStatus
	e.notifyWorkflowCallback(ctx, run)
	e.submitWebhook(ctx, job, run)
}

func (e *Executor) handleTimeout(ctx context.Context, run *domain.JobRun, job *domain.Job, execTrace *domain.ExecutionTrace) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "executor.HandleTimeout")
	defer span.End()

	if e.circuitBreaker {
		if err := e.store.RecordEndpointCircuitFailure(ctx, job.EndpointURL, time.Now().UTC(), e.circuitThreshold, e.circuitOpenFor); err != nil {
			e.logger.Warn("failed to record circuit breaker timeout", "endpoint", job.EndpointURL, "error", err)
		}
	}

	e.logger.Warn(
		"run timed out",
		"run_id", run.ID,
		"job_id", run.JobID,
		"attempt", run.Attempt,
		"timeout_secs", job.TimeoutSecs,
	)

	if run.Attempt < job.MaxAttempts {
		retryAt := NextRetryAt(run.Attempt)
		err := e.store.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusQueued, map[string]any{
			"attempt":       run.Attempt + 1,
			"next_retry_at": retryAt,
			"error":         "execution timed out",
			"error_class":   "transient",
			"started_at":    nil,
			"finished_at":   nil,
		})
		if err != nil {
			e.logger.Error(
				"failed to re-enqueue timed out run",
				"run_id", run.ID,
				"job_id", run.JobID,
				"error", err,
			)
		} else {
			e.recordRunTransition(ctx, domain.StatusExecuting, domain.StatusQueued)
			e.publishEvent(ctx, run, map[string]any{"from": "executing", "to": "queued", "attempt": run.Attempt + 1})
		}
		return
	}

	now := time.Now()
	fields := map[string]any{
		"finished_at": now,
		"error":       "execution timed out",
		"error_class": "transient",
	}
	if execTrace != nil {
		fields["execution_trace"] = execTrace
	}
	err := e.store.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusTimedOut, fields)
	if err != nil {
		e.logger.Error(
			"failed to mark run timed_out",
			"run_id", run.ID,
			"job_id", run.JobID,
			"error", err,
		)
		return
	}
	e.recordRunTransition(ctx, domain.StatusExecuting, domain.StatusTimedOut)
	e.publishEvent(ctx, run, map[string]any{"from": "executing", "to": "timed_out"})
	run.Status = domain.StatusTimedOut
	e.notifyWorkflowCallback(ctx, run)
	e.submitWebhook(ctx, job, run)
}

func (e *Executor) submitWebhook(ctx context.Context, job *domain.Job, run *domain.JobRun) {
	detached := context.WithoutCancel(ctx)
	e.pool.Submit(detached, func() {
		webhookCtx, wCancel := context.WithTimeout(detached, 15*time.Second)
		defer wCancel()
		SendWebhook(webhookCtx, job, run)
	})
}

func (e *Executor) handleSystemFailure(ctx context.Context, run *domain.JobRun, reason string) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "executor.HandleSystemFailure")
	defer span.End()

	now := time.Now()
	err := e.store.UpdateRunStatus(ctx, run.ID, run.Status, domain.StatusSystemFailed, map[string]any{
		"finished_at": now,
		"error":       reason,
		"error_class": "server",
	})
	if err != nil {
		e.logger.Error(
			"failed to mark system failure",
			"run_id", run.ID,
			"job_id", run.JobID,
			"error", err,
		)
		return
	}
	e.recordRunTransition(ctx, run.Status, domain.StatusSystemFailed)
	e.publishEvent(ctx, run, map[string]any{"from": string(run.Status), "to": "system_failed", "error": reason})
	run.Status = domain.StatusSystemFailed
	e.notifyWorkflowCallback(ctx, run)
	// No webhook for system failures — job may not be available
}

func (e *Executor) recordRunTransition(ctx context.Context, fromStatus, toStatus domain.RunStatus) {
	if e.metrics == nil {
		return
	}

	e.metrics.RunTransitions.Add(ctx, 1, metric.WithAttributes(
		attribute.String("from", string(fromStatus)),
		attribute.String("to", string(toStatus)),
	))
}

func durationMillisecondsAtLeastOne(d time.Duration) int64 {
	if d <= 0 {
		return 0
	}
	ms := d.Milliseconds()
	if ms == 0 {
		return 1
	}
	return ms
}
