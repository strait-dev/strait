//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
	"strait/internal/testutil"

	"github.com/stretchr/testify/require"
)

// ListJobSLOs.

func TestSLO_ListJobSLOs_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-slo-list")
	slo := &domain.JobSLO{
		ID:          newID(),
		JobID:       job.ID,
		ProjectID:   job.ProjectID,
		Metric:      "success_rate",
		Target:      99.9,
		WindowHours: 24,
	}
	require.NoError(t, q.CreateJobSLO(ctx, slo))

	results, err := q.ListJobSLOs(ctx, job.ID)
	require.NoError(t, err)
	require.Len(t, results,

		1)
	require.Equal(t, "success_rate",

		results[0].
			Metric)

}

func TestSLO_ListJobSLOs_Empty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	results, err := q.ListJobSLOs(ctx, newID())
	require.NoError(t, err)
	require.Len(t, results,

		0)

}

func TestSLO_ListJobSLOs_WithEvaluation(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-slo-eval")
	slo := &domain.JobSLO{
		ID:          newID(),
		JobID:       job.ID,
		ProjectID:   job.ProjectID,
		Metric:      "p95_latency_secs",
		Target:      5.0,
		WindowHours: 168,
	}
	require.NoError(t, q.CreateJobSLO(ctx, slo))

	eval := &domain.JobSLOEvaluation{
		ID:              newID(),
		SLOID:           slo.ID,
		CurrentValue:    4.5,
		BudgetRemaining: 0.5,
		EvaluatedAt:     time.Now().UTC(),
	}
	require.NoError(t, q.InsertSLOEvaluation(ctx,
		eval))

	results, err := q.ListJobSLOs(ctx, job.ID)
	require.NoError(t, err)
	require.Len(t, results,

		1)
	require.False(t, results[0].CurrentValue ==
		nil || *results[0].CurrentValue !=
		4.5,
	)

}

// DeleteJobSLO.

func TestSLO_DeleteJobSLO_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-slo-delete")
	slo := &domain.JobSLO{
		ID:          newID(),
		JobID:       job.ID,
		ProjectID:   job.ProjectID,
		Metric:      "success_rate",
		Target:      99.0,
		WindowHours: 24,
	}
	require.NoError(t, q.CreateJobSLO(ctx, slo))
	require.NoError(t, q.DeleteJobSLO(ctx, slo.ID))

	got, err := q.GetJobSLO(ctx, slo.ID)
	require.NoError(t, err)
	require.Nil(t, got)

}

func TestSLO_DeleteJobSLO_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	err := q.DeleteJobSLO(ctx, newID())
	require.Error(t, err)

}

func TestSLO_DeleteJobSLO_Idempotent(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-slo-delete-idem")
	slo := &domain.JobSLO{
		ID:          newID(),
		JobID:       job.ID,
		ProjectID:   job.ProjectID,
		Metric:      "success_rate",
		Target:      99.0,
		WindowHours: 24,
	}
	require.NoError(t, q.CreateJobSLO(ctx, slo))
	require.NoError(t, q.DeleteJobSLO(ctx, slo.ID))
	require.Error(t, q.DeleteJobSLO(ctx,
		slo.ID),
	)

	// Second delete should error.

}

// GetJobSLO.

func TestSLO_GetJobSLO_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-slo-get")
	slo := &domain.JobSLO{
		ID:          newID(),
		JobID:       job.ID,
		ProjectID:   job.ProjectID,
		Metric:      "p99_latency_secs",
		Target:      1.0,
		WindowHours: 720,
	}
	require.NoError(t, q.CreateJobSLO(ctx, slo))

	got, err := q.GetJobSLO(ctx, slo.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "p99_latency_secs",

		got.Metric,
	)
	require.EqualValues(t, 1.0, got.
		Target)

}

func TestSLO_GetJobSLO_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	got, err := q.GetJobSLO(ctx, newID())
	require.NoError(t, err)
	require.Nil(t, got)

}

