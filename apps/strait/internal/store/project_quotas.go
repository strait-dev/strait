package store

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
)

// AuditRetentionOverride is a per-project override of the server-wide
// audit retention window. Populated from project_quotas.audit_retention_days.
//
// Days > 0 means "retain for N days, overriding the default". Days == 0 is
// interpreted at the scheduler layer (see scheduler.reapAuditEvents) as
// "disable retention trimming for this project entirely".
type AuditRetentionOverride struct {
	ProjectID string
	Days      int
}

// ListAuditRetentionOverrides returns every project that has a non-default
// audit_retention_days set in project_quotas. Projects without a row (or
// with the default value) are left to the server-wide default applied by
// the reaper.
func (q *Queries) ListAuditRetentionOverrides(ctx context.Context) ([]AuditRetentionOverride, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListAuditRetentionOverrides")
	defer span.End()

	rows, err := q.db.Query(ctx, `
		SELECT project_id, audit_retention_days
		FROM project_quotas
		WHERE audit_retention_days IS NOT NULL
	`)
	if err != nil {
		return nil, fmt.Errorf("list audit retention overrides: %w", err)
	}
	defer rows.Close()

	var out []AuditRetentionOverride
	for rows.Next() {
		var ov AuditRetentionOverride
		if err := rows.Scan(&ov.ProjectID, &ov.Days); err != nil {
			return nil, fmt.Errorf("scan audit retention override: %w", err)
		}
		out = append(out, ov)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate audit retention overrides: %w", err)
	}
	return out, nil
}
