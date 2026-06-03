package api

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"strait/internal/billing"
	"strait/internal/domain"
)

func TestHandleGetPlansLaunchCatalog(t *testing.T) {
	srv := &Server{}
	out, err := srv.handleGetPlans(context.Background(), &GetPlansInput{})
	if err != nil {
		t.Fatalf("handleGetPlans returned error: %v", err)
	}
	if len(out.Body.Plans) != 6 {
		t.Fatalf("plans length = %d, want 6", len(out.Body.Plans))
	}

	byTier := make(map[string]PlanResponse, len(out.Body.Plans))
	for _, plan := range out.Body.Plans {
		byTier[plan.Tier] = plan
	}

	business := byTier["business"]
	if len(business.RoadmapFeatures) == 0 {
		t.Fatal("business roadmap features should be present for display only")
	}
	if want := billing.GetPlanCatalog(domain.PlanBusiness).RoadmapFeatures; !slices.Equal(business.RoadmapFeatures, want) {
		t.Fatalf("business roadmap features = %v, want generated catalog %v", business.RoadmapFeatures, want)
	}

	enterprise := byTier["enterprise"]
	if enterprise.MaxRunsPerMonth != -1 {
		t.Fatalf("enterprise max runs = %d, want unlimited", enterprise.MaxRunsPerMonth)
	}
	if want := billing.GetPlanCatalog(domain.PlanEnterprise).RoadmapFeatures; !slices.Equal(enterprise.RoadmapFeatures, want) {
		t.Fatalf("enterprise roadmap features = %v, want generated catalog %v", enterprise.RoadmapFeatures, want)
	}

	raw, err := json.Marshal(out.Body)
	if err != nil {
		t.Fatalf("marshal plans: %v", err)
	}
	var decoded struct {
		Plans []map[string]any `json:"plans"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal plans: %v", err)
	}
	for _, plan := range decoded.Plans {
		for _, inactive := range []string{
			"has_sso",
			"has_scim",
			"has_ip_allowlisting",
			"has_static_ips",
			"has_vpc_peering",
			"has_data_residency",
			"has_dedicated_compute",
			"has_reserved_capacity",
			"has_priority_queue",
			"has_session_management",
			"has_secret_rotation",
			"has_siem_export",
		} {
			if _, ok := plan[inactive]; ok {
				t.Fatalf("plan %q exposes inactive roadmap field %q in active entitlement response", plan["tier"], inactive)
			}
		}
	}
}