// ListAllJobSLOs.

func TestSLO_ListAllJobSLOs_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job1 := mustCreateJob(t, ctx, q, "project-slo-all-1")
	job2 := mustCreateJob(t, ctx, q, "project-slo-all-2")

	for _, jid := range []string{job1.ID, job2.ID} {
		slo := &domain.JobSLO{
			ID:          newID(),
			JobID:       jid,
			ProjectID:   "project-slo-all-1",
			Metric:      "success_rate",
			Target:      99.0,
			WindowHours: 24,
		}
		require.NoError(t, q.CreateJobSLO(ctx, slo))

	}

	all, err := q.ListAllJobSLOs(ctx)
	require.NoError(t, err)
	require.GreaterOrEqual(
		t,
		len(all), 2)

}

func TestSLO_ListAllJobSLOs_Empty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	all, err := q.ListAllJobSLOs(ctx)
	require.NoError(t, err)
	require.Len(t, all, 0)

}

// GetEndpointHealthScore.

func TestHealth_GetEndpointHealthScore_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	score := &domain.EndpointHealthScore{
		EndpointURL:   "https://example.com/health-get",
		HealthScore:   85.0,
		SuccessRate:   0.95,
		TimeoutRate:   0.01,
		LatencyScore:  0.9,
		TotalRequests: 100,
		LastLatencyMs: 120.5,
	}
	require.NoError(t, q.UpsertEndpointHealthScore(ctx, score))

	got, err := q.GetEndpointHealthScore(ctx, "https://example.com/health-get")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.EqualValues(t, 85.0,
		got.
			HealthScore,
	)
	require.EqualValues(t, 100, got.
		TotalRequests,
	)

}

func TestHealth_GetEndpointHealthScore_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	got, err := q.GetEndpointHealthScore(ctx, "https://nonexistent.example.com/health")
	require.NoError(t, err)
	require.Nil(t, got)

}

// UpsertEndpointHealthScore.

func TestHealth_UpsertEndpointHealthScore_Insert(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	score := &domain.EndpointHealthScore{
		EndpointURL:   "https://example.com/health-insert",
		HealthScore:   90.0,
		SuccessRate:   0.98,
		TimeoutRate:   0.0,
		LatencyScore:  0.95,
		TotalRequests: 50,
		LastLatencyMs: 80.0,
	}
	require.NoError(t, q.UpsertEndpointHealthScore(ctx, score))

	got, err := q.GetEndpointHealthScore(ctx, score.EndpointURL)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.EqualValues(t, 0.98,
		got.
			SuccessRate,
	)

}

func TestHealth_UpsertEndpointHealthScore_Update(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	endpoint := "https://example.com/health-update"
	initial := &domain.EndpointHealthScore{
		EndpointURL:   endpoint,
		HealthScore:   50.0,
		SuccessRate:   0.5,
		TimeoutRate:   0.1,
		LatencyScore:  0.5,
		TotalRequests: 10,
		LastLatencyMs: 200.0,
	}
	require.NoError(t, q.UpsertEndpointHealthScore(ctx, initial))

	gotInitial, err := q.GetEndpointHealthScore(ctx, endpoint)
	require.NoError(t, err)
	require.NotNil(t, gotInitial)

	initialUpdatedAt := gotInitial.UpdatedAt
	var xminBeforeNoop string
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT xmin::text
		FROM endpoint_health_scores
		WHERE endpoint_url = $1`,

		endpoint,
	).Scan(&xminBeforeNoop))

	same := &domain.EndpointHealthScore{
		EndpointURL:   endpoint,
		HealthScore:   50.0,
		SuccessRate:   0.5,
		TimeoutRate:   0.1,
		LatencyScore:  0.5,
		TotalRequests: 10,
		LastLatencyMs: 200.0,
	}
	require.NoError(t, q.UpsertEndpointHealthScore(ctx, same))

	var xminAfterNoop string
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT xmin::text
		FROM endpoint_health_scores
		WHERE endpoint_url = $1`,

		endpoint,
	).Scan(&xminAfterNoop))
	require.Equal(t, xminBeforeNoop,

		xminAfterNoop,
	)

	gotSame, err := q.GetEndpointHealthScore(ctx, endpoint)
	require.NoError(t, err)
	require.NotNil(t, gotSame)
	require.True(t, gotSame.
		UpdatedAt.
		Equal(initialUpdatedAt))

	updated := &domain.EndpointHealthScore{
		EndpointURL:   endpoint,
		HealthScore:   90.0,
		SuccessRate:   0.99,
		TimeoutRate:   0.0,
		LatencyScore:  0.95,
		TotalRequests: 200,
		LastLatencyMs: 50.0,
	}
	require.NoError(t, q.UpsertEndpointHealthScore(ctx, updated))

	got, err := q.GetEndpointHealthScore(ctx, endpoint)
	require.NoError(t, err)
	require.EqualValues(t, 90.0,
		got.
			HealthScore,
	)
	require.EqualValues(t, 200, got.
		TotalRequests,
	)

}

