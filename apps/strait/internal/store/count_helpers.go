package store

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
)

// CountCronJobsByOrg counts scheduled jobs and workflows with a non-empty cron
// expression across all projects belonging to the given org.
func (q *Queries) CountCronJobsByOrg(ctx context.Context, orgID string) (int, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CountCronJobsByOrg")
	defer span.End()

	var count int
	err := q.db.QueryRow(ctx, `
		WITH active_projects AS (
			SELECT id FROM projects WHERE org_id = $1 AND deleted_at IS NULL
		)
		SELECT
			(SELECT COUNT(*) FROM jobs WHERE project_id IN (SELECT id FROM active_projects) AND cron IS NOT NULL AND cron != '') +
			(SELECT COUNT(*) FROM workflows WHERE project_id IN (SELECT id FROM active_projects) AND cron IS NOT NULL AND cron != '')
	`, orgID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count cron schedules by org: %w", err)
	}
	return count, nil
}

// CountEnvironmentsByOrg counts environments across all active projects in an org.
func (q *Queries) CountEnvironmentsByOrg(ctx context.Context, orgID string) (int, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CountEnvironmentsByOrg")
	defer span.End()

	var count int
	err := q.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM environments
		WHERE project_id IN (SELECT id FROM projects WHERE org_id = $1 AND deleted_at IS NULL)
	`, orgID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count environments by org: %w", err)
	}
	return count, nil
}

// CountEnvironmentsByProject counts environments belonging to a project.
func (q *Queries) CountEnvironmentsByProject(ctx context.Context, projectID string) (int, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CountEnvironmentsByProject")
	defer span.End()

	var count int
	err := q.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM environments
		WHERE project_id = $1
	`, projectID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count environments by project: %w", err)
	}
	return count, nil
}

// DeleteRunsByOrgOlderThan is the soft-delete replacement for the
// previous physical DELETE. Instead of creating dead tuples scattered across
// every partition, it sets visible_until = NOW() which is HOT-update
// eligible (the column is intentionally not indexed). pg_partman remains
// the authoritative physical reaper via partition drop.
//
// Returns the number of rows marked invisible. The method name is kept for
// API stability; the external contract (rows disappear from user-facing
// listings after the call) is preserved as long as readers filter by the
// MaskedClause fragment.
func (q *Queries) DeleteRunsByOrgOlderThan(ctx context.Context, orgID string, retention time.Duration) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteRunsByOrgOlderThan")
	defer span.End()

	cutoff := time.Now().Add(-retention)
	result, err := q.db.Exec(ctx, `
		WITH to_mask AS (
			SELECT jr.id FROM job_runs jr
			JOIN jobs j ON jr.job_id = j.id
			LEFT JOIN job_run_read_state s ON s.run_id = jr.id
			WHERE j.project_id IN (SELECT id FROM projects WHERE org_id = $1 AND deleted_at IS NULL)
			  AND COALESCE(s.status, jr.status) IN ('completed', 'failed', 'canceled', 'timed_out')
			  AND COALESCE(s.finished_at, jr.finished_at) IS NOT NULL
			  AND COALESCE(s.finished_at, jr.finished_at) < $2
			  AND jr.visible_until IS NULL
			LIMIT 1000
		)
		UPDATE job_runs
		SET visible_until = NOW()
		WHERE id IN (SELECT id FROM to_mask)
	`, orgID, cutoff)
	if err != nil {
		return 0, fmt.Errorf("mask runs by org older than %v: %w", retention, err)
	}
	return result.RowsAffected(), nil
}

// VisibleRunsClause is the shared SQL fragment that readers append to their
// WHERE clause to hide soft-deleted rows. Leave out if the query is an
// internal reaper scan that needs to see masked rows.
const VisibleRunsClause = "(visible_until IS NULL OR visible_until > NOW())"

