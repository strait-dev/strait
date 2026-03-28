//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"
	"strait/internal/testutil"
)

func TestCreateWorkflowDynamicExpansion(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-workflow-dynamic-expansion"
	job := mustCreateJob(t, ctx, q, projectID)
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: testutil.Ptr(projectID),
		Name:      testutil.Ptr("Dynamic Workflow"),
		Slug:      testutil.Ptr("dynamic-workflow"),
	})
	parent := testutil.MustCreateWorkflowStep(t, ctx, q, wf.ID, &testutil.WorkflowStepOpts{
		JobID:   testutil.Ptr(job.ID),
		StepRef: testutil.Ptr("plan"),
	})
	if err := q.CreateWorkflowVersionSnapshot(ctx, wf.ID, wf.Version); err != nil {
		t.Fatalf("CreateWorkflowVersionSnapshot() error = %v", err)
	}

	wfRun := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{ProjectID: testutil.Ptr(projectID)})
	parentRun := testutil.MustCreateWorkflowStepRun(t, ctx, q, wfRun.ID, parent.ID, &testutil.WorkflowStepRunOpts{
		Status:  testutil.Ptr(domain.StepCompleted),
		StepRef: testutil.Ptr(parent.StepRef),
	})

	expansions := []store.DynamicWorkflowExpansion{{
		Step: domain.WorkflowStep{
			JobID:     job.ID,
			StepRef:   "draft",
			DependsOn: []string{"plan"},
			Condition: json.RawMessage(`{"type":"step_status","step_ref":"plan","status":"completed"}`),
			Payload:   json.RawMessage(`{"kind":"draft"}`),
		},
		StepRun: domain.WorkflowStepRun{
			ID:            newID(),
			Status:        domain.StepWaiting,
			DepsCompleted: 0,
			DepsRequired:  1,
		},
	}}

	if err := q.CreateWorkflowDynamicExpansion(ctx, wfRun.ID, parentRun.ID, expansions); err != nil {
		t.Fatalf("CreateWorkflowDynamicExpansion() error = %v", err)
	}

	dynamicSteps, err := q.ListDynamicWorkflowStepsByWorkflowRun(ctx, wfRun.ID)
	if err != nil {
		t.Fatalf("ListDynamicWorkflowStepsByWorkflowRun() error = %v", err)
	}
	if len(dynamicSteps) != 1 {
		t.Fatalf("len(dynamicSteps) = %d, want 1", len(dynamicSteps))
	}
	if dynamicSteps[0].StepRef != "draft" {
		t.Fatalf("dynamic step_ref = %q, want draft", dynamicSteps[0].StepRef)
	}
	if !jsonEqual(dynamicSteps[0].Condition, expansions[0].Step.Condition) {
		t.Fatalf("dynamic step condition = %s, want %s", string(dynamicSteps[0].Condition), string(expansions[0].Step.Condition))
	}
	if !jsonEqual(dynamicSteps[0].Payload, expansions[0].Step.Payload) {
		t.Fatalf("dynamic step payload = %s, want %s", string(dynamicSteps[0].Payload), string(expansions[0].Step.Payload))
	}

	stepRun, err := q.GetWorkflowStepRun(ctx, expansions[0].StepRun.ID)
	if err != nil {
		t.Fatalf("GetWorkflowStepRun() error = %v", err)
	}
	if stepRun.WorkflowStepID != "" {
		t.Fatalf("WorkflowStepID = %q, want empty for dynamic step", stepRun.WorkflowStepID)
	}

	results, err := q.IncrementStepDeps(ctx, wfRun.ID, parent.StepRef)
	if err != nil {
		t.Fatalf("IncrementStepDeps() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].StepRunID != expansions[0].StepRun.ID {
		t.Fatalf("StepRunID = %q, want %q", results[0].StepRunID, expansions[0].StepRun.ID)
	}
	if results[0].JobID != job.ID {
		t.Fatalf("JobID = %q, want %q", results[0].JobID, job.ID)
	}
	if !jsonEqual(results[0].Condition, expansions[0].Step.Condition) {
		t.Fatalf("Condition = %s, want %s", string(results[0].Condition), string(expansions[0].Step.Condition))
	}
	if !jsonEqual(results[0].Payload, expansions[0].Step.Payload) {
		t.Fatalf("Payload = %s, want %s", string(results[0].Payload), string(expansions[0].Step.Payload))
	}
}

func TestCreateWorkflowDynamicExpansion_ConcurrentDuplicateStepRef(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-workflow-dynamic-expansion-concurrent"
	job := mustCreateJob(t, ctx, q, projectID)
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: testutil.Ptr(projectID),
		Name:      testutil.Ptr("Dynamic Workflow Concurrent"),
		Slug:      testutil.Ptr("dynamic-workflow-concurrent"),
	})
	parent := testutil.MustCreateWorkflowStep(t, ctx, q, wf.ID, &testutil.WorkflowStepOpts{
		JobID:   testutil.Ptr(job.ID),
		StepRef: testutil.Ptr("plan"),
	})
	wfRun := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{ProjectID: testutil.Ptr(projectID)})
	parentRun := testutil.MustCreateWorkflowStepRun(t, ctx, q, wfRun.ID, parent.ID, &testutil.WorkflowStepRunOpts{
		Status:  testutil.Ptr(domain.StepCompleted),
		StepRef: testutil.Ptr(parent.StepRef),
	})

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := range 2 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			errs <- q.CreateWorkflowDynamicExpansion(ctx, wfRun.ID, parentRun.ID, []store.DynamicWorkflowExpansion{{
				Step: domain.WorkflowStep{
					JobID:     job.ID,
					StepRef:   "draft",
					DependsOn: []string{"plan"},
				},
				StepRun: domain.WorkflowStepRun{
					ID:            newID(),
					Status:        domain.StepWaiting,
					DepsCompleted: 0,
					DepsRequired:  1,
				},
			}})
		}(i)
	}
	wg.Wait()
	close(errs)

	var successCount int
	var failureCount int
	for err := range errs {
		if err == nil {
			successCount++
			continue
		}
		failureCount++
	}
	if successCount != 1 || failureCount != 1 {
		t.Fatalf("successCount=%d failureCount=%d, want 1/1", successCount, failureCount)
	}

	dynamicSteps, err := q.ListDynamicWorkflowStepsByWorkflowRun(ctx, wfRun.ID)
	if err != nil {
		t.Fatalf("ListDynamicWorkflowStepsByWorkflowRun() error = %v", err)
	}
	if len(dynamicSteps) != 1 {
		t.Fatalf("len(dynamicSteps) = %d, want 1", len(dynamicSteps))
	}
}
