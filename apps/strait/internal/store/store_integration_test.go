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
	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	require.Nil(t, testRedisErr)

	env := &testutil.TestEnv{DB: testDB, Redis: testRedis}
	require.NoError(t, env.
		Clean(ctx),
	)

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
	require.NoError(t, err)

	q := mustStore(t)
	job, err := q.GetJob(ctx, jobID)
	require.NoError(t, err)
	require.Equal(t, jobID,

		job.ID)

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
	require.True(t, errors.Is(err, wantErr))

	q := mustStore(t)
	_, err = q.GetJob(ctx, jobID)
	require.True(t, errors.Is(err, store.
		ErrJobNotFound,
	))

}

func TestUpsertJobMemoryWithQuota_ConcurrentPerJobLimit(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "project-job-memory-quota-concurrent")
	require.NoError(t, q.CreateJob(ctx,
		job))

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
			require.Failf(t, "test failure", "UpsertJobMemoryWithQuota() error = %v", err)
		}
	}
	require.EqualValues(t, 1, successes)
	require.EqualValues(t, 1, quotaErrors)

	total, err := q.SumJobMemorySizeBytes(ctx, job.ID)
	require.NoError(t, err)
	require.EqualValues(t, 8, total)

	items, err := q.ListJobMemory(ctx, job.ID)
	require.NoError(t, err)
	require.Len(t, items, 1)

}

func TestUpsertJobMemoryWithQuota_ReplacingExistingKeyUsesNetDelta(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "project-job-memory-quota-replace")
	require.NoError(t, q.CreateJob(ctx,
		job))

	initial := &domain.JobMemory{
		JobID:     job.ID,
		ProjectID: job.ProjectID,
		MemoryKey: "profile",
		Value:     json.RawMessage(`"123456789"`),
		SizeBytes: 9,
	}
	require.NoError(t, q.UpsertJobMemoryWithQuota(ctx, initial,
		1024, 10))

	replacement := &domain.JobMemory{
		JobID:     job.ID,
		ProjectID: job.ProjectID,
		MemoryKey: "profile",
		Value:     json.RawMessage(`"1234567890"`),
		SizeBytes: 10,
	}
	require.NoError(t, q.UpsertJobMemoryWithQuota(ctx, replacement,
		1024,
		10))

	var beforeNoopXmin string
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT xmin::text
		FROM job_memory
		WHERE job_id = $1 AND memory_key = $2`,

		job.ID,
		"profile",
	).Scan(&beforeNoopXmin))

	sameReplacement := &domain.JobMemory{
		JobID:     job.ID,
		ProjectID: job.ProjectID,
		MemoryKey: "profile",
		Value:     json.RawMessage(`"1234567890"`),
		SizeBytes: 10,
	}
	require.NoError(t, q.UpsertJobMemoryWithQuota(ctx, sameReplacement,
		1024,
		10))

	var afterNoopXmin string
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT xmin::text
		FROM job_memory
		WHERE job_id = $1 AND memory_key = $2`,

		job.ID,
		"profile",
	).Scan(&afterNoopXmin))
	require.Equal(t, beforeNoopXmin,

		afterNoopXmin,
	)

	got, err := q.GetJobMemory(ctx, job.ID, "profile")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.EqualValues(t, 10, got.
		SizeBytes,
	)

	total, err := q.SumJobMemorySizeBytes(ctx, job.ID)
	require.NoError(t, err)
	require.EqualValues(t, 10, total)

}

func TestCreateJob(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "project-create-job")
	require.NoError(t, q.CreateJob(ctx,
		job))
	require.NotEqual(t, "",

		job.ID)
	require.False(t, job.CreatedAt.
		IsZero())
	require.False(t, job.UpdatedAt.
		IsZero())

	got, err := q.GetJob(ctx, job.ID)
	require.NoError(t, err)

	assertJobEqual(t, job, got)
}

func TestCreateJob_CustomID(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	customID := newID()
	job := baseJob(customID, "project-create-job-custom-id")
	require.NoError(t, q.CreateJob(ctx,
		job))
	require.Equal(t, customID,

		job.ID,
	)

}

func TestGetJob(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "project-get-job")
	require.NoError(t, q.CreateJob(ctx,
		job))

	got, err := q.GetJob(ctx, job.ID)
	require.NoError(t, err)

	assertJobEqual(t, job, got)
}

func TestGetJob_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, err := q.GetJob(ctx, newID())
	require.True(t, errors.Is(err, store.
		ErrJobNotFound,
	))

}

func TestGetJobBySlug(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "project-get-job-by-slug")
	require.NoError(t, q.CreateJob(ctx,
		job))

	got, err := q.GetJobBySlug(ctx, job.ProjectID, job.Slug)
	require.NoError(t, err)

	assertJobEqual(t, job, got)
}

func TestGetJobBySlug_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "project-get-job-by-slug-not-found")
	require.NoError(t, q.CreateJob(ctx,
		job))

	_, err := q.GetJobBySlug(ctx, job.ProjectID, "does-not-exist")
	require.True(t, errors.Is(err, store.
		ErrJobNotFound,
	))

}

func TestEndpointCircuitState_OpensAndBlocksDispatch(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	endpoint := "https://example.com/circuit-open"
	now := time.Now().UTC()
	require.NoError(t, q.RecordEndpointCircuitFailure(ctx,
		endpoint, now,
		2, 2*time.
			Minute),
	)

	allowed, retryAt, err := q.CanDispatchEndpoint(ctx, endpoint, now)
	require.NoError(t, err)
	require.True(t, allowed)
	require.Nil(t, retryAt)
	require.NoError(t, q.RecordEndpointCircuitFailure(ctx,
		endpoint, now.Add(time.Second),
		2, 2*time.
			Minute,
	))

	allowed, retryAt, err = q.CanDispatchEndpoint(ctx, endpoint, now.Add(2*time.Second))
	require.NoError(t, err)
	require.False(t, allowed)
	require.NotNil(t, retryAt)

	state, err := q.GetEndpointCircuitState(ctx, endpoint)
	require.NoError(t, err)
	require.NotNil(t, state)
	require.Equal(t, domain.
		CircuitStateOpen,

		state.State,
	)

}

func TestEndpointCircuitState_NewEndpointDoesNotCreateCircuitRow(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	endpoint := "https://example.com/circuit-fast-path-" + newID()
	allowed, retryAt, err := q.CanDispatchEndpoint(ctx, endpoint, time.Now().UTC())
	require.NoError(t, err)
	require.True(t, allowed)
	require.Nil(t, retryAt)

	state, err := q.GetEndpointCircuitState(ctx, endpoint)
	require.NoError(t, err)
	require.Nil(t, state)

}

func TestEndpointCircuitState_ExpiredOpenCircuitResetsOnDispatch(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	endpoint := "https://example.com/circuit-expired-" + newID()
	now := time.Now().UTC()
	require.NoError(t, q.RecordEndpointCircuitFailure(ctx,
		endpoint, now.Add(-time.Minute),
		1, time.
			Second),
	)

	allowed, retryAt, err := q.CanDispatchEndpoint(ctx, endpoint, now.Add(2*time.Second))
	require.NoError(t, err)
	require.True(t, allowed)
	require.Nil(t, retryAt)

	state, err := q.GetEndpointCircuitState(ctx, endpoint)
	require.NoError(t, err)
	require.NotNil(t, state)
	require.Equal(t, domain.
		CircuitStateClosed,

		state.State,
	)
	require.EqualValues(t, 0, state.
		ConsecutiveFailures,
	)
	require.Nil(t, state.
		HalfOpenUntil,
	)

}

func TestEndpointCircuitState_ClosesAfterSuccess(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	endpoint := "https://example.com/circuit-close"
	now := time.Now().UTC()
	require.NoError(t, q.RecordEndpointCircuitFailure(ctx,
		endpoint, now,
		1, time.Minute,
	))
	require.NoError(t, q.RecordEndpointCircuitSuccess(ctx,
		endpoint))

	state, err := q.GetEndpointCircuitState(ctx, endpoint)
	require.NoError(t, err)
	require.NotNil(t, state)
	require.Equal(t, domain.
		CircuitStateClosed,

		state.State,
	)
	require.EqualValues(t, 0, state.
		ConsecutiveFailures,
	)

	allowed, retryAt, err := q.CanDispatchEndpoint(ctx, endpoint, now.Add(time.Second))
	require.NoError(t, err)
	require.True(t, allowed)
	require.Nil(t, retryAt)

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
		require.NoError(t, q.CreateJob(ctx,
			job))

	}

	jobs, err := q.ListJobs(ctx, projectID, 10000, nil)
	require.NoError(t, err)
	require.Len(t, jobs, 3)

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
		require.NoError(t, q.CreateJob(ctx,
			job))

	}

	other := baseJob(newID(), otherProject)
	require.NoError(t, q.CreateJob(ctx,
		other),
	)

	jobs, err := q.ListJobs(ctx, targetProject, 10000, nil)
	require.NoError(t, err)
	require.Len(t, jobs, 2)

	for _, job := range jobs {
		require.Equal(t, targetProject,

			job.
				ProjectID,
		)

	}
}

func TestListJobsByTag(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-list-jobs-by-tag"
	jobA := baseJob(newID(), projectID)
	jobA.Tags = map[string]string{"team": "core", "service": "scheduler"}
	require.NoError(t, q.CreateJob(ctx,
		jobA))

	jobB := baseJob(newID(), projectID)
	jobB.Tags = map[string]string{"team": "platform"}
	require.NoError(t, q.CreateJob(ctx,
		jobB))

	jobC := baseJob(newID(), projectID)
	require.NoError(t, q.CreateJob(ctx,
		jobC))

	jobs, err := q.ListJobsByTag(ctx, projectID, "team", "core", 10000, nil)
	require.NoError(t, err)
	require.Len(t, jobs, 1)
	require.Equal(t, jobA.ID,

		jobs[0].
			ID)
	require.Equal(t, "scheduler",

		jobs[0].Tags["service"],
	)

	jobs, err = q.ListJobsByTag(ctx, projectID, "team", "", 10000, nil)
	require.NoError(t, err)
	require.Len(t, jobs, 2)

}

func TestUpdateJob(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "project-update-job")
	require.NoError(t, q.CreateJob(ctx,
		job))

	prevUpdatedAt := job.UpdatedAt
	job.Name = "updated-name"
	job.Slug = "updated-slug"
	job.EndpointURL = "https://example.com/new-endpoint"
	require.NoError(t, q.UpdateJob(ctx,
		job))
	require.True(t, job.UpdatedAt.
		After(prevUpdatedAt))

	got, err := q.GetJob(ctx, job.ID)
	require.NoError(t, err)
	require.False(t, got.Name !=
		"updated-name" ||
		got.Slug !=
			"updated-slug" ||
		got.
			EndpointURL !=
			"https://example.com/new-endpoint",
	)

}

func TestDeleteJob(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "project-delete-job")
	require.NoError(t, q.CreateJob(ctx,
		job))
	require.NoError(t, q.DeleteJob(ctx,
		job.ID,
	))

	_, err := q.GetJob(ctx, job.ID)
	require.True(t, errors.Is(err, store.
		ErrJobNotFound,
	))

}

func TestDeleteJob_RemovesJobMemory(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "project-delete-job-memory")
	require.NoError(t, q.CreateJob(ctx,
		job))

	memory := &domain.JobMemory{
		JobID:     job.ID,
		ProjectID: job.ProjectID,
		MemoryKey: "cursor",
		Value:     json.RawMessage(`{"page":1}`),
		SizeBytes: len(`{"page":1}`),
	}
	require.NoError(t, q.UpsertJobMemory(ctx,
		memory))
	require.NoError(t, q.DeleteJob(ctx,
		job.ID,
	))

	memories, err := q.ListJobMemory(ctx, job.ID)
	require.NoError(t, err)
	require.Len(t, memories,

		0)

}

func TestListCronJobs(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	enabledCron := baseJob(newID(), "project-list-cron-jobs")
	enabledCron.Cron = "*/5 * * * *"
	enabledCron.Enabled = true
	require.NoError(t, q.CreateJob(ctx,
		enabledCron,
	))

	disabledCron := baseJob(newID(), "project-list-cron-jobs")
	disabledCron.Cron = "*/10 * * * *"
	disabledCron.Enabled = false
	require.NoError(t, q.CreateJob(ctx,
		disabledCron,
	))

	enabledNoCron := baseJob(newID(), "project-list-cron-jobs")
	enabledNoCron.Cron = ""
	enabledNoCron.Enabled = true
	require.NoError(t, q.CreateJob(ctx,
		enabledNoCron,
	))

	jobs, err := q.ListCronJobs(ctx)
	require.NoError(t, err)
	require.Len(t, jobs, 1)
	require.Equal(t, enabledCron.
		ID,
		jobs[0].ID,
	)

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
	require.NoError(t, q.CreateRun(ctx,
		run))
	require.EqualValues(t, 1, run.
		Attempt)
	require.Equal(t, domain.
		TriggerManual,
		run.
			TriggeredBy,
	)
	require.False(t, run.CreatedAt.
		IsZero())

	got, err := q.GetRun(ctx, run.ID)
	require.NoError(t, err)

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
	require.NoError(t, q.CreateRun(ctx,
		run))
	require.Equal(t, domain.
		StatusDelayed,
		run.
			Status)

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
	require.NoError(t, q.CreateRun(ctx,
		run))

	got, err := q.GetRun(ctx, run.ID)
	require.NoError(t, err)

	assertRunEqual(t, run, got)
}

func TestGetRun_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, err := q.GetRun(ctx, newID())
	require.True(t, errors.Is(err, store.
		ErrRunNotFound,
	))

}

func TestGetRunByIdempotencyKey(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-get-run-idempotency")
	run := baseRun(job, newID())
	run.IdempotencyKey = newID()
	require.NoError(t, q.CreateRun(ctx,
		run))

	got, err := q.GetRunByIdempotencyKey(ctx, job.ID, run.IdempotencyKey)
	require.NoError(t, err)
	require.NotNil(t, got)

	assertRunEqual(t, run, got)
}

func TestGetRunByIdempotencyKey_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-get-run-idempotency-not-found")

	got, err := q.GetRunByIdempotencyKey(ctx, job.ID, "missing-key")
	require.NoError(t, err)
	require.Nil(t, got)

}

func TestGetRunByIdempotencyKey_AllowsTerminalReplayAndReturnsLatest(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-idempotency-replay")
	key := newID()

	first := baseRun(job, newID())
	first.IdempotencyKey = key
	require.NoError(t, q.CreateRun(ctx,
		first),
	)
	require.NoError(t, q.UpdateRunStatus(ctx,
		first.ID, domain.
			StatusQueued,
		domain.
			StatusDequeued,

		map[string]any{"started_at": time.Now().UTC()}))
	require.NoError(t, q.UpdateRunStatus(ctx,
		first.ID, domain.
			StatusDequeued,
		domain.
			StatusExecuting,

		map[string]any{
			"started_at": time.
				Now().UTC(),
		}))
	require.NoError(t, q.UpdateRunStatus(ctx,
		first.ID, domain.
			StatusExecuting,
		domain.
			StatusFailed,

		map[string]any{"finished_at": time.Now().UTC(),
			"error": "exhausted"}))

	second := baseRun(job, newID())
	second.IdempotencyKey = key
	require.NoError(t, q.CreateRun(ctx,
		second,
	))

	got, err := q.GetRunByIdempotencyKey(ctx, job.ID, key)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, second.
		ID, got.ID,
	)

}

// TestIdempotencyIndex_ConsolidatedShape verifies migration 000255 dropped the
// partial-on-terminal-status index and replaced it with a non-partial-on-status
// index keyed only on (job_id, idempotency_key). The shape prevents write
// amplification on terminal status flips.
func TestIdempotencyIndex_ConsolidatedShape(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	var terminalExists bool
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'idx_runs_idempotency_terminal')`,
	).Scan(
		&terminalExists,
	))
	assert.False(t, terminalExists)

	var newExists bool
	var indexDef string
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'idx_runs_idempotency'),
		        COALESCE((SELECT indexdef FROM pg_indexes WHERE indexname = 'idx_runs_idempotency'), '')`,
	).Scan(
		&newExists, &indexDef))
	require.True(t, newExists)
	assert.True(t, strings.Contains(indexDef,

		"(job_id, idempotency_key)",
	),
	)
	assert.True(t, strings.Contains(indexDef,

		"idempotency_key IS NOT NULL",
	))
	assert.False(t, strings.Contains(
		indexDef,
		"status"))

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
	require.NoError(t, q.CreateRun(ctx,
		run))

	if _, err := testDB.Pool.Exec(ctx,
		`UPDATE job_runs SET status = 'completed', finished_at = NOW() - INTERVAL '25 hours' WHERE id = $1`,
		run.ID,
	); err != nil {
		require.Failf(t, "test failure",

			"backdate finished_at: %v", err)
	}

	got, err := q.GetRunByIdempotencyKey(ctx, job.ID, key)
	require.NoError(t, err)
	require.Nil(t, got)

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
	require.NoError(t, q.CreateRun(ctx,
		run))

	if _, err := testDB.Pool.Exec(ctx,
		`UPDATE job_runs SET status = 'completed', finished_at = NOW() - INTERVAL '1 hour' WHERE id = $1`,
		run.ID,
	); err != nil {
		require.Failf(t, "test failure",

			"set finished_at: %v", err)
	}

	got, err := q.GetRunByIdempotencyKey(ctx, job.ID, key)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, run.ID,

		got.ID)

}

func TestCreateRun_IdempotencyConflict_ActiveRun(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-idem-conflict")
	key := newID()

	first := baseRun(job, newID())
	first.IdempotencyKey = key
	require.NoError(t, q.CreateRun(ctx,
		first),
	)

	// Second run with same key+job while first is still queued (active) → conflict.
	second := baseRun(job, newID())
	second.IdempotencyKey = key
	err := q.CreateRun(ctx, second)
	require.True(t, errors.Is(err, domain.
		ErrIdempotencyConflict,
	))

}

func TestCreateRun_IdempotencyConflict_AllowsAfterTerminal(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-idem-terminal")
	key := newID()

	first := baseRun(job, newID())
	first.IdempotencyKey = key
	require.NoError(t, q.CreateRun(ctx,
		first),
	)
	require.NoError(t, q.UpdateRunStatus(ctx,
		first.ID, domain.
			StatusQueued,
		domain.
			StatusDequeued,

		map[string]any{"started_at": time.Now().UTC()}))
	require.NoError(t, q.UpdateRunStatus(ctx,
		first.ID, domain.
			StatusDequeued,
		domain.
			StatusExecuting,

		map[string]any{}))
	require.NoError(t, q.UpdateRunStatus(ctx,
		first.ID, domain.
			StatusExecuting,
		domain.
			StatusFailed,

		map[string]any{"finished_at": time.Now().UTC(),
			"error": "done"}))

	// Transition first to terminal state.

	// Second run with same key should succeed now.
	second := baseRun(job, newID())
	second.IdempotencyKey = key
	require.NoError(t, q.CreateRun(ctx,
		second,
	))

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
	require.NoError(t, q.CreateRun(ctx,
		runA))

	// Same key on different job → should succeed.
	runB := baseRun(jobB, newID())
	runB.IdempotencyKey = key
	require.NoError(t, q.CreateRun(ctx,
		runB))

	// Same key on same job → conflict.
	runA2 := baseRun(jobA, newID())
	runA2.IdempotencyKey = key
	err := q.CreateRun(ctx, runA2)
	require.True(t, errors.Is(err, domain.
		ErrIdempotencyConflict,
	))

}

func TestIdempotencyIndex_NullKeyAllowsDuplicates(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-idem-null")

	// Two runs with empty key (stored as NULL) on same job → both succeed.
	run1 := baseRun(job, newID())
	require.NoError(t, q.CreateRun(ctx,
		run1))

	run2 := baseRun(job, newID())
	require.NoError(t, q.CreateRun(ctx,
		run2))

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
			require.NoError(t, q.CreateRun(ctx,
				run))
			require.NoError(t, q.UpdateRunStatus(ctx,
				run.ID, domain.
					StatusQueued,
				domain.StatusDequeued,

				map[string]any{"started_at": time.
					Now().
					UTC()}))
			require.NoError(t, q.UpdateRunStatus(ctx,
				run.ID, domain.
					StatusDequeued,
				domain.
					StatusExecuting,

				map[string]any{}),
			)
			require.NoError(t, q.UpdateRunStatus(ctx,
				run.ID, domain.
					StatusExecuting,
				tt.status,
				map[string]any{"finished_at": time.Now().
					UTC(),
					"error": "test",
				}))

			// Move through FSM to terminal.

			// New run with same key should succeed.
			run2 := baseRun(job, newID())
			run2.IdempotencyKey = key
			require.NoError(t, q.CreateRun(ctx,
				run2))

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
		require.NoError(t, q.CreateRun(ctx,
			run))

	}

	runs, err := q.ListRunsByJob(ctx, job.ID, 2, 1)
	require.NoError(t, err)
	require.Len(t, runs, 2)

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
	require.NoError(t, q.CreateRun(ctx,
		queued1,
	))

	failed := baseRun(job, newID())
	failed.Status = domain.StatusFailed
	require.NoError(t, q.CreateRun(ctx,
		failed,
	))

	queued2 := baseRun(job, newID())
	queued2.Status = domain.StatusQueued
	require.NoError(t, q.CreateRun(ctx,
		queued2,
	))

	otherRun := baseRun(other, newID())
	require.NoError(t, q.CreateRun(ctx,
		otherRun,
	))

	status := domain.StatusQueued
	filtered, err := q.ListRunsByProject(ctx, projectID, &status, nil, nil, nil, nil, nil, nil, nil, 10, nil)
	require.NoError(t, err)
	require.Len(t, filtered,

		2)

	for _, run := range filtered {
		require.False(t, run.ProjectID !=
			projectID ||
			run.Status !=
				domain.StatusQueued,
		)

	}

	firstPage, err := q.ListRunsByProject(ctx, projectID, nil, nil, nil, nil, nil, nil, nil, nil, 2, nil)
	require.NoError(t, err)
	require.Len(t, firstPage,

		2)

	assertTimesDesc(t, extractRunCreatedAt(firstPage))

	cursor := firstPage[len(firstPage)-1].CreatedAt
	secondPage, err := q.ListRunsByProject(ctx, projectID, nil, nil, nil, nil, nil, nil, nil, nil, 2, &cursor)
	require.NoError(t, err)
	require.Len(t, secondPage,

		1)

}

func TestListRunsByProject_MetadataFilter(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-list-runs-by-project-metadata"
	job := mustCreateJob(t, ctx, q, projectID)

	runProd := baseRun(job, newID())
	require.NoError(t, q.CreateRun(ctx,
		runProd,
	))
	require.NoError(t, q.UpdateRunMetadata(ctx,
		runProd.ID,
		map[string]string{"env": "prod",
			"region": "eu",
		}))

	runStage := baseRun(job, newID())
	require.NoError(t, q.CreateRun(ctx,
		runStage,
	))
	require.NoError(t, q.UpdateRunMetadata(ctx,
		runStage.
			ID, map[string]string{"env": "stage"}))

	runNoMetadata := baseRun(job, newID())
	require.NoError(t, q.CreateRun(ctx,
		runNoMetadata,
	))

	key := "env"
	value := "prod"
	filtered, err := q.ListRunsByProject(ctx, projectID, nil, &key, &value, nil, nil, nil, nil, nil, 20, nil)
	require.NoError(t, err)
	require.Len(t, filtered,

		1)
	require.Equal(t, runProd.
		ID, filtered[0].ID,
	)

	keyOnly, err := q.ListRunsByProject(ctx, projectID, nil, &key, nil, nil, nil, nil, nil, nil, 20, nil)
	require.NoError(t, err)
	require.Len(t, keyOnly,

		2)

}

func TestUpdateRunStatus(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-update-run-status")
	run := baseRun(job, newID())
	require.NoError(t, q.CreateRun(ctx,
		run))

	startedAt := time.Now().UTC().Truncate(time.Microsecond)
	fields := map[string]any{
		"started_at": startedAt,
	}
	require.NoError(t, q.UpdateRunStatus(ctx,
		run.ID, domain.
			StatusQueued,
		domain.StatusDequeued,

		fields))

	got, err := q.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		StatusDequeued,
		got.
			Status)
	require.False(t, got.StartedAt ==
		nil || !got.StartedAt.
		Equal(startedAt))

}

