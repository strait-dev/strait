package billing

import (
	"testing"

	"strait/internal/domain"
)

// allEnterpriseFeatures lists every Feature constant that should be enterprise-only.
var allEnterpriseFeatures = []Feature{
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

// allFeatures lists every defined Feature constant in the registry.
var allFeatures = []Feature{
	FeatureHTTPMode,
	FeatureApprovalGates,
	FeatureSubWorkflows,
	FeatureJobChaining,
	FeatureCompensatingTxns,
	FeatureCanaryDeployments,
	FeatureAuditLogs,
	FeatureSSO,
	FeatureSLA,
	FeatureRBAC,
	FeatureAllCronOverlap,
	FeatureAIAssistantBYOK,
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

func TestRegistry_EnterpriseAllowsAllFeatures(t *testing.T) {
	t.Parallel()
	r := NewStaticRegistry()
	for _, f := range allFeatures {
		if !r.AllowsFeature(domain.PlanEnterprise, f) {
			t.Errorf("Enterprise should allow feature %q", f)
		}
	}
}

func TestRegistry_FreeBlocksEnterpriseFeatures(t *testing.T) {
	t.Parallel()
	r := NewStaticRegistry()
	for _, f := range allEnterpriseFeatures {
		if r.AllowsFeature(domain.PlanFree, f) {
			t.Errorf("Free should block enterprise feature %q", f)
		}
	}
}

func TestRegistry_StarterBlocksEnterpriseFeatures(t *testing.T) {
	t.Parallel()
	r := NewStaticRegistry()
	for _, f := range allEnterpriseFeatures {
		if r.AllowsFeature(domain.PlanStarter, f) {
			t.Errorf("Starter should block enterprise feature %q", f)
		}
	}
}

func TestRegistry_ProBlocksEnterpriseFeatures(t *testing.T) {
	t.Parallel()
	r := NewStaticRegistry()
	for _, f := range allEnterpriseFeatures {
		if r.AllowsFeature(domain.PlanPro, f) {
			t.Errorf("Pro should block enterprise feature %q", f)
		}
	}
}

func TestRegistry_ScaleBlocksEnterpriseFeatures(t *testing.T) {
	t.Parallel()
	r := NewStaticRegistry()
	for _, f := range allEnterpriseFeatures {
		if r.AllowsFeature(domain.PlanScale, f) {
			t.Errorf("Scale should block enterprise feature %q", f)
		}
	}
}

func TestRegistry_EveryFeatureHasCase(t *testing.T) {
	t.Parallel()
	r := NewStaticRegistry()
	// Enterprise has every feature enabled, so AllowsFeature should return
	// true for all known features. If a new Feature constant is added without
	// an AllowsFeature case, it will hit the default: false branch.
	for _, f := range allFeatures {
		if !r.AllowsFeature(domain.PlanEnterprise, f) {
			t.Errorf("feature %q returns false for enterprise -- missing case in AllowsFeature?", f)
		}
	}
}
