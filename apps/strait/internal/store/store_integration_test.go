//go:build integration

package store_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/sourcegraph/conc"

	"strait/internal/domain"
	"strait/internal/queue"
	"strait/internal/store"
	"strait/internal/testutil"
)

var (
	testDB        *testutil.TestDB
	testRedis     *testutil.TestRedis
	testRedisErr  error
	testRedisOnce sync.Once
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	var err error
	testDB, err = testutil.SetupSharedTestDB(ctx, "../../migrations", "store")
	if err != nil {
		log.Fatalf("setup test db: %v", err)
	}

	code := m.Run()
	if testRedis != nil {
		testRedis.Cleanup(ctx)
	}
	testDB.Cleanup(ctx)
	os.Exit(code)
}

func mustEnv(t *testing.T, ctx context.Context) *testutil.TestEnv {
	t.Helper()
	testRedisOnce.Do(func() {
		testRedis, testRedisErr = testutil.SetupSharedTestRedis(ctx, "store")
	})
	if testRedisErr != nil {
		t.Fatalf("setup test redis: %v", testRedisErr)
	}
	env := &testutil.TestEnv{DB: testDB, Redis: testRedis}
	if err := env.Clean(ctx); err != nil {
		t.Fatalf("clean test env: %v", err)
	}
	return env
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

func TestUpsertJobMemoryWithQuota_ConcurrentPerJobLimit(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "project-job-memory-quota-concurrent")
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	start := make(chan struct{})
	errs := make(chan error, 2)
	keys := []string{"alpha", "beta"}

	var wg conc.WaitGroup
	for _, key := range keys {
		wg.Go(func() {
			<-start
			mem := &domain.JobMemory{
				JobID:     job.ID,
				ProjectID: job.ProjectID,
				MemoryKey: key,
				Value:     json.RawMessage(`"12345678"`),
				SizeBytes: 8,
			}
			errs <- store.New(testDB.Pool).UpsertJobMemoryWithQuota(ctx, mem, 1024, 10)
		})
	}

	close(start)
	wg.Wait()
	close(errs)

	successes := 0
	quotaErrors := 0
	for err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, store.ErrJobMemoryPerJobLimitExceeded):
			quotaErrors++
		default:
			t.Fatalf("UpsertJobMemoryWithQuota() error = %v", err)
		}
	}

	if successes != 1 {
		t.Fatalf("successes = %d, want 1", successes)
	}
	if quotaErrors != 1 {
		t.Fatalf("quotaErrors = %d, want 1", quotaErrors)
	}

	total, err := q.SumJobMemorySizeBytes(ctx, job.ID)
	if err != nil {
		t.Fatalf("SumJobMemorySizeBytes() error = %v", err)
	}
	if total != 8 {
		t.Fatalf("total size = %d, want 8", total)
	}

	items, err := q.ListJobMemory(ctx, job.ID)
	if err != nil {
		t.Fatalf("ListJobMemory() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("ListJobMemory() len = %d, want 1", len(items))
	}
}

