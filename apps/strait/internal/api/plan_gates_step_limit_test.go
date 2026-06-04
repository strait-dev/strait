package api

import (
	"context"
	"strings"
	"testing"

	"strait/internal/billing"
	"strait/internal/domain"
)

// stepLimitCase exercises checkWorkflowStepLimit at and around the plan's
// MaxWorkflowDAGSteps boundary. limit == -1 means the plan is unlimited.
type stepLimitCase struct {
	name      string
	tier      domain.PlanTier
	limit     int
	stepCount int
	wantErr   bool
}

func newStepLimitServer(tier domain.PlanTier) *Server {
	enforcer := &mockHTTPModeEnforcer{
		mockBillingEnforcer: mockBillingEnforcer{
			projectOrgMap: map[string]string{"proj-1": "org-1"},
		},
		planLimits: billing.GetPlanLimits(tier),
	}
	return &Server{
		edition:         domain.EditionCloud,
		billingEnforcer: enforcer,
	}
}

func TestCheckWorkflowStepLimit_TierBoundaries(t *testing.T) {
	t.Parallel()

	cases := []stepLimitCase{
		{name: "free at limit", tier: domain.PlanFree, limit: billing.MaxDAGStepsFree, stepCount: billing.MaxDAGStepsFree, wantErr: false},
		{name: "free over limit", tier: domain.PlanFree, limit: billing.MaxDAGStepsFree, stepCount: billing.MaxDAGStepsFree + 1, wantErr: true},
		{name: "free single step", tier: domain.PlanFree, limit: billing.MaxDAGStepsFree, stepCount: 1, wantErr: false},
		{name: "free zero steps", tier: domain.PlanFree, limit: billing.MaxDAGStepsFree, stepCount: 0, wantErr: false},

		{name: "starter at limit", tier: domain.PlanStarter, limit: billing.MaxDAGStepsStarter, stepCount: billing.MaxDAGStepsStarter, wantErr: false},
		{name: "starter over limit", tier: domain.PlanStarter, limit: billing.MaxDAGStepsStarter, stepCount: billing.MaxDAGStepsStarter + 1, wantErr: true},
		{name: "starter accepts free count", tier: domain.PlanStarter, limit: billing.MaxDAGStepsStarter, stepCount: billing.MaxDAGStepsFree + 1, wantErr: false},

		{name: "pro at limit", tier: domain.PlanPro, limit: billing.MaxDAGStepsPro, stepCount: billing.MaxDAGStepsPro, wantErr: false},
		{name: "pro over limit", tier: domain.PlanPro, limit: billing.MaxDAGStepsPro, stepCount: billing.MaxDAGStepsPro + 1, wantErr: true},
		{name: "pro accepts starter count", tier: domain.PlanPro, limit: billing.MaxDAGStepsPro, stepCount: billing.MaxDAGStepsStarter + 1, wantErr: false},

		{name: "scale at limit", tier: domain.PlanScale, limit: billing.MaxDAGStepsScale, stepCount: billing.MaxDAGStepsScale, wantErr: false},
		{name: "scale over limit", tier: domain.PlanScale, limit: billing.MaxDAGStepsScale, stepCount: billing.MaxDAGStepsScale + 1, wantErr: true},
		{name: "scale accepts pro count", tier: domain.PlanScale, limit: billing.MaxDAGStepsScale, stepCount: billing.MaxDAGStepsPro + 1, wantErr: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := newStepLimitServer(tc.tier)

			gotLimits := billing.GetPlanLimits(tc.tier)
			if gotLimits.MaxWorkflowDAGSteps != tc.limit {
				t.Fatalf("plan %q: expected MaxWorkflowDAGSteps=%d, got %d (catalog drift)", tc.tier, tc.limit, gotLimits.MaxWorkflowDAGSteps)
			}

			err := s.checkWorkflowStepLimit(context.Background(), "proj-1", tc.stepCount)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %d steps on %s plan, got nil", tc.stepCount, tc.tier)
				}
				return
			}
			if err != nil {
				t.Fatalf("expected no error for %d steps on %s plan (limit %d), got: %v", tc.stepCount, tc.tier, tc.limit, err)
			}
		})
	}
}

