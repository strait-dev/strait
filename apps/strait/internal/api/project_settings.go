package api

import (
	"context"

	"strait/internal/domain"

	"github.com/danielgtaylor/huma/v2"
)

type ProjectSettingsResponse struct {
	ProjectID          string `json:"project_id"`
	PlanTier           string `json:"plan_tier"`
	MaxKeyLifetimeDays int    `json:"max_key_lifetime_days"`
}
type UpdateProjectSettingsRequest struct {
	MaxKeyLifetimeDays *int `json:"max_key_lifetime_days,omitempty"`
}

type GetProjectSettingsInput struct {
	ProjectID string `path:"projectID"`
}
type GetProjectSettingsOutput struct{ Body ProjectSettingsResponse }

func (s *Server) handleGetProjectSettings(ctx context.Context, input *GetProjectSettingsInput) (*GetProjectSettingsOutput, error) {
	projectID := input.ProjectID
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	if err := s.validateProjectBelongsToCallerOrg(ctx, projectID); err != nil {
		return nil, huma.Error403Forbidden("access denied")
	}
	quota, err := s.store.GetProjectQuota(ctx, projectID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get project settings")
	}
	resp := ProjectSettingsResponse{ProjectID: projectID, PlanTier: string(domain.PlanFree)}
	if quota != nil {
		resp.MaxKeyLifetimeDays = quota.MaxKeyLifetimeDays
		if quota.PlanTier != "" {
			resp.PlanTier = quota.PlanTier
		}
	}
	return &GetProjectSettingsOutput{Body: resp}, nil
}

type UpdateProjectSettingsInput struct {
	ProjectID string `path:"projectID"`
	Body      UpdateProjectSettingsRequest
}
type UpdateProjectSettingsOutput struct{ Body ProjectSettingsResponse }

func (s *Server) handleUpdateProjectSettings(ctx context.Context, input *UpdateProjectSettingsInput) (*UpdateProjectSettingsOutput, error) {
	projectID := input.ProjectID
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	if err := s.validateProjectBelongsToCallerOrg(ctx, projectID); err != nil {
		return nil, huma.Error403Forbidden("access denied")
	}
	if input.Body.MaxKeyLifetimeDays != nil {
		days := *input.Body.MaxKeyLifetimeDays
		if days < 0 {
			return nil, huma.Error400BadRequest("max_key_lifetime_days must be >= 0")
		}
		if err := s.store.UpdateProjectMaxKeyLifetimeDays(ctx, projectID, days); err != nil {
			return nil, huma.Error500InternalServerError("failed to update project settings")
		}
		if s.quotaCache != nil {
			if quota, err := s.store.GetProjectQuota(ctx, projectID); err == nil && quota != nil {
				s.quotaCache.InvalidateWithVersionContext(ctx, projectID, projectQuotaCacheVersion(quota))
			} else {
				s.quotaCache.InvalidateContext(ctx, projectID)
			}
		}
	}
	out, err := s.handleGetProjectSettings(ctx, &GetProjectSettingsInput{ProjectID: projectID})
	if err != nil {
		return nil, err
	}

	s.emitAuditEvent(ctx, domain.AuditActionProjectSettingsUpdated, "project_settings", projectID, map[string]any{
		"changes": input.Body,
	})

	return &UpdateProjectSettingsOutput{Body: out.Body}, nil
}
