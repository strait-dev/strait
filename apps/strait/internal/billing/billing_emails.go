package billing

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"time"

	"github.com/resend/resend-go/v2"
)

// BillingEmailSender sends billing notification emails via Resend.
type BillingEmailSender struct {
	fromEmail string
	logger    *slog.Logger
	sendEmail billingEmailSendFunc
}

type billingEmailSendFunc func(context.Context, *resend.SendEmailRequest) error

// NewBillingEmailSender creates a billing email sender. Returns nil if apiKey is empty.
func NewBillingEmailSender(apiKey, fromEmail string, logger *slog.Logger) *BillingEmailSender {
	if apiKey == "" {
		return nil
	}
	if fromEmail == "" {
		fromEmail = "billing@strait.dev"
	}
	if logger == nil {
		logger = slog.Default()
	}
	client := resend.NewClient(apiKey)
	return &BillingEmailSender{
		fromEmail: fromEmail,
		logger:    logger,
		sendEmail: func(ctx context.Context, req *resend.SendEmailRequest) error {
			_, err := client.Emails.SendWithContext(ctx, req)
			return err
		},
	}
}

// SendSpendingLimitWarning sends an email when spend reaches 80% of limit.
func (s *BillingEmailSender) SendSpendingLimitWarning(ctx context.Context, to []string, planName, currentSpend, limit, percent string) {
	if s == nil || len(to) == 0 {
		return
	}
	subject := "Spending limit warning - " + percent + " used"
	body := spendingLimitWarningHTML(planName, currentSpend, limit, percent)
	s.send(ctx, to, subject, body)
}

// SendOverageAlert sends an email when the org enters overage.
func (s *BillingEmailSender) SendOverageAlert(ctx context.Context, to []string, planName, overageAmount, includedCredit string) {
	if s == nil || len(to) == 0 {
		return
	}
	subject := "Overage alert - " + planName + " plan"
	body := overageAlertHTML(planName, overageAmount, includedCredit)
	s.send(ctx, to, subject, body)
}

// SendPaymentFailed sends an email when payment fails.
func (s *BillingEmailSender) SendPaymentFailed(ctx context.Context, to []string, planName string, gracePeriodEnd time.Time) {
	if s == nil || len(to) == 0 {
		return
	}
	subject := "Action required: payment failed"
	body := paymentFailedHTML(planName, gracePeriodEnd.Format("January 2, 2006"))
	s.send(ctx, to, subject, body)
}

// SendPlanChanged sends a plan change confirmation email.
func (s *BillingEmailSender) SendPlanChanged(ctx context.Context, to []string, previousPlan, newPlan string) {
	if s == nil || len(to) == 0 {
		return
	}
	subject := "Plan changed to " + newPlan
	body := planChangedHTML(previousPlan, newPlan, time.Now().Format("January 2, 2006"))
	s.send(ctx, to, subject, body)
}

// SendEnterpriseContractReminder sends a contract renewal or expiry reminder email.
// If autoRenew is true, the email is a renewal notice. Otherwise it is an expiry warning.
func (s *BillingEmailSender) SendEnterpriseContractReminder(ctx context.Context, to []string, contractEndDate string, autoRenew bool, daysRemaining int) {
	if s == nil || len(to) == 0 {
		return
	}
	var subject, body string
	if autoRenew {
		subject = fmt.Sprintf("Enterprise contract renewing in %d days", daysRemaining)
		body = contractRenewalHTML(contractEndDate, daysRemaining)
	} else {
		subject = fmt.Sprintf("Enterprise contract expiring in %d days", daysRemaining)
		body = contractExpiryHTML(contractEndDate, daysRemaining)
	}
	s.send(ctx, to, subject, body)
}

// SendDowngradeHTTPJobsWarning notifies org admins that HTTP-mode jobs will be paused
// when the pending downgrade takes effect at the end of the billing period.
func (s *BillingEmailSender) SendDowngradeHTTPJobsWarning(ctx context.Context, to []string, periodEnd string, jobCount int) {
	if s == nil || len(to) == 0 {
		return
	}
	subject := fmt.Sprintf("Your %d HTTP-mode jobs will be paused on %s", jobCount, periodEnd)
	body := downgradeHTTPJobsWarningHTML(periodEnd, jobCount)
	s.send(ctx, to, subject, body)
}

