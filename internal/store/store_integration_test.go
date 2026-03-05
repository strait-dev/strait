//go:build integration

package store_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"strconv"
	"testing"
	"time"

	"orchestrator/internal/domain"
	"orchestrator/internal/store"
	"orchestrator/internal/testutil"

	"github.com/google/uuid"
)

var testDB *testutil.TestDB

func TestMain(m *testing.M) {
	ctx := context.Background()

	var err error
	testDB, err = testutil.SetupTestDB(ctx, "../../migrations")
	if err != nil {
		log.Fatalf("setup test db: %v", err)
	}

	code := m.Run()
	testDB.Cleanup(ctx)
	os.Exit(code)
}

func TestWithTx_CommitsOnSuccess(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	jobID := newID()
	err := store.WithTx(ctx, testDB.Pool, func(q *store.Queries) error {
		job := baseJob(jobID, "project-withtx-commit")
		return q.CreateJob(ctx, job)
	})
	if err != nil {
		t.Fatalf("WithTx() error = %v", err)
	}

	q := mustStore(t)
	job, err := q.GetJob(ctx, jobID)
	if err != nil {
		t.Fatalf("GetJob() error = %v", err)
	}
	if job.ID != jobID {
		t.Fatalf("job.ID = %q, want %q", job.ID, jobID)
	}
}

func TestWithTx_RollsBackOnError(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	jobID := newID()
	wantErr := errors.New("force rollback")
	err := store.WithTx(ctx, testDB.Pool, func(q *store.Queries) error {
		job := baseJob(jobID, "project-withtx-rollback")
		if createErr := q.CreateJob(ctx, job); createErr != nil {
			return createErr
		}
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("WithTx() error = %v, want %v", err, wantErr)
	}

	q := mustStore(t)
	_, err = q.GetJob(ctx, jobID)
	if !errors.Is(err, store.ErrJobNotFound) {
		t.Fatalf("GetJob() error = %v, want ErrJobNotFound", err)
	}
}

func TestCreateJob(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "project-create-job")
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	if job.ID == "" {
		t.Fatal("CreateJob() did not set ID")
	}
	if job.CreatedAt.IsZero() {
		t.Fatal("CreateJob() did not set CreatedAt")
	}
	if job.UpdatedAt.IsZero() {
		t.Fatal("CreateJob() did not set UpdatedAt")
	}

	got, err := q.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetJob() error = %v", err)
	}

	assertJobEqual(t, job, got)
}

func TestCreateJob_CustomID(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	customID := newID()
	job := baseJob(customID, "project-create-job-custom-id")

	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	if job.ID != customID {
		t.Fatalf("CreateJob() ID = %q, want %q", job.ID, customID)
	}
}

func TestGetJob(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "project-get-job")
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	got, err := q.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetJob() error = %v", err)
	}

	assertJobEqual(t, job, got)
}

func TestGetJob_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, err := q.GetJob(ctx, newID())
	if !errors.Is(err, store.ErrJobNotFound) {
		t.Fatalf("GetJob() error = %v, want ErrJobNotFound", err)
	}
}

func TestGetJobBySlug(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "project-get-job-by-slug")
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	got, err := q.GetJobBySlug(ctx, job.ProjectID, job.Slug)
	if err != nil {
		t.Fatalf("GetJobBySlug() error = %v", err)
	}

	assertJobEqual(t, job, got)
}

func TestGetJobBySlug_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "project-get-job-by-slug-not-found")
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	_, err := q.GetJobBySlug(ctx, job.ProjectID, "does-not-exist")
	if !errors.Is(err, store.ErrJobNotFound) {
		t.Fatalf("GetJobBySlug() error = %v, want ErrJobNotFound", err)
	}
}

