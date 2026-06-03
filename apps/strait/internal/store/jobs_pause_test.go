//go:build integration

package store_test

import (
	"context"
	"testing"
	"time"

	"strait/internal/domain"
)

func TestPauseJob_Success(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-pause-store")

	if err := q.PauseJob(ctx, job.ID, "investigating spike"); err != nil {
		t.Fatalf("PauseJob() error = %v", err)
	}

	got, err := q.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetJob() error = %v", err)
	}
	if !got.Paused {
		t.Fatal("expected Paused=true after PauseJob")
	}
	if got.PausedAt == nil {
		t.Fatal("expected PausedAt to be set")
	}
	if time.Since(*got.PausedAt) > 5*time.Second {
		t.Fatalf("PausedAt too old: %v", got.PausedAt)
	}
	if got.PauseReason != "investigating spike" {
		t.Fatalf("expected PauseReason='investigating spike', got %q", got.PauseReason)
	}
}

func TestPauseJob_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	err := q.PauseJob(ctx, "nonexistent-job-id", "reason")
	if err == nil {
		t.Fatal("expected error for nonexistent job")
	}
}

func TestResumeJob_Success(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-resume-store")

	if err := q.PauseJob(ctx, job.ID, "pausing first"); err != nil {
		t.Fatalf("PauseJob() error = %v", err)
	}

	if err := q.ResumeJob(ctx, job.ID); err != nil {
		t.Fatalf("ResumeJob() error = %v", err)
	}

	got, err := q.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetJob() error = %v", err)
	}
	if got.Paused {
		t.Fatal("expected Paused=false after ResumeJob")
	}
	if got.PausedAt != nil {
		t.Fatalf("expected PausedAt=nil after ResumeJob, got %v", got.PausedAt)
	}
	if got.PauseReason != "" {
		t.Fatalf("expected PauseReason='' after ResumeJob, got %q", got.PauseReason)
	}
}

func TestPauseJob_SkipsDuplicateSameReasonWrite(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-pause-noop")
	if err := q.PauseJob(ctx, job.ID, "incident"); err != nil {
		t.Fatalf("PauseJob() error = %v", err)
	}

	var pausedXmin string
	var pausedAt time.Time
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT xmin::text, paused_at
		FROM jobs
		WHERE id = $1`,
		job.ID,
	).Scan(&pausedXmin, &pausedAt); err != nil {
		t.Fatalf("query paused job version: %v", err)
	}

	if err := q.PauseJob(ctx, job.ID, "incident"); err != nil {
		t.Fatalf("duplicate PauseJob() error = %v", err)
	}

	var duplicateXmin string
	var duplicatePausedAt time.Time
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT xmin::text, paused_at
		FROM jobs
		WHERE id = $1`,
		job.ID,
	).Scan(&duplicateXmin, &duplicatePausedAt); err != nil {
		t.Fatalf("query duplicate paused job version: %v", err)
	}
	if duplicateXmin != pausedXmin {
		t.Fatalf("duplicate pause changed xmin from %s to %s", pausedXmin, duplicateXmin)
	}
	if !duplicatePausedAt.Equal(pausedAt) {
		t.Fatalf("duplicate pause changed paused_at from %s to %s", pausedAt, duplicatePausedAt)
	}

	if err := q.PauseJob(ctx, job.ID, "new incident"); err != nil {
		t.Fatalf("PauseJob() with new reason error = %v", err)
	}
	got, err := q.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetJob() error = %v", err)
	}
	if got.PauseReason != "new incident" {
		t.Fatalf("pause reason = %q, want new incident", got.PauseReason)
	}
	var changedXmin string
	if err := testDB.Pool.QueryRow(ctx, `SELECT xmin::text FROM jobs WHERE id = $1`, job.ID).Scan(&changedXmin); err != nil {
		t.Fatalf("query changed paused job version: %v", err)
	}
	if changedXmin == duplicateXmin {
		t.Fatalf("pause reason change kept xmin %s, want a real update", changedXmin)
	}
}

func TestResumeJob_SkipsDuplicateWrite(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-resume-noop")
	if err := q.PauseJob(ctx, job.ID, "temporary"); err != nil {
		t.Fatalf("PauseJob() error = %v", err)
	}
	if err := q.ResumeJob(ctx, job.ID); err != nil {
		t.Fatalf("ResumeJob() error = %v", err)
	}

	var resumedXmin string
	var resumedUpdatedAt time.Time
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT xmin::text, updated_at
		FROM jobs
		WHERE id = $1`,
		job.ID,
	).Scan(&resumedXmin, &resumedUpdatedAt); err != nil {
		t.Fatalf("query resumed job version: %v", err)
	}

	if err := q.ResumeJob(ctx, job.ID); err != nil {
		t.Fatalf("duplicate ResumeJob() error = %v", err)
	}

	var duplicateXmin string
	var duplicateUpdatedAt time.Time
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT xmin::text, updated_at
		FROM jobs
		WHERE id = $1`,
		job.ID,
	).Scan(&duplicateXmin, &duplicateUpdatedAt); err != nil {
		t.Fatalf("query duplicate resumed job version: %v", err)
	}
	if duplicateXmin != resumedXmin {
		t.Fatalf("duplicate resume changed xmin from %s to %s", resumedXmin, duplicateXmin)
	}
	if !duplicateUpdatedAt.Equal(resumedUpdatedAt) {
		t.Fatalf("duplicate resume changed updated_at from %s to %s", resumedUpdatedAt, duplicateUpdatedAt)
	}
}

