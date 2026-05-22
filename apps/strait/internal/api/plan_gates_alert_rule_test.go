package api

import (
	"context"
	"strings"
	"testing"
)

// The alert-rule HTTP handler does not exist yet (rules currently live in the
// telemetry package, not the API). The plan gate is wired anyway so the gate
// is in place when the handler lands; these tests exercise the function
// directly via a server with no real routes.

// TestCheckAlertRuleLimit_FreeTier_RejectsZeroCap proves Free (cap=0) returns
// the "not available" message regardless of currentCount.
func TestCheckAlertRuleLimit_FreeTier_RejectsZeroCap(t *testing.T) {
	t.Parallel()

	enforcer := &tunableLimitsEnforcer{limits: freeLimits()}
	srv := newServerWithEnforcer(t, &APIStoreMock{}, &mockQueue{}, enforcer)

	err := srv.checkAlertRuleLimit(context.Background(), "proj-1", 0)
	if err == nil {
		t.Fatal("expected free-tier rejection, got nil")
	}
	if !strings.Contains(err.Error(), "not available") {
		t.Errorf("error must mention feature is not available, got: %v", err)
	}
}

// TestCheckAlertRuleLimit_ProTier_BlocksAtCap verifies the cap=50 boundary on
// Pro tier: 50 existing rules → reject 51st.
func TestCheckAlertRuleLimit_ProTier_BlocksAtCap(t *testing.T) {
	t.Parallel()

	enforcer := &tunableLimitsEnforcer{limits: proLimits()}
	srv := newServerWithEnforcer(t, &APIStoreMock{}, &mockQueue{}, enforcer)

	err := srv.checkAlertRuleLimit(context.Background(), "proj-1", 50)
	if err == nil {
		t.Fatal("expected at-cap rejection, got nil")
	}
	if !strings.Contains(err.Error(), "50 alert rules") || !strings.Contains(err.Error(), "have 50") {
		t.Errorf("error must report cap and current count, got: %v", err)
	}
}

// TestCheckAlertRuleLimit_ProTier_BelowCap_Allows verifies that 49 < 50 is
// allowed (the cap is exclusive on the new entry; >= triggers the gate).
func TestCheckAlertRuleLimit_ProTier_BelowCap_Allows(t *testing.T) {
	t.Parallel()

	enforcer := &tunableLimitsEnforcer{limits: proLimits()}
	srv := newServerWithEnforcer(t, &APIStoreMock{}, &mockQueue{}, enforcer)

	if err := srv.checkAlertRuleLimit(context.Background(), "proj-1", 49); err != nil {
		t.Fatalf("below-cap must allow; got %v", err)
	}
}

// TestCheckAlertRuleLimit_EnterpriseUnlimited_Allows verifies that the
// unlimited tier never rejects regardless of currentCount.
func TestCheckAlertRuleLimit_EnterpriseUnlimited_Allows(t *testing.T) {
	t.Parallel()

	enforcer := &tunableLimitsEnforcer{limits: enterpriseLimits()}
	srv := newServerWithEnforcer(t, &APIStoreMock{}, &mockQueue{}, enforcer)

	if err := srv.checkAlertRuleLimit(context.Background(), "proj-1", 9999); err != nil {
		t.Fatalf("unlimited tier must never reject; got %v", err)
	}
}

// TestCheckAlertRuleLimit_NilEnforcer_FailsOpen confirms that the
// community-edition path (no enforcer) does not reject.
func TestCheckAlertRuleLimit_NilEnforcer_FailsOpen(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	if err := srv.checkAlertRuleLimit(context.Background(), "proj-1", 9999); err != nil {
		t.Fatalf("nil enforcer must fail open; got %v", err)
	}
}