func TestListJobs(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-list-jobs"
	for i := range 3 {
		job := baseJob(newID(), projectID)
		job.Name = "job-list-" + uuid.Must(uuid.NewV7()).String()
		job.Slug = "slug-list-" + uuid.Must(uuid.NewV7()).String()
		job.MaxAttempts = i + 1
		if err := q.CreateJob(ctx, job); err != nil {
			t.Fatalf("CreateJob() error = %v", err)
		}
	}

	jobs, err := q.ListJobs(ctx, projectID)
	if err != nil {
		t.Fatalf("ListJobs() error = %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("ListJobs() len = %d, want 3", len(jobs))
	}

	assertTimesDesc(t, extractJobCreatedAt(jobs))
}

func TestListJobs_FiltersByProject(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	targetProject := "project-list-jobs-filter-target"
	otherProject := "project-list-jobs-filter-other"

	for range 2 {
		job := baseJob(newID(), targetProject)
		if err := q.CreateJob(ctx, job); err != nil {
			t.Fatalf("CreateJob() target error = %v", err)
		}
	}

	other := baseJob(newID(), otherProject)
	if err := q.CreateJob(ctx, other); err != nil {
		t.Fatalf("CreateJob() other error = %v", err)
	}

	jobs, err := q.ListJobs(ctx, targetProject)
	if err != nil {
		t.Fatalf("ListJobs() error = %v", err)
	}

	if len(jobs) != 2 {
		t.Fatalf("ListJobs() len = %d, want 2", len(jobs))
	}

	for _, job := range jobs {
		if job.ProjectID != targetProject {
			t.Fatalf("ListJobs() project_id = %q, want %q", job.ProjectID, targetProject)
		}
	}
}

func TestUpdateJob(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "project-update-job")
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	prevUpdatedAt := job.UpdatedAt
	job.Name = "updated-name"
	job.Slug = "updated-slug"
	job.EndpointURL = "https://example.com/new-endpoint"

	if err := q.UpdateJob(ctx, job); err != nil {
		t.Fatalf("UpdateJob() error = %v", err)
	}

	if !job.UpdatedAt.After(prevUpdatedAt) {
		t.Fatalf("UpdateJob() UpdatedAt = %v, want after %v", job.UpdatedAt, prevUpdatedAt)
	}

	got, err := q.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetJob() error = %v", err)
	}

	if got.Name != "updated-name" || got.Slug != "updated-slug" || got.EndpointURL != "https://example.com/new-endpoint" {
		t.Fatalf("updated values not persisted: got name=%q slug=%q endpoint=%q", got.Name, got.Slug, got.EndpointURL)
	}
}

func TestDeleteJob(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "project-delete-job")
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	if err := q.DeleteJob(ctx, job.ID); err != nil {
		t.Fatalf("DeleteJob() error = %v", err)
	}

	_, err := q.GetJob(ctx, job.ID)
	if !errors.Is(err, store.ErrJobNotFound) {
		t.Fatalf("GetJob() after delete error = %v, want ErrJobNotFound", err)
	}
}

func TestListCronJobs(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	enabledCron := baseJob(newID(), "project-list-cron-jobs")
	enabledCron.Cron = "*/5 * * * *"
	enabledCron.Enabled = true
	if err := q.CreateJob(ctx, enabledCron); err != nil {
		t.Fatalf("CreateJob() enabled cron error = %v", err)
	}

	disabledCron := baseJob(newID(), "project-list-cron-jobs")
	disabledCron.Cron = "*/10 * * * *"
	disabledCron.Enabled = false
	if err := q.CreateJob(ctx, disabledCron); err != nil {
		t.Fatalf("CreateJob() disabled cron error = %v", err)
	}

	enabledNoCron := baseJob(newID(), "project-list-cron-jobs")
	enabledNoCron.Cron = ""
	enabledNoCron.Enabled = true
	if err := q.CreateJob(ctx, enabledNoCron); err != nil {
		t.Fatalf("CreateJob() enabled no cron error = %v", err)
	}

	jobs, err := q.ListCronJobs(ctx)
	if err != nil {
		t.Fatalf("ListCronJobs() error = %v", err)
	}

	if len(jobs) != 1 {
		t.Fatalf("ListCronJobs() len = %d, want 1", len(jobs))
	}
	if jobs[0].ID != enabledCron.ID {
		t.Fatalf("ListCronJobs() id = %q, want %q", jobs[0].ID, enabledCron.ID)
	}
}

