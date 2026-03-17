package api

import (
	"fmt"
	"net/http"

	"strait/internal/compute"
	"strait/internal/domain"
)

// validateRegionForPlan validates a region against plan-based gating rules.
// Returns an HTTP error response if validation fails.
// Returns true if validation passed (or gating disabled), false if an error was sent.
func (s *Server) validateRegionForPlan(w http.ResponseWriter, r *http.Request, projectID, region string) bool {
	if region == "" {
		return true
	}
	if !compute.IsValidRegion(region) {
		respondError(w, r, http.StatusBadRequest, "invalid region: "+region)
		return false
	}
	if !s.config.EnforceRegionGating {
		return true
	}
	tier := s.getProjectPlanTier(r, projectID)
	if !domain.IsRegionAllowed(tier, region) {
		respondError(w, r, http.StatusForbidden, "region "+region+" is not available on your plan")
		return false
	}
	return true
}

// validatePreferredRegionsForPlan validates a preferred regions list against plan-based gating.
// Returns true if validation passed, false if an error was sent.
func (s *Server) validatePreferredRegionsForPlan(w http.ResponseWriter, r *http.Request, projectID string, regions []string) bool {
	if len(regions) == 0 {
		return true
	}

	// Validate all region codes in a single pass.
	for _, pr := range regions {
		if !compute.IsValidRegion(pr) {
			respondError(w, r, http.StatusBadRequest, "invalid preferred region: "+pr)
			return false
		}
	}

	if !s.config.EnforceRegionGating {
		return true
	}

	tier := s.getProjectPlanTier(r, projectID)
	cfg := domain.GetPlanConfig(tier)

	if !cfg.MultiRegion {
		respondError(w, r, http.StatusForbidden, "multi-region is not available on your plan")
		return false
	}
	if len(regions) > cfg.MaxRegions {
		respondError(w, r, http.StatusBadRequest, fmt.Sprintf("too many preferred regions (max %d for your plan)", cfg.MaxRegions))
		return false
	}

	// Validate each region is allowed on the plan.
	for _, pr := range regions {
		if !domain.IsRegionAllowed(tier, pr) {
			respondError(w, r, http.StatusForbidden, "region "+pr+" is not available on your plan")
			return false
		}
	}

	return true
}

// getProjectPlanTier fetches the plan tier for a project, defaulting to free.
func (s *Server) getProjectPlanTier(r *http.Request, projectID string) domain.PlanTier {
	quota, err := s.store.GetProjectQuota(r.Context(), projectID)
	if err != nil || quota == nil {
		return domain.PlanFree
	}
	if quota.PlanTier == "" {
		return domain.PlanFree
	}
	return domain.PlanTier(quota.PlanTier)
}
