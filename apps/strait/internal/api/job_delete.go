package api

import (
	"context"
	"errors"
	"log/slog"

	"github.com/danielgtaylor/huma/v2"

	"strait/internal/domain"
	"strait/internal/store"
)

// DeleteJobInput is the typed input for deleting a job.
type DeleteJobInput struct {
	JobID string `path:"jobID"`
}

func (s *Server) handleDeleteJob(ctx context.Context, input *DeleteJobInput) (*struct{}, error) {
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

	if err := s.store.DeleteJob(ctx, input.JobID); err != nil {
		if errors.Is(err, store.ErrJobNotFound) {
			return nil, huma.Error404NotFound("job not found")
		}
		if errors.Is(err, store.ErrJobHasActiveRuns) {
			return nil, huma.Error409Conflict("job has active runs — cancel them first or wait for completion")
		}
		return nil, huma.Error500InternalServerError("failed to delete job")
	}
	s.invalidateJobCaches(ctx, input.JobID, 0)

	slog.Info("job deleted",
		"job_id", input.JobID,
		"actor", actorFromContext(ctx),
		"project_id", projectIDFromContext(ctx))
	s.emitAuditEvent(ctx, domain.AuditActionJobDeleted, "job", input.JobID, nil)

	return nil, nil
}
