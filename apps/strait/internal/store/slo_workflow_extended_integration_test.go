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
	if err := q.CreateJobSLO(ctx, slo); err != nil {
		t.Fatalf("CreateJobSLO() error = %v", err)
	}

	results, err := q.ListJobSLOs(ctx, job.ID)
	if err != nil {
		t.Fatalf("ListJobSLOs() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len = %d, want 1", len(results))
	}
	if results[0].Metric != "success_rate" {
		t.Fatalf("metric = %q, want success_rate", results[0].Metric)
	}
}

func TestSLO_ListJobSLOs_Empty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	results, err := q.ListJobSLOs(ctx, newID())
	if err != nil {
		t.Fatalf("ListJobSLOs() error = %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("len = %d, want 0", len(results))
	}
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
	if err := q.CreateJobSLO(ctx, slo); err != nil {
		t.Fatalf("CreateJobSLO() error = %v", err)
	}

	eval := &domain.JobSLOEvaluation{
		ID:              newID(),
		SLOID:           slo.ID,
		CurrentValue:    4.5,
		BudgetRemaining: 0.5,
		EvaluatedAt:     time.Now().UTC(),
	}
	if err := q.InsertSLOEvaluation(ctx, eval); err != nil {
		t.Fatalf("InsertSLOEvaluation() error = %v", err)
	}

	results, err := q.ListJobSLOs(ctx, job.ID)
	if err != nil {
		t.Fatalf("ListJobSLOs() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len = %d, want 1", len(results))
	}
	if results[0].CurrentValue == nil || *results[0].CurrentValue != 4.5 {
		t.Fatalf("current_value = %v, want 4.5", results[0].CurrentValue)
	}
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
	if err := q.CreateJobSLO(ctx, slo); err != nil {
		t.Fatalf("CreateJobSLO() error = %v", err)
	}

	if err := q.DeleteJobSLO(ctx, slo.ID); err != nil {
		t.Fatalf("DeleteJobSLO() error = %v", err)
	}

	got, err := q.GetJobSLO(ctx, slo.ID)
	if err != nil {
		t.Fatalf("GetJobSLO() error = %v", err)
	}
	if got != nil {
		t.Fatal("expected nil after delete")
	}
}

func TestSLO_DeleteJobSLO_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	err := q.DeleteJobSLO(ctx, newID())
	if err == nil {
		t.Fatal("expected error for nonexistent SLO")
	}
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
	if err := q.CreateJobSLO(ctx, slo); err != nil {
		t.Fatalf("CreateJobSLO() error = %v", err)
	}

	if err := q.DeleteJobSLO(ctx, slo.ID); err != nil {
		t.Fatalf("DeleteJobSLO() first error = %v", err)
	}
	// Second delete should error.
	if err := q.DeleteJobSLO(ctx, slo.ID); err == nil {
		t.Fatal("expected error on second delete")
	}
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
	if err := q.CreateJobSLO(ctx, slo); err != nil {
		t.Fatalf("CreateJobSLO() error = %v", err)
	}

	got, err := q.GetJobSLO(ctx, slo.ID)
	if err != nil {
		t.Fatalf("GetJobSLO() error = %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil SLO")
	}
	if got.Metric != "p99_latency_secs" {
		t.Fatalf("metric = %q, want p99_latency_secs", got.Metric)
	}
	if got.Target != 1.0 {
		t.Fatalf("target = %f, want 1.0", got.Target)
	}
}

func TestSLO_GetJobSLO_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	got, err := q.GetJobSLO(ctx, newID())
	if err != nil {
		t.Fatalf("GetJobSLO() error = %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
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
		if err := q.CreateJobSLO(ctx, slo); err != nil {
			t.Fatalf("CreateJobSLO() error = %v", err)
		}
	}

	all, err := q.ListAllJobSLOs(ctx)
	if err != nil {
		t.Fatalf("ListAllJobSLOs() error = %v", err)
	}
	if len(all) < 2 {
		t.Fatalf("len = %d, want >= 2", len(all))
	}
}

