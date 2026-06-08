package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"strait/internal/domain"
	"strait/internal/httputil"
	"strait/internal/store"
)

// completeRunWithWebhook atomically updates run status and enqueues a webhook
// delivery within a single database transaction. If the job has no webhook URL
// or no txPool is configured, it falls back to a plain status update.
// The run must be in StatusExecuting when this is called.
func (e *Executor) completeRunWithWebhook(ctx context.Context, run *domain.JobRun, job *domain.Job, to domain.RunStatus, fields map[string]any) error {
	completion := newTerminalRunCompletion(run, job, to, fields)
	return e.retryTerminalCompletion(ctx, run, job, func(ctx context.Context) error {
		return e.completeRunWithWebhookOnce(ctx, run, job, completion)
	})
}

func (e *Executor) completeRunWithWebhookOnce(ctx context.Context, run *domain.JobRun, job *domain.Job, completion terminalRunCompletion) error {
	if e.txPool != nil {
		return store.WithTx(ctx, e.txPool, func(q *store.Queries) error {
			if err := q.UpdateRunStatus(ctx, run.ID, completion.from, completion.to, completion.fields); err != nil {
				return err
			}
			if completion.recordEndpointSuccess {
				if err := q.RecordEndpointCircuitSuccess(ctx, endpointStateKey(job.ProjectID, job.EndpointURL)); err != nil {
					return err
				}
			}
			if !completion.enqueueWebhook {
				return nil
			}
			_, err := q.EnqueueRunWebhook(ctx, job, completion.webhookRun, e.webhookMaxRetry)
			return err
		})
	}
	if completion.enqueueWebhook {
		e.logger.Warn("txPool not configured, webhook delivery skipped for completed run",
			"run_id", run.ID, "job_id", job.ID, "webhook_url", httputil.RedactURLForLog(job.WebhookURL))
	}
	return e.store.UpdateRunStatus(ctx, run.ID, completion.from, completion.to, completion.fields)
}

func (e *Executor) retryTerminalCompletion(
	ctx context.Context,
	run *domain.JobRun,
	job *domain.Job,
	fn func(context.Context) error,
) error {
	err := fn(ctx)
	if err == nil || !isRetryableTerminalCompletionError(err) {
		return err
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return fmt.Errorf("terminal run persistence retry stopped: %w: %w", ctxErr, err)
	}

	timeout := e.terminalRetryTimeout
	if timeout == 0 {
		timeout = defaultTerminalRetryTimeout
	}
	if timeout < 0 {
		return err
	}
	retryCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	backoff := e.terminalRetryInitial
	if backoff <= 0 {
		backoff = defaultTerminalRetryInitial
	}
	maxBackoff := e.terminalRetryMax
	if maxBackoff <= 0 {
		maxBackoff = defaultTerminalRetryMax
	}
	if maxBackoff < backoff {
		maxBackoff = backoff
	}

	attempt := 1
	for {
		e.logger.Warn(
			"retrying terminal run persistence after transient database failure",
			"run_id", run.ID,
			"job_id", job.ID,
			"attempt", attempt,
			"backoff", backoff,
			"error", err,
		)
		timer := time.NewTimer(backoff)
		select {
		case <-retryCtx.Done():
			timer.Stop()
			return fmt.Errorf("terminal run persistence retry exhausted: %w: %w", retryCtx.Err(), err)
		case <-timer.C:
		}

		err = fn(retryCtx)
		if err == nil {
			return nil
		}
		if retryCtx.Err() != nil || !isRetryableTerminalCompletionError(err) {
			return err
		}
		attempt++
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func isRetryableTerminalCompletionError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	return isRetryablePostgresError(err)
}

type terminalRunCompletion struct {
	from                  domain.RunStatus
	to                    domain.RunStatus
	fields                map[string]any
	webhookRun            *domain.JobRun
	recordEndpointSuccess bool
	enqueueWebhook        bool
}

func newTerminalRunCompletion(run *domain.JobRun, job *domain.Job, to domain.RunStatus, fields map[string]any) terminalRunCompletion {
	return terminalRunCompletion{
		from:                  domain.StatusExecuting,
		to:                    to,
		fields:                fields,
		webhookRun:            runForTerminalWebhook(run, to, fields),
		recordEndpointSuccess: to == domain.StatusCompleted && job.EndpointURL != "",
		enqueueWebhook:        job.WebhookURL != "",
	}
}

func runForTerminalWebhook(run *domain.JobRun, status domain.RunStatus, fields map[string]any) *domain.JobRun {
	webhookRun := *run
	webhookRun.Status = status
	if result, ok := fields["result"].(json.RawMessage); ok {
		webhookRun.Result = result
	}
	if errMsg, ok := fields["error"].(string); ok {
		webhookRun.Error = errMsg
	}
	if finishedAt, ok := fields["finished_at"].(time.Time); ok {
		webhookRun.FinishedAt = &finishedAt
	}
	return &webhookRun
}
