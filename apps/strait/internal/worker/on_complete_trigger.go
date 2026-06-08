package worker

import (
	"context"
	"encoding/json"
	"log/slog"
	"strconv"

	"strait/internal/domain"
)

// OnCompleteTrigger handles triggering a workflow or job when a job run completes.
type OnCompleteTrigger struct {
	workflowLookup  WorkflowLookup
	workflowTrigger WorkflowTriggerer
	jobLookup       JobLookup
	jobEnqueuer     JobEnqueuer
	logger          *slog.Logger
}

// NewOnCompleteTrigger creates a new OnCompleteTrigger.
func NewOnCompleteTrigger(lookup WorkflowLookup, trigger WorkflowTriggerer, jobLookup JobLookup, jobEnqueuer JobEnqueuer, logger *slog.Logger) *OnCompleteTrigger {
	if logger == nil {
		logger = slog.Default()
	}
	return &OnCompleteTrigger{
		workflowLookup:  lookup,
		workflowTrigger: trigger,
		jobLookup:       jobLookup,
		jobEnqueuer:     jobEnqueuer,
		logger:          logger,
	}
}

// MaybeTrigger checks if the job has on_complete trigger configured
// and triggers the downstream workflow or job if the run completed successfully.
func (t *OnCompleteTrigger) MaybeTrigger(ctx context.Context, run *domain.JobRun, job *domain.Job, result json.RawMessage) {
	if run == nil || job == nil {
		return
	}
	if run.Status != domain.StatusCompleted {
		return
	}

	hasWorkflow := job.OnCompleteTriggerWorkflow != ""
	hasJob := job.OnCompleteTriggerJob != ""
	if !hasWorkflow && !hasJob {
		return
	}

	payload := result
	if len(job.OnCompletePayloadMapping) > 0 {
		mapped, mapErr := applyPayloadMapping(result, job.OnCompletePayloadMapping)
		if mapErr != nil {
			t.logger.Warn("on_complete payload mapping failed, using full result",
				"job_id", job.ID,
				"run_id", run.ID,
				"error", mapErr,
			)
		} else {
			payload = mapped
		}
	}

	if hasWorkflow {
		t.triggerWorkflow(ctx, run, job, payload, job.OnCompleteTriggerWorkflow, domain.TriggerJobCompletion)
	}
	if hasJob {
		t.triggerJob(ctx, run, job, payload, job.OnCompleteTriggerJob, domain.TriggerJobChain)
	}
}

// MaybeTriggerOnFailure checks if the job has on_failure trigger configured
// and triggers the downstream workflow or job when the run reaches a terminal failure state.
func (t *OnCompleteTrigger) MaybeTriggerOnFailure(ctx context.Context, run *domain.JobRun, job *domain.Job, errMsg string) {
	if run == nil || job == nil {
		return
	}

	hasWorkflow := job.OnFailureTriggerWorkflow != ""
	hasJob := job.OnFailureTriggerJob != ""
	if !hasWorkflow && !hasJob {
		return
	}

	if !isTerminalFailureStatus(run.Status) {
		return
	}

	failurePayload, _ := marshalOnFailurePayload(job, run, errMsg)

	payload := json.RawMessage(failurePayload)
	if len(job.OnFailurePayloadMapping) > 0 {
		mapped, mapErr := applyPayloadMapping(failurePayload, job.OnFailurePayloadMapping)
		if mapErr != nil {
			t.logger.Warn("on_failure payload mapping failed, using full failure context",
				"job_id", job.ID,
				"run_id", run.ID,
				"error", mapErr,
			)
		} else {
			payload = mapped
		}
	}

	if hasWorkflow {
		t.triggerWorkflow(ctx, run, job, payload, job.OnFailureTriggerWorkflow, domain.TriggerJobFailure)
	}
	if hasJob {
		t.triggerJob(ctx, run, job, payload, job.OnFailureTriggerJob, domain.TriggerJobFailure)
	}
}

func isTerminalFailureStatus(status domain.RunStatus) bool {
	switch status {
	case domain.StatusDeadLetter, domain.StatusTimedOut, domain.StatusCrashed, domain.StatusSystemFailed:
		return true
	default:
		return false
	}
}

func marshalOnFailurePayload(job *domain.Job, run *domain.JobRun, errMsg string) ([]byte, error) {
	originalInput := []byte(run.Payload)
	if originalInput == nil {
		originalInput = []byte("null")
	} else if !json.Valid(originalInput) {
		return json.Marshal(onFailurePayload{
			SourceJobID:   job.ID,
			SourceRunID:   run.ID,
			Error:         errMsg,
			ErrorClass:    run.ErrorClass,
			Status:        string(run.Status),
			Attempt:       run.Attempt,
			OriginalInput: run.Payload,
		})
	}

	const (
		onFailurePayloadFixedBytes = 98
		maxIntDecimalBytes         = 20
	)
	payload := make([]byte, 0,
		onFailurePayloadFixedBytes+maxIntDecimalBytes+len(job.ID)+len(run.ID)+len(errMsg)+len(run.ErrorClass)+len(run.Status)+len(originalInput))
	payload = append(payload, `{"source_job_id":`...)
	payload = strconv.AppendQuote(payload, job.ID)
	payload = append(payload, `,"source_run_id":`...)
	payload = strconv.AppendQuote(payload, run.ID)
	payload = append(payload, `,"error":`...)
	payload = strconv.AppendQuote(payload, errMsg)
	payload = append(payload, `,"error_class":`...)
	payload = strconv.AppendQuote(payload, run.ErrorClass)
	payload = append(payload, `,"status":`...)
	payload = strconv.AppendQuote(payload, string(run.Status))
	payload = append(payload, `,"attempt":`...)
	payload = strconv.AppendInt(payload, int64(run.Attempt), 10)
	payload = append(payload, `,"original_input":`...)
	payload = append(payload, originalInput...)
	payload = append(payload, '}')
	return payload, nil
}

type onFailurePayload struct {
	SourceJobID   string          `json:"source_job_id"`
	SourceRunID   string          `json:"source_run_id"`
	Error         string          `json:"error"`
	ErrorClass    string          `json:"error_class"`
	Status        string          `json:"status"`
	Attempt       int             `json:"attempt"`
	OriginalInput json.RawMessage `json:"original_input"`
}
