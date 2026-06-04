package worker

import (
	"context"
	"encoding/json"
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
