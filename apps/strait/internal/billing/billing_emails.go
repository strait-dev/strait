package billing

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"strait/internal/transactional"
)

// TransactionalEmailClient is the narrow app-email surface used by billing.
type TransactionalEmailClient interface {
	Send(ctx context.Context, req transactional.Request) error
}

// BillingEmailSender sends billing notification intents through apps/app.
type BillingEmailSender struct {
	fromEmail string
	logger    *slog.Logger
	client    TransactionalEmailClient
}

// NewBillingEmailSender creates a billing email sender. Returns nil if the
// transactional email client is not configured.
func NewBillingEmailSender(client TransactionalEmailClient, fromEmail string, logger *slog.Logger) *BillingEmailSender {
	if client == nil {
		return nil
	}
	if fromEmail == "" {
		fromEmail = "billing@strait.dev"
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &BillingEmailSender{
		fromEmail: fromEmail,
		logger:    logger,
		client:    client,
	}
}

// SendSpendingLimitWarning sends an email when spend reaches 80% of limit.
func (s *BillingEmailSender) SendSpendingLimitWarning(ctx context.Context, to []string, planName, currentSpend, limit, percent string) {
	if s == nil || len(to) == 0 {
		return
	}
	s.send(ctx, to, "billing.spending_limit_warning", map[string]any{
		"name":          "",
		"planName":      planName,
		"currentSpend":  currentSpend,
		"spendingLimit": limit,
		"percentUsed":   percent,
	}, "billing:spending_limit_warning:%s:%s:%s", recipientsKey(to), planName, percent)
}

// SendOverageAlert sends an email when the org enters overage.
func (s *BillingEmailSender) SendOverageAlert(ctx context.Context, to []string, planName, overageAmount, includedCredit string) {
	if s == nil || len(to) == 0 {
		return
	}
	s.send(ctx, to, "billing.overage_alert", map[string]any{
		"name":              "",
		"planName":          planName,
		"overageAmount":     overageAmount,
		"includedAllowance": includedCredit,
	}, "billing:overage_alert:%s:%s:%s", recipientsKey(to), planName, overageAmount)
}

// SendPaymentFailed sends an email when payment fails.
func (s *BillingEmailSender) SendPaymentFailed(ctx context.Context, to []string, planName string, gracePeriodEnd time.Time) {
	if s == nil || len(to) == 0 {
		return
	}
	graceDate := gracePeriodEnd.Format("January 2, 2006")
	s.send(ctx, to, "billing.payment_failed", map[string]any{
		"name":           "",
		"planName":       planName,
		"gracePeriodEnd": graceDate,
	}, "billing:payment_failed:%s:%s:%s", recipientsKey(to), planName, gracePeriodEnd.Format("2006-01-02"))
}

// SendPlanChanged sends a plan change confirmation email.
func (s *BillingEmailSender) SendPlanChanged(ctx context.Context, to []string, previousPlan, newPlan string) {
	if s == nil || len(to) == 0 {
		return
	}
	effectiveDate := time.Now().Format("January 2, 2006")
	s.send(ctx, to, "billing.plan_changed", map[string]any{
		"name":          "",
		"previousPlan":  previousPlan,
		"newPlan":       newPlan,
		"effectiveDate": effectiveDate,
	}, "billing:plan_changed:%s:%s:%s", recipientsKey(to), previousPlan, newPlan)
}

// SendEnterpriseContractReminder sends a contract renewal or expiry reminder email.
func (s *BillingEmailSender) SendEnterpriseContractReminder(ctx context.Context, to []string, contractEndDate string, autoRenew bool, daysRemaining int) {
	if s == nil || len(to) == 0 {
		return
	}
	s.send(ctx, to, "billing.enterprise_contract_reminder", map[string]any{
		"contractEndDate": contractEndDate,
		"autoRenew":       autoRenew,
		"daysRemaining":   daysRemaining,
	}, "billing:enterprise_contract_reminder:%s:%s:%t:%d", recipientsKey(to), contractEndDate, autoRenew, daysRemaining)
}

// SendDowngradeHTTPJobsWarning notifies org admins that HTTP-mode jobs will be paused.
func (s *BillingEmailSender) SendDowngradeHTTPJobsWarning(ctx context.Context, to []string, periodEnd string, jobCount int) {
	if s == nil || len(to) == 0 {
		return
	}
	s.send(ctx, to, "billing.downgrade_http_jobs_warning", map[string]any{
		"periodEnd": periodEnd,
		"jobCount":  jobCount,
	}, "billing:downgrade_http_jobs_warning:%s:%s:%d", recipientsKey(to), periodEnd, jobCount)
}

// SendContractExpired notifies org admins that their enterprise contract has expired.
func (s *BillingEmailSender) SendContractExpired(ctx context.Context, to []string, contractEndDate string) {
	if s == nil || len(to) == 0 {
		return
	}
	s.send(ctx, to, "billing.contract_expired", map[string]any{
		"contractEndDate": contractEndDate,
	}, "billing:contract_expired:%s:%s", recipientsKey(to), contractEndDate)
}

// SendTrialEndingSoon handles Stripe's trial-ending webhook for legacy or
// contract-specific temporary access without advertising self-serve trials.
func (s *BillingEmailSender) SendTrialEndingSoon(ctx context.Context, to []string, trialEndDate string, daysRemaining int) {
	if s == nil || len(to) == 0 {
		return
	}
	s.send(ctx, to, "billing.trial_ending_soon", map[string]any{
		"trialEndDate":  trialEndDate,
		"daysRemaining": daysRemaining,
	}, "billing:trial_ending_soon:%s:%s:%d", recipientsKey(to), trialEndDate, daysRemaining)
}

// SendDisputeAlert notifies org admins that a charge has been disputed.
func (s *BillingEmailSender) SendDisputeAlert(ctx context.Context, to []string, disputeAmount string) {
	if s == nil || len(to) == 0 {
		return
	}
	s.send(ctx, to, "billing.dispute_alert", map[string]any{
		"disputeAmount": disputeAmount,
	}, "billing:dispute_alert:%s:%s", recipientsKey(to), disputeAmount)
}

// SendInvoiceUpcoming notifies org admins about an upcoming invoice.
func (s *BillingEmailSender) SendInvoiceUpcoming(ctx context.Context, to []string, amountDue, dueDate string) {
	if s == nil || len(to) == 0 {
		return
	}
	s.send(ctx, to, "billing.invoice_upcoming", map[string]any{
		"amountDue": amountDue,
		"dueDate":   dueDate,
	}, "billing:invoice_upcoming:%s:%s:%s", recipientsKey(to), amountDue, dueDate)
}

// SendDunningStep sends the per-step dunning reminder email driven by the
// Dunner state machine. Step values are defined in dunning.go.
func (s *BillingEmailSender) SendDunningStep(ctx context.Context, to []string, planName string, step int) {
	if s == nil || len(to) == 0 {
		return
	}
	s.send(ctx, to, "billing.dunning_step", map[string]any{
		"planName": planName,
		"step":     step,
	}, "billing:dunning_step:%s:%s:%d", recipientsKey(to), planName, step)
}

func (s *BillingEmailSender) send(ctx context.Context, to []string, template string, props map[string]any, idempotencyFormat string, idempotencyArgs ...any) {
	if s.client == nil {
		return
	}
	err := s.client.Send(ctx, transactional.Request{
		Template:       template,
		To:             to,
		From:           s.fromEmail,
		IdempotencyKey: fmt.Sprintf(idempotencyFormat, idempotencyArgs...),
		Props:          props,
	})
	if err != nil {
		logger := s.logger
		if logger == nil {
			logger = slog.Default()
		}
		logger.Warn("failed to send billing email",
			"template", template, "recipients", len(to), "error", err)
	}
}

func recipientsKey(to []string) string {
	return strings.Join(to, ",")
}
