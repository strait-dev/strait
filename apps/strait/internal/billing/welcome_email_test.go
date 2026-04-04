package billing

import (
	"strings"
	"testing"
)

func TestWelcomeEmailHTML_EscapesHTMLInPlanName(t *testing.T) {
	t.Parallel()
	output := welcomeEmailHTML("<script>alert(1)</script>", "$49.99")
	if strings.Contains(output, "<script>") {
		t.Fatal("HTML injection not escaped in plan name")
	}
	if !strings.Contains(output, "&lt;script&gt;") {
		t.Fatal("expected escaped script tag")
	}
}

func TestWelcomeEmailHTML_EscapesHTMLInCredit(t *testing.T) {
	t.Parallel()
	injection := "<img src=x onerror=alert(1)>"
	output := welcomeEmailHTML("Pro", injection)
	// The raw injection string should not appear unescaped.
	// html.EscapeString turns "<" and ">" into "&lt;" and "&gt;".
	if strings.Contains(output, injection) {
		t.Fatal("HTML injection not escaped in credit")
	}
	if !strings.Contains(output, "&lt;img src=x onerror=alert(1)&gt;") {
		t.Fatal("expected escaped img tag in credit")
	}
}

func TestWelcomeEmailHTML_NormalValues(t *testing.T) {
	t.Parallel()
	output := welcomeEmailHTML("Pro", "$49.99")
	if !strings.Contains(output, "Welcome to Strait Pro!") {
		t.Fatal("expected plan name in output")
	}
	if !strings.Contains(output, "$49.99") {
		t.Fatal("expected credit amount in output")
	}
}

func TestWelcomeEmailHTML_ContainsStructure(t *testing.T) {
	t.Parallel()
	output := welcomeEmailHTML("Starter", "$19.99")
	if !strings.Contains(output, "Set spending limit") {
		t.Fatal("expected spending limit CTA")
	}
	if !strings.Contains(output, "support@strait.dev") {
		t.Fatal("expected support email")
	}
	if !strings.Contains(output, "billing") {
		t.Fatal("expected billing link")
	}
}

func FuzzWelcomeEmailHTML(f *testing.F) {
	f.Add("Pro", "$49.99")
	f.Add("<script>", "<img>")
	f.Add("", "")
	f.Add("Plan&Name", "$0.00")

	f.Fuzz(func(t *testing.T, planName, credit string) {
		result := welcomeEmailHTML(planName, credit)
		if strings.Contains(result, "<script>") {
			t.Error("unescaped script tag in output")
		}
	})
}

func TestEnterpriseWelcomeEmailHTML_ContainsCSM(t *testing.T) {
	t.Parallel()
	output := enterpriseWelcomeEmailHTML()
	if !strings.Contains(output, "Customer Success Manager") {
		t.Fatal("enterprise welcome email should mention CSM")
	}
}

func TestEnterpriseWelcomeEmailHTML_ContainsOnboarding(t *testing.T) {
	t.Parallel()
	output := enterpriseWelcomeEmailHTML()
	if !strings.Contains(output, "onboarding") {
		t.Fatal("enterprise welcome email should mention onboarding")
	}
}

func TestEnterpriseWelcomeEmailHTML_ContainsSSO(t *testing.T) {
	t.Parallel()
	output := enterpriseWelcomeEmailHTML()
	if !strings.Contains(output, "SSO") {
		t.Fatal("enterprise welcome email should mention SSO")
	}
}

func TestEnterpriseWelcomeEmailHTML_ContainsSCIM(t *testing.T) {
	t.Parallel()
	output := enterpriseWelcomeEmailHTML()
	if !strings.Contains(output, "SCIM") {
		t.Fatal("enterprise welcome email should mention SCIM")
	}
}

func TestEnterpriseWelcomeEmailHTML_ContainsSLA(t *testing.T) {
	t.Parallel()
	output := enterpriseWelcomeEmailHTML()
	if !strings.Contains(output, "SLA") {
		t.Fatal("enterprise welcome email should mention SLA")
	}
}

func TestEnterpriseWelcomeEmailHTML_ContainsDedicatedCompute(t *testing.T) {
	t.Parallel()
	output := enterpriseWelcomeEmailHTML()
	if !strings.Contains(output, "Dedicated compute") {
		t.Fatal("enterprise welcome email should mention dedicated compute")
	}
}

func TestCreditDisplayUSD_Enterprise(t *testing.T) {
	t.Parallel()
	got := creditDisplayUSD("enterprise")
	if got != "Custom (per contract)" {
		t.Errorf("creditDisplayUSD(enterprise) = %q, want %q", got, "Custom (per contract)")
	}
}

func TestCreditDisplayUSD_StarterUnchanged(t *testing.T) {
	t.Parallel()
	got := creditDisplayUSD("starter")
	if got != "$19.99" {
		t.Errorf("creditDisplayUSD(starter) = %q, want %q", got, "$19.99")
	}
}

func TestContractRenewalHTML_ContainsDate(t *testing.T) {
	t.Parallel()
	output := contractRenewalHTML("April 1, 2027", 30)
	if !strings.Contains(output, "April 1, 2027") {
		t.Fatal("contract renewal email should contain the end date")
	}
	if !strings.Contains(output, "auto-renew") {
		t.Fatal("contract renewal email should mention auto-renew")
	}
}

func TestContractExpiryHTML_ContainsDate(t *testing.T) {
	t.Parallel()
	output := contractExpiryHTML("April 1, 2027", 7)
	if !strings.Contains(output, "April 1, 2027") {
		t.Fatal("contract expiry email should contain the end date")
	}
	if !strings.Contains(output, "expires") {
		t.Fatal("contract expiry email should mention expiration")
	}
	if !strings.Contains(output, "Scale") {
		t.Fatal("contract expiry email should mention fallback to Scale plan")
	}
}
