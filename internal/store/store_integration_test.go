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

	"strait/internal/domain"
	"strait/internal/store"
	"strait/internal/testutil"

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

func TestEndpointCircuitState_OpensAndBlocksDispatch(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	endpoint := "https://example.com/circuit-open"
	now := time.Now().UTC()

	if err := q.RecordEndpointCircuitFailure(ctx, endpoint, now, 2, 2*time.Minute); err != nil {
		t.Fatalf("RecordEndpointCircuitFailure() first error = %v", err)
	}

	allowed, retryAt, err := q.CanDispatchEndpoint(ctx, endpoint, now)
	if err != nil {
		t.Fatalf("CanDispatchEndpoint() after first failure error = %v", err)
	}
	if !allowed {
		t.Fatal("CanDispatchEndpoint() after first failure = false, want true")
	}
	if retryAt != nil {
		t.Fatalf("retryAt after first failure = %v, want nil", retryAt)
	}

	if err := q.RecordEndpointCircuitFailure(ctx, endpoint, now.Add(time.Second), 2, 2*time.Minute); err != nil {
		t.Fatalf("RecordEndpointCircuitFailure() second error = %v", err)
	}

	allowed, retryAt, err = q.CanDispatchEndpoint(ctx, endpoint, now.Add(2*time.Second))
	if err != nil {
		t.Fatalf("CanDispatchEndpoint() open error = %v", err)
	}
	if allowed {
		t.Fatal("CanDispatchEndpoint() open = true, want false")
	}
	if retryAt == nil {
		t.Fatal("retryAt = nil, want non-nil while open")
	}

	state, err := q.GetEndpointCircuitState(ctx, endpoint)
	if err != nil {
		t.Fatalf("GetEndpointCircuitState() error = %v", err)
	}
	if state == nil {
		t.Fatal("GetEndpointCircuitState() = nil, want state")
	}
	if state.State != domain.CircuitStateOpen {
		t.Fatalf("state = %s, want %s", state.State, domain.CircuitStateOpen)
	}
}

