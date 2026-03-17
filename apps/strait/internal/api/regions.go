package api

import (
	"net/http"

	"strait/internal/compute"
)

// RegionResponse is the API representation of a region.
type RegionResponse struct {
	Code      string `json:"code"`
	Label     string `json:"label"`
	City      string `json:"city"`
	Country   string `json:"country"`
	Continent string `json:"continent"`
}

// RegionsListResponse wraps the region list.
type RegionsListResponse struct {
	Regions []RegionResponse `json:"regions"`
}

func (s *Server) handleListRegions(w http.ResponseWriter, _ *http.Request) {
	allRegions := compute.AllRegions()

	regions := make([]RegionResponse, len(allRegions))
	for i, reg := range allRegions {
		regions[i] = RegionResponse{
			Code:      reg.Code,
			Label:     reg.Label,
			City:      reg.City,
			Country:   reg.Country,
			Continent: reg.Continent,
		}
	}

	respondJSON(w, http.StatusOK, RegionsListResponse{Regions: regions})
}
