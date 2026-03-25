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

// ResendEmailSender is the subset of the Resend client used for sending emails.
type ResendEmailSender interface {
	SendWithContext(ctx context.Context, params *resend.SendEmailRequest) (*resend.SendEmailResponse, error)
}

// UsageReportEmailer sends monthly PDF usage reports to org admins when their
// billing period ends. Runs daily and checks for orgs whose period ended yesterday.
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
			re.checkAndSend(context.WithoutCancel(ctx))
		}
	}
}

func (re *UsageReportEmailer) checkAndSend(ctx context.Context) {
	today := time.Now().UTC().Format("2006-01-02")
	if re.lastRunDate == today {
		return
	}
	re.lastRunDate = today

	// Find orgs whose billing period ended yesterday.
	orgIDs, err := re.store.ListAllSubscribedOrgIDs(ctx)
	if err != nil {
		re.logger.Warn("usage report emailer: failed to list subscribed orgs", "error", err)
		return
	}

	yesterday := time.Now().UTC().Add(-24 * time.Hour).Truncate(24 * time.Hour)

	for _, orgID := range orgIDs {
		sub, subErr := re.store.GetOrgSubscription(ctx, orgID)
		if subErr != nil {
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

		// Check if the billing period ended yesterday.
		if sub.CurrentPeriodEnd == nil {
			continue
		}
		periodEnd := sub.CurrentPeriodEnd.UTC().Truncate(24 * time.Hour)
		if !periodEnd.Equal(yesterday) {
			continue
		}

		// Dedup: skip if already sent for this period.
		alreadySent, dedupErr := re.store.HasSentUsageReport(ctx, orgID, periodEnd)
		if dedupErr != nil {
			re.logger.Warn("usage report emailer: dedup check failed",
				"org_id", orgID, "error", dedupErr)
			continue
		}
		if alreadySent {
			continue
		}

		re.sendReport(ctx, orgID, sub)
	}
}

func (re *UsageReportEmailer) sendReport(ctx context.Context, orgID string, sub *billing.OrgSubscription) {
	// Determine the previous period.
	var periodStart, periodEnd time.Time
	if sub.CurrentPeriodStart != nil && sub.CurrentPeriodEnd != nil {
		periodEnd = *sub.CurrentPeriodEnd
		periodStart = *sub.CurrentPeriodStart
	} else {
		return
	}

	// Generate PDF.
	pdfBytes, err := billing.ExportPDF(ctx, re.store, orgID, billing.ExportPeriod{
		From: periodStart,
		To:   periodEnd,
	})
	if err != nil {
		re.logger.Warn("usage report emailer: failed to generate PDF",
			"org_id", orgID, "error", err)
		return
	}

	// Get admin emails.
	emails, err := re.store.ListOrgAdminEmails(ctx, orgID)
	if err != nil {
		re.logger.Warn("usage report emailer: failed to list admin emails",
			"org_id", orgID, "error", err)
		return
	}
	if len(emails) == 0 {
		// Record as sent to avoid retrying every day for orgs with no email recipients.
		_ = re.store.RecordSentUsageReport(ctx, orgID, periodEnd)
		re.logger.Debug("usage report emailer: no admin emails for org, skipping",
			"org_id", orgID)
		return
	}

	filename := fmt.Sprintf("strait-usage-%s-to-%s.pdf",
		periodStart.Format("2006-01-02"),
		periodEnd.Format("2006-01-02"))

	subject := fmt.Sprintf("Your Strait usage report: %s to %s",
		periodStart.Format("Jan 2"),
		periodEnd.Format("Jan 2, 2006"))

	htmlBody := buildUsageReportHTML(orgID, sub.PlanTier, periodStart, periodEnd)

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
		return
	}

	// Record successful send for deduplication.
	if err := re.store.RecordSentUsageReport(ctx, orgID, periodEnd); err != nil {
		re.logger.Warn("usage report emailer: failed to record sent report",
			"org_id", orgID, "error", err)
	}

	re.logger.Info("usage report email sent",
		"org_id", orgID,
		"recipients", len(emails),
		"period", fmt.Sprintf("%s to %s", periodStart.Format("2006-01-02"), periodEnd.Format("2006-01-02")),
	)
}

func buildUsageReportHTML(orgID, planTier string, periodStart, periodEnd time.Time) string {
	return fmt.Sprintf(`<div style="font-family:sans-serif;max-width:600px;margin:0 auto">
<h2>Monthly Usage Report</h2>
<p>Here is your usage summary for <strong>%s</strong> (%s plan).</p>
<p>Period: %s to %s</p>
<p>Your detailed usage report is attached as a PDF.</p>
<p>To manage your billing and spending limits, visit your <a href="https://app.strait.dev/settings/billing">billing settings</a>.</p>
<hr style="border:none;border-top:1px solid #e0e0e0;margin:20px 0">
<p style="font-size:12px;color:#666">This is an automated email from Strait. You can disable monthly reports in your organization settings.</p>
</div>`, orgID, planTier, periodStart.Format("Jan 2"), periodEnd.Format("Jan 2, 2006"))
}