func TestResumeJob_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	err := q.ResumeJob(ctx, "nonexistent-job-id")
	if err == nil {
		t.Fatal("expected error for nonexistent job")
	}
}

func TestCreateJob_DefaultPausedFalse(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-default-pause")

	got, err := q.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetJob() error = %v", err)
	}
	if got.Paused {
		t.Fatal("expected new job to have Paused=false")
	}
	if got.PausedAt != nil {
		t.Fatalf("expected new job to have PausedAt=nil, got %v", got.PausedAt)
	}
	if got.PauseReason != "" {
		t.Fatalf("expected new job to have PauseReason='', got %q", got.PauseReason)
	}
}

func TestUpdateJob_PreservesPauseState(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-update-pause")

	if err := q.PauseJob(ctx, job.ID, "preserve this"); err != nil {
		t.Fatalf("PauseJob() error = %v", err)
	}

	// Re-read to get latest version.
	job, err := q.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetJob() error = %v", err)
	}

	// Update an unrelated field.
	job.Name = "updated-name"
	if err := q.UpdateJob(ctx, job); err != nil {
		t.Fatalf("UpdateJob() error = %v", err)
	}

	got, err := q.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetJob() error = %v", err)
	}
	if got.Name != "updated-name" {
		t.Fatalf("expected name 'updated-name', got %q", got.Name)
	}
	if !got.Paused {
		t.Fatal("expected Paused to remain true after unrelated update")
	}
	if got.PauseReason != "preserve this" {
		t.Fatalf("expected PauseReason preserved, got %q", got.PauseReason)
	}
}

func TestListCronJobs_ExcludesPausedJobs(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-cron-pause")

	// Verify the cron job appears before pausing.
	jobsBefore, err := q.ListCronJobs(ctx)
	if err != nil {
		t.Fatalf("ListCronJobs() error = %v", err)
	}
	var foundBefore bool
	for _, j := range jobsBefore {
		if j.ID == job.ID {
			foundBefore = true
			break
		}
	}
	if !foundBefore {
		t.Fatal("expected cron job to appear in ListCronJobs before pausing")
	}

	// Pause the job.
	if err := q.PauseJob(ctx, job.ID, "paused cron job"); err != nil {
		t.Fatalf("PauseJob() error = %v", err)
	}

	// Paused cron job should NOT appear in ListCronJobs.
	jobsAfter, err := q.ListCronJobs(ctx)
	if err != nil {
		t.Fatalf("ListCronJobs() error = %v", err)
	}
	for _, j := range jobsAfter {
		if j.ID == job.ID {
			t.Fatal("paused cron job should NOT appear in ListCronJobs")
		}
	}
}

func TestListCronJobs_ResumedJobReappears(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-cron-resume")

	// Pause then resume.
	if err := q.PauseJob(ctx, job.ID, "temporary pause"); err != nil {
		t.Fatalf("PauseJob() error = %v", err)
	}
	if err := q.ResumeJob(ctx, job.ID); err != nil {
		t.Fatalf("ResumeJob() error = %v", err)
	}

	jobs, err := q.ListCronJobs(ctx)
	if err != nil {
		t.Fatalf("ListCronJobs() error = %v", err)
	}

	var found bool
	for _, j := range jobs {
		if j.ID == job.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected resumed cron job to reappear in ListCronJobs")
	}
}

func TestListCronJobs_MixedPausedAndActive(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-cron-mixed"
	pausedJob := mustCreateJob(t, ctx, q, projectID)
	activeJob := mustCreateJob(t, ctx, q, projectID)

	if err := q.PauseJob(ctx, pausedJob.ID, "paused"); err != nil {
		t.Fatalf("PauseJob() error = %v", err)
	}

	jobs, err := q.ListCronJobs(ctx)
	if err != nil {
		t.Fatalf("ListCronJobs() error = %v", err)
	}

	var foundPaused, foundActive bool
	for _, j := range jobs {
		if j.ID == pausedJob.ID {
			foundPaused = true
		}
		if j.ID == activeJob.ID {
			foundActive = true
		}
	}
	if foundPaused {
		t.Fatal("paused job should NOT appear in ListCronJobs")
	}
	if !foundActive {
		t.Fatal("active job should appear in ListCronJobs")
	}
}

