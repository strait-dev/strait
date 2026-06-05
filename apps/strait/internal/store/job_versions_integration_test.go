//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

func TestCreateJobVersion(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-job-version-create")

	v := &domain.JobVersion{
		ID:          newID(),
		JobID:       job.ID,
		Version:     1,
		VersionID:   "vid-1",
		Name:        job.Name,
		Slug:        job.Slug,
		Description: "first version",
		EndpointURL: "https://example.com/v1",
		MaxAttempts: 3,
		TimeoutSecs: 30,
	}
	require.NoError(t, q.CreateJobVersion(ctx, v))
	require.False(t, v.CreatedAt.
		IsZero())

}

func TestGetJobVersion(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-job-version-get")

	v := &domain.JobVersion{
		ID:            newID(),
		JobID:         job.ID,
		Version:       1,
		Name:          job.Name,
		Slug:          job.Slug,
		EndpointURL:   "https://example.com/v1",
		MaxAttempts:   3,
		TimeoutSecs:   30,
		Tags:          map[string]string{"team": "core"},
		PayloadSchema: json.RawMessage(`{"type":"object"}`),
	}
	require.NoError(t, q.CreateJobVersion(ctx, v))

	got, err := q.GetJobVersion(ctx, job.ID, 1)
	require.NoError(t, err)
	require.Equal(t, v.ID,
		got.
			ID)
	require.Equal(t, "core",

		got.Tags["team"])

	// Not found.
	_, err = q.GetJobVersion(ctx, job.ID, 99)
	require.Error(t, err)

}

func TestListJobVersionsByJob(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-job-version-list")

	for i := 1; i <= 3; i++ {
		v := &domain.JobVersion{
			ID:          newID(),
			JobID:       job.ID,
			Version:     i,
			Name:        job.Name,
			Slug:        job.Slug,
			EndpointURL: "https://example.com/v",
			MaxAttempts: 3,
			TimeoutSecs: 30,
		}
		require.NoError(t, q.CreateJobVersion(ctx, v))

	}

	versions, err := q.ListJobVersionsByJob(ctx, job.ID, 10, nil)
	require.NoError(t, err)
	require.Len(t, versions,

		3)
	require.False(t, versions[0].Version !=
		3 ||
		versions[2].Version !=
			1)

	// Should be ordered version DESC.

	// Empty.
	empty, err := q.ListJobVersionsByJob(ctx, newID(), 10, nil)
	require.NoError(t, err)
	require.Len(t, empty, 0)

}

func TestGetJobVersionByVersionID_LookupByNanoid(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-job-version-by-vid")

	v := &domain.JobVersion{
		ID:          newID(),
		JobID:       job.ID,
		Version:     1,
		VersionID:   "nanoid-abc-123",
		Name:        job.Name,
		Slug:        job.Slug,
		EndpointURL: "https://example.com/v1",
		MaxAttempts: 3,
		TimeoutSecs: 30,
	}
	require.NoError(t, q.CreateJobVersion(ctx, v))

	got, err := q.GetJobVersionByVersionID(ctx, "nanoid-abc-123")
	require.NoError(t, err)
	require.Equal(t, v.ID,
		got.
			ID)

	// Not found.
	_, err = q.GetJobVersionByVersionID(ctx, "nonexistent")
	require.True(t, errors.Is(err, store.
		ErrJobNotFound,
	))

}

func TestGetJobAtVersion_SnapshotAndFallback(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-job-at-version")

	v := &domain.JobVersion{
		ID:          newID(),
		JobID:       job.ID,
		Version:     1,
		Name:        "versioned-name",
		Slug:        job.Slug,
		EndpointURL: "https://example.com/v1",
		MaxAttempts: 3,
		TimeoutSecs: 30,
	}
	require.NoError(t, q.CreateJobVersion(ctx, v))

	got, err := q.GetJobAtVersion(ctx, job.ID, 1)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "versioned-name",

		got.Name)

	// Fallback for a version that does not exist should return the live job.
	fallback, err := q.GetJobAtVersion(ctx, job.ID, 999)
	require.NoError(t, err)
	require.Equal(t, job.ID,

		fallback.
			ID)

}

