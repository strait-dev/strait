//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"
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
	if err := q.CreateJobGroup(ctx, group); err != nil {
		t.Fatalf("CreateJobGroup() error = %v", err)
	}
	if group.ID == "" {
		t.Fatal("CreateJobGroup() did not set ID")
	}
	if group.CreatedAt.IsZero() {
		t.Fatal("CreateJobGroup() did not set CreatedAt")
	}
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
	if err := q.CreateJobGroup(ctx, group); err != nil {
		t.Fatalf("CreateJobGroup() error = %v", err)
	}

	got, err := q.GetJobGroup(ctx, group.ID)
	if err != nil {
		t.Fatalf("GetJobGroup() error = %v", err)
	}
	if got.Name != "Get Group" {
		t.Fatalf("GetJobGroup() name = %q, want %q", got.Name, "Get Group")
	}

	// Not found.
	_, err = q.GetJobGroup(ctx, newID())
	if !errors.Is(err, store.ErrJobGroupNotFound) {
		t.Fatalf("GetJobGroup(notfound) error = %v, want ErrJobGroupNotFound", err)
	}
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
		if err := q.CreateJobGroup(ctx, group); err != nil {
			t.Fatalf("CreateJobGroup(%d) error = %v", i, err)
		}
	}

	groups, err := q.ListJobGroups(ctx, projectID, 100, nil)
	if err != nil {
		t.Fatalf("ListJobGroups() error = %v", err)
	}
	if len(groups) != 3 {
		t.Fatalf("ListJobGroups() len = %d, want 3", len(groups))
	}

	// Empty project.
	empty, err := q.ListJobGroups(ctx, "nonexistent", 100, nil)
	if err != nil {
		t.Fatalf("ListJobGroups(empty) error = %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("ListJobGroups(empty) len = %d, want 0", len(empty))
	}
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
	if err := q.CreateJobGroup(ctx, group); err != nil {
		t.Fatalf("CreateJobGroup() error = %v", err)
	}

	group.Name = "Updated"
	group.Slug = "updated"
	group.Description = "Updated description"
	if err := q.UpdateJobGroup(ctx, group); err != nil {
		t.Fatalf("UpdateJobGroup() error = %v", err)
	}

	got, err := q.GetJobGroup(ctx, group.ID)
	if err != nil {
		t.Fatalf("GetJobGroup() error = %v", err)
	}
	if got.Name != "Updated" {
		t.Fatalf("name = %q, want Updated", got.Name)
	}

	// Not found.
	notFound := &domain.JobGroup{ID: newID(), ProjectID: "x", Name: "x", Slug: "x"}
	if err := q.UpdateJobGroup(ctx, notFound); !errors.Is(err, store.ErrJobGroupNotFound) {
		t.Fatalf("UpdateJobGroup(notfound) error = %v, want ErrJobGroupNotFound", err)
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
	if err := q.CreateJobGroup(ctx, group); err != nil {
		t.Fatalf("CreateJobGroup() error = %v", err)
	}

	if err := q.DeleteJobGroup(ctx, group.ID); err != nil {
		t.Fatalf("DeleteJobGroup() error = %v", err)
	}

	_, err := q.GetJobGroup(ctx, group.ID)
	if !errors.Is(err, store.ErrJobGroupNotFound) {
		t.Fatalf("GetJobGroup(deleted) error = %v, want ErrJobGroupNotFound", err)
	}

	// Not found.
	if err := q.DeleteJobGroup(ctx, newID()); !errors.Is(err, store.ErrJobGroupNotFound) {
		t.Fatalf("DeleteJobGroup(notfound) error = %v, want ErrJobGroupNotFound", err)
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
	if err := q.CreateJobGroup(ctx, group); err != nil {
		t.Fatalf("CreateJobGroup() error = %v", err)
	}

	job := baseJob(newID(), projectID)
	job.GroupID = group.ID
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	jobs, err := q.ListJobsByGroup(ctx, group.ID, 100, nil)
	if err != nil {
		t.Fatalf("ListJobsByGroup() error = %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("ListJobsByGroup() len = %d, want 1", len(jobs))
	}

	// Empty group.
	empty, err := q.ListJobsByGroup(ctx, newID(), 100, nil)
	if err != nil {
		t.Fatalf("ListJobsByGroup(empty) error = %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("ListJobsByGroup(empty) len = %d, want 0", len(empty))
	}
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
	if err := q.CreateJobGroup(ctx, group); err != nil {
		t.Fatalf("CreateJobGroup() error = %v", err)
	}

	job := baseJob(newID(), projectID)
	job.GroupID = group.ID
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	if err := q.PauseJobsByGroup(ctx, group.ID); err != nil {
		t.Fatalf("PauseJobsByGroup() error = %v", err)
	}

	paused, err := q.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetJob() error = %v", err)
	}
	if !paused.Paused {
		t.Fatal("job should be paused after PauseJobsByGroup")
	}

	if err := q.ResumeJobsByGroup(ctx, group.ID); err != nil {
		t.Fatalf("ResumeJobsByGroup() error = %v", err)
	}

	resumed, err := q.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetJob() error = %v", err)
	}
	if resumed.Paused {
		t.Fatal("job should not be paused after ResumeJobsByGroup")
	}

	// Not found group.
	if err := q.PauseJobsByGroup(ctx, newID()); !errors.Is(err, store.ErrJobGroupNotFound) {
		t.Fatalf("PauseJobsByGroup(notfound) error = %v, want ErrJobGroupNotFound", err)
	}
	if err := q.ResumeJobsByGroup(ctx, newID()); !errors.Is(err, store.ErrJobGroupNotFound) {
		t.Fatalf("ResumeJobsByGroup(notfound) error = %v, want ErrJobGroupNotFound", err)
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
	if err := q.CreateJobGroup(ctx, group); err != nil {
		t.Fatalf("CreateJobGroup() error = %v", err)
	}

	job := baseJob(newID(), projectID)
	job.GroupID = group.ID
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	run := baseRun(job, newID())
	run.Status = domain.StatusCompleted
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	stats, err := q.GetJobGroupStats(ctx, group.ID)
	if err != nil {
		t.Fatalf("GetJobGroupStats() error = %v", err)
	}
	if stats.GroupID != group.ID {
		t.Fatalf("GroupID = %q, want %q", stats.GroupID, group.ID)
	}
	if stats.RunCounts["completed"] != 1 {
		t.Fatalf("RunCounts[completed] = %d, want 1", stats.RunCounts["completed"])
	}

	// Not found.
	_, err = q.GetJobGroupStats(ctx, newID())
	if !errors.Is(err, store.ErrJobGroupNotFound) {
		t.Fatalf("GetJobGroupStats(notfound) error = %v, want ErrJobGroupNotFound", err)
	}
}
