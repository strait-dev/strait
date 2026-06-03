package api

import (
	"context"
	"strings"
	"testing"

	"strait/internal/billing"
)

func TestPlanGate_RoadmapFeatureDoesNotReturnUpgradeCTA(t *testing.T) {
	t.Parallel()

	enforcer := &tunableLimitsEnforcer{limits: enterpriseLimits()}
	srv := newServerWithEnforcer(t, &APIStoreMock{}, &mockQueue{}, enforcer)

	err := srv.checkFeatureAllowed(context.Background(), "proj-1", billing.FeatureIPAllowlisting, "IP allowlisting")
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
}