func TestCreateRun(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-create-run")
	now := time.Now().UTC().Truncate(time.Microsecond)
	run := baseRun(job, newID())
	run.Payload = []byte(`{"x":1}`)
	run.Result = []byte(`{"ok":true}`)
	run.Error = "boom"
	scheduledAt := now.Add(-2 * time.Minute)
	startedAt := now.Add(-90 * time.Second)
	finishedAt := now.Add(-30 * time.Second)
	heartbeatAt := now.Add(-45 * time.Second)
	nextRetryAt := now.Add(2 * time.Minute)
	expiresAt := now.Add(10 * time.Minute)
	run.ScheduledAt = &scheduledAt
	run.StartedAt = &startedAt
	run.FinishedAt = &finishedAt
	run.HeartbeatAt = &heartbeatAt
	run.NextRetryAt = &nextRetryAt
	run.ExpiresAt = &expiresAt
	run.ParentRunID = ""
	run.Priority = 7
	run.IdempotencyKey = newID()

	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	if run.Attempt != 1 {
		t.Fatalf("CreateRun() Attempt = %d, want 1", run.Attempt)
	}
	if run.TriggeredBy != domain.TriggerManual {
		t.Fatalf("CreateRun() TriggeredBy = %q, want %q", run.TriggeredBy, domain.TriggerManual)
	}
	if run.CreatedAt.IsZero() {
		t.Fatal("CreateRun() did not set CreatedAt")
	}

	got, err := q.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}

	assertRunEqual(t, run, got)
}

func TestCreateRun_Delayed(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-create-run-delayed")
	run := baseRun(job, newID())
	scheduledAt := time.Now().Add(2 * time.Hour)
	run.ScheduledAt = &scheduledAt

	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	if run.Status != domain.StatusDelayed {
		t.Fatalf("CreateRun() Status = %q, want %q", run.Status, domain.StatusDelayed)
	}
}

func TestGetRun(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-get-run")
	run := baseRun(job, newID())
	run.Status = domain.StatusExecuting
	run.Attempt = 3
	run.Payload = []byte(`{"input":true}`)
	run.Result = []byte(`{"out":123}`)
	run.Error = "none"
	now := time.Now().UTC().Truncate(time.Microsecond)
	scheduledAt := now.Add(-time.Hour)
	startedAt := now.Add(-30 * time.Minute)
	finishedAt := now.Add(-10 * time.Minute)
	heartbeatAt := now.Add(-2 * time.Minute)
	nextRetryAt := now.Add(5 * time.Minute)
	expiresAt := now.Add(45 * time.Minute)
	run.ScheduledAt = &scheduledAt
	run.StartedAt = &startedAt
	run.FinishedAt = &finishedAt
	run.HeartbeatAt = &heartbeatAt
	run.NextRetryAt = &nextRetryAt
	run.ExpiresAt = &expiresAt
	run.Priority = 10
	run.IdempotencyKey = newID()

	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	got, err := q.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}

	assertRunEqual(t, run, got)
}

func TestGetRun_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, err := q.GetRun(ctx, newID())
	if !errors.Is(err, store.ErrRunNotFound) {
		t.Fatalf("GetRun() error = %v, want ErrRunNotFound", err)
	}
}

func TestGetRunByIdempotencyKey(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-get-run-idempotency")
	run := baseRun(job, newID())
	run.IdempotencyKey = newID()

	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	got, err := q.GetRunByIdempotencyKey(ctx, job.ID, run.IdempotencyKey)
	if err != nil {
		t.Fatalf("GetRunByIdempotencyKey() error = %v", err)
	}
	if got == nil {
		t.Fatal("GetRunByIdempotencyKey() returned nil run")
	}

	assertRunEqual(t, run, got)
}

func TestGetRunByIdempotencyKey_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-get-run-idempotency-not-found")

	got, err := q.GetRunByIdempotencyKey(ctx, job.ID, "missing-key")
	if err != nil {
		t.Fatalf("GetRunByIdempotencyKey() error = %v, want nil", err)
	}
	if got != nil {
		t.Fatalf("GetRunByIdempotencyKey() run = %+v, want nil", got)
	}
}