func TestSLO_ListAllJobSLOs_Empty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	all, err := q.ListAllJobSLOs(ctx)
	if err != nil {
		t.Fatalf("ListAllJobSLOs() error = %v", err)
	}
	if len(all) != 0 {
		t.Fatalf("len = %d, want 0", len(all))
	}
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
	if err := q.UpsertEndpointHealthScore(ctx, score); err != nil {
		t.Fatalf("UpsertEndpointHealthScore() error = %v", err)
	}

	got, err := q.GetEndpointHealthScore(ctx, "https://example.com/health-get")
	if err != nil {
		t.Fatalf("GetEndpointHealthScore() error = %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil score")
	}
	if got.HealthScore != 85.0 {
		t.Fatalf("health_score = %f, want 85.0", got.HealthScore)
	}
	if got.TotalRequests != 100 {
		t.Fatalf("total_requests = %d, want 100", got.TotalRequests)
	}
}

func TestHealth_GetEndpointHealthScore_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	got, err := q.GetEndpointHealthScore(ctx, "https://nonexistent.example.com/health")
	if err != nil {
		t.Fatalf("GetEndpointHealthScore() error = %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
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
	if err := q.UpsertEndpointHealthScore(ctx, score); err != nil {
		t.Fatalf("UpsertEndpointHealthScore() error = %v", err)
	}

	got, err := q.GetEndpointHealthScore(ctx, score.EndpointURL)
	if err != nil {
		t.Fatalf("GetEndpointHealthScore() error = %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil")
	}
	if got.SuccessRate != 0.98 {
		t.Fatalf("success_rate = %f, want 0.98", got.SuccessRate)
	}
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
	if err := q.UpsertEndpointHealthScore(ctx, initial); err != nil {
		t.Fatalf("insert error = %v", err)
	}

	updated := &domain.EndpointHealthScore{
		EndpointURL:   endpoint,
		HealthScore:   90.0,
		SuccessRate:   0.99,
		TimeoutRate:   0.0,
		LatencyScore:  0.95,
		TotalRequests: 200,
		LastLatencyMs: 50.0,
	}
	if err := q.UpsertEndpointHealthScore(ctx, updated); err != nil {
		t.Fatalf("update error = %v", err)
	}

	got, err := q.GetEndpointHealthScore(ctx, endpoint)
	if err != nil {
		t.Fatalf("GetEndpointHealthScore() error = %v", err)
	}
	if got.HealthScore != 90.0 {
		t.Fatalf("health_score = %f, want 90.0", got.HealthScore)
	}
	if got.TotalRequests != 200 {
		t.Fatalf("total_requests = %d, want 200", got.TotalRequests)
	}
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
	if err := q.UpsertEndpointHealthScore(ctx, score); err != nil {
		t.Fatalf("UpsertEndpointHealthScore() error = %v", err)
	}

	got, err := q.GetEndpointHealthScore(ctx, score.EndpointURL)
	if err != nil {
		t.Fatalf("GetEndpointHealthScore() error = %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil")
	}
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

	if err := q.CreateWorkflowRunBootstrap(ctx, store.CreateWorkflowRunBootstrapParams{Run: run, StepRuns: stepRuns, StartedAt: now}); err != nil {
		t.Fatalf("CreateWorkflowRunBootstrap() error = %v", err)
	}

	got, err := q.GetWorkflowRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetWorkflowRun() error = %v", err)
	}
	if got.Status != domain.WfStatusRunning {
		t.Fatalf("status = %s, want running", got.Status)
	}
	if got.StartedAt == nil {
		t.Fatal("started_at should be set")
	}
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

	if err := q.CreateWorkflowRunBootstrap(ctx, store.CreateWorkflowRunBootstrapParams{Run: run, StepRuns: stepRuns, StartedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("CreateWorkflowRunBootstrap() error = %v", err)
	}

	got, err := q.GetWorkflowRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetWorkflowRun() error = %v", err)
	}
	if got.Status != domain.WfStatusRunning {
		t.Fatalf("status = %s, want running", got.Status)
	}
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
	if err := q.CreateWorkflowRunBootstrap(ctx, store.CreateWorkflowRunBootstrapParams{Run: run, StepRuns: nil, StartedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("CreateWorkflowRunBootstrap() error = %v", err)
	}

	got, err := q.GetWorkflowRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetWorkflowRun() error = %v", err)
	}
	if got.Status != domain.WfStatusRunning {
		t.Fatalf("status = %s, want running", got.Status)
	}
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
	if err := q.CreateWorkflowRun(ctx, run); err != nil {
		t.Fatalf("CreateWorkflowRun() error = %v", err)
	}
	past := time.Now().UTC().Add(-2 * time.Hour)
	if err := q.UpdateWorkflowRunStatus(ctx, run.ID, domain.WfStatusPending, domain.WfStatusRunning, map[string]any{"started_at": past}); err != nil {
		t.Fatalf("UpdateWorkflowRunStatus() error = %v", err)
	}

	stalled, err := q.ListStalledWorkflowRuns(ctx, 1*time.Hour)
	if err != nil {
		t.Fatalf("ListStalledWorkflowRuns() error = %v", err)
	}

	found := false
	for _, r := range stalled {
		if r.ID == run.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("stalled run %s not found in results (len=%d)", run.ID, len(stalled))
	}
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
	if err := q.CreateWorkflowRun(ctx, run); err != nil {
		t.Fatalf("CreateWorkflowRun() error = %v", err)
	}
	if err := q.UpdateWorkflowRunStatus(ctx, run.ID, domain.WfStatusPending, domain.WfStatusRunning, map[string]any{"started_at": time.Now().UTC()}); err != nil {
		t.Fatalf("UpdateWorkflowRunStatus() error = %v", err)
	}

	stalled, err := q.ListStalledWorkflowRuns(ctx, 1*time.Hour)
	if err != nil {
		t.Fatalf("ListStalledWorkflowRuns() error = %v", err)
	}
	for _, r := range stalled {
		if r.ID == run.ID {
			t.Fatal("recently started run should not be stalled")
		}
	}
}

