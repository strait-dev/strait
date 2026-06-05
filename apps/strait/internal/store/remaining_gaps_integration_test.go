//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
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
// GetEventSubscription

func TestGetEventSubscription_FoundAndNotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	src := &domain.EventSource{
		ProjectID: "project-get-sub-" + newID(),
		Name:      "sub-source-" + newID(),
		Enabled:   true,
	}
	require.NoError(t, q.CreateEventSource(ctx,
		src))

	sub := &domain.EventSubscription{
		SourceID:   src.ID,
		TargetType: "job",
		TargetID:   newID(),
		FilterExpr: json.RawMessage(`{"event":"push"}`),
		Enabled:    true,
	}
	require.NoError(t, q.CreateEventSubscription(ctx, sub))

	// Found path.
	got, err := q.GetEventSubscription(ctx, sub.ID)
	require.NoError(t, err)
	require.Equal(t, sub.ID,

		got.ID)
	require.Equal(t, src.ID,

		got.SourceID,
	)
	require.Equal(t, "job",

		got.TargetType,
	)
	require.Equal(t, sub.TargetID,

		got.
			TargetID)
	require.True(t, got.Enabled)

	// Not-found path.
	if _, err := q.GetEventSubscription(ctx, "sub-does-not-exist-"+newID()); !errors.Is(err, store.ErrEventSubscriptionNotFound) {
		require.Failf(t, "test failure",

			"not-found err = %v, want ErrEventSubscriptionNotFound", err)
	}
}

// GetJobDependency

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
	require.NoError(t, q.CreateJobDependency(ctx,
		dep))

	// Found path.
	got, err := q.GetJobDependency(ctx, dep.ID)
	require.NoError(t, err)
	require.Equal(t, dep.ID,

		got.ID)
	require.Equal(t, job.ID,

		got.JobID,
	)
	require.Equal(t, depJob.
		ID, got.DependsOnJobID,
	)

	// Not-found path.
	if _, err := q.GetJobDependency(ctx, "dep-does-not-exist-"+newID()); !errors.Is(err, store.ErrJobDependencyNotFound) {
		require.Failf(t, "test failure",

			"not-found err = %v, want ErrJobDependencyNotFound", err)
	}
}

// CountEnvironmentsByOrg

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
		require.NoError(t, q.CreateProject(ctx, p))

		projectsA = append(projectsA, p)
		for range 2 {
			env := &domain.Environment{
				ID:        "env-" + newID(),
				ProjectID: p.ID,
				Name:      "env-" + newID(),
				Slug:      "env-" + newID(),
			}
			require.NoError(t, q.CreateEnvironment(ctx,
				env))

		}
	}

	// One project in org B with 1 environment.
	projB := &domain.Project{ID: "proj-b-" + newID(), Name: "B", OrgID: orgB}
	require.NoError(t, q.CreateProject(ctx, projB))

	envB := &domain.Environment{
		ID:        "env-" + newID(),
		ProjectID: projB.ID,
		Name:      "env-" + newID(),
		Slug:      "env-" + newID(),
	}
	require.NoError(t, q.CreateEnvironment(ctx,
		envB))

	countA, err := q.CountEnvironmentsByOrg(ctx, orgA)
	require.NoError(t, err)
	require.EqualValues(t, 4, countA)

	countB, err := q.CountEnvironmentsByOrg(ctx, orgB)
	require.NoError(t, err)
	require.EqualValues(t, 1, countB)

	// Soft-delete one of org A's projects: its 2 environments must drop
	// from the count because the subquery excludes deleted_at IS NOT NULL
	// projects.
	if _, err := testDB.Pool.Exec(ctx,
		`UPDATE projects SET deleted_at = NOW() WHERE id = $1`, projectsA[0].ID,
	); err != nil {
		require.Failf(t, "test failure",

			"soft-delete project: %v", err)
	}

	countA, err = q.CountEnvironmentsByOrg(ctx, orgA)
	require.NoError(t, err)
	require.EqualValues(t, 2, countA)

	// Empty org returns zero.
	countEmpty, err := q.CountEnvironmentsByOrg(ctx, "org-empty-"+newID())
	require.NoError(t, err)
	require.EqualValues(t, 0, countEmpty)

}

// CountWebhookSubscriptionsByOrg

func TestCountWebhookSubscriptionsByOrg_CrossOrgAndActiveFilter(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgA := "org-ws-a-" + newID()
	orgB := "org-ws-b-" + newID()

	projA := &domain.Project{ID: "proj-a-" + newID(), Name: "A", OrgID: orgA}
	require.NoError(t, q.CreateProject(ctx, projA))

	projB := &domain.Project{ID: "proj-b-" + newID(), Name: "B", OrgID: orgB}
	require.NoError(t, q.CreateProject(ctx, projB))

	// Seed three subscriptions in project A: two active, one inactive.
	// CountWebhookSubscriptionsByOrg must include only the active ones.
	activeA1 := &domain.WebhookSubscription{
		ProjectID:  projA.ID,
		WebhookURL: "https://example.com/a1-" + newID(),
		EventTypes: []string{"run.completed"},
		Secret:     "s",
		Active:     true,
	}
	require.NoError(t, q.CreateWebhookSubscription(ctx, activeA1))

	activeA2 := &domain.WebhookSubscription{
		ProjectID:  projA.ID,
		WebhookURL: "https://example.com/a2-" + newID(),
		EventTypes: []string{"run.completed"},
		Secret:     "s",
		Active:     true,
	}
	require.NoError(t, q.CreateWebhookSubscription(ctx, activeA2))

	inactiveA := &domain.WebhookSubscription{
		ProjectID:  projA.ID,
		WebhookURL: "https://example.com/a-inactive-" + newID(),
		EventTypes: []string{"run.completed"},
		Secret:     "s",
		Active:     false,
	}
	require.NoError(t, q.CreateWebhookSubscription(ctx, inactiveA))

	// One active in project B.
	activeB := &domain.WebhookSubscription{
		ProjectID:  projB.ID,
		WebhookURL: "https://example.com/b-" + newID(),
		EventTypes: []string{"run.completed"},
		Secret:     "s",
		Active:     true,
	}
	require.NoError(t, q.CreateWebhookSubscription(ctx, activeB))

	countA, err := q.CountWebhookSubscriptionsByOrg(ctx, orgA)
	require.NoError(t, err)
	require.EqualValues(t, 2, countA)

	countB, err := q.CountWebhookSubscriptionsByOrg(ctx, orgB)
	require.NoError(t, err)
	require.EqualValues(t, 1, countB)

	// Soft-delete project A to prove the subquery excludes its
	// subscriptions from the org count.
	if _, err := testDB.Pool.Exec(ctx,
		`UPDATE projects SET deleted_at = NOW() WHERE id = $1`, projA.ID,
	); err != nil {
		require.Failf(t, "test failure",

			"soft-delete project: %v", err)
	}

	countA, err = q.CountWebhookSubscriptionsByOrg(ctx, orgA)
	require.NoError(t, err)
	require.EqualValues(t, 0, countA)

}
