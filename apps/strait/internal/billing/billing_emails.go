package billing

import (
	"context"
	"log/slog"
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
	s.send(ctx, transactional.BillingSpendingLimitWarningRequest(to, s.fromEmail, planName, currentSpend, limit, percent))
}

// SendOverageAlert sends an email when the org enters overage.
func (s *BillingEmailSender) SendOverageAlert(ctx context.Context, to []string, planName, overageAmount, includedCredit string) {
	if s == nil || len(to) == 0 {
		return
	}
	s.send(ctx, transactional.BillingOverageAlertRequest(to, s.fromEmail, planName, overageAmount, includedCredit))
}

// SendPaymentFailed sends an email when payment fails.
func (s *BillingEmailSender) SendPaymentFailed(ctx context.Context, to []string, planName string, gracePeriodEnd time.Time) {
	if s == nil || len(to) == 0 {
		return
	}
	s.send(ctx, transactional.BillingPaymentFailedRequest(to, s.fromEmail, planName, gracePeriodEnd))
}

// SendPlanChanged sends a plan change confirmation email.
func (s *BillingEmailSender) SendPlanChanged(ctx context.Context, to []string, previousPlan, newPlan string) {
	if s == nil || len(to) == 0 {
		return
	}
	effectiveDate := time.Now().Format("January 2, 2006")
	s.send(ctx, transactional.BillingPlanChangedRequest(to, s.fromEmail, previousPlan, newPlan, effectiveDate))
}

// SendEnterpriseContractReminder sends a contract renewal or expiry reminder email.
func (s *BillingEmailSender) SendEnterpriseContractReminder(ctx context.Context, to []string, contractEndDate string, autoRenew bool, daysRemaining int) {
	if s == nil || len(to) == 0 {
		return
	}
	s.send(ctx, transactional.BillingEnterpriseContractReminderRequest(to, s.fromEmail, contractEndDate, autoRenew, daysRemaining))
}

// SendDowngradeHTTPJobsWarning notifies org admins that HTTP-mode jobs will be paused.
func (s *BillingEmailSender) SendDowngradeHTTPJobsWarning(ctx context.Context, to []string, periodEnd string, jobCount int) {
	if s == nil || len(to) == 0 {
		return
	}
	s.send(ctx, transactional.BillingDowngradeHTTPJobsWarningRequest(to, s.fromEmail, periodEnd, jobCount))
}

// SendContractExpired notifies org admins that their enterprise contract has expired.
func (s *BillingEmailSender) SendContractExpired(ctx context.Context, to []string, contractEndDate string) {
	if s == nil || len(to) == 0 {
		return
	}
	s.send(ctx, transactional.BillingContractExpiredRequest(to, s.fromEmail, contractEndDate))
}

// SendTrialEndingSoon handles Stripe's trial-ending webhook for legacy or
// contract-specific temporary access without advertising self-serve trials.
func (s *BillingEmailSender) SendTrialEndingSoon(ctx context.Context, to []string, trialEndDate string, daysRemaining int) {
	if s == nil || len(to) == 0 {
		return
	}
	s.send(ctx, transactional.BillingTrialEndingSoonRequest(to, s.fromEmail, trialEndDate, daysRemaining))
}

// SendDisputeAlert notifies org admins that a charge has been disputed.
func (s *BillingEmailSender) SendDisputeAlert(ctx context.Context, to []string, disputeAmount string) {
	if s == nil || len(to) == 0 {
		return
	}
	s.send(ctx, transactional.BillingDisputeAlertRequest(to, s.fromEmail, disputeAmount))
}

// SendInvoiceUpcoming notifies org admins about an upcoming invoice.
func (s *BillingEmailSender) SendInvoiceUpcoming(ctx context.Context, to []string, amountDue, dueDate string) {
	if s == nil || len(to) == 0 {
		return
	}
	s.send(ctx, transactional.BillingInvoiceUpcomingRequest(to, s.fromEmail, amountDue, dueDate))
}

// SendDunningStep sends the per-step dunning reminder email driven by the
// Dunner state machine. Step values are defined in dunning.go.
func (s *BillingEmailSender) SendDunningStep(ctx context.Context, to []string, planName string, step int) {
	if s == nil || len(to) == 0 {
		return
	}
	s.send(ctx, transactional.BillingDunningStepRequest(to, s.fromEmail, planName, step))
}

func (s *BillingEmailSender) send(ctx context.Context, req transactional.Request) {
	if s.client == nil {
		return
	}
	err := s.client.Send(ctx, req)
	if err != nil {
		logger := s.logger
		if logger == nil {
			logger = slog.Default()
		}
		logger.Warn("failed to send billing email",
			"template", string(req.Template), "recipients", len(req.To), "error", err)
	}
}
