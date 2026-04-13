//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"
)

// Integration coverage for four store methods that had zero test
// references as of PR #92:
//   - GetEventSubscription  (event_sources.go:227)
//   - GetJobDependency       (job_dependencies.go:91)
//   - CountEnvironmentsByOrg (count_helpers.go:31)
//   - CountWebhookSubscriptionsByOrg (count_helpers.go:182)
//
// All four have existing Create* siblings already tested, so setup
// reuses the same helpers.

// -----------------------------------------------------------------------.
// GetEventSubscription
// -----------------------------------------------------------------------.

func TestGetEventSubscription_FoundAndNotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	src := &domain.EventSource{
		ProjectID: "project-get-sub-" + newID(),
		Name:      "sub-source-" + newID(),
		Enabled:   true,
	}
	if err := q.CreateEventSource(ctx, src); err != nil {
		t.Fatalf("CreateEventSource: %v", err)
	}
	sub := &domain.EventSubscription{
		SourceID:   src.ID,
		TargetType: "job",
		TargetID:   newID(),
		FilterExpr: json.RawMessage(`{"event":"push"}`),
		Enabled:    true,
	}
	if err := q.CreateEventSubscription(ctx, sub); err != nil {
		t.Fatalf("CreateEventSubscription: %v", err)
	}

	// Found path.
	got, err := q.GetEventSubscription(ctx, sub.ID)
	if err != nil {
		t.Fatalf("GetEventSubscription(%q): %v", sub.ID, err)
	}
	if got.ID != sub.ID {
		t.Fatalf("id = %q, want %q", got.ID, sub.ID)
	}
	if got.SourceID != src.ID {
		t.Fatalf("source id = %q, want %q", got.SourceID, src.ID)
	}
	if got.TargetType != "job" {
		t.Fatalf("target type = %q, want job", got.TargetType)
	}
	if got.TargetID != sub.TargetID {
		t.Fatalf("target id = %q, want %q", got.TargetID, sub.TargetID)
	}
	if !got.Enabled {
		t.Fatal("enabled = false, want true")
	}

	// Not-found path.
	if _, err := q.GetEventSubscription(ctx, "sub-does-not-exist-"+newID()); !errors.Is(err, store.ErrEventSubscriptionNotFound) {
		t.Fatalf("not-found err = %v, want ErrEventSubscriptionNotFound", err)
	}
}

// -----------------------------------------------------------------------.
// GetJobDependency
// -----------------------------------------------------------------------.

func TestGetJobDependency_FoundAndNotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-get-dep-" + newID()
	job := mustCreateJob(t, ctx, q, projectID)
	depJob := mustCreateJob(t, ctx, q, projectID)

	dep := &domain.JobDependency{
		JobID:          job.ID,
		DependsOnJobID: depJob.ID,
	}
	if err := q.CreateJobDependency(ctx, dep); err != nil {
		t.Fatalf("CreateJobDependency: %v", err)
	}

	// Found path.
	got, err := q.GetJobDependency(ctx, dep.ID)
	if err != nil {
		t.Fatalf("GetJobDependency(%q): %v", dep.ID, err)
	}
	if got.ID != dep.ID {
		t.Fatalf("id = %q, want %q", got.ID, dep.ID)
	}
	if got.JobID != job.ID {
		t.Fatalf("job id = %q, want %q", got.JobID, job.ID)
	}
	if got.DependsOnJobID != depJob.ID {
		t.Fatalf("depends_on = %q, want %q", got.DependsOnJobID, depJob.ID)
	}

	// Not-found path.
	if _, err := q.GetJobDependency(ctx, "dep-does-not-exist-"+newID()); !errors.Is(err, store.ErrJobDependencyNotFound) {
		t.Fatalf("not-found err = %v, want ErrJobDependencyNotFound", err)
	}
}

// -----------------------------------------------------------------------.
// CountEnvironmentsByOrg
// -----------------------------------------------------------------------.