func TestHealth_UpsertEndpointHealthScore_ZeroValues(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	score := &domain.EndpointHealthScore{
		EndpointURL:   "https://example.com/health-zero",
		HealthScore:   0.0,
		SuccessRate:   0.0,
		TimeoutRate:   0.0,
		LatencyScore:  0.0,
		TotalRequests: 0,
		LastLatencyMs: 0.0,
	}
	require.NoError(t, q.UpsertEndpointHealthScore(ctx, score))

	got, err := q.GetEndpointHealthScore(ctx, score.EndpointURL)
	require.NoError(t, err)
	require.NotNil(t, got)

}

// CreateWorkflowRunBootstrap.

func TestWorkflowRun_CreateWorkflowRunBootstrap_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-wf-bootstrap-" + newID()
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: new(projectID),
	})
	stepJob := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: new(projectID)})
	step := testutil.MustCreateWorkflowStep(t, ctx, q, wf.ID, &testutil.WorkflowStepOpts{
		JobID:   new(stepJob.ID),
		StepRef: new("step-a"),
	})

	run := testutil.BuildWorkflowRun(wf.ID, &testutil.WorkflowRunOpts{ProjectID: new(projectID)})
	stepRuns := []domain.WorkflowStepRun{
		{
			ID:             newID(),
			WorkflowRunID:  run.ID,
			WorkflowStepID: step.ID,
			StepRef:        "step-a",
			Status:         domain.StepPending,
		},
	}
	now := time.Now().UTC()
	require.NoError(t, q.CreateWorkflowRunBootstrap(ctx, run, stepRuns,
		now),
	)

	got, err := q.GetWorkflowRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		WfStatusRunning,
		got.
			Status)
	require.NotNil(t, got.StartedAt)

}

func TestWorkflowRun_CreateWorkflowRunBootstrap_MultipleSteps(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-wf-bootstrap-multi-" + newID()
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: new(projectID),
	})
	stepJob := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: new(projectID)})
	stepA := testutil.MustCreateWorkflowStep(t, ctx, q, wf.ID, &testutil.WorkflowStepOpts{
		JobID:   new(stepJob.ID),
		StepRef: new("step-a"),
	})
	stepB := testutil.MustCreateWorkflowStep(t, ctx, q, wf.ID, &testutil.WorkflowStepOpts{
		JobID:   new(stepJob.ID),
		StepRef: new("step-b"),
	})

	run := testutil.BuildWorkflowRun(wf.ID, &testutil.WorkflowRunOpts{ProjectID: new(projectID)})
	stepRuns := []domain.WorkflowStepRun{
		{ID: newID(), WorkflowRunID: run.ID, WorkflowStepID: stepA.ID, StepRef: "step-a", Status: domain.StepPending},
		{ID: newID(), WorkflowRunID: run.ID, WorkflowStepID: stepB.ID, StepRef: "step-b", Status: domain.StepPending},
	}
	require.NoError(t, q.CreateWorkflowRunBootstrap(ctx, run, stepRuns,
		time.
			Now().UTC()))

	got, err := q.GetWorkflowRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		WfStatusRunning,
		got.
			Status)

}

