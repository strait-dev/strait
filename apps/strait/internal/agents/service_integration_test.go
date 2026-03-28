//go:build integration

package agents_test

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"slices"
	"sync"
	"testing"

	"strait/internal/agents"
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

func TestServiceLifecycleReusesJobRuns(t *testing.T) {
	ctx := context.Background()
	q := store.New(testDB.Pool)
	mustClean(t, ctx)
	projectID := mustCreateProject(t, ctx, q)

	svc := agents.NewService(q, testDB.Pool)
	agent, err := svc.CreateAgent(ctx, agents.CreateAgentRequest{
		ProjectID:   projectID,
		Name:        "Support Agent",
		Slug:        "support-agent",
		Description: "Handles support tickets",
		Model:       "gpt-5.4",
		Config:      json.RawMessage(`{"temperature":0.2}`),
		Actor:       "user-1",
	})
	if err != nil {
		t.Fatalf("CreateAgent() error = %v", err)
	}

	backingJob, err := q.GetJob(ctx, agent.JobID)
	if err != nil {
		t.Fatalf("GetJob() error = %v", err)
	}
	if backingJob.Enabled {
		t.Fatal("expected backing job to be disabled")
	}
	if backingJob.Slug != "__agent__support-agent" {
		t.Fatalf("backingJob.Slug = %q, want __agent__support-agent", backingJob.Slug)
	}

	deployment, err := svc.DeployAgent(ctx, projectID, agent.ID, "user-1")
	if err != nil {
		t.Fatalf("DeployAgent() error = %v", err)
	}
	if deployment.Status != domain.AgentDeploymentStatusDeployed {
		t.Fatalf("deployment.Status = %s, want deployed", deployment.Status)
	}

	run, err := svc.RunAgent(ctx, agents.RunAgentRequest{
		ProjectID: projectID,
		AgentID:   agent.ID,
		Payload:   json.RawMessage(`{"prompt":"hello"}`),
		Actor:     "user-1",
	})
	if err != nil {
		t.Fatalf("RunAgent() error = %v", err)
	}
	if run.JobID != agent.JobID {
		t.Fatalf("run.JobID = %q, want %q", run.JobID, agent.JobID)
	}
	if run.Status != domain.StatusCompleted {
		t.Fatalf("run.Status = %s, want completed", run.Status)
	}

	storedRun, err := q.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if storedRun.JobID != agent.JobID {
		t.Fatalf("storedRun.JobID = %q, want %q", storedRun.JobID, agent.JobID)
	}

	runs, err := svc.ListAgentRuns(ctx, projectID, agent.ID, 10, 0)
	if err != nil {
		t.Fatalf("ListAgentRuns() error = %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("ListAgentRuns() len = %d, want 1", len(runs))
	}

	if err := svc.DeleteAgent(ctx, projectID, agent.ID); err != nil {
		t.Fatalf("DeleteAgent() error = %v", err)
	}
	if _, err := q.GetAgent(ctx, agent.ID); !errors.Is(err, store.ErrAgentNotFound) {
		t.Fatalf("GetAgent(after delete) error = %v, want ErrAgentNotFound", err)
	}
}

func TestServiceRunAgentFailurePersistsFailedRun(t *testing.T) {
	ctx := context.Background()
	q := store.New(testDB.Pool)
	mustClean(t, ctx)
	projectID := mustCreateProject(t, ctx, q)

	svc := agents.NewService(q, testDB.Pool)
	agent, err := svc.CreateAgent(ctx, agents.CreateAgentRequest{
		ProjectID: projectID,
		Name:      "Failure Agent",
		Slug:      "failure-agent",
		Model:     "gpt-5.4",
		Actor:     "user-1",
	})
	if err != nil {
		t.Fatalf("CreateAgent() error = %v", err)
	}
	if _, err := svc.DeployAgent(ctx, projectID, agent.ID, "user-1"); err != nil {
		t.Fatalf("DeployAgent() error = %v", err)
	}

	run, err := svc.RunAgent(ctx, agents.RunAgentRequest{
		ProjectID: projectID,
		AgentID:   agent.ID,
		Payload:   json.RawMessage(`{"_stub_error":"boom"}`),
		Actor:     "user-1",
	})
	if err != nil {
		t.Fatalf("RunAgent() error = %v", err)
	}
	if run.Status != domain.StatusFailed {
		t.Fatalf("run.Status = %s, want failed", run.Status)
	}

	events, err := q.ListEvents(ctx, run.ID, 10, nil)
	if err != nil {
		t.Fatalf("ListEvents() error = %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("ListEvents() len = %d, want 2", len(events))
	}
	if events[1].Type != domain.EventError {
		t.Fatalf("events[1].Type = %s, want error", events[1].Type)
	}
}

func TestServiceDeployAgentConcurrentVersions(t *testing.T) {
	ctx := context.Background()
	q := store.New(testDB.Pool)
	mustClean(t, ctx)
	projectID := mustCreateProject(t, ctx, q)

	svc := agents.NewService(q, testDB.Pool)
	agent, err := svc.CreateAgent(ctx, agents.CreateAgentRequest{
		ProjectID: projectID,
		Name:      "Concurrent Agent",
		Slug:      "concurrent-agent",
		Model:     "gpt-5.4",
		Actor:     "user-1",
	})
	if err != nil {
		t.Fatalf("CreateAgent() error = %v", err)
	}

	var wg sync.WaitGroup
	results := make(chan *domain.AgentDeployment, 2)
	errs := make(chan error, 2)

	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			deployment, err := svc.DeployAgent(ctx, projectID, agent.ID, "user-1")
			if err != nil {
				errs <- err
				return
			}
			results <- deployment
		}()
	}

	wg.Wait()
	close(results)
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("DeployAgent() concurrent error = %v", err)
		}
	}

	versions := make([]int, 0, 2)
	for deployment := range results {
		versions = append(versions, deployment.Version)
		if deployment.Status != domain.AgentDeploymentStatusDeployed {
			t.Fatalf("deployment.Status = %s, want deployed", deployment.Status)
		}
	}

	slices.Sort(versions)
	if !slices.Equal(versions, []int{1, 2}) {
		t.Fatalf("versions = %v, want [1 2]", versions)
	}
}

func mustClean(t *testing.T, ctx context.Context) {
	t.Helper()
	if err := testDB.CleanTables(ctx); err != nil {
		t.Fatalf("CleanTables() error = %v", err)
	}
}

func mustCreateProject(t *testing.T, ctx context.Context, q *store.Queries) string {
	t.Helper()

	project := &domain.Project{
		ID:    "proj-" + newID(),
		OrgID: "org-" + newID(),
		Name:  "Agents Test Project",
	}
	if err := q.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	return project.ID
}

func newID() string {
	return uuid.Must(uuid.NewV7()).String()
}
