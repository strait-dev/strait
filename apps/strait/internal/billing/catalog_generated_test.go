package billing

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"strait/internal/domain"
)

func TestGeneratedCatalogHashMatchesSource(t *testing.T) {
	sourcePath := filepath.Join("..", "..", "..", "..", "packages", "billing", "catalog", "strait-pricing.json")
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read pricing catalog source: %v", err)
	}
	sum := sha256.Sum256(source)
	if got := hex.EncodeToString(sum[:]); got != PricingCatalogHash {
		t.Fatalf("pricing catalog hash = %s, want %s; run bun run --cwd packages/billing generate", PricingCatalogHash, got)
	}
}

func TestLaunchCatalogKeepsRoadmapSecurityFeaturesInactive(t *testing.T) {
	for _, tier := range []struct {
		name   string
		limits OrgPlanLimits
	}{
		{"business", GetPlanLimits(domain.PlanBusiness)},
		{"enterprise", GetPlanLimits(domain.PlanEnterprise)},
	} {
		if tier.limits.HasSSO || tier.limits.HasSCIM || tier.limits.HasIPAllowlisting ||
			tier.limits.HasStaticIPs || tier.limits.HasVPCPeering || tier.limits.HasDataResidency {
			t.Fatalf("%s exposes a roadmap security feature as an active entitlement: %+v", tier.name, tier.limits)
		}
	}
}
