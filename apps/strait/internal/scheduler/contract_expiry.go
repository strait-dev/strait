package scheduler

import (
	"context"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"strait/internal/billing"
)

// ContractExpiryStore defines the store operations needed by the contract expiry checker.
type ContractExpiryStore interface {
	ListExpiringContracts(ctx context.Context, withinDays int) ([]billing.EnterpriseContract, error)
	ListExpiredContracts(ctx context.Context) ([]billing.EnterpriseContract, error)
	ListOrgAdminEmails(ctx context.Context, orgID string) ([]string, error)
	UpdatePaymentStatus(ctx context.Context, orgID string, status string, graceEnd *time.Time) error
}

// ContractExpiredEmailSender sends contract expired notifications.
type ContractExpiredEmailSender interface {
	SendContractExpired(ctx context.Context, to []string, contractEndDate string)
}

// ContractExpiryEmailSender sends contract-related reminder emails.
type ContractExpiryEmailSender interface {
	SendEnterpriseContractReminder(ctx context.Context, to []string, contractEndDate string, autoRenew bool, daysRemaining int)
}

type contractReminderClaimStore interface {
	ClaimContractReminderSend(ctx context.Context, orgID string, contractEndDate time.Time, reminderWindowDays int) (bool, error)
}

// ContractExpiryChecker periodically checks for enterprise contracts
// approaching expiry and sends renewal or expiry reminder emails.
type ContractExpiryChecker struct {
	store      ContractExpiryStore
	emails     ContractExpiryEmailSender
	interval   time.Duration
	reminderMu sync.Mutex
	reminders  map[string]struct{}
}

// NewContractExpiryChecker creates a new contract expiry checker.
func NewContractExpiryChecker(store ContractExpiryStore, emails ContractExpiryEmailSender, interval time.Duration) *ContractExpiryChecker {
	return &ContractExpiryChecker{
		store:     store,
		emails:    emails,
		interval:  interval,
		reminders: make(map[string]struct{}),
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
			runSchedulerCycleCheckIn(ctx, c.interval, func() {
				c.check(context.WithoutCancel(ctx))
			})
		}
	}
}

func (c *ContractExpiryChecker) check(ctx context.Context) {
	// Check 30-day window for initial reminders.
	c.sendReminders(ctx, 30)
	// Check 7-day window for final reminders.
	c.sendReminders(ctx, 7)
	// Restrict orgs whose non-renewing contracts have expired.
	c.restrictExpiredContracts(ctx)
}

// restrictExpiredContracts finds contracts that have already expired (end_date in the past)
// with AutoRenew=false and sets the org's payment_status to "restricted".
func (c *ContractExpiryChecker) restrictExpiredContracts(ctx context.Context) {
	contracts, err := c.store.ListExpiredContracts(ctx)
	if err != nil {
		return
	}

	for _, contract := range contracts {
		if contract.AutoRenew {
			continue
		}

		if err := c.store.UpdatePaymentStatus(ctx, contract.OrgID, "restricted", nil); err != nil {
			slog.Warn("contract expiry: failed to restrict org",
				"org_id", contract.OrgID, "error", err)
			continue
		}

		slog.Warn("contract expired, org restricted",
			"org_id", contract.OrgID,
			"contract_end", contract.ContractEndDate,
		)
	}
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
		if withinDays == 30 && daysRemaining <= 7 {
			continue
		}
		if c.reminderAlreadySent(contract, withinDays) {
			continue
		}

		emails, emailErr := c.store.ListOrgAdminEmails(ctx, contract.OrgID)
		if emailErr != nil {
			slog.Warn("contract expiry checker: failed to get admin emails",
				"org_id", contract.OrgID, "error", emailErr)
			continue
		}
		if len(emails) == 0 {
			continue
		}

		claimed, claimErr := c.claimReminderSend(ctx, contract, withinDays)
		if claimErr != nil {
			slog.Warn("contract expiry checker: failed to claim reminder send",
				"org_id", contract.OrgID, "within_days", withinDays, "error", claimErr)
			continue
		}
		if !claimed {
			continue
		}

		endDateStr := contract.ContractEndDate.Format("January 2, 2006")

		if c.emails != nil {
			c.emails.SendEnterpriseContractReminder(ctx, emails, endDateStr, contract.AutoRenew, daysRemaining)
		}
		c.markReminderSent(contract, withinDays)

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

func (c *ContractExpiryChecker) reminderAlreadySent(contract billing.EnterpriseContract, withinDays int) bool {
	c.reminderMu.Lock()
	defer c.reminderMu.Unlock()
	_, ok := c.reminders[contractReminderKey(contract, withinDays)]
	return ok
}

func (c *ContractExpiryChecker) claimReminderSend(ctx context.Context, contract billing.EnterpriseContract, withinDays int) (bool, error) {
	if claimStore, ok := c.store.(contractReminderClaimStore); ok {
		return claimStore.ClaimContractReminderSend(ctx, contract.OrgID, contract.ContractEndDate, withinDays)
	}
	if c.reminderAlreadySent(contract, withinDays) {
		return false, nil
	}
	return true, nil
}

func (c *ContractExpiryChecker) markReminderSent(contract billing.EnterpriseContract, withinDays int) {
	c.reminderMu.Lock()
	defer c.reminderMu.Unlock()
	c.reminders[contractReminderKey(contract, withinDays)] = struct{}{}
}

func contractReminderKey(contract billing.EnterpriseContract, withinDays int) string {
	return contract.OrgID + ":" + contract.ContractEndDate.UTC().Format(time.RFC3339) + ":" + strconv.Itoa(withinDays)
}
