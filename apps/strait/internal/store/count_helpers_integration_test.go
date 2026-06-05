//go:build integration

package store_test

import (
	"context"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

func TestCountCronJobsByOrg(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-count-cron-" + newID()
	projectID := "proj-count-cron-" + newID()
	otherOrgID := "org-count-cron-other-" + newID()
	otherProjectID := "proj-count-cron-other-" + newID()
	require.NoError(t, q.CreateProject(ctx, &domain.
		Project{ID: projectID,

		OrgID: orgID, Name: "P1",
	}))
	require.NoError(t, q.CreateProject(ctx, &domain.
		Project{ID: otherProjectID,

		OrgID: otherOrgID,

		Name: "P2"}))

	// Create projects in each org.

	// No cron jobs yet.
	count, err := q.CountCronJobsByOrg(ctx, orgID)
	require.NoError(t, err)
	require.EqualValues(t, 0, count)

	// Create two cron jobs in our org.
	for range 2 {
		job := baseJob(newID(), projectID)
		job.Cron = "*/5 * * * *"
		require.NoError(t, q.CreateJob(ctx,
			job))

	}

	// Create a non-cron job in our org.
	noCron := baseJob(newID(), projectID)
	noCron.Cron = ""
	require.NoError(t, q.CreateJob(ctx,
		noCron))

	// Create a cron workflow in our org. Scheduled workflow runs consume the
	// same plan quota as scheduled jobs.
	workflow := &domain.Workflow{
		ID:        newID(),
		ProjectID: projectID,
		Name:      "scheduled workflow",
		Slug:      "scheduled-workflow",
		Enabled:   true,
		Cron:      "15 * * * *",
		Version:   1,
	}
	require.NoError(t, q.CreateWorkflow(ctx, workflow))

	// Create a cron job in the other org.
	otherJob := baseJob(newID(), otherProjectID)
	otherJob.Cron = "0 * * * *"
	require.NoError(t, q.CreateJob(ctx,
		otherJob,
	))

	count, err = q.CountCronJobsByOrg(ctx, orgID)
	require.NoError(t, err)
	require.EqualValues(t, 3, count)

	// Other org has its own count.
	otherCount, err := q.CountCronJobsByOrg(ctx, otherOrgID)
	require.NoError(t, err)
	require.EqualValues(t, 1, otherCount)

}

func TestCountEnvironmentsByProject(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-count-envs-" + newID()
	otherProjectID := "proj-count-envs-other-" + newID()

	// No environments yet.
	count, err := q.CountEnvironmentsByProject(ctx, projectID)
	require.NoError(t, err)
	require.EqualValues(t, 0, count)

	// Create two environments in our project.
	for i := range 2 {
		env := &domain.Environment{
			ProjectID: projectID,
			Name:      "env-" + newID(),
			Slug:      "env-slug-" + newID(),
		}
		_ = i
		require.NoError(t, q.CreateEnvironment(ctx,
			env))

	}

	// Create one in another project.
	otherEnv := &domain.Environment{
		ProjectID: otherProjectID,
		Name:      "other-env",
		Slug:      "other-env-slug",
	}
	require.NoError(t, q.CreateEnvironment(ctx,
		otherEnv))

	count, err = q.CountEnvironmentsByProject(ctx, projectID)
	require.NoError(t, err)
	require.EqualValues(t, 2, count)

}

func TestCountWebhookSubscriptionsByProject(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-count-ws-" + newID()
	otherProjectID := "proj-count-ws-other-" + newID()

	// No subscriptions yet.
	count, err := q.CountWebhookSubscriptionsByProject(ctx, projectID)
	require.NoError(t, err)
	require.EqualValues(t, 0, count)

	// Create two active subscriptions.
	for range 2 {
		sub := &domain.WebhookSubscription{
			ProjectID:  projectID,
			WebhookURL: "https://example.com/hook-" + newID(),
			EventTypes: []string{"run.completed"},
			Secret:     "secret",
			Active:     true,
		}
		require.NoError(t, q.CreateWebhookSubscription(ctx, sub))

	}

	// Create an inactive subscription in our project (should not count).
	inactive := &domain.WebhookSubscription{
		ProjectID:  projectID,
		WebhookURL: "https://example.com/inactive",
		EventTypes: []string{"run.failed"},
		Secret:     "secret",
		Active:     false,
	}
	require.NoError(t, q.CreateWebhookSubscription(ctx, inactive))

	// Create one in another project.
	otherSub := &domain.WebhookSubscription{
		ProjectID:  otherProjectID,
		WebhookURL: "https://example.com/other",
		EventTypes: []string{"run.completed"},
		Secret:     "secret",
		Active:     true,
	}
	require.NoError(t, q.CreateWebhookSubscription(ctx, otherSub))

	count, err = q.CountWebhookSubscriptionsByProject(ctx, projectID)
	require.NoError(t, err)
	require.EqualValues(t, 2, count)

}

func TestDeleteRunsByOrgOlderThan(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-delete-runs-" + newID()
	projectID := "proj-delete-runs-" + newID()
	require.NoError(t, q.CreateProject(ctx, &domain.
		Project{ID: projectID,

		OrgID: orgID, Name: "P",
	}))

	job := baseJob(newID(), projectID)
	require.NoError(t, q.CreateJob(ctx,
		job))

	// Create a completed run with finished_at in the past.
	run := baseRun(job, newID())
	run.Status = domain.StatusCompleted
	require.NoError(t, q.CreateRun(ctx,
		run))

	past := time.Now().UTC().Add(-48 * time.Hour)
	if _, err := testDB.Pool.Exec(ctx,
		"UPDATE job_runs SET status = 'completed', finished_at = $1 WHERE id = $2", past, run.ID); err != nil {
		require.Failf(t, "test failure",

			"update finished_at: %v", err)
	}

	// Create a recent completed run (should not be deleted).
	recentRun := baseRun(job, newID())
	recentRun.Status = domain.StatusCompleted
	require.NoError(t, q.CreateRun(ctx,
		recentRun,
	))

	recentTime := time.Now().UTC().Add(-1 * time.Hour)
	if _, err := testDB.Pool.Exec(ctx,
		"UPDATE job_runs SET status = 'completed', finished_at = $1 WHERE id = $2", recentTime, recentRun.ID); err != nil {
		require.Failf(t, "test failure",

			"update finished_at(recent): %v", err)
	}

	// Delete runs older than 24 hours.
	deleted, err := q.DeleteRunsByOrgOlderThan(ctx, orgID, 24*time.Hour)
	require.NoError(t, err)
	require.EqualValues(t, 1, deleted)

	// Second call should delete zero.
	deleted2, err := q.DeleteRunsByOrgOlderThan(ctx, orgID, 24*time.Hour)
	require.NoError(t, err)
	require.EqualValues(t, 0, deleted2)

}

func TestDeleteWorkflowRunsByOrgOlderThan(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-delete-wfruns-" + newID()
	projectID := "proj-delete-wfruns-" + newID()
	require.NoError(t, q.CreateProject(ctx, &domain.
		Project{ID: projectID,

		OrgID: orgID, Name: "P",
	}))

	wf := &domain.Workflow{ID: newID(), ProjectID: projectID, Name: "wf", Slug: "wf-slug-" + newID(), Enabled: true, Version: 1}
	require.NoError(t, q.CreateWorkflow(ctx, wf))

	wfRun := &domain.WorkflowRun{ID: newID(), WorkflowID: wf.ID, ProjectID: projectID, Status: domain.WfStatusCompleted, TriggeredBy: "manual"}
	require.NoError(t, q.CreateWorkflowRun(ctx,
		wfRun))

	past := time.Now().UTC().Add(-48 * time.Hour)
	if _, err := testDB.Pool.Exec(ctx,
		"UPDATE workflow_runs SET status = 'completed', finished_at = $1 WHERE id = $2", past, wfRun.ID); err != nil {
		require.Failf(t, "test failure",

			"update finished_at: %v", err)
	}

	deleted, err := q.DeleteWorkflowRunsByOrgOlderThan(ctx, orgID, 24*time.Hour)
	require.NoError(t, err)
	require.EqualValues(t, 1, deleted)

}

func TestDeactivateExcessEnvironments(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-deactivate-envs-" + newID()
	projectID := "proj-deactivate-envs-" + newID()
	require.NoError(t, q.CreateProject(ctx, &domain.
		Project{ID: projectID,

		OrgID: orgID, Name: "P",
	}))

	// Create 5 environments.
	for range 5 {
		env := &domain.Environment{
			ProjectID: projectID,
			Name:      "env-" + newID(),
			Slug:      "env-slug-" + newID(),
		}
		require.NoError(t, q.CreateEnvironment(ctx,
			env))

		// Small sleep to ensure different created_at ordering.
		time.Sleep(5 * time.Millisecond)
	}

	// Keep only 3 -- should deactivate 2 oldest.
	deactivated, err := q.DeactivateExcessEnvironments(ctx, orgID, 3)
	require.NoError(t, err)
	require.EqualValues(t, 2, deactivated)

	// Counting should reflect only the remaining active ones.
	count, err := q.CountEnvironmentsByProject(ctx, projectID)
	require.NoError(t, err)
	require.EqualValues(t, 3, count)

}

func TestDeactivateExcessEnvironments_PreservesStandardEnvironments(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-deactivate-envs-standard-" + newID()
	projectID := "proj-deactivate-envs-standard-" + newID()
	require.NoError(t, q.CreateProject(ctx, &domain.
		Project{ID: projectID,

		OrgID: orgID, Name: "P",
	}))
	require.NoError(t, q.CreateStandardEnvironments(ctx, projectID))

	standardBefore, err := q.ListEnvironments(ctx, projectID, 10, nil)
	require.NoError(t, err)

	standardIDs := map[string]bool{}
	for _, env := range standardBefore {
		if env.IsStandard {
			standardIDs[env.ID] = true
		}
	}
	require.Len(t, standardIDs,

		3)

	time.Sleep(5 * time.Millisecond)
	for range 5 {
		env := &domain.Environment{
			ProjectID: projectID,
			Name:      "custom-env-" + newID(),
			Slug:      "custom-env-slug-" + newID(),
		}
		require.NoError(t, q.CreateEnvironment(ctx,
			env))

		time.Sleep(5 * time.Millisecond)
	}

	deactivated, err := q.DeactivateExcessEnvironments(ctx, orgID, 2)
	require.NoError(t, err)
	require.EqualValues(t, 3, deactivated)

	remaining, err := q.ListEnvironments(ctx, projectID, 20, nil)
	require.NoError(t, err)

	var standardCount, customCount int
	for _, env := range remaining {
		if env.IsStandard {
			standardCount++
			require.True(t, standardIDs[env.ID])

			continue
		}
		customCount++
	}
	require.EqualValues(t, 3, standardCount)
	require.EqualValues(t, 2, customCount)

}

func TestDeactivateExcessCronJobs(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-deactivate-cron-" + newID()
	projectID := "proj-deactivate-cron-" + newID()
	require.NoError(t, q.CreateProject(ctx, &domain.
		Project{ID: projectID,

		OrgID: orgID, Name: "P",
	}))

	// Create 4 cron jobs.
	for range 4 {
		job := baseJob(newID(), projectID)
		job.Cron = "*/5 * * * *"
		require.NoError(t, q.CreateJob(ctx,
			job))

		time.Sleep(5 * time.Millisecond)
	}

	// Keep only 2 -- should clear cron on 2 oldest.
	deactivated, err := q.DeactivateExcessCronJobs(ctx, orgID, 2)
	require.NoError(t, err)
	require.Len(t, deactivated,

		2)

	count, err := q.CountCronJobsByOrg(ctx, orgID)
	require.NoError(t, err)
	require.EqualValues(t, 2, count)

}

func TestDeactivateExcessCronJobs_IncludesWorkflows(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-deactivate-cron-workflows-" + newID()
	projectID := "proj-deactivate-cron-workflows-" + newID()
	require.NoError(t, q.CreateProject(ctx, &domain.
		Project{ID: projectID,

		OrgID: orgID, Name: "P",
	}))

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	oldJob := baseJob(newID(), projectID)
	oldJob.Cron = "*/5 * * * *"
	require.NoError(t, q.CreateJob(ctx,
		oldJob))

	newJob := baseJob(newID(), projectID)
	newJob.Cron = "*/5 * * * *"
	require.NoError(t, q.CreateJob(ctx,
		newJob))

	oldWorkflow := &domain.Workflow{
		ID:        newID(),
		ProjectID: projectID,
		Name:      "old scheduled workflow",
		Slug:      "old-scheduled-workflow",
		Enabled:   true,
		Cron:      "0 * * * *",
		Version:   1,
	}
	require.NoError(t, q.CreateWorkflow(ctx, oldWorkflow))

	newWorkflow := &domain.Workflow{
		ID:        newID(),
		ProjectID: projectID,
		Name:      "new scheduled workflow",
		Slug:      "new-scheduled-workflow",
		Enabled:   true,
		Cron:      "15 * * * *",
		Version:   1,
	}
	require.NoError(t, q.CreateWorkflow(ctx, newWorkflow))

	updates := []struct {
		table string
		id    string
		at    time.Time
	}{
		{table: "jobs", id: oldJob.ID, at: base},
		{table: "workflows", id: oldWorkflow.ID, at: base.Add(time.Hour)},
		{table: "jobs", id: newJob.ID, at: base.Add(2 * time.Hour)},
		{table: "workflows", id: newWorkflow.ID, at: base.Add(3 * time.Hour)},
	}
	for _, update := range updates {
		if _, err := testDB.Pool.Exec(ctx, "UPDATE "+update.table+" SET updated_at = $2 WHERE id = $1", update.id, update.at); err != nil {
			require.Failf(t, "test failure",

				"set %s updated_at for %s: %v", update.table, update.id, err)
		}
	}

	deactivated, err := q.DeactivateExcessCronJobs(ctx, orgID, 2)
	require.NoError(t, err)
	require.Len(t, deactivated,

		2)

	assertCronCleared := func(table, id string, wantCleared bool) {
		t.Helper()
		var cron string
		require.NoError(t, testDB.
			Pool.QueryRow(ctx,
			"SELECT COALESCE(cron, '') FROM "+
				table+" WHERE id = $1",

			id).Scan(&cron))
		require.Equal(t, wantCleared,

			cron ==
				"")

	}
	assertCronCleared("jobs", oldJob.ID, true)
	assertCronCleared("workflows", oldWorkflow.ID, true)
	assertCronCleared("jobs", newJob.ID, false)
	assertCronCleared("workflows", newWorkflow.ID, false)
}

func TestDeactivateExcessWebhookSubscriptions(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-deactivate-ws-" + newID()
	projectID := "proj-deactivate-ws-" + newID()
	require.NoError(t, q.CreateProject(ctx, &domain.
		Project{ID: projectID,

		OrgID: orgID, Name: "P",
	}))

	// Create 4 active webhook subscriptions.
	for i := range 4 {
		sub := &domain.WebhookSubscription{
			ProjectID:  projectID,
			WebhookURL: "https://example.com/hook-" + newID(),
			EventTypes: []string{"run.completed"},
			Secret:     "secret",
			Active:     true,
		}
		_ = i
		require.NoError(t, q.CreateWebhookSubscription(ctx, sub))

		time.Sleep(5 * time.Millisecond)
	}

	// Keep only 2.
	deactivated, err := q.DeactivateExcessWebhookSubscriptions(ctx, orgID, 2)
	require.NoError(t, err)
	require.EqualValues(t, 2, deactivated)

	count, err := q.CountWebhookSubscriptionsByProject(ctx, projectID)
	require.NoError(t, err)
	require.EqualValues(t, 2, count)

}

func TestDeactivateExcessLogDrains_DisablesOldest(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-log-drain-trim-" + newID()
	projectID := "proj-log-drain-trim-" + newID()
	require.NoError(t, q.CreateProject(ctx, &domain.
		Project{ID: projectID,

		OrgID: orgID, Name: "P",
	}))

	baseTime := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	ids := make([]string, 4)
	for i := range ids {
		ids[i] = "drain-trim-" + newID()
		drain := &domain.LogDrain{
			ID:          ids[i],
			ProjectID:   projectID,
			Name:        "drain",
			DrainType:   "http",
			EndpointURL: "https://example.com/logs",
			AuthType:    "none",
			Enabled:     true,
		}
		require.NoError(t, q.CreateLogDrain(ctx, drain))

		if _, err := testDB.Pool.Exec(ctx, `
			UPDATE log_drains
			SET created_at = $2, updated_at = $2
			WHERE id = $1
		`, ids[i], baseTime.Add(time.Duration(i)*time.Minute)); err != nil {
			require.Failf(t, "test failure",

				"set log drain created_at(%d): %v", i, err)
		}
	}

	deactivated, err := q.DeactivateExcessLogDrains(ctx, orgID, 2)
	require.NoError(t, err)
	require.EqualValues(t, 2, deactivated)

	for i, id := range ids {
		var enabled bool
		require.NoError(t, testDB.
			Pool.QueryRow(ctx,
			`SELECT enabled FROM log_drains WHERE id = $1`,

			id).Scan(&enabled))

		wantEnabled := i >= 2
		require.Equal(t, wantEnabled,

			enabled,
		)

	}
}

func TestDeactivateExcessNotificationChannelsByProject_DisablesOldest(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-notification-trim-" + newID()
	projectID := "proj-notification-trim-" + newID()
	require.NoError(t, q.CreateProject(ctx, &domain.
		Project{ID: projectID,

		OrgID: orgID, Name: "P",
	}))

	baseTime := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	ids := make([]string, 4)
	for i := range ids {
		ids[i] = "notification-trim-" + newID()
		channel := &domain.NotificationChannel{
			ID:          ids[i],
			ProjectID:   projectID,
			ChannelType: domain.ChannelTypeWebhook,
			Name:        "ops",
			Config:      []byte(`{"url":"https://example.com/hooks/ops"}`),
			Enabled:     true,
		}
		require.NoError(t, q.CreateNotificationChannel(ctx, channel))

		if _, err := testDB.Pool.Exec(ctx, `
			UPDATE notification_channels
			SET created_at = $2, updated_at = $2
			WHERE id = $1
		`, ids[i], baseTime.Add(time.Duration(i)*time.Minute)); err != nil {
			require.Failf(t, "test failure",

				"set notification channel created_at(%d): %v", i, err)
		}
	}

	deactivated, err := q.DeactivateExcessNotificationChannelsByProject(ctx, projectID, 2)
	require.NoError(t, err)
	require.EqualValues(t, 2, deactivated)

	for i, id := range ids {
		var enabled bool
		require.NoError(t, testDB.
			Pool.QueryRow(ctx,
			`SELECT enabled FROM notification_channels WHERE id = $1`,

			id).Scan(&enabled))

		wantEnabled := i >= 2
		require.Equal(t, wantEnabled,

			enabled,
		)

	}
}

func TestDeactivateExcessWebhookSubscriptions_DisablesOldest(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-webhook-trim-" + newID()
	projectID := "proj-webhook-trim-" + newID()
	require.NoError(t, q.CreateProject(ctx, &domain.
		Project{ID: projectID,

		OrgID: orgID, Name: "P",
	}))

	baseTime := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	ids := make([]string, 4)
	for i := range ids {
		ids[i] = "webhook-trim-" + newID()
		sub := &domain.WebhookSubscription{
			ID:         ids[i],
			ProjectID:  projectID,
			WebhookURL: "https://example.com/hook-" + newID(),
			EventTypes: []string{"run.completed"},
			Secret:     "secret",
			Active:     true,
		}
		require.NoError(t, q.CreateWebhookSubscription(ctx, sub))

		if _, err := testDB.Pool.Exec(ctx, `
			UPDATE webhook_subscriptions
			SET created_at = $2
			WHERE id = $1
		`, ids[i], baseTime.Add(time.Duration(i)*time.Minute)); err != nil {
			require.Failf(t, "test failure",

				"set webhook subscription created_at(%d): %v", i, err)
		}
	}

	deactivated, err := q.DeactivateExcessWebhookSubscriptions(ctx, orgID, 2)
	require.NoError(t, err)
	require.EqualValues(t, 2, deactivated)

	for i, id := range ids {
		var active bool
		require.NoError(t, testDB.
			Pool.QueryRow(ctx,
			`SELECT active FROM webhook_subscriptions WHERE id = $1`,

			id).Scan(&active))

		wantActive := i >= 2
		require.Equal(t, wantActive,

			active,
		)

	}
}