func TestEndpointCircuitState_ClosesAfterSuccess(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	endpoint := "https://example.com/circuit-close"
	now := time.Now().UTC()

	if err := q.RecordEndpointCircuitFailure(ctx, endpoint, now, 1, time.Minute); err != nil {
		t.Fatalf("RecordEndpointCircuitFailure() error = %v", err)
	}

	if err := q.RecordEndpointCircuitSuccess(ctx, endpoint); err != nil {
		t.Fatalf("RecordEndpointCircuitSuccess() error = %v", err)
	}

	state, err := q.GetEndpointCircuitState(ctx, endpoint)
	if err != nil {
		t.Fatalf("GetEndpointCircuitState() error = %v", err)
	}
	if state == nil {
		t.Fatal("GetEndpointCircuitState() = nil, want state")
	}
	if state.State != domain.CircuitStateClosed {
		t.Fatalf("state = %s, want %s", state.State, domain.CircuitStateClosed)
	}
	if state.ConsecutiveFailures != 0 {
		t.Fatalf("consecutive_failures = %d, want 0", state.ConsecutiveFailures)
	}

	allowed, retryAt, err := q.CanDispatchEndpoint(ctx, endpoint, now.Add(time.Second))
	if err != nil {
		t.Fatalf("CanDispatchEndpoint() error = %v", err)
	}
	if !allowed {
		t.Fatal("CanDispatchEndpoint() = false, want true")
	}
	if retryAt != nil {
		t.Fatalf("retryAt = %v, want nil", retryAt)
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

	jobs, err := q.ListJobs(ctx, projectID, 10000, nil)
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

	jobs, err := q.ListJobs(ctx, targetProject, 10000, nil)
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

func TestListJobsByTag(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-list-jobs-by-tag"
	jobA := baseJob(newID(), projectID)
	jobA.Tags = map[string]string{"team": "core", "service": "scheduler"}
	if err := q.CreateJob(ctx, jobA); err != nil {
		t.Fatalf("CreateJob(jobA) error = %v", err)
	}

	jobB := baseJob(newID(), projectID)
	jobB.Tags = map[string]string{"team": "platform"}
	if err := q.CreateJob(ctx, jobB); err != nil {
		t.Fatalf("CreateJob(jobB) error = %v", err)
	}

	jobC := baseJob(newID(), projectID)
	if err := q.CreateJob(ctx, jobC); err != nil {
		t.Fatalf("CreateJob(jobC) error = %v", err)
	}

	jobs, err := q.ListJobsByTag(ctx, projectID, "team", "core", 10000, nil)
	if err != nil {
		t.Fatalf("ListJobsByTag() error = %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("ListJobsByTag() len = %d, want 1", len(jobs))
	}
	if jobs[0].ID != jobA.ID {
		t.Fatalf("ListJobsByTag() id = %s, want %s", jobs[0].ID, jobA.ID)
	}
	if jobs[0].Tags["service"] != "scheduler" {
		t.Fatalf("ListJobsByTag() service tag = %q, want %q", jobs[0].Tags["service"], "scheduler")
	}

	jobs, err = q.ListJobsByTag(ctx, projectID, "team", "", 10000, nil)
	if err != nil {
		t.Fatalf("ListJobsByTag(key-only) error = %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("ListJobsByTag(key-only) len = %d, want 2", len(jobs))
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

func TestGetRunByIdempotencyKey_AllowsTerminalReplayAndReturnsLatest(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-idempotency-replay")
	key := newID()

	first := baseRun(job, newID())
	first.IdempotencyKey = key
	if err := q.CreateRun(ctx, first); err != nil {
		t.Fatalf("CreateRun(first) error = %v", err)
	}

	if err := q.UpdateRunStatus(ctx, first.ID, domain.StatusQueued, domain.StatusDequeued, map[string]any{
		"started_at": time.Now().UTC(),
	}); err != nil {
		t.Fatalf("UpdateRunStatus(first->dequeued) error = %v", err)
	}

	if err := q.UpdateRunStatus(ctx, first.ID, domain.StatusDequeued, domain.StatusExecuting, map[string]any{
		"started_at": time.Now().UTC(),
	}); err != nil {
		t.Fatalf("UpdateRunStatus(first->executing) error = %v", err)
	}

	if err := q.UpdateRunStatus(ctx, first.ID, domain.StatusExecuting, domain.StatusFailed, map[string]any{
		"finished_at": time.Now().UTC(),
		"error":       "exhausted",
	}); err != nil {
		t.Fatalf("UpdateRunStatus(first->failed) error = %v", err)
	}

	second := baseRun(job, newID())
	second.IdempotencyKey = key
	if err := q.CreateRun(ctx, second); err != nil {
		t.Fatalf("CreateRun(second) error = %v", err)
	}

	got, err := q.GetRunByIdempotencyKey(ctx, job.ID, key)
	if err != nil {
		t.Fatalf("GetRunByIdempotencyKey() error = %v", err)
	}
	if got == nil {
		t.Fatal("GetRunByIdempotencyKey() returned nil run")
	}
	if got.ID != second.ID {
		t.Fatalf("GetRunByIdempotencyKey() id = %s, want %s", got.ID, second.ID)
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
	filtered, err := q.ListRunsByProject(ctx, projectID, &status, nil, nil, 10, nil)
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

	firstPage, err := q.ListRunsByProject(ctx, projectID, nil, nil, nil, 2, nil)
	if err != nil {
		t.Fatalf("ListRunsByProject() first page error = %v", err)
	}
	if len(firstPage) != 2 {
		t.Fatalf("ListRunsByProject() first page len = %d, want 2", len(firstPage))
	}
	assertTimesDesc(t, extractRunCreatedAt(firstPage))

	cursor := firstPage[len(firstPage)-1].CreatedAt
	secondPage, err := q.ListRunsByProject(ctx, projectID, nil, nil, nil, 2, &cursor)
	if err != nil {
		t.Fatalf("ListRunsByProject() second page error = %v", err)
	}
	if len(secondPage) != 1 {
		t.Fatalf("ListRunsByProject() second page len = %d, want 1", len(secondPage))
	}
}

func TestListRunsByProject_MetadataFilter(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-list-runs-by-project-metadata"
	job := mustCreateJob(t, ctx, q, projectID)

	runProd := baseRun(job, newID())
	if err := q.CreateRun(ctx, runProd); err != nil {
		t.Fatalf("CreateRun() runProd error = %v", err)
	}
	if err := q.UpdateRunMetadata(ctx, runProd.ID, map[string]string{"env": "prod", "region": "eu"}); err != nil {
		t.Fatalf("UpdateRunMetadata() runProd error = %v", err)
	}

	runStage := baseRun(job, newID())
	if err := q.CreateRun(ctx, runStage); err != nil {
		t.Fatalf("CreateRun() runStage error = %v", err)
	}
	if err := q.UpdateRunMetadata(ctx, runStage.ID, map[string]string{"env": "stage"}); err != nil {
		t.Fatalf("UpdateRunMetadata() runStage error = %v", err)
	}

	runNoMetadata := baseRun(job, newID())
	if err := q.CreateRun(ctx, runNoMetadata); err != nil {
		t.Fatalf("CreateRun() runNoMetadata error = %v", err)
	}

	key := "env"
	value := "prod"
	filtered, err := q.ListRunsByProject(ctx, projectID, nil, &key, &value, 20, nil)
	if err != nil {
		t.Fatalf("ListRunsByProject() metadata key/value error = %v", err)
	}
	if len(filtered) != 1 {
		t.Fatalf("ListRunsByProject() metadata key/value len = %d, want 1", len(filtered))
	}
	if filtered[0].ID != runProd.ID {
		t.Fatalf("ListRunsByProject() metadata key/value id = %s, want %s", filtered[0].ID, runProd.ID)
	}

	keyOnly, err := q.ListRunsByProject(ctx, projectID, nil, &key, nil, 20, nil)
	if err != nil {
		t.Fatalf("ListRunsByProject() metadata key error = %v", err)
	}
	if len(keyOnly) != 2 {
		t.Fatalf("ListRunsByProject() metadata key len = %d, want 2", len(keyOnly))
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

	children, err := q.ListChildRuns(ctx, parent.ID, 10000, nil)
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

	events, err := q.ListEvents(ctx, run.ID, 10000, nil)
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

	events, err := q.ListEvents(ctx, run.ID, 10000, nil)
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

func TestRunCheckpoints(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-run-checkpoints")
	run := mustCreateRun(t, ctx, q, job)

	cp1 := &domain.RunCheckpoint{RunID: run.ID, Source: "sdk", State: json.RawMessage(`{"step":1}`)}
	if err := q.CreateRunCheckpoint(ctx, cp1); err != nil {
		t.Fatalf("CreateRunCheckpoint() error = %v", err)
	}
	cp2 := &domain.RunCheckpoint{RunID: run.ID, Source: "auto", State: json.RawMessage(`{"step":2}`)}
	if err := q.CreateRunCheckpoint(ctx, cp2); err != nil {
		t.Fatalf("CreateRunCheckpoint() error = %v", err)
	}

	checkpoints, err := q.ListRunCheckpoints(ctx, run.ID, 10, nil)
	if err != nil {
		t.Fatalf("ListRunCheckpoints() error = %v", err)
	}
	if len(checkpoints) != 2 {
		t.Fatalf("ListRunCheckpoints() len = %d, want 2", len(checkpoints))
	}
	if checkpoints[0].Sequence <= checkpoints[1].Sequence {
		t.Fatalf("expected descending sequence order, got %d then %d", checkpoints[0].Sequence, checkpoints[1].Sequence)
	}
}

func TestRunUsagePricingAndToolCallsAndOutputs(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-run-usage")
	run := mustCreateRun(t, ctx, q, job)

	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO pricing_catalog (id, provider, model, input_cost_microusd, output_cost_microusd, active, effective_from)
		VALUES ($1, $2, $3, $4, $5, TRUE, NOW())
	`, newID(), "openai", "gpt-4", int64(3), int64(7)); err != nil {
		t.Fatalf("insert pricing error = %v", err)
	}

	usage := &domain.RunUsage{
		RunID:            run.ID,
		Provider:         "openai",
		Model:            "gpt-4",
		PromptTokens:     10,
		CompletionTokens: 5,
	}
	if err := q.CreateRunUsage(ctx, usage); err != nil {
		t.Fatalf("CreateRunUsage() error = %v", err)
	}
	if usage.CostMicrousd != int64(65) {
		t.Fatalf("CreateRunUsage() cost = %d, want 65", usage.CostMicrousd)
	}

	usages, err := q.ListRunUsage(ctx, run.ID, 10, nil)
	if err != nil {
		t.Fatalf("ListRunUsage() error = %v", err)
	}
	if len(usages) != 1 {
		t.Fatalf("ListRunUsage() len = %d, want 1", len(usages))
	}

	call := &domain.RunToolCall{RunID: run.ID, ToolName: "search", Input: json.RawMessage(`{"q":"x"}`), Output: json.RawMessage(`{"ok":true}`), DurationMs: 120}
	if err := q.CreateRunToolCall(ctx, call); err != nil {
		t.Fatalf("CreateRunToolCall() error = %v", err)
	}
	calls, err := q.ListRunToolCalls(ctx, run.ID, 10, nil)
	if err != nil {
		t.Fatalf("ListRunToolCalls() error = %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("ListRunToolCalls() len = %d, want 1", len(calls))
	}

	out := &domain.RunOutput{RunID: run.ID, OutputKey: "final", Schema: json.RawMessage(`{"type":"object"}`), Value: json.RawMessage(`{"name":"leo"}`)}
	if err := q.UpsertRunOutput(ctx, out); err != nil {
		t.Fatalf("UpsertRunOutput() error = %v", err)
	}
	out.Value = json.RawMessage(`{"name":"leo2"}`)
	if err := q.UpsertRunOutput(ctx, out); err != nil {
		t.Fatalf("UpsertRunOutput() second error = %v", err)
	}
	outputs, err := q.ListRunOutputs(ctx, run.ID, 10000, nil)
	if err != nil {
		t.Fatalf("ListRunOutputs() error = %v", err)
	}
	if len(outputs) != 1 {
		t.Fatalf("ListRunOutputs() len = %d, want 1", len(outputs))
	}
	if !jsonEqual(outputs[0].Value, json.RawMessage(`{"name":"leo2"}`)) {
		t.Fatalf("ListRunOutputs() value = %s, want updated value", string(outputs[0].Value))
	}
}

func TestUpdateRunMetadata(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-run-metadata")
	run := mustCreateRun(t, ctx, q, job)

	if err := q.UpdateRunMetadata(ctx, run.ID, map[string]string{"env": "prod", "team": "core"}); err != nil {
		t.Fatalf("UpdateRunMetadata() first error = %v", err)
	}

	stored, err := q.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() after first metadata update error = %v", err)
	}
	if stored.Metadata["env"] != "prod" || stored.Metadata["team"] != "core" {
		t.Fatalf("metadata after first update = %+v", stored.Metadata)
	}

	if err := q.UpdateRunMetadata(ctx, run.ID, map[string]string{"team": "infra", "region": "eu"}); err != nil {
		t.Fatalf("UpdateRunMetadata() second error = %v", err)
	}

	stored, err = q.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() after second metadata update error = %v", err)
	}
	if stored.Metadata["env"] != "prod" || stored.Metadata["team"] != "infra" || stored.Metadata["region"] != "eu" {
		t.Fatalf("metadata after second update = %+v", stored.Metadata)
	}
}

func TestAreAllDescendantsTerminal(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-descendants-terminal")
	parent := mustCreateRun(t, ctx, q, job)
	child := baseRun(job, newID())
	child.ParentRunID = parent.ID
	child.Status = domain.StatusExecuting
	if err := q.CreateRun(ctx, child); err != nil {
		t.Fatalf("CreateRun(child) error = %v", err)
	}

	allTerminal, err := q.AreAllDescendantsTerminal(ctx, parent.ID)
	if err != nil {
		t.Fatalf("AreAllDescendantsTerminal() error = %v", err)
	}
	if allTerminal {
		t.Fatal("AreAllDescendantsTerminal() = true, want false")
	}

	if err := q.UpdateRunStatus(ctx, child.ID, domain.StatusExecuting, domain.StatusCompleted, map[string]any{"finished_at": time.Now()}); err != nil {
		t.Fatalf("UpdateRunStatus() error = %v", err)
	}
	allTerminal, err = q.AreAllDescendantsTerminal(ctx, parent.ID)
	if err != nil {
		t.Fatalf("AreAllDescendantsTerminal() error = %v", err)
	}
	if !allTerminal {
		t.Fatal("AreAllDescendantsTerminal() = false, want true")
	}
}

func TestSecret_JobSecretCRUD(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	q.SetSecretEncryptionKey("test-encryption-key-32bytes!!!!")
	mustClean(t, ctx)

	projectID := "project-secret-crud"
	job := mustCreateJob(t, ctx, q, projectID)

	globalSecret := &domain.JobSecret{
		ProjectID:      projectID,
		Environment:    "prod",
		SecretKey:      "GLOBAL_TOKEN",
		EncryptedValue: "global-value",
	}
	if err := q.CreateJobSecret(ctx, globalSecret); err != nil {
		t.Fatalf("CreateJobSecret(global) error = %v", err)
	}
	if globalSecret.ID == "" {
		t.Fatal("CreateJobSecret(global) did not set ID")
	}
	if globalSecret.KeyVersion != 1 {
		t.Fatalf("CreateJobSecret(global) key version = %d, want 1", globalSecret.KeyVersion)
	}
	if globalSecret.CreatedAt.IsZero() || globalSecret.UpdatedAt.IsZero() {
		t.Fatal("CreateJobSecret(global) did not set timestamps")
	}

	jobSecret := &domain.JobSecret{
		ProjectID:      projectID,
		JobID:          job.ID,
		Environment:    "prod",
		SecretKey:      "API_TOKEN",
		EncryptedValue: "job-value",
	}
	if err := q.CreateJobSecret(ctx, jobSecret); err != nil {
		t.Fatalf("CreateJobSecret(job) error = %v", err)
	}

	dupSecret := &domain.JobSecret{
		ProjectID:      projectID,
		JobID:          job.ID,
		Environment:    "prod",
		SecretKey:      "API_TOKEN",
		EncryptedValue: "duplicate",
	}
	if err := q.CreateJobSecret(ctx, dupSecret); err == nil {
		t.Fatal("CreateJobSecret(duplicate) error = nil, want error")
	}

	gotJobSecret, err := q.GetJobSecret(ctx, jobSecret.ID)
	if err != nil {
		t.Fatalf("GetJobSecret() error = %v", err)
	}
	if gotJobSecret.ID != jobSecret.ID || gotJobSecret.ProjectID != projectID || gotJobSecret.JobID != job.ID || gotJobSecret.SecretKey != "API_TOKEN" {
		t.Fatalf("GetJobSecret() mismatch: got %+v", *gotJobSecret)
	}
	if gotJobSecret.EncryptedValue != "job-value" {
		t.Fatalf("GetJobSecret() value = %q, want %q", gotJobSecret.EncryptedValue, "job-value")
	}

	_, err = q.GetJobSecret(ctx, newID())
	if !errors.Is(err, store.ErrJobSecretNotFound) {
		t.Fatalf("GetJobSecret(not found) error = %v, want ErrJobSecretNotFound", err)
	}

	allProd, err := q.ListJobSecrets(ctx, projectID, "", "prod", 10000, nil)
	if err != nil {
		t.Fatalf("ListJobSecrets(project+env) error = %v", err)
	}
	if len(allProd) != 2 {
		t.Fatalf("ListJobSecrets(project+env) len = %d, want 2", len(allProd))
	}

	jobOnly, err := q.ListJobSecrets(ctx, projectID, job.ID, "prod", 10000, nil)
	if err != nil {
		t.Fatalf("ListJobSecrets(project+job+env) error = %v", err)
	}
	if len(jobOnly) != 1 || jobOnly[0].ID != jobSecret.ID {
		t.Fatalf("ListJobSecrets(project+job+env) = %+v, want only %q", jobOnly, jobSecret.ID)
	}

	noneForEnv, err := q.ListJobSecrets(ctx, projectID, "", "staging", 10000, nil)
	if err != nil {
		t.Fatalf("ListJobSecrets(staging) error = %v", err)
	}
	if len(noneForEnv) != 0 {
		t.Fatalf("ListJobSecrets(staging) len = %d, want 0", len(noneForEnv))
	}

	byJob, err := q.ListJobSecretsByJob(ctx, job.ID, "prod")
	if err != nil {
		t.Fatalf("ListJobSecretsByJob() error = %v", err)
	}
	if len(byJob) != 2 {
		t.Fatalf("ListJobSecretsByJob() len = %d, want 2", len(byJob))
	}
	if byJob[0].ID != globalSecret.ID || byJob[1].ID != jobSecret.ID {
		t.Fatalf("ListJobSecretsByJob() order mismatch: got IDs [%q, %q], want [%q, %q]", byJob[0].ID, byJob[1].ID, globalSecret.ID, jobSecret.ID)
	}

	byJobNone, err := q.ListJobSecretsByJob(ctx, job.ID, "staging")
	if err != nil {
		t.Fatalf("ListJobSecretsByJob(staging) error = %v", err)
	}
	if len(byJobNone) != 0 {
		t.Fatalf("ListJobSecretsByJob(staging) len = %d, want 0", len(byJobNone))
	}

	if err := q.DeleteJobSecret(ctx, jobSecret.ID); err != nil {
		t.Fatalf("DeleteJobSecret() error = %v", err)
	}
	_, err = q.GetJobSecret(ctx, jobSecret.ID)
	if !errors.Is(err, store.ErrJobSecretNotFound) {
		t.Fatalf("GetJobSecret(after delete) error = %v, want ErrJobSecretNotFound", err)
	}

	if err := q.DeleteJobSecret(ctx, newID()); !errors.Is(err, store.ErrJobSecretNotFound) {
		t.Fatalf("DeleteJobSecret(not found) error = %v, want ErrJobSecretNotFound", err)
	}
}

func TestAPIKey_CRUD(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-api-key-crud"
	expiresAt := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Microsecond)
	key1 := &domain.APIKey{
		ProjectID: projectID,
		Name:      "primary",
		KeyHash:   "hash-" + newID(),
		KeyPrefix: "sk_test_abc",
		Scopes:    []string{"jobs:read", "jobs:trigger"},
		ExpiresAt: &expiresAt,
	}
	if err := q.CreateAPIKey(ctx, key1); err != nil {
		t.Fatalf("CreateAPIKey(key1) error = %v", err)
	}
	if key1.ID == "" {
		t.Fatal("CreateAPIKey(key1) did not set ID")
	}
	if key1.CreatedAt.IsZero() {
		t.Fatal("CreateAPIKey(key1) did not set CreatedAt")
	}

	key2 := &domain.APIKey{
		ProjectID: projectID,
		Name:      "secondary",
		KeyHash:   "hash-" + newID(),
		KeyPrefix: "sk_test_def",
		Scopes:    []string{"jobs:read"},
	}
	if err := q.CreateAPIKey(ctx, key2); err != nil {
		t.Fatalf("CreateAPIKey(key2) error = %v", err)
	}

	otherProjectKey := &domain.APIKey{
		ProjectID: "project-api-key-other",
		Name:      "other",
		KeyHash:   "hash-" + newID(),
		KeyPrefix: "sk_test_xyz",
		Scopes:    []string{},
	}
	if err := q.CreateAPIKey(ctx, otherProjectKey); err != nil {
		t.Fatalf("CreateAPIKey(other project) error = %v", err)
	}

	dup := &domain.APIKey{
		ProjectID: projectID,
		Name:      "duplicate-hash",
		KeyHash:   key1.KeyHash,
		KeyPrefix: "sk_test_dup",
	}
	if err := q.CreateAPIKey(ctx, dup); err == nil {
		t.Fatal("CreateAPIKey(duplicate hash) error = nil, want error")
	}

	got, err := q.GetAPIKeyByHash(ctx, key1.KeyHash)
	if err != nil {
		t.Fatalf("GetAPIKeyByHash() error = %v", err)
	}
	if got.ID != key1.ID || got.ProjectID != key1.ProjectID || got.Name != key1.Name || got.KeyHash != key1.KeyHash {
		t.Fatalf("GetAPIKeyByHash() mismatch: got %+v want %+v", *got, *key1)
	}
	if got.ExpiresAt == nil || !got.ExpiresAt.Equal(*key1.ExpiresAt) {
		t.Fatalf("GetAPIKeyByHash() expires_at mismatch: got %v want %v", got.ExpiresAt, key1.ExpiresAt)
	}

	_, err = q.GetAPIKeyByHash(ctx, "missing-hash")
	if err == nil {
		t.Fatal("GetAPIKeyByHash(missing) error = nil, want error")
	}

	keys, err := q.ListAPIKeysByProject(ctx, projectID, 10000, nil)
	if err != nil {
		t.Fatalf("ListAPIKeysByProject() error = %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("ListAPIKeysByProject() len = %d, want 2", len(keys))
	}
	if keys[0].ID != key2.ID || keys[1].ID != key1.ID {
		t.Fatalf("ListAPIKeysByProject() order mismatch: got IDs [%q, %q], want [%q, %q]", keys[0].ID, keys[1].ID, key2.ID, key1.ID)
	}

	none, err := q.ListAPIKeysByProject(ctx, "project-api-key-none", 10000, nil)
	if err != nil {
		t.Fatalf("ListAPIKeysByProject(none) error = %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("ListAPIKeysByProject(none) len = %d, want 0", len(none))
	}

	if err := q.RevokeAPIKey(ctx, key1.ID); err != nil {
		t.Fatalf("RevokeAPIKey() error = %v", err)
	}
	revoked, err := q.GetAPIKeyByHash(ctx, key1.KeyHash)
	if err != nil {
		t.Fatalf("GetAPIKeyByHash(revoked) error = %v", err)
	}
	if revoked.RevokedAt == nil {
		t.Fatal("GetAPIKeyByHash(revoked) revoked_at = nil, want non-nil")
	}

	keys, err = q.ListAPIKeysByProject(ctx, projectID, 10000, nil)
	if err != nil {
		t.Fatalf("ListAPIKeysByProject(after revoke) error = %v", err)
	}
	if len(keys) != 1 || keys[0].ID != key2.ID {
		t.Fatalf("ListAPIKeysByProject(after revoke) = %+v, want only %q", keys, key2.ID)
	}

	if err := q.RevokeAPIKey(ctx, key1.ID); err == nil {
		t.Fatal("RevokeAPIKey(already revoked) error = nil, want error")
	}
}

func TestAPIKey_TouchLastUsed(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	key := &domain.APIKey{
		ProjectID: "project-api-key-touch",
		Name:      "touch-key",
		KeyHash:   "hash-" + newID(),
		KeyPrefix: "sk_touch",
		Scopes:    []string{"jobs:trigger"},
	}
	if err := q.CreateAPIKey(ctx, key); err != nil {
		t.Fatalf("CreateAPIKey() error = %v", err)
	}

	storedBefore, err := q.GetAPIKeyByHash(ctx, key.KeyHash)
	if err != nil {
		t.Fatalf("GetAPIKeyByHash(before touch) error = %v", err)
	}
	if storedBefore.LastUsedAt != nil {
		t.Fatalf("LastUsedAt before touch = %v, want nil", storedBefore.LastUsedAt)
	}

	if err := q.TouchAPIKeyLastUsed(ctx, key.ID); err != nil {
		t.Fatalf("TouchAPIKeyLastUsed(existing) error = %v", err)
	}

	storedAfter, err := q.GetAPIKeyByHash(ctx, key.KeyHash)
	if err != nil {
		t.Fatalf("GetAPIKeyByHash(after touch) error = %v", err)
	}
	if storedAfter.LastUsedAt == nil {
		t.Fatal("LastUsedAt after touch = nil, want non-nil")
	}

	if err := q.TouchAPIKeyLastUsed(ctx, newID()); err != nil {
		t.Fatalf("TouchAPIKeyLastUsed(missing) error = %v, want nil", err)
	}
}

func TestJobVersion_CRUD(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-job-version-crud")

	v1 := &domain.JobVersion{
		ID:            newID(),
		JobID:         job.ID,
		Version:       1,
		Name:          job.Name,
		Slug:          job.Slug,
		Description:   "first version",
		Cron:          "*/5 * * * *",
		PayloadSchema: json.RawMessage(`{"type":"object","properties":{"ok":{"type":"boolean"}}}`),
		Tags:          map[string]string{"team": "core", "service": "strait"},
		EndpointURL:   "https://example.com/v1",
		MaxAttempts:   3,
		TimeoutSecs:   30,
		WebhookURL:    "https://example.com/webhook-v1",
		WebhookSecret: "secret-v1",
		RunTTLSecs:    3600,
	}
	if err := q.CreateJobVersion(ctx, v1); err != nil {
		t.Fatalf("CreateJobVersion(v1) error = %v", err)
	}
	if v1.CreatedAt.IsZero() {
		t.Fatal("CreateJobVersion(v1) did not set CreatedAt")
	}

	v2 := &domain.JobVersion{
		ID:                  newID(),
		JobID:               job.ID,
		Version:             2,
		Name:                "job-v2",
		Slug:                job.Slug,
		Description:         "second version",
		EndpointURL:         "https://example.com/v2",
		FallbackEndpointURL: "https://fallback.example.com/v2",
		MaxAttempts:         5,
		TimeoutSecs:         60,
		Tags:                map[string]string{"team": "platform"},
	}
	if err := q.CreateJobVersion(ctx, v2); err != nil {
		t.Fatalf("CreateJobVersion(v2) error = %v", err)
	}

	vDup := &domain.JobVersion{
		ID:          newID(),
		JobID:       job.ID,
		Version:     2,
		Name:        "duplicate",
		Slug:        "duplicate",
		EndpointURL: "https://example.com/dup",
		MaxAttempts: 1,
		TimeoutSecs: 1,
	}
	if err := q.CreateJobVersion(ctx, vDup); err == nil {
		t.Fatal("CreateJobVersion(duplicate version) error = nil, want error")
	}

	versions, err := q.ListJobVersionsByJob(ctx, job.ID, 10000, nil)
	if err != nil {
		t.Fatalf("ListJobVersionsByJob() error = %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("ListJobVersionsByJob() len = %d, want 2", len(versions))
	}
	if versions[0].Version != 2 || versions[1].Version != 1 {
		t.Fatalf("ListJobVersionsByJob() order mismatch: got versions [%d, %d], want [2, 1]", versions[0].Version, versions[1].Version)
	}
	if versions[0].FallbackEndpointURL != v2.FallbackEndpointURL {
		t.Fatalf("ListJobVersionsByJob() fallback endpoint = %q, want %q", versions[0].FallbackEndpointURL, v2.FallbackEndpointURL)
	}

	empty, err := q.ListJobVersionsByJob(ctx, newID(), 100, nil)
	if err != nil {
		t.Fatalf("ListJobVersionsByJob(unknown job) error = %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("ListJobVersionsByJob(unknown job) len = %d, want 0", len(empty))
	}

	gotV1, err := q.GetJobVersion(ctx, job.ID, 1)
	if err != nil {
		t.Fatalf("GetJobVersion(v1) error = %v", err)
	}
	if gotV1.ID != v1.ID || gotV1.Name != v1.Name || gotV1.EndpointURL != v1.EndpointURL {
		t.Fatalf("GetJobVersion(v1) mismatch: got %+v want %+v", *gotV1, *v1)
	}
	if !jsonEqual(gotV1.PayloadSchema, v1.PayloadSchema) {
		t.Fatalf("GetJobVersion(v1) payload schema mismatch: got %s want %s", string(gotV1.PayloadSchema), string(v1.PayloadSchema))
	}
	if len(gotV1.Tags) != len(v1.Tags) {
		t.Fatalf("GetJobVersion(v1) tags len = %d, want %d", len(gotV1.Tags), len(v1.Tags))
	}
	for k, want := range v1.Tags {
		if gotV1.Tags[k] != want {
			t.Fatalf("GetJobVersion(v1) tags[%q] = %q, want %q", k, gotV1.Tags[k], want)
		}
	}

	_, err = q.GetJobVersion(ctx, job.ID, 99)
	if err == nil {
		t.Fatal("GetJobVersion(not found) error = nil, want error")
	}
}

func TestWebhookDelivery_CRUD(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-webhook-delivery-crud")
	run := mustCreateRun(t, ctx, q, job)

	nextRetry := time.Now().UTC().Add(-2 * time.Minute)
	statusCode := 502
	delivery1 := &domain.WebhookDelivery{
		RunID:          run.ID,
		JobID:          job.ID,
		WebhookURL:     "https://example.com/webhook/1",
		Status:         "pending",
		Attempts:       1,
		MaxAttempts:    5,
		LastStatusCode: &statusCode,
		LastError:      "bad gateway",
		NextRetryAt:    &nextRetry,
	}
	if err := q.CreateWebhookDelivery(ctx, delivery1); err != nil {
		t.Fatalf("CreateWebhookDelivery(delivery1) error = %v", err)
	}
	if delivery1.ID == "" {
		t.Fatal("CreateWebhookDelivery(delivery1) did not set ID")
	}
	if delivery1.CreatedAt.IsZero() || delivery1.UpdatedAt.IsZero() {
		t.Fatal("CreateWebhookDelivery(delivery1) did not set timestamps")
	}

	delivery2 := &domain.WebhookDelivery{
		RunID:       run.ID,
		JobID:       job.ID,
		WebhookURL:  "https://example.com/webhook/2",
		Status:      "failed",
		Attempts:    3,
		MaxAttempts: 3,
		LastError:   "timeout",
	}
	if err := q.CreateWebhookDelivery(ctx, delivery2); err != nil {
		t.Fatalf("CreateWebhookDelivery(delivery2) error = %v", err)
	}

	dupID := &domain.WebhookDelivery{
		ID:          delivery1.ID,
		RunID:       run.ID,
		JobID:       job.ID,
		WebhookURL:  "https://example.com/webhook/dup",
		Status:      "pending",
		Attempts:    0,
		MaxAttempts: 3,
	}
	if err := q.CreateWebhookDelivery(ctx, dupID); err == nil {
		t.Fatal("CreateWebhookDelivery(duplicate id) error = nil, want error")
	}

	got, err := q.GetWebhookDelivery(ctx, delivery1.ID)
	if err != nil {
		t.Fatalf("GetWebhookDelivery(existing) error = %v", err)
	}
	if got.ID != delivery1.ID || got.RunID != run.ID || got.JobID != job.ID || got.Status != "pending" {
		t.Fatalf("GetWebhookDelivery(existing) mismatch: got %+v", *got)
	}

	_, err = q.GetWebhookDelivery(ctx, newID())
	if err == nil {
		t.Fatal("GetWebhookDelivery(missing) error = nil, want error")
	}

	deliveredAt := time.Now().UTC()
	newStatusCode := 200
	delivery1.Status = "delivered"
	delivery1.Attempts = 2
	delivery1.LastStatusCode = &newStatusCode
	delivery1.LastError = ""
	delivery1.NextRetryAt = nil
	delivery1.DeliveredAt = &deliveredAt
	if err := q.UpdateWebhookDelivery(ctx, delivery1); err != nil {
		t.Fatalf("UpdateWebhookDelivery(existing) error = %v", err)
	}
	if delivery1.UpdatedAt.IsZero() {
		t.Fatal("UpdateWebhookDelivery(existing) did not set UpdatedAt")
	}

	got, err = q.GetWebhookDelivery(ctx, delivery1.ID)
	if err != nil {
		t.Fatalf("GetWebhookDelivery(after update) error = %v", err)
	}
	if got.Status != "delivered" || got.Attempts != 2 {
		t.Fatalf("GetWebhookDelivery(after update) mismatch: got %+v", *got)
	}
	if got.DeliveredAt == nil {
		t.Fatal("GetWebhookDelivery(after update) delivered_at = nil, want non-nil")
	}
	if got.NextRetryAt != nil {
		t.Fatalf("GetWebhookDelivery(after update) next_retry_at = %v, want nil", got.NextRetryAt)
	}

	missingDelivery := &domain.WebhookDelivery{
		ID:             newID(),
		Status:         "pending",
		Attempts:       1,
		LastStatusCode: &newStatusCode,
	}
	if err := q.UpdateWebhookDelivery(ctx, missingDelivery); err == nil {
		t.Fatal("UpdateWebhookDelivery(missing) error = nil, want error")
	}

	all, err := q.ListWebhookDeliveries(ctx, job.ProjectID, "", 10, nil)
	if err != nil {
		t.Fatalf("ListWebhookDeliveries(all) error = %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("ListWebhookDeliveries(all) len = %d, want 2", len(all))
	}
	if all[0].ID != delivery2.ID || all[1].ID != delivery1.ID {
		t.Fatalf("ListWebhookDeliveries(all) order mismatch: got IDs [%q, %q], want [%q, %q]", all[0].ID, all[1].ID, delivery2.ID, delivery1.ID)
	}

	delivered, err := q.ListWebhookDeliveries(ctx, job.ProjectID, "delivered", 10, nil)
	if err != nil {
		t.Fatalf("ListWebhookDeliveries(delivered) error = %v", err)
	}
	if len(delivered) != 1 || delivered[0].ID != delivery1.ID {
		t.Fatalf("ListWebhookDeliveries(delivered) = %+v, want only %q", delivered, delivery1.ID)
	}

	none, err := q.ListWebhookDeliveries(ctx, job.ProjectID, "pending", 0, nil)
	if err != nil {
		t.Fatalf("ListWebhookDeliveries(limit 0) error = %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("ListWebhookDeliveries(limit 0) len = %d, want 0", len(none))
	}
}

func TestWebhookDelivery_PendingRetries(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-webhook-pending-retries")
	run := mustCreateRun(t, ctx, q, job)

	older := time.Now().UTC().Add(-5 * time.Minute)
	recent := time.Now().UTC().Add(-1 * time.Minute)
	future := time.Now().UTC().Add(5 * time.Minute)

	dueOld := &domain.WebhookDelivery{
		RunID:       run.ID,
		JobID:       job.ID,
		WebhookURL:  "https://example.com/webhook/due-old",
		Status:      "pending",
		Attempts:    1,
		MaxAttempts: 3,
		NextRetryAt: &older,
	}
	if err := q.CreateWebhookDelivery(ctx, dueOld); err != nil {
		t.Fatalf("CreateWebhookDelivery(due old) error = %v", err)
	}

	dueRecent := &domain.WebhookDelivery{
		RunID:       run.ID,
		JobID:       job.ID,
		WebhookURL:  "https://example.com/webhook/due-recent",
		Status:      "pending",
		Attempts:    2,
		MaxAttempts: 3,
		NextRetryAt: &recent,
	}
	if err := q.CreateWebhookDelivery(ctx, dueRecent); err != nil {
		t.Fatalf("CreateWebhookDelivery(due recent) error = %v", err)
	}

	notDueFuture := &domain.WebhookDelivery{
		RunID:       run.ID,
		JobID:       job.ID,
		WebhookURL:  "https://example.com/webhook/future",
		Status:      "pending",
		Attempts:    1,
		MaxAttempts: 3,
		NextRetryAt: &future,
	}
	if err := q.CreateWebhookDelivery(ctx, notDueFuture); err != nil {
		t.Fatalf("CreateWebhookDelivery(future) error = %v", err)
	}

	pendingNoRetry := &domain.WebhookDelivery{
		RunID:       run.ID,
		JobID:       job.ID,
		WebhookURL:  "https://example.com/webhook/no-retry",
		Status:      "pending",
		Attempts:    1,
		MaxAttempts: 3,
	}
	if err := q.CreateWebhookDelivery(ctx, pendingNoRetry); err != nil {
		t.Fatalf("CreateWebhookDelivery(no retry) error = %v", err)
	}

	failedDue := &domain.WebhookDelivery{
		RunID:       run.ID,
		JobID:       job.ID,
		WebhookURL:  "https://example.com/webhook/failed",
		Status:      "failed",
		Attempts:    3,
		MaxAttempts: 3,
		NextRetryAt: &older,
	}
	if err := q.CreateWebhookDelivery(ctx, failedDue); err != nil {
		t.Fatalf("CreateWebhookDelivery(failed due) error = %v", err)
	}

	pending, err := q.ListPendingWebhookRetries(ctx)
	if err != nil {
		t.Fatalf("ListPendingWebhookRetries() error = %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("ListPendingWebhookRetries() len = %d, want 2", len(pending))
	}
	if pending[0].ID != dueOld.ID || pending[1].ID != dueRecent.ID {
		t.Fatalf("ListPendingWebhookRetries() order mismatch: got IDs [%q, %q], want [%q, %q]", pending[0].ID, pending[1].ID, dueOld.ID, dueRecent.ID)
	}

	now := time.Now().UTC()
	statusCode := 200
	dueOld.Status = "delivered"
	dueOld.DeliveredAt = &now
	dueOld.NextRetryAt = nil
	dueOld.LastStatusCode = &statusCode
	if err := q.UpdateWebhookDelivery(ctx, dueOld); err != nil {
		t.Fatalf("UpdateWebhookDelivery(due old delivered) error = %v", err)
	}

	dueRecent.Status = "delivered"
	dueRecent.DeliveredAt = &now
	dueRecent.NextRetryAt = nil
	dueRecent.LastStatusCode = &statusCode
	if err := q.UpdateWebhookDelivery(ctx, dueRecent); err != nil {
		t.Fatalf("UpdateWebhookDelivery(due recent delivered) error = %v", err)
	}

	nonePending, err := q.ListPendingWebhookRetries(ctx)
	if err != nil {
		t.Fatalf("ListPendingWebhookRetries(after updates) error = %v", err)
	}
	if len(nonePending) != 0 {
		t.Fatalf("ListPendingWebhookRetries(after updates) len = %d, want 0", len(nonePending))
	}
}

func TestJobGroup_CRUD(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-job-group-crud"
	group := &domain.JobGroup{
		ID:          newID(),
		ProjectID:   projectID,
		Name:        "Core Jobs",
		Slug:        "core-jobs",
		Description: "Core grouped jobs",
	}

	if err := q.CreateJobGroup(ctx, group); err != nil {
		t.Fatalf("CreateJobGroup() error = %v", err)
	}
	if group.CreatedAt.IsZero() {
		t.Fatal("CreateJobGroup() did not set CreatedAt")
	}
	if group.UpdatedAt.IsZero() {
		t.Fatal("CreateJobGroup() did not set UpdatedAt")
	}

	gotGroup, err := q.GetJobGroup(ctx, group.ID)
	if err != nil {
		t.Fatalf("GetJobGroup() error = %v", err)
	}
	if gotGroup.ID != group.ID || gotGroup.ProjectID != group.ProjectID || gotGroup.Name != group.Name || gotGroup.Slug != group.Slug || gotGroup.Description != group.Description {
		t.Fatalf("GetJobGroup() mismatch: got %+v want %+v", *gotGroup, *group)
	}

	groups, err := q.ListJobGroups(ctx, projectID, 100, nil)
	if err != nil {
		t.Fatalf("ListJobGroups() error = %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("ListJobGroups() len = %d, want 1", len(groups))
	}

	jobInGroup := baseJob(newID(), projectID)
	jobInGroup.GroupID = group.ID
	if err := q.CreateJob(ctx, jobInGroup); err != nil {
		t.Fatalf("CreateJob(jobInGroup) error = %v", err)
	}
	jobOutsideGroup := baseJob(newID(), projectID)
	if err := q.CreateJob(ctx, jobOutsideGroup); err != nil {
		t.Fatalf("CreateJob(jobOutsideGroup) error = %v", err)
	}

	jobsByGroup, err := q.ListJobsByGroup(ctx, group.ID, 100, nil)
	if err != nil {
		t.Fatalf("ListJobsByGroup() error = %v", err)
	}
	if len(jobsByGroup) != 1 {
		t.Fatalf("ListJobsByGroup() len = %d, want 1", len(jobsByGroup))
	}
	if jobsByGroup[0].ID != jobInGroup.ID {
		t.Fatalf("ListJobsByGroup() job id = %q, want %q", jobsByGroup[0].ID, jobInGroup.ID)
	}

	emptyJobs, err := q.ListJobsByGroup(ctx, newID(), 100, nil)
	if err != nil {
		t.Fatalf("ListJobsByGroup() empty error = %v", err)
	}
	if len(emptyJobs) != 0 {
		t.Fatalf("ListJobsByGroup() empty len = %d, want 0", len(emptyJobs))
	}

	group.Name = "Core Jobs Updated"
	group.Slug = "core-jobs-updated"
	group.Description = "Updated description"
	if err := q.UpdateJobGroup(ctx, group); err != nil {
		t.Fatalf("UpdateJobGroup() error = %v", err)
	}

	updated, err := q.GetJobGroup(ctx, group.ID)
	if err != nil {
		t.Fatalf("GetJobGroup() after update error = %v", err)
	}
	if updated.Name != group.Name || updated.Slug != group.Slug || updated.Description != group.Description {
		t.Fatalf("updated group mismatch: got %+v want %+v", *updated, *group)
	}
	if err := q.DeleteJob(ctx, jobInGroup.ID); err != nil {
		t.Fatalf("DeleteJob(jobInGroup) error = %v", err)
	}

	if err := q.DeleteJobGroup(ctx, group.ID); err != nil {
		t.Fatalf("DeleteJobGroup() error = %v", err)
	}
	if _, err := q.GetJobGroup(ctx, group.ID); !errors.Is(err, store.ErrJobGroupNotFound) {
		t.Fatalf("GetJobGroup() after delete error = %v, want ErrJobGroupNotFound", err)
	}

	if _, err := q.GetJobGroup(ctx, newID()); !errors.Is(err, store.ErrJobGroupNotFound) {
		t.Fatalf("GetJobGroup() not found error = %v, want ErrJobGroupNotFound", err)
	}

	notFoundGroup := &domain.JobGroup{ID: newID(), ProjectID: projectID, Name: "missing", Slug: "missing"}
	if err := q.UpdateJobGroup(ctx, notFoundGroup); !errors.Is(err, store.ErrJobGroupNotFound) {
		t.Fatalf("UpdateJobGroup() not found error = %v, want ErrJobGroupNotFound", err)
	}

	if err := q.DeleteJobGroup(ctx, newID()); !errors.Is(err, store.ErrJobGroupNotFound) {
		t.Fatalf("DeleteJobGroup() not found error = %v, want ErrJobGroupNotFound", err)
	}

	emptyGroups, err := q.ListJobGroups(ctx, "project-job-group-empty", 100, nil)
	if err != nil {
		t.Fatalf("ListJobGroups() empty project error = %v", err)
	}
	if len(emptyGroups) != 0 {
		t.Fatalf("ListJobGroups() empty project len = %d, want 0", len(emptyGroups))
	}
}

func TestJobDependency_CRUD(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-job-dependency-crud"
	job := mustCreateJob(t, ctx, q, projectID)
	depJobA := mustCreateJob(t, ctx, q, projectID)
	depJobB := mustCreateJob(t, ctx, q, projectID)

	dep := &domain.JobDependency{
		ID:             newID(),
		JobID:          job.ID,
		DependsOnJobID: depJobA.ID,
	}
	if err := q.CreateJobDependency(ctx, dep); err != nil {
		t.Fatalf("CreateJobDependency() error = %v", err)
	}
	if dep.CreatedAt.IsZero() {
		t.Fatal("CreateJobDependency() did not set CreatedAt")
	}
	if dep.Condition != "completed" {
		t.Fatalf("CreateJobDependency() condition = %q, want completed", dep.Condition)
	}

	deps, err := q.ListJobDependencies(ctx, job.ID, 100, nil)
	if err != nil {
		t.Fatalf("ListJobDependencies() error = %v", err)
	}
	if len(deps) != 1 {
		t.Fatalf("ListJobDependencies() len = %d, want 1", len(deps))
	}
	if deps[0].ID != dep.ID || deps[0].JobID != dep.JobID || deps[0].DependsOnJobID != dep.DependsOnJobID || deps[0].Condition != dep.Condition {
		t.Fatalf("ListJobDependencies() mismatch: got %+v want %+v", deps[0], *dep)
	}

	dep2 := &domain.JobDependency{
		JobID:          job.ID,
		DependsOnJobID: depJobB.ID,
		Condition:      "failed",
	}
	if err := q.CreateJobDependency(ctx, dep2); err != nil {
		t.Fatalf("CreateJobDependency() second error = %v", err)
	}

	if err := q.DeleteJobDependency(ctx, dep.ID); err != nil {
		t.Fatalf("DeleteJobDependency() error = %v", err)
	}

	deps, err = q.ListJobDependencies(ctx, job.ID, 100, nil)
	if err != nil {
		t.Fatalf("ListJobDependencies() after delete error = %v", err)
	}
	if len(deps) != 1 {
		t.Fatalf("ListJobDependencies() after delete len = %d, want 1", len(deps))
	}
	if deps[0].ID != dep2.ID {
		t.Fatalf("ListJobDependencies() remaining id = %q, want %q", deps[0].ID, dep2.ID)
	}

	if err := q.CreateJobDependency(ctx, &domain.JobDependency{JobID: job.ID, DependsOnJobID: job.ID}); err == nil {
		t.Fatal("CreateJobDependency() self dependency error = nil, want error")
	}

	duplicate := &domain.JobDependency{JobID: job.ID, DependsOnJobID: depJobB.ID}
	if err := q.CreateJobDependency(ctx, duplicate); err == nil {
		t.Fatal("CreateJobDependency() duplicate error = nil, want error")
	}

	missing := &domain.JobDependency{JobID: newID(), DependsOnJobID: depJobA.ID}
	if err := q.CreateJobDependency(ctx, missing); err == nil {
		t.Fatal("CreateJobDependency() missing job FK error = nil, want error")
	}

	emptyDeps, err := q.ListJobDependencies(ctx, depJobA.ID, 100, nil)
	if err != nil {
		t.Fatalf("ListJobDependencies() empty error = %v", err)
	}
	if len(emptyDeps) != 0 {
		t.Fatalf("ListJobDependencies() empty len = %d, want 0", len(emptyDeps))
	}

	if err := q.DeleteJobDependency(ctx, newID()); err != nil {
		t.Fatalf("DeleteJobDependency() missing id error = %v, want nil", err)
	}
}

func TestEnvironment_CRUD(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-environment-crud"
	parent := &domain.Environment{
		ProjectID: projectID,
		Name:      "Shared",
		Slug:      "shared",
		Variables: map[string]string{"GLOBAL": "1"},
	}
	if err := q.CreateEnvironment(ctx, parent); err != nil {
		t.Fatalf("CreateEnvironment(parent) error = %v", err)
	}

	env := &domain.Environment{
		ID:        newID(),
		ProjectID: projectID,
		Name:      "Production",
		Slug:      "production",
		Variables: map[string]string{"DB_HOST": "db.internal", "REGION": "us-east-1"},
	}
	if err := q.CreateEnvironment(ctx, env); err != nil {
		t.Fatalf("CreateEnvironment() error = %v", err)
	}
	if env.CreatedAt.IsZero() {
		t.Fatal("CreateEnvironment() did not set CreatedAt")
	}
	if env.UpdatedAt.IsZero() {
		t.Fatal("CreateEnvironment() did not set UpdatedAt")
	}

	gotEnv, err := q.GetEnvironment(ctx, env.ID)
	if err != nil {
		t.Fatalf("GetEnvironment() error = %v", err)
	}
	if gotEnv.ID != env.ID || gotEnv.ProjectID != env.ProjectID || gotEnv.Name != env.Name || gotEnv.Slug != env.Slug {
		t.Fatalf("GetEnvironment() mismatch: got %+v want %+v", *gotEnv, *env)
	}
	if len(gotEnv.Variables) != len(env.Variables) {
		t.Fatalf("GetEnvironment() variables len = %d, want %d", len(gotEnv.Variables), len(env.Variables))
	}
	for k, want := range env.Variables {
		if gotEnv.Variables[k] != want {
			t.Fatalf("GetEnvironment() variable %q = %q, want %q", k, gotEnv.Variables[k], want)
		}
	}

	envs, err := q.ListEnvironments(ctx, projectID, 100, nil)
	if err != nil {
		t.Fatalf("ListEnvironments() error = %v", err)
	}
	if len(envs) != 2 {
		t.Fatalf("ListEnvironments() len = %d, want 2", len(envs))
	}

	env.Name = "Production Updated"
	env.Slug = "production-updated"
	env.ParentID = parent.ID
	env.Variables = map[string]string{"DB_HOST": "db.updated", "NEW_KEY": "value"}
	if err := q.UpdateEnvironment(ctx, env); err != nil {
		t.Fatalf("UpdateEnvironment() error = %v", err)
	}

	updated, err := q.GetEnvironment(ctx, env.ID)
	if err != nil {
		t.Fatalf("GetEnvironment() after update error = %v", err)
	}
	if updated.Name != env.Name || updated.Slug != env.Slug || updated.ParentID != env.ParentID {
		t.Fatalf("updated environment mismatch: got %+v want %+v", *updated, *env)
	}
	if len(updated.Variables) != len(env.Variables) {
		t.Fatalf("updated variables len = %d, want %d", len(updated.Variables), len(env.Variables))
	}
	for k, want := range env.Variables {
		if updated.Variables[k] != want {
			t.Fatalf("updated variable %q = %q, want %q", k, updated.Variables[k], want)
		}
	}

	if err := q.DeleteEnvironment(ctx, env.ID); err != nil {
		t.Fatalf("DeleteEnvironment() error = %v", err)
	}
	if _, err := q.GetEnvironment(ctx, env.ID); !errors.Is(err, store.ErrEnvironmentNotFound) {
		t.Fatalf("GetEnvironment() after delete error = %v, want ErrEnvironmentNotFound", err)
	}

	if _, err := q.GetEnvironment(ctx, newID()); !errors.Is(err, store.ErrEnvironmentNotFound) {
		t.Fatalf("GetEnvironment() not found error = %v, want ErrEnvironmentNotFound", err)
	}

	notFoundEnv := &domain.Environment{ID: newID(), ProjectID: projectID, Name: "missing", Slug: "missing"}
	if err := q.UpdateEnvironment(ctx, notFoundEnv); !errors.Is(err, store.ErrEnvironmentNotFound) {
		t.Fatalf("UpdateEnvironment() not found error = %v, want ErrEnvironmentNotFound", err)
	}

	if err := q.DeleteEnvironment(ctx, newID()); !errors.Is(err, store.ErrEnvironmentNotFound) {
		t.Fatalf("DeleteEnvironment() not found error = %v, want ErrEnvironmentNotFound", err)
	}

	emptyEnvs, err := q.ListEnvironments(ctx, "project-environment-empty", 100, nil)
	if err != nil {
		t.Fatalf("ListEnvironments() empty project error = %v", err)
	}
	if len(emptyEnvs) != 0 {
		t.Fatalf("ListEnvironments() empty project len = %d, want 0", len(emptyEnvs))
	}
}

func TestEnvironment_InheritanceResolution(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-environment-inheritance"
	parent := &domain.Environment{
		ProjectID: projectID,
		Name:      "Parent",
		Slug:      "parent",
		Variables: map[string]string{"A": "1", "SHARED": "parent", "P": "p"},
	}
	if err := q.CreateEnvironment(ctx, parent); err != nil {
		t.Fatalf("CreateEnvironment(parent) error = %v", err)
	}

	child := &domain.Environment{
		ProjectID: projectID,
		Name:      "Child",
		Slug:      "child",
		ParentID:  parent.ID,
		Variables: map[string]string{"B": "2", "SHARED": "child"},
	}
	if err := q.CreateEnvironment(ctx, child); err != nil {
		t.Fatalf("CreateEnvironment(child) error = %v", err)
	}

	grandchild := &domain.Environment{
		ProjectID: projectID,
		Name:      "Grandchild",
		Slug:      "grandchild",
		ParentID:  child.ID,
		Variables: map[string]string{"C": "3", "B": "override"},
	}
	if err := q.CreateEnvironment(ctx, grandchild); err != nil {
		t.Fatalf("CreateEnvironment(grandchild) error = %v", err)
	}

	resolved, err := q.GetResolvedEnvironmentVariables(ctx, grandchild.ID)
	if err != nil {
		t.Fatalf("GetResolvedEnvironmentVariables() error = %v", err)
	}
	want := map[string]string{"A": "1", "P": "p", "SHARED": "child", "B": "override", "C": "3"}
	if len(resolved) != len(want) {
		t.Fatalf("GetResolvedEnvironmentVariables() len = %d, want %d", len(resolved), len(want))
	}
	for k, v := range want {
		if resolved[k] != v {
			t.Fatalf("resolved[%q] = %q, want %q", k, resolved[k], v)
		}
	}

	rootOnly, err := q.GetResolvedEnvironmentVariables(ctx, parent.ID)
	if err != nil {
		t.Fatalf("GetResolvedEnvironmentVariables(parent) error = %v", err)
	}
	if len(rootOnly) != len(parent.Variables) {
		t.Fatalf("GetResolvedEnvironmentVariables(parent) len = %d, want %d", len(rootOnly), len(parent.Variables))
	}
	for k, v := range parent.Variables {
		if rootOnly[k] != v {
			t.Fatalf("rootOnly[%q] = %q, want %q", k, rootOnly[k], v)
		}
	}

	if _, err := q.GetResolvedEnvironmentVariables(ctx, newID()); !errors.Is(err, store.ErrEnvironmentNotFound) {
		t.Fatalf("GetResolvedEnvironmentVariables() missing error = %v, want ErrEnvironmentNotFound", err)
	}

	deepProjectID := "project-environment-deep"
	var prevID string
	for i := range 11 {
		env := &domain.Environment{
			ProjectID: deepProjectID,
			Name:      "DeepEnv" + strconv.Itoa(i),
			Slug:      "deep-env-" + strconv.Itoa(i),
			ParentID:  prevID,
			Variables: map[string]string{"depth": strconv.Itoa(i)},
		}
		if err := q.CreateEnvironment(ctx, env); err != nil {
			t.Fatalf("CreateEnvironment(deep %d) error = %v", i, err)
		}
		prevID = env.ID
	}

	if _, err := q.GetResolvedEnvironmentVariables(ctx, prevID); err == nil {
		t.Fatal("GetResolvedEnvironmentVariables() deep chain error = nil, want error")
	}
}

func TestGetJobHealthStats(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-health-stats")

	now := time.Now().UTC()
	since := now.Add(-time.Hour)

	type runFixture struct {
		status      domain.RunStatus
		startOffset time.Duration
		endOffset   time.Duration
		setTimes    bool
		createdAt   time.Time
	}

	fixtures := []runFixture{
		{status: domain.StatusCompleted, startOffset: -60 * time.Second, endOffset: -50 * time.Second, setTimes: true, createdAt: now.Add(-55 * time.Second)},
		{status: domain.StatusFailed, startOffset: -120 * time.Second, endOffset: -100 * time.Second, setTimes: true, createdAt: now.Add(-110 * time.Second)},
		{status: domain.StatusTimedOut, startOffset: -180 * time.Second, endOffset: -150 * time.Second, setTimes: true, createdAt: now.Add(-170 * time.Second)},
		{status: domain.StatusCrashed, startOffset: -240 * time.Second, endOffset: -200 * time.Second, setTimes: true, createdAt: now.Add(-230 * time.Second)},
		{status: domain.StatusSystemFailed, startOffset: -300 * time.Second, endOffset: -250 * time.Second, setTimes: true, createdAt: now.Add(-290 * time.Second)},
		{status: domain.StatusCanceled, setTimes: false, createdAt: now.Add(-20 * time.Second)},
		{status: domain.StatusExpired, setTimes: false, createdAt: now.Add(-10 * time.Second)},
		{status: domain.StatusCompleted, startOffset: -3 * time.Hour, endOffset: -2*time.Hour - 50*time.Minute, setTimes: true, createdAt: now.Add(-3 * time.Hour)},
	}

	for i, fixture := range fixtures {
		runID := newID()
		var startedAt any
		var finishedAt any
		if fixture.setTimes {
			start := now.Add(fixture.startOffset)
			end := now.Add(fixture.endOffset)
			startedAt = start
			finishedAt = end
		}

		if _, err := testDB.Pool.Exec(ctx, `
			INSERT INTO job_runs (
				id, job_id, project_id, status, attempt, payload, triggered_by, created_at, started_at, finished_at
			) VALUES ($1, $2, $3, $4, 1, '{}'::jsonb, 'manual', $5, $6, $7)
		`, runID, job.ID, job.ProjectID, fixture.status, fixture.createdAt, startedAt, finishedAt); err != nil {
			t.Fatalf("insert job_runs fixture %d error = %v", i, err)
		}
	}

	stats, err := q.GetJobHealthStats(ctx, job.ID, since)
	if err != nil {
		t.Fatalf("GetJobHealthStats() error = %v", err)
	}

	if stats.TotalRuns != 7 {
		t.Fatalf("TotalRuns = %d, want 7", stats.TotalRuns)
	}
	if stats.CompletedRuns != 1 {
		t.Fatalf("CompletedRuns = %d, want 1", stats.CompletedRuns)
	}
	if stats.FailedRuns != 1 {
		t.Fatalf("FailedRuns = %d, want 1", stats.FailedRuns)
	}
	if stats.TimedOutRuns != 1 {
		t.Fatalf("TimedOutRuns = %d, want 1", stats.TimedOutRuns)
	}
	if stats.CrashedRuns != 2 {
		t.Fatalf("CrashedRuns = %d, want 2", stats.CrashedRuns)
	}
	if stats.CanceledRuns != 1 {
		t.Fatalf("CanceledRuns = %d, want 1", stats.CanceledRuns)
	}

	wantSuccessRate := 100.0 / 7.0
	if stats.SuccessRate < wantSuccessRate-0.0001 || stats.SuccessRate > wantSuccessRate+0.0001 {
		t.Fatalf("SuccessRate = %f, want %f", stats.SuccessRate, wantSuccessRate)
	}
	if stats.AvgDurationSecs < 29.999 || stats.AvgDurationSecs > 30.001 {
		t.Fatalf("AvgDurationSecs = %f, want 30", stats.AvgDurationSecs)
	}
	if stats.P95DurationSecs < 47.999 || stats.P95DurationSecs > 48.001 {
		t.Fatalf("P95DurationSecs = %f, want 48", stats.P95DurationSecs)
	}

	emptyJob := mustCreateJob(t, ctx, q, "project-health-stats-empty")
	emptyStats, err := q.GetJobHealthStats(ctx, emptyJob.ID, since)
	if err != nil {
		t.Fatalf("GetJobHealthStats() empty error = %v", err)
	}
	if emptyStats.TotalRuns != 0 || emptyStats.CompletedRuns != 0 || emptyStats.FailedRuns != 0 || emptyStats.TimedOutRuns != 0 || emptyStats.CrashedRuns != 0 || emptyStats.CanceledRuns != 0 {
		t.Fatalf("GetJobHealthStats() empty stats = %+v, want all zero", *emptyStats)
	}
}

func TestBatchUpdateJobsEnabled(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-batch-update-enabled"
	job1 := baseJob(newID(), projectID)
	job2 := baseJob(newID(), projectID)
	job3 := baseJob(newID(), projectID)

	if err := q.CreateJob(ctx, job1); err != nil {
		t.Fatalf("CreateJob(job1) error = %v", err)
	}
	if err := q.CreateJob(ctx, job2); err != nil {
		t.Fatalf("CreateJob(job2) error = %v", err)
	}
	if err := q.CreateJob(ctx, job3); err != nil {
		t.Fatalf("CreateJob(job3) error = %v", err)
	}

	updated, err := q.BatchUpdateJobsEnabled(ctx, []string{job1.ID, job2.ID, newID()}, false)
	if err != nil {
		t.Fatalf("BatchUpdateJobsEnabled() disable error = %v", err)
	}
	if updated != 2 {
		t.Fatalf("BatchUpdateJobsEnabled() disable rows = %d, want 2", updated)
	}

	got1, err := q.GetJob(ctx, job1.ID)
	if err != nil {
		t.Fatalf("GetJob(job1) error = %v", err)
	}
	got2, err := q.GetJob(ctx, job2.ID)
	if err != nil {
		t.Fatalf("GetJob(job2) error = %v", err)
	}
	got3, err := q.GetJob(ctx, job3.ID)
	if err != nil {
		t.Fatalf("GetJob(job3) error = %v", err)
	}
	if got1.Enabled {
		t.Fatal("job1 enabled = true, want false")
	}
	if got2.Enabled {
		t.Fatal("job2 enabled = true, want false")
	}
	if !got3.Enabled {
		t.Fatal("job3 enabled = false, want true")
	}

	updated, err = q.BatchUpdateJobsEnabled(ctx, []string{job2.ID}, true)
	if err != nil {
		t.Fatalf("BatchUpdateJobsEnabled() re-enable error = %v", err)
	}
	if updated != 1 {
		t.Fatalf("BatchUpdateJobsEnabled() re-enable rows = %d, want 1", updated)
	}

	got2, err = q.GetJob(ctx, job2.ID)
	if err != nil {
		t.Fatalf("GetJob(job2) after re-enable error = %v", err)
	}
	if !got2.Enabled {
		t.Fatal("job2 enabled after re-enable = false, want true")
	}

	updated, err = q.BatchUpdateJobsEnabled(ctx, nil, false)
	if err != nil {
		t.Fatalf("BatchUpdateJobsEnabled() empty ids error = %v", err)
	}
	if updated != 0 {
		t.Fatalf("BatchUpdateJobsEnabled() empty ids rows = %d, want 0", updated)
	}
}

func TestWorkflow_CRUD(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-workflow-crud"
	workflow := &domain.Workflow{
		ProjectID:   projectID,
		Name:        "Workflow A",
		Slug:        "workflow-a",
		Description: "initial description",
		Enabled:     true,
	}
	if err := q.CreateWorkflow(ctx, workflow); err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}
	if workflow.ID == "" {
		t.Fatal("CreateWorkflow() did not set ID")
	}
	if workflow.Version != 1 {
		t.Fatalf("CreateWorkflow() version = %d, want 1", workflow.Version)
	}
	if workflow.CreatedAt.IsZero() {
		t.Fatal("CreateWorkflow() did not set CreatedAt")
	}
	if workflow.UpdatedAt.IsZero() {
		t.Fatal("CreateWorkflow() did not set UpdatedAt")
	}

	got, err := q.GetWorkflow(ctx, workflow.ID)
	if err != nil {
		t.Fatalf("GetWorkflow() error = %v", err)
	}
	if got.ID != workflow.ID || got.ProjectID != workflow.ProjectID || got.Name != workflow.Name || got.Slug != workflow.Slug || got.Description != workflow.Description || got.Enabled != workflow.Enabled || got.Version != workflow.Version {
		t.Fatalf("GetWorkflow() mismatch: got %+v want %+v", *got, *workflow)
	}

	bySlug, err := q.GetWorkflowBySlug(ctx, workflow.ProjectID, workflow.Slug)
	if err != nil {
		t.Fatalf("GetWorkflowBySlug() error = %v", err)
	}
	if bySlug.ID != workflow.ID {
		t.Fatalf("GetWorkflowBySlug() id = %q, want %q", bySlug.ID, workflow.ID)
	}

	other := &domain.Workflow{
		ProjectID: projectID,
		Name:      "Workflow B",
		Slug:      "workflow-b",
		Enabled:   false,
	}
	if err := q.CreateWorkflow(ctx, other); err != nil {
		t.Fatalf("CreateWorkflow(other) error = %v", err)
	}

	workflows, err := q.ListWorkflows(ctx, projectID, 100, nil)
	if err != nil {
		t.Fatalf("ListWorkflows() error = %v", err)
	}
	if len(workflows) != 2 {
		t.Fatalf("ListWorkflows() len = %d, want 2", len(workflows))
	}

	workflow.Name = "Workflow A Updated"
	workflow.Slug = "workflow-a-updated"
	workflow.Description = "updated description"
	workflow.Enabled = false
	originalVersion := got.Version
	if err := q.UpdateWorkflow(ctx, workflow); err != nil {
		t.Fatalf("UpdateWorkflow() error = %v", err)
	}
	if workflow.Version != originalVersion+1 {
		t.Fatalf("UpdateWorkflow() version = %d, want %d", workflow.Version, originalVersion+1)
	}

	updated, err := q.GetWorkflow(ctx, workflow.ID)
	if err != nil {
		t.Fatalf("GetWorkflow() after update error = %v", err)
	}
	if updated.Name != workflow.Name || updated.Slug != workflow.Slug || updated.Description != workflow.Description || updated.Enabled != workflow.Enabled || updated.Version != workflow.Version {
		t.Fatalf("updated workflow mismatch: got %+v want %+v", *updated, *workflow)
	}

	if err := q.DeleteWorkflow(ctx, workflow.ID); err != nil {
		t.Fatalf("DeleteWorkflow() error = %v", err)
	}
	if _, err := q.GetWorkflow(ctx, workflow.ID); !errors.Is(err, store.ErrWorkflowNotFound) {
		t.Fatalf("GetWorkflow() after delete error = %v, want ErrWorkflowNotFound", err)
	}

	if _, err := q.GetWorkflow(ctx, newID()); !errors.Is(err, store.ErrWorkflowNotFound) {
		t.Fatalf("GetWorkflow() not found error = %v, want ErrWorkflowNotFound", err)
	}
	if _, err := q.GetWorkflowBySlug(ctx, projectID, "missing-slug"); !errors.Is(err, store.ErrWorkflowNotFound) {
		t.Fatalf("GetWorkflowBySlug() not found error = %v, want ErrWorkflowNotFound", err)
	}

	missing := &domain.Workflow{ID: newID(), ProjectID: projectID, Name: "missing", Slug: "missing"}
	if err := q.UpdateWorkflow(ctx, missing); !errors.Is(err, store.ErrWorkflowNotFound) {
		t.Fatalf("UpdateWorkflow() not found error = %v, want ErrWorkflowNotFound", err)
	}

	empty, err := q.ListWorkflows(ctx, "project-workflow-empty", 100, nil)
	if err != nil {
		t.Fatalf("ListWorkflows() empty project error = %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("ListWorkflows() empty project len = %d, want 0", len(empty))
	}
}

func TestWorkflowStep_CRUD(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-workflow-step-crud"
	job := mustCreateJob(t, ctx, q, projectID)
	workflow := &domain.Workflow{
		ProjectID: projectID,
		Name:      "Step Workflow",
		Slug:      "step-workflow",
		Enabled:   true,
	}
	if err := q.CreateWorkflow(ctx, workflow); err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}

	step := &domain.WorkflowStep{
		WorkflowID: workflow.ID,
		JobID:      job.ID,
		StepRef:    "extract",
		DependsOn:  []string{},
		Condition:  json.RawMessage(`{"type":"step_status","step_ref":"extract","status":"completed"}`),
		Payload:    json.RawMessage(`{"batch":1}`),
	}
	if err := q.CreateWorkflowStep(ctx, step); err != nil {
		t.Fatalf("CreateWorkflowStep() error = %v", err)
	}
	if step.ID == "" {
		t.Fatal("CreateWorkflowStep() did not set ID")
	}
	if step.CreatedAt.IsZero() {
		t.Fatal("CreateWorkflowStep() did not set CreatedAt")
	}
	if step.OnFailure != domain.FailWorkflow {
		t.Fatalf("CreateWorkflowStep() on_failure = %q, want %q", step.OnFailure, domain.FailWorkflow)
	}

	got, err := q.GetWorkflowStep(ctx, step.ID)
	if err != nil {
		t.Fatalf("GetWorkflowStep() error = %v", err)
	}
	if got.ID != step.ID || got.WorkflowID != step.WorkflowID || got.JobID != step.JobID || got.StepRef != step.StepRef || got.OnFailure != step.OnFailure {
		t.Fatalf("GetWorkflowStep() mismatch: got %+v want %+v", *got, *step)
	}
	if !jsonEqual(got.Condition, step.Condition) {
		t.Fatalf("GetWorkflowStep() condition = %s, want %s", string(got.Condition), string(step.Condition))
	}
	if !jsonEqual(got.Payload, step.Payload) {
		t.Fatalf("GetWorkflowStep() payload = %s, want %s", string(got.Payload), string(step.Payload))
	}

	dependent := &domain.WorkflowStep{
		WorkflowID: workflow.ID,
		JobID:      job.ID,
		StepRef:    "transform",
		DependsOn:  []string{"extract"},
		OnFailure:  domain.SkipDependents,
	}
	if err := q.CreateWorkflowStep(ctx, dependent); err != nil {
		t.Fatalf("CreateWorkflowStep(dependent) error = %v", err)
	}

	steps, err := q.ListStepsByWorkflow(ctx, workflow.ID)
	if err != nil {
		t.Fatalf("ListStepsByWorkflow() error = %v", err)
	}
	if len(steps) != 2 {
		t.Fatalf("ListStepsByWorkflow() len = %d, want 2", len(steps))
	}
	if steps[0].ID != step.ID || steps[1].ID != dependent.ID {
		t.Fatalf("ListStepsByWorkflow() ids = [%q, %q], want [%q, %q]", steps[0].ID, steps[1].ID, step.ID, dependent.ID)
	}

	if err := q.DeleteStepsByWorkflow(ctx, workflow.ID); err != nil {
		t.Fatalf("DeleteStepsByWorkflow() error = %v", err)
	}
	steps, err = q.ListStepsByWorkflow(ctx, workflow.ID)
	if err != nil {
		t.Fatalf("ListStepsByWorkflow() after delete error = %v", err)
	}
	if len(steps) != 0 {
		t.Fatalf("ListStepsByWorkflow() after delete len = %d, want 0", len(steps))
	}

	if _, err := q.GetWorkflowStep(ctx, step.ID); !errors.Is(err, store.ErrWorkflowStepNotFound) {
		t.Fatalf("GetWorkflowStep() after delete error = %v, want ErrWorkflowStepNotFound", err)
	}

	empty, err := q.ListStepsByWorkflow(ctx, newID())
	if err != nil {
		t.Fatalf("ListStepsByWorkflow() empty error = %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("ListStepsByWorkflow() empty len = %d, want 0", len(empty))
	}
}

func TestWorkflowRun_CRUD(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-workflow-run-crud"
	workflow := &domain.Workflow{
		ProjectID: projectID,
		Name:      "Run Workflow",
		Slug:      "run-workflow",
		Enabled:   true,
	}
	if err := q.CreateWorkflow(ctx, workflow); err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}

	firstPayload := json.RawMessage(`{"input":"one"}`)
	run1 := &domain.WorkflowRun{
		WorkflowID: workflow.ID,
		ProjectID:  workflow.ProjectID,
		Payload:    firstPayload,
	}
	if err := q.CreateWorkflowRun(ctx, run1); err != nil {
		t.Fatalf("CreateWorkflowRun(run1) error = %v", err)
	}
	if run1.ID == "" {
		t.Fatal("CreateWorkflowRun(run1) did not set ID")
	}
	if run1.Status != domain.WfStatusPending {
		t.Fatalf("CreateWorkflowRun(run1) status = %q, want %q", run1.Status, domain.WfStatusPending)
	}
	if run1.TriggeredBy != domain.TriggerManual {
		t.Fatalf("CreateWorkflowRun(run1) triggered_by = %q, want %q", run1.TriggeredBy, domain.TriggerManual)
	}
	if run1.CreatedAt.IsZero() {
		t.Fatal("CreateWorkflowRun(run1) did not set CreatedAt")
	}

	got1, err := q.GetWorkflowRun(ctx, run1.ID)
	if err != nil {
		t.Fatalf("GetWorkflowRun(run1) error = %v", err)
	}
	if got1.ID != run1.ID || got1.WorkflowID != run1.WorkflowID || got1.ProjectID != run1.ProjectID || got1.Status != run1.Status || got1.TriggeredBy != run1.TriggeredBy {
		t.Fatalf("GetWorkflowRun(run1) mismatch: got %+v want %+v", *got1, *run1)
	}
	if !jsonEqual(got1.Payload, run1.Payload) {
		t.Fatalf("GetWorkflowRun(run1) payload = %s, want %s", string(got1.Payload), string(run1.Payload))
	}

	time.Sleep(5 * time.Millisecond)
	run2 := &domain.WorkflowRun{
		WorkflowID:  workflow.ID,
		ProjectID:   workflow.ProjectID,
		Status:      domain.WfStatusRunning,
		TriggeredBy: domain.TriggerCron,
		Payload:     json.RawMessage(`{"input":"two"}`),
	}
	if err := q.CreateWorkflowRun(ctx, run2); err != nil {
		t.Fatalf("CreateWorkflowRun(run2) error = %v", err)
	}

	time.Sleep(5 * time.Millisecond)
	run3 := &domain.WorkflowRun{
		WorkflowID: workflow.ID,
		ProjectID:  workflow.ProjectID,
		Status:     domain.WfStatusFailed,
		Error:      "boom",
		Payload:    json.RawMessage(`{"input":"three"}`),
	}
	if err := q.CreateWorkflowRun(ctx, run3); err != nil {
		t.Fatalf("CreateWorkflowRun(run3) error = %v", err)
	}

	listed, err := q.ListWorkflowRuns(ctx, workflow.ID, 10, nil)
	if err != nil {
		t.Fatalf("ListWorkflowRuns() error = %v", err)
	}
	if len(listed) != 3 {
		t.Fatalf("ListWorkflowRuns() len = %d, want 3", len(listed))
	}
	if listed[0].ID != run3.ID || listed[1].ID != run2.ID || listed[2].ID != run1.ID {
		t.Fatalf("ListWorkflowRuns() ids = [%q, %q, %q], want [%q, %q, %q]", listed[0].ID, listed[1].ID, listed[2].ID, run3.ID, run2.ID, run1.ID)
	}

	// Cursor-based pagination: use created_at of the first result as cursor to get the next page
	cursor := listed[0].CreatedAt
	paged, err := q.ListWorkflowRuns(ctx, workflow.ID, 1, &cursor)
	if err != nil {
		t.Fatalf("ListWorkflowRuns() paged error = %v", err)
	}
	if len(paged) != 1 {
		t.Fatalf("ListWorkflowRuns() paged len = %d, want 1", len(paged))
	}
	if paged[0].ID != run2.ID {
		t.Fatalf("ListWorkflowRuns() paged id = %q, want %q", paged[0].ID, run2.ID)
	}

	allByProject, err := q.ListWorkflowRunsByProject(ctx, projectID, nil, 10, nil)
	if err != nil {
		t.Fatalf("ListWorkflowRunsByProject(nil status) error = %v", err)
	}
	if len(allByProject) != 3 {
		t.Fatalf("ListWorkflowRunsByProject(nil status) len = %d, want 3", len(allByProject))
	}

	status := domain.WfStatusRunning
	onlyRunning, err := q.ListWorkflowRunsByProject(ctx, projectID, &status, 10, nil)
	if err != nil {
		t.Fatalf("ListWorkflowRunsByProject(running) error = %v", err)
	}
	if len(onlyRunning) != 1 {
		t.Fatalf("ListWorkflowRunsByProject(running) len = %d, want 1", len(onlyRunning))
	}
	if onlyRunning[0].ID != run2.ID {
		t.Fatalf("ListWorkflowRunsByProject(running) id = %q, want %q", onlyRunning[0].ID, run2.ID)
	}

	if _, err := q.GetWorkflowRun(ctx, newID()); !errors.Is(err, store.ErrWorkflowRunNotFound) {
		t.Fatalf("GetWorkflowRun() not found error = %v, want ErrWorkflowRunNotFound", err)
	}
}

func TestWorkflowRun_StatusTransition(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-workflow-run-transition"
	workflow := &domain.Workflow{
		ProjectID: projectID,
		Name:      "Transition Workflow",
		Slug:      "transition-workflow",
		Enabled:   true,
	}
	if err := q.CreateWorkflow(ctx, workflow); err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}

	run := &domain.WorkflowRun{
		WorkflowID: workflow.ID,
		ProjectID:  workflow.ProjectID,
	}
	if err := q.CreateWorkflowRun(ctx, run); err != nil {
		t.Fatalf("CreateWorkflowRun() error = %v", err)
	}

	startedAt := time.Now().UTC().Truncate(time.Microsecond)
	if err := q.UpdateWorkflowRunStatus(ctx, run.ID, domain.WfStatusPending, domain.WfStatusRunning, map[string]any{
		"started_at": startedAt,
	}); err != nil {
		t.Fatalf("UpdateWorkflowRunStatus(pending->running) error = %v", err)
	}

	running, err := q.GetWorkflowRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetWorkflowRun() after running transition error = %v", err)
	}
	if running.Status != domain.WfStatusRunning {
		t.Fatalf("status after running transition = %q, want %q", running.Status, domain.WfStatusRunning)
	}
	if running.StartedAt == nil || !running.StartedAt.Equal(startedAt) {
		t.Fatalf("started_at after running transition = %v, want %v", running.StartedAt, startedAt)
	}

	finishedAt := startedAt.Add(2 * time.Minute)
	if err := q.UpdateWorkflowRunStatus(ctx, run.ID, domain.WfStatusRunning, domain.WfStatusCompleted, map[string]any{
		"finished_at": finishedAt,
	}); err != nil {
		t.Fatalf("UpdateWorkflowRunStatus(running->completed) error = %v", err)
	}

	completed, err := q.GetWorkflowRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetWorkflowRun() after completed transition error = %v", err)
	}
	if completed.Status != domain.WfStatusCompleted {
		t.Fatalf("status after completed transition = %q, want %q", completed.Status, domain.WfStatusCompleted)
	}
	if completed.FinishedAt == nil || !completed.FinishedAt.Equal(finishedAt) {
		t.Fatalf("finished_at after completed transition = %v, want %v", completed.FinishedAt, finishedAt)
	}

	if err := q.UpdateWorkflowRunStatus(ctx, run.ID, domain.WfStatusCompleted, domain.WfStatusRunning, nil); err == nil {
		t.Fatal("UpdateWorkflowRunStatus() invalid transition error = nil, want error")
	}

	if err := q.UpdateWorkflowRunStatus(ctx, run.ID, domain.WfStatusRunning, domain.WfStatusCanceled, nil); err == nil {
		t.Fatal("UpdateWorkflowRunStatus() conflict error = nil, want error")
	}

	if err := q.UpdateWorkflowRunStatus(ctx, run.ID, domain.WfStatusCompleted, domain.WfStatusCompleted, map[string]any{"bad_field": "x"}); err == nil {
		t.Fatal("UpdateWorkflowRunStatus() invalid field error = nil, want error")
	}
}

func TestWorkflowStepRun_CRUD(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-workflow-step-run-crud"
	job := mustCreateJob(t, ctx, q, projectID)
	workflow := &domain.Workflow{
		ProjectID: projectID,
		Name:      "Step Run Workflow",
		Slug:      "step-run-workflow",
		Enabled:   true,
	}
	if err := q.CreateWorkflow(ctx, workflow); err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}

	step := &domain.WorkflowStep{
		WorkflowID: workflow.ID,
		JobID:      job.ID,
		StepRef:    "s1",
		DependsOn:  []string{},
	}
	if err := q.CreateWorkflowStep(ctx, step); err != nil {
		t.Fatalf("CreateWorkflowStep() error = %v", err)
	}

	run := &domain.WorkflowRun{
		WorkflowID: workflow.ID,
		ProjectID:  workflow.ProjectID,
	}
	if err := q.CreateWorkflowRun(ctx, run); err != nil {
		t.Fatalf("CreateWorkflowRun() error = %v", err)
	}

	jobRun := baseRun(job, newID())
	if err := q.CreateRun(ctx, jobRun); err != nil {
		t.Fatalf("CreateRun(jobRun) error = %v", err)
	}

	sr := &domain.WorkflowStepRun{
		WorkflowRunID:  run.ID,
		WorkflowStepID: step.ID,
		StepRef:        step.StepRef,
		DepsCompleted:  0,
		DepsRequired:   0,
	}
	if err := q.CreateWorkflowStepRun(ctx, sr); err != nil {
		t.Fatalf("CreateWorkflowStepRun() error = %v", err)
	}
	if sr.ID == "" {
		t.Fatal("CreateWorkflowStepRun() did not set ID")
	}
	if sr.Status != domain.StepPending {
		t.Fatalf("CreateWorkflowStepRun() status = %q, want %q", sr.Status, domain.StepPending)
	}
	if sr.CreatedAt.IsZero() {
		t.Fatal("CreateWorkflowStepRun() did not set CreatedAt")
	}

	got, err := q.GetWorkflowStepRun(ctx, sr.ID)
	if err != nil {
		t.Fatalf("GetWorkflowStepRun() error = %v", err)
	}
	if got.ID != sr.ID || got.WorkflowRunID != sr.WorkflowRunID || got.WorkflowStepID != sr.WorkflowStepID || got.StepRef != sr.StepRef || got.Status != sr.Status || got.DepsCompleted != sr.DepsCompleted || got.DepsRequired != sr.DepsRequired {
		t.Fatalf("GetWorkflowStepRun() mismatch: got %+v want %+v", *got, *sr)
	}

	nilStepRun, err := q.GetStepRunByJobRunID(ctx, newID())
	if err != nil {
		t.Fatalf("GetStepRunByJobRunID() missing error = %v", err)
	}
	if nilStepRun != nil {
		t.Fatalf("GetStepRunByJobRunID() missing run = %+v, want nil", *nilStepRun)
	}

	startedAt := time.Now().UTC().Truncate(time.Microsecond)
	if err := q.UpdateStepRunStatus(ctx, sr.ID, domain.StepRunning, map[string]any{
		"job_run_id": jobRun.ID,
		"started_at": startedAt,
	}); err != nil {
		t.Fatalf("UpdateStepRunStatus() error = %v", err)
	}

	byJobRunID, err := q.GetStepRunByJobRunID(ctx, jobRun.ID)
	if err != nil {
		t.Fatalf("GetStepRunByJobRunID() error = %v", err)
	}
	if byJobRunID == nil {
		t.Fatal("GetStepRunByJobRunID() returned nil")
	}
	if byJobRunID.ID != sr.ID || byJobRunID.JobRunID != jobRun.ID || byJobRunID.Status != domain.StepRunning {
		t.Fatalf("GetStepRunByJobRunID() mismatch: got %+v", *byJobRunID)
	}
	if byJobRunID.StartedAt == nil || !byJobRunID.StartedAt.Equal(startedAt) {
		t.Fatalf("GetStepRunByJobRunID() started_at = %v, want %v", byJobRunID.StartedAt, startedAt)
	}

	time.Sleep(5 * time.Millisecond)
	second := &domain.WorkflowStepRun{
		WorkflowRunID:  run.ID,
		WorkflowStepID: step.ID,
		StepRef:        "s2",
		Status:         domain.StepWaiting,
		DepsCompleted:  0,
		DepsRequired:   1,
	}
	if err := q.CreateWorkflowStepRun(ctx, second); err != nil {
		t.Fatalf("CreateWorkflowStepRun(second) error = %v", err)
	}

	list, err := q.ListStepRunsByWorkflowRun(ctx, run.ID, 100, nil)
	if err != nil {
		t.Fatalf("ListStepRunsByWorkflowRun() error = %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("ListStepRunsByWorkflowRun() len = %d, want 2", len(list))
	}
	if list[0].ID != sr.ID || list[1].ID != second.ID {
		t.Fatalf("ListStepRunsByWorkflowRun() ids = [%q, %q], want [%q, %q]", list[0].ID, list[1].ID, sr.ID, second.ID)
	}

	if _, err := q.GetWorkflowStepRun(ctx, newID()); !errors.Is(err, store.ErrWorkflowStepRunNotFound) {
		t.Fatalf("GetWorkflowStepRun() not found error = %v, want ErrWorkflowStepRunNotFound", err)
	}

	if err := q.UpdateStepRunStatus(ctx, sr.ID, domain.StepCompleted, map[string]any{"bad_field": "x"}); err == nil {
		t.Fatal("UpdateStepRunStatus() invalid field error = nil, want error")
	}
}

func TestWorkflowStepRun_IncrementDeps(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-workflow-step-run-increment"
	job := mustCreateJob(t, ctx, q, projectID)
	workflow := &domain.Workflow{
		ProjectID: projectID,
		Name:      "Increment Workflow",
		Slug:      "increment-workflow",
		Enabled:   true,
	}
	if err := q.CreateWorkflow(ctx, workflow); err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}

	parent := &domain.WorkflowStep{
		WorkflowID: workflow.ID,
		JobID:      job.ID,
		StepRef:    "extract",
		DependsOn:  []string{},
	}
	if err := q.CreateWorkflowStep(ctx, parent); err != nil {
		t.Fatalf("CreateWorkflowStep(parent) error = %v", err)
	}

	child := &domain.WorkflowStep{
		WorkflowID: workflow.ID,
		JobID:      job.ID,
		StepRef:    "aggregate",
		DependsOn:  []string{"extract"},
		Condition:  json.RawMessage(`{"type":"step_status","step_ref":"extract","status":"completed"}`),
		Payload:    json.RawMessage(`{"kind":"agg"}`),
	}
	if err := q.CreateWorkflowStep(ctx, child); err != nil {
		t.Fatalf("CreateWorkflowStep(child) error = %v", err)
	}

	run := &domain.WorkflowRun{
		WorkflowID: workflow.ID,
		ProjectID:  workflow.ProjectID,
	}
	if err := q.CreateWorkflowRun(ctx, run); err != nil {
		t.Fatalf("CreateWorkflowRun() error = %v", err)
	}

	waiting := &domain.WorkflowStepRun{
		WorkflowRunID:  run.ID,
		WorkflowStepID: child.ID,
		StepRef:        child.StepRef,
		Status:         domain.StepWaiting,
		DepsCompleted:  0,
		DepsRequired:   2,
	}
	if err := q.CreateWorkflowStepRun(ctx, waiting); err != nil {
		t.Fatalf("CreateWorkflowStepRun(waiting) error = %v", err)
	}

	first, err := q.IncrementStepDeps(ctx, run.ID, parent.StepRef)
	if err != nil {
		t.Fatalf("IncrementStepDeps() first error = %v", err)
	}
	if len(first) != 1 {
		t.Fatalf("IncrementStepDeps() first len = %d, want 1", len(first))
	}
	if first[0].StepRunID != waiting.ID || first[0].StepRef != waiting.StepRef || first[0].DepsCompleted != 1 || first[0].DepsRequired != 2 || first[0].JobID != child.JobID || first[0].WorkflowRunID != run.ID {
		t.Fatalf("IncrementStepDeps() first result mismatch: got %+v", first[0])
	}
	if !jsonEqual(first[0].Condition, child.Condition) {
		t.Fatalf("IncrementStepDeps() first condition = %s, want %s", string(first[0].Condition), string(child.Condition))
	}
	if !jsonEqual(first[0].Payload, child.Payload) {
		t.Fatalf("IncrementStepDeps() first payload = %s, want %s", string(first[0].Payload), string(child.Payload))
	}

	second, err := q.IncrementStepDeps(ctx, run.ID, parent.StepRef)
	if err != nil {
		t.Fatalf("IncrementStepDeps() second error = %v", err)
	}
	if len(second) != 1 {
		t.Fatalf("IncrementStepDeps() second len = %d, want 1", len(second))
	}
	if second[0].DepsCompleted != 2 {
		t.Fatalf("IncrementStepDeps() second deps_completed = %d, want 2", second[0].DepsCompleted)
	}

	stored, err := q.GetWorkflowStepRun(ctx, waiting.ID)
	if err != nil {
		t.Fatalf("GetWorkflowStepRun() after increments error = %v", err)
	}
	if stored.DepsCompleted != 2 {
		t.Fatalf("GetWorkflowStepRun() deps_completed = %d, want 2", stored.DepsCompleted)
	}

	none, err := q.IncrementStepDeps(ctx, run.ID, "missing-ref")
	if err != nil {
		t.Fatalf("IncrementStepDeps() missing ref error = %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("IncrementStepDeps() missing ref len = %d, want 0", len(none))
	}
}

func TestWorkflowStepRun_GetOutputs(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-workflow-step-run-outputs"
	job := mustCreateJob(t, ctx, q, projectID)
	workflow := &domain.Workflow{
		ProjectID: projectID,
		Name:      "Output Workflow",
		Slug:      "output-workflow",
		Enabled:   true,
	}
	if err := q.CreateWorkflow(ctx, workflow); err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}

	stepA := &domain.WorkflowStep{WorkflowID: workflow.ID, JobID: job.ID, StepRef: "a", DependsOn: []string{}}
	if err := q.CreateWorkflowStep(ctx, stepA); err != nil {
		t.Fatalf("CreateWorkflowStep(stepA) error = %v", err)
	}
	stepB := &domain.WorkflowStep{WorkflowID: workflow.ID, JobID: job.ID, StepRef: "b", DependsOn: []string{}}
	if err := q.CreateWorkflowStep(ctx, stepB); err != nil {
		t.Fatalf("CreateWorkflowStep(stepB) error = %v", err)
	}
	stepC := &domain.WorkflowStep{WorkflowID: workflow.ID, JobID: job.ID, StepRef: "c", DependsOn: []string{}}
	if err := q.CreateWorkflowStep(ctx, stepC); err != nil {
		t.Fatalf("CreateWorkflowStep(stepC) error = %v", err)
	}

	run := &domain.WorkflowRun{
		WorkflowID: workflow.ID,
		ProjectID:  workflow.ProjectID,
	}
	if err := q.CreateWorkflowRun(ctx, run); err != nil {
		t.Fatalf("CreateWorkflowRun() error = %v", err)
	}

	outA := json.RawMessage(`{"value":"A"}`)
	outB := json.RawMessage(`{"value":"B"}`)

	srA := &domain.WorkflowStepRun{
		WorkflowRunID:  run.ID,
		WorkflowStepID: stepA.ID,
		StepRef:        stepA.StepRef,
		Status:         domain.StepCompleted,
		Output:         outA,
	}
	if err := q.CreateWorkflowStepRun(ctx, srA); err != nil {
		t.Fatalf("CreateWorkflowStepRun(srA) error = %v", err)
	}

	srB := &domain.WorkflowStepRun{
		WorkflowRunID:  run.ID,
		WorkflowStepID: stepB.ID,
		StepRef:        stepB.StepRef,
		Status:         domain.StepCompleted,
		Output:         outB,
	}
	if err := q.CreateWorkflowStepRun(ctx, srB); err != nil {
		t.Fatalf("CreateWorkflowStepRun(srB) error = %v", err)
	}

	srC := &domain.WorkflowStepRun{
		WorkflowRunID:  run.ID,
		WorkflowStepID: stepC.ID,
		StepRef:        stepC.StepRef,
		Status:         domain.StepRunning,
	}
	if err := q.CreateWorkflowStepRun(ctx, srC); err != nil {
		t.Fatalf("CreateWorkflowStepRun(srC) error = %v", err)
	}

	outputs, err := q.GetStepOutputs(ctx, run.ID, []string{"a", "b", "c", "missing"})
	if err != nil {
		t.Fatalf("GetStepOutputs() error = %v", err)
	}
	if len(outputs) != 2 {
		t.Fatalf("GetStepOutputs() len = %d, want 2", len(outputs))
	}
	if !jsonEqual(outputs["a"], outA) {
		t.Fatalf("GetStepOutputs()[a] = %s, want %s", string(outputs["a"]), string(outA))
	}
	if !jsonEqual(outputs["b"], outB) {
		t.Fatalf("GetStepOutputs()[b] = %s, want %s", string(outputs["b"]), string(outB))
	}
	if _, ok := outputs["c"]; ok {
		t.Fatalf("GetStepOutputs()[c] present = true, want false")
	}

	empty, err := q.GetStepOutputs(ctx, run.ID, []string{"missing"})
	if err != nil {
		t.Fatalf("GetStepOutputs() empty error = %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("GetStepOutputs() empty len = %d, want 0", len(empty))
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

	testutil.AssertEqual(t, got, want, testutil.IgnoreFields(domain.Job{}, "PayloadSchema"))

	if !jsonEqual(got.PayloadSchema, want.PayloadSchema) {
		t.Fatalf("payload_schema mismatch: got %s want %s", string(got.PayloadSchema), string(want.PayloadSchema))
	}
}

func assertRunEqual(t *testing.T, want, got *domain.JobRun) {
	t.Helper()

	if got == nil {
		t.Fatal("run is nil")
	}

	testutil.AssertEqual(t, got, want, testutil.IgnoreFields(domain.JobRun{}, "Payload", "Result"), testutil.EquateEmpty())

	if len(got.Payload) != 0 || len(want.Payload) != 0 {
		testutil.AssertJSONEqual(t, got.Payload, want.Payload)
	}
	if len(got.Result) != 0 || len(want.Result) != 0 {
		testutil.AssertJSONEqual(t, got.Result, want.Result)
	}

	assertTimePtrEqual(t, "scheduled_at", want.ScheduledAt, got.ScheduledAt)
	assertTimePtrEqual(t, "started_at", want.StartedAt, got.StartedAt)
	assertTimePtrEqual(t, "finished_at", want.FinishedAt, got.FinishedAt)
	assertTimePtrEqual(t, "heartbeat_at", want.HeartbeatAt, got.HeartbeatAt)
	assertTimePtrEqual(t, "next_retry_at", want.NextRetryAt, got.NextRetryAt)
	assertTimePtrEqual(t, "expires_at", want.ExpiresAt, got.ExpiresAt)

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

func TestQuota_CountProjectQueuedRuns(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-quota-queued")

	queued := baseRun(job, newID())
	queued.Status = domain.StatusQueued
	if err := q.CreateRun(ctx, queued); err != nil {
		t.Fatalf("CreateRun() queued error = %v", err)
	}

	delayed := baseRun(job, newID())
	delayed.Status = domain.StatusDelayed
	scheduled := time.Now().UTC().Add(2 * time.Minute)
	delayed.ScheduledAt = &scheduled
	if err := q.CreateRun(ctx, delayed); err != nil {
		t.Fatalf("CreateRun() delayed error = %v", err)
	}

	executing := baseRun(job, newID())
	executing.Status = domain.StatusExecuting
	if err := q.CreateRun(ctx, executing); err != nil {
		t.Fatalf("CreateRun() executing error = %v", err)
	}

	otherJob := mustCreateJob(t, ctx, q, "project-quota-queued-other")
	otherQueued := baseRun(otherJob, newID())
	otherQueued.Status = domain.StatusQueued
	if err := q.CreateRun(ctx, otherQueued); err != nil {
		t.Fatalf("CreateRun() other queued error = %v", err)
	}

	count, err := q.CountProjectQueuedRuns(ctx, job.ProjectID)
	if err != nil {
		t.Fatalf("CountProjectQueuedRuns() error = %v", err)
	}
	if count != 2 {
		t.Fatalf("CountProjectQueuedRuns() = %d, want 2", count)
	}
}

func TestQuota_CountProjectActiveRuns(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-quota-active")

	dequeued := baseRun(job, newID())
	dequeued.Status = domain.StatusDequeued
	started := time.Now().UTC().Add(-1 * time.Minute)
	dequeued.StartedAt = &started
	if err := q.CreateRun(ctx, dequeued); err != nil {
		t.Fatalf("CreateRun() dequeued error = %v", err)
	}

	executing := baseRun(job, newID())
	executing.Status = domain.StatusExecuting
	heartbeat := time.Now().UTC()
	executing.HeartbeatAt = &heartbeat
	if err := q.CreateRun(ctx, executing); err != nil {
		t.Fatalf("CreateRun() executing error = %v", err)
	}

	queued := baseRun(job, newID())
	queued.Status = domain.StatusQueued
	if err := q.CreateRun(ctx, queued); err != nil {
		t.Fatalf("CreateRun() queued error = %v", err)
	}

	otherJob := mustCreateJob(t, ctx, q, "project-quota-active-other")
	otherExecuting := baseRun(otherJob, newID())
	otherExecuting.Status = domain.StatusExecuting
	if err := q.CreateRun(ctx, otherExecuting); err != nil {
		t.Fatalf("CreateRun() other executing error = %v", err)
	}

	count, err := q.CountProjectActiveRuns(ctx, job.ProjectID)
	if err != nil {
		t.Fatalf("CountProjectActiveRuns() error = %v", err)
	}
	if count != 2 {
		t.Fatalf("CountProjectActiveRuns() = %d, want 2", count)
	}
}

func TestQuota_GetProjectQuota(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-quota-get"
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO project_quotas (project_id, max_queued_runs, max_executing_runs, max_jobs, timezone)
		VALUES ($1, $2, $3, $4, $5)
	`, projectID, 15, 7, 40, "America/New_York"); err != nil {
		t.Fatalf("insert project_quotas error = %v", err)
	}

	quota, err := q.GetProjectQuota(ctx, projectID)
	if err != nil {
		t.Fatalf("GetProjectQuota() error = %v", err)
	}
	if quota == nil {
		t.Fatal("GetProjectQuota() = nil, want quota")
	}
	if quota.ProjectID != projectID || quota.MaxQueuedRuns != 15 || quota.MaxExecutingRuns != 7 || quota.MaxJobs != 40 || quota.Timezone != "America/New_York" {
		t.Fatalf("GetProjectQuota() = %+v, want expected values", *quota)
	}

	missing, err := q.GetProjectQuota(ctx, "project-quota-missing")
	if err != nil {
		t.Fatalf("GetProjectQuota() missing error = %v", err)
	}
	if missing != nil {
		t.Fatalf("GetProjectQuota() missing = %+v, want nil", *missing)
	}
}

func TestRunMgmt_ListStaleRuns(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-list-stale-runs")

	stale := baseRun(job, newID())
	stale.Status = domain.StatusExecuting
	if err := q.CreateRun(ctx, stale); err != nil {
		t.Fatalf("CreateRun() stale error = %v", err)
	}

	fresh := baseRun(job, newID())
	fresh.Status = domain.StatusExecuting
	if err := q.CreateRun(ctx, fresh); err != nil {
		t.Fatalf("CreateRun() fresh error = %v", err)
	}

	queued := baseRun(job, newID())
	queued.Status = domain.StatusQueued
	if err := q.CreateRun(ctx, queued); err != nil {
		t.Fatalf("CreateRun() queued error = %v", err)
	}

	oldHeartbeat := time.Now().UTC().Add(-10 * time.Minute)
	if _, err := testDB.Pool.Exec(ctx, "UPDATE job_runs SET heartbeat_at = $1 WHERE id = $2", oldHeartbeat, stale.ID); err != nil {
		t.Fatalf("update stale heartbeat error = %v", err)
	}
	recentHeartbeat := time.Now().UTC().Add(-1 * time.Minute)
	if _, err := testDB.Pool.Exec(ctx, "UPDATE job_runs SET heartbeat_at = $1 WHERE id = $2", recentHeartbeat, fresh.ID); err != nil {
		t.Fatalf("update fresh heartbeat error = %v", err)
	}

	runs, err := q.ListStaleRuns(ctx, 5*time.Minute)
	if err != nil {
		t.Fatalf("ListStaleRuns() error = %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("ListStaleRuns() len = %d, want 1", len(runs))
	}
	if runs[0].ID != stale.ID {
		t.Fatalf("ListStaleRuns() run ID = %q, want %q", runs[0].ID, stale.ID)
	}
}

func TestRunMgmt_ListDueRuns(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-list-due-runs")

	due := baseRun(job, newID())
	due.Status = domain.StatusDelayed
	dueAt := time.Now().UTC().Add(-10 * time.Minute)
	due.ScheduledAt = &dueAt
	if err := q.CreateRun(ctx, due); err != nil {
		t.Fatalf("CreateRun() due error = %v", err)
	}

	notDue := baseRun(job, newID())
	notDue.Status = domain.StatusDelayed
	notDueAt := time.Now().UTC().Add(10 * time.Minute)
	notDue.ScheduledAt = &notDueAt
	if err := q.CreateRun(ctx, notDue); err != nil {
		t.Fatalf("CreateRun() notDue error = %v", err)
	}

	queuedPast := baseRun(job, newID())
	queuedPast.Status = domain.StatusQueued
	queuedPastAt := time.Now().UTC().Add(-20 * time.Minute)
	queuedPast.ScheduledAt = &queuedPastAt
	if err := q.CreateRun(ctx, queuedPast); err != nil {
		t.Fatalf("CreateRun() queuedPast error = %v", err)
	}

	runs, err := q.ListDueRuns(ctx)
	if err != nil {
		t.Fatalf("ListDueRuns() error = %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("ListDueRuns() len = %d, want 1", len(runs))
	}
	if runs[0].ID != due.ID {
		t.Fatalf("ListDueRuns() run ID = %q, want %q", runs[0].ID, due.ID)
	}
}

func TestRunMgmt_ListExpiredRuns(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-list-expired-runs")

	past := time.Now().UTC().Add(-10 * time.Minute)
	future := time.Now().UTC().Add(10 * time.Minute)

	expiredDelayed := baseRun(job, newID())
	expiredDelayed.Status = domain.StatusDelayed
	expiredDelayed.ExpiresAt = &past
	if err := q.CreateRun(ctx, expiredDelayed); err != nil {
		t.Fatalf("CreateRun() expiredDelayed error = %v", err)
	}

	expiredQueued := baseRun(job, newID())
	expiredQueued.Status = domain.StatusQueued
	expiredQueued.ExpiresAt = &past
	if err := q.CreateRun(ctx, expiredQueued); err != nil {
		t.Fatalf("CreateRun() expiredQueued error = %v", err)
	}

	notExpiredQueued := baseRun(job, newID())
	notExpiredQueued.Status = domain.StatusQueued
	notExpiredQueued.ExpiresAt = &future
	if err := q.CreateRun(ctx, notExpiredQueued); err != nil {
		t.Fatalf("CreateRun() notExpiredQueued error = %v", err)
	}

	expiredExecuting := baseRun(job, newID())
	expiredExecuting.Status = domain.StatusExecuting
	expiredExecuting.ExpiresAt = &past
	if err := q.CreateRun(ctx, expiredExecuting); err != nil {
		t.Fatalf("CreateRun() expiredExecuting error = %v", err)
	}

	runs, err := q.ListExpiredRuns(ctx)
	if err != nil {
		t.Fatalf("ListExpiredRuns() error = %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("ListExpiredRuns() len = %d, want 2", len(runs))
	}

	got := map[string]bool{}
	for _, run := range runs {
		got[run.ID] = true
	}
	if !got[expiredDelayed.ID] || !got[expiredQueued.ID] {
		t.Fatalf("ListExpiredRuns() IDs = %+v, want %q and %q", got, expiredDelayed.ID, expiredQueued.ID)
	}
}

func TestRunMgmt_ListStaleDequeued(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-list-stale-dequeued")

	stale := baseRun(job, newID())
	stale.Status = domain.StatusDequeued
	if err := q.CreateRun(ctx, stale); err != nil {
		t.Fatalf("CreateRun() stale error = %v", err)
	}

	fresh := baseRun(job, newID())
	fresh.Status = domain.StatusDequeued
	if err := q.CreateRun(ctx, fresh); err != nil {
		t.Fatalf("CreateRun() fresh error = %v", err)
	}

	executing := baseRun(job, newID())
	executing.Status = domain.StatusExecuting
	if err := q.CreateRun(ctx, executing); err != nil {
		t.Fatalf("CreateRun() executing error = %v", err)
	}

	oldStartedAt := time.Now().UTC().Add(-10 * time.Minute)
	if _, err := testDB.Pool.Exec(ctx, "UPDATE job_runs SET started_at = $1 WHERE id = $2", oldStartedAt, stale.ID); err != nil {
		t.Fatalf("update stale started_at error = %v", err)
	}
	recentStartedAt := time.Now().UTC().Add(-1 * time.Minute)
	if _, err := testDB.Pool.Exec(ctx, "UPDATE job_runs SET started_at = $1 WHERE id = $2", recentStartedAt, fresh.ID); err != nil {
		t.Fatalf("update fresh started_at error = %v", err)
	}

	runs, err := q.ListStaleDequeued(ctx, 5*time.Minute)
	if err != nil {
		t.Fatalf("ListStaleDequeued() error = %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("ListStaleDequeued() len = %d, want 1", len(runs))
	}
	if runs[0].ID != stale.ID {
		t.Fatalf("ListStaleDequeued() run ID = %q, want %q", runs[0].ID, stale.ID)
	}
}

func TestAnalytics_FindRecentRunByPayload(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-find-recent-payload")

	matchingPayload := json.RawMessage(`{"kind":"sync","count":2}`)
	nonMatchingPayload := json.RawMessage(`{"kind":"sync","count":3}`)
	since := time.Now().UTC().Add(-30 * time.Minute)

	oldMatch := baseRun(job, newID())
	oldMatch.Payload = matchingPayload
	if err := q.CreateRun(ctx, oldMatch); err != nil {
		t.Fatalf("CreateRun() oldMatch error = %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, "UPDATE job_runs SET created_at = $1 WHERE id = $2", since.Add(-10*time.Minute), oldMatch.ID); err != nil {
		t.Fatalf("update oldMatch created_at error = %v", err)
	}

	recentMatch := baseRun(job, newID())
	recentMatch.Payload = matchingPayload
	if err := q.CreateRun(ctx, recentMatch); err != nil {
		t.Fatalf("CreateRun() recentMatch error = %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, "UPDATE job_runs SET created_at = $1 WHERE id = $2", since.Add(5*time.Minute), recentMatch.ID); err != nil {
		t.Fatalf("update recentMatch created_at error = %v", err)
	}

	newestMatch := baseRun(job, newID())
	newestMatch.Payload = matchingPayload
	if err := q.CreateRun(ctx, newestMatch); err != nil {
		t.Fatalf("CreateRun() newestMatch error = %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, "UPDATE job_runs SET created_at = $1 WHERE id = $2", since.Add(10*time.Minute), newestMatch.ID); err != nil {
		t.Fatalf("update newestMatch created_at error = %v", err)
	}

	nonMatch := baseRun(job, newID())
	nonMatch.Payload = nonMatchingPayload
	if err := q.CreateRun(ctx, nonMatch); err != nil {
		t.Fatalf("CreateRun() nonMatch error = %v", err)
	}

	got, err := q.FindRecentRunByPayload(ctx, job.ID, matchingPayload, since)
	if err != nil {
		t.Fatalf("FindRecentRunByPayload() error = %v", err)
	}
	if got == nil {
		t.Fatal("FindRecentRunByPayload() = nil, want run")
	}
	if got.ID != newestMatch.ID {
		t.Fatalf("FindRecentRunByPayload() ID = %q, want %q", got.ID, newestMatch.ID)
	}

	missing, err := q.FindRecentRunByPayload(ctx, job.ID, json.RawMessage(`{"kind":"other"}`), since)
	if err != nil {
		t.Fatalf("FindRecentRunByPayload() missing error = %v", err)
	}
	if missing != nil {
		t.Fatalf("FindRecentRunByPayload() missing = %+v, want nil", *missing)
	}
}

func TestAnalytics_CountRunsForJobSince(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-count-runs-since")
	otherJob := mustCreateJob(t, ctx, q, "project-count-runs-since-other")
	since := time.Now().UTC().Add(-15 * time.Minute)

	oldRun := baseRun(job, newID())
	if err := q.CreateRun(ctx, oldRun); err != nil {
		t.Fatalf("CreateRun() oldRun error = %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, "UPDATE job_runs SET created_at = $1 WHERE id = $2", since.Add(-1*time.Minute), oldRun.ID); err != nil {
		t.Fatalf("update oldRun created_at error = %v", err)
	}

	recentRun1 := baseRun(job, newID())
	if err := q.CreateRun(ctx, recentRun1); err != nil {
		t.Fatalf("CreateRun() recentRun1 error = %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, "UPDATE job_runs SET created_at = $1 WHERE id = $2", since.Add(1*time.Minute), recentRun1.ID); err != nil {
		t.Fatalf("update recentRun1 created_at error = %v", err)
	}

	recentRun2 := baseRun(job, newID())
	if err := q.CreateRun(ctx, recentRun2); err != nil {
		t.Fatalf("CreateRun() recentRun2 error = %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, "UPDATE job_runs SET created_at = $1 WHERE id = $2", since.Add(2*time.Minute), recentRun2.ID); err != nil {
		t.Fatalf("update recentRun2 created_at error = %v", err)
	}

	otherJobRun := baseRun(otherJob, newID())
	if err := q.CreateRun(ctx, otherJobRun); err != nil {
		t.Fatalf("CreateRun() otherJobRun error = %v", err)
	}

	count, err := q.CountRunsForJobSince(ctx, job.ID, since)
	if err != nil {
		t.Fatalf("CountRunsForJobSince() error = %v", err)
	}
	if count != 2 {
		t.Fatalf("CountRunsForJobSince() = %d, want 2", count)
	}
}

func TestEvents_ListEventsByRunFiltered(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-events-filtered")
	run := mustCreateRun(t, ctx, q, job)
	otherRun := mustCreateRun(t, ctx, q, job)

	events := []*domain.RunEvent{
		{ID: newID(), RunID: run.ID, Type: domain.EventLog, Level: "info", Message: "info-log", Data: json.RawMessage(`{"idx":1}`)},
		{ID: newID(), RunID: run.ID, Type: domain.EventLog, Level: "error", Message: "error-log", Data: json.RawMessage(`{"idx":2}`)},
		{ID: newID(), RunID: run.ID, Type: domain.EventStateChange, Level: "info", Message: "state-change", Data: json.RawMessage(`{"idx":3}`)},
		{ID: newID(), RunID: otherRun.ID, Type: domain.EventLog, Level: "info", Message: "other-run", Data: json.RawMessage(`{"idx":4}`)},
	}

	for i := range events {
		if err := q.InsertEvent(ctx, events[i]); err != nil {
			t.Fatalf("InsertEvent() event %d error = %v", i, err)
		}
	}

	allForRun, err := q.ListEventsByRunFiltered(ctx, run.ID, "", "", 100, nil)
	if err != nil {
		t.Fatalf("ListEventsByRunFiltered() all error = %v", err)
	}
	if len(allForRun) != 3 {
		t.Fatalf("ListEventsByRunFiltered() all len = %d, want 3", len(allForRun))
	}

	infoOnly, err := q.ListEventsByRunFiltered(ctx, run.ID, "info", "", 100, nil)
	if err != nil {
		t.Fatalf("ListEventsByRunFiltered() level error = %v", err)
	}
	if len(infoOnly) != 2 {
		t.Fatalf("ListEventsByRunFiltered() level len = %d, want 2", len(infoOnly))
	}

	logOnly, err := q.ListEventsByRunFiltered(ctx, run.ID, "", string(domain.EventLog), 100, nil)
	if err != nil {
		t.Fatalf("ListEventsByRunFiltered() type error = %v", err)
	}
	if len(logOnly) != 2 {
		t.Fatalf("ListEventsByRunFiltered() type len = %d, want 2", len(logOnly))
	}

	infoLogs, err := q.ListEventsByRunFiltered(ctx, run.ID, "info", string(domain.EventLog), 100, nil)
	if err != nil {
		t.Fatalf("ListEventsByRunFiltered() level+type error = %v", err)
	}
	if len(infoLogs) != 1 {
		t.Fatalf("ListEventsByRunFiltered() level+type len = %d, want 1", len(infoLogs))
	}
	if infoLogs[0].ID != events[0].ID {
		t.Fatalf("ListEventsByRunFiltered() level+type ID = %q, want %q", infoLogs[0].ID, events[0].ID)
	}
}

// ============ Phase 8: Tests for previously untested store methods ============

func TestWorkflowRunLabels_CRUD(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-wf-run-labels"
	wfID := newID()
	wf := &domain.Workflow{ID: wfID, ProjectID: projectID, Name: "wf-labels", Slug: "wf-labels-slug", Enabled: true, Version: 1}
	if err := q.CreateWorkflow(ctx, wf); err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}

	wfRunID := newID()
	wfRun := &domain.WorkflowRun{ID: wfRunID, WorkflowID: wfID, ProjectID: projectID, Status: domain.WfStatusPending, TriggeredBy: "manual"}
	if err := q.CreateWorkflowRun(ctx, wfRun); err != nil {
		t.Fatalf("CreateWorkflowRun() error = %v", err)
	}

	// Create labels
	labels := map[string]string{"env": "staging", "team": "backend"}
	if err := q.CreateWorkflowRunLabels(ctx, wfRunID, labels); err != nil {
		t.Fatalf("CreateWorkflowRunLabels() error = %v", err)
	}

	// List labels
	got, err := q.ListWorkflowRunLabels(ctx, wfRunID)
	if err != nil {
		t.Fatalf("ListWorkflowRunLabels() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListWorkflowRunLabels() len = %d, want 2", len(got))
	}
	if got["env"] != "staging" {
		t.Fatalf("label env = %q, want staging", got["env"])
	}
	if got["team"] != "backend" {
		t.Fatalf("label team = %q, want backend", got["team"])
	}

	// Empty labels noop
	if err := q.CreateWorkflowRunLabels(ctx, wfRunID, map[string]string{}); err != nil {
		t.Fatalf("CreateWorkflowRunLabels(empty) error = %v", err)
	}

	// Upsert
	if err := q.CreateWorkflowRunLabels(ctx, wfRunID, map[string]string{"env": "production"}); err != nil {
		t.Fatalf("CreateWorkflowRunLabels(upsert) error = %v", err)
	}
	got2, _ := q.ListWorkflowRunLabels(ctx, wfRunID)
	if got2["env"] != "production" {
		t.Fatalf("label env after upsert = %q, want production", got2["env"])
	}

	// Empty for unknown run
	empty, err := q.ListWorkflowRunLabels(ctx, newID())
	if err != nil {
		t.Fatalf("ListWorkflowRunLabels(unknown) error = %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("ListWorkflowRunLabels(unknown) len = %d, want 0", len(empty))
	}
}

func TestWorkflowStepApproval_CRUD(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-step-approval"
	job := mustCreateJob(t, ctx, q, projectID)
	wf := &domain.Workflow{ID: newID(), ProjectID: projectID, Name: "wf-approval", Slug: "wf-approval-slug", Enabled: true, Version: 1}
	if err := q.CreateWorkflow(ctx, wf); err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}
	step := &domain.WorkflowStep{ID: newID(), WorkflowID: wf.ID, JobID: job.ID, StepRef: "approval-step"}
	if err := q.CreateWorkflowStep(ctx, step); err != nil {
		t.Fatalf("CreateWorkflowStep() error = %v", err)
	}
	wfRun := &domain.WorkflowRun{ID: newID(), WorkflowID: wf.ID, ProjectID: projectID, Status: domain.WfStatusRunning, TriggeredBy: "manual"}
	if err := q.CreateWorkflowRun(ctx, wfRun); err != nil {
		t.Fatalf("CreateWorkflowRun() error = %v", err)
	}
	stepRun := &domain.WorkflowStepRun{ID: newID(), WorkflowRunID: wfRun.ID, WorkflowStepID: step.ID, StepRef: "approval-step", Status: domain.StepPending}
	if err := q.CreateWorkflowStepRun(ctx, stepRun); err != nil {
		t.Fatalf("CreateWorkflowStepRun() error = %v", err)
	}

	// Create approval
	now := time.Now().UTC().Truncate(time.Microsecond)
	expires := now.Add(1 * time.Hour)
	approval := &domain.WorkflowStepApproval{
		ID:                newID(),
		WorkflowRunID:     wfRun.ID,
		WorkflowStepRunID: stepRun.ID,
		Approvers:         []string{"alice", "bob"},
		Status:            "pending",
		RequestedAt:       now,
		ExpiresAt:         &expires,
	}
	if err := q.CreateWorkflowStepApproval(ctx, approval); err != nil {
		t.Fatalf("CreateWorkflowStepApproval() error = %v", err)
	}

	// Get by step run ID
	got, err := q.GetWorkflowStepApprovalByStepRunID(ctx, stepRun.ID)
	if err != nil {
		t.Fatalf("GetWorkflowStepApprovalByStepRunID() error = %v", err)
	}
	if got.ID != approval.ID {
		t.Fatalf("approval ID = %q, want %q", got.ID, approval.ID)
	}
	if got.Status != "pending" {
		t.Fatalf("approval status = %q, want pending", got.Status)
	}
	if len(got.Approvers) != 2 {
		t.Fatalf("approval approvers len = %d, want 2", len(got.Approvers))
	}

	// Update approval
	approvedAt := now.Add(5 * time.Minute)
	if err := q.UpdateWorkflowStepApproval(ctx, approval.ID, "approved", "alice", &approvedAt, ""); err != nil {
		t.Fatalf("UpdateWorkflowStepApproval() error = %v", err)
	}
	updated, _ := q.GetWorkflowStepApprovalByStepRunID(ctx, stepRun.ID)
	if updated.Status != "approved" {
		t.Fatalf("updated status = %q, want approved", updated.Status)
	}
	if updated.ApprovedBy != "alice" {
		t.Fatalf("updated approved_by = %q, want alice", updated.ApprovedBy)
	}

	// Update not found
	if err := q.UpdateWorkflowStepApproval(ctx, newID(), "approved", "bob", &approvedAt, ""); err == nil {
		t.Fatal("UpdateWorkflowStepApproval(unknown) error = nil, want error")
	}
}

func TestListExpiredWorkflowStepApprovals(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-expired-approvals"
	job := mustCreateJob(t, ctx, q, projectID)
	wf := &domain.Workflow{ID: newID(), ProjectID: projectID, Name: "wf-expired", Slug: "wf-expired-slug", Enabled: true, Version: 1}
	if err := q.CreateWorkflow(ctx, wf); err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}
	step := &domain.WorkflowStep{ID: newID(), WorkflowID: wf.ID, JobID: job.ID, StepRef: "exp-step"}
	if err := q.CreateWorkflowStep(ctx, step); err != nil {
		t.Fatalf("CreateWorkflowStep() error = %v", err)
	}
	wfRun := &domain.WorkflowRun{ID: newID(), WorkflowID: wf.ID, ProjectID: projectID, Status: domain.WfStatusRunning, TriggeredBy: "manual"}
	if err := q.CreateWorkflowRun(ctx, wfRun); err != nil {
		t.Fatalf("CreateWorkflowRun() error = %v", err)
	}

	// Create two step runs
	sr1 := &domain.WorkflowStepRun{ID: newID(), WorkflowRunID: wfRun.ID, WorkflowStepID: step.ID, StepRef: "exp-step-1", Status: domain.StepPending}
	if err := q.CreateWorkflowStepRun(ctx, sr1); err != nil {
		t.Fatalf("CreateWorkflowStepRun(1) error = %v", err)
	}
	sr2 := &domain.WorkflowStepRun{ID: newID(), WorkflowRunID: wfRun.ID, WorkflowStepID: step.ID, StepRef: "exp-step-2", Status: domain.StepPending}
	if err := q.CreateWorkflowStepRun(ctx, sr2); err != nil {
		t.Fatalf("CreateWorkflowStepRun(2) error = %v", err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	pastExpiry := now.Add(-1 * time.Hour)
	futureExpiry := now.Add(1 * time.Hour)

	// Expired pending approval
	a1 := &domain.WorkflowStepApproval{ID: newID(), WorkflowRunID: wfRun.ID, WorkflowStepRunID: sr1.ID, Approvers: []string{"alice"}, Status: "pending", RequestedAt: now, ExpiresAt: &pastExpiry}
	if err := q.CreateWorkflowStepApproval(ctx, a1); err != nil {
		t.Fatalf("CreateWorkflowStepApproval(expired) error = %v", err)
	}

	// Not-expired pending approval
	a2 := &domain.WorkflowStepApproval{ID: newID(), WorkflowRunID: wfRun.ID, WorkflowStepRunID: sr2.ID, Approvers: []string{"bob"}, Status: "pending", RequestedAt: now, ExpiresAt: &futureExpiry}
	if err := q.CreateWorkflowStepApproval(ctx, a2); err != nil {
		t.Fatalf("CreateWorkflowStepApproval(future) error = %v", err)
	}

	expired, err := q.ListExpiredWorkflowStepApprovals(ctx)
	if err != nil {
		t.Fatalf("ListExpiredWorkflowStepApprovals() error = %v", err)
	}
	if len(expired) != 1 {
		t.Fatalf("ListExpiredWorkflowStepApprovals() len = %d, want 1", len(expired))
	}
	if expired[0].ID != a1.ID {
		t.Fatalf("expired approval ID = %q, want %q", expired[0].ID, a1.ID)
	}
}

func TestCountRunningWorkflowRuns(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-count-running-wfruns"
	wf := &domain.Workflow{ID: newID(), ProjectID: projectID, Name: "wf-count", Slug: "wf-count-slug", Enabled: true, Version: 1}
	if err := q.CreateWorkflow(ctx, wf); err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}

	// No runs yet
	count, err := q.CountRunningWorkflowRuns(ctx, wf.ID)
	if err != nil {
		t.Fatalf("CountRunningWorkflowRuns() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0", count)
	}

	// Create pending run (not running)
	wfRun1 := &domain.WorkflowRun{ID: newID(), WorkflowID: wf.ID, ProjectID: projectID, Status: domain.WfStatusPending, TriggeredBy: "manual"}
	if err := q.CreateWorkflowRun(ctx, wfRun1); err != nil {
		t.Fatalf("CreateWorkflowRun(pending) error = %v", err)
	}

	// Create running run
	wfRun2 := &domain.WorkflowRun{ID: newID(), WorkflowID: wf.ID, ProjectID: projectID, Status: domain.WfStatusPending, TriggeredBy: "manual"}
	if err := q.CreateWorkflowRun(ctx, wfRun2); err != nil {
		t.Fatalf("CreateWorkflowRun(running) error = %v", err)
	}
	if err := q.UpdateWorkflowRunStatus(ctx, wfRun2.ID, domain.WfStatusPending, domain.WfStatusRunning, nil); err != nil {
		t.Fatalf("UpdateWorkflowRunStatus() error = %v", err)
	}

	count, err = q.CountRunningWorkflowRuns(ctx, wf.ID)
	if err != nil {
		t.Fatalf("CountRunningWorkflowRuns() after running error = %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}
}

func TestGetStepRunByWorkflowRunAndRef(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-step-run-by-ref"
	job := mustCreateJob(t, ctx, q, projectID)
	wf := &domain.Workflow{ID: newID(), ProjectID: projectID, Name: "wf-ref", Slug: "wf-ref-slug", Enabled: true, Version: 1}
	if err := q.CreateWorkflow(ctx, wf); err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}
	step := &domain.WorkflowStep{ID: newID(), WorkflowID: wf.ID, JobID: job.ID, StepRef: "my-step"}
	if err := q.CreateWorkflowStep(ctx, step); err != nil {
		t.Fatalf("CreateWorkflowStep() error = %v", err)
	}
	wfRun := &domain.WorkflowRun{ID: newID(), WorkflowID: wf.ID, ProjectID: projectID, Status: domain.WfStatusRunning, TriggeredBy: "manual"}
	if err := q.CreateWorkflowRun(ctx, wfRun); err != nil {
		t.Fatalf("CreateWorkflowRun() error = %v", err)
	}
	sr := &domain.WorkflowStepRun{ID: newID(), WorkflowRunID: wfRun.ID, WorkflowStepID: step.ID, StepRef: "my-step", Status: domain.StepPending}
	if err := q.CreateWorkflowStepRun(ctx, sr); err != nil {
		t.Fatalf("CreateWorkflowStepRun() error = %v", err)
	}

	got, err := q.GetStepRunByWorkflowRunAndRef(ctx, wfRun.ID, "my-step")
	if err != nil {
		t.Fatalf("GetStepRunByWorkflowRunAndRef() error = %v", err)
	}
	if got.ID != sr.ID {
		t.Fatalf("step run ID = %q, want %q", got.ID, sr.ID)
	}
	if got.StepRef != "my-step" {
		t.Fatalf("step run ref = %q, want my-step", got.StepRef)
	}
}

func TestDeleteWorkflowRunsFinishedBefore(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-delete-old-wfruns"
	wf := &domain.Workflow{ID: newID(), ProjectID: projectID, Name: "wf-delete", Slug: "wf-delete-slug", Enabled: true, Version: 1}
	if err := q.CreateWorkflow(ctx, wf); err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}

	// Create a completed run with finished_at in the past
	wfRun := &domain.WorkflowRun{ID: newID(), WorkflowID: wf.ID, ProjectID: projectID, Status: domain.WfStatusPending, TriggeredBy: "manual"}
	if err := q.CreateWorkflowRun(ctx, wfRun); err != nil {
		t.Fatalf("CreateWorkflowRun() error = %v", err)
	}
	if err := q.UpdateWorkflowRunStatus(ctx, wfRun.ID, domain.WfStatusPending, domain.WfStatusRunning, nil); err != nil {
		t.Fatalf("UpdateWorkflowRunStatus(running) error = %v", err)
	}
	oldTime := time.Now().UTC().Add(-48 * time.Hour)
	if err := q.UpdateWorkflowRunStatus(ctx, wfRun.ID, domain.WfStatusRunning, domain.WfStatusCompleted, map[string]any{"finished_at": oldTime}); err != nil {
		t.Fatalf("UpdateWorkflowRunStatus(completed) error = %v", err)
	}

	// Create a recent running run (should not be deleted)
	wfRun2 := &domain.WorkflowRun{ID: newID(), WorkflowID: wf.ID, ProjectID: projectID, Status: domain.WfStatusPending, TriggeredBy: "manual"}
	if err := q.CreateWorkflowRun(ctx, wfRun2); err != nil {
		t.Fatalf("CreateWorkflowRun(recent) error = %v", err)
	}
	if err := q.UpdateWorkflowRunStatus(ctx, wfRun2.ID, domain.WfStatusPending, domain.WfStatusRunning, nil); err != nil {
		t.Fatalf("UpdateWorkflowRunStatus(recent) error = %v", err)
	}

	deleted, err := q.DeleteWorkflowRunsFinishedBefore(ctx, time.Now().UTC().Add(-24*time.Hour), 100)
	if err != nil {
		t.Fatalf("DeleteWorkflowRunsFinishedBefore() error = %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}

	// Verify the completed run is gone
	_, err = q.GetWorkflowRun(ctx, wfRun.ID)
	if !errors.Is(err, store.ErrWorkflowRunNotFound) {
		t.Fatalf("GetWorkflowRun(deleted) error = %v, want ErrWorkflowRunNotFound", err)
	}

	// Verify the running run still exists
	_, err = q.GetWorkflowRun(ctx, wfRun2.ID)
	if err != nil {
		t.Fatalf("GetWorkflowRun(running) error = %v", err)
	}
}

func TestGetWorkflowRunsByParent(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-wfrun-parent"
	wf := &domain.Workflow{ID: newID(), ProjectID: projectID, Name: "wf-parent", Slug: "wf-parent-slug", Enabled: true, Version: 1}
	if err := q.CreateWorkflow(ctx, wf); err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}

	parentRun := &domain.WorkflowRun{ID: newID(), WorkflowID: wf.ID, ProjectID: projectID, Status: domain.WfStatusRunning, TriggeredBy: "manual"}
	if err := q.CreateWorkflowRun(ctx, parentRun); err != nil {
		t.Fatalf("CreateWorkflowRun(parent) error = %v", err)
	}

	childRun := &domain.WorkflowRun{ID: newID(), WorkflowID: wf.ID, ProjectID: projectID, Status: domain.WfStatusPending, TriggeredBy: "workflow"}
	if err := q.CreateWorkflowRun(ctx, childRun); err != nil {
		t.Fatalf("CreateWorkflowRun(child) error = %v", err)
	}
	_, err := testDB.Pool.Exec(ctx, `UPDATE workflow_runs SET parent_workflow_run_id = $1 WHERE id = $2`, parentRun.ID, childRun.ID)
	if err != nil {
		t.Fatalf("set parent_workflow_run_id error = %v", err)
	}

	children, err := q.GetWorkflowRunsByParent(ctx, parentRun.ID)
	if err != nil {
		t.Fatalf("GetWorkflowRunsByParent() error = %v", err)
	}
	if len(children) != 1 {
		t.Fatalf("children len = %d, want 1", len(children))
	}
	if children[0].ID != childRun.ID {
		t.Fatalf("child ID = %q, want %q", children[0].ID, childRun.ID)
	}

	// Empty for unknown parent
	empty, err := q.GetWorkflowRunsByParent(ctx, newID())
	if err != nil {
		t.Fatalf("GetWorkflowRunsByParent(unknown) error = %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("GetWorkflowRunsByParent(unknown) len = %d, want 0", len(empty))
	}
}

func TestListDeadLetterRuns(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-dead-letter"
	job := mustCreateJob(t, ctx, q, projectID)
	run := mustCreateRun(t, ctx, q, job)

	// Transition run to dead_letter: queued -> dequeued -> executing -> dead_letter
	if err := q.UpdateRunStatus(ctx, run.ID, domain.StatusQueued, domain.StatusDequeued, nil); err != nil {
		t.Fatalf("UpdateRunStatus(dequeued) error = %v", err)
	}
	if err := q.UpdateRunStatus(ctx, run.ID, domain.StatusDequeued, domain.StatusExecuting, nil); err != nil {
		t.Fatalf("UpdateRunStatus(executing) error = %v", err)
	}
	if err := q.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusDeadLetter, nil); err != nil {
		t.Fatalf("UpdateRunStatus(dead_letter) error = %v", err)
	}

	// Create another non-dead-letter run
	run2 := mustCreateRun(t, ctx, q, job)
	_ = run2

	runs, err := q.ListDeadLetterRuns(ctx, projectID, 50, nil)
	if err != nil {
		t.Fatalf("ListDeadLetterRuns() error = %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("ListDeadLetterRuns() len = %d, want 1", len(runs))
	}
	if runs[0].ID != run.ID {
		t.Fatalf("dead letter run ID = %q, want %q", runs[0].ID, run.ID)
	}
	if runs[0].Status != domain.StatusDeadLetter {
		t.Fatalf("dead letter run status = %q, want dead_letter", runs[0].Status)
	}
}

func TestReplayDeadLetterRun(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-replay-dead-letter"
	job := mustCreateJob(t, ctx, q, projectID)
	run := mustCreateRun(t, ctx, q, job)

	// Transition to dead_letter
	if err := q.UpdateRunStatus(ctx, run.ID, domain.StatusQueued, domain.StatusDequeued, nil); err != nil {
		t.Fatalf("UpdateRunStatus(dequeued) error = %v", err)
	}
	if err := q.UpdateRunStatus(ctx, run.ID, domain.StatusDequeued, domain.StatusExecuting, nil); err != nil {
		t.Fatalf("UpdateRunStatus(executing) error = %v", err)
	}
	if err := q.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusDeadLetter, nil); err != nil {
		t.Fatalf("UpdateRunStatus(dead_letter) error = %v", err)
	}

	// Replay
	replayed, err := q.ReplayDeadLetterRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("ReplayDeadLetterRun() error = %v", err)
	}
	if replayed.Status != domain.StatusQueued {
		t.Fatalf("replayed status = %q, want queued", replayed.Status)
	}
	if replayed.Attempt != 1 {
		t.Fatalf("replayed attempt = %d, want 1", replayed.Attempt)
	}

	// Replay non-dead-letter run should fail
	run2 := mustCreateRun(t, ctx, q, job)
	_, err = q.ReplayDeadLetterRun(ctx, run2.ID)
	if err == nil {
		t.Fatal("ReplayDeadLetterRun(non-dead-letter) error = nil, want error")
	}
}

func TestSumRunCostMicrousd(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-sum-cost")
	run := mustCreateRun(t, ctx, q, job)

	// No usage yet
	total, err := q.SumRunCostMicrousd(ctx, run.ID)
	if err != nil {
		t.Fatalf("SumRunCostMicrousd() error = %v", err)
	}
	if total != 0 {
		t.Fatalf("total = %d, want 0", total)
	}

	// Add usage records
	u1 := &domain.RunUsage{ID: newID(), RunID: run.ID, Model: "gpt-4", PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150, CostMicrousd: 1000, Provider: "openai"}
	if err := q.CreateRunUsage(ctx, u1); err != nil {
		t.Fatalf("CreateRunUsage(1) error = %v", err)
	}
	u2 := &domain.RunUsage{ID: newID(), RunID: run.ID, Model: "gpt-4", PromptTokens: 200, CompletionTokens: 100, TotalTokens: 300, CostMicrousd: 2500, Provider: "openai"}
	if err := q.CreateRunUsage(ctx, u2); err != nil {
		t.Fatalf("CreateRunUsage(2) error = %v", err)
	}

	total, err = q.SumRunCostMicrousd(ctx, run.ID)
	if err != nil {
		t.Fatalf("SumRunCostMicrousd() after usage error = %v", err)
	}
	if total != 3500 {
		t.Fatalf("total = %d, want 3500", total)
	}
}

func TestSumProjectDailyCostMicrousd(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-daily-cost"
	job := mustCreateJob(t, ctx, q, projectID)
	run := mustCreateRun(t, ctx, q, job)

	u := &domain.RunUsage{ID: newID(), RunID: run.ID, Model: "gpt-4", PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150, CostMicrousd: 5000, Provider: "openai"}
	if err := q.CreateRunUsage(ctx, u); err != nil {
		t.Fatalf("CreateRunUsage() error = %v", err)
	}

	total, err := q.SumProjectDailyCostMicrousd(ctx, projectID, "UTC")
	if err != nil {
		t.Fatalf("SumProjectDailyCostMicrousd() error = %v", err)
	}
	if total != 5000 {
		t.Fatalf("total = %d, want 5000", total)
	}

	// Different project should have 0
	emptyTotal, err := q.SumProjectDailyCostMicrousd(ctx, "project-nonexistent", "UTC")
	if err != nil {
		t.Fatalf("SumProjectDailyCostMicrousd(empty) error = %v", err)
	}
	if emptyTotal != 0 {
		t.Fatalf("emptyTotal = %d, want 0", emptyTotal)
	}
}

func TestIncrementStepRunAttempt(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-increment-attempt"
	job := mustCreateJob(t, ctx, q, projectID)
	wf := &domain.Workflow{ID: newID(), ProjectID: projectID, Name: "wf-inc", Slug: "wf-inc-slug", Enabled: true, Version: 1}
	if err := q.CreateWorkflow(ctx, wf); err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}
	step := &domain.WorkflowStep{ID: newID(), WorkflowID: wf.ID, JobID: job.ID, StepRef: "inc-step"}
	if err := q.CreateWorkflowStep(ctx, step); err != nil {
		t.Fatalf("CreateWorkflowStep() error = %v", err)
	}
	wfRun := &domain.WorkflowRun{ID: newID(), WorkflowID: wf.ID, ProjectID: projectID, Status: domain.WfStatusRunning, TriggeredBy: "manual"}
	if err := q.CreateWorkflowRun(ctx, wfRun); err != nil {
		t.Fatalf("CreateWorkflowRun() error = %v", err)
	}
	sr := &domain.WorkflowStepRun{ID: newID(), WorkflowRunID: wfRun.ID, WorkflowStepID: step.ID, StepRef: "inc-step", Status: domain.StepPending}
	if err := q.CreateWorkflowStepRun(ctx, sr); err != nil {
		t.Fatalf("CreateWorkflowStepRun() error = %v", err)
	}

	// Attempt starts at 1 (default set by CreateWorkflowStepRun), increment to 2
	if err := q.IncrementStepRunAttempt(ctx, sr.ID, 2); err != nil {
		t.Fatalf("IncrementStepRunAttempt(1->2) error = %v", err)
	}

	// Increment to 3
	if err := q.IncrementStepRunAttempt(ctx, sr.ID, 3); err != nil {
		t.Fatalf("IncrementStepRunAttempt(2->3) error = %v", err)
	}

	// Optimistic lock: trying to increment to 3 again should fail (current is 3, expects 2)
	if err := q.IncrementStepRunAttempt(ctx, sr.ID, 3); err == nil {
		t.Fatal("IncrementStepRunAttempt(stale) error = nil, want error")
	}

	// Unknown step run should fail
	if err := q.IncrementStepRunAttempt(ctx, newID(), 1); err == nil {
		t.Fatal("IncrementStepRunAttempt(unknown) error = nil, want error")
	}
}

func TestListCronWorkflows(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-cron-workflows"

	// Create a workflow with cron
	wf1 := &domain.Workflow{ID: newID(), ProjectID: projectID, Name: "wf-cron", Slug: "wf-cron-slug", Enabled: true, Version: 1}
	if err := q.CreateWorkflow(ctx, wf1); err != nil {
		t.Fatalf("CreateWorkflow(cron) error = %v", err)
	}
	_, err := testDB.Pool.Exec(ctx, `UPDATE workflows SET cron = $1 WHERE id = $2`, "0 * * * *", wf1.ID)
	if err != nil {
		t.Fatalf("set cron error = %v", err)
	}

	// Create a workflow without cron
	wf2 := &domain.Workflow{ID: newID(), ProjectID: projectID, Name: "wf-no-cron", Slug: "wf-no-cron-slug", Enabled: true, Version: 1}
	if err := q.CreateWorkflow(ctx, wf2); err != nil {
		t.Fatalf("CreateWorkflow(no-cron) error = %v", err)
	}

	// Create a disabled workflow with cron
	wf3 := &domain.Workflow{ID: newID(), ProjectID: projectID, Name: "wf-disabled-cron", Slug: "wf-disabled-cron-slug", Enabled: false, Version: 1}
	if err := q.CreateWorkflow(ctx, wf3); err != nil {
		t.Fatalf("CreateWorkflow(disabled) error = %v", err)
	}
	_, err = testDB.Pool.Exec(ctx, `UPDATE workflows SET cron = $1 WHERE id = $2`, "*/5 * * * *", wf3.ID)
	if err != nil {
		t.Fatalf("set cron disabled error = %v", err)
	}

	cronWfs, err := q.ListCronWorkflows(ctx)
	if err != nil {
		t.Fatalf("ListCronWorkflows() error = %v", err)
	}
	if len(cronWfs) != 1 {
		t.Fatalf("ListCronWorkflows() len = %d, want 1", len(cronWfs))
	}
	if cronWfs[0].ID != wf1.ID {
		t.Fatalf("cron workflow ID = %q, want %q", cronWfs[0].ID, wf1.ID)
	}
}

func TestListRunLineage(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-run-lineage"
	job := mustCreateJob(t, ctx, q, projectID)

	// Create a chain: run1 -> run2 -> run3
	run1 := mustCreateRun(t, ctx, q, job)
	run2 := mustCreateRun(t, ctx, q, job)
	run3 := mustCreateRun(t, ctx, q, job)

	_, err := testDB.Pool.Exec(ctx, `UPDATE job_runs SET continuation_of = $1, lineage_depth = $2 WHERE id = $3`, run1.ID, 1, run2.ID)
	if err != nil {
		t.Fatalf("set continuation_of run2 error = %v", err)
	}
	_, err = testDB.Pool.Exec(ctx, `UPDATE job_runs SET continuation_of = $1, lineage_depth = $2 WHERE id = $3`, run2.ID, 2, run3.ID)
	if err != nil {
		t.Fatalf("set continuation_of run3 error = %v", err)
	}

	// Query lineage from run3 (should walk back to run1 and return all 3)
	lineage, err := q.ListRunLineage(ctx, run3.ID, 100, nil)
	if err != nil {
		t.Fatalf("ListRunLineage() error = %v", err)
	}
	if len(lineage) != 3 {
		t.Fatalf("ListRunLineage() len = %d, want 3", len(lineage))
	}
	// Should be ordered by lineage_depth ASC (run1 first)
	if lineage[0].ID != run1.ID {
		t.Fatalf("lineage[0].ID = %q, want %q (root)", lineage[0].ID, run1.ID)
	}
	if lineage[2].ID != run3.ID {
		t.Fatalf("lineage[2].ID = %q, want %q (leaf)", lineage[2].ID, run3.ID)
	}
}

// ============================================================================
// Phase 3-5: Store integration tests for untested methods + edge cases
// ============================================================================

func TestGetDebugBundle(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-debug-bundle"
	job := mustCreateJob(t, ctx, q, projectID)
	run := mustCreateRun(t, ctx, q, job)

	// Insert an event
	event := &domain.RunEvent{ID: newID(), RunID: run.ID, Type: domain.EventLog, Level: "info", Message: "hello", Data: json.RawMessage(`{"key":"value"}`)}
	if err := q.InsertEvent(ctx, event); err != nil {
		t.Fatalf("InsertEvent() error = %v", err)
	}

	// Insert a checkpoint
	cp := &domain.RunCheckpoint{RunID: run.ID, Source: "sdk", State: json.RawMessage(`{"step":1}`)}
	if err := q.CreateRunCheckpoint(ctx, cp); err != nil {
		t.Fatalf("CreateRunCheckpoint() error = %v", err)
	}

	// Insert a usage record
	usage := &domain.RunUsage{ID: newID(), RunID: run.ID, Provider: "openai", Model: "gpt-4", PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15, CostMicrousd: 100}
	if err := q.CreateRunUsage(ctx, usage); err != nil {
		t.Fatalf("CreateRunUsage() error = %v", err)
	}

	// Insert a tool call
	tc := &domain.RunToolCall{ID: newID(), RunID: run.ID, ToolName: "search", Input: json.RawMessage(`{"q":"test"}`), Output: json.RawMessage(`{"r":"ok"}`), DurationMs: 42, Status: "completed"}
	if err := q.CreateRunToolCall(ctx, tc); err != nil {
		t.Fatalf("CreateRunToolCall() error = %v", err)
	}

	// Insert an output
	out := &domain.RunOutput{ID: newID(), RunID: run.ID, OutputKey: "result", Value: json.RawMessage(`{"v":1}`)}
	if err := q.UpsertRunOutput(ctx, out); err != nil {
		t.Fatalf("UpsertRunOutput() error = %v", err)
	}

	bundle, err := q.GetDebugBundle(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetDebugBundle() error = %v", err)
	}
	if bundle.Run == nil {
		t.Fatal("GetDebugBundle() run is nil")
	}
	if bundle.Run.ID != run.ID {
		t.Fatalf("bundle.Run.ID = %q, want %q", bundle.Run.ID, run.ID)
	}
	if len(bundle.Events) != 1 {
		t.Fatalf("bundle.Events len = %d, want 1", len(bundle.Events))
	}
	if len(bundle.Checkpoints) != 1 {
		t.Fatalf("bundle.Checkpoints len = %d, want 1", len(bundle.Checkpoints))
	}
	if len(bundle.Usage) != 1 {
		t.Fatalf("bundle.Usage len = %d, want 1", len(bundle.Usage))
	}
	if len(bundle.ToolCalls) != 1 {
		t.Fatalf("bundle.ToolCalls len = %d, want 1", len(bundle.ToolCalls))
	}
	if len(bundle.Outputs) != 1 {
		t.Fatalf("bundle.Outputs len = %d, want 1", len(bundle.Outputs))
	}

	// Nonexistent run
	_, err = q.GetDebugBundle(ctx, newID())
	if !errors.Is(err, store.ErrRunNotFound) {
		t.Fatalf("GetDebugBundle(unknown) error = %v, want ErrRunNotFound", err)
	}
}

func TestGetDebugBundle_EmptyCollections(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-debug-bundle-empty")
	run := mustCreateRun(t, ctx, q, job)

	bundle, err := q.GetDebugBundle(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetDebugBundle() error = %v", err)
	}
	if bundle.Run == nil {
		t.Fatal("bundle.Run is nil")
	}
	if bundle.Events == nil {
		t.Fatal("bundle.Events is nil, want empty slice")
	}
	if len(bundle.Events) != 0 {
		t.Fatalf("bundle.Events len = %d, want 0", len(bundle.Events))
	}
	if bundle.Checkpoints == nil {
		t.Fatal("bundle.Checkpoints is nil, want empty slice")
	}
	if len(bundle.Checkpoints) != 0 {
		t.Fatalf("bundle.Checkpoints len = %d, want 0", len(bundle.Checkpoints))
	}
	if bundle.Usage == nil {
		t.Fatal("bundle.Usage is nil, want empty slice")
	}
	if len(bundle.Usage) != 0 {
		t.Fatalf("bundle.Usage len = %d, want 0", len(bundle.Usage))
	}
	if bundle.ToolCalls == nil {
		t.Fatal("bundle.ToolCalls is nil, want empty slice")
	}
	if len(bundle.ToolCalls) != 0 {
		t.Fatalf("bundle.ToolCalls len = %d, want 0", len(bundle.ToolCalls))
	}
	if bundle.Outputs == nil {
		t.Fatal("bundle.Outputs is nil, want empty slice")
	}
	if len(bundle.Outputs) != 0 {
		t.Fatalf("bundle.Outputs len = %d, want 0", len(bundle.Outputs))
	}
}

func TestUpdateRunDebugMode(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-debug-mode")
	run := mustCreateRun(t, ctx, q, job)

	// Initially false
	got, err := q.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if got.DebugMode {
		t.Fatal("initial debug_mode = true, want false")
	}

	// Enable debug mode
	if err := q.UpdateRunDebugMode(ctx, run.ID, true); err != nil {
		t.Fatalf("UpdateRunDebugMode(true) error = %v", err)
	}
	got, err = q.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() after enable error = %v", err)
	}
	if !got.DebugMode {
		t.Fatal("debug_mode = false after enable, want true")
	}

	// Disable debug mode
	if err := q.UpdateRunDebugMode(ctx, run.ID, false); err != nil {
		t.Fatalf("UpdateRunDebugMode(false) error = %v", err)
	}
	got, err = q.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() after disable error = %v", err)
	}
	if got.DebugMode {
		t.Fatal("debug_mode = true after disable, want false")
	}

	// Nonexistent run
	err = q.UpdateRunDebugMode(ctx, newID(), true)
	if !errors.Is(err, store.ErrRunNotFound) {
		t.Fatalf("UpdateRunDebugMode(unknown) error = %v, want ErrRunNotFound", err)
	}
}

func TestCreateWorkflowVersionSnapshot(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-wf-version-snapshot"
	job := mustCreateJob(t, ctx, q, projectID)

	// Create workflow
	wf := &domain.Workflow{ID: newID(), ProjectID: projectID, Name: "wf-snap", Slug: "wf-snap-slug", Enabled: true, Version: 1}
	if err := q.CreateWorkflow(ctx, wf); err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}

	// Add steps
	step1 := &domain.WorkflowStep{ID: newID(), WorkflowID: wf.ID, JobID: job.ID, StepRef: "step-a"}
	if err := q.CreateWorkflowStep(ctx, step1); err != nil {
		t.Fatalf("CreateWorkflowStep(a) error = %v", err)
	}
	step2 := &domain.WorkflowStep{ID: newID(), WorkflowID: wf.ID, JobID: job.ID, StepRef: "step-b", DependsOn: []string{"step-a"}}
	if err := q.CreateWorkflowStep(ctx, step2); err != nil {
		t.Fatalf("CreateWorkflowStep(b) error = %v", err)
	}

	// Snapshot version 1
	if err := q.CreateWorkflowVersionSnapshot(ctx, wf.ID, 1); err != nil {
		t.Fatalf("CreateWorkflowVersionSnapshot(v1) error = %v", err)
	}

	// List steps by version 1
	steps, err := q.ListStepsByWorkflowVersion(ctx, wf.ID, 1)
	if err != nil {
		t.Fatalf("ListStepsByWorkflowVersion(v1) error = %v", err)
	}
	if len(steps) != 2 {
		t.Fatalf("ListStepsByWorkflowVersion(v1) len = %d, want 2", len(steps))
	}

	// Verify step refs
	refs := make(map[string]bool)
	for _, s := range steps {
		refs[s.StepRef] = true
	}
	if !refs["step-a"] || !refs["step-b"] {
		t.Fatalf("expected step-a and step-b, got refs: %v", refs)
	}

	// Snapshot nonexistent workflow
	err = q.CreateWorkflowVersionSnapshot(ctx, newID(), 1)
	if !errors.Is(err, store.ErrWorkflowNotFound) {
		t.Fatalf("CreateWorkflowVersionSnapshot(unknown) error = %v, want ErrWorkflowNotFound", err)
	}
}

func TestListStepsByWorkflowVersion_Empty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	// No snapshot exists for this workflow/version
	steps, err := q.ListStepsByWorkflowVersion(ctx, newID(), 99)
	if err != nil {
		t.Fatalf("ListStepsByWorkflowVersion(empty) error = %v", err)
	}
	if len(steps) != 0 {
		t.Fatalf("ListStepsByWorkflowVersion(empty) len = %d, want 0", len(steps))
	}
}

func TestWorkflowVersionSnapshot_MultipleVersions(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-wf-multi-version"
	job := mustCreateJob(t, ctx, q, projectID)

	wf := &domain.Workflow{ID: newID(), ProjectID: projectID, Name: "wf-multi", Slug: "wf-multi-slug", Enabled: true, Version: 1}
	if err := q.CreateWorkflow(ctx, wf); err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}

	// V1: one step
	step1 := &domain.WorkflowStep{ID: newID(), WorkflowID: wf.ID, JobID: job.ID, StepRef: "only-step-v1"}
	if err := q.CreateWorkflowStep(ctx, step1); err != nil {
		t.Fatalf("CreateWorkflowStep(v1) error = %v", err)
	}
	if err := q.CreateWorkflowVersionSnapshot(ctx, wf.ID, 1); err != nil {
		t.Fatalf("CreateWorkflowVersionSnapshot(v1) error = %v", err)
	}

	// Update workflow to v2
	wf.Name = "wf-multi-v2"
	if err := q.UpdateWorkflow(ctx, wf); err != nil {
		t.Fatalf("UpdateWorkflow() error = %v", err)
	}
	if wf.Version != 2 {
		t.Fatalf("wf.Version = %d, want 2", wf.Version)
	}

	// Add a second step for v2
	step2 := &domain.WorkflowStep{ID: newID(), WorkflowID: wf.ID, JobID: job.ID, StepRef: "new-step-v2"}
	if err := q.CreateWorkflowStep(ctx, step2); err != nil {
		t.Fatalf("CreateWorkflowStep(v2) error = %v", err)
	}
	if err := q.CreateWorkflowVersionSnapshot(ctx, wf.ID, 2); err != nil {
		t.Fatalf("CreateWorkflowVersionSnapshot(v2) error = %v", err)
	}

	// V1 should have 1 step
	v1Steps, err := q.ListStepsByWorkflowVersion(ctx, wf.ID, 1)
	if err != nil {
		t.Fatalf("ListStepsByWorkflowVersion(v1) error = %v", err)
	}
	if len(v1Steps) != 1 {
		t.Fatalf("v1 steps len = %d, want 1", len(v1Steps))
	}
	if v1Steps[0].StepRef != "only-step-v1" {
		t.Fatalf("v1 step ref = %q, want only-step-v1", v1Steps[0].StepRef)
	}

	// V2 should have 2 steps
	v2Steps, err := q.ListStepsByWorkflowVersion(ctx, wf.ID, 2)
	if err != nil {
		t.Fatalf("ListStepsByWorkflowVersion(v2) error = %v", err)
	}
	if len(v2Steps) != 2 {
		t.Fatalf("v2 steps len = %d, want 2", len(v2Steps))
	}
}

func TestCreateRunUsage_Dedicated(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-usage-dedicated")
	run := mustCreateRun(t, ctx, q, job)

	u1 := &domain.RunUsage{ID: newID(), RunID: run.ID, Provider: "openai", Model: "gpt-4", PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150, CostMicrousd: 1000}
	if err := q.CreateRunUsage(ctx, u1); err != nil {
		t.Fatalf("CreateRunUsage(1) error = %v", err)
	}
	if u1.CreatedAt.IsZero() {
		t.Fatal("u1.CreatedAt is zero")
	}

	u2 := &domain.RunUsage{ID: newID(), RunID: run.ID, Provider: "anthropic", Model: "claude-3", PromptTokens: 200, CompletionTokens: 100, TotalTokens: 300, CostMicrousd: 2000}
	if err := q.CreateRunUsage(ctx, u2); err != nil {
		t.Fatalf("CreateRunUsage(2) error = %v", err)
	}

	u3 := &domain.RunUsage{ID: newID(), RunID: run.ID, Provider: "openai", Model: "gpt-3.5", PromptTokens: 50, CompletionTokens: 25, TotalTokens: 75, CostMicrousd: 500}
	if err := q.CreateRunUsage(ctx, u3); err != nil {
		t.Fatalf("CreateRunUsage(3) error = %v", err)
	}

	usages, err := q.ListRunUsage(ctx, run.ID, 100, nil)
	if err != nil {
		t.Fatalf("ListRunUsage() error = %v", err)
	}
	if len(usages) != 3 {
		t.Fatalf("ListRunUsage() len = %d, want 3", len(usages))
	}

	// Verify DESC ordering by created_at
	for i := 1; i < len(usages); i++ {
		if usages[i].CreatedAt.After(usages[i-1].CreatedAt) {
			t.Fatalf("usages not DESC at index %d", i)
		}
	}

	// Verify field values (most recent first = u3)
	if usages[0].Provider != "openai" || usages[0].Model != "gpt-3.5" {
		t.Fatalf("usages[0] = %s/%s, want openai/gpt-3.5", usages[0].Provider, usages[0].Model)
	}
	if usages[0].PromptTokens != 50 || usages[0].CompletionTokens != 25 || usages[0].TotalTokens != 75 {
		t.Fatalf("usages[0] tokens = %d/%d/%d, want 50/25/75", usages[0].PromptTokens, usages[0].CompletionTokens, usages[0].TotalTokens)
	}
	if usages[0].CostMicrousd != 500 {
		t.Fatalf("usages[0] cost = %d, want 500", usages[0].CostMicrousd)
	}

	// Empty for unknown run
	emptyUsages, err := q.ListRunUsage(ctx, newID(), 100, nil)
	if err != nil {
		t.Fatalf("ListRunUsage(unknown) error = %v", err)
	}
	if len(emptyUsages) != 0 {
		t.Fatalf("ListRunUsage(unknown) len = %d, want 0", len(emptyUsages))
	}
}

func TestRunUsage_Pagination(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-usage-pagination")
	run := mustCreateRun(t, ctx, q, job)

	// Create 5 usage records
	for i := 0; i < 5; i++ {
		u := &domain.RunUsage{ID: newID(), RunID: run.ID, Provider: "openai", Model: "gpt-4", PromptTokens: i + 1, CompletionTokens: 1, TotalTokens: i + 2, CostMicrousd: int64((i + 1) * 100)}
		if err := q.CreateRunUsage(ctx, u); err != nil {
			t.Fatalf("CreateRunUsage(%d) error = %v", i, err)
		}
	}

	// First page
	page1, err := q.ListRunUsage(ctx, run.ID, 2, nil)
	if err != nil {
		t.Fatalf("ListRunUsage(page1) error = %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("page1 len = %d, want 2", len(page1))
	}

	// Second page using cursor
	cursor := page1[len(page1)-1].CreatedAt
	page2, err := q.ListRunUsage(ctx, run.ID, 2, &cursor)
	if err != nil {
		t.Fatalf("ListRunUsage(page2) error = %v", err)
	}
	if len(page2) != 2 {
		t.Fatalf("page2 len = %d, want 2", len(page2))
	}

	// Ensure no overlap
	for _, p1 := range page1 {
		for _, p2 := range page2 {
			if p1.ID == p2.ID {
				t.Fatalf("overlap between page1 and page2: %s", p1.ID)
			}
		}
	}

	// Third page
	cursor2 := page2[len(page2)-1].CreatedAt
	page3, err := q.ListRunUsage(ctx, run.ID, 2, &cursor2)
	if err != nil {
		t.Fatalf("ListRunUsage(page3) error = %v", err)
	}
	if len(page3) != 1 {
		t.Fatalf("page3 len = %d, want 1", len(page3))
	}

	// Total unique IDs = 5
	allIDs := make(map[string]bool)
	for _, u := range page1 {
		allIDs[u.ID] = true
	}
	for _, u := range page2 {
		allIDs[u.ID] = true
	}
	for _, u := range page3 {
		allIDs[u.ID] = true
	}
	if len(allIDs) != 5 {
		t.Fatalf("total unique usage records = %d, want 5", len(allIDs))
	}
}

func TestCreateRunToolCall_Dedicated(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-toolcall-dedicated")
	run := mustCreateRun(t, ctx, q, job)

	// Tool call with full fields
	tc1 := &domain.RunToolCall{
		ID:         newID(),
		RunID:      run.ID,
		ToolName:   "search",
		Input:      json.RawMessage(`{"query":"test"}`),
		Output:     json.RawMessage(`{"results":[1,2,3]}`),
		DurationMs: 150,
		Status:     "completed",
	}
	if err := q.CreateRunToolCall(ctx, tc1); err != nil {
		t.Fatalf("CreateRunToolCall(full) error = %v", err)
	}
	if tc1.CreatedAt.IsZero() {
		t.Fatal("tc1.CreatedAt is zero")
	}

	// Tool call with minimal fields (no input/output, zero duration)
	tc2 := &domain.RunToolCall{
		ID:       newID(),
		RunID:    run.ID,
		ToolName: "noop",
		Status:   "completed",
	}
	if err := q.CreateRunToolCall(ctx, tc2); err != nil {
		t.Fatalf("CreateRunToolCall(minimal) error = %v", err)
	}

	// Tool call with error status
	tc3 := &domain.RunToolCall{
		ID:       newID(),
		RunID:    run.ID,
		ToolName: "fail-tool",
		Input:    json.RawMessage(`{"x":1}`),
		Status:   "error",
	}
	if err := q.CreateRunToolCall(ctx, tc3); err != nil {
		t.Fatalf("CreateRunToolCall(error) error = %v", err)
	}

	calls, err := q.ListRunToolCalls(ctx, run.ID, 100, nil)
	if err != nil {
		t.Fatalf("ListRunToolCalls() error = %v", err)
	}
	if len(calls) != 3 {
		t.Fatalf("ListRunToolCalls() len = %d, want 3", len(calls))
	}

	// Verify DESC ordering
	for i := 1; i < len(calls); i++ {
		if calls[i].CreatedAt.After(calls[i-1].CreatedAt) {
			t.Fatalf("calls not DESC at index %d", i)
		}
	}

	// Empty for unknown run
	empty, err := q.ListRunToolCalls(ctx, newID(), 100, nil)
	if err != nil {
		t.Fatalf("ListRunToolCalls(unknown) error = %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("ListRunToolCalls(unknown) len = %d, want 0", len(empty))
	}
}

func TestRunToolCalls_EmptyInputOutput(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-toolcall-nil")
	run := mustCreateRun(t, ctx, q, job)

	tc := &domain.RunToolCall{
		ID:       newID(),
		RunID:    run.ID,
		ToolName: "empty-tool",
		Status:   "completed",
		// Input and Output intentionally nil
	}
	if err := q.CreateRunToolCall(ctx, tc); err != nil {
		t.Fatalf("CreateRunToolCall(nil io) error = %v", err)
	}

	calls, err := q.ListRunToolCalls(ctx, run.ID, 10, nil)
	if err != nil {
		t.Fatalf("ListRunToolCalls() error = %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("len = %d, want 1", len(calls))
	}
	if calls[0].ToolName != "empty-tool" {
		t.Fatalf("ToolName = %q, want empty-tool", calls[0].ToolName)
	}
	if calls[0].Input != nil {
		t.Fatalf("Input = %s, want nil", string(calls[0].Input))
	}
	if calls[0].Output != nil {
		t.Fatalf("Output = %s, want nil", string(calls[0].Output))
	}
	if calls[0].DurationMs != 0 {
		t.Fatalf("DurationMs = %d, want 0", calls[0].DurationMs)
	}
}

func TestUpsertRunOutput_Dedicated(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-output-upsert")
	run := mustCreateRun(t, ctx, q, job)

	// Insert initial output
	out := &domain.RunOutput{
		ID:        newID(),
		RunID:     run.ID,
		OutputKey: "my-key",
		Schema:    json.RawMessage(`{"type":"string"}`),
		Value:     json.RawMessage(`"initial-value"`),
	}
	if err := q.UpsertRunOutput(ctx, out); err != nil {
		t.Fatalf("UpsertRunOutput(insert) error = %v", err)
	}

	// Verify
	outputs, err := q.ListRunOutputs(ctx, run.ID, 100, nil)
	if err != nil {
		t.Fatalf("ListRunOutputs() error = %v", err)
	}
	if len(outputs) != 1 {
		t.Fatalf("ListRunOutputs() len = %d, want 1", len(outputs))
	}
	if !jsonEqual(outputs[0].Value, json.RawMessage(`"initial-value"`)) {
		t.Fatalf("value = %s, want \"initial-value\"", string(outputs[0].Value))
	}

	// Upsert same (run_id, output_key) with new value
	out2 := &domain.RunOutput{
		ID:        newID(),
		RunID:     run.ID,
		OutputKey: "my-key",
		Schema:    json.RawMessage(`{"type":"number"}`),
		Value:     json.RawMessage(`42`),
	}
	if err := q.UpsertRunOutput(ctx, out2); err != nil {
		t.Fatalf("UpsertRunOutput(upsert) error = %v", err)
	}

	// Should still be 1 output (upserted, not duplicated)
	outputs, err = q.ListRunOutputs(ctx, run.ID, 100, nil)
	if err != nil {
		t.Fatalf("ListRunOutputs() after upsert error = %v", err)
	}
	if len(outputs) != 1 {
		t.Fatalf("after upsert len = %d, want 1", len(outputs))
	}
	if !jsonEqual(outputs[0].Value, json.RawMessage(`42`)) {
		t.Fatalf("value after upsert = %s, want 42", string(outputs[0].Value))
	}
	if !jsonEqual(outputs[0].Schema, json.RawMessage(`{"type":"number"}`)) {
		t.Fatalf("schema after upsert = %s, want {\"type\":\"number\"}", string(outputs[0].Schema))
	}
}

func TestListRunOutputs_Dedicated(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-output-list")
	run := mustCreateRun(t, ctx, q, job)

	// Create multiple outputs with different keys
	keys := []string{"charlie", "alpha", "bravo"}
	for _, key := range keys {
		out := &domain.RunOutput{
			ID:        newID(),
			RunID:     run.ID,
			OutputKey: key,
			Value:     json.RawMessage(`"val-` + key + `"`),
		}
		if err := q.UpsertRunOutput(ctx, out); err != nil {
			t.Fatalf("UpsertRunOutput(%s) error = %v", key, err)
		}
	}

	outputs, err := q.ListRunOutputs(ctx, run.ID, 100, nil)
	if err != nil {
		t.Fatalf("ListRunOutputs() error = %v", err)
	}
	if len(outputs) != 3 {
		t.Fatalf("len = %d, want 3", len(outputs))
	}

	// Verify ASC ordering by output_key
	if outputs[0].OutputKey != "alpha" {
		t.Fatalf("outputs[0].OutputKey = %q, want alpha", outputs[0].OutputKey)
	}
	if outputs[1].OutputKey != "bravo" {
		t.Fatalf("outputs[1].OutputKey = %q, want bravo", outputs[1].OutputKey)
	}
	if outputs[2].OutputKey != "charlie" {
		t.Fatalf("outputs[2].OutputKey = %q, want charlie", outputs[2].OutputKey)
	}

	// Empty for unknown run
	empty, err := q.ListRunOutputs(ctx, newID(), 100, nil)
	if err != nil {
		t.Fatalf("ListRunOutputs(unknown) error = %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("ListRunOutputs(unknown) len = %d, want 0", len(empty))
	}
}

func TestRunOutput_NullSchema(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-output-null-schema")
	run := mustCreateRun(t, ctx, q, job)

	out := &domain.RunOutput{
		ID:        newID(),
		RunID:     run.ID,
		OutputKey: "no-schema",
		Value:     json.RawMessage(`{"data":"hello"}`),
		// Schema intentionally nil
	}
	if err := q.UpsertRunOutput(ctx, out); err != nil {
		t.Fatalf("UpsertRunOutput(nil schema) error = %v", err)
	}

	outputs, err := q.ListRunOutputs(ctx, run.ID, 10, nil)
	if err != nil {
		t.Fatalf("ListRunOutputs() error = %v", err)
	}
	if len(outputs) != 1 {
		t.Fatalf("len = %d, want 1", len(outputs))
	}
	if outputs[0].Schema != nil {
		t.Fatalf("Schema = %s, want nil", string(outputs[0].Schema))
	}
	if !jsonEqual(outputs[0].Value, json.RawMessage(`{"data":"hello"}`)) {
		t.Fatalf("Value = %s, want {\"data\":\"hello\"}", string(outputs[0].Value))
	}
}

func TestRunOutput_LargeValue(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-output-large")
	run := mustCreateRun(t, ctx, q, job)

	// Build a large JSON value (>10KB)
	items := make([]string, 500)
	for i := range items {
		items[i] = `"item-` + strconv.Itoa(i) + `"`
	}
	largeJSON := `[` + items[0]
	for i := 1; i < len(items); i++ {
		largeJSON += `,` + items[i]
	}
	largeJSON += `]`

	out := &domain.RunOutput{
		ID:        newID(),
		RunID:     run.ID,
		OutputKey: "large-output",
		Value:     json.RawMessage(largeJSON),
	}
	if err := q.UpsertRunOutput(ctx, out); err != nil {
		t.Fatalf("UpsertRunOutput(large) error = %v", err)
	}

	outputs, err := q.ListRunOutputs(ctx, run.ID, 10, nil)
	if err != nil {
		t.Fatalf("ListRunOutputs() error = %v", err)
	}
	if len(outputs) != 1 {
		t.Fatalf("len = %d, want 1", len(outputs))
	}
	if !jsonEqual(outputs[0].Value, json.RawMessage(largeJSON)) {
		t.Fatalf("large value mismatch: got %d bytes, want %d bytes", len(outputs[0].Value), len(largeJSON))
	}
}

func TestListJobsByGroup(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-jobs-by-group"

	// Create a job group
	group := &domain.JobGroup{ID: newID(), ProjectID: projectID, Name: "test-group", Slug: "test-group-slug"}
	if err := q.CreateJobGroup(ctx, group); err != nil {
		t.Fatalf("CreateJobGroup() error = %v", err)
	}

	// Create 3 jobs and assign them to the group
	for i := 0; i < 3; i++ {
		job := baseJob(newID(), projectID)
		job.Name = "group-job-" + strconv.Itoa(i)
		job.Slug = "group-job-slug-" + strconv.Itoa(i)
		if err := q.CreateJob(ctx, job); err != nil {
			t.Fatalf("CreateJob(%d) error = %v", i, err)
		}
		_, err := testDB.Pool.Exec(ctx, `UPDATE jobs SET group_id = $1 WHERE id = $2`, group.ID, job.ID)
		if err != nil {
			t.Fatalf("assign job to group error = %v", err)
		}
	}

	// Create a job NOT in the group
	jobOutside := baseJob(newID(), projectID)
	jobOutside.Name = "outside-job"
	jobOutside.Slug = "outside-slug"
	if err := q.CreateJob(ctx, jobOutside); err != nil {
		t.Fatalf("CreateJob(outside) error = %v", err)
	}

	// List jobs by group
	jobs, err := q.ListJobsByGroup(ctx, group.ID, 100, nil)
	if err != nil {
		t.Fatalf("ListJobsByGroup() error = %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("ListJobsByGroup() len = %d, want 3", len(jobs))
	}

	// Nonexistent group returns empty
	empty, err := q.ListJobsByGroup(ctx, newID(), 100, nil)
	if err != nil {
		t.Fatalf("ListJobsByGroup(unknown) error = %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("ListJobsByGroup(unknown) len = %d, want 0", len(empty))
	}
}

func TestListJobsByGroup_Pagination(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-group-pagination"
	group := &domain.JobGroup{ID: newID(), ProjectID: projectID, Name: "pg-group", Slug: "pg-group-slug"}
	if err := q.CreateJobGroup(ctx, group); err != nil {
		t.Fatalf("CreateJobGroup() error = %v", err)
	}

	// Create 5 jobs in the group
	for i := 0; i < 5; i++ {
		job := baseJob(newID(), projectID)
		job.Name = "pg-job-" + strconv.Itoa(i)
		job.Slug = "pg-job-slug-" + strconv.Itoa(i)
		if err := q.CreateJob(ctx, job); err != nil {
			t.Fatalf("CreateJob(%d) error = %v", i, err)
		}
		_, err := testDB.Pool.Exec(ctx, `UPDATE jobs SET group_id = $1 WHERE id = $2`, group.ID, job.ID)
		if err != nil {
			t.Fatalf("assign job to group error = %v", err)
		}
	}

	// Page 1: limit=2
	page1, err := q.ListJobsByGroup(ctx, group.ID, 2, nil)
	if err != nil {
		t.Fatalf("ListJobsByGroup(page1) error = %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("page1 len = %d, want 2", len(page1))
	}

	// Page 2: use cursor
	cursor := page1[len(page1)-1].CreatedAt
	page2, err := q.ListJobsByGroup(ctx, group.ID, 2, &cursor)
	if err != nil {
		t.Fatalf("ListJobsByGroup(page2) error = %v", err)
	}
	if len(page2) != 2 {
		t.Fatalf("page2 len = %d, want 2", len(page2))
	}

	// Ensure no overlap
	for _, j1 := range page1 {
		for _, j2 := range page2 {
			if j1.ID == j2.ID {
				t.Fatalf("overlap between page1 and page2: %s", j1.ID)
			}
		}
	}

	// Page 3: last item
	cursor2 := page2[len(page2)-1].CreatedAt
	page3, err := q.ListJobsByGroup(ctx, group.ID, 2, &cursor2)
	if err != nil {
		t.Fatalf("ListJobsByGroup(page3) error = %v", err)
	}
	if len(page3) != 1 {
		t.Fatalf("page3 len = %d, want 1", len(page3))
	}
}

func TestGetWorkflowBySlug(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-wf-by-slug"
	wf := &domain.Workflow{
		ID:          newID(),
		ProjectID:   projectID,
		Name:        "My Workflow",
		Slug:        "my-workflow-slug",
		Description: "A test workflow",
		Enabled:     true,
		TimeoutSecs: 300,
		Version:     1,
	}
	if err := q.CreateWorkflow(ctx, wf); err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}

	// Retrieve by slug
	got, err := q.GetWorkflowBySlug(ctx, projectID, "my-workflow-slug")
	if err != nil {
		t.Fatalf("GetWorkflowBySlug() error = %v", err)
	}
	if got.ID != wf.ID {
		t.Fatalf("ID = %q, want %q", got.ID, wf.ID)
	}
	if got.Name != "My Workflow" {
		t.Fatalf("Name = %q, want \"My Workflow\"", got.Name)
	}
	if got.Slug != "my-workflow-slug" {
		t.Fatalf("Slug = %q, want my-workflow-slug", got.Slug)
	}
	if got.Description != "A test workflow" {
		t.Fatalf("Description = %q, want \"A test workflow\"", got.Description)
	}
	if !got.Enabled {
		t.Fatal("Enabled = false, want true")
	}
	if got.TimeoutSecs != 300 {
		t.Fatalf("TimeoutSecs = %d, want 300", got.TimeoutSecs)
	}

	// Nonexistent slug
	_, err = q.GetWorkflowBySlug(ctx, projectID, "nonexistent-slug")
	if !errors.Is(err, store.ErrWorkflowNotFound) {
		t.Fatalf("GetWorkflowBySlug(unknown) error = %v, want ErrWorkflowNotFound", err)
	}

	// Wrong project
	_, err = q.GetWorkflowBySlug(ctx, "wrong-project", "my-workflow-slug")
	if !errors.Is(err, store.ErrWorkflowNotFound) {
		t.Fatalf("GetWorkflowBySlug(wrong project) error = %v, want ErrWorkflowNotFound", err)
	}
}

func TestListWorkflowRunsByProject(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-wfrun-by-project"
	wf := &domain.Workflow{ID: newID(), ProjectID: projectID, Name: "wf-list", Slug: "wf-list-slug", Enabled: true, Version: 1}
	if err := q.CreateWorkflow(ctx, wf); err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}

	// Create 3 workflow runs with different statuses
	run1 := &domain.WorkflowRun{ID: newID(), WorkflowID: wf.ID, ProjectID: projectID, Status: domain.WfStatusPending, TriggeredBy: "manual"}
	if err := q.CreateWorkflowRun(ctx, run1); err != nil {
		t.Fatalf("CreateWorkflowRun(1) error = %v", err)
	}

	run2 := &domain.WorkflowRun{ID: newID(), WorkflowID: wf.ID, ProjectID: projectID, Status: domain.WfStatusPending, TriggeredBy: "manual"}
	if err := q.CreateWorkflowRun(ctx, run2); err != nil {
		t.Fatalf("CreateWorkflowRun(2) error = %v", err)
	}
	// Transition to running
	if err := q.UpdateWorkflowRunStatus(ctx, run2.ID, domain.WfStatusPending, domain.WfStatusRunning, nil); err != nil {
		t.Fatalf("UpdateWorkflowRunStatus(running) error = %v", err)
	}

	run3 := &domain.WorkflowRun{ID: newID(), WorkflowID: wf.ID, ProjectID: projectID, Status: domain.WfStatusPending, TriggeredBy: "cron"}
	if err := q.CreateWorkflowRun(ctx, run3); err != nil {
		t.Fatalf("CreateWorkflowRun(3) error = %v", err)
	}
	if err := q.UpdateWorkflowRunStatus(ctx, run3.ID, domain.WfStatusPending, domain.WfStatusRunning, nil); err != nil {
		t.Fatalf("UpdateWorkflowRunStatus(3-running) error = %v", err)
	}
	if err := q.UpdateWorkflowRunStatus(ctx, run3.ID, domain.WfStatusRunning, domain.WfStatusCompleted, map[string]any{"finished_at": time.Now()}); err != nil {
		t.Fatalf("UpdateWorkflowRunStatus(3-completed) error = %v", err)
	}

	// List all — should be 3
	all, err := q.ListWorkflowRunsByProject(ctx, projectID, nil, 100, nil)
	if err != nil {
		t.Fatalf("ListWorkflowRunsByProject(all) error = %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("all len = %d, want 3", len(all))
	}

	// Filter by running status
	runningStatus := domain.WfStatusRunning
	running, err := q.ListWorkflowRunsByProject(ctx, projectID, &runningStatus, 100, nil)
	if err != nil {
		t.Fatalf("ListWorkflowRunsByProject(running) error = %v", err)
	}
	if len(running) != 1 {
		t.Fatalf("running len = %d, want 1", len(running))
	}
	if running[0].ID != run2.ID {
		t.Fatalf("running[0].ID = %q, want %q", running[0].ID, run2.ID)
	}

	// Filter by completed status
	completedStatus := domain.WfStatusCompleted
	completed, err := q.ListWorkflowRunsByProject(ctx, projectID, &completedStatus, 100, nil)
	if err != nil {
		t.Fatalf("ListWorkflowRunsByProject(completed) error = %v", err)
	}
	if len(completed) != 1 {
		t.Fatalf("completed len = %d, want 1", len(completed))
	}
	if completed[0].ID != run3.ID {
		t.Fatalf("completed[0].ID = %q, want %q", completed[0].ID, run3.ID)
	}

	// Pagination: limit=2
	page1, err := q.ListWorkflowRunsByProject(ctx, projectID, nil, 2, nil)
	if err != nil {
		t.Fatalf("ListWorkflowRunsByProject(page1) error = %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("page1 len = %d, want 2", len(page1))
	}

	cursor := page1[len(page1)-1].CreatedAt
	page2, err := q.ListWorkflowRunsByProject(ctx, projectID, nil, 2, &cursor)
	if err != nil {
		t.Fatalf("ListWorkflowRunsByProject(page2) error = %v", err)
	}
	if len(page2) != 1 {
		t.Fatalf("page2 len = %d, want 1", len(page2))
	}

	// Different project should be empty
	empty, err := q.ListWorkflowRunsByProject(ctx, "nonexistent-project", nil, 100, nil)
	if err != nil {
		t.Fatalf("ListWorkflowRunsByProject(empty) error = %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("empty len = %d, want 0", len(empty))
	}
}

func TestWorkflowVersionSnapshot(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-wf-version-snapshot-basic"
	job := mustCreateJob(t, ctx, q, projectID)
	wf := &domain.Workflow{ID: newID(), ProjectID: projectID, Name: "wf-version", Slug: "wf-version-slug", Enabled: true, Version: 1}
	if err := q.CreateWorkflow(ctx, wf); err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}

	step := &domain.WorkflowStep{ID: newID(), WorkflowID: wf.ID, JobID: job.ID, StepRef: "step-version"}
	if err := q.CreateWorkflowStep(ctx, step); err != nil {
		t.Fatalf("CreateWorkflowStep() error = %v", err)
	}

	if err := q.CreateWorkflowVersionSnapshot(ctx, wf.ID, 1); err != nil {
		t.Fatalf("CreateWorkflowVersionSnapshot() error = %v", err)
	}

	steps, err := q.ListStepsByWorkflowVersion(ctx, wf.ID, 1)
	if err != nil {
		t.Fatalf("ListStepsByWorkflowVersion() error = %v", err)
	}
	if len(steps) != 1 {
		t.Fatalf("ListStepsByWorkflowVersion() len = %d, want 1", len(steps))
	}
	if steps[0].StepRef != "step-version" {
		t.Fatalf("ListStepsByWorkflowVersion() step_ref = %q, want step-version", steps[0].StepRef)
	}
}

func TestListTimedOutWorkflowRuns(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-timeout-wf-runs"
	wf := &domain.Workflow{ID: newID(), ProjectID: projectID, Name: "wf-timeout", Slug: "wf-timeout-slug", Enabled: true, Version: 1}
	if err := q.CreateWorkflow(ctx, wf); err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}

	runningExpired := &domain.WorkflowRun{ID: newID(), WorkflowID: wf.ID, ProjectID: projectID, Status: domain.WfStatusPending, TriggeredBy: "manual"}
	if err := q.CreateWorkflowRun(ctx, runningExpired); err != nil {
		t.Fatalf("CreateWorkflowRun(runningExpired) error = %v", err)
	}
	if err := q.UpdateWorkflowRunStatus(ctx, runningExpired.ID, domain.WfStatusPending, domain.WfStatusRunning, nil); err != nil {
		t.Fatalf("UpdateWorkflowRunStatus(runningExpired->running) error = %v", err)
	}
	_, err := testDB.Pool.Exec(ctx, "UPDATE workflow_runs SET expires_at=$1 WHERE id=$2", time.Now().UTC().Add(-2*time.Hour), runningExpired.ID)
	if err != nil {
		t.Fatalf("set expires_at runningExpired error = %v", err)
	}

	pausedExpired := &domain.WorkflowRun{ID: newID(), WorkflowID: wf.ID, ProjectID: projectID, Status: domain.WfStatusPending, TriggeredBy: "manual"}
	if err := q.CreateWorkflowRun(ctx, pausedExpired); err != nil {
		t.Fatalf("CreateWorkflowRun(pausedExpired) error = %v", err)
	}
	if err := q.UpdateWorkflowRunStatus(ctx, pausedExpired.ID, domain.WfStatusPending, domain.WfStatusRunning, nil); err != nil {
		t.Fatalf("UpdateWorkflowRunStatus(pausedExpired->running) error = %v", err)
	}
	if err := q.UpdateWorkflowRunStatus(ctx, pausedExpired.ID, domain.WfStatusRunning, domain.WfStatusPaused, nil); err != nil {
		t.Fatalf("UpdateWorkflowRunStatus(pausedExpired->paused) error = %v", err)
	}
	_, err = testDB.Pool.Exec(ctx, "UPDATE workflow_runs SET expires_at=$1 WHERE id=$2", time.Now().UTC().Add(-1*time.Hour), pausedExpired.ID)
	if err != nil {
		t.Fatalf("set expires_at pausedExpired error = %v", err)
	}

	runningNotExpired := &domain.WorkflowRun{ID: newID(), WorkflowID: wf.ID, ProjectID: projectID, Status: domain.WfStatusPending, TriggeredBy: "manual"}
	if err := q.CreateWorkflowRun(ctx, runningNotExpired); err != nil {
		t.Fatalf("CreateWorkflowRun(runningNotExpired) error = %v", err)
	}
	if err := q.UpdateWorkflowRunStatus(ctx, runningNotExpired.ID, domain.WfStatusPending, domain.WfStatusRunning, nil); err != nil {
		t.Fatalf("UpdateWorkflowRunStatus(runningNotExpired->running) error = %v", err)
	}
	_, err = testDB.Pool.Exec(ctx, "UPDATE workflow_runs SET expires_at=$1 WHERE id=$2", time.Now().UTC().Add(1*time.Hour), runningNotExpired.ID)
	if err != nil {
		t.Fatalf("set expires_at runningNotExpired error = %v", err)
	}

	completedExpired := &domain.WorkflowRun{ID: newID(), WorkflowID: wf.ID, ProjectID: projectID, Status: domain.WfStatusPending, TriggeredBy: "manual"}
	if err := q.CreateWorkflowRun(ctx, completedExpired); err != nil {
		t.Fatalf("CreateWorkflowRun(completedExpired) error = %v", err)
	}
	if err := q.UpdateWorkflowRunStatus(ctx, completedExpired.ID, domain.WfStatusPending, domain.WfStatusRunning, nil); err != nil {
		t.Fatalf("UpdateWorkflowRunStatus(completedExpired->running) error = %v", err)
	}
	if err := q.UpdateWorkflowRunStatus(ctx, completedExpired.ID, domain.WfStatusRunning, domain.WfStatusCompleted, map[string]any{"finished_at": time.Now().UTC()}); err != nil {
		t.Fatalf("UpdateWorkflowRunStatus(completedExpired->completed) error = %v", err)
	}
	_, err = testDB.Pool.Exec(ctx, "UPDATE workflow_runs SET expires_at=$1 WHERE id=$2", time.Now().UTC().Add(-3*time.Hour), completedExpired.ID)
	if err != nil {
		t.Fatalf("set expires_at completedExpired error = %v", err)
	}

	runs, err := q.ListTimedOutWorkflowRuns(ctx)
	if err != nil {
		t.Fatalf("ListTimedOutWorkflowRuns() error = %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("ListTimedOutWorkflowRuns() len = %d, want 2", len(runs))
	}
	if runs[0].ID != runningExpired.ID {
		t.Fatalf("runs[0].ID = %q, want %q", runs[0].ID, runningExpired.ID)
	}
	if runs[1].ID != pausedExpired.ID {
		t.Fatalf("runs[1].ID = %q, want %q", runs[1].ID, pausedExpired.ID)
	}
	if runs[0].Status != domain.WfStatusRunning {
		t.Fatalf("runs[0].status = %q, want running", runs[0].Status)
	}
	if runs[1].Status != domain.WfStatusPaused {
		t.Fatalf("runs[1].status = %q, want paused", runs[1].Status)
	}
}

// --- RBAC integration tests ---

func TestCreateProjectRole(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	role := &domain.ProjectRole{
		ProjectID:   "proj-rbac",
		Name:        "deployer",
		Description: "Can deploy jobs",
		Permissions: []string{"jobs:write", "jobs:trigger"},
	}
	if err := q.CreateProjectRole(ctx, role); err != nil {
		t.Fatalf("CreateProjectRole() error = %v", err)
	}
	if role.ID == "" {
		t.Fatal("role.ID should be set")
	}
	if role.CreatedAt.IsZero() {
		t.Fatal("role.CreatedAt should be set")
	}
}

func TestCreateProjectRole_DuplicateName(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	role := &domain.ProjectRole{
		ProjectID:   "proj-rbac-dup",
		Name:        "viewer",
		Permissions: []string{"jobs:read"},
	}
	if err := q.CreateProjectRole(ctx, role); err != nil {
		t.Fatalf("first CreateProjectRole() error = %v", err)
	}

	role2 := &domain.ProjectRole{
		ProjectID:   "proj-rbac-dup",
		Name:        "viewer",
		Permissions: []string{"runs:read"},
	}
	err := q.CreateProjectRole(ctx, role2)
	if err == nil {
		t.Fatal("expected error for duplicate role name, got nil")
	}
}

func TestListProjectRoles(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	for _, name := range []string{"admin", "viewer", "deployer"} {
		role := &domain.ProjectRole{
			ProjectID:   "proj-list-roles",
			Name:        name,
			Permissions: []string{"jobs:read"},
		}
		if err := q.CreateProjectRole(ctx, role); err != nil {
			t.Fatalf("CreateProjectRole(%s) error = %v", name, err)
		}
	}

	roles, err := q.ListProjectRoles(ctx, "proj-list-roles")
	if err != nil {
		t.Fatalf("ListProjectRoles() error = %v", err)
	}
	if len(roles) != 3 {
		t.Fatalf("len(roles) = %d, want 3", len(roles))
	}
}

func TestDeleteProjectRole_SystemProtected(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	role := &domain.ProjectRole{
		ProjectID:   "proj-sys-role",
		Name:        "system-admin",
		Permissions: []string{"*"},
		IsSystem:    true,
	}
	if err := q.CreateProjectRole(ctx, role); err != nil {
		t.Fatalf("CreateProjectRole() error = %v", err)
	}

	err := q.DeleteProjectRole(ctx, role.ID)
	if !errors.Is(err, store.ErrRoleNotFound) {
		t.Fatalf("DeleteProjectRole(system) = %v, want ErrRoleNotFound", err)
	}
}

func TestAssignMemberRole_Upsert(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	role1 := &domain.ProjectRole{
		ProjectID:   "proj-assign",
		Name:        "viewer",
		Permissions: []string{"jobs:read"},
	}
	if err := q.CreateProjectRole(ctx, role1); err != nil {
		t.Fatalf("CreateProjectRole(viewer) error = %v", err)
	}

	role2 := &domain.ProjectRole{
		ProjectID:   "proj-assign",
		Name:        "admin",
		Permissions: []string{"*"},
	}
	if err := q.CreateProjectRole(ctx, role2); err != nil {
		t.Fatalf("CreateProjectRole(admin) error = %v", err)
	}

	m := &domain.ProjectMemberRole{
		ProjectID: "proj-assign",
		UserID:    "user-1",
		RoleID:    role1.ID,
		GrantedBy: "setup",
	}
	if err := q.AssignMemberRole(ctx, m); err != nil {
		t.Fatalf("AssignMemberRole() error = %v", err)
	}

	// Upsert: reassign to admin.
	m2 := &domain.ProjectMemberRole{
		ProjectID: "proj-assign",
		UserID:    "user-1",
		RoleID:    role2.ID,
		GrantedBy: "admin",
	}
	if err := q.AssignMemberRole(ctx, m2); err != nil {
		t.Fatalf("AssignMemberRole(upsert) error = %v", err)
	}

	got, err := q.GetMemberRole(ctx, "proj-assign", "user-1")
	if err != nil {
		t.Fatalf("GetMemberRole() error = %v", err)
	}
	if got.RoleID != role2.ID {
		t.Fatalf("after upsert, RoleID = %q, want %q", got.RoleID, role2.ID)
	}
}

func TestGetUserPermissions(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	role := &domain.ProjectRole{
		ProjectID:   "proj-perms",
		Name:        "operator",
		Permissions: []string{"jobs:read", "jobs:write", "runs:read"},
	}
	if err := q.CreateProjectRole(ctx, role); err != nil {
		t.Fatalf("CreateProjectRole() error = %v", err)
	}

	m := &domain.ProjectMemberRole{
		ProjectID: "proj-perms",
		UserID:    "user-perms",
		RoleID:    role.ID,
	}
	if err := q.AssignMemberRole(ctx, m); err != nil {
		t.Fatalf("AssignMemberRole() error = %v", err)
	}

	perms, err := q.GetUserPermissions(ctx, "proj-perms", "user-perms")
	if err != nil {
		t.Fatalf("GetUserPermissions() error = %v", err)
	}
	if len(perms) != 3 {
		t.Fatalf("len(perms) = %d, want 3", len(perms))
	}
}

func TestGetUserPermissions_NoRole(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	perms, err := q.GetUserPermissions(ctx, "proj-no-role", "unknown-user")
	if err != nil {
		t.Fatalf("GetUserPermissions() error = %v", err)
	}
	if perms != nil {
		t.Fatalf("perms = %v, want nil", perms)
	}
}

func TestResourcePolicy_CRUD(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	p := &domain.ResourcePolicy{
		ProjectID:    "proj-policy",
		ResourceType: "job",
		ResourceID:   "job-123",
		UserID:       "user-pol",
		Actions:      []string{"trigger", "read"},
	}
	if err := q.CreateResourcePolicy(ctx, p); err != nil {
		t.Fatalf("CreateResourcePolicy() error = %v", err)
	}
	if p.ID == "" {
		t.Fatal("policy.ID should be set")
	}

	actions, err := q.GetResourcePolicies(ctx, "job", "job-123", "user-pol")
	if err != nil {
		t.Fatalf("GetResourcePolicies() error = %v", err)
	}
	if len(actions) != 2 {
		t.Fatalf("len(actions) = %d, want 2", len(actions))
	}

	policies, err := q.ListResourcePolicies(ctx, "job", "job-123")
	if err != nil {
		t.Fatalf("ListResourcePolicies() error = %v", err)
	}
	if len(policies) != 1 {
		t.Fatalf("len(policies) = %d, want 1", len(policies))
	}

	if err := q.DeleteResourcePolicy(ctx, p.ID); err != nil {
		t.Fatalf("DeleteResourcePolicy() error = %v", err)
	}

	actions2, err := q.GetResourcePolicies(ctx, "job", "job-123", "user-pol")
	if err != nil {
		t.Fatalf("GetResourcePolicies() after delete error = %v", err)
	}
	if actions2 != nil {
		t.Fatalf("actions after delete = %v, want nil", actions2)
	}
}

// --- Actor integration tests ---

func TestUpsertKnownActor(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	if err := q.UpsertKnownActor(ctx, "actor-1", "alice@example.com", "Alice"); err != nil {
		t.Fatalf("UpsertKnownActor() error = %v", err)
	}

	actor, err := q.GetKnownActor(ctx, "actor-1")
	if err != nil {
		t.Fatalf("GetKnownActor() error = %v", err)
	}
	if actor == nil {
		t.Fatal("actor should not be nil")
	}
	if actor.Email != "alice@example.com" {
		t.Fatalf("actor.Email = %q, want %q", actor.Email, "alice@example.com")
	}
	if actor.Name != "Alice" {
		t.Fatalf("actor.Name = %q, want %q", actor.Name, "Alice")
	}
}

func TestUpsertKnownActor_PreservesExisting(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	if err := q.UpsertKnownActor(ctx, "actor-2", "bob@example.com", "Bob"); err != nil {
		t.Fatalf("first upsert error = %v", err)
	}

	// Second upsert with empty name should keep "Bob".
	if err := q.UpsertKnownActor(ctx, "actor-2", "bob-new@example.com", ""); err != nil {
		t.Fatalf("second upsert error = %v", err)
	}

	actor, err := q.GetKnownActor(ctx, "actor-2")
	if err != nil {
		t.Fatalf("GetKnownActor() error = %v", err)
	}
	if actor.Name != "Bob" {
		t.Fatalf("actor.Name = %q, want %q (should be preserved)", actor.Name, "Bob")
	}
	if actor.Email != "bob-new@example.com" {
		t.Fatalf("actor.Email = %q, want %q (should be updated)", actor.Email, "bob-new@example.com")
	}
}

func TestGetKnownActor_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	actor, err := q.GetKnownActor(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("GetKnownActor() error = %v", err)
	}
	if actor != nil {
		t.Fatalf("actor = %v, want nil", actor)
	}
}

// --- Version ID + created_by integration tests ---

func TestCreateJob_SetsVersionID(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "proj-vid")
	job.CreatedBy = "user-creator"
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}
	if job.VersionID == "" {
		t.Fatal("job.VersionID should be set after create")
	}
	if job.CreatedBy != "user-creator" {
		t.Fatalf("job.CreatedBy = %q, want %q", job.CreatedBy, "user-creator")
	}

	// Read back.
	got, err := q.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetJob() error = %v", err)
	}
	if got.VersionID != job.VersionID {
		t.Fatalf("read back VersionID = %q, want %q", got.VersionID, job.VersionID)
	}
	if got.CreatedBy != "user-creator" {
		t.Fatalf("read back CreatedBy = %q, want %q", got.CreatedBy, "user-creator")
	}
}

func TestUpdateJob_GeneratesNewVersionID(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "proj-vid-upd")
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}
	oldVersionID := job.VersionID

	job.Name = "updated-name"
	job.UpdatedBy = "user-updater"
	if err := q.UpdateJob(ctx, job); err != nil {
		t.Fatalf("UpdateJob() error = %v", err)
	}

	if job.VersionID == oldVersionID {
		t.Fatal("VersionID should change after update")
	}
	if job.VersionID == "" {
		t.Fatal("new VersionID should not be empty")
	}
}

// --- Workflow version ID + created_by tests ---

func TestCreateWorkflow_SetsVersionID(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	w := &domain.Workflow{
		ProjectID: "proj-wf-vid",
		Name:      "wf-test",
		Slug:      "wf-test",
		Enabled:   true,
		CreatedBy: "user-wf-creator",
	}
	if err := q.CreateWorkflow(ctx, w); err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}
	if w.VersionID == "" {
		t.Fatal("workflow.VersionID should be set after create")
	}
	if w.VersionPolicy != domain.VersionPolicyPin {
		t.Fatalf("workflow.VersionPolicy = %q, want %q", w.VersionPolicy, domain.VersionPolicyPin)
	}

	got, err := q.GetWorkflow(ctx, w.ID)
	if err != nil {
		t.Fatalf("GetWorkflow() error = %v", err)
	}
	if got.VersionID != w.VersionID {
		t.Fatalf("read back VersionID = %q, want %q", got.VersionID, w.VersionID)
	}
	if got.CreatedBy != "user-wf-creator" {
		t.Fatalf("read back CreatedBy = %q, want %q", got.CreatedBy, "user-wf-creator")
	}
}

func TestUpdateWorkflow_GeneratesNewVersionID(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	w := &domain.Workflow{
		ProjectID: "proj-wf-vid-upd",
		Name:      "wf-upd",
		Slug:      "wf-upd",
		Enabled:   true,
	}
	if err := q.CreateWorkflow(ctx, w); err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}
	oldVersionID := w.VersionID

	w.Name = "wf-updated"
	w.UpdatedBy = "user-updater"
	if err := q.UpdateWorkflow(ctx, w); err != nil {
		t.Fatalf("UpdateWorkflow() error = %v", err)
	}

	if w.VersionID == oldVersionID {
		t.Fatal("VersionID should change after update")
	}
	if w.VersionID == "" {
		t.Fatal("new VersionID should not be empty")
	}
	if w.Version != 2 {
		t.Fatalf("version = %d, want 2", w.Version)
	}
}

func TestCreateJob_DefaultVersionPolicy(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "proj-vpol")
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	got, err := q.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetJob() error = %v", err)
	}
	if got.VersionPolicy != domain.VersionPolicyPin {
		t.Fatalf("VersionPolicy = %q, want %q", got.VersionPolicy, domain.VersionPolicyPin)
	}
}

// --- DeleteResourcePolicy sentinel test ---

func TestDeleteResourcePolicy_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	err := q.DeleteResourcePolicy(ctx, "nonexistent-policy-id")
	if !errors.Is(err, store.ErrResourcePolicyNotFound) {
		t.Fatalf("DeleteResourcePolicy() = %v, want ErrResourcePolicyNotFound", err)
	}
}

// --- RemoveMemberRole sentinel test ---

func TestRemoveMemberRole_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	err := q.RemoveMemberRole(ctx, "proj-nonexistent", "user-nonexistent")
	if !errors.Is(err, store.ErrMemberNotFound) {
		t.Fatalf("RemoveMemberRole() = %v, want ErrMemberNotFound", err)
	}
}

// --- UpdateProjectRole integration test ---

func TestUpdateProjectRole(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	role := &domain.ProjectRole{
		ProjectID:   "proj-update-role",
		Name:        "deployer",
		Description: "Original",
		Permissions: []string{"jobs:write"},
	}
	if err := q.CreateProjectRole(ctx, role); err != nil {
		t.Fatalf("CreateProjectRole() error = %v", err)
	}

	role.Name = "deployer-v2"
	role.Description = "Updated"
	role.Permissions = []string{"jobs:write", "jobs:trigger"}
	if err := q.UpdateProjectRole(ctx, role); err != nil {
		t.Fatalf("UpdateProjectRole() error = %v", err)
	}

	got, err := q.GetProjectRole(ctx, role.ID)
	if err != nil {
		t.Fatalf("GetProjectRole() error = %v", err)
	}
	if got.Name != "deployer-v2" {
		t.Fatalf("Name = %q, want %q", got.Name, "deployer-v2")
	}
	if len(got.Permissions) != 2 {
		t.Fatalf("len(Permissions) = %d, want 2", len(got.Permissions))
	}
}

func TestUpdateProjectRole_SystemRoleBlocked(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	role := &domain.ProjectRole{
		ProjectID:   "proj-sys-update",
		Name:        "sys-admin",
		Permissions: []string{"*"},
		IsSystem:    true,
	}
	if err := q.CreateProjectRole(ctx, role); err != nil {
		t.Fatalf("CreateProjectRole() error = %v", err)
	}

	role.Name = "hacked"
	err := q.UpdateProjectRole(ctx, role)
	if !errors.Is(err, store.ErrRoleNotFound) {
		t.Fatalf("UpdateProjectRole(system) = %v, want ErrRoleNotFound", err)
	}
}

// --- ListProjectMembers test ---

func TestListProjectMembers(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	role := &domain.ProjectRole{
		ProjectID:   "proj-list-members",
		Name:        "viewer",
		Permissions: []string{"jobs:read"},
	}
	if err := q.CreateProjectRole(ctx, role); err != nil {
		t.Fatalf("CreateProjectRole() error = %v", err)
	}

	for _, userID := range []string{"user-a", "user-b", "user-c"} {
		m := &domain.ProjectMemberRole{
			ProjectID: "proj-list-members",
			UserID:    userID,
			RoleID:    role.ID,
		}
		if err := q.AssignMemberRole(ctx, m); err != nil {
			t.Fatalf("AssignMemberRole(%s) error = %v", userID, err)
		}
	}

	members, err := q.ListProjectMembers(ctx, "proj-list-members")
	if err != nil {
		t.Fatalf("ListProjectMembers() error = %v", err)
	}
	if len(members) != 3 {
		t.Fatalf("len(members) = %d, want 3", len(members))
	}
}

// --- Tags query tests ---

func TestListJobsByTag_KeyOnly(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "proj-tags-key")
	job.Tags = map[string]string{"env": "prod", "team": "backend"}
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	// Search by key only (any value)
	jobs, err := q.ListJobsByTag(ctx, "proj-tags-key", "env", "", 50, nil)
	if err != nil {
		t.Fatalf("ListJobsByTag(key-only) error = %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}

	// Key doesn't exist
	jobs2, err := q.ListJobsByTag(ctx, "proj-tags-key", "missing", "", 50, nil)
	if err != nil {
		t.Fatalf("ListJobsByTag(missing key) error = %v", err)
	}
	if len(jobs2) != 0 {
		t.Fatalf("len(jobs) = %d, want 0", len(jobs2))
	}
}

func TestListJobsByTag_KeyValue(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job1 := baseJob(newID(), "proj-tags-kv")
	job1.Tags = map[string]string{"env": "prod"}
	if err := q.CreateJob(ctx, job1); err != nil {
		t.Fatalf("CreateJob(1) error = %v", err)
	}

	job2 := baseJob(newID(), "proj-tags-kv")
	job2.Tags = map[string]string{"env": "staging"}
	if err := q.CreateJob(ctx, job2); err != nil {
		t.Fatalf("CreateJob(2) error = %v", err)
	}

	jobs, err := q.ListJobsByTag(ctx, "proj-tags-kv", "env", "prod", 50, nil)
	if err != nil {
		t.Fatalf("ListJobsByTag() error = %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}
	if jobs[0].ID != job1.ID {
		t.Fatalf("got job %q, want %q", jobs[0].ID, job1.ID)
	}
}

func TestListWorkflowsByTag(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	w := &domain.Workflow{
		ProjectID: "proj-wf-tags",
		Name:      "tagged-wf",
		Slug:      "tagged-wf",
		Enabled:   true,
		Tags:      map[string]string{"service": "payments"},
	}
	if err := q.CreateWorkflow(ctx, w); err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}

	workflows, err := q.ListWorkflowsByTag(ctx, "proj-wf-tags", "service", "payments", 50, nil)
	if err != nil {
		t.Fatalf("ListWorkflowsByTag() error = %v", err)
	}
	if len(workflows) != 1 {
		t.Fatalf("len(workflows) = %d, want 1", len(workflows))
	}

	// Wrong value
	workflows2, err := q.ListWorkflowsByTag(ctx, "proj-wf-tags", "service", "wrong", 50, nil)
	if err != nil {
		t.Fatalf("ListWorkflowsByTag(wrong value) error = %v", err)
	}
	if len(workflows2) != 0 {
		t.Fatalf("len(workflows) = %d, want 0", len(workflows2))
	}
}

func TestJobEmptyTags(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "proj-empty-tags")
	// No tags set (nil)
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	got, err := q.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetJob() error = %v", err)
	}
	if got.Tags != nil {
		t.Fatalf("expected nil tags, got %v", got.Tags)
	}
}

// ====================================================================
// Test hardening: RBAC store
// ====================================================================

func TestDeleteProjectRole_CustomRole(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	role := &domain.ProjectRole{
		ProjectID:   "proj-del-custom",
		Name:        "custom-role",
		Description: "A custom role",
		Permissions: []string{"jobs:read"},
		IsSystem:    false,
	}
	if err := q.CreateProjectRole(ctx, role); err != nil {
		t.Fatalf("CreateProjectRole() error = %v", err)
	}

	if err := q.DeleteProjectRole(ctx, role.ID); err != nil {
		t.Fatalf("DeleteProjectRole() error = %v (should succeed for non-system)", err)
	}

	// Verify it's gone.
	_, err := q.GetProjectRole(ctx, role.ID)
	if !errors.Is(err, store.ErrRoleNotFound) {
		t.Fatalf("expected ErrRoleNotFound after delete, got %v", err)
	}
}

func TestGetMemberRole_Exists(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	role := &domain.ProjectRole{
		ProjectID:   "proj-get-member",
		Name:        "admin",
		Permissions: []string{"*"},
	}
	if err := q.CreateProjectRole(ctx, role); err != nil {
		t.Fatalf("CreateProjectRole() error = %v", err)
	}

	m := &domain.ProjectMemberRole{
		ProjectID: "proj-get-member",
		UserID:    "user-get-member",
		RoleID:    role.ID,
		GrantedBy: "admin-user",
	}
	if err := q.AssignMemberRole(ctx, m); err != nil {
		t.Fatalf("AssignMemberRole() error = %v", err)
	}

	got, err := q.GetMemberRole(ctx, "proj-get-member", "user-get-member")
	if err != nil {
		t.Fatalf("GetMemberRole() error = %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil member role")
	}
	if got.RoleID != role.ID {
		t.Fatalf("got.RoleID = %q, want %q", got.RoleID, role.ID)
	}
	if got.GrantedBy != "admin-user" {
		t.Fatalf("got.GrantedBy = %q, want %q", got.GrantedBy, "admin-user")
	}
}

func TestGetMemberRole_NotExists(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	got, err := q.GetMemberRole(ctx, "proj-nonexistent", "user-nonexistent")
	if err != nil {
		t.Fatalf("GetMemberRole() error = %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil for non-existent member, got %+v", got)
	}
}

func TestCreateResourcePolicy_Upsert(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	p := &domain.ResourcePolicy{
		ProjectID:    "proj-rp-upsert",
		ResourceType: "job",
		ResourceID:   "job-1",
		UserID:       "user-1",
		Actions:      []string{"read"},
	}
	if err := q.CreateResourcePolicy(ctx, p); err != nil {
		t.Fatalf("CreateResourcePolicy() error = %v", err)
	}

	// Upsert with new actions.
	p2 := &domain.ResourcePolicy{
		ProjectID:    "proj-rp-upsert",
		ResourceType: "job",
		ResourceID:   "job-1",
		UserID:       "user-1",
		Actions:      []string{"read", "write"},
	}
	if err := q.CreateResourcePolicy(ctx, p2); err != nil {
		t.Fatalf("CreateResourcePolicy(upsert) error = %v", err)
	}

	// Read back — should be updated.
	actions, err := q.GetResourcePolicies(ctx, "job", "job-1", "user-1")
	if err != nil {
		t.Fatalf("GetResourcePolicies() error = %v", err)
	}
	if len(actions) != 2 {
		t.Fatalf("actions = %v, want 2 items", actions)
	}
}

func TestListResourcePolicies_Empty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	policies, err := q.ListResourcePolicies(ctx, "job", "nonexistent")
	if err != nil {
		t.Fatalf("ListResourcePolicies() error = %v", err)
	}
	if policies == nil {
		t.Fatal("expected non-nil empty slice")
	}
	if len(policies) != 0 {
		t.Fatalf("expected 0 policies, got %d", len(policies))
	}
}

func TestListResourcePolicies_MultipleUsers(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	for _, userID := range []string{"user-a", "user-b", "user-c"} {
		p := &domain.ResourcePolicy{
			ProjectID:    "proj-rp-multi",
			ResourceType: "workflow",
			ResourceID:   "wf-1",
			UserID:       userID,
			Actions:      []string{"read"},
		}
		if err := q.CreateResourcePolicy(ctx, p); err != nil {
			t.Fatalf("CreateResourcePolicy(%s) error = %v", userID, err)
		}
	}

	policies, err := q.ListResourcePolicies(ctx, "workflow", "wf-1")
	if err != nil {
		t.Fatalf("ListResourcePolicies() error = %v", err)
	}
	if len(policies) != 3 {
		t.Fatalf("expected 3 policies, got %d", len(policies))
	}
}

func TestGetResourcePolicies_WrongUser(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	p := &domain.ResourcePolicy{
		ProjectID:    "proj-rp-wrong",
		ResourceType: "job",
		ResourceID:   "job-1",
		UserID:       "user-a",
		Actions:      []string{"read"},
	}
	if err := q.CreateResourcePolicy(ctx, p); err != nil {
		t.Fatalf("CreateResourcePolicy() error = %v", err)
	}

	actions, err := q.GetResourcePolicies(ctx, "job", "job-1", "user-b")
	if err != nil {
		t.Fatalf("GetResourcePolicies() error = %v", err)
	}
	if actions != nil {
		t.Fatalf("expected nil for wrong user, got %v", actions)
	}
}

func TestListProjectRoles_EmptyProject(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	roles, err := q.ListProjectRoles(ctx, "proj-no-roles")
	if err != nil {
		t.Fatalf("ListProjectRoles() error = %v", err)
	}
	if len(roles) != 0 {
		t.Fatalf("expected 0 roles, got %d", len(roles))
	}
}

func TestGetUserPermissions_WildcardRole(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	role := &domain.ProjectRole{
		ProjectID:   "proj-perm-wildcard",
		Name:        "super-admin",
		Permissions: []string{"*"},
	}
	if err := q.CreateProjectRole(ctx, role); err != nil {
		t.Fatalf("CreateProjectRole() error = %v", err)
	}

	m := &domain.ProjectMemberRole{
		ProjectID: "proj-perm-wildcard",
		UserID:    "user-wildcard",
		RoleID:    role.ID,
	}
	if err := q.AssignMemberRole(ctx, m); err != nil {
		t.Fatalf("AssignMemberRole() error = %v", err)
	}

	perms, err := q.GetUserPermissions(ctx, "proj-perm-wildcard", "user-wildcard")
	if err != nil {
		t.Fatalf("GetUserPermissions() error = %v", err)
	}
	if len(perms) != 1 || perms[0] != "*" {
		t.Fatalf("perms = %v, want [*]", perms)
	}
}

func TestCreateProjectRole_IDAutoGenerated(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	role := &domain.ProjectRole{
		ProjectID:   "proj-autoid",
		Name:        "autoid-role",
		Permissions: []string{"jobs:read"},
	}
	if err := q.CreateProjectRole(ctx, role); err != nil {
		t.Fatalf("CreateProjectRole() error = %v", err)
	}
	if role.ID == "" {
		t.Fatal("role.ID should be auto-generated")
	}
}

func TestUpdateProjectRole_NameChange(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	role := &domain.ProjectRole{
		ProjectID:   "proj-role-rename",
		Name:        "original-name",
		Permissions: []string{"jobs:read"},
	}
	if err := q.CreateProjectRole(ctx, role); err != nil {
		t.Fatalf("CreateProjectRole() error = %v", err)
	}

	role.Name = "new-name"
	role.Description = "updated desc"
	if err := q.UpdateProjectRole(ctx, role); err != nil {
		t.Fatalf("UpdateProjectRole() error = %v", err)
	}

	got, err := q.GetProjectRole(ctx, role.ID)
	if err != nil {
		t.Fatalf("GetProjectRole() error = %v", err)
	}
	if got.Name != "new-name" {
		t.Fatalf("Name = %q, want %q", got.Name, "new-name")
	}
	if got.Description != "updated desc" {
		t.Fatalf("Description = %q, want %q", got.Description, "updated desc")
	}
}

// ====================================================================
// Test hardening: Actors
// ====================================================================

func TestUpsertKnownActor_UpdateEmail(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	if err := q.UpsertKnownActor(ctx, "actor-update-email", "old@example.com", "Alice"); err != nil {
		t.Fatalf("UpsertKnownActor() error = %v", err)
	}

	// Update email.
	if err := q.UpsertKnownActor(ctx, "actor-update-email", "new@example.com", ""); err != nil {
		t.Fatalf("UpsertKnownActor(update) error = %v", err)
	}

	got, err := q.GetKnownActor(ctx, "actor-update-email")
	if err != nil {
		t.Fatalf("GetKnownActor() error = %v", err)
	}
	if got.Email != "new@example.com" {
		t.Fatalf("Email = %q, want %q", got.Email, "new@example.com")
	}
	// Name should be preserved (empty string in second upsert).
	if got.Name != "Alice" {
		t.Fatalf("Name = %q, want %q (should be preserved)", got.Name, "Alice")
	}
}

func TestUpsertKnownActor_BothEmpty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	if err := q.UpsertKnownActor(ctx, "actor-both-empty", "orig@example.com", "Bob"); err != nil {
		t.Fatalf("UpsertKnownActor() error = %v", err)
	}

	// Upsert with both empty — should preserve originals.
	if err := q.UpsertKnownActor(ctx, "actor-both-empty", "", ""); err != nil {
		t.Fatalf("UpsertKnownActor(both empty) error = %v", err)
	}

	got, err := q.GetKnownActor(ctx, "actor-both-empty")
	if err != nil {
		t.Fatalf("GetKnownActor() error = %v", err)
	}
	if got.Email != "orig@example.com" {
		t.Fatalf("Email = %q, want preserved %q", got.Email, "orig@example.com")
	}
	if got.Name != "Bob" {
		t.Fatalf("Name = %q, want preserved %q", got.Name, "Bob")
	}
}

func TestUpsertKnownActor_SyncedAtUpdates(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	if err := q.UpsertKnownActor(ctx, "actor-synced-at", "a@b.com", "A"); err != nil {
		t.Fatalf("UpsertKnownActor(1) error = %v", err)
	}
	got1, err := q.GetKnownActor(ctx, "actor-synced-at")
	if err != nil {
		t.Fatalf("GetKnownActor(1) error = %v", err)
	}

	// Second upsert.
	if err := q.UpsertKnownActor(ctx, "actor-synced-at", "a@b.com", "A"); err != nil {
		t.Fatalf("UpsertKnownActor(2) error = %v", err)
	}
	got2, err := q.GetKnownActor(ctx, "actor-synced-at")
	if err != nil {
		t.Fatalf("GetKnownActor(2) error = %v", err)
	}

	if got2.SyncedAt.Before(got1.SyncedAt) {
		t.Fatalf("synced_at should not go backwards: %v < %v", got2.SyncedAt, got1.SyncedAt)
	}
}

func TestGetKnownActor_AllFields(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	if err := q.UpsertKnownActor(ctx, "actor-all-fields", "all@example.com", "All Fields"); err != nil {
		t.Fatalf("UpsertKnownActor() error = %v", err)
	}

	got, err := q.GetKnownActor(ctx, "actor-all-fields")
	if err != nil {
		t.Fatalf("GetKnownActor() error = %v", err)
	}
	if got.ID != "actor-all-fields" {
		t.Fatalf("ID = %q, want %q", got.ID, "actor-all-fields")
	}
	if got.Email != "all@example.com" {
		t.Fatalf("Email = %q", got.Email)
	}
	if got.Name != "All Fields" {
		t.Fatalf("Name = %q", got.Name)
	}
	if got.SyncedAt.IsZero() {
		t.Fatal("SyncedAt should not be zero")
	}
	// AvatarURL is not set via upsert, so it should be empty.
	if got.AvatarURL != "" {
		t.Fatalf("AvatarURL = %q, want empty", got.AvatarURL)
	}
}

// ====================================================================
// Test hardening: Jobs with new fields
// ====================================================================

func TestCreateJob_VersionIDPrefix(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "proj-vid-prefix")
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	if job.VersionID == "" || job.VersionID[:4] != "ver_" {
		t.Fatalf("VersionID = %q, want prefix 'ver_'", job.VersionID)
	}
}

func TestCreateJob_UpdatedByEmpty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "proj-empty-updatedby")
	job.CreatedBy = "creator"
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	got, err := q.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetJob() error = %v", err)
	}
	if got.CreatedBy != "creator" {
		t.Fatalf("CreatedBy = %q, want %q", got.CreatedBy, "creator")
	}
	if got.UpdatedBy != "" {
		t.Fatalf("UpdatedBy = %q, want empty on initial create", got.UpdatedBy)
	}
}

