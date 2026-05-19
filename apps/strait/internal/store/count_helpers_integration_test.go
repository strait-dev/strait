//go:build integration

package store_test

import (
	"context"
	"testing"
	"time"

	"strait/internal/domain"
)

func TestCountCronJobsByOrg(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-count-cron-" + newID()
	projectID := "proj-count-cron-" + newID()
	otherOrgID := "org-count-cron-other-" + newID()
	otherProjectID := "proj-count-cron-other-" + newID()

	// Create projects in each org.
	if err := q.CreateProject(ctx, &domain.Project{ID: projectID, OrgID: orgID, Name: "P1"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if err := q.CreateProject(ctx, &domain.Project{ID: otherProjectID, OrgID: otherOrgID, Name: "P2"}); err != nil {
		t.Fatalf("CreateProject(other) error = %v", err)
	}

	// No cron jobs yet.
	count, err := q.CountCronJobsByOrg(ctx, orgID)
	if err != nil {
		t.Fatalf("CountCronJobsByOrg() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0", count)
	}

	// Create two cron jobs in our org.
	for range 2 {
		job := baseJob(newID(), projectID)
		job.Cron = "*/5 * * * *"
		if err := q.CreateJob(ctx, job); err != nil {
			t.Fatalf("CreateJob(cron) error = %v", err)
		}
	}

	// Create a non-cron job in our org.
	noCron := baseJob(newID(), projectID)
	noCron.Cron = ""
	if err := q.CreateJob(ctx, noCron); err != nil {
		t.Fatalf("CreateJob(no cron) error = %v", err)
	}

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
	if err := q.CreateWorkflow(ctx, workflow); err != nil {
		t.Fatalf("CreateWorkflow(cron) error = %v", err)
	}

	// Create a cron job in the other org.
	otherJob := baseJob(newID(), otherProjectID)
	otherJob.Cron = "0 * * * *"
	if err := q.CreateJob(ctx, otherJob); err != nil {
		t.Fatalf("CreateJob(other org) error = %v", err)
	}

	count, err = q.CountCronJobsByOrg(ctx, orgID)
	if err != nil {
		t.Fatalf("CountCronJobsByOrg() error = %v", err)
	}
	if count != 3 {
		t.Fatalf("count = %d, want 3", count)
	}

	// Other org has its own count.
	otherCount, err := q.CountCronJobsByOrg(ctx, otherOrgID)
	if err != nil {
		t.Fatalf("CountCronJobsByOrg(other) error = %v", err)
	}
	if otherCount != 1 {
		t.Fatalf("other count = %d, want 1", otherCount)
	}
}

func TestCountEnvironmentsByProject(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-count-envs-" + newID()
	otherProjectID := "proj-count-envs-other-" + newID()

	// No environments yet.
	count, err := q.CountEnvironmentsByProject(ctx, projectID)
	if err != nil {
		t.Fatalf("CountEnvironmentsByProject() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0", count)
	}

	// Create two environments in our project.
	for i := range 2 {
		env := &domain.Environment{
			ProjectID: projectID,
			Name:      "env-" + newID(),
			Slug:      "env-slug-" + newID(),
		}
		_ = i
		if err := q.CreateEnvironment(ctx, env); err != nil {
			t.Fatalf("CreateEnvironment() error = %v", err)
		}
	}

	// Create one in another project.
	otherEnv := &domain.Environment{
		ProjectID: otherProjectID,
		Name:      "other-env",
		Slug:      "other-env-slug",
	}
	if err := q.CreateEnvironment(ctx, otherEnv); err != nil {
		t.Fatalf("CreateEnvironment(other) error = %v", err)
	}

	count, err = q.CountEnvironmentsByProject(ctx, projectID)
	if err != nil {
		t.Fatalf("CountEnvironmentsByProject() error = %v", err)
	}
	if count != 2 {
		t.Fatalf("count = %d, want 2", count)
	}
}

func TestCountWebhookSubscriptionsByProject(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-count-ws-" + newID()
	otherProjectID := "proj-count-ws-other-" + newID()

	// No subscriptions yet.
	count, err := q.CountWebhookSubscriptionsByProject(ctx, projectID)
	if err != nil {
		t.Fatalf("CountWebhookSubscriptionsByProject() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0", count)
	}

	// Create two active subscriptions.
	for range 2 {
		sub := &domain.WebhookSubscription{
			ProjectID:  projectID,
			WebhookURL: "https://example.com/hook-" + newID(),
			EventTypes: []string{"run.completed"},
			Secret:     "secret",
			Active:     true,
		}
		if err := q.CreateWebhookSubscription(ctx, sub); err != nil {
			t.Fatalf("CreateWebhookSubscription() error = %v", err)
		}
	}

	// Create an inactive subscription in our project (should not count).
	inactive := &domain.WebhookSubscription{
		ProjectID:  projectID,
		WebhookURL: "https://example.com/inactive",
		EventTypes: []string{"run.failed"},
		Secret:     "secret",
		Active:     false,
	}
	if err := q.CreateWebhookSubscription(ctx, inactive); err != nil {
		t.Fatalf("CreateWebhookSubscription(inactive) error = %v", err)
	}

	// Create one in another project.
	otherSub := &domain.WebhookSubscription{
		ProjectID:  otherProjectID,
		WebhookURL: "https://example.com/other",
		EventTypes: []string{"run.completed"},
		Secret:     "secret",
		Active:     true,
	}
	if err := q.CreateWebhookSubscription(ctx, otherSub); err != nil {
		t.Fatalf("CreateWebhookSubscription(other) error = %v", err)
	}

	count, err = q.CountWebhookSubscriptionsByProject(ctx, projectID)
	if err != nil {
		t.Fatalf("CountWebhookSubscriptionsByProject() error = %v", err)
	}
	if count != 2 {
		t.Fatalf("count = %d, want 2", count)
	}
}

func TestDeleteRunsByOrgOlderThan(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-delete-runs-" + newID()
	projectID := "proj-delete-runs-" + newID()

	if err := q.CreateProject(ctx, &domain.Project{ID: projectID, OrgID: orgID, Name: "P"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	job := baseJob(newID(), projectID)
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	// Create a completed run with finished_at in the past.
	run := baseRun(job, newID())
	run.Status = domain.StatusCompleted
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}
	past := time.Now().UTC().Add(-48 * time.Hour)
	if _, err := testDB.Pool.Exec(ctx,
		"UPDATE job_runs SET status = 'completed', finished_at = $1 WHERE id = $2", past, run.ID); err != nil {
		t.Fatalf("update finished_at: %v", err)
	}

	// Create a recent completed run (should not be deleted).
	recentRun := baseRun(job, newID())
	recentRun.Status = domain.StatusCompleted
	if err := q.CreateRun(ctx, recentRun); err != nil {
		t.Fatalf("CreateRun(recent) error = %v", err)
	}
	recentTime := time.Now().UTC().Add(-1 * time.Hour)
	if _, err := testDB.Pool.Exec(ctx,
		"UPDATE job_runs SET status = 'completed', finished_at = $1 WHERE id = $2", recentTime, recentRun.ID); err != nil {
		t.Fatalf("update finished_at(recent): %v", err)
	}

	// Delete runs older than 24 hours.
	deleted, err := q.DeleteRunsByOrgOlderThan(ctx, orgID, 24*time.Hour)
	if err != nil {
		t.Fatalf("DeleteRunsByOrgOlderThan() error = %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}

	// Second call should delete zero.
	deleted2, err := q.DeleteRunsByOrgOlderThan(ctx, orgID, 24*time.Hour)
	if err != nil {
		t.Fatalf("DeleteRunsByOrgOlderThan(second) error = %v", err)
	}
	if deleted2 != 0 {
		t.Fatalf("deleted2 = %d, want 0", deleted2)
	}
}

func TestDeleteWorkflowRunsByOrgOlderThan(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-delete-wfruns-" + newID()
	projectID := "proj-delete-wfruns-" + newID()

	if err := q.CreateProject(ctx, &domain.Project{ID: projectID, OrgID: orgID, Name: "P"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	wf := &domain.Workflow{ID: newID(), ProjectID: projectID, Name: "wf", Slug: "wf-slug-" + newID(), Enabled: true, Version: 1}
	if err := q.CreateWorkflow(ctx, wf); err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}

	wfRun := &domain.WorkflowRun{ID: newID(), WorkflowID: wf.ID, ProjectID: projectID, Status: domain.WfStatusCompleted, TriggeredBy: "manual"}
	if err := q.CreateWorkflowRun(ctx, wfRun); err != nil {
		t.Fatalf("CreateWorkflowRun() error = %v", err)
	}
	past := time.Now().UTC().Add(-48 * time.Hour)
	if _, err := testDB.Pool.Exec(ctx,
		"UPDATE workflow_runs SET status = 'completed', finished_at = $1 WHERE id = $2", past, wfRun.ID); err != nil {
		t.Fatalf("update finished_at: %v", err)
	}

	deleted, err := q.DeleteWorkflowRunsByOrgOlderThan(ctx, orgID, 24*time.Hour)
	if err != nil {
		t.Fatalf("DeleteWorkflowRunsByOrgOlderThan() error = %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}
}

func TestDeactivateExcessEnvironments(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-deactivate-envs-" + newID()
	projectID := "proj-deactivate-envs-" + newID()

	if err := q.CreateProject(ctx, &domain.Project{ID: projectID, OrgID: orgID, Name: "P"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	// Create 5 environments.
	for i := range 5 {
		env := &domain.Environment{
			ProjectID: projectID,
			Name:      "env-" + newID(),
			Slug:      "env-slug-" + newID(),
		}
		_ = i
		if err := q.CreateEnvironment(ctx, env); err != nil {
			t.Fatalf("CreateEnvironment(%d) error = %v", i, err)
		}
		// Small sleep to ensure different created_at ordering.
		time.Sleep(5 * time.Millisecond)
	}

	// Keep only 3 -- should deactivate 2 oldest.
	deactivated, err := q.DeactivateExcessEnvironments(ctx, orgID, 3)
	if err != nil {
		t.Fatalf("DeactivateExcessEnvironments() error = %v", err)
	}
	if deactivated != 2 {
		t.Fatalf("deactivated = %d, want 2", deactivated)
	}

	// Counting should reflect only the remaining active ones.
	count, err := q.CountEnvironmentsByProject(ctx, projectID)
	if err != nil {
		t.Fatalf("CountEnvironmentsByProject() error = %v", err)
	}
	if count != 3 {
		t.Fatalf("count after deactivation = %d, want 3", count)
	}
}

func TestDeactivateExcessEnvironments_PreservesStandardEnvironments(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-deactivate-envs-standard-" + newID()
	projectID := "proj-deactivate-envs-standard-" + newID()

	if err := q.CreateProject(ctx, &domain.Project{ID: projectID, OrgID: orgID, Name: "P"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if err := q.CreateStandardEnvironments(ctx, projectID); err != nil {
		t.Fatalf("CreateStandardEnvironments() error = %v", err)
	}

	standardBefore, err := q.ListEnvironments(ctx, projectID, 10, nil)
	if err != nil {
		t.Fatalf("ListEnvironments(before) error = %v", err)
	}
	standardIDs := map[string]bool{}
	for _, env := range standardBefore {
		if env.IsStandard {
			standardIDs[env.ID] = true
		}
	}
	if len(standardIDs) != 3 {
		t.Fatalf("standard environment count before cleanup = %d, want 3", len(standardIDs))
	}

	time.Sleep(5 * time.Millisecond)
	for i := range 5 {
		env := &domain.Environment{
			ProjectID: projectID,
			Name:      "custom-env-" + newID(),
			Slug:      "custom-env-slug-" + newID(),
		}
		if err := q.CreateEnvironment(ctx, env); err != nil {
			t.Fatalf("CreateEnvironment(%d) error = %v", i, err)
		}
		time.Sleep(5 * time.Millisecond)
	}

	deactivated, err := q.DeactivateExcessEnvironments(ctx, orgID, 2)
	if err != nil {
		t.Fatalf("DeactivateExcessEnvironments() error = %v", err)
	}
	if deactivated != 3 {
		t.Fatalf("deactivated custom environments = %d, want 3", deactivated)
	}

	remaining, err := q.ListEnvironments(ctx, projectID, 20, nil)
	if err != nil {
		t.Fatalf("ListEnvironments(after) error = %v", err)
	}
	var standardCount, customCount int
	for _, env := range remaining {
		if env.IsStandard {
			standardCount++
			if !standardIDs[env.ID] {
				t.Fatalf("unexpected standard environment after cleanup: %s", env.ID)
			}
			continue
		}
		customCount++
	}
	if standardCount != 3 {
		t.Fatalf("standard environment count after cleanup = %d, want 3", standardCount)
	}
	if customCount != 2 {
		t.Fatalf("custom environment count after cleanup = %d, want 2", customCount)
	}
}

func TestDeactivateExcessCronJobs(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-deactivate-cron-" + newID()
	projectID := "proj-deactivate-cron-" + newID()

	if err := q.CreateProject(ctx, &domain.Project{ID: projectID, OrgID: orgID, Name: "P"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	// Create 4 cron jobs.
	for i := range 4 {
		job := baseJob(newID(), projectID)
		job.Cron = "*/5 * * * *"
		if err := q.CreateJob(ctx, job); err != nil {
			t.Fatalf("CreateJob(%d) error = %v", i, err)
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Keep only 2 -- should clear cron on 2 oldest.
	deactivated, err := q.DeactivateExcessCronJobs(ctx, orgID, 2)
	if err != nil {
		t.Fatalf("DeactivateExcessCronJobs() error = %v", err)
	}
	if len(deactivated) != 2 {
		t.Fatalf("deactivated = %d, want 2", len(deactivated))
	}

	count, err := q.CountCronJobsByOrg(ctx, orgID)
	if err != nil {
		t.Fatalf("CountCronJobsByOrg() error = %v", err)
	}
	if count != 2 {
		t.Fatalf("count after deactivation = %d, want 2", count)
	}
}

func TestDeactivateExcessCronJobs_IncludesWorkflows(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-deactivate-cron-workflows-" + newID()
	projectID := "proj-deactivate-cron-workflows-" + newID()
	if err := q.CreateProject(ctx, &domain.Project{ID: projectID, OrgID: orgID, Name: "P"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	oldJob := baseJob(newID(), projectID)
	oldJob.Cron = "*/5 * * * *"
	if err := q.CreateJob(ctx, oldJob); err != nil {
		t.Fatalf("CreateJob(old) error = %v", err)
	}
	newJob := baseJob(newID(), projectID)
	newJob.Cron = "*/5 * * * *"
	if err := q.CreateJob(ctx, newJob); err != nil {
		t.Fatalf("CreateJob(new) error = %v", err)
	}
	oldWorkflow := &domain.Workflow{
		ID:        newID(),
		ProjectID: projectID,
		Name:      "old scheduled workflow",
		Slug:      "old-scheduled-workflow",
		Enabled:   true,
		Cron:      "0 * * * *",
		Version:   1,
	}
	if err := q.CreateWorkflow(ctx, oldWorkflow); err != nil {
		t.Fatalf("CreateWorkflow(old) error = %v", err)
	}
	newWorkflow := &domain.Workflow{
		ID:        newID(),
		ProjectID: projectID,
		Name:      "new scheduled workflow",
		Slug:      "new-scheduled-workflow",
		Enabled:   true,
		Cron:      "15 * * * *",
		Version:   1,
	}
	if err := q.CreateWorkflow(ctx, newWorkflow); err != nil {
		t.Fatalf("CreateWorkflow(new) error = %v", err)
	}
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
			t.Fatalf("set %s updated_at for %s: %v", update.table, update.id, err)
		}
	}

	deactivated, err := q.DeactivateExcessCronJobs(ctx, orgID, 2)
	if err != nil {
		t.Fatalf("DeactivateExcessCronJobs() error = %v", err)
	}
	if len(deactivated) != 2 {
		t.Fatalf("deactivated = %d, want 2", len(deactivated))
	}
	assertCronCleared := func(table, id string, wantCleared bool) {
		t.Helper()
		var cron string
		if err := testDB.Pool.QueryRow(ctx, "SELECT COALESCE(cron, '') FROM "+table+" WHERE id = $1", id).Scan(&cron); err != nil {
			t.Fatalf("query %s cron for %s: %v", table, id, err)
		}
		if gotCleared := cron == ""; gotCleared != wantCleared {
			t.Fatalf("%s %s cron cleared = %v, want %v (cron=%q)", table, id, gotCleared, wantCleared, cron)
		}
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

	if err := q.CreateProject(ctx, &domain.Project{ID: projectID, OrgID: orgID, Name: "P"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

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
		if err := q.CreateWebhookSubscription(ctx, sub); err != nil {
			t.Fatalf("CreateWebhookSubscription(%d) error = %v", i, err)
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Keep only 2.
	deactivated, err := q.DeactivateExcessWebhookSubscriptions(ctx, orgID, 2)
	if err != nil {
		t.Fatalf("DeactivateExcessWebhookSubscriptions() error = %v", err)
	}
	if deactivated != 2 {
		t.Fatalf("deactivated = %d, want 2", deactivated)
	}

	count, err := q.CountWebhookSubscriptionsByProject(ctx, projectID)
	if err != nil {
		t.Fatalf("CountWebhookSubscriptionsByProject() error = %v", err)
	}
	if count != 2 {
		t.Fatalf("count after deactivation = %d, want 2", count)
	}
}
