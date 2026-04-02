package scheduler

import (
	"context"
	"log/slog"
	"time"

	"strait/internal/billing"
)

// ContractExpiryStore defines the store operations needed by the contract expiry checker.
type ContractExpiryStore interface {
	ListExpiringContracts(ctx context.Context, withinDays int) ([]billing.EnterpriseContract, error)
	ListOrgAdminEmails(ctx context.Context, orgID string) ([]string, error)
}

// ContractExpiryEmailSender sends contract-related reminder emails.
type ContractExpiryEmailSender interface {
	SendEnterpriseContractReminder(ctx context.Context, to []string, contractEndDate string, autoRenew bool, daysRemaining int)
}

// ContractExpiryChecker periodically checks for enterprise contracts
// approaching expiry and sends renewal or expiry reminder emails.
type ContractExpiryChecker struct {
	store    ContractExpiryStore
	emails   ContractExpiryEmailSender
	interval time.Duration
}

// NewContractExpiryChecker creates a new contract expiry checker.
func NewContractExpiryChecker(store ContractExpiryStore, emails ContractExpiryEmailSender, interval time.Duration) *ContractExpiryChecker {
	return &ContractExpiryChecker{
		store:    store,
		emails:   emails,
		interval: interval,
	}
}

// Run starts the periodic contract expiry check loop.
func (c *ContractExpiryChecker) Run(ctx context.Context) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.check(context.WithoutCancel(ctx))
		}
	}
}

func (c *ContractExpiryChecker) check(ctx context.Context) {
	// Check 30-day window for initial reminders.
	c.sendReminders(ctx, 30)
	// Check 7-day window for final reminders.
	c.sendReminders(ctx, 7)
}

func (c *ContractExpiryChecker) sendReminders(ctx context.Context, withinDays int) {
	contracts, err := c.store.ListExpiringContracts(ctx, withinDays)
	if err != nil {
		slog.Warn("contract expiry checker: failed to list expiring contracts",
			"within_days", withinDays, "error", err)
		return
	}

	for _, contract := range contracts {
		daysRemaining := max(int(time.Until(contract.ContractEndDate).Hours()/24), 0)

		emails, emailErr := c.store.ListOrgAdminEmails(ctx, contract.OrgID)
		if emailErr != nil {
			slog.Warn("contract expiry checker: failed to get admin emails",
				"org_id", contract.OrgID, "error", emailErr)
			continue
		}
		if len(emails) == 0 {
			continue
		}

		endDateStr := contract.ContractEndDate.Format("January 2, 2006")

		if c.emails != nil {
			c.emails.SendEnterpriseContractReminder(ctx, emails, endDateStr, contract.AutoRenew, daysRemaining)
		}

		if contract.AutoRenew {
			slog.Info("contract expiry checker: sent renewal reminder",
				"org_id", contract.OrgID,
				"enterprise_tier", contract.EnterpriseTier,
				"days_remaining", daysRemaining,
			)
		} else {
			slog.Warn("contract expiry checker: sent expiry warning",
				"org_id", contract.OrgID,
				"enterprise_tier", contract.EnterpriseTier,
				"days_remaining", daysRemaining,
			)
		}
	}
}