func TestWorkflowRun_CreateWorkflowRunBootstrap_NoSteps(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-wf-bootstrap-no-steps-" + newID()
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: new(projectID),
	})

	run := testutil.BuildWorkflowRun(wf.ID, &testutil.WorkflowRunOpts{ProjectID: new(projectID)})
	require.NoError(t, q.CreateWorkflowRunBootstrap(ctx, run, nil,
		time.Now().UTC()),
	)

	got, err := q.GetWorkflowRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		WfStatusRunning,
		got.
			Status)

}

// ListStalledWorkflowRuns.

func TestWorkflowRun_ListStalledWorkflowRuns_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-wf-stalled-" + newID()
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: new(projectID),
	})

	// Create a running workflow run with started_at in the past.
	run := testutil.BuildWorkflowRun(wf.ID, &testutil.WorkflowRunOpts{ProjectID: new(projectID)})
	require.NoError(t, q.CreateWorkflowRun(ctx,
		run))

	past := time.Now().UTC().Add(-2 * time.Hour)
	require.NoError(t, q.UpdateWorkflowRunStatus(ctx, run.ID, domain.
		WfStatusPending,

		domain.WfStatusRunning, map[string]any{"started_at": past}))

	stalled, err := q.ListStalledWorkflowRuns(ctx, 1*time.Hour)
	require.NoError(t, err)

	found := false
	for _, r := range stalled {
		if r.ID == run.ID {
			found = true
		}
	}
	require.True(t, found)

}

func TestWorkflowRun_ListStalledWorkflowRuns_ExcludesRecentRuns(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-wf-stalled-recent-" + newID()
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: new(projectID),
	})

	// Create a recently started running workflow.
	run := testutil.BuildWorkflowRun(wf.ID, &testutil.WorkflowRunOpts{ProjectID: new(projectID)})
	require.NoError(t, q.CreateWorkflowRun(ctx,
		run))
	require.NoError(t, q.UpdateWorkflowRunStatus(ctx, run.ID, domain.
		WfStatusPending,

		domain.WfStatusRunning, map[string]any{"started_at": time.Now().UTC()}))

	stalled, err := q.ListStalledWorkflowRuns(ctx, 1*time.Hour)
	require.NoError(t, err)

	for _, r := range stalled {
		require.NotEqual(t, run.
			ID, r.ID)

	}
}

func TestWorkflowRun_ListStalledWorkflowRuns_Empty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	stalled, err := q.ListStalledWorkflowRuns(ctx, 1*time.Hour)
	require.NoError(t, err)
	require.Len(t, stalled,

		0)

}

// CountActiveWorkflowRunsByVersion.

func TestWorkflowRun_CountActiveWorkflowRunsByVersion_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-wf-count-ver-" + newID()
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: new(projectID),
	})
	versionID := "v-" + newID()

	// Create 2 active runs with this version.
	for range 2 {
		run := testutil.BuildWorkflowRun(wf.ID, &testutil.WorkflowRunOpts{ProjectID: new(projectID)})
		run.WorkflowVersionID = versionID
		require.NoError(t, q.CreateWorkflowRun(ctx,
			run))

	}

	count, err := q.CountActiveWorkflowRunsByVersion(ctx, wf.ID, versionID)
	require.NoError(t, err)
	require.EqualValues(t, 2, count)

}

