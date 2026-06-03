package api

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"strait/internal/billing"
	"strait/internal/domain"
)

// checkRegionForPlan validates a region against plan-based gating rules, returning an error.
func (s *Server) checkRegionForPlan(ctx context.Context, projectID, region string) error {
	if region == "" {
		return nil
	}
	if !domain.IsValidRegion(region) {
		return huma.Error400BadRequest("invalid region: " + region)
	}
	if !s.config.EnforceRegionGating {
		return nil
	}

	limits := s.getRegionPlanLimits(ctx, projectID)
	if limits == nil || len(limits.AllowedRegions) == 0 {
		return nil
	}
	if !containsRegion(limits.AllowedRegions, region) {
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
		if !domain.IsValidRegion(pr) {
			return huma.Error400BadRequest("invalid preferred region: " + pr)
		}
	}

	if !s.config.EnforceRegionGating {
		return nil
	}

	limits := s.getRegionPlanLimits(ctx, projectID)
	if limits == nil {
		return nil
	}

	if len(limits.AllowedRegions) == 0 {
		return nil
	}

	// Validate each region is allowed on the plan.
	for _, pr := range regions {
		if !containsRegion(limits.AllowedRegions, pr) {
			return huma.Error403Forbidden("region " + pr + " is not available on your plan")
		}
	}

	return nil
}

func (s *Server) getRegionPlanLimits(ctx context.Context, projectID string) *billing.OrgPlanLimits {
	if s.billingEnforcer != nil {
		orgID, err := s.billingEnforcer.GetProjectOrgID(ctx, projectID)
		if err == nil && orgID != "" {
			limits, limErr := s.billingEnforcer.GetOrgPlanLimits(ctx, orgID)
			if limErr == nil {
				return &limits
			}
		}
	}

	quota, err := s.store.GetProjectQuota(ctx, projectID)
	if err != nil || quota == nil || quota.PlanTier == "" {
		limits := billing.GetPlanLimits(domain.PlanFree)
		return &limits
	}
	limits := billing.GetPlanLimits(domain.PlanTier(quota.PlanTier))
	return &limits
}

func containsRegion(regions []string, region string) bool {
	for _, candidate := range regions {
		if candidate == region {
			return true
		}
	}
	return false
}
