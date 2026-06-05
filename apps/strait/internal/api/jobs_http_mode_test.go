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

// mockHTTPModeEnforcer implements BillingEnforcer with configurable plan limits.
type mockHTTPModeEnforcer struct {
	mockBillingEnforcer
	planLimits billing.OrgPlanLimits
	orgErr     error
	limitsErr  error
}

func (m *mockHTTPModeEnforcer) GetProjectOrgID(ctx context.Context, projectID string) (string, error) {
	if m.orgErr != nil {
		return "", m.orgErr
	}
	return m.mockBillingEnforcer.GetProjectOrgID(ctx, projectID)
}

func (m *mockHTTPModeEnforcer) GetOrgPlanLimits(_ context.Context, _ string) (billing.OrgPlanLimits, error) {
	if m.limitsErr != nil {
		return billing.OrgPlanLimits{}, m.limitsErr
	}
	return m.planLimits, nil
}

func TestCheckHTTPModeAllowed_FreePlanAllowed(t *testing.T) {
	t.Parallel()

	// HTTP mode is available on all plans including free.
	enforcer := &mockHTTPModeEnforcer{
		mockBillingEnforcer: mockBillingEnforcer{
			projectOrgMap: map[string]string{"proj-1": "org-1"},
		},
		planLimits: billing.GetPlanLimits(domain.PlanFree),
	}

	s := &Server{
		edition:         domain.EditionCloud,
		billingEnforcer: enforcer,
	}

	err := s.checkHTTPModeAllowed(context.Background(), domain.ExecutionModeHTTP, "proj-1")
	require.NoError(t, err)

}

func TestCheckHTTPModeAllowed_StarterPlanAllowed(t *testing.T) {
	t.Parallel()

	// HTTP mode is available on all plans including starter.
	enforcer := &mockHTTPModeEnforcer{
		mockBillingEnforcer: mockBillingEnforcer{
			projectOrgMap: map[string]string{"proj-1": "org-1"},
		},
		planLimits: billing.GetPlanLimits(domain.PlanStarter),
	}

	s := &Server{
		edition:         domain.EditionCloud,
		billingEnforcer: enforcer,
	}

	err := s.checkHTTPModeAllowed(context.Background(), domain.ExecutionModeHTTP, "proj-1")
	require.NoError(t, err)

}

func TestCheckHTTPModeAllowed_ProPlanAllowed(t *testing.T) {
	t.Parallel()

	enforcer := &mockHTTPModeEnforcer{
		mockBillingEnforcer: mockBillingEnforcer{
			projectOrgMap: map[string]string{"proj-1": "org-1"},
		},
		planLimits: billing.GetPlanLimits(domain.PlanPro),
	}

	s := &Server{
		edition:         domain.EditionCloud,
		billingEnforcer: enforcer,
	}

	err := s.checkHTTPModeAllowed(context.Background(), domain.ExecutionModeHTTP, "proj-1")
	require.NoError(t, err)

}

func TestCheckHTTPModeAllowed_CommunityEditionAllowed(t *testing.T) {
	t.Parallel()

	// Community edition should not gate HTTP mode regardless of plan.
	s := &Server{
		edition: domain.EditionCommunity,
	}

	err := s.checkHTTPModeAllowed(context.Background(), domain.ExecutionModeHTTP, "proj-1")
	require.NoError(t, err)

}

func TestCheckHTTPModeAllowed_WorkerModeSkipped(t *testing.T) {
	t.Parallel()

	// Worker mode should always pass (not gated).
	s := &Server{
		edition: domain.EditionCloud,
	}

	err := s.checkHTTPModeAllowed(context.Background(), domain.ExecutionModeWorker, "proj-1")
	require.NoError(t, err)

}

func TestCheckHTTPModeAllowed_CloudNilEnforcerFailsClosed(t *testing.T) {
	t.Parallel()

	s := &Server{
		edition:         domain.EditionCloud,
		billingEnforcer: nil,
	}

	err := s.checkHTTPModeAllowed(context.Background(), domain.ExecutionModeHTTP, "proj-1")
	require.Error(t, err)
	assert.Contains(t, err.
		Error(), "billing enforcement unavailable",
	)

}

func TestCheckHTTPModeAllowed_CommunityNilEnforcerAllowed(t *testing.T) {
	t.Parallel()

	s := &Server{
		edition:         domain.EditionCommunity,
		billingEnforcer: nil,
	}

	err := s.checkHTTPModeAllowed(context.Background(), domain.ExecutionModeHTTP, "proj-1")
	require.NoError(t, err)

}

func TestCheckHTTPModeAllowed_OrgLookupErrorFailsClosed(t *testing.T) {
	t.Parallel()

	enforcer := &mockHTTPModeEnforcer{
		mockBillingEnforcer: mockBillingEnforcer{
			projectOrgMap: map[string]string{"proj-1": "org-1"},
		},
		orgErr: errors.New("org lookup unavailable"),
	}
	s := &Server{
		edition:         domain.EditionCloud,
		billingEnforcer: enforcer,
	}

	err := s.checkHTTPModeAllowed(context.Background(), domain.ExecutionModeHTTP, "proj-1")
	require.Error(t, err)
	require.True(
		t, strings.Contains(err.
			Error(),

			"billing enforcement unavailable",
		))

}

func TestCheckHTTPModeAllowed_PlanLookupErrorFailsClosed(t *testing.T) {
	t.Parallel()

	enforcer := &mockHTTPModeEnforcer{
		mockBillingEnforcer: mockBillingEnforcer{
			projectOrgMap: map[string]string{"proj-1": "org-1"},
		},
		limitsErr: errors.New("plan lookup unavailable"),
	}
	s := &Server{
		edition:         domain.EditionCloud,
		billingEnforcer: enforcer,
	}

	err := s.checkHTTPModeAllowed(context.Background(), domain.ExecutionModeHTTP, "proj-1")
	require.Error(t, err)
	require.True(
		t, strings.Contains(err.
			Error(),

			"billing enforcement unavailable",
		))

}

func TestCheckHTTPModeAllowed_EnterprisePlanAllowed(t *testing.T) {
	t.Parallel()

	enforcer := &mockHTTPModeEnforcer{
		mockBillingEnforcer: mockBillingEnforcer{
			projectOrgMap: map[string]string{"proj-1": "org-1"},
		},
		planLimits: billing.GetPlanLimits(domain.PlanEnterprise),
	}

	s := &Server{
		edition:         domain.EditionCloud,
		billingEnforcer: enforcer,
	}

	err := s.checkHTTPModeAllowed(context.Background(), domain.ExecutionModeHTTP, "proj-1")
	require.NoError(t, err)

}

func TestCheckHTTPModeAllowed_UnavailablePlanDoesNotAdvertiseUpgrade(t *testing.T) {
	t.Parallel()

	limits := billing.GetPlanLimits(domain.PlanFree)
	limits.AllowsHTTPMode = false
	enforcer := &mockHTTPModeEnforcer{
		mockBillingEnforcer: mockBillingEnforcer{
			projectOrgMap: map[string]string{"proj-1": "org-1"},
		},
		planLimits: limits,
	}

	s := &Server{
		edition:         domain.EditionCloud,
		billingEnforcer: enforcer,
	}

	err := s.checkHTTPModeAllowed(context.Background(), domain.ExecutionModeHTTP, "proj-1")
	require.Error(t, err)

	msg := err.Error()
	for _, forbidden := range []string{"Pro plan", "$49.99", "Upgrade"} {
		require.False(t, strings.Contains(
			msg, forbidden,
		))

	}
}
