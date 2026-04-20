package api

import (
	"context"
	"errors"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/danielgtaylor/huma/v2"
)

type CreateJobGroupRequest struct {
	ProjectID   string `json:"project_id" validate:"required"`
	Name        string `json:"name" validate:"required,max=255"`
	Slug        string `json:"slug" validate:"required,max=255"`
	Description string `json:"description,omitempty" validate:"max=2000"`
}
type UpdateJobGroupRequest struct {
	Name        *string `json:"name,omitempty"`
	Slug        *string `json:"slug,omitempty"`
	Description *string `json:"description,omitempty"`
}

type CreateJobGroupInput struct{ Body CreateJobGroupRequest }
type CreateJobGroupOutput struct{ Body *domain.JobGroup }

func (s *Server) handleCreateJobGroup(ctx context.Context, input *CreateJobGroupInput) (*CreateJobGroupOutput, error) {
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}
	if err := requireProjectMatch(ctx, req.ProjectID); err != nil {
		return nil, huma.Error404NotFound("job group not found")
	}
	group := &domain.JobGroup{ProjectID: req.ProjectID, Name: req.Name, Slug: req.Slug, Description: req.Description}
	if err := s.store.CreateJobGroup(ctx, group); err != nil {
		return nil, huma.Error500InternalServerError("failed to create job group")
	}
	s.emitAuditEvent(ctx, domain.AuditActionJobGroupCreated, "job_group", group.ID, map[string]any{
		"name": group.Name,
		"slug": group.Slug,
	})
	return &CreateJobGroupOutput{Body: group}, nil
}

type GetJobGroupInput struct {
	GroupID string `path:"groupID"`
}
type GetJobGroupOutput struct{ Body *domain.JobGroup }

func (s *Server) handleGetJobGroup(ctx context.Context, input *GetJobGroupInput) (*GetJobGroupOutput, error) {
	group, err := s.store.GetJobGroup(ctx, input.GroupID)
	if err != nil {
		if errors.Is(err, store.ErrJobGroupNotFound) {
			return nil, huma.Error404NotFound("job group not found")
		}
		return nil, huma.Error500InternalServerError("failed to get job group")
	}
	if err := requireProjectMatch(ctx, group.ProjectID); err != nil {
		return nil, huma.Error404NotFound("job group not found")
	}
	return &GetJobGroupOutput{Body: group}, nil
}

type ListJobGroupsInput struct {
	Limit  string `query:"limit"`
	Cursor string `query:"cursor"`
}
type ListJobGroupsOutput struct{ Body PaginatedResponse }

func (s *Server) handleListJobGroups(ctx context.Context, input *ListJobGroupsInput) (*ListJobGroupsOutput, error) {
	limit, cursor, err := parsePaginationFromStrings(input.Limit, input.Cursor)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	groups, err := s.store.ListJobGroups(ctx, projectIDFromContext(ctx), limit+1, cursor)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list job groups")
	}
	return &ListJobGroupsOutput{Body: paginatedResult(groups, limit, func(g domain.JobGroup) string { return g.CreatedAt.Format(time.RFC3339Nano) })}, nil
}

type UpdateJobGroupInput struct {
	GroupID string `path:"groupID"`
	Body    UpdateJobGroupRequest
}
type UpdateJobGroupOutput struct{ Body *domain.JobGroup }

func (s *Server) handleUpdateJobGroup(ctx context.Context, input *UpdateJobGroupInput) (*UpdateJobGroupOutput, error) {
	group, err := s.store.GetJobGroup(ctx, input.GroupID)
	if err != nil {
		if errors.Is(err, store.ErrJobGroupNotFound) {
			return nil, huma.Error404NotFound("job group not found")
		}
		return nil, huma.Error500InternalServerError("failed to get job group")
	}
	if err := requireProjectMatch(ctx, group.ProjectID); err != nil {
		return nil, huma.Error404NotFound("job group not found")
	}
	req := input.Body
	if req.Name != nil {
		group.Name = *req.Name
	}
	if req.Slug != nil {
		group.Slug = *req.Slug
	}
	if req.Description != nil {
		group.Description = *req.Description
	}
	if err := s.store.UpdateJobGroup(ctx, group); err != nil {
		if errors.Is(err, store.ErrJobGroupNotFound) {
			return nil, huma.Error404NotFound("job group not found")
		}
		return nil, huma.Error500InternalServerError("failed to update job group")
	}
	s.emitAuditEvent(ctx, domain.AuditActionJobGroupUpdated, "job_group", group.ID, map[string]any{
		"changes": req,
		"name":    group.Name,
	})
	return &UpdateJobGroupOutput{Body: group}, nil
}

type DeleteJobGroupInput struct {
	GroupID string `path:"groupID"`
}

func (s *Server) handleDeleteJobGroup(ctx context.Context, input *DeleteJobGroupInput) (*struct{}, error) {
	group, err := s.store.GetJobGroup(ctx, input.GroupID)
	if err != nil {
		if errors.Is(err, store.ErrJobGroupNotFound) {
			return nil, huma.Error404NotFound("job group not found")
		}
		return nil, huma.Error500InternalServerError("failed to get job group")
	}
	if err := requireProjectMatch(ctx, group.ProjectID); err != nil {
		return nil, huma.Error404NotFound("job group not found")
	}
	if err := s.store.DeleteJobGroup(ctx, input.GroupID); err != nil {
		if errors.Is(err, store.ErrJobGroupNotFound) {
			return nil, huma.Error404NotFound("job group not found")
		}
		return nil, huma.Error500InternalServerError("failed to delete job group")
	}
	s.emitAuditEvent(ctx, domain.AuditActionJobGroupDeleted, "job_group", input.GroupID, map[string]any{
		"name": group.Name,
		"slug": group.Slug,
	})
	return nil, nil
}

