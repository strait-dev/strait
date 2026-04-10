package api

import (
	"context"
	"fmt"

	"github.com/danielgtaylor/huma/v2"

	"strait/internal/compute"
	"strait/internal/domain"
)

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

// getProjectPlanTierCtx fetches the Jobs plan tier for a project, defaulting to free.
//
// The source of truth is organization_subscriptions.plan_tier, resolved via the
// billing enforcer. The project_quotas.plan_tier column is write-dead (nothing
// writes it, every read returns empty) and must not be used. This function used
// to read from project_quotas which meant region gating was permanently stuck
// in free-tier mode regardless of actual subscription state.
func (s *Server) getProjectPlanTierCtx(ctx context.Context, projectID string) domain.PlanTier {
	if s.billingEnforcer == nil {
		return domain.PlanFree
	}
	tier, err := s.billingEnforcer.GetJobsPlanForProject(ctx, projectID)
	if err != nil || tier == "" {
		return domain.PlanFree
	}
	return tier
}