func TestUpdateRunStatusMetadataIsAppendOnly(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-update-run-status-metadata")
	run := baseRun(job, newID())
	require.NoError(t, q.CreateRun(ctx,
		run))

	fields := map[string]any{
		"metadata": map[string]string{
			"snooze_count": "1",
			"phase":        "retry",
		},
	}
	require.NoError(t, q.UpdateRunStatus(ctx,
		run.ID, domain.
			StatusQueued,
		domain.StatusDequeued,

		fields))

	var ledgerHasSnooze bool
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT metadata ? 'snooze_count' FROM job_runs WHERE id = $1`,

		run.ID,
	).Scan(&ledgerHasSnooze))
	require.False(t, ledgerHasSnooze)

	got, err := q.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.False(t, got.Metadata["snooze_count"] != "1" ||
		got.Metadata["phase"] !=
			"retry",
	)

	key := "snooze_count"
	value := "1"
	listed, err := q.ListRunsByProject(ctx, run.ProjectID, nil, &key, &value, nil, nil, nil, nil, nil, 10, nil)
	require.NoError(t, err)
	require.False(t, len(listed) != 1 ||
		listed[0].ID !=
			run.ID)

}

func TestUpdateRunStatusReturningOld(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-update-run-status-old")
	run := baseRun(job, newID())
	require.NoError(t, q.CreateRun(ctx,
		run))

	oldStatus, err := q.UpdateRunStatusReturningOld(ctx, run.ID, domain.StatusQueued, domain.StatusDequeued, nil)
	require.NoError(t, err)
	require.Equal(t, domain.
		StatusQueued,
		oldStatus,
	)

	got, err := q.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		StatusDequeued,
		got.
			Status)

}

func TestUpdateRunStatus_InvalidTransition(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-update-run-status-invalid")
	run := baseRun(job, newID())
	run.Status = domain.StatusCompleted
	require.NoError(t, q.CreateRun(ctx,
		run))

	err := q.UpdateRunStatus(ctx, run.ID, domain.StatusCompleted, domain.StatusExecuting, nil)
	require.Error(t, err)

}

func TestUpdateRunStatus_OptimisticLock(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-update-run-status-lock")
	run := baseRun(job, newID())
	require.NoError(t, q.CreateRun(ctx,
		run))

	err := q.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusCompleted, nil)
	require.True(t, errors.Is(err, store.
		ErrRunConflict,
	))

}

func TestUpdateRunStatus_WithFields(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-update-run-status-fields")
	run := baseRun(job, newID())
	run.Status = domain.StatusExecuting
	require.NoError(t, q.CreateRun(ctx,
		run))

	finishedAt := time.Now().UTC().Truncate(time.Microsecond)
	fields := map[string]any{
		"result":      []byte(`{"ok":true}`),
		"error":       "execution failed",
		"finished_at": finishedAt,
	}
	require.NoError(t, q.UpdateRunStatus(ctx,
		run.ID, domain.
			StatusExecuting,
		domain.
			StatusCompleted,

		fields,
	))

	got, err := q.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		StatusCompleted,
		got.
			Status)
	require.True(t, jsonEqual(got.Result,
		[]byte(`{"ok":true}`)))
	require.Equal(t, "execution failed",

		got.Error,
	)
	require.False(t, got.FinishedAt ==
		nil ||
		!got.FinishedAt.
			Equal(finishedAt))

}

func TestUpdateHeartbeat(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-update-heartbeat")
	run := baseRun(job, newID())
	require.NoError(t, q.CreateRun(ctx,
		run))
	require.NoError(t, q.UpdateHeartbeat(ctx,
		run.ID))

	got, err := q.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.False(t, got.HeartbeatAt ==
		nil ||
		got.HeartbeatAt.
			IsZero())

}

func TestUpdateHeartbeat_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	err := q.UpdateHeartbeat(ctx, newID())
	require.True(t, errors.Is(err, store.
		ErrRunNotFound,
	))

}

func TestListChildRuns(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-list-child-runs")
	parent := baseRun(job, newID())
	require.NoError(t, q.CreateRun(ctx,
		parent,
	))

	child1 := baseRun(job, newID())
	child1.ParentRunID = parent.ID
	require.NoError(t, q.CreateRun(ctx,
		child1,
	))

	child2 := baseRun(job, newID())
	child2.ParentRunID = parent.ID
	require.NoError(t, q.CreateRun(ctx,
		child2,
	))

	children, err := q.ListChildRuns(ctx, parent.ID, 10000, nil)
	require.NoError(t, err)
	require.Len(t, children,

		2)

	for _, child := range children {
		require.Equal(t, parent.
			ID, child.
			ParentRunID,
		)

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
	require.NoError(t, q.CreateRun(ctx,
		oldCompleted,
	))
	require.NoError(t, q.UpdateRunStatus(ctx,
		oldCompleted.
			ID, domain.StatusExecuting,

		domain.
			StatusCompleted,

		map[string]any{"finished_at": finishedOldCompleted}))

	// Backdate created_at so the run is in a cold partition (the reaper's
	// hot-partition filter skips the current month).
	if _, err := testDB.Pool.Exec(ctx, `UPDATE job_runs SET created_at = $1 WHERE id = $2`, finishedOldCompleted, oldCompleted.ID); err != nil {
		require.Failf(t, "test failure",

			"backdate oldCompleted: %v", err)
	}

	oldTimedOut := baseRun(job, newID())
	oldTimedOut.Status = domain.StatusTimedOut
	finishedOldTimedOut := now.Add(-91 * 24 * time.Hour)
	oldTimedOut.FinishedAt = &finishedOldTimedOut
	require.NoError(t, q.CreateRun(ctx,
		oldTimedOut,
	))

	if _, err := testDB.Pool.Exec(ctx, `UPDATE job_runs SET created_at = $1 WHERE id = $2`, finishedOldTimedOut, oldTimedOut.ID); err != nil {
		require.Failf(t, "test failure",

			"backdate oldTimedOut: %v", err)
	}

	recentCompleted := baseRun(job, newID())
	recentCompleted.Status = domain.StatusCompleted
	finishedRecent := now.Add(-5 * 24 * time.Hour)
	recentCompleted.FinishedAt = &finishedRecent
	require.NoError(t, q.CreateRun(ctx,
		recentCompleted,
	))

	queued := baseRun(job, newID())
	queued.Status = domain.StatusQueued
	require.NoError(t, q.CreateRun(ctx,
		queued,
	))

	seedRetentionSideRows(t, ctx, oldCompleted.ID, oldTimedOut.ID, recentCompleted.ID, queued.ID)

	deleted, err := q.DeleteTerminalRunsPastRetention(ctx, 30*24*time.Hour, 90*24*time.Hour)
	require.NoError(t, err)
	require.EqualValues(t, 2, deleted)

	if _, err := q.GetRun(ctx, oldCompleted.ID); !errors.Is(err, store.ErrRunNotFound) {
		require.Failf(t, "test failure",

			"GetRun(oldCompleted) error = %v, want ErrRunNotFound", err)
	}
	if _, err := q.GetRun(ctx, oldTimedOut.ID); !errors.Is(err, store.ErrRunNotFound) {
		require.Failf(t, "test failure",

			"GetRun(oldTimedOut) error = %v, want ErrRunNotFound", err)
	}
	if _, err := q.GetRun(ctx, recentCompleted.ID); err != nil {
		require.Failf(t, "test failure",

			"GetRun(recentCompleted) error = %v", err)
	}
	if _, err := q.GetRun(ctx, queued.ID); err != nil {
		require.Failf(t, "test failure",

			"GetRun(queued) error = %v", err)
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
		require.Failf(t, "test failure",

			"seed batch job_runs: %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_run_active_claims (run_id, ready_generation, attempt)
		SELECT run_id, ready_generation, attempt
		FROM job_run_state
		WHERE run_id LIKE $1 || '%'`, prefix); err != nil {
		require.Failf(t, "test failure",

			"seed batch active claims: %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_run_lifecycle_events (run_id, from_status, to_status, attempt, fields)
		SELECT run_id, 'queued', 'completed', attempt, '{"source":"retention-batch"}'::jsonb
		FROM job_run_state
		WHERE run_id LIKE $1 || '%'`, prefix); err != nil {
		require.Failf(t, "test failure",

			"seed batch lifecycle events: %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_run_ready_events (run_id, ready_generation, attempt, reason)
		SELECT run_id, ready_generation, attempt, 'retention_batch'
		FROM job_run_state
		WHERE run_id LIKE $1 || '%'
		ON CONFLICT DO NOTHING`, prefix); err != nil {
		require.Failf(t, "test failure",

			"seed batch ready events: %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_retries (run_id, next_retry_at, attempt, scheduled_at, cleared)
		SELECT run_id, NOW() + INTERVAL '1 minute', attempt + 1, NOW(), FALSE
		FROM job_run_state
		WHERE run_id LIKE $1 || '%'`, prefix); err != nil {
		require.Failf(t, "test failure",

			"seed batch retries: %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_run_priority_events (run_id, priority)
		SELECT run_id, priority + 1
		FROM job_run_state
		WHERE run_id LIKE $1 || '%'`, prefix); err != nil {
		require.Failf(t, "test failure",

			"seed batch priority events: %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_run_visibility_events (run_id, visible_until)
		SELECT run_id, NOW() + INTERVAL '1 hour'
		FROM job_run_state
		WHERE run_id LIKE $1 || '%'`, prefix); err != nil {
		require.Failf(t, "test failure",

			"seed batch visibility events: %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_run_cache_versions (run_id, cache_version)
		SELECT run_id, 2
		FROM job_run_state
		WHERE run_id LIKE $1 || '%'`, prefix); err != nil {
		require.Failf(t, "test failure",

			"seed batch cache versions: %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_run_heartbeats (run_id, heartbeat_at)
		SELECT run_id, NOW()
		FROM job_run_state
		WHERE run_id LIKE $1 || '%'
		ON CONFLICT DO NOTHING`, prefix); err != nil {
		require.Failf(t, "test failure",

			"seed batch heartbeats: %v", err)
	}

	for _, table := range []string{"job_runs", "job_run_state", "job_run_active_claims", "job_run_lifecycle_events", "job_run_ready_events", "job_retries", "job_run_priority_events", "job_run_visibility_events", "job_run_cache_versions", "job_run_heartbeats"} {
		require.EqualValues(t, runCount,

			countRunRowsByPrefix(t, ctx,
				table, prefix),
		)

	}

	deleted, err := q.DeleteTerminalRunsPastRetention(ctx, 30*24*time.Hour, 90*24*time.Hour)
	require.NoError(t, err)
	require.EqualValues(t, 5000,
		deleted,
	)

	for _, table := range []string{"job_runs", "job_run_state", "job_run_active_claims", "job_run_lifecycle_events", "job_run_ready_events", "job_retries", "job_run_priority_events", "job_run_visibility_events", "job_run_cache_versions", "job_run_heartbeats"} {
		require.Equal(t, runCount-
			deleted,
			countRunRowsByPrefix(t, ctx, table,
				prefix),
		)

	}

	deleted, err = q.DeleteTerminalRunsPastRetention(ctx, 30*24*time.Hour, 90*24*time.Hour)
	require.NoError(t, err)
	require.EqualValues(t, runCount-
		5000,
		deleted)

	for _, table := range []string{"job_runs", "job_run_state", "job_run_active_claims", "job_run_lifecycle_events", "job_run_ready_events", "job_retries", "job_run_priority_events", "job_run_visibility_events", "job_run_cache_versions", "job_run_heartbeats"} {
		require.EqualValues(t, 0, countRunRowsByPrefix(t,
			ctx, table,
			prefix))

	}
}

func seedRetentionSideRows(t *testing.T, ctx context.Context, runIDs ...string) {
	t.Helper()

	for _, runID := range runIDs {
		if _, err := testDB.Pool.Exec(ctx, `
			INSERT INTO job_run_active_claims (run_id, ready_generation, attempt)
			VALUES ($1, 0, 1)
			ON CONFLICT DO NOTHING`, runID); err != nil {
			require.Failf(t, "test failure",

				"seed active claim for %s: %v", runID, err)
		}
		if _, err := testDB.Pool.Exec(ctx, `
			INSERT INTO job_run_lifecycle_events (run_id, from_status, to_status, attempt, fields)
			VALUES ($1, 'queued', 'executing', 1, '{"source":"retention-test"}'::jsonb)`, runID); err != nil {
			require.Failf(t, "test failure",

				"seed lifecycle event for %s: %v", runID, err)
		}
		if _, err := testDB.Pool.Exec(ctx, `
			INSERT INTO job_run_ready_events (run_id, ready_generation, attempt, reason)
			VALUES ($1, 0, 1, 'retention_test')
			ON CONFLICT DO NOTHING`, runID); err != nil {
			require.Failf(t, "test failure",

				"seed ready event for %s: %v", runID, err)
		}
		if _, err := testDB.Pool.Exec(ctx, `
			INSERT INTO job_retries (run_id, next_retry_at, attempt, scheduled_at, cleared)
			VALUES ($1, NOW() + INTERVAL '1 minute', 2, NOW(), FALSE)`, runID); err != nil {
			require.Failf(t, "test failure",

				"seed retry for %s: %v", runID, err)
		}
		if _, err := testDB.Pool.Exec(ctx, `
			INSERT INTO job_run_priority_events (run_id, priority)
			VALUES ($1, 10)`, runID); err != nil {
			require.Failf(t, "test failure",

				"seed priority event for %s: %v", runID, err)
		}
		if _, err := testDB.Pool.Exec(ctx, `
			INSERT INTO job_run_visibility_events (run_id, visible_until)
			VALUES ($1, NOW() + INTERVAL '1 hour')`, runID); err != nil {
			require.Failf(t, "test failure",

				"seed visibility event for %s: %v", runID, err)
		}
		if _, err := testDB.Pool.Exec(ctx, `
			INSERT INTO job_run_cache_versions (run_id, cache_version)
			VALUES ($1, 2)`, runID); err != nil {
			require.Failf(t, "test failure",

				"seed cache version for %s: %v", runID, err)
		}
		if _, err := testDB.Pool.Exec(ctx, `
			INSERT INTO job_run_heartbeats (run_id, heartbeat_at)
			VALUES ($1, NOW())
			ON CONFLICT DO NOTHING`, runID); err != nil {
			require.Failf(t, "test failure",

				"seed heartbeat for %s: %v", runID, err)
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
		require.EqualValues(t, 0, countRunSideTableRows(
			t, ctx, table,
			runID))

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
		require.NotEqual(t, 0,
			countRunSideTableRows(t, ctx,
				table, runID))

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
		require.Failf(t, "test failure", "unknown side table %q", table)
	}

	var count int64
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		query, runID,
	).Scan(&count))

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
		require.Failf(t, "test failure", "unknown run table %q", table)
	}

	var count int64
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		query, prefix,
	).Scan(&count))

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
	require.NoError(t, q.InsertEvent(
		ctx, event,
	))
	require.False(t, event.
		CreatedAt.
		IsZero())

	events, err := q.ListEvents(ctx, run.ID, 10000, nil)
	require.NoError(t, err)
	require.Len(t, events,
		1,
	)

	got := events[0]
	require.False(t, got.ID !=
		event.
			ID || got.
		RunID != event.
		RunID || got.
		Type != event.
		Type ||
		got.Level !=
			event.Level ||
		got.
			Message !=
			event.Message,
	)
	require.True(t, jsonEqual(got.Data,
		event.
			Data))

}

func TestSDKActiveRunMutationsRequireActiveAttempt(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-sdk-active-mutations")
	activeRun := mustCreateRun(t, ctx, q, job)
	require.NoError(t, q.UpdateRunStatus(ctx,
		activeRun.ID,
		domain.StatusQueued,
		domain.
			StatusDequeued,

		nil,
	))
	require.NoError(t, q.UpdateRunStatus(ctx,
		activeRun.ID,
		domain.StatusDequeued,
		domain.
			StatusExecuting,

		nil))

	event := &domain.RunEvent{RunID: activeRun.ID, Type: domain.EventLog, Level: "info", Message: "active event", Data: []byte(`{"ok":true}`)}
	require.NoError(t, q.InsertEventForActiveRun(ctx, event,
		activeRun.Attempt,
	))
	require.False(t, event.
		CreatedAt.
		IsZero())
	require.NoError(t, q.UpdateRunMetadataForActiveRun(ctx,
		activeRun.ID,
		map[string]string{"sdk": "active"}, activeRun.
			Attempt))
	require.NoError(t, q.UpdateRunMetadataForActiveRun(ctx,
		activeRun.ID,
		map[string]string{"sdk": "active-v2",
			"phase": "two"},
		activeRun.
			Attempt))

	var metadataEventsBeforeNoop int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT COUNT(*)
		FROM job_run_lifecycle_events
		WHERE run_id = $1
		  AND fields ? 'metadata'`,

		activeRun.
			ID).Scan(&metadataEventsBeforeNoop))

	var cacheVersionsBeforeNoop int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT COUNT(*)
		FROM job_run_cache_versions
		WHERE run_id = $1`,

		activeRun.
			ID).Scan(&cacheVersionsBeforeNoop))
	require.NoError(t, q.UpdateRunMetadataForActiveRun(ctx,
		activeRun.ID,
		map[string]string{"sdk": "active-v2",
			"phase": "two"},
		activeRun.
			Attempt))

	var metadataEventsAfterNoop int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT COUNT(*)
		FROM job_run_lifecycle_events
		WHERE run_id = $1
		  AND fields ? 'metadata'`,

		activeRun.
			ID).Scan(&metadataEventsAfterNoop))
	require.Equal(t, metadataEventsBeforeNoop,

		metadataEventsAfterNoop,
	)

	var cacheVersionsAfterNoop int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT COUNT(*)
		FROM job_run_cache_versions
		WHERE run_id = $1`,

		activeRun.
			ID).Scan(&cacheVersionsAfterNoop))
	require.Equal(t, cacheVersionsBeforeNoop,

		cacheVersionsAfterNoop,
	)
	require.NoError(t, q.UpdateHeartbeatForActiveRun(ctx,
		activeRun.ID, activeRun.
			Attempt,
	))

	checkpoint := &domain.RunCheckpoint{RunID: activeRun.ID, Source: "sdk", State: json.RawMessage(`{"cursor":1}`)}
	require.NoError(t, q.CreateRunCheckpointForActiveRun(
		ctx, checkpoint,
		activeRun.
			Attempt,
	))
	require.EqualValues(t, 1, checkpoint.
		Sequence,
	)

	state := &domain.RunState{RunID: activeRun.ID, StateKey: "cursor", Value: json.RawMessage(`{"step":1}`)}
	require.NoError(t, q.UpsertRunStateForActiveRun(ctx,
		state, activeRun.
			Attempt))

	initialStateUpdatedAt := state.UpdatedAt
	var stateXminBeforeNoop string
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT xmin::text
		FROM run_state
		WHERE run_id = $1 AND state_key = $2`,

		activeRun.
			ID, "cursor",
	).Scan(&stateXminBeforeNoop))

	sameState := &domain.RunState{RunID: activeRun.ID, StateKey: "cursor", Value: json.RawMessage(`{"step":1}`)}
	require.NoError(t, q.UpsertRunStateForActiveRun(ctx,
		sameState, activeRun.
			Attempt,
	))

	var stateXminAfterNoop string
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT xmin::text
		FROM run_state
		WHERE run_id = $1 AND state_key = $2`,

		activeRun.
			ID, "cursor",
	).Scan(&stateXminAfterNoop))
	require.Equal(t, stateXminBeforeNoop,

		stateXminAfterNoop,
	)
	require.True(t, sameState.
		UpdatedAt.
		Equal(
			initialStateUpdatedAt,
		))

	output := &domain.RunOutput{RunID: activeRun.ID, OutputKey: "final", Value: json.RawMessage(`{"ok":true}`)}
	require.NoError(t, q.UpsertRunOutputForActiveRun(ctx,
		output, activeRun.
			Attempt),
	)

	initialOutputID := output.ID
	initialOutputCreatedAt := output.CreatedAt
	var outputXminBeforeNoop string
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT xmin::text
		FROM run_outputs
		WHERE run_id = $1 AND output_key = $2`,

		activeRun.
			ID, "final",
	).Scan(&outputXminBeforeNoop))

	sameOutput := &domain.RunOutput{RunID: activeRun.ID, OutputKey: "final", Value: json.RawMessage(`{"ok":true}`)}
	require.NoError(t, q.UpsertRunOutputForActiveRun(ctx,
		sameOutput, activeRun.
			Attempt,
	))

	var outputXminAfterNoop string
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT xmin::text
		FROM run_outputs
		WHERE run_id = $1 AND output_key = $2`,

		activeRun.
			ID, "final",
	).Scan(&outputXminAfterNoop))
	require.Equal(t, outputXminBeforeNoop,

		outputXminAfterNoop,
	)
	require.Equal(t, initialOutputID,

		sameOutput.
			ID)
	require.True(t, sameOutput.
		CreatedAt.
		Equal(initialOutputCreatedAt))

	resourceSnapshot := &domain.RunResourceSnapshot{RunID: activeRun.ID, CPUPercent: 10, MemoryMB: 128}
	require.NoError(t, q.CreateRunResourceSnapshotForActiveRun(ctx, resourceSnapshot,

		activeRun.
			Attempt,
	))

	iteration := &domain.RunIteration{RunID: activeRun.ID, Iteration: 1, Description: "active iteration"}
	require.NoError(t, q.CreateRunIterationForActiveRun(ctx,
		iteration, activeRun.
			Attempt,
	))

	memory := &domain.JobMemory{JobID: activeRun.JobID, ProjectID: activeRun.ProjectID, MemoryKey: "cursor", Value: json.RawMessage(`{"step":1}`), SizeBytes: len(`{"step":1}`)}
	require.NoError(t, q.UpsertJobMemoryWithQuotaForActiveRun(ctx, activeRun.
		ID, memory,
		1024,
		1024,
		activeRun.
			Attempt,
	))

	var memoryXminBeforeNoop string
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT xmin::text
		FROM job_memory
		WHERE job_id = $1 AND memory_key = $2`,

		activeRun.
			JobID,
		"cursor",
	).
		Scan(&memoryXminBeforeNoop))

	sameMemory := &domain.JobMemory{JobID: activeRun.JobID, ProjectID: activeRun.ProjectID, MemoryKey: "cursor", Value: json.RawMessage(`{"step":1}`), SizeBytes: len(`{"step":1}`)}
	require.NoError(t, q.UpsertJobMemoryWithQuotaForActiveRun(ctx, activeRun.
		ID, sameMemory,

		1024,
		1024, activeRun.
			Attempt,
	))

	var memoryXminAfterNoop string
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT xmin::text
		FROM job_memory
		WHERE job_id = $1 AND memory_key = $2`,

		activeRun.
			JobID,
		"cursor",
	).
		Scan(&memoryXminAfterNoop))
	require.Equal(t, memoryXminBeforeNoop,

		memoryXminAfterNoop,
	)
	require.NoError(t, q.DeleteRunStateForActiveRun(ctx,
		activeRun.ID, "cursor",
		activeRun.
			Attempt,
	))
	require.NoError(t, q.DeleteJobMemoryForActiveRun(ctx,
		activeRun.ID, activeRun.
			JobID,
		"cursor",

		activeRun.
			Attempt))

	var ledgerHasSDK bool
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT metadata ? 'sdk' FROM job_runs WHERE id = $1`,

		activeRun.
			ID).
		Scan(&ledgerHasSDK),
	)
	require.False(t, ledgerHasSDK)

	stored, err := q.GetRun(ctx, activeRun.ID)
	require.NoError(t, err)
	require.False(t, stored.
		Metadata["sdk"] !=
		"active-v2" ||
		stored.Metadata["phase"] != "two",
	)

	cachedRun, _, err := q.GetRunWithCacheVersion(ctx, activeRun.ID)
	require.NoError(t, err)
	require.False(t, cachedRun.
		Metadata["sdk"] !=
		"active-v2" ||
		cachedRun.
			Metadata["phase"] != "two",
	)

	byIDs, err := q.GetRunsByIDs(ctx, []string{activeRun.ID})
	require.NoError(t, err)
	require.False(t, byIDs[activeRun.
		ID].Metadata["sdk"] !=
		"active-v2" ||
		byIDs[activeRun.
			ID].Metadata["phase"] != "two",
	)

	metadataKey := "sdk"
	metadataValue := "active-v2"
	listed, err := q.ListRunsByProject(ctx, activeRun.ProjectID, nil, &metadataKey, &metadataValue, nil, nil, nil, nil, nil, 10, nil)
	require.NoError(t, err)
	require.False(t, len(listed) != 1 ||
		listed[0].ID !=
			activeRun.ID)

	filtered, err := q.ListRunsByProjectFiltered(ctx, activeRun.ProjectID, nil, nil, "", "", nil, &metadataKey, &metadataValue, nil, nil, nil, nil, nil, 10, nil)
	require.NoError(t, err)
	require.False(t, len(filtered) !=
		1 || filtered[0].ID !=
		activeRun.ID)
	require.NotNil(t, stored.
		HeartbeatAt,
	)

	if err := q.InsertEventForActiveRun(ctx, &domain.RunEvent{RunID: activeRun.ID, Type: domain.EventLog, Message: "stale"}, activeRun.Attempt+1); !errors.Is(err, store.ErrRunConflict) {
		require.Failf(t, "test failure",

			"InsertEventForActiveRun(stale attempt) error = %v, want ErrRunConflict", err)
	}
	if err := q.EnsureRunActiveForAttempt(ctx, activeRun.ID, activeRun.Attempt+1); !errors.Is(err, store.ErrRunConflict) {
		require.Failf(t, "test failure",

			"EnsureRunActiveForAttempt(stale attempt) error = %v, want ErrRunConflict", err)
	}
	if err := q.CreateRunResourceSnapshotForActiveRun(ctx, &domain.RunResourceSnapshot{RunID: activeRun.ID}, activeRun.Attempt+1); !errors.Is(err, store.ErrRunConflict) {
		require.Failf(t, "test failure",

			"CreateRunResourceSnapshotForActiveRun(stale attempt) error = %v, want ErrRunConflict", err)
	}
	if err := q.CreateRunIterationForActiveRun(ctx, &domain.RunIteration{RunID: activeRun.ID, Iteration: 2}, activeRun.Attempt+1); !errors.Is(err, store.ErrRunConflict) {
		require.Failf(t, "test failure",

			"CreateRunIterationForActiveRun(stale attempt) error = %v, want ErrRunConflict", err)
	}
	if err := q.UpsertJobMemoryWithQuotaForActiveRun(ctx, activeRun.ID, &domain.JobMemory{JobID: activeRun.JobID, ProjectID: activeRun.ProjectID, MemoryKey: "stale", Value: json.RawMessage(`true`), SizeBytes: 4}, 1024, 1024, activeRun.Attempt+1); !errors.Is(err, store.ErrRunConflict) {
		require.Failf(t, "test failure",

			"UpsertJobMemoryWithQuotaForActiveRun(stale attempt) error = %v, want ErrRunConflict", err)
	}

	terminalRun := mustCreateRun(t, ctx, q, job)
	require.NoError(t, q.UpdateRunStatus(ctx,
		terminalRun.
			ID, domain.StatusQueued,
		domain.
			StatusDequeued,

		nil,
	))
	require.NoError(t, q.UpdateRunStatus(ctx,
		terminalRun.
			ID, domain.StatusDequeued,

		domain.
			StatusExecuting,

		nil))
	require.NoError(t, q.UpdateRunStatus(ctx,
		terminalRun.
			ID, domain.StatusExecuting,

		domain.
			StatusCompleted,

		map[string]any{"finished_at": time.Now()}))

	for _, err := range map[string]error{
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
		require.True(t, errors.Is(err, store.
			ErrRunConflict,
		))

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
		require.NoError(t, q.InsertEvent(
			ctx, event,
		))

	}

	events, err := q.ListEvents(ctx, run.ID, 10000, nil)
	require.NoError(t, err)
	require.Len(t, events,
		3,
	)

	for _, event := range events {
		require.Equal(t, run.ID,

			event.RunID,
		)

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
	require.NoError(t, q.CreateRun(ctx,
		queued,
	))

	executing := baseRun(job, newID())
	executing.Status = domain.StatusExecuting
	require.NoError(t, q.CreateRun(ctx,
		executing,
	))

	delayed := baseRun(job, newID())
	delayed.Status = domain.StatusDelayed
	require.NoError(t, q.CreateRun(ctx,
		delayed,
	))

	failed := baseRun(job, newID())
	failed.Status = domain.StatusFailed
	require.NoError(t, q.CreateRun(ctx,
		failed,
	))

	stats, err := q.QueueStats(ctx)
	require.NoError(t, err)
	require.False(t, stats.
		Queued !=
		1 || stats.
		Executing !=
		1 || stats.Delayed !=
		1,
	)

	require.NoError(t, q.UpdateRunStatus(ctx,
		queued.ID,
		domain.StatusQueued,
		domain.StatusDequeued,
		nil,
	))
	require.NoError(t, q.UpdateRunStatus(ctx,
		queued.ID,
		domain.StatusDequeued,
		domain.StatusExecuting,
		nil,
	))
	require.NoError(t, q.UpdateRunStatus(ctx,
		queued.ID,
		domain.StatusExecuting,
		domain.StatusCompleted,
		nil,
	))

	stats, err = q.QueueStats(ctx)
	require.NoError(t, err)
	require.False(t, stats.
		Queued !=
		0 || stats.
		Executing !=
		1 || stats.Delayed !=
		1,
	)
}

func mustStore(t *testing.T) *store.Queries {
	t.Helper()
	require.False(t, testDB ==
		nil ||
		testDB.Pool ==
			nil)

	return store.New(testDB.Pool)
}

func mustClean(t *testing.T, ctx context.Context) {
	t.Helper()
	require.NoError(t, testDB.
		CleanTables(ctx),
	)

}

func TestRunCheckpoints(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-run-checkpoints")
	run := mustCreateRun(t, ctx, q, job)

	cp1 := &domain.RunCheckpoint{RunID: run.ID, Source: "sdk", State: json.RawMessage(`{"step":1}`)}
	require.NoError(t, q.CreateRunCheckpoint(ctx,
		cp1))

	cp2 := &domain.RunCheckpoint{RunID: run.ID, Source: "auto", State: json.RawMessage(`{"step":2}`)}
	require.NoError(t, q.CreateRunCheckpoint(ctx,
		cp2))

	checkpoints, err := q.ListRunCheckpoints(ctx, run.ID, 10, nil)
	require.NoError(t, err)
	require.Len(t, checkpoints,

		2)
	require.False(t, checkpoints[0].Sequence <=
		checkpoints[1].Sequence)

}

func TestRunCheckpointsConcurrentSequenceAllocation(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-run-checkpoints-concurrent")
	run := mustCreateRun(t, ctx, q, job)
	require.NoError(t, q.UpdateRunStatus(ctx,
		run.ID, domain.
			StatusQueued,
		domain.StatusDequeued,

		nil))
	require.NoError(t, q.UpdateRunStatus(ctx,
		run.ID, domain.
			StatusDequeued,
		domain.
			StatusExecuting,

		nil))

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
		require.NoError(t, err)

	}

	checkpoints, err := q.ListRunCheckpoints(ctx, run.ID, checkpointCount, nil)
	require.NoError(t, err)
	require.Len(t, checkpoints,

		checkpointCount,
	)

	seen := make(map[int]bool, checkpointCount)
	for _, checkpoint := range checkpoints {
		seen[checkpoint.Sequence] = true
	}
	for sequence := 1; sequence <= checkpointCount; sequence++ {
		require.True(t, seen[sequence])

	}
}

func TestRunOutputsRemainActive(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-run-output")
	run := mustCreateRun(t, ctx, q, job)

	out := &domain.RunOutput{RunID: run.ID, OutputKey: "final", Schema: json.RawMessage(`{"type":"object"}`), Value: json.RawMessage(`{"name":"leo"}`)}
	require.NoError(t, q.UpsertRunOutput(ctx,
		out))

	initialOutputID := out.ID
	initialOutputCreatedAt := out.CreatedAt
	var outputXminBeforeNoop string
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT xmin::text
		FROM run_outputs
		WHERE run_id = $1 AND output_key = $2`,

		run.ID,
		"final").
		Scan(&outputXminBeforeNoop))

	sameOut := &domain.RunOutput{RunID: run.ID, OutputKey: "final", Schema: json.RawMessage(`{"type":"object"}`), Value: json.RawMessage(`{"name":"leo"}`)}
	require.NoError(t, q.UpsertRunOutput(ctx,
		sameOut))

	var outputXminAfterNoop string
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT xmin::text
		FROM run_outputs
		WHERE run_id = $1 AND output_key = $2`,

		run.ID,
		"final").
		Scan(&outputXminAfterNoop))
	require.Equal(t, outputXminBeforeNoop,

		outputXminAfterNoop,
	)
	require.Equal(t, initialOutputID,

		sameOut.
			ID)
	require.True(t, sameOut.
		CreatedAt.
		Equal(initialOutputCreatedAt))

	out.Value = json.RawMessage(`{"name":"leo2"}`)
	require.NoError(t, q.UpsertRunOutput(ctx,
		out))

	outputs, err := q.ListRunOutputs(ctx, run.ID, 10000, nil)
	require.NoError(t, err)
	require.Len(t, outputs,

		1)
	require.True(t, jsonEqual(outputs[0].Value,
		json.RawMessage(`{"name":"leo2"}`)))

}

