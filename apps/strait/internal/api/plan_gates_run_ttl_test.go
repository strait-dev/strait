package api

import (
	"context"
	"errors"
	"strings"
	"testing"

	"strait/internal/billing"
	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCheckRunTTLLimit_ZeroTTL_NoCap proves the gate ignores zero (the
// platform-default sentinel) regardless of plan tier — every tier must accept
// jobs that don't pin a TTL explicitly.
func TestCheckRunTTLLimit_ZeroTTL_NoCap(t *testing.T) {
	t.Parallel()
	enforcer := &tunableLimitsEnforcer{limits: freeLimits()}
	srv := newServerWithEnforcer(t, &APIStoreMock{}, &mockQueue{}, enforcer)
	require.NoError(t, srv.
		checkRunTTLLimit(context.
			Background(), "proj-1", 0,
		))

}

// TestCheckRunTTLLimit_FreeAtLimit_Allows verifies the cap is inclusive — Free
// retains 7 days, so a request for exactly 7*86400 must succeed.
func TestCheckRunTTLLimit_FreeAtLimit_Allows(t *testing.T) {
	t.Parallel()
	limits := freeLimits()
	enforcer := &tunableLimitsEnforcer{limits: limits}
	srv := newServerWithEnforcer(t, &APIStoreMock{}, &mockQueue{}, enforcer)

	maxTTL := limits.RetentionDays * 86400
	require.NoError(t, srv.
		checkRunTTLLimit(context.
			Background(), "proj-1", maxTTL,
		))

}

// TestCheckRunTTLLimit_FreeOverLimit_Rejects walks one second past the cap and
// asserts the rejection message names the plan, retention window, and the
// requested value so the customer can self-diagnose.
func TestCheckRunTTLLimit_FreeOverLimit_Rejects(t *testing.T) {
	t.Parallel()
	limits := freeLimits()
	enforcer := &tunableLimitsEnforcer{limits: limits}
	srv := newServerWithEnforcer(t, &APIStoreMock{}, &mockQueue{}, enforcer)

	maxTTL := limits.RetentionDays * 86400
	err := srv.checkRunTTLLimit(context.Background(), "proj-1", maxTTL+1)
	require.Error(t, err)

	for _, fragment := range []string{limits.DisplayName, "retains", "run_ttl_secs"} {
		assert.True(t,
			strings.Contains(err.
				Error(),
				fragment,
			))

	}
}

// TestCheckRunTTLLimit_EnterpriseUnlimited_Allows confirms RetentionDays=-1
// short-circuits the cap entirely.
func TestCheckRunTTLLimit_EnterpriseUnlimited_Allows(t *testing.T) {
	t.Parallel()
	enforcer := &tunableLimitsEnforcer{limits: enterpriseLimits()}
	srv := newServerWithEnforcer(t, &APIStoreMock{}, &mockQueue{}, enforcer)
	require.NoError(t, srv.
		checkRunTTLLimit(context.
			Background(), "proj-1", 365*
			86400))

}

func TestCheckRunTTLLimit_CloudNilEnforcerFailsClosed(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	srv.edition = domain.EditionCloud

	err := srv.checkRunTTLLimit(context.Background(), "proj-1", 999_999_999)
	require.Error(t, err)
	assert.Contains(t, err.
		Error(), "billing enforcement unavailable",
	)

}

// TestCheckRunTTLLimit_CommunityNilEnforcerFailsOpen confirms self-hosted
// builds (no enforcer) accept any ttl.
func TestCheckRunTTLLimit_CommunityNilEnforcerFailsOpen(t *testing.T) {
	t.Parallel()
	srv := &Server{edition: domain.EditionCommunity}
	require.NoError(t, srv.
		checkRunTTLLimit(context.
			Background(), "proj-1", 999_999_999,
		))

}

func TestCheckRunTTLLimit_OrgLookupErrorFailsClosed(t *testing.T) {
	t.Parallel()
	enforcer := &tunableLimitsEnforcer{limits: freeLimits(), orgErr: errors.New("org lookup unavailable")}
	srv := newServerWithEnforcer(t, &APIStoreMock{}, &mockQueue{}, enforcer)

	err := srv.checkRunTTLLimit(context.Background(), "proj-1", 999_999)
	require.Error(t, err)
	require.True(
		t, strings.Contains(err.
			Error(),

			"billing enforcement unavailable",
		))

}

func TestCheckRunTTLLimit_PlanLookupErrorFailsClosed(t *testing.T) {
	t.Parallel()
	enforcer := &tunableLimitsEnforcer{limitsErr: errors.New("plan lookup unavailable")}
	srv := newServerWithEnforcer(t, &APIStoreMock{}, &mockQueue{}, enforcer)

	err := srv.checkRunTTLLimit(context.Background(), "proj-1", 999_999)
	require.Error(t, err)
	require.True(
		t, strings.Contains(err.
			Error(),

			"billing enforcement unavailable",
		))

}

// TestCheckRunTTLLimit_RetentionZero_NoCap matches the production behavior of
// the unset / unknown tier path: when the plan exposes no retention, the gate
// declines to enforce rather than guessing a default.
func TestCheckRunTTLLimit_RetentionZero_NoCap(t *testing.T) {
	t.Parallel()
	limits := billing.OrgPlanLimits{DisplayName: "Custom", RetentionDays: 0}
	enforcer := &tunableLimitsEnforcer{limits: limits}
	srv := newServerWithEnforcer(t, &APIStoreMock{}, &mockQueue{}, enforcer)
	require.NoError(t, srv.
		checkRunTTLLimit(context.
			Background(), "proj-1", 999_999_999,
		))

}
