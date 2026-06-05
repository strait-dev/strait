package billing

import (
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
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
		assert.True(t, r.
			AllowsFeature(domain.PlanEnterprise,

				f))

	}
}

func TestRegistry_FreeBlocksEnterpriseFeatures(t *testing.T) {
	t.Parallel()
	r := NewStaticRegistry()
	for _, f := range roadmapEnterpriseFeatures {
		assert.False(t, r.
			AllowsFeature(domain.PlanFree,

				f))

	}
}

func TestRegistry_StarterBlocksEnterpriseFeatures(t *testing.T) {
	t.Parallel()
	r := NewStaticRegistry()
	for _, f := range roadmapEnterpriseFeatures {
		assert.False(t, r.
			AllowsFeature(domain.PlanStarter,

				f))

	}
}

func TestRegistry_ProBlocksEnterpriseFeatures(t *testing.T) {
	t.Parallel()
	r := NewStaticRegistry()
	for _, f := range roadmapEnterpriseFeatures {
		assert.False(t, r.
			AllowsFeature(domain.PlanPro,

				f))

	}
}

func TestRegistry_ScaleBlocksEnterpriseFeatures(t *testing.T) {
	t.Parallel()
	r := NewStaticRegistry()
	for _, f := range roadmapEnterpriseFeatures {
		assert.False(t, r.
			AllowsFeature(domain.PlanScale,

				f))

	}
}

func TestRegistry_EnterpriseBlocksRoadmapFeatures(t *testing.T) {
	t.Parallel()
	r := NewStaticRegistry()
	for _, f := range roadmapEnterpriseFeatures {
		assert.False(t, r.
			AllowsFeature(domain.PlanEnterprise,

				f))
		assert.True(t, IsRoadmapFeature(f))

	}
}

func TestRegistry_EveryLaunchActiveFeatureHasCase(t *testing.T) {
	t.Parallel()
	r := NewStaticRegistry()
	for _, f := range launchActiveFeatures {
		assert.True(t, r.
			AllowsFeature(domain.PlanEnterprise,

				f))

	}
}
