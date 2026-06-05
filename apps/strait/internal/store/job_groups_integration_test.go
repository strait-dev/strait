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

func TestCreateJobGroup(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	group := &domain.JobGroup{
		ProjectID:   "project-create-job-group",
		Name:        "Test Group",
		Slug:        "test-group",
		Description: "A test group",
	}
	require.NoError(t, q.CreateJobGroup(ctx, group))
	require.NotEqual(t, "",

		group.ID)
	require.False(t, group.
		CreatedAt.
		IsZero())

}

func TestGetJobGroup(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	group := &domain.JobGroup{
		ProjectID:   "project-get-job-group",
		Name:        "Get Group",
		Slug:        "get-group",
		Description: "Group for get test",
	}
	require.NoError(t, q.CreateJobGroup(ctx, group))

	got, err := q.GetJobGroup(ctx, group.ID)
	require.NoError(t, err)
	require.Equal(t, "Get Group",

		got.
			Name)

	// Not found.
	_, err = q.GetJobGroup(ctx, newID())
	require.True(t, errors.Is(err, store.
		ErrJobGroupNotFound,
	))

}

func TestListJobGroups(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-list-job-groups"
	for i := range 3 {
		group := &domain.JobGroup{
			ProjectID: projectID,
			Name:      "Group " + string(rune('A'+i)),
			Slug:      "group-" + string(rune('a'+i)),
		}
		require.NoError(t, q.CreateJobGroup(ctx, group))

	}

	groups, err := q.ListJobGroups(ctx, projectID, 100, nil)
	require.NoError(t, err)
	require.Len(t, groups,
		3,
	)

	// Empty project.
	empty, err := q.ListJobGroups(ctx, "nonexistent", 100, nil)
	require.NoError(t, err)
	require.Len(t, empty, 0)

}

func TestUpdateJobGroup(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	group := &domain.JobGroup{
		ProjectID: "project-update-job-group",
		Name:      "Original",
		Slug:      "original",
	}
	require.NoError(t, q.CreateJobGroup(ctx, group))

	group.Name = "Updated"
	group.Slug = "updated"
	group.Description = "Updated description"
	require.NoError(t, q.UpdateJobGroup(ctx, group))

	got, err := q.GetJobGroup(ctx, group.ID)
	require.NoError(t, err)
	require.Equal(t, "Updated",

		got.Name,
	)

	// Not found.
	notFound := &domain.JobGroup{ID: newID(), ProjectID: "x", Name: "x", Slug: "x"}
	if err := q.UpdateJobGroup(ctx, notFound); !errors.Is(err, store.ErrJobGroupNotFound) {
		require.Failf(t, "test failure",

			"UpdateJobGroup(notfound) error = %v, want ErrJobGroupNotFound", err)
	}
}

func TestDeleteJobGroup(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	group := &domain.JobGroup{
		ProjectID: "project-delete-job-group",
		Name:      "Delete Me",
		Slug:      "delete-me",
	}
	require.NoError(t, q.CreateJobGroup(ctx, group))
	require.NoError(t, q.DeleteJobGroup(ctx, group.
		ID))

	_, err := q.GetJobGroup(ctx, group.ID)
	require.True(t, errors.Is(err, store.
		ErrJobGroupNotFound,
	))

	// Not found.
	if err := q.DeleteJobGroup(ctx, newID()); !errors.Is(err, store.ErrJobGroupNotFound) {
		require.Failf(t, "test failure",

			"DeleteJobGroup(notfound) error = %v, want ErrJobGroupNotFound", err)
	}
}

func TestListJobsByGroup_ReturnsOnlyGroupedJobs(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-jobs-by-group"
	group := &domain.JobGroup{
		ProjectID: projectID,
		Name:      "Jobs Group",
		Slug:      "jobs-group",
	}
	require.NoError(t, q.CreateJobGroup(ctx, group))

	job := baseJob(newID(), projectID)
	job.GroupID = group.ID
	require.NoError(t, q.CreateJob(ctx,
		job))

	jobs, err := q.ListJobsByGroup(ctx, group.ID, 100, nil)
	require.NoError(t, err)
	require.Len(t, jobs, 1)

	// Empty group.
	empty, err := q.ListJobsByGroup(ctx, newID(), 100, nil)
	require.NoError(t, err)
	require.Len(t, empty, 0)

}

func TestPauseAndResumeJobsByGroup(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-pause-resume-group"
	group := &domain.JobGroup{
		ProjectID: projectID,
		Name:      "Pause Group",
		Slug:      "pause-group",
	}
	require.NoError(t, q.CreateJobGroup(ctx, group))

	job := baseJob(newID(), projectID)
	job.GroupID = group.ID
	require.NoError(t, q.CreateJob(ctx,
		job))
	require.NoError(t, q.PauseJobsByGroup(ctx, group.
		ID))

	paused, err := q.GetJob(ctx, job.ID)
	require.NoError(t, err)
	require.True(t, paused.
		Paused,
	)
	require.NoError(t, q.ResumeJobsByGroup(ctx,
		group.ID))

	resumed, err := q.GetJob(ctx, job.ID)
	require.NoError(t, err)
	require.False(t, resumed.
		Paused)

	// Not found group.
	if err := q.PauseJobsByGroup(ctx, newID()); !errors.Is(err, store.ErrJobGroupNotFound) {
		require.Failf(t, "test failure",

			"PauseJobsByGroup(notfound) error = %v, want ErrJobGroupNotFound", err)
	}
	if err := q.ResumeJobsByGroup(ctx, newID()); !errors.Is(err, store.ErrJobGroupNotFound) {
		require.Failf(t, "test failure",

			"ResumeJobsByGroup(notfound) error = %v, want ErrJobGroupNotFound", err)
	}
}

func TestGetJobGroupStats(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-group-stats"
	group := &domain.JobGroup{
		ProjectID: projectID,
		Name:      "Stats Group",
		Slug:      "stats-group",
	}
	require.NoError(t, q.CreateJobGroup(ctx, group))

	job := baseJob(newID(), projectID)
	job.GroupID = group.ID
	require.NoError(t, q.CreateJob(ctx,
		job))

	run := baseRun(job, newID())
	run.Status = domain.StatusCompleted
	require.NoError(t, q.CreateRun(ctx,
		run))

	stats, err := q.GetJobGroupStats(ctx, group.ID)
	require.NoError(t, err)
	require.Equal(t, group.
		ID,
		stats.
			GroupID)
	require.EqualValues(t, 1, stats.
		RunCounts["completed"])

	// Not found.
	_, err = q.GetJobGroupStats(ctx, newID())
	require.True(t, errors.Is(err, store.
		ErrJobGroupNotFound,
	))

}
