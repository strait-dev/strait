package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/danielgtaylor/huma/v2"

	"strait/internal/domain"
	"strait/internal/store"
)

const maxPauseReasonLen = 500

// PauseJobRequest is the optional body for pausing a job.
type PauseJobRequest struct {
	Reason string `json:"reason,omitempty" maxLength:"500"`
}

// PauseJobInput is the typed input for pausing a job.
type PauseJobInput struct {
	JobID string `path:"jobID"`
	Body  PauseJobRequest
}

// PauseJobOutput is the typed output for pausing a job.
type PauseJobOutput struct {
	Body *domain.Job
}

func (s *Server) handlePauseJob(ctx context.Context, input *PauseJobInput) (*PauseJobOutput, error) {
	if len(input.Body.Reason) > maxPauseReasonLen {
		return nil, huma.Error400BadRequest(fmt.Sprintf("reason must be %d characters or fewer", maxPauseReasonLen))
	}

	job, err := s.store.GetJob(ctx, input.JobID)
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

	alreadyPaused := job.Paused

	if !alreadyPaused {
		if err := s.store.PauseJob(ctx, input.JobID, input.Body.Reason); err != nil {
			if errors.Is(err, store.ErrJobNotFound) {
				return nil, huma.Error404NotFound("job not found")
			}
			return nil, huma.Error500InternalServerError("failed to pause job")
		}

		slog.Info("job paused",
			"job_id", input.JobID,
			"reason", input.Body.Reason,
			"actor", actorFromContext(ctx),
			"project_id", projectIDFromContext(ctx))
		s.emitAuditEvent(ctx, domain.AuditActionJobPaused, "job", input.JobID, map[string]any{
			"reason": input.Body.Reason,
		})
	}

	updated, err := s.store.GetJob(ctx, input.JobID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get updated job")
	}

	return &PauseJobOutput{Body: updated}, nil
}

// ResumeJobInput is the typed input for resuming a job.
type ResumeJobInput struct {
	JobID string `path:"jobID"`
}

// ResumeJobOutput is the typed output for resuming a job.
type ResumeJobOutput struct {
	Body *domain.Job
}

func (s *Server) handleResumeJob(ctx context.Context, input *ResumeJobInput) (*ResumeJobOutput, error) {
	job, err := s.store.GetJob(ctx, input.JobID)
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

	wasPaused := job.Paused

	if wasPaused {
		if err := s.store.ResumeJob(ctx, input.JobID); err != nil {
			if errors.Is(err, store.ErrJobNotFound) {
				return nil, huma.Error404NotFound("job not found")
			}
			return nil, huma.Error500InternalServerError("failed to resume job")
		}

		slog.Info("job resumed",
			"job_id", input.JobID,
			"actor", actorFromContext(ctx),
			"project_id", projectIDFromContext(ctx))
		s.emitAuditEvent(ctx, domain.AuditActionJobResumed, "job", input.JobID, nil)
	}

	updated, err := s.store.GetJob(ctx, input.JobID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get updated job")
	}

	return &ResumeJobOutput{Body: updated}, nil
}
