package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/danielgtaylor/huma/v2"
)

// maxIdempotencyKeyLength is the maximum allowed length for idempotency keys.
// Keys exceeding this limit are rejected with 400 to protect the DB index.
const maxIdempotencyKeyLength = 256

type TriggerRequest struct {
	Payload        json.RawMessage   `json:"payload,omitempty"`
	Tags           map[string]string `json:"tags,omitempty"`
	ScheduledAt    *time.Time        `json:"scheduled_at,omitempty"`
	Priority       int               `json:"priority,omitempty" validate:"min=0,max=10"`
	DryRun         bool              `json:"dry_run,omitempty"`
	TTLSecs        *int              `json:"ttl_secs,omitempty" validate:"omitempty,min=0,max=2592000"`
	ConcurrencyKey string            `json:"concurrency_key,omitempty" validate:"max=256"`
	DebounceKey    string            `json:"debounce_key,omitempty" validate:"max=256"`
	BatchKey       string            `json:"batch_key,omitempty" validate:"max=256"`
}

const maxTriggerTTLSecs = 30 * 24 * 60 * 60

type TriggerJobInput struct {
	JobID             string `path:"jobID"`
	XIdempotencyKey   string `header:"X-Idempotency-Key"`
	IdempotencyKeyAlt string `header:"Idempotency-Key"`
	Traceparent       string `header:"Traceparent" maxLength:"256"`
	Tracestate        string `header:"Tracestate" maxLength:"8192"`
	SentryTrace       string `header:"Sentry-Trace" maxLength:"8192"`
	Baggage           string `header:"Baggage" maxLength:"8192"`
	Body              TriggerRequest
}

type TriggerJobOutput struct {
	Body any
}

func (s *Server) handleTriggerJob(ctx context.Context, input *TriggerJobInput) (*TriggerJobOutput, error) {
	job, err := s.loadTriggerJob(ctx, input.JobID)
	if err != nil {
		return nil, err
	}

	req := input.Body
	if err := s.validateTriggerJobInput(input, &req); err != nil {
		return nil, err
	}

	if req.DryRun {
		return s.handleTriggerDryRun(ctx, job.ID, req)
	}

	state, idempotencyHit, err := s.prepareTriggerRequest(ctx, input, job, req)
	if err != nil {
		return nil, err
	}
	if idempotencyHit != nil {
		return nil, idempotencyHit
	}

	if dedupOutput, err := s.triggerDedupOutput(ctx, state); err != nil || dedupOutput != nil {
		return dedupOutput, err
	}
	if debounceOutput, handled, err := s.handleDebounceTrigger(ctx, state); err != nil || handled {
		return debounceOutput, err
	}
	if batchOutput, handled, err := s.handleBatchTrigger(ctx, input, state); err != nil || handled {
		return batchOutput, err
	}

	return s.handleImmediateTrigger(ctx, input, state)
}

type triggerRequestState struct {
	job            *domain.Job
	req            TriggerRequest
	payload        json.RawMessage
	payloadHash    string
	idempotencyKey string
	projectQuota   *store.ProjectQuota
}

func (s *Server) prepareTriggerRequest(
	ctx context.Context,
	input *TriggerJobInput,
	job *domain.Job,
	req TriggerRequest,
) (*triggerRequestState, *rawStatusError, error) {
	if err := validatePayloadAgainstSchema(req.Payload, job.PayloadSchema); err != nil {
		return nil, nil, huma.Error400BadRequest("payload validation failed: " + err.Error())
	}

	payload, payloadHash, err := canonicalizePayload(req.Payload)
	if err != nil {
		return nil, nil, huma.Error400BadRequest("invalid payload: " + err.Error())
	}

	idempotencyKey, err := triggerIdempotencyKey(input)
	if err != nil {
		return nil, nil, err
	}
	idempotencyHit, err := s.triggerIdempotencyHit(ctx, job, idempotencyKey)
	if err != nil {
		return nil, nil, err
	}
	if idempotencyHit != nil {
		return nil, idempotencyHit, nil
	}

	if err := s.checkTriggerDispatchPriority(ctx, job.ProjectID, req.Priority); err != nil {
		return nil, nil, err
	}

	projectQuota, err := s.quotaCache.Get(ctx, job.ProjectID)
	if err != nil {
		return nil, nil, huma.Error500InternalServerError("failed to load project quota")
	}

	if err := s.checkTriggerDailyCostBudget(ctx, job.ProjectID, projectQuota); err != nil {
		return nil, nil, err
	}

	return &triggerRequestState{
		job:            job,
		req:            req,
		payload:        payload,
		payloadHash:    payloadHash,
		idempotencyKey: idempotencyKey,
		projectQuota:   projectQuota,
	}, idempotencyHit, nil
}

func (s *Server) loadTriggerJob(ctx context.Context, jobID string) (*domain.Job, error) {
	job, err := s.loadRunCreationJob(ctx, jobID, "trigger_job.project_match", "handleTriggerJob")
	if err != nil {
		return nil, err
	}
	if err := ensureJobTriggerable(job); err != nil {
		return nil, err
	}
	return job, nil
}

func (s *Server) loadRunCreationJob(ctx context.Context, jobID, auditAction, handlerName string) (*domain.Job, error) {
	if err := validateRunCreationJobID(jobID); err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	job, err := s.store.GetJob(ctx, jobID)
	if err != nil {
		if errors.Is(err, store.ErrJobNotFound) {
			return nil, huma.Error404NotFound("job not found")
		}
		return nil, huma.Error500InternalServerError("failed to get job")
	}

	if err := requireProjectMatch(ctx, job.ProjectID); err != nil {
		return nil, huma.Error404NotFound("job not found")
	}
	if err := requireEnvironmentMatch(ctx, job.EnvironmentID); err != nil {
		return nil, huma.Error404NotFound("job not found")
	}
	s.emitInternalSecretBypassAuditIfProjectless(ctx, auditAction, handlerName, "job", job.ID)
	return job, nil
}

func (s *Server) validateTriggerJobInput(input *TriggerJobInput, req *TriggerRequest) error {
	if err := s.validate.Struct(req); err != nil {
		return newValidationError(err)
	}
	if err := validateTriggerTraceHeaders(input); err != nil {
		return huma.Error400BadRequest(err.Error())
	}
	if err := validatePayloadSize(req.Payload); err != nil {
		return huma.Error400BadRequest(err.Error())
	}
	if err := validateTags(req.Tags); err != nil {
		return huma.Error400BadRequest(err.Error())
	}
	if err := validateTriggerScheduledAt(req.ScheduledAt); err != nil {
		return huma.Error400BadRequest(err.Error())
	}
	return nil
}

func (s *Server) handleTriggerDryRun(ctx context.Context, jobID string, req TriggerRequest) (*TriggerJobOutput, error) {
	result, err := s.validateTriggerRequest(ctx, jobID, req)
	if err != nil {
		var statusErr huma.StatusError
		if errors.As(err, &statusErr) {
			return nil, statusErr
		}
		return nil, huma.Error400BadRequest(err.Error())
	}
	return nil, &rawStatusError{status: http.StatusOK, body: result}
}

func (s *Server) enqueueTriggerRun(ctx context.Context, tx store.DBTX, run *domain.JobRun) error {
	if tx != nil {
		return s.queue.EnqueueInTx(ctx, tx, run)
	}
	return s.queue.Enqueue(ctx, run)
}
