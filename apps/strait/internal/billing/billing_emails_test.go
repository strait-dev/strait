package billing

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/resend/resend-go/v2"
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
	s.SendContractExpired(context.Background(), nil, "")
	s.SendTrialEndingSoon(context.Background(), nil, "", 0)
	s.SendDisputeAlert(context.Background(), nil, "")
	s.SendInvoiceUpcoming(context.Background(), nil, "", "")
	s.SendDunningStep(context.Background(), nil, "", 0)
}

func TestBillingEmailSender_EmptyRecipients(t *testing.T) {
	t.Parallel()
	calls := 0
	s := testBillingEmailSender(func(context.Context, *resend.SendEmailRequest) error {
		calls++
		return nil
	})

	// Empty to slice should not panic or send.
	s.SendSpendingLimitWarning(context.Background(), []string{}, "Pro", "$50", "$100", "80%")
	s.SendOverageAlert(context.Background(), []string{}, "Pro", "$10", "$50")
	s.SendPaymentFailed(context.Background(), []string{}, "Pro", time.Now())
	s.SendPlanChanged(context.Background(), []string{}, "Pro", "Scale")
	s.SendEnterpriseContractReminder(context.Background(), []string{}, "2026-12-31", false, 30)
	s.SendDowngradeHTTPJobsWarning(context.Background(), []string{}, "2026-05-01", 3)
	s.SendContractExpired(context.Background(), []string{}, "2026-12-31")
	s.SendTrialEndingSoon(context.Background(), []string{}, "2026-12-31", 7)
	s.SendDisputeAlert(context.Background(), []string{}, "$25.00")
	s.SendInvoiceUpcoming(context.Background(), []string{}, "$100.00", "2026-05-01")
	s.SendDunningStep(context.Background(), []string{}, "Pro", 1)
	assert.Zero(t, calls)
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

func TestBillingEmailSender_SendsExpectedMessages(t *testing.T) {
	t.Parallel()

	graceEnd := time.Date(2026, time.April, 15, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name         string
		send         func(*BillingEmailSender)
		wantSubject  string
		wantContains []string
	}{
		{
			name: "spending limit warning",
			send: func(sender *BillingEmailSender) {
				sender.SendSpendingLimitWarning(context.Background(), []string{"admin@example.com"}, "Pro", "$80.00", "$100.00", "80%")
			},
			wantSubject:  "Spending limit warning - 80% used",
			wantContains: []string{"Pro", "$80.00", "$100.00", "80%"},
		},
		{
			name: "overage alert",
			send: func(sender *BillingEmailSender) {
				sender.SendOverageAlert(context.Background(), []string{"admin@example.com"}, "Scale", "$25.00", "100000")
			},
			wantSubject:  "Overage alert - Scale plan",
			wantContains: []string{"Scale", "$25.00", "100000"},
		},
		{
			name: "payment failed",
			send: func(sender *BillingEmailSender) {
				sender.SendPaymentFailed(context.Background(), []string{"admin@example.com"}, "Starter", graceEnd)
			},
			wantSubject:  "Action required: payment failed",
			wantContains: []string{"Starter", "April 15, 2026"},
		},
		{
			name: "plan changed",
			send: func(sender *BillingEmailSender) {
				sender.SendPlanChanged(context.Background(), []string{"admin@example.com"}, "Starter", "Pro")
			},
			wantSubject:  "Plan changed to Pro",
			wantContains: []string{"Starter", "Pro"},
		},
		{
			name: "contract renewal",
			send: func(sender *BillingEmailSender) {
				sender.SendEnterpriseContractReminder(context.Background(), []string{"admin@example.com"}, "2026-12-31", true, 30)
			},
			wantSubject:  "Enterprise contract renewing in 30 days",
			wantContains: []string{"2026-12-31", "auto-renew"},
		},
		{
			name: "contract expiry",
			send: func(sender *BillingEmailSender) {
				sender.SendEnterpriseContractReminder(context.Background(), []string{"admin@example.com"}, "2026-12-31", false, 7)
			},
			wantSubject:  "Enterprise contract expiring in 7 days",
			wantContains: []string{"2026-12-31", "expires"},
		},
		{
			name: "http jobs downgrade",
			send: func(sender *BillingEmailSender) {
				sender.SendDowngradeHTTPJobsWarning(context.Background(), []string{"admin@example.com"}, "2026-05-01", 3)
			},
			wantSubject:  "Your 3 HTTP-mode jobs will be paused on 2026-05-01",
			wantContains: []string{"2026-05-01", "3 HTTP-mode job(s)"},
		},
		{
			name: "contract expired",
			send: func(sender *BillingEmailSender) {
				sender.SendContractExpired(context.Background(), []string{"admin@example.com"}, "2026-01-31")
			},
			wantSubject:  "Your enterprise contract has expired",
			wantContains: []string{"2026-01-31", "restricted mode"},
		},
		{
			name: "trial ending",
			send: func(sender *BillingEmailSender) {
				sender.SendTrialEndingSoon(context.Background(), []string{"admin@example.com"}, "2026-06-30", 5)
			},
			wantSubject:  "Temporary access ends in 5 days",
			wantContains: []string{"2026-06-30", "5 days from now"},
		},
		{
			name: "dispute alert",
			send: func(sender *BillingEmailSender) {
				sender.SendDisputeAlert(context.Background(), []string{"admin@example.com"}, "$25.00")
			},
			wantSubject:  "Payment dispute received",
			wantContains: []string{"$25.00", "dispute"},
		},
		{
			name: "invoice upcoming",
			send: func(sender *BillingEmailSender) {
				sender.SendInvoiceUpcoming(context.Background(), []string{"admin@example.com"}, "$125.00", "2026-07-01")
			},
			wantSubject:  "Upcoming invoice",
			wantContains: []string{"$125.00", "2026-07-01"},
		},
		{
			name: "dunning step",
			send: func(sender *BillingEmailSender) {
				sender.SendDunningStep(context.Background(), []string{"admin@example.com"}, "Business", 4)
			},
			wantSubject:  "Access restricted — payment required",
			wantContains: []string{"Business", "restricted mode"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got *resend.SendEmailRequest
			sender := testBillingEmailSender(func(_ context.Context, req *resend.SendEmailRequest) error {
				got = req
				return nil
			})

			tc.send(sender)

			require.NotNil(t, got)
			assert.Equal(t, "billing@example.com", got.From)
			assert.Equal(t, []string{"admin@example.com"}, got.To)
			assert.Equal(t, tc.wantSubject, got.Subject)
			for _, want := range tc.wantContains {
				assert.Contains(t, got.Html, want)
			}
		})
	}
}

func TestBillingEmailSender_SendHandlesTransportEdges(t *testing.T) {
	t.Parallel()

	noTransport := &BillingEmailSender{
		fromEmail: "billing@example.com",
		logger:    slog.New(slog.DiscardHandler),
	}
	assert.NotPanics(t, func() {
		noTransport.SendDisputeAlert(context.Background(), []string{"admin@example.com"}, "$25.00")
	})

	sendErr := errors.New("resend unavailable")
	failing := testBillingEmailSender(func(context.Context, *resend.SendEmailRequest) error {
		return sendErr
	})
	failing.logger = nil
	assert.NotPanics(t, func() {
		failing.SendInvoiceUpcoming(context.Background(), []string{"admin@example.com"}, "$125.00", "2026-07-01")
	})
}

func testBillingEmailSender(send billingEmailSendFunc) *BillingEmailSender {
	return &BillingEmailSender{
		fromEmail: "billing@example.com",
		logger:    slog.New(slog.DiscardHandler),
		sendEmail: send,
	}
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