func TestUpdateRunMetadata(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-run-metadata")
	run := mustCreateRun(t, ctx, q, job)
	require.NoError(t, q.UpdateRunMetadata(ctx,
		run.ID, map[string]string{
			"env":  "prod",
			"team": "core"}))

	stored, err := q.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.False(t, stored.
		Metadata["env"] !=
		"prod" ||
		stored.Metadata["team"] !=
			"core")
	require.NoError(t, q.UpdateRunMetadata(ctx,
		run.ID, map[string]string{
			"team": "infra",

			"region": "eu"}),
	)

	stored, err = q.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.False(t, stored.
		Metadata["env"] !=
		"prod" ||
		stored.Metadata["team"] !=
			"infra" ||
		stored.
			Metadata["region"] != "eu")

	var beforeNoopXmin string
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT xmin::text FROM job_runs WHERE id = $1`,

		run.ID).Scan(&beforeNoopXmin))
	require.NoError(t, q.UpdateRunMetadata(ctx,
		run.ID, map[string]string{
			"team": "infra",

			"region": "eu"}),
	)

	var afterNoopXmin string
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT xmin::text FROM job_runs WHERE id = $1`,

		run.ID).Scan(&afterNoopXmin))
	require.Equal(t, beforeNoopXmin,

		afterNoopXmin,
	)

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
	require.NoError(t, q.CreateRun(ctx,
		child),
	)

	allTerminal, err := q.AreAllDescendantsTerminal(ctx, parent.ID)
	require.NoError(t, err)
	require.False(t, allTerminal)
	require.NoError(t, q.UpdateRunStatus(ctx,
		child.ID, domain.
			StatusExecuting,
		domain.
			StatusCompleted,

		map[string]any{"finished_at": time.
			Now()}))

	allTerminal, err = q.AreAllDescendantsTerminal(ctx, parent.ID)
	require.NoError(t, err)
	require.True(t, allTerminal)

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
	require.NoError(t, q.CreateJobSecret(ctx,
		globalSecret,
	))
	require.NotEqual(t, "",

		globalSecret.
			ID)
	require.EqualValues(t, 1, globalSecret.
		KeyVersion,
	)
	require.False(t, globalSecret.
		CreatedAt.
		IsZero() || globalSecret.
		UpdatedAt.
		IsZero())

	jobSecret := &domain.JobSecret{
		ProjectID:      projectID,
		JobID:          job.ID,
		Environment:    "prod",
		SecretKey:      "API_TOKEN",
		EncryptedValue: "job-value",
	}
	require.NoError(t, q.CreateJobSecret(ctx,
		jobSecret))

	dupSecret := &domain.JobSecret{
		ProjectID:      projectID,
		JobID:          job.ID,
		Environment:    "prod",
		SecretKey:      "API_TOKEN",
		EncryptedValue: "duplicate",
	}
	require.Error(t, q.CreateJobSecret(ctx, dupSecret))

	gotJobSecret, err := q.GetJobSecret(ctx, jobSecret.ID, jobSecret.ProjectID)
	require.NoError(t, err)
	require.False(t, gotJobSecret.
		ID !=
		jobSecret.
			ID || gotJobSecret.
		ProjectID !=
		projectID ||
		gotJobSecret.
			JobID != job.
			ID || gotJobSecret.
		SecretKey !=
		"API_TOKEN",
	)
	require.Equal(t, "job-value",

		gotJobSecret.
			Value,
	)

	_, err = q.GetJobSecret(ctx, newID(), projectID)
	require.True(t, errors.Is(err, store.
		ErrJobSecretNotFound,
	))

	allProd, err := q.ListJobSecrets(ctx, projectID, "", "prod", 10000, nil)
	require.NoError(t, err)
	require.Len(t, allProd,

		2)

	jobOnly, err := q.ListJobSecrets(ctx, projectID, job.ID, "prod", 10000, nil)
	require.NoError(t, err)
	require.False(t, len(jobOnly) !=
		1 || jobOnly[0].ID !=
		jobSecret.ID)

	noneForEnv, err := q.ListJobSecrets(ctx, projectID, "", "staging", 10000, nil)
	require.NoError(t, err)
	require.Len(t, noneForEnv,

		0)

	byJob, err := q.ListJobSecretsByJob(ctx, job.ID, "prod")
	require.NoError(t, err)
	require.Len(t, byJob, 1)
	require.Equal(t, jobSecret.
		ID, byJob[0].ID,
	)

	byJobNone, err := q.ListJobSecretsByJob(ctx, job.ID, "staging")
	require.NoError(t, err)
	require.Len(t, byJobNone,

		0)
	require.NoError(t, q.DeleteJobSecret(ctx,
		jobSecret.ID,
		jobSecret.ProjectID,
	))

	_, err = q.GetJobSecret(ctx, jobSecret.ID, jobSecret.ProjectID)
	require.True(t, errors.Is(err, store.
		ErrJobSecretNotFound,
	))

	if err := q.DeleteJobSecret(ctx, newID(), projectID); !errors.Is(err, store.ErrJobSecretNotFound) {
		require.Failf(t, "test failure",

			"DeleteJobSecret(not found) error = %v, want ErrJobSecretNotFound", err)
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
	require.NoError(t, oldQ.
		CreateJobSecret(ctx,
			secret))

	newQ := mustStore(t)
	newQ.SetSecretEncryptionKey("new-secret-encryption-key")
	newQ.SetOldSecretEncryptionKeys([]string{"old-secret-encryption-key"})

	got, err := newQ.GetJobSecret(ctx, secret.ID, secret.ProjectID)
	require.NoError(t, err)
	require.Equal(t, "legacy-value",

		got.Value,
	)

	withoutOld := mustStore(t)
	withoutOld.SetSecretEncryptionKey("new-secret-encryption-key")
	if _, err := withoutOld.GetJobSecret(ctx, secret.ID, secret.ProjectID); err == nil {
		require.Fail(t,

			"GetJobSecret(without old key) error = nil, want decrypt failure")
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
	require.NoError(t, q.CreateJobSecret(ctx,
		globalSecret,
	))

	duplicateGlobal := &domain.JobSecret{
		ProjectID:      projectID,
		Environment:    "prod",
		SecretKey:      "SHARED_TOKEN",
		EncryptedValue: "duplicate",
	}
	require.Error(t, q.CreateJobSecret(ctx, duplicateGlobal))

	var count int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT COUNT(*)
		FROM job_secrets
		WHERE project_id = $1 AND job_id IS NULL AND environment = 'prod' AND secret_key = 'SHARED_TOKEN'
	`,

		projectID).Scan(&count))
	require.EqualValues(t, 1, count)

	sameKeyDifferentEnv := &domain.JobSecret{
		ProjectID:      projectID,
		Environment:    "staging",
		SecretKey:      "SHARED_TOKEN",
		EncryptedValue: "staging",
	}
	require.NoError(t, q.CreateJobSecret(ctx,
		sameKeyDifferentEnv,
	))

	otherProjectSecret := &domain.JobSecret{
		ProjectID:      "other-project-" + newID(),
		Environment:    "prod",
		SecretKey:      "SHARED_TOKEN",
		EncryptedValue: "other-project",
	}
	require.NoError(t, q.CreateJobSecret(ctx,
		otherProjectSecret,
	))

	jobScopedSecret := &domain.JobSecret{
		ProjectID:      projectID,
		JobID:          job.ID,
		Environment:    "prod",
		SecretKey:      "SHARED_TOKEN",
		EncryptedValue: "job-scoped",
	}
	require.NoError(t, q.CreateJobSecret(ctx,
		jobScopedSecret,
	))

}

func TestSecret_ListJobSecretsByJobUsesJobEnvironment(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	q.SetSecretEncryptionKey("test-encryption-key-32bytes!!!!")
	mustClean(t, ctx)

	projectID := "project-secret-env-" + newID()
	prod := &domain.Environment{ProjectID: projectID, Name: "Production", Slug: "production"}
	require.NoError(t, q.CreateEnvironment(ctx,
		prod))

	staging := &domain.Environment{ProjectID: projectID, Name: "Staging", Slug: "staging"}
	require.NoError(t, q.CreateEnvironment(ctx,
		staging))

	job := mustCreateJob(t, ctx, q, projectID)
	job.EnvironmentID = staging.ID
	require.NoError(t, q.UpdateJob(ctx,
		job))

	prodSecret := &domain.JobSecret{ProjectID: projectID, JobID: job.ID, Environment: prod.ID, SecretKey: "TOKEN", EncryptedValue: "prod"}
	require.Error(t, q.CreateJobSecret(ctx, prodSecret))

	stagingGlobal := &domain.JobSecret{ProjectID: projectID, Environment: staging.ID, SecretKey: "GLOBAL_TOKEN", EncryptedValue: "staging-global"}
	require.NoError(t, q.CreateJobSecret(ctx,
		stagingGlobal,
	))

	stagingSecret := &domain.JobSecret{ProjectID: projectID, JobID: job.ID, Environment: staging.ID, SecretKey: "JOB_TOKEN", EncryptedValue: "staging-job"}
	require.NoError(t, q.CreateJobSecret(ctx,
		stagingSecret,
	))

	got, err := q.ListJobSecretsByJob(ctx, job.ID, prod.ID)
	require.NoError(t, err)
	require.Len(t, got, 1)

	for _, secret := range got {
		require.Equal(t, staging.
			ID, secret.
			Environment,
		)
		require.Equal(t, job.ID,

			secret.JobID,
		)

	}
}

func TestSecret_ListJobSecretsByJobDefaultsEnvlessJobToProduction(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	q.SetSecretEncryptionKey("test-encryption-key-32bytes!!!!")
	mustClean(t, ctx)

	projectID := "project-secret-envless-" + newID()
	prod := &domain.Environment{ProjectID: projectID, Name: "Production", Slug: "production"}
	require.NoError(t, q.CreateEnvironment(ctx,
		prod))

	job := mustCreateJob(t, ctx, q, projectID)
	require.Equal(t, "", job.
		EnvironmentID,
	)

	productionSecret := &domain.JobSecret{ProjectID: projectID, Environment: prod.ID, SecretKey: "TOKEN", EncryptedValue: "production"}
	require.NoError(t, q.CreateJobSecret(ctx,
		productionSecret,
	))

	got, err := q.ListJobSecretsByJob(ctx, job.ID, "")
	require.NoError(t, err)
	require.Len(t, got, 0)

}

func TestSecret_ListJobSecretsByJobExcludesEnvironmentWideSecrets(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	q.SetSecretEncryptionKey("test-encryption-key-32bytes!!!!")
	mustClean(t, ctx)

	projectID := "project-secret-precedence-" + newID()
	prod := &domain.Environment{ProjectID: projectID, Name: "Production", Slug: "production"}
	require.NoError(t, q.CreateEnvironment(ctx,
		prod))

	job := mustCreateJob(t, ctx, q, projectID)

	jobSecret := &domain.JobSecret{ProjectID: projectID, JobID: job.ID, Environment: prod.ID, SecretKey: "TOKEN", EncryptedValue: "job-specific"}
	require.NoError(t, q.CreateJobSecret(ctx,
		jobSecret))

	globalSecret := &domain.JobSecret{ProjectID: projectID, Environment: prod.ID, SecretKey: "TOKEN", EncryptedValue: "global"}
	require.NoError(t, q.CreateJobSecret(ctx,
		globalSecret,
	))

	got, err := q.ListJobSecretsByJob(ctx, job.ID, prod.ID)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, jobSecret.
		ID, got[0].ID)

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
	require.NoError(t, q.CreateAPIKey(ctx, key1))
	require.NotEqual(t, "",

		key1.ID)
	require.False(t, key1.CreatedAt.
		IsZero())

	key2 := &domain.APIKey{
		ProjectID: projectID,
		Name:      "secondary",
		KeyHash:   "hash-" + newID(),
		KeyPrefix: "sk_test_def",
		Scopes:    []string{"jobs:read"},
	}
	require.NoError(t, q.CreateAPIKey(ctx, key2))

	otherProjectKey := &domain.APIKey{
		ProjectID: "project-api-key-other",
		Name:      "other",
		KeyHash:   "hash-" + newID(),
		KeyPrefix: "sk_test_xyz",
		Scopes:    []string{},
	}
	require.NoError(t, q.CreateAPIKey(ctx, otherProjectKey))

	dup := &domain.APIKey{
		ProjectID: projectID,
		Name:      "duplicate-hash",
		KeyHash:   key1.KeyHash,
		KeyPrefix: "sk_test_dup",
	}
	require.Error(t, q.CreateAPIKey(ctx,
		dup))

	got, err := q.GetAPIKeyByHash(ctx, key1.KeyHash)
	require.NoError(t, err)
	require.False(t, got.ID !=
		key1.ID ||
		got.
			ProjectID !=
			key1.ProjectID ||
		got.Name !=
			key1.
				Name ||
		got.KeyHash !=
			key1.
				KeyHash,
	)
	require.False(t, got.ExpiresAt ==
		nil || !got.ExpiresAt.
		Equal(*key1.ExpiresAt))

	_, err = q.GetAPIKeyByHash(ctx, "missing-hash")
	require.Error(t, err)

	keys, err := q.ListAPIKeysByProject(ctx, projectID, 10000, nil)
	require.NoError(t, err)
	require.Len(t, keys, 2)
	require.False(t, keys[0].
		ID != key2.
		ID ||
		keys[1].ID !=
			key1.ID)

	none, err := q.ListAPIKeysByProject(ctx, "project-api-key-none", 10000, nil)
	require.NoError(t, err)
	require.Len(t, none, 0)
	require.NoError(t, q.RevokeAPIKey(ctx, key1.
		ID))

	revoked, err := q.GetAPIKeyByHash(ctx, key1.KeyHash)
	require.NoError(t, err)
	require.NotNil(t, revoked.
		RevokedAt,
	)

	keys, err = q.ListAPIKeysByProject(ctx, projectID, 10000, nil)
	require.NoError(t, err)
	require.False(t, len(keys) != 1 ||
		keys[0].
			ID != key2.
			ID)
	require.Error(t, q.RevokeAPIKey(ctx,
		key1.
			ID))

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
	require.NoError(t, q.CreateAPIKey(ctx, key))

	storedBefore, err := q.GetAPIKeyByHash(ctx, key.KeyHash)
	require.NoError(t, err)
	require.Nil(t, storedBefore.
		LastUsedAt,
	)
	require.NoError(t, q.TouchAPIKeyLastUsed(ctx,
		key.ID),
	)

	storedAfter, err := q.GetAPIKeyByHash(ctx, key.KeyHash)
	require.NoError(t, err)
	require.NotNil(t, storedAfter.
		LastUsedAt,
	)
	require.NoError(t, q.TouchAPIKeyLastUsed(ctx,
		newID(),
	))

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
	require.NoError(t, q.CreateJobVersion(ctx,
		v1))
	require.False(t, v1.CreatedAt.
		IsZero())

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
	require.NoError(t, q.CreateJobVersion(ctx,
		v2))

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
	require.Error(t, q.CreateJobVersion(ctx, vDup))

	versions, err := q.ListJobVersionsByJob(ctx, job.ID, 10000, nil)
	require.NoError(t, err)
	require.Len(t, versions,

		2)
	require.False(t, versions[0].Version !=
		2 ||
		versions[1].Version != 1)
	require.Equal(t, v2.FallbackEndpointURL,

		versions[0].
			FallbackEndpointURL,
	)

	empty, err := q.ListJobVersionsByJob(ctx, newID(), 100, nil)
	require.NoError(t, err)
	require.Len(t, empty, 0)

	gotV1, err := q.GetJobVersion(ctx, job.ID, 1)
	require.NoError(t, err)
	require.False(t, gotV1.
		ID !=
		v1.ID ||
		gotV1.
			Name != v1.
			Name || gotV1.EndpointURL !=
		v1.
			EndpointURL,
	)
	require.True(t, jsonEqual(gotV1.PayloadSchema,

		v1.PayloadSchema,
	))
	require.Len(t, gotV1.Tags,

		len(v1.
			Tags))

	for k, want := range v1.Tags {
		require.Equal(t, want,
			gotV1.
				Tags[k])

	}

	_, err = q.GetJobVersion(ctx, job.ID, 99)
	require.Error(t, err)

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
	require.NoError(t, q.CreateWebhookDelivery(ctx, delivery1))
	require.NotEqual(t, "",

		delivery1.
			ID)
	require.False(t, delivery1.
		CreatedAt.
		IsZero() || delivery1.
		UpdatedAt.IsZero())

	delivery2 := &domain.WebhookDelivery{
		RunID:       run.ID,
		JobID:       job.ID,
		WebhookURL:  "https://example.com/webhook/2",
		Status:      "failed",
		Attempts:    3,
		MaxAttempts: 3,
		LastError:   "timeout",
	}
	require.NoError(t, q.CreateWebhookDelivery(ctx, delivery2))

	dupID := &domain.WebhookDelivery{
		ID:          delivery1.ID,
		RunID:       run.ID,
		JobID:       job.ID,
		WebhookURL:  "https://example.com/webhook/dup",
		Status:      "pending",
		Attempts:    0,
		MaxAttempts: 3,
	}
	require.Error(t, q.CreateWebhookDelivery(ctx,
		dupID))

	got, err := q.GetWebhookDelivery(ctx, delivery1.ID)
	require.NoError(t, err)
	require.False(t, got.ID !=
		delivery1.
			ID ||
		got.RunID !=
			run.ID || got.
		JobID != job.
		ID ||
		got.
			Status !=
			"pending")

	_, err = q.GetWebhookDelivery(ctx, newID())
	require.Error(t, err)

	deliveredAt := time.Now().UTC()
	newStatusCode := 200
	delivery1.Status = "delivered"
	delivery1.Attempts = 2
	delivery1.LastStatusCode = &newStatusCode
	delivery1.LastError = ""
	delivery1.NextRetryAt = nil
	delivery1.DeliveredAt = &deliveredAt
	require.NoError(t, q.UpdateWebhookDelivery(ctx, delivery1))
	require.False(t, delivery1.
		UpdatedAt.
		IsZero())

	got, err = q.GetWebhookDelivery(ctx, delivery1.ID)
	require.NoError(t, err)
	require.False(t, got.Status !=
		"delivered" ||
		got.Attempts !=
			2)
	require.NotNil(t, got.DeliveredAt)
	require.Nil(t, got.
		NextRetryAt,
	)

	missingDelivery := &domain.WebhookDelivery{
		ID:             newID(),
		Status:         "pending",
		Attempts:       1,
		LastStatusCode: &newStatusCode,
	}
	require.Error(t, q.UpdateWebhookDelivery(ctx,
		missingDelivery,
	))

	all, err := q.ListWebhookDeliveries(ctx, job.ProjectID, "", 10, nil)
	require.NoError(t, err)
	require.Len(t, all, 2)
	require.False(t, all[0].
		ID != delivery2.
		ID ||
		all[1].
			ID != delivery1.ID,
	)

	delivered, err := q.ListWebhookDeliveries(ctx, job.ProjectID, "delivered", 10, nil)
	require.NoError(t, err)
	require.False(t, len(delivered) !=
		1 || delivered[0].
		ID != delivery1.ID,
	)

	none, err := q.ListWebhookDeliveries(ctx, job.ProjectID, "pending", 0, nil)
	require.NoError(t, err)
	require.Len(t, none, 0)

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
	require.NoError(t, q.CreateWebhookDelivery(ctx, dueOld))

	dueRecent := &domain.WebhookDelivery{
		RunID:       run.ID,
		JobID:       job.ID,
		WebhookURL:  "https://example.com/webhook/due-recent",
		Status:      "pending",
		Attempts:    2,
		MaxAttempts: 3,
		NextRetryAt: &recent,
	}
	require.NoError(t, q.CreateWebhookDelivery(ctx, dueRecent))

	notDueFuture := &domain.WebhookDelivery{
		RunID:       run.ID,
		JobID:       job.ID,
		WebhookURL:  "https://example.com/webhook/future",
		Status:      "pending",
		Attempts:    1,
		MaxAttempts: 3,
		NextRetryAt: &future,
	}
	require.NoError(t, q.CreateWebhookDelivery(ctx, notDueFuture))

	pendingNoRetry := &domain.WebhookDelivery{
		RunID:       run.ID,
		JobID:       job.ID,
		WebhookURL:  "https://example.com/webhook/no-retry",
		Status:      "pending",
		Attempts:    1,
		MaxAttempts: 3,
	}
	require.NoError(t, q.CreateWebhookDelivery(ctx, pendingNoRetry))

	failedDue := &domain.WebhookDelivery{
		RunID:       run.ID,
		JobID:       job.ID,
		WebhookURL:  "https://example.com/webhook/failed",
		Status:      "failed",
		Attempts:    3,
		MaxAttempts: 3,
		NextRetryAt: &older,
	}
	require.NoError(t, q.CreateWebhookDelivery(ctx, failedDue))

	pending, err := q.ListPendingWebhookRetries(ctx)
	require.NoError(t, err)
	require.Len(t, pending,

		2)
	require.False(t, pending[0].ID !=
		dueOld.ID ||
		pending[1].ID != dueRecent.
			ID)

	now := time.Now().UTC()
	statusCode := 200
	dueOld.Status = "delivered"
	dueOld.DeliveredAt = &now
	dueOld.NextRetryAt = nil
	dueOld.LastStatusCode = &statusCode
	require.NoError(t, q.UpdateWebhookDelivery(ctx, dueOld))

	dueRecent.Status = "delivered"
	dueRecent.DeliveredAt = &now
	dueRecent.NextRetryAt = nil
	dueRecent.LastStatusCode = &statusCode
	require.NoError(t, q.UpdateWebhookDelivery(ctx, dueRecent))

	nonePending, err := q.ListPendingWebhookRetries(ctx)
	require.NoError(t, err)
	require.Len(t, nonePending,

		0)

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
	require.NoError(t, err)
	require.NotEqual(t, "",

		enqueued.
			ID)
	require.Equal(t, run.ID,

		enqueued.
			RunID)
	require.Equal(t, "", enqueued.
		EventTriggerID,
	)
	require.Equal(t, job.WebhookURL,

		enqueued.
			WebhookURL)
	require.Equal(t, job.WebhookSecret,

		enqueued.
			WebhookSecret,
	)
	require.Equal(t, domain.
		WebhookStatusPending,

		enqueued.
			Status)
	require.EqualValues(t, 7, enqueued.
		MaxAttempts,
	)
	require.NotNil(t, enqueued.
		NextRetryAt,
	)

	var payloadRaw []byte
	var payloadSize int
	var eventType string
	var webhookSecret *string
	err = testDB.Pool.QueryRow(ctx, `
		SELECT payload, payload_size_bytes, event_type, webhook_secret
		FROM webhook_deliveries
		WHERE id = $1`, enqueued.ID,
	).Scan(&payloadRaw, &payloadSize, &eventType, &webhookSecret)
	require.NoError(t, err)
	require.Equal(t, len(payloadRaw),
		payloadSize,
	)
	require.Equal(t, "run.completed",

		eventType,
	)
	require.False(t, webhookSecret ==
		nil || *webhookSecret !=
		job.WebhookSecret,
	)

	var payload map[string]any
	require.NoError(t, json.
		Unmarshal(payloadRaw,
			&payload,
		))
	require.Equal(t, run.ID,

		payload["run_id"],
	)
	require.Equal(t, run.JobID,

		payload["job_id"])
	require.Equal(t, run.ProjectID,

		payload["project_id"],
	)
	require.Equal(t, string(
		run.Status,
	), payload["status"])

	evtID := newID()
	expiresAt := time.Now().UTC().Add(time.Hour)
	_, err = testDB.Pool.Exec(ctx, `
		INSERT INTO event_triggers (id, event_key, project_id, source_type, expires_at)
		VALUES ($1, $2, $3, 'webhook', $4)`,
		evtID, "evt-key-"+evtID, job.ProjectID, expiresAt,
	)
	require.NoError(t, err)

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
	require.NoError(t, q.CreateWebhookDelivery(ctx, eventDelivery))

	pendingRun, err := q.ListPendingRunWebhookDeliveries(ctx)
	require.NoError(t, err)
	require.Len(t, pendingRun,

		1)
	require.Equal(t, enqueued.
		ID, pendingRun[0].ID)
	require.Equal(t, job.WebhookSecret,

		pendingRun[0].WebhookSecret,
	)

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
	require.NoError(t, q.CreateJobGroup(ctx, group))
	require.False(t, group.
		CreatedAt.
		IsZero())
	require.False(t, group.
		UpdatedAt.
		IsZero())

	gotGroup, err := q.GetJobGroup(ctx, group.ID)
	require.NoError(t, err)
	require.False(t, gotGroup.
		ID != group.
		ID ||
		gotGroup.
			ProjectID != group.
			ProjectID ||
		gotGroup.
			Name != group.
			Name ||
		gotGroup.
			Slug !=
			group.Slug ||
		gotGroup.
			Description != group.Description,
	)

	groups, err := q.ListJobGroups(ctx, projectID, 100, nil)
	require.NoError(t, err)
	require.Len(t, groups,
		1,
	)

	jobInGroup := baseJob(newID(), projectID)
	jobInGroup.GroupID = group.ID
	require.NoError(t, q.CreateJob(ctx,
		jobInGroup,
	))

	jobOutsideGroup := baseJob(newID(), projectID)
	require.NoError(t, q.CreateJob(ctx,
		jobOutsideGroup,
	))

	jobsByGroup, err := q.ListJobsByGroup(ctx, group.ID, 100, nil)
	require.NoError(t, err)
	require.Len(t, jobsByGroup,

		1)
	require.Equal(t, jobInGroup.
		ID, jobsByGroup[0].ID)

	emptyJobs, err := q.ListJobsByGroup(ctx, newID(), 100, nil)
	require.NoError(t, err)
	require.Len(t, emptyJobs,

		0)

	group.Name = "Core Jobs Updated"
	group.Slug = "core-jobs-updated"
	group.Description = "Updated description"
	require.NoError(t, q.UpdateJobGroup(ctx, group))

	updated, err := q.GetJobGroup(ctx, group.ID)
	require.NoError(t, err)
	require.False(t, updated.
		Name !=
		group.Name ||
		updated.
			Slug != group.Slug ||
		updated.
			Description !=
			group.
				Description,
	)
	require.NoError(t, q.DeleteJob(ctx,
		jobInGroup.
			ID))
	require.NoError(t, q.DeleteJobGroup(ctx, group.
		ID))

	if _, err := q.GetJobGroup(ctx, group.ID); !errors.Is(err, store.ErrJobGroupNotFound) {
		require.Failf(t, "test failure",

			"GetJobGroup() after delete error = %v, want ErrJobGroupNotFound", err)
	}

	if _, err := q.GetJobGroup(ctx, newID()); !errors.Is(err, store.ErrJobGroupNotFound) {
		require.Failf(t, "test failure",

			"GetJobGroup() not found error = %v, want ErrJobGroupNotFound", err)
	}

	notFoundGroup := &domain.JobGroup{ID: newID(), ProjectID: projectID, Name: "missing", Slug: "missing"}
	if err := q.UpdateJobGroup(ctx, notFoundGroup); !errors.Is(err, store.ErrJobGroupNotFound) {
		require.Failf(t, "test failure",

			"UpdateJobGroup() not found error = %v, want ErrJobGroupNotFound", err)
	}

	if err := q.DeleteJobGroup(ctx, newID()); !errors.Is(err, store.ErrJobGroupNotFound) {
		require.Failf(t, "test failure",

			"DeleteJobGroup() not found error = %v, want ErrJobGroupNotFound", err)
	}

	emptyGroups, err := q.ListJobGroups(ctx, "project-job-group-empty", 100, nil)
	require.NoError(t, err)
	require.Len(t, emptyGroups,

		0)

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
	require.NoError(t, q.CreateJobDependency(ctx,
		dep))
	require.False(t, dep.CreatedAt.
		IsZero())
	require.Equal(t, "completed",

		dep.
			Condition,
	)

	deps, err := q.ListJobDependencies(ctx, job.ID, 100, nil)
	require.NoError(t, err)
	require.Len(t, deps, 1)
	require.False(t, deps[0].
		ID != dep.
		ID || deps[0].JobID !=
		dep.JobID ||
		deps[0].DependsOnJobID !=
			dep.DependsOnJobID ||
		deps[0].Condition !=
			dep.
				Condition,
	)

	dep2 := &domain.JobDependency{
		JobID:          job.ID,
		DependsOnJobID: depJobB.ID,
		Condition:      "failed",
	}
	require.NoError(t, q.CreateJobDependency(ctx,
		dep2))
	require.NoError(t, q.DeleteJobDependency(ctx,
		dep.ID),
	)

	deps, err = q.ListJobDependencies(ctx, job.ID, 100, nil)
	require.NoError(t, err)
	require.Len(t, deps, 1)
	require.Equal(t, dep2.ID,

		deps[0].
			ID)
	require.Error(t, q.CreateJobDependency(ctx,
		&domain.JobDependency{JobID: job.ID,
			DependsOnJobID: job.ID,
		}))

	duplicate := &domain.JobDependency{JobID: job.ID, DependsOnJobID: depJobB.ID}
	require.Error(t, q.CreateJobDependency(ctx,
		duplicate,
	))

	missing := &domain.JobDependency{JobID: newID(), DependsOnJobID: depJobA.ID}
	require.Error(t, q.CreateJobDependency(ctx,
		missing))

	emptyDeps, err := q.ListJobDependencies(ctx, depJobA.ID, 100, nil)
	require.NoError(t, err)
	require.Len(t, emptyDeps,

		0)
	require.NoError(t, q.DeleteJobDependency(ctx,
		newID(),
	))

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
	require.NoError(t, q.CreateEnvironment(ctx,
		parent))

	env := &domain.Environment{
		ID:        newID(),
		ProjectID: projectID,
		Name:      "Production",
		Slug:      "production",
		Variables: map[string]string{"DB_HOST": "db.internal", "REGION": "us-east-1"},
	}
	require.NoError(t, q.CreateEnvironment(ctx,
		env))
	require.False(t, env.CreatedAt.
		IsZero())
	require.False(t, env.UpdatedAt.
		IsZero())

	gotEnv, err := q.GetEnvironment(ctx, env.ID, projectID)
	require.NoError(t, err)
	require.False(t, gotEnv.
		ID != env.
		ID || gotEnv.
		ProjectID !=
		env.ProjectID ||
		gotEnv.
			Name !=
			env.
				Name ||
		gotEnv.Slug !=
			env.Slug,
	)
	require.Len(t, gotEnv.Variables,

		len(env.Variables))

	for k, want := range env.Variables {
		require.Equal(t, want,
			gotEnv.
				Variables[k],
		)

	}

	envs, err := q.ListEnvironments(ctx, projectID, 100, nil)
	require.NoError(t, err)
	require.Len(t, envs, 2)

	env.Name = "Production Updated"
	env.Slug = "production-updated"
	env.ParentID = parent.ID
	env.Variables = map[string]string{"DB_HOST": "db.updated", "NEW_KEY": "value"}
	require.NoError(t, q.UpdateEnvironment(ctx,
		env))

	updated, err := q.GetEnvironment(ctx, env.ID, projectID)
	require.NoError(t, err)
	require.False(t, updated.
		Name !=
		env.Name ||
		updated.
			Slug != env.Slug ||
		updated.
			ParentID !=
			env.ParentID,
	)
	require.Len(t, updated.
		Variables,

		len(env.
			Variables))

	for k, want := range env.Variables {
		require.Equal(t, want,
			updated.
				Variables[k])

	}
	require.NoError(t, q.DeleteEnvironment(ctx, env.ID, projectID))

	if _, err := q.GetEnvironment(ctx, env.ID, projectID); !errors.Is(err, store.ErrEnvironmentNotFound) {
		require.Failf(t, "test failure",

			"GetEnvironment() after delete error = %v, want ErrEnvironmentNotFound", err)
	}

	if _, err := q.GetEnvironment(ctx, newID(), projectID); !errors.Is(err, store.ErrEnvironmentNotFound) {
		require.Failf(t, "test failure",

			"GetEnvironment() not found error = %v, want ErrEnvironmentNotFound", err)
	}

	notFoundEnv := &domain.Environment{ID: newID(), ProjectID: projectID, Name: "missing", Slug: "missing"}
	if err := q.UpdateEnvironment(ctx, notFoundEnv); !errors.Is(err, store.ErrEnvironmentNotFound) {
		require.Failf(t, "test failure",

			"UpdateEnvironment() not found error = %v, want ErrEnvironmentNotFound", err)
	}

	if err := q.DeleteEnvironment(ctx, newID(), projectID); !errors.Is(err, store.ErrEnvironmentNotFound) {
		require.Failf(t, "test failure",

			"DeleteEnvironment() not found error = %v, want ErrEnvironmentNotFound", err)
	}

	emptyEnvs, err := q.ListEnvironments(ctx, "project-environment-empty", 100, nil)
	require.NoError(t, err)
	require.Len(t, emptyEnvs,

		0)

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
	require.NoError(t, q.CreateEnvironment(ctx,
		parent))

	child := &domain.Environment{
		ProjectID: projectID,
		Name:      "Child",
		Slug:      "child",
		ParentID:  parent.ID,
		Variables: map[string]string{"B": "2", "SHARED": "child"},
	}
	require.NoError(t, q.CreateEnvironment(ctx,
		child))

	grandchild := &domain.Environment{
		ProjectID: projectID,
		Name:      "Grandchild",
		Slug:      "grandchild",
		ParentID:  child.ID,
		Variables: map[string]string{"C": "3", "B": "override"},
	}
	require.NoError(t, q.CreateEnvironment(ctx,
		grandchild,
	))

	resolved, err := q.GetResolvedEnvironmentVariables(ctx, grandchild.ID)
	require.NoError(t, err)

	want := map[string]string{"A": "1", "P": "p", "SHARED": "child", "B": "override", "C": "3"}
	require.Len(t, resolved,

		len(want))

	for k, v := range want {
		require.Equal(t, v, resolved[k])

	}

	rootOnly, err := q.GetResolvedEnvironmentVariables(ctx, parent.ID)
	require.NoError(t, err)
	require.Len(t, rootOnly,

		len(parent.
			Variables,
		))

	for k, v := range parent.Variables {
		require.Equal(t, v, rootOnly[k])

	}

	if _, err := q.GetResolvedEnvironmentVariables(ctx, newID()); !errors.Is(err, store.ErrEnvironmentNotFound) {
		require.Failf(t, "test failure",

			"GetResolvedEnvironmentVariables() missing error = %v, want ErrEnvironmentNotFound", err)
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
		require.NoError(t, q.CreateEnvironment(ctx,
			env))

		prevID = env.ID
	}

	if _, err := q.GetResolvedEnvironmentVariables(ctx, prevID); err == nil {
		require.Fail(t,

			"GetResolvedEnvironmentVariables() deep chain error = nil, want error")
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
	require.NoError(t, q.CreateEnvironment(ctx,
		parent))

	child := &domain.Environment{
		ProjectID: "project-environment-child-tenant",
		Name:      "Child",
		Slug:      "child",
		ParentID:  parent.ID,
		Variables: map[string]string{"CHILD_ONLY": "ok", "SHARED": "child"},
	}
	require.NoError(t, q.CreateEnvironment(ctx,
		child))

	resolved, err := q.GetResolvedEnvironmentVariables(ctx, child.ID)
	require.NoError(t, err)
	require.Equal(t, "", resolved["PARENT_ONLY"])
	require.Equal(t, "ok",
		resolved["CHILD_ONLY"])
	require.Equal(t, "child",

		resolved["SHARED"])

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
			require.Failf(t, "test failure",

				"insert job_runs fixture %d error = %v", i, err)
		}
	}

	stats, err := q.GetJobHealthStats(ctx, job.ID, since)
	require.NoError(t, err)
	require.EqualValues(t, 7, stats.
		TotalRuns,
	)
	require.EqualValues(t, 1, stats.
		CompletedRuns,
	)
	require.EqualValues(t, 1, stats.
		FailedRuns,
	)
	require.EqualValues(t, 1, stats.
		TimedOutRuns,
	)
	require.EqualValues(t, 2, stats.
		CrashedRuns,
	)
	require.EqualValues(t, 1, stats.
		CanceledRuns,
	)

	wantSuccessRate := 100.0 / 7.0
	require.False(t, stats.
		SuccessRate <
		wantSuccessRate-
			0.0001 || stats.SuccessRate >
		wantSuccessRate+
			0.0001,
	)
	require.False(t, stats.
		AvgDurationSecs <
		29.999 ||
		stats.
			AvgDurationSecs >
			30.001,
	)
	require.False(t, stats.
		P95DurationSecs <
		47.999 ||
		stats.
			P95DurationSecs >
			48.001,
	)

	counts, err := q.GetJobHealthCounts(ctx, job.ID, since)
	require.NoError(t, err)
	require.False(t, counts.
		TotalRuns !=
		stats.
			TotalRuns ||
		counts.CompletedRuns !=
			stats.CompletedRuns ||
		counts.FailedRuns !=
			stats.
				FailedRuns ||
		counts.TimedOutRuns !=
			stats.TimedOutRuns ||
		counts.CrashedRuns !=
			stats.CrashedRuns || counts.
		CanceledRuns != stats.
		CanceledRuns || counts.ExpiredRuns !=
		stats.ExpiredRuns,
	)
	require.False(t, counts.
		AvgDurationSecs !=
		0 || counts.
		P95DurationSecs !=
		0 || counts.
		P99DurationSecs !=
		0)

	emptyJob := mustCreateJob(t, ctx, q, "project-health-stats-empty")
	emptyStats, err := q.GetJobHealthStats(ctx, emptyJob.ID, since)
	require.NoError(t, err)
	require.False(t, emptyStats.
		TotalRuns !=
		0 ||
		emptyStats.
			CompletedRuns !=
			0 || emptyStats.
		FailedRuns !=
		0 || emptyStats.
		TimedOutRuns !=
		0 || emptyStats.
		CrashedRuns !=
		0 || emptyStats.
		CanceledRuns != 0)

}

func TestBatchUpdateJobsEnabled(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-batch-update-enabled"
	job1 := baseJob(newID(), projectID)
	job2 := baseJob(newID(), projectID)
	job3 := baseJob(newID(), projectID)
	require.NoError(t, q.CreateJob(ctx,
		job1))
	require.NoError(t, q.CreateJob(ctx,
		job2))
	require.NoError(t, q.CreateJob(ctx,
		job3))

	updated, err := q.BatchUpdateJobsEnabled(ctx, []string{job1.ID, job2.ID, newID()}, false, projectID)
	require.NoError(t, err)
	require.EqualValues(t, 2, updated)

	got1, err := q.GetJob(ctx, job1.ID)
	require.NoError(t, err)

	got2, err := q.GetJob(ctx, job2.ID)
	require.NoError(t, err)

	got3, err := q.GetJob(ctx, job3.ID)
	require.NoError(t, err)
	require.False(t, got1.Enabled)
	require.False(t, got2.Enabled)
	require.True(t, got3.Enabled)

	updated, err = q.BatchUpdateJobsEnabled(ctx, []string{job2.ID}, true, projectID)
	require.NoError(t, err)
	require.EqualValues(t, 1, updated)

	got2, err = q.GetJob(ctx, job2.ID)
	require.NoError(t, err)
	require.True(t, got2.Enabled)

	updated, err = q.BatchUpdateJobsEnabled(ctx, nil, false, projectID)
	require.NoError(t, err)
	require.EqualValues(t, 0, updated)

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
	require.NoError(t, q.CreateWorkflow(ctx, workflow))
	require.NotEqual(t, "",

		workflow.
			ID)
	require.EqualValues(t, 1, workflow.
		Version,
	)
	require.False(t, workflow.
		CreatedAt.
		IsZero())
	require.False(t, workflow.
		UpdatedAt.
		IsZero())

	got, err := q.GetWorkflow(ctx, workflow.ID)
	require.NoError(t, err)
	require.False(t, got.ID !=
		workflow.
			ID ||
		got.ProjectID !=
			workflow.ProjectID ||
		got.Name !=
			workflow.Name ||
		got.
			Slug != workflow.
			Slug ||
		got.Description !=
			workflow.Description ||
		got.Enabled != workflow.
			Enabled || got.Version != workflow.
		Version)

	bySlug, err := q.GetWorkflowBySlug(ctx, workflow.ProjectID, workflow.Slug)
	require.NoError(t, err)
	require.Equal(t, workflow.
		ID, bySlug.
		ID)

	other := &domain.Workflow{
		ProjectID: projectID,
		Name:      "Workflow B",
		Slug:      "workflow-b",
		Enabled:   false,
	}
	require.NoError(t, q.CreateWorkflow(ctx, other))

	workflows, err := q.ListWorkflows(ctx, projectID, 100, nil)
	require.NoError(t, err)
	require.Len(t, workflows,

		2)

	workflow.Name = "Workflow A Updated"
	workflow.Slug = "workflow-a-updated"
	workflow.Description = "updated description"
	workflow.Enabled = false
	originalVersion := got.Version
	require.NoError(t, q.UpdateWorkflow(ctx, workflow))
	require.Equal(t, originalVersion+
		1, workflow.
		Version,
	)

	updated, err := q.GetWorkflow(ctx, workflow.ID)
	require.NoError(t, err)
	require.False(t, updated.
		Name !=
		workflow.
			Name || updated.
		Slug != workflow.
		Slug ||
		updated.
			Description !=
			workflow.
				Description ||
		updated.
			Enabled !=
			workflow.
				Enabled || updated.Version !=
		workflow.Version,
	)
	require.NoError(t, q.DeleteWorkflow(ctx, workflow.
		ID),
	)

	if _, err := q.GetWorkflow(ctx, workflow.ID); !errors.Is(err, store.ErrWorkflowNotFound) {
		require.Failf(t, "test failure",

			"GetWorkflow() after delete error = %v, want ErrWorkflowNotFound", err)
	}

	if _, err := q.GetWorkflow(ctx, newID()); !errors.Is(err, store.ErrWorkflowNotFound) {
		require.Failf(t, "test failure",

			"GetWorkflow() not found error = %v, want ErrWorkflowNotFound", err)
	}
	if _, err := q.GetWorkflowBySlug(ctx, projectID, "missing-slug"); !errors.Is(err, store.ErrWorkflowNotFound) {
		require.Failf(t, "test failure",

			"GetWorkflowBySlug() not found error = %v, want ErrWorkflowNotFound", err)
	}

	missing := &domain.Workflow{ID: newID(), ProjectID: projectID, Name: "missing", Slug: "missing"}
	if err := q.UpdateWorkflow(ctx, missing); !errors.Is(err, store.ErrWorkflowNotFound) {
		require.Failf(t, "test failure",

			"UpdateWorkflow() not found error = %v, want ErrWorkflowNotFound", err)
	}

	empty, err := q.ListWorkflows(ctx, "project-workflow-empty", 100, nil)
	require.NoError(t, err)
	require.Len(t, empty, 0)

}

func TestCreateWorkflow_RejectsMismatchedProjectContext(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	err := q.WithTx(ctx, func(txCtx context.Context, tx store.DBTX) error {
		txq := store.New(tx)
		require.NoError(t, txq.
			SetProjectContext(txCtx,
				"project-workflow-authorized",
			))

		mismatched := &domain.Workflow{
			ProjectID: "project-workflow-attacker",
			Name:      "Mismatched Workflow",
			Slug:      "mismatched-workflow-" + newID(),
			Enabled:   true,
		}
		if err := txq.CreateWorkflow(txCtx, mismatched); !errors.Is(err, store.ErrProjectContextMismatch) {
			require.Failf(t, "test failure",

				"CreateWorkflow(mismatched project context) = %v, want ErrProjectContextMismatch", err)
		}

		matching := &domain.Workflow{
			ProjectID: "project-workflow-authorized",
			Name:      "Authorized Workflow",
			Slug:      "authorized-workflow-" + newID(),
			Enabled:   true,
		}
		require.NoError(t, txq.
			CreateWorkflow(txCtx,
				matching,
			))

		return nil
	})
	require.NoError(t, err)

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
	require.NoError(t, q.CreateWorkflow(ctx, workflow))

	step := &domain.WorkflowStep{
		WorkflowID:    workflow.ID,
		JobID:         job.ID,
		StepRef:       "extract",
		DependsOn:     []string{},
		Condition:     json.RawMessage(`{"type":"step_status","step_ref":"extract","status":"completed"}`),
		Payload:       json.RawMessage(`{"batch":1}`),
		ResourceClass: "medium",
	}
	require.NoError(t, q.CreateWorkflowStep(ctx,
		step))
	require.NotEqual(t, "",

		step.ID)
	require.False(t, step.CreatedAt.
		IsZero())
	require.Equal(t, domain.
		FailWorkflow,
		step.
			OnFailure)
	require.Equal(t, "medium",

		step.ResourceClass,
	)

	got, err := q.GetWorkflowStep(ctx, step.ID)
	require.NoError(t, err)
	require.False(t, got.ID !=
		step.ID ||
		got.
			WorkflowID !=
			step.WorkflowID ||
		got.JobID !=
			step.
				JobID || got.
		StepRef !=
		step.StepRef ||
		got.OnFailure !=
			step.
				OnFailure)
	require.Equal(t, "medium",

		got.ResourceClass,
	)
	require.True(t, jsonEqual(got.Condition,
		step.
			Condition,
	))
	require.True(t, jsonEqual(got.Payload,
		step.
			Payload))

	dependent := &domain.WorkflowStep{
		WorkflowID: workflow.ID,
		JobID:      job.ID,
		StepRef:    "transform",
		DependsOn:  []string{"extract"},
		OnFailure:  domain.SkipDependents,
	}
	require.NoError(t, q.CreateWorkflowStep(ctx,
		dependent,
	))

	steps, err := q.ListStepsByWorkflow(ctx, workflow.ID)
	require.NoError(t, err)
	require.Len(t, steps, 2)
	require.False(t, steps[0].ID != step.
		ID ||
		steps[1].ID !=
			dependent.ID,
	)
	require.NoError(t, q.DeleteStepsByWorkflow(ctx, workflow.
		ID))

	steps, err = q.ListStepsByWorkflow(ctx, workflow.ID)
	require.NoError(t, err)
	require.Len(t, steps, 0)

	if _, err := q.GetWorkflowStep(ctx, step.ID); !errors.Is(err, store.ErrWorkflowStepNotFound) {
		require.Failf(t, "test failure",

			"GetWorkflowStep() after delete error = %v, want ErrWorkflowStepNotFound", err)
	}

	empty, err := q.ListStepsByWorkflow(ctx, newID())
	require.NoError(t, err)
	require.Len(t, empty, 0)

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
	require.NoError(t, q.CreateWorkflow(ctx, workflowA))

	stepA := &domain.WorkflowStep{
		WorkflowID: workflowA.ID,
		JobID:      jobA.ID,
		StepRef:    "own",
		DependsOn:  []string{},
	}
	require.NoError(t, q.CreateWorkflowStep(ctx,
		stepA))

	jobB := mustCreateJob(t, ctx, q, projectB)
	workflowB := &domain.Workflow{
		ProjectID: projectB,
		Name:      "Step RLS B",
		Slug:      "step-rls-b-" + newID(),
		Enabled:   true,
	}
	require.NoError(t, q.CreateWorkflow(ctx, workflowB))

	stepB := &domain.WorkflowStep{
		WorkflowID: workflowB.ID,
		JobID:      jobB.ID,
		StepRef:    "foreign",
		DependsOn:  []string{},
	}
	require.NoError(t, q.CreateWorkflowStep(ctx,
		stepB))

	runAsProject(t, ctx, projectA, false, func(txq *store.Queries) {
		own, err := txq.GetWorkflowStep(ctx, stepA.ID)
		require.NoError(t, err)
		require.Equal(t, stepA.
			ID,
			own.ID,
		)

		if _, err := txq.GetWorkflowStep(ctx, stepB.ID); !errors.Is(err, store.ErrWorkflowStepNotFound) {
			require.Failf(t, "test failure",

				"GetWorkflowStep(foreign) error = %v, want ErrWorkflowStepNotFound", err)
		}

		foreignSteps, err := txq.ListStepsByWorkflow(ctx, workflowB.ID)
		require.NoError(t, err)
		require.Len(t, foreignSteps,

			0)
		require.NoError(t, txq.
			DeleteStepsByWorkflow(ctx, workflowB.
				ID))

	})

	foreignAfterDelete, err := q.GetWorkflowStep(ctx, stepB.ID)
	require.NoError(t, err)
	require.Equal(t, stepB.
		ID,
		foreignAfterDelete.
			ID)

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
	require.NoError(t, q.CreateWorkflow(ctx, workflow))

	firstPayload := json.RawMessage(`{"input":"one"}`)
	run1 := &domain.WorkflowRun{
		WorkflowID: workflow.ID,
		ProjectID:  workflow.ProjectID,
		Payload:    firstPayload,
	}
	require.NoError(t, q.CreateWorkflowRun(ctx,
		run1))
	require.NotEqual(t, "",

		run1.ID)
	require.Equal(t, domain.
		WfStatusPending,
		run1.
			Status)
	require.Equal(t, domain.
		TriggerManual,
		run1.
			TriggeredBy,
	)
	require.False(t, run1.CreatedAt.
		IsZero())

	got1, err := q.GetWorkflowRun(ctx, run1.ID)
	require.NoError(t, err)
	require.False(t, got1.ID !=
		run1.
			ID || got1.
		WorkflowID !=
		run1.WorkflowID ||
		got1.
			ProjectID !=
			run1.ProjectID ||
		got1.
			Status !=
			run1.
				Status || got1.
		TriggeredBy !=
		run1.TriggeredBy,
	)
	require.True(t, jsonEqual(got1.Payload,
		run1.
			Payload),
	)

	time.Sleep(5 * time.Millisecond)
	run2 := &domain.WorkflowRun{
		WorkflowID:  workflow.ID,
		ProjectID:   workflow.ProjectID,
		Status:      domain.WfStatusRunning,
		TriggeredBy: domain.TriggerCron,
		Payload:     json.RawMessage(`{"input":"two"}`),
	}
	require.NoError(t, q.CreateWorkflowRun(ctx,
		run2))

	time.Sleep(5 * time.Millisecond)
	run3 := &domain.WorkflowRun{
		WorkflowID: workflow.ID,
		ProjectID:  workflow.ProjectID,
		Status:     domain.WfStatusFailed,
		Error:      "boom",
		Payload:    json.RawMessage(`{"input":"three"}`),
	}
	require.NoError(t, q.CreateWorkflowRun(ctx,
		run3))

	listed, err := q.ListWorkflowRuns(ctx, workflow.ID, 10, nil)
	require.NoError(t, err)
	require.Len(t, listed,
		3,
	)
	require.False(t, listed[0].ID !=
		run3.ID ||
		listed[1].
			ID != run2.ID ||
		listed[2].
			ID !=
			run1.ID,
	)

	// Cursor-based pagination: use created_at of the first result as cursor to get the next page
	cursor := listed[0].CreatedAt
	paged, err := q.ListWorkflowRuns(ctx, workflow.ID, 1, &cursor)
	require.NoError(t, err)
	require.Len(t, paged, 1)
	require.Equal(t, run2.ID,

		paged[0].ID)

	allByProject, err := q.ListWorkflowRunsByProject(ctx, projectID, nil, 10, nil)
	require.NoError(t, err)
	require.Len(t, allByProject,

		3)

	status := domain.WfStatusRunning
	onlyRunning, err := q.ListWorkflowRunsByProject(ctx, projectID, &status, 10, nil)
	require.NoError(t, err)
	require.Len(t, onlyRunning,

		1)
	require.Equal(t, run2.ID,

		onlyRunning[0].ID,
	)

	if _, err := q.GetWorkflowRun(ctx, newID()); !errors.Is(err, store.ErrWorkflowRunNotFound) {
		require.Failf(t, "test failure",

			"GetWorkflowRun() not found error = %v, want ErrWorkflowRunNotFound", err)
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
	require.NoError(t, q.CreateWorkflow(ctx, workflow))

	run := &domain.WorkflowRun{
		WorkflowID: workflow.ID,
		ProjectID:  workflow.ProjectID,
	}
	require.NoError(t, q.CreateWorkflowRun(ctx,
		run))

	startedAt := time.Now().UTC().Truncate(time.Microsecond)
	require.NoError(t, q.UpdateWorkflowRunStatus(ctx, run.
		ID, domain.WfStatusPending,

		domain.
			WfStatusRunning,

		map[string]any{"started_at": startedAt}))

	running, err := q.GetWorkflowRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		WfStatusRunning,
		running.
			Status,
	)
	require.False(t, running.
		StartedAt ==
		nil ||
		!running.
			StartedAt.Equal(
			startedAt),
	)

	finishedAt := startedAt.Add(2 * time.Minute)
	require.NoError(t, q.UpdateWorkflowRunStatus(ctx, run.
		ID, domain.WfStatusRunning,

		domain.
			WfStatusCompleted,

		map[string]any{"finished_at": finishedAt}))

	completed, err := q.GetWorkflowRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		WfStatusCompleted,

		completed.
			Status)
	require.False(t, completed.
		FinishedAt ==
		nil ||
		!completed.
			FinishedAt.
			Equal(finishedAt),
	)
	require.Error(t, q.UpdateWorkflowRunStatus(ctx, run.ID,
		domain.WfStatusCompleted,

		domain.
			WfStatusRunning,

		nil))
	require.Error(t, q.UpdateWorkflowRunStatus(ctx, run.ID,
		domain.WfStatusRunning,

		domain.
			WfStatusCanceled,

		nil))
	require.Error(t, q.UpdateWorkflowRunStatus(ctx, run.ID,
		domain.WfStatusCompleted,

		domain.
			WfStatusCompleted,

		map[string]any{"bad_field": "x"}))

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
	require.NoError(t, q.CreateWorkflow(ctx, workflow))

	step := &domain.WorkflowStep{
		WorkflowID: workflow.ID,
		JobID:      job.ID,
		StepRef:    "s1",
		DependsOn:  []string{},
	}
	require.NoError(t, q.CreateWorkflowStep(ctx,
		step))

	run := &domain.WorkflowRun{
		WorkflowID: workflow.ID,
		ProjectID:  workflow.ProjectID,
	}
	require.NoError(t, q.CreateWorkflowRun(ctx,
		run))

	jobRun := baseRun(job, newID())
	require.NoError(t, q.CreateRun(ctx,
		jobRun,
	))

	sr := &domain.WorkflowStepRun{
		WorkflowRunID:  run.ID,
		WorkflowStepID: step.ID,
		StepRef:        step.StepRef,
		DepsCompleted:  0,
		DepsRequired:   0,
	}
	require.NoError(t, q.CreateWorkflowStepRun(ctx, sr))
	require.NotEqual(t, "",

		sr.ID)
	require.Equal(t, domain.
		StepPending,
		sr.Status,
	)
	require.False(t, sr.CreatedAt.
		IsZero())

	got, err := q.GetWorkflowStepRun(ctx, sr.ID)
	require.NoError(t, err)
	require.False(t, got.ID !=
		sr.ID ||
		got.WorkflowRunID !=
			sr.WorkflowRunID ||
		got.
			WorkflowStepID !=
			sr.WorkflowStepID ||
		got.StepRef !=
			sr.StepRef ||
		got.
			Status != sr.Status || got.
		DepsCompleted != sr.DepsCompleted ||
		got.DepsRequired !=
			sr.DepsRequired)

	nilStepRun, err := q.GetStepRunByJobRunID(ctx, newID())
	require.NoError(t, err)
	require.Nil(t, nilStepRun)

	startedAt := time.Now().UTC().Truncate(time.Microsecond)
	require.NoError(t, q.UpdateStepRunStatus(ctx,
		sr.ID,
		domain.StepRunning,
		map[string]any{"job_run_id": jobRun.
			ID, "started_at": startedAt,
		}))

	byJobRunID, err := q.GetStepRunByJobRunID(ctx, jobRun.ID)
	require.NoError(t, err)
	require.NotNil(t, byJobRunID)
	require.False(t, byJobRunID.
		ID !=
		sr.ID ||
		byJobRunID.
			JobRunID != jobRun.
			ID || byJobRunID.
		Status !=
		domain.
			StepRunning,
	)
	require.False(t, byJobRunID.
		StartedAt ==
		nil ||
		!byJobRunID.
			StartedAt.
			Equal(startedAt))

	time.Sleep(5 * time.Millisecond)
	second := &domain.WorkflowStepRun{
		WorkflowRunID:  run.ID,
		WorkflowStepID: step.ID,
		StepRef:        "s2",
		Status:         domain.StepWaiting,
		DepsCompleted:  0,
		DepsRequired:   1,
	}
	require.NoError(t, q.CreateWorkflowStepRun(ctx, second))

	list, err := q.ListStepRunsByWorkflowRun(ctx, run.ID, 100, nil)
	require.NoError(t, err)
	require.Len(t, list, 2)
	require.False(t, list[0].
		ID != sr.
		ID || list[1].ID !=
		second.ID)

	if _, err := q.GetWorkflowStepRun(ctx, newID()); !errors.Is(err, store.ErrWorkflowStepRunNotFound) {
		require.Failf(t, "test failure",

			"GetWorkflowStepRun() not found error = %v, want ErrWorkflowStepRunNotFound", err)
	}
	require.Error(t, q.UpdateStepRunStatus(ctx,
		sr.ID, domain.
			StepCompleted,
		map[string]any{"bad_field": "x"}))

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
	require.NoError(t, q.CreateWorkflow(ctx, workflow))

	parent := &domain.WorkflowStep{
		WorkflowID: workflow.ID,
		JobID:      job.ID,
		StepRef:    "extract",
		DependsOn:  []string{},
	}
	require.NoError(t, q.CreateWorkflowStep(ctx,
		parent))

	secondParent := &domain.WorkflowStep{
		WorkflowID: workflow.ID,
		JobID:      job.ID,
		StepRef:    "transform",
		DependsOn:  []string{},
	}
	require.NoError(t, q.CreateWorkflowStep(ctx,
		secondParent,
	))

	child := &domain.WorkflowStep{
		WorkflowID: workflow.ID,
		JobID:      job.ID,
		StepRef:    "aggregate",
		DependsOn:  []string{"extract", "transform"},
		Condition:  json.RawMessage(`{"type":"step_status","step_ref":"extract","status":"completed"}`),
		Payload:    json.RawMessage(`{"kind":"agg"}`),
	}
	require.NoError(t, q.CreateWorkflowStep(ctx,
		child))
	require.NoError(t, q.CreateWorkflowVersionSnapshot(ctx,
		workflow.ID, 1,
	))

	run := &domain.WorkflowRun{
		WorkflowID: workflow.ID,
		ProjectID:  workflow.ProjectID,
	}
	require.NoError(t, q.CreateWorkflowRun(ctx,
		run))

	completedParent := &domain.WorkflowStepRun{
		WorkflowRunID:  run.ID,
		WorkflowStepID: parent.ID,
		StepRef:        parent.StepRef,
		Status:         domain.StepCompleted,
		DepsCompleted:  0,
		DepsRequired:   0,
	}
	require.NoError(t, q.CreateWorkflowStepRun(ctx, completedParent))

	waiting := &domain.WorkflowStepRun{
		WorkflowRunID:  run.ID,
		WorkflowStepID: child.ID,
		StepRef:        child.StepRef,
		Status:         domain.StepWaiting,
		DepsCompleted:  0,
		DepsRequired:   2,
	}
	require.NoError(t, q.CreateWorkflowStepRun(ctx, waiting))

	first, err := q.IncrementStepDeps(ctx, run.ID, parent.StepRef)
	require.NoError(t, err)
	require.Len(t, first, 1)
	require.False(t, first[0].StepRunID !=
		waiting.
			ID ||
		first[0].StepRef !=
			waiting.
				StepRef ||
		first[0].DepsCompleted !=
			1 || first[0].DepsRequired !=
		2 ||

		first[0].JobID == nil || *first[0].JobID != child.
		JobID || first[0].WorkflowRunID !=
		run.ID)
	require.True(t, jsonEqual(first[0].Condition,
		child.Condition,
	))
	require.True(t, jsonEqual(first[0].Payload,
		child.Payload,
	))

	duplicate, err := q.IncrementStepDeps(ctx, run.ID, parent.StepRef)
	require.NoError(t, err)
	require.Len(t, duplicate,

		0)

	completedSecondParent := &domain.WorkflowStepRun{
		WorkflowRunID:  run.ID,
		WorkflowStepID: secondParent.ID,
		StepRef:        secondParent.StepRef,
		Status:         domain.StepCompleted,
		DepsCompleted:  0,
		DepsRequired:   0,
	}
	require.NoError(t, q.CreateWorkflowStepRun(ctx, completedSecondParent))

	second, err := q.IncrementStepDeps(ctx, run.ID, secondParent.StepRef)
	require.NoError(t, err)
	require.Len(t, second,
		1,
	)
	require.EqualValues(t, 2, second[0].DepsCompleted)

	stored, err := q.GetWorkflowStepRun(ctx, waiting.ID)
	require.NoError(t, err)
	require.EqualValues(t, 2, stored.
		DepsCompleted,
	)

	none, err := q.IncrementStepDeps(ctx, run.ID, "missing-ref")
	require.NoError(t, err)
	require.Len(t, none, 0)

}

func TestWorkflowStepRun_TargetedListings(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-workflow-step-run-targeted-listings"
	job := mustCreateJob(t, ctx, q, projectID)
	workflow := &domain.Workflow{ProjectID: projectID, Name: "Targeted Workflow", Slug: "targeted-workflow", Enabled: true}
	require.NoError(t, q.CreateWorkflow(ctx, workflow))

	steps := []*domain.WorkflowStep{
		{WorkflowID: workflow.ID, JobID: job.ID, StepRef: "a"},
		{WorkflowID: workflow.ID, JobID: job.ID, StepRef: "b", DependsOn: []string{"a"}},
		{WorkflowID: workflow.ID, JobID: job.ID, StepRef: "c"},
		{WorkflowID: workflow.ID, JobID: job.ID, StepRef: "d", DependsOn: []string{"a"}},
	}
	for _, step := range steps {
		require.NoError(t, q.CreateWorkflowStep(ctx,
			step))

	}

	run := &domain.WorkflowRun{WorkflowID: workflow.ID, ProjectID: workflow.ProjectID}
	require.NoError(t, q.CreateWorkflowRun(ctx,
		run))

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
		require.NoError(t, q.CreateWorkflowStepRun(ctx, &seed[i]))

	}

	running, err := q.ListRunningStepRunsByWorkflowRun(ctx, run.ID, 100)
	require.NoError(t, err)
	require.False(t, len(running) !=
		1 || running[0].StepRef !=
		"a")

	runnable, err := q.ListRunnableStepRunsByWorkflowRun(ctx, run.ID, 100)
	require.NoError(t, err)
	require.Len(t, runnable,

		2)
	require.False(t, runnable[0].StepRef !=
		"b" ||
		runnable[1].StepRef !=
			"c")

	statuses, err := q.ListStepRunStatusesByWorkflowRun(ctx, run.ID)
	require.NoError(t, err)
	require.Len(t, statuses,

		4)
	require.False(t, statuses["a"] !=
		domain.StepRunning ||
		statuses["b"] !=
			domain.
				StepWaiting ||
		statuses["c"] != domain.
			StepPending ||
		statuses["d"] != domain.
			StepWaiting)

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
	require.NoError(t, q.CreateWorkflow(ctx, workflow))

	stepA := &domain.WorkflowStep{WorkflowID: workflow.ID, JobID: job.ID, StepRef: "a", DependsOn: []string{}}
	require.NoError(t, q.CreateWorkflowStep(ctx,
		stepA))

	stepB := &domain.WorkflowStep{WorkflowID: workflow.ID, JobID: job.ID, StepRef: "b", DependsOn: []string{}}
	require.NoError(t, q.CreateWorkflowStep(ctx,
		stepB))

	stepC := &domain.WorkflowStep{WorkflowID: workflow.ID, JobID: job.ID, StepRef: "c", DependsOn: []string{}}
	require.NoError(t, q.CreateWorkflowStep(ctx,
		stepC))

	run := &domain.WorkflowRun{
		WorkflowID: workflow.ID,
		ProjectID:  workflow.ProjectID,
	}
	require.NoError(t, q.CreateWorkflowRun(ctx,
		run))

	outA := json.RawMessage(`{"value":"A"}`)
	outB := json.RawMessage(`{"value":"B"}`)

	srA := &domain.WorkflowStepRun{
		WorkflowRunID:  run.ID,
		WorkflowStepID: stepA.ID,
		StepRef:        stepA.StepRef,
		Status:         domain.StepCompleted,
		Output:         outA,
	}
	require.NoError(t, q.CreateWorkflowStepRun(ctx, srA))

	srB := &domain.WorkflowStepRun{
		WorkflowRunID:  run.ID,
		WorkflowStepID: stepB.ID,
		StepRef:        stepB.StepRef,
		Status:         domain.StepCompleted,
		Output:         outB,
	}
	require.NoError(t, q.CreateWorkflowStepRun(ctx, srB))

	srC := &domain.WorkflowStepRun{
		WorkflowRunID:  run.ID,
		WorkflowStepID: stepC.ID,
		StepRef:        stepC.StepRef,
		Status:         domain.StepRunning,
	}
	require.NoError(t, q.CreateWorkflowStepRun(ctx, srC))

	outputs, err := q.GetStepOutputs(ctx, run.ID, []string{"a", "b", "c", "missing"})
	require.NoError(t, err)
	require.Len(t, outputs,

		2)
	require.True(t, jsonEqual(outputs["a"], outA))
	require.True(t, jsonEqual(outputs["b"], outB))

	if _, ok := outputs["c"]; ok {
		require.Failf(t, "test failure",

			"GetStepOutputs()[c] present = true, want false")
	}

	empty, err := q.GetStepOutputs(ctx, run.ID, []string{"missing"})
	require.NoError(t, err)
	require.Len(t, empty, 0)

}

func mustCreateJob(t *testing.T, ctx context.Context, q *store.Queries, projectID string) *domain.Job {
	t.Helper()

	job := baseJob(newID(), projectID)
	require.NoError(t, q.CreateJob(ctx,
		job))

	return job
}

func mustCreateRun(t *testing.T, ctx context.Context, q *store.Queries, job *domain.Job) *domain.JobRun {
	t.Helper()

	run := baseRun(job, newID())
	require.NoError(t, q.CreateRun(ctx,
		run))

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
	require.NotNil(t, got)

	testutil.AssertEqual(t, got, want, testutil.IgnoreFields(domain.Job{}, "PayloadSchema"))
	require.True(t, jsonEqual(got.PayloadSchema,

		want.PayloadSchema,
	))

}

func assertRunEqual(t *testing.T, want, got *domain.JobRun) {
	t.Helper()
	require.NotNil(t, got)

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
	require.False(t, want ==

		nil || got ==
		nil,
	)
	require.True(t, want.Equal(*got))

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
		require.False(t, times[i-
			1].Before(times[i]))

	}
}

func assertTimesAsc(t *testing.T, times []time.Time) {
	t.Helper()

	for i := 1; i < len(times); i++ {
		require.False(t, times[i].Before(
			times[i-1]))

	}
}

func assertEventTimesAsc(t *testing.T, events []domain.RunEvent) {
	t.Helper()

	for i := 1; i < len(events); i++ {
		require.False(t, events[i].CreatedAt.
			Before(events[i-
				1].CreatedAt))

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
	require.NoError(t, q.CreateRun(ctx,
		queued,
	))

	delayed := baseRun(job, newID())
	delayed.Status = domain.StatusDelayed
	scheduled := time.Now().UTC().Add(2 * time.Minute)
	delayed.ScheduledAt = &scheduled
	require.NoError(t, q.CreateRun(ctx,
		delayed,
	))

	executing := baseRun(job, newID())
	executing.Status = domain.StatusExecuting
	require.NoError(t, q.CreateRun(ctx,
		executing,
	))

	otherJob := mustCreateJob(t, ctx, q, "project-quota-queued-other")
	otherQueued := baseRun(otherJob, newID())
	otherQueued.Status = domain.StatusQueued
	require.NoError(t, q.CreateRun(ctx,
		otherQueued,
	))

	count, err := q.CountProjectQueuedRuns(ctx, job.ProjectID)
	require.NoError(t, err)
	require.EqualValues(t, 2, count)

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
	require.NoError(t, q.CreateRun(ctx,
		dequeued,
	))

	executing := baseRun(job, newID())
	executing.Status = domain.StatusExecuting
	heartbeat := time.Now().UTC()
	executing.HeartbeatAt = &heartbeat
	require.NoError(t, q.CreateRun(ctx,
		executing,
	))

	queued := baseRun(job, newID())
	queued.Status = domain.StatusQueued
	require.NoError(t, q.CreateRun(ctx,
		queued,
	))

	otherJob := mustCreateJob(t, ctx, q, "project-quota-active-other")
	otherExecuting := baseRun(otherJob, newID())
	otherExecuting.Status = domain.StatusExecuting
	require.NoError(t, q.CreateRun(ctx,
		otherExecuting,
	))

	count, err := q.CountProjectActiveRuns(ctx, job.ProjectID)
	require.NoError(t, err)
	require.EqualValues(t, 2, count)

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
		require.Failf(t, "test failure",

			"insert project_quotas error = %v", err)
	}

	quota, err := q.GetProjectQuota(ctx, projectID)
	require.NoError(t, err)
	require.NotNil(t, quota)
	require.False(t, quota.
		ProjectID !=
		projectID ||
		quota.
			MaxQueuedRuns !=
			15 || quota.
		MaxExecutingRuns !=
		7 || quota.
		MaxJobs !=
		40 || quota.
		Timezone !=
		"America/New_York",
	)

	missing, err := q.GetProjectQuota(ctx, "project-quota-missing")
	require.NoError(t, err)
	require.Nil(t, missing)

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
	require.NoError(t, q.CreateRun(ctx,
		stale),
	)

	fresh := baseRun(job, newID())
	fresh.Status = domain.StatusExecuting
	freshStarted := time.Now().UTC().Add(-5 * time.Minute)
	fresh.StartedAt = &freshStarted
	require.NoError(t, q.CreateRun(ctx,
		fresh),
	)

	queued := baseRun(job, newID())
	queued.Status = domain.StatusQueued
	require.NoError(t, q.CreateRun(ctx,
		queued,
	))

	// Heartbeat liveness is read from the job_run_heartbeats side table.
	oldHeartbeat := time.Now().UTC().Add(-10 * time.Minute)
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_run_heartbeats (run_id, heartbeat_at, cleared)
		VALUES ($1, $2, FALSE)`,
		stale.ID, oldHeartbeat); err != nil {
		require.Failf(t, "test failure",

			"insert stale heartbeat error = %v", err)
	}
	recentHeartbeat := time.Now().UTC().Add(-1 * time.Minute)
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_run_heartbeats (run_id, heartbeat_at, cleared)
		VALUES ($1, $2, FALSE)`,
		fresh.ID, recentHeartbeat); err != nil {
		require.Failf(t, "test failure",

			"insert fresh heartbeat error = %v", err)
	}

	runs, err := q.ListStaleRuns(ctx, 5*time.Minute)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	require.Equal(t, stale.
		ID,
		runs[0].ID)

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
	require.NoError(t, q.CreateRun(ctx,
		httpRun,
	))

	workerRun := baseRun(job, newID())
	workerRun.Status = domain.StatusExecuting
	workerRun.ExecutionMode = domain.ExecutionModeWorker
	workerRun.StartedAt = &started
	require.NoError(t, q.CreateRun(ctx,
		workerRun,
	))

	oldHeartbeat := time.Now().UTC().Add(-10 * time.Minute)
	for _, id := range []string{httpRun.ID, workerRun.ID} {
		if _, err := testDB.Pool.Exec(ctx, `
			INSERT INTO job_run_heartbeats (run_id, heartbeat_at, cleared)
			VALUES ($1, $2, FALSE)`,
			id, oldHeartbeat); err != nil {
			require.Failf(t, "test failure",

				"insert heartbeat error = %v", err)
		}
	}

	runs, err := q.ListStaleRuns(ctx, 5*time.Minute)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	require.Equal(t, httpRun.
		ID, runs[0].ID)

}

func TestRunMgmt_ListStaleRuns_IncludesActiveClaimOverlay(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-list-stale-active-claim")

	stale := baseRun(job, newID())
	stale.Status = domain.StatusQueued
	require.NoError(t, q.CreateRun(ctx, stale))
	fresh := baseRun(job, newID())
	fresh.Status = domain.StatusQueued
	require.NoError(t, q.CreateRun(ctx, fresh))

	insertActiveClaim := func(runID string, startedAt time.Time) {
		t.Helper()
		_, err := testDB.Pool.Exec(ctx, `
			INSERT INTO job_run_active_claims (run_id, ready_generation, attempt, started_at)
			SELECT run_id, ready_generation, attempt, $2
			FROM job_run_state
			WHERE run_id = $1`,
			runID, startedAt,
		)
		require.NoErrorf(t, err, "insert active claim for %s", runID)
	}
	insertActiveClaim(stale.ID, time.Now().UTC().Add(-15*time.Minute))
	insertActiveClaim(fresh.ID, time.Now().UTC().Add(-1*time.Minute))

	runs, err := q.ListStaleRuns(ctx, 5*time.Minute)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	require.Equal(t, stale.ID, runs[0].ID)
	require.Equal(t, domain.StatusExecuting, runs[0].Status)
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
	require.NoError(t, q.CreateRun(ctx,
		due))

	notDue := baseRun(job, newID())
	notDue.Status = domain.StatusDelayed
	notDueAt := time.Now().UTC().Add(10 * time.Minute)
	notDue.ScheduledAt = &notDueAt
	require.NoError(t, q.CreateRun(ctx,
		notDue,
	))

	queuedPast := baseRun(job, newID())
	queuedPast.Status = domain.StatusQueued
	queuedPastAt := time.Now().UTC().Add(-20 * time.Minute)
	queuedPast.ScheduledAt = &queuedPastAt
	require.NoError(t, q.CreateRun(ctx,
		queuedPast,
	))

	runs, err := q.ListDueRuns(ctx)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	require.Equal(t, due.ID,

		runs[0].
			ID)

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
	require.NoError(t, q.CreateRun(ctx,
		expiredDelayed,
	))

	expiredQueued := baseRun(job, newID())
	expiredQueued.Status = domain.StatusQueued
	expiredQueued.ExpiresAt = &past
	require.NoError(t, q.CreateRun(ctx,
		expiredQueued,
	))

	notExpiredQueued := baseRun(job, newID())
	notExpiredQueued.Status = domain.StatusQueued
	notExpiredQueued.ExpiresAt = &future
	require.NoError(t, q.CreateRun(ctx,
		notExpiredQueued,
	),
	)

	expiredExecuting := baseRun(job, newID())
	expiredExecuting.Status = domain.StatusExecuting
	expiredExecuting.ExpiresAt = &past
	require.NoError(t, q.CreateRun(ctx,
		expiredExecuting,
	),
	)

	expiredActiveClaim := baseRun(job, newID())
	expiredActiveClaim.Status = domain.StatusQueued
	expiredActiveClaim.ExpiresAt = &past
	require.NoError(t, q.CreateRun(ctx, expiredActiveClaim))
	_, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_run_active_claims (run_id, ready_generation, attempt, started_at)
		SELECT run_id, ready_generation, attempt, NOW()
		FROM job_run_state
		WHERE run_id = $1`,
		expiredActiveClaim.ID,
	)
	require.NoError(t, err)

	runs, err := q.ListExpiredRuns(ctx)
	require.NoError(t, err)
	require.Len(t, runs, 2)

	got := map[string]bool{}
	for _, run := range runs {
		got[run.ID] = true
	}
	require.False(t, !got[expiredDelayed.
		ID] ||
		!got[expiredQueued.
			ID])

}

