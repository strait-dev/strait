package api

import (
	"context"
	"testing"
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
	if business.HasSSO || business.HasSCIM || business.HasIPAllowlisting {
		t.Fatalf("business exposes roadmap security features as active entitlements: %+v", business)
	}
	if len(business.RoadmapFeatures) == 0 {
		t.Fatal("business roadmap features should be present for display only")
	}

	enterprise := byTier["enterprise"]
	if enterprise.HasSSO || enterprise.HasStaticIPs || enterprise.HasVPCPeering || enterprise.HasDataResidency {
		t.Fatalf("enterprise exposes roadmap deployment features as active entitlements: %+v", enterprise)
	}
	if enterprise.MaxRunsPerMonth != -1 {
		t.Fatalf("enterprise max runs = %d, want unlimited", enterprise.MaxRunsPerMonth)
	}
}