func TestWorkflowRun_CountActiveWorkflowRunsByVersion_ExcludesTerminal(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-wf-count-terminal-" + newID()
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: new(projectID),
	})
	versionID := "v-" + newID()

	// One pending, one completed.
	pending := testutil.BuildWorkflowRun(wf.ID, &testutil.WorkflowRunOpts{ProjectID: new(projectID)})
	pending.WorkflowVersionID = versionID
	require.NoError(t, q.CreateWorkflowRun(ctx,
		pending))

	completed := testutil.BuildWorkflowRun(wf.ID, &testutil.WorkflowRunOpts{
		ProjectID: new(projectID),
		Status:    testutil.Ptr(domain.WfStatusCompleted),
	})
	completed.WorkflowVersionID = versionID
	require.NoError(t, q.CreateWorkflowRun(ctx,
		completed))

	count, err := q.CountActiveWorkflowRunsByVersion(ctx, wf.ID, versionID)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)

}

func TestWorkflowRun_CountActiveWorkflowRunsByVersion_Zero(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	count, err := q.CountActiveWorkflowRunsByVersion(ctx, newID(), "v-nonexistent")
	require.NoError(t, err)
	require.EqualValues(t, 0, count)

}

// ListActiveWorkflowVersions.

func TestWorkflowRun_ListActiveWorkflowVersions_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-wf-active-ver-" + newID()
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: new(projectID),
	})
	vID1 := "v1-" + newID()
	vID2 := "v2-" + newID()

	for _, vid := range []string{vID1, vID2} {
		run := testutil.BuildWorkflowRun(wf.ID, &testutil.WorkflowRunOpts{ProjectID: new(projectID)})
		run.WorkflowVersionID = vid
		require.NoError(t, q.CreateWorkflowRun(ctx,
			run))

	}

	versions, err := q.ListActiveWorkflowVersions(ctx, wf.ID)
	require.NoError(t, err)
	require.GreaterOrEqual(
		t,
		len(versions), 2)

}

func TestWorkflowRun_ListActiveWorkflowVersions_Empty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	versions, err := q.ListActiveWorkflowVersions(ctx, newID())
	require.NoError(t, err)
	require.Len(t, versions,

		0)

}

func TestWorkflowRun_ListActiveWorkflowVersions_StatusCounts(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-wf-ver-counts-" + newID()
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: new(projectID),
	})
	vid := "v-counts-" + newID()

	// 1 pending, 1 running.
	pending := testutil.BuildWorkflowRun(wf.ID, &testutil.WorkflowRunOpts{ProjectID: new(projectID)})
	pending.WorkflowVersionID = vid
	require.NoError(t, q.CreateWorkflowRun(ctx,
		pending))

	running := testutil.BuildWorkflowRun(wf.ID, &testutil.WorkflowRunOpts{ProjectID: new(projectID)})
	running.WorkflowVersionID = vid
	require.NoError(t, q.CreateWorkflowRun(ctx,
		running))
	require.NoError(t, q.UpdateWorkflowRunStatus(ctx, running.ID,
		domain.WfStatusPending,

		domain.WfStatusRunning,
		map[string]any{"started_at": time.Now().UTC()}))

	versions, err := q.ListActiveWorkflowVersions(ctx, wf.ID)
	require.NoError(t, err)

	found := false
	for _, v := range versions {
		if v.VersionID == vid {
			found = true
			require.EqualValues(t, 1, v.Pending)
			require.EqualValues(t, 1, v.Running)
			require.EqualValues(t, 2, v.Total)

		}
	}
	require.True(t, found)

}

// GetWorkflowSnapshot.
// getWorkflowSnapshotByVersion is private, tested indirectly via GetOrCreateWorkflowSnapshot.

func TestWorkflowSnapshot_GetWorkflowSnapshot_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-wf-snapshot-" + newID()
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: new(projectID),
	})

	snapshot, err := q.GetOrCreateWorkflowSnapshot(ctx, &domain.Workflow{
		ID:        wf.ID,
		ProjectID: projectID,
		Name:      wf.Name,
		Slug:      wf.Slug,
		Version:   1,
	}, nil)
	require.NoError(t, err)

	got, err := q.GetWorkflowSnapshot(ctx, projectID, snapshot.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, wf.ID,

		got.WorkflowID,
	)

	// Verify definition is valid JSON.
	var def json.RawMessage
	require.NoError(t, json.
		Unmarshal(got.Definition,
			&def))

}

