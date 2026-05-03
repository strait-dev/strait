package api

import (
	"context"

	"strait/internal/domain"
)

type RegionResponse struct {
	Code         string          `json:"code"`
	Label        string          `json:"label"`
	City         string          `json:"city"`
	Country      string          `json:"country"`
	Continent    string          `json:"continent"`
	Availability map[string]bool `json:"availability,omitempty"`
}

type RegionsListResponse struct {
	Regions []RegionResponse `json:"regions"`
}

type ListRegionsInput struct{}
type ListRegionsOutput struct{ Body RegionsListResponse }

func (s *Server) handleListRegions(_ context.Context, _ *ListRegionsInput) (*ListRegionsOutput, error) {
	allRegions := domain.AllRegions()
	regions := make([]RegionResponse, len(allRegions))
	for i, reg := range allRegions {
		regions[i] = RegionResponse{
			Code: reg.Code, Label: reg.Label, City: reg.City, Country: reg.Country, Continent: reg.Continent,
			Availability: map[string]bool{
				string(domain.PlanFree):       domain.IsRegionAllowed(domain.PlanFree, reg.Code),
				string(domain.PlanStarter):    domain.IsRegionAllowed(domain.PlanStarter, reg.Code),
				string(domain.PlanPro):        domain.IsRegionAllowed(domain.PlanPro, reg.Code),
				string(domain.PlanEnterprise): domain.IsRegionAllowed(domain.PlanEnterprise, reg.Code),
			},
		}
	}
	return &ListRegionsOutput{Body: RegionsListResponse{Regions: regions}}, nil
}
