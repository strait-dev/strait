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
// Days > 0 means "retain for N days, overriding the default". Days == 0 with
// audit_retention_override_set=true is interpreted at the scheduler layer
// (see scheduler.reapAuditEvents) as "disable retention trimming for this
// project entirely". Default quota rows have override_set=false and inherit
// the server-wide default.
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
		WHERE audit_retention_override_set = TRUE
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

// GetAuditRetentionDays returns the per-project audit retention override
// in project_quotas.audit_retention_days. The second return value reports
// whether an explicit override row is present:
//
//   - (N, true, nil)  — project has an explicit override (including N = 0,
//     which means "disable retention trimming for this project").
//   - (0, false, nil) — no override row exists; the caller should fall back
//     to config.AuditRetentionDefaultDays.
//
// The distinction matters: an explicit 0 must NOT be confused with absence.
// An absent row means "inherit the default"; an explicit 0 means "never
// trim" and the reaper honors that by skipping the project (Phase 2).
func (q *Queries) GetAuditRetentionDays(ctx context.Context, projectID string) (int, bool, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetAuditRetentionDays")
	defer span.End()

	var days int
	var overrideSet bool
	err := q.db.QueryRow(ctx, `
		SELECT audit_retention_days, audit_retention_override_set
		FROM project_quotas
		WHERE project_id = $1
	`, projectID).Scan(&days, &overrideSet)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("get audit retention days: %w", err)
	}
	if !overrideSet {
		// Row exists for unrelated quota settings, but no retention override
		// was explicitly configured.
		return 0, false, nil
	}
	return days, true, nil
}

// SetAuditRetentionDays persists a per-project audit retention override.
// A value of 0 is a legitimate, meaningful setting: it tells the reaper
// to skip the project entirely. Negative values are rejected by the API
// handler before this method is called — the store performs no extra
// validation.
//
// Upserts into project_quotas so a project without any prior quota row
// still gets its retention recorded without disturbing other columns.
func (q *Queries) SetAuditRetentionDays(ctx context.Context, projectID string, days int) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.SetAuditRetentionDays")
	defer span.End()

	_, err := q.db.Exec(ctx, `
		INSERT INTO project_quotas (project_id, audit_retention_days, audit_retention_override_set)
		VALUES ($1, $2, TRUE)
		ON CONFLICT (project_id) DO UPDATE SET
			audit_retention_days = EXCLUDED.audit_retention_days,
			audit_retention_override_set = TRUE
	`, projectID, days)
	if err != nil {
		return fmt.Errorf("set audit retention days: %w", err)
	}
	return nil
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

	var rowCap int64
	err := q.db.QueryRow(ctx, `
		SELECT audit_export_row_cap
		FROM project_quotas
		WHERE project_id = $1
	`, projectID).Scan(&rowCap)
	if err != nil {
		// No row yet — treat as "inherit default".
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}
		return 0, fmt.Errorf("get audit export row cap: %w", err)
	}
	return rowCap, nil
}

// SetAuditExportRowCap persists a per-project override for the audit
// export row cap. 0 means "inherit the server default". Negative values
// are rejected by the API handler before this is called — the store
// enforces no extra validation.
//
// Upserts into project_quotas so a project without any prior quota row
// still gets its cap recorded. Every other column keeps its default.
func (q *Queries) SetAuditExportRowCap(ctx context.Context, projectID string, rowCap int64) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.SetAuditExportRowCap")
	defer span.End()

	_, err := q.db.Exec(ctx, `
		INSERT INTO project_quotas (project_id, audit_export_row_cap)
		VALUES ($1, $2)
		ON CONFLICT (project_id) DO UPDATE SET audit_export_row_cap = EXCLUDED.audit_export_row_cap
	`, projectID, rowCap)
	if err != nil {
		return fmt.Errorf("set audit export row cap: %w", err)
	}
	return nil
}