func TestWorkflowRun_ListStalledWorkflowRuns_Empty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	stalled, err := q.ListStalledWorkflowRuns(ctx, 1*time.Hour)
	if err != nil {
		t.Fatalf("ListStalledWorkflowRuns() error = %v", err)
	}
	if len(stalled) != 0 {
		t.Fatalf("len = %d, want 0", len(stalled))
	}
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
		if err := q.CreateWorkflowRun(ctx, run); err != nil {
			t.Fatalf("CreateWorkflowRun() error = %v", err)
		}
	}

	count, err := q.CountActiveWorkflowRunsByVersion(ctx, wf.ID, versionID)
	if err != nil {
		t.Fatalf("CountActiveWorkflowRunsByVersion() error = %v", err)
	}
	if count != 2 {
		t.Fatalf("count = %d, want 2", count)
	}
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
	if err := q.CreateWorkflowRun(ctx, pending); err != nil {
		t.Fatalf("CreateWorkflowRun(pending) error = %v", err)
	}

	completed := testutil.BuildWorkflowRun(wf.ID, &testutil.WorkflowRunOpts{
		ProjectID: new(projectID),
		Status:    testutil.Ptr(domain.WfStatusCompleted),
	})
	completed.WorkflowVersionID = versionID
	if err := q.CreateWorkflowRun(ctx, completed); err != nil {
		t.Fatalf("CreateWorkflowRun(completed) error = %v", err)
	}

	count, err := q.CountActiveWorkflowRunsByVersion(ctx, wf.ID, versionID)
	if err != nil {
		t.Fatalf("CountActiveWorkflowRunsByVersion() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}
}

func TestWorkflowRun_CountActiveWorkflowRunsByVersion_Zero(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	count, err := q.CountActiveWorkflowRunsByVersion(ctx, newID(), "v-nonexistent")
	if err != nil {
		t.Fatalf("CountActiveWorkflowRunsByVersion() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0", count)
	}
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
		if err := q.CreateWorkflowRun(ctx, run); err != nil {
			t.Fatalf("CreateWorkflowRun() error = %v", err)
		}
	}

	versions, err := q.ListActiveWorkflowVersions(ctx, wf.ID)
	if err != nil {
		t.Fatalf("ListActiveWorkflowVersions() error = %v", err)
	}
	if len(versions) < 2 {
		t.Fatalf("len = %d, want >= 2", len(versions))
	}
}