type ListJobsByGroupInput struct {
	GroupID string `path:"groupID"`
	Limit   string `query:"limit"`
	Cursor  string `query:"cursor"`
}
type ListJobsByGroupOutput struct{ Body PaginatedResponse }

func (s *Server) handleListJobsByGroup(ctx context.Context, input *ListJobsByGroupInput) (*ListJobsByGroupOutput, error) {
	group, groupErr := s.store.GetJobGroup(ctx, input.GroupID)
	if groupErr != nil && !errors.Is(groupErr, store.ErrJobGroupNotFound) {
		return nil, huma.Error500InternalServerError("failed to get job group")
	}
	// For deleted groups, return empty results rather than 404.
	// For cross-project groups, return empty results (same as not found).
	if group != nil {
		if pmErr := requireProjectMatch(ctx, group.ProjectID); pmErr != nil {
			return &ListJobsByGroupOutput{Body: PaginatedResponse{Data: []any{}, HasMore: false}}, nil //nolint:nilerr // intentional: return empty results for cross-project
		}
	}
	limit, cursor, err := parsePaginationFromStrings(input.Limit, input.Cursor)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	jobs, err := s.store.ListJobsByGroup(ctx, input.GroupID, limit+1, cursor)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list jobs by group")
	}
	return &ListJobsByGroupOutput{Body: paginatedResult(jobs, limit, func(j domain.Job) string { return j.CreatedAt.Format(time.RFC3339Nano) })}, nil
}

type PauseAllJobsByGroupInput struct {
	GroupID string `path:"groupID"`
}
type PauseAllJobsByGroupOutput struct{ Body map[string]string }

func (s *Server) handlePauseAllJobsByGroup(ctx context.Context, input *PauseAllJobsByGroupInput) (*PauseAllJobsByGroupOutput, error) {
	group, err := s.store.GetJobGroup(ctx, input.GroupID)
	if err != nil {
		if errors.Is(err, store.ErrJobGroupNotFound) {
			return nil, huma.Error404NotFound("job group not found")
		}
		return nil, huma.Error500InternalServerError("failed to get job group")
	}
	if err := requireProjectMatch(ctx, group.ProjectID); err != nil {
		return nil, huma.Error404NotFound("job group not found")
	}
	if err := s.store.PauseJobsByGroup(ctx, input.GroupID); err != nil {
		if errors.Is(err, store.ErrJobGroupNotFound) {
			return nil, huma.Error404NotFound("job group not found")
		}
		return nil, huma.Error500InternalServerError("failed to pause jobs in group")
	}
	s.emitAuditEvent(ctx, domain.AuditActionJobGroupPausedAll, "job_group", input.GroupID, map[string]any{
		"name": group.Name,
	})
	return &PauseAllJobsByGroupOutput{Body: map[string]string{"status": "paused"}}, nil
}

type ResumeAllJobsByGroupInput struct {
	GroupID string `path:"groupID"`
}
type ResumeAllJobsByGroupOutput struct{ Body map[string]string }

func (s *Server) handleResumeAllJobsByGroup(ctx context.Context, input *ResumeAllJobsByGroupInput) (*ResumeAllJobsByGroupOutput, error) {
	group, err := s.store.GetJobGroup(ctx, input.GroupID)
	if err != nil {
		if errors.Is(err, store.ErrJobGroupNotFound) {
			return nil, huma.Error404NotFound("job group not found")
		}
		return nil, huma.Error500InternalServerError("failed to get job group")
	}
	if err := requireProjectMatch(ctx, group.ProjectID); err != nil {
		return nil, huma.Error404NotFound("job group not found")
	}
	if err := s.store.ResumeJobsByGroup(ctx, input.GroupID); err != nil {
		if errors.Is(err, store.ErrJobGroupNotFound) {
			return nil, huma.Error404NotFound("job group not found")
		}
		return nil, huma.Error500InternalServerError("failed to resume jobs in group")
	}
	s.emitAuditEvent(ctx, domain.AuditActionJobGroupResumedAll, "job_group", input.GroupID, map[string]any{
		"name": group.Name,
	})
	return &ResumeAllJobsByGroupOutput{Body: map[string]string{"status": "resumed"}}, nil
}

type GetJobGroupStatsInput struct {
	GroupID string `path:"groupID"`
}
type GetJobGroupStatsOutput struct{ Body any }

func (s *Server) handleGetJobGroupStats(ctx context.Context, input *GetJobGroupStatsInput) (*GetJobGroupStatsOutput, error) {
	group, err := s.store.GetJobGroup(ctx, input.GroupID)
	if err != nil {
		if errors.Is(err, store.ErrJobGroupNotFound) {
			return nil, huma.Error404NotFound("job group not found")
		}
		return nil, huma.Error500InternalServerError("failed to get job group")
	}
	if err := requireProjectMatch(ctx, group.ProjectID); err != nil {
		return nil, huma.Error404NotFound("job group not found")
	}
	stats, err := s.store.GetJobGroupStats(ctx, input.GroupID)
	if err != nil {
		if errors.Is(err, store.ErrJobGroupNotFound) {
			return nil, huma.Error404NotFound("job group not found")
		}
		return nil, huma.Error500InternalServerError("failed to get job group stats")
	}
	return &GetJobGroupStatsOutput{Body: stats}, nil
}
