package api

import (
	"context"
	"errors"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/danielgtaylor/huma/v2"
)

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

	load := func(loadCtx context.Context, loadJobID string) (*domain.Job, error) {
		return s.store.GetJob(loadCtx, loadJobID)
	}
	job, err := s.triggerJobCache.Get(ctx, jobID, load)
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
	s.emitInternalSecretBypassAuditIfProjectless(ctx, job.ProjectID, auditAction, handlerName, "job", job.ID)
	return job, nil
}
