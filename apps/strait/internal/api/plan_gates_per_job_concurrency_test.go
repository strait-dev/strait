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

// TestCheckPerJobConcurrencyLimit_ZeroValues_NoOp pins the platform-default
// behavior: zero on either field means "use the engine default", so the gate
// must not enforce a cap when both inputs are zero.
func TestCheckPerJobConcurrencyLimit_ZeroValues_NoOp(t *testing.T) {
	t.Parallel()
	enforcer := &tunableLimitsEnforcer{limits: freeLimits()}
	srv := newServerWithEnforcer(t, &APIStoreMock{}, &mockQueue{}, enforcer)
	require.NoError(t, srv.
		checkPerJobConcurrencyLimit(context.Background(),
			"proj-1", 0, 0))

}

// TestCheckPerJobConcurrencyLimit_FreeAtLimit_Allows verifies the cap is
// inclusive — a per-job concurrency exactly equal to the org-wide cap is
// accepted (the engine still enforces the org-wide limit at dispatch).
func TestCheckPerJobConcurrencyLimit_FreeAtLimit_Allows(t *testing.T) {
	t.Parallel()
	limits := freeLimits()
	enforcer := &tunableLimitsEnforcer{limits: limits}
	srv := newServerWithEnforcer(t, &APIStoreMock{}, &mockQueue{}, enforcer)
	require.NoError(t, srv.
		checkPerJobConcurrencyLimit(context.Background(),
			"proj-1", limits.MaxConcurrentRuns,

			0))
	require.NoError(t, srv.
		checkPerJobConcurrencyLimit(context.Background(),
			"proj-1", 0, limits.MaxConcurrentRuns,
		))

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
	require.Error(t, err)

	for _, fragment := range []string{limits.DisplayName, "max_concurrency"} {
		assert.True(t,
			strings.Contains(err.
				Error(),
				fragment,
			))

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
	require.Error(t, err)
	assert.True(t,
		strings.Contains(err.
			Error(),
			"max_concurrency_per_key",
		))

}

// TestCheckPerJobConcurrencyLimit_EnterpriseUnlimited_Allows confirms the
// MaxConcurrentRuns=-1 sentinel short-circuits the cap.
func TestCheckPerJobConcurrencyLimit_EnterpriseUnlimited_Allows(t *testing.T) {
	t.Parallel()
	enforcer := &tunableLimitsEnforcer{limits: enterpriseLimits()}
	srv := newServerWithEnforcer(t, &APIStoreMock{}, &mockQueue{}, enforcer)
	require.NoError(t, srv.
		checkPerJobConcurrencyLimit(context.Background(),
			"proj-1", 100_000, 100_000,
		))

}

func TestCheckPerJobConcurrencyLimit_CloudNilEnforcerFailsClosed(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	srv.edition = domain.EditionCloud

	err := srv.checkPerJobConcurrencyLimit(context.Background(), "proj-1", 999_999, 999_999)
	require.Error(t, err)
	assert.Contains(t, err.
		Error(), "billing enforcement unavailable",
	)

}

// TestCheckPerJobConcurrencyLimit_CommunityNilEnforcerFailsOpen confirms
// self-hosted builds (no enforcer wired) accept any concurrency setting.
func TestCheckPerJobConcurrencyLimit_CommunityNilEnforcerFailsOpen(t *testing.T) {
	t.Parallel()
	srv := &Server{edition: domain.EditionCommunity}
	require.NoError(t, srv.
		checkPerJobConcurrencyLimit(context.Background(),
			"proj-1", 999_999, 999_999,
		))

}

func TestCheckPerJobConcurrencyLimit_OrgLookupErrorFailsClosed(t *testing.T) {
	t.Parallel()
	enforcer := &tunableLimitsEnforcer{limits: freeLimits(), orgErr: errors.New("org lookup unavailable")}
	srv := newServerWithEnforcer(t, &APIStoreMock{}, &mockQueue{}, enforcer)

	err := srv.checkPerJobConcurrencyLimit(context.Background(), "proj-1", 999, 0)
	require.Error(t, err)
	require.True(
		t, strings.Contains(err.
			Error(),

			"billing enforcement unavailable",
		))

}

func TestCheckPerJobConcurrencyLimit_PlanLookupErrorFailsClosed(t *testing.T) {
	t.Parallel()
	enforcer := &tunableLimitsEnforcer{limitsErr: errors.New("plan lookup unavailable")}
	srv := newServerWithEnforcer(t, &APIStoreMock{}, &mockQueue{}, enforcer)

	err := srv.checkPerJobConcurrencyLimit(context.Background(), "proj-1", 999, 0)
	require.Error(t, err)
	require.True(
		t, strings.Contains(err.
			Error(),

			"billing enforcement unavailable",
		))

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
	require.Error(t, err)
	assert.True(t,
		strings.Contains(err.
			Error(),
			"max_concurrency",
		))

}
