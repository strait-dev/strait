package billing

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestNewBillingEmailSender_EmptyKey_ReturnsNil(t *testing.T) {
	t.Parallel()
	s := NewBillingEmailSender("", "", nil)
	if s != nil {
		t.Fatal("expected nil for empty API key")
	}
}

func TestNewBillingEmailSender_ValidKey_ReturnsNonNil(t *testing.T) {
	t.Parallel()
	s := NewBillingEmailSender("re_test", "", nil)
	if s == nil {
		t.Fatal("expected non-nil for valid API key")
	}
}

func TestSpendingLimitWarningHTML_EscapesHTML(t *testing.T) {
	t.Parallel()
	html := spendingLimitWarningHTML("<script>alert(1)</script>", "$50", "$100", "80%")
	if strings.Contains(html, "<script>") {
		t.Fatal("HTML not escaped in planName")
	}
	if !strings.Contains(html, "&lt;script&gt;") {
		t.Fatal("expected escaped script tag")
	}
}

func TestSpendingLimitWarningHTML_ContainsValues(t *testing.T) {
	t.Parallel()
	html := spendingLimitWarningHTML("Pro", "$42.50", "$100.00", "80%")
	if !strings.Contains(html, "Pro") {
		t.Fatal("expected plan name")
	}
	if !strings.Contains(html, "80%") {
		t.Fatal("expected percent")
	}
	if !strings.Contains(html, "$100.00") {
		t.Fatal("expected limit")
	}
}

func TestOverageAlertHTML_EscapesHTML(t *testing.T) {
	t.Parallel()
	html := overageAlertHTML("<img>", "$10", "50000")
	if strings.Contains(html, "<img>") {
		t.Fatal("HTML not escaped")
	}
}

func TestOverageAlertHTML_UsesRunAllowanceLanguage(t *testing.T) {
	t.Parallel()
	html := overageAlertHTML("Starter", "$10", "50000")
	if !strings.Contains(html, "included allowance of 50000 orchestration runs") {
		t.Fatal("expected orchestration run allowance language")
	}
	if strings.Contains(html, "included credit") {
		t.Fatal("overage alert must not use compute credit language")
	}
}

func TestPaymentFailedHTML_ContainsGracePeriod(t *testing.T) {
	t.Parallel()
	html := paymentFailedHTML("Starter", "April 15, 2026")
	if !strings.Contains(html, "April 15, 2026") {
		t.Fatal("expected grace period date")
	}
	if !strings.Contains(html, "Starter") {
		t.Fatal("expected plan name")
	}
}

func TestPlanChangedHTML_ContainsBothPlans(t *testing.T) {
	t.Parallel()
	html := planChangedHTML("Starter", "Pro", "March 30, 2026")
	if !strings.Contains(html, "Starter") {
		t.Fatal("expected previous plan")
	}
	if !strings.Contains(html, "Pro") {
		t.Fatal("expected new plan")
	}
}

func TestBillingEmailSender_NilSafety(t *testing.T) {
	t.Parallel()
	var s *BillingEmailSender
	// All methods should be safe to call on nil receiver.
	s.SendSpendingLimitWarning(context.Background(), nil, "", "", "", "")
	s.SendOverageAlert(context.Background(), nil, "", "", "")
	s.SendPaymentFailed(context.Background(), nil, "", time.Now())
	s.SendPlanChanged(context.Background(), nil, "", "")
	s.SendEnterpriseContractReminder(context.Background(), nil, "", true, 30)
	s.SendDowngradeHTTPJobsWarning(context.Background(), nil, "", 0)
}

func TestBillingEmailSender_EmptyRecipients(t *testing.T) {
	t.Parallel()
	s := NewBillingEmailSender("re_test_key", "", nil)
	if s == nil {
		t.Fatal("expected non-nil sender")
		return
	}
	// Empty to slice should not panic or send.
	s.SendSpendingLimitWarning(context.Background(), []string{}, "Pro", "$50", "$100", "80%")
	s.SendOverageAlert(context.Background(), []string{}, "Pro", "$10", "$50")
	s.SendPaymentFailed(context.Background(), []string{}, "Pro", time.Now())
	s.SendPlanChanged(context.Background(), []string{}, "Pro", "Scale")
	s.SendEnterpriseContractReminder(context.Background(), []string{}, "2026-12-31", false, 30)
	s.SendDowngradeHTTPJobsWarning(context.Background(), []string{}, "2026-05-01", 3)
}

func TestNewBillingEmailSender_DefaultFromEmail(t *testing.T) {
	t.Parallel()
	s := NewBillingEmailSender("re_test_key", "", nil)
	if s == nil {
		t.Fatal("expected non-nil sender")
		return
	}
	if s.fromEmail != "billing@strait.dev" {
		t.Errorf("fromEmail = %q, want billing@strait.dev", s.fromEmail)
	}
}

func TestNewBillingEmailSender_CustomFromEmail(t *testing.T) {
	t.Parallel()
	s := NewBillingEmailSender("re_test_key", "custom@example.com", nil)
	if s == nil {
		t.Fatal("expected non-nil sender")
		return
	}
	if s.fromEmail != "custom@example.com" {
		t.Errorf("fromEmail = %q, want custom@example.com", s.fromEmail)
	}
}

func TestDowngradeHTTPJobsWarningHTML_EscapesHTML(t *testing.T) {
	t.Parallel()
	html := downgradeHTTPJobsWarningHTML("<script>alert(1)</script>", 3)
	if strings.Contains(html, "<script>") {
		t.Fatal("HTML not escaped in periodEnd")
	}
}

func FuzzBillingEmailHTML(f *testing.F) {
	f.Add("Pro", "$49.99", "$100", "80%")
	f.Add("<script>", "<img>", "<a>", "<b>")
	f.Add("", "", "", "")

	f.Fuzz(func(t *testing.T, a, b, c, d string) {
		r1 := spendingLimitWarningHTML(a, b, c, d)
		if strings.Contains(r1, "<script>") && strings.Contains(a, "<script>") {
			t.Error("unescaped HTML in spending limit warning")
		}
		r2 := overageAlertHTML(a, b, c)
		_ = r2
		r3 := paymentFailedHTML(a, b)
		_ = r3
		r4 := planChangedHTML(a, b, c)
		_ = r4
	})
}
