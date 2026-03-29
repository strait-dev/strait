package store

import (
	"context"
	"fmt"

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
