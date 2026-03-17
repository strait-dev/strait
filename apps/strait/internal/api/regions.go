package api

import (
	"net/http"

	"strait/internal/compute"
	"strait/internal/domain"
)

// RegionResponse is the API representation of a region.
type RegionResponse struct {
	Code         string            `json:"code"`
	Label        string            `json:"label"`
	City         string            `json:"city"`
	Country      string            `json:"country"`
	Continent    string            `json:"continent"`
	Availability map[string]bool   `json:"availability,omitempty"`
}

// RegionsListResponse wraps the region list.
type RegionsListResponse struct {
	Regions []RegionResponse `json:"regions"`
}

func (s *Server) handleListRegions(w http.ResponseWriter, _ *http.Request) {
	allRegions := compute.AllRegions()

	regions := make([]RegionResponse, len(allRegions))
	for i, reg := range allRegions {
		resp := RegionResponse{
			Code:      reg.Code,
			Label:     reg.Label,
			City:      reg.City,
			Country:   reg.Country,
			Continent: reg.Continent,
		}

		// Annotate per-plan availability.
		resp.Availability = map[string]bool{
			string(domain.PlanFree):         domain.IsRegionAllowed(domain.PlanFree, reg.Code),
			string(domain.PlanStarter):      domain.IsRegionAllowed(domain.PlanStarter, reg.Code),
			string(domain.PlanProfessional): domain.IsRegionAllowed(domain.PlanProfessional, reg.Code),
			string(domain.PlanEnterprise):   domain.IsRegionAllowed(domain.PlanEnterprise, reg.Code),
		}

		regions[i] = resp
	}

	respondJSON(w, http.StatusOK, RegionsListResponse{Regions: regions})
}
