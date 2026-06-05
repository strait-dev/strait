package billing

import (
	"context"
	"errors"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// projectBudgetCase compactly enumerates the 2x2 budget matrix
// (action ∈ {notify, block} × spend vs budget) plus the "no row"
// fall-through. Each case asserts the precise behavior contract that
// dispatch relies on.
type projectBudgetCase struct {
	name        string
	budget      int64
	action      string
	spend       int64
	wantErrCode string // empty == expect nil
}

func TestEnforcer_CheckProjectBudgetLimit_Matrix(t *testing.T) {
	t.Parallel()

	cases := []projectBudgetCase{
		{name: "no quota row defaults to notify", budget: -1, action: "notify", spend: 0},
		{name: "notify under budget", budget: 100, action: "notify", spend: 50},
		{name: "notify over budget", budget: 100, action: "notify", spend: 200},
		{name: "block under budget", budget: 100, action: "block", spend: 50},
		{name: "block at budget", budget: 100, action: "block", spend: 100, wantErrCode: "project_budget_reached"},
		{name: "block over budget", budget: 100, action: "block", spend: 200, wantErrCode: "project_budget_reached"},
		{name: "block budget=0 always rejects", budget: 0, action: "block", spend: 1, wantErrCode: "project_budget_reached"},
		{name: "reject at budget", budget: 100, action: "reject", spend: 100, wantErrCode: "project_budget_reached"},
		{name: "reject over budget", budget: 100, action: "reject", spend: 200, wantErrCode: "project_budget_reached"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			enforcer, store, _ := setupEnforcer(t)

			store.subscriptions = map[string]*OrgSubscription{
				"org-pb": {OrgID: "org-pb", PlanTier: string(domain.PlanPro), Status: "active"},
			}
			store.getProjectOrgIDFn = func(_ context.Context, _ string) (string, error) {
				return "org-pb", nil
			}
			store.getProjectBudgetFn = func(_ context.Context, _ string) (int64, string, error) {
				return tc.budget, tc.action, nil
			}
			store.getProjectPeriodSpendFn = func(_ context.Context, _ string, _ time.Time) (int64, error) {
				return tc.spend, nil
			}

			err := enforcer.CheckProjectBudgetLimit(context.Background(), "proj-pb")
			if tc.wantErrCode == "" {
				require.NoError(t,
					err)

				return
			}
			var lim *LimitError
			require.True(t, errors.As(err,
				&lim))
			assert.Equal(t, tc.
				wantErrCode,
				lim.Code,
			)
			assert.Equal(t, tc.
				budget,
				lim.Limit)
			assert.Equal(t, tc.
				spend, lim.
				CurrentUsage,
			)

		})
	}
}

// TestEnforcer_CheckProjectBudgetLimit_EmptyProjectID guards the
// "early return" contract: callers should be free to pass an empty
// projectID without nil-guarding the call.
func TestEnforcer_CheckProjectBudgetLimit_EmptyProjectID(t *testing.T) {
	t.Parallel()
	enforcer, _, _ := setupEnforcer(t)
	assert.NoError(t,
		enforcer.
			CheckProjectBudgetLimit(context.Background(),
				"",
			),
	)

}

// TestEnforcer_CheckProjectBudgetLimit_NilEnforcer protects the
// community-edition path where the enforcer is wired as nil.
func TestEnforcer_CheckProjectBudgetLimit_NilEnforcer(t *testing.T) {
	t.Parallel()
	var enforcer *Enforcer
	assert.NoError(t,
		enforcer.
			CheckProjectBudgetLimit(context.Background(),
				"proj-x",
			))

}

// TestEnforcer_CheckProjectBudgetLimit_BudgetReadFailsClosed confirms that a
// transient DB error reading project_quotas cannot bypass a blocking budget.
func TestEnforcer_CheckProjectBudgetLimit_BudgetReadFailsClosed(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)

	store.getProjectBudgetFn = func(_ context.Context, _ string) (int64, string, error) {
		return 0, "", errors.New("transient db error")
	}

	err := enforcer.CheckProjectBudgetLimit(context.Background(), "proj-x")
	var lim *LimitError
	require.True(t, errors.As(err,
		&lim))
	require.Equal(t,
		"service_degraded",
		lim.
			Code)

}

// TestEnforcer_CheckProjectBudgetLimit_SpendReadFailsClosed mirrors the
// above: a usage_records read error cannot bypass a blocking budget.
func TestEnforcer_CheckProjectBudgetLimit_SpendReadFailsClosed(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)

	store.subscriptions = map[string]*OrgSubscription{
		"org-pb": {OrgID: "org-pb", PlanTier: string(domain.PlanPro), Status: "active"},
	}
	store.getProjectOrgIDFn = func(_ context.Context, _ string) (string, error) {
		return "org-pb", nil
	}
	store.getProjectBudgetFn = func(_ context.Context, _ string) (int64, string, error) {
		return 100, "block", nil
	}
	store.getProjectPeriodSpendFn = func(_ context.Context, _ string, _ time.Time) (int64, error) {
		return 0, errors.New("transient db error")
	}

	err := enforcer.CheckProjectBudgetLimit(context.Background(), "proj-x")
	var lim *LimitError
	require.True(t, errors.As(err,
		&lim))
	require.Equal(t,
		"service_degraded",
		lim.
			Code)

}

// TestEnforcer_CheckProjectBudgetLimit_OrgResolutionFailsClosed confirms that
// a project→org lookup error cannot bypass a blocking project budget.
func TestEnforcer_CheckProjectBudgetLimit_OrgResolutionFailsClosed(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)

	store.getProjectBudgetFn = func(_ context.Context, _ string) (int64, string, error) {
		return 100, "block", nil
	}
	store.getProjectOrgIDFn = func(_ context.Context, _ string) (string, error) {
		return "", errors.New("project not found")
	}

	err := enforcer.CheckProjectBudgetLimit(context.Background(), "proj-orphan")
	var lim *LimitError
	require.True(t, errors.As(err,
		&lim))
	require.Equal(t,
		"service_degraded",
		lim.
			Code)

}
