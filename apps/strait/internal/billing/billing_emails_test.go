package billing

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBillingEmailSender_EmptyKey_ReturnsNil(t *testing.T) {
	t.Parallel()
	s := NewBillingEmailSender("", "", nil)
	require.Nil(t, s)
}

func TestNewBillingEmailSender_ValidKey_ReturnsNonNil(t *testing.T) {
	t.Parallel()
	s := NewBillingEmailSender("re_test", "", nil)
	require.NotNil(t,
		s)
}

func TestSpendingLimitWarningHTML_EscapesHTML(t *testing.T) {
	t.Parallel()
	html := spendingLimitWarningHTML("<script>alert(1)</script>", "$50", "$100", "80%")
	require.NotContains(t,
		html, "<script>")
	require.Contains(t, html, "&lt;script&gt;")
}

func TestSpendingLimitWarningHTML_ContainsValues(t *testing.T) {
	t.Parallel()
	html := spendingLimitWarningHTML("Pro", "$42.50", "$100.00", "80%")
	require.Contains(t, html, "Pro")
	require.Contains(t, html, "80%")
	require.Contains(t, html, "$100.00")
}

func TestOverageAlertHTML_EscapesHTML(t *testing.T) {
	t.Parallel()
	html := overageAlertHTML("<img>", "$10", "50000")
	require.NotContains(t,
		html, "<img>")
}

func TestOverageAlertHTML_UsesRunAllowanceLanguage(t *testing.T) {
	t.Parallel()
	html := overageAlertHTML("Starter", "$10", "50000")
	require.Contains(t, html, "included allowance of 50000 orchestration runs")
	require.NotContains(t,
		html, "included credit")
}

func TestPaymentFailedHTML_ContainsGracePeriod(t *testing.T) {
	t.Parallel()
	html := paymentFailedHTML("Starter", "April 15, 2026")
	require.Contains(t, html, "April 15, 2026")
	require.Contains(t, html, "Starter")
}

func TestPlanChangedHTML_ContainsBothPlans(t *testing.T) {
	t.Parallel()
	html := planChangedHTML("Starter", "Pro", "March 30, 2026")
	require.Contains(t, html, "Starter")
	require.Contains(t, html, "Pro")
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
	require.NotNil(t,
		s)

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
	require.NotNil(t,
		s)
	assert.Equal(t, "billing@strait.dev",

		s.
			fromEmail,
	)
}

func TestNewBillingEmailSender_CustomFromEmail(t *testing.T) {
	t.Parallel()
	s := NewBillingEmailSender("re_test_key", "custom@example.com", nil)
	require.NotNil(t,
		s)
	assert.Equal(t, "custom@example.com",

		s.
			fromEmail,
	)
}

func TestDowngradeHTTPJobsWarningHTML_EscapesHTML(t *testing.T) {
	t.Parallel()
	html := downgradeHTTPJobsWarningHTML("<script>alert(1)</script>", 3)
	require.NotContains(t,
		html, "<script>")
}

func FuzzBillingEmailHTML(f *testing.F) {
	f.Add("Pro", "$49.99", "$100", "80%")
	f.Add("<script>", "<img>", "<a>", "<b>")
	f.Add("", "", "", "")

	f.Fuzz(func(t *testing.T, a, b, c, d string) {
		r1 := spendingLimitWarningHTML(a, b, c, d)
		assert.False(t, strings.Contains(r1, "<script>") &&
			strings.Contains(a, "<script>"))

		r2 := overageAlertHTML(a, b, c)
		_ = r2
		r3 := paymentFailedHTML(a, b)
		_ = r3
		r4 := planChangedHTML(a, b, c)
		_ = r4
	})
}
