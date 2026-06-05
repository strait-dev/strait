//go:build integration

package store_test

import (
	"context"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

func TestPauseJob_Success(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-pause-store")
	require.NoError(t, q.PauseJob(ctx,
		job.ID, "investigating spike",
	))

	got, err := q.GetJob(ctx, job.ID)
	require.NoError(t, err)
	require.True(t, got.Paused)
	require.NotNil(t, got.PausedAt)
	require.LessOrEqual(t,
		time.
			Since(*got.PausedAt), 5*time.
			Second,
	)
	require.Equal(t, "investigating spike",

		got.
			PauseReason,
	)

}

func TestPauseJob_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	err := q.PauseJob(ctx, "nonexistent-job-id", "reason")
	require.Error(t, err)

}

func TestResumeJob_Success(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-resume-store")
	require.NoError(t, q.PauseJob(ctx,
		job.ID, "pausing first",
	))
	require.NoError(t, q.ResumeJob(ctx,
		job.ID))

	got, err := q.GetJob(ctx, job.ID)
	require.NoError(t, err)
	require.False(t, got.Paused)
	require.Nil(t, got.
		PausedAt,
	)
	require.Equal(t, "", got.
		PauseReason,
	)

}

func TestPauseJob_SkipsDuplicateSameReasonWrite(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-pause-noop")
	require.NoError(t, q.PauseJob(ctx,
		job.ID, "incident",
	))

	var pausedXmin string
	var pausedAt time.Time
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT xmin::text, paused_at
		FROM jobs
		WHERE id = $1`,

		job.ID).Scan(&pausedXmin, &pausedAt))
	require.NoError(t, q.PauseJob(ctx,
		job.ID, "incident",
	))

	var duplicateXmin string
	var duplicatePausedAt time.Time
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT xmin::text, paused_at
		FROM jobs
		WHERE id = $1`,

		job.ID).Scan(&duplicateXmin,
		&duplicatePausedAt))
	require.Equal(t, pausedXmin,

		duplicateXmin,
	)
	require.True(t, duplicatePausedAt.
		Equal(pausedAt))
	require.NoError(t, q.PauseJob(ctx,
		job.ID, "new incident",
	))

	got, err := q.GetJob(ctx, job.ID)
	require.NoError(t, err)
	require.Equal(t, "new incident",

		got.PauseReason,
	)

	var changedXmin string
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT xmin::text FROM jobs WHERE id = $1`,

		job.
			ID).Scan(&changedXmin))
	require.NotEqual(t, duplicateXmin,

		changedXmin,
	)

}

func TestResumeJob_SkipsDuplicateWrite(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-resume-noop")
	require.NoError(t, q.PauseJob(ctx,
		job.ID, "temporary",
	),
	)
	require.NoError(t, q.ResumeJob(ctx,
		job.ID))

	var resumedXmin string
	var resumedUpdatedAt time.Time
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT xmin::text, updated_at
		FROM jobs
		WHERE id = $1`,

		job.ID).Scan(&resumedXmin,
		&resumedUpdatedAt))
	require.NoError(t, q.ResumeJob(ctx,
		job.ID))

	var duplicateXmin string
	var duplicateUpdatedAt time.Time
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT xmin::text, updated_at
		FROM jobs
		WHERE id = $1`,

		job.ID).Scan(&duplicateXmin,
		&duplicateUpdatedAt))
	require.Equal(t, resumedXmin,

		duplicateXmin,
	)
	require.True(t, duplicateUpdatedAt.
		Equal(resumedUpdatedAt))

}

func TestResumeJob_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	err := q.ResumeJob(ctx, "nonexistent-job-id")
	require.Error(t, err)

}

func TestCreateJob_DefaultPausedFalse(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-default-pause")

	got, err := q.GetJob(ctx, job.ID)
	require.NoError(t, err)
	require.False(t, got.Paused)
	require.Nil(t, got.
		PausedAt,
	)
	require.Equal(t, "", got.
		PauseReason,
	)

}

func TestUpdateJob_PreservesPauseState(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-update-pause")
	require.NoError(t, q.PauseJob(ctx,
		job.ID, "preserve this",
	))

	// Re-read to get latest version.
	job, err := q.GetJob(ctx, job.ID)
	require.NoError(t, err)

	// Update an unrelated field.
	job.Name = "updated-name"
	require.NoError(t, q.UpdateJob(ctx,
		job))

	got, err := q.GetJob(ctx, job.ID)
	require.NoError(t, err)
	require.Equal(t, "updated-name",

		got.Name)
	require.True(t, got.Paused)
	require.Equal(t, "preserve this",

		got.PauseReason,
	)

}

func TestListCronJobs_ExcludesPausedJobs(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-cron-pause")

	// Verify the cron job appears before pausing.
	jobsBefore, err := q.ListCronJobs(ctx)
	require.NoError(t, err)

	var foundBefore bool
	for _, j := range jobsBefore {
		if j.ID == job.ID {
			foundBefore = true
			break
		}
	}
	require.True(t, foundBefore)
	require.NoError(t, q.PauseJob(ctx,
		job.ID, "paused cron job",
	))

	// Pause the job.

	// Paused cron job should NOT appear in ListCronJobs.
	jobsAfter, err := q.ListCronJobs(ctx)
	require.NoError(t, err)

	for _, j := range jobsAfter {
		require.NotEqual(t, job.
			ID, j.ID)

	}
}

func TestListCronJobs_ResumedJobReappears(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-cron-resume")
	require.NoError(t, q.PauseJob(ctx,
		job.ID, "temporary pause",
	))
	require.NoError(t, q.ResumeJob(ctx,
		job.ID))

	// Pause then resume.

	jobs, err := q.ListCronJobs(ctx)
	require.NoError(t, err)

	var found bool
	for _, j := range jobs {
		if j.ID == job.ID {
			found = true
			break
		}
	}
	require.True(t, found)

}

func TestListCronJobs_MixedPausedAndActive(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-cron-mixed"
	pausedJob := mustCreateJob(t, ctx, q, projectID)
	activeJob := mustCreateJob(t, ctx, q, projectID)
	require.NoError(t, q.PauseJob(ctx,
		pausedJob.
			ID, "paused",
	))

	jobs, err := q.ListCronJobs(ctx)
	require.NoError(t, err)

	var foundPaused, foundActive bool
	for _, j := range jobs {
		if j.ID == pausedJob.ID {
			foundPaused = true
		}
		if j.ID == activeJob.ID {
			foundActive = true
		}
	}
	require.False(t, foundPaused)
	require.True(t, foundActive)

}

func TestListCronJobs_LongPauseNoCronAccumulation(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-cron-longpause")
	require.NoError(t, q.PauseJob(ctx,
		job.ID, "long maintenance window",
	))

	// Pause the job.

	// Simulate multiple cron scheduler ticks -- each calls ListCronJobs.
	// The paused job should never be returned, so no runs would be queued.
	for range 10 {
		jobs, err := q.ListCronJobs(ctx)
		require.NoError(t, err)

		for _, j := range jobs {
			require.NotEqual(t, job.
				ID, j.ID)

		}
	}
	require.NoError(t, q.ResumeJob(ctx,
		job.ID))

	// Resume and verify it appears again.

	jobs, err := q.ListCronJobs(ctx)
	require.NoError(t, err)

	var found bool
	for _, j := range jobs {
		if j.ID == job.ID {
			found = true
			break
		}
	}
	require.True(t, found)

}

func TestGroupPause_ExcludesCronJobs(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	// Create a group and a cron job in it.
	group := &domain.JobGroup{
		ID:        newID(),
		ProjectID: "project-group-cron",
		Name:      "test-group",
	}
	require.NoError(t, q.CreateJobGroup(ctx, group))

	job := baseJob(newID(), "project-group-cron")
	job.GroupID = group.ID
	require.NoError(t, q.CreateJob(ctx,
		job))
	require.NoError(t, q.PauseJobsByGroup(ctx, group.
		ID))

	// Pause via group.

	// Cron should not see the group-paused job.
	jobs, err := q.ListCronJobs(ctx)
	require.NoError(t, err)

	for _, j := range jobs {
		require.NotEqual(t, job.
			ID, j.ID)

	}
	require.NoError(t, q.ResumeJobsByGroup(ctx,
		group.ID))

	// Resume via group.

	// Should reappear.
	jobs, err = q.ListCronJobs(ctx)
	require.NoError(t, err)

	var found bool
	for _, j := range jobs {
		if j.ID == job.ID {
			found = true
			break
		}
	}
	require.True(t, found)

}

func TestPauseJobsByGroup_SkipsDuplicateGroupPauseWrites(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	group := &domain.JobGroup{
		ID:        newID(),
		ProjectID: "project-group-pause-noop",
		Name:      "test-group",
	}
	require.NoError(t, q.CreateJobGroup(ctx, group))

	job := baseJob(newID(), group.ProjectID)
	job.GroupID = group.ID
	require.NoError(t, q.CreateJob(ctx,
		job))
	require.NoError(t, q.PauseJobsByGroup(ctx, group.
		ID))

	var pausedXmin string
	var pausedAt time.Time
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT xmin::text, paused_at
		FROM jobs
		WHERE id = $1`,

		job.ID).Scan(&pausedXmin, &pausedAt))
	require.NoError(t, q.PauseJobsByGroup(ctx, group.
		ID))

	var duplicateXmin string
	var duplicatePausedAt time.Time
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT xmin::text, paused_at
		FROM jobs
		WHERE id = $1`,

		job.ID).Scan(&duplicateXmin,
		&duplicatePausedAt))
	require.Equal(t, pausedXmin,

		duplicateXmin,
	)
	require.True(t, duplicatePausedAt.
		Equal(pausedAt))
	require.NoError(t, q.ResumeJobsByGroup(ctx,
		group.ID))

	var resumedXmin string
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT xmin::text FROM jobs WHERE id = $1`,

		job.
			ID).Scan(&resumedXmin))
	require.NotEqual(t, duplicateXmin,

		resumedXmin,
	)
	require.NoError(t, q.ResumeJobsByGroup(ctx,
		group.ID))

	var duplicateResumeXmin string
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT xmin::text FROM jobs WHERE id = $1`,

		job.
			ID).Scan(&duplicateResumeXmin))
	require.Equal(t, resumedXmin,

		duplicateResumeXmin,
	)

}