func TestRunMgmt_ListStaleDequeued(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-list-stale-dequeued")

	stale := baseRun(job, newID())
	stale.Status = domain.StatusDequeued
	require.NoError(t, q.CreateRun(ctx,
		stale),
	)

	fresh := baseRun(job, newID())
	fresh.Status = domain.StatusDequeued
	require.NoError(t, q.CreateRun(ctx,
		fresh),
	)

	executing := baseRun(job, newID())
	executing.Status = domain.StatusExecuting
	require.NoError(t, q.CreateRun(ctx,
		executing,
	))

	oldStartedAt := time.Now().UTC().Add(-10 * time.Minute)
	if _, err := testDB.Pool.Exec(ctx, "UPDATE job_runs SET started_at = $1 WHERE id = $2", oldStartedAt, stale.ID); err != nil {
		require.Failf(t, "test failure",

			"update stale started_at error = %v", err)
	}
	recentStartedAt := time.Now().UTC().Add(-1 * time.Minute)
	if _, err := testDB.Pool.Exec(ctx, "UPDATE job_runs SET started_at = $1 WHERE id = $2", recentStartedAt, fresh.ID); err != nil {
		require.Failf(t, "test failure",

			"update fresh started_at error = %v", err)
	}

	runs, err := q.ListStaleDequeued(ctx, 5*time.Minute)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	require.Equal(t, stale.
		ID,
		runs[0].ID)

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
	require.NoError(t, q.CreateRun(ctx,
		oldMatch,
	))

	if _, err := testDB.Pool.Exec(ctx, "UPDATE job_runs SET created_at = $1 WHERE id = $2", since.Add(-10*time.Minute), oldMatch.ID); err != nil {
		require.Failf(t, "test failure",

			"update oldMatch created_at error = %v", err)
	}

	recentMatch := baseRun(job, newID())
	recentMatch.Payload = matchingPayload
	require.NoError(t, q.CreateRun(ctx,
		recentMatch,
	))

	if _, err := testDB.Pool.Exec(ctx, "UPDATE job_runs SET created_at = $1 WHERE id = $2", since.Add(5*time.Minute), recentMatch.ID); err != nil {
		require.Failf(t, "test failure",

			"update recentMatch created_at error = %v", err)
	}

	newestMatch := baseRun(job, newID())
	newestMatch.Payload = matchingPayload
	require.NoError(t, q.CreateRun(ctx,
		newestMatch,
	))

	if _, err := testDB.Pool.Exec(ctx, "UPDATE job_runs SET created_at = $1 WHERE id = $2", since.Add(10*time.Minute), newestMatch.ID); err != nil {
		require.Failf(t, "test failure",

			"update newestMatch created_at error = %v", err)
	}

	nonMatch := baseRun(job, newID())
	nonMatch.Payload = nonMatchingPayload
	require.NoError(t, q.CreateRun(ctx,
		nonMatch,
	))

	got, err := q.FindRecentRunByPayload(ctx, job.ID, matchingPayload, since)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, newestMatch.
		ID,
		got.ID)

	missing, err := q.FindRecentRunByPayload(ctx, job.ID, json.RawMessage(`{"kind":"other"}`), since)
	require.NoError(t, err)
	require.Nil(t, missing)

}

