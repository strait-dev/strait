package billing

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProjectBudget_RaceUnderConcurrency drives 50 concurrent
// CheckProjectBudgetLimit calls against a project sitting just below
// its monthly budget. The point isn't to assert a precise allow/deny
// split (the underlying spend doesn't actually move in this test);
// it's to confirm we do not panic, deadlock, or trip the race
// detector on the shared enforcer + mock store.
func TestProjectBudget_RaceUnderConcurrency(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)

	store.subscriptions = map[string]*OrgSubscription{
		"org-pb": {OrgID: "org-pb", PlanTier: string(domain.PlanPro), Status: "active"},
	}
	store.getProjectOrgIDFn = func(_ context.Context, _ string) (string, error) {
		return "org-pb", nil
	}
	store.getProjectBudgetFn = func(_ context.Context, _ string) (int64, string, error) {
		return 100_000, "block", nil
	}
	store.getProjectPeriodSpendFn = func(_ context.Context, _ string, _ time.Time) (int64, error) {
		return 99_999, nil
	}

	const n = 50
	var wg sync.WaitGroup
	wg.Add(n)
	var calls atomic.Int32
	for range n {
		concWG.Go(func() {
			defer wg.Done()
			_ = enforcer.CheckProjectBudgetLimit(context.Background(), "proj-pb")
			calls.Add(1)
		})
	}
	wg.Wait()
	assert.Equal(t, int32(n), calls.
		Load())
}

// TestProjectBudget_OrgLimitVsProjectLimit_DistinctErrors locks the
// independent rejection codes so a future refactor can't accidentally
// merge the two error paths and double-charge upgrade prompts.
func TestProjectBudget_OrgLimitVsProjectLimit_DistinctErrors(t *testing.T) {
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
		return 200, nil
	}

	err := enforcer.CheckProjectBudgetLimit(context.Background(), "proj-pb")
	var lim *LimitError
	require.ErrorAs(t, err, &lim)
	assert.Equal(t, "project_budget_reached",

		lim.Code,
	)
	assert.NotEqual(t,
		"spending_limit_reached",

		lim.Code,
	)
}

// TestProjectBudget_NotifyActionDoesNotLeakIntoBlock is a regression
// guard: a project with action='notify' must NEVER block, regardless
// of how high the spend overshoots. This locks in the user-visible
// contract that "notify" is an alerting tier, not a soft block.
func TestProjectBudget_NotifyActionDoesNotLeakIntoBlock(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)

	store.subscriptions = map[string]*OrgSubscription{
		"org-pb": {OrgID: "org-pb", PlanTier: string(domain.PlanPro), Status: "active"},
	}
	store.getProjectOrgIDFn = func(_ context.Context, _ string) (string, error) {
		return "org-pb", nil
	}
	store.getProjectBudgetFn = func(_ context.Context, _ string) (int64, string, error) {
		return 100, "notify", nil
	}
	// 1000x over budget — must still allow under "notify".
	store.getProjectPeriodSpendFn = func(_ context.Context, _ string, _ time.Time) (int64, error) {
		return 100_000, nil
	}
	assert.NoError(t,
		enforcer.
			CheckProjectBudgetLimit(context.Background(), "proj-pb"))
}

// TestProjectBudget_UnknownActionTreatedAsNotify documents the
// fallback contract for any future action value (e.g., "throttle")
// that hasn't been explicitly wired into block semantics. The safe
// default is "do not block" — an unknown action must not become a
// silent block-by-default, which would surprise customers post-
// migration.
func TestProjectBudget_UnknownActionTreatedAsNotify(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)

	store.subscriptions = map[string]*OrgSubscription{
		"org-pb": {OrgID: "org-pb", PlanTier: string(domain.PlanPro), Status: "active"},
	}
	store.getProjectOrgIDFn = func(_ context.Context, _ string) (string, error) {
		return "org-pb", nil
	}
	store.getProjectBudgetFn = func(_ context.Context, _ string) (int64, string, error) {
		return 100, "throttle-future", nil // not yet implemented
	}
	store.getProjectPeriodSpendFn = func(_ context.Context, _ string, _ time.Time) (int64, error) {
		return 200, nil
	}
	assert.NoError(t,
		enforcer.
			CheckProjectBudgetLimit(context.Background(), "proj-pb"))
}
