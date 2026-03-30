package billing

import (
	"context"
	"fmt"
	"html"

	"strait/internal/domain"

	"github.com/resend/resend-go/v2"
)

// creditDisplayUSD returns a human-readable dollar string for a plan's included credit.
func creditDisplayUSD(tier domain.PlanTier) string {
	switch tier {
	case domain.PlanStarter:
		return "$19.99"
	case domain.PlanPro:
		return "$49.99"
	case domain.PlanScale:
		return "$99.00"
	default:
		return "$0.00"
	}
}

// planDisplayName returns the user-facing name for a plan tier.
func planDisplayName(tier domain.PlanTier) string {
	switch tier {
	case domain.PlanStarter:
		return "Starter"
	case domain.PlanPro:
		return "Pro"
	case domain.PlanScale:
		return "Scale"
	case domain.PlanEnterprise:
		return "Enterprise"
	default:
		return "Free"
	}
}

// NewResendWelcomeEmailFunc creates a WelcomeEmailFunc that sends a welcome
// email via Resend when a user subscribes to a paid plan.
func NewResendWelcomeEmailFunc(apiKey, fromEmail string) WelcomeEmailFunc {
	if fromEmail == "" {
		fromEmail = "noreply@strait.dev"
	}
	client := resend.NewClient(apiKey)

	return func(ctx context.Context, _ string, tier domain.PlanTier, customerEmail string) error {
		if !isValidEmail(customerEmail) {
			return fmt.Errorf("invalid email address: %q", customerEmail)
		}
		name := planDisplayName(tier)
		credit := creditDisplayUSD(tier)
		subject := fmt.Sprintf("Welcome to Strait %s!", name)

		html := welcomeEmailHTML(name, credit)

		_, err := client.Emails.SendWithContext(ctx, &resend.SendEmailRequest{
			From:    fromEmail,
			To:      []string{customerEmail},
			Subject: subject,
			Html:    html,
		})
		if err != nil {
			return fmt.Errorf("send welcome email via resend: %w", err)
		}
		return nil
	}
}

// welcomeEmailHTML returns the HTML body for the paid plan welcome email.
// This mirrors the React Email template in packages/transactional but is
// rendered server-side as a static string to avoid a Node.js dependency.
func welcomeEmailHTML(planName, includedCredit string) string {
	safePlan := html.EscapeString(planName)
	safeCredit := html.EscapeString(includedCredit)
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
        Welcome to Strait %s!
      </td>
    </tr>
    <tr><td style="height:16px;"></td></tr>
    <tr>
      <td style="font-size:14px;color:#8D8D8D;line-height:1.6;">
        Thank you for upgrading to the %s plan.
      </td>
    </tr>
    <tr><td style="height:16px;"></td></tr>
    <tr>
      <td style="font-size:14px;color:#8D8D8D;line-height:1.6;">
        Your plan includes <strong style="color:#252525;">%s</strong> in monthly compute credits. To control costs beyond your included credit, we recommend setting a spending limit:
      </td>
    </tr>
    <tr><td style="height:16px;"></td></tr>
    <tr>
      <td>
        <a href="https://app.usestrait.com/app/settings/billing" style="display:inline-block;padding:10px 24px;background-color:#171717;color:#FFFFFF;font-size:14px;font-weight:500;text-decoration:none;border-radius:4px;">
          Set spending limit
        </a>
      </td>
    </tr>
    <tr><td style="height:16px;"></td></tr>
    <tr>
      <td style="font-size:14px;color:#8D8D8D;line-height:1.6;">
        Here are some things you can do now:
      </td>
    </tr>
    <tr><td style="height:8px;"></td></tr>
    <tr>
      <td style="font-size:14px;color:#8D8D8D;line-height:1.8;">
        &bull; View your <a href="https://app.usestrait.com/app/settings/billing" style="color:#171717;text-decoration:underline;">billing dashboard</a><br/>
        &bull; Explore your <a href="https://app.usestrait.com/app/workflows" style="color:#171717;text-decoration:underline;">workflows</a><br/>
        &bull; Monitor your <a href="https://app.usestrait.com/app/runs" style="color:#171717;text-decoration:underline;">runs and events</a>
      </td>
    </tr>
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
</html>`, safePlan, safePlan, safeCredit)
}
