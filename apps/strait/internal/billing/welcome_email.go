package billing

import (
	"context"
	"fmt"
	"html"
	"strconv"

	"strait/internal/domain"

	"github.com/resend/resend-go/v2"
)

// runAllowanceDisplay returns the user-facing monthly orchestration-run allowance.
func runAllowanceDisplay(tier domain.PlanTier) string {
	limits := GetPlanLimits(tier)
	if tier == domain.PlanEnterprise || limits.MaxRunsPerMonth < 0 {
		return "Custom (per contract)"
	}
	return strconv.FormatInt(int64(limits.MaxRunsPerMonth), 10)
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
	client := resend.NewClient(apiKey)
	return newWelcomeEmailFunc(fromEmail, func(ctx context.Context, req *resend.SendEmailRequest) error {
		_, err := client.Emails.SendWithContext(ctx, req)
		return err
	})
}

type welcomeEmailSendFunc func(context.Context, *resend.SendEmailRequest) error

func newWelcomeEmailFunc(fromEmail string, sendEmail welcomeEmailSendFunc) WelcomeEmailFunc {
	if fromEmail == "" {
		fromEmail = "noreply@strait.dev"
	}

	return func(ctx context.Context, _ string, tier domain.PlanTier, customerEmail string) error {
		if !isValidEmail(customerEmail) {
			return fmt.Errorf("invalid email address: %q", customerEmail)
		}
		name := planDisplayName(tier)
		runAllowance := runAllowanceDisplay(tier)
		subject := fmt.Sprintf("Welcome to Strait %s!", name)

		var body string
		if tier == domain.PlanEnterprise {
			body = enterpriseWelcomeEmailHTML()
		} else {
			body = welcomeEmailHTML(name, runAllowance)
		}

		err := sendEmail(ctx, &resend.SendEmailRequest{
			From:    fromEmail,
			To:      []string{customerEmail},
			Subject: subject,
			Html:    body,
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
func welcomeEmailHTML(planName, monthlyRunAllowance string) string {
	safePlan := html.EscapeString(planName)
	safeRunAllowance := html.EscapeString(monthlyRunAllowance)
	body := fmt.Sprintf(`<tr>
      <td style="font-size:14px;color:#8D8D8D;line-height:1.6;">
        Thank you for upgrading to the %s plan.
      </td>
    </tr>
    <tr><td style="height:16px;"></td></tr>
    <tr>
      <td style="font-size:14px;color:#8D8D8D;line-height:1.6;">
        Your plan includes <strong style="color:#252525;">%s</strong> orchestration runs per month. To control overage beyond your included allowance, we recommend setting a spending cap:
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
    </tr>`, safePlan, safeRunAllowance)
	return billingEmailWrapper(fmt.Sprintf("Welcome to Strait %s!", safePlan), body)
}

// enterpriseWelcomeEmailHTML returns the HTML body for the enterprise plan welcome email.
// Mentions dedicated CSM, onboarding, and contract-specific launch features.
func enterpriseWelcomeEmailHTML() string {
	body := `<tr>
      <td style="font-size:14px;color:#8D8D8D;line-height:1.6;">
        Welcome to Strait Enterprise. Your dedicated Customer Success Manager will reach out within 1 business day to schedule your onboarding session.
      </td>
    </tr>
    <tr><td style="height:16px;"></td></tr>
    <tr>
      <td style="font-size:14px;color:#8D8D8D;line-height:1.6;">
        Your Enterprise launch plan includes:
      </td>
    </tr>
    <tr><td style="height:8px;"></td></tr>
    <tr>
      <td style="font-size:14px;color:#8D8D8D;line-height:1.8;">
        &bull; <strong style="color:#252525;">Custom orchestration run allowance</strong> and overage terms from your contract<br/>
        &bull; <strong style="color:#252525;">Custom concurrency, step caps, and history retention</strong><br/>
        &bull; <strong style="color:#252525;">Consolidated invoicing</strong> for contracted organizations<br/>
        &bull; <strong style="color:#252525;">SLA target and support terms</strong> from your contract<br/>
        &bull; <strong style="color:#252525;">Dedicated Slack channel</strong> for direct engineering support
      </td>
    </tr>
    <tr><td style="height:16px;"></td></tr>
    <tr>
      <td style="font-size:14px;color:#8D8D8D;line-height:1.6;">
        Roadmap and contact-sales items such as SSO/SAML, SCIM, static IPs, VPC peering, data residency, single-tenant orchestration, and BYO-cloud are not launch entitlements unless they are explicitly committed in your contract.
      </td>
    </tr>
    <tr><td style="height:16px;"></td></tr>
    <tr>
      <td>
        <a href="https://app.usestrait.com/app/billing" style="display:inline-block;padding:10px 24px;background-color:#171717;color:#FFFFFF;font-size:14px;font-weight:500;text-decoration:none;border-radius:4px;">
          View billing dashboard
        </a>
      </td>
    </tr>
    <tr><td style="height:16px;"></td></tr>
    <tr>
      <td style="font-size:14px;color:#8D8D8D;line-height:1.6;">
        If you have any questions before your onboarding session, reply to this email or contact us at <a href="mailto:leo@strait.dev" style="color:#171717;text-decoration:underline;">leo@strait.dev</a>.
      </td>
    </tr>`
	return billingEmailWrapper("Welcome to Strait Enterprise!", body)
}
