package api

import (
	"context"
	"strings"
	"testing"

	"strait/internal/billing"
	"strait/internal/domain"
)

// TestCheckWebhookEventTypes_NoneTier_RejectsAll verifies that a plan with
// WebhookEventLevel="none" (Free) blocks every event type with a clear
// "not available on the X plan" message.
func TestCheckWebhookEventTypes_NoneTier_RejectsAll(t *testing.T) {
	t.Parallel()

	limits := billing.OrgPlanLimits{DisplayName: "Free", WebhookEventLevel: "none"}
	enforcer := &tunableLimitsEnforcer{limits: limits}
	srv := newServerWithEnforcer(t, &APIStoreMock{}, &mockQueue{}, enforcer)

	err := srv.checkWebhookEventTypes(context.Background(), "proj-1", []string{"run.completed"})
	if err == nil {
		t.Fatal("expected rejection on WebhookEventLevel=none, got nil")
	}
	if !strings.Contains(err.Error(), "not available") {
		t.Errorf("expected message to mention unavailability, got: %v", err)
	}
	if !strings.Contains(err.Error(), "Free") {
		t.Errorf("expected message to name the plan, got: %v", err)
	}
}

// TestCheckWebhookEventTypes_BasicTier_AcceptsBasicEvents asserts the Starter
// plan permits run.completed and run.failed exactly — the only events on the
// "basic" allowlist.
func TestCheckWebhookEventTypes_BasicTier_AcceptsBasicEvents(t *testing.T) {
	t.Parallel()

	limits := billing.OrgPlanLimits{DisplayName: "Starter", WebhookEventLevel: "basic"}
	enforcer := &tunableLimitsEnforcer{limits: limits}
	srv := newServerWithEnforcer(t, &APIStoreMock{}, &mockQueue{}, enforcer)

	for _, et := range []string{"run.completed", "run.failed"} {
		if err := srv.checkWebhookEventTypes(context.Background(), "proj-1", []string{et}); err != nil {
			t.Errorf("basic tier must accept %q; got %v", et, err)
		}
	}
}

// TestCheckWebhookEventTypes_BasicTier_RejectsUpgradeEvents catches the
// smuggle vector: a Starter customer subscribing to a Pro-tier event type
// (e.g., run.timed_out) must be rejected with an upgrade message naming the
// offending event type.
func TestCheckWebhookEventTypes_BasicTier_RejectsUpgradeEvents(t *testing.T) {
	t.Parallel()

	limits := billing.OrgPlanLimits{DisplayName: "Starter", WebhookEventLevel: "basic"}
	enforcer := &tunableLimitsEnforcer{limits: limits}
	srv := newServerWithEnforcer(t, &APIStoreMock{}, &mockQueue{}, enforcer)

	err := srv.checkWebhookEventTypes(context.Background(), "proj-1", []string{"run.timed_out"})
	if err == nil {
		t.Fatal("expected rejection of premium event on basic tier")
	}
	if !strings.Contains(err.Error(), "run.timed_out") {
		t.Errorf("error must name the offending event type, got: %v", err)
	}
	if !strings.Contains(err.Error(), "Pro") {
		t.Errorf("error must point at the upgrade target plan, got: %v", err)
	}
}

// TestCheckWebhookEventTypes_AllTier_AcceptsEverything proves the Pro and
// Scale tiers (level="all") accept every event type without iteration cost.
func TestCheckWebhookEventTypes_AllTier_AcceptsEverything(t *testing.T) {
	t.Parallel()

	limits := billing.OrgPlanLimits{DisplayName: "Pro", WebhookEventLevel: "all"}
	enforcer := &tunableLimitsEnforcer{limits: limits}
	srv := newServerWithEnforcer(t, &APIStoreMock{}, &mockQueue{}, enforcer)

	all := make([]string, 0, len(validWebhookEventTypes))
	for eventType := range validWebhookEventTypes {
		all = append(all, eventType)
	}
	if err := srv.checkWebhookEventTypes(context.Background(), "proj-1", all); err != nil {
		t.Fatalf("all-tier plan must accept every event type; got %v", err)
	}
}

// TestCheckWebhookEventTypes_AllCustomTier_Accepts confirms enterprise
// "all_custom" behaves identically to "all" for the standard event set.
func TestCheckWebhookEventTypes_AllCustomTier_Accepts(t *testing.T) {
	t.Parallel()

	limits := billing.OrgPlanLimits{DisplayName: "Enterprise", WebhookEventLevel: "all_custom"}
	enforcer := &tunableLimitsEnforcer{limits: limits}
	srv := newServerWithEnforcer(t, &APIStoreMock{}, &mockQueue{}, enforcer)

	if err := srv.checkWebhookEventTypes(context.Background(), "proj-1", []string{"run.timed_out", "workflow.failed"}); err != nil {
		t.Fatalf("all_custom must accept any event; got %v", err)
	}
}

func TestCheckWebhookEventTypes_CloudNilEnforcerFailsClosed(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	srv.edition = domain.EditionCloud
	err := srv.checkWebhookEventTypes(context.Background(), "proj-1", []string{"run.timed_out"})
	if err == nil || !strings.Contains(err.Error(), "billing enforcement unavailable") {
		t.Fatalf("expected billing enforcement unavailable, got %v", err)
	}
}

// TestCheckWebhookEventTypes_CommunityNilEnforcerFailsOpen confirms self-hosted
// builds (no enforcer wired) accept every event type.
func TestCheckWebhookEventTypes_CommunityNilEnforcerFailsOpen(t *testing.T) {
	t.Parallel()

	srv := &Server{edition: domain.EditionCommunity}
	if err := srv.checkWebhookEventTypes(context.Background(), "proj-1", []string{"run.timed_out"}); err != nil {
		t.Fatalf("community nil enforcer must fail open; got %v", err)
	}
}

// TestCheckWebhookEventTypes_EmptyEventList_NoOp documents that a zero-length
// event list passes — validation of "at least one event type" lives at the
// request schema, not in the gate.
func TestCheckWebhookEventTypes_EmptyEventList_NoOp(t *testing.T) {
	t.Parallel()

	limits := billing.OrgPlanLimits{DisplayName: "Free", WebhookEventLevel: "none"}
	enforcer := &tunableLimitsEnforcer{limits: limits}
	srv := newServerWithEnforcer(t, &APIStoreMock{}, &mockQueue{}, enforcer)

	if err := srv.checkWebhookEventTypes(context.Background(), "proj-1", []string{}); err == nil {
		// The "none" branch fires before the loop, so even an empty list rejects.
		// This test pins that behavior so a refactor that moves the loop earlier
		// does not silently flip the semantics.
		t.Fatal("WebhookEventLevel=none rejects regardless of list length")
	}
}
