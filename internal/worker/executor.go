package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"orchestrator/internal/domain"
	"orchestrator/internal/pubsub"
	"orchestrator/internal/queue"
	"orchestrator/internal/store"
)

// Executor polls the queue and executes job runs via HTTP dispatch.
type Executor struct {
	pool         *Pool
	queue        queue.Queue
	store        store.Store
	httpClient   *http.Client
	pollInterval time.Duration
	heartbeat    *HeartbeatSender
	publisher    pubsub.Publisher
	logger       *slog.Logger
}

// ExecutorConfig holds configuration for the Executor.
type ExecutorConfig struct {
	Pool              *Pool
	Queue             queue.Queue
	Store             store.Store
	Publisher         pubsub.Publisher
	PollInterval      time.Duration
	HeartbeatInterval time.Duration
}

func NewExecutor(cfg ExecutorConfig) *Executor {
	return &Executor{
		pool:         cfg.Pool,
		queue:        cfg.Queue,
		store:        cfg.Store,
		httpClient: &http.Client{
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
				TLSHandshakeTimeout: 10 * time.Second,
			},
		},
		pollInterval: cfg.PollInterval,
		heartbeat:    NewHeartbeatSender(cfg.Store, cfg.HeartbeatInterval),
		publisher:    cfg.Publisher,
		logger:       slog.Default(),
	}
}

func (e *Executor) publishEvent(ctx context.Context, run *domain.JobRun, eventType string, data map[string]any) {
	if e.publisher == nil {
		return
	}

	event := map[string]any{
		"type":       eventType,
		"run_id":     run.ID,
		"job_id":     run.JobID,
		"project_id": run.ProjectID,
		"timestamp":  time.Now().UTC(),
	}
	for k, v := range data {
		event[k] = v
	}

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
	run, err := e.queue.Dequeue(ctx)
	if err != nil {
		e.logger.Error("dequeue failed", "error", err)
		return
	}
	if run == nil {
		return
	}

	e.logger.Info(
		"dequeued run",
		"run_id", run.ID,
		"job_id", run.JobID,
		"project_id", run.ProjectID,
		"attempt", run.Attempt,
	)

	// Use detached context for in-flight work — shutdown stops polling but
	// lets running jobs complete naturally (bounded by their timeout).
	execCtx := context.WithoutCancel(ctx)
	e.pool.Submit(ctx, func() {
		e.execute(execCtx, run)
	})
}

func (e *Executor) execute(ctx context.Context, run *domain.JobRun) {
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
	e.publishEvent(ctx, run, "status_change", map[string]any{"from": "dequeued", "to": "executing"})

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
			e.handleFailure(ctx, run, job, err.Error())
		}
		return
	}

	e.handleSuccess(ctx, run, job, result)
}

func (e *Executor) dispatch(ctx context.Context, job *domain.Job, run *domain.JobRun) (json.RawMessage, error) {
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

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http dispatch: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("endpoint returned %d: %s", resp.StatusCode, string(respBody))
	}

	if len(respBody) > 0 {
		return json.RawMessage(respBody), nil
	}

	return nil, nil
}

func (e *Executor) handleSuccess(ctx context.Context, run *domain.JobRun, job *domain.Job, result json.RawMessage) {
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

	e.logger.Info(
		"run completed",
		"run_id", run.ID,
		"job_id", run.JobID,
		"attempt", run.Attempt,
	)
	e.publishEvent(ctx, run, "status_change", map[string]any{"from": "executing", "to": "completed"})
	run.Status = domain.StatusCompleted
	e.pool.Submit(context.Background(), func() {
		webhookCtx, wCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer wCancel()
		SendWebhook(webhookCtx, job, run)
	})
}

func (e *Executor) handleFailure(ctx context.Context, run *domain.JobRun, job *domain.Job, errMsg string) {
	e.logger.Warn(
		"run failed",
		"run_id", run.ID,
		"job_id", run.JobID,
		"attempt", run.Attempt,
		"max_attempts", job.MaxAttempts,
		"error", errMsg,
	)

	if run.Attempt < job.MaxAttempts {
		retryAt := NextRetryAt(run.Attempt)
		err := e.store.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusQueued, map[string]any{
			"attempt":       run.Attempt + 1,
			"next_retry_at": retryAt,
			"error":         errMsg,
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
			e.logger.Info(
				"run re-enqueued for retry",
				"run_id", run.ID,
				"job_id", run.JobID,
				"attempt", run.Attempt+1,
				"next_retry_at", retryAt,
			)
			e.publishEvent(ctx, run, "status_change", map[string]any{"from": "executing", "to": "queued", "attempt": run.Attempt + 1})
		}
		return
	}

	now := time.Now()
	err := e.store.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusFailed, map[string]any{
		"finished_at": now,
		"error":       errMsg,
	})
	if err != nil {
		e.logger.Error(
			"failed to mark run failed",
			"run_id", run.ID,
			"job_id", run.JobID,
			"error", err,
		)
		return
	}
	e.publishEvent(ctx, run, "status_change", map[string]any{"from": "executing", "to": "failed", "error": errMsg})
	run.Status = domain.StatusFailed
	e.pool.Submit(context.Background(), func() {
		webhookCtx, wCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer wCancel()
		SendWebhook(webhookCtx, job, run)
	})
}

func (e *Executor) handleTimeout(ctx context.Context, run *domain.JobRun, job *domain.Job) {
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
			e.publishEvent(ctx, run, "status_change", map[string]any{"from": "executing", "to": "queued", "attempt": run.Attempt + 1})
		}
		return
	}

	now := time.Now()
	err := e.store.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusTimedOut, map[string]any{
		"finished_at": now,
		"error":       "execution timed out",
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
	e.publishEvent(ctx, run, "status_change", map[string]any{"from": "executing", "to": "timed_out"})
	run.Status = domain.StatusTimedOut
	e.pool.Submit(context.Background(), func() {
		webhookCtx, wCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer wCancel()
		SendWebhook(webhookCtx, job, run)
	})
}

func (e *Executor) handleSystemFailure(ctx context.Context, run *domain.JobRun, reason string) {
	now := time.Now()
	err := e.store.UpdateRunStatus(ctx, run.ID, run.Status, domain.StatusSystemFailed, map[string]any{
		"finished_at": now,
		"error":       reason,
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
	e.publishEvent(ctx, run, "status_change", map[string]any{"from": string(run.Status), "to": "system_failed", "error": reason})
	// No webhook for system failures — job may not be available
}