func TestCheckWorkflowStepLimit_UnlimitedTiers(t *testing.T) {
	t.Parallel()

	// Business and Enterprise carry MaxWorkflowDAGSteps = -1 (unlimited).
	for _, tier := range []domain.PlanTier{domain.PlanBusiness, domain.PlanEnterprise} {
		t.Run(string(tier), func(t *testing.T) {
			t.Parallel()

			limits := billing.GetPlanLimits(tier)
			if limits.MaxWorkflowDAGSteps != -1 {
				t.Fatalf("plan %q: expected MaxWorkflowDAGSteps=-1 (unlimited), got %d", tier, limits.MaxWorkflowDAGSteps)
			}

			s := newStepLimitServer(tier)

			// A step count well above any tiered limit must be accepted.
			if err := s.checkWorkflowStepLimit(context.Background(), "proj-1", billing.MaxDAGStepsScale*100); err != nil {
				t.Fatalf("expected no error for unlimited %s plan, got: %v", tier, err)
			}
		})
	}
}

func TestCheckWorkflowStepLimit_CloudNilEnforcerFailsClosed(t *testing.T) {
	t.Parallel()

	s := &Server{
		edition:         domain.EditionCloud,
		billingEnforcer: nil,
	}

	if err := s.checkWorkflowStepLimit(context.Background(), "proj-1", 1_000_000); err != nil {
		if strings.Contains(err.Error(), "billing enforcement unavailable") {
			return
		}
		t.Fatalf("expected billing enforcement unavailable, got: %v", err)
	}
	t.Fatal("expected cloud nil enforcer to fail closed")
}

func TestCheckWorkflowStepLimit_CloudEmptyOrgFailsClosed(t *testing.T) {
	t.Parallel()

	s := &Server{
		edition: domain.EditionCloud,
		billingEnforcer: &mockHTTPModeEnforcer{
			mockBillingEnforcer: mockBillingEnforcer{},
			planLimits:          billing.GetPlanLimits(domain.PlanFree),
		},
	}

	err := s.checkWorkflowStepLimit(context.Background(), "proj-1", 1_000_000)
	if err == nil {
		t.Fatal("expected cloud empty org lookup to fail closed")
	}
	if !strings.Contains(err.Error(), "billing enforcement unavailable") {
		t.Fatalf("expected billing enforcement unavailable, got: %v", err)
	}
}

func TestCheckWorkflowStepLimit_CommunityNilEnforcerFailsOpen(t *testing.T) {
	t.Parallel()

	s := &Server{
		edition:         domain.EditionCommunity,
		billingEnforcer: nil,
	}

	if err := s.checkWorkflowStepLimit(context.Background(), "proj-1", 1_000_000); err != nil {
		t.Fatalf("expected community nil enforcer to fail open, got: %v", err)
	}
}

func TestCheckWorkflowStepLimit_CommunityEditionFailsOpen(t *testing.T) {
	t.Parallel()

	// Community edition does not gate plans. Even with a configured enforcer
	// the limit must not apply -- self-hosted users do not have a plan.
	s := &Server{
		edition: domain.EditionCommunity,
		billingEnforcer: &mockHTTPModeEnforcer{
			mockBillingEnforcer: mockBillingEnforcer{
				projectOrgMap: map[string]string{"proj-1": "org-1"},
			},
			planLimits: billing.GetPlanLimits(domain.PlanFree),
		},
	}

	// Far above the free-plan cap; community edition must allow it.
	if err := s.checkWorkflowStepLimit(context.Background(), "proj-1", billing.MaxDAGStepsFree*1000); err != nil {
		t.Fatalf("expected community edition to fail open, got: %v", err)
	}
}

func TestCheckWorkflowStepLimit_ErrorMessageMentionsPlanAndCounts(t *testing.T) {
	t.Parallel()

	// The 400 message is user-facing; it must surface the plan name, the
	// allowed limit, and the requested count so the operator can see exactly
	// what to fix without inspecting logs.
	s := newStepLimitServer(domain.PlanFree)

	requested := billing.MaxDAGStepsFree + 7
	err := s.checkWorkflowStepLimit(context.Background(), "proj-1", requested)
	if err == nil {
		t.Fatalf("expected error for over-limit step count, got nil")
	}

	limits := billing.GetPlanLimits(domain.PlanFree)
	msg := err.Error()
	if !strings.Contains(msg, limits.DisplayName) {
		t.Errorf("expected error to contain plan name %q, got: %s", limits.DisplayName, msg)
	}
	if !strings.Contains(msg, "/settings/billing") {
		t.Errorf("expected error to point to /settings/billing, got: %s", msg)
	}
}