func TestAnalytics_CountRunsForJobSince(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-count-runs-since")
	otherJob := mustCreateJob(t, ctx, q, "project-count-runs-since-other")
	since := time.Now().UTC().Add(-15 * time.Minute)

	oldRun := baseRun(job, newID())
	require.NoError(t, q.CreateRun(ctx,
		oldRun,
	))

	if _, err := testDB.Pool.Exec(ctx, "UPDATE job_runs SET created_at = $1 WHERE id = $2", since.Add(-1*time.Minute), oldRun.ID); err != nil {
		require.Failf(t, "test failure",

			"update oldRun created_at error = %v", err)
	}

	recentRun1 := baseRun(job, newID())
	require.NoError(t, q.CreateRun(ctx,
		recentRun1,
	))

	if _, err := testDB.Pool.Exec(ctx, "UPDATE job_runs SET created_at = $1 WHERE id = $2", since.Add(1*time.Minute), recentRun1.ID); err != nil {
		require.Failf(t, "test failure",

			"update recentRun1 created_at error = %v", err)
	}

	recentRun2 := baseRun(job, newID())
	require.NoError(t, q.CreateRun(ctx,
		recentRun2,
	))

	if _, err := testDB.Pool.Exec(ctx, "UPDATE job_runs SET created_at = $1 WHERE id = $2", since.Add(2*time.Minute), recentRun2.ID); err != nil {
		require.Failf(t, "test failure",

			"update recentRun2 created_at error = %v", err)
	}

	otherJobRun := baseRun(otherJob, newID())
	require.NoError(t, q.CreateRun(ctx,
		otherJobRun,
	))

	count, err := q.CountRunsForJobSince(ctx, job.ID, since)
	require.NoError(t, err)
	require.EqualValues(t, 2, count)

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
		require.NoError(t, q.InsertEvent(
			ctx, events[i]))

	}

	allForRun, err := q.ListEventsByRunFiltered(ctx, run.ID, "", "", 100, nil)
	require.NoError(t, err)
	require.Len(t, allForRun,

		3)

	infoOnly, err := q.ListEventsByRunFiltered(ctx, run.ID, "info", "", 100, nil)
	require.NoError(t, err)
	require.Len(t, infoOnly,

		2)

	logOnly, err := q.ListEventsByRunFiltered(ctx, run.ID, "", string(domain.EventLog), 100, nil)
	require.NoError(t, err)
	require.Len(t, logOnly,

		2)

	infoLogs, err := q.ListEventsByRunFiltered(ctx, run.ID, "info", string(domain.EventLog), 100, nil)
	require.NoError(t, err)
	require.Len(t, infoLogs,

		1)
	require.Equal(t, events[0].ID, infoLogs[0].
		ID)

}

func TestWorkflowRunLabels_CRUD(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-wf-run-labels"
	wfID := newID()
	wf := &domain.Workflow{ID: wfID, ProjectID: projectID, Name: "wf-labels", Slug: "wf-labels-slug", Enabled: true, Version: 1}
	require.NoError(t, q.CreateWorkflow(ctx, wf))

	wfRunID := newID()
	wfRun := &domain.WorkflowRun{ID: wfRunID, WorkflowID: wfID, ProjectID: projectID, Status: domain.WfStatusPending, TriggeredBy: "manual"}
	require.NoError(t, q.CreateWorkflowRun(ctx,
		wfRun))

	// Create labels
	labels := map[string]string{"env": "staging", "team": "backend"}
	require.NoError(t, q.CreateWorkflowRunLabels(ctx, wfRunID,
		labels))

	// List labels
	got, err := q.ListWorkflowRunLabels(ctx, wfRunID)
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, "staging",

		got["env"])
	require.Equal(t, "backend",

		got["team"])
	require.NoError(t, q.CreateWorkflowRunLabels(ctx, wfRunID,
		map[string]string{}))
	require.NoError(t, q.CreateWorkflowRunLabels(ctx, wfRunID,
		map[string]string{"env": "production"}))

	// Empty labels noop

	// Upsert

	got2, _ := q.ListWorkflowRunLabels(ctx, wfRunID)
	require.Equal(t, "production",

		got2["env"],
	)

	// Empty for unknown run
	empty, err := q.ListWorkflowRunLabels(ctx, newID())
	require.NoError(t, err)
	require.Len(t, empty, 0)

}

func TestWorkflowStepApproval_CRUD(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-step-approval"
	job := mustCreateJob(t, ctx, q, projectID)
	wf := &domain.Workflow{ID: newID(), ProjectID: projectID, Name: "wf-approval", Slug: "wf-approval-slug", Enabled: true, Version: 1}
	require.NoError(t, q.CreateWorkflow(ctx, wf))

	step := &domain.WorkflowStep{ID: newID(), WorkflowID: wf.ID, JobID: job.ID, StepRef: "approval-step"}
	require.NoError(t, q.CreateWorkflowStep(ctx,
		step))

	wfRun := &domain.WorkflowRun{ID: newID(), WorkflowID: wf.ID, ProjectID: projectID, Status: domain.WfStatusRunning, TriggeredBy: "manual"}
	require.NoError(t, q.CreateWorkflowRun(ctx,
		wfRun))

	stepRun := &domain.WorkflowStepRun{ID: newID(), WorkflowRunID: wfRun.ID, WorkflowStepID: step.ID, StepRef: "approval-step", Status: domain.StepPending}
	require.NoError(t, q.CreateWorkflowStepRun(ctx, stepRun))

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
	require.NoError(t, q.CreateWorkflowStepApproval(ctx,
		approval))

	// Get by step run ID
	got, err := q.GetWorkflowStepApprovalByStepRunID(ctx, stepRun.ID)
	require.NoError(t, err)
	require.Equal(t, approval.
		ID, got.
		ID)
	require.Equal(t, "pending",

		got.Status,
	)
	require.Len(t, got.Approvers,

		2)

	// Update approval
	approvedAt := now.Add(5 * time.Minute)
	require.NoError(t, q.UpdateWorkflowStepApproval(ctx,
		approval.ID, "approved",
		"alice",

		&approvedAt,
		""),
	)

	updated, _ := q.GetWorkflowStepApprovalByStepRunID(ctx, stepRun.ID)
	require.Equal(t, "approved",

		updated.
			Status,
	)
	require.Equal(t, "alice",

		updated.
			ApprovedBy,
	)
	require.Error(t, q.UpdateWorkflowStepApproval(ctx, newID(), "approved",
		"bob", &approvedAt,
		"",
	))

	// Update not found

	missing, err := q.GetWorkflowStepApprovalByStepRunID(ctx, newID())
	require.NoError(t, err)
	require.Nil(t, missing)

}

func TestListExpiredWorkflowStepApprovals(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-expired-approvals"
	job := mustCreateJob(t, ctx, q, projectID)
	wf := &domain.Workflow{ID: newID(), ProjectID: projectID, Name: "wf-expired", Slug: "wf-expired-slug", Enabled: true, Version: 1}
	require.NoError(t, q.CreateWorkflow(ctx, wf))

	step := &domain.WorkflowStep{ID: newID(), WorkflowID: wf.ID, JobID: job.ID, StepRef: "exp-step"}
	require.NoError(t, q.CreateWorkflowStep(ctx,
		step))

	wfRun := &domain.WorkflowRun{ID: newID(), WorkflowID: wf.ID, ProjectID: projectID, Status: domain.WfStatusRunning, TriggeredBy: "manual"}
	require.NoError(t, q.CreateWorkflowRun(ctx,
		wfRun))

	// Create two step runs
	sr1 := &domain.WorkflowStepRun{ID: newID(), WorkflowRunID: wfRun.ID, WorkflowStepID: step.ID, StepRef: "exp-step-1", Status: domain.StepPending}
	require.NoError(t, q.CreateWorkflowStepRun(ctx, sr1))

	sr2 := &domain.WorkflowStepRun{ID: newID(), WorkflowRunID: wfRun.ID, WorkflowStepID: step.ID, StepRef: "exp-step-2", Status: domain.StepPending}
	require.NoError(t, q.CreateWorkflowStepRun(ctx, sr2))

	now := time.Now().UTC().Truncate(time.Microsecond)
	pastExpiry := now.Add(-1 * time.Hour)
	futureExpiry := now.Add(1 * time.Hour)

	// Expired pending approval
	a1 := &domain.WorkflowStepApproval{ID: newID(), WorkflowRunID: wfRun.ID, WorkflowStepRunID: sr1.ID, Approvers: []string{"alice"}, Status: "pending", RequestedAt: now, ExpiresAt: &pastExpiry}
	require.NoError(t, q.CreateWorkflowStepApproval(ctx,
		a1))

	// Not-expired pending approval
	a2 := &domain.WorkflowStepApproval{ID: newID(), WorkflowRunID: wfRun.ID, WorkflowStepRunID: sr2.ID, Approvers: []string{"bob"}, Status: "pending", RequestedAt: now, ExpiresAt: &futureExpiry}
	require.NoError(t, q.CreateWorkflowStepApproval(ctx,
		a2))

	expired, err := q.ListExpiredWorkflowStepApprovals(ctx)
	require.NoError(t, err)
	require.Len(t, expired,

		1)
	require.Equal(t, a1.ID,

		expired[0].ID)

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
	require.NoError(t, q.CreateNotificationChannel(ctx, channel))

	delivery := &domain.NotificationDelivery{
		ID:          newID(),
		ChannelID:   channel.ID,
		ProjectID:   projectID,
		EventType:   domain.NotificationEventApprovalRequested,
		Payload:     json.RawMessage(`{"step_ref":"review"}`),
		Status:      "pending",
		MaxAttempts: 3,
	}
	require.NoError(t, q.CreateNotificationDelivery(ctx,
		delivery))

	firstClaim, err := q.ClaimPendingNotificationDeliveries(ctx, 1, time.Minute)
	require.NoError(t, err)
	require.Len(t, firstClaim,

		1)
	require.Equal(t, "processing",

		firstClaim[0].Status)
	require.NotEqual(t, "",

		firstClaim[0].ClaimToken,
	)
	require.NotNil(t, firstClaim[0].LeaseExpiry)

	secondClaim, err := q.ClaimPendingNotificationDeliveries(ctx, 1, time.Minute)
	require.NoError(t, err)

	// Verify our specific delivery is NOT re-claimed (others may exist from parallel tests).
	for _, s := range secondClaim {
		require.NotEqual(t, channel.
			ID, s.
			ChannelID,
		)

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
	require.NoError(t, q.CreateNotificationDelivery(ctx,
		expiringDelivery),
	)

	expiredClaim, err := q.ClaimPendingNotificationDeliveries(ctx, 1, -time.Second)
	require.NoError(t, err)
	require.Len(t, expiredClaim,

		1)

	reclaimed, err := q.ClaimPendingNotificationDeliveries(ctx, 10, time.Minute)
	require.NoError(t, err)

	// Find our specific expired delivery in the reclaimed batch (other tests may have deliveries too).
	var reclaimedDelivery *domain.NotificationDelivery
	for i := range reclaimed {
		if reclaimed[i].ID == expiringDelivery.ID {
			reclaimedDelivery = &reclaimed[i]
			break
		}
	}
	require.NotNil(t, reclaimedDelivery)
	require.NotEqual(t, expiredClaim[0].ClaimToken,
		reclaimedDelivery.
			ClaimToken,
	)

	expiredClaim[0].Status = "failed"
	updated, err := q.UpdateClaimedNotificationDelivery(ctx, &expiredClaim[0])
	require.NoError(t, err)
	require.False(t, updated)

	reclaimedDelivery.Status = "delivered"
	now := time.Now().UTC()
	reclaimedDelivery.DeliveredAt = &now
	updated, err = q.UpdateClaimedNotificationDelivery(ctx, reclaimedDelivery)
	require.NoError(t, err)
	require.True(t, updated)

	deliveries, err := q.ListNotificationDeliveries(ctx, projectID, 10, nil)
	require.NoError(t, err)

	byID := make(map[string]domain.NotificationDelivery, len(deliveries))
	for _, got := range deliveries {
		byID[got.ID] = got
	}

	if got := byID[delivery.ID]; got.Status != "processing" {
		require.Failf(t, "test failure",

			"claimed delivery status = %q, want processing", got.Status)
	}
	if got := byID[expiringDelivery.ID]; got.Status != "delivered" {
		require.Failf(t, "test failure",

			"reclaimed delivery status = %q, want delivered", got.Status)
	}
}

func TestCountRunningWorkflowRuns(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-count-running-wfruns"
	wf := &domain.Workflow{ID: newID(), ProjectID: projectID, Name: "wf-count", Slug: "wf-count-slug", Enabled: true, Version: 1}
	require.NoError(t, q.CreateWorkflow(ctx, wf))

	// No runs yet
	count, err := q.CountRunningWorkflowRuns(ctx, wf.ID)
	require.NoError(t, err)
	require.EqualValues(t, 0, count)

	// Create pending run (not running)
	wfRun1 := &domain.WorkflowRun{ID: newID(), WorkflowID: wf.ID, ProjectID: projectID, Status: domain.WfStatusPending, TriggeredBy: "manual"}
	require.NoError(t, q.CreateWorkflowRun(ctx,
		wfRun1))

	// Create running run
	wfRun2 := &domain.WorkflowRun{ID: newID(), WorkflowID: wf.ID, ProjectID: projectID, Status: domain.WfStatusPending, TriggeredBy: "manual"}
	require.NoError(t, q.CreateWorkflowRun(ctx,
		wfRun2))
	require.NoError(t, q.UpdateWorkflowRunStatus(ctx, wfRun2.
		ID, domain.WfStatusPending,

		domain.
			WfStatusRunning,

		nil))

	count, err = q.CountRunningWorkflowRuns(ctx, wf.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)

}

func TestGetStepRunByWorkflowRunAndRef(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-step-run-by-ref"
	job := mustCreateJob(t, ctx, q, projectID)
	wf := &domain.Workflow{ID: newID(), ProjectID: projectID, Name: "wf-ref", Slug: "wf-ref-slug", Enabled: true, Version: 1}
	require.NoError(t, q.CreateWorkflow(ctx, wf))

	step := &domain.WorkflowStep{ID: newID(), WorkflowID: wf.ID, JobID: job.ID, StepRef: "my-step"}
	require.NoError(t, q.CreateWorkflowStep(ctx,
		step))

	wfRun := &domain.WorkflowRun{ID: newID(), WorkflowID: wf.ID, ProjectID: projectID, Status: domain.WfStatusRunning, TriggeredBy: "manual"}
	require.NoError(t, q.CreateWorkflowRun(ctx,
		wfRun))

	sr := &domain.WorkflowStepRun{ID: newID(), WorkflowRunID: wfRun.ID, WorkflowStepID: step.ID, StepRef: "my-step", Status: domain.StepPending}
	require.NoError(t, q.CreateWorkflowStepRun(ctx, sr))

	got, err := q.GetStepRunByWorkflowRunAndRef(ctx, wfRun.ID, "my-step")
	require.NoError(t, err)
	require.Equal(t, sr.ID,

		got.ID)
	require.Equal(t, "my-step",

		got.StepRef,
	)

}

func TestDeleteWorkflowRunsFinishedBefore(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-delete-old-wfruns"
	wf := &domain.Workflow{ID: newID(), ProjectID: projectID, Name: "wf-delete", Slug: "wf-delete-slug", Enabled: true, Version: 1}
	require.NoError(t, q.CreateWorkflow(ctx, wf))

	// Create a completed run with finished_at in the past
	wfRun := &domain.WorkflowRun{ID: newID(), WorkflowID: wf.ID, ProjectID: projectID, Status: domain.WfStatusPending, TriggeredBy: "manual"}
	require.NoError(t, q.CreateWorkflowRun(ctx,
		wfRun))
	require.NoError(t, q.UpdateWorkflowRunStatus(ctx, wfRun.
		ID, domain.WfStatusPending,

		domain.
			WfStatusRunning,

		nil))

	oldTime := time.Now().UTC().Add(-48 * time.Hour)
	require.NoError(t, q.UpdateWorkflowRunStatus(ctx, wfRun.
		ID, domain.WfStatusRunning,

		domain.
			WfStatusCompleted,

		map[string]any{
			"finished_at": oldTime,
		}))

	// Create a recent running run (should not be deleted)
	wfRun2 := &domain.WorkflowRun{ID: newID(), WorkflowID: wf.ID, ProjectID: projectID, Status: domain.WfStatusPending, TriggeredBy: "manual"}
	require.NoError(t, q.CreateWorkflowRun(ctx,
		wfRun2))
	require.NoError(t, q.UpdateWorkflowRunStatus(ctx, wfRun2.
		ID, domain.WfStatusPending,

		domain.
			WfStatusRunning,

		nil))

	deleted, err := q.DeleteWorkflowRunsFinishedBefore(ctx, time.Now().UTC().Add(-24*time.Hour), 100)
	require.NoError(t, err)
	require.EqualValues(t, 1, deleted)

	// Verify the completed run is gone
	_, err = q.GetWorkflowRun(ctx, wfRun.ID)
	require.True(t, errors.Is(err, store.
		ErrWorkflowRunNotFound,
	))

	// Verify the running run still exists
	_, err = q.GetWorkflowRun(ctx, wfRun2.ID)
	require.NoError(t, err)

}

func TestGetWorkflowRunsByParent(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-wfrun-parent"
	wf := &domain.Workflow{ID: newID(), ProjectID: projectID, Name: "wf-parent", Slug: "wf-parent-slug", Enabled: true, Version: 1}
	require.NoError(t, q.CreateWorkflow(ctx, wf))

	parentRun := &domain.WorkflowRun{ID: newID(), WorkflowID: wf.ID, ProjectID: projectID, Status: domain.WfStatusRunning, TriggeredBy: "manual"}
	require.NoError(t, q.CreateWorkflowRun(ctx,
		parentRun,
	))

	childRun := &domain.WorkflowRun{ID: newID(), WorkflowID: wf.ID, ProjectID: projectID, Status: domain.WfStatusPending, TriggeredBy: "workflow"}
	require.NoError(t, q.CreateWorkflowRun(ctx,
		childRun),
	)

	_, err := testDB.Pool.Exec(ctx, `UPDATE workflow_runs SET parent_workflow_run_id = $1 WHERE id = $2`, parentRun.ID, childRun.ID)
	require.NoError(t, err)

	children, err := q.GetWorkflowRunsByParent(ctx, parentRun.ID)
	require.NoError(t, err)
	require.Len(t, children,

		1)
	require.Equal(t, childRun.
		ID, children[0].
		ID)

	// Empty for unknown parent
	empty, err := q.GetWorkflowRunsByParent(ctx, newID())
	require.NoError(t, err)
	require.Len(t, empty, 0)

}

func TestListDeadLetterRuns(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-dead-letter"
	job := mustCreateJob(t, ctx, q, projectID)
	run := mustCreateRun(t, ctx, q, job)
	require.NoError(t, q.UpdateRunStatus(ctx,
		run.ID, domain.
			StatusQueued,
		domain.StatusDequeued,

		nil))
	require.NoError(t, q.UpdateRunStatus(ctx,
		run.ID, domain.
			StatusDequeued,
		domain.
			StatusExecuting,

		nil))
	require.NoError(t, q.UpdateRunStatus(ctx,
		run.ID, domain.
			StatusExecuting,
		domain.
			StatusDeadLetter,

		nil),
	)

	// Transition run to dead_letter: queued -> dequeued -> executing -> dead_letter

	// Create another non-dead-letter run
	run2 := mustCreateRun(t, ctx, q, job)
	_ = run2

	runs, err := q.ListDeadLetterRuns(ctx, projectID, 50, nil)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	require.Equal(t, run.ID,

		runs[0].
			ID)
	require.Equal(t, domain.
		StatusDeadLetter,

		runs[0].Status,
	)

}

func TestReplayDeadLetterRun(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-replay-dead-letter"
	job := mustCreateJob(t, ctx, q, projectID)
	run := mustCreateRun(t, ctx, q, job)
	require.NoError(t, q.UpdateRunStatus(ctx,
		run.ID, domain.
			StatusQueued,
		domain.StatusDequeued,

		nil))
	require.NoError(t, q.UpdateRunStatus(ctx,
		run.ID, domain.
			StatusDequeued,
		domain.
			StatusExecuting,

		nil))
	require.NoError(t, q.UpdateRunStatus(ctx,
		run.ID, domain.
			StatusExecuting,
		domain.
			StatusDeadLetter,

		nil),
	)

	// Transition to dead_letter

	// Replay
	replayed, err := q.ReplayDeadLetterRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		StatusQueued,
		replayed.
			Status,
	)
	require.EqualValues(t, 1, replayed.
		Attempt,
	)

	// Replay non-dead-letter run should fail
	run2 := mustCreateRun(t, ctx, q, job)
	_, err = q.ReplayDeadLetterRun(ctx, run2.ID)
	require.Error(t, err)

}

func TestSumRunCostMicrousd(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-sum-cost")
	run := mustCreateRun(t, ctx, q, job)

	// No launch cost event yet.
	total, err := q.SumRunCostMicrousd(ctx, run.ID)
	require.NoError(t, err)
	require.EqualValues(t, 0, total)

	_, err = testDB.Pool.Exec(ctx, `
		INSERT INTO billing_cost_events (
			idempotency_key, org_id, project_id, period_date, execution_mode,
			compute_cost_microusd, created_at
		) VALUES ($1, $2, $3, CURRENT_DATE, $4, $5, NOW())
	`, "strait:cost_recorded:"+run.ID, "org-sum-cost", job.ProjectID, "http", int64(3500))
	require.NoError(t, err)

	total, err = q.SumRunCostMicrousd(ctx, run.ID)
	require.NoError(t, err)
	require.EqualValues(t, 3500,
		total,
	)

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
		require.Failf(t, "test failure",

			"insert billing cost event: %v", err)
	}

	total, err := q.SumProjectDailyCostMicrousd(ctx, projectID, "UTC")
	require.NoError(t, err)
	require.EqualValues(t, 5000,
		total,
	)

	// Different project should have 0
	emptyTotal, err := q.SumProjectDailyCostMicrousd(ctx, "project-nonexistent", "UTC")
	require.NoError(t, err)
	require.EqualValues(t, 0, emptyTotal)

}

func TestIncrementStepRunAttempt(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-increment-attempt"
	job := mustCreateJob(t, ctx, q, projectID)
	wf := &domain.Workflow{ID: newID(), ProjectID: projectID, Name: "wf-inc", Slug: "wf-inc-slug", Enabled: true, Version: 1}
	require.NoError(t, q.CreateWorkflow(ctx, wf))

	step := &domain.WorkflowStep{ID: newID(), WorkflowID: wf.ID, JobID: job.ID, StepRef: "inc-step"}
	require.NoError(t, q.CreateWorkflowStep(ctx,
		step))

	wfRun := &domain.WorkflowRun{ID: newID(), WorkflowID: wf.ID, ProjectID: projectID, Status: domain.WfStatusRunning, TriggeredBy: "manual"}
	require.NoError(t, q.CreateWorkflowRun(ctx,
		wfRun))

	sr := &domain.WorkflowStepRun{ID: newID(), WorkflowRunID: wfRun.ID, WorkflowStepID: step.ID, StepRef: "inc-step", Status: domain.StepPending}
	require.NoError(t, q.CreateWorkflowStepRun(ctx, sr))
	require.NoError(t, q.IncrementStepRunAttempt(ctx, sr.
		ID, 2))
	require.NoError(t, q.IncrementStepRunAttempt(ctx, sr.
		ID, 3))
	require.Error(t, q.IncrementStepRunAttempt(ctx, sr.ID,
		3))
	require.Error(t, q.IncrementStepRunAttempt(ctx, newID(), 1))

	// Attempt starts at 1 (default set by CreateWorkflowStepRun), increment to 2

	// Increment to 3

	// Optimistic lock: trying to increment to 3 again should fail (current is 3, expects 2)

	// Unknown step run should fail

}

func TestListCronWorkflows(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-cron-workflows"
	project := &domain.Project{ID: projectID, OrgID: "org-cron-workflows", Name: "Cron Workflows"}
	require.NoError(t, q.CreateProject(ctx, project))

	// Create a workflow with cron
	wf1 := &domain.Workflow{ID: newID(), ProjectID: projectID, Name: "wf-cron", Slug: "wf-cron-slug", Enabled: true, Version: 1}
	require.NoError(t, q.CreateWorkflow(ctx, wf1))

	_, err := testDB.Pool.Exec(ctx, `UPDATE workflows SET cron = $1 WHERE id = $2`, "0 * * * *", wf1.ID)
	require.NoError(t, err)

	// Create a workflow without cron
	wf2 := &domain.Workflow{ID: newID(), ProjectID: projectID, Name: "wf-no-cron", Slug: "wf-no-cron-slug", Enabled: true, Version: 1}
	require.NoError(t, q.CreateWorkflow(ctx, wf2))

	// Create a disabled workflow with cron
	wf3 := &domain.Workflow{ID: newID(), ProjectID: projectID, Name: "wf-disabled-cron", Slug: "wf-disabled-cron-slug", Enabled: false, Version: 1}
	require.NoError(t, q.CreateWorkflow(ctx, wf3))

	_, err = testDB.Pool.Exec(ctx, `UPDATE workflows SET cron = $1 WHERE id = $2`, "*/5 * * * *", wf3.ID)
	require.NoError(t, err)

	suspendedProject := &domain.Project{ID: "project-cron-workflows-suspended", OrgID: project.OrgID, Name: "Suspended Cron Workflows"}
	require.NoError(t, q.CreateProject(ctx, suspendedProject))

	wfSuspended := &domain.Workflow{ID: newID(), ProjectID: suspendedProject.ID, Name: "wf-suspended-cron", Slug: "wf-suspended-cron-slug", Enabled: true, Version: 1}
	require.NoError(t, q.CreateWorkflow(ctx, wfSuspended))

	if _, err := testDB.Pool.Exec(ctx, `UPDATE workflows SET cron = $1 WHERE id = $2`, "*/7 * * * *", wfSuspended.ID); err != nil {
		require.Failf(t, "test failure",

			"set cron suspended error = %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `UPDATE projects SET suspended = true WHERE id = $1`, suspendedProject.ID); err != nil {
		require.Failf(t, "test failure",

			"suspend project error = %v", err)
	}

	deletedProject := &domain.Project{ID: "project-cron-workflows-deleted", OrgID: project.OrgID, Name: "Deleted Cron Workflows"}
	require.NoError(t, q.CreateProject(ctx, deletedProject))

	wfDeleted := &domain.Workflow{ID: newID(), ProjectID: deletedProject.ID, Name: "wf-deleted-cron", Slug: "wf-deleted-cron-slug", Enabled: true, Version: 1}
	require.NoError(t, q.CreateWorkflow(ctx, wfDeleted))

	if _, err := testDB.Pool.Exec(ctx, `UPDATE workflows SET cron = $1 WHERE id = $2`, "*/11 * * * *", wfDeleted.ID); err != nil {
		require.Failf(t, "test failure",

			"set cron deleted error = %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `UPDATE projects SET deleted_at = NOW() WHERE id = $1`, deletedProject.ID); err != nil {
		require.Failf(t, "test failure",

			"delete project row error = %v", err)
	}

	cronWfs, err := q.ListCronWorkflows(ctx)
	require.NoError(t, err)
	require.Len(t, cronWfs,

		1)
	require.Equal(t, wf1.ID,

		cronWfs[0].ID)

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
	require.NoError(t, err)

	_, err = testDB.Pool.Exec(ctx, `UPDATE job_runs SET continuation_of = $1, lineage_depth = $2 WHERE id = $3`, run2.ID, 2, run3.ID)
	require.NoError(t, err)
	require.NoError(t, q.UpdateRunStatus(ctx,
		run3.ID, domain.
			StatusQueued,
		domain.StatusDequeued,

		nil))
	require.NoError(t, q.UpdateRunStatus(ctx,
		run3.ID, domain.
			StatusDequeued,
		domain.
			StatusExecuting,

		nil))
	require.NoError(t, q.UpdateRunStatus(ctx,
		run3.ID, domain.
			StatusExecuting,
		domain.
			StatusCompleted,

		map[string]any{
			"finished_at": time.
				Now().UTC().Truncate(time.Microsecond), "result": json.RawMessage(`{"lineage":true}`)}))

	// Query lineage from run3 (should walk back to run1 and return all 3)
	lineage, err := q.ListRunLineage(ctx, run3.ID, 100, nil)
	require.NoError(t, err)
	require.Len(t, lineage,

		3)
	require.Equal(t, run1.ID,

		lineage[0].ID)
	require.Equal(t, run3.ID,

		lineage[2].ID)
	require.Equal(t, domain.
		StatusCompleted,
		lineage[2].Status,
	)
	require.True(t, jsonEqual(lineage[2].Result,
		[]byte(`{"lineage":true}`)))

	// Should be ordered by lineage_depth ASC (run1 first)

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
	require.NoError(t, q.InsertEvent(
		ctx, event,
	))

	// Insert a checkpoint
	cp := &domain.RunCheckpoint{RunID: run.ID, Source: "sdk", State: json.RawMessage(`{"step":1}`)}
	require.NoError(t, q.CreateRunCheckpoint(ctx,
		cp))

	// Insert an output
	out := &domain.RunOutput{ID: newID(), RunID: run.ID, OutputKey: "result", Value: json.RawMessage(`{"v":1}`)}
	require.NoError(t, q.UpsertRunOutput(ctx,
		out))

	bundle, err := q.GetDebugBundle(ctx, run.ID)
	require.NoError(t, err)
	require.NotNil(t, bundle.
		Run)
	require.Equal(t, run.ID,

		bundle.Run.
			ID)
	require.Len(t, bundle.Events,

		1)
	require.Len(t, bundle.Checkpoints,

		1)
	require.Len(t, bundle.Outputs,

		1)

	// Nonexistent run
	_, err = q.GetDebugBundle(ctx, newID())
	require.True(t, errors.Is(err, store.
		ErrRunNotFound,
	))

}

func TestGetDebugBundle_EmptyCollections(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-debug-bundle-empty")
	run := mustCreateRun(t, ctx, q, job)

	bundle, err := q.GetDebugBundle(ctx, run.ID)
	require.NoError(t, err)
	require.NotNil(t, bundle.
		Run)
	require.NotNil(t, bundle.
		Events)
	require.Len(t, bundle.Events,

		0)
	require.NotNil(t, bundle.
		Checkpoints,
	)
	require.Len(t, bundle.Checkpoints,

		0)
	require.NotNil(t, bundle.
		Outputs)
	require.Len(t, bundle.Outputs,

		0)

}

func TestUpdateRunDebugMode(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-debug-mode")
	run := mustCreateRun(t, ctx, q, job)

	// Initially false
	got, err := q.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.False(t, got.DebugMode)
	require.NoError(t, q.UpdateRunDebugMode(ctx,
		run.ID,
		true))

	// Enable debug mode

	got, err = q.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.True(t, got.DebugMode)

	var beforeNoopXmin string
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT xmin::text FROM job_runs WHERE id = $1`,

		run.ID).Scan(&beforeNoopXmin))
	require.NoError(t, q.UpdateRunDebugMode(ctx,
		run.ID,
		true))

	var afterNoopXmin string
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT xmin::text FROM job_runs WHERE id = $1`,

		run.ID).Scan(&afterNoopXmin))
	require.Equal(t, beforeNoopXmin,

		afterNoopXmin,
	)
	require.NoError(t, q.UpdateRunDebugMode(ctx,
		run.ID,
		false))

	// Disable debug mode

	got, err = q.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.False(t, got.DebugMode)

	// Nonexistent run
	err = q.UpdateRunDebugMode(ctx, newID(), true)
	require.True(t, errors.Is(err, store.
		ErrRunNotFound,
	))

}

