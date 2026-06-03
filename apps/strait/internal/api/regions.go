package api

import (
	"context"

	"strait/internal/billing"
	"strait/internal/domain"
)

// RegionResponse is the public API shape for execution-region metadata.
type RegionResponse struct {
	Code         string          `json:"code"`
	Label        string          `json:"label"`
	City         string          `json:"city"`
	Country      string          `json:"country"`
	Continent    string          `json:"continent"`
	Availability map[string]bool `json:"availability,omitempty"`
}

// GetRegionsOutput is the typed output for the list regions endpoint.
type GetRegionsOutput struct {
	Body struct {
		Regions []RegionResponse `json:"regions"`
	}
}

func toRegionResponse(region domain.RegionInfo) RegionResponse {
	availability := make(map[string]bool, len(domain.AllPlanTiers()))
	for _, tier := range domain.AllPlanTiers() {
		availability[string(tier)] = isRegionAllowedByCatalog(tier, region.Code)
	}

	return RegionResponse{
		Code:         region.Code,
		Label:        region.Label,
		City:         region.City,
		Country:      region.Country,
		Continent:    region.Continent,
		Availability: availability,
	}
}

// handleGetRegions returns all supported execution regions with display metadata.
func (s *Server) handleGetRegions(_ context.Context, _ *struct{}) (*GetRegionsOutput, error) {
	regions := domain.AllRegions()
	out := &GetRegionsOutput{}
	out.Body.Regions = make([]RegionResponse, 0, len(regions))
	for _, region := range regions {
		out.Body.Regions = append(out.Body.Regions, toRegionResponse(region))
	}
	return out, nil
}

func isRegionAllowedByCatalog(tier domain.PlanTier, region string) bool {
	limits := billing.GetPlanLimits(tier)
	return len(limits.AllowedRegions) == 0 || containsRegion(limits.AllowedRegions, region)
}
