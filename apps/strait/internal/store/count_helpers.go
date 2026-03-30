package store

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
)

// CountCronJobsByOrg counts jobs with a non-empty cron expression across all
// projects belonging to the given org.
func (q *Queries) CountCronJobsByOrg(ctx context.Context, orgID string) (int, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CountCronJobsByOrg")
	defer span.End()

	var count int
	err := q.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM jobs
		WHERE project_id IN (SELECT id FROM projects WHERE org_id = $1 AND deleted_at IS NULL)
		  AND cron IS NOT NULL AND cron != ''
		  AND deleted_at IS NULL
	`, orgID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count cron jobs by org: %w", err)
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
		WHERE project_id = $1 AND deleted_at IS NULL
	`, projectID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count environments by project: %w", err)
	}
	return count, nil
}

// DeleteRunsByOrgOlderThan deletes terminal job runs for an org that are older
// than the given retention duration. Returns the number of deleted rows.
func (q *Queries) DeleteRunsByOrgOlderThan(ctx context.Context, orgID string, retention time.Duration) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteRunsByOrgOlderThan")
	defer span.End()

	cutoff := time.Now().Add(-retention)
	result, err := q.db.Exec(ctx, `
		DELETE FROM job_runs
		WHERE id IN (
			SELECT jr.id FROM job_runs jr
			JOIN jobs j ON jr.job_id = j.id
			WHERE j.project_id IN (SELECT id FROM projects WHERE org_id = $1 AND deleted_at IS NULL)
			  AND jr.status IN ('completed', 'failed', 'canceled', 'timed_out')
			  AND jr.finished_at IS NOT NULL
			  AND jr.finished_at < $2
			LIMIT 1000
		)
	`, orgID, cutoff)
	if err != nil {
		return 0, fmt.Errorf("delete runs by org older than %v: %w", retention, err)
	}
	return result.RowsAffected(), nil
}

// DeactivateExcessCronJobs disables cron jobs beyond the given limit for an org.
// Keeps the most recently updated jobs and clears the cron field on excess ones.
func (q *Queries) DeactivateExcessCronJobs(ctx context.Context, orgID string, maxSchedules int) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeactivateExcessCronJobs")
	defer span.End()

	result, err := q.db.Exec(ctx, `
		UPDATE jobs SET cron = '', updated_at = NOW()
		WHERE id IN (
			SELECT j.id FROM jobs j
			WHERE j.project_id IN (SELECT id FROM projects WHERE org_id = $1 AND deleted_at IS NULL)
			  AND j.cron IS NOT NULL AND j.cron != ''
			  AND j.deleted_at IS NULL
			ORDER BY j.updated_at ASC
			OFFSET $2
		)
	`, orgID, maxSchedules)
	if err != nil {
		return 0, fmt.Errorf("deactivate excess cron jobs: %w", err)
	}
	return result.RowsAffected(), nil
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
			ORDER BY ws.created_at ASC
			OFFSET $2
		)
	`, orgID, maxEndpoints)
	if err != nil {
		return 0, fmt.Errorf("deactivate excess webhook subscriptions: %w", err)
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
		WHERE project_id = $1
	`, projectID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count webhook subscriptions by project: %w", err)
	}
	return count, nil
}