func TestUpdateJob_SetsUpdatedBy(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "proj-updatedby")
	job.CreatedBy = "original-creator"
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	job.Name = "Updated Name"
	job.UpdatedBy = "editor-user"
	if err := q.UpdateJob(ctx, job); err != nil {
		t.Fatalf("UpdateJob() error = %v", err)
	}

	got, err := q.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetJob() error = %v", err)
	}
	if got.CreatedBy != "original-creator" {
		t.Fatalf("CreatedBy = %q, should not change after update", got.CreatedBy)
	}
	if got.UpdatedBy != "editor-user" {
		t.Fatalf("UpdatedBy = %q, want %q", got.UpdatedBy, "editor-user")
	}
}

func TestUpdateJob_VersionIncrements(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "proj-ver-inc")
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}
	if job.Version != 1 {
		t.Fatalf("initial version = %d, want 1", job.Version)
	}

	job.Name = "Update 1"
	if err := q.UpdateJob(ctx, job); err != nil {
		t.Fatalf("UpdateJob(1) error = %v", err)
	}
	got, _ := q.GetJob(ctx, job.ID)
	if got.Version != 2 {
		t.Fatalf("version after 1st update = %d, want 2", got.Version)
	}

	got.Name = "Update 2"
	if err := q.UpdateJob(ctx, got); err != nil {
		t.Fatalf("UpdateJob(2) error = %v", err)
	}
	got2, _ := q.GetJob(ctx, job.ID)
	if got2.Version != 3 {
		t.Fatalf("version after 2nd update = %d, want 3", got2.Version)
	}
}

