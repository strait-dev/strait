//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

func TestCreateProject(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	st := mustStore(t)

	p := &domain.Project{
		ID:    newID(),
		OrgID: "org-proj-create",
		Name:  "Test Project",
	}
	require.NoError(t, st.CreateProject(ctx, p))

	got, err := st.GetProject(ctx, p.ID)
	require.NoError(t, err)
	require.Equal(t, p.ID,
		got.
			ID)
	require.Equal(t, p.OrgID,

		got.OrgID,
	)
	require.Equal(t, p.Name,

		got.Name,
	)
	require.False(t, got.CreatedAt.
		IsZero())
	require.False(t, got.UpdatedAt.
		IsZero())

}

func TestCreateProject_Upsert(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	st := mustStore(t)

	id := newID()
	p := &domain.Project{ID: id, OrgID: "org-upsert", Name: "Original"}
	require.NoError(t, st.CreateProject(ctx, p))

	p2 := &domain.Project{ID: id, OrgID: "org-upsert", Name: "Updated"}
	require.NoError(t, st.CreateProject(ctx, p2))

	got, err := st.GetProject(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "Updated",

		got.Name,
	)

}

func TestCreateProject_UpsertPreservesOrgID(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	st := mustStore(t)

	id := newID()
	p := &domain.Project{ID: id, OrgID: "org-preserve", Name: "First"}
	require.NoError(t, st.CreateProject(ctx, p))

	// Upsert with empty org_id should preserve existing.
	p2 := &domain.Project{ID: id, OrgID: "", Name: "Second"}
	require.NoError(t, st.CreateProject(ctx, p2))

	got, err := st.GetProject(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "org-preserve",

		got.OrgID)

}

func TestGetProject_NotFound(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	st := mustStore(t)

	_, err := st.GetProject(ctx, "nonexistent-project")
	require.True(t, errors.Is(err, store.
		ErrProjectNotFound,
	))

}

func TestListProjectsByOrg(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	st := mustStore(t)

	orgA := "org-list-a"
	orgB := "org-list-b"

	for i, org := range []string{orgA, orgA, orgB} {
		p := &domain.Project{ID: newID(), OrgID: org, Name: "Project " + string(rune('A'+i))}
		require.NoError(t, st.CreateProject(ctx, p))

	}

	got, err := st.ListProjectsByOrg(ctx, orgA)
	require.NoError(t, err)
	require.Len(t, got, 2)

	for _, p := range got {
		require.Equal(t, orgA,
			p.
				OrgID)

	}
}

func TestListProjectsByOrg_Empty(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	st := mustStore(t)

	got, err := st.ListProjectsByOrg(ctx, "org-empty")
	require.NoError(t, err)
	require.Len(t, got, 0)

}

func TestDeleteProject(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	st := mustStore(t)

	p := &domain.Project{ID: newID(), OrgID: "org-delete", Name: "Delete Me"}
	require.NoError(t, st.CreateProject(ctx, p))
	require.NoError(t, st.DeleteProject(ctx, p.ID))

	_, err := st.GetProject(ctx, p.ID)
	require.True(t, errors.Is(err, store.
		ErrProjectNotFound,
	))

	count, err := st.CountProjectsByOrg(ctx, p.OrgID)
	require.NoError(t, err)
	require.EqualValues(t, 0, count)

	projects, err := st.ListProjectsByOrg(ctx, p.OrgID)
	require.NoError(t, err)
	require.Len(t, projects,

		0)

}

func TestDeleteProject_DisablesWorkflows(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	st := mustStore(t)

	p := &domain.Project{ID: newID(), OrgID: "org-delete-workflows", Name: "Delete Workflows"}
	require.NoError(t, st.CreateProject(ctx, p))

	wf := &domain.Workflow{
		ID:        newID(),
		ProjectID: p.ID,
		Name:      "delete-project-workflow",
		Slug:      "delete-project-workflow",
		Enabled:   true,
		Version:   1,
		Cron:      "*/5 * * * *",
	}
	require.NoError(t, st.CreateWorkflow(ctx, wf))
	require.NoError(t, st.DeleteProject(ctx, p.ID))

	var enabled bool
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT enabled FROM workflows WHERE id = $1`,

		wf.ID).Scan(&enabled))
	require.False(t, enabled)

}

func TestDeleteProject_RecreateRevivesProject(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	st := mustStore(t)

	projectID := newID()
	initial := &domain.Project{ID: projectID, OrgID: "org-revive", Name: "Original"}
	require.NoError(t, st.CreateProject(ctx, initial))
	require.NoError(t, st.DeleteProject(ctx, projectID))

	recreated := &domain.Project{ID: projectID, OrgID: "org-revive", Name: "Recreated"}
	require.NoError(t, st.CreateProject(ctx, recreated))

	got, err := st.GetProject(ctx, projectID)
	require.NoError(t, err)
	require.Equal(t, "Recreated",

		got.
			Name)

	count, err := st.CountProjectsByOrg(ctx, "org-revive")
	require.NoError(t, err)
	require.EqualValues(t, 1, count)

}

func TestDeleteProject_NonExistent(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	st := mustStore(t)

	err := st.DeleteProject(ctx, "nonexistent-project")
	require.True(t, errors.Is(err, store.
		ErrProjectNotFound,
	))

}

func TestCountProjectsByOrg(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)
	st := mustStore(t)

	org := "org-count"
	for range 3 {
		p := &domain.Project{ID: newID(), OrgID: org, Name: "P"}
		require.NoError(t, st.CreateProject(ctx, p))

	}
	// Different org
	p := &domain.Project{ID: newID(), OrgID: "org-other", Name: "P"}
	require.NoError(t, st.CreateProject(ctx, p))

	count, err := st.CountProjectsByOrg(ctx, org)
	require.NoError(t, err)
	require.EqualValues(t, 3, count)

	count2, err := st.CountProjectsByOrg(ctx, "org-other")
	require.NoError(t, err)
	require.EqualValues(t, 1, count2)

}