// DeleteWorkflowRunsByOrgOlderThan deletes terminal workflow runs for an org that
// are older than the given retention duration.
func (q *Queries) DeleteWorkflowRunsByOrgOlderThan(ctx context.Context, orgID string, retention time.Duration) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteWorkflowRunsByOrgOlderThan")
	defer span.End()

	cutoff := time.Now().Add(-retention)
	result, err := q.db.Exec(ctx, `
		DELETE FROM workflow_runs
		WHERE id IN (
			SELECT wr.id FROM workflow_runs wr
			JOIN workflows w ON wr.workflow_id = w.id
			WHERE w.project_id IN (SELECT id FROM projects WHERE org_id = $1 AND deleted_at IS NULL)
			  AND wr.status IN ('completed', 'failed', 'timed_out', 'canceled', 'compensated', 'compensation_failed')
			  AND wr.finished_at IS NOT NULL
			  AND wr.finished_at < $2
			LIMIT 1000
		)
	`, orgID, cutoff)
	if err != nil {
		return 0, fmt.Errorf("delete workflow runs by org older than %v: %w", retention, err)
	}
	return result.RowsAffected(), nil
}

// DeactivateExcessEnvironments marks excess environments as deleted for an org.
// Keeps the most recently created environments up to the limit, deactivating the oldest.
func (q *Queries) DeactivateExcessEnvironments(ctx context.Context, orgID string, maxEnvironments int) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeactivateExcessEnvironments")
	defer span.End()

	// ORDER BY created_at DESC keeps the newest environments (first N rows after OFFSET
	// are skipped). The subquery returns oldest environments beyond the limit.
	result, err := q.db.Exec(ctx, `
		DELETE FROM environments
		WHERE id IN (
			SELECT e.id FROM environments e
			WHERE e.project_id IN (SELECT id FROM projects WHERE org_id = $1 AND deleted_at IS NULL)
			  AND e.is_standard = false
			ORDER BY e.created_at DESC
			OFFSET $2
		)
	`, orgID, maxEnvironments)
	if err != nil {
		return 0, fmt.Errorf("deactivate excess environments: %w", err)
	}
	return result.RowsAffected(), nil
}

// DeactivateExcessCronJobs disables cron jobs and workflows beyond the given
// shared schedule limit for an org. Keeps the most recently updated schedules
// and clears the cron field on the oldest excess ones. Returns the IDs whose
// cron was cleared.
func (q *Queries) DeactivateExcessCronJobs(ctx context.Context, orgID string, maxSchedules int) ([]string, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeactivateExcessCronJobs")
	defer span.End()

	rows, err := q.db.Query(ctx, `
		WITH active_projects AS (
			SELECT id FROM projects WHERE org_id = $1 AND deleted_at IS NULL
		),
		ranked_schedules AS (
			SELECT 'job' AS kind, j.id, j.updated_at
			FROM jobs j
			WHERE j.project_id IN (SELECT id FROM active_projects)
			  AND j.cron IS NOT NULL AND j.cron != ''
			UNION ALL
			SELECT 'workflow' AS kind, w.id, w.updated_at
			FROM workflows w
			WHERE w.project_id IN (SELECT id FROM active_projects)
			  AND w.cron IS NOT NULL AND w.cron != ''
		),
		excess_schedules AS (
			SELECT kind, id
			FROM ranked_schedules
			ORDER BY updated_at DESC, id DESC
			OFFSET $2
		),
		disabled_jobs AS (
			UPDATE jobs SET cron = '', updated_at = NOW()
			WHERE id IN (SELECT id FROM excess_schedules WHERE kind = 'job')
			RETURNING id
		),
		disabled_workflows AS (
			UPDATE workflows SET cron = '', updated_at = NOW()
			WHERE id IN (SELECT id FROM excess_schedules WHERE kind = 'workflow')
			RETURNING id
		)
		SELECT id FROM disabled_jobs
		UNION ALL
		SELECT id FROM disabled_workflows
	`, orgID, maxSchedules)
	if err != nil {
		return nil, fmt.Errorf("deactivate excess cron schedules: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if scanErr := rows.Scan(&id); scanErr != nil {
			return nil, fmt.Errorf("scan deactivated cron schedule id: %w", scanErr)
		}
		ids = append(ids, id)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, fmt.Errorf("iterating deactivated cron schedules: %w", rowsErr)
	}
	return ids, nil
}

