package api

import (
	"context"
	"encoding/json"
	"time"
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
