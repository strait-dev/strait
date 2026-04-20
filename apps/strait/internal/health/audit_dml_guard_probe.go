package health

import (
	"context"
	"fmt"
)

// AuditDMLPrivilegeChecker reports whether UPDATE on audit_events is
// restricted for the current database role. Implemented by *store.Queries
// via SELECT has_table_privilege(current_user, 'audit_events', 'UPDATE').
//
// When UPDATE is not restricted for any column other than `signature`,
// the DML guardrail from migration 000187 is a silent no-op — a compromised
// application process could rewrite audit rows without detection. The
// guard probe reports degraded in that case so oncall can page even
// though the service is still functional.
type AuditDMLPrivilegeChecker interface {
	// AuditEventsUpdateRestricted returns true when the DML guard is in
	// effect (UPDATE limited to the signature column), false when the
	// current role has full UPDATE. A nil error with false is a valid
	// "unrestricted" outcome — it is not a probe failure.
	AuditEventsUpdateRestricted(ctx context.Context) (bool, error)
}

// NewAuditDMLGuardProbe returns a non-critical Checker that verifies the
// audit_events DML restriction landed for the current database role. On
// self-hosted installs that do not provision the strait_app role, the
// migration is a no-op — this probe surfaces that as a degraded signal
// so operators know the tamper-evident UPDATE restriction is not being
// enforced at the role level (the HMAC chain still detects tampering
// forensically; the role restriction is defense-in-depth).
//
// The probe is advisory (non-critical) — missing restrictions should not
// take the service down, but should page oncall and block SOC 2 evidence
// gates that require the restriction to be enforced.
func NewAuditDMLGuardProbe(checker AuditDMLPrivilegeChecker) Checker {
	return NewCriticalChecker("audit_dml_guard", false, func(ctx context.Context) error {
		if checker == nil {
			return nil
		}
		restricted, err := checker.AuditEventsUpdateRestricted(ctx)
		if err != nil {
			return fmt.Errorf("audit DML privilege probe failed: %w", err)
		}
		if !restricted {
			return fmt.Errorf("audit_events UPDATE is not restricted for current role; migration 000187 is a no-op on this install — see SELFHOST.md for the strait_app role prerequisite")
		}
		return nil
	})
}