func TestWorkflowRun_ListActiveWorkflowVersions_Empty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	versions, err := q.ListActiveWorkflowVersions(ctx, newID())
	if err != nil {
		t.Fatalf("ListActiveWorkflowVersions() error = %v", err)
	}
	if len(versions) != 0 {
		t.Fatalf("len = %d, want 0", len(versions))
	}
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
	if err := q.CreateWorkflowRun(ctx, pending); err != nil {
		t.Fatalf("CreateWorkflowRun(pending) error = %v", err)
	}

	running := testutil.BuildWorkflowRun(wf.ID, &testutil.WorkflowRunOpts{ProjectID: new(projectID)})
	running.WorkflowVersionID = vid
	if err := q.CreateWorkflowRun(ctx, running); err != nil {
		t.Fatalf("CreateWorkflowRun(running) error = %v", err)
	}
	if err := q.UpdateWorkflowRunStatus(ctx, running.ID, domain.WfStatusPending, domain.WfStatusRunning, map[string]any{"started_at": time.Now().UTC()}); err != nil {
		t.Fatalf("UpdateWorkflowRunStatus() error = %v", err)
	}

	versions, err := q.ListActiveWorkflowVersions(ctx, wf.ID)
	if err != nil {
		t.Fatalf("ListActiveWorkflowVersions() error = %v", err)
	}

	found := false
	for _, v := range versions {
		if v.VersionID == vid {
			found = true
			if v.Pending != 1 {
				t.Fatalf("pending = %d, want 1", v.Pending)
			}
			if v.Running != 1 {
				t.Fatalf("running = %d, want 1", v.Running)
			}
			if v.Total != 2 {
				t.Fatalf("total = %d, want 2", v.Total)
			}
		}
	}
	if !found {
		t.Fatal("version not found in results")
	}
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
	if err != nil {
		t.Fatalf("GetOrCreateWorkflowSnapshot() error = %v", err)
	}

	got, err := q.GetWorkflowSnapshot(ctx, snapshot.ID)
	if err != nil {
		t.Fatalf("GetWorkflowSnapshot() error = %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil snapshot")
	}
	if got.WorkflowID != wf.ID {
		t.Fatalf("workflow_id = %q, want %q", got.WorkflowID, wf.ID)
	}

	// Verify definition is valid JSON.
	var def json.RawMessage
	if err := json.Unmarshal(got.Definition, &def); err != nil {
		t.Fatalf("definition is not valid JSON: %v", err)
	}
}

func TestWorkflowSnapshot_GetWorkflowSnapshot_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	got, err := q.GetWorkflowSnapshot(ctx, newID())
	if err != nil {
		t.Fatalf("GetWorkflowSnapshot() error = %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestWorkflowSnapshot_GetWorkflowSnapshot_Dedup(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-wf-snapshot-dedup-" + newID()
	versionID := "vid-" + newID()
	if err := q.SetProjectContext(ctx, projectID); err != nil {
		t.Fatalf("SetProjectContext() error = %v", err)
	}
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
	if err != nil {
		t.Fatalf("first GetOrCreateWorkflowSnapshot() error = %v", err)
	}

	snap2, err := q.GetOrCreateWorkflowSnapshot(ctx, wfObj, nil)
	if err != nil {
		t.Fatalf("second GetOrCreateWorkflowSnapshot() error = %v", err)
	}

	if snap1.ID != snap2.ID {
		t.Fatalf("snapshots should be deduped: %q != %q", snap1.ID, snap2.ID)
	}
}

func TestWorkflowSnapshot_DedupIncludesStepOverrides(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-wf-snapshot-override-" + newID()
	versionID := "vid-" + newID()
	if err := q.SetProjectContext(ctx, projectID); err != nil {
		t.Fatalf("SetProjectContext() error = %v", err)
	}
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
	if err != nil {
		t.Fatalf("GetOrCreateWorkflowSnapshot(full) error = %v", err)
	}
	override, err := q.GetOrCreateWorkflowSnapshot(ctx, wfObj, []domain.WorkflowStep{stepA})
	if err != nil {
		t.Fatalf("GetOrCreateWorkflowSnapshot(override) error = %v", err)
	}
	if full.ID == override.ID {
		t.Fatalf("override snapshot reused full snapshot %q", full.ID)
	}

	overrideAgain, err := q.GetOrCreateWorkflowSnapshot(ctx, wfObj, []domain.WorkflowStep{stepA})
	if err != nil {
		t.Fatalf("GetOrCreateWorkflowSnapshot(override again) error = %v", err)
	}
	if overrideAgain.ID != override.ID {
		t.Fatalf("identical override snapshots should be deduped: %q != %q", overrideAgain.ID, override.ID)
	}

	got, err := q.GetWorkflowSnapshot(ctx, override.ID)
	if err != nil {
		t.Fatalf("GetWorkflowSnapshot(override) error = %v", err)
	}
	def, err := store.ParseSnapshotDefinition(got.Definition)
	if err != nil {
		t.Fatalf("ParseSnapshotDefinition(override) error = %v", err)
	}
	if len(def.Steps) != 1 || def.Steps[0].StepRef != "a" {
		t.Fatalf("override snapshot steps = %+v, want only step a", def.Steps)
	}
}

// ReplayWebhookDelivery.

func TestWebhookDelivery_ReplayWebhookDelivery_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-webhook-replay")
	run := mustCreateRun(t, ctx, q, job)

	original, err := q.EnqueueRunWebhook(ctx, job, run, 3)
	if err != nil {
		t.Fatalf("EnqueueRunWebhook() error = %v", err)
	}

	replayed, err := q.ReplayWebhookDelivery(ctx, original.ID)
	if err != nil {
		t.Fatalf("ReplayWebhookDelivery() error = %v", err)
	}
	if replayed.ID == original.ID {
		t.Fatal("replayed should have a new ID")
	}
	if replayed.Status != domain.WebhookStatusPending {
		t.Fatalf("status = %q, want pending", replayed.Status)
	}
	if replayed.Attempts != 0 {
		t.Fatalf("attempts = %d, want 0", replayed.Attempts)
	}
}