func TestCreateJob_CustomVersionPolicy(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "proj-custom-policy")
	job.VersionPolicy = domain.VersionPolicyLatest
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	got, err := q.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetJob() error = %v", err)
	}
	if got.VersionPolicy != domain.VersionPolicyLatest {
		t.Fatalf("VersionPolicy = %q, want %q", got.VersionPolicy, domain.VersionPolicyLatest)
	}
}

func TestDeleteJob_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	err := q.DeleteJob(ctx, "nonexistent-job-id")
	if !errors.Is(err, store.ErrJobNotFound) {
		t.Fatalf("DeleteJob(nonexistent) error = %v, want ErrJobNotFound", err)
	}
}

func TestDeleteJob_NoRunsSuccess(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "proj-del-norun")
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	if err := q.DeleteJob(ctx, job.ID); err != nil {
		t.Fatalf("DeleteJob() error = %v", err)
	}

	_, err := q.GetJob(ctx, job.ID)
	if !errors.Is(err, store.ErrJobNotFound) {
		t.Fatalf("GetJob after delete error = %v, want ErrJobNotFound", err)
	}
}

func TestDeleteJob_ActiveRunsBlocked(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "proj-del-active")
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	// Create a queued run.
	run := baseRun(job, newID())
	run.Status = domain.StatusQueued
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	err := q.DeleteJob(ctx, job.ID)
	if !errors.Is(err, store.ErrJobHasActiveRuns) {
		t.Fatalf("DeleteJob(active runs) error = %v, want ErrJobHasActiveRuns", err)
	}
}