func TestCreateWorkflowVersionSnapshot(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-wf-version-snapshot"
	job := mustCreateJob(t, ctx, q, projectID)

	// Create workflow
	wf := &domain.Workflow{ID: newID(), ProjectID: projectID, Name: "wf-snap", Slug: "wf-snap-slug", Enabled: true, Version: 1}
	require.NoError(t, q.CreateWorkflow(ctx, wf))

	// Add steps
	step1 := &domain.WorkflowStep{ID: newID(), WorkflowID: wf.ID, JobID: job.ID, StepRef: "step-a"}
	require.NoError(t, q.CreateWorkflowStep(ctx,
		step1))

	step2 := &domain.WorkflowStep{ID: newID(), WorkflowID: wf.ID, JobID: job.ID, StepRef: "step-b", DependsOn: []string{"step-a"}}
	require.NoError(t, q.CreateWorkflowStep(ctx,
		step2))
	require.NoError(t, q.CreateWorkflowVersionSnapshot(ctx,
		wf.ID, 1))

	// Snapshot version 1

	// List steps by version 1
	steps, err := q.ListStepsByWorkflowVersion(ctx, wf.ID, 1)
	require.NoError(t, err)
	require.Len(t, steps, 2)

	// Verify step refs and that returned IDs map to canonical workflow_steps IDs.
	refs := make(map[string]bool)
	idByRef := make(map[string]string, len(steps))
	for _, s := range steps {
		refs[s.StepRef] = true
		idByRef[s.StepRef] = s.ID
	}
	require.False(t, !refs["step-a"] ||
		!refs["step-b"])
	require.False(t, idByRef["step-a"] != step1.
		ID || idByRef["step-b"] !=
		step2.ID)

	// Snapshot nonexistent workflow
	err = q.CreateWorkflowVersionSnapshot(ctx, newID(), 1)
	require.True(t, errors.Is(err, store.
		ErrWorkflowNotFound,
	))

}

func TestListStepsByWorkflowVersion_Empty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	// No snapshot exists for this workflow/version
	steps, err := q.ListStepsByWorkflowVersion(ctx, newID(), 99)
	require.NoError(t, err)
	require.Len(t, steps, 0)

}

func TestWorkflowVersionSnapshot_MultipleVersions(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-wf-multi-version"
	job := mustCreateJob(t, ctx, q, projectID)

	wf := &domain.Workflow{ID: newID(), ProjectID: projectID, Name: "wf-multi", Slug: "wf-multi-slug", Enabled: true, Version: 1}
	require.NoError(t, q.CreateWorkflow(ctx, wf))

	// V1: one step
	step1 := &domain.WorkflowStep{ID: newID(), WorkflowID: wf.ID, JobID: job.ID, StepRef: "only-step-v1"}
	require.NoError(t, q.CreateWorkflowStep(ctx,
		step1))
	require.NoError(t, q.CreateWorkflowVersionSnapshot(ctx,
		wf.ID, 1))

	// Update workflow to v2
	wf.Name = "wf-multi-v2"
	require.NoError(t, q.UpdateWorkflow(ctx, wf))
	require.EqualValues(t, 2, wf.
		Version,
	)

	// Add a second step for v2
	step2 := &domain.WorkflowStep{ID: newID(), WorkflowID: wf.ID, JobID: job.ID, StepRef: "new-step-v2"}
	require.NoError(t, q.CreateWorkflowStep(ctx,
		step2))
	require.NoError(t, q.CreateWorkflowVersionSnapshot(ctx,
		wf.ID, 2))

	// V1 should have 1 step
	v1Steps, err := q.ListStepsByWorkflowVersion(ctx, wf.ID, 1)
	require.NoError(t, err)
	require.Len(t, v1Steps,

		1)
	require.Equal(t, "only-step-v1",

		v1Steps[0].StepRef)

	// V2 should have 2 steps
	v2Steps, err := q.ListStepsByWorkflowVersion(ctx, wf.ID, 2)
	require.NoError(t, err)
	require.Len(t, v2Steps,

		2)

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
	require.NoError(t, q.UpsertRunOutput(ctx,
		out))

	outputs, err := q.ListRunOutputs(ctx, run.ID, 100, nil)
	require.NoError(t, err)
	require.Len(t, outputs,

		1)
	require.True(t, jsonEqual(outputs[0].Value,
		json.RawMessage(`"initial-value"`)))

	// Upsert same (run_id, output_key) with new value
	out2 := &domain.RunOutput{
		ID:        newID(),
		RunID:     run.ID,
		OutputKey: "my-key",
		Schema:    json.RawMessage(`{"type":"number"}`),
		Value:     json.RawMessage(`42`),
	}
	require.NoError(t, q.UpsertRunOutput(ctx,
		out2))

	// Should still be 1 output (upserted, not duplicated)
	outputs, err = q.ListRunOutputs(ctx, run.ID, 100, nil)
	require.NoError(t, err)
	require.Len(t, outputs,

		1)
	require.True(t, jsonEqual(outputs[0].Value,
		json.RawMessage(`42`)))
	require.True(t, jsonEqual(outputs[0].Schema,
		json.RawMessage(`{"type":"number"}`)))

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
		require.NoError(t, q.UpsertRunOutput(ctx,
			out))

	}

	outputs, err := q.ListRunOutputs(ctx, run.ID, 100, nil)
	require.NoError(t, err)
	require.Len(t, outputs,

		3)
	require.Equal(t, "alpha",

		outputs[0].OutputKey,
	)
	require.Equal(t, "bravo",

		outputs[1].OutputKey,
	)
	require.Equal(t, "charlie",

		outputs[2].OutputKey,
	)

	// Empty for unknown run
	empty, err := q.ListRunOutputs(ctx, newID(), 100, nil)
	require.NoError(t, err)
	require.Len(t, empty, 0)

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
	require.NoError(t, q.UpsertRunOutput(ctx,
		out))

	outputs, err := q.ListRunOutputs(ctx, run.ID, 10, nil)
	require.NoError(t, err)
	require.Len(t, outputs,

		1)
	require.Nil(t, outputs[0].
		Schema)
	require.True(t, jsonEqual(outputs[0].Value,
		json.RawMessage(`{"data":"hello"}`)),
	)

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
	require.NoError(t, q.UpsertRunOutput(ctx,
		out))

	outputs, err := q.ListRunOutputs(ctx, run.ID, 10, nil)
	require.NoError(t, err)
	require.Len(t, outputs,

		1)
	require.True(t, jsonEqual(outputs[0].Value,
		json.RawMessage(largeJSON.
			String())),
	)

}

func TestListJobsByGroup(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-jobs-by-group"

	// Create a job group
	group := &domain.JobGroup{ID: newID(), ProjectID: projectID, Name: "test-group", Slug: "test-group-slug"}
	require.NoError(t, q.CreateJobGroup(ctx, group))

	// Create 3 jobs and assign them to the group
	for i := range 3 {
		job := baseJob(newID(), projectID)
		job.Name = "group-job-" + strconv.Itoa(i)
		job.Slug = "group-job-slug-" + strconv.Itoa(i)
		require.NoError(t, q.CreateJob(ctx,
			job))

		_, err := testDB.Pool.Exec(ctx, `UPDATE jobs SET group_id = $1 WHERE id = $2`, group.ID, job.ID)
		require.NoError(t, err)

	}

	// Create a job NOT in the group
	jobOutside := baseJob(newID(), projectID)
	jobOutside.Name = "outside-job"
	jobOutside.Slug = "outside-slug"
	require.NoError(t, q.CreateJob(ctx,
		jobOutside,
	))

	// List jobs by group
	jobs, err := q.ListJobsByGroup(ctx, group.ID, 100, nil)
	require.NoError(t, err)
	require.Len(t, jobs, 3)

	// Nonexistent group returns empty
	empty, err := q.ListJobsByGroup(ctx, newID(), 100, nil)
	require.NoError(t, err)
	require.Len(t, empty, 0)

}

func TestListJobsByGroup_Pagination(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-group-pagination"
	group := &domain.JobGroup{ID: newID(), ProjectID: projectID, Name: "pg-group", Slug: "pg-group-slug"}
	require.NoError(t, q.CreateJobGroup(ctx, group))

	// Create 5 jobs in the group
	for i := range 5 {
		job := baseJob(newID(), projectID)
		job.Name = "pg-job-" + strconv.Itoa(i)
		job.Slug = "pg-job-slug-" + strconv.Itoa(i)
		require.NoError(t, q.CreateJob(ctx,
			job))

		_, err := testDB.Pool.Exec(ctx, `UPDATE jobs SET group_id = $1 WHERE id = $2`, group.ID, job.ID)
		require.NoError(t, err)

	}

	// Page 1: limit=2
	page1, err := q.ListJobsByGroup(ctx, group.ID, 2, nil)
	require.NoError(t, err)
	require.Len(t, page1, 2)

	// Page 2: use cursor
	cursor := page1[len(page1)-1].CreatedAt
	page2, err := q.ListJobsByGroup(ctx, group.ID, 2, &cursor)
	require.NoError(t, err)
	require.Len(t, page2, 2)

	// Ensure no overlap
	for _, j1 := range page1 {
		for _, j2 := range page2 {
			require.NotEqual(t, j2.
				ID,
				j1.ID)

		}
	}

	// Page 3: last item
	cursor2 := page2[len(page2)-1].CreatedAt
	page3, err := q.ListJobsByGroup(ctx, group.ID, 2, &cursor2)
	require.NoError(t, err)
	require.Len(t, page3, 1)

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
	require.NoError(t, q.CreateWorkflow(ctx, wf))

	// Retrieve by slug
	got, err := q.GetWorkflowBySlug(ctx, projectID, "my-workflow-slug")
	require.NoError(t, err)
	require.Equal(t, wf.ID,

		got.ID)
	require.Equal(t, "My Workflow",

		got.
			Name)
	require.Equal(t, "my-workflow-slug",

		got.Slug,
	)
	require.Equal(t, "A test workflow",

		got.Description,
	)
	require.True(t, got.Enabled)
	require.EqualValues(t, 300, got.
		TimeoutSecs,
	)

	// Nonexistent slug
	_, err = q.GetWorkflowBySlug(ctx, projectID, "nonexistent-slug")
	require.True(t, errors.Is(err, store.
		ErrWorkflowNotFound,
	))

	// Wrong project
	_, err = q.GetWorkflowBySlug(ctx, "wrong-project", "my-workflow-slug")
	require.True(t, errors.Is(err, store.
		ErrWorkflowNotFound,
	))

}

func TestListWorkflowRunsByProject(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-wfrun-by-project"
	wf := &domain.Workflow{ID: newID(), ProjectID: projectID, Name: "wf-list", Slug: "wf-list-slug", Enabled: true, Version: 1}
	require.NoError(t, q.CreateWorkflow(ctx, wf))

	// Create 3 workflow runs with different statuses
	run1 := &domain.WorkflowRun{ID: newID(), WorkflowID: wf.ID, ProjectID: projectID, Status: domain.WfStatusPending, TriggeredBy: "manual"}
	require.NoError(t, q.CreateWorkflowRun(ctx,
		run1))

	run2 := &domain.WorkflowRun{ID: newID(), WorkflowID: wf.ID, ProjectID: projectID, Status: domain.WfStatusPending, TriggeredBy: "manual"}
	require.NoError(t, q.CreateWorkflowRun(ctx,
		run2))
	require.NoError(t, q.UpdateWorkflowRunStatus(ctx, run2.
		ID, domain.WfStatusPending,

		domain.
			WfStatusRunning,

		nil))

	// Transition to running

	run3 := &domain.WorkflowRun{ID: newID(), WorkflowID: wf.ID, ProjectID: projectID, Status: domain.WfStatusPending, TriggeredBy: "cron"}
	require.NoError(t, q.CreateWorkflowRun(ctx,
		run3))
	require.NoError(t, q.UpdateWorkflowRunStatus(ctx, run3.
		ID, domain.WfStatusPending,

		domain.
			WfStatusRunning,

		nil))
	require.NoError(t, q.UpdateWorkflowRunStatus(ctx, run3.
		ID, domain.WfStatusRunning,

		domain.
			WfStatusCompleted,

		map[string]any{"finished_at": time.
			Now()}))

	// List all — should be 3
	all, err := q.ListWorkflowRunsByProject(ctx, projectID, nil, 100, nil)
	require.NoError(t, err)
	require.Len(t, all, 3)

	// Filter by running status
	runningStatus := domain.WfStatusRunning
	running, err := q.ListWorkflowRunsByProject(ctx, projectID, &runningStatus, 100, nil)
	require.NoError(t, err)
	require.Len(t, running,

		1)
	require.Equal(t, run2.ID,

		running[0].ID)

	// Filter by completed status
	completedStatus := domain.WfStatusCompleted
	completed, err := q.ListWorkflowRunsByProject(ctx, projectID, &completedStatus, 100, nil)
	require.NoError(t, err)
	require.Len(t, completed,

		1)
	require.Equal(t, run3.ID,

		completed[0].ID)

	// Pagination: limit=2
	page1, err := q.ListWorkflowRunsByProject(ctx, projectID, nil, 2, nil)
	require.NoError(t, err)
	require.Len(t, page1, 2)

	cursor := page1[len(page1)-1].CreatedAt
	page2, err := q.ListWorkflowRunsByProject(ctx, projectID, nil, 2, &cursor)
	require.NoError(t, err)
	require.Len(t, page2, 1)

	// Different project should be empty
	empty, err := q.ListWorkflowRunsByProject(ctx, "nonexistent-project", nil, 100, nil)
	require.NoError(t, err)
	require.Len(t, empty, 0)

}

func TestWorkflowVersionSnapshot(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-wf-version-snapshot-basic"
	job := mustCreateJob(t, ctx, q, projectID)
	wf := &domain.Workflow{ID: newID(), ProjectID: projectID, Name: "wf-version", Slug: "wf-version-slug", Enabled: true, Version: 1}
	require.NoError(t, q.CreateWorkflow(ctx, wf))

	step := &domain.WorkflowStep{ID: newID(), WorkflowID: wf.ID, JobID: job.ID, StepRef: "step-version"}
	require.NoError(t, q.CreateWorkflowStep(ctx,
		step))
	require.NoError(t, q.CreateWorkflowVersionSnapshot(ctx,
		wf.ID, 1))

	steps, err := q.ListStepsByWorkflowVersion(ctx, wf.ID, 1)
	require.NoError(t, err)
	require.Len(t, steps, 1)
	require.Equal(t, "step-version",

		steps[0].
			StepRef)

}

func TestListTimedOutWorkflowRuns(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-timeout-wf-runs"
	wf := &domain.Workflow{ID: newID(), ProjectID: projectID, Name: "wf-timeout", Slug: "wf-timeout-slug", Enabled: true, Version: 1}
	require.NoError(t, q.CreateWorkflow(ctx, wf))

	runningExpired := &domain.WorkflowRun{ID: newID(), WorkflowID: wf.ID, ProjectID: projectID, Status: domain.WfStatusPending, TriggeredBy: "manual"}
	require.NoError(t, q.CreateWorkflowRun(ctx,
		runningExpired,
	))
	require.NoError(t, q.UpdateWorkflowRunStatus(ctx, runningExpired.
		ID, domain.
		WfStatusPending,

		domain.WfStatusRunning,

		nil))

	_, err := testDB.Pool.Exec(ctx, "UPDATE workflow_runs SET expires_at=$1 WHERE id=$2", time.Now().UTC().Add(-2*time.Hour), runningExpired.ID)
	require.NoError(t, err)

	pausedExpired := &domain.WorkflowRun{ID: newID(), WorkflowID: wf.ID, ProjectID: projectID, Status: domain.WfStatusPending, TriggeredBy: "manual"}
	require.NoError(t, q.CreateWorkflowRun(ctx,
		pausedExpired,
	))
	require.NoError(t, q.UpdateWorkflowRunStatus(ctx, pausedExpired.
		ID, domain.
		WfStatusPending,

		domain.
			WfStatusRunning,

		nil))
	require.NoError(t, q.UpdateWorkflowRunStatus(ctx, pausedExpired.
		ID, domain.
		WfStatusRunning,

		domain.
			WfStatusPaused,

		nil))

	_, err = testDB.Pool.Exec(ctx, "UPDATE workflow_runs SET expires_at=$1 WHERE id=$2", time.Now().UTC().Add(-1*time.Hour), pausedExpired.ID)
	require.NoError(t, err)

	runningNotExpired := &domain.WorkflowRun{ID: newID(), WorkflowID: wf.ID, ProjectID: projectID, Status: domain.WfStatusPending, TriggeredBy: "manual"}
	require.NoError(t, q.CreateWorkflowRun(ctx,
		runningNotExpired,
	))
	require.NoError(t, q.UpdateWorkflowRunStatus(ctx, runningNotExpired.
		ID,
		domain.WfStatusPending,

		domain.
			WfStatusRunning,
		nil))

	_, err = testDB.Pool.Exec(ctx, "UPDATE workflow_runs SET expires_at=$1 WHERE id=$2", time.Now().UTC().Add(1*time.Hour), runningNotExpired.ID)
	require.NoError(t, err)

	completedExpired := &domain.WorkflowRun{ID: newID(), WorkflowID: wf.ID, ProjectID: projectID, Status: domain.WfStatusPending, TriggeredBy: "manual"}
	require.NoError(t, q.CreateWorkflowRun(ctx,
		completedExpired,
	))
	require.NoError(t, q.UpdateWorkflowRunStatus(ctx, completedExpired.
		ID,
		domain.WfStatusPending,

		domain.WfStatusRunning,

		nil))
	require.NoError(t, q.UpdateWorkflowRunStatus(ctx, completedExpired.
		ID,
		domain.WfStatusRunning,

		domain.WfStatusCompleted,

		map[string]any{"finished_at": time.
			Now().UTC()}))

	_, err = testDB.Pool.Exec(ctx, "UPDATE workflow_runs SET expires_at=$1 WHERE id=$2", time.Now().UTC().Add(-3*time.Hour), completedExpired.ID)
	require.NoError(t, err)

	runs, err := q.ListTimedOutWorkflowRuns(ctx)
	require.NoError(t, err)
	require.Len(t, runs, 2)
	require.Equal(t, runningExpired.
		ID,
		runs[0].ID)
	require.Equal(t, pausedExpired.
		ID,
		runs[1].
			ID)
	require.Equal(t, domain.
		WfStatusRunning,
		runs[0].Status,
	)
	require.Equal(t, domain.
		WfStatusPaused,
		runs[1].Status,
	)

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
	require.NoError(t, q.CreateProjectRole(ctx,
		role))
	require.NotEqual(t, "",

		role.ID)
	require.False(t, role.CreatedAt.
		IsZero())

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
	require.NoError(t, q.CreateProjectRole(ctx,
		role))

	role2 := &domain.ProjectRole{
		ProjectID:   "proj-rbac-dup",
		Name:        "viewer",
		Permissions: []string{"runs:read"},
	}
	err := q.CreateProjectRole(ctx, role2)
	require.Error(t, err)

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
		require.NoError(t, q.CreateProjectRole(ctx,
			role))

	}

	roles, err := q.ListProjectRoles(ctx, "proj-list-roles", 100, nil)
	require.NoError(t, err)
	require.Len(t, roles, 3)

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
	require.NoError(t, q.CreateProjectRole(ctx,
		role))

	err := q.DeleteProjectRole(ctx, role.ID)
	require.True(t, errors.Is(err, store.
		ErrRoleNotFound,
	),
	)

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
	require.NoError(t, q.CreateProjectRole(ctx,
		role1))

	role2 := &domain.ProjectRole{
		ProjectID:   "proj-assign",
		Name:        "admin",
		Permissions: []string{"*"},
	}
	require.NoError(t, q.CreateProjectRole(ctx,
		role2))

	m := &domain.ProjectMemberRole{
		ProjectID: "proj-assign",
		UserID:    "user-1",
		RoleID:    role1.ID,
		GrantedBy: "setup",
	}
	require.NoError(t, q.AssignMemberRole(ctx,
		m))

	// Upsert: reassign to admin.
	m2 := &domain.ProjectMemberRole{
		ProjectID: "proj-assign",
		UserID:    "user-1",
		RoleID:    role2.ID,
		GrantedBy: "admin",
	}
	require.NoError(t, q.AssignMemberRole(ctx,
		m2))

	got, err := q.GetMemberRole(ctx, "proj-assign", "user-1")
	require.NoError(t, err)
	require.Equal(t, role2.
		ID,
		got.RoleID,
	)

}

