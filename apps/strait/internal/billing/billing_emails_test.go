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
	html := overageAlertHTML("<img>", "$10", "$50")
	if strings.Contains(html, "<img>") {
		t.Fatal("HTML not escaped")
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
