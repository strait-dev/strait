package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"strait/internal/compute"
	"strait/internal/domain"
)

// validateRegionForPlan validates a region against plan-based gating rules.
// Returns an HTTP error response if validation fails.
// Returns true if validation passed (or gating disabled), false if an error was sent.
func (s *Server) validateRegionForPlan(w http.ResponseWriter, r *http.Request, projectID, region string) bool {
	if err := s.checkRegionForPlan(r.Context(), projectID, region); err != nil {
		writeTypedError(w, r, err)
		return false
	}
	return true
}

// checkRegionForPlan validates a region against plan-based gating rules, returning an error.
func (s *Server) checkRegionForPlan(ctx context.Context, projectID, region string) error {
	if region == "" {
		return nil
	}
	if !compute.IsValidRegion(region) {
		return huma.Error400BadRequest("invalid region: " + region)
	}
	if !s.config.EnforceRegionGating {
		return nil
	}
	tier := s.getProjectPlanTierCtx(ctx, projectID)
	if !domain.IsRegionAllowed(tier, region) {
		return huma.Error403Forbidden("region " + region + " is not available on your plan")
	}
	return nil
}

// checkPreferredRegionsForPlan validates a preferred regions list against plan-based gating, returning an error.
func (s *Server) checkPreferredRegionsForPlan(ctx context.Context, projectID string, regions []string) error {
	if len(regions) == 0 {
		return nil
	}

	// Validate all region codes in a single pass.
	for _, pr := range regions {
		if !compute.IsValidRegion(pr) {
			return huma.Error400BadRequest("invalid preferred region: " + pr)
		}
	}

	if !s.config.EnforceRegionGating {
		return nil
	}

	tier := s.getProjectPlanTierCtx(ctx, projectID)
	cfg := domain.GetPlanConfig(tier)

	if !cfg.MultiRegion {
		return huma.Error403Forbidden("multi-region is not available on your plan")
	}
	if len(regions) > cfg.MaxRegions {
		return huma.Error400BadRequest(fmt.Sprintf("too many preferred regions (max %d for your plan)", cfg.MaxRegions))
	}

	// Validate each region is allowed on the plan.
	for _, pr := range regions {
		if !domain.IsRegionAllowed(tier, pr) {
			return huma.Error403Forbidden("region " + pr + " is not available on your plan")
		}
	}

	return nil
}

// getProjectPlanTierCtx fetches the plan tier for a project, defaulting to free.
func (s *Server) getProjectPlanTierCtx(ctx context.Context, projectID string) domain.PlanTier {
	quota, err := s.store.GetProjectQuota(ctx, projectID)
	if err != nil || quota == nil {
		return domain.PlanFree
	}
	if quota.PlanTier == "" {
		return domain.PlanFree
	}
	return domain.PlanTier(quota.PlanTier)
}
