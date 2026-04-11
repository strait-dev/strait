//go:build integration

package agents_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"strait/internal/agents"
	"strait/internal/domain"
	"strait/internal/store"
)

// Cross-tenant adversarial tests for the agents service. These sit
// above the pure RLS tests in internal/store/rls_agents_isolation_*.go
// and exercise the service-layer guards that catch cross-tenant
// spoofing attempts before they reach the database:
//
//   * Passing another project's environment_id to DeployAgentToEnv
//     (guarded at service.go:605: env.ProjectID != agent.ProjectID)
//   * Passing another project's deployment_id via the canary router
//   * Replaying another project's run through ReplayAgentRun
//
// These run in plain integration mode (no RLS tx wrapping) because the
// test is whether the service rejects the request — RLS gives defense
// in depth but the primary guard is the application check.

func TestCrossTenant_DeployAgentToEnv_EnvironmentIDSpoof(t *testing.T) {
	ctx := context.Background()
	q := store.New(testDB.Pool)
	mustClean(t, ctx)

	projA := mustCreateProject(t, ctx, q)
	projB := mustCreateProject(t, ctx, q)

	// Environment lives in project B. An attacker on project A can
	// read the env ID from a leaked log or a probing attack and tries
	// to deploy agent A into that env.
	envB := &domain.Environment{
		ID: "env-spoof-" + newID(), ProjectID: projB,
		Name: "prod", Slug: "prod",
	}
	if err := q.CreateEnvironment(ctx, envB); err != nil {
		t.Fatalf("CreateEnvironment(B) error = %v", err)
	}

	svc := agents.NewService(q, testDB.Pool, agents.WithJWTSigningKey(runtimeTestJWTKey))
	defer closeService(svc)

	agentA, err := svc.CreateAgent(ctx, agents.CreateAgentRequest{
		ProjectID: projA,
		Name:      "Spoofer",
		Slug:      "spoofer",
		Model:     "gpt-5.4",
		Actor:     "attacker",
	})
	if err != nil {
		t.Fatalf("CreateAgent(A) error = %v", err)
	}

	_, err = svc.DeployAgentToEnv(ctx, projA, agentA.ID, envB.ID, "attacker")
	if err == nil {
		t.Fatal("expected DeployAgentToEnv to reject a cross-project environment_id")
	}
	if !strings.Contains(err.Error(), "does not belong") && !strings.Contains(err.Error(), "environment") {
		t.Logf("note: error message did not mention environment ownership: %v", err)
	}
}

func TestCrossTenant_RunAgent_EnvironmentIDSpoof(t *testing.T) {
	// If the attacker passes a valid environment_id from another
	// project to RunAgent, the store-level
	// GetLatestAgentDeploymentByEnvironment will not find a deployment
	// for (agent_id=A, environment_id=B), so the flow must return
	// ErrNotDeployed rather than accidentally dispatching.
	ctx := context.Background()
	q := store.New(testDB.Pool)
	mustClean(t, ctx)

	projA := mustCreateProject(t, ctx, q)
	projB := mustCreateProject(t, ctx, q)

	envB := &domain.Environment{
		ID: "env-spoof-run-" + newID(), ProjectID: projB,
		Name: "prod", Slug: "prod",
	}
	if err := q.CreateEnvironment(ctx, envB); err != nil {
		t.Fatalf("CreateEnvironment(B) error = %v", err)
	}

	svc := agents.NewService(q, testDB.Pool, agents.WithJWTSigningKey(runtimeTestJWTKey))
	defer closeService(svc)

	agentA, err := svc.CreateAgent(ctx, agents.CreateAgentRequest{
		ProjectID: projA,
		Name:      "Spoofer Run",
		Slug:      "spoofer-run",
		Model:     "gpt-5.4",
		Actor:     "attacker",
	})
	if err != nil {
		t.Fatalf("CreateAgent() error = %v", err)
	}

	_, err = svc.RunAgent(ctx, agents.RunAgentRequest{
		ProjectID:     projA,
		AgentID:       agentA.ID,
		EnvironmentID: envB.ID, // B's env passed by an attacker on A
		Payload:       json.RawMessage(`{}`),
		Actor:         "attacker",
	})
	if err == nil {
		t.Fatal("expected cross-tenant RunAgent to fail")
	}
}