func TestWorkflowSnapshot_GetWorkflowSnapshot_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	got, err := q.GetWorkflowSnapshot(ctx, newID(), newID())
	require.NoError(t, err)
	require.Nil(t, got)

}

// TestWorkflowSnapshot_GetWorkflowSnapshot_CrossTenant guards the tenant
// scoping added to workflow_snapshots: a snapshot id from one project must not
// resolve under another project's scope.
func TestWorkflowSnapshot_GetWorkflowSnapshot_CrossTenant(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	ownerProject := "project-wf-snap-owner-" + newID()
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: new(ownerProject),
	})
	snapshot, err := q.GetOrCreateWorkflowSnapshot(ctx, &domain.Workflow{
		ID:        wf.ID,
		ProjectID: ownerProject,
		Name:      wf.Name,
		Slug:      wf.Slug,
		Version:   1,
	}, nil)
	require.NoError(t, err)

	owned, err := q.GetWorkflowSnapshot(ctx, ownerProject, snapshot.ID)
	require.NoError(t, err)
	require.NotNil(t, owned)

	other, err := q.GetWorkflowSnapshot(ctx, "project-wf-snap-attacker-"+newID(), snapshot.ID)
	require.NoError(t, err)
	require.Nil(t, other, "snapshot must not resolve under a different project")
}

// TestWorkflowSnapshot_VersionlessDedup is the regression guard for unbounded
// duplicate snapshots: a versionless workflow (no version_id) re-creating the
// same definition must reuse the existing snapshot rather than insert a new row
// each time (the ON CONFLICT partial index only covers version_id != '').
func TestWorkflowSnapshot_VersionlessDedup(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-wf-snap-versionless-" + newID()
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: new(projectID),
	})
	create := func() (*domain.WorkflowSnapshot, error) {
		return q.GetOrCreateWorkflowSnapshot(ctx, &domain.Workflow{
			ID:        wf.ID,
			ProjectID: projectID,
			Name:      wf.Name,
			Slug:      wf.Slug,
			Version:   1,
			// No VersionID: exercises the versionless path.
		}, nil)
	}

	first, err := create()
	require.NoError(t, err)
	second, err := create()
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID, "identical versionless definition must dedupe to one snapshot")
}

func TestWorkflowSnapshot_GetWorkflowSnapshot_Dedup(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-wf-snapshot-dedup-" + newID()
	versionID := "vid-" + newID()
	require.NoError(t, q.SetProjectContext(ctx,
		projectID))

	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: new(projectID),
	})

	wfObj := &domain.Workflow{
		ID:        wf.ID,
		ProjectID: projectID,
		Name:      wf.Name,
		Slug:      wf.Slug,
		Version:   1,
		VersionID: versionID,
	}

	snap1, err := q.GetOrCreateWorkflowSnapshot(ctx, wfObj, nil)
	require.NoError(t, err)

	snap2, err := q.GetOrCreateWorkflowSnapshot(ctx, wfObj, nil)
	require.NoError(t, err)
	require.Equal(t, snap2.
		ID,
		snap1.
			ID)

}

