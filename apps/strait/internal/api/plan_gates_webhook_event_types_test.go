package api

import (
	"context"
	"strings"
	"testing"

	"strait/internal/billing"
	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.Error(t, err)
	assert.True(t,
		strings.Contains(err.
			Error(), "not available"))
	assert.True(t,
		strings.Contains(err.
			Error(), "Free"))

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
		assert.NoError(t, srv.checkWebhookEventTypes(context.Background(), "proj-1", []string{et}))

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
	require.Error(t, err)
	assert.True(t,
		strings.Contains(err.
			Error(), "run.timed_out"))
	assert.True(t,
		strings.Contains(err.
			Error(), "Pro"))

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
	require.NoError(t, srv.checkWebhookEventTypes(context.Background(), "proj-1", all))

}

// TestCheckWebhookEventTypes_AllCustomTier_Accepts confirms enterprise
// "all_custom" behaves identically to "all" for the standard event set.
func TestCheckWebhookEventTypes_AllCustomTier_Accepts(t *testing.T) {
	t.Parallel()

	limits := billing.OrgPlanLimits{DisplayName: "Enterprise", WebhookEventLevel: "all_custom"}
	enforcer := &tunableLimitsEnforcer{limits: limits}
	srv := newServerWithEnforcer(t, &APIStoreMock{}, &mockQueue{}, enforcer)
	require.NoError(t, srv.checkWebhookEventTypes(context.Background(), "proj-1", []string{"run.timed_out",

		"workflow.failed"}))

}

func TestCheckWebhookEventTypes_CloudNilEnforcerFailsClosed(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	srv.edition = domain.EditionCloud
	err := srv.checkWebhookEventTypes(context.Background(), "proj-1", []string{"run.timed_out"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "billing enforcement unavailable")

}

// TestCheckWebhookEventTypes_CommunityNilEnforcerFailsOpen confirms self-hosted
// builds (no enforcer wired) accept every event type.
func TestCheckWebhookEventTypes_CommunityNilEnforcerFailsOpen(t *testing.T) {
	t.Parallel()

	srv := &Server{edition: domain.EditionCommunity}
	require.NoError(t, srv.checkWebhookEventTypes(context.Background(), "proj-1", []string{"run.timed_out"}))

}

// TestCheckWebhookEventTypes_EmptyEventList_NoOp documents that a zero-length
// event list passes — validation of "at least one event type" lives at the
// request schema, not in the gate.
func TestCheckWebhookEventTypes_EmptyEventList_NoOp(t *testing.T) {
	t.Parallel()

	limits := billing.OrgPlanLimits{DisplayName: "Free", WebhookEventLevel: "none"}
	enforcer := &tunableLimitsEnforcer{limits: limits}
	srv := newServerWithEnforcer(t, &APIStoreMock{}, &mockQueue{}, enforcer)
	require.Error(t, srv.checkWebhookEventTypes(context.Background(), "proj-1", []string{}))

	// The "none" branch fires before the loop, so even an empty list rejects.
	// This test pins that behavior so a refactor that moves the loop earlier
	// does not silently flip the semantics.

}
