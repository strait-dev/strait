package billing

import (
	"context"
	"testing"

	"strait/internal/domain"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func setupAgentEnforcer(t *testing.T, store *mockBillingStore) *Enforcer {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return NewEnforcer(store, rdb, nil)
}

func TestCheckAgentSpendingLimit_FreePlan_UnderCap(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{
		projectOrgMap:   map[string]string{"proj-1": "org-1"},
		subscriptions:   map[string]*OrgSubscription{"org-1": {OrgID: "org-1", PlanTier: "free", AgentPlanTier: "agent_free"}},
		agentSpendByOrg: map[string]int64{"org-1": 500_000}, // $0.50 — under $1 cap
	}
	enforcer := setupAgentEnforcer(t, store)

	err := enforcer.CheckAgentSpendingLimit(context.Background(), "proj-1")
	if err != nil {
		t.Errorf("expected no error (under cap), got: %v", err)
	}
}

func TestCheckAgentSpendingLimit_FreePlan_AtCap(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{
		projectOrgMap:   map[string]string{"proj-1": "org-1"},
		subscriptions:   map[string]*OrgSubscription{"org-1": {OrgID: "org-1", PlanTier: "free", AgentPlanTier: "agent_free"}},
		agentSpendByOrg: map[string]int64{"org-1": 1_000_000}, // exactly $1 — at cap
	}
	enforcer := setupAgentEnforcer(t, store)

	err := enforcer.CheckAgentSpendingLimit(context.Background(), "proj-1")
	if err == nil {
		t.Fatal("expected error at $1 cap, got nil")
	}

	var limitErr *LimitError
	if le, ok := err.(*LimitError); ok {
		limitErr = le
	} else {
		t.Fatalf("expected *LimitError, got %T: %v", err, err)
	}
	if limitErr.Code != "agent_spending_limit_reached" {
		t.Errorf("error code = %q, want agent_spending_limit_reached", limitErr.Code)
	}
	if limitErr.Limit != AgentCreditFreeMicrousd {
		t.Errorf("limit = %d, want %d", limitErr.Limit, AgentCreditFreeMicrousd)
	}
}

func TestCheckAgentSpendingLimit_MakerPlan_NoCapByDefault(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{
		projectOrgMap:   map[string]string{"proj-1": "org-1"},
		subscriptions:   map[string]*OrgSubscription{"org-1": {OrgID: "org-1", PlanTier: "starter", AgentPlanTier: "agent_maker", AgentSpendingLimitMicrousd: -1}},
		agentSpendByOrg: map[string]int64{"org-1": 100_000_000}, // $100 in overage
	}
	enforcer := setupAgentEnforcer(t, store)

	err := enforcer.CheckAgentSpendingLimit(context.Background(), "proj-1")
	if err != nil {
		t.Errorf("paid plan with no spending cap should allow overage, got: %v", err)
	}
}

func TestCheckAgentSpendingLimit_MakerPlan_WithSpendingCap(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{
		projectOrgMap:   map[string]string{"proj-1": "org-1"},
		subscriptions:   map[string]*OrgSubscription{"org-1": {OrgID: "org-1", PlanTier: "starter", AgentPlanTier: "agent_maker", AgentSpendingLimitMicrousd: 50_000_000}},
		agentSpendByOrg: map[string]int64{"org-1": 50_000_000}, // exactly at $50 cap
	}
	enforcer := setupAgentEnforcer(t, store)

	err := enforcer.CheckAgentSpendingLimit(context.Background(), "proj-1")
	if err == nil {
		t.Fatal("expected error at spending cap, got nil")
	}
	var limitErr *LimitError
	if le, ok := err.(*LimitError); ok {
		limitErr = le
	} else {
		t.Fatalf("expected *LimitError, got %T", err)
	}
	if limitErr.Code != "agent_spending_limit_reached" {
		t.Errorf("error code = %q, want agent_spending_limit_reached", limitErr.Code)
	}
}

func TestCheckAgentSpendingLimit_EmptyProjectID(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{}
	enforcer := setupAgentEnforcer(t, store)

	err := enforcer.CheckAgentSpendingLimit(context.Background(), "")
	if err != nil {
		t.Errorf("empty project ID should return nil, got: %v", err)
	}
}

func TestCheckAgentSpendingLimit_NoSubscription_DefaultsFree(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{
		projectOrgMap:   map[string]string{"proj-1": "org-1"},
		agentSpendByOrg: map[string]int64{"org-1": 2_000_000}, // $2 — over free cap
	}
	enforcer := setupAgentEnforcer(t, store)

	err := enforcer.CheckAgentSpendingLimit(context.Background(), "proj-1")
	if err == nil {
		t.Fatal("no subscription + over free cap should reject")
	}
}

func TestGetAgentPlanForProject_ReturnsCorrectTier(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{
		projectOrgMap: map[string]string{"proj-1": "org-1"},
		subscriptions: map[string]*OrgSubscription{"org-1": {OrgID: "org-1", AgentPlanTier: "agent_growth"}},
	}
	enforcer := setupAgentEnforcer(t, store)

	tier, err := enforcer.GetAgentPlanForProject(context.Background(), "proj-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tier != "agent_growth" {
		t.Errorf("tier = %q, want agent_growth", tier)
	}
}

func TestGetAgentPlanForProject_DefaultsFreeOnMissing(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{}
	enforcer := setupAgentEnforcer(t, store)

	tier, err := enforcer.GetAgentPlanForProject(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tier != string(domain.AgentPlanFree) {
		t.Errorf("tier = %q, want %q", tier, domain.AgentPlanFree)
	}
}

func TestGetAgentPlanForProject_EmptyProjectID(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{}
	enforcer := setupAgentEnforcer(t, store)

	tier, err := enforcer.GetAgentPlanForProject(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tier != string(domain.AgentPlanFree) {
		t.Errorf("tier = %q, want %q", tier, domain.AgentPlanFree)
	}
}

func TestEffectiveMaxProjects_HigherAgentPlanWins(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-1": {OrgID: "org-1", PlanTier: "free", AgentPlanTier: "agent_growth"},
		},
	}
	enforcer := setupAgentEnforcer(t, store)

	maxProjects := enforcer.effectiveMaxProjects(context.Background(), "org-1")
	agentLimits := GetAgentPlanLimits(domain.AgentPlanGrowth)
	if maxProjects < agentLimits.MaxProjectsPerOrg {
		t.Errorf("effective max projects = %d, want >= %d (agent growth)", maxProjects, agentLimits.MaxProjectsPerOrg)
	}
}

func TestEffectiveMaxMembers_HigherJobsPlanWins(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-1": {OrgID: "org-1", PlanTier: "pro", AgentPlanTier: "agent_free"},
		},
	}
	enforcer := setupAgentEnforcer(t, store)

	maxMembers := enforcer.effectiveMaxMembers(context.Background(), "org-1")
	jobLimits := GetPlanLimits(domain.PlanPro)
	if maxMembers != jobLimits.MaxMembersPerOrg {
		t.Errorf("effective max members = %d, want %d (jobs pro)", maxMembers, jobLimits.MaxMembersPerOrg)
	}
}

