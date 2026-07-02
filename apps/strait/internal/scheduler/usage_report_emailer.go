package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"strait/internal/billing"
	"strait/internal/transactional"
)

// UsageReportEmailerStore defines the store operations needed by UsageReportEmailer.
// All methods are provided by billing.Store.
type UsageReportEmailerStore interface {
	billing.Store
}

type usageReportClaimStore interface {
	ClaimUsageReportSend(ctx context.Context, orgID string, periodEnd time.Time) (bool, error)
	FinalizeUsageReportSend(ctx context.Context, orgID string, periodEnd time.Time) error
	ReleaseUsageReportSendClaim(ctx context.Context, orgID string, periodEnd time.Time) error
}

// TransactionalEmailSender is the subset of the app email client used for sending emails.
type TransactionalEmailSender interface {
	Send(ctx context.Context, req transactional.Request) error
}

// UsageReportEmailer sends monthly PDF usage reports to org admins after their
// billing period ends. Runs daily and catches up any ended period that has not
// been claimed yet.
type UsageReportEmailer struct {
	store     UsageReportEmailerStore
	emailAPI  TransactionalEmailSender
	fromEmail string
	interval  time.Duration
	logger    *slog.Logger
	// lastRunDate prevents running more than once per day.
	lastRunDate string
}

// NewUsageReportEmailer creates a new monthly usage report emailer.
func NewUsageReportEmailer(store UsageReportEmailerStore, emailAPI TransactionalEmailSender, fromEmail string, interval time.Duration) *UsageReportEmailer {
	if interval <= 0 {
		interval = time.Hour
	}
	if fromEmail == "" {
		fromEmail = "billing@strait.dev"
	}
	return &UsageReportEmailer{
		store:     store,
		emailAPI:  emailAPI,
		fromEmail: fromEmail,
		interval:  interval,
		logger:    slog.Default(),
	}
}

// Run starts the monthly report email loop. Blocks until ctx is canceled.
func (re *UsageReportEmailer) Run(ctx context.Context) {
	ticker := time.NewTicker(re.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runSchedulerCycleCheckIn(ctx, re.interval, func() {
				re.checkAndSend(context.WithoutCancel(ctx))
			})
		}
	}
}

func (re *UsageReportEmailer) checkAndSend(ctx context.Context) {
	now := time.Now().UTC()
	today := now.Format("2006-01-02")
	if re.lastRunDate == today {
		return
	}

	// Find orgs whose billing period has ended and whose report has not been
	// claimed yet. This intentionally catches up missed scheduler days.
	orgIDs, err := re.store.ListAllSubscribedOrgIDs(ctx)
	if err != nil {
		re.logger.Warn("usage report emailer: failed to list subscribed orgs", "error", err)
		return
	}

	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	completed := true

	for _, orgID := range orgIDs {
		sub, subErr := re.store.GetOrgSubscription(ctx, orgID)
		if subErr != nil {
			completed = false
			continue
		}

		candidate, ok := newUsageReportCandidate(orgID, sub, todayStart)
		if !ok {
			continue
		}

		claimed, dedupErr := re.claimReportSend(ctx, candidate.orgID, candidate.periodEnd)
		if dedupErr != nil {
			re.logger.Warn("usage report emailer: report claim failed",
				"org_id", orgID, "error", dedupErr)
			completed = false
			continue
		}
		if !claimed {
			continue
		}

		if !re.sendReport(ctx, candidate.orgID, candidate.sub) {
			completed = false
			re.releaseReportClaim(ctx, candidate.orgID, candidate.periodEnd)
		}
	}
	if completed {
		re.lastRunDate = today
	}
}

func (re *UsageReportEmailer) claimReportSend(ctx context.Context, orgID string, periodEnd time.Time) (bool, error) {
	if claimStore, ok := re.store.(usageReportClaimStore); ok {
		return claimStore.ClaimUsageReportSend(ctx, orgID, periodEnd)
	}
	alreadySent, err := re.store.HasSentUsageReport(ctx, orgID, periodEnd)
	if err != nil || alreadySent {
		return false, err
	}
	if err := re.store.RecordSentUsageReport(ctx, orgID, periodEnd); err != nil {
		return false, err
	}
	return true, nil
}