func TestListCronJobs_LongPauseNoCronAccumulation(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-cron-longpause")

	// Pause the job.
	if err := q.PauseJob(ctx, job.ID, "long maintenance window"); err != nil {
		t.Fatalf("PauseJob() error = %v", err)
	}

	// Simulate multiple cron scheduler ticks -- each calls ListCronJobs.
	// The paused job should never be returned, so no runs would be queued.
	for i := range 10 {
		jobs, err := q.ListCronJobs(ctx)
		if err != nil {
			t.Fatalf("ListCronJobs() tick %d error = %v", i, err)
		}
		for _, j := range jobs {
			if j.ID == job.ID {
				t.Fatalf("paused job should not appear in ListCronJobs on tick %d", i)
			}
		}
	}

	// Resume and verify it appears again.
	if err := q.ResumeJob(ctx, job.ID); err != nil {
		t.Fatalf("ResumeJob() error = %v", err)
	}

	jobs, err := q.ListCronJobs(ctx)
	if err != nil {
		t.Fatalf("ListCronJobs() after resume error = %v", err)
	}
	var found bool
	for _, j := range jobs {
		if j.ID == job.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected job to reappear after resume")
	}
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
	if err := q.CreateJobGroup(ctx, group); err != nil {
		t.Fatalf("CreateJobGroup() error = %v", err)
	}

	job := baseJob(newID(), "project-group-cron")
	job.GroupID = group.ID
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	// Pause via group.
	if err := q.PauseJobsByGroup(ctx, group.ID); err != nil {
		t.Fatalf("PauseJobsByGroup() error = %v", err)
	}

	// Cron should not see the group-paused job.
	jobs, err := q.ListCronJobs(ctx)
	if err != nil {
		t.Fatalf("ListCronJobs() error = %v", err)
	}
	for _, j := range jobs {
		if j.ID == job.ID {
			t.Fatal("group-paused job should NOT appear in ListCronJobs")
		}
	}

	// Resume via group.
	if err := q.ResumeJobsByGroup(ctx, group.ID); err != nil {
		t.Fatalf("ResumeJobsByGroup() error = %v", err)
	}

	// Should reappear.
	jobs, err = q.ListCronJobs(ctx)
	if err != nil {
		t.Fatalf("ListCronJobs() after group resume error = %v", err)
	}
	var found bool
	for _, j := range jobs {
		if j.ID == job.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected job to reappear after group resume")
	}
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
	if err := q.CreateJobGroup(ctx, group); err != nil {
		t.Fatalf("CreateJobGroup() error = %v", err)
	}

	job := baseJob(newID(), group.ProjectID)
	job.GroupID = group.ID
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	if err := q.PauseJobsByGroup(ctx, group.ID); err != nil {
		t.Fatalf("PauseJobsByGroup() error = %v", err)
	}
	var pausedXmin string
	var pausedAt time.Time
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT xmin::text, paused_at
		FROM jobs
		WHERE id = $1`,
		job.ID,
	).Scan(&pausedXmin, &pausedAt); err != nil {
		t.Fatalf("query group-paused job version: %v", err)
	}

	if err := q.PauseJobsByGroup(ctx, group.ID); err != nil {
		t.Fatalf("duplicate PauseJobsByGroup() error = %v", err)
	}
	var duplicateXmin string
	var duplicatePausedAt time.Time
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT xmin::text, paused_at
		FROM jobs
		WHERE id = $1`,
		job.ID,
	).Scan(&duplicateXmin, &duplicatePausedAt); err != nil {
		t.Fatalf("query duplicate group-paused job version: %v", err)
	}
	if duplicateXmin != pausedXmin {
		t.Fatalf("duplicate group pause changed xmin from %s to %s", pausedXmin, duplicateXmin)
	}
	if !duplicatePausedAt.Equal(pausedAt) {
		t.Fatalf("duplicate group pause changed paused_at from %s to %s", pausedAt, duplicatePausedAt)
	}

	if err := q.ResumeJobsByGroup(ctx, group.ID); err != nil {
		t.Fatalf("ResumeJobsByGroup() error = %v", err)
	}
	var resumedXmin string
	if err := testDB.Pool.QueryRow(ctx, `SELECT xmin::text FROM jobs WHERE id = $1`, job.ID).Scan(&resumedXmin); err != nil {
		t.Fatalf("query group-resumed job version: %v", err)
	}
	if resumedXmin == duplicateXmin {
		t.Fatalf("group resume kept xmin %s, want a real update", resumedXmin)
	}

	if err := q.ResumeJobsByGroup(ctx, group.ID); err != nil {
		t.Fatalf("duplicate ResumeJobsByGroup() error = %v", err)
	}
	var duplicateResumeXmin string
	if err := testDB.Pool.QueryRow(ctx, `SELECT xmin::text FROM jobs WHERE id = $1`, job.ID).Scan(&duplicateResumeXmin); err != nil {
		t.Fatalf("query duplicate group-resumed job version: %v", err)
	}
	if duplicateResumeXmin != resumedXmin {
		t.Fatalf("duplicate group resume changed xmin from %s to %s", resumedXmin, duplicateResumeXmin)
	}
}
