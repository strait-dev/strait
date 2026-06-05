package api

import (
	"context"
	"encoding/json"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/danielgtaylor/huma/v2"
)

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
