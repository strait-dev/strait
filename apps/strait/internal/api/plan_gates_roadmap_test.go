package api

import (
	"context"
	"errors"
	"strings"
	"testing"

	"strait/internal/billing"
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
			if err == nil {
				t.Fatal("expected roadmap feature rejection")
			}
			msg := err.Error()
			if !strings.Contains(msg, "roadmap/contact-sales only at launch") {
				t.Fatalf("roadmap rejection should explain launch status, got: %s", msg)
			}
			if strings.Contains(strings.ToLower(msg), "upgrade") {
				t.Fatalf("roadmap rejection must not return an upgrade CTA, got: %s", msg)
			}
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
	if err == nil {
		t.Fatal("expected feature gate to fail closed when org lookup fails")
	}
	if !strings.Contains(err.Error(), "billing enforcement unavailable") {
		t.Fatalf("error = %v, want billing enforcement unavailable", err)
	}
}

func TestPlanGate_FeaturePlanLookupErrorFailsClosed(t *testing.T) {
	t.Parallel()

	enforcer := &tunableLimitsEnforcer{
		limitsErr: errors.New("plan lookup unavailable"),
	}
	srv := newServerWithEnforcer(t, &APIStoreMock{}, &mockQueue{}, enforcer)

	err := srv.checkFeatureAllowed(context.Background(), "proj-1", billing.FeatureAuditLogs, "Audit logs")
	if err == nil {
		t.Fatal("expected feature gate to fail closed when plan lookup fails")
	}
	if !strings.Contains(err.Error(), "billing enforcement unavailable") {
		t.Fatalf("error = %v, want billing enforcement unavailable", err)
	}
}
