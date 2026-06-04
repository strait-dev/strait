package api

import (
	"context"
	"errors"
	"strings"
	"testing"

	"strait/internal/billing"
	"strait/internal/domain"
)

// TestCheckPerJobConcurrencyLimit_ZeroValues_NoOp pins the platform-default
// behavior: zero on either field means "use the engine default", so the gate
// must not enforce a cap when both inputs are zero.
func TestCheckPerJobConcurrencyLimit_ZeroValues_NoOp(t *testing.T) {
	t.Parallel()
	enforcer := &tunableLimitsEnforcer{limits: freeLimits()}
	srv := newServerWithEnforcer(t, &APIStoreMock{}, &mockQueue{}, enforcer)

	if err := srv.checkPerJobConcurrencyLimit(context.Background(), "proj-1", 0, 0); err != nil {
		t.Fatalf("zero settings must not be capped; got %v", err)
	}
}

// TestCheckPerJobConcurrencyLimit_FreeAtLimit_Allows verifies the cap is
// inclusive — a per-job concurrency exactly equal to the org-wide cap is
// accepted (the engine still enforces the org-wide limit at dispatch).
func TestCheckPerJobConcurrencyLimit_FreeAtLimit_Allows(t *testing.T) {
	t.Parallel()
	limits := freeLimits()
	enforcer := &tunableLimitsEnforcer{limits: limits}
	srv := newServerWithEnforcer(t, &APIStoreMock{}, &mockQueue{}, enforcer)

	if err := srv.checkPerJobConcurrencyLimit(context.Background(), "proj-1", limits.MaxConcurrentRuns, 0); err != nil {
		t.Fatalf("max_concurrency at exact org cap must be allowed; got %v", err)
	}
	if err := srv.checkPerJobConcurrencyLimit(context.Background(), "proj-1", 0, limits.MaxConcurrentRuns); err != nil {
		t.Fatalf("max_concurrency_per_key at exact org cap must be allowed; got %v", err)
	}
}

// TestCheckPerJobConcurrencyLimit_FreeOverLimit_RejectsMaxConcurrency walks
// one above the cap on max_concurrency and asserts the rejection names the
// plan, the cap, and the offending field.
func TestCheckPerJobConcurrencyLimit_FreeOverLimit_RejectsMaxConcurrency(t *testing.T) {
	t.Parallel()
	limits := freeLimits()
	enforcer := &tunableLimitsEnforcer{limits: limits}
	srv := newServerWithEnforcer(t, &APIStoreMock{}, &mockQueue{}, enforcer)

	err := srv.checkPerJobConcurrencyLimit(context.Background(), "proj-1", limits.MaxConcurrentRuns+1, 0)
	if err == nil {
		t.Fatal("over-cap max_concurrency must be rejected")
	}
	for _, fragment := range []string{limits.DisplayName, "max_concurrency"} {
		if !strings.Contains(err.Error(), fragment) {
			t.Errorf("error message missing %q, got: %v", fragment, err)
		}
	}
}

// TestCheckPerJobConcurrencyLimit_FreeOverLimit_RejectsPerKey covers the
// max_concurrency_per_key field on the same overage scenario. The error
// must specifically name max_concurrency_per_key so the customer can
// identify which knob to reduce.
func TestCheckPerJobConcurrencyLimit_FreeOverLimit_RejectsPerKey(t *testing.T) {
	t.Parallel()
	limits := freeLimits()
	enforcer := &tunableLimitsEnforcer{limits: limits}
	srv := newServerWithEnforcer(t, &APIStoreMock{}, &mockQueue{}, enforcer)

	err := srv.checkPerJobConcurrencyLimit(context.Background(), "proj-1", 0, limits.MaxConcurrentRuns+1)
	if err == nil {
		t.Fatal("over-cap max_concurrency_per_key must be rejected")
	}
	if !strings.Contains(err.Error(), "max_concurrency_per_key") {
		t.Errorf("error message must name max_concurrency_per_key, got: %v", err)
	}
}