func TestAssignMemberRoleWithOrgLimit_SerializesConcurrentNewMembers(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-member-limit-race"
	orgID := "org-member-limit-race"
	require.NoError(t, q.CreateProject(ctx, &domain.
		Project{ID: projectID,
		OrgID: orgID,
		Name:  "members",
	}))

	role := &domain.ProjectRole{
		ProjectID:   projectID,
		Name:        "viewer",
		Permissions: []string{"jobs:read"},
	}
	require.NoError(t, q.CreateProjectRole(ctx,
		role))

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
			require.Failf(t, "test failure", "unexpected assignment error: %v", err)
		}
	}
	require.False(t, successes !=
		1 ||
		limitErrors !=
			1)

	var count int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT COUNT(DISTINCT pmr.user_id)
		FROM project_member_roles pmr
		JOIN projects p ON p.id = pmr.project_id
		WHERE p.org_id = $1`,

		orgID).Scan(&count))
	require.EqualValues(t, 1, count)

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
	require.NoError(t, q.CreateProjectRole(ctx,
		role))

	m := &domain.ProjectMemberRole{
		ProjectID: "proj-perms",
		UserID:    "user-perms",
		RoleID:    role.ID,
	}
	require.NoError(t, q.AssignMemberRole(ctx,
		m))

	perms, err := q.GetUserPermissions(ctx, "proj-perms", "user-perms")
	require.NoError(t, err)
	require.Len(t, perms, 3)

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
	require.NoError(t, q.CreateProjectRole(ctx,
		role))

	m := &domain.ProjectMemberRole{
		ProjectID: "proj-perms-version",
		UserID:    "user-perms-version",
		RoleID:    role.ID,
	}
	require.NoError(t, q.AssignMemberRole(ctx,
		m))

	perms, version, err := q.GetUserPermissionsWithVersion(ctx, "proj-perms-version", "user-perms-version")
	require.NoError(t, err)
	require.False(t, len(perms) != 1 ||
		perms[0] != "jobs:read",
	)
	require.False(t, version <=
		0)

	role.Permissions = []string{"jobs:read", "jobs:write"}
	require.NoError(t, q.UpdateProjectRole(ctx,
		role))

	perms, updatedVersion, err := q.GetUserPermissionsWithVersion(ctx, "proj-perms-version", "user-perms-version")
	require.NoError(t, err)
	require.Len(t, perms, 2)
	require.False(t, updatedVersion <=
		version,
	)

}

func TestGetUserPermissions_NoRole(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	perms, err := q.GetUserPermissions(ctx, "proj-no-role", "unknown-user")
	require.NoError(t, err)
	require.Nil(t, perms)

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
	require.NoError(t, q.CreateProjectRole(ctx,
		foreign))

	local := &domain.ProjectRole{
		ProjectID:    "proj-rbac-parent-local-" + newID(),
		Name:         "local-child",
		Permissions:  []string{"jobs:read"},
		ParentRoleID: foreign.ID,
	}
	err := q.CreateProjectRole(ctx, local)
	require.True(t, errors.Is(err, store.
		ErrRoleNotFound,
	),
	)

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
	require.NoError(t, q.CreateProjectRole(ctx,
		foreign))

	local := &domain.ProjectRole{
		ProjectID:   "proj-rbac-update-local-" + newID(),
		Name:        "local-child",
		Permissions: []string{"jobs:read"},
	}
	require.NoError(t, q.CreateProjectRole(ctx,
		local))

	local.ParentRoleID = foreign.ID
	err := q.UpdateProjectRole(ctx, local)
	require.True(t, errors.Is(err, store.
		ErrRoleNotFound,
	),
	)

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
	require.NoError(t, q.CreateProjectRole(ctx,
		foreign))

	local := &domain.ProjectRole{
		ProjectID:   localProjectID,
		Name:        "local-reader",
		Permissions: []string{"jobs:read"},
	}
	require.NoError(t, q.CreateProjectRole(ctx,
		local))

	if _, err := testDB.Pool.Exec(ctx, `UPDATE project_roles SET parent_role_id = $1 WHERE id = $2`, foreign.ID, local.ID); err != nil {
		require.Failf(t, "test failure",

			"force cross-project parent_role_id error = %v", err)
	}

	member := &domain.ProjectMemberRole{
		ProjectID: localProjectID,
		UserID:    "user-rbac-" + newID(),
		RoleID:    local.ID,
	}
	require.NoError(t, q.AssignMemberRole(ctx,
		member))

	perms, err := q.GetUserPermissions(ctx, localProjectID, member.UserID)
	require.NoError(t, err)
	require.True(t, slices.
		Contains(perms,
			"jobs:read",
		))
	require.False(t, slices.
		Contains(
			perms, "rbac:manage",
		))

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
	require.NoError(t, q.CreateResourcePolicy(
		ctx, p))
	require.NotEqual(t, "",

		p.ID)

	actions, err := q.GetResourcePolicies(ctx, "proj-policy", "job", "job-123", "user-pol")
	require.NoError(t, err)
	require.Len(t, actions,

		2)

	policies, err := q.ListResourcePolicies(ctx, "proj-policy", "job", "job-123", 50, nil)
	require.NoError(t, err)
	require.Len(t, policies,

		1)

	if _, _, err := q.DeleteResourcePolicy(ctx, "proj-policy", p.ID); err != nil {
		require.Failf(t, "test failure",

			"DeleteResourcePolicy() error = %v", err)
	}

	actions2, err := q.GetResourcePolicies(ctx, "proj-policy", "job", "job-123", "user-pol")
	require.NoError(t, err)
	require.Nil(t, actions2)

}

func TestUpsertKnownActor(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)
	require.NoError(t, q.UpsertKnownActor(ctx,
		"actor-1",
		"alice@example.com",
		"Alice",
	))

	actor, err := q.GetKnownActor(ctx, "actor-1")
	require.NoError(t, err)
	require.NotNil(t, actor)
	require.Equal(t, "alice@example.com",

		actor.
			Email)
	require.Equal(t, "Alice",

		actor.Name,
	)

}

func TestUpsertKnownActor_PreservesExisting(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)
	require.NoError(t, q.UpsertKnownActor(ctx,
		"actor-2",
		"bob@example.com",
		"Bob"))
	require.NoError(t, q.UpsertKnownActor(ctx,
		"actor-2",
		"bob-new@example.com",
		""),
	)

	// Second upsert with empty name should keep "Bob".

	actor, err := q.GetKnownActor(ctx, "actor-2")
	require.NoError(t, err)
	require.Equal(t, "Bob",

		actor.Name,
	)
	require.Equal(t, "bob-new@example.com",

		actor.
			Email)

}

func TestGetKnownActor_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	actor, err := q.GetKnownActor(ctx, "nonexistent")
	require.NoError(t, err)
	require.Nil(t, actor)

}

func TestCreateJob_SetsVersionID(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "proj-vid")
	job.CreatedBy = "user-creator"
	require.NoError(t, q.CreateJob(ctx,
		job))
	require.NotEqual(t, "",

		job.VersionID,
	)
	require.Equal(t, "user-creator",

		job.CreatedBy,
	)

	// Read back.
	got, err := q.GetJob(ctx, job.ID)
	require.NoError(t, err)
	require.Equal(t, job.VersionID,

		got.
			VersionID,
	)
	require.Equal(t, "user-creator",

		got.CreatedBy,
	)

}

func TestUpdateJob_GeneratesNewVersionID(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "proj-vid-upd")
	require.NoError(t, q.CreateJob(ctx,
		job))

	oldVersionID := job.VersionID

	job.Name = "updated-name"
	job.UpdatedBy = "user-updater"
	require.NoError(t, q.UpdateJob(ctx,
		job))
	require.NotEqual(t, oldVersionID,

		job.VersionID,
	)
	require.NotEqual(t, "",

		job.VersionID,
	)

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
	require.NoError(t, q.CreateWorkflow(ctx, w))
	require.NotEqual(t, "",

		w.VersionID,
	)
	require.Equal(t, domain.
		VersionPolicyPin,

		w.VersionPolicy,
	)

	got, err := q.GetWorkflow(ctx, w.ID)
	require.NoError(t, err)
	require.Equal(t, w.VersionID,

		got.
			VersionID,
	)
	require.Equal(t, "user-wf-creator",

		got.CreatedBy,
	)

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
	require.NoError(t, q.CreateWorkflow(ctx, w))

	oldVersionID := w.VersionID

	w.Name = "wf-updated"
	w.UpdatedBy = "user-updater"
	require.NoError(t, q.UpdateWorkflow(ctx, w))
	require.NotEqual(t, oldVersionID,

		w.VersionID,
	)
	require.NotEqual(t, "",

		w.VersionID,
	)
	require.EqualValues(t, 2, w.Version)

}

func TestCreateJob_DefaultVersionPolicy(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "proj-vpol")
	require.NoError(t, q.CreateJob(ctx,
		job))

	got, err := q.GetJob(ctx, job.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		VersionPolicyPin,

		got.VersionPolicy,
	)

}

func TestDeleteResourcePolicy_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, _, err := q.DeleteResourcePolicy(ctx, "proj-missing", "nonexistent-policy-id")
	require.True(t, errors.Is(err, store.
		ErrResourcePolicyNotFound,
	))

}

func TestRemoveMemberRole_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	err := q.RemoveMemberRole(ctx, "proj-nonexistent", "user-nonexistent")
	require.True(t, errors.Is(err, store.
		ErrMemberNotFound,
	))

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
	require.NoError(t, q.CreateProjectRole(ctx,
		role))

	role.Name = "deployer-v2"
	role.Description = "Updated"
	role.Permissions = []string{"jobs:write", "jobs:trigger"}
	require.NoError(t, q.UpdateProjectRole(ctx,
		role))

	got, err := q.GetProjectRole(ctx, role.ID)
	require.NoError(t, err)
	require.Equal(t, "deployer-v2",

		got.
			Name)
	require.Len(t, got.Permissions,

		2,
	)

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
	require.NoError(t, q.CreateProjectRole(ctx,
		role))

	role.Name = "hacked"
	err := q.UpdateProjectRole(ctx, role)
	require.True(t, errors.Is(err, store.
		ErrRoleNotFound,
	),
	)

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
	require.NoError(t, q.CreateProjectRole(ctx,
		role))

	for _, userID := range []string{"user-a", "user-b", "user-c"} {
		m := &domain.ProjectMemberRole{
			ProjectID: "proj-list-members",
			UserID:    userID,
			RoleID:    role.ID,
		}
		require.NoError(t, q.AssignMemberRole(ctx,
			m))

	}

	members, err := q.ListProjectMembers(ctx, "proj-list-members", 100, nil)
	require.NoError(t, err)
	require.Len(t, members,

		3)

}

func TestListJobsByTag_KeyOnly(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "proj-tags-key")
	job.Tags = map[string]string{"env": "prod", "team": "backend"}
	require.NoError(t, q.CreateJob(ctx,
		job))

	// Search by key only (any value)
	jobs, err := q.ListJobsByTag(ctx, "proj-tags-key", "env", "", 50, nil)
	require.NoError(t, err)
	require.Len(t, jobs, 1)

	// Key doesn't exist
	jobs2, err := q.ListJobsByTag(ctx, "proj-tags-key", "missing", "", 50, nil)
	require.NoError(t, err)
	require.Len(t, jobs2, 0)

}

func TestListJobsByTag_KeyValue(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job1 := baseJob(newID(), "proj-tags-kv")
	job1.Tags = map[string]string{"env": "prod"}
	require.NoError(t, q.CreateJob(ctx,
		job1))

	job2 := baseJob(newID(), "proj-tags-kv")
	job2.Tags = map[string]string{"env": "staging"}
	require.NoError(t, q.CreateJob(ctx,
		job2))

	jobs, err := q.ListJobsByTag(ctx, "proj-tags-kv", "env", "prod", 50, nil)
	require.NoError(t, err)
	require.Len(t, jobs, 1)
	require.Equal(t, job1.ID,

		jobs[0].
			ID)

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
	require.NoError(t, q.CreateWorkflow(ctx, w))

	workflows, err := q.ListWorkflowsByTag(ctx, "proj-wf-tags", "service", "payments", 50, nil)
	require.NoError(t, err)
	require.Len(t, workflows,

		1)

	// Wrong value
	workflows2, err := q.ListWorkflowsByTag(ctx, "proj-wf-tags", "service", "wrong", 50, nil)
	require.NoError(t, err)
	require.Len(t, workflows2,

		0)

}

func TestJobEmptyTags(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "proj-empty-tags")
	require.NoError(t, q.CreateJob(ctx,
		job))

	// No tags set (nil)

	got, err := q.GetJob(ctx, job.ID)
	require.NoError(t, err)
	require.Nil(t, got.
		Tags)

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
	require.NoError(t, q.CreateProjectRole(ctx,
		role))
	require.NoError(t, q.DeleteProjectRole(ctx,
		role.ID))

	// Verify it's gone.
	_, err := q.GetProjectRole(ctx, role.ID)
	require.True(t, errors.Is(err, store.
		ErrRoleNotFound,
	),
	)

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
	require.NoError(t, q.CreateProjectRole(ctx,
		role))

	m := &domain.ProjectMemberRole{
		ProjectID: "proj-get-member",
		UserID:    "user-get-member",
		RoleID:    role.ID,
		GrantedBy: "admin-user",
	}
	require.NoError(t, q.AssignMemberRole(ctx,
		m))

	got, err := q.GetMemberRole(ctx, "proj-get-member", "user-get-member")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, role.ID,

		got.RoleID,
	)
	require.Equal(t, "admin-user",

		got.
			GrantedBy,
	)

}

func TestGetMemberRole_NotExists(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	got, err := q.GetMemberRole(ctx, "proj-nonexistent", "user-nonexistent")
	require.NoError(t, err)
	require.Nil(t, got)

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
	require.NoError(t, q.CreateResourcePolicy(
		ctx, p))

	// Upsert with new actions.
	p2 := &domain.ResourcePolicy{
		ProjectID:    "proj-rp-upsert",
		ResourceType: "job",
		ResourceID:   "job-1",
		UserID:       "user-1",
		Actions:      []string{"read", "write"},
	}
	require.NoError(t, q.CreateResourcePolicy(
		ctx, p2))

	// Read back — should be updated.
	actions, err := q.GetResourcePolicies(ctx, "proj-rp-upsert", "job", "job-1", "user-1")
	require.NoError(t, err)
	require.Len(t, actions,

		2)

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
		require.NoError(t, q.CreateResourcePolicy(
			ctx, p))

	}

	a, err := q.ListResourcePolicies(ctx, "proj-rp-scope-a", "job", "shared-job", 10, nil)
	require.NoError(t, err)
	require.False(t, len(a) !=
		1 || a[0].ProjectID !=
		"proj-rp-scope-a",
	)

	bActions, err := q.GetResourcePolicies(ctx, "proj-rp-scope-b", "job", "shared-job", "shared-user")
	require.NoError(t, err)
	require.True(t, slices.
		Contains(bActions,

			"proj-rp-scope-b",
		))

	if _, _, err := q.DeleteResourcePolicy(ctx, "proj-rp-scope-b", a[0].ID); !errors.Is(err, store.ErrResourcePolicyNotFound) {
		require.Failf(t, "test failure",

			"DeleteResourcePolicy(cross-project) error = %v, want ErrResourcePolicyNotFound", err)
	}
	if _, _, err := q.DeleteResourcePolicy(ctx, "proj-rp-scope-a", a[0].ID); err != nil {
		require.Failf(t, "test failure",

			"DeleteResourcePolicy(project A) error = %v", err)
	}
}

func TestListResourcePolicies_Empty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	policies, err := q.ListResourcePolicies(ctx, "proj-empty", "job", "nonexistent", 50, nil)
	require.NoError(t, err)
	require.NotNil(t, policies)
	require.Len(t, policies,

		0)

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
		require.NoError(t, q.CreateResourcePolicy(
			ctx, p))

	}

	policies, err := q.ListResourcePolicies(ctx, "proj-rp-multi", "workflow", "wf-1", 50, nil)
	require.NoError(t, err)
	require.Len(t, policies,

		3)

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
	require.NoError(t, q.CreateResourcePolicy(
		ctx, p))

	actions, err := q.GetResourcePolicies(ctx, "proj-rp-wrong", "job", "job-1", "user-b")
	require.NoError(t, err)
	require.Nil(t, actions)

}

func TestListProjectRoles_EmptyProject(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	roles, err := q.ListProjectRoles(ctx, "proj-no-roles", 100, nil)
	require.NoError(t, err)
	require.Len(t, roles, 0)

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
	require.NoError(t, q.CreateProjectRole(ctx,
		role))

	m := &domain.ProjectMemberRole{
		ProjectID: "proj-perm-wildcard",
		UserID:    "user-wildcard",
		RoleID:    role.ID,
	}
	require.NoError(t, q.AssignMemberRole(ctx,
		m))

	perms, err := q.GetUserPermissions(ctx, "proj-perm-wildcard", "user-wildcard")
	require.NoError(t, err)
	require.False(t, len(perms) != 1 ||
		perms[0] != "*")

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
	require.NoError(t, q.CreateProjectRole(ctx,
		role))
	require.NotEqual(t, "",

		role.ID)

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
	require.NoError(t, q.CreateProjectRole(ctx,
		role))

	role.Name = "new-name"
	role.Description = "updated desc"
	require.NoError(t, q.UpdateProjectRole(ctx,
		role))

	got, err := q.GetProjectRole(ctx, role.ID)
	require.NoError(t, err)
	require.Equal(t, "new-name",

		got.
			Name)
	require.Equal(t, "updated desc",

		got.Description,
	)

}

// Test hardening: Actors

func TestUpsertKnownActor_UpdateEmail(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)
	require.NoError(t, q.UpsertKnownActor(ctx,
		"actor-update-email",
		"old@example.com",

		"Alice",
	))
	require.NoError(t, q.UpsertKnownActor(ctx,
		"actor-update-email",
		"new@example.com",

		""),
	)

	// Update email.

	got, err := q.GetKnownActor(ctx, "actor-update-email")
	require.NoError(t, err)
	require.Equal(t, "new@example.com",

		got.Email,
	)
	require.Equal(t, "Alice",

		got.Name,
	)

	// Name should be preserved (empty string in second upsert).

}

func TestUpsertKnownActor_BothEmpty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)
	require.NoError(t, q.UpsertKnownActor(ctx,
		"actor-both-empty",
		"orig@example.com",

		"Bob",
	))
	require.NoError(t, q.UpsertKnownActor(ctx,
		"actor-both-empty",
		"", ""),
	)

	// Upsert with both empty — should preserve originals.

	got, err := q.GetKnownActor(ctx, "actor-both-empty")
	require.NoError(t, err)
	require.Equal(t, "orig@example.com",

		got.Email,
	)
	require.Equal(t, "Bob",

		got.Name)

}

func TestUpsertKnownActor_SyncedAtUpdates(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)
	require.NoError(t, q.UpsertKnownActor(ctx,
		"actor-synced-at",
		"a@b.com",
		"A"))

	got1, err := q.GetKnownActor(ctx, "actor-synced-at")
	require.NoError(t, err)
	require.NoError(t, q.UpsertKnownActor(ctx,
		"actor-synced-at",
		"a@b.com",
		"A"))

	// Second upsert.

	got2, err := q.GetKnownActor(ctx, "actor-synced-at")
	require.NoError(t, err)
	require.False(t, got2.SyncedAt.
		Before(got1.
			SyncedAt))

}

func TestGetKnownActor_AllFields(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)
	require.NoError(t, q.UpsertKnownActor(ctx,
		"actor-all-fields",
		"all@example.com",

		"All Fields",
	))

	got, err := q.GetKnownActor(ctx, "actor-all-fields")
	require.NoError(t, err)
	require.Equal(t, "actor-all-fields",

		got.ID,
	)
	require.Equal(t, "all@example.com",

		got.Email,
	)
	require.Equal(t, "All Fields",

		got.
			Name)
	require.False(t, got.SyncedAt.
		IsZero())
	require.Equal(t, "", got.
		AvatarURL,
	)

	// AvatarURL is not set via upsert, so it should be empty.

}

// Test hardening: Jobs with new fields

func TestCreateJob_VersionIDPrefix(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "proj-vid-prefix")
	require.NoError(t, q.CreateJob(ctx,
		job))
	require.False(t, job.VersionID ==
		"" || job.
		VersionID[:4] != "ver_")

}

func TestCreateJob_UpdatedByEmpty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "proj-empty-updatedby")
	job.CreatedBy = "creator"
	require.NoError(t, q.CreateJob(ctx,
		job))

	got, err := q.GetJob(ctx, job.ID)
	require.NoError(t, err)
	require.Equal(t, "creator",

		got.CreatedBy,
	)
	require.Equal(t, "", got.
		UpdatedBy,
	)

}

func TestUpdateJob_SetsUpdatedBy(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "proj-updatedby")
	job.CreatedBy = "original-creator"
	require.NoError(t, q.CreateJob(ctx,
		job))

	job.Name = "Updated Name"
	job.UpdatedBy = "editor-user"
	require.NoError(t, q.UpdateJob(ctx,
		job))

	got, err := q.GetJob(ctx, job.ID)
	require.NoError(t, err)
	require.Equal(t, "original-creator",

		got.CreatedBy,
	)
	require.Equal(t, "editor-user",

		got.
			UpdatedBy,
	)

}

func TestUpdateJob_VersionIncrements(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "proj-ver-inc")
	require.NoError(t, q.CreateJob(ctx,
		job))
	require.EqualValues(t, 1, job.
		Version)

	job.Name = "Update 1"
	require.NoError(t, q.UpdateJob(ctx,
		job))

	got, _ := q.GetJob(ctx, job.ID)
	require.EqualValues(t, 2, got.
		Version)

	got.Name = "Update 2"
	require.NoError(t, q.UpdateJob(ctx,
		got))

	got2, _ := q.GetJob(ctx, job.ID)
	require.EqualValues(t, 3, got2.
		Version)

}

func TestCreateJob_CustomVersionPolicy(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "proj-custom-policy")
	job.VersionPolicy = domain.VersionPolicyLatest
	require.NoError(t, q.CreateJob(ctx,
		job))

	got, err := q.GetJob(ctx, job.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		VersionPolicyLatest,

		got.VersionPolicy,
	)

}

func TestDeleteJob_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	err := q.DeleteJob(ctx, "nonexistent-job-id")
	require.True(t, errors.Is(err, store.
		ErrJobNotFound,
	))

}

func TestDeleteJob_NoRunsSuccess(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "proj-del-norun")
	require.NoError(t, q.CreateJob(ctx,
		job))
	require.NoError(t, q.DeleteJob(ctx,
		job.ID,
	))

	_, err := q.GetJob(ctx, job.ID)
	require.True(t, errors.Is(err, store.
		ErrJobNotFound,
	))

}

func TestDeleteJob_ActiveRunsBlocked(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "proj-del-active")
	require.NoError(t, q.CreateJob(ctx,
		job))

	// Create a queued run.
	run := baseRun(job, newID())
	run.Status = domain.StatusQueued
	require.NoError(t, q.CreateRun(ctx,
		run))

	err := q.DeleteJob(ctx, job.ID)
	require.True(t, errors.Is(err, store.
		ErrJobHasActiveRuns,
	))

}

func TestDeleteJob_CompletedRunsAllowed(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := baseJob(newID(), "proj-del-completed")
	require.NoError(t, q.CreateJob(ctx,
		job))

	// Create a completed run.
	run := baseRun(job, newID())
	run.Status = domain.StatusQueued
	require.NoError(t, q.CreateRun(ctx,
		run))
	require.NoError(t, q.UpdateRunStatus(ctx,
		run.ID, domain.
			StatusQueued,
		domain.StatusDequeued,

		nil))
	require.NoError(t, q.UpdateRunStatus(ctx,
		run.ID, domain.
			StatusDequeued,
		domain.
			StatusExecuting,

		nil))
	require.NoError(t, q.UpdateRunStatus(ctx,
		run.ID, domain.
			StatusExecuting,
		domain.
			StatusCompleted,

		nil))

	// Transition to completed.

	seedRetentionSideRows(t, ctx, run.ID)
	require.NoError(t, q.DeleteJob(ctx,
		job.ID,
	))

	// Delete should succeed now (only completed runs).

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
	require.NoError(t, q.CreateJob(ctx,
		job))

	jobs, err := q.ListJobs(ctx, projID, 50, nil)
	require.NoError(t, err)
	require.Len(t, jobs, 1)
	require.NotEqual(t, "",

		jobs[0].VersionID,
	)
	require.Equal(t, "list-creator",

		jobs[0].CreatedBy,
	)
	require.False(t, jobs[0].
		Tags ==
		nil || jobs[0].Tags["env"] != "prod")

}

func TestGetJobBySlug_IncludesNewFields(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projID := "proj-slug-fields-" + newID()
	job := baseJob(newID(), projID)
	job.Tags = map[string]string{"tier": "premium"}
	job.CreatedBy = "slug-creator"
	require.NoError(t, q.CreateJob(ctx,
		job))

	got, err := q.GetJobBySlug(ctx, projID, job.Slug)
	require.NoError(t, err)
	require.NotEqual(t, "",

		got.VersionID,
	)
	require.Equal(t, domain.
		VersionPolicyPin,

		got.VersionPolicy,
	)
	require.Equal(t, "premium",

		got.Tags["tier"])

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
	require.NoError(t, q.CreateWorkflow(ctx, w))

	got, err := q.GetWorkflow(ctx, w.ID)
	require.NoError(t, err)
	require.NotNil(t, got.Tags)
	require.False(t, got.Tags["team"] !=
		"core" ||
		got.Tags["env"] != "staging",
	)

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
	require.NoError(t, q.CreateWorkflow(ctx, w))

	w.Tags = map[string]string{"new": "value"}
	require.NoError(t, q.UpdateWorkflow(ctx, w))

	got, err := q.GetWorkflow(ctx, w.ID)
	require.NoError(t, err)
	require.Equal(t, "value",

		got.Tags["new"])

	if _, ok := got.Tags["old"]; ok {
		require.Fail(t,

			"old tag should be replaced")
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
	require.NoError(t, q.CreateWorkflow(ctx, w))

	w.UpdatedBy = "editor"
	w.Name = "wf-updated"
	require.NoError(t, q.UpdateWorkflow(ctx, w))

	got, err := q.GetWorkflow(ctx, w.ID)
	require.NoError(t, err)
	require.Equal(t, "editor",

		got.UpdatedBy,
	)

}

// Test hardening: Tags queries

func TestListJobsByTag_MultipleTagsOnJob(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projID := "proj-multi-tags-" + newID()
	job := baseJob(newID(), projID)
	job.Tags = map[string]string{"team": "core", "env": "prod", "service": "api"}
	require.NoError(t, q.CreateJob(ctx,
		job))

	// Search for just "team" key.
	jobs, err := q.ListJobsByTag(ctx, projID, "team", "", 50, nil)
	require.NoError(t, err)
	require.Len(t, jobs, 1)

	// Search for "env":"prod".
	jobs2, err := q.ListJobsByTag(ctx, projID, "env", "prod", 50, nil)
	require.NoError(t, err)
	require.Len(t, jobs2, 1)

	// Search for non-existent tag.
	jobs3, err := q.ListJobsByTag(ctx, projID, "missing", "", 50, nil)
	require.NoError(t, err)
	require.Len(t, jobs3, 0)

}

func TestListJobsByTag_CrossProjectIsolation(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projA := "proj-iso-a-" + newID()
	projB := "proj-iso-b-" + newID()

	jobA := baseJob(newID(), projA)
	jobA.Tags = map[string]string{"team": "core"}
	require.NoError(t, q.CreateJob(ctx,
		jobA))

	jobB := baseJob(newID(), projB)
	jobB.Tags = map[string]string{"team": "core"}
	require.NoError(t, q.CreateJob(ctx,
		jobB))

	// Query project A — should only return jobA.
	jobsA, err := q.ListJobsByTag(ctx, projA, "team", "core", 50, nil)
	require.NoError(t, err)
	require.Len(t, jobsA, 1)
	require.Equal(t, jobA.ID,

		jobsA[0].ID)

	// Query project B — should only return jobB.
	jobsB, err := q.ListJobsByTag(ctx, projB, "team", "core", 50, nil)
	require.NoError(t, err)
	require.Len(t, jobsB, 1)
	require.Equal(t, jobB.ID,

		jobsB[0].ID)

}

func TestListJobsByTag_SpecialCharsInTagValue(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projID := "proj-special-tags-" + newID()
	job := baseJob(newID(), projID)
	job.Tags = map[string]string{"note": "hello world: special & chars <> 🚀"}
	require.NoError(t, q.CreateJob(ctx,
		job))

	jobs, err := q.ListJobsByTag(ctx, projID, "note", "hello world: special & chars <> 🚀", 50, nil)
	require.NoError(t, err)
	require.Len(t, jobs, 1)

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
	require.NoError(t, q.CreateWorkflow(ctx, w))

	// Key-only search.
	workflows, err := q.ListWorkflowsByTag(ctx, projID, "env", "", 50, nil)
	require.NoError(t, err)
	require.Len(t, workflows,

		1)

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
	require.NoError(t, q.CreateJob(ctx,
		job))

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
	require.NoError(t, pq.Enqueue(ctx,
		run))

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
	require.NoError(t, pq.Enqueue(ctx,
		run2))

	runs, err := q.ListRunsByTag(ctx, projectID, "team", "infra", 100, nil)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	require.Equal(t, run.ID,

		runs[0].
			ID)

}

func TestListWorkflowRunsByTag(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-wfruntag-" + newID()
	wf := &domain.Workflow{ProjectID: projectID, Name: "WF RunTag", Slug: "wf-runtag-" + newID(), Tags: map[string]string{"env": "staging"}, Enabled: true}
	require.NoError(t, q.CreateWorkflow(ctx, wf))

	wfRun := &domain.WorkflowRun{
		WorkflowID:      wf.ID,
		ProjectID:       projectID,
		Tags:            map[string]string{"release": "v2"},
		Status:          domain.WfStatusPending,
		TriggeredBy:     domain.TriggerManual,
		WorkflowVersion: wf.Version,
	}
	require.NoError(t, q.CreateWorkflowRun(ctx,
		wfRun))

	wfRun2 := &domain.WorkflowRun{
		WorkflowID:      wf.ID,
		ProjectID:       projectID,
		Tags:            map[string]string{"release": "v1"},
		Status:          domain.WfStatusPending,
		TriggeredBy:     domain.TriggerManual,
		WorkflowVersion: wf.Version,
	}
	require.NoError(t, q.CreateWorkflowRun(ctx,
		wfRun2))

	runs, err := q.ListWorkflowRunsByTag(ctx, projectID, "release", "v2", 100, nil)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	require.Equal(t, wfRun.
		ID,
		runs[0].ID)

}

func TestAuditEvents_CreateAndListFiltersAndSort(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-audit-events-" + newID()
	base := time.Now().UTC().Add(-10 * time.Minute).Truncate(time.Microsecond)

	ev1 := &domain.AuditEvent{ProjectID: projectID, ActorID: "actor-a", ActorType: "user", Action: "job.create", ResourceType: "job", ResourceID: "job-1"}
	require.NoError(t, q.CreateAuditEvent(ctx,
		ev1))
	require.Equal(t, "{}",
		string(ev1.
			Details),
	)

	ev2 := &domain.AuditEvent{ProjectID: projectID, ActorID: "actor-a", ActorType: "user", Action: "job.update", ResourceType: "job", ResourceID: "job-2", Details: json.RawMessage(`{"changed":true}`)}
	require.NoError(t, q.CreateAuditEvent(ctx,
		ev2))

	ev3 := &domain.AuditEvent{ProjectID: projectID, ActorID: "actor-b", ActorType: "api_key", Action: "workflow.update", ResourceType: "workflow", ResourceID: "wf-1"}
	require.NoError(t, q.CreateAuditEvent(ctx,
		ev3))

	evOther := &domain.AuditEvent{ProjectID: "project-other-audit-" + newID(), ActorID: "actor-a", ActorType: "user", Action: "job.create", ResourceType: "job", ResourceID: "job-x"}
	require.NoError(t, q.CreateAuditEvent(ctx,
		evOther))

	if _, err := testDB.Pool.Exec(ctx, `UPDATE audit_events SET created_at = $2 WHERE id = $1`, ev1.ID, base.Add(1*time.Minute)); err != nil {
		require.Failf(t, "test failure",

			"set created_at ev1 error = %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `UPDATE audit_events SET created_at = $2 WHERE id = $1`, ev2.ID, base.Add(2*time.Minute)); err != nil {
		require.Failf(t, "test failure",

			"set created_at ev2 error = %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `UPDATE audit_events SET created_at = $2 WHERE id = $1`, ev3.ID, base.Add(3*time.Minute)); err != nil {
		require.Failf(t, "test failure",

			"set created_at ev3 error = %v", err)
	}

	allDesc, err := q.ListAuditEvents(ctx, projectID, "", "", "", 10, nil, nil, nil, false)
	require.NoError(t, err)
	require.Len(t, allDesc,

		3)
	require.False(t, allDesc[0].ID !=
		ev3.ID ||
		allDesc[1].ID != ev2.ID ||
		allDesc[2].ID !=
			ev1.ID,
	)

	allAsc, err := q.ListAuditEvents(ctx, projectID, "", "", "", 10, nil, nil, nil, true)
	require.NoError(t, err)
	require.Len(t, allAsc,
		3,
	)
	require.False(t, allAsc[0].ID !=
		ev1.ID ||
		allAsc[1].
			ID != ev2.ID || allAsc[2].ID !=
		ev3.
			ID)

	actorA, err := q.ListAuditEvents(ctx, projectID, "actor-a", "", "", 10, nil, nil, nil, false)
	require.NoError(t, err)
	require.Len(t, actorA,
		2,
	)

	resource, err := q.ListAuditEvents(ctx, projectID, "", "job", "job-2", 10, nil, nil, nil, false)
	require.NoError(t, err)
	require.False(t, len(resource) !=
		1 || resource[0].ID !=
		ev2.ID)

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
		require.NoError(t, q.CreateAuditEvent(ctx,
			ev))

		ids = append(ids, ev.ID)
		if _, err := testDB.Pool.Exec(ctx, `UPDATE audit_events SET created_at = $2 WHERE id = $1`, ev.ID, base.Add(time.Duration(i+1)*time.Minute)); err != nil {
			require.Failf(t, "test failure",

				"set created_at(%d) error = %v", i, err)
		}
	}

	page1, err := q.ListAuditEvents(ctx, projectID, "", "", "", 2, nil, nil, nil, false)
	require.NoError(t, err)
	require.Len(t, page1, 2)

	cursor := page1[len(page1)-1].CreatedAt
	page2, err := q.ListAuditEvents(ctx, projectID, "", "", "", 2, &cursor, nil, nil, false)
	require.NoError(t, err)
	require.Len(t, page2, 2)

	for _, p1 := range page1 {
		for _, p2 := range page2 {
			require.NotEqual(t, p2.
				ID,
				p1.ID)

		}
	}

	from := base.Add(2 * time.Minute)
	to := base.Add(3 * time.Minute)
	ranged, err := q.ListAuditEvents(ctx, projectID, "", "", "", 10, nil, &from, &to, true)
	require.NoError(t, err)
	require.Len(t, ranged,
		2,
	)
	require.False(t, ranged[0].ID !=
		ids[1] ||
		ranged[1].
			ID != ids[2])

}

func TestTagPolicy_CreateListDelete(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-tag-policy-" + newID()
	userID := "user-" + newID()

	p1 := &domain.TagPolicy{ProjectID: projectID, ResourceType: "job", UserID: userID, TagKey: "team", TagValue: "payments", Actions: []string{"jobs:read", "jobs:trigger"}}
	require.NoError(t, q.CreateTagPolicy(ctx,
		p1))
	require.False(t, p1.ID ==
		"" || p1.
		CreatedAt.
		IsZero(),
	)

	p2 := &domain.TagPolicy{ProjectID: projectID, ResourceType: "job", UserID: userID, TagKey: "env", Actions: []string{"jobs:read"}}
	require.NoError(t, q.CreateTagPolicy(ctx,
		p2))

	list, err := q.ListTagPolicies(ctx, projectID, "job", userID, 10, nil)
	require.NoError(t, err)
	require.Len(t, list, 2)

	cursor := list[len(list)-1].CreatedAt
	next, err := q.ListTagPolicies(ctx, projectID, "job", userID, 10, &cursor)
	require.NoError(t, err)
	require.Len(t, next, 0)

	deletedProjectID, deletedUserID, err := q.DeleteTagPolicy(ctx, projectID, p1.ID)
	require.NoError(t, err)
	require.False(t, deletedProjectID !=
		projectID ||
		deletedUserID !=
			userID,
	)

	_, _, err = q.DeleteTagPolicy(ctx, projectID, p1.ID)
	require.True(t, errors.Is(err, store.
		ErrTagPolicyNotFound,
	))

}

func TestTagPolicy_DeleteIsProjectScoped(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	p := &domain.TagPolicy{ProjectID: "project-tag-scope-a", ResourceType: "job", UserID: "user-tag-scope", TagKey: "team", TagValue: "payments", Actions: []string{"jobs:read"}}
	require.NoError(t, q.CreateTagPolicy(ctx,
		p))

	if _, _, err := q.DeleteTagPolicy(ctx, "project-tag-scope-b", p.ID); !errors.Is(err, store.ErrTagPolicyNotFound) {
		require.Failf(t, "test failure",

			"DeleteTagPolicy(cross-project) error = %v, want ErrTagPolicyNotFound", err)
	}
	if _, _, err := q.DeleteTagPolicy(ctx, "project-tag-scope-a", p.ID); err != nil {
		require.Failf(t, "test failure",

			"DeleteTagPolicy(owner project) error = %v", err)
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
	for _, p := range policies {
		require.NoError(t, q.CreateTagPolicy(ctx,
			p))

	}

	tags := map[string]string{"team": "payments", "env": "prod"}
	actions, err := q.GetTagPolicyActions(ctx, projectID, "job", userID, tags)
	require.NoError(t, err)
	require.Len(t, actions,

		3)

	want := map[string]bool{"jobs:read": true, "jobs:trigger": true, "jobs:write": true}
	for _, action := range actions {
		require.True(t, want[action])

		delete(want, action)
	}
	require.Len(t, want, 0)

	none, err := q.GetTagPolicyActions(ctx, projectID, "job", userID, map[string]string{"team": "core"})
	require.NoError(t, err)
	require.Nil(t, none)

}

func TestSeedProjectSystemRoles_Idempotent(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-seed-roles-" + newID()
	require.NoError(t, q.SeedProjectSystemRoles(ctx, projectID))
	require.NoError(t, q.SeedProjectSystemRoles(ctx, projectID))

	roles, err := q.ListProjectRoles(ctx, projectID, 50, nil)
	require.NoError(t, err)
	require.Len(t, roles, len(domain.
		SystemRolePermissions,
	))

	seen := make(map[string]bool, len(roles))
	for _, role := range roles {
		seen[role.Name] = true
		require.True(t, role.IsSystem)

	}
	for name := range domain.SystemRolePermissions {
		require.True(t, seen[name])

	}
}

func TestGetAPIKeyByID(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	key := &domain.APIKey{ProjectID: "project-api-by-id-" + newID(), Name: "key-by-id", KeyHash: "hash-" + newID(), KeyPrefix: "sk_by_id", Scopes: []string{"jobs:read"}}
	require.NoError(t, q.CreateAPIKey(ctx, key))

	got, err := q.GetAPIKeyByID(ctx, key.ID)
	require.NoError(t, err)
	require.False(t, got.ID !=
		key.ID ||
		got.ProjectID !=
			key.ProjectID ||
		got.KeyHash !=
			key.
				KeyHash,
	)

}

func TestMarkAPIKeyRotated(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-api-rotate-" + newID()
	oldKey := &domain.APIKey{ProjectID: projectID, Name: "old", KeyHash: "hash-" + newID(), KeyPrefix: "sk_old", Scopes: []string{"jobs:read"}}
	newKey := &domain.APIKey{ProjectID: projectID, Name: "new", KeyHash: "hash-" + newID(), KeyPrefix: "sk_new", Scopes: []string{"jobs:read"}}
	require.NoError(t, q.CreateAPIKey(ctx, oldKey))
	require.NoError(t, q.CreateAPIKey(ctx, newKey))

	grace := time.Now().UTC().Add(30 * time.Minute).Truncate(time.Microsecond)
	require.NoError(t, q.MarkAPIKeyRotated(ctx,
		oldKey.ID,
		newKey.ID, grace,
	))

	got, err := q.GetAPIKeyByID(ctx, oldKey.ID)
	require.NoError(t, err)
	require.Equal(t, newKey.
		ID, got.ReplacedByKeyID,
	)
	require.False(t, got.GraceExpiresAt ==
		nil ||
		!got.GraceExpiresAt.
			Equal(grace))

}

func TestCreateRotatedAPIKey_AtomicallyCreatesAndLinks(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-api-create-rotated-" + newID()
	oldKey := &domain.APIKey{ProjectID: projectID, Name: "old", KeyHash: "hash-" + newID(), KeyPrefix: "sk_old", Scopes: []string{"jobs:read"}}
	newKey := &domain.APIKey{ProjectID: projectID, Name: "new", KeyHash: "hash-" + newID(), KeyPrefix: "sk_new", Scopes: []string{"jobs:read"}}
	require.NoError(t, q.CreateAPIKey(ctx, oldKey))

	grace := time.Now().UTC().Add(30 * time.Minute).Truncate(time.Microsecond)
	require.NoError(t, q.CreateRotatedAPIKey(ctx,
		oldKey.
			ID, newKey, grace,
	))
	require.NotEqual(t, "",

		newKey.ID,
	)

	oldStored, err := q.GetAPIKeyByID(ctx, oldKey.ID)
	require.NoError(t, err)
	require.Equal(t, newKey.
		ID, oldStored.
		ReplacedByKeyID,
	)

	newStored, err := q.GetAPIKeyByID(ctx, newKey.ID)
	require.NoError(t, err)
	require.Nil(t, newStored.
		RevokedAt,
	)

}

func TestCreateRotatedAPIKey_RollsBackNewKeyWhenOldKeyCannotBeLinked(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-api-create-rotated-rollback-" + newID()
	newKey := &domain.APIKey{ProjectID: projectID, Name: "new", KeyHash: "hash-" + newID(), KeyPrefix: "sk_new", Scopes: []string{"jobs:read"}}
	require.Error(t, q.CreateRotatedAPIKey(ctx,
		"missing-old-key",
		newKey,
		time.Now().UTC().
			Add(time.
				Hour)),
	)
	require.NotEqual(t, "",

		newKey.ID,
	)

	if _, err := q.GetAPIKeyByID(ctx, newKey.ID); err == nil {
		require.Fail(t,

			"rolled-back rotated key remained queryable by ID")
	}
	if _, err := q.GetAPIKeyByHash(ctx, newKey.KeyHash); err == nil {
		require.Fail(t,

			"rolled-back rotated key remained queryable by hash")
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
	require.NoError(t, q.UpdateJob(ctx,
		job))

	v1, err := q.GetJobAtVersion(ctx, job.ID, 1)
	require.NoError(t, err)
	require.False(t, v1.Name !=
		jobV1Name ||
		v1.
			EndpointURL !=
			jobV1Endpoint ||
		v1.Version !=
			1)

	v2, err := q.GetJobAtVersion(ctx, job.ID, job.Version)
	require.NoError(t, err)
	require.False(t, v2.Name !=
		job.Name ||
		v2.
			EndpointURL !=
			job.EndpointURL,
	)

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
	require.NoError(t, q.UpdateJob(ctx,
		job))

	versions, err := q.ListJobVersionsByJob(ctx, job.ID, 10, nil)
	require.NoError(t, err)
	require.Len(t, versions,

		1)
	require.NotEqual(t, "",

		versions[0].VersionID,
	)

	got, err := q.GetJobVersionByVersionID(ctx, versions[0].VersionID)
	require.NoError(t, err)
	require.False(t, got.ID !=
		versions[0].ID ||
		got.Version !=
			1 || got.Name !=
		"job-version-id-v1",
	)

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
		require.Failf(t, "test failure",

			"set workflow cron fields error = %v", err)
	}
	wf.Cron = "*/5 * * * *"
	wf.CronTimezone = "UTC"

	stepRef := "step-" + newID()
	require.NoError(t, q.CreateWorkflowStep(ctx,
		&domain.
			WorkflowStep{ID: newID(), WorkflowID: wf.
			ID, JobID: job.ID, StepRef: stepRef,
		}))
	require.NoError(t, q.CreateWorkflowVersionSnapshot(ctx,
		wf.ID, wf.Version,
	))

	v1VersionID := "wfv1-" + newID()
	if _, err := testDB.Pool.Exec(ctx, `UPDATE workflow_versions SET version_id = $3 WHERE workflow_id = $1 AND version = $2`, wf.ID, 1, v1VersionID); err != nil {
		require.Failf(t, "test failure",

			"set workflow version_id v1 error = %v", err)
	}

	wf.Name = "wf-versions-v2"
	require.NoError(t, q.UpdateWorkflow(ctx, wf))
	require.NoError(t, q.CreateWorkflowVersionSnapshot(ctx,
		wf.ID, wf.Version,
	))

	v2VersionID := "wfv2-" + newID()
	if _, err := testDB.Pool.Exec(ctx, `UPDATE workflow_versions SET version_id = $3 WHERE workflow_id = $1 AND version = $2`, wf.ID, 2, v2VersionID); err != nil {
		require.Failf(t, "test failure",

			"set workflow version_id v2 error = %v", err)
	}

	versions, err := q.ListWorkflowVersions(ctx, wf.ID, 10)
	require.NoError(t, err)
	require.Len(t, versions,

		2)
	require.False(t, versions[0].Version !=
		2 ||
		versions[1].Version != 1)

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
		require.Failf(t, "test failure",

			"set workflow cron fields error = %v", err)
	}
	require.NoError(t, q.CreateWorkflowVersionSnapshot(ctx,
		wf.ID, wf.Version,
	))

	versionID := "wfid-" + newID()
	if _, err := testDB.Pool.Exec(ctx, `UPDATE workflow_versions SET version_id = $3 WHERE workflow_id = $1 AND version = $2`, wf.ID, wf.Version, versionID); err != nil {
		require.Failf(t, "test failure",

			"set workflow version_id error = %v", err)
	}

	versions, err := q.ListWorkflowVersions(ctx, wf.ID, 10)
	require.NoError(t, err)
	require.Len(t, versions,

		1)
	require.NotEqual(t, "",

		versions[0].VersionID,
	)

	got, err := q.GetWorkflowVersionByVersionID(ctx, wf.ID, versions[0].VersionID)
	require.NoError(t, err)
	require.False(t, got.ID !=
		versions[0].ID ||
		got.WorkflowID !=
			wf.ID ||
		got.Version !=
			wf.Version,
	)

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
	require.NoError(t, q.CreateWebhookSubscription(ctx, sub))

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
	require.NoError(t, q.CreateWebhookDelivery(ctx, failed))

	retried, err := q.RetryWebhookDelivery(ctx, failed.ID)
	require.NoError(t, err)
	require.False(t, retried.
		Status !=
		domain.
			WebhookStatusPending ||
		retried.
			Attempts !=
			0,
	)
	require.NotNil(t, retried.
		NextRetryAt,
	)
	require.False(t, retried.
		LastStatusCode !=
		nil || retried.
		LastError !=
		"" || retried.
		DeliveredAt !=
		nil,
	)

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
	require.NoError(t, q.CreateWebhookSubscription(ctx, sub))

	past := time.Now().UTC().Add(-2 * time.Minute)
	future := time.Now().UTC().Add(20 * time.Minute)
	due := &domain.WebhookDelivery{RunID: run.ID, JobID: job.ID, WebhookURL: job.WebhookURL, Status: domain.WebhookStatusPending, Attempts: 1, MaxAttempts: 5, NextRetryAt: &past}
	notDue := &domain.WebhookDelivery{RunID: run.ID, JobID: job.ID, WebhookURL: job.WebhookURL, Status: domain.WebhookStatusPending, Attempts: 1, MaxAttempts: 5, NextRetryAt: &future}
	failedDue := &domain.WebhookDelivery{RunID: run.ID, JobID: job.ID, WebhookURL: job.WebhookURL, Status: domain.WebhookStatusFailed, Attempts: 1, MaxAttempts: 5, NextRetryAt: &past}
	for _, d := range []*domain.WebhookDelivery{due, notDue, failedDue} {
		require.NoError(t, q.CreateWebhookDelivery(ctx, d))

	}

	pending, err := q.ListPendingWebhookRetries(ctx)
	require.NoError(t, err)
	require.False(t, len(pending) !=
		1 || pending[0].ID !=
		due.ID)

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
	require.NoError(t, q.CreateWebhookDelivery(ctx, due))

	claimed, err := q.ClaimPendingWebhookRetries(ctx, 10, time.Minute)
	require.NoError(t, err)
	require.False(t, len(claimed) !=
		1 || claimed[0].ID !=
		due.ID)
	require.False(t, claimed[0].ClaimToken ==
		"" || claimed[0].LeaseExpiresAt ==
		nil,
	)
	require.Equal(t, due.WebhookSecret,

		claimed[0].WebhookSecret,
	)

	claimedAgain, err := q.ClaimPendingWebhookRetries(ctx, 10, time.Minute)
	require.NoError(t, err)
	require.Len(t, claimedAgain,

		0)

	listed, err := q.ListPendingWebhookRetries(ctx)
	require.NoError(t, err)
	require.Len(t, listed,
		0,
	)

	wrongToken := claimed[0]
	wrongToken.ClaimToken = "wrong-token"
	wrongToken.Status = domain.WebhookStatusDelivered
	now := time.Now().UTC()
	wrongToken.DeliveredAt = &now
	updated, err := q.UpdateClaimedWebhookDelivery(ctx, &wrongToken)
	require.NoError(t, err)
	require.False(t, updated)

	if _, err := testDB.Pool.Exec(ctx, `
		UPDATE webhook_deliveries
		SET lease_expires_at = NOW() - INTERVAL '1 second'
		WHERE id = $1`, due.ID); err != nil {
		require.Failf(t, "test failure",

			"expire webhook claim lease: %v", err)
	}

	reclaimed, err := q.ClaimPendingWebhookRetries(ctx, 10, time.Minute)
	require.NoError(t, err)
	require.False(t, len(reclaimed) !=
		1 || reclaimed[0].
		ID != due.ID)
	require.NotEqual(t, claimed[0].ClaimToken,

		reclaimed[0].ClaimToken)

	statusCode := 200
	reclaimed[0].Status = domain.WebhookStatusDelivered
	reclaimed[0].DeliveredAt = &now
	reclaimed[0].NextRetryAt = nil
	reclaimed[0].LastStatusCode = &statusCode
	updated, err = q.UpdateClaimedWebhookDelivery(ctx, &reclaimed[0])
	require.NoError(t, err)
	require.True(t, updated)

	got, err := q.GetWebhookDelivery(ctx, due.ID)
	require.NoError(t, err)
	require.False(t, got.Status !=
		domain.
			WebhookStatusDelivered ||
		got.DeliveredAt ==
			nil ||
		got.
			NextRetryAt !=
			nil)

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
	require.NoError(t, q.CreateWebhookSubscription(ctx, sub))

	deliveredOld := &domain.WebhookDelivery{RunID: run.ID, JobID: job.ID, WebhookURL: job.WebhookURL, Status: domain.WebhookStatusDelivered, Attempts: 1, MaxAttempts: 3}
	deadOld := &domain.WebhookDelivery{RunID: run.ID, JobID: job.ID, WebhookURL: job.WebhookURL, Status: domain.WebhookStatusDead, Attempts: 3, MaxAttempts: 3}
	deliveredRecent := &domain.WebhookDelivery{RunID: run.ID, JobID: job.ID, WebhookURL: job.WebhookURL, Status: domain.WebhookStatusDelivered, Attempts: 1, MaxAttempts: 3}
	pendingOld := &domain.WebhookDelivery{RunID: run.ID, JobID: job.ID, WebhookURL: job.WebhookURL, Status: domain.WebhookStatusPending, Attempts: 1, MaxAttempts: 3}
	for _, d := range []*domain.WebhookDelivery{deliveredOld, deadOld, deliveredRecent, pendingOld} {
		require.NoError(t, q.CreateWebhookDelivery(ctx, d))

	}

	old := time.Now().UTC().Add(-48 * time.Hour)
	recent := time.Now().UTC().Add(-15 * time.Minute)
	if _, err := testDB.Pool.Exec(ctx, `UPDATE webhook_deliveries SET created_at = $2 WHERE id = $1`, deliveredOld.ID, old); err != nil {
		require.Failf(t, "test failure",

			"set created_at deliveredOld error = %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `UPDATE webhook_deliveries SET created_at = $2 WHERE id = $1`, deadOld.ID, old.Add(1*time.Minute)); err != nil {
		require.Failf(t, "test failure",

			"set created_at deadOld error = %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `UPDATE webhook_deliveries SET created_at = $2 WHERE id = $1`, deliveredRecent.ID, recent); err != nil {
		require.Failf(t, "test failure",

			"set created_at deliveredRecent error = %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `UPDATE webhook_deliveries SET created_at = $2 WHERE id = $1`, pendingOld.ID, old); err != nil {
		require.Failf(t, "test failure",

			"set created_at pendingOld error = %v", err)
	}

	deleted, err := q.DeleteOldWebhookDeliveries(ctx, time.Now().UTC().Add(-24*time.Hour), 10)
	require.NoError(t, err)
	require.EqualValues(t, 2, deleted)

	if _, err := q.GetWebhookDelivery(ctx, deliveredOld.ID); err == nil {
		require.Fail(t,

			"GetWebhookDelivery(deliveredOld) error = nil, want error")
	}
	if _, err := q.GetWebhookDelivery(ctx, deadOld.ID); err == nil {
		require.Fail(t,

			"GetWebhookDelivery(deadOld) error = nil, want error")
	}
	if _, err := q.GetWebhookDelivery(ctx, deliveredRecent.ID); err != nil {
		require.Failf(t, "test failure",

			"GetWebhookDelivery(deliveredRecent) error = %v", err)
	}
	if _, err := q.GetWebhookDelivery(ctx, pendingOld.ID); err != nil {
		require.Failf(t, "test failure",

			"GetWebhookDelivery(pendingOld) error = %v", err)
	}
}

func TestPauseJobsByGroup(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-pause-group-" + newID()
	group := &domain.JobGroup{ID: newID(), ProjectID: projectID, Name: "pause-group", Slug: "pause-group-" + newID()}
	require.NoError(t, q.CreateJobGroup(ctx, group))

	inGroupA := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: new(projectID), Enabled: new(true)})
	inGroupB := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: new(projectID), Enabled: new(true)})
	outside := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: new(projectID), Enabled: new(true)})
	for i, jobID := range []string{inGroupA.ID, inGroupB.ID} {
		if _, err := testDB.Pool.Exec(ctx, `UPDATE jobs SET group_id = $1 WHERE id = $2`, group.ID, jobID); err != nil {
			require.Failf(t, "test failure",

				"assign group (%d) error = %v", i, err)
		}
	}
	require.NoError(t, q.PauseJobsByGroup(ctx,
		group.ID))

	gotA, err := q.GetJob(ctx, inGroupA.ID)
	require.NoError(t, err)

	gotB, err := q.GetJob(ctx, inGroupB.ID)
	require.NoError(t, err)

	gotOut, err := q.GetJob(ctx, outside.ID)
	require.NoError(t, err)
	require.False(t, !gotA.
		Paused ||
		!gotB.Paused,
	)
	require.False(t, gotOut.
		Paused)

}

func TestResumeJobsByGroup(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-resume-group-" + newID()
	group := &domain.JobGroup{ID: newID(), ProjectID: projectID, Name: "resume-group", Slug: "resume-group-" + newID()}
	require.NoError(t, q.CreateJobGroup(ctx, group))

	inGroupA := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: new(projectID), Enabled: new(true)})
	inGroupB := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: new(projectID), Enabled: new(true)})
	for i, jobID := range []string{inGroupA.ID, inGroupB.ID} {
		if _, err := testDB.Pool.Exec(ctx, `UPDATE jobs SET group_id = $1 WHERE id = $2`, group.ID, jobID); err != nil {
			require.Failf(t, "test failure",

				"assign group (%d) error = %v", i, err)
		}
	}
	require.NoError(t, q.PauseJobsByGroup(ctx,
		group.ID))
	require.NoError(t, q.ResumeJobsByGroup(ctx,
		group.ID),
	)

	// Pause first, then resume.

	gotA, err := q.GetJob(ctx, inGroupA.ID)
	require.NoError(t, err)

	gotB, err := q.GetJob(ctx, inGroupB.ID)
	require.NoError(t, err)
	require.False(t, gotA.Paused ||
		gotB.
			Paused,
	)

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
		require.NoError(t, pq.Enqueue(ctx,
			run))

		startedAt := now.Add(-time.Duration(i+12) * time.Minute)
		finishedAt := startedAt.Add(time.Duration(20+i*5) * time.Second)
		createdAt := startedAt.Add(-15 * time.Second)
		if _, err := testDB.Pool.Exec(ctx, `
			UPDATE job_runs
			SET status = $2, started_at = $3, finished_at = $4, created_at = $5
			WHERE id = $1
		`, run.ID, st, startedAt, finishedAt, createdAt); err != nil {
			require.Failf(t, "test failure",

				"update run(%d) status/times error = %v", i, err)
		}
	}

	analytics, err := q.GetPerformanceAnalytics(ctx, projectID, 24)
	require.NoError(t, err)
	require.False(t, analytics.
		Throughput.
		Completed !=
		3 ||
		analytics.Throughput.
			Failed !=
			2 || analytics.
		Throughput.
		TimedOut !=
		1)
	require.EqualValues(t, 24, analytics.
		Throughput.
		PeriodHours,
	)
	require.Len(t, analytics.
		SlowestJobs,
		1)
	require.False(t, analytics.
		SlowestJobs[0].
		JobID != job.
		ID || analytics.
		SlowestJobs[0].TotalRuns !=
		6 ||
		analytics.
			SlowestJobs[0].FailedRuns !=
			2,
	)
	require.False(t, analytics.
		HealthSummary.
		TotalJobs !=
		1 || analytics.HealthSummary.
		ActiveJobs !=
		1)
	require.False(t, analytics.
		HealthSummary.
		SuccessRate <
		0.49 || analytics.
		HealthSummary.
		SuccessRate >
		0.51,
	)

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
	for _, run := range []*domain.JobRun{runRecentCompleted, runRecentFailed, runOldCompleted} {
		require.NoError(t, pq.Enqueue(ctx,
			run))

	}

	recentStartA := now.Add(-8 * time.Minute)
	recentEndA := recentStartA.Add(20 * time.Second)
	recentStartB := now.Add(-6 * time.Minute)
	recentEndB := recentStartB.Add(30 * time.Second)
	oldStart := now.Add(-4 * time.Hour)
	oldEnd := oldStart.Add(40 * time.Second)

	if _, err := testDB.Pool.Exec(ctx, `UPDATE job_runs SET status = 'completed', created_at = $2, started_at = $3, finished_at = $4 WHERE id = $1`, runRecentCompleted.ID, now.Add(-8*time.Minute), recentStartA, recentEndA); err != nil {
		require.Failf(t, "test failure",

			"update runRecentCompleted error = %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `UPDATE job_runs SET status = 'failed', created_at = $2, started_at = $3, finished_at = $4 WHERE id = $1`, runRecentFailed.ID, now.Add(-6*time.Minute), recentStartB, recentEndB); err != nil {
		require.Failf(t, "test failure",

			"update runRecentFailed error = %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `UPDATE job_runs SET status = 'completed', created_at = $2, started_at = $3, finished_at = $4 WHERE id = $1`, runOldCompleted.ID, now.Add(-4*time.Hour), oldStart, oldEnd); err != nil {
		require.Failf(t, "test failure",

			"update runOldCompleted error = %v", err)
	}

	stats, err := q.GetJobHealthStats(ctx, job.ID, now.Add(-1*time.Hour))
	require.NoError(t, err)
	require.False(t, stats.
		TotalRuns !=
		2 || stats.
		CompletedRuns !=
		1 || stats.
		FailedRuns !=
		1)
	require.False(t, stats.
		SuccessRate <
		49.99 ||
		stats.SuccessRate >
			50.01,
	)

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
	require.NoError(t, q.CreateEventTrigger(ctx,
		trigger),
	)

	got, err := q.GetEventTriggerByEventKey(ctx, trigger.EventKey)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, trigger.
		ID, got.
		ID)
	require.Equal(t, run.ID,

		got.JobRunID,
	)
	require.True(t, jsonEqual(got.RequestPayload,

		trigger.
			RequestPayload))

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
	require.NoError(t, q.CreateEventTrigger(ctx,
		foreignTrigger,
	))

	got, err := q.GetEventTriggerByEventKeyForProject(ctx, foreignTrigger.EventKey, "proj-event-trigger-local-"+newID())
	require.NoError(t, err)
	require.Nil(t, got)

	got, err = q.GetEventTriggerByEventKeyForProject(ctx, foreignTrigger.EventKey, foreignProjectID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, foreignTrigger.
		ID,
		got.ID,
	)

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
	require.NoError(t, q.CreateEventTrigger(ctx,
		triggerA,
	))
	require.NoError(t, q.CreateEventTrigger(ctx,
		triggerB,
	))

	gotA, err := q.GetEventTriggerByEventKeyForProject(ctx, eventKey, projectA)
	require.NoError(t, err)
	require.False(t, gotA ==

		nil || gotA.
		ID !=
		triggerA.ID,
	)

	gotB, err := q.GetEventTriggerByEventKeyForProject(ctx, eventKey, projectB)
	require.NoError(t, err)
	require.False(t, gotB ==

		nil || gotB.
		ID !=
		triggerB.ID,
	)

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
	require.NoError(t, q.CreateEventTrigger(ctx,
		first))

	err := q.CreateEventTrigger(ctx, &second)
	require.True(t, errors.Is(err, store.
		ErrEventKeyConflict,
	))
	require.False(t, strings.Contains(err.Error(), eventKey))

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
	require.NoError(t, q.CreateEventTrigger(ctx,
		trigger),
	)

	got, err := q.GetEventTriggerByStepRunID(ctx, stepRun.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, trigger.
		ID, got.
		ID)

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
	require.NoError(t, q.CreateEventTrigger(ctx,
		trigger),
	)

	got, err := q.GetEventTriggerByJobRunID(ctx, run.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, trigger.
		ID, got.
		ID)

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
	require.NoError(t, q.UpdateEventTriggerStatus(ctx, trigger.
		ID, domain.
		EventTriggerStatusReceived,

		response,
		&receivedAt,
		errMsg,
	))

	got, err := q.GetEventTriggerByEventKey(ctx, trigger.EventKey)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, domain.
		EventTriggerStatusReceived,

		got.
			Status)
	require.False(t, got.ReceivedAt ==
		nil ||
		!got.ReceivedAt.
			Equal(receivedAt))
	require.True(t, jsonEqual(got.ResponsePayload,

		response,
	))
	require.Equal(t, errMsg,

		got.Error,
	)

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
	require.True(t, errors.Is(err, store.
		ErrEventTriggerConflict,
	))

	got, err := q.GetEventTriggerByEventKey(ctx, trigger.EventKey)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, domain.
		EventTriggerStatusReceived,

		got.
			Status)

}

func TestSetEventTriggerSentBy(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-event-trigger-sent-by-" + newID()
	_, run := mustCreateJobRunWithBuildFactory(t, ctx, q, projectID, domain.StatusWaiting)
	trigger := mustCreateJobRunEventTrigger(t, ctx, q, projectID, run.ID, domain.EventTriggerStatusWaiting, "evt-sent-by-"+newID(), time.Now().UTC(), nil, nil)
	require.NoError(t, q.SetEventTriggerSentBy(ctx, trigger.
		ID, "api-key-123",
	))

	got, err := q.GetEventTriggerByEventKey(ctx, trigger.EventKey)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "api-key-123",

		got.
			SentBy,
	)

}

func TestUpdateEventTriggerNotifyStatus(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-event-trigger-notify-status-" + newID()
	_, run := mustCreateJobRunWithBuildFactory(t, ctx, q, projectID, domain.StatusWaiting)
	trigger := mustCreateJobRunEventTrigger(t, ctx, q, projectID, run.ID, domain.EventTriggerStatusWaiting, "evt-notify-status-"+newID(), time.Now().UTC(), nil, nil)
	require.NoError(t, q.UpdateEventTriggerNotifyStatus(ctx,
		trigger.ID, "sent",
	))

	got, err := q.GetEventTriggerByEventKey(ctx, trigger.EventKey)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "sent",

		got.NotifyStatus,
	)

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
	require.NoError(t, err)
	require.Len(t, waiting,

		2)

	byWorkflowRun, err := q.ListEventTriggersByProject(ctx, projectID, "", "", wfRun.ID, "", 20, nil)
	require.NoError(t, err)
	require.Len(t, byWorkflowRun,

		2)

	bySourceType, err := q.ListEventTriggersByProject(ctx, projectID, "", "", "", "job_run", 20, nil)
	require.NoError(t, err)
	require.Len(t, bySourceType,

		1)
	require.Equal(t, "job_run",

		bySourceType[0].SourceType,
	)

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
	require.NoError(t, q.CreateEventTrigger(ctx,
		prod))
	require.NoError(t, q.CreateEventTrigger(ctx,
		staging),
	)

	all, err := q.ListEventTriggersByProject(ctx, projectID, "", domain.EventTriggerStatusWaiting, "", "", 10, nil)
	require.NoError(t, err)
	require.Len(t, all, 2)

	filtered, err := q.ListEventTriggersByProject(ctx, projectID, "env-prod", domain.EventTriggerStatusWaiting, "", "", 10, nil)
	require.NoError(t, err)
	require.Len(t, filtered,

		1)
	require.Equal(t, prod.ID,

		filtered[0].ID)

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
		require.NoError(t, q.CreateEventTrigger(ctx,
			trigger),
		)

	}

	all, err := q.GetEventTriggerStats(ctx, projectID, "")
	require.NoError(t, err)
	require.False(t, all.TotalCount !=
		3 || all.
		WaitingCount !=
		2 || all.ReceivedCount !=
		1,
	)

	prod, err := q.GetEventTriggerStats(ctx, projectID, "env-prod")
	require.NoError(t, err)
	require.False(t, prod.TotalCount !=
		2 || prod.
		WaitingCount !=
		1 || prod.
		ReceivedCount !=
		1)

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
	require.NoError(t, err)
	require.Len(t, global,
		2,
	)

	scoped, err := q.ListEventTriggersByKeyPrefix(ctx, prefix, projectID)
	require.NoError(t, err)
	require.Len(t, scoped,
		1,
	)
	require.Equal(t, matchA.
		ID, scoped[0].ID)
	require.False(t, global[0].ID !=
		matchA.ID &&
		global[1].ID != matchA.ID,
	)
	require.False(t, global[0].ID !=
		matchOtherProject.
			ID &&
		global[1].ID !=
			matchOtherProject.
				ID,
	)

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
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, old.ID,

		list[0].
			ID)

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
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, expired.
		ID, list[0].ID)

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
	require.NoError(t, err)
	require.Len(t, list, 2)

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
	require.False(t, !seenStep ||
		!seenJob,
	)

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
	require.NoError(t, err)
	require.EqualValues(t, 1, affected)

	gotWaiting, err := q.GetEventTriggerByEventKey(ctx, waiting.EventKey)
	require.NoError(t, err)
	require.NotNil(t, gotWaiting)
	require.Equal(t, domain.
		EventTriggerStatusCanceled,

		gotWaiting.
			Status)

	gotReceived, err := q.GetEventTriggerByEventKey(ctx, received.EventKey)
	require.NoError(t, err)
	require.NotNil(t, gotReceived)
	require.Equal(t, domain.
		EventTriggerStatusReceived,

		gotReceived.
			Status,
	)

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
	require.NoError(t, q.CancelEventTriggerByJobRun(ctx,
		run.ID))

	gotWaiting, err := q.GetEventTriggerByEventKey(ctx, waiting.EventKey)
	require.NoError(t, err)
	require.NotNil(t, gotWaiting)
	require.Equal(t, domain.
		EventTriggerStatusCanceled,

		gotWaiting.
			Status)

	gotReceived, err := q.GetEventTriggerByEventKey(ctx, received.EventKey)
	require.NoError(t, err)
	require.NotNil(t, gotReceived)
	require.Equal(t, domain.
		EventTriggerStatusReceived,

		gotReceived.
			Status,
	)

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
	require.NoError(t, err)
	require.EqualValues(t, 3, count)

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
		require.NoError(t, q.CreateEventTrigger(ctx,
			trigger),
		)

	}

	allProject, err := q.CountEventTriggersFinishedBeforeForProject(ctx, projectID, "", now.Add(-time.Hour))
	require.NoError(t, err)
	require.EqualValues(t, 2, allProject)

	prodOnly, err := q.CountEventTriggersFinishedBeforeForProject(ctx, projectID, "env-prod", now.Add(-time.Hour))
	require.NoError(t, err)
	require.EqualValues(t, 1, prodOnly)

	deleted, err := q.DeleteEventTriggersFinishedBeforeForProject(ctx, projectID, "env-prod", now.Add(-time.Hour), 10)
	require.NoError(t, err)
	require.EqualValues(t, 1, deleted)

	if got, err := q.GetEventTriggerByEventKey(ctx, prod.EventKey); err != nil {
		require.Failf(t, "test failure",

			"GetEventTriggerByEventKey(prod) error = %v", err)
	} else if got != nil {
		require.Failf(t, "test failure",

			"prod trigger still exists with ID %q", got.ID)
	}
	if got, err := q.GetEventTriggerByEventKey(ctx, staging.EventKey); err != nil {
		require.Failf(t, "test failure",

			"GetEventTriggerByEventKey(staging) error = %v", err)
	} else if got == nil {
		require.Fail(t,

			"staging trigger was deleted by env-prod purge")
	}
	if got, err := q.GetEventTriggerByEventKey(ctx, otherProject.EventKey); err != nil {
		require.Failf(t, "test failure",

			"GetEventTriggerByEventKey(otherProject) error = %v", err)
	} else if got == nil {
		require.Fail(t,

			"other project trigger was deleted by env-prod purge")
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
	require.NoError(t, err)
	require.EqualValues(t, 1, deleted)

	remainingAfterFirst, err := q.CountEventTriggersFinishedBefore(ctx, now.Add(-time.Hour))
	require.NoError(t, err)
	require.EqualValues(t, 1, remainingAfterFirst)

	deleted, err = q.DeleteEventTriggersFinishedBefore(ctx, now.Add(-time.Hour), 10)
	require.NoError(t, err)
	require.EqualValues(t, 1, deleted)

	if got, err := q.GetEventTriggerByEventKey(ctx, oldA.EventKey); err != nil {
		require.Failf(t, "test failure",

			"GetEventTriggerByEventKey(oldA) error = %v", err)
	} else if got != nil {
		require.Failf(t, "test failure",

			"oldA still exists with ID %q", got.ID)
	}
	if got, err := q.GetEventTriggerByEventKey(ctx, oldB.EventKey); err != nil {
		require.Failf(t, "test failure",

			"GetEventTriggerByEventKey(oldB) error = %v", err)
	} else if got != nil {
		require.Failf(t, "test failure",

			"oldB still exists with ID %q", got.ID)
	}
	if got, err := q.GetEventTriggerByEventKey(ctx, recent.EventKey); err != nil {
		require.Failf(t, "test failure",

			"GetEventTriggerByEventKey(recent) error = %v", err)
	} else if got == nil {
		require.Fail(t,

			"recent trigger should exist")
	}
	if got, err := q.GetEventTriggerByEventKey(ctx, waiting.EventKey); err != nil {
		require.Failf(t, "test failure",

			"GetEventTriggerByEventKey(waiting) error = %v", err)
	} else if got == nil {
		require.Fail(t,

			"waiting trigger should exist")
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
	require.NoError(t, err)
	require.Len(t, updatedIDs,

		2)

	gotA, err := q.GetEventTriggerByEventKey(ctx, triggerA.EventKey)
	require.NoError(t, err)
	require.NotNil(t, gotA)
	require.Equal(t, domain.
		EventTriggerStatusReceived,

		gotA.
			Status)
	require.Equal(t, "batch-sender",

		gotA.SentBy,
	)
	require.True(t, jsonEqual(gotA.ResponsePayload,

		payload,
	))

	gotB, err := q.GetEventTriggerByEventKey(ctx, triggerB.EventKey)
	require.NoError(t, err)
	require.NotNil(t, gotB)
	require.Equal(t, domain.
		EventTriggerStatusReceived,

		gotB.
			Status)
	require.False(t, gotB.ReceivedAt ==
		nil ||
		!gotB.ReceivedAt.
			Equal(receivedAt))

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
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT ready_generation
		FROM job_run_state
		WHERE run_id = $1`,

		run.ID).Scan(&beforeGeneration))

	payload := json.RawMessage(`{"checkpoint":"resume"}`)
	receivedAt := time.Now().UTC()
	require.NoError(t, q.ReceiveEventAndRequeueRun(ctx, trigger.
		ID, payload,
		receivedAt,
		run.
			ID))

	updatedRun, err := q.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		StatusQueued,
		updatedRun.
			Status,
	)

	var ledgerStatus, stateStatus domain.RunStatus
	var afterGeneration int64
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT jr.status, s.status, s.ready_generation
		FROM job_runs jr
		JOIN job_run_state s ON s.run_id = jr.id
		WHERE jr.id = $1`,

		run.ID).Scan(&ledgerStatus,
		&stateStatus, &afterGeneration,
	))
	require.Equal(t, domain.
		StatusWaiting,
		ledgerStatus,
	)
	require.Equal(t, domain.
		StatusQueued,
		stateStatus,
	)
	require.Equal(t, beforeGeneration+
		1, afterGeneration,
	)

	checkpoint, err := q.GetLatestCheckpoint(ctx, run.ID)
	require.NoError(t, err)
	require.NotNil(t, checkpoint)
	require.Equal(t, "event_trigger",

		checkpoint.
			Source)
	require.True(t, jsonEqual(checkpoint.
		State,
		payload))

	updatedTrigger, err := q.GetEventTriggerByEventKey(ctx, trigger.EventKey)
	require.NoError(t, err)
	require.NotNil(t, updatedTrigger)
	require.Equal(t, domain.
		EventTriggerStatusReceived,

		updatedTrigger.
			Status,
	)
	require.True(t, jsonEqual(updatedTrigger.
		ResponsePayload,

		payload))

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
	require.NoError(t, err)
	require.EqualValues(t, 2, count)

}

func TestAdvisoryLockTryAndRelease(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	lockID := int64(12345)
	acquired, err := q.TryAdvisoryLock(ctx, lockID)
	require.NoError(t, err)
	require.True(t, acquired)
	require.NoError(t, q.ReleaseAdvisoryLock(ctx,
		lockID),
	)

}

func TestAdvisoryLockConcurrentAcrossConnections(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	lockID := int64(23456)
	connA, err := testDB.Pool.Acquire(ctx)
	require.NoError(t, err)

	defer connA.Release()

	connB, err := testDB.Pool.Acquire(ctx)
	require.NoError(t, err)

	defer connB.Release()

	qa := store.New(connA)
	qb := store.New(connB)

	acquiredA, err := qa.TryAdvisoryLock(ctx, lockID)
	require.NoError(t, err)
	require.True(t, acquiredA)

	acquiredB, err := qb.TryAdvisoryLock(ctx, lockID)
	require.NoError(t, err)
	require.False(t, acquiredB)
	require.NoError(t, qa.ReleaseAdvisoryLock(
		ctx, lockID,
	))

	acquiredBAfterRelease, err := qb.TryAdvisoryLock(ctx, lockID)
	require.NoError(t, err)
	require.True(t, acquiredBAfterRelease)
	require.NoError(t, qb.ReleaseAdvisoryLock(
		ctx, lockID,
	))
	require.NoError(t, q.ReleaseAdvisoryLock(ctx,
		lockID),
	)

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
		require.Fail(t, "RunWithAdvisoryLock did not start fn")
	}

	acquiredWhileHeld, err := q.TryAdvisoryLock(ctx, lockID)
	require.NoError(t, err)
	require.False(t, acquiredWhileHeld)

	close(releaseFn)
	select {
	case err := <-fnDone:
		if err != nil {
			require.Failf(t, "test failure",

				"RunWithAdvisoryLock() error = %v", err)
		}
	case <-time.After(5 * time.Second):
		require.Fail(t, "RunWithAdvisoryLock did not finish")
	}

	acquiredAfterRelease, err := q.TryAdvisoryLock(ctx, lockID)
	require.NoError(t, err)
	require.True(t, acquiredAfterRelease)
	require.NoError(t, q.ReleaseAdvisoryLock(ctx,
		lockID),
	)

}

func TestRunWithAdvisoryLockReportsNotAcquired(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	lockID := int64(334456)
	conn, err := testDB.Pool.Acquire(ctx)
	require.NoError(t, err)

	defer conn.Release()

	heldByConn := store.New(conn)
	acquired, err := heldByConn.TryAdvisoryLock(ctx, lockID)
	require.NoError(t, err)
	require.True(t, acquired)

	defer func() {
		require.NoError(t, heldByConn.
			ReleaseAdvisoryLock(ctx,
				lockID))

	}()

	ran := false
	acquired, err = q.RunWithAdvisoryLock(ctx, lockID, func(context.Context) error {
		ran = true
		return nil
	})
	require.NoError(t, err)
	require.False(t, acquired)
	require.False(t, ran)

}

func TestRunWithAdvisoryLockReleasesAfterPanic(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	lockID := int64(334457)
	func() {
		defer func() {
			require.NotNil(t, recover())

		}()
		_, _ = q.RunWithAdvisoryLock(ctx, lockID, func(context.Context) error {
			panic("locked section failed")
		})
	}()

	acquiredAfterPanic, err := q.TryAdvisoryLock(ctx, lockID)
	require.NoError(t, err)
	require.True(t, acquiredAfterPanic)
	require.NoError(t, q.ReleaseAdvisoryLock(ctx,
		lockID),
	)

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
	require.NoError(t, err)

	acquired, err := q.TryAdvisoryLock(ctx, lockID)
	require.NoError(t, err)
	require.True(t, acquired)
	require.NoError(t, q.ReleaseAdvisoryLock(ctx,
		lockID),
	)

}

func mustCreateJobRunWithBuildFactory(t *testing.T, ctx context.Context, q *store.Queries, projectID string, status domain.RunStatus) (*domain.Job, *domain.JobRun) {
	t.Helper()

	job := testutil.BuildJob(&testutil.JobOpts{
		ID:        new(newID()),
		ProjectID: new(projectID),
		Name:      new("job-" + newID()),
		Slug:      new("job-slug-" + newID()),
	})
	require.NoError(t, q.CreateJob(ctx,
		job))

	run := testutil.BuildRun(job, &testutil.RunOpts{
		ID:     new(newID()),
		Status: new(status),
	})
	require.NoError(t, q.CreateRun(ctx,
		run))

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
	require.NoError(t, q.CreateEventTrigger(ctx,
		trigger),
	)

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
	require.NoError(t, q.CreateEventTrigger(ctx,
		trigger),
	)

	return trigger
}
