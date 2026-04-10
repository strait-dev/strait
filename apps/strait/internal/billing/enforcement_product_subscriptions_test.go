package billing

import (
	"context"
	"testing"

	"strait/internal/domain"
)

func TestEnforcer_GetJobsPlanForProject_ReturnsPlanFromOrgSubscription(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)

	store.projectOrgMap = map[string]string{"proj-1": "org-1"}
	store.subscriptions = map[string]*OrgSubscription{
		"org-1": {OrgID: "org-1", PlanTier: string(domain.PlanPro), Status: "active"},
	}

	tier, err := enforcer.GetJobsPlanForProject(context.Background(), "proj-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tier != domain.PlanPro {
		t.Fatalf("expected PlanPro, got %q", tier)
	}
}

func TestEnforcer_GetJobsPlanForProject_FailsOpenToFree(t *testing.T) {
	t.Parallel()
	enforcer, _, _ := setupEnforcer(t)

	// No org mapping or subscription — should fail open to free.
	tier, err := enforcer.GetJobsPlanForProject(context.Background(), "proj-unknown")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tier != domain.PlanFree {
		t.Fatalf("expected PlanFree on unknown project, got %q", tier)
	}
}

func TestEnforcer_GetJobsPlanForProject_EmptyProjectID(t *testing.T) {
	t.Parallel()
	enforcer, _, _ := setupEnforcer(t)
	tier, err := enforcer.GetJobsPlanForProject(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tier != domain.PlanFree {
		t.Fatalf("expected PlanFree for empty projectID, got %q", tier)
	}
}

func TestEnforcer_GetJobsPlanForProject_EmptyTierCoercedToFree(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)

	store.projectOrgMap = map[string]string{"proj-1": "org-1"}
	store.subscriptions = map[string]*OrgSubscription{
		"org-1": {OrgID: "org-1", PlanTier: "", Status: "active"},
	}

	tier, err := enforcer.GetJobsPlanForProject(context.Background(), "proj-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tier != domain.PlanFree {
		t.Fatalf("expected PlanFree for empty tier, got %q", tier)
	}
}

func TestEnforcer_HasJobsSubscription(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)

	store.subscriptions = map[string]*OrgSubscription{
		"org-free":  {OrgID: "org-free", PlanTier: string(domain.PlanFree), Status: "active"},
		"org-empty": {OrgID: "org-empty", PlanTier: "", Status: "active"},
		"org-paid":  {OrgID: "org-paid", PlanTier: string(domain.PlanStarter), Status: "active"},
		"org-scale": {OrgID: "org-scale", PlanTier: string(domain.PlanScale), Status: "active"},
		"org-agent": {OrgID: "org-agent", PlanTier: string(domain.PlanFree), AgentPlanTier: string(domain.AgentPlanGrowth), Status: "active"},
	}

	cases := []struct {
		orgID string
		want  bool
	}{
		{"org-free", false},
		{"org-empty", false},
		{"org-paid", true},
		{"org-scale", true},
		{"org-agent", false}, // paid agents but no jobs subscription
		{"org-missing", false},
		{"", false},
	}
	for _, tc := range cases {
		got := enforcer.HasJobsSubscription(context.Background(), tc.orgID)
		if got != tc.want {
			t.Errorf("HasJobsSubscription(%q) = %v, want %v", tc.orgID, got, tc.want)
		}
	}
}

func TestEnforcer_HasAgentsSubscription(t *testing.T) {
	t.Parallel()
	enforcer, store, _ := setupEnforcer(t)

	store.subscriptions = map[string]*OrgSubscription{
		"org-agent-free":   {OrgID: "org-agent-free", AgentPlanTier: string(domain.AgentPlanFree), Status: "active"},
		"org-agent-empty":  {OrgID: "org-agent-empty", AgentPlanTier: "", Status: "active"},
		"org-agent-paid":   {OrgID: "org-agent-paid", AgentPlanTier: string(domain.AgentPlanMaker), Status: "active"},
		"org-agent-growth": {OrgID: "org-agent-growth", AgentPlanTier: string(domain.AgentPlanGrowth), Status: "active"},
		"org-jobs-only":    {OrgID: "org-jobs-only", PlanTier: string(domain.PlanPro), AgentPlanTier: string(domain.AgentPlanFree), Status: "active"},
	}

	cases := []struct {
		orgID string
		want  bool
	}{
		{"org-agent-free", false},
		{"org-agent-empty", false},
		{"org-agent-paid", true},
		{"org-agent-growth", true},
		{"org-jobs-only", false}, // paid jobs but no agents subscription
		{"org-missing", false},
		{"", false},
	}
	for _, tc := range cases {
		got := enforcer.HasAgentsSubscription(context.Background(), tc.orgID)
		if got != tc.want {
			t.Errorf("HasAgentsSubscription(%q) = %v, want %v", tc.orgID, got, tc.want)
		}
	}
}

func TestEnforcer_HasJobsAndAgentsSubscription_Independent(t *testing.T) {
	t.Parallel()
	// A customer can hold either, both, or neither subscription independently.
	enforcer, store, _ := setupEnforcer(t)

	store.subscriptions = map[string]*OrgSubscription{
		"neither": {OrgID: "neither", PlanTier: string(domain.PlanFree), AgentPlanTier: string(domain.AgentPlanFree)},
		"jobs":    {OrgID: "jobs", PlanTier: string(domain.PlanPro), AgentPlanTier: string(domain.AgentPlanFree)},
		"agents":  {OrgID: "agents", PlanTier: string(domain.PlanFree), AgentPlanTier: string(domain.AgentPlanMaker)},
		"both":    {OrgID: "both", PlanTier: string(domain.PlanScale), AgentPlanTier: string(domain.AgentPlanGrowth)},
	}

	cases := []struct {
		orgID    string
		wantJobs bool
		wantAgts bool
	}{
		{"neither", false, false},
		{"jobs", true, false},
		{"agents", false, true},
		{"both", true, true},
	}
	ctx := context.Background()
	for _, tc := range cases {
		if got := enforcer.HasJobsSubscription(ctx, tc.orgID); got != tc.wantJobs {
			t.Errorf("%s: HasJobsSubscription = %v, want %v", tc.orgID, got, tc.wantJobs)
		}
		if got := enforcer.HasAgentsSubscription(ctx, tc.orgID); got != tc.wantAgts {
			t.Errorf("%s: HasAgentsSubscription = %v, want %v", tc.orgID, got, tc.wantAgts)
		}
	}
}