func (re *UsageReportEmailer) releaseReportClaim(ctx context.Context, orgID string, periodEnd time.Time) {
	if claimStore, ok := re.store.(usageReportClaimStore); ok {
		if err := claimStore.ReleaseUsageReportSendClaim(ctx, orgID, periodEnd); err != nil {
			re.logger.Warn("usage report emailer: failed to release report claim", "org_id", orgID, "error", err)
		}
	}
}

func (re *UsageReportEmailer) sendReport(ctx context.Context, orgID string, sub *billing.OrgSubscription) bool {
	// Determine the previous period.
	var periodStart, periodEnd time.Time
	if sub.CurrentPeriodStart != nil && sub.CurrentPeriodEnd != nil {
		periodEnd = *sub.CurrentPeriodEnd
		periodStart = *sub.CurrentPeriodStart
	} else {
		return false
	}

	// Generate PDF.
	pdfBytes, err := billing.ExportPDF(ctx, re.store, orgID, billing.ExportPeriod{
		From: periodStart,
		To:   periodEnd,
	})
	if err != nil {
		re.logger.Warn("usage report emailer: failed to generate PDF",
			"org_id", orgID, "error", err)
		return false
	}

	// Get admin emails.
	emails, err := re.store.ListOrgAdminEmails(ctx, orgID)
	if err != nil {
		re.logger.Warn("usage report emailer: failed to list admin emails",
			"org_id", orgID, "error", err)
		return false
	}
	if len(emails) == 0 {
		// Record as sent to avoid retrying every day for orgs with no email recipients.
		_ = re.finalizeReportSent(ctx, orgID, periodEnd)
		re.logger.Debug("usage report emailer: no admin emails for org, skipping",
			"org_id", orgID)
		return true
	}
	if re.emailAPI == nil {
		re.logger.Warn("usage report emailer: transactional email client is not configured",
			"org_id", orgID)
		return false
	}

	// Get plan details and addon info for the report.
	var addonCount int
	if addons, err := re.store.ListActiveAddons(ctx, orgID); err == nil {
		addonCount = len(addons)
	}
	var periodSpend int64
	if usage, err := re.store.GetOrgUsageForPeriod(ctx, orgID, periodStart, periodEnd); err == nil {
		periodSpend = sumUsageRecordSpend(usage)
	}
	overage := max(periodSpend, 0)
	overageAmount := ""
	if overage > 0 {
		overageAmount = fmt.Sprintf("$%.2f", float64(overage)/1_000_000)
	}

	req := transactional.BillingUsageReportRequest(
		emails,
		re.fromEmail,
		orgID,
		sub.PlanTier,
		periodStart,
		periodEnd,
		addonCount,
		overageAmount,
		pdfBytes,
	)

	if err := re.emailAPI.Send(ctx, req); err != nil {
		re.logger.Warn("usage report emailer: failed to send email",
			"org_id", orgID, "error", err)
		return false
	}

	// Record successful send for deduplication.
	if err := re.finalizeReportSent(ctx, orgID, periodEnd); err != nil {
		re.logger.Warn("usage report emailer: failed to record sent report",
			"org_id", orgID, "error", err)
	}

	re.logger.Info("usage report email sent",
		"org_id", orgID,
		"recipients", len(emails),
		"period", fmt.Sprintf("%s to %s", periodStart.Format("2006-01-02"), periodEnd.Format("2006-01-02")),
	)
	return true
}

func (re *UsageReportEmailer) finalizeReportSent(ctx context.Context, orgID string, periodEnd time.Time) error {
	if claimStore, ok := re.store.(usageReportClaimStore); ok {
		return claimStore.FinalizeUsageReportSend(ctx, orgID, periodEnd)
	}
	return re.store.RecordSentUsageReport(ctx, orgID, periodEnd)
}

func sumUsageRecordSpend(records []billing.UsageRecord) int64 {
	var total int64
	for _, record := range records {
		total += record.ComputeCostMicro + record.UsageCostMicro
	}
	return total
}
