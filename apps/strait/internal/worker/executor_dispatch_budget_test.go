package worker

import (
	"testing"

	"strait/internal/billing"
	"strait/internal/domain"
)

// TestDispatchProjectBudget_NotifyDoesNotBlock confirms that the
// dispatch path does NOT reject runs for projects whose
// project_quotas row carries action='notify' (the default for new
// rows). This is the bedrock contract: budgets are alerting until
// the customer explicitly opts into block mode.
func TestDispatchProjectBudget_NotifyDoesNotBlock(t *testing.T) {
	t.Parallel()
	sub := &billing.OrgSubscription{
		OrgID:                 "org-pb-notify",
		PlanTier:              string(domain.PlanPro),
		Status:                "active",
		SpendingLimitMicrousd: -1,
	}
	h := newDispatchHarnessWithBudget(t, sub, 0, 100_000, "notify", 200_000)

	runDispatch(h, "run-pb-notify")
	if sawSystemFailed(h.store) {
		t.Errorf("dispatch was rejected for notify-action project budget; expected pass-through")
	}
}

// TestDispatchProjectBudget_BlockUnderBudget_Proceeds confirms
// the happy path: block-mode budget but the project is well
// under the budget, dispatch must proceed.
func TestDispatchProjectBudget_BlockUnderBudget_Proceeds(t *testing.T) {
	t.Parallel()
	sub := &billing.OrgSubscription{
		OrgID:                 "org-pb-block-under",
		PlanTier:              string(domain.PlanPro),
		Status:                "active",
		SpendingLimitMicrousd: -1,
	}
	h := newDispatchHarnessWithBudget(t, sub, 0, 1_000_000, "block", 100_000)

	runDispatch(h, "run-pb-block-under")
	if sawSystemFailed(h.store) {
		t.Errorf("dispatch was rejected with project spend below the budget")
	}
}

// TestDispatchProjectBudget_BlockOverBudget_Rejects verifies that spend over
// the budget with action='block' rejects the run before any counter increments.
func TestDispatchProjectBudget_BlockOverBudget_Rejects(t *testing.T) {
	t.Parallel()
	sub := &billing.OrgSubscription{
		OrgID:                 "org-pb-block-over",
		PlanTier:              string(domain.PlanPro),
		Status:                "active",
		SpendingLimitMicrousd: -1,
	}
	h := newDispatchHarnessWithBudget(t, sub, 0, 1_000_000, "block", 5_000_000)

	runDispatch(h, "run-pb-block-over")
	if !sawSystemFailed(h.store) {
		t.Errorf("expected system_failed when project budget exceeded")
	}
}

// TestDispatchProjectBudget_BlockAtBudget_Rejects locks in the
// limit-inclusive `>=` semantics shared with CheckSpendingLimit.
// Hitting the budget exactly counts as reached.
func TestDispatchProjectBudget_BlockAtBudget_Rejects(t *testing.T) {
	t.Parallel()
	sub := &billing.OrgSubscription{
		OrgID:                 "org-pb-block-at",
		PlanTier:              string(domain.PlanPro),
		Status:                "active",
		SpendingLimitMicrousd: -1,
	}
	h := newDispatchHarnessWithBudget(t, sub, 0, 2_500_000, "block", 2_500_000)

	runDispatch(h, "run-pb-block-at")
	if !sawSystemFailed(h.store) {
		t.Errorf("expected rejection at exact-equal project budget; isOverageLimitReached uses >=")
	}
}

// TestDispatchProjectBudget_NoQuotaRow_Proceeds: a project with no
// project_quotas row resolves to (-1, "notify") in the store and
// must fall through cleanly.
func TestDispatchProjectBudget_NoQuotaRow_Proceeds(t *testing.T) {
	t.Parallel()
	sub := &billing.OrgSubscription{
		OrgID:                 "org-pb-noquota",
		PlanTier:              string(domain.PlanPro),
		Status:                "active",
		SpendingLimitMicrousd: -1,
	}
	// projectBudget=-1, action="notify" → no row → no-op.
	h := newDispatchHarnessWithBudget(t, sub, 0, -1, "notify", 0)

	runDispatch(h, "run-pb-noquota")
	if sawSystemFailed(h.store) {
		t.Errorf("project without quota row should not be blocked by project budget check")
	}
}