func TestListRunsByJob(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-list-runs-by-job")
	for i := range 5 {
		run := baseRun(job, newID())
		run.Priority = i
		if err := q.CreateRun(ctx, run); err != nil {
			t.Fatalf("CreateRun() error = %v", err)
		}
	}

	runs, err := q.ListRunsByJob(ctx, job.ID, 2, 1)
	if err != nil {
		t.Fatalf("ListRunsByJob() error = %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("ListRunsByJob() len = %d, want 2", len(runs))
	}

	assertTimesDesc(t, extractRunCreatedAt(runs))
}

func TestListRunsByProject(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-list-runs-by-project"
	job := mustCreateJob(t, ctx, q, projectID)
	other := mustCreateJob(t, ctx, q, "project-list-runs-by-project-other")

	queued1 := baseRun(job, newID())
	queued1.Status = domain.StatusQueued
	if err := q.CreateRun(ctx, queued1); err != nil {
		t.Fatalf("CreateRun() queued1 error = %v", err)
	}

	failed := baseRun(job, newID())
	failed.Status = domain.StatusFailed
	if err := q.CreateRun(ctx, failed); err != nil {
		t.Fatalf("CreateRun() failed error = %v", err)
	}

	queued2 := baseRun(job, newID())
	queued2.Status = domain.StatusQueued
	if err := q.CreateRun(ctx, queued2); err != nil {
		t.Fatalf("CreateRun() queued2 error = %v", err)
	}

	otherRun := baseRun(other, newID())
	if err := q.CreateRun(ctx, otherRun); err != nil {
		t.Fatalf("CreateRun() other project error = %v", err)
	}

	status := domain.StatusQueued
	filtered, err := q.ListRunsByProject(ctx, projectID, &status, 10, nil)
	if err != nil {
		t.Fatalf("ListRunsByProject() filtered error = %v", err)
	}
	if len(filtered) != 2 {
		t.Fatalf("ListRunsByProject() filtered len = %d, want 2", len(filtered))
	}
	for _, run := range filtered {
		if run.ProjectID != projectID || run.Status != domain.StatusQueued {
			t.Fatalf("filtered run mismatch: project=%q status=%q", run.ProjectID, run.Status)
		}
	}

	firstPage, err := q.ListRunsByProject(ctx, projectID, nil, 2, nil)
	if err != nil {
		t.Fatalf("ListRunsByProject() first page error = %v", err)
	}
	if len(firstPage) != 2 {
		t.Fatalf("ListRunsByProject() first page len = %d, want 2", len(firstPage))
	}
	assertTimesDesc(t, extractRunCreatedAt(firstPage))

	cursor := firstPage[len(firstPage)-1].CreatedAt
	secondPage, err := q.ListRunsByProject(ctx, projectID, nil, 2, &cursor)
	if err != nil {
		t.Fatalf("ListRunsByProject() second page error = %v", err)
	}
	if len(secondPage) != 1 {
		t.Fatalf("ListRunsByProject() second page len = %d, want 1", len(secondPage))
	}
}

func TestUpdateRunStatus(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-update-run-status")
	run := baseRun(job, newID())
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	startedAt := time.Now().UTC().Truncate(time.Microsecond)
	fields := map[string]any{
		"started_at": startedAt,
	}

	if err := q.UpdateRunStatus(ctx, run.ID, domain.StatusQueued, domain.StatusDequeued, fields); err != nil {
		t.Fatalf("UpdateRunStatus() error = %v", err)
	}

	got, err := q.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}

	if got.Status != domain.StatusDequeued {
		t.Fatalf("status = %q, want %q", got.Status, domain.StatusDequeued)
	}
	if got.StartedAt == nil || !got.StartedAt.Equal(startedAt) {
		t.Fatalf("started_at = %v, want %v", got.StartedAt, startedAt)
	}
}

func TestUpdateRunStatus_InvalidTransition(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-update-run-status-invalid")
	run := baseRun(job, newID())
	run.Status = domain.StatusCompleted
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	err := q.UpdateRunStatus(ctx, run.ID, domain.StatusCompleted, domain.StatusExecuting, nil)
	if err == nil {
		t.Fatal("UpdateRunStatus() expected error, got nil")
	}
}

