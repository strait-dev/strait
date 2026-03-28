//go:build integration

package workflow

import (
	"context"
	"encoding/json"
	"log"
	"log/slog"
	"os"
	"sync"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"
	"strait/internal/testutil"

	"github.com/google/uuid"
)

var integrationDB *testutil.TestDB

func TestMain(m *testing.M) {
	ctx := context.Background()

	var err error
	integrationDB, err = testutil.SetupTestDB(ctx, "../../migrations")
	if err != nil {
		log.Fatalf("setup test db: %v", err)
	}

	code := m.Run()
	integrationDB.Cleanup(ctx)
	os.Exit(code)
}

type integrationQueue struct {
	mu   sync.Mutex
	runs []*domain.JobRun
}

func (q *integrationQueue) Enqueue(_ context.Context, run *domain.JobRun) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if run.ID == "" {
		run.ID = uuid.Must(uuid.NewV7()).String()
	}
	run.Status = domain.StatusQueued

	clone := *run
	q.runs = append(q.runs, &clone)
	return nil
}

func TestStepCallback_OnJobRunTerminal_DynamicExpansionIntegration(t *testing.T) {
	ctx := context.Background()
	if err := integrationDB.CleanTables(ctx); err != nil {
		t.Fatalf("CleanTables() error = %v", err)
	}

	q := store.New(integrationDB.Pool)
	project := &domain.Project{ID: uuid.Must(uuid.NewV7()).String(), OrgID: "org-1", Name: "Workflow Integration"}
	if err := q.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	job := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{
		ProjectID:   testutil.Ptr(project.ID),
		Name:        testutil.Ptr("Planner"),
		Slug:        testutil.Ptr("planner"),
		EndpointURL: testutil.Ptr("https://example.com/planner"),
	})
	workerJob := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{
		ProjectID:   testutil.Ptr(project.ID),
		Name:        testutil.Ptr("Worker"),
		Slug:        testutil.Ptr("worker"),
		EndpointURL: testutil.Ptr("https://example.com/worker"),
	})
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: testutil.Ptr(project.ID),
		Name:      testutil.Ptr("Runtime Expansion"),
		Slug:      testutil.Ptr("runtime-expansion"),
	})
	step := testutil.MustCreateWorkflowStep(t, ctx, q, wf.ID, &testutil.WorkflowStepOpts{
		JobID:   testutil.Ptr(job.ID),
		StepRef: testutil.Ptr("plan"),
	})
	if err := q.CreateWorkflowVersionSnapshot(ctx, wf.ID, wf.Version); err != nil {
		t.Fatalf("CreateWorkflowVersionSnapshot() error = %v", err)
	}

	statusRunning := domain.WfStatusRunning
	wfRun := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{
		ProjectID: testutil.Ptr(project.ID),
		Status:    &statusRunning,
	})
	stepRun := testutil.MustCreateWorkflowStepRun(t, ctx, q, wfRun.ID, step.ID, &testutil.WorkflowStepRunOpts{
		StepRef:  testutil.Ptr(step.StepRef),
		JobRunID: testutil.Ptr("run-plan"),
		Status:   testutil.Ptr(domain.StepRunning),
	})

	queue := &integrationQueue{}
	engine := NewWorkflowEngine(q, queue, slog.Default())
	callback := NewStepCallback(q, engine, slog.Default())

	err := callback.OnJobRunTerminal(ctx, &domain.JobRun{
		ID:                "run-plan",
		JobID:             job.ID,
		WorkflowStepRunID: stepRun.ID,
		Status:            domain.StatusCompleted,
		Result: json.RawMessage(`{
			"dynamic_steps": [
				{
					"step_ref": "draft",
					"job_id": "` + workerJob.ID + `",
					"depends_on": ["plan"],
					"payload": {"kind": "draft"}
				}
			]
		}`),
	})
	if err != nil {
		t.Fatalf("OnJobRunTerminal() error = %v", err)
	}

	dynamicSteps, err := q.ListDynamicWorkflowStepsByWorkflowRun(ctx, wfRun.ID)
	if err != nil {
		t.Fatalf("ListDynamicWorkflowStepsByWorkflowRun() error = %v", err)
	}
	if len(dynamicSteps) != 1 {
		t.Fatalf("len(dynamicSteps) = %d, want 1", len(dynamicSteps))
	}

	stepRuns, err := q.ListStepRunsByWorkflowRun(ctx, wfRun.ID, 100, nil)
	if err != nil {
		t.Fatalf("ListStepRunsByWorkflowRun() error = %v", err)
	}
	if len(stepRuns) != 2 {
		t.Fatalf("len(stepRuns) = %d, want 2", len(stepRuns))
	}

	var draftRun *domain.WorkflowStepRun
	for i := range stepRuns {
		if stepRuns[i].StepRef == "draft" {
			draftRun = &stepRuns[i]
			break
		}
	}
	if draftRun == nil {
		t.Fatal("dynamic step run draft not found")
	}
	if draftRun.Status != domain.StepRunning {
		t.Fatalf("draftRun.Status = %s, want running", draftRun.Status)
	}
	if draftRun.JobRunID == "" {
		t.Fatal("draftRun.JobRunID = empty, want enqueued job run id")
	}

	queue.mu.Lock()
	defer queue.mu.Unlock()
	if len(queue.runs) != 1 {
		t.Fatalf("len(queue.runs) = %d, want 1", len(queue.runs))
	}
	if queue.runs[0].JobID != workerJob.ID {
		t.Fatalf("queued JobID = %q, want %q", queue.runs[0].JobID, workerJob.ID)
	}
}