func TestCrossTenant_ReplayAgentRun_FromOtherProject(t *testing.T) {
	// Attacker knows the run ID of a run in another project. Replay
	// must reject because the run belongs to a different agent.
	ctx := context.Background()
	q := store.New(testDB.Pool)
	mustClean(t, ctx)

	projA := mustCreateProject(t, ctx, q)
	projB := mustCreateProject(t, ctx, q)

	svc := agents.NewService(q, testDB.Pool, agents.WithJWTSigningKey(runtimeTestJWTKey))
	defer closeService(svc)

	agentA, err := svc.CreateAgent(ctx, agents.CreateAgentRequest{
		ProjectID: projA,
		Name:      "A Run Owner",
		Slug:      "a-run-owner",
		Model:     "gpt-5.4",
		Actor:     "user-1",
	})
	if err != nil {
		t.Fatalf("CreateAgent(A) error = %v", err)
	}
	agentB, err := svc.CreateAgent(ctx, agents.CreateAgentRequest{
		ProjectID: projB,
		Name:      "B Attacker",
		Slug:      "b-attacker",
		Model:     "gpt-5.4",
		Actor:     "attacker",
	})
	if err != nil {
		t.Fatalf("CreateAgent(B) error = %v", err)
	}

	// Manufacture a terminal job_run under project A so replay has a
	// valid target to attempt.
	run := &domain.JobRun{
		ID:          "run-cross-" + newID(),
		JobID:       agentA.JobID,
		ProjectID:   projA,
		Status:      domain.StatusCompleted,
		Attempt:     1,
		TriggeredBy: domain.TriggerManual,
	}
	if err := q.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	// Attacker tries to replay project A's run against agent B.
	_, err = svc.ReplayAgentRun(ctx, agents.ReplayAgentRunRequest{
		ProjectID:     projB,
		AgentID:       agentB.ID,
		OriginalRunID: run.ID,
		Actor:         "attacker",
	})
	if err == nil {
		t.Fatal("expected ReplayAgentRun to reject cross-project run")
	}
}

func TestCrossTenant_CreateAgent_EnforcesOwnProject(t *testing.T) {
	// Sanity check: CreateAgent must only operate on the project
	// passed in, even though the backing job could theoretically be
	// reparented. The service-layer CreateAgentRequest carries the
	// project_id explicitly; the job it creates inherits it.
	ctx := context.Background()
	q := store.New(testDB.Pool)
	mustClean(t, ctx)

	projA := mustCreateProject(t, ctx, q)
	projB := mustCreateProject(t, ctx, q)

	svc := agents.NewService(q, testDB.Pool, agents.WithJWTSigningKey(runtimeTestJWTKey))
	defer closeService(svc)

	agentA, err := svc.CreateAgent(ctx, agents.CreateAgentRequest{
		ProjectID: projA,
		Name:      "Isolation",
		Slug:      "isolation",
		Model:     "gpt-5.4",
		Actor:     "user-1",
	})
	if err != nil {
		t.Fatalf("CreateAgent() error = %v", err)
	}

	// Under project B's context (no RLS wrapping here, just the
	// service call), GetAgent must still find agent A because we're
	// using a non-RLS query. But the service's GetAgent takes a
	// project_id and filters; asking under project B should fail.
	if _, err := svc.GetAgent(ctx, projB, agentA.ID); err == nil {
		t.Fatal("expected GetAgent(projB, agentA) to fail at the service-layer filter")
	}
}