func downgradeHTTPJobsWarningHTML(periodEnd string, jobCount int) string {
	safeDate := html.EscapeString(periodEnd)
	body := fmt.Sprintf(`<tr>
      <td style="font-size:14px;color:#8D8D8D;line-height:1.6;">
        Your plan downgrade takes effect on <strong style="color:#252525;">%s</strong>. At that time, your <strong style="color:#252525;">%d HTTP-mode job(s)</strong> will be automatically paused because the new plan does not support HTTP execution mode.
      </td>
    </tr>
    <tr><td style="height:16px;"></td></tr>
    <tr>
      <td style="font-size:14px;color:#8D8D8D;line-height:1.6;">
        Your job configurations and run history will be fully preserved. To keep your HTTP jobs running, upgrade back to Pro or higher before the period ends.
      </td>
    </tr>
    <tr><td style="height:16px;"></td></tr>
    <tr>
      <td>
        <a href="https://app.strait.dev/app/upgrade" style="display:inline-block;padding:10px 24px;background-color:#171717;color:#FFFFFF;font-size:14px;font-weight:500;text-decoration:none;border-radius:4px;">
          Upgrade plan
        </a>
      </td>
    </tr>`, safeDate, jobCount)
	return billingEmailWrapper("HTTP jobs will be paused", body)
}

// SendContractExpired notifies org admins that their enterprise contract has expired.
func (s *BillingEmailSender) SendContractExpired(ctx context.Context, to []string, contractEndDate string) {
	if s == nil || len(to) == 0 {
		return
	}
	safeDate := html.EscapeString(contractEndDate)
	body := fmt.Sprintf(`<tr>
      <td style="font-size:14px;color:#8D8D8D;line-height:1.6;">
        Your Enterprise contract expired on <strong style="color:#252525;">%s</strong>. Your organization has been placed in restricted mode. New job runs will be blocked until your contract is renewed.
      </td>
    </tr>
    <tr><td style="height:16px;"></td></tr>
    <tr>
      <td>
        <a href="mailto:leo@strait.dev" style="display:inline-block;padding:10px 24px;background-color:#171717;color:#FFFFFF;font-size:14px;font-weight:500;text-decoration:none;border-radius:4px;">
          Contact sales to renew
        </a>
      </td>
    </tr>`, safeDate)
	s.send(ctx, to, "Your enterprise contract has expired", billingEmailWrapper("Enterprise contract expired", body))
}

// SendTrialEndingSoon handles Stripe's trial-ending webhook for legacy or
// contract-specific temporary access without advertising self-serve trials.
func (s *BillingEmailSender) SendTrialEndingSoon(ctx context.Context, to []string, trialEndDate string, daysRemaining int) {
	if s == nil || len(to) == 0 {
		return
	}
	subject := fmt.Sprintf("Temporary access ends in %d days", daysRemaining)
	body := trialEndingHTML(trialEndDate, daysRemaining)
	s.send(ctx, to, subject, body)
}

// SendDisputeAlert notifies org admins that a charge has been disputed.
func (s *BillingEmailSender) SendDisputeAlert(ctx context.Context, to []string, disputeAmount string) {
	if s == nil || len(to) == 0 {
		return
	}
	body := disputeAlertHTML(disputeAmount)
	s.send(ctx, to, "Payment dispute received", body)
}

// SendInvoiceUpcoming notifies org admins about an upcoming invoice.
func (s *BillingEmailSender) SendInvoiceUpcoming(ctx context.Context, to []string, amountDue, dueDate string) {
	if s == nil || len(to) == 0 {
		return
	}
	body := invoiceUpcomingHTML(amountDue, dueDate)
	s.send(ctx, to, "Upcoming invoice", body)
}

// SendDunningStep sends the per-step dunning reminder email driven by the
// Dunner state machine. Step values are defined in dunning.go; unrecognized
// steps fall back to a generic past-due message.
func (s *BillingEmailSender) SendDunningStep(ctx context.Context, to []string, planName string, step int) {
	if s == nil || len(to) == 0 {
		return
	}
	subject, body := dunningStepCopy(planName, step)
	s.send(ctx, to, subject, body)
}