func TestUpdateRunStatus_OptimisticLock(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-update-run-status-lock")
	run := baseRun(job, newID())
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	err := q.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusCompleted, nil)
	if !errors.Is(err, store.ErrRunConflict) {
		t.Fatalf("UpdateRunStatus() error = %v, want ErrRunConflict", err)
	}
}

func TestUpdateRunStatus_WithFields(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-update-run-status-fields")
	run := baseRun(job, newID())
	run.Status = domain.StatusExecuting
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	finishedAt := time.Now().UTC().Truncate(time.Microsecond)
	fields := map[string]any{
		"result":      []byte(`{"ok":true}`),
		"error":       "execution failed",
		"finished_at": finishedAt,
	}

	if err := q.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusCompleted, fields); err != nil {
		t.Fatalf("UpdateRunStatus() error = %v", err)
	}

	got, err := q.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}

	if got.Status != domain.StatusCompleted {
		t.Fatalf("status = %q, want %q", got.Status, domain.StatusCompleted)
	}
	if !jsonEqual(got.Result, []byte(`{"ok":true}`)) {
		t.Fatalf("result = %s, want %s", string(got.Result), `{"ok":true}`)
	}
	if got.Error != "execution failed" {
		t.Fatalf("error = %q, want %q", got.Error, "execution failed")
	}
	if got.FinishedAt == nil || !got.FinishedAt.Equal(finishedAt) {
		t.Fatalf("finished_at = %v, want %v", got.FinishedAt, finishedAt)
	}
}

func TestUpdateHeartbeat(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-update-heartbeat")
	run := baseRun(job, newID())
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	if err := q.UpdateHeartbeat(ctx, run.ID); err != nil {
		t.Fatalf("UpdateHeartbeat() error = %v", err)
	}

	got, err := q.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}

	if got.HeartbeatAt == nil || got.HeartbeatAt.IsZero() {
		t.Fatalf("heartbeat_at = %v, want non-nil", got.HeartbeatAt)
	}
}

func TestUpdateHeartbeat_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	err := q.UpdateHeartbeat(ctx, newID())
	if !errors.Is(err, store.ErrRunNotFound) {
		t.Fatalf("UpdateHeartbeat() error = %v, want ErrRunNotFound", err)
	}
}

func TestListChildRuns(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-list-child-runs")
	parent := baseRun(job, newID())
	if err := q.CreateRun(ctx, parent); err != nil {
		t.Fatalf("CreateRun() parent error = %v", err)
	}

	child1 := baseRun(job, newID())
	child1.ParentRunID = parent.ID
	if err := q.CreateRun(ctx, child1); err != nil {
		t.Fatalf("CreateRun() child1 error = %v", err)
	}

	child2 := baseRun(job, newID())
	child2.ParentRunID = parent.ID
	if err := q.CreateRun(ctx, child2); err != nil {
		t.Fatalf("CreateRun() child2 error = %v", err)
	}

	children, err := q.ListChildRuns(ctx, parent.ID)
	if err != nil {
		t.Fatalf("ListChildRuns() error = %v", err)
	}
	if len(children) != 2 {
		t.Fatalf("ListChildRuns() len = %d, want 2", len(children))
	}

	for _, child := range children {
		if child.ParentRunID != parent.ID {
			t.Fatalf("child parent_run_id = %q, want %q", child.ParentRunID, parent.ID)
		}
	}

	assertTimesAsc(t, extractRunCreatedAt(children))
}

