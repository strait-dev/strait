package billing

import (
	"testing"

	"strait/internal/domain"
)

// roadmapEnterpriseFeatures lists enterprise/security features that are known
// to the registry but are not launch-active entitlements.
var roadmapEnterpriseFeatures = []Feature{
	FeatureSSO,
	FeatureDedicatedCompute,
	FeatureStaticIPs,
	FeatureVPCPeering,
	FeatureSCIM,
	FeatureDataResidency,
	FeatureCustomRBAC,
	FeaturePriorityQueue,
	FeatureIPAllowlisting,
	FeatureSessionManagement,
	FeatureSecretRotation,
	FeatureSIEMExport,
}

// launchActiveFeatures lists every feature the launch catalog actively gates.
var launchActiveFeatures = []Feature{
	FeatureHTTPMode,
	FeatureApprovalGates,
	FeatureSubWorkflows,
	FeatureJobChaining,
	FeatureCompensatingTxns,
	FeatureCanaryDeployments,
	FeatureAuditLogs,
	FeatureSLA,
	FeatureRBAC,
	FeatureAllCronOverlap,
}

func TestRegistry_EnterpriseAllowsLaunchActiveFeatures(t *testing.T) {
	t.Parallel()
	r := NewStaticRegistry()
	for _, f := range launchActiveFeatures {
		if !r.AllowsFeature(domain.PlanEnterprise, f) {
			t.Errorf("Enterprise should allow feature %q", f)
		}
	}
}

func TestRegistry_FreeBlocksEnterpriseFeatures(t *testing.T) {
	t.Parallel()
	r := NewStaticRegistry()
	for _, f := range roadmapEnterpriseFeatures {
		if r.AllowsFeature(domain.PlanFree, f) {
			t.Errorf("Free should block enterprise feature %q", f)
		}
	}
}

func TestRegistry_StarterBlocksEnterpriseFeatures(t *testing.T) {
	t.Parallel()
	r := NewStaticRegistry()
	for _, f := range roadmapEnterpriseFeatures {
		if r.AllowsFeature(domain.PlanStarter, f) {
			t.Errorf("Starter should block enterprise feature %q", f)
		}
	}
}

func TestRegistry_ProBlocksEnterpriseFeatures(t *testing.T) {
	t.Parallel()
	r := NewStaticRegistry()
	for _, f := range roadmapEnterpriseFeatures {
		if r.AllowsFeature(domain.PlanPro, f) {
			t.Errorf("Pro should block enterprise feature %q", f)
		}
	}
}

func TestRegistry_ScaleBlocksEnterpriseFeatures(t *testing.T) {
	t.Parallel()
	r := NewStaticRegistry()
	for _, f := range roadmapEnterpriseFeatures {
		if r.AllowsFeature(domain.PlanScale, f) {
			t.Errorf("Scale should block enterprise feature %q", f)
		}
	}
}

func TestRegistry_EnterpriseBlocksRoadmapFeatures(t *testing.T) {
	t.Parallel()
	r := NewStaticRegistry()
	for _, f := range roadmapEnterpriseFeatures {
		if r.AllowsFeature(domain.PlanEnterprise, f) {
			t.Errorf("Enterprise should block launch-roadmap feature %q", f)
		}
		if !IsRoadmapFeature(f) {
			t.Errorf("feature %q should be marked roadmap", f)
		}
	}
}

func TestRegistry_EveryLaunchActiveFeatureHasCase(t *testing.T) {
	t.Parallel()
	r := NewStaticRegistry()
	for _, f := range launchActiveFeatures {
		if !r.AllowsFeature(domain.PlanEnterprise, f) {
			t.Errorf("feature %q returns false for enterprise -- missing case in AllowsFeature?", f)
		}
	}
}