func dunningStepCopy(planName string, step int) (string, string) {
	safePlan := html.EscapeString(planName)
	switch step {
	case 1:
		return "Payment failed — action required",
			billingEmailWrapper("Payment failed",
				fmt.Sprintf(`<tr><td style="font-size:14px;color:#8D8D8D;line-height:1.6;">We could not collect payment for your <strong>%s</strong> plan. Update your billing details to avoid service disruption.</td></tr>`, safePlan))
	case 2:
		return "Payment still past due (day 3)",
			billingEmailWrapper("Payment past due",
				fmt.Sprintf(`<tr><td style="font-size:14px;color:#8D8D8D;line-height:1.6;">Three days have passed without a successful payment on your <strong>%s</strong> plan. Please update your billing details.</td></tr>`, safePlan))
	case 3:
		return "Payment still past due (day 7)",
			billingEmailWrapper("Payment past due",
				fmt.Sprintf(`<tr><td style="font-size:14px;color:#8D8D8D;line-height:1.6;">Your <strong>%s</strong> plan is one week past due. Access will be restricted in seven more days if payment is not received.</td></tr>`, safePlan))
	case 4:
		return "Access restricted — payment required",
			billingEmailWrapper("Access restricted",
				fmt.Sprintf(`<tr><td style="font-size:14px;color:#8D8D8D;line-height:1.6;">Your <strong>%s</strong> plan has entered restricted mode after 14 days without payment. New runs are blocked until your invoice is paid.</td></tr>`, safePlan))
	case 5:
		return "Final notice before suspension",
			billingEmailWrapper("Final notice",
				fmt.Sprintf(`<tr><td style="font-size:14px;color:#8D8D8D;line-height:1.6;">This is the final notice for your <strong>%s</strong> plan. The subscription will be suspended in 30 days if no payment is received.</td></tr>`, safePlan))
	case 6:
		return "Subscription suspended",
			billingEmailWrapper("Subscription suspended",
				fmt.Sprintf(`<tr><td style="font-size:14px;color:#8D8D8D;line-height:1.6;">Your <strong>%s</strong> subscription has been suspended. Contact support to reactivate.</td></tr>`, safePlan))
	default:
		return "Payment past due",
			billingEmailWrapper("Payment past due",
				fmt.Sprintf(`<tr><td style="font-size:14px;color:#8D8D8D;line-height:1.6;">Your <strong>%s</strong> plan has an outstanding balance. Please update your billing details.</td></tr>`, safePlan))
	}
}

func trialEndingHTML(endDate string, daysRemaining int) string {
	safeDate := html.EscapeString(endDate)
	body := fmt.Sprintf(`<tr>
      <td style="font-size:14px;color:#8D8D8D;line-height:1.6;">
        Your temporary Strait access ends on <strong style="color:#252525;">%s</strong> (%d days from now). Choose a launch plan or update billing to keep paid-plan limits.
      </td>
    </tr>
    <tr><td style="height:16px;"></td></tr>
    <tr>
      <td>
        <a href="https://app.strait.dev/app/billing" style="display:inline-block;padding:10px 24px;background-color:#171717;color:#FFFFFF;font-size:14px;font-weight:500;text-decoration:none;border-radius:4px;">
          Manage billing
        </a>
      </td>
    </tr>`, safeDate, daysRemaining)
	return billingEmailWrapper("Temporary access ending soon", body)
}

func disputeAlertHTML(amount string) string {
	safeAmount := html.EscapeString(amount)
	body := fmt.Sprintf(`<tr>
      <td style="font-size:14px;color:#8D8D8D;line-height:1.6;">
        A payment dispute of <strong style="color:#252525;">%s</strong> has been opened on your account. Your service will continue while the dispute is under review.
      </td>
    </tr>
    <tr><td style="height:16px;"></td></tr>
    <tr>
      <td style="font-size:14px;color:#8D8D8D;line-height:1.6;">
        If this dispute is not resolved, your account may be restricted. Please check your email for details from your payment provider.
      </td>
    </tr>`, safeAmount)
	return billingEmailWrapper("Payment dispute received", body)
}

func invoiceUpcomingHTML(amountDue, dueDate string) string {
	safeAmount := html.EscapeString(amountDue)
	safeDate := html.EscapeString(dueDate)
	body := fmt.Sprintf(`<tr>
      <td style="font-size:14px;color:#8D8D8D;line-height:1.6;">
        Your next Strait invoice of <strong style="color:#252525;">%s</strong> will be charged on <strong style="color:#252525;">%s</strong>.
      </td>
    </tr>
    <tr><td style="height:16px;"></td></tr>
    <tr>
      <td>
        <a href="https://app.strait.dev/app/billing" style="display:inline-block;padding:10px 24px;background-color:#171717;color:#FFFFFF;font-size:14px;font-weight:500;text-decoration:none;border-radius:4px;">
          View billing
        </a>
      </td>
    </tr>`, safeAmount, safeDate)
	return billingEmailWrapper("Upcoming invoice", body)
}