func TestDeleteJob_CompletedRunsAllowed(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "proj-del-completed")
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	// Create a completed run.
	run := baseRun(job, newID())
	run.Status = domain.StatusQueued
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}
	// Transition to completed.
	if err := q.UpdateRunStatus(ctx, run.ID, domain.StatusQueued, domain.StatusDequeued, nil); err != nil {
		t.Fatalf("UpdateRunStatus(dequeued) error = %v", err)
	}
	if err := q.UpdateRunStatus(ctx, run.ID, domain.StatusDequeued, domain.StatusExecuting, nil); err != nil {
		t.Fatalf("UpdateRunStatus(executing) error = %v", err)
	}
	if err := q.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusCompleted, nil); err != nil {
		t.Fatalf("UpdateRunStatus(completed) error = %v", err)
	}

	// Delete should succeed now (only completed runs).
	if err := q.DeleteJob(ctx, job.ID); err != nil {
		t.Fatalf("DeleteJob(completed runs) error = %v (should succeed)", err)
	}
}

func TestListJobs_IncludesNewFields(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projID := "proj-list-new-fields-" + newID()
	job := baseJob(newID(), projID)
	job.Tags = map[string]string{"env": "prod"}
	job.CreatedBy = "list-creator"
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	jobs, err := q.ListJobs(ctx, projID, 50, nil)
	if err != nil {
		t.Fatalf("ListJobs() error = %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}
	if jobs[0].VersionID == "" {
		t.Fatal("ListJobs should return version_id")
	}
	if jobs[0].CreatedBy != "list-creator" {
		t.Fatalf("CreatedBy = %q, want %q", jobs[0].CreatedBy, "list-creator")
	}
	if jobs[0].Tags == nil || jobs[0].Tags["env"] != "prod" {
		t.Fatalf("Tags = %v, want {env:prod}", jobs[0].Tags)
	}
}

func TestGetJobBySlug_IncludesNewFields(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projID := "proj-slug-fields-" + newID()
	job := baseJob(newID(), projID)
	job.Tags = map[string]string{"tier": "premium"}
	job.CreatedBy = "slug-creator"
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	got, err := q.GetJobBySlug(ctx, projID, job.Slug)
	if err != nil {
		t.Fatalf("GetJobBySlug() error = %v", err)
	}
	if got.VersionID == "" {
		t.Fatal("GetJobBySlug should return version_id")
	}
	if got.VersionPolicy != domain.VersionPolicyPin {
		t.Fatalf("VersionPolicy = %q, want %q", got.VersionPolicy, domain.VersionPolicyPin)
	}
	if got.Tags["tier"] != "premium" {
		t.Fatalf("Tags = %v", got.Tags)
	}
}

// ====================================================================
// Test hardening: Workflows with new fields
// ====================================================================

func TestCreateWorkflow_TagsPersisted(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	w := &domain.Workflow{
		ProjectID: "proj-wf-tags-persist",
		Name:      "tagged-wf",
		Slug:      "tagged-wf-" + newID(),
		Enabled:   true,
		Tags:      map[string]string{"team": "core", "env": "staging"},
	}
	if err := q.CreateWorkflow(ctx, w); err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}

	got, err := q.GetWorkflow(ctx, w.ID)
	if err != nil {
		t.Fatalf("GetWorkflow() error = %v", err)
	}
	if got.Tags == nil {
		t.Fatal("expected non-nil tags")
	}
	if got.Tags["team"] != "core" || got.Tags["env"] != "staging" {
		t.Fatalf("Tags = %v, want {team:core, env:staging}", got.Tags)
	}
}

func TestUpdateWorkflow_TagsUpdated(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	w := &domain.Workflow{
		ProjectID: "proj-wf-tags-update",
		Name:      "wf-update-tags",
		Slug:      "wf-update-tags-" + newID(),
		Enabled:   true,
		Tags:      map[string]string{"old": "value"},
	}
	if err := q.CreateWorkflow(ctx, w); err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}

	w.Tags = map[string]string{"new": "value"}
	if err := q.UpdateWorkflow(ctx, w); err != nil {
		t.Fatalf("UpdateWorkflow() error = %v", err)
	}

	got, err := q.GetWorkflow(ctx, w.ID)
	if err != nil {
		t.Fatalf("GetWorkflow() error = %v", err)
	}
	if got.Tags["new"] != "value" {
		t.Fatalf("Tags = %v, want {new:value}", got.Tags)
	}
	if _, ok := got.Tags["old"]; ok {
		t.Fatal("old tag should be replaced")
	}
}

