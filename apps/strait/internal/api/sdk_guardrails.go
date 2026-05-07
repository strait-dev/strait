package api

import (
	"context"

	"strait/internal/domain"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
)

type sdkIterationRequest struct {
	Iteration   int    `json:"iteration" validate:"required,min=1"`
	Description string `json:"description,omitempty"`
}

type SDKIterationInput struct {
	RunID string `path:"runID"`
	Body  sdkIterationRequest
}

type SDKIterationOutput struct {
	Body *domain.RunIteration
}

func (s *Server) handleSDKIteration(ctx context.Context, input *SDKIterationInput) (*SDKIterationOutput, error) {
	w := responseWriterFromContext(ctx)
	if w != nil {
		applySDKResponseHeaders(ctx, w)
	}

	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}

	// Iteration count limit check.
	run, runErr := s.store.GetRun(ctx, input.RunID)
	if runErr == nil && run != nil {
		job, jobErr := s.store.GetJob(ctx, run.JobID)
		quota, qErr := s.store.GetProjectQuota(ctx, run.ProjectID)
		var quotaLimit int
		if qErr == nil && quota != nil {
			quotaLimit = quota.MaxIterationsPerRun
		}
		var jobLimit int
		if jobErr == nil && job != nil {
			jobLimit = job.MaxIterationsPerRun
		}
		iterLimit := resolveGuardrailInt(quotaLimit, jobLimit)
		if iterLimit > 0 {
			count, cErr := s.store.CountRunIterations(ctx, input.RunID)
			if cErr == nil && count >= iterLimit {
				return nil, &rawStatusError{
					status: 429,
					body: map[string]any{
						"error": "iteration_limit_exceeded", "current": count, "limit": iterLimit,
					},
				}
			}
		}
	}

	iter := &domain.RunIteration{
		ID:          uuid.Must(uuid.NewV7()).String(),
		RunID:       input.RunID,
		Iteration:   req.Iteration,
		Description: req.Description,
	}
	var err error
	if guardedStore, ok := s.store.(activeRunMutationStore); ok {
		err = guardedStore.CreateRunIterationForActiveRun(ctx, iter, runTokenAttemptFromContext(ctx))
	} else {
		err = s.store.CreateRunIteration(ctx, iter)
	}
	if err != nil {
		if sdkErr := s.guardedSDKMutationError(ctx, err); sdkErr != nil {
			return nil, sdkErr
		}
		return nil, huma.Error500InternalServerError("failed to create run iteration")
	}

	return &SDKIterationOutput{Body: iter}, nil
}

// resolveGuardrailInt64 returns the job-level limit if set, otherwise the quota-level limit.
func resolveGuardrailInt64(quotaLimit, jobLimit int64) int64 {
	if jobLimit > 0 {
		return jobLimit
	}
	return quotaLimit
}

// resolveGuardrailInt returns the job-level limit if set, otherwise the quota-level limit.
func resolveGuardrailInt(quotaLimit, jobLimit int) int {
	if jobLimit > 0 {
		return jobLimit
	}
	return quotaLimit
}