func contractRenewalHTML(endDate string, daysRemaining int) string {
	safeDate := html.EscapeString(endDate)
	body := fmt.Sprintf(`<tr>
      <td style="font-size:14px;color:#8D8D8D;line-height:1.6;">
        Your Enterprise contract is set to auto-renew on <strong style="color:#252525;">%s</strong> (%d days from now).
      </td>
    </tr>
    <tr><td style="height:16px;"></td></tr>
    <tr>
      <td style="font-size:14px;color:#8D8D8D;line-height:1.6;">
        No action is required. Your contract terms will continue unchanged. If you need to modify your contract, contact your Customer Success Manager or email <a href="mailto:leo@strait.dev" style="color:#171717;text-decoration:underline;">leo@strait.dev</a>.
      </td>
    </tr>`, safeDate, daysRemaining)
	return billingEmailWrapper("Contract renewal notice", body)
}

func contractExpiryHTML(endDate string, daysRemaining int) string {
	safeDate := html.EscapeString(endDate)
	body := fmt.Sprintf(`<tr>
      <td style="font-size:14px;color:#8D8D8D;line-height:1.6;">
        Your Enterprise contract expires on <strong style="color:#252525;">%s</strong> (%d days from now).
      </td>
    </tr>
    <tr><td style="height:16px;"></td></tr>
    <tr>
      <td style="font-size:14px;color:#8D8D8D;line-height:1.6;">
        After expiry, your organization will be moved to the Scale plan. To renew your Enterprise contract, contact your Customer Success Manager or email <a href="mailto:leo@strait.dev" style="color:#171717;text-decoration:underline;">leo@strait.dev</a>.
      </td>
    </tr>
    <tr><td style="height:16px;"></td></tr>
    <tr>
      <td>
        <a href="mailto:leo@strait.dev" style="display:inline-block;padding:10px 24px;background-color:#171717;color:#FFFFFF;font-size:14px;font-weight:500;text-decoration:none;border-radius:4px;">
          Contact sales
        </a>
      </td>
    </tr>`, safeDate, daysRemaining)
	return billingEmailWrapper("Enterprise contract expiring soon", body)
}

func (s *BillingEmailSender) send(ctx context.Context, to []string, subject, htmlBody string) {
	if s.sendEmail == nil {
		return
	}

	err := s.sendEmail(ctx, &resend.SendEmailRequest{
		From:    s.fromEmail,
		To:      to,
		Subject: subject,
		Html:    htmlBody,
	})
	if err != nil {
		logger := s.logger
		if logger == nil {
			logger = slog.Default()
		}
		logger.Warn("failed to send billing email",
			"subject", subject, "recipients", len(to), "error", err)
	}
}