func TestDeleteTerminalRunsPastRetention(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-delete-retention")
	now := time.Now().UTC().Truncate(time.Microsecond)

	oldCompleted := baseRun(job, newID())
	oldCompleted.Status = domain.StatusCompleted
	finishedOldCompleted := now.Add(-31 * 24 * time.Hour)
	oldCompleted.FinishedAt = &finishedOldCompleted
	if err := q.CreateRun(ctx, oldCompleted); err != nil {
		t.Fatalf("CreateRun() oldCompleted error = %v", err)
	}

	oldTimedOut := baseRun(job, newID())
	oldTimedOut.Status = domain.StatusTimedOut
	finishedOldTimedOut := now.Add(-91 * 24 * time.Hour)
	oldTimedOut.FinishedAt = &finishedOldTimedOut
	if err := q.CreateRun(ctx, oldTimedOut); err != nil {
		t.Fatalf("CreateRun() oldTimedOut error = %v", err)
	}

	recentCompleted := baseRun(job, newID())
	recentCompleted.Status = domain.StatusCompleted
	finishedRecent := now.Add(-5 * 24 * time.Hour)
	recentCompleted.FinishedAt = &finishedRecent
	if err := q.CreateRun(ctx, recentCompleted); err != nil {
		t.Fatalf("CreateRun() recentCompleted error = %v", err)
	}

	queued := baseRun(job, newID())
	queued.Status = domain.StatusQueued
	if err := q.CreateRun(ctx, queued); err != nil {
		t.Fatalf("CreateRun() queued error = %v", err)
	}

	deleted, err := q.DeleteTerminalRunsPastRetention(ctx, 30*24*time.Hour, 90*24*time.Hour)
	if err != nil {
		t.Fatalf("DeleteTerminalRunsPastRetention() error = %v", err)
	}
	if deleted != 2 {
		t.Fatalf("deleted = %d, want 2", deleted)
	}

	if _, err := q.GetRun(ctx, oldCompleted.ID); !errors.Is(err, store.ErrRunNotFound) {
		t.Fatalf("GetRun(oldCompleted) error = %v, want ErrRunNotFound", err)
	}
	if _, err := q.GetRun(ctx, oldTimedOut.ID); !errors.Is(err, store.ErrRunNotFound) {
		t.Fatalf("GetRun(oldTimedOut) error = %v, want ErrRunNotFound", err)
	}
	if _, err := q.GetRun(ctx, recentCompleted.ID); err != nil {
		t.Fatalf("GetRun(recentCompleted) error = %v", err)
	}
	if _, err := q.GetRun(ctx, queued.ID); err != nil {
		t.Fatalf("GetRun(queued) error = %v", err)
	}
}