// DeactivateExcessWebhookSubscriptions deactivates webhook subscriptions beyond
// the given limit for an org. Keeps the most recently created ones.
func (q *Queries) DeactivateExcessWebhookSubscriptions(ctx context.Context, orgID string, maxEndpoints int) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeactivateExcessWebhookSubscriptions")
	defer span.End()

	result, err := q.db.Exec(ctx, `
		UPDATE webhook_subscriptions SET active = false
		WHERE id IN (
			SELECT ws.id FROM webhook_subscriptions ws
			WHERE ws.project_id IN (SELECT id FROM projects WHERE org_id = $1 AND deleted_at IS NULL)
			  AND ws.active = true
			ORDER BY ws.created_at DESC, ws.id DESC
			OFFSET $2
		)
	`, orgID, maxEndpoints)
	if err != nil {
		return 0, fmt.Errorf("deactivate excess webhook subscriptions: %w", err)
	}
	return result.RowsAffected(), nil
}

// CountWebhookSubscriptionsByOrg counts active webhook subscriptions across all projects in an org.
func (q *Queries) CountWebhookSubscriptionsByOrg(ctx context.Context, orgID string) (int, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CountWebhookSubscriptionsByOrg")
	defer span.End()

	return q.countWebhookSubscriptionsByOrgIgnoringProjectRLS(ctx, orgID)
}

// CountLogDrainsByOrg counts log drains across all active projects in an org.
func (q *Queries) CountLogDrainsByOrg(ctx context.Context, orgID string) (int, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CountLogDrainsByOrg")
	defer span.End()

	var count int
	err := q.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM log_drains
		WHERE project_id IN (SELECT id FROM projects WHERE org_id = $1 AND deleted_at IS NULL)
	`, orgID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count log drains by org: %w", err)
	}
	return count, nil
}

// CountNotificationChannelsByProject counts notification channels for a project.
func (q *Queries) CountNotificationChannelsByProject(ctx context.Context, projectID string) (int, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CountNotificationChannelsByProject")
	defer span.End()

	var count int
	err := q.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM notification_channels
		WHERE project_id = $1
	`, projectID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count notification channels by project: %w", err)
	}
	return count, nil
}

// DeactivateExcessLogDrains disables log drains beyond the given org-wide limit.
// Keeps the most recently created drains and disables the oldest excess ones.
func (q *Queries) DeactivateExcessLogDrains(ctx context.Context, orgID string, maxDrains int) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeactivateExcessLogDrains")
	defer span.End()

	result, err := q.db.Exec(ctx, `
		UPDATE log_drains SET enabled = false, updated_at = NOW()
		WHERE id IN (
			SELECT ld.id FROM log_drains ld
			WHERE ld.project_id IN (SELECT id FROM projects WHERE org_id = $1 AND deleted_at IS NULL)
			  AND ld.enabled = true
			ORDER BY ld.created_at DESC, ld.id DESC
			OFFSET $2
		)
	`, orgID, maxDrains)
	if err != nil {
		return 0, fmt.Errorf("deactivate excess log drains: %w", err)
	}
	return result.RowsAffected(), nil
}

// DeactivateExcessNotificationChannelsByProject disables notification channels
// beyond the per-project limit. Keeps the most recently created channels and
// disables the oldest excess ones.
func (q *Queries) DeactivateExcessNotificationChannelsByProject(ctx context.Context, projectID string, maxChannels int) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeactivateExcessNotificationChannelsByProject")
	defer span.End()

	result, err := q.db.Exec(ctx, `
		UPDATE notification_channels SET enabled = false, updated_at = NOW()
		WHERE id IN (
			SELECT nc.id FROM notification_channels nc
			WHERE nc.project_id = $1
			  AND nc.enabled = true
			ORDER BY nc.created_at DESC, nc.id DESC
			OFFSET $2
		)
	`, projectID, maxChannels)
	if err != nil {
		return 0, fmt.Errorf("deactivate excess notification channels: %w", err)
	}
	return result.RowsAffected(), nil
}

// CountWebhookSubscriptionsByProject counts webhook subscriptions for a project.
func (q *Queries) CountWebhookSubscriptionsByProject(ctx context.Context, projectID string) (int, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CountWebhookSubscriptionsByProject")
	defer span.End()

	var count int
	err := q.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM webhook_subscriptions
		WHERE project_id = $1 AND active = true
	`, projectID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count webhook subscriptions by project: %w", err)
	}
	return count, nil
}