func TestGetJobAtVersion_PreservesExecutionSnapshot(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-job-version-execution-snapshot")

	poisonPinned := 7
	job.Name = "pinned-execution-config"
	job.ExecutionMode = domain.ExecutionModeWorker
	job.Queue = "critical"
	job.PreferredRegions = []string{"iad1", "sfo1"}
	job.PoisonPillThreshold = &poisonPinned
	job.DebounceWindowSecs = 11
	job.BatchWindowSecs = 12
	job.BatchMaxSize = 13
	job.OnCompleteTriggerWorkflow = "workflow-pinned"
	job.OnCompleteTriggerJob = "job-pinned"
	job.OnCompletePayloadMapping = json.RawMessage(`{"complete":"pinned"}`)
	job.OnFailureTriggerJob = "job-failure-pinned"
	job.OnFailureTriggerWorkflow = "workflow-failure-pinned"
	job.OnFailurePayloadMapping = json.RawMessage(`{"failure":"pinned"}`)
	job.EndpointSigningSecret = "signing-secret-pinned"
	require.NoError(t, q.UpdateJob(ctx,
		job))
	require.NoError(t, q.PauseJob(ctx,
		job.ID, "versioned pause",
	),
	)

	job, err := q.GetJob(ctx, job.ID)
	require.NoError(t, err)

	pinnedVersion := job.Version

	poisonLive := 3
	job.Name = "live-execution-config"
	job.ExecutionMode = domain.ExecutionModeHTTP
	job.Queue = "default"
	job.PreferredRegions = []string{"fra1"}
	job.PoisonPillThreshold = &poisonLive
	job.DebounceWindowSecs = 21
	job.BatchWindowSecs = 22
	job.BatchMaxSize = 23
	job.OnCompleteTriggerWorkflow = "workflow-live"
	job.OnCompleteTriggerJob = "job-live"
	job.OnCompletePayloadMapping = json.RawMessage(`{"complete":"live"}`)
	job.OnFailureTriggerJob = "job-failure-live"
	job.OnFailureTriggerWorkflow = "workflow-failure-live"
	job.OnFailurePayloadMapping = json.RawMessage(`{"failure":"live"}`)
	job.EndpointSigningSecret = "signing-secret-live"
	require.NoError(t, q.UpdateJob(ctx,
		job))
	require.NoError(t, q.ResumeJob(ctx,
		job.ID))

	got, err := q.GetJobAtVersion(ctx, job.ID, pinnedVersion)
	require.NoError(t, err)
	require.Equal(t, "pinned-execution-config",

		got.Name)
	require.Equal(t, domain.
		ExecutionModeWorker,

		got.ExecutionMode,
	)
	require.Equal(t, "critical",

		got.
			Queue)
	require.True(t, reflect.
		DeepEqual(got.PreferredRegions,
			[]string{"iad1",
				"sfo1"}))
	require.False(t, got.PoisonPillThreshold ==
		nil || *got.PoisonPillThreshold !=
		poisonPinned,
	)
	require.False(t, got.DebounceWindowSecs !=
		11 ||
		got.BatchWindowSecs !=
			12 || got.
		BatchMaxSize !=
		13)
	require.False(t, got.OnCompleteTriggerWorkflow !=
		"workflow-pinned" ||
		got.OnCompleteTriggerJob !=
			"job-pinned")
	require.True(t, jsonEqual(got.OnCompletePayloadMapping,

		json.
			RawMessage(`{"complete":"pinned"}`)))
	require.False(t, got.OnFailureTriggerWorkflow !=
		"workflow-failure-pinned" ||
		got.
			OnFailureTriggerJob !=
			"job-failure-pinned",
	)
	require.True(t, jsonEqual(got.OnFailurePayloadMapping,

		json.RawMessage(
			`{"failure":"pinned"}`,
		),
	))
	require.False(t, !got.Paused ||
		got.
			PauseReason !=
			"versioned pause" ||
		got.PausedAt ==
			nil)
	require.Equal(t, "signing-secret-pinned",

		got.
			EndpointSigningSecret,
	)

}

func TestUpdateJob_StaleVersionDoesNotCreateSnapshot(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-job-version-stale-snapshot")
	stale := *job

	job.Name = "winner"
	require.NoError(t, q.UpdateJob(ctx,
		job))

	stale.Name = "stale"
	if err := q.UpdateJob(ctx, &stale); !errors.Is(err, store.ErrJobVersionConflict) {
		require.Failf(t, "test failure",

			"UpdateJob(stale) error = %v, want ErrJobVersionConflict", err)
	}

	var poisoned int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT COUNT(*) FROM job_versions WHERE job_id = $1 AND version = $2
	`,

		job.ID, job.Version).Scan(
		&poisoned))
	require.EqualValues(t, 0, poisoned)

	current, err := q.GetJob(ctx, job.ID)
	require.NoError(t, err)

	current.Name = "valid-after-stale"
	require.NoError(t, q.UpdateJob(ctx,
		current),
	)
	require.Equal(t, job.Version+
		1,
		current.Version,
	)

}

func TestUpdateJob_StaleVersionDoesNotBlockFutureSnapshot(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-job-version-stale-future")
	stale := *job

	job.Name = "winner-before-stale"
	require.NoError(t, q.UpdateJob(ctx,
		job))

	winnerVersion := job.Version

	stale.Name = "stale-poison"
	if err := q.UpdateJob(ctx, &stale); !errors.Is(err, store.ErrJobVersionConflict) {
		require.Failf(t, "test failure",

			"UpdateJob(stale) error = %v, want ErrJobVersionConflict", err)
	}

	current, err := q.GetJob(ctx, job.ID)
	require.NoError(t, err)

	current.Name = "valid-after-stale"
	require.NoError(t, q.UpdateJob(ctx,
		current),
	)

	versioned, err := q.GetJobAtVersion(ctx, job.ID, winnerVersion)
	require.NoError(t, err)
	require.Equal(t, "winner-before-stale",

		versioned.
			Name)
	require.Equal(t, winnerVersion+
		1, current.
		Version)

}