func TestInsertEvent(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-insert-event")
	run := mustCreateRun(t, ctx, q, job)

	event := &domain.RunEvent{
		ID:      newID(),
		RunID:   run.ID,
		Type:    domain.EventStateChange,
		Level:   "info",
		Message: "state changed",
		Data:    []byte(`{"from":"queued","to":"dequeued"}`),
	}

	if err := q.InsertEvent(ctx, event); err != nil {
		t.Fatalf("InsertEvent() error = %v", err)
	}
	if event.CreatedAt.IsZero() {
		t.Fatal("InsertEvent() did not set CreatedAt")
	}

	events, err := q.ListEvents(ctx, run.ID)
	if err != nil {
		t.Fatalf("ListEvents() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("ListEvents() len = %d, want 1", len(events))
	}

	got := events[0]
	if got.ID != event.ID || got.RunID != event.RunID || got.Type != event.Type || got.Level != event.Level || got.Message != event.Message {
		t.Fatalf("event mismatch: got %+v want %+v", got, *event)
	}
	if !jsonEqual(got.Data, event.Data) {
		t.Fatalf("event data = %s, want %s", string(got.Data), string(event.Data))
	}
}

func TestListEvents(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-list-events")
	run := mustCreateRun(t, ctx, q, job)

	for i := range 3 {
		event := &domain.RunEvent{
			ID:      newID(),
			RunID:   run.ID,
			Type:    domain.EventLog,
			Level:   "info",
			Message: "event",
			Data:    []byte(`{"index":` + strconv.Itoa(i) + `}`),
		}
		if err := q.InsertEvent(ctx, event); err != nil {
			t.Fatalf("InsertEvent() error = %v", err)
		}
	}

	events, err := q.ListEvents(ctx, run.ID)
	if err != nil {
		t.Fatalf("ListEvents() error = %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("ListEvents() len = %d, want 3", len(events))
	}

	for _, event := range events {
		if event.RunID != run.ID {
			t.Fatalf("event run_id = %q, want %q", event.RunID, run.ID)
		}
	}

	assertEventTimesAsc(t, events)
}

func TestQueueStats(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-queue-stats")

	queued := baseRun(job, newID())
	queued.Status = domain.StatusQueued
	if err := q.CreateRun(ctx, queued); err != nil {
		t.Fatalf("CreateRun() queued error = %v", err)
	}

	executing := baseRun(job, newID())
	executing.Status = domain.StatusExecuting
	if err := q.CreateRun(ctx, executing); err != nil {
		t.Fatalf("CreateRun() executing error = %v", err)
	}

	delayed := baseRun(job, newID())
	delayed.Status = domain.StatusDelayed
	if err := q.CreateRun(ctx, delayed); err != nil {
		t.Fatalf("CreateRun() delayed error = %v", err)
	}

	failed := baseRun(job, newID())
	failed.Status = domain.StatusFailed
	if err := q.CreateRun(ctx, failed); err != nil {
		t.Fatalf("CreateRun() failed error = %v", err)
	}

	stats, err := q.QueueStats(ctx)
	if err != nil {
		t.Fatalf("QueueStats() error = %v", err)
	}

	if stats.Queued != 1 || stats.Executing != 1 || stats.Delayed != 1 {
		t.Fatalf("QueueStats() = %+v, want queued=1 executing=1 delayed=1", *stats)
	}
}

func mustStore(t *testing.T) *store.Queries {
	t.Helper()

	if testDB == nil || testDB.Pool == nil {
		t.Fatal("testDB is not initialized")
	}

	return store.New(testDB.Pool)
}

func mustClean(t *testing.T, ctx context.Context) {
	t.Helper()

	if err := testDB.CleanTables(ctx); err != nil {
		t.Fatalf("CleanTables() error = %v", err)
	}
}

func mustCreateJob(t *testing.T, ctx context.Context, q *store.Queries, projectID string) *domain.Job {
	t.Helper()

	job := baseJob(newID(), projectID)
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	return job
}

func mustCreateRun(t *testing.T, ctx context.Context, q *store.Queries, job *domain.Job) *domain.JobRun {
	t.Helper()

	run := baseRun(job, newID())
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	return run
}

func baseJob(id, projectID string) *domain.Job {
	return &domain.Job{
		ID:            id,
		ProjectID:     projectID,
		Name:          "job-" + id,
		Slug:          "slug-" + id,
		Description:   "job description",
		Cron:          "*/5 * * * *",
		PayloadSchema: []byte(`{"type":"object"}`),
		EndpointURL:   "https://example.com/webhook",
		MaxAttempts:   5,
		TimeoutSecs:   120,
		Enabled:       true,
		WebhookURL:    "https://example.com/job-callback",
		WebhookSecret: "secret-value",
	}
}

func baseRun(job *domain.Job, id string) *domain.JobRun {
	return &domain.JobRun{
		ID:          id,
		JobID:       job.ID,
		ProjectID:   job.ProjectID,
		Status:      domain.StatusQueued,
		Attempt:     1,
		Payload:     []byte(`{"hello":"world"}`),
		TriggeredBy: domain.TriggerManual,
		Priority:    0,
	}
}

func assertJobEqual(t *testing.T, want, got *domain.Job) {
	t.Helper()

	if got == nil {
		t.Fatal("job is nil")
	}

	if got.ID != want.ID ||
		got.ProjectID != want.ProjectID ||
		got.Name != want.Name ||
		got.Slug != want.Slug ||
		got.Description != want.Description ||
		got.Cron != want.Cron ||
		got.EndpointURL != want.EndpointURL ||
		got.MaxAttempts != want.MaxAttempts ||
		got.TimeoutSecs != want.TimeoutSecs ||
		got.Enabled != want.Enabled ||
		got.WebhookURL != want.WebhookURL ||
		got.WebhookSecret != want.WebhookSecret {
		t.Fatalf("job mismatch: got %+v want %+v", *got, *want)
	}

	if !jsonEqual(got.PayloadSchema, want.PayloadSchema) {
		t.Fatalf("payload_schema mismatch: got %s want %s", string(got.PayloadSchema), string(want.PayloadSchema))
	}

	if !got.CreatedAt.Equal(want.CreatedAt) {
		t.Fatalf("created_at mismatch: got %v want %v", got.CreatedAt, want.CreatedAt)
	}
	if !got.UpdatedAt.Equal(want.UpdatedAt) {
		t.Fatalf("updated_at mismatch: got %v want %v", got.UpdatedAt, want.UpdatedAt)
	}
}

func assertRunEqual(t *testing.T, want, got *domain.JobRun) {
	t.Helper()

	if got == nil {
		t.Fatal("run is nil")
	}

	if got.ID != want.ID ||
		got.JobID != want.JobID ||
		got.ProjectID != want.ProjectID ||
		got.Status != want.Status ||
		got.Attempt != want.Attempt ||
		got.Error != want.Error ||
		got.TriggeredBy != want.TriggeredBy ||
		got.ParentRunID != want.ParentRunID ||
		got.Priority != want.Priority ||
		got.IdempotencyKey != want.IdempotencyKey {
		t.Fatalf("run mismatch: got %+v want %+v", *got, *want)
	}

	if !jsonEqual(got.Payload, want.Payload) {
		t.Fatalf("payload mismatch: got %s want %s", string(got.Payload), string(want.Payload))
	}
	if !jsonEqual(got.Result, want.Result) {
		t.Fatalf("result mismatch: got %s want %s", string(got.Result), string(want.Result))
	}

	assertTimePtrEqual(t, "scheduled_at", want.ScheduledAt, got.ScheduledAt)
	assertTimePtrEqual(t, "started_at", want.StartedAt, got.StartedAt)
	assertTimePtrEqual(t, "finished_at", want.FinishedAt, got.FinishedAt)
	assertTimePtrEqual(t, "heartbeat_at", want.HeartbeatAt, got.HeartbeatAt)
	assertTimePtrEqual(t, "next_retry_at", want.NextRetryAt, got.NextRetryAt)
	assertTimePtrEqual(t, "expires_at", want.ExpiresAt, got.ExpiresAt)

	if !got.CreatedAt.Equal(want.CreatedAt) {
		t.Fatalf("created_at mismatch: got %v want %v", got.CreatedAt, want.CreatedAt)
	}
}

func assertTimePtrEqual(t *testing.T, field string, want, got *time.Time) {
	t.Helper()

	if want == nil && got == nil {
		return
	}
	if want == nil || got == nil {
		t.Fatalf("%s mismatch: got %v want %v", field, got, want)
	}
	if !want.Equal(*got) {
		t.Fatalf("%s mismatch: got %v want %v", field, got, want)
	}
}

func extractJobCreatedAt(jobs []domain.Job) []time.Time {
	times := make([]time.Time, 0, len(jobs))
	for _, job := range jobs {
		times = append(times, job.CreatedAt)
	}

	return times
}

func extractRunCreatedAt(runs []domain.JobRun) []time.Time {
	times := make([]time.Time, 0, len(runs))
	for _, run := range runs {
		times = append(times, run.CreatedAt)
	}

	return times
}

func assertTimesDesc(t *testing.T, times []time.Time) {
	t.Helper()

	for i := 1; i < len(times); i++ {
		if times[i-1].Before(times[i]) {
			t.Fatalf("times not DESC at index %d: %v before %v", i, times[i-1], times[i])
		}
	}
}

func assertTimesAsc(t *testing.T, times []time.Time) {
	t.Helper()

	for i := 1; i < len(times); i++ {
		if times[i].Before(times[i-1]) {
			t.Fatalf("times not ASC at index %d: %v before %v", i, times[i], times[i-1])
		}
	}
}

func assertEventTimesAsc(t *testing.T, events []domain.RunEvent) {
	t.Helper()

	for i := 1; i < len(events); i++ {
		if events[i].CreatedAt.Before(events[i-1].CreatedAt) {
			t.Fatalf("events not ASC at index %d: %v before %v", i, events[i].CreatedAt, events[i-1].CreatedAt)
		}
	}
}

func newID() string {
	return uuid.Must(uuid.NewV7()).String()
}

func jsonEqual(a, b []byte) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	var va, vb any
	if err := json.Unmarshal(a, &va); err != nil {
		return bytes.Equal(a, b)
	}
	if err := json.Unmarshal(b, &vb); err != nil {
		return bytes.Equal(a, b)
	}
	ra, _ := json.Marshal(va)
	rb, _ := json.Marshal(vb)
	return bytes.Equal(ra, rb)
}
