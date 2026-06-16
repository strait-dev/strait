package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"strait/internal/domain"

	"github.com/danielgtaylor/huma/v2"
)

func triggerIdempotencyKey(input *TriggerJobInput) (string, error) {
	idempotencyKey := input.XIdempotencyKey
	if idempotencyKey == "" {
		idempotencyKey = input.IdempotencyKeyAlt
	}
	if len(idempotencyKey) > maxIdempotencyKeyLength {
		return "", huma.Error400BadRequest(
			fmt.Sprintf("idempotency key must be %d characters or fewer", maxIdempotencyKeyLength))
	}
	return idempotencyKey, nil
}

func (s *Server) triggerIdempotencyHit(ctx context.Context, job *domain.Job, idempotencyKey string) (*rawStatusError, error) {
	if idempotencyKey == "" {
		return nil, nil
	}

	existingRun, err := s.store.GetRunByIdempotencyKey(ctx, job.ID, idempotencyKey)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to check idempotency key")
	}
	if existingRun == nil {
		return nil, nil
	}

	idempotencyKeyHash := hashIdempotencyKey(idempotencyKey)
	slog.Info("idempotency hit",
		"job_id", job.ID,
		"idempotency_key_hash", idempotencyKeyHash,
		"existing_run_id", existingRun.ID,
		"existing_run_status", existingRun.Status)
	return idempotencyResponse(existingRun), nil
}

func (s *Server) resolveTriggerIdempotencyConflict(ctx context.Context, job *domain.Job, idempotencyKey string, err error) error {
	if !errors.Is(err, domain.ErrIdempotencyConflict) || idempotencyKey == "" {
		return nil
	}

	// The unique index is the final idempotency boundary when concurrent
	// requests pass the app-level lookup at the same time.
	existingRun, retryErr := s.store.GetRunByIdempotencyKey(ctx, job.ID, idempotencyKey)
	if retryErr != nil {
		slog.Error("idempotency conflict retry failed",
			"job_id", job.ID,
			"idempotency_key_hash", hashIdempotencyKey(idempotencyKey),
			"error", retryErr)
		return huma.Error500InternalServerError("failed to check idempotency key after conflict")
	}
	if existingRun == nil {
		slog.Error("idempotency conflict retry returned nil",
			"job_id", job.ID,
			"idempotency_key_hash", hashIdempotencyKey(idempotencyKey))
		return nil
	}

	slog.Warn("idempotency conflict resolved",
		"job_id", job.ID,
		"idempotency_key_hash", hashIdempotencyKey(idempotencyKey),
		"winning_run_id", existingRun.ID)
	return idempotencyResponse(existingRun)
}

type triggerIdempotencyResponse struct {
	ID             string           `json:"id"`
	Status         domain.RunStatus `json:"status"`
	IdempotencyHit bool             `json:"idempotency_hit"`
}

func idempotencyResponse(run *domain.JobRun) *rawStatusError {
	return &rawStatusError{status: http.StatusOK, body: triggerIdempotencyResponse{
		ID:             run.ID,
		Status:         run.Status,
		IdempotencyHit: true,
	}}
}