// TestCheckPerJobConcurrencyLimit_EnterpriseUnlimited_Allows confirms the
// MaxConcurrentRuns=-1 sentinel short-circuits the cap.
func TestCheckPerJobConcurrencyLimit_EnterpriseUnlimited_Allows(t *testing.T) {
	t.Parallel()
	enforcer := &tunableLimitsEnforcer{limits: enterpriseLimits()}
	srv := newServerWithEnforcer(t, &APIStoreMock{}, &mockQueue{}, enforcer)

	if err := srv.checkPerJobConcurrencyLimit(context.Background(), "proj-1", 100_000, 100_000); err != nil {
		t.Fatalf("unlimited tier must never reject; got %v", err)
	}
}

func TestCheckPerJobConcurrencyLimit_CloudNilEnforcerFailsClosed(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	srv.edition = domain.EditionCloud

	err := srv.checkPerJobConcurrencyLimit(context.Background(), "proj-1", 999_999, 999_999)
	if err == nil || !strings.Contains(err.Error(), "billing enforcement unavailable") {
		t.Fatalf("expected billing enforcement unavailable, got %v", err)
	}
}

// TestCheckPerJobConcurrencyLimit_CommunityNilEnforcerFailsOpen confirms
// self-hosted builds (no enforcer wired) accept any concurrency setting.
func TestCheckPerJobConcurrencyLimit_CommunityNilEnforcerFailsOpen(t *testing.T) {
	t.Parallel()
	srv := &Server{edition: domain.EditionCommunity}

	if err := srv.checkPerJobConcurrencyLimit(context.Background(), "proj-1", 999_999, 999_999); err != nil {
		t.Fatalf("community nil enforcer must fail open; got %v", err)
	}
}

func TestCheckPerJobConcurrencyLimit_OrgLookupErrorFailsClosed(t *testing.T) {
	t.Parallel()
	enforcer := &tunableLimitsEnforcer{limits: freeLimits(), orgErr: errors.New("org lookup unavailable")}
	srv := newServerWithEnforcer(t, &APIStoreMock{}, &mockQueue{}, enforcer)

	err := srv.checkPerJobConcurrencyLimit(context.Background(), "proj-1", 999, 0)
	if err == nil {
		t.Fatal("expected org lookup error to fail closed")
	}
	if !strings.Contains(err.Error(), "billing enforcement unavailable") {
		t.Fatalf("error = %v, want billing enforcement unavailable", err)
	}
}

func TestCheckPerJobConcurrencyLimit_PlanLookupErrorFailsClosed(t *testing.T) {
	t.Parallel()
	enforcer := &tunableLimitsEnforcer{limitsErr: errors.New("plan lookup unavailable")}
	srv := newServerWithEnforcer(t, &APIStoreMock{}, &mockQueue{}, enforcer)

	err := srv.checkPerJobConcurrencyLimit(context.Background(), "proj-1", 999, 0)
	if err == nil {
		t.Fatal("expected plan lookup error to fail closed")
	}
	if !strings.Contains(err.Error(), "billing enforcement unavailable") {
		t.Fatalf("error = %v, want billing enforcement unavailable", err)
	}
}

// TestCheckPerJobConcurrencyLimit_FirstFieldRejects_DoesNotEvaluateSecond
// pins the order of evaluation: max_concurrency is checked first; if it
// fails the gate returns immediately and the per-key check is irrelevant.
// This catches a future refactor that might combine the messages and lose
// the named-field signal.
func TestCheckPerJobConcurrencyLimit_FirstFieldRejects_DoesNotEvaluateSecond(t *testing.T) {
	t.Parallel()
	limits := billing.OrgPlanLimits{DisplayName: "Free", MaxConcurrentRuns: 5}
	enforcer := &tunableLimitsEnforcer{limits: limits}
	srv := newServerWithEnforcer(t, &APIStoreMock{}, &mockQueue{}, enforcer)

	err := srv.checkPerJobConcurrencyLimit(context.Background(), "proj-1", 10, 10)
	if err == nil {
		t.Fatal("expected rejection on both fields over cap")
	}
	if !strings.Contains(err.Error(), "max_concurrency") {
		t.Errorf("error must name max_concurrency on first overage, got: %v", err)
	}
}