func TestUpdateWorkflow_SetsUpdatedBy(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	w := &domain.Workflow{
		ProjectID: "proj-wf-updatedby",
		Name:      "wf-updatedby",
		Slug:      "wf-updatedby-" + newID(),
		Enabled:   true,
		CreatedBy: "creator",
	}
	if err := q.CreateWorkflow(ctx, w); err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}

	w.UpdatedBy = "editor"
	w.Name = "wf-updated"
	if err := q.UpdateWorkflow(ctx, w); err != nil {
		t.Fatalf("UpdateWorkflow() error = %v", err)
	}

	got, err := q.GetWorkflow(ctx, w.ID)
	if err != nil {
		t.Fatalf("GetWorkflow() error = %v", err)
	}
	if got.UpdatedBy != "editor" {
		t.Fatalf("UpdatedBy = %q, want %q", got.UpdatedBy, "editor")
	}
}

// ====================================================================
// Test hardening: Tags queries
// ====================================================================

func TestListJobsByTag_MultipleTagsOnJob(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projID := "proj-multi-tags-" + newID()
	job := baseJob(newID(), projID)
	job.Tags = map[string]string{"team": "core", "env": "prod", "service": "api"}
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	// Search for just "team" key.
	jobs, err := q.ListJobsByTag(ctx, projID, "team", "", 50, nil)
	if err != nil {
		t.Fatalf("ListJobsByTag(team) error = %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}

	// Search for "env":"prod".
	jobs2, err := q.ListJobsByTag(ctx, projID, "env", "prod", 50, nil)
	if err != nil {
		t.Fatalf("ListJobsByTag(env:prod) error = %v", err)
	}
	if len(jobs2) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs2))
	}

	// Search for non-existent tag.
	jobs3, err := q.ListJobsByTag(ctx, projID, "missing", "", 50, nil)
	if err != nil {
		t.Fatalf("ListJobsByTag(missing) error = %v", err)
	}
	if len(jobs3) != 0 {
		t.Fatalf("expected 0 jobs, got %d", len(jobs3))
	}
}