func TestWebhookDelivery_ReplayWebhookDelivery_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, err := q.ReplayWebhookDelivery(ctx, newID())
	if err == nil {
		t.Fatal("expected error for nonexistent delivery")
	}
}

func TestWebhookDelivery_ReplayWebhookDelivery_PreservesJobID(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-webhook-replay-job")
	run := mustCreateRun(t, ctx, q, job)

	original, err := q.EnqueueRunWebhook(ctx, job, run, 3)
	if err != nil {
		t.Fatalf("EnqueueRunWebhook() error = %v", err)
	}

	replayed, err := q.ReplayWebhookDelivery(ctx, original.ID)
	if err != nil {
		t.Fatalf("ReplayWebhookDelivery() error = %v", err)
	}
	if replayed.JobID != original.JobID {
		t.Fatalf("job_id = %q, want %q", replayed.JobID, original.JobID)
	}
}

// CountPendingWebhookDeliveries.

func TestWebhookDelivery_CountPendingWebhookDeliveries_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-webhook-count")
	run := mustCreateRun(t, ctx, q, job)

	if _, err := q.EnqueueRunWebhook(ctx, job, run, 3); err != nil {
		t.Fatalf("EnqueueRunWebhook() error = %v", err)
	}

	count, err := q.CountPendingWebhookDeliveries(ctx)
	if err != nil {
		t.Fatalf("CountPendingWebhookDeliveries() error = %v", err)
	}
	if count < 1 {
		t.Fatalf("count = %d, want >= 1", count)
	}
}

func TestWebhookDelivery_CountPendingWebhookDeliveries_Zero(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	count, err := q.CountPendingWebhookDeliveries(ctx)
	if err != nil {
		t.Fatalf("CountPendingWebhookDeliveries() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0", count)
	}
}

func TestWebhookDelivery_CountPendingWebhookDeliveries_ExcludesDelivered(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-webhook-count-delivered")
	run := mustCreateRun(t, ctx, q, job)

	delivery, err := q.EnqueueRunWebhook(ctx, job, run, 3)
	if err != nil {
		t.Fatalf("EnqueueRunWebhook() error = %v", err)
	}

	// Mark as delivered.
	now := time.Now().UTC()
	delivery.Status = domain.WebhookStatusDelivered
	delivery.DeliveredAt = &now
	if err := q.UpdateWebhookDelivery(ctx, delivery); err != nil {
		t.Fatalf("UpdateWebhookDelivery() error = %v", err)
	}

	count, err := q.CountPendingWebhookDeliveries(ctx)
	if err != nil {
		t.Fatalf("CountPendingWebhookDeliveries() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0", count)
	}
}
