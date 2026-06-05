package api

import (
	"context"
	"strings"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.Error(t, err)
	assert.True(t,
		strings.Contains(err.
			Error(), "not available"))

}

// TestCheckAlertRuleLimit_ProTier_RejectsZeroCap verifies alert rules are not
// launch-active until the HTTP handler and storage path exist.
func TestCheckAlertRuleLimit_ProTier_RejectsZeroCap(t *testing.T) {
	t.Parallel()

	enforcer := &tunableLimitsEnforcer{limits: proLimits()}
	srv := newServerWithEnforcer(t, &APIStoreMock{}, &mockQueue{}, enforcer)

	err := srv.checkAlertRuleLimit(context.Background(), "proj-1", 0)
	require.Error(t, err)
	assert.True(t,
		strings.Contains(err.
			Error(), "not available"))

}

// TestCheckAlertRuleLimit_Enterprise_RejectsUntilLaunchActive verifies
// Enterprise does not imply an active alert-rule product claim at launch.
func TestCheckAlertRuleLimit_Enterprise_RejectsUntilLaunchActive(t *testing.T) {
	t.Parallel()

	enforcer := &tunableLimitsEnforcer{limits: enterpriseLimits()}
	srv := newServerWithEnforcer(t, &APIStoreMock{}, &mockQueue{}, enforcer)

	err := srv.checkAlertRuleLimit(context.Background(), "proj-1", 0)
	require.Error(t, err)

}

// TestCheckAlertRuleLimit_NilEnforcer_FailsOpen confirms that the
// community-edition path (no enforcer) does not reject.
func TestCheckAlertRuleLimit_NilEnforcer_FailsOpen(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	srv.edition = domain.EditionCommunity
	require.NoError(t, srv.checkAlertRuleLimit(context.Background(),

		"proj-1", 9999))

}