func TestListJobsByTag_CrossProjectIsolation(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projA := "proj-iso-a-" + newID()
	projB := "proj-iso-b-" + newID()

	jobA := baseJob(newID(), projA)
	jobA.Tags = map[string]string{"team": "core"}
	if err := q.CreateJob(ctx, jobA); err != nil {
		t.Fatalf("CreateJob(A) error = %v", err)
	}

	jobB := baseJob(newID(), projB)
	jobB.Tags = map[string]string{"team": "core"}
	if err := q.CreateJob(ctx, jobB); err != nil {
		t.Fatalf("CreateJob(B) error = %v", err)
	}

	// Query project A — should only return jobA.
	jobsA, err := q.ListJobsByTag(ctx, projA, "team", "core", 50, nil)
	if err != nil {
		t.Fatalf("ListJobsByTag(projA) error = %v", err)
	}
	if len(jobsA) != 1 {
		t.Fatalf("projA: expected 1 job, got %d", len(jobsA))
	}
	if jobsA[0].ID != jobA.ID {
		t.Fatalf("projA: got job %q, want %q", jobsA[0].ID, jobA.ID)
	}

	// Query project B — should only return jobB.
	jobsB, err := q.ListJobsByTag(ctx, projB, "team", "core", 50, nil)
	if err != nil {
		t.Fatalf("ListJobsByTag(projB) error = %v", err)
	}
	if len(jobsB) != 1 {
		t.Fatalf("projB: expected 1 job, got %d", len(jobsB))
	}
	if jobsB[0].ID != jobB.ID {
		t.Fatalf("projB: got job %q, want %q", jobsB[0].ID, jobB.ID)
	}
}

func TestListJobsByTag_SpecialCharsInTagValue(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projID := "proj-special-tags-" + newID()
	job := baseJob(newID(), projID)
	job.Tags = map[string]string{"note": "hello world: special & chars <> 🚀"}
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	jobs, err := q.ListJobsByTag(ctx, projID, "note", "hello world: special & chars <> 🚀", 50, nil)
	if err != nil {
		t.Fatalf("ListJobsByTag(special chars) error = %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job with special chars tag, got %d", len(jobs))
	}
}

func TestListWorkflowsByTag_KeyOnly(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projID := "proj-wf-tag-key-" + newID()
	w := &domain.Workflow{
		ProjectID: projID,
		Name:      "wf-tagged",
		Slug:      "wf-tagged-" + newID(),
		Enabled:   true,
		Tags:      map[string]string{"env": "staging"},
	}
	if err := q.CreateWorkflow(ctx, w); err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}

	// Key-only search.
	workflows, err := q.ListWorkflowsByTag(ctx, projID, "env", "", 50, nil)
	if err != nil {
		t.Fatalf("ListWorkflowsByTag(key-only) error = %v", err)
	}
	if len(workflows) != 1 {
		t.Fatalf("expected 1 workflow, got %d", len(workflows))
	}
}
