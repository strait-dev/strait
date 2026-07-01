package billing

import (
	"context"
	"fmt"
	"strconv"

	"strait/internal/domain"
	"strait/internal/transactional"
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

// NewTransactionalWelcomeEmailFunc creates a WelcomeEmailFunc that asks
// apps/app to render and send the paid-plan welcome email.
func NewTransactionalWelcomeEmailFunc(client TransactionalEmailClient, fromEmail string) WelcomeEmailFunc {
	if client == nil {
		return nil
	}
	if fromEmail == "" {
		fromEmail = "noreply@strait.dev"
	}

	return func(ctx context.Context, orgID string, tier domain.PlanTier, customerEmail string) error {
		if !isValidEmail(customerEmail) {
			return fmt.Errorf("invalid email address: %q", customerEmail)
		}

		template := "billing.paid_plan_welcome"
		props := map[string]any{
			"name":                "",
			"planName":            planDisplayName(tier),
			"monthlyRunAllowance": runAllowanceDisplay(tier),
		}
		if tier == domain.PlanEnterprise {
			template = "billing.enterprise_welcome"
			props = map[string]any{}
		}

		err := client.Send(ctx, transactional.Request{
			Template:       template,
			To:             []string{customerEmail},
			From:           fromEmail,
			IdempotencyKey: fmt.Sprintf("billing:welcome:%s:%s:%s", orgID, tier, customerEmail),
			Props:          props,
		})
		if err != nil {
			return fmt.Errorf("send welcome email through transactional endpoint: %w", err)
		}
		return nil
	}
}
