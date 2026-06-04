package api

import (
	"context"
	"strings"
	"testing"

	"strait/internal/billing"
	"strait/internal/domain"
)

// cronIntervalCase walks a single (tier, cron expression) pair through
// checkCronMinInterval. wantErr captures whether the schedule must be
// rejected on that tier; the helper plan_limits table on each plan defines
// CronMinIntervalSec which the gate compares against.
type cronIntervalCase struct {
	name     string
	tier     domain.PlanTier
	cronExpr string
	wantErr  bool
}

func newCronIntervalServer(tier domain.PlanTier) *Server {
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

func TestCheckCronMinInterval_FreeRejectsEveryMinute(t *testing.T) {
	t.Parallel()

	cases := []cronIntervalCase{
		// Free plan = 300s minimum.
		{name: "free rejects every minute", tier: domain.PlanFree, cronExpr: "* * * * *", wantErr: true},
		{name: "free rejects every 2 minutes", tier: domain.PlanFree, cronExpr: "*/2 * * * *", wantErr: true},
		{name: "free rejects every 4 minutes", tier: domain.PlanFree, cronExpr: "*/4 * * * *", wantErr: true},
		{name: "free accepts every 5 minutes", tier: domain.PlanFree, cronExpr: "*/5 * * * *", wantErr: false},
		{name: "free accepts every 15 minutes", tier: domain.PlanFree, cronExpr: "*/15 * * * *", wantErr: false},
		{name: "free accepts hourly", tier: domain.PlanFree, cronExpr: "0 * * * *", wantErr: false},
		{name: "free accepts daily", tier: domain.PlanFree, cronExpr: "0 9 * * *", wantErr: false},

		// Starter plan = 60s minimum. Standard 5-field cron cannot express
		// sub-minute schedules, so every-minute is the floor and must pass.
		{name: "starter accepts every minute", tier: domain.PlanStarter, cronExpr: "* * * * *", wantErr: false},
		{name: "starter accepts every 5 minutes", tier: domain.PlanStarter, cronExpr: "*/5 * * * *", wantErr: false},

		// Pro plan = 30s minimum. Same reasoning as Starter -- the format
		// caps at 60s granularity, so every Pro schedule is allowed.
		{name: "pro accepts every minute", tier: domain.PlanPro, cronExpr: "* * * * *", wantErr: false},

		// Variable-gap schedule: smallest gap is Friday->Monday weekend gap
		// (3 days), well above any plan minimum.
		{name: "free accepts mon/fri 9am", tier: domain.PlanFree, cronExpr: "0 9 * * MON,FRI", wantErr: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := newCronIntervalServer(tc.tier)
			err := s.checkCronMinInterval(context.Background(), "proj-1", tc.cronExpr)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q on %s plan, got nil", tc.cronExpr, tc.tier)
				}
				return
			}
			if err != nil {
				t.Fatalf("expected no error for %q on %s plan, got: %v", tc.cronExpr, tc.tier, err)
			}
		})
	}
}

func TestCheckCronMinInterval_UnlimitedTiersAcceptAnyCron(t *testing.T) {
	t.Parallel()

	// Scale (1s), Business (0s), Enterprise (0s) carry minimums that the
	// 5-field cron parser cannot violate. Every well-formed schedule passes.
	for _, tier := range []domain.PlanTier{domain.PlanScale, domain.PlanBusiness, domain.PlanEnterprise} {
		t.Run(string(tier), func(t *testing.T) {
			t.Parallel()

			s := newCronIntervalServer(tier)
			for _, expr := range []string{"* * * * *", "*/5 * * * *", "0 9 * * *", "0 9 * * MON,FRI"} {
				if err := s.checkCronMinInterval(context.Background(), "proj-1", expr); err != nil {
					t.Errorf("expected %s plan to accept %q, got: %v", tier, expr, err)
				}
			}
		})
	}
}

func TestCheckCronMinInterval_EmptyCronIsNoop(t *testing.T) {
	t.Parallel()

	// Job records may carry an empty cron (one-shot or webhook-only); the
	// gate must treat that as nothing to check.
	s := newCronIntervalServer(domain.PlanFree)
	if err := s.checkCronMinInterval(context.Background(), "proj-1", ""); err != nil {
		t.Fatalf("expected empty cron to be a no-op, got: %v", err)
	}
}

func TestCheckCronMinInterval_CloudNilEnforcerFailsClosed(t *testing.T) {
	t.Parallel()

	s := &Server{
		edition:         domain.EditionCloud,
		billingEnforcer: nil,
	}
	err := s.checkCronMinInterval(context.Background(), "proj-1", "* * * * *")
	if err == nil || !strings.Contains(err.Error(), "billing enforcement unavailable") {
		t.Fatalf("expected billing enforcement unavailable, got: %v", err)
	}
}

func TestCheckCronMinInterval_CommunityNilEnforcerFailsOpen(t *testing.T) {
	t.Parallel()

	s := &Server{
		edition:         domain.EditionCommunity,
		billingEnforcer: nil,
	}
	if err := s.checkCronMinInterval(context.Background(), "proj-1", "* * * * *"); err != nil {
		t.Fatalf("expected community nil enforcer to fail open, got: %v", err)
	}
}

func TestCheckCronMinInterval_CommunityEditionFailsOpen(t *testing.T) {
	t.Parallel()

	// Self-hosted users do not carry a plan; the gate must allow any
	// expression even when an enforcer is wired up by accident.
	s := &Server{
		edition: domain.EditionCommunity,
		billingEnforcer: &mockHTTPModeEnforcer{
			mockBillingEnforcer: mockBillingEnforcer{
				projectOrgMap: map[string]string{"proj-1": "org-1"},
			},
			planLimits: billing.GetPlanLimits(domain.PlanFree),
		},
	}
	if err := s.checkCronMinInterval(context.Background(), "proj-1", "* * * * *"); err != nil {
		t.Fatalf("expected community edition to fail open, got: %v", err)
	}
}

func TestCheckCronMinInterval_MalformedCronFailsOpen(t *testing.T) {
	t.Parallel()

	// Request-level validation (validateCreateJobCronFields) rejects malformed
	// expressions earlier in the pipeline. If one slips past, the gate must
	// not panic or block -- the prior layer is the source of truth.
	s := newCronIntervalServer(domain.PlanFree)
	for _, expr := range []string{"not a cron", "* * *", "@@@", "60 * * * *"} {
		if err := s.checkCronMinInterval(context.Background(), "proj-1", expr); err != nil {
			t.Errorf("expected gate to fail open for malformed %q, got: %v", expr, err)
		}
	}
}

func TestCheckCronMinInterval_ErrorMessageIsActionable(t *testing.T) {
	t.Parallel()

	// The 400 surfaces to the operator -- it must include the plan name, the
	// required minimum, and the requested gap so they know which knob to turn.
	s := newCronIntervalServer(domain.PlanFree)
	err := s.checkCronMinInterval(context.Background(), "proj-1", "* * * * *")
	if err == nil {
		t.Fatalf("expected error for sub-minimum schedule, got nil")
	}

	limits := billing.GetPlanLimits(domain.PlanFree)
	msg := err.Error()
	if !strings.Contains(msg, limits.DisplayName) {
		t.Errorf("expected message to include plan name %q, got: %s", limits.DisplayName, msg)
	}
	if !strings.Contains(msg, "/settings/billing") {
		t.Errorf("expected message to point to /settings/billing, got: %s", msg)
	}
}