func TestWorkflowSnapshot_DedupIncludesStepOverrides(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-wf-snapshot-override-" + newID()
	versionID := "vid-" + newID()
	require.NoError(t, q.SetProjectContext(ctx,
		projectID))

	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: new(projectID),
	})

	wfObj := &domain.Workflow{
		ID:        wf.ID,
		ProjectID: projectID,
		Name:      wf.Name,
		Slug:      wf.Slug,
		Version:   1,
		VersionID: versionID,
	}
	stepA := domain.WorkflowStep{ID: newID(), WorkflowID: wf.ID, StepRef: "a", JobID: newID()}
	stepB := domain.WorkflowStep{ID: newID(), WorkflowID: wf.ID, StepRef: "b", JobID: newID(), DependsOn: []string{"a"}}

	full, err := q.GetOrCreateWorkflowSnapshot(ctx, wfObj, []domain.WorkflowStep{stepA, stepB})
	require.NoError(t, err)

	override, err := q.GetOrCreateWorkflowSnapshot(ctx, wfObj, []domain.WorkflowStep{stepA})
	require.NoError(t, err)
	require.NotEqual(t, override.
		ID,
		full.ID)

	overrideAgain, err := q.GetOrCreateWorkflowSnapshot(ctx, wfObj, []domain.WorkflowStep{stepA})
	require.NoError(t, err)
	require.Equal(t, override.
		ID, overrideAgain.
		ID)

	got, err := q.GetWorkflowSnapshot(ctx, projectID, override.ID)
	require.NoError(t, err)

	def, err := store.ParseSnapshotDefinition(got.Definition)
	require.NoError(t, err)
	require.False(t, len(def.
		Steps) !=
		1 || def.
		Steps[0].StepRef !=
		"a")

}

// ReplayWebhookDelivery.

func TestWebhookDelivery_ReplayWebhookDelivery_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-webhook-replay")
	run := mustCreateRun(t, ctx, q, job)

	original, err := q.EnqueueRunWebhook(ctx, job, run, 3)
	require.NoError(t, err)

	replayed, err := q.ReplayWebhookDelivery(ctx, job.ProjectID, original.ID)
	require.NoError(t, err)
	require.NotEqual(t, original.
		ID,
		replayed.ID,
	)
	require.Equal(t, domain.
		WebhookStatusPending,

		replayed.Status,
	)
	require.EqualValues(t, 0, replayed.
		Attempts,
	)

}

func TestWebhookDelivery_ReplayWebhookDelivery_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, err := q.ReplayWebhookDelivery(ctx, "missing-project", newID())
	require.Error(t, err)

}

func TestWebhookDelivery_ReplayWebhookDelivery_PreservesJobID(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-webhook-replay-job")
	run := mustCreateRun(t, ctx, q, job)

	original, err := q.EnqueueRunWebhook(ctx, job, run, 3)
	require.NoError(t, err)

	replayed, err := q.ReplayWebhookDelivery(ctx, job.ProjectID, original.ID)
	require.NoError(t, err)
	require.Equal(t, original.
		JobID,
		replayed.JobID,
	)

}

// CountPendingWebhookDeliveries.

func TestWebhookDelivery_CountPendingWebhookDeliveries_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-webhook-count")
	run := mustCreateRun(t, ctx, q, job)

	if _, err := q.EnqueueRunWebhook(ctx, job, run, 3); err != nil {
		require.Failf(t, "test failure",

			"EnqueueRunWebhook() error = %v", err)
	}

	count, err := q.CountPendingWebhookDeliveries(ctx)
	require.NoError(t, err)
	require.GreaterOrEqual(
		t,
		count,
		int64(1))

}

func TestWebhookDelivery_CountPendingWebhookDeliveries_Zero(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	count, err := q.CountPendingWebhookDeliveries(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 0, count)

}

func TestWebhookDelivery_CountPendingWebhookDeliveries_ExcludesDelivered(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-webhook-count-delivered")
	run := mustCreateRun(t, ctx, q, job)

	delivery, err := q.EnqueueRunWebhook(ctx, job, run, 3)
	require.NoError(t, err)

	// Mark as delivered.
	now := time.Now().UTC()
	delivery.Status = domain.WebhookStatusDelivered
	delivery.DeliveredAt = &now
	require.NoError(t, q.UpdateWebhookDelivery(ctx,
		delivery))

	count, err := q.CountPendingWebhookDeliveries(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 0, count)

}