func TestAgentPlanLimits_Values(t *testing.T) {
	t.Parallel()

	tests := []struct {
		tier       domain.PlanTier
		wantDefs   int
		wantConc   int
		wantCredit int64
	}{
		{domain.AgentPlanFree, 3, 5, AgentCreditFreeMicrousd},
		{domain.AgentPlanMaker, 10, 25, AgentCreditMakerMicrousd},
		{domain.AgentPlanGrowth, -1, 200, AgentCreditGrowthMicrousd},
	}

	for _, tt := range tests {
		t.Run(string(tt.tier), func(t *testing.T) {
			t.Parallel()
			limits := GetAgentPlanLimits(tt.tier)
			if limits.MaxAgentDefinitions != tt.wantDefs {
				t.Errorf("MaxAgentDefinitions = %d, want %d", limits.MaxAgentDefinitions, tt.wantDefs)
			}
			if limits.MaxAgentConcurrentRuns != tt.wantConc {
				t.Errorf("MaxAgentConcurrentRuns = %d, want %d", limits.MaxAgentConcurrentRuns, tt.wantConc)
			}
			if limits.AgentCreditMicrousd != tt.wantCredit {
				t.Errorf("AgentCreditMicrousd = %d, want %d", limits.AgentCreditMicrousd, tt.wantCredit)
			}
		})
	}
}

func TestGetAgentPlanLimits_UnknownDefaultsToFree(t *testing.T) {
	t.Parallel()
	limits := GetAgentPlanLimits("nonexistent_tier")
	freeLimits := GetAgentPlanLimits(domain.AgentPlanFree)
	if limits.MaxAgentDefinitions != freeLimits.MaxAgentDefinitions {
		t.Errorf("unknown tier did not default to free: got %d, want %d", limits.MaxAgentDefinitions, freeLimits.MaxAgentDefinitions)
	}
}
