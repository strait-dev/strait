package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"strait/internal/billing"
	"strait/internal/domain"

	"github.com/resend/resend-go/v2"
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

// ResendEmailSender is the subset of the Resend client used for sending emails.
type ResendEmailSender interface {
	SendWithContext(ctx context.Context, params *resend.SendEmailRequest) (*resend.SendEmailResponse, error)
}

// UsageReportEmailer sends monthly PDF usage reports to org admins after their
// billing period ends. Runs daily and catches up any ended period that has not
// been claimed yet.
type UsageReportEmailer struct {
	store     UsageReportEmailerStore
	emailAPI  ResendEmailSender
	fromEmail string
	interval  time.Duration
	logger    *slog.Logger
	// lastRunDate prevents running more than once per day.
	lastRunDate string
}

// NewUsageReportEmailer creates a new monthly usage report emailer.
func NewUsageReportEmailer(store UsageReportEmailerStore, emailAPI ResendEmailSender, fromEmail string, interval time.Duration) *UsageReportEmailer {
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

		// Only send for paid plans.
		tier := billing.GetPlanLimits(domain.PlanTier(sub.PlanTier))
		if tier.PriceMonthlyUsd == 0 && tier.PriceAnnualUsd == 0 {
			continue // free or enterprise with custom billing
		}

		// Check opt-in preference.
		if !sub.MonthlyUsageEmail {
			continue
		}

		// Check if the billing period ended before today. Same-day periods are
		// left for tomorrow so usage aggregation has settled.
		if sub.CurrentPeriodEnd == nil {
			continue
		}
		periodEnd := sub.CurrentPeriodEnd.UTC().Truncate(24 * time.Hour)
		if !periodEnd.Before(todayStart) {
			continue
		}

		claimed, dedupErr := re.claimReportSend(ctx, orgID, periodEnd)
		if dedupErr != nil {
			re.logger.Warn("usage report emailer: report claim failed",
				"org_id", orgID, "error", dedupErr)
			completed = false
			continue
		}
		if !claimed {
			continue
		}

		if !re.sendReport(ctx, orgID, sub) {
			completed = false
			re.releaseReportClaim(ctx, orgID, periodEnd)
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

	filename := fmt.Sprintf("strait-usage-%s-to-%s.pdf",
		periodStart.Format("2006-01-02"),
		periodEnd.Format("2006-01-02"))

	subject := fmt.Sprintf("Your Strait usage report: %s to %s",
		periodStart.Format("Jan 2"),
		periodEnd.Format("Jan 2, 2006"))

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

	htmlBody := buildUsageReportHTML(orgID, sub.PlanTier, periodStart, periodEnd, 0, addonCount, overage)

	req := &resend.SendEmailRequest{
		From:    re.fromEmail,
		To:      emails,
		Subject: subject,
		Html:    htmlBody,
		Attachments: []*resend.Attachment{
			{
				Filename: filename,
				Content:  pdfBytes,
			},
		},
	}

	if _, err := re.emailAPI.SendWithContext(ctx, req); err != nil {
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

func buildUsageReportHTML(orgID, planTier string, periodStart, periodEnd time.Time, creditMicro int64, addonCount int, overageMicro int64) string {
	_ = creditMicro
	overageUsd := fmt.Sprintf("$%.2f", float64(overageMicro)/1_000_000)

	addonLine := ""
	if addonCount > 0 {
		addonLine = fmt.Sprintf(`<p><strong>Active add-ons:</strong> %d pack(s)</p>`, addonCount)
	}

	overageLine := ""
	if overageMicro > 0 {
		overageLine = fmt.Sprintf(`<p><strong>Overage:</strong> %s beyond the included run allowance</p>`, overageUsd)
	}

	return fmt.Sprintf(`<div style="font-family:sans-serif;max-width:600px;margin:0 auto">
<h2>Monthly Usage Report</h2>
<p>Here is your usage summary for <strong>%s</strong> (%s plan).</p>
<p>Period: %s to %s</p>
<p><strong>Included allowance:</strong> metered orchestration runs for this billing period</p>
%s%s<p>Your detailed usage report is attached as a PDF.</p>
<p>To manage your billing and spending limits, visit your <a href="https://app.strait.dev/app/billing">billing settings</a>.</p>
<hr style="border:none;border-top:1px solid #e0e0e0;margin:20px 0">
<p style="font-size:12px;color:#666">This is an automated email from Strait. You can disable monthly reports in your organization settings.</p>
</div>`, orgID, planTier, periodStart.Format("Jan 2"), periodEnd.Format("Jan 2, 2006"),
		addonLine, overageLine)
}
