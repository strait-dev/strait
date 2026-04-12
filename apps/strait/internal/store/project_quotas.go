package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
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

// GetAuditExportRowCap returns the per-project override for the maximum
// number of rows a single audit export may stream. A return value of 0
// means "no override" — the caller must fall back to the server-wide
// default (config.AuditExportRowCapDefault).
//
// Absent rows (projects with no project_quotas entry) yield 0 as well,
// so a project without any quota row is indistinguishable from a project
// that explicitly inherits the default.
func (q *Queries) GetAuditExportRowCap(ctx context.Context, projectID string) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetAuditExportRowCap")
	defer span.End()

	var cap int64
	err := q.db.QueryRow(ctx, `
		SELECT audit_export_row_cap
		FROM project_quotas
		WHERE project_id = $1
	`, projectID).Scan(&cap)
	if err != nil {
		// No row yet — treat as "inherit default".
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}
		return 0, fmt.Errorf("get audit export row cap: %w", err)
	}
	return cap, nil
}

// SetAuditExportRowCap persists a per-project override for the audit
// export row cap. 0 means "inherit the server default". Negative values
// are rejected by the API handler before this is called — the store
// enforces no extra validation.
//
// Upserts into project_quotas so a project without any prior quota row
// still gets its cap recorded. Every other column keeps its default.
func (q *Queries) SetAuditExportRowCap(ctx context.Context, projectID string, cap int64) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.SetAuditExportRowCap")
	defer span.End()

	_, err := q.db.Exec(ctx, `
		INSERT INTO project_quotas (project_id, audit_export_row_cap)
		VALUES ($1, $2)
		ON CONFLICT (project_id) DO UPDATE SET audit_export_row_cap = EXCLUDED.audit_export_row_cap
	`, projectID, cap)
	if err != nil {
		return fmt.Errorf("set audit export row cap: %w", err)
	}
	return nil
}
