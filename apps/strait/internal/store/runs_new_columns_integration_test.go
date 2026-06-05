//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

func TestCreateRun_WithBatchIDAndConcurrencyKey(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	job := mustCreateJob(t, ctx, q, "project-run-new-cols")

	batchOp := &domain.BatchOperation{
		ID:        newID(),
		ProjectID: job.ProjectID,
		JobID:     job.ID,
		ItemCount: 5,
		CreatedBy: "test",
	}
	require.NoError(t, q.CreateBatchOperation(ctx,
		batchOp))

	run := baseRun(job, newID())
	run.BatchID = batchOp.ID
	run.ConcurrencyKey = "tenant-123"
	require.NoError(t, q.CreateRun(ctx,
		run))

	got, err := q.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, batchOp.
		ID, got.
		BatchID)
	require.Equal(t, "tenant-123",

		got.
			ConcurrencyKey,
	)

}

func TestCreateRun_NilBatchIDAndConcurrencyKey(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	job := mustCreateJob(t, ctx, q, "project-run-nil-cols")

	run := baseRun(job, newID())
	require.NoError(t, q.CreateRun(ctx,
		run))

	// Leave BatchID and ConcurrencyKey at zero values.

	got, err := q.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, "", got.
		BatchID)
	require.Equal(t, "", got.
		ConcurrencyKey,
	)

}

func TestListRunsByProject_TriggeredByFilter(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	job := mustCreateJob(t, ctx, q, "project-triggered-filter")

	manualRun := baseRun(job, newID())
	manualRun.TriggeredBy = domain.TriggerManual
	require.NoError(t, q.CreateRun(ctx,
		manualRun,
	))

	cronRun := baseRun(job, newID())
	cronRun.TriggeredBy = domain.TriggerCron
	require.NoError(t, q.CreateRun(ctx,
		cronRun),
	)

	triggeredBy := domain.TriggerManual
	runs, err := q.ListRunsByProject(ctx, job.ProjectID, nil, nil, nil, &triggeredBy, nil, nil, nil, nil, 20, nil)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	require.Equal(t, domain.
		TriggerManual,
		runs[0].TriggeredBy)

}

func TestListRunsByProject_PayloadContainsFilter(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	job := mustCreateJob(t, ctx, q, "project-payload-filter")

	run1 := baseRun(job, newID())
	run1.Payload = json.RawMessage(`{"hello":"world","extra":"data"}`)
	require.NoError(t, q.CreateRun(ctx,
		run1))

	run2 := baseRun(job, newID())
	run2.Payload = json.RawMessage(`{"other":"payload"}`)
	require.NoError(t, q.CreateRun(ctx,
		run2))

	runs, err := q.ListRunsByProject(ctx, job.ProjectID, nil, nil, nil, nil, nil, json.RawMessage(`{"hello":"world"}`), nil, nil, 20, nil)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	require.Equal(t, run1.ID,

		runs[0].
			ID)

}

func TestListRunsByProjectFiltered_ComposesStatusTagAndEnvironment(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	projectID := "project-filtered-runs"
	require.NoError(t, q.CreateProject(ctx, &domain.
		Project{ID: projectID,
		OrgID: "org-filtered-runs",
		Name:  "Filtered Runs"}))
	require.NoError(t, q.CreateEnvironment(ctx,
		&domain.Environment{ID: "env-prod",

			ProjectID: projectID,

			Name: "Production", Slug: "production"}))
	require.NoError(t, q.CreateEnvironment(ctx,
		&domain.Environment{ID: "env-staging",

			ProjectID: projectID,

			Name: "Staging", Slug: "staging"}))

	prodJob := baseJob(newID(), projectID)
	prodJob.EnvironmentID = "env-prod"
	require.NoError(t, q.CreateJob(ctx,
		prodJob),
	)

	stagingJob := baseJob(newID(), projectID)
	stagingJob.EnvironmentID = "env-staging"
	require.NoError(t, q.CreateJob(ctx,
		stagingJob,
	))

	wantRun := baseRun(prodJob, newID())
	wantRun.Tags = map[string]string{"team": "core"}
	require.NoError(t, q.CreateRun(ctx,
		wantRun),
	)

	wrongEnv := baseRun(stagingJob, newID())
	wrongEnv.Tags = map[string]string{"team": "core"}
	require.NoError(t, q.CreateRun(ctx,
		wrongEnv,
	))

	wrongTag := baseRun(prodJob, newID())
	wrongTag.Tags = map[string]string{"team": "edge"}
	require.NoError(t, q.CreateRun(ctx,
		wrongTag,
	))

	failedStatus := domain.StatusFailed
	wrongStatus := baseRun(prodJob, newID())
	wrongStatus.Status = failedStatus
	wrongStatus.Tags = map[string]string{"team": "core"}
	require.NoError(t, q.CreateRun(ctx,
		wrongStatus,
	))

	envID := "env-prod"
	runs, err := q.ListRunsByProjectFiltered(
		ctx,
		projectID,
		nil,
		[]domain.RunStatus{domain.StatusQueued},
		"team",
		"core",
		&envID,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		20,
		nil,
	)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	require.Equal(t, wantRun.
		ID, runs[0].ID)

}
