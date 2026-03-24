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
	for i := 0; i < 10; i++ {
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
