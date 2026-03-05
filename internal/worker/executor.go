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
	"strconv"
	"strings"
	"time"

	"orchestrator/internal/domain"
	"orchestrator/internal/pubsub"
	"orchestrator/internal/queue"
	"orchestrator/internal/telemetry"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// ExecutorStore is the subset of store operations needed by Executor.
type ExecutorStore interface {
	GetJob(ctx context.Context, id string) (*domain.Job, error)
	UpdateRunStatus(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error
	UpdateHeartbeat(ctx context.Context, id string) error
	CanDispatchEndpoint(ctx context.Context, endpointURL string, now time.Time) (bool, *time.Time, error)
	RecordEndpointCircuitFailure(ctx context.Context, endpointURL string, now time.Time, threshold int, openDuration time.Duration) error
	RecordEndpointCircuitSuccess(ctx context.Context, endpointURL string) error
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
	smartRetry       bool
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
	SmartRetry        bool
}

const (
	defaultCircuitFailureThreshold = 5
	defaultCircuitOpenDuration     = time.Minute
)

func NewExecutor(cfg ExecutorConfig) *Executor {
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{
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
		smartRetry:       cfg.SmartRetry,
		circuitThreshold: defaultCircuitFailureThreshold,
		circuitOpenFor:   defaultCircuitOpenDuration,
		logger:           slog.Default(),
	}
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
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result, err := e.dispatch(execCtx, job, run)
	if err != nil {
		if execCtx.Err() == context.DeadlineExceeded {
			e.handleTimeout(ctx, run, job)
		} else {
			e.handleFailure(ctx, run, job, err)
		}
		return
	}

	e.handleSuccess(ctx, run, job, result)
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

	var body io.Reader
	if len(run.Payload) > 0 {
		body = bytes.NewReader(run.Payload)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, job.EndpointURL, body)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Run-ID", run.ID)
	req.Header.Set("X-Job-ID", run.JobID)
	req.Header.Set("X-Attempt", fmt.Sprintf("%d", run.Attempt))

	resp, err := e.httpClient.Do(req) //nolint:gosec // URL from validated job config
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

func (e *Executor) handleSuccess(ctx context.Context, run *domain.JobRun, job *domain.Job, result json.RawMessage) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "executor.HandleSuccess")
	defer span.End()

	now := time.Now()
	fields := map[string]any{
		"finished_at": now,
	}
	if len(result) > 0 {
		fields["result"] = result
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

func (e *Executor) handleFailure(ctx context.Context, run *domain.JobRun, job *domain.Job, err error) {
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
		retryAt := NextRetryAt(run.Attempt)
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
	updateErr := e.store.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusFailed, map[string]any{
		"finished_at": now,
		"error":       errMsg,
		"error_class": errClass,
	})
	if updateErr != nil {
		e.logger.Error(
			"failed to mark run failed",
			"run_id", run.ID,
			"job_id", run.JobID,
			"error", updateErr,
		)
		return
	}
	e.recordRunTransition(ctx, domain.StatusExecuting, domain.StatusFailed)
	e.publishEvent(ctx, run, map[string]any{"from": "executing", "to": "failed", "error": errMsg})
	run.Status = domain.StatusFailed
	e.notifyWorkflowCallback(ctx, run)
	e.submitWebhook(ctx, job, run)
}

func (e *Executor) handleTimeout(ctx context.Context, run *domain.JobRun, job *domain.Job) {
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
	err := e.store.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusTimedOut, map[string]any{
		"finished_at": now,
		"error":       "execution timed out",
		"error_class": "transient",
	})
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
