package health

import (
	"context"
	"fmt"
)

// AuditDeadletterCounter reports the current row count in the audit
// events deadletter table. Implemented by *store.Queries in production.
type AuditDeadletterCounter interface {
	CountAuditEventsDeadletter(ctx context.Context) (int64, error)
}

// NewAuditProbe returns a Checker that degrades health when the audit
// deadletter table is non-empty. The deadletter contains audit events
// that failed to write to the primary audit_events table after
// exhausting in-memory retries — any row is a compliance signal and
// oncall should investigate.
//
// Alerting rules for this probe live in strait-dev/infra RUNBOOK.md
// under the audit-emit-health section. The health endpoint reports
// degraded (not down) on a non-empty deadletter, so ops can page
// without taking the service out.
func NewAuditProbe(store AuditDeadletterCounter) Checker {
	return NewChecker("audit_emit_health", func(ctx context.Context) error {
		if store == nil {
			return nil
		}
		n, err := store.CountAuditEventsDeadletter(ctx)
		if err != nil {
			return fmt.Errorf("audit deadletter count query failed: %w", err)
		}
		if n > 0 {
			return fmt.Errorf("audit deadletter has %d unreplayed row(s); investigate", n)
		}
		return nil
	})
}