func TestUpsertJobMemoryWithQuota_ReplacingExistingKeyUsesNetDelta(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "project-job-memory-quota-replace")
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	initial := &domain.JobMemory{
		JobID:     job.ID,
		ProjectID: job.ProjectID,
		MemoryKey: "profile",
		Value:     json.RawMessage(`"123456789"`),
		SizeBytes: 9,
	}
	if err := q.UpsertJobMemoryWithQuota(ctx, initial, 1024, 10); err != nil {
		t.Fatalf("UpsertJobMemoryWithQuota(initial) error = %v", err)
	}

	replacement := &domain.JobMemory{
		JobID:     job.ID,
		ProjectID: job.ProjectID,
		MemoryKey: "profile",
		Value:     json.RawMessage(`"1234567890"`),
		SizeBytes: 10,
	}
	if err := q.UpsertJobMemoryWithQuota(ctx, replacement, 1024, 10); err != nil {
		t.Fatalf("UpsertJobMemoryWithQuota(replacement) error = %v", err)
	}
	var beforeNoopXmin string
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT xmin::text
		FROM job_memory
		WHERE job_id = $1 AND memory_key = $2`,
		job.ID,
		"profile",
	).Scan(&beforeNoopXmin); err != nil {
		t.Fatalf("query job_memory xmin before no-op: %v", err)
	}
	sameReplacement := &domain.JobMemory{
		JobID:     job.ID,
		ProjectID: job.ProjectID,
		MemoryKey: "profile",
		Value:     json.RawMessage(`"1234567890"`),
		SizeBytes: 10,
	}
	if err := q.UpsertJobMemoryWithQuota(ctx, sameReplacement, 1024, 10); err != nil {
		t.Fatalf("UpsertJobMemoryWithQuota(no-op replacement) error = %v", err)
	}
	var afterNoopXmin string
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT xmin::text
		FROM job_memory
		WHERE job_id = $1 AND memory_key = $2`,
		job.ID,
		"profile",
	).Scan(&afterNoopXmin); err != nil {
		t.Fatalf("query job_memory xmin after no-op: %v", err)
	}
	if afterNoopXmin != beforeNoopXmin {
		t.Fatalf("job_memory no-op changed xmin from %s to %s", beforeNoopXmin, afterNoopXmin)
	}

	got, err := q.GetJobMemory(ctx, job.ID, "profile")
	if err != nil {
		t.Fatalf("GetJobMemory() error = %v", err)
	}
	if got == nil {
		t.Fatal("expected memory row")
	}
	if got.SizeBytes != 10 {
		t.Fatalf("SizeBytes = %d, want 10", got.SizeBytes)
	}

	total, err := q.SumJobMemorySizeBytes(ctx, job.ID)
	if err != nil {
		t.Fatalf("SumJobMemorySizeBytes() error = %v", err)
	}
	if total != 10 {
		t.Fatalf("total size = %d, want 10", total)
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

func TestEndpointCircuitState_NewEndpointDoesNotCreateCircuitRow(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	endpoint := "https://example.com/circuit-fast-path-" + newID()
	allowed, retryAt, err := q.CanDispatchEndpoint(ctx, endpoint, time.Now().UTC())
	if err != nil {
		t.Fatalf("CanDispatchEndpoint() error = %v", err)
	}
	if !allowed {
		t.Fatal("CanDispatchEndpoint() = false, want true")
	}
	if retryAt != nil {
		t.Fatalf("retryAt = %v, want nil", retryAt)
	}

	state, err := q.GetEndpointCircuitState(ctx, endpoint)
	if err != nil {
		t.Fatalf("GetEndpointCircuitState() error = %v", err)
	}
	if state != nil {
		t.Fatalf("GetEndpointCircuitState() = %#v, want nil", state)
	}
}

func TestEndpointCircuitState_ExpiredOpenCircuitResetsOnDispatch(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	endpoint := "https://example.com/circuit-expired-" + newID()
	now := time.Now().UTC()
	if err := q.RecordEndpointCircuitFailure(ctx, endpoint, now.Add(-time.Minute), 1, time.Second); err != nil {
		t.Fatalf("RecordEndpointCircuitFailure() error = %v", err)
	}

	allowed, retryAt, err := q.CanDispatchEndpoint(ctx, endpoint, now.Add(2*time.Second))
	if err != nil {
		t.Fatalf("CanDispatchEndpoint() error = %v", err)
	}
	if !allowed {
		t.Fatal("CanDispatchEndpoint() = false, want true after expired open circuit")
	}
	if retryAt != nil {
		t.Fatalf("retryAt = %v, want nil", retryAt)
	}

	state, err := q.GetEndpointCircuitState(ctx, endpoint)
	if err != nil {
		t.Fatalf("GetEndpointCircuitState() error = %v", err)
	}
	if state == nil {
		t.Fatal("GetEndpointCircuitState() = nil, want reset state")
	}
	if state.State != domain.CircuitStateClosed {
		t.Fatalf("state = %s, want %s", state.State, domain.CircuitStateClosed)
	}
	if state.ConsecutiveFailures != 0 {
		t.Fatalf("consecutive_failures = %d, want 0", state.ConsecutiveFailures)
	}
	if state.HalfOpenUntil != nil {
		t.Fatalf("half_open_until = %v, want nil", state.HalfOpenUntil)
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

func TestDeleteJob_RemovesJobMemory(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "project-delete-job-memory")
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}
	memory := &domain.JobMemory{
		JobID:     job.ID,
		ProjectID: job.ProjectID,
		MemoryKey: "cursor",
		Value:     json.RawMessage(`{"page":1}`),
		SizeBytes: len(`{"page":1}`),
	}
	if err := q.UpsertJobMemory(ctx, memory); err != nil {
		t.Fatalf("UpsertJobMemory() error = %v", err)
	}

	if err := q.DeleteJob(ctx, job.ID); err != nil {
		t.Fatalf("DeleteJob() error = %v", err)
	}

	memories, err := q.ListJobMemory(ctx, job.ID)
	if err != nil {
		t.Fatalf("ListJobMemory() error = %v", err)
	}
	if len(memories) != 0 {
		t.Fatalf("ListJobMemory() len = %d, want 0 after job delete", len(memories))
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

// TestIdempotencyIndex_ConsolidatedShape verifies migration 000255 dropped the
// partial-on-terminal-status index and replaced it with a non-partial-on-status
// index keyed only on (job_id, idempotency_key). The shape prevents write
// amplification on terminal status flips.
func TestIdempotencyIndex_ConsolidatedShape(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	var terminalExists bool
	if err := testDB.Pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'idx_runs_idempotency_terminal')`,
	).Scan(&terminalExists); err != nil {
		t.Fatalf("query pg_indexes for idx_runs_idempotency_terminal: %v", err)
	}
	if terminalExists {
		t.Errorf("idx_runs_idempotency_terminal must be dropped by migration 000255; still present")
	}

	var newExists bool
	var indexDef string
	if err := testDB.Pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'idx_runs_idempotency'),
		        COALESCE((SELECT indexdef FROM pg_indexes WHERE indexname = 'idx_runs_idempotency'), '')`,
	).Scan(&newExists, &indexDef); err != nil {
		t.Fatalf("query pg_indexes for idx_runs_idempotency: %v", err)
	}
	if !newExists {
		t.Fatal("idx_runs_idempotency must be created by migration 000255; missing")
	}
	if !strings.Contains(indexDef, "(job_id, idempotency_key)") {
		t.Errorf("idx_runs_idempotency key shape unexpected: %s", indexDef)
	}
	if !strings.Contains(indexDef, "idempotency_key IS NOT NULL") {
		t.Errorf("idx_runs_idempotency partial predicate must be NOT NULL only: %s", indexDef)
	}
	if strings.Contains(indexDef, "status") {
		t.Errorf("idx_runs_idempotency must not be partial on status (write amplifier): %s", indexDef)
	}
}

// TestGetRunByIdempotencyKey_TerminalOutsideWindow verifies the 24h window
// in the read query: a terminal run finished more than 24h ago is not
// returned (so a new run can be triggered with the same key).
func TestGetRunByIdempotencyKey_TerminalOutsideWindow(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-idempotency-window")
	key := newID()

	run := baseRun(job, newID())
	run.IdempotencyKey = key
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	if _, err := testDB.Pool.Exec(ctx,
		`UPDATE job_runs SET status = 'completed', finished_at = NOW() - INTERVAL '25 hours' WHERE id = $1`,
		run.ID,
	); err != nil {
		t.Fatalf("backdate finished_at: %v", err)
	}

	got, err := q.GetRunByIdempotencyKey(ctx, job.ID, key)
	if err != nil {
		t.Fatalf("GetRunByIdempotencyKey() error = %v", err)
	}
	if got != nil {
		t.Fatalf("GetRunByIdempotencyKey() = %+v, want nil for terminal run finished > 24h ago", got)
	}
}

// TestGetRunByIdempotencyKey_TerminalInsideWindow is the positive counterpart
// of TerminalOutsideWindow: a terminal run that finished within the 24h
// replay window is still returned.
func TestGetRunByIdempotencyKey_TerminalInsideWindow(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-idempotency-window-fresh")
	key := newID()

	run := baseRun(job, newID())
	run.IdempotencyKey = key
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	if _, err := testDB.Pool.Exec(ctx,
		`UPDATE job_runs SET status = 'completed', finished_at = NOW() - INTERVAL '1 hour' WHERE id = $1`,
		run.ID,
	); err != nil {
		t.Fatalf("set finished_at: %v", err)
	}

	got, err := q.GetRunByIdempotencyKey(ctx, job.ID, key)
	if err != nil {
		t.Fatalf("GetRunByIdempotencyKey() error = %v", err)
	}
	if got == nil {
		t.Fatal("GetRunByIdempotencyKey() returned nil; want recent terminal run")
	}
	if got.ID != run.ID {
		t.Fatalf("GetRunByIdempotencyKey() id = %s, want %s", got.ID, run.ID)
	}
}

func TestCreateRun_IdempotencyConflict_ActiveRun(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-idem-conflict")
	key := newID()

	first := baseRun(job, newID())
	first.IdempotencyKey = key
	if err := q.CreateRun(ctx, first); err != nil {
		t.Fatalf("CreateRun(first) error = %v", err)
	}

	// Second run with same key+job while first is still queued (active) → conflict.
	second := baseRun(job, newID())
	second.IdempotencyKey = key
	err := q.CreateRun(ctx, second)
	if !errors.Is(err, domain.ErrIdempotencyConflict) {
		t.Fatalf("CreateRun(second) error = %v, want ErrIdempotencyConflict", err)
	}
}

func TestCreateRun_IdempotencyConflict_AllowsAfterTerminal(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-idem-terminal")
	key := newID()

	first := baseRun(job, newID())
	first.IdempotencyKey = key
	if err := q.CreateRun(ctx, first); err != nil {
		t.Fatalf("CreateRun(first) error = %v", err)
	}

	// Transition first to terminal state.
	if err := q.UpdateRunStatus(ctx, first.ID, domain.StatusQueued, domain.StatusDequeued, map[string]any{
		"started_at": time.Now().UTC(),
	}); err != nil {
		t.Fatalf("UpdateRunStatus(dequeued) error = %v", err)
	}
	if err := q.UpdateRunStatus(ctx, first.ID, domain.StatusDequeued, domain.StatusExecuting, map[string]any{}); err != nil {
		t.Fatalf("UpdateRunStatus(executing) error = %v", err)
	}
	if err := q.UpdateRunStatus(ctx, first.ID, domain.StatusExecuting, domain.StatusFailed, map[string]any{
		"finished_at": time.Now().UTC(),
		"error":       "done",
	}); err != nil {
		t.Fatalf("UpdateRunStatus(failed) error = %v", err)
	}

	// Second run with same key should succeed now.
	second := baseRun(job, newID())
	second.IdempotencyKey = key
	if err := q.CreateRun(ctx, second); err != nil {
		t.Fatalf("CreateRun(second) after terminal error = %v", err)
	}
}

func TestIdempotencyIndex_UniquePerJobAndKey(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	jobA := mustCreateJob(t, ctx, q, "project-idem-scope-a")
	jobB := mustCreateJob(t, ctx, q, "project-idem-scope-b")
	key := newID()

	runA := baseRun(jobA, newID())
	runA.IdempotencyKey = key
	if err := q.CreateRun(ctx, runA); err != nil {
		t.Fatalf("CreateRun(jobA) error = %v", err)
	}

	// Same key on different job → should succeed.
	runB := baseRun(jobB, newID())
	runB.IdempotencyKey = key
	if err := q.CreateRun(ctx, runB); err != nil {
		t.Fatalf("CreateRun(jobB) error = %v, want nil (different job)", err)
	}

	// Same key on same job → conflict.
	runA2 := baseRun(jobA, newID())
	runA2.IdempotencyKey = key
	err := q.CreateRun(ctx, runA2)
	if !errors.Is(err, domain.ErrIdempotencyConflict) {
		t.Fatalf("CreateRun(jobA duplicate) error = %v, want ErrIdempotencyConflict", err)
	}
}

func TestIdempotencyIndex_NullKeyAllowsDuplicates(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-idem-null")

	// Two runs with empty key (stored as NULL) on same job → both succeed.
	run1 := baseRun(job, newID())
	if err := q.CreateRun(ctx, run1); err != nil {
		t.Fatalf("CreateRun(run1) error = %v", err)
	}
	run2 := baseRun(job, newID())
	if err := q.CreateRun(ctx, run2); err != nil {
		t.Fatalf("CreateRun(run2) error = %v", err)
	}
}

func TestIdempotencyIndex_AllTerminalStatusesAllowReuse(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	terminalStatuses := []struct {
		name   string
		status domain.RunStatus
	}{
		{"failed", domain.StatusFailed},
		{"completed", domain.StatusCompleted},
		{"timed_out", domain.StatusTimedOut},
		{"canceled", domain.StatusCanceled},
	}

	for _, tt := range terminalStatuses {
		t.Run(tt.name, func(t *testing.T) {
			mustClean(t, ctx)
			job := mustCreateJob(t, ctx, q, "project-idem-"+tt.name)
			key := newID()

			run := baseRun(job, newID())
			run.IdempotencyKey = key
			if err := q.CreateRun(ctx, run); err != nil {
				t.Fatalf("CreateRun error = %v", err)
			}

			// Move through FSM to terminal.
			if err := q.UpdateRunStatus(ctx, run.ID, domain.StatusQueued, domain.StatusDequeued, map[string]any{
				"started_at": time.Now().UTC(),
			}); err != nil {
				t.Fatalf("dequeued error = %v", err)
			}
			if err := q.UpdateRunStatus(ctx, run.ID, domain.StatusDequeued, domain.StatusExecuting, map[string]any{}); err != nil {
				t.Fatalf("executing error = %v", err)
			}
			if err := q.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, tt.status, map[string]any{
				"finished_at": time.Now().UTC(),
				"error":       "test",
			}); err != nil {
				t.Fatalf("terminal error = %v", err)
			}

			// New run with same key should succeed.
			run2 := baseRun(job, newID())
			run2.IdempotencyKey = key
			if err := q.CreateRun(ctx, run2); err != nil {
				t.Fatalf("CreateRun after %s error = %v", tt.name, err)
			}
		})
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
	filtered, err := q.ListRunsByProject(ctx, projectID, &status, nil, nil, nil, nil, nil, nil, nil, 10, nil)
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

	firstPage, err := q.ListRunsByProject(ctx, projectID, nil, nil, nil, nil, nil, nil, nil, nil, 2, nil)
	if err != nil {
		t.Fatalf("ListRunsByProject() first page error = %v", err)
	}
	if len(firstPage) != 2 {
		t.Fatalf("ListRunsByProject() first page len = %d, want 2", len(firstPage))
	}
	assertTimesDesc(t, extractRunCreatedAt(firstPage))

	cursor := firstPage[len(firstPage)-1].CreatedAt
	secondPage, err := q.ListRunsByProject(ctx, projectID, nil, nil, nil, nil, nil, nil, nil, nil, 2, &cursor)
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
	filtered, err := q.ListRunsByProject(ctx, projectID, nil, &key, &value, nil, nil, nil, nil, nil, 20, nil)
	if err != nil {
		t.Fatalf("ListRunsByProject() metadata key/value error = %v", err)
	}
	if len(filtered) != 1 {
		t.Fatalf("ListRunsByProject() metadata key/value len = %d, want 1", len(filtered))
	}
	if filtered[0].ID != runProd.ID {
		t.Fatalf("ListRunsByProject() metadata key/value id = %s, want %s", filtered[0].ID, runProd.ID)
	}

	keyOnly, err := q.ListRunsByProject(ctx, projectID, nil, &key, nil, nil, nil, nil, nil, nil, 20, nil)
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

func TestUpdateRunStatusMetadataIsAppendOnly(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-update-run-status-metadata")
	run := baseRun(job, newID())
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	fields := map[string]any{
		"metadata": map[string]string{
			"snooze_count": "1",
			"phase":        "retry",
		},
	}
	if err := q.UpdateRunStatus(ctx, run.ID, domain.StatusQueued, domain.StatusDequeued, fields); err != nil {
		t.Fatalf("UpdateRunStatus() error = %v", err)
	}

	var ledgerHasSnooze bool
	if err := testDB.Pool.QueryRow(ctx, `SELECT metadata ? 'snooze_count' FROM job_runs WHERE id = $1`, run.ID).Scan(&ledgerHasSnooze); err != nil {
		t.Fatalf("query ledger metadata: %v", err)
	}
	if ledgerHasSnooze {
		t.Fatal("job_runs metadata contains transition metadata, want append-only lifecycle event")
	}

	got, err := q.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if got.Metadata["snooze_count"] != "1" || got.Metadata["phase"] != "retry" {
		t.Fatalf("metadata = %+v, want lifecycle metadata overlay", got.Metadata)
	}

	key := "snooze_count"
	value := "1"
	listed, err := q.ListRunsByProject(ctx, run.ProjectID, nil, &key, &value, nil, nil, nil, nil, nil, 10, nil)
	if err != nil {
		t.Fatalf("ListRunsByProject() error = %v", err)
	}
	if len(listed) != 1 || listed[0].ID != run.ID {
		t.Fatalf("ListRunsByProject() = %+v, want run %s", listed, run.ID)
	}
}

func TestUpdateRunStatusReturningOld(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-update-run-status-old")
	run := baseRun(job, newID())
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	oldStatus, err := q.UpdateRunStatusReturningOld(ctx, run.ID, domain.StatusQueued, domain.StatusDequeued, nil)
	if err != nil {
		t.Fatalf("UpdateRunStatusReturningOld() error = %v", err)
	}
	if oldStatus != domain.StatusQueued {
		t.Fatalf("old status = %q, want %q", oldStatus, domain.StatusQueued)
	}

	got, err := q.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if got.Status != domain.StatusDequeued {
		t.Fatalf("status = %q, want %q", got.Status, domain.StatusDequeued)
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
	oldCompleted.Status = domain.StatusExecuting
	finishedOldCompleted := now.Add(-31 * 24 * time.Hour)
	if err := q.CreateRun(ctx, oldCompleted); err != nil {
		t.Fatalf("CreateRun() oldCompleted error = %v", err)
	}
	if err := q.UpdateRunStatus(ctx, oldCompleted.ID, domain.StatusExecuting, domain.StatusCompleted, map[string]any{
		"finished_at": finishedOldCompleted,
	}); err != nil {
		t.Fatalf("UpdateRunStatus() oldCompleted error = %v", err)
	}
	// Backdate created_at so the run is in a cold partition (the reaper's
	// hot-partition filter skips the current month).
	if _, err := testDB.Pool.Exec(ctx, `UPDATE job_runs SET created_at = $1 WHERE id = $2`, finishedOldCompleted, oldCompleted.ID); err != nil {
		t.Fatalf("backdate oldCompleted: %v", err)
	}

	oldTimedOut := baseRun(job, newID())
	oldTimedOut.Status = domain.StatusTimedOut
	finishedOldTimedOut := now.Add(-91 * 24 * time.Hour)
	oldTimedOut.FinishedAt = &finishedOldTimedOut
	if err := q.CreateRun(ctx, oldTimedOut); err != nil {
		t.Fatalf("CreateRun() oldTimedOut error = %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `UPDATE job_runs SET created_at = $1 WHERE id = $2`, finishedOldTimedOut, oldTimedOut.ID); err != nil {
		t.Fatalf("backdate oldTimedOut: %v", err)
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

	seedRetentionSideRows(t, ctx, oldCompleted.ID, oldTimedOut.ID, recentCompleted.ID, queued.ID)

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

	for _, runID := range []string{oldCompleted.ID, oldTimedOut.ID} {
		assertNoRunRetentionSideRows(t, ctx, runID)
	}
	for _, runID := range []string{recentCompleted.ID, queued.ID} {
		assertRunRetentionSideRowsRemain(t, ctx, runID)
	}
}

func TestDeleteTerminalRunsPastRetention_BatchCleansSideRows(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-delete-retention-batch")
	const prefix = "retention-batch-run-"
	const runCount = 5101
	finishedAt := time.Now().UTC().Add(-45 * 24 * time.Hour)

	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_runs (id, job_id, project_id, status, attempt, triggered_by, created_at, finished_at)
		SELECT $1 || gs::TEXT, $2, $3, 'completed', 1, 'manual', $4, $4
		FROM generate_series(1, $5) AS gs`, prefix, job.ID, job.ProjectID, finishedAt, runCount); err != nil {
		t.Fatalf("seed batch job_runs: %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_run_active_claims (run_id, ready_generation, attempt)
		SELECT run_id, ready_generation, attempt
		FROM job_run_state
		WHERE run_id LIKE $1 || '%'`, prefix); err != nil {
		t.Fatalf("seed batch active claims: %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_run_lifecycle_events (run_id, from_status, to_status, attempt, fields)
		SELECT run_id, 'queued', 'completed', attempt, '{"source":"retention-batch"}'::jsonb
		FROM job_run_state
		WHERE run_id LIKE $1 || '%'`, prefix); err != nil {
		t.Fatalf("seed batch lifecycle events: %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_run_ready_events (run_id, ready_generation, attempt, reason)
		SELECT run_id, ready_generation, attempt, 'retention_batch'
		FROM job_run_state
		WHERE run_id LIKE $1 || '%'
		ON CONFLICT DO NOTHING`, prefix); err != nil {
		t.Fatalf("seed batch ready events: %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_retries (run_id, next_retry_at, attempt, scheduled_at, cleared)
		SELECT run_id, NOW() + INTERVAL '1 minute', attempt + 1, NOW(), FALSE
		FROM job_run_state
		WHERE run_id LIKE $1 || '%'`, prefix); err != nil {
		t.Fatalf("seed batch retries: %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_run_priority_events (run_id, priority)
		SELECT run_id, priority + 1
		FROM job_run_state
		WHERE run_id LIKE $1 || '%'`, prefix); err != nil {
		t.Fatalf("seed batch priority events: %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_run_visibility_events (run_id, visible_until)
		SELECT run_id, NOW() + INTERVAL '1 hour'
		FROM job_run_state
		WHERE run_id LIKE $1 || '%'`, prefix); err != nil {
		t.Fatalf("seed batch visibility events: %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_run_cache_versions (run_id, cache_version)
		SELECT run_id, 2
		FROM job_run_state
		WHERE run_id LIKE $1 || '%'`, prefix); err != nil {
		t.Fatalf("seed batch cache versions: %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_run_heartbeats (run_id, heartbeat_at)
		SELECT run_id, NOW()
		FROM job_run_state
		WHERE run_id LIKE $1 || '%'
		ON CONFLICT DO NOTHING`, prefix); err != nil {
		t.Fatalf("seed batch heartbeats: %v", err)
	}

	for _, table := range []string{"job_runs", "job_run_state", "job_run_active_claims", "job_run_lifecycle_events", "job_run_ready_events", "job_retries", "job_run_priority_events", "job_run_visibility_events", "job_run_cache_versions", "job_run_heartbeats"} {
		if count := countRunRowsByPrefix(t, ctx, table, prefix); count != runCount {
			t.Fatalf("%s seeded rows = %d, want %d", table, count, runCount)
		}
	}

	deleted, err := q.DeleteTerminalRunsPastRetention(ctx, 30*24*time.Hour, 90*24*time.Hour)
	if err != nil {
		t.Fatalf("DeleteTerminalRunsPastRetention(first) error = %v", err)
	}
	if deleted != 5000 {
		t.Fatalf("first deleted = %d, want 5000", deleted)
	}

	for _, table := range []string{"job_runs", "job_run_state", "job_run_active_claims", "job_run_lifecycle_events", "job_run_ready_events", "job_retries", "job_run_priority_events", "job_run_visibility_events", "job_run_cache_versions", "job_run_heartbeats"} {
		if count := countRunRowsByPrefix(t, ctx, table, prefix); count != runCount-deleted {
			t.Fatalf("%s rows after first batch = %d, want %d", table, count, runCount-deleted)
		}
	}

	deleted, err = q.DeleteTerminalRunsPastRetention(ctx, 30*24*time.Hour, 90*24*time.Hour)
	if err != nil {
		t.Fatalf("DeleteTerminalRunsPastRetention(second) error = %v", err)
	}
	if deleted != runCount-5000 {
		t.Fatalf("second deleted = %d, want %d", deleted, runCount-5000)
	}

	for _, table := range []string{"job_runs", "job_run_state", "job_run_active_claims", "job_run_lifecycle_events", "job_run_ready_events", "job_retries", "job_run_priority_events", "job_run_visibility_events", "job_run_cache_versions", "job_run_heartbeats"} {
		if count := countRunRowsByPrefix(t, ctx, table, prefix); count != 0 {
			t.Fatalf("%s rows after second batch = %d, want 0", table, count)
		}
	}
}

func seedRetentionSideRows(t *testing.T, ctx context.Context, runIDs ...string) {
	t.Helper()

	for _, runID := range runIDs {
		if _, err := testDB.Pool.Exec(ctx, `
			INSERT INTO job_run_active_claims (run_id, ready_generation, attempt)
			VALUES ($1, 0, 1)
			ON CONFLICT DO NOTHING`, runID); err != nil {
			t.Fatalf("seed active claim for %s: %v", runID, err)
		}
		if _, err := testDB.Pool.Exec(ctx, `
			INSERT INTO job_run_lifecycle_events (run_id, from_status, to_status, attempt, fields)
			VALUES ($1, 'queued', 'executing', 1, '{"source":"retention-test"}'::jsonb)`, runID); err != nil {
			t.Fatalf("seed lifecycle event for %s: %v", runID, err)
		}
		if _, err := testDB.Pool.Exec(ctx, `
			INSERT INTO job_run_ready_events (run_id, ready_generation, attempt, reason)
			VALUES ($1, 0, 1, 'retention_test')
			ON CONFLICT DO NOTHING`, runID); err != nil {
			t.Fatalf("seed ready event for %s: %v", runID, err)
		}
		if _, err := testDB.Pool.Exec(ctx, `
			INSERT INTO job_retries (run_id, next_retry_at, attempt, scheduled_at, cleared)
			VALUES ($1, NOW() + INTERVAL '1 minute', 2, NOW(), FALSE)`, runID); err != nil {
			t.Fatalf("seed retry for %s: %v", runID, err)
		}
		if _, err := testDB.Pool.Exec(ctx, `
			INSERT INTO job_run_priority_events (run_id, priority)
			VALUES ($1, 10)`, runID); err != nil {
			t.Fatalf("seed priority event for %s: %v", runID, err)
		}
		if _, err := testDB.Pool.Exec(ctx, `
			INSERT INTO job_run_visibility_events (run_id, visible_until)
			VALUES ($1, NOW() + INTERVAL '1 hour')`, runID); err != nil {
			t.Fatalf("seed visibility event for %s: %v", runID, err)
		}
		if _, err := testDB.Pool.Exec(ctx, `
			INSERT INTO job_run_cache_versions (run_id, cache_version)
			VALUES ($1, 2)`, runID); err != nil {
			t.Fatalf("seed cache version for %s: %v", runID, err)
		}
		if _, err := testDB.Pool.Exec(ctx, `
			INSERT INTO job_run_heartbeats (run_id, heartbeat_at)
			VALUES ($1, NOW())
			ON CONFLICT DO NOTHING`, runID); err != nil {
			t.Fatalf("seed heartbeat for %s: %v", runID, err)
		}
	}
}

func assertNoRunRetentionSideRows(t *testing.T, ctx context.Context, runID string) {
	t.Helper()

	for _, table := range []string{
		"job_run_state",
		"job_run_terminal_state",
		"job_run_active_claims",
		"job_run_lifecycle_events",
		"job_run_ready_events",
		"job_retries",
		"job_run_priority_events",
		"job_run_visibility_events",
		"job_run_cache_versions",
		"job_run_heartbeats",
	} {
		if count := countRunSideTableRows(t, ctx, table, runID); count != 0 {
			t.Fatalf("%s rows for deleted run %s = %d, want 0", table, runID, count)
		}
	}
}

func assertRunRetentionSideRowsRemain(t *testing.T, ctx context.Context, runID string) {
	t.Helper()

	for _, table := range []string{
		"job_run_state",
		"job_run_active_claims",
		"job_run_lifecycle_events",
		"job_run_ready_events",
		"job_retries",
		"job_run_priority_events",
		"job_run_visibility_events",
		"job_run_cache_versions",
		"job_run_heartbeats",
	} {
		if count := countRunSideTableRows(t, ctx, table, runID); count == 0 {
			t.Fatalf("%s rows for retained run %s = 0, want at least 1", table, runID)
		}
	}
}

func countRunSideTableRows(t *testing.T, ctx context.Context, table, runID string) int64 {
	t.Helper()

	var query string
	switch table {
	case "job_run_state":
		query = `SELECT COUNT(*) FROM job_run_state WHERE run_id = $1`
	case "job_run_terminal_state":
		query = `SELECT COUNT(*) FROM job_run_terminal_state WHERE run_id = $1`
	case "job_run_active_claims":
		query = `SELECT COUNT(*) FROM job_run_active_claims WHERE run_id = $1`
	case "job_run_lifecycle_events":
		query = `SELECT COUNT(*) FROM job_run_lifecycle_events WHERE run_id = $1`
	case "job_run_ready_events":
		query = `SELECT COUNT(*) FROM job_run_ready_events WHERE run_id = $1`
	case "job_retries":
		query = `SELECT COUNT(*) FROM job_retries WHERE run_id = $1`
	case "job_run_priority_events":
		query = `SELECT COUNT(*) FROM job_run_priority_events WHERE run_id = $1`
	case "job_run_visibility_events":
		query = `SELECT COUNT(*) FROM job_run_visibility_events WHERE run_id = $1`
	case "job_run_cache_versions":
		query = `SELECT COUNT(*) FROM job_run_cache_versions WHERE run_id = $1`
	case "job_run_heartbeats":
		query = `SELECT COUNT(*) FROM job_run_heartbeats WHERE run_id = $1`
	default:
		t.Fatalf("unknown side table %q", table)
	}

	var count int64
	if err := testDB.Pool.QueryRow(ctx, query, runID).Scan(&count); err != nil {
		t.Fatalf("count %s rows for %s: %v", table, runID, err)
	}
	return count
}

func countRunRowsByPrefix(t *testing.T, ctx context.Context, table, prefix string) int64 {
	t.Helper()

	var query string
	switch table {
	case "job_runs":
		query = `SELECT COUNT(*) FROM job_runs WHERE id LIKE $1 || '%'`
	case "job_run_state":
		query = `SELECT COUNT(*) FROM job_run_state WHERE run_id LIKE $1 || '%'`
	case "job_run_active_claims":
		query = `SELECT COUNT(*) FROM job_run_active_claims WHERE run_id LIKE $1 || '%'`
	case "job_run_lifecycle_events":
		query = `SELECT COUNT(*) FROM job_run_lifecycle_events WHERE run_id LIKE $1 || '%'`
	case "job_run_ready_events":
		query = `SELECT COUNT(*) FROM job_run_ready_events WHERE run_id LIKE $1 || '%'`
	case "job_retries":
		query = `SELECT COUNT(*) FROM job_retries WHERE run_id LIKE $1 || '%'`
	case "job_run_priority_events":
		query = `SELECT COUNT(*) FROM job_run_priority_events WHERE run_id LIKE $1 || '%'`
	case "job_run_visibility_events":
		query = `SELECT COUNT(*) FROM job_run_visibility_events WHERE run_id LIKE $1 || '%'`
	case "job_run_cache_versions":
		query = `SELECT COUNT(*) FROM job_run_cache_versions WHERE run_id LIKE $1 || '%'`
	case "job_run_heartbeats":
		query = `SELECT COUNT(*) FROM job_run_heartbeats WHERE run_id LIKE $1 || '%'`
	default:
		t.Fatalf("unknown run table %q", table)
	}

	var count int64
	if err := testDB.Pool.QueryRow(ctx, query, prefix).Scan(&count); err != nil {
		t.Fatalf("count %s rows with prefix %s: %v", table, prefix, err)
	}
	return count
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

func TestSDKActiveRunMutationsRequireActiveAttempt(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-sdk-active-mutations")
	activeRun := mustCreateRun(t, ctx, q, job)
	if err := q.UpdateRunStatus(ctx, activeRun.ID, domain.StatusQueued, domain.StatusDequeued, nil); err != nil {
		t.Fatalf("dequeue active run: %v", err)
	}
	if err := q.UpdateRunStatus(ctx, activeRun.ID, domain.StatusDequeued, domain.StatusExecuting, nil); err != nil {
		t.Fatalf("execute active run: %v", err)
	}

	event := &domain.RunEvent{RunID: activeRun.ID, Type: domain.EventLog, Level: "info", Message: "active event", Data: []byte(`{"ok":true}`)}
	if err := q.InsertEventForActiveRun(ctx, event, activeRun.Attempt); err != nil {
		t.Fatalf("InsertEventForActiveRun(active) error = %v", err)
	}
	if event.CreatedAt.IsZero() {
		t.Fatal("InsertEventForActiveRun(active) did not set CreatedAt")
	}

	if err := q.UpdateRunMetadataForActiveRun(ctx, activeRun.ID, map[string]string{"sdk": "active"}, activeRun.Attempt); err != nil {
		t.Fatalf("UpdateRunMetadataForActiveRun(active) error = %v", err)
	}
	if err := q.UpdateRunMetadataForActiveRun(ctx, activeRun.ID, map[string]string{"sdk": "active-v2", "phase": "two"}, activeRun.Attempt); err != nil {
		t.Fatalf("UpdateRunMetadataForActiveRun(active overwrite) error = %v", err)
	}
	var metadataEventsBeforeNoop int
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM job_run_lifecycle_events
		WHERE run_id = $1
		  AND fields ? 'metadata'`,
		activeRun.ID,
	).Scan(&metadataEventsBeforeNoop); err != nil {
		t.Fatalf("query active metadata events before no-op: %v", err)
	}
	var cacheVersionsBeforeNoop int
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM job_run_cache_versions
		WHERE run_id = $1`,
		activeRun.ID,
	).Scan(&cacheVersionsBeforeNoop); err != nil {
		t.Fatalf("query cache versions before active metadata no-op: %v", err)
	}
	if err := q.UpdateRunMetadataForActiveRun(ctx, activeRun.ID, map[string]string{"sdk": "active-v2", "phase": "two"}, activeRun.Attempt); err != nil {
		t.Fatalf("UpdateRunMetadataForActiveRun(active no-op) error = %v", err)
	}
	var metadataEventsAfterNoop int
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM job_run_lifecycle_events
		WHERE run_id = $1
		  AND fields ? 'metadata'`,
		activeRun.ID,
	).Scan(&metadataEventsAfterNoop); err != nil {
		t.Fatalf("query active metadata events after no-op: %v", err)
	}
	if metadataEventsAfterNoop != metadataEventsBeforeNoop {
		t.Fatalf("active metadata no-op events = %d, want %d", metadataEventsAfterNoop, metadataEventsBeforeNoop)
	}
	var cacheVersionsAfterNoop int
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM job_run_cache_versions
		WHERE run_id = $1`,
		activeRun.ID,
	).Scan(&cacheVersionsAfterNoop); err != nil {
		t.Fatalf("query cache versions after active metadata no-op: %v", err)
	}
	if cacheVersionsAfterNoop != cacheVersionsBeforeNoop {
		t.Fatalf("active metadata no-op cache versions = %d, want %d", cacheVersionsAfterNoop, cacheVersionsBeforeNoop)
	}
	if err := q.UpdateHeartbeatForActiveRun(ctx, activeRun.ID, activeRun.Attempt); err != nil {
		t.Fatalf("UpdateHeartbeatForActiveRun(active) error = %v", err)
	}
	checkpoint := &domain.RunCheckpoint{RunID: activeRun.ID, Source: "sdk", State: json.RawMessage(`{"cursor":1}`)}
	if err := q.CreateRunCheckpointForActiveRun(ctx, checkpoint, activeRun.Attempt); err != nil {
		t.Fatalf("CreateRunCheckpointForActiveRun(active) error = %v", err)
	}
	if checkpoint.Sequence != 1 {
		t.Fatalf("checkpoint sequence = %d, want 1", checkpoint.Sequence)
	}
	state := &domain.RunState{RunID: activeRun.ID, StateKey: "cursor", Value: json.RawMessage(`{"step":1}`)}
	if err := q.UpsertRunStateForActiveRun(ctx, state, activeRun.Attempt); err != nil {
		t.Fatalf("UpsertRunStateForActiveRun(active) error = %v", err)
	}
	initialStateUpdatedAt := state.UpdatedAt
	var stateXminBeforeNoop string
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT xmin::text
		FROM run_state
		WHERE run_id = $1 AND state_key = $2`,
		activeRun.ID,
		"cursor",
	).Scan(&stateXminBeforeNoop); err != nil {
		t.Fatalf("query active run_state xmin before no-op: %v", err)
	}
	sameState := &domain.RunState{RunID: activeRun.ID, StateKey: "cursor", Value: json.RawMessage(`{"step":1}`)}
	if err := q.UpsertRunStateForActiveRun(ctx, sameState, activeRun.Attempt); err != nil {
		t.Fatalf("UpsertRunStateForActiveRun(active no-op) error = %v", err)
	}
	var stateXminAfterNoop string
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT xmin::text
		FROM run_state
		WHERE run_id = $1 AND state_key = $2`,
		activeRun.ID,
		"cursor",
	).Scan(&stateXminAfterNoop); err != nil {
		t.Fatalf("query active run_state xmin after no-op: %v", err)
	}
	if stateXminAfterNoop != stateXminBeforeNoop {
		t.Fatalf("active run_state no-op changed xmin from %s to %s", stateXminBeforeNoop, stateXminAfterNoop)
	}
	if !sameState.UpdatedAt.Equal(initialStateUpdatedAt) {
		t.Fatalf("active run_state no-op updated_at = %v, want %v", sameState.UpdatedAt, initialStateUpdatedAt)
	}
	output := &domain.RunOutput{RunID: activeRun.ID, OutputKey: "final", Value: json.RawMessage(`{"ok":true}`)}
	if err := q.UpsertRunOutputForActiveRun(ctx, output, activeRun.Attempt); err != nil {
		t.Fatalf("UpsertRunOutputForActiveRun(active) error = %v", err)
	}
	initialOutputID := output.ID
	initialOutputCreatedAt := output.CreatedAt
	var outputXminBeforeNoop string
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT xmin::text
		FROM run_outputs
		WHERE run_id = $1 AND output_key = $2`,
		activeRun.ID,
		"final",
	).Scan(&outputXminBeforeNoop); err != nil {
		t.Fatalf("query active run_outputs xmin before no-op: %v", err)
	}
	sameOutput := &domain.RunOutput{RunID: activeRun.ID, OutputKey: "final", Value: json.RawMessage(`{"ok":true}`)}
	if err := q.UpsertRunOutputForActiveRun(ctx, sameOutput, activeRun.Attempt); err != nil {
		t.Fatalf("UpsertRunOutputForActiveRun(active no-op) error = %v", err)
	}
	var outputXminAfterNoop string
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT xmin::text
		FROM run_outputs
		WHERE run_id = $1 AND output_key = $2`,
		activeRun.ID,
		"final",
	).Scan(&outputXminAfterNoop); err != nil {
		t.Fatalf("query active run_outputs xmin after no-op: %v", err)
	}
	if outputXminAfterNoop != outputXminBeforeNoop {
		t.Fatalf("active run_outputs no-op changed xmin from %s to %s", outputXminBeforeNoop, outputXminAfterNoop)
	}
	if sameOutput.ID != initialOutputID {
		t.Fatalf("active run_outputs no-op id = %q, want %q", sameOutput.ID, initialOutputID)
	}
	if !sameOutput.CreatedAt.Equal(initialOutputCreatedAt) {
		t.Fatalf("active run_outputs no-op created_at = %v, want %v", sameOutput.CreatedAt, initialOutputCreatedAt)
	}
	resourceSnapshot := &domain.RunResourceSnapshot{RunID: activeRun.ID, CPUPercent: 10, MemoryMB: 128}
	if err := q.CreateRunResourceSnapshotForActiveRun(ctx, resourceSnapshot, activeRun.Attempt); err != nil {
		t.Fatalf("CreateRunResourceSnapshotForActiveRun(active) error = %v", err)
	}
	iteration := &domain.RunIteration{RunID: activeRun.ID, Iteration: 1, Description: "active iteration"}
	if err := q.CreateRunIterationForActiveRun(ctx, iteration, activeRun.Attempt); err != nil {
		t.Fatalf("CreateRunIterationForActiveRun(active) error = %v", err)
	}
	memory := &domain.JobMemory{JobID: activeRun.JobID, ProjectID: activeRun.ProjectID, MemoryKey: "cursor", Value: json.RawMessage(`{"step":1}`), SizeBytes: len(`{"step":1}`)}
	if err := q.UpsertJobMemoryWithQuotaForActiveRun(ctx, activeRun.ID, memory, 1024, 1024, activeRun.Attempt); err != nil {
		t.Fatalf("UpsertJobMemoryWithQuotaForActiveRun(active) error = %v", err)
	}
	var memoryXminBeforeNoop string
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT xmin::text
		FROM job_memory
		WHERE job_id = $1 AND memory_key = $2`,
		activeRun.JobID,
		"cursor",
	).Scan(&memoryXminBeforeNoop); err != nil {
		t.Fatalf("query active job_memory xmin before no-op: %v", err)
	}
	sameMemory := &domain.JobMemory{JobID: activeRun.JobID, ProjectID: activeRun.ProjectID, MemoryKey: "cursor", Value: json.RawMessage(`{"step":1}`), SizeBytes: len(`{"step":1}`)}
	if err := q.UpsertJobMemoryWithQuotaForActiveRun(ctx, activeRun.ID, sameMemory, 1024, 1024, activeRun.Attempt); err != nil {
		t.Fatalf("UpsertJobMemoryWithQuotaForActiveRun(active no-op) error = %v", err)
	}
	var memoryXminAfterNoop string
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT xmin::text
		FROM job_memory
		WHERE job_id = $1 AND memory_key = $2`,
		activeRun.JobID,
		"cursor",
	).Scan(&memoryXminAfterNoop); err != nil {
		t.Fatalf("query active job_memory xmin after no-op: %v", err)
	}
	if memoryXminAfterNoop != memoryXminBeforeNoop {
		t.Fatalf("active job_memory no-op changed xmin from %s to %s", memoryXminBeforeNoop, memoryXminAfterNoop)
	}
	if err := q.DeleteRunStateForActiveRun(ctx, activeRun.ID, "cursor", activeRun.Attempt); err != nil {
		t.Fatalf("DeleteRunStateForActiveRun(active) error = %v", err)
	}
	if err := q.DeleteJobMemoryForActiveRun(ctx, activeRun.ID, activeRun.JobID, "cursor", activeRun.Attempt); err != nil {
		t.Fatalf("DeleteJobMemoryForActiveRun(active) error = %v", err)
	}

	var ledgerHasSDK bool
	if err := testDB.Pool.QueryRow(ctx, `SELECT metadata ? 'sdk' FROM job_runs WHERE id = $1`, activeRun.ID).Scan(&ledgerHasSDK); err != nil {
		t.Fatalf("query ledger metadata: %v", err)
	}
	if ledgerHasSDK {
		t.Fatal("job_runs metadata contains sdk key, want active metadata stored append-only")
	}

	stored, err := q.GetRun(ctx, activeRun.ID)
	if err != nil {
		t.Fatalf("GetRun(active) error = %v", err)
	}
	if stored.Metadata["sdk"] != "active-v2" || stored.Metadata["phase"] != "two" {
		t.Fatalf("metadata = %+v, want append-only active metadata overlay", stored.Metadata)
	}
	cachedRun, _, err := q.GetRunWithCacheVersion(ctx, activeRun.ID)
	if err != nil {
		t.Fatalf("GetRunWithCacheVersion(active) error = %v", err)
	}
	if cachedRun.Metadata["sdk"] != "active-v2" || cachedRun.Metadata["phase"] != "two" {
		t.Fatalf("cached metadata = %+v, want append-only active metadata overlay", cachedRun.Metadata)
	}
	byIDs, err := q.GetRunsByIDs(ctx, []string{activeRun.ID})
	if err != nil {
		t.Fatalf("GetRunsByIDs(active) error = %v", err)
	}
	if byIDs[activeRun.ID].Metadata["sdk"] != "active-v2" || byIDs[activeRun.ID].Metadata["phase"] != "two" {
		t.Fatalf("GetRunsByIDs metadata = %+v, want append-only active metadata overlay", byIDs[activeRun.ID].Metadata)
	}
	metadataKey := "sdk"
	metadataValue := "active-v2"
	listed, err := q.ListRunsByProject(ctx, activeRun.ProjectID, nil, &metadataKey, &metadataValue, nil, nil, nil, nil, nil, 10, nil)
	if err != nil {
		t.Fatalf("ListRunsByProject(active metadata filter) error = %v", err)
	}
	if len(listed) != 1 || listed[0].ID != activeRun.ID {
		t.Fatalf("ListRunsByProject(active metadata filter) = %+v, want active run %s", listed, activeRun.ID)
	}
	filtered, err := q.ListRunsByProjectFiltered(ctx, activeRun.ProjectID, nil, nil, "", "", nil, &metadataKey, &metadataValue, nil, nil, nil, nil, nil, 10, nil)
	if err != nil {
		t.Fatalf("ListRunsByProjectFiltered(active metadata filter) error = %v", err)
	}
	if len(filtered) != 1 || filtered[0].ID != activeRun.ID {
		t.Fatalf("ListRunsByProjectFiltered(active metadata filter) = %+v, want active run %s", filtered, activeRun.ID)
	}
	if stored.HeartbeatAt == nil {
		t.Fatal("heartbeat_at was not updated")
	}

	if err := q.InsertEventForActiveRun(ctx, &domain.RunEvent{RunID: activeRun.ID, Type: domain.EventLog, Message: "stale"}, activeRun.Attempt+1); !errors.Is(err, store.ErrRunConflict) {
		t.Fatalf("InsertEventForActiveRun(stale attempt) error = %v, want ErrRunConflict", err)
	}
	if err := q.EnsureRunActiveForAttempt(ctx, activeRun.ID, activeRun.Attempt+1); !errors.Is(err, store.ErrRunConflict) {
		t.Fatalf("EnsureRunActiveForAttempt(stale attempt) error = %v, want ErrRunConflict", err)
	}
	if err := q.CreateRunResourceSnapshotForActiveRun(ctx, &domain.RunResourceSnapshot{RunID: activeRun.ID}, activeRun.Attempt+1); !errors.Is(err, store.ErrRunConflict) {
		t.Fatalf("CreateRunResourceSnapshotForActiveRun(stale attempt) error = %v, want ErrRunConflict", err)
	}
	if err := q.CreateRunIterationForActiveRun(ctx, &domain.RunIteration{RunID: activeRun.ID, Iteration: 2}, activeRun.Attempt+1); !errors.Is(err, store.ErrRunConflict) {
		t.Fatalf("CreateRunIterationForActiveRun(stale attempt) error = %v, want ErrRunConflict", err)
	}
	if err := q.UpsertJobMemoryWithQuotaForActiveRun(ctx, activeRun.ID, &domain.JobMemory{JobID: activeRun.JobID, ProjectID: activeRun.ProjectID, MemoryKey: "stale", Value: json.RawMessage(`true`), SizeBytes: 4}, 1024, 1024, activeRun.Attempt+1); !errors.Is(err, store.ErrRunConflict) {
		t.Fatalf("UpsertJobMemoryWithQuotaForActiveRun(stale attempt) error = %v, want ErrRunConflict", err)
	}

	terminalRun := mustCreateRun(t, ctx, q, job)
	if err := q.UpdateRunStatus(ctx, terminalRun.ID, domain.StatusQueued, domain.StatusDequeued, nil); err != nil {
		t.Fatalf("dequeue terminal run: %v", err)
	}
	if err := q.UpdateRunStatus(ctx, terminalRun.ID, domain.StatusDequeued, domain.StatusExecuting, nil); err != nil {
		t.Fatalf("execute terminal run: %v", err)
	}
	if err := q.UpdateRunStatus(ctx, terminalRun.ID, domain.StatusExecuting, domain.StatusCompleted, map[string]any{"finished_at": time.Now()}); err != nil {
		t.Fatalf("complete terminal run: %v", err)
	}

	for name, err := range map[string]error{
		"event":        q.InsertEventForActiveRun(ctx, &domain.RunEvent{RunID: terminalRun.ID, Type: domain.EventLog, Message: "late"}, terminalRun.Attempt),
		"metadata":     q.UpdateRunMetadataForActiveRun(ctx, terminalRun.ID, map[string]string{"late": "true"}, terminalRun.Attempt),
		"heartbeat":    q.UpdateHeartbeatForActiveRun(ctx, terminalRun.ID, terminalRun.Attempt),
		"checkpoint":   q.CreateRunCheckpointForActiveRun(ctx, &domain.RunCheckpoint{RunID: terminalRun.ID, State: json.RawMessage(`{"late":true}`)}, terminalRun.Attempt),
		"state":        q.UpsertRunStateForActiveRun(ctx, &domain.RunState{RunID: terminalRun.ID, StateKey: "late", Value: json.RawMessage(`true`)}, terminalRun.Attempt),
		"output":       q.UpsertRunOutputForActiveRun(ctx, &domain.RunOutput{RunID: terminalRun.ID, OutputKey: "final", Value: json.RawMessage(`true`)}, terminalRun.Attempt),
		"delete-state": q.DeleteRunStateForActiveRun(ctx, terminalRun.ID, "late", terminalRun.Attempt),
		"memory": q.UpsertJobMemoryWithQuotaForActiveRun(ctx, terminalRun.ID, &domain.JobMemory{
			JobID: terminalRun.JobID, ProjectID: terminalRun.ProjectID, MemoryKey: "late", Value: json.RawMessage(`true`), SizeBytes: 4,
		}, 1024, 1024, terminalRun.Attempt),
		"delete-memory":     q.DeleteJobMemoryForActiveRun(ctx, terminalRun.ID, terminalRun.JobID, "late", terminalRun.Attempt),
		"resource-snapshot": q.CreateRunResourceSnapshotForActiveRun(ctx, &domain.RunResourceSnapshot{RunID: terminalRun.ID}, terminalRun.Attempt),
		"iteration":         q.CreateRunIterationForActiveRun(ctx, &domain.RunIteration{RunID: terminalRun.ID, Iteration: 1}, terminalRun.Attempt),
		"ensure":            q.EnsureRunActiveForAttempt(ctx, terminalRun.ID, terminalRun.Attempt),
	} {
		if !errors.Is(err, store.ErrRunConflict) {
			t.Fatalf("%s terminal mutation error = %v, want ErrRunConflict", name, err)
		}
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

func TestRunCheckpointsConcurrentSequenceAllocation(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-run-checkpoints-concurrent")
	run := mustCreateRun(t, ctx, q, job)
	if err := q.UpdateRunStatus(ctx, run.ID, domain.StatusQueued, domain.StatusDequeued, nil); err != nil {
		t.Fatalf("UpdateRunStatus(dequeued) error = %v", err)
	}
	if err := q.UpdateRunStatus(ctx, run.ID, domain.StatusDequeued, domain.StatusExecuting, nil); err != nil {
		t.Fatalf("UpdateRunStatus(executing) error = %v", err)
	}

	const checkpointCount = 32
	errs := make(chan error, checkpointCount)
	var wg sync.WaitGroup
	for i := range checkpointCount {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			checkpoint := &domain.RunCheckpoint{
				RunID:  run.ID,
				Source: "sdk",
				State:  json.RawMessage(`{"cursor":` + strconv.Itoa(i) + `}`),
			}
			errs <- q.CreateRunCheckpointForActiveRun(ctx, checkpoint, run.Attempt)
		}(i)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("CreateRunCheckpointForActiveRun() concurrent error = %v", err)
		}
	}

	checkpoints, err := q.ListRunCheckpoints(ctx, run.ID, checkpointCount, nil)
	if err != nil {
		t.Fatalf("ListRunCheckpoints() error = %v", err)
	}
	if len(checkpoints) != checkpointCount {
		t.Fatalf("ListRunCheckpoints() len = %d, want %d", len(checkpoints), checkpointCount)
	}
	seen := make(map[int]bool, checkpointCount)
	for _, checkpoint := range checkpoints {
		seen[checkpoint.Sequence] = true
	}
	for sequence := 1; sequence <= checkpointCount; sequence++ {
		if !seen[sequence] {
			t.Fatalf("missing checkpoint sequence %d; got %#v", sequence, seen)
		}
	}
}

func TestRunOutputsRemainActive(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-run-output")
	run := mustCreateRun(t, ctx, q, job)

	out := &domain.RunOutput{RunID: run.ID, OutputKey: "final", Schema: json.RawMessage(`{"type":"object"}`), Value: json.RawMessage(`{"name":"leo"}`)}
	if err := q.UpsertRunOutput(ctx, out); err != nil {
		t.Fatalf("UpsertRunOutput() error = %v", err)
	}
	initialOutputID := out.ID
	initialOutputCreatedAt := out.CreatedAt
	var outputXminBeforeNoop string
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT xmin::text
		FROM run_outputs
		WHERE run_id = $1 AND output_key = $2`,
		run.ID,
		"final",
	).Scan(&outputXminBeforeNoop); err != nil {
		t.Fatalf("query run_outputs xmin before no-op: %v", err)
	}
	sameOut := &domain.RunOutput{RunID: run.ID, OutputKey: "final", Schema: json.RawMessage(`{"type":"object"}`), Value: json.RawMessage(`{"name":"leo"}`)}
	if err := q.UpsertRunOutput(ctx, sameOut); err != nil {
		t.Fatalf("UpsertRunOutput() no-op error = %v", err)
	}
	var outputXminAfterNoop string
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT xmin::text
		FROM run_outputs
		WHERE run_id = $1 AND output_key = $2`,
		run.ID,
		"final",
	).Scan(&outputXminAfterNoop); err != nil {
		t.Fatalf("query run_outputs xmin after no-op: %v", err)
	}
	if outputXminAfterNoop != outputXminBeforeNoop {
		t.Fatalf("run_outputs no-op changed xmin from %s to %s", outputXminBeforeNoop, outputXminAfterNoop)
	}
	if sameOut.ID != initialOutputID {
		t.Fatalf("run_outputs no-op id = %q, want %q", sameOut.ID, initialOutputID)
	}
	if !sameOut.CreatedAt.Equal(initialOutputCreatedAt) {
		t.Fatalf("run_outputs no-op created_at = %v, want %v", sameOut.CreatedAt, initialOutputCreatedAt)
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

	var beforeNoopXmin string
	if err := testDB.Pool.QueryRow(ctx,
		`SELECT xmin::text FROM job_runs WHERE id = $1`,
		run.ID,
	).Scan(&beforeNoopXmin); err != nil {
		t.Fatalf("query metadata xmin before no-op: %v", err)
	}
	if err := q.UpdateRunMetadata(ctx, run.ID, map[string]string{"team": "infra", "region": "eu"}); err != nil {
		t.Fatalf("UpdateRunMetadata() no-op error = %v", err)
	}
	var afterNoopXmin string
	if err := testDB.Pool.QueryRow(ctx,
		`SELECT xmin::text FROM job_runs WHERE id = $1`,
		run.ID,
	).Scan(&afterNoopXmin); err != nil {
		t.Fatalf("query metadata xmin after no-op: %v", err)
	}
	if afterNoopXmin != beforeNoopXmin {
		t.Fatalf("metadata no-op changed xmin from %s to %s", beforeNoopXmin, afterNoopXmin)
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

	gotJobSecret, err := q.GetJobSecret(ctx, jobSecret.ID, jobSecret.ProjectID)
	if err != nil {
		t.Fatalf("GetJobSecret() error = %v", err)
	}
	if gotJobSecret.ID != jobSecret.ID || gotJobSecret.ProjectID != projectID || gotJobSecret.JobID != job.ID || gotJobSecret.SecretKey != "API_TOKEN" {
		t.Fatalf("GetJobSecret() mismatch: got %+v", *gotJobSecret)
	}
	if gotJobSecret.Value != "job-value" {
		t.Fatalf("GetJobSecret() value = %q, want %q", gotJobSecret.Value, "job-value")
	}
	if gotJobSecret.EncryptedValue == "job-value" {
		t.Fatal("GetJobSecret() overwrote EncryptedValue with decrypted plaintext")
	}

	_, err = q.GetJobSecret(ctx, newID(), projectID)
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
	if len(byJob) != 1 {
		t.Fatalf("ListJobSecretsByJob() len = %d, want only job-scoped secret", len(byJob))
	}
	if byJob[0].ID != jobSecret.ID {
		t.Fatalf("ListJobSecretsByJob() = %+v, want only %q; global secret %q must not auto-dispatch", byJob, jobSecret.ID, globalSecret.ID)
	}

	byJobNone, err := q.ListJobSecretsByJob(ctx, job.ID, "staging")
	if err != nil {
		t.Fatalf("ListJobSecretsByJob(staging) error = %v", err)
	}
	if len(byJobNone) != 0 {
		t.Fatalf("ListJobSecretsByJob(staging) len = %d, want 0", len(byJobNone))
	}

	if err := q.DeleteJobSecret(ctx, jobSecret.ID, jobSecret.ProjectID); err != nil {
		t.Fatalf("DeleteJobSecret() error = %v", err)
	}
	_, err = q.GetJobSecret(ctx, jobSecret.ID, jobSecret.ProjectID)
	if !errors.Is(err, store.ErrJobSecretNotFound) {
		t.Fatalf("GetJobSecret(after delete) error = %v, want ErrJobSecretNotFound", err)
	}

	if err := q.DeleteJobSecret(ctx, newID(), projectID); !errors.Is(err, store.ErrJobSecretNotFound) {
		t.Fatalf("DeleteJobSecret(not found) error = %v, want ErrJobSecretNotFound", err)
	}
}

func TestSecret_JobSecretDecryptsWithOldEncryptionKey(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	oldQ := mustStore(t)
	oldQ.SetSecretEncryptionKey("old-secret-encryption-key")
	projectID := "project-secret-old-key"
	job := mustCreateJob(t, ctx, oldQ, projectID)

	secret := &domain.JobSecret{
		ProjectID:      projectID,
		JobID:          job.ID,
		Environment:    "prod",
		SecretKey:      "API_TOKEN",
		EncryptedValue: "legacy-value",
	}
	if err := oldQ.CreateJobSecret(ctx, secret); err != nil {
		t.Fatalf("CreateJobSecret(old key) error = %v", err)
	}

	newQ := mustStore(t)
	newQ.SetSecretEncryptionKey("new-secret-encryption-key")
	newQ.SetOldSecretEncryptionKeys([]string{"old-secret-encryption-key"})

	got, err := newQ.GetJobSecret(ctx, secret.ID, secret.ProjectID)
	if err != nil {
		t.Fatalf("GetJobSecret(with old key) error = %v", err)
	}
	if got.Value != "legacy-value" {
		t.Fatalf("GetJobSecret(with old key) value = %q, want legacy-value", got.Value)
	}

	withoutOld := mustStore(t)
	withoutOld.SetSecretEncryptionKey("new-secret-encryption-key")
	if _, err := withoutOld.GetJobSecret(ctx, secret.ID, secret.ProjectID); err == nil {
		t.Fatal("GetJobSecret(without old key) error = nil, want decrypt failure")
	}
}

func TestSecret_GlobalSecretUniqueness(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	q.SetSecretEncryptionKey("test-encryption-key-32bytes!!!!")
	mustClean(t, ctx)

	projectID := "project-global-secret-" + newID()
	job := mustCreateJob(t, ctx, q, projectID)

	globalSecret := &domain.JobSecret{
		ProjectID:      projectID,
		Environment:    "prod",
		SecretKey:      "SHARED_TOKEN",
		EncryptedValue: "first",
	}
	if err := q.CreateJobSecret(ctx, globalSecret); err != nil {
		t.Fatalf("CreateJobSecret(global) error = %v", err)
	}

	duplicateGlobal := &domain.JobSecret{
		ProjectID:      projectID,
		Environment:    "prod",
		SecretKey:      "SHARED_TOKEN",
		EncryptedValue: "duplicate",
	}
	if err := q.CreateJobSecret(ctx, duplicateGlobal); err == nil {
		t.Fatal("CreateJobSecret(duplicate global) error = nil, want unique violation")
	}

	var count int
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM job_secrets
		WHERE project_id = $1 AND job_id IS NULL AND environment = 'prod' AND secret_key = 'SHARED_TOKEN'
	`, projectID).Scan(&count); err != nil {
		t.Fatalf("count global secrets: %v", err)
	}
	if count != 1 {
		t.Fatalf("global secret count = %d, want 1", count)
	}

	sameKeyDifferentEnv := &domain.JobSecret{
		ProjectID:      projectID,
		Environment:    "staging",
		SecretKey:      "SHARED_TOKEN",
		EncryptedValue: "staging",
	}
	if err := q.CreateJobSecret(ctx, sameKeyDifferentEnv); err != nil {
		t.Fatalf("CreateJobSecret(same key different env) error = %v", err)
	}

	otherProjectSecret := &domain.JobSecret{
		ProjectID:      "other-project-" + newID(),
		Environment:    "prod",
		SecretKey:      "SHARED_TOKEN",
		EncryptedValue: "other-project",
	}
	if err := q.CreateJobSecret(ctx, otherProjectSecret); err != nil {
		t.Fatalf("CreateJobSecret(same key different project) error = %v", err)
	}

	jobScopedSecret := &domain.JobSecret{
		ProjectID:      projectID,
		JobID:          job.ID,
		Environment:    "prod",
		SecretKey:      "SHARED_TOKEN",
		EncryptedValue: "job-scoped",
	}
	if err := q.CreateJobSecret(ctx, jobScopedSecret); err != nil {
		t.Fatalf("CreateJobSecret(job-scoped same key) error = %v", err)
	}
}

func TestSecret_ListJobSecretsByJobUsesJobEnvironment(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	q.SetSecretEncryptionKey("test-encryption-key-32bytes!!!!")
	mustClean(t, ctx)

	projectID := "project-secret-env-" + newID()
	prod := &domain.Environment{ProjectID: projectID, Name: "Production", Slug: "production"}
	if err := q.CreateEnvironment(ctx, prod); err != nil {
		t.Fatalf("CreateEnvironment(prod) error = %v", err)
	}
	staging := &domain.Environment{ProjectID: projectID, Name: "Staging", Slug: "staging"}
	if err := q.CreateEnvironment(ctx, staging); err != nil {
		t.Fatalf("CreateEnvironment(staging) error = %v", err)
	}
	job := mustCreateJob(t, ctx, q, projectID)
	job.EnvironmentID = staging.ID
	if err := q.UpdateJob(ctx, job); err != nil {
		t.Fatalf("UpdateJob(environment) error = %v", err)
	}

	prodSecret := &domain.JobSecret{ProjectID: projectID, JobID: job.ID, Environment: prod.ID, SecretKey: "TOKEN", EncryptedValue: "prod"}
	if err := q.CreateJobSecret(ctx, prodSecret); err == nil {
		t.Fatal("CreateJobSecret(prod for staging job) error = nil, want mismatch")
	}
	stagingGlobal := &domain.JobSecret{ProjectID: projectID, Environment: staging.ID, SecretKey: "GLOBAL_TOKEN", EncryptedValue: "staging-global"}
	if err := q.CreateJobSecret(ctx, stagingGlobal); err != nil {
		t.Fatalf("CreateJobSecret(staging global) error = %v", err)
	}
	stagingSecret := &domain.JobSecret{ProjectID: projectID, JobID: job.ID, Environment: staging.ID, SecretKey: "JOB_TOKEN", EncryptedValue: "staging-job"}
	if err := q.CreateJobSecret(ctx, stagingSecret); err != nil {
		t.Fatalf("CreateJobSecret(staging job) error = %v", err)
	}

	got, err := q.ListJobSecretsByJob(ctx, job.ID, prod.ID)
	if err != nil {
		t.Fatalf("ListJobSecretsByJob() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ListJobSecretsByJob() len = %d, want only staging job secret", len(got))
	}
	for _, secret := range got {
		if secret.Environment != staging.ID {
			t.Fatalf("secret environment = %q, want staging env %q", secret.Environment, staging.ID)
		}
		if secret.JobID != job.ID {
			t.Fatalf("secret job_id = %q, want %q; environment-wide secrets must not auto-dispatch", secret.JobID, job.ID)
		}
	}
}

func TestSecret_ListJobSecretsByJobDefaultsEnvlessJobToProduction(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	q.SetSecretEncryptionKey("test-encryption-key-32bytes!!!!")
	mustClean(t, ctx)

	projectID := "project-secret-envless-" + newID()
	prod := &domain.Environment{ProjectID: projectID, Name: "Production", Slug: "production"}
	if err := q.CreateEnvironment(ctx, prod); err != nil {
		t.Fatalf("CreateEnvironment(prod) error = %v", err)
	}
	job := mustCreateJob(t, ctx, q, projectID)
	if job.EnvironmentID != "" {
		t.Fatalf("fixture job EnvironmentID = %q, want empty", job.EnvironmentID)
	}

	productionSecret := &domain.JobSecret{ProjectID: projectID, Environment: prod.ID, SecretKey: "TOKEN", EncryptedValue: "production"}
	if err := q.CreateJobSecret(ctx, productionSecret); err != nil {
		t.Fatalf("CreateJobSecret(production) error = %v", err)
	}

	got, err := q.ListJobSecretsByJob(ctx, job.ID, "")
	if err != nil {
		t.Fatalf("ListJobSecretsByJob() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("ListJobSecretsByJob() = %+v, want no environment-wide secret %q", got, productionSecret.ID)
	}
}

func TestSecret_ListJobSecretsByJobExcludesEnvironmentWideSecrets(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	q.SetSecretEncryptionKey("test-encryption-key-32bytes!!!!")
	mustClean(t, ctx)

	projectID := "project-secret-precedence-" + newID()
	prod := &domain.Environment{ProjectID: projectID, Name: "Production", Slug: "production"}
	if err := q.CreateEnvironment(ctx, prod); err != nil {
		t.Fatalf("CreateEnvironment(prod) error = %v", err)
	}
	job := mustCreateJob(t, ctx, q, projectID)

	jobSecret := &domain.JobSecret{ProjectID: projectID, JobID: job.ID, Environment: prod.ID, SecretKey: "TOKEN", EncryptedValue: "job-specific"}
	if err := q.CreateJobSecret(ctx, jobSecret); err != nil {
		t.Fatalf("CreateJobSecret(job) error = %v", err)
	}
	globalSecret := &domain.JobSecret{ProjectID: projectID, Environment: prod.ID, SecretKey: "TOKEN", EncryptedValue: "global"}
	if err := q.CreateJobSecret(ctx, globalSecret); err != nil {
		t.Fatalf("CreateJobSecret(global) error = %v", err)
	}

	got, err := q.ListJobSecretsByJob(ctx, job.ID, prod.ID)
	if err != nil {
		t.Fatalf("ListJobSecretsByJob() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ListJobSecretsByJob() len = %d, want only job-scoped secret", len(got))
	}
	if got[0].ID != jobSecret.ID {
		t.Fatalf("ListJobSecretsByJob() = %+v, want job-scoped %q; environment-wide %q must not dispatch", got, jobSecret.ID, globalSecret.ID)
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

func TestWebhookDelivery_EnqueueRunWebhookAndListPendingRun(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-webhook-enqueue-run")
	run := mustCreateRun(t, ctx, q, job)
	run.Status = domain.StatusCompleted
	run.Result = []byte(`{"ok":true}`)

	enqueued, err := q.EnqueueRunWebhook(ctx, job, run, 7)
	if err != nil {
		t.Fatalf("EnqueueRunWebhook() error = %v", err)
	}
	if enqueued.ID == "" {
		t.Fatal("EnqueueRunWebhook() did not set ID")
	}
	if enqueued.RunID != run.ID {
		t.Fatalf("EnqueueRunWebhook() run_id = %q, want %q", enqueued.RunID, run.ID)
	}
	if enqueued.EventTriggerID != "" {
		t.Fatalf("EnqueueRunWebhook() event_trigger_id = %q, want empty", enqueued.EventTriggerID)
	}
	if enqueued.WebhookURL != job.WebhookURL {
		t.Fatalf("EnqueueRunWebhook() webhook_url = %q, want %q", enqueued.WebhookURL, job.WebhookURL)
	}
	if enqueued.WebhookSecret != job.WebhookSecret {
		t.Fatalf("EnqueueRunWebhook() webhook_secret = %q, want %q", enqueued.WebhookSecret, job.WebhookSecret)
	}
	if enqueued.Status != domain.WebhookStatusPending {
		t.Fatalf("EnqueueRunWebhook() status = %q, want %q", enqueued.Status, domain.WebhookStatusPending)
	}
	if enqueued.MaxAttempts != 7 {
		t.Fatalf("EnqueueRunWebhook() max_attempts = %d, want 7", enqueued.MaxAttempts)
	}
	if enqueued.NextRetryAt == nil {
		t.Fatal("EnqueueRunWebhook() next_retry_at = nil, want non-nil")
	}

	var payloadRaw []byte
	var payloadSize int
	var eventType string
	var webhookSecret *string
	err = testDB.Pool.QueryRow(ctx, `
		SELECT payload, payload_size_bytes, event_type, webhook_secret
		FROM webhook_deliveries
		WHERE id = $1`, enqueued.ID,
	).Scan(&payloadRaw, &payloadSize, &eventType, &webhookSecret)
	if err != nil {
		t.Fatalf("query enqueued webhook delivery payload error = %v", err)
	}
	if payloadSize != len(payloadRaw) {
		t.Fatalf("payload_size_bytes = %d, want %d", payloadSize, len(payloadRaw))
	}
	if eventType != "run.completed" {
		t.Fatalf("event_type = %q, want %q", eventType, "run.completed")
	}
	if webhookSecret == nil || *webhookSecret != job.WebhookSecret {
		t.Fatalf("webhook_secret = %v, want %q", webhookSecret, job.WebhookSecret)
	}

	var payload map[string]any
	if err := json.Unmarshal(payloadRaw, &payload); err != nil {
		t.Fatalf("unmarshal payload error = %v", err)
	}
	if payload["run_id"] != run.ID {
		t.Fatalf("payload run_id = %v, want %q", payload["run_id"], run.ID)
	}
	if payload["job_id"] != run.JobID {
		t.Fatalf("payload job_id = %v, want %q", payload["job_id"], run.JobID)
	}
	if payload["project_id"] != run.ProjectID {
		t.Fatalf("payload project_id = %v, want %q", payload["project_id"], run.ProjectID)
	}
	if payload["status"] != string(run.Status) {
		t.Fatalf("payload status = %v, want %q", payload["status"], run.Status)
	}

	evtID := newID()
	expiresAt := time.Now().UTC().Add(time.Hour)
	_, err = testDB.Pool.Exec(ctx, `
		INSERT INTO event_triggers (id, event_key, project_id, source_type, expires_at)
		VALUES ($1, $2, $3, 'webhook', $4)`,
		evtID, "evt-key-"+evtID, job.ProjectID, expiresAt,
	)
	if err != nil {
		t.Fatalf("insert event_trigger error = %v", err)
	}

	now := time.Now().UTC().Add(-1 * time.Minute)
	eventDelivery := &domain.WebhookDelivery{
		JobID:          job.ID,
		RunID:          run.ID,
		EventTriggerID: evtID,
		WebhookURL:     "https://example.com/event",
		Status:         domain.WebhookStatusPending,
		Attempts:       0,
		MaxAttempts:    3,
		NextRetryAt:    &now,
	}
	if err := q.CreateWebhookDelivery(ctx, eventDelivery); err != nil {
		t.Fatalf("CreateWebhookDelivery(event) error = %v", err)
	}

	pendingRun, err := q.ListPendingRunWebhookDeliveries(ctx)
	if err != nil {
		t.Fatalf("ListPendingRunWebhookDeliveries() error = %v", err)
	}
	if len(pendingRun) != 1 {
		t.Fatalf("ListPendingRunWebhookDeliveries() len = %d, want 1", len(pendingRun))
	}
	if pendingRun[0].ID != enqueued.ID {
		t.Fatalf("ListPendingRunWebhookDeliveries() id = %q, want %q", pendingRun[0].ID, enqueued.ID)
	}
	if pendingRun[0].WebhookSecret != job.WebhookSecret {
		t.Fatalf("ListPendingRunWebhookDeliveries() webhook_secret = %q, want %q", pendingRun[0].WebhookSecret, job.WebhookSecret)
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
	q.SetSecretEncryptionKey("0123456789abcdef0123456789abcdef")
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

	gotEnv, err := q.GetEnvironment(ctx, env.ID, env.ProjectID)
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

	updated, err := q.GetEnvironment(ctx, env.ID, env.ProjectID)
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

	if err := q.DeleteEnvironment(ctx, env.ID, env.ProjectID); err != nil {
		t.Fatalf("DeleteEnvironment() error = %v", err)
	}
	if _, err := q.GetEnvironment(ctx, env.ID, env.ProjectID); !errors.Is(err, store.ErrEnvironmentNotFound) {
		t.Fatalf("GetEnvironment() after delete error = %v, want ErrEnvironmentNotFound", err)
	}

	if _, err := q.GetEnvironment(ctx, newID(), projectID); !errors.Is(err, store.ErrEnvironmentNotFound) {
		t.Fatalf("GetEnvironment() not found error = %v, want ErrEnvironmentNotFound", err)
	}

	notFoundEnv := &domain.Environment{ID: newID(), ProjectID: projectID, Name: "missing", Slug: "missing"}
	if err := q.UpdateEnvironment(ctx, notFoundEnv); !errors.Is(err, store.ErrEnvironmentNotFound) {
		t.Fatalf("UpdateEnvironment() not found error = %v, want ErrEnvironmentNotFound", err)
	}

	if err := q.DeleteEnvironment(ctx, newID(), projectID); !errors.Is(err, store.ErrEnvironmentNotFound) {
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
	q.SetSecretEncryptionKey("0123456789abcdef0123456789abcdef")
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

func TestEnvironment_InheritanceResolutionDoesNotCrossProjects(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	q.SetSecretEncryptionKey("0123456789abcdef0123456789abcdef")
	mustClean(t, ctx)

	parent := &domain.Environment{
		ProjectID: "project-environment-parent-tenant",
		Name:      "Parent",
		Slug:      "parent",
		Variables: map[string]string{"PARENT_ONLY": "leak", "SHARED": "parent"},
	}
	if err := q.CreateEnvironment(ctx, parent); err != nil {
		t.Fatalf("CreateEnvironment(parent) error = %v", err)
	}

	child := &domain.Environment{
		ProjectID: "project-environment-child-tenant",
		Name:      "Child",
		Slug:      "child",
		ParentID:  parent.ID,
		Variables: map[string]string{"CHILD_ONLY": "ok", "SHARED": "child"},
	}
	if err := q.CreateEnvironment(ctx, child); err != nil {
		t.Fatalf("CreateEnvironment(child) error = %v", err)
	}

	resolved, err := q.GetResolvedEnvironmentVariables(ctx, child.ID)
	if err != nil {
		t.Fatalf("GetResolvedEnvironmentVariables() error = %v", err)
	}
	if got := resolved["PARENT_ONLY"]; got != "" {
		t.Fatalf("resolved inherited cross-project parent secret = %q", got)
	}
	if got := resolved["CHILD_ONLY"]; got != "ok" {
		t.Fatalf("resolved CHILD_ONLY = %q, want ok", got)
	}
	if got := resolved["SHARED"]; got != "child" {
		t.Fatalf("resolved SHARED = %q, want child", got)
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

	counts, err := q.GetJobHealthCounts(ctx, job.ID, since)
	if err != nil {
		t.Fatalf("GetJobHealthCounts() error = %v", err)
	}
	if counts.TotalRuns != stats.TotalRuns ||
		counts.CompletedRuns != stats.CompletedRuns ||
		counts.FailedRuns != stats.FailedRuns ||
		counts.TimedOutRuns != stats.TimedOutRuns ||
		counts.CrashedRuns != stats.CrashedRuns ||
		counts.CanceledRuns != stats.CanceledRuns ||
		counts.ExpiredRuns != stats.ExpiredRuns {
		t.Fatalf("GetJobHealthCounts() = %+v, want count fields from %+v", *counts, *stats)
	}
	if counts.AvgDurationSecs != 0 || counts.P95DurationSecs != 0 || counts.P99DurationSecs != 0 {
		t.Fatalf("GetJobHealthCounts() duration fields = avg:%f p95:%f p99:%f, want zeros", counts.AvgDurationSecs, counts.P95DurationSecs, counts.P99DurationSecs)
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

	updated, err := q.BatchUpdateJobsEnabled(ctx, []string{job1.ID, job2.ID, newID()}, false, projectID)
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

	updated, err = q.BatchUpdateJobsEnabled(ctx, []string{job2.ID}, true, projectID)
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

	updated, err = q.BatchUpdateJobsEnabled(ctx, nil, false, projectID)
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

func TestCreateWorkflow_RejectsMismatchedProjectContext(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	err := q.WithTx(ctx, func(txCtx context.Context, tx store.DBTX) error {
		txq := store.New(tx)
		if err := txq.SetProjectContext(txCtx, "project-workflow-authorized"); err != nil {
			t.Fatalf("SetProjectContext() error = %v", err)
		}

		mismatched := &domain.Workflow{
			ProjectID: "project-workflow-attacker",
			Name:      "Mismatched Workflow",
			Slug:      "mismatched-workflow-" + newID(),
			Enabled:   true,
		}
		if err := txq.CreateWorkflow(txCtx, mismatched); !errors.Is(err, store.ErrProjectContextMismatch) {
			t.Fatalf("CreateWorkflow(mismatched project context) = %v, want ErrProjectContextMismatch", err)
		}

		matching := &domain.Workflow{
			ProjectID: "project-workflow-authorized",
			Name:      "Authorized Workflow",
			Slug:      "authorized-workflow-" + newID(),
			Enabled:   true,
		}
		if err := txq.CreateWorkflow(txCtx, matching); err != nil {
			t.Fatalf("CreateWorkflow(matching project context) error = %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WithTx() error = %v", err)
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
		WorkflowID:    workflow.ID,
		JobID:         job.ID,
		StepRef:       "extract",
		DependsOn:     []string{},
		Condition:     json.RawMessage(`{"type":"step_status","step_ref":"extract","status":"completed"}`),
		Payload:       json.RawMessage(`{"batch":1}`),
		ResourceClass: "medium",
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
	if step.ResourceClass != "medium" {
		t.Fatalf("CreateWorkflowStep() resource_class = %q, want medium", step.ResourceClass)
	}

	got, err := q.GetWorkflowStep(ctx, step.ID)
	if err != nil {
		t.Fatalf("GetWorkflowStep() error = %v", err)
	}
	if got.ID != step.ID || got.WorkflowID != step.WorkflowID || got.JobID != step.JobID || got.StepRef != step.StepRef || got.OnFailure != step.OnFailure {
		t.Fatalf("GetWorkflowStep() mismatch: got %+v want %+v", *got, *step)
	}
	if got.ResourceClass != "medium" {
		t.Fatalf("GetWorkflowStep() resource_class = %q, want medium", got.ResourceClass)
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

func TestWorkflowStepLookupScopesThroughParentWorkflowUnderRLS(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectA := "project-workflow-step-rls-a-" + newID()
	projectB := "project-workflow-step-rls-b-" + newID()

	jobA := mustCreateJob(t, ctx, q, projectA)
	workflowA := &domain.Workflow{
		ProjectID: projectA,
		Name:      "Step RLS A",
		Slug:      "step-rls-a-" + newID(),
		Enabled:   true,
	}
	if err := q.CreateWorkflow(ctx, workflowA); err != nil {
		t.Fatalf("CreateWorkflow(A) error = %v", err)
	}
	stepA := &domain.WorkflowStep{
		WorkflowID: workflowA.ID,
		JobID:      jobA.ID,
		StepRef:    "own",
		DependsOn:  []string{},
	}
	if err := q.CreateWorkflowStep(ctx, stepA); err != nil {
		t.Fatalf("CreateWorkflowStep(A) error = %v", err)
	}

	jobB := mustCreateJob(t, ctx, q, projectB)
	workflowB := &domain.Workflow{
		ProjectID: projectB,
		Name:      "Step RLS B",
		Slug:      "step-rls-b-" + newID(),
		Enabled:   true,
	}
	if err := q.CreateWorkflow(ctx, workflowB); err != nil {
		t.Fatalf("CreateWorkflow(B) error = %v", err)
	}
	stepB := &domain.WorkflowStep{
		WorkflowID: workflowB.ID,
		JobID:      jobB.ID,
		StepRef:    "foreign",
		DependsOn:  []string{},
	}
	if err := q.CreateWorkflowStep(ctx, stepB); err != nil {
		t.Fatalf("CreateWorkflowStep(B) error = %v", err)
	}

	runAsProject(t, ctx, projectA, false, func(txq *store.Queries) {
		own, err := txq.GetWorkflowStep(ctx, stepA.ID)
		if err != nil {
			t.Fatalf("GetWorkflowStep(own) error = %v", err)
		}
		if own.ID != stepA.ID {
			t.Fatalf("GetWorkflowStep(own) ID = %q, want %q", own.ID, stepA.ID)
		}

		if _, err := txq.GetWorkflowStep(ctx, stepB.ID); !errors.Is(err, store.ErrWorkflowStepNotFound) {
			t.Fatalf("GetWorkflowStep(foreign) error = %v, want ErrWorkflowStepNotFound", err)
		}

		foreignSteps, err := txq.ListStepsByWorkflow(ctx, workflowB.ID)
		if err != nil {
			t.Fatalf("ListStepsByWorkflow(foreign) error = %v", err)
		}
		if len(foreignSteps) != 0 {
			t.Fatalf("ListStepsByWorkflow(foreign) len = %d, want 0", len(foreignSteps))
		}

		if err := txq.DeleteStepsByWorkflow(ctx, workflowB.ID); err != nil {
			t.Fatalf("DeleteStepsByWorkflow(foreign) error = %v", err)
		}
	})

	foreignAfterDelete, err := q.GetWorkflowStep(ctx, stepB.ID)
	if err != nil {
		t.Fatalf("GetWorkflowStep(foreign after blocked delete) error = %v", err)
	}
	if foreignAfterDelete.ID != stepB.ID {
		t.Fatalf("foreign step deleted or changed: got %q, want %q", foreignAfterDelete.ID, stepB.ID)
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

	secondParent := &domain.WorkflowStep{
		WorkflowID: workflow.ID,
		JobID:      job.ID,
		StepRef:    "transform",
		DependsOn:  []string{},
	}
	if err := q.CreateWorkflowStep(ctx, secondParent); err != nil {
		t.Fatalf("CreateWorkflowStep(second parent) error = %v", err)
	}

	child := &domain.WorkflowStep{
		WorkflowID: workflow.ID,
		JobID:      job.ID,
		StepRef:    "aggregate",
		DependsOn:  []string{"extract", "transform"},
		Condition:  json.RawMessage(`{"type":"step_status","step_ref":"extract","status":"completed"}`),
		Payload:    json.RawMessage(`{"kind":"agg"}`),
	}
	if err := q.CreateWorkflowStep(ctx, child); err != nil {
		t.Fatalf("CreateWorkflowStep(child) error = %v", err)
	}

	if err := q.CreateWorkflowVersionSnapshot(ctx, workflow.ID, 1); err != nil {
		t.Fatalf("CreateWorkflowVersionSnapshot() error = %v", err)
	}

	run := &domain.WorkflowRun{
		WorkflowID: workflow.ID,
		ProjectID:  workflow.ProjectID,
	}
	if err := q.CreateWorkflowRun(ctx, run); err != nil {
		t.Fatalf("CreateWorkflowRun() error = %v", err)
	}

	completedParent := &domain.WorkflowStepRun{
		WorkflowRunID:  run.ID,
		WorkflowStepID: parent.ID,
		StepRef:        parent.StepRef,
		Status:         domain.StepCompleted,
		DepsCompleted:  0,
		DepsRequired:   0,
	}
	if err := q.CreateWorkflowStepRun(ctx, completedParent); err != nil {
		t.Fatalf("CreateWorkflowStepRun(completed parent) error = %v", err)
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
	if first[0].StepRunID != waiting.ID || first[0].StepRef != waiting.StepRef || first[0].DepsCompleted != 1 || first[0].DepsRequired != 2 || first[0].JobID == nil || *first[0].JobID != child.JobID || first[0].WorkflowRunID != run.ID {
		t.Fatalf("IncrementStepDeps() first result mismatch: got %+v", first[0])
	}
	if !jsonEqual(first[0].Condition, child.Condition) {
		t.Fatalf("IncrementStepDeps() first condition = %s, want %s", string(first[0].Condition), string(child.Condition))
	}
	if !jsonEqual(first[0].Payload, child.Payload) {
		t.Fatalf("IncrementStepDeps() first payload = %s, want %s", string(first[0].Payload), string(child.Payload))
	}

	duplicate, err := q.IncrementStepDeps(ctx, run.ID, parent.StepRef)
	if err != nil {
		t.Fatalf("IncrementStepDeps() duplicate error = %v", err)
	}
	if len(duplicate) != 0 {
		t.Fatalf("IncrementStepDeps() duplicate len = %d, want 0", len(duplicate))
	}

	completedSecondParent := &domain.WorkflowStepRun{
		WorkflowRunID:  run.ID,
		WorkflowStepID: secondParent.ID,
		StepRef:        secondParent.StepRef,
		Status:         domain.StepCompleted,
		DepsCompleted:  0,
		DepsRequired:   0,
	}
	if err := q.CreateWorkflowStepRun(ctx, completedSecondParent); err != nil {
		t.Fatalf("CreateWorkflowStepRun(completed second parent) error = %v", err)
	}

	second, err := q.IncrementStepDeps(ctx, run.ID, secondParent.StepRef)
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

func TestWorkflowStepRun_TargetedListings(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-workflow-step-run-targeted-listings"
	job := mustCreateJob(t, ctx, q, projectID)
	workflow := &domain.Workflow{ProjectID: projectID, Name: "Targeted Workflow", Slug: "targeted-workflow", Enabled: true}
	if err := q.CreateWorkflow(ctx, workflow); err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}

	steps := []*domain.WorkflowStep{
		{WorkflowID: workflow.ID, JobID: job.ID, StepRef: "a"},
		{WorkflowID: workflow.ID, JobID: job.ID, StepRef: "b", DependsOn: []string{"a"}},
		{WorkflowID: workflow.ID, JobID: job.ID, StepRef: "c"},
		{WorkflowID: workflow.ID, JobID: job.ID, StepRef: "d", DependsOn: []string{"a"}},
	}
	for _, step := range steps {
		if err := q.CreateWorkflowStep(ctx, step); err != nil {
			t.Fatalf("CreateWorkflowStep(%s) error = %v", step.StepRef, err)
		}
	}

	run := &domain.WorkflowRun{WorkflowID: workflow.ID, ProjectID: workflow.ProjectID}
	if err := q.CreateWorkflowRun(ctx, run); err != nil {
		t.Fatalf("CreateWorkflowRun() error = %v", err)
	}

	stepByRef := map[string]*domain.WorkflowStep{}
	for _, step := range steps {
		stepByRef[step.StepRef] = step
	}
	seed := []domain.WorkflowStepRun{
		{WorkflowRunID: run.ID, WorkflowStepID: stepByRef["a"].ID, StepRef: "a", Status: domain.StepRunning, DepsCompleted: 0, DepsRequired: 0},
		{WorkflowRunID: run.ID, WorkflowStepID: stepByRef["b"].ID, StepRef: "b", Status: domain.StepWaiting, DepsCompleted: 1, DepsRequired: 1},
		{WorkflowRunID: run.ID, WorkflowStepID: stepByRef["c"].ID, StepRef: "c", Status: domain.StepPending, DepsCompleted: 0, DepsRequired: 0},
		{WorkflowRunID: run.ID, WorkflowStepID: stepByRef["d"].ID, StepRef: "d", Status: domain.StepWaiting, DepsCompleted: 0, DepsRequired: 1},
	}
	for i := range seed {
		if err := q.CreateWorkflowStepRun(ctx, &seed[i]); err != nil {
			t.Fatalf("CreateWorkflowStepRun(%s) error = %v", seed[i].StepRef, err)
		}
	}

	running, err := q.ListRunningStepRunsByWorkflowRun(ctx, run.ID, 100)
	if err != nil {
		t.Fatalf("ListRunningStepRunsByWorkflowRun() error = %v", err)
	}
	if len(running) != 1 || running[0].StepRef != "a" {
		t.Fatalf("running step refs = %+v, want [a]", lo.Map(running, func(sr domain.WorkflowStepRun, _ int) string { return sr.StepRef }))
	}

	runnable, err := q.ListRunnableStepRunsByWorkflowRun(ctx, run.ID, 100)
	if err != nil {
		t.Fatalf("ListRunnableStepRunsByWorkflowRun() error = %v", err)
	}
	if len(runnable) != 2 {
		t.Fatalf("ListRunnableStepRunsByWorkflowRun() len = %d, want 2", len(runnable))
	}
	if runnable[0].StepRef != "b" || runnable[1].StepRef != "c" {
		t.Fatalf("runnable order = [%s,%s], want [b,c]", runnable[0].StepRef, runnable[1].StepRef)
	}

	statuses, err := q.ListStepRunStatusesByWorkflowRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("ListStepRunStatusesByWorkflowRun() error = %v", err)
	}
	if len(statuses) != 4 {
		t.Fatalf("status map len = %d, want 4", len(statuses))
	}
	if statuses["a"] != domain.StepRunning || statuses["b"] != domain.StepWaiting || statuses["c"] != domain.StepPending || statuses["d"] != domain.StepWaiting {
		t.Fatalf("unexpected statuses map: %+v", statuses)
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
		ID:            id,
		JobID:         job.ID,
		ProjectID:     job.ProjectID,
		Status:        domain.StatusQueued,
		Attempt:       1,
		Payload:       []byte(`{"hello":"world"}`),
		TriggeredBy:   domain.TriggerManual,
		Priority:      0,
		ExecutionMode: domain.ExecutionModeHTTP,
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
	staleStarted := time.Now().UTC().Add(-15 * time.Minute)
	stale.StartedAt = &staleStarted
	if err := q.CreateRun(ctx, stale); err != nil {
		t.Fatalf("CreateRun() stale error = %v", err)
	}

	fresh := baseRun(job, newID())
	fresh.Status = domain.StatusExecuting
	freshStarted := time.Now().UTC().Add(-5 * time.Minute)
	fresh.StartedAt = &freshStarted
	if err := q.CreateRun(ctx, fresh); err != nil {
		t.Fatalf("CreateRun() fresh error = %v", err)
	}

	queued := baseRun(job, newID())
	queued.Status = domain.StatusQueued
	if err := q.CreateRun(ctx, queued); err != nil {
		t.Fatalf("CreateRun() queued error = %v", err)
	}

	// Heartbeat liveness is read from the job_run_heartbeats side table.
	oldHeartbeat := time.Now().UTC().Add(-10 * time.Minute)
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_run_heartbeats (run_id, heartbeat_at, cleared)
		VALUES ($1, $2, FALSE)`,
		stale.ID, oldHeartbeat); err != nil {
		t.Fatalf("insert stale heartbeat error = %v", err)
	}
	recentHeartbeat := time.Now().UTC().Add(-1 * time.Minute)
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_run_heartbeats (run_id, heartbeat_at, cleared)
		VALUES ($1, $2, FALSE)`,
		fresh.ID, recentHeartbeat); err != nil {
		t.Fatalf("insert fresh heartbeat error = %v", err)
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

func TestRunMgmt_ListStaleRuns_ExcludesWorkerMode(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-list-stale-runs-worker")

	httpRun := baseRun(job, newID())
	httpRun.Status = domain.StatusExecuting
	httpRun.ExecutionMode = domain.ExecutionModeHTTP
	started := time.Now().UTC().Add(-15 * time.Minute)
	httpRun.StartedAt = &started
	if err := q.CreateRun(ctx, httpRun); err != nil {
		t.Fatalf("CreateRun() http error = %v", err)
	}

	workerRun := baseRun(job, newID())
	workerRun.Status = domain.StatusExecuting
	workerRun.ExecutionMode = domain.ExecutionModeWorker
	workerRun.StartedAt = &started
	if err := q.CreateRun(ctx, workerRun); err != nil {
		t.Fatalf("CreateRun() worker error = %v", err)
	}

	oldHeartbeat := time.Now().UTC().Add(-10 * time.Minute)
	for _, id := range []string{httpRun.ID, workerRun.ID} {
		if _, err := testDB.Pool.Exec(ctx, `
			INSERT INTO job_run_heartbeats (run_id, heartbeat_at, cleared)
			VALUES ($1, $2, FALSE)`,
			id, oldHeartbeat); err != nil {
			t.Fatalf("insert heartbeat error = %v", err)
		}
	}

	runs, err := q.ListStaleRuns(ctx, 5*time.Minute)
	if err != nil {
		t.Fatalf("ListStaleRuns() error = %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("ListStaleRuns() len = %d, want 1", len(runs))
	}
	if runs[0].ID != httpRun.ID {
		t.Fatalf("ListStaleRuns() run ID = %q, want http run %q", runs[0].ID, httpRun.ID)
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

	missing, err := q.GetWorkflowStepApprovalByStepRunID(ctx, newID())
	if err != nil {
		t.Fatalf("GetWorkflowStepApprovalByStepRunID(missing) error = %v", err)
	}
	if missing != nil {
		t.Fatalf("GetWorkflowStepApprovalByStepRunID(missing) = %#v, want nil", missing)
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

func TestNotificationDeliveryClaimLifecycle(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-notification-claims"
	channel := &domain.NotificationChannel{
		ID:          newID(),
		ProjectID:   projectID,
		ChannelType: domain.ChannelTypeWebhook,
		Name:        "ops",
		Config:      []byte(`{"url":"https://example.com/hooks/ops"}`),
		Enabled:     true,
	}
	if err := q.CreateNotificationChannel(ctx, channel); err != nil {
		t.Fatalf("CreateNotificationChannel() error = %v", err)
	}

	delivery := &domain.NotificationDelivery{
		ID:          newID(),
		ChannelID:   channel.ID,
		ProjectID:   projectID,
		EventType:   domain.NotificationEventApprovalRequested,
		Payload:     json.RawMessage(`{"step_ref":"review"}`),
		Status:      "pending",
		MaxAttempts: 3,
	}
	if err := q.CreateNotificationDelivery(ctx, delivery); err != nil {
		t.Fatalf("CreateNotificationDelivery() error = %v", err)
	}

	firstClaim, err := q.ClaimPendingNotificationDeliveries(ctx, 1, time.Minute)
	if err != nil {
		t.Fatalf("ClaimPendingNotificationDeliveries(first) error = %v", err)
	}
	if len(firstClaim) != 1 {
		t.Fatalf("first claim len = %d, want 1", len(firstClaim))
	}
	if firstClaim[0].Status != "processing" {
		t.Fatalf("first claim status = %q, want processing", firstClaim[0].Status)
	}
	if firstClaim[0].ClaimToken == "" {
		t.Fatal("first claim token = empty, want non-empty")
	}
	if firstClaim[0].LeaseExpiry == nil {
		t.Fatal("first claim lease expiry = nil, want non-nil")
	}

	secondClaim, err := q.ClaimPendingNotificationDeliveries(ctx, 1, time.Minute)
	if err != nil {
		t.Fatalf("ClaimPendingNotificationDeliveries(second) error = %v", err)
	}
	// Verify our specific delivery is NOT re-claimed (others may exist from parallel tests).
	for _, s := range secondClaim {
		if s.ChannelID == channel.ID {
			t.Fatal("our delivery was re-claimed, should be excluded (already processing)")
		}
	}

	expiringDelivery := &domain.NotificationDelivery{
		ID:          newID(),
		ChannelID:   channel.ID,
		ProjectID:   projectID,
		EventType:   domain.NotificationEventApprovalReminder,
		Payload:     json.RawMessage(`{"step_ref":"review"}`),
		Status:      "pending",
		MaxAttempts: 3,
	}
	if err := q.CreateNotificationDelivery(ctx, expiringDelivery); err != nil {
		t.Fatalf("CreateNotificationDelivery(expiring) error = %v", err)
	}

	expiredClaim, err := q.ClaimPendingNotificationDeliveries(ctx, 1, -time.Second)
	if err != nil {
		t.Fatalf("ClaimPendingNotificationDeliveries(expired) error = %v", err)
	}
	if len(expiredClaim) != 1 {
		t.Fatalf("expired claim len = %d, want 1", len(expiredClaim))
	}

	reclaimed, err := q.ClaimPendingNotificationDeliveries(ctx, 10, time.Minute)
	if err != nil {
		t.Fatalf("ClaimPendingNotificationDeliveries(reclaimed) error = %v", err)
	}
	// Find our specific expired delivery in the reclaimed batch (other tests may have deliveries too).
	var reclaimedDelivery *domain.NotificationDelivery
	for i := range reclaimed {
		if reclaimed[i].ID == expiringDelivery.ID {
			reclaimedDelivery = &reclaimed[i]
			break
		}
	}
	if reclaimedDelivery == nil {
		t.Fatalf("expired delivery %s not found in reclaimed batch of %d", expiringDelivery.ID, len(reclaimed))
	}
	if reclaimedDelivery.ClaimToken == expiredClaim[0].ClaimToken {
		t.Fatal("reclaimed claim token was not rotated")
	}

	expiredClaim[0].Status = "failed"
	updated, err := q.UpdateClaimedNotificationDelivery(ctx, &expiredClaim[0])
	if err != nil {
		t.Fatalf("UpdateClaimedNotificationDelivery(stale) error = %v", err)
	}
	if updated {
		t.Fatal("UpdateClaimedNotificationDelivery(stale) = true, want false")
	}

	reclaimedDelivery.Status = "delivered"
	now := time.Now().UTC()
	reclaimedDelivery.DeliveredAt = &now
	updated, err = q.UpdateClaimedNotificationDelivery(ctx, reclaimedDelivery)
	if err != nil {
		t.Fatalf("UpdateClaimedNotificationDelivery(reclaimed) error = %v", err)
	}
	if !updated {
		t.Fatal("UpdateClaimedNotificationDelivery(reclaimed) = false, want true")
	}

	deliveries, err := q.ListNotificationDeliveries(ctx, projectID, 10, nil)
	if err != nil {
		t.Fatalf("ListNotificationDeliveries() error = %v", err)
	}

	byID := make(map[string]domain.NotificationDelivery, len(deliveries))
	for _, got := range deliveries {
		byID[got.ID] = got
	}

	if got := byID[delivery.ID]; got.Status != "processing" {
		t.Fatalf("claimed delivery status = %q, want processing", got.Status)
	}
	if got := byID[expiringDelivery.ID]; got.Status != "delivered" {
		t.Fatalf("reclaimed delivery status = %q, want delivered", got.Status)
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

	// No launch cost event yet.
	total, err := q.SumRunCostMicrousd(ctx, run.ID)
	if err != nil {
		t.Fatalf("SumRunCostMicrousd() error = %v", err)
	}
	if total != 0 {
		t.Fatalf("total = %d, want 0", total)
	}

	_, err = testDB.Pool.Exec(ctx, `
		INSERT INTO billing_cost_events (
			idempotency_key, org_id, project_id, period_date, execution_mode,
			compute_cost_microusd, created_at
		) VALUES ($1, $2, $3, CURRENT_DATE, $4, $5, NOW())
	`, "strait:cost_recorded:"+run.ID, "org-sum-cost", job.ProjectID, "http", int64(3500))
	if err != nil {
		t.Fatalf("insert billing cost event: %v", err)
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
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO billing_cost_events (
			idempotency_key, org_id, project_id, period_date, execution_mode,
			compute_cost_microusd, created_at
		) VALUES ($1, $2, $3, CURRENT_DATE, $4, $5, NOW())
	`, "test:daily-cost:"+newID(), "org-daily-cost", projectID, "http", int64(5000)); err != nil {
		t.Fatalf("insert billing cost event: %v", err)
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
	project := &domain.Project{ID: projectID, OrgID: "org-cron-workflows", Name: "Cron Workflows"}
	if err := q.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject(active) error = %v", err)
	}

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

	suspendedProject := &domain.Project{ID: "project-cron-workflows-suspended", OrgID: project.OrgID, Name: "Suspended Cron Workflows"}
	if err := q.CreateProject(ctx, suspendedProject); err != nil {
		t.Fatalf("CreateProject(suspended) error = %v", err)
	}
	wfSuspended := &domain.Workflow{ID: newID(), ProjectID: suspendedProject.ID, Name: "wf-suspended-cron", Slug: "wf-suspended-cron-slug", Enabled: true, Version: 1}
	if err := q.CreateWorkflow(ctx, wfSuspended); err != nil {
		t.Fatalf("CreateWorkflow(suspended project) error = %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `UPDATE workflows SET cron = $1 WHERE id = $2`, "*/7 * * * *", wfSuspended.ID); err != nil {
		t.Fatalf("set cron suspended error = %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `UPDATE projects SET suspended = true WHERE id = $1`, suspendedProject.ID); err != nil {
		t.Fatalf("suspend project error = %v", err)
	}

	deletedProject := &domain.Project{ID: "project-cron-workflows-deleted", OrgID: project.OrgID, Name: "Deleted Cron Workflows"}
	if err := q.CreateProject(ctx, deletedProject); err != nil {
		t.Fatalf("CreateProject(deleted) error = %v", err)
	}
	wfDeleted := &domain.Workflow{ID: newID(), ProjectID: deletedProject.ID, Name: "wf-deleted-cron", Slug: "wf-deleted-cron-slug", Enabled: true, Version: 1}
	if err := q.CreateWorkflow(ctx, wfDeleted); err != nil {
		t.Fatalf("CreateWorkflow(deleted project) error = %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `UPDATE workflows SET cron = $1 WHERE id = $2`, "*/11 * * * *", wfDeleted.ID); err != nil {
		t.Fatalf("set cron deleted error = %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `UPDATE projects SET deleted_at = NOW() WHERE id = $1`, deletedProject.ID); err != nil {
		t.Fatalf("delete project row error = %v", err)
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
	if err := q.UpdateRunStatus(ctx, run3.ID, domain.StatusQueued, domain.StatusDequeued, nil); err != nil {
		t.Fatalf("UpdateRunStatus(run3 dequeued) error = %v", err)
	}
	if err := q.UpdateRunStatus(ctx, run3.ID, domain.StatusDequeued, domain.StatusExecuting, nil); err != nil {
		t.Fatalf("UpdateRunStatus(run3 executing) error = %v", err)
	}
	if err := q.UpdateRunStatus(ctx, run3.ID, domain.StatusExecuting, domain.StatusCompleted, map[string]any{
		"finished_at": time.Now().UTC().Truncate(time.Microsecond),
		"result":      json.RawMessage(`{"lineage":true}`),
	}); err != nil {
		t.Fatalf("UpdateRunStatus(run3 completed) error = %v", err)
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
	if lineage[2].Status != domain.StatusCompleted {
		t.Fatalf("lineage[2].Status = %q, want completed", lineage[2].Status)
	}
	if !jsonEqual(lineage[2].Result, []byte(`{"lineage":true}`)) {
		t.Fatalf("lineage[2].Result = %s, want terminal result", string(lineage[2].Result))
	}
}

// Store integration tests for untested methods + edge cases

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
	var beforeNoopXmin string
	if err := testDB.Pool.QueryRow(ctx,
		`SELECT xmin::text FROM job_runs WHERE id = $1`,
		run.ID,
	).Scan(&beforeNoopXmin); err != nil {
		t.Fatalf("query debug_mode xmin before no-op: %v", err)
	}
	if err := q.UpdateRunDebugMode(ctx, run.ID, true); err != nil {
		t.Fatalf("UpdateRunDebugMode(true no-op) error = %v", err)
	}
	var afterNoopXmin string
	if err := testDB.Pool.QueryRow(ctx,
		`SELECT xmin::text FROM job_runs WHERE id = $1`,
		run.ID,
	).Scan(&afterNoopXmin); err != nil {
		t.Fatalf("query debug_mode xmin after no-op: %v", err)
	}
	if afterNoopXmin != beforeNoopXmin {
		t.Fatalf("debug_mode no-op changed xmin from %s to %s", beforeNoopXmin, afterNoopXmin)
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

	// Verify step refs and that returned IDs map to canonical workflow_steps IDs.
	refs := make(map[string]bool)
	idByRef := make(map[string]string, len(steps))
	for _, s := range steps {
		refs[s.StepRef] = true
		idByRef[s.StepRef] = s.ID
	}
	if !refs["step-a"] || !refs["step-b"] {
		t.Fatalf("expected step-a and step-b, got refs: %v", refs)
	}
	if idByRef["step-a"] != step1.ID || idByRef["step-b"] != step2.ID {
		t.Fatalf("unexpected step IDs by ref: got %+v, want step-a=%s step-b=%s", idByRef, step1.ID, step2.ID)
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
	var largeJSON strings.Builder
	largeJSON.WriteString(`[` + items[0])
	for i := 1; i < len(items); i++ {
		largeJSON.WriteString(`,` + items[i])
	}
	largeJSON.WriteString(`]`)

	out := &domain.RunOutput{
		ID:        newID(),
		RunID:     run.ID,
		OutputKey: "large-output",
		Value:     json.RawMessage(largeJSON.String()),
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
	if !jsonEqual(outputs[0].Value, json.RawMessage(largeJSON.String())) {
		t.Fatalf("large value mismatch: got %d bytes, want %d bytes", len(outputs[0].Value), len(largeJSON.String()))
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
	for i := range 3 {
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
	for i := range 5 {
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

	roles, err := q.ListProjectRoles(ctx, "proj-list-roles", 100, nil)
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

func TestAssignMemberRoleWithOrgLimit_SerializesConcurrentNewMembers(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-member-limit-race"
	orgID := "org-member-limit-race"
	if err := q.CreateProject(ctx, &domain.Project{ID: projectID, OrgID: orgID, Name: "members"}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	role := &domain.ProjectRole{
		ProjectID:   projectID,
		Name:        "viewer",
		Permissions: []string{"jobs:read"},
	}
	if err := q.CreateProjectRole(ctx, role); err != nil {
		t.Fatalf("CreateProjectRole() error = %v", err)
	}

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg conc.WaitGroup
	for _, userID := range []string{"user-limit-a", "user-limit-b"} {
		userID := userID
		wg.Go(func() {
			<-start
			errs <- q.AssignMemberRoleWithOrgLimit(ctx, &domain.ProjectMemberRole{
				ProjectID: projectID,
				UserID:    userID,
				RoleID:    role.ID,
				GrantedBy: "tester",
			}, orgID, 1)
		})
	}
	close(start)
	wg.Wait()
	close(errs)

	successes := 0
	limitErrors := 0
	for err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, store.ErrMemberLimitReached):
			limitErrors++
		default:
			t.Fatalf("unexpected assignment error: %v", err)
		}
	}
	if successes != 1 || limitErrors != 1 {
		t.Fatalf("successes=%d limitErrors=%d, want 1/1", successes, limitErrors)
	}

	var count int
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT COUNT(DISTINCT pmr.user_id)
		FROM project_member_roles pmr
		JOIN projects p ON p.id = pmr.project_id
		WHERE p.org_id = $1`, orgID).Scan(&count); err != nil {
		t.Fatalf("count members: %v", err)
	}
	if count != 1 {
		t.Fatalf("member count = %d, want 1", count)
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

func TestGetUserPermissionsWithVersion(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	role := &domain.ProjectRole{
		ProjectID:   "proj-perms-version",
		Name:        "operator",
		Permissions: []string{"jobs:read"},
	}
	if err := q.CreateProjectRole(ctx, role); err != nil {
		t.Fatalf("CreateProjectRole() error = %v", err)
	}

	m := &domain.ProjectMemberRole{
		ProjectID: "proj-perms-version",
		UserID:    "user-perms-version",
		RoleID:    role.ID,
	}
	if err := q.AssignMemberRole(ctx, m); err != nil {
		t.Fatalf("AssignMemberRole() error = %v", err)
	}

	perms, version, err := q.GetUserPermissionsWithVersion(ctx, "proj-perms-version", "user-perms-version")
	if err != nil {
		t.Fatalf("GetUserPermissionsWithVersion() error = %v", err)
	}
	if len(perms) != 1 || perms[0] != "jobs:read" {
		t.Fatalf("permissions = %v, want [jobs:read]", perms)
	}
	if version <= 0 {
		t.Fatalf("version = %d, want positive", version)
	}

	role.Permissions = []string{"jobs:read", "jobs:write"}
	if err := q.UpdateProjectRole(ctx, role); err != nil {
		t.Fatalf("UpdateProjectRole() error = %v", err)
	}
	perms, updatedVersion, err := q.GetUserPermissionsWithVersion(ctx, "proj-perms-version", "user-perms-version")
	if err != nil {
		t.Fatalf("GetUserPermissionsWithVersion(after update) error = %v", err)
	}
	if len(perms) != 2 {
		t.Fatalf("updated permissions = %v, want two permissions", perms)
	}
	if updatedVersion <= version {
		t.Fatalf("updated version = %d, want > %d", updatedVersion, version)
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

func TestCreateProjectRole_RejectsCrossProjectParent(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	foreign := &domain.ProjectRole{
		ProjectID:   "proj-rbac-parent-foreign-" + newID(),
		Name:        "foreign-admin",
		Permissions: []string{"rbac:manage"},
	}
	if err := q.CreateProjectRole(ctx, foreign); err != nil {
		t.Fatalf("CreateProjectRole(foreign) error = %v", err)
	}

	local := &domain.ProjectRole{
		ProjectID:    "proj-rbac-parent-local-" + newID(),
		Name:         "local-child",
		Permissions:  []string{"jobs:read"},
		ParentRoleID: foreign.ID,
	}
	err := q.CreateProjectRole(ctx, local)
	if !errors.Is(err, store.ErrRoleNotFound) {
		t.Fatalf("CreateProjectRole(cross-project parent) = %v, want ErrRoleNotFound", err)
	}
}

func TestUpdateProjectRole_RejectsCrossProjectParent(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	foreign := &domain.ProjectRole{
		ProjectID:   "proj-rbac-update-foreign-" + newID(),
		Name:        "foreign-admin",
		Permissions: []string{"rbac:manage"},
	}
	if err := q.CreateProjectRole(ctx, foreign); err != nil {
		t.Fatalf("CreateProjectRole(foreign) error = %v", err)
	}

	local := &domain.ProjectRole{
		ProjectID:   "proj-rbac-update-local-" + newID(),
		Name:        "local-child",
		Permissions: []string{"jobs:read"},
	}
	if err := q.CreateProjectRole(ctx, local); err != nil {
		t.Fatalf("CreateProjectRole(local) error = %v", err)
	}

	local.ParentRoleID = foreign.ID
	err := q.UpdateProjectRole(ctx, local)
	if !errors.Is(err, store.ErrRoleNotFound) {
		t.Fatalf("UpdateProjectRole(cross-project parent) = %v, want ErrRoleNotFound", err)
	}
}

func TestGetUserPermissions_IgnoresCrossProjectInheritedRole(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	localProjectID := "proj-rbac-perms-local-" + newID()
	foreignProjectID := "proj-rbac-perms-foreign-" + newID()

	foreign := &domain.ProjectRole{
		ProjectID:   foreignProjectID,
		Name:        "foreign-admin",
		Permissions: []string{"rbac:manage"},
	}
	if err := q.CreateProjectRole(ctx, foreign); err != nil {
		t.Fatalf("CreateProjectRole(foreign) error = %v", err)
	}
	local := &domain.ProjectRole{
		ProjectID:   localProjectID,
		Name:        "local-reader",
		Permissions: []string{"jobs:read"},
	}
	if err := q.CreateProjectRole(ctx, local); err != nil {
		t.Fatalf("CreateProjectRole(local) error = %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `UPDATE project_roles SET parent_role_id = $1 WHERE id = $2`, foreign.ID, local.ID); err != nil {
		t.Fatalf("force cross-project parent_role_id error = %v", err)
	}

	member := &domain.ProjectMemberRole{
		ProjectID: localProjectID,
		UserID:    "user-rbac-" + newID(),
		RoleID:    local.ID,
	}
	if err := q.AssignMemberRole(ctx, member); err != nil {
		t.Fatalf("AssignMemberRole() error = %v", err)
	}

	perms, err := q.GetUserPermissions(ctx, localProjectID, member.UserID)
	if err != nil {
		t.Fatalf("GetUserPermissions() error = %v", err)
	}
	if !slices.Contains(perms, "jobs:read") {
		t.Fatalf("permissions = %v, want local jobs:read", perms)
	}
	if slices.Contains(perms, "rbac:manage") {
		t.Fatalf("permissions leaked cross-project inherited permission: %v", perms)
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

	actions, err := q.GetResourcePolicies(ctx, "proj-policy", "job", "job-123", "user-pol")
	if err != nil {
		t.Fatalf("GetResourcePolicies() error = %v", err)
	}
	if len(actions) != 2 {
		t.Fatalf("len(actions) = %d, want 2", len(actions))
	}

	policies, err := q.ListResourcePolicies(ctx, "proj-policy", "job", "job-123", 50, nil)
	if err != nil {
		t.Fatalf("ListResourcePolicies() error = %v", err)
	}
	if len(policies) != 1 {
		t.Fatalf("len(policies) = %d, want 1", len(policies))
	}

	if _, _, err := q.DeleteResourcePolicy(ctx, "proj-policy", p.ID); err != nil {
		t.Fatalf("DeleteResourcePolicy() error = %v", err)
	}

	actions2, err := q.GetResourcePolicies(ctx, "proj-policy", "job", "job-123", "user-pol")
	if err != nil {
		t.Fatalf("GetResourcePolicies() after delete error = %v", err)
	}
	if actions2 != nil {
		t.Fatalf("actions after delete = %v, want nil", actions2)
	}
}

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

func TestDeleteResourcePolicy_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, _, err := q.DeleteResourcePolicy(ctx, "proj-missing", "nonexistent-policy-id")
	if !errors.Is(err, store.ErrResourcePolicyNotFound) {
		t.Fatalf("DeleteResourcePolicy() = %v, want ErrResourcePolicyNotFound", err)
	}
}

func TestRemoveMemberRole_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	err := q.RemoveMemberRole(ctx, "proj-nonexistent", "user-nonexistent")
	if !errors.Is(err, store.ErrMemberNotFound) {
		t.Fatalf("RemoveMemberRole() = %v, want ErrMemberNotFound", err)
	}
}

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

	members, err := q.ListProjectMembers(ctx, "proj-list-members", 100, nil)
	if err != nil {
		t.Fatalf("ListProjectMembers() error = %v", err)
	}
	if len(members) != 3 {
		t.Fatalf("len(members) = %d, want 3", len(members))
	}
}

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

// Test hardening: RBAC store

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
	actions, err := q.GetResourcePolicies(ctx, "proj-rp-upsert", "job", "job-1", "user-1")
	if err != nil {
		t.Fatalf("GetResourcePolicies() error = %v", err)
	}
	if len(actions) != 2 {
		t.Fatalf("actions = %v, want 2 items", actions)
	}
}

func TestResourcePolicy_ProjectScopedLookupAndDelete(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	for _, projectID := range []string{"proj-rp-scope-a", "proj-rp-scope-b"} {
		p := &domain.ResourcePolicy{
			ProjectID:    projectID,
			ResourceType: "job",
			ResourceID:   "shared-job",
			UserID:       "shared-user",
			Actions:      []string{"read", projectID},
		}
		if err := q.CreateResourcePolicy(ctx, p); err != nil {
			t.Fatalf("CreateResourcePolicy(%s) error = %v", projectID, err)
		}
	}

	a, err := q.ListResourcePolicies(ctx, "proj-rp-scope-a", "job", "shared-job", 10, nil)
	if err != nil {
		t.Fatalf("ListResourcePolicies(project A) error = %v", err)
	}
	if len(a) != 1 || a[0].ProjectID != "proj-rp-scope-a" {
		t.Fatalf("project A policies = %+v, want exactly project A", a)
	}

	bActions, err := q.GetResourcePolicies(ctx, "proj-rp-scope-b", "job", "shared-job", "shared-user")
	if err != nil {
		t.Fatalf("GetResourcePolicies(project B) error = %v", err)
	}
	if !slices.Contains(bActions, "proj-rp-scope-b") {
		t.Fatalf("project B actions = %v, want project B marker", bActions)
	}

	if _, _, err := q.DeleteResourcePolicy(ctx, "proj-rp-scope-b", a[0].ID); !errors.Is(err, store.ErrResourcePolicyNotFound) {
		t.Fatalf("DeleteResourcePolicy(cross-project) error = %v, want ErrResourcePolicyNotFound", err)
	}
	if _, _, err := q.DeleteResourcePolicy(ctx, "proj-rp-scope-a", a[0].ID); err != nil {
		t.Fatalf("DeleteResourcePolicy(project A) error = %v", err)
	}
}

func TestListResourcePolicies_Empty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	policies, err := q.ListResourcePolicies(ctx, "proj-empty", "job", "nonexistent", 50, nil)
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

	policies, err := q.ListResourcePolicies(ctx, "proj-rp-multi", "workflow", "wf-1", 50, nil)
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

	actions, err := q.GetResourcePolicies(ctx, "proj-rp-wrong", "job", "job-1", "user-b")
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

	roles, err := q.ListProjectRoles(ctx, "proj-no-roles", 100, nil)
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

// Test hardening: Actors

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

// Test hardening: Jobs with new fields

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
	seedRetentionSideRows(t, ctx, run.ID)

	// Delete should succeed now (only completed runs).
	if err := q.DeleteJob(ctx, job.ID); err != nil {
		t.Fatalf("DeleteJob(completed runs) error = %v (should succeed)", err)
	}
	assertNoRunRetentionSideRows(t, ctx, run.ID)
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

// Test hardening: Workflows with new fields

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

// Test hardening: Tags queries

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

func TestListRunsByTag(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)
	pq := queue.NewPostgresRunWriter(testDB.Pool)

	projectID := "proj-runtag-" + newID()
	job := &domain.Job{
		ProjectID:   projectID,
		Name:        "RunTag Job",
		Slug:        "run-tag-" + newID(),
		EndpointURL: "https://example.com/hook",
		Enabled:     true,
	}
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("create job: %v", err)
	}

	run := &domain.JobRun{
		JobID:         job.ID,
		ProjectID:     projectID,
		Tags:          map[string]string{"team": "infra"},
		Status:        domain.StatusQueued,
		Attempt:       1,
		TriggeredBy:   domain.TriggerManual,
		JobVersion:    job.Version,
		ExecutionMode: domain.ExecutionModeHTTP,
	}
	if err := pq.Enqueue(ctx, run); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	run2 := &domain.JobRun{
		JobID:         job.ID,
		ProjectID:     projectID,
		Tags:          map[string]string{"team": "platform"},
		Status:        domain.StatusQueued,
		Attempt:       1,
		TriggeredBy:   domain.TriggerManual,
		JobVersion:    job.Version,
		ExecutionMode: domain.ExecutionModeHTTP,
	}
	if err := pq.Enqueue(ctx, run2); err != nil {
		t.Fatalf("enqueue run2: %v", err)
	}

	runs, err := q.ListRunsByTag(ctx, projectID, "team", "infra", 100, nil)
	if err != nil {
		t.Fatalf("ListRunsByTag() error = %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 tagged run, got %d", len(runs))
	}
	if runs[0].ID != run.ID {
		t.Fatalf("expected run %s, got %s", run.ID, runs[0].ID)
	}
}

func TestListWorkflowRunsByTag(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-wfruntag-" + newID()
	wf := &domain.Workflow{ProjectID: projectID, Name: "WF RunTag", Slug: "wf-runtag-" + newID(), Tags: map[string]string{"env": "staging"}, Enabled: true}
	if err := q.CreateWorkflow(ctx, wf); err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}

	wfRun := &domain.WorkflowRun{
		WorkflowID:      wf.ID,
		ProjectID:       projectID,
		Tags:            map[string]string{"release": "v2"},
		Status:          domain.WfStatusPending,
		TriggeredBy:     domain.TriggerManual,
		WorkflowVersion: wf.Version,
	}
	if err := q.CreateWorkflowRun(ctx, wfRun); err != nil {
		t.Fatalf("CreateWorkflowRun() error = %v", err)
	}

	wfRun2 := &domain.WorkflowRun{
		WorkflowID:      wf.ID,
		ProjectID:       projectID,
		Tags:            map[string]string{"release": "v1"},
		Status:          domain.WfStatusPending,
		TriggeredBy:     domain.TriggerManual,
		WorkflowVersion: wf.Version,
	}
	if err := q.CreateWorkflowRun(ctx, wfRun2); err != nil {
		t.Fatalf("CreateWorkflowRun() 2 error = %v", err)
	}

	runs, err := q.ListWorkflowRunsByTag(ctx, projectID, "release", "v2", 100, nil)
	if err != nil {
		t.Fatalf("ListWorkflowRunsByTag() error = %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 tagged workflow run, got %d", len(runs))
	}
	if runs[0].ID != wfRun.ID {
		t.Fatalf("expected run %s, got %s", wfRun.ID, runs[0].ID)
	}
}

func TestAuditEvents_CreateAndListFiltersAndSort(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-audit-events-" + newID()
	base := time.Now().UTC().Add(-10 * time.Minute).Truncate(time.Microsecond)

	ev1 := &domain.AuditEvent{ProjectID: projectID, ActorID: "actor-a", ActorType: "user", Action: "job.create", ResourceType: "job", ResourceID: "job-1"}
	if err := q.CreateAuditEvent(ctx, ev1); err != nil {
		t.Fatalf("CreateAuditEvent(ev1) error = %v", err)
	}
	if string(ev1.Details) != "{}" {
		t.Fatalf("CreateAuditEvent(ev1) details = %s, want {}", string(ev1.Details))
	}

	ev2 := &domain.AuditEvent{ProjectID: projectID, ActorID: "actor-a", ActorType: "user", Action: "job.update", ResourceType: "job", ResourceID: "job-2", Details: json.RawMessage(`{"changed":true}`)}
	if err := q.CreateAuditEvent(ctx, ev2); err != nil {
		t.Fatalf("CreateAuditEvent(ev2) error = %v", err)
	}

	ev3 := &domain.AuditEvent{ProjectID: projectID, ActorID: "actor-b", ActorType: "api_key", Action: "workflow.update", ResourceType: "workflow", ResourceID: "wf-1"}
	if err := q.CreateAuditEvent(ctx, ev3); err != nil {
		t.Fatalf("CreateAuditEvent(ev3) error = %v", err)
	}

	evOther := &domain.AuditEvent{ProjectID: "project-other-audit-" + newID(), ActorID: "actor-a", ActorType: "user", Action: "job.create", ResourceType: "job", ResourceID: "job-x"}
	if err := q.CreateAuditEvent(ctx, evOther); err != nil {
		t.Fatalf("CreateAuditEvent(evOther) error = %v", err)
	}

	if _, err := testDB.Pool.Exec(ctx, `UPDATE audit_events SET created_at = $2 WHERE id = $1`, ev1.ID, base.Add(1*time.Minute)); err != nil {
		t.Fatalf("set created_at ev1 error = %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `UPDATE audit_events SET created_at = $2 WHERE id = $1`, ev2.ID, base.Add(2*time.Minute)); err != nil {
		t.Fatalf("set created_at ev2 error = %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `UPDATE audit_events SET created_at = $2 WHERE id = $1`, ev3.ID, base.Add(3*time.Minute)); err != nil {
		t.Fatalf("set created_at ev3 error = %v", err)
	}

	allDesc, err := q.ListAuditEvents(ctx, projectID, "", "", "", 10, nil, nil, nil, false)
	if err != nil {
		t.Fatalf("ListAuditEvents(desc) error = %v", err)
	}
	if len(allDesc) != 3 {
		t.Fatalf("ListAuditEvents(desc) len = %d, want 3", len(allDesc))
	}
	if allDesc[0].ID != ev3.ID || allDesc[1].ID != ev2.ID || allDesc[2].ID != ev1.ID {
		t.Fatalf("ListAuditEvents(desc) order IDs = [%q %q %q], want [%q %q %q]", allDesc[0].ID, allDesc[1].ID, allDesc[2].ID, ev3.ID, ev2.ID, ev1.ID)
	}

	allAsc, err := q.ListAuditEvents(ctx, projectID, "", "", "", 10, nil, nil, nil, true)
	if err != nil {
		t.Fatalf("ListAuditEvents(asc) error = %v", err)
	}
	if len(allAsc) != 3 {
		t.Fatalf("ListAuditEvents(asc) len = %d, want 3", len(allAsc))
	}
	if allAsc[0].ID != ev1.ID || allAsc[1].ID != ev2.ID || allAsc[2].ID != ev3.ID {
		t.Fatalf("ListAuditEvents(asc) order IDs = [%q %q %q], want [%q %q %q]", allAsc[0].ID, allAsc[1].ID, allAsc[2].ID, ev1.ID, ev2.ID, ev3.ID)
	}

	actorA, err := q.ListAuditEvents(ctx, projectID, "actor-a", "", "", 10, nil, nil, nil, false)
	if err != nil {
		t.Fatalf("ListAuditEvents(actor) error = %v", err)
	}
	if len(actorA) != 2 {
		t.Fatalf("ListAuditEvents(actor) len = %d, want 2", len(actorA))
	}

	resource, err := q.ListAuditEvents(ctx, projectID, "", "job", "job-2", 10, nil, nil, nil, false)
	if err != nil {
		t.Fatalf("ListAuditEvents(resource) error = %v", err)
	}
	if len(resource) != 1 || resource[0].ID != ev2.ID {
		t.Fatalf("ListAuditEvents(resource) = %+v, want only %q", resource, ev2.ID)
	}
}

func TestAuditEvents_PaginationAndTimeRange(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-audit-page-" + newID()
	base := time.Now().UTC().Add(-20 * time.Minute).Truncate(time.Microsecond)

	ids := make([]string, 0, 4)
	for i := range 4 {
		ev := &domain.AuditEvent{
			ProjectID:    projectID,
			ActorID:      "actor-page",
			ActorType:    "user",
			Action:       "job.update",
			ResourceType: "job",
			ResourceID:   "job-page",
		}
		if err := q.CreateAuditEvent(ctx, ev); err != nil {
			t.Fatalf("CreateAuditEvent(%d) error = %v", i, err)
		}
		ids = append(ids, ev.ID)
		if _, err := testDB.Pool.Exec(ctx, `UPDATE audit_events SET created_at = $2 WHERE id = $1`, ev.ID, base.Add(time.Duration(i+1)*time.Minute)); err != nil {
			t.Fatalf("set created_at(%d) error = %v", i, err)
		}
	}

	page1, err := q.ListAuditEvents(ctx, projectID, "", "", "", 2, nil, nil, nil, false)
	if err != nil {
		t.Fatalf("ListAuditEvents(page1) error = %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("ListAuditEvents(page1) len = %d, want 2", len(page1))
	}

	cursor := page1[len(page1)-1].CreatedAt
	page2, err := q.ListAuditEvents(ctx, projectID, "", "", "", 2, &cursor, nil, nil, false)
	if err != nil {
		t.Fatalf("ListAuditEvents(page2) error = %v", err)
	}
	if len(page2) != 2 {
		t.Fatalf("ListAuditEvents(page2) len = %d, want 2", len(page2))
	}
	for _, p1 := range page1 {
		for _, p2 := range page2 {
			if p1.ID == p2.ID {
				t.Fatalf("pagination overlap id = %q", p1.ID)
			}
		}
	}

	from := base.Add(2 * time.Minute)
	to := base.Add(3 * time.Minute)
	ranged, err := q.ListAuditEvents(ctx, projectID, "", "", "", 10, nil, &from, &to, true)
	if err != nil {
		t.Fatalf("ListAuditEvents(range) error = %v", err)
	}
	if len(ranged) != 2 {
		t.Fatalf("ListAuditEvents(range) len = %d, want 2", len(ranged))
	}
	if ranged[0].ID != ids[1] || ranged[1].ID != ids[2] {
		t.Fatalf("ListAuditEvents(range) IDs = [%q %q], want [%q %q]", ranged[0].ID, ranged[1].ID, ids[1], ids[2])
	}
}

func TestTagPolicy_CreateListDelete(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-tag-policy-" + newID()
	userID := "user-" + newID()

	p1 := &domain.TagPolicy{ProjectID: projectID, ResourceType: "job", UserID: userID, TagKey: "team", TagValue: "payments", Actions: []string{"jobs:read", "jobs:trigger"}}
	if err := q.CreateTagPolicy(ctx, p1); err != nil {
		t.Fatalf("CreateTagPolicy(p1) error = %v", err)
	}
	if p1.ID == "" || p1.CreatedAt.IsZero() {
		t.Fatalf("CreateTagPolicy(p1) did not set ID/CreatedAt: %+v", *p1)
	}

	p2 := &domain.TagPolicy{ProjectID: projectID, ResourceType: "job", UserID: userID, TagKey: "env", Actions: []string{"jobs:read"}}
	if err := q.CreateTagPolicy(ctx, p2); err != nil {
		t.Fatalf("CreateTagPolicy(p2) error = %v", err)
	}

	list, err := q.ListTagPolicies(ctx, projectID, "job", userID, 10, nil)
	if err != nil {
		t.Fatalf("ListTagPolicies() error = %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("ListTagPolicies() len = %d, want 2", len(list))
	}

	cursor := list[len(list)-1].CreatedAt
	next, err := q.ListTagPolicies(ctx, projectID, "job", userID, 10, &cursor)
	if err != nil {
		t.Fatalf("ListTagPolicies(cursor) error = %v", err)
	}
	if len(next) != 0 {
		t.Fatalf("ListTagPolicies(cursor) len = %d, want 0", len(next))
	}

	deletedProjectID, deletedUserID, err := q.DeleteTagPolicy(ctx, projectID, p1.ID)
	if err != nil {
		t.Fatalf("DeleteTagPolicy() error = %v", err)
	}
	if deletedProjectID != projectID || deletedUserID != userID {
		t.Fatalf("DeleteTagPolicy() returned project/user = %q/%q, want %q/%q", deletedProjectID, deletedUserID, projectID, userID)
	}

	_, _, err = q.DeleteTagPolicy(ctx, projectID, p1.ID)
	if !errors.Is(err, store.ErrTagPolicyNotFound) {
		t.Fatalf("DeleteTagPolicy(not found) error = %v, want ErrTagPolicyNotFound", err)
	}
}

func TestTagPolicy_DeleteIsProjectScoped(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	p := &domain.TagPolicy{ProjectID: "project-tag-scope-a", ResourceType: "job", UserID: "user-tag-scope", TagKey: "team", TagValue: "payments", Actions: []string{"jobs:read"}}
	if err := q.CreateTagPolicy(ctx, p); err != nil {
		t.Fatalf("CreateTagPolicy() error = %v", err)
	}
	if _, _, err := q.DeleteTagPolicy(ctx, "project-tag-scope-b", p.ID); !errors.Is(err, store.ErrTagPolicyNotFound) {
		t.Fatalf("DeleteTagPolicy(cross-project) error = %v, want ErrTagPolicyNotFound", err)
	}
	if _, _, err := q.DeleteTagPolicy(ctx, "project-tag-scope-a", p.ID); err != nil {
		t.Fatalf("DeleteTagPolicy(owner project) error = %v", err)
	}
}

func TestTagPolicy_GetActions(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-tag-actions-" + newID()
	userID := "user-" + newID()

	policies := []*domain.TagPolicy{
		{ProjectID: projectID, ResourceType: "job", UserID: userID, TagKey: "team", TagValue: "payments", Actions: []string{"jobs:read", "jobs:trigger"}},
		{ProjectID: projectID, ResourceType: "job", UserID: userID, TagKey: "env", TagValue: "", Actions: []string{"jobs:read", "jobs:write"}},
		{ProjectID: projectID, ResourceType: "job", UserID: "other-user", TagKey: "team", TagValue: "payments", Actions: []string{"jobs:admin"}},
	}
	for i, p := range policies {
		if err := q.CreateTagPolicy(ctx, p); err != nil {
			t.Fatalf("CreateTagPolicy(%d) error = %v", i, err)
		}
	}

	tags := map[string]string{"team": "payments", "env": "prod"}
	actions, err := q.GetTagPolicyActions(ctx, projectID, "job", userID, tags)
	if err != nil {
		t.Fatalf("GetTagPolicyActions() error = %v", err)
	}
	if len(actions) != 3 {
		t.Fatalf("GetTagPolicyActions() len = %d, want 3", len(actions))
	}
	want := map[string]bool{"jobs:read": true, "jobs:trigger": true, "jobs:write": true}
	for _, action := range actions {
		if !want[action] {
			t.Fatalf("unexpected action %q", action)
		}
		delete(want, action)
	}
	if len(want) != 0 {
		t.Fatalf("missing actions: %v", want)
	}

	none, err := q.GetTagPolicyActions(ctx, projectID, "job", userID, map[string]string{"team": "core"})
	if err != nil {
		t.Fatalf("GetTagPolicyActions(non-match) error = %v", err)
	}
	if none != nil {
		t.Fatalf("GetTagPolicyActions(non-match) = %v, want nil", none)
	}
}

func TestSeedProjectSystemRoles_Idempotent(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-seed-roles-" + newID()
	if err := q.SeedProjectSystemRoles(ctx, projectID); err != nil {
		t.Fatalf("SeedProjectSystemRoles(first) error = %v", err)
	}
	if err := q.SeedProjectSystemRoles(ctx, projectID); err != nil {
		t.Fatalf("SeedProjectSystemRoles(second) error = %v", err)
	}

	roles, err := q.ListProjectRoles(ctx, projectID, 50, nil)
	if err != nil {
		t.Fatalf("ListProjectRoles() error = %v", err)
	}
	if len(roles) != len(domain.SystemRolePermissions) {
		t.Fatalf("ListProjectRoles() len = %d, want %d", len(roles), len(domain.SystemRolePermissions))
	}
	seen := make(map[string]bool, len(roles))
	for _, role := range roles {
		seen[role.Name] = true
		if !role.IsSystem {
			t.Fatalf("role %q IsSystem = false, want true", role.Name)
		}
	}
	for name := range domain.SystemRolePermissions {
		if !seen[name] {
			t.Fatalf("missing seeded role %q", name)
		}
	}
}

func TestGetAPIKeyByID(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	key := &domain.APIKey{ProjectID: "project-api-by-id-" + newID(), Name: "key-by-id", KeyHash: "hash-" + newID(), KeyPrefix: "sk_by_id", Scopes: []string{"jobs:read"}}
	if err := q.CreateAPIKey(ctx, key); err != nil {
		t.Fatalf("CreateAPIKey() error = %v", err)
	}

	got, err := q.GetAPIKeyByID(ctx, key.ID)
	if err != nil {
		t.Fatalf("GetAPIKeyByID() error = %v", err)
	}
	if got.ID != key.ID || got.ProjectID != key.ProjectID || got.KeyHash != key.KeyHash {
		t.Fatalf("GetAPIKeyByID() mismatch: got %+v want %+v", *got, *key)
	}
}

func TestMarkAPIKeyRotated(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-api-rotate-" + newID()
	oldKey := &domain.APIKey{ProjectID: projectID, Name: "old", KeyHash: "hash-" + newID(), KeyPrefix: "sk_old", Scopes: []string{"jobs:read"}}
	newKey := &domain.APIKey{ProjectID: projectID, Name: "new", KeyHash: "hash-" + newID(), KeyPrefix: "sk_new", Scopes: []string{"jobs:read"}}
	if err := q.CreateAPIKey(ctx, oldKey); err != nil {
		t.Fatalf("CreateAPIKey(old) error = %v", err)
	}
	if err := q.CreateAPIKey(ctx, newKey); err != nil {
		t.Fatalf("CreateAPIKey(new) error = %v", err)
	}

	grace := time.Now().UTC().Add(30 * time.Minute).Truncate(time.Microsecond)
	if err := q.MarkAPIKeyRotated(ctx, oldKey.ID, newKey.ID, grace); err != nil {
		t.Fatalf("MarkAPIKeyRotated() error = %v", err)
	}

	got, err := q.GetAPIKeyByID(ctx, oldKey.ID)
	if err != nil {
		t.Fatalf("GetAPIKeyByID(old) error = %v", err)
	}
	if got.ReplacedByKeyID != newKey.ID {
		t.Fatalf("ReplacedByKeyID = %q, want %q", got.ReplacedByKeyID, newKey.ID)
	}
	if got.GraceExpiresAt == nil || !got.GraceExpiresAt.Equal(grace) {
		t.Fatalf("GraceExpiresAt = %v, want %v", got.GraceExpiresAt, grace)
	}
}

func TestCreateRotatedAPIKey_AtomicallyCreatesAndLinks(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-api-create-rotated-" + newID()
	oldKey := &domain.APIKey{ProjectID: projectID, Name: "old", KeyHash: "hash-" + newID(), KeyPrefix: "sk_old", Scopes: []string{"jobs:read"}}
	newKey := &domain.APIKey{ProjectID: projectID, Name: "new", KeyHash: "hash-" + newID(), KeyPrefix: "sk_new", Scopes: []string{"jobs:read"}}
	if err := q.CreateAPIKey(ctx, oldKey); err != nil {
		t.Fatalf("CreateAPIKey(old) error = %v", err)
	}

	grace := time.Now().UTC().Add(30 * time.Minute).Truncate(time.Microsecond)
	if err := q.CreateRotatedAPIKey(ctx, oldKey.ID, newKey, grace); err != nil {
		t.Fatalf("CreateRotatedAPIKey() error = %v", err)
	}
	if newKey.ID == "" {
		t.Fatal("CreateRotatedAPIKey() did not set new key ID")
	}

	oldStored, err := q.GetAPIKeyByID(ctx, oldKey.ID)
	if err != nil {
		t.Fatalf("GetAPIKeyByID(old) error = %v", err)
	}
	if oldStored.ReplacedByKeyID != newKey.ID {
		t.Fatalf("ReplacedByKeyID = %q, want %q", oldStored.ReplacedByKeyID, newKey.ID)
	}
	newStored, err := q.GetAPIKeyByID(ctx, newKey.ID)
	if err != nil {
		t.Fatalf("GetAPIKeyByID(new) error = %v", err)
	}
	if newStored.RevokedAt != nil {
		t.Fatalf("new key revoked_at = %v, want nil", newStored.RevokedAt)
	}
}

func TestCreateRotatedAPIKey_RollsBackNewKeyWhenOldKeyCannotBeLinked(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-api-create-rotated-rollback-" + newID()
	newKey := &domain.APIKey{ProjectID: projectID, Name: "new", KeyHash: "hash-" + newID(), KeyPrefix: "sk_new", Scopes: []string{"jobs:read"}}
	if err := q.CreateRotatedAPIKey(ctx, "missing-old-key", newKey, time.Now().UTC().Add(time.Hour)); err == nil {
		t.Fatal("CreateRotatedAPIKey() error = nil, want old key link failure")
	}
	if newKey.ID == "" {
		t.Fatal("CreateRotatedAPIKey() did not assign new key ID before rollback")
	}
	if _, err := q.GetAPIKeyByID(ctx, newKey.ID); err == nil {
		t.Fatal("rolled-back rotated key remained queryable by ID")
	}
	if _, err := q.GetAPIKeyByHash(ctx, newKey.KeyHash); err == nil {
		t.Fatal("rolled-back rotated key remained queryable by hash")
	}
}

func TestGetJobAtVersion(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-job-at-version-" + newID()
	job := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{
		ProjectID:   new(projectID),
		Name:        new("job-v1"),
		Slug:        new("job-at-version-" + newID()),
		EndpointURL: new("https://example.com/v1"),
	})
	jobV1Name := job.Name
	jobV1Endpoint := job.EndpointURL

	job.Name = "job-v2"
	job.EndpointURL = "https://example.com/v2"
	job.UpdatedBy = "user-update"
	if err := q.UpdateJob(ctx, job); err != nil {
		t.Fatalf("UpdateJob() error = %v", err)
	}

	v1, err := q.GetJobAtVersion(ctx, job.ID, 1)
	if err != nil {
		t.Fatalf("GetJobAtVersion(v1) error = %v", err)
	}
	if v1.Name != jobV1Name || v1.EndpointURL != jobV1Endpoint || v1.Version != 1 {
		t.Fatalf("GetJobAtVersion(v1) = %+v, want name=%q endpoint=%q version=1", *v1, jobV1Name, jobV1Endpoint)
	}

	v2, err := q.GetJobAtVersion(ctx, job.ID, job.Version)
	if err != nil {
		t.Fatalf("GetJobAtVersion(v2) error = %v", err)
	}
	if v2.Name != job.Name || v2.EndpointURL != job.EndpointURL {
		t.Fatalf("GetJobAtVersion(v2) = %+v, want live job %+v", *v2, *job)
	}
}

func TestGetJobVersionByVersionID(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-job-version-id-" + newID()
	job := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{
		ProjectID:   new(projectID),
		Name:        new("job-version-id-v1"),
		Slug:        new("job-version-id-" + newID()),
		EndpointURL: new("https://example.com/version-id-v1"),
	})

	job.Name = "job-version-id-v2"
	if err := q.UpdateJob(ctx, job); err != nil {
		t.Fatalf("UpdateJob() error = %v", err)
	}

	versions, err := q.ListJobVersionsByJob(ctx, job.ID, 10, nil)
	if err != nil {
		t.Fatalf("ListJobVersionsByJob() error = %v", err)
	}
	if len(versions) != 1 {
		t.Fatalf("ListJobVersionsByJob() len = %d, want 1", len(versions))
	}
	if versions[0].VersionID == "" {
		t.Fatal("ListJobVersionsByJob()[0].VersionID is empty")
	}

	got, err := q.GetJobVersionByVersionID(ctx, versions[0].VersionID)
	if err != nil {
		t.Fatalf("GetJobVersionByVersionID() error = %v", err)
	}
	if got.ID != versions[0].ID || got.Version != 1 || got.Name != "job-version-id-v1" {
		t.Fatalf("GetJobVersionByVersionID() = %+v, want id=%q version=1 name=%q", *got, versions[0].ID, "job-version-id-v1")
	}
}

func TestListWorkflowVersions(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-workflow-versions-" + newID()
	job := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: new(projectID)})
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID:   new(projectID),
		Name:        new("wf-versions"),
		Slug:        new("wf-versions-" + newID()),
		Description: new("workflow versions test"),
	})
	if _, err := testDB.Pool.Exec(ctx, `UPDATE workflows SET cron = $2, cron_timezone = $3 WHERE id = $1`, wf.ID, "*/5 * * * *", "UTC"); err != nil {
		t.Fatalf("set workflow cron fields error = %v", err)
	}
	wf.Cron = "*/5 * * * *"
	wf.CronTimezone = "UTC"

	stepRef := "step-" + newID()
	if err := q.CreateWorkflowStep(ctx, &domain.WorkflowStep{ID: newID(), WorkflowID: wf.ID, JobID: job.ID, StepRef: stepRef}); err != nil {
		t.Fatalf("CreateWorkflowStep() error = %v", err)
	}
	if err := q.CreateWorkflowVersionSnapshot(ctx, wf.ID, wf.Version); err != nil {
		t.Fatalf("CreateWorkflowVersionSnapshot(v1) error = %v", err)
	}
	v1VersionID := "wfv1-" + newID()
	if _, err := testDB.Pool.Exec(ctx, `UPDATE workflow_versions SET version_id = $3 WHERE workflow_id = $1 AND version = $2`, wf.ID, 1, v1VersionID); err != nil {
		t.Fatalf("set workflow version_id v1 error = %v", err)
	}

	wf.Name = "wf-versions-v2"
	if err := q.UpdateWorkflow(ctx, wf); err != nil {
		t.Fatalf("UpdateWorkflow() error = %v", err)
	}
	if err := q.CreateWorkflowVersionSnapshot(ctx, wf.ID, wf.Version); err != nil {
		t.Fatalf("CreateWorkflowVersionSnapshot(v2) error = %v", err)
	}
	v2VersionID := "wfv2-" + newID()
	if _, err := testDB.Pool.Exec(ctx, `UPDATE workflow_versions SET version_id = $3 WHERE workflow_id = $1 AND version = $2`, wf.ID, 2, v2VersionID); err != nil {
		t.Fatalf("set workflow version_id v2 error = %v", err)
	}

	versions, err := q.ListWorkflowVersions(ctx, wf.ID, 10)
	if err != nil {
		t.Fatalf("ListWorkflowVersions() error = %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("ListWorkflowVersions() len = %d, want 2", len(versions))
	}
	if versions[0].Version != 2 || versions[1].Version != 1 {
		t.Fatalf("ListWorkflowVersions() order versions = [%d %d], want [2 1]", versions[0].Version, versions[1].Version)
	}
}

func TestGetWorkflowVersionByVersionID(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-workflow-version-id-" + newID()
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID:   new(projectID),
		Name:        new("wf-version-id"),
		Slug:        new("wf-version-id-" + newID()),
		Description: new("workflow version id test"),
	})
	if _, err := testDB.Pool.Exec(ctx, `UPDATE workflows SET cron = $2, cron_timezone = $3 WHERE id = $1`, wf.ID, "*/10 * * * *", "UTC"); err != nil {
		t.Fatalf("set workflow cron fields error = %v", err)
	}

	if err := q.CreateWorkflowVersionSnapshot(ctx, wf.ID, wf.Version); err != nil {
		t.Fatalf("CreateWorkflowVersionSnapshot() error = %v", err)
	}
	versionID := "wfid-" + newID()
	if _, err := testDB.Pool.Exec(ctx, `UPDATE workflow_versions SET version_id = $3 WHERE workflow_id = $1 AND version = $2`, wf.ID, wf.Version, versionID); err != nil {
		t.Fatalf("set workflow version_id error = %v", err)
	}

	versions, err := q.ListWorkflowVersions(ctx, wf.ID, 10)
	if err != nil {
		t.Fatalf("ListWorkflowVersions() error = %v", err)
	}
	if len(versions) != 1 {
		t.Fatalf("ListWorkflowVersions() len = %d, want 1", len(versions))
	}
	if versions[0].VersionID == "" {
		t.Fatal("ListWorkflowVersions()[0].VersionID is empty")
	}

	got, err := q.GetWorkflowVersionByVersionID(ctx, wf.ID, versions[0].VersionID)
	if err != nil {
		t.Fatalf("GetWorkflowVersionByVersionID() error = %v", err)
	}
	if got.ID != versions[0].ID || got.WorkflowID != wf.ID || got.Version != wf.Version {
		t.Fatalf("GetWorkflowVersionByVersionID() = %+v, want %+v", *got, versions[0])
	}
}

func TestRetryWebhookDelivery(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-webhook-retry-" + newID()
	job := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{
		ProjectID:     new(projectID),
		WebhookURL:    new("https://example.com/webhook-retry"),
		WebhookSecret: new("whsec-retry"),
	})
	run := testutil.MustCreateRun(t, ctx, q, job, nil)

	sub := &domain.WebhookSubscription{ProjectID: projectID, WebhookURL: "https://example.com/sub-retry", EventTypes: []string{"run.completed"}, Secret: "secret", Active: true}
	if err := q.CreateWebhookSubscription(ctx, sub); err != nil {
		t.Fatalf("CreateWebhookSubscription() error = %v", err)
	}

	statusCode := 502
	failed := &domain.WebhookDelivery{
		RunID:          run.ID,
		JobID:          job.ID,
		WebhookURL:     job.WebhookURL,
		RetryPolicy:    domain.WebhookRetryPolicyExponential,
		Status:         domain.WebhookStatusFailed,
		Attempts:       3,
		MaxAttempts:    5,
		LastStatusCode: &statusCode,
		LastError:      "bad gateway",
	}
	if err := q.CreateWebhookDelivery(ctx, failed); err != nil {
		t.Fatalf("CreateWebhookDelivery(failed) error = %v", err)
	}

	retried, err := q.RetryWebhookDelivery(ctx, failed.ID)
	if err != nil {
		t.Fatalf("RetryWebhookDelivery() error = %v", err)
	}
	if retried.Status != domain.WebhookStatusPending || retried.Attempts != 0 {
		t.Fatalf("RetryWebhookDelivery() status/attempts = %q/%d, want %q/0", retried.Status, retried.Attempts, domain.WebhookStatusPending)
	}
	if retried.NextRetryAt == nil {
		t.Fatal("RetryWebhookDelivery() next_retry_at = nil, want non-nil")
	}
	if retried.LastStatusCode != nil || retried.LastError != "" || retried.DeliveredAt != nil {
		t.Fatalf("RetryWebhookDelivery() reset fields mismatch: %+v", *retried)
	}
}

func TestListPendingWebhookRetries(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-webhook-pending-" + newID()
	job := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{
		ProjectID:     new(projectID),
		WebhookURL:    new("https://example.com/webhook-pending"),
		WebhookSecret: new("whsec-pending"),
	})
	run := testutil.MustCreateRun(t, ctx, q, job, nil)

	sub := &domain.WebhookSubscription{ProjectID: projectID, WebhookURL: "https://example.com/sub-pending", EventTypes: []string{"run.failed"}, Secret: "secret", Active: true}
	if err := q.CreateWebhookSubscription(ctx, sub); err != nil {
		t.Fatalf("CreateWebhookSubscription() error = %v", err)
	}

	past := time.Now().UTC().Add(-2 * time.Minute)
	future := time.Now().UTC().Add(20 * time.Minute)
	due := &domain.WebhookDelivery{RunID: run.ID, JobID: job.ID, WebhookURL: job.WebhookURL, Status: domain.WebhookStatusPending, Attempts: 1, MaxAttempts: 5, NextRetryAt: &past}
	notDue := &domain.WebhookDelivery{RunID: run.ID, JobID: job.ID, WebhookURL: job.WebhookURL, Status: domain.WebhookStatusPending, Attempts: 1, MaxAttempts: 5, NextRetryAt: &future}
	failedDue := &domain.WebhookDelivery{RunID: run.ID, JobID: job.ID, WebhookURL: job.WebhookURL, Status: domain.WebhookStatusFailed, Attempts: 1, MaxAttempts: 5, NextRetryAt: &past}
	for i, d := range []*domain.WebhookDelivery{due, notDue, failedDue} {
		if err := q.CreateWebhookDelivery(ctx, d); err != nil {
			t.Fatalf("CreateWebhookDelivery(%d) error = %v", i, err)
		}
	}

	pending, err := q.ListPendingWebhookRetries(ctx)
	if err != nil {
		t.Fatalf("ListPendingWebhookRetries() error = %v", err)
	}
	if len(pending) != 1 || pending[0].ID != due.ID {
		t.Fatalf("ListPendingWebhookRetries() = %+v, want only %q", pending, due.ID)
	}
}

func TestClaimPendingWebhookRetries_LeaseAndTokenBoundUpdate(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-webhook-claim-" + newID()
	job := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{
		ProjectID:  new(projectID),
		WebhookURL: new("https://example.com/webhook-claim"),
	})
	run := testutil.MustCreateRun(t, ctx, q, job, nil)

	past := time.Now().UTC().Add(-2 * time.Minute)
	due := &domain.WebhookDelivery{
		RunID:         run.ID,
		JobID:         job.ID,
		WebhookURL:    job.WebhookURL,
		WebhookSecret: "claim-secret",
		Status:        domain.WebhookStatusPending,
		Attempts:      1,
		MaxAttempts:   5,
		NextRetryAt:   &past,
	}
	if err := q.CreateWebhookDelivery(ctx, due); err != nil {
		t.Fatalf("CreateWebhookDelivery() error = %v", err)
	}

	claimed, err := q.ClaimPendingWebhookRetries(ctx, 10, time.Minute)
	if err != nil {
		t.Fatalf("ClaimPendingWebhookRetries(first) error = %v", err)
	}
	if len(claimed) != 1 || claimed[0].ID != due.ID {
		t.Fatalf("ClaimPendingWebhookRetries(first) = %+v, want only %q", claimed, due.ID)
	}
	if claimed[0].ClaimToken == "" || claimed[0].LeaseExpiresAt == nil {
		t.Fatalf("claimed delivery missing lease fields: %+v", claimed[0])
	}
	if claimed[0].WebhookSecret != due.WebhookSecret {
		t.Fatalf("claimed webhook_secret = %q, want %q", claimed[0].WebhookSecret, due.WebhookSecret)
	}

	claimedAgain, err := q.ClaimPendingWebhookRetries(ctx, 10, time.Minute)
	if err != nil {
		t.Fatalf("ClaimPendingWebhookRetries(second) error = %v", err)
	}
	if len(claimedAgain) != 0 {
		t.Fatalf("ClaimPendingWebhookRetries(second) len = %d, want 0", len(claimedAgain))
	}

	listed, err := q.ListPendingWebhookRetries(ctx)
	if err != nil {
		t.Fatalf("ListPendingWebhookRetries(claimed) error = %v", err)
	}
	if len(listed) != 0 {
		t.Fatalf("ListPendingWebhookRetries(claimed) len = %d, want 0", len(listed))
	}

	wrongToken := claimed[0]
	wrongToken.ClaimToken = "wrong-token"
	wrongToken.Status = domain.WebhookStatusDelivered
	now := time.Now().UTC()
	wrongToken.DeliveredAt = &now
	updated, err := q.UpdateClaimedWebhookDelivery(ctx, &wrongToken)
	if err != nil {
		t.Fatalf("UpdateClaimedWebhookDelivery(wrong token) error = %v", err)
	}
	if updated {
		t.Fatal("UpdateClaimedWebhookDelivery(wrong token) updated = true, want false")
	}

	if _, err := testDB.Pool.Exec(ctx, `
		UPDATE webhook_deliveries
		SET lease_expires_at = NOW() - INTERVAL '1 second'
		WHERE id = $1`, due.ID); err != nil {
		t.Fatalf("expire webhook claim lease: %v", err)
	}

	reclaimed, err := q.ClaimPendingWebhookRetries(ctx, 10, time.Minute)
	if err != nil {
		t.Fatalf("ClaimPendingWebhookRetries(after lease expiry) error = %v", err)
	}
	if len(reclaimed) != 1 || reclaimed[0].ID != due.ID {
		t.Fatalf("ClaimPendingWebhookRetries(after lease expiry) = %+v, want only %q", reclaimed, due.ID)
	}
	if reclaimed[0].ClaimToken == claimed[0].ClaimToken {
		t.Fatal("ClaimPendingWebhookRetries(after lease expiry) reused stale claim token")
	}

	statusCode := 200
	reclaimed[0].Status = domain.WebhookStatusDelivered
	reclaimed[0].DeliveredAt = &now
	reclaimed[0].NextRetryAt = nil
	reclaimed[0].LastStatusCode = &statusCode
	updated, err = q.UpdateClaimedWebhookDelivery(ctx, &reclaimed[0])
	if err != nil {
		t.Fatalf("UpdateClaimedWebhookDelivery(correct token) error = %v", err)
	}
	if !updated {
		t.Fatal("UpdateClaimedWebhookDelivery(correct token) updated = false, want true")
	}

	got, err := q.GetWebhookDelivery(ctx, due.ID)
	if err != nil {
		t.Fatalf("GetWebhookDelivery(after claimed update) error = %v", err)
	}
	if got.Status != domain.WebhookStatusDelivered || got.DeliveredAt == nil || got.NextRetryAt != nil {
		t.Fatalf("GetWebhookDelivery(after claimed update) = %+v, want delivered with no retry", *got)
	}
}

func TestDeleteOldWebhookDeliveries(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-webhook-cleanup-" + newID()
	job := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{
		ProjectID:     new(projectID),
		WebhookURL:    new("https://example.com/webhook-cleanup"),
		WebhookSecret: new("whsec-cleanup"),
	})
	run := testutil.MustCreateRun(t, ctx, q, job, nil)

	sub := &domain.WebhookSubscription{ProjectID: projectID, WebhookURL: "https://example.com/sub-cleanup", EventTypes: []string{"run.completed"}, Secret: "secret", Active: true}
	if err := q.CreateWebhookSubscription(ctx, sub); err != nil {
		t.Fatalf("CreateWebhookSubscription() error = %v", err)
	}

	deliveredOld := &domain.WebhookDelivery{RunID: run.ID, JobID: job.ID, WebhookURL: job.WebhookURL, Status: domain.WebhookStatusDelivered, Attempts: 1, MaxAttempts: 3}
	deadOld := &domain.WebhookDelivery{RunID: run.ID, JobID: job.ID, WebhookURL: job.WebhookURL, Status: domain.WebhookStatusDead, Attempts: 3, MaxAttempts: 3}
	deliveredRecent := &domain.WebhookDelivery{RunID: run.ID, JobID: job.ID, WebhookURL: job.WebhookURL, Status: domain.WebhookStatusDelivered, Attempts: 1, MaxAttempts: 3}
	pendingOld := &domain.WebhookDelivery{RunID: run.ID, JobID: job.ID, WebhookURL: job.WebhookURL, Status: domain.WebhookStatusPending, Attempts: 1, MaxAttempts: 3}
	for i, d := range []*domain.WebhookDelivery{deliveredOld, deadOld, deliveredRecent, pendingOld} {
		if err := q.CreateWebhookDelivery(ctx, d); err != nil {
			t.Fatalf("CreateWebhookDelivery(%d) error = %v", i, err)
		}
	}

	old := time.Now().UTC().Add(-48 * time.Hour)
	recent := time.Now().UTC().Add(-15 * time.Minute)
	if _, err := testDB.Pool.Exec(ctx, `UPDATE webhook_deliveries SET created_at = $2 WHERE id = $1`, deliveredOld.ID, old); err != nil {
		t.Fatalf("set created_at deliveredOld error = %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `UPDATE webhook_deliveries SET created_at = $2 WHERE id = $1`, deadOld.ID, old.Add(1*time.Minute)); err != nil {
		t.Fatalf("set created_at deadOld error = %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `UPDATE webhook_deliveries SET created_at = $2 WHERE id = $1`, deliveredRecent.ID, recent); err != nil {
		t.Fatalf("set created_at deliveredRecent error = %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `UPDATE webhook_deliveries SET created_at = $2 WHERE id = $1`, pendingOld.ID, old); err != nil {
		t.Fatalf("set created_at pendingOld error = %v", err)
	}

	deleted, err := q.DeleteOldWebhookDeliveries(ctx, time.Now().UTC().Add(-24*time.Hour), 10)
	if err != nil {
		t.Fatalf("DeleteOldWebhookDeliveries() error = %v", err)
	}
	if deleted != 2 {
		t.Fatalf("DeleteOldWebhookDeliveries() deleted = %d, want 2", deleted)
	}

	if _, err := q.GetWebhookDelivery(ctx, deliveredOld.ID); err == nil {
		t.Fatal("GetWebhookDelivery(deliveredOld) error = nil, want error")
	}
	if _, err := q.GetWebhookDelivery(ctx, deadOld.ID); err == nil {
		t.Fatal("GetWebhookDelivery(deadOld) error = nil, want error")
	}
	if _, err := q.GetWebhookDelivery(ctx, deliveredRecent.ID); err != nil {
		t.Fatalf("GetWebhookDelivery(deliveredRecent) error = %v", err)
	}
	if _, err := q.GetWebhookDelivery(ctx, pendingOld.ID); err != nil {
		t.Fatalf("GetWebhookDelivery(pendingOld) error = %v", err)
	}
}

func TestPauseJobsByGroup(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-pause-group-" + newID()
	group := &domain.JobGroup{ID: newID(), ProjectID: projectID, Name: "pause-group", Slug: "pause-group-" + newID()}
	if err := q.CreateJobGroup(ctx, group); err != nil {
		t.Fatalf("CreateJobGroup() error = %v", err)
	}

	inGroupA := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: new(projectID), Enabled: new(true)})
	inGroupB := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: new(projectID), Enabled: new(true)})
	outside := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: new(projectID), Enabled: new(true)})
	for i, jobID := range []string{inGroupA.ID, inGroupB.ID} {
		if _, err := testDB.Pool.Exec(ctx, `UPDATE jobs SET group_id = $1 WHERE id = $2`, group.ID, jobID); err != nil {
			t.Fatalf("assign group (%d) error = %v", i, err)
		}
	}

	if err := q.PauseJobsByGroup(ctx, group.ID); err != nil {
		t.Fatalf("PauseJobsByGroup() error = %v", err)
	}

	gotA, err := q.GetJob(ctx, inGroupA.ID)
	if err != nil {
		t.Fatalf("GetJob(inGroupA) error = %v", err)
	}
	gotB, err := q.GetJob(ctx, inGroupB.ID)
	if err != nil {
		t.Fatalf("GetJob(inGroupB) error = %v", err)
	}
	gotOut, err := q.GetJob(ctx, outside.ID)
	if err != nil {
		t.Fatalf("GetJob(outside) error = %v", err)
	}
	if !gotA.Paused || !gotB.Paused {
		t.Fatalf("jobs in group paused after pause: A=%v B=%v", gotA.Paused, gotB.Paused)
	}
	if gotOut.Paused {
		t.Fatal("outside job paused = true, want false")
	}
}

func TestResumeJobsByGroup(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-resume-group-" + newID()
	group := &domain.JobGroup{ID: newID(), ProjectID: projectID, Name: "resume-group", Slug: "resume-group-" + newID()}
	if err := q.CreateJobGroup(ctx, group); err != nil {
		t.Fatalf("CreateJobGroup() error = %v", err)
	}

	inGroupA := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: new(projectID), Enabled: new(true)})
	inGroupB := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: new(projectID), Enabled: new(true)})
	for i, jobID := range []string{inGroupA.ID, inGroupB.ID} {
		if _, err := testDB.Pool.Exec(ctx, `UPDATE jobs SET group_id = $1 WHERE id = $2`, group.ID, jobID); err != nil {
			t.Fatalf("assign group (%d) error = %v", i, err)
		}
	}

	// Pause first, then resume.
	if err := q.PauseJobsByGroup(ctx, group.ID); err != nil {
		t.Fatalf("PauseJobsByGroup() error = %v", err)
	}

	if err := q.ResumeJobsByGroup(ctx, group.ID); err != nil {
		t.Fatalf("ResumeJobsByGroup() error = %v", err)
	}

	gotA, err := q.GetJob(ctx, inGroupA.ID)
	if err != nil {
		t.Fatalf("GetJob(inGroupA) error = %v", err)
	}
	gotB, err := q.GetJob(ctx, inGroupB.ID)
	if err != nil {
		t.Fatalf("GetJob(inGroupB) error = %v", err)
	}
	if gotA.Paused || gotB.Paused {
		t.Fatalf("jobs in group still paused after resume: A=%v B=%v", gotA.Paused, gotB.Paused)
	}
}

func TestGetPerformanceAnalytics(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)
	pq := queue.NewPostgresRunWriter(testDB.Pool)

	projectID := "project-analytics-" + newID()
	job := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: new(projectID), Slug: new("analytics-job-" + newID())})

	statuses := []domain.RunStatus{
		domain.StatusCompleted,
		domain.StatusCompleted,
		domain.StatusCompleted,
		domain.StatusFailed,
		domain.StatusFailed,
		domain.StatusTimedOut,
	}

	now := time.Now().UTC()
	for i, st := range statuses {
		run := testutil.BuildRun(job, &testutil.RunOpts{Status: testutil.Ptr(domain.StatusQueued)})
		if err := pq.Enqueue(ctx, run); err != nil {
			t.Fatalf("Enqueue(%d) error = %v", i, err)
		}
		startedAt := now.Add(-time.Duration(i+12) * time.Minute)
		finishedAt := startedAt.Add(time.Duration(20+i*5) * time.Second)
		createdAt := startedAt.Add(-15 * time.Second)
		if _, err := testDB.Pool.Exec(ctx, `
			UPDATE job_runs
			SET status = $2, started_at = $3, finished_at = $4, created_at = $5
			WHERE id = $1
		`, run.ID, st, startedAt, finishedAt, createdAt); err != nil {
			t.Fatalf("update run(%d) status/times error = %v", i, err)
		}
	}

	analytics, err := q.GetPerformanceAnalytics(ctx, projectID, 24)
	if err != nil {
		t.Fatalf("GetPerformanceAnalytics() error = %v", err)
	}
	if analytics.Throughput.Completed != 3 || analytics.Throughput.Failed != 2 || analytics.Throughput.TimedOut != 1 {
		t.Fatalf("Throughput = %+v, want completed=3 failed=2 timed_out=1", analytics.Throughput)
	}
	if analytics.Throughput.PeriodHours != 24 {
		t.Fatalf("Throughput.PeriodHours = %d, want 24", analytics.Throughput.PeriodHours)
	}
	if len(analytics.SlowestJobs) != 1 {
		t.Fatalf("SlowestJobs len = %d, want 1", len(analytics.SlowestJobs))
	}
	if analytics.SlowestJobs[0].JobID != job.ID || analytics.SlowestJobs[0].TotalRuns != 6 || analytics.SlowestJobs[0].FailedRuns != 2 {
		t.Fatalf("SlowestJobs[0] = %+v, want job_id=%q total_runs=6 failed_runs=2", analytics.SlowestJobs[0], job.ID)
	}
	if analytics.HealthSummary.TotalJobs != 1 || analytics.HealthSummary.ActiveJobs != 1 {
		t.Fatalf("HealthSummary jobs = %+v, want total=1 active=1", analytics.HealthSummary)
	}
	if analytics.HealthSummary.SuccessRate < 0.49 || analytics.HealthSummary.SuccessRate > 0.51 {
		t.Fatalf("HealthSummary.SuccessRate = %f, want about 0.5", analytics.HealthSummary.SuccessRate)
	}
}

func TestGetJobHealthStats_RecentWindow(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)
	pq := queue.NewPostgresRunWriter(testDB.Pool)

	projectID := "project-health-window-" + newID()
	job := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: new(projectID)})

	now := time.Now().UTC()
	runRecentCompleted := testutil.BuildRun(job, nil)
	runRecentFailed := testutil.BuildRun(job, nil)
	runOldCompleted := testutil.BuildRun(job, nil)
	for i, run := range []*domain.JobRun{runRecentCompleted, runRecentFailed, runOldCompleted} {
		if err := pq.Enqueue(ctx, run); err != nil {
			t.Fatalf("Enqueue(%d) error = %v", i, err)
		}
	}

	recentStartA := now.Add(-8 * time.Minute)
	recentEndA := recentStartA.Add(20 * time.Second)
	recentStartB := now.Add(-6 * time.Minute)
	recentEndB := recentStartB.Add(30 * time.Second)
	oldStart := now.Add(-4 * time.Hour)
	oldEnd := oldStart.Add(40 * time.Second)

	if _, err := testDB.Pool.Exec(ctx, `UPDATE job_runs SET status = 'completed', created_at = $2, started_at = $3, finished_at = $4 WHERE id = $1`, runRecentCompleted.ID, now.Add(-8*time.Minute), recentStartA, recentEndA); err != nil {
		t.Fatalf("update runRecentCompleted error = %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `UPDATE job_runs SET status = 'failed', created_at = $2, started_at = $3, finished_at = $4 WHERE id = $1`, runRecentFailed.ID, now.Add(-6*time.Minute), recentStartB, recentEndB); err != nil {
		t.Fatalf("update runRecentFailed error = %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `UPDATE job_runs SET status = 'completed', created_at = $2, started_at = $3, finished_at = $4 WHERE id = $1`, runOldCompleted.ID, now.Add(-4*time.Hour), oldStart, oldEnd); err != nil {
		t.Fatalf("update runOldCompleted error = %v", err)
	}

	stats, err := q.GetJobHealthStats(ctx, job.ID, now.Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("GetJobHealthStats() error = %v", err)
	}
	if stats.TotalRuns != 2 || stats.CompletedRuns != 1 || stats.FailedRuns != 1 {
		t.Fatalf("GetJobHealthStats() = %+v, want total=2 completed=1 failed=1", *stats)
	}
	if stats.SuccessRate < 49.99 || stats.SuccessRate > 50.01 {
		t.Fatalf("SuccessRate = %f, want 50", stats.SuccessRate)
	}
}

func TestEventTriggerCreateAndGetByEventKey(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-event-trigger-create-" + newID()
	_, run := mustCreateJobRunWithBuildFactory(t, ctx, q, projectID, domain.StatusWaiting)

	now := time.Now().UTC()
	trigger := &domain.EventTrigger{
		ID:             newID(),
		EventKey:       "evt-create-" + newID(),
		ProjectID:      projectID,
		SourceType:     "job_run",
		JobRunID:       run.ID,
		Status:         domain.EventTriggerStatusWaiting,
		RequestPayload: json.RawMessage(`{"kind":"create"}`),
		TimeoutSecs:    120,
		RequestedAt:    now,
		ExpiresAt:      now.Add(2 * time.Minute),
		NotifyURL:      "https://example.com/notify",
		NotifyStatus:   "pending",
		TriggerType:    "event",
	}

	if err := q.CreateEventTrigger(ctx, trigger); err != nil {
		t.Fatalf("CreateEventTrigger() error = %v", err)
	}

	got, err := q.GetEventTriggerByEventKey(ctx, trigger.EventKey)
	if err != nil {
		t.Fatalf("GetEventTriggerByEventKey() error = %v", err)
	}
	if got == nil {
		t.Fatal("GetEventTriggerByEventKey() = nil")
	}
	if got.ID != trigger.ID {
		t.Fatalf("ID = %q, want %q", got.ID, trigger.ID)
	}
	if got.JobRunID != run.ID {
		t.Fatalf("JobRunID = %q, want %q", got.JobRunID, run.ID)
	}
	if !jsonEqual(got.RequestPayload, trigger.RequestPayload) {
		t.Fatalf("RequestPayload = %s, want %s", string(got.RequestPayload), string(trigger.RequestPayload))
	}
}

func TestEventTriggerGetByEventKeyForProjectScopesLookup(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	foreignProjectID := "proj-event-trigger-foreign-" + newID()
	_, foreignRun := mustCreateJobRunWithBuildFactory(t, ctx, q, foreignProjectID, domain.StatusWaiting)

	now := time.Now().UTC()
	foreignTrigger := &domain.EventTrigger{
		ID:             newID(),
		EventKey:       "evt-scoped-" + newID(),
		ProjectID:      foreignProjectID,
		SourceType:     "job_run",
		JobRunID:       foreignRun.ID,
		Status:         domain.EventTriggerStatusWaiting,
		RequestPayload: json.RawMessage(`{"kind":"foreign"}`),
		TimeoutSecs:    120,
		RequestedAt:    now,
		ExpiresAt:      now.Add(2 * time.Minute),
		TriggerType:    "event",
	}
	if err := q.CreateEventTrigger(ctx, foreignTrigger); err != nil {
		t.Fatalf("CreateEventTrigger() error = %v", err)
	}

	got, err := q.GetEventTriggerByEventKeyForProject(ctx, foreignTrigger.EventKey, "proj-event-trigger-local-"+newID())
	if err != nil {
		t.Fatalf("GetEventTriggerByEventKeyForProject(local) error = %v", err)
	}
	if got != nil {
		t.Fatalf("GetEventTriggerByEventKeyForProject(local) = %q, want nil", got.ID)
	}

	got, err = q.GetEventTriggerByEventKeyForProject(ctx, foreignTrigger.EventKey, foreignProjectID)
	if err != nil {
		t.Fatalf("GetEventTriggerByEventKeyForProject(foreign) error = %v", err)
	}
	if got == nil {
		t.Fatal("GetEventTriggerByEventKeyForProject(foreign) = nil")
	}
	if got.ID != foreignTrigger.ID {
		t.Fatalf("ID = %q, want %q", got.ID, foreignTrigger.ID)
	}
}

func TestEventTriggerCreate_AllowsSameEventKeyAcrossProjects(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectA := "proj-event-trigger-same-key-a-" + newID()
	projectB := "proj-event-trigger-same-key-b-" + newID()
	_, runA := mustCreateJobRunWithBuildFactory(t, ctx, q, projectA, domain.StatusWaiting)
	_, runB := mustCreateJobRunWithBuildFactory(t, ctx, q, projectB, domain.StatusWaiting)

	now := time.Now().UTC()
	eventKey := "evt-shared-" + newID()
	triggerA := &domain.EventTrigger{
		ID:             newID(),
		EventKey:       eventKey,
		ProjectID:      projectA,
		SourceType:     "job_run",
		JobRunID:       runA.ID,
		Status:         domain.EventTriggerStatusWaiting,
		RequestPayload: json.RawMessage(`{"project":"a"}`),
		TimeoutSecs:    120,
		RequestedAt:    now,
		ExpiresAt:      now.Add(2 * time.Minute),
		TriggerType:    "event",
	}
	triggerB := &domain.EventTrigger{
		ID:             newID(),
		EventKey:       eventKey,
		ProjectID:      projectB,
		SourceType:     "job_run",
		JobRunID:       runB.ID,
		Status:         domain.EventTriggerStatusWaiting,
		RequestPayload: json.RawMessage(`{"project":"b"}`),
		TimeoutSecs:    120,
		RequestedAt:    now,
		ExpiresAt:      now.Add(2 * time.Minute),
		TriggerType:    "event",
	}

	if err := q.CreateEventTrigger(ctx, triggerA); err != nil {
		t.Fatalf("CreateEventTrigger(projectA) error = %v", err)
	}
	if err := q.CreateEventTrigger(ctx, triggerB); err != nil {
		t.Fatalf("CreateEventTrigger(projectB) error = %v", err)
	}

	gotA, err := q.GetEventTriggerByEventKeyForProject(ctx, eventKey, projectA)
	if err != nil {
		t.Fatalf("GetEventTriggerByEventKeyForProject(projectA) error = %v", err)
	}
	if gotA == nil || gotA.ID != triggerA.ID {
		t.Fatalf("projectA trigger = %#v, want ID %q", gotA, triggerA.ID)
	}

	gotB, err := q.GetEventTriggerByEventKeyForProject(ctx, eventKey, projectB)
	if err != nil {
		t.Fatalf("GetEventTriggerByEventKeyForProject(projectB) error = %v", err)
	}
	if gotB == nil || gotB.ID != triggerB.ID {
		t.Fatalf("projectB trigger = %#v, want ID %q", gotB, triggerB.ID)
	}
}

func TestEventTriggerCreate_RejectsDuplicateEventKeyWithinProject(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-event-trigger-dupe-key-" + newID()
	_, run := mustCreateJobRunWithBuildFactory(t, ctx, q, projectID, domain.StatusWaiting)

	now := time.Now().UTC()
	eventKey := "evt-dupe-" + newID()
	first := &domain.EventTrigger{
		ID:             newID(),
		EventKey:       eventKey,
		ProjectID:      projectID,
		SourceType:     "job_run",
		JobRunID:       run.ID,
		Status:         domain.EventTriggerStatusWaiting,
		RequestPayload: json.RawMessage(`{"attempt":1}`),
		TimeoutSecs:    120,
		RequestedAt:    now,
		ExpiresAt:      now.Add(2 * time.Minute),
		TriggerType:    "event",
	}
	second := *first
	second.ID = newID()
	second.RequestPayload = json.RawMessage(`{"attempt":2}`)

	if err := q.CreateEventTrigger(ctx, first); err != nil {
		t.Fatalf("CreateEventTrigger(first) error = %v", err)
	}

	err := q.CreateEventTrigger(ctx, &second)
	if !errors.Is(err, store.ErrEventKeyConflict) {
		t.Fatalf("CreateEventTrigger(second) error = %v, want ErrEventKeyConflict", err)
	}
	if strings.Contains(err.Error(), eventKey) {
		t.Fatalf("duplicate error leaked event key %q: %v", eventKey, err)
	}
}

func TestEventTriggerGetByStepRunID(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-event-trigger-step-" + newID()
	_, wfRun, stepRun := mustCreateWorkflowStepFixture(t, ctx, q, projectID, domain.StepWaiting)
	now := time.Now().UTC()
	trigger := &domain.EventTrigger{
		ID:                newID(),
		EventKey:          "evt-step-" + newID(),
		ProjectID:         projectID,
		SourceType:        "workflow_step",
		WorkflowRunID:     wfRun.ID,
		WorkflowStepRunID: stepRun.ID,
		Status:            domain.EventTriggerStatusWaiting,
		RequestPayload:    json.RawMessage(`{"kind":"wait-step"}`),
		TimeoutSecs:       90,
		RequestedAt:       now,
		ExpiresAt:         now.Add(3 * time.Minute),
	}

	if err := q.CreateEventTrigger(ctx, trigger); err != nil {
		t.Fatalf("CreateEventTrigger() error = %v", err)
	}

	got, err := q.GetEventTriggerByStepRunID(ctx, stepRun.ID)
	if err != nil {
		t.Fatalf("GetEventTriggerByStepRunID() error = %v", err)
	}
	if got == nil {
		t.Fatal("GetEventTriggerByStepRunID() = nil")
	}
	if got.ID != trigger.ID {
		t.Fatalf("ID = %q, want %q", got.ID, trigger.ID)
	}
}

func TestEventTriggerGetByJobRunID(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-event-trigger-job-" + newID()
	_, run := mustCreateJobRunWithBuildFactory(t, ctx, q, projectID, domain.StatusWaiting)
	now := time.Now().UTC()
	trigger := &domain.EventTrigger{
		ID:          newID(),
		EventKey:    "evt-job-" + newID(),
		ProjectID:   projectID,
		SourceType:  "job_run",
		JobRunID:    run.ID,
		Status:      domain.EventTriggerStatusWaiting,
		TimeoutSecs: 45,
		RequestedAt: now,
		ExpiresAt:   now.Add(5 * time.Minute),
	}

	if err := q.CreateEventTrigger(ctx, trigger); err != nil {
		t.Fatalf("CreateEventTrigger() error = %v", err)
	}

	got, err := q.GetEventTriggerByJobRunID(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetEventTriggerByJobRunID() error = %v", err)
	}
	if got == nil {
		t.Fatal("GetEventTriggerByJobRunID() = nil")
	}
	if got.ID != trigger.ID {
		t.Fatalf("ID = %q, want %q", got.ID, trigger.ID)
	}
}

func TestUpdateEventTriggerStatus(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-event-trigger-update-status-" + newID()
	_, run := mustCreateJobRunWithBuildFactory(t, ctx, q, projectID, domain.StatusWaiting)
	trigger := mustCreateJobRunEventTrigger(t, ctx, q, projectID, run.ID, domain.EventTriggerStatusWaiting, "evt-update-status-"+newID(), time.Now().UTC().Add(-time.Minute), nil, nil)

	response := json.RawMessage(`{"ok":true}`)
	receivedAt := time.Now().UTC().Truncate(time.Microsecond)
	errMsg := "delivery failed once"
	if err := q.UpdateEventTriggerStatus(ctx, trigger.ID, domain.EventTriggerStatusReceived, response, &receivedAt, errMsg); err != nil {
		t.Fatalf("UpdateEventTriggerStatus() error = %v", err)
	}

	got, err := q.GetEventTriggerByEventKey(ctx, trigger.EventKey)
	if err != nil {
		t.Fatalf("GetEventTriggerByEventKey() error = %v", err)
	}
	if got == nil {
		t.Fatal("GetEventTriggerByEventKey() = nil")
	}
	if got.Status != domain.EventTriggerStatusReceived {
		t.Fatalf("Status = %q, want %q", got.Status, domain.EventTriggerStatusReceived)
	}
	if got.ReceivedAt == nil || !got.ReceivedAt.Equal(receivedAt) {
		t.Fatalf("ReceivedAt = %v, want %v", got.ReceivedAt, receivedAt)
	}
	if !jsonEqual(got.ResponsePayload, response) {
		t.Fatalf("ResponsePayload = %s, want %s", string(got.ResponsePayload), string(response))
	}
	if got.Error != errMsg {
		t.Fatalf("Error = %q, want %q", got.Error, errMsg)
	}
}

func TestUpdateEventTriggerStatusFrom_Conflict(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-event-trigger-cas-" + newID()
	_, run := mustCreateJobRunWithBuildFactory(t, ctx, q, projectID, domain.StatusWaiting)

	now := time.Now().UTC()
	receivedAt := now.Add(time.Minute)
	trigger := mustCreateJobRunEventTrigger(t, ctx, q, projectID, run.ID, domain.EventTriggerStatusReceived, "evt-cas-conflict-"+newID(), now, &receivedAt, nil)

	err := q.UpdateEventTriggerStatusFrom(ctx, trigger.ID, domain.EventTriggerStatusWaiting, domain.EventTriggerStatusCanceled, nil, nil, "canceled by stale request")
	if !errors.Is(err, store.ErrEventTriggerConflict) {
		t.Fatalf("UpdateEventTriggerStatusFrom() error = %v, want ErrEventTriggerConflict", err)
	}

	got, err := q.GetEventTriggerByEventKey(ctx, trigger.EventKey)
	if err != nil {
		t.Fatalf("GetEventTriggerByEventKey() error = %v", err)
	}
	if got == nil {
		t.Fatal("trigger disappeared")
	}
	if got.Status != domain.EventTriggerStatusReceived {
		t.Fatalf("Status = %q, want %q", got.Status, domain.EventTriggerStatusReceived)
	}
}

func TestSetEventTriggerSentBy(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-event-trigger-sent-by-" + newID()
	_, run := mustCreateJobRunWithBuildFactory(t, ctx, q, projectID, domain.StatusWaiting)
	trigger := mustCreateJobRunEventTrigger(t, ctx, q, projectID, run.ID, domain.EventTriggerStatusWaiting, "evt-sent-by-"+newID(), time.Now().UTC(), nil, nil)

	if err := q.SetEventTriggerSentBy(ctx, trigger.ID, "api-key-123"); err != nil {
		t.Fatalf("SetEventTriggerSentBy() error = %v", err)
	}

	got, err := q.GetEventTriggerByEventKey(ctx, trigger.EventKey)
	if err != nil {
		t.Fatalf("GetEventTriggerByEventKey() error = %v", err)
	}
	if got == nil {
		t.Fatal("GetEventTriggerByEventKey() = nil")
	}
	if got.SentBy != "api-key-123" {
		t.Fatalf("SentBy = %q, want %q", got.SentBy, "api-key-123")
	}
}

func TestUpdateEventTriggerNotifyStatus(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-event-trigger-notify-status-" + newID()
	_, run := mustCreateJobRunWithBuildFactory(t, ctx, q, projectID, domain.StatusWaiting)
	trigger := mustCreateJobRunEventTrigger(t, ctx, q, projectID, run.ID, domain.EventTriggerStatusWaiting, "evt-notify-status-"+newID(), time.Now().UTC(), nil, nil)

	if err := q.UpdateEventTriggerNotifyStatus(ctx, trigger.ID, "sent"); err != nil {
		t.Fatalf("UpdateEventTriggerNotifyStatus() error = %v", err)
	}

	got, err := q.GetEventTriggerByEventKey(ctx, trigger.EventKey)
	if err != nil {
		t.Fatalf("GetEventTriggerByEventKey() error = %v", err)
	}
	if got == nil {
		t.Fatal("GetEventTriggerByEventKey() = nil")
	}
	if got.NotifyStatus != "sent" {
		t.Fatalf("NotifyStatus = %q, want %q", got.NotifyStatus, "sent")
	}
}

func TestListEventTriggersByProject_Filters(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-event-trigger-list-project-" + newID()
	otherProjectID := "proj-event-trigger-list-project-other-" + newID()

	_, wfRun, stepRun := mustCreateWorkflowStepFixture(t, ctx, q, projectID, domain.StepWaiting)
	_, run := mustCreateJobRunWithBuildFactory(t, ctx, q, projectID, domain.StatusWaiting)
	_, runOther := mustCreateJobRunWithBuildFactory(t, ctx, q, otherProjectID, domain.StatusWaiting)

	now := time.Now().UTC()
	mustCreateWorkflowStepEventTrigger(t, ctx, q, projectID, wfRun.ID, stepRun.ID, domain.EventTriggerStatusWaiting, "evt-list-proj-wf-waiting-"+newID(), now.Add(-30*time.Second), nil)
	mustCreateWorkflowStepEventTrigger(t, ctx, q, projectID, wfRun.ID, stepRun.ID, domain.EventTriggerStatusReceived, "evt-list-proj-wf-received-"+newID(), now.Add(-20*time.Second), new(now.Add(-10*time.Second)))
	mustCreateJobRunEventTrigger(t, ctx, q, projectID, run.ID, domain.EventTriggerStatusWaiting, "evt-list-proj-job-waiting-"+newID(), now.Add(-10*time.Second), nil, nil)
	mustCreateJobRunEventTrigger(t, ctx, q, otherProjectID, runOther.ID, domain.EventTriggerStatusWaiting, "evt-list-proj-other-"+newID(), now.Add(-5*time.Second), nil, nil)

	waiting, err := q.ListEventTriggersByProject(ctx, projectID, "", domain.EventTriggerStatusWaiting, "", "", 20, nil)
	if err != nil {
		t.Fatalf("ListEventTriggersByProject(status) error = %v", err)
	}
	if len(waiting) != 2 {
		t.Fatalf("ListEventTriggersByProject(status) len = %d, want 2", len(waiting))
	}

	byWorkflowRun, err := q.ListEventTriggersByProject(ctx, projectID, "", "", wfRun.ID, "", 20, nil)
	if err != nil {
		t.Fatalf("ListEventTriggersByProject(workflowRunID) error = %v", err)
	}
	if len(byWorkflowRun) != 2 {
		t.Fatalf("ListEventTriggersByProject(workflowRunID) len = %d, want 2", len(byWorkflowRun))
	}

	bySourceType, err := q.ListEventTriggersByProject(ctx, projectID, "", "", "", "job_run", 20, nil)
	if err != nil {
		t.Fatalf("ListEventTriggersByProject(sourceType) error = %v", err)
	}
	if len(bySourceType) != 1 {
		t.Fatalf("ListEventTriggersByProject(sourceType) len = %d, want 1", len(bySourceType))
	}
	if bySourceType[0].SourceType != "job_run" {
		t.Fatalf("SourceType = %q, want %q", bySourceType[0].SourceType, "job_run")
	}
}

func TestListEventTriggersByProject_EnvironmentFilter(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-event-trigger-list-env-" + newID()
	_, run := mustCreateJobRunWithBuildFactory(t, ctx, q, projectID, domain.StatusWaiting)
	now := time.Now().UTC()

	prod := &domain.EventTrigger{
		ID:            newID(),
		EventKey:      "evt-list-env-prod-" + newID(),
		ProjectID:     projectID,
		EnvironmentID: "env-prod",
		SourceType:    "job_run",
		JobRunID:      run.ID,
		Status:        domain.EventTriggerStatusWaiting,
		RequestedAt:   now.Add(-2 * time.Minute),
		ExpiresAt:     now.Add(5 * time.Minute),
	}
	staging := &domain.EventTrigger{
		ID:            newID(),
		EventKey:      "evt-list-env-staging-" + newID(),
		ProjectID:     projectID,
		EnvironmentID: "env-staging",
		SourceType:    "job_run",
		JobRunID:      run.ID,
		Status:        domain.EventTriggerStatusWaiting,
		RequestedAt:   now.Add(-time.Minute),
		ExpiresAt:     now.Add(5 * time.Minute),
	}
	if err := q.CreateEventTrigger(ctx, prod); err != nil {
		t.Fatalf("CreateEventTrigger(prod) error = %v", err)
	}
	if err := q.CreateEventTrigger(ctx, staging); err != nil {
		t.Fatalf("CreateEventTrigger(staging) error = %v", err)
	}

	all, err := q.ListEventTriggersByProject(ctx, projectID, "", domain.EventTriggerStatusWaiting, "", "", 10, nil)
	if err != nil {
		t.Fatalf("ListEventTriggersByProject(all envs) error = %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("ListEventTriggersByProject(all envs) len = %d, want 2", len(all))
	}

	filtered, err := q.ListEventTriggersByProject(ctx, projectID, "env-prod", domain.EventTriggerStatusWaiting, "", "", 10, nil)
	if err != nil {
		t.Fatalf("ListEventTriggersByProject(env-prod) error = %v", err)
	}
	if len(filtered) != 1 {
		t.Fatalf("ListEventTriggersByProject(env-prod) len = %d, want 1", len(filtered))
	}
	if filtered[0].ID != prod.ID {
		t.Fatalf("ListEventTriggersByProject(env-prod) id = %q, want %q", filtered[0].ID, prod.ID)
	}
}

func TestGetEventTriggerStats_EnvironmentFilter(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-event-trigger-stats-env-" + newID()
	_, run := mustCreateJobRunWithBuildFactory(t, ctx, q, projectID, domain.StatusWaiting)
	now := time.Now().UTC()
	receivedAt := now.Add(time.Minute)

	triggers := []*domain.EventTrigger{
		{
			ID:            newID(),
			EventKey:      "evt-stats-env-prod-waiting-" + newID(),
			ProjectID:     projectID,
			EnvironmentID: "env-prod",
			SourceType:    "job_run",
			JobRunID:      run.ID,
			Status:        domain.EventTriggerStatusWaiting,
			RequestedAt:   now,
			ExpiresAt:     now.Add(5 * time.Minute),
		},
		{
			ID:            newID(),
			EventKey:      "evt-stats-env-prod-received-" + newID(),
			ProjectID:     projectID,
			EnvironmentID: "env-prod",
			SourceType:    "job_run",
			JobRunID:      run.ID,
			Status:        domain.EventTriggerStatusReceived,
			RequestedAt:   now,
			ReceivedAt:    &receivedAt,
			ExpiresAt:     now.Add(5 * time.Minute),
		},
		{
			ID:            newID(),
			EventKey:      "evt-stats-env-staging-waiting-" + newID(),
			ProjectID:     projectID,
			EnvironmentID: "env-staging",
			SourceType:    "job_run",
			JobRunID:      run.ID,
			Status:        domain.EventTriggerStatusWaiting,
			RequestedAt:   now,
			ExpiresAt:     now.Add(5 * time.Minute),
		},
	}
	for _, trigger := range triggers {
		if err := q.CreateEventTrigger(ctx, trigger); err != nil {
			t.Fatalf("CreateEventTrigger(%s) error = %v", trigger.EventKey, err)
		}
	}

	all, err := q.GetEventTriggerStats(ctx, projectID, "")
	if err != nil {
		t.Fatalf("GetEventTriggerStats(all envs) error = %v", err)
	}
	if all.TotalCount != 3 || all.WaitingCount != 2 || all.ReceivedCount != 1 {
		t.Fatalf("GetEventTriggerStats(all envs) = total %d waiting %d received %d, want 3/2/1", all.TotalCount, all.WaitingCount, all.ReceivedCount)
	}

	prod, err := q.GetEventTriggerStats(ctx, projectID, "env-prod")
	if err != nil {
		t.Fatalf("GetEventTriggerStats(env-prod) error = %v", err)
	}
	if prod.TotalCount != 2 || prod.WaitingCount != 1 || prod.ReceivedCount != 1 {
		t.Fatalf("GetEventTriggerStats(env-prod) = total %d waiting %d received %d, want 2/1/1", prod.TotalCount, prod.WaitingCount, prod.ReceivedCount)
	}
}

func TestListEventTriggersByKeyPrefix_ProjectScoping(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-event-trigger-list-prefix-" + newID()
	otherProjectID := "proj-event-trigger-list-prefix-other-" + newID()
	_, runA := mustCreateJobRunWithBuildFactory(t, ctx, q, projectID, domain.StatusWaiting)
	_, runB := mustCreateJobRunWithBuildFactory(t, ctx, q, otherProjectID, domain.StatusWaiting)

	prefix := "batch.prefix."
	now := time.Now().UTC()
	matchA := mustCreateJobRunEventTrigger(t, ctx, q, projectID, runA.ID, domain.EventTriggerStatusWaiting, prefix+"one", now.Add(-3*time.Second), nil, nil)
	mustCreateJobRunEventTrigger(t, ctx, q, projectID, runA.ID, domain.EventTriggerStatusReceived, prefix+"received", now.Add(-2*time.Second), new(now.Add(-time.Second)), nil)
	mustCreateJobRunEventTrigger(t, ctx, q, projectID, runA.ID, domain.EventTriggerStatusWaiting, "other.prefix."+newID(), now.Add(-time.Second), nil, nil)
	matchOtherProject := mustCreateJobRunEventTrigger(t, ctx, q, otherProjectID, runB.ID, domain.EventTriggerStatusWaiting, prefix+"two", now, nil, nil)

	global, err := q.ListEventTriggersByKeyPrefix(ctx, prefix, "")
	if err != nil {
		t.Fatalf("ListEventTriggersByKeyPrefix(global) error = %v", err)
	}
	if len(global) != 2 {
		t.Fatalf("ListEventTriggersByKeyPrefix(global) len = %d, want 2", len(global))
	}

	scoped, err := q.ListEventTriggersByKeyPrefix(ctx, prefix, projectID)
	if err != nil {
		t.Fatalf("ListEventTriggersByKeyPrefix(scoped) error = %v", err)
	}
	if len(scoped) != 1 {
		t.Fatalf("ListEventTriggersByKeyPrefix(scoped) len = %d, want 1", len(scoped))
	}
	if scoped[0].ID != matchA.ID {
		t.Fatalf("ListEventTriggersByKeyPrefix(scoped) id = %q, want %q", scoped[0].ID, matchA.ID)
	}

	if global[0].ID != matchA.ID && global[1].ID != matchA.ID {
		t.Fatalf("ListEventTriggersByKeyPrefix(global) missing id %q", matchA.ID)
	}
	if global[0].ID != matchOtherProject.ID && global[1].ID != matchOtherProject.ID {
		t.Fatalf("ListEventTriggersByKeyPrefix(global) missing id %q", matchOtherProject.ID)
	}
}

func TestListEventTriggersByProject_Cursor(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-event-trigger-list-cursor-" + newID()
	_, run := mustCreateJobRunWithBuildFactory(t, ctx, q, projectID, domain.StatusWaiting)
	now := time.Now().UTC()
	mustCreateJobRunEventTrigger(t, ctx, q, projectID, run.ID, domain.EventTriggerStatusWaiting, "evt-list-cursor-new-"+newID(), now.Add(-time.Minute), nil, nil)
	middle := mustCreateJobRunEventTrigger(t, ctx, q, projectID, run.ID, domain.EventTriggerStatusWaiting, "evt-list-cursor-middle-"+newID(), now.Add(-2*time.Minute), nil, nil)
	old := mustCreateJobRunEventTrigger(t, ctx, q, projectID, run.ID, domain.EventTriggerStatusWaiting, "evt-list-cursor-old-"+newID(), now.Add(-3*time.Minute), nil, nil)

	list, err := q.ListEventTriggersByProject(ctx, projectID, "", "", "", "", 10, new(middle.RequestedAt))
	if err != nil {
		t.Fatalf("ListEventTriggersByProject(cursor) error = %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("ListEventTriggersByProject(cursor) len = %d, want 1", len(list))
	}
	if list[0].ID != old.ID {
		t.Fatalf("ListEventTriggersByProject(cursor) id = %q, want %q", list[0].ID, old.ID)
	}
}

func TestListEventTriggersExpired(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-event-trigger-list-expired-" + newID()
	_, run := mustCreateJobRunWithBuildFactory(t, ctx, q, projectID, domain.StatusWaiting)
	now := time.Now().UTC()
	expired := mustCreateJobRunEventTrigger(t, ctx, q, projectID, run.ID, domain.EventTriggerStatusWaiting, "evt-expired-old-"+newID(), now.Add(-2*time.Hour), nil, new(now.Add(-time.Minute)))
	mustCreateJobRunEventTrigger(t, ctx, q, projectID, run.ID, domain.EventTriggerStatusWaiting, "evt-expired-future-"+newID(), now.Add(-2*time.Hour), nil, new(now.Add(2*time.Hour)))
	mustCreateJobRunEventTrigger(t, ctx, q, projectID, run.ID, domain.EventTriggerStatusReceived, "evt-expired-received-"+newID(), now.Add(-2*time.Hour), new(now.Add(-time.Minute)), new(now.Add(-time.Minute)))

	list, err := q.ListExpiredEventTriggers(ctx)
	if err != nil {
		t.Fatalf("ListExpiredEventTriggers() error = %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("ListExpiredEventTriggers() len = %d, want 1", len(list))
	}
	if list[0].ID != expired.ID {
		t.Fatalf("ListExpiredEventTriggers() id = %q, want %q", list[0].ID, expired.ID)
	}
}

func TestListEventTriggersReceivedWithStaleSteps(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-event-trigger-list-stale-" + newID()
	_, wfRun, stepRun := mustCreateWorkflowStepFixture(t, ctx, q, projectID, domain.StepWaiting)
	_, jobRun := mustCreateJobRunWithBuildFactory(t, ctx, q, projectID, domain.StatusWaiting)

	oldReceived := time.Now().UTC().Add(-time.Minute)
	staleStep := mustCreateWorkflowStepEventTrigger(t, ctx, q, projectID, wfRun.ID, stepRun.ID, domain.EventTriggerStatusReceived, "evt-stale-step-"+newID(), oldReceived.Add(-time.Second), &oldReceived)
	staleJob := mustCreateJobRunEventTrigger(t, ctx, q, projectID, jobRun.ID, domain.EventTriggerStatusReceived, "evt-stale-job-"+newID(), oldReceived.Add(-time.Second), &oldReceived, nil)
	recentReceived := time.Now().UTC().Add(-10 * time.Second)
	mustCreateJobRunEventTrigger(t, ctx, q, projectID, jobRun.ID, domain.EventTriggerStatusReceived, "evt-not-stale-"+newID(), recentReceived.Add(-time.Second), &recentReceived, nil)

	list, err := q.ListReceivedEventTriggersWithStaleSteps(ctx)
	if err != nil {
		t.Fatalf("ListReceivedEventTriggersWithStaleSteps() error = %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("ListReceivedEventTriggersWithStaleSteps() len = %d, want 2", len(list))
	}

	seenStep := false
	seenJob := false
	for _, trigger := range list {
		if trigger.ID == staleStep.ID {
			seenStep = true
		}
		if trigger.ID == staleJob.ID {
			seenJob = true
		}
	}
	if !seenStep || !seenJob {
		t.Fatalf("stale IDs not found: step=%v job=%v", seenStep, seenJob)
	}
}

func TestCancelEventTriggersByWorkflowRun_WaitingOnly(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-event-trigger-cancel-wf-" + newID()
	_, wfRun, stepRun := mustCreateWorkflowStepFixture(t, ctx, q, projectID, domain.StepWaiting)
	waiting := mustCreateWorkflowStepEventTrigger(t, ctx, q, projectID, wfRun.ID, stepRun.ID, domain.EventTriggerStatusWaiting, "evt-cancel-wf-waiting-"+newID(), time.Now().UTC().Add(-time.Minute), nil)
	receivedAt := time.Now().UTC().Add(-time.Minute)
	received := mustCreateWorkflowStepEventTrigger(t, ctx, q, projectID, wfRun.ID, stepRun.ID, domain.EventTriggerStatusReceived, "evt-cancel-wf-received-"+newID(), time.Now().UTC().Add(-time.Minute), &receivedAt)

	affected, err := q.CancelEventTriggersByWorkflowRun(ctx, wfRun.ID)
	if err != nil {
		t.Fatalf("CancelEventTriggersByWorkflowRun() error = %v", err)
	}
	if affected != 1 {
		t.Fatalf("CancelEventTriggersByWorkflowRun() affected = %d, want 1", affected)
	}

	gotWaiting, err := q.GetEventTriggerByEventKey(ctx, waiting.EventKey)
	if err != nil {
		t.Fatalf("GetEventTriggerByEventKey(waiting) error = %v", err)
	}
	if gotWaiting == nil {
		t.Fatal("GetEventTriggerByEventKey(waiting) = nil")
	}
	if gotWaiting.Status != domain.EventTriggerStatusCanceled {
		t.Fatalf("waiting.Status = %q, want %q", gotWaiting.Status, domain.EventTriggerStatusCanceled)
	}

	gotReceived, err := q.GetEventTriggerByEventKey(ctx, received.EventKey)
	if err != nil {
		t.Fatalf("GetEventTriggerByEventKey(received) error = %v", err)
	}
	if gotReceived == nil {
		t.Fatal("GetEventTriggerByEventKey(received) = nil")
	}
	if gotReceived.Status != domain.EventTriggerStatusReceived {
		t.Fatalf("received.Status = %q, want %q", gotReceived.Status, domain.EventTriggerStatusReceived)
	}
}

func TestCancelEventTriggerByJobRun_WaitingOnly(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-event-trigger-cancel-job-" + newID()
	_, run := mustCreateJobRunWithBuildFactory(t, ctx, q, projectID, domain.StatusWaiting)
	waiting := mustCreateJobRunEventTrigger(t, ctx, q, projectID, run.ID, domain.EventTriggerStatusWaiting, "evt-cancel-job-waiting-"+newID(), time.Now().UTC(), nil, nil)
	receivedAt := time.Now().UTC().Add(-time.Minute)
	received := mustCreateJobRunEventTrigger(t, ctx, q, projectID, run.ID, domain.EventTriggerStatusReceived, "evt-cancel-job-received-"+newID(), time.Now().UTC(), &receivedAt, nil)

	if err := q.CancelEventTriggerByJobRun(ctx, run.ID); err != nil {
		t.Fatalf("CancelEventTriggerByJobRun() error = %v", err)
	}

	gotWaiting, err := q.GetEventTriggerByEventKey(ctx, waiting.EventKey)
	if err != nil {
		t.Fatalf("GetEventTriggerByEventKey(waiting) error = %v", err)
	}
	if gotWaiting == nil {
		t.Fatal("GetEventTriggerByEventKey(waiting) = nil")
	}
	if gotWaiting.Status != domain.EventTriggerStatusCanceled {
		t.Fatalf("waiting.Status = %q, want %q", gotWaiting.Status, domain.EventTriggerStatusCanceled)
	}

	gotReceived, err := q.GetEventTriggerByEventKey(ctx, received.EventKey)
	if err != nil {
		t.Fatalf("GetEventTriggerByEventKey(received) error = %v", err)
	}
	if gotReceived == nil {
		t.Fatal("GetEventTriggerByEventKey(received) = nil")
	}
	if gotReceived.Status != domain.EventTriggerStatusReceived {
		t.Fatalf("received.Status = %q, want %q", gotReceived.Status, domain.EventTriggerStatusReceived)
	}
}

func TestCountEventTriggersFinishedBefore(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-event-trigger-count-finished-" + newID()
	_, run := mustCreateJobRunWithBuildFactory(t, ctx, q, projectID, domain.StatusWaiting)

	now := time.Now().UTC()
	oldReceived := now.Add(-3 * time.Hour)
	mustCreateJobRunEventTrigger(t, ctx, q, projectID, run.ID, domain.EventTriggerStatusReceived, "evt-count-finished-received-"+newID(), now.Add(-4*time.Hour), &oldReceived, nil)
	mustCreateJobRunEventTrigger(t, ctx, q, projectID, run.ID, domain.EventTriggerStatusTimedOut, "evt-count-finished-timeout-"+newID(), now.Add(-4*time.Hour), nil, new(now.Add(-2*time.Hour)))
	mustCreateJobRunEventTrigger(t, ctx, q, projectID, run.ID, domain.EventTriggerStatusCanceled, "evt-count-finished-canceled-"+newID(), now.Add(-4*time.Hour), nil, new(now.Add(-2*time.Hour)))
	mustCreateJobRunEventTrigger(t, ctx, q, projectID, run.ID, domain.EventTriggerStatusWaiting, "evt-count-finished-waiting-"+newID(), now.Add(-4*time.Hour), nil, new(now.Add(-2*time.Hour)))
	mustCreateJobRunEventTrigger(t, ctx, q, projectID, run.ID, domain.EventTriggerStatusReceived, "evt-count-finished-recent-"+newID(), now.Add(-time.Hour), new(now.Add(-time.Minute)), nil)

	count, err := q.CountEventTriggersFinishedBefore(ctx, now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("CountEventTriggersFinishedBefore() error = %v", err)
	}
	if count != 3 {
		t.Fatalf("CountEventTriggersFinishedBefore() = %d, want 3", count)
	}
}

func TestEventTriggersFinishedBeforeForProject_EnvironmentFilter(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-event-trigger-finished-env-" + newID()
	otherProjectID := "proj-event-trigger-finished-env-other-" + newID()
	_, run := mustCreateJobRunWithBuildFactory(t, ctx, q, projectID, domain.StatusWaiting)
	_, otherRun := mustCreateJobRunWithBuildFactory(t, ctx, q, otherProjectID, domain.StatusWaiting)

	now := time.Now().UTC()
	oldReceived := now.Add(-3 * time.Hour)
	prod := &domain.EventTrigger{
		ID:              newID(),
		EventKey:        "evt-finished-env-prod-" + newID(),
		ProjectID:       projectID,
		EnvironmentID:   "env-prod",
		SourceType:      "job_run",
		JobRunID:        run.ID,
		Status:          domain.EventTriggerStatusReceived,
		RequestedAt:     now.Add(-4 * time.Hour),
		ReceivedAt:      &oldReceived,
		ExpiresAt:       now.Add(time.Hour),
		ResponsePayload: json.RawMessage(`{"ok":true}`),
	}
	staging := &domain.EventTrigger{
		ID:            newID(),
		EventKey:      "evt-finished-env-staging-" + newID(),
		ProjectID:     projectID,
		EnvironmentID: "env-staging",
		SourceType:    "job_run",
		JobRunID:      run.ID,
		Status:        domain.EventTriggerStatusTimedOut,
		RequestedAt:   now.Add(-4 * time.Hour),
		ExpiresAt:     now.Add(-2 * time.Hour),
	}
	otherProject := &domain.EventTrigger{
		ID:            newID(),
		EventKey:      "evt-finished-env-other-project-" + newID(),
		ProjectID:     otherProjectID,
		EnvironmentID: "env-prod",
		SourceType:    "job_run",
		JobRunID:      otherRun.ID,
		Status:        domain.EventTriggerStatusReceived,
		RequestedAt:   now.Add(-4 * time.Hour),
		ReceivedAt:    &oldReceived,
		ExpiresAt:     now.Add(time.Hour),
	}
	for _, trigger := range []*domain.EventTrigger{prod, staging, otherProject} {
		if err := q.CreateEventTrigger(ctx, trigger); err != nil {
			t.Fatalf("CreateEventTrigger(%s) error = %v", trigger.EventKey, err)
		}
	}

	allProject, err := q.CountEventTriggersFinishedBeforeForProject(ctx, projectID, "", now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("CountEventTriggersFinishedBeforeForProject(all envs) error = %v", err)
	}
	if allProject != 2 {
		t.Fatalf("CountEventTriggersFinishedBeforeForProject(all envs) = %d, want 2", allProject)
	}

	prodOnly, err := q.CountEventTriggersFinishedBeforeForProject(ctx, projectID, "env-prod", now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("CountEventTriggersFinishedBeforeForProject(env-prod) error = %v", err)
	}
	if prodOnly != 1 {
		t.Fatalf("CountEventTriggersFinishedBeforeForProject(env-prod) = %d, want 1", prodOnly)
	}

	deleted, err := q.DeleteEventTriggersFinishedBeforeForProject(ctx, projectID, "env-prod", now.Add(-time.Hour), 10)
	if err != nil {
		t.Fatalf("DeleteEventTriggersFinishedBeforeForProject(env-prod) error = %v", err)
	}
	if deleted != 1 {
		t.Fatalf("DeleteEventTriggersFinishedBeforeForProject(env-prod) = %d, want 1", deleted)
	}
	if got, err := q.GetEventTriggerByEventKey(ctx, prod.EventKey); err != nil {
		t.Fatalf("GetEventTriggerByEventKey(prod) error = %v", err)
	} else if got != nil {
		t.Fatalf("prod trigger still exists with ID %q", got.ID)
	}
	if got, err := q.GetEventTriggerByEventKey(ctx, staging.EventKey); err != nil {
		t.Fatalf("GetEventTriggerByEventKey(staging) error = %v", err)
	} else if got == nil {
		t.Fatal("staging trigger was deleted by env-prod purge")
	}
	if got, err := q.GetEventTriggerByEventKey(ctx, otherProject.EventKey); err != nil {
		t.Fatalf("GetEventTriggerByEventKey(otherProject) error = %v", err)
	} else if got == nil {
		t.Fatal("other project trigger was deleted by env-prod purge")
	}
}

func TestDeleteEventTriggersFinishedBefore(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-event-trigger-delete-finished-" + newID()
	_, run := mustCreateJobRunWithBuildFactory(t, ctx, q, projectID, domain.StatusWaiting)

	now := time.Now().UTC()
	oldReceived := now.Add(-3 * time.Hour)
	oldA := mustCreateJobRunEventTrigger(t, ctx, q, projectID, run.ID, domain.EventTriggerStatusReceived, "evt-delete-finished-a-"+newID(), now.Add(-4*time.Hour), &oldReceived, nil)
	oldB := mustCreateJobRunEventTrigger(t, ctx, q, projectID, run.ID, domain.EventTriggerStatusTimedOut, "evt-delete-finished-b-"+newID(), now.Add(-4*time.Hour), nil, new(now.Add(-2*time.Hour)))
	recent := mustCreateJobRunEventTrigger(t, ctx, q, projectID, run.ID, domain.EventTriggerStatusCanceled, "evt-delete-finished-recent-"+newID(), now.Add(-time.Hour), nil, new(now.Add(-time.Minute)))
	waiting := mustCreateJobRunEventTrigger(t, ctx, q, projectID, run.ID, domain.EventTriggerStatusWaiting, "evt-delete-finished-waiting-"+newID(), now.Add(-4*time.Hour), nil, new(now.Add(-2*time.Hour)))

	deleted, err := q.DeleteEventTriggersFinishedBefore(ctx, now.Add(-time.Hour), 1)
	if err != nil {
		t.Fatalf("DeleteEventTriggersFinishedBefore() first error = %v", err)
	}
	if deleted != 1 {
		t.Fatalf("DeleteEventTriggersFinishedBefore() first = %d, want 1", deleted)
	}

	remainingAfterFirst, err := q.CountEventTriggersFinishedBefore(ctx, now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("CountEventTriggersFinishedBefore() after first delete error = %v", err)
	}
	if remainingAfterFirst != 1 {
		t.Fatalf("remaining after first delete = %d, want 1", remainingAfterFirst)
	}

	deleted, err = q.DeleteEventTriggersFinishedBefore(ctx, now.Add(-time.Hour), 10)
	if err != nil {
		t.Fatalf("DeleteEventTriggersFinishedBefore() second error = %v", err)
	}
	if deleted != 1 {
		t.Fatalf("DeleteEventTriggersFinishedBefore() second = %d, want 1", deleted)
	}

	if got, err := q.GetEventTriggerByEventKey(ctx, oldA.EventKey); err != nil {
		t.Fatalf("GetEventTriggerByEventKey(oldA) error = %v", err)
	} else if got != nil {
		t.Fatalf("oldA still exists with ID %q", got.ID)
	}
	if got, err := q.GetEventTriggerByEventKey(ctx, oldB.EventKey); err != nil {
		t.Fatalf("GetEventTriggerByEventKey(oldB) error = %v", err)
	} else if got != nil {
		t.Fatalf("oldB still exists with ID %q", got.ID)
	}
	if got, err := q.GetEventTriggerByEventKey(ctx, recent.EventKey); err != nil {
		t.Fatalf("GetEventTriggerByEventKey(recent) error = %v", err)
	} else if got == nil {
		t.Fatal("recent trigger should exist")
	}
	if got, err := q.GetEventTriggerByEventKey(ctx, waiting.EventKey); err != nil {
		t.Fatalf("GetEventTriggerByEventKey(waiting) error = %v", err)
	} else if got == nil {
		t.Fatal("waiting trigger should exist")
	}
}

func TestBatchReceiveEventTriggers(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-event-trigger-batch-receive-" + newID()
	_, run := mustCreateJobRunWithBuildFactory(t, ctx, q, projectID, domain.StatusWaiting)
	now := time.Now().UTC()
	triggerA := mustCreateJobRunEventTrigger(t, ctx, q, projectID, run.ID, domain.EventTriggerStatusWaiting, "evt-batch-a-"+newID(), now.Add(-2*time.Minute), nil, nil)
	triggerB := mustCreateJobRunEventTrigger(t, ctx, q, projectID, run.ID, domain.EventTriggerStatusWaiting, "evt-batch-b-"+newID(), now.Add(-time.Minute), nil, nil)

	payload := json.RawMessage(`{"batched":true}`)
	receivedAt := time.Now().UTC().Truncate(time.Microsecond)
	updatedIDs, err := q.BatchReceiveEventTriggers(ctx, []string{triggerA.ID, triggerB.ID}, payload, receivedAt, "batch-sender")
	if err != nil {
		t.Fatalf("BatchReceiveEventTriggers() error = %v", err)
	}
	if len(updatedIDs) != 2 {
		t.Fatalf("BatchReceiveEventTriggers() len = %d, want 2", len(updatedIDs))
	}

	gotA, err := q.GetEventTriggerByEventKey(ctx, triggerA.EventKey)
	if err != nil {
		t.Fatalf("GetEventTriggerByEventKey(triggerA) error = %v", err)
	}
	if gotA == nil {
		t.Fatal("GetEventTriggerByEventKey(triggerA) = nil")
	}
	if gotA.Status != domain.EventTriggerStatusReceived {
		t.Fatalf("triggerA status = %q, want %q", gotA.Status, domain.EventTriggerStatusReceived)
	}
	if gotA.SentBy != "batch-sender" {
		t.Fatalf("triggerA sent_by = %q, want %q", gotA.SentBy, "batch-sender")
	}
	if !jsonEqual(gotA.ResponsePayload, payload) {
		t.Fatalf("triggerA response payload = %s, want %s", string(gotA.ResponsePayload), string(payload))
	}

	gotB, err := q.GetEventTriggerByEventKey(ctx, triggerB.EventKey)
	if err != nil {
		t.Fatalf("GetEventTriggerByEventKey(triggerB) error = %v", err)
	}
	if gotB == nil {
		t.Fatal("GetEventTriggerByEventKey(triggerB) = nil")
	}
	if gotB.Status != domain.EventTriggerStatusReceived {
		t.Fatalf("triggerB status = %q, want %q", gotB.Status, domain.EventTriggerStatusReceived)
	}
	if gotB.ReceivedAt == nil || !gotB.ReceivedAt.Equal(receivedAt) {
		t.Fatalf("triggerB received_at = %v, want %v", gotB.ReceivedAt, receivedAt)
	}
}

func TestReceiveEventAndRequeueRun(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-event-trigger-requeue-run-" + newID()
	runStatus := domain.StatusWaiting
	job := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: new(projectID)})
	run := testutil.MustCreateRun(t, ctx, q, job, &testutil.RunOpts{Status: new(runStatus)})
	trigger := mustCreateJobRunEventTrigger(t, ctx, q, projectID, run.ID, domain.EventTriggerStatusWaiting, "evt-requeue-"+newID(), time.Now().UTC().Add(-time.Minute), nil, nil)

	var beforeGeneration int64
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT ready_generation
		FROM job_run_state
		WHERE run_id = $1`,
		run.ID,
	).Scan(&beforeGeneration); err != nil {
		t.Fatalf("query ready_generation before receive: %v", err)
	}

	payload := json.RawMessage(`{"checkpoint":"resume"}`)
	receivedAt := time.Now().UTC()
	if err := q.ReceiveEventAndRequeueRun(ctx, trigger.ID, payload, receivedAt, run.ID); err != nil {
		t.Fatalf("ReceiveEventAndRequeueRun() error = %v", err)
	}

	updatedRun, err := q.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if updatedRun.Status != domain.StatusQueued {
		t.Fatalf("run status = %q, want %q", updatedRun.Status, domain.StatusQueued)
	}
	var ledgerStatus, stateStatus domain.RunStatus
	var afterGeneration int64
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT jr.status, s.status, s.ready_generation
		FROM job_runs jr
		JOIN job_run_state s ON s.run_id = jr.id
		WHERE jr.id = $1`,
		run.ID,
	).Scan(&ledgerStatus, &stateStatus, &afterGeneration); err != nil {
		t.Fatalf("query split run state after receive: %v", err)
	}
	if ledgerStatus != domain.StatusWaiting {
		t.Fatalf("job_runs status = %q, want immutable waiting ledger status", ledgerStatus)
	}
	if stateStatus != domain.StatusQueued {
		t.Fatalf("job_run_state status = %q, want queued", stateStatus)
	}
	if afterGeneration != beforeGeneration+1 {
		t.Fatalf("ready_generation = %d, want %d", afterGeneration, beforeGeneration+1)
	}
	checkpoint, err := q.GetLatestCheckpoint(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetLatestCheckpoint() error = %v", err)
	}
	if checkpoint == nil {
		t.Fatal("GetLatestCheckpoint() = nil")
	}
	if checkpoint.Source != "event_trigger" {
		t.Fatalf("checkpoint source = %q, want event_trigger", checkpoint.Source)
	}
	if !jsonEqual(checkpoint.State, payload) {
		t.Fatalf("checkpoint state = %s, want %s", string(checkpoint.State), string(payload))
	}

	updatedTrigger, err := q.GetEventTriggerByEventKey(ctx, trigger.EventKey)
	if err != nil {
		t.Fatalf("GetEventTriggerByEventKey() error = %v", err)
	}
	if updatedTrigger == nil {
		t.Fatal("GetEventTriggerByEventKey() = nil")
	}
	if updatedTrigger.Status != domain.EventTriggerStatusReceived {
		t.Fatalf("trigger status = %q, want %q", updatedTrigger.Status, domain.EventTriggerStatusReceived)
	}
	if !jsonEqual(updatedTrigger.ResponsePayload, payload) {
		t.Fatalf("trigger response payload = %s, want %s", string(updatedTrigger.ResponsePayload), string(payload))
	}
}

func TestCountActiveEventTriggersByProject(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-event-trigger-count-active-" + newID()
	otherProjectID := "proj-event-trigger-count-active-other-" + newID()
	_, runA := mustCreateJobRunWithBuildFactory(t, ctx, q, projectID, domain.StatusWaiting)
	_, runB := mustCreateJobRunWithBuildFactory(t, ctx, q, otherProjectID, domain.StatusWaiting)
	now := time.Now().UTC()

	mustCreateJobRunEventTrigger(t, ctx, q, projectID, runA.ID, domain.EventTriggerStatusWaiting, "evt-count-active-a-"+newID(), now, nil, nil)
	mustCreateJobRunEventTrigger(t, ctx, q, projectID, runA.ID, domain.EventTriggerStatusWaiting, "evt-count-active-b-"+newID(), now.Add(time.Second), nil, nil)
	mustCreateJobRunEventTrigger(t, ctx, q, projectID, runA.ID, domain.EventTriggerStatusReceived, "evt-count-active-received-"+newID(), now.Add(2*time.Second), new(now.Add(3*time.Second)), nil)
	mustCreateJobRunEventTrigger(t, ctx, q, otherProjectID, runB.ID, domain.EventTriggerStatusWaiting, "evt-count-active-other-"+newID(), now.Add(4*time.Second), nil, nil)

	count, err := q.CountActiveEventTriggersByProject(ctx, projectID)
	if err != nil {
		t.Fatalf("CountActiveEventTriggersByProject() error = %v", err)
	}
	if count != 2 {
		t.Fatalf("CountActiveEventTriggersByProject() = %d, want 2", count)
	}
}

func TestAdvisoryLockTryAndRelease(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	lockID := int64(12345)
	acquired, err := q.TryAdvisoryLock(ctx, lockID)
	if err != nil {
		t.Fatalf("TryAdvisoryLock() error = %v", err)
	}
	if !acquired {
		t.Fatal("TryAdvisoryLock() = false, want true")
	}

	if err := q.ReleaseAdvisoryLock(ctx, lockID); err != nil {
		t.Fatalf("ReleaseAdvisoryLock() error = %v", err)
	}
}

func TestAdvisoryLockConcurrentAcrossConnections(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	lockID := int64(23456)
	connA, err := testDB.Pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("Acquire(connA) error = %v", err)
	}
	defer connA.Release()

	connB, err := testDB.Pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("Acquire(connB) error = %v", err)
	}
	defer connB.Release()

	qa := store.New(connA)
	qb := store.New(connB)

	acquiredA, err := qa.TryAdvisoryLock(ctx, lockID)
	if err != nil {
		t.Fatalf("TryAdvisoryLock(connA) error = %v", err)
	}
	if !acquiredA {
		t.Fatal("TryAdvisoryLock(connA) = false, want true")
	}

	acquiredB, err := qb.TryAdvisoryLock(ctx, lockID)
	if err != nil {
		t.Fatalf("TryAdvisoryLock(connB) error = %v", err)
	}
	if acquiredB {
		t.Fatal("TryAdvisoryLock(connB) = true, want false")
	}

	if err := qa.ReleaseAdvisoryLock(ctx, lockID); err != nil {
		t.Fatalf("ReleaseAdvisoryLock(connA) error = %v", err)
	}

	acquiredBAfterRelease, err := qb.TryAdvisoryLock(ctx, lockID)
	if err != nil {
		t.Fatalf("TryAdvisoryLock(connB after release) error = %v", err)
	}
	if !acquiredBAfterRelease {
		t.Fatal("TryAdvisoryLock(connB after release) = false, want true")
	}

	if err := qb.ReleaseAdvisoryLock(ctx, lockID); err != nil {
		t.Fatalf("ReleaseAdvisoryLock(connB) error = %v", err)
	}

	if err := q.ReleaseAdvisoryLock(ctx, lockID); err != nil {
		t.Fatalf("ReleaseAdvisoryLock(cleanup) error = %v", err)
	}
}

func TestRunWithAdvisoryLockPinsAndReleasesConnection(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	lockID := int64(334455)
	fnStarted := make(chan struct{})
	releaseFn := make(chan struct{})
	fnDone := make(chan error, 1)
	concWG.Go(func() {
		_, err := q.RunWithAdvisoryLock(ctx, lockID, func(context.Context) error {
			close(fnStarted)
			<-releaseFn
			return nil
		})
		fnDone <- err
	})

	select {
	case <-fnStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("RunWithAdvisoryLock did not start fn")
	}

	acquiredWhileHeld, err := q.TryAdvisoryLock(ctx, lockID)
	if err != nil {
		t.Fatalf("TryAdvisoryLock(while held) error = %v", err)
	}
	if acquiredWhileHeld {
		t.Fatal("TryAdvisoryLock(while held) = true, want false")
	}

	close(releaseFn)
	select {
	case err := <-fnDone:
		if err != nil {
			t.Fatalf("RunWithAdvisoryLock() error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("RunWithAdvisoryLock did not finish")
	}

	acquiredAfterRelease, err := q.TryAdvisoryLock(ctx, lockID)
	if err != nil {
		t.Fatalf("TryAdvisoryLock(after release) error = %v", err)
	}
	if !acquiredAfterRelease {
		t.Fatal("TryAdvisoryLock(after release) = false, want true")
	}
	if err := q.ReleaseAdvisoryLock(ctx, lockID); err != nil {
		t.Fatalf("ReleaseAdvisoryLock(after release) error = %v", err)
	}
}

func TestRunWithAdvisoryLockReportsNotAcquired(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	lockID := int64(334456)
	conn, err := testDB.Pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	defer conn.Release()

	heldByConn := store.New(conn)
	acquired, err := heldByConn.TryAdvisoryLock(ctx, lockID)
	if err != nil {
		t.Fatalf("TryAdvisoryLock() error = %v", err)
	}
	if !acquired {
		t.Fatal("TryAdvisoryLock() = false, want true")
	}
	defer func() {
		if err := heldByConn.ReleaseAdvisoryLock(ctx, lockID); err != nil {
			t.Fatalf("ReleaseAdvisoryLock() error = %v", err)
		}
	}()

	ran := false
	acquired, err = q.RunWithAdvisoryLock(ctx, lockID, func(context.Context) error {
		ran = true
		return nil
	})
	if err != nil {
		t.Fatalf("RunWithAdvisoryLock() error = %v", err)
	}
	if acquired {
		t.Fatal("RunWithAdvisoryLock() acquired = true, want false")
	}
	if ran {
		t.Fatal("RunWithAdvisoryLock ran fn when lock was held")
	}
}

func TestRunWithAdvisoryLockReleasesAfterPanic(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	lockID := int64(334457)
	func() {
		defer func() {
			if rec := recover(); rec == nil {
				t.Fatal("expected panic to propagate")
			}
		}()
		_, _ = q.RunWithAdvisoryLock(ctx, lockID, func(context.Context) error {
			panic("locked section failed")
		})
	}()

	acquiredAfterPanic, err := q.TryAdvisoryLock(ctx, lockID)
	if err != nil {
		t.Fatalf("TryAdvisoryLock(after panic) error = %v", err)
	}
	if !acquiredAfterPanic {
		t.Fatal("TryAdvisoryLock(after panic) = false, want true")
	}
	if err := q.ReleaseAdvisoryLock(ctx, lockID); err != nil {
		t.Fatalf("ReleaseAdvisoryLock(after panic) error = %v", err)
	}
}

func TestAdvisoryLockXactLockWithinTransaction(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	lockID := int64(34567)
	err := store.WithTx(ctx, testDB.Pool, func(txQ *store.Queries) error {
		if lockErr := txQ.AdvisoryXactLock(ctx, lockID); lockErr != nil {
			return lockErr
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WithTx(AdvisoryXactLock) error = %v", err)
	}

	acquired, err := q.TryAdvisoryLock(ctx, lockID)
	if err != nil {
		t.Fatalf("TryAdvisoryLock() error = %v", err)
	}
	if !acquired {
		t.Fatal("TryAdvisoryLock() = false, want true")
	}

	if err := q.ReleaseAdvisoryLock(ctx, lockID); err != nil {
		t.Fatalf("ReleaseAdvisoryLock() error = %v", err)
	}
}

func mustCreateJobRunWithBuildFactory(t *testing.T, ctx context.Context, q *store.Queries, projectID string, status domain.RunStatus) (*domain.Job, *domain.JobRun) {
	t.Helper()

	job := testutil.BuildJob(&testutil.JobOpts{
		ID:        new(newID()),
		ProjectID: new(projectID),
		Name:      new("job-" + newID()),
		Slug:      new("job-slug-" + newID()),
	})
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	run := testutil.BuildRun(job, &testutil.RunOpts{
		ID:     new(newID()),
		Status: new(status),
	})
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	return job, run
}

func mustCreateWorkflowStepFixture(t *testing.T, ctx context.Context, q *store.Queries, projectID string, stepStatus domain.StepRunStatus) (*domain.Workflow, *domain.WorkflowRun, *domain.WorkflowStepRun) {
	t.Helper()

	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: new(projectID),
		Name:      new("workflow-" + newID()),
		Slug:      new("workflow-slug-" + newID()),
	})
	stepJob := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: new(projectID)})
	step := testutil.MustCreateWorkflowStep(t, ctx, q, wf.ID, &testutil.WorkflowStepOpts{JobID: new(stepJob.ID), StepRef: new("step-" + newID())})
	wfRun := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{ProjectID: new(projectID)})
	stepRun := testutil.MustCreateWorkflowStepRun(t, ctx, q, wfRun.ID, step.ID, &testutil.WorkflowStepRunOpts{Status: new(stepStatus), StepRef: new(step.StepRef)})

	return wf, wfRun, stepRun
}

func mustCreateJobRunEventTrigger(
	t *testing.T,
	ctx context.Context,
	q *store.Queries,
	projectID string,
	jobRunID string,
	status string,
	eventKey string,
	requestedAt time.Time,
	receivedAt *time.Time,
	expiresAt *time.Time,
) *domain.EventTrigger {
	t.Helper()

	triggerExpiresAt := requestedAt.Add(5 * time.Minute)
	if expiresAt != nil {
		triggerExpiresAt = *expiresAt
	}

	trigger := &domain.EventTrigger{
		ID:             newID(),
		EventKey:       eventKey,
		ProjectID:      projectID,
		SourceType:     "job_run",
		JobRunID:       jobRunID,
		Status:         status,
		RequestPayload: json.RawMessage(`{"source":"job"}`),
		TimeoutSecs:    300,
		RequestedAt:    requestedAt,
		ReceivedAt:     receivedAt,
		ExpiresAt:      triggerExpiresAt,
		NotifyURL:      "https://example.com/notify-job",
		NotifyStatus:   "pending",
		TriggerType:    "event",
	}
	if err := q.CreateEventTrigger(ctx, trigger); err != nil {
		t.Fatalf("CreateEventTrigger() error = %v", err)
	}

	return trigger
}

func mustCreateWorkflowStepEventTrigger(
	t *testing.T,
	ctx context.Context,
	q *store.Queries,
	projectID string,
	workflowRunID string,
	workflowStepRunID string,
	status string,
	eventKey string,
	requestedAt time.Time,
	receivedAt *time.Time,
) *domain.EventTrigger {
	t.Helper()

	trigger := &domain.EventTrigger{
		ID:                newID(),
		EventKey:          eventKey,
		ProjectID:         projectID,
		SourceType:        "workflow_step",
		WorkflowRunID:     workflowRunID,
		WorkflowStepRunID: workflowStepRunID,
		Status:            status,
		RequestPayload:    json.RawMessage(`{"source":"step"}`),
		TimeoutSecs:       180,
		RequestedAt:       requestedAt,
		ReceivedAt:        receivedAt,
		ExpiresAt:         requestedAt.Add(10 * time.Minute),
		NotifyStatus:      "pending",
		TriggerType:       "event",
	}
	if err := q.CreateEventTrigger(ctx, trigger); err != nil {
		t.Fatalf("CreateEventTrigger() error = %v", err)
	}

	return trigger
}