func TestCountEnvironmentsByOrg_CrossOrgAndSoftDelete(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgA := "org-env-a-" + newID()
	orgB := "org-env-b-" + newID()

	// Two projects in org A with 2 environments each.
	projectsA := make([]*domain.Project, 0, 2)
	for range 2 {
		p := &domain.Project{ID: "proj-a-" + newID(), Name: "A", OrgID: orgA}
		if err := q.CreateProject(ctx, p); err != nil {
			t.Fatalf("CreateProject(A): %v", err)
		}
		projectsA = append(projectsA, p)
		for range 2 {
			env := &domain.Environment{
				ID:        "env-" + newID(),
				ProjectID: p.ID,
				Name:      "env-" + newID(),
				Slug:      "env-" + newID(),
			}
			if err := q.CreateEnvironment(ctx, env); err != nil {
				t.Fatalf("CreateEnvironment(A): %v", err)
			}
		}
	}

	// One project in org B with 1 environment.
	projB := &domain.Project{ID: "proj-b-" + newID(), Name: "B", OrgID: orgB}
	if err := q.CreateProject(ctx, projB); err != nil {
		t.Fatalf("CreateProject(B): %v", err)
	}
	envB := &domain.Environment{
		ID:        "env-" + newID(),
		ProjectID: projB.ID,
		Name:      "env-" + newID(),
		Slug:      "env-" + newID(),
	}
	if err := q.CreateEnvironment(ctx, envB); err != nil {
		t.Fatalf("CreateEnvironment(B): %v", err)
	}

	countA, err := q.CountEnvironmentsByOrg(ctx, orgA)
	if err != nil {
		t.Fatalf("CountEnvironmentsByOrg(A): %v", err)
	}
	if countA != 4 {
		t.Fatalf("orgA count = %d, want 4", countA)
	}

	countB, err := q.CountEnvironmentsByOrg(ctx, orgB)
	if err != nil {
		t.Fatalf("CountEnvironmentsByOrg(B): %v", err)
	}
	if countB != 1 {
		t.Fatalf("orgB count = %d, want 1", countB)
	}

	// Soft-delete one of org A's projects: its 2 environments must drop
	// from the count because the subquery excludes deleted_at IS NOT NULL
	// projects.
	if _, err := testDB.Pool.Exec(ctx,
		`UPDATE projects SET deleted_at = NOW() WHERE id = $1`, projectsA[0].ID,
	); err != nil {
		t.Fatalf("soft-delete project: %v", err)
	}

	countA, err = q.CountEnvironmentsByOrg(ctx, orgA)
	if err != nil {
		t.Fatalf("CountEnvironmentsByOrg(A after soft-delete): %v", err)
	}
	if countA != 2 {
		t.Fatalf("orgA count after soft-delete = %d, want 2", countA)
	}

	// Empty org returns zero.
	countEmpty, err := q.CountEnvironmentsByOrg(ctx, "org-empty-"+newID())
	if err != nil {
		t.Fatalf("CountEnvironmentsByOrg(empty): %v", err)
	}
	if countEmpty != 0 {
		t.Fatalf("empty org count = %d, want 0", countEmpty)
	}
}

// -----------------------------------------------------------------------.
// CountWebhookSubscriptionsByOrg
// -----------------------------------------------------------------------.

func TestCountWebhookSubscriptionsByOrg_CrossOrgAndActiveFilter(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgA := "org-ws-a-" + newID()
	orgB := "org-ws-b-" + newID()

	projA := &domain.Project{ID: "proj-a-" + newID(), Name: "A", OrgID: orgA}
	if err := q.CreateProject(ctx, projA); err != nil {
		t.Fatalf("CreateProject(A): %v", err)
	}
	projB := &domain.Project{ID: "proj-b-" + newID(), Name: "B", OrgID: orgB}
	if err := q.CreateProject(ctx, projB); err != nil {
		t.Fatalf("CreateProject(B): %v", err)
	}

	// Seed three subscriptions in project A: two active, one inactive.
	// CountWebhookSubscriptionsByOrg must include only the active ones.
	activeA1 := &domain.WebhookSubscription{
		ProjectID:  projA.ID,
		WebhookURL: "https://example.com/a1-" + newID(),
		EventTypes: []string{"run.completed"},
		Secret:     "s",
		Active:     true,
	}
	if err := q.CreateWebhookSubscription(ctx, activeA1); err != nil {
		t.Fatalf("CreateWebhookSubscription(activeA1): %v", err)
	}
	activeA2 := &domain.WebhookSubscription{
		ProjectID:  projA.ID,
		WebhookURL: "https://example.com/a2-" + newID(),
		EventTypes: []string{"run.completed"},
		Secret:     "s",
		Active:     true,
	}
	if err := q.CreateWebhookSubscription(ctx, activeA2); err != nil {
		t.Fatalf("CreateWebhookSubscription(activeA2): %v", err)
	}
	inactiveA := &domain.WebhookSubscription{
		ProjectID:  projA.ID,
		WebhookURL: "https://example.com/a-inactive-" + newID(),
		EventTypes: []string{"run.completed"},
		Secret:     "s",
		Active:     false,
	}
	if err := q.CreateWebhookSubscription(ctx, inactiveA); err != nil {
		t.Fatalf("CreateWebhookSubscription(inactiveA): %v", err)
	}

	// One active in project B.
	activeB := &domain.WebhookSubscription{
		ProjectID:  projB.ID,
		WebhookURL: "https://example.com/b-" + newID(),
		EventTypes: []string{"run.completed"},
		Secret:     "s",
		Active:     true,
	}
	if err := q.CreateWebhookSubscription(ctx, activeB); err != nil {
		t.Fatalf("CreateWebhookSubscription(activeB): %v", err)
	}

	countA, err := q.CountWebhookSubscriptionsByOrg(ctx, orgA)
	if err != nil {
		t.Fatalf("CountWebhookSubscriptionsByOrg(A): %v", err)
	}
	if countA != 2 {
		t.Fatalf("orgA count = %d, want 2 (inactive should be excluded)", countA)
	}

	countB, err := q.CountWebhookSubscriptionsByOrg(ctx, orgB)
	if err != nil {
		t.Fatalf("CountWebhookSubscriptionsByOrg(B): %v", err)
	}
	if countB != 1 {
		t.Fatalf("orgB count = %d, want 1", countB)
	}

	// Soft-delete project A to prove the subquery excludes its
	// subscriptions from the org count.
	if _, err := testDB.Pool.Exec(ctx,
		`UPDATE projects SET deleted_at = NOW() WHERE id = $1`, projA.ID,
	); err != nil {
		t.Fatalf("soft-delete project: %v", err)
	}

	countA, err = q.CountWebhookSubscriptionsByOrg(ctx, orgA)
	if err != nil {
		t.Fatalf("CountWebhookSubscriptionsByOrg(A after soft-delete): %v", err)
	}
	if countA != 0 {
		t.Fatalf("orgA count after soft-delete = %d, want 0", countA)
	}
}
