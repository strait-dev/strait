package billing

import (
	"context"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWelcomeEmailHTML_EscapesHTMLInPlanName(t *testing.T) {
	t.Parallel()
	output := welcomeEmailHTML("<script>alert(1)</script>", "$49.99")
	require.NotContains(t,
		output, "<script>")
	require.Contains(t, output, "&lt;script&gt;")
}

func TestWelcomeEmailHTML_EscapesHTMLInCredit(t *testing.T) {
	t.Parallel()
	injection := "<img src=x onerror=alert(1)>"
	output := welcomeEmailHTML("Pro", injection)
	require.NotContains(t,
		output, injection)
	require.Contains(t, output, "&lt;img src=x onerror=alert(1)&gt;")

	// The raw injection string should not appear unescaped.
	// html.EscapeString turns "<" and ">" into "&lt;" and "&gt;".
}

func TestWelcomeEmailHTML_NormalValues(t *testing.T) {
	t.Parallel()
	output := welcomeEmailHTML("Pro", "1000000")
	require.Contains(t, output, "Welcome to Strait Pro!")
	require.Contains(t, output, "1000000")
}

func TestWelcomeEmailHTML_ContainsStructure(t *testing.T) {
	t.Parallel()
	output := welcomeEmailHTML("Starter", "$19.99")
	require.Contains(t, output, "Set spending limit")
	require.Contains(t, output, "support@strait.dev")
	require.Contains(t, output, "billing")
}

func FuzzWelcomeEmailHTML(f *testing.F) {
	f.Add("Pro", "$49.99")
	f.Add("<script>", "<img>")
	f.Add("", "")
	f.Add("Plan&Name", "$0.00")

	f.Fuzz(func(t *testing.T, planName, credit string) {
		result := welcomeEmailHTML(planName, credit)
		assert.NotContains(t, result, "<script>")
	})
}

func TestEnterpriseWelcomeEmailHTML_ContainsCSM(t *testing.T) {
	t.Parallel()
	output := enterpriseWelcomeEmailHTML()
	require.Contains(t, output, "Customer Success Manager")
}

func TestEnterpriseWelcomeEmailHTML_ContainsOnboarding(t *testing.T) {
	t.Parallel()
	output := enterpriseWelcomeEmailHTML()
	require.Contains(t, output, "onboarding")
}

func TestEnterpriseWelcomeEmailHTML_MarksSSOAsRoadmap(t *testing.T) {
	t.Parallel()
	output := enterpriseWelcomeEmailHTML()
	require.Contains(t, output, "Roadmap and contact-sales items such as SSO/SAML")
	require.NotContains(t,
		output, "SSO/SAML + SCIM")
}

func TestEnterpriseWelcomeEmailHTML_DoesNotPromiseNetworkControls(t *testing.T) {
	t.Parallel()
	output := enterpriseWelcomeEmailHTML()
	require.NotContains(t,
		output, "Static IPs, VPC peering, data residency")
}

func TestEnterpriseWelcomeEmailHTML_ContainsSLA(t *testing.T) {
	t.Parallel()
	output := enterpriseWelcomeEmailHTML()
	require.Contains(t, output, "SLA")
}

func TestEnterpriseWelcomeEmailHTML_DoesNotPromiseDedicatedCompute(t *testing.T) {
	t.Parallel()
	output := enterpriseWelcomeEmailHTML()
	require.NotContains(t,
		output, "Dedicated compute")
}

func TestRunAllowanceDisplay_Enterprise(t *testing.T) {
	t.Parallel()
	got := runAllowanceDisplay("enterprise")
	assert.Equal(t, "Custom (per contract)",

		got)
}

func TestRunAllowanceDisplay_Starter(t *testing.T) {
	t.Parallel()
	got := runAllowanceDisplay("starter")
	assert.Equal(t, "50000",
		got,
	)
}

func TestContractRenewalHTML_ContainsDate(t *testing.T) {
	t.Parallel()
	output := contractRenewalHTML("April 1, 2027", 30)
	require.Contains(t, output, "April 1, 2027")
	require.Contains(t, output, "auto-renew")
}

func TestContractExpiryHTML_ContainsDate(t *testing.T) {
	t.Parallel()
	output := contractExpiryHTML("April 1, 2027", 7)
	require.Contains(t, output, "April 1, 2027")
	require.Contains(t, output, "expires")
	require.Contains(t, output, "Scale")
}

func TestRunAllowanceDisplay_AllTiers(t *testing.T) {
	t.Parallel()
	tests := []struct {
		tier string
		want string
	}{
		{"free", "5000"},
		{"starter", "50000"},
		{"pro", "1000000"},
		{"scale", "5000000"},
		{"business", "25000000"},
		{"enterprise", "Custom (per contract)"},
		{"unknown", "5000"},
	}
	for _, tt := range tests {
		t.Run(tt.tier, func(t *testing.T) {
			t.Parallel()
			got := runAllowanceDisplay(domain.PlanTier(tt.tier))
			assert.Equal(t, tt.
				want, got,
			)
		})
	}
}

func TestPlanDisplayName_AllTiers(t *testing.T) {
	t.Parallel()
	tests := []struct {
		tier string
		want string
	}{
		{"free", "Free"},
		{"starter", "Starter"},
		{"pro", "Pro"},
		{"scale", "Scale"},
		{"enterprise", "Enterprise"},
		{"unknown", "Free"},
	}
	for _, tt := range tests {
		t.Run(tt.tier, func(t *testing.T) {
			t.Parallel()
			got := planDisplayName(domain.PlanTier(tt.tier))
			assert.Equal(t, tt.
				want, got,
			)
		})
	}
}

func TestNewResendWelcomeEmailFunc_InvalidEmail(t *testing.T) {
	t.Parallel()
	fn := NewResendWelcomeEmailFunc("re_test_key", "")
	err := fn(context.Background(), "org-1", domain.PlanStarter, "not-an-email")
	require.Error(t,
		err)
}

func TestNewResendWelcomeEmailFunc_DefaultFromEmail(t *testing.T) {
	t.Parallel()
	_ = NewResendWelcomeEmailFunc("re_test_key", "")
	// Just verifying no panic with empty fromEmail (defaults to "noreply@strait.dev").
}