// billingEmailWrapper returns the shared HTML shell for all billing emails.
// heading is the bold title; bodyRows contains the unique <tr> rows for that email.
func billingEmailWrapper(heading, bodyRows string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
</head>
<body style="margin:0;padding:0;background-color:#FFFFFF;font-family:'Geist',Helvetica,Arial,sans-serif;">
  <table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="margin:40px auto;max-width:500px;border:1px solid #EBEBEB;border-radius:2px;padding:32px 40px;">
    <tr>
      <td>
        <img src="https://app.usestrait.com/static/strait-logo-black.svg" alt="Strait" width="150" style="display:block;" />
      </td>
    </tr>
    <tr><td style="height:16px;"></td></tr>
    <tr>
      <td style="font-size:18px;font-weight:600;color:#252525;letter-spacing:-0.02em;">
        %s
      </td>
    </tr>
    <tr><td style="height:16px;"></td></tr>
    %s
    <tr><td style="height:16px;"></td></tr>
    <tr>
      <td style="font-size:12px;color:#8D8D8D;line-height:1.6;">
        If you have any questions, just reply to this email or contact our support team at <a href="mailto:support@strait.dev" style="color:#171717;text-decoration:underline;">support@strait.dev</a>.
      </td>
    </tr>
    <tr><td style="height:16px;"></td></tr>
    <tr>
      <td style="border-top:1px solid #EBEBEB;padding-top:16px;">
        <span style="font-size:12px;color:#8D8D8D;">&copy; 2026 Strait, All rights reserved</span>
      </td>
    </tr>
  </table>
</body>
</html>`, heading, bodyRows)
}

// spendingLimitWarningHTML returns the HTML body for the spending limit warning email.
func spendingLimitWarningHTML(planName, currentSpend, limit, percent string) string {
	safePlan := html.EscapeString(planName)
	safeSpend := html.EscapeString(currentSpend)
	safeLimit := html.EscapeString(limit)
	safePercent := html.EscapeString(percent)
	body := fmt.Sprintf(`<tr>
      <td style="font-size:14px;color:#8D8D8D;line-height:1.6;">
        Your %s plan has used <strong style="color:#252525;">%s</strong> of your %s spending limit (%s spent this billing period).
      </td>
    </tr>
    <tr><td style="height:16px;"></td></tr>
    <tr>
      <td>
        <a href="https://app.usestrait.com/app/settings/billing" style="display:inline-block;padding:10px 24px;background-color:#171717;color:#FFFFFF;font-size:14px;font-weight:500;text-decoration:none;border-radius:4px;">
          Adjust spending limit
        </a>
      </td>
    </tr>`, safePlan, safePercent, safeLimit, safeSpend)
	return billingEmailWrapper("Spending limit warning", body)
}

// overageAlertHTML returns the HTML body for the overage alert email.
func overageAlertHTML(planName, overageAmount, includedAllowance string) string {
	safePlan := html.EscapeString(planName)
	safeOverage := html.EscapeString(overageAmount)
	safeAllowance := html.EscapeString(includedAllowance)
	body := fmt.Sprintf(`<tr>
      <td style="font-size:14px;color:#8D8D8D;line-height:1.6;">
        Your %s plan has exceeded its included allowance of %s orchestration runs. Current overage: <strong style="color:#252525;">%s</strong>. Set a spending cap to control costs.
      </td>
    </tr>
    <tr><td style="height:16px;"></td></tr>
    <tr>
      <td>
        <a href="https://app.usestrait.com/app/settings/billing" style="display:inline-block;padding:10px 24px;background-color:#171717;color:#FFFFFF;font-size:14px;font-weight:500;text-decoration:none;border-radius:4px;">
          Set spending limit
        </a>
      </td>
    </tr>`, safePlan, safeAllowance, safeOverage)
	return billingEmailWrapper("Overage alert", body)
}

// paymentFailedHTML returns the HTML body for the payment failed email.
func paymentFailedHTML(planName, gracePeriodEnd string) string {
	safePlan := html.EscapeString(planName)
	safeGrace := html.EscapeString(gracePeriodEnd)
	body := fmt.Sprintf(`<tr>
      <td style="font-size:14px;color:#8D8D8D;line-height:1.6;">
        We were unable to process payment for your %s plan. Your account is in a grace period until <strong style="color:#252525;">%s</strong>. Please update your payment method to avoid service interruption.
      </td>
    </tr>
    <tr><td style="height:16px;"></td></tr>
    <tr>
      <td>
        <a href="https://app.usestrait.com/app/settings/billing" style="display:inline-block;padding:10px 24px;background-color:#171717;color:#FFFFFF;font-size:14px;font-weight:500;text-decoration:none;border-radius:4px;">
          Update payment method
        </a>
      </td>
    </tr>`, safePlan, safeGrace)
	return billingEmailWrapper("Payment failed", body)
}

// planChangedHTML returns the HTML body for the plan changed email.
func planChangedHTML(previousPlan, newPlan, effectiveDate string) string {
	safePrev := html.EscapeString(previousPlan)
	safeNew := html.EscapeString(newPlan)
	safeDate := html.EscapeString(effectiveDate)
	body := fmt.Sprintf(`<tr>
      <td style="font-size:14px;color:#8D8D8D;line-height:1.6;">
        Your plan has been changed from <strong style="color:#252525;">%s</strong> to <strong style="color:#252525;">%s</strong>, effective %s.
      </td>
    </tr>
    <tr><td style="height:16px;"></td></tr>
    <tr>
      <td>
        <a href="https://app.usestrait.com/app/settings/billing" style="display:inline-block;padding:10px 24px;background-color:#171717;color:#FFFFFF;font-size:14px;font-weight:500;text-decoration:none;border-radius:4px;">
          View billing
        </a>
      </td>
    </tr>`, safePrev, safeNew, safeDate)
	return billingEmailWrapper("Plan changed", body)
}
