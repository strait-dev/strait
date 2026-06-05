package api

import (
	"context"
	"errors"
	"strings"
	"testing"

	"strait/internal/billing"

	"github.com/stretchr/testify/require"
)

func TestPlanGate_RoadmapFeaturesDoNotReturnUpgradeCTA(t *testing.T) {
	t.Parallel()

	enforcer := &tunableLimitsEnforcer{limits: enterpriseLimits()}
	srv := newServerWithEnforcer(t, &APIStoreMock{}, &mockQueue{}, enforcer)

	tests := []struct {
		name        string
		feature     billing.Feature
		featureName string
	}{
		{name: "sso", feature: billing.FeatureSSO, featureName: "SSO/SAML"},
		{name: "scim", feature: billing.FeatureSCIM, featureName: "SCIM"},
		{name: "ip_allowlisting", feature: billing.FeatureIPAllowlisting, featureName: "IP allowlisting"},
		{name: "static_ips", feature: billing.FeatureStaticIPs, featureName: "static IPs"},
		{name: "vpc_peering", feature: billing.FeatureVPCPeering, featureName: "VPC peering"},
		{name: "data_residency", feature: billing.FeatureDataResidency, featureName: "data residency"},
		{name: "custom_rbac", feature: billing.FeatureCustomRBAC, featureName: "custom RBAC"},
		{name: "dedicated_compute", feature: billing.FeatureDedicatedCompute, featureName: "dedicated compute"},
		{name: "priority_queue", feature: billing.FeaturePriorityQueue, featureName: "priority queue"},
		{name: "session_management", feature: billing.FeatureSessionManagement, featureName: "session management"},
		{name: "secret_rotation", feature: billing.FeatureSecretRotation, featureName: "secret rotation"},
		{name: "siem_export", feature: billing.FeatureSIEMExport, featureName: "SIEM export"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := srv.checkFeatureAllowed(context.Background(), "proj-1", tt.feature, tt.featureName)
			require.Error(t, err)

			msg := err.Error()
			require.Contains(
				t, msg, "roadmap/contact-sales only at launch")
			require.NotContains(t, strings.ToLower(msg), "upgrade")
		})
	}
}

func TestPlanGate_FeatureOrgLookupErrorFailsClosed(t *testing.T) {
	t.Parallel()

	enforcer := &tunableLimitsEnforcer{
		limits: freeLimits(),
		orgErr: errors.New("org lookup unavailable"),
	}
	srv := newServerWithEnforcer(t, &APIStoreMock{}, &mockQueue{}, enforcer)

	err := srv.checkFeatureAllowed(context.Background(), "proj-1", billing.FeatureAuditLogs, "Audit logs")
	require.Error(t, err)
	require.Contains(
		t, err.
			Error(), "billing enforcement unavailable")
}

func TestPlanGate_FeaturePlanLookupErrorFailsClosed(t *testing.T) {
	t.Parallel()

	enforcer := &tunableLimitsEnforcer{
		limitsErr: errors.New("plan lookup unavailable"),
	}
	srv := newServerWithEnforcer(t, &APIStoreMock{}, &mockQueue{}, enforcer)

	err := srv.checkFeatureAllowed(context.Background(), "proj-1", billing.FeatureAuditLogs, "Audit logs")
	require.Error(t, err)
	require.Contains(
		t, err.
			Error(), "billing enforcement unavailable")
}
