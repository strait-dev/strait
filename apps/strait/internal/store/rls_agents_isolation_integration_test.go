//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"
)

// RLS isolation tests for every agent-related table protected by
// migration 000185. Uses the `runAsProject` helper from
// rls_isolation_integration_test.go which wraps store calls in a tx,
// sets app.current_project_id, and drops to the strait_app non-
// superuser role. See that file for the full rationale.
//
// These tests must fail hard if any of the following regress:
//   * migration 000185 policies on agent tables
//   * migration 000185 policies on project_secrets + Phase C quota tables
//   * rlsTxMiddleware / ContextWithTx / ctxAwareDBTX routing

// ---------------------------------------------------------------------------
// agents
// ---------------------------------------------------------------------------

func TestRLS_Agents_CrossTenantIsolation(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	projA := "proj-rls-agents-a-" + newID()
	projB := "proj-rls-agents-b-" + newID()
	if err := q.CreateProject(ctx, &domain.Project{ID: projA, OrgID: "org-rls-a", Name: "A"}); err != nil {
		t.Fatalf("CreateProject(A) error = %v", err)
	}
	if err := q.CreateProject(ctx, &domain.Project{ID: projB, OrgID: "org-rls-b", Name: "B"}); err != nil {
		t.Fatalf("CreateProject(B) error = %v", err)
	}
	jobA := mustCreateJob(t, ctx, q, projA)
	jobB := mustCreateJob(t, ctx, q, projB)

	var agentA, agentB *domain.Agent
	runAsProject(t, ctx, projA, true, func(qtx *store.Queries) {
		agentA = &domain.Agent{
			ID: newID(), ProjectID: projA, JobID: jobA.ID,
			Name: "A Agent", Slug: "a-" + newID(), Model: "gpt-5.4",
			Config: json.RawMessage(`{}`),
		}
		if err := qtx.CreateAgent(ctx, agentA); err != nil {
			t.Fatalf("CreateAgent(A) error = %v", err)
		}
	})
	runAsProject(t, ctx, projB, true, func(qtx *store.Queries) {
		agentB = &domain.Agent{
			ID: newID(), ProjectID: projB, JobID: jobB.ID,
			Name: "B Agent", Slug: "b-" + newID(), Model: "gpt-5.4",
			Config: json.RawMessage(`{}`),
		}
		if err := qtx.CreateAgent(ctx, agentB); err != nil {
			t.Fatalf("CreateAgent(B) error = %v", err)
		}
	})

	// Under project A's context, project B's agent must be invisible.
	runAsProject(t, ctx, projA, false, func(qtx *store.Queries) {
		if _, err := qtx.GetAgent(ctx, agentB.ID); err == nil {
			t.Fatal("expected ErrAgentNotFound for cross-tenant GetAgent")
		}
		items, err := qtx.ListAgents(ctx, projA, 10, nil)
		if err != nil {
			t.Fatalf("ListAgents(A) error = %v", err)
		}
		if len(items) != 1 || items[0].ID != agentA.ID {
			t.Fatalf("ListAgents(A) = %v, want [agentA]", items)
		}
	})

	// And project B sees only its own agent.
	runAsProject(t, ctx, projB, false, func(qtx *store.Queries) {
		items, err := qtx.ListAgents(ctx, projB, 10, nil)
		if err != nil {
			t.Fatalf("ListAgents(B) error = %v", err)
		}
		if len(items) != 1 || items[0].ID != agentB.ID {
			t.Fatalf("ListAgents(B) = %v, want [agentB]", items)
		}
	})
}

func TestRLS_Agents_UpdateIsolation(t *testing.T) {
	// UpdateAgent under project B's context MUST NOT touch an agent in
	// project A. RLS routes the UPDATE through the SELECT filter.
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	projA := "proj-rls-upd-a-" + newID()
	projB := "proj-rls-upd-b-" + newID()
	for _, p := range []string{projA, projB} {
		if err := q.CreateProject(ctx, &domain.Project{ID: p, OrgID: "org-rls-upd-" + p, Name: p}); err != nil {
			t.Fatalf("CreateProject(%s) error = %v", p, err)
		}
	}
	jobA := mustCreateJob(t, ctx, q, projA)
	agentA := &domain.Agent{
		ID: newID(), ProjectID: projA, JobID: jobA.ID,
		Name: "A", Slug: "a-" + newID(), Model: "gpt-5.4",
		Config: json.RawMessage(`{}`),
	}
	if err := q.CreateAgent(ctx, agentA); err != nil {
		t.Fatalf("CreateAgent(A) error = %v", err)
	}

	// Attempt UPDATE under project B's context — the row is filtered out
	// by RLS so the UPDATE affects zero rows and returns ErrAgentNotFound.
	runAsProject(t, ctx, projB, false, func(qtx *store.Queries) {
		copy := *agentA
		copy.Name = "HIJACKED"
		// CreateAgent/UpdateAgent use Exec under the hood. An UPDATE
		// hidden by RLS yields zero rows affected which the store
		// translates to ErrAgentNotFound.
		if err := qtx.UpdateAgent(ctx, &copy); err == nil {
			t.Fatal("expected cross-tenant UpdateAgent to fail under RLS")
		}
	})

	// The original row is untouched.
	got, err := q.GetAgent(ctx, agentA.ID)
	if err != nil {
		t.Fatalf("GetAgent() error = %v", err)
	}
	if got.Name == "HIJACKED" {
		t.Fatal("cross-tenant UPDATE modified project A's agent")
	}
}

func TestRLS_Agents_DeleteIsolation(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	projA := "proj-rls-del-a-" + newID()
	projB := "proj-rls-del-b-" + newID()
	for _, p := range []string{projA, projB} {
		if err := q.CreateProject(ctx, &domain.Project{ID: p, OrgID: "org-rls-del-" + p, Name: p}); err != nil {
			t.Fatalf("CreateProject(%s) error = %v", p, err)
		}
	}
	jobA := mustCreateJob(t, ctx, q, projA)
	agentA := &domain.Agent{
		ID: newID(), ProjectID: projA, JobID: jobA.ID,
		Name: "A", Slug: "a-" + newID(), Model: "gpt-5.4",
		Config: json.RawMessage(`{}`),
	}
	if err := q.CreateAgent(ctx, agentA); err != nil {
		t.Fatalf("CreateAgent(A) error = %v", err)
	}

	runAsProject(t, ctx, projB, false, func(qtx *store.Queries) {
		if err := qtx.DeleteAgent(ctx, agentA.ID); err == nil {
			t.Fatal("expected cross-tenant DeleteAgent to fail under RLS")
		}
	})

	// Row must still exist from the owner's viewpoint.
	if _, err := q.GetAgent(ctx, agentA.ID); err != nil {
		t.Fatalf("GetAgent() after cross-tenant delete = %v, want row still present", err)
	}
}

// ---------------------------------------------------------------------------
// agent_deployments — routed via agents parent
// ---------------------------------------------------------------------------

func TestRLS_AgentDeployments_CrossTenantIsolation(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	projA := "proj-rls-ad-a-" + newID()
	projB := "proj-rls-ad-b-" + newID()
	for _, p := range []string{projA, projB} {
		if err := q.CreateProject(ctx, &domain.Project{ID: p, OrgID: "org-rls-ad-" + p, Name: p}); err != nil {
			t.Fatalf("CreateProject(%s) error = %v", p, err)
		}
	}
	envA := &domain.Environment{ID: newID(), ProjectID: projA, Name: "prod", Slug: "prod"}
	if err := q.CreateEnvironment(ctx, envA); err != nil {
		t.Fatalf("CreateEnvironment(A) error = %v", err)
	}
	jobA := mustCreateJob(t, ctx, q, projA)
	agentA := &domain.Agent{
		ID: newID(), ProjectID: projA, JobID: jobA.ID,
		Name: "A", Slug: "a-" + newID(), Model: "gpt-5.4",
		Config: json.RawMessage(`{}`),
	}
	if err := q.CreateAgent(ctx, agentA); err != nil {
		t.Fatalf("CreateAgent() error = %v", err)
	}
	dep := &domain.AgentDeployment{
		ID: newID(), AgentID: agentA.ID, EnvironmentID: envA.ID, Version: 1,
		Status: domain.AgentDeploymentStatusDeployed, Provider: "local_stub",
		ConfigSnapshot: json.RawMessage(`{}`),
	}
	if err := q.CreateAgentDeployment(ctx, dep); err != nil {
		t.Fatalf("CreateAgentDeployment() error = %v", err)
	}

	// Under project B's context, the EXISTS policy on agent_deployments
	// routes through agents and finds nothing because agent A is
	// invisible.
	runAsProject(t, ctx, projB, false, func(qtx *store.Queries) {
		if _, err := qtx.GetAgentDeploymentByID(ctx, dep.ID); err == nil {
			t.Fatal("expected cross-tenant GetAgentDeploymentByID to fail under RLS")
		}
		items, err := qtx.ListAgentDeployments(ctx, agentA.ID, 10, nil)
		if err != nil {
			t.Fatalf("ListAgentDeployments() error = %v", err)
		}
		if len(items) != 0 {
			t.Fatalf("got %d deployments, want 0 under cross-tenant RLS", len(items))
		}
	})
}

// ---------------------------------------------------------------------------
// agent_canary_deployments
// ---------------------------------------------------------------------------

func TestRLS_AgentCanaryDeployments_CrossTenantIsolation(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	projA := "proj-rls-canary-a-" + newID()
	projB := "proj-rls-canary-b-" + newID()
	for _, p := range []string{projA, projB} {
		if err := q.CreateProject(ctx, &domain.Project{ID: p, OrgID: "org-rls-canary-" + p, Name: p}); err != nil {
			t.Fatalf("CreateProject(%s) error = %v", p, err)
		}
	}
	jobA := mustCreateJob(t, ctx, q, projA)
	agentA := &domain.Agent{
		ID: newID(), ProjectID: projA, JobID: jobA.ID,
		Name: "A", Slug: "a-" + newID(), Model: "gpt-5.4",
		Config: json.RawMessage(`{}`),
	}
	if err := q.CreateAgent(ctx, agentA); err != nil {
		t.Fatalf("CreateAgent() error = %v", err)
	}
	source := &domain.AgentDeployment{
		ID: newID(), AgentID: agentA.ID, Version: 1,
		Status: domain.AgentDeploymentStatusDeployed, Provider: "local_stub",
		ConfigSnapshot: json.RawMessage(`{}`),
	}
	target := &domain.AgentDeployment{
		ID: newID(), AgentID: agentA.ID, Version: 2,
		Status: domain.AgentDeploymentStatusDeployed, Provider: "local_stub",
		ConfigSnapshot: json.RawMessage(`{}`),
	}
	if err := q.CreateAgentDeployment(ctx, source); err != nil {
		t.Fatalf("CreateAgentDeployment(source) error = %v", err)
	}
	if err := q.CreateAgentDeployment(ctx, target); err != nil {
		t.Fatalf("CreateAgentDeployment(target) error = %v", err)
	}
	canary := &domain.AgentCanaryDeployment{
		ID:                 newID(),
		AgentID:            agentA.ID,
		ProjectID:          projA,
		SourceDeploymentID: source.ID,
		TargetDeploymentID: target.ID,
		TrafficPct:         25,
		Status:             "active",
	}
	if err := q.CreateAgentCanaryDeployment(ctx, canary); err != nil {
		t.Fatalf("CreateAgentCanaryDeployment() error = %v", err)
	}

	runAsProject(t, ctx, projB, false, func(qtx *store.Queries) {
		got, err := qtx.GetActiveAgentCanary(ctx, agentA.ID)
		if err != nil {
			t.Fatalf("GetActiveAgentCanary() error = %v", err)
		}
		if got != nil {
			t.Fatalf("got canary %v, want nil under cross-tenant RLS", got)
		}
	})
}

// ---------------------------------------------------------------------------
// agent_messages
// ---------------------------------------------------------------------------

func TestRLS_AgentMessages_CrossTenantIsolation(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	projA := "proj-rls-msg-a-" + newID()
	projB := "proj-rls-msg-b-" + newID()
	for _, p := range []string{projA, projB} {
		if err := q.CreateProject(ctx, &domain.Project{ID: p, OrgID: "org-rls-msg-" + p, Name: p}); err != nil {
			t.Fatalf("CreateProject(%s) error = %v", p, err)
		}
	}
	jobA := mustCreateJob(t, ctx, q, projA)
	agentA := &domain.Agent{
		ID: newID(), ProjectID: projA, JobID: jobA.ID,
		Name: "A", Slug: "a-" + newID(), Model: "gpt-5.4",
		Config: json.RawMessage(`{}`),
	}
	if err := q.CreateAgent(ctx, agentA); err != nil {
		t.Fatalf("CreateAgent() error = %v", err)
	}

	chainID := "chain-rls-" + newID()
	msg := &domain.AgentMessage{
		ID:            newID(),
		ProjectID:     projA,
		SourceAgentID: agentA.ID,
		TargetAgentID: agentA.ID,
		ChainID:       chainID,
		ChainDepth:    0,
		Payload:       json.RawMessage(`{"secret":"cross-tenant-must-not-leak"}`),
		Status:        domain.AgentMessagePending,
	}
	if err := q.CreateAgentMessage(ctx, msg); err != nil {
		t.Fatalf("CreateAgentMessage() error = %v", err)
	}

	runAsProject(t, ctx, projB, false, func(qtx *store.Queries) {
		got, err := qtx.ListAgentMessagesByChain(ctx, chainID)
		if err != nil {
			t.Fatalf("ListAgentMessagesByChain() error = %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("got %d messages, want 0 under cross-tenant RLS (payload would leak)", len(got))
		}
	})
}

// ---------------------------------------------------------------------------
// project_secrets — the Phase D platform primitive
// ---------------------------------------------------------------------------

func TestRLS_ProjectSecrets_CrossTenantIsolation_ListByEnv(t *testing.T) {
	// The worry: an attacker knows a valid environment_id from another
	// project and passes it to ListProjectSecretsByEnv. RLS must stop
	// the read even though the env_id matches a real row in the db.
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStoreWithEncryption(t)
	projA := "proj-rls-sec-a-" + newID()
	projB := "proj-rls-sec-b-" + newID()
	for _, p := range []string{projA, projB} {
		if err := q.CreateProject(ctx, &domain.Project{ID: p, OrgID: "org-rls-sec-" + p, Name: p}); err != nil {
			t.Fatalf("CreateProject(%s) error = %v", p, err)
		}
	}
	envA := &domain.Environment{ID: newID(), ProjectID: projA, Name: "prod", Slug: "prod"}
	if err := q.CreateEnvironment(ctx, envA); err != nil {
		t.Fatalf("CreateEnvironment() error = %v", err)
	}

	// Write a secret under project A.
	if err := q.CreateProjectSecret(ctx, &domain.ProjectSecret{
		ProjectID:      projA,
		EnvironmentID:  envA.ID,
		SecretKey:      "DATABASE_URL",
		EncryptedValue: "postgres://very-secret",
	}); err != nil {
		t.Fatalf("CreateProjectSecret() error = %v", err)
	}

	// Under project B's context, the list must return zero rows even
	// when passing projA's actual environment_id.
	runAsProject(t, ctx, projB, false, func(qtx *store.Queries) {
		qtx.SetSecretEncryptionKey("test-secret-encryption-key-32chr!")
		got, err := qtx.ListProjectSecretsByEnv(ctx, projA, envA.ID)
		if err != nil {
			t.Fatalf("ListProjectSecretsByEnv() error = %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("got %d secrets under cross-tenant context, want 0", len(got))
		}
	})
}

func TestRLS_ProjectSecrets_CrossTenantIsolation_ListForJob(t *testing.T) {
	// The Jobs worker path: ListProjectSecretsForJob merges project-wide
	// and job-scoped secrets. Both branches must be filtered by RLS.
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStoreWithEncryption(t)
	projA := "proj-rls-secjob-a-" + newID()
	projB := "proj-rls-secjob-b-" + newID()
	for _, p := range []string{projA, projB} {
		if err := q.CreateProject(ctx, &domain.Project{ID: p, OrgID: "org-rls-secjob-" + p, Name: p}); err != nil {
			t.Fatalf("CreateProject(%s) error = %v", p, err)
		}
	}
	envA := &domain.Environment{ID: newID(), ProjectID: projA, Name: "prod", Slug: "prod"}
	if err := q.CreateEnvironment(ctx, envA); err != nil {
		t.Fatalf("CreateEnvironment() error = %v", err)
	}
	jobA := mustCreateJob(t, ctx, q, projA)

	if err := q.CreateProjectSecret(ctx, &domain.ProjectSecret{
		ProjectID:      projA,
		EnvironmentID:  envA.ID,
		SecretKey:      "PROJECT_WIDE",
		EncryptedValue: "pw-value",
	}); err != nil {
		t.Fatalf("CreateProjectSecret(pw) error = %v", err)
	}
	if err := q.CreateProjectSecret(ctx, &domain.ProjectSecret{
		ProjectID:      projA,
		EnvironmentID:  envA.ID,
		JobID:          jobA.ID,
		SecretKey:      "JOB_OVERRIDE",
		EncryptedValue: "job-value",
	}); err != nil {
		t.Fatalf("CreateProjectSecret(job) error = %v", err)
	}

	runAsProject(t, ctx, projB, false, func(qtx *store.Queries) {
		qtx.SetSecretEncryptionKey("test-secret-encryption-key-32chr!")
		got, err := qtx.ListProjectSecretsForJob(ctx, projA, jobA.ID, envA.ID)
		if err != nil {
			t.Fatalf("ListProjectSecretsForJob() error = %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("got %d secrets via cross-tenant ListProjectSecretsForJob, want 0", len(got))
		}
	})
}

// ---------------------------------------------------------------------------
// Phase C split quota tables
// ---------------------------------------------------------------------------

func TestRLS_ProjectAgentQuota_CrossTenantIsolation(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	projA := "proj-rls-aq-a-" + newID()
	projB := "proj-rls-aq-b-" + newID()
	for _, p := range []string{projA, projB} {
		if err := q.CreateProject(ctx, &domain.Project{ID: p, OrgID: "org-rls-aq-" + p, Name: p}); err != nil {
			t.Fatalf("CreateProject(%s) error = %v", p, err)
		}
	}
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO project_agent_quotas (project_id, max_agents, max_agent_runs_per_month, max_agent_channels)
		VALUES ($1, 10, 500, 3)
	`, projA); err != nil {
		t.Fatalf("insert project_agent_quotas(A): %v", err)
	}

	runAsProject(t, ctx, projB, false, func(qtx *store.Queries) {
		got, err := qtx.GetProjectAgentQuota(ctx, projA)
		if err != nil {
			t.Fatalf("GetProjectAgentQuota(A) under B error = %v", err)
		}
		if got != nil {
			t.Fatalf("got %+v, want nil under cross-tenant RLS", got)
		}
	})
}

func TestRLS_ProjectJobQuota_CrossTenantIsolation(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	projA := "proj-rls-jq-a-" + newID()
	projB := "proj-rls-jq-b-" + newID()
	for _, p := range []string{projA, projB} {
		if err := q.CreateProject(ctx, &domain.Project{ID: p, OrgID: "org-rls-jq-" + p, Name: p}); err != nil {
			t.Fatalf("CreateProject(%s) error = %v", p, err)
		}
	}
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO project_job_quotas (project_id, max_jobs, max_queued_runs, max_executing_runs)
		VALUES ($1, 50, 500, 100)
	`, projA); err != nil {
		t.Fatalf("insert project_job_quotas(A): %v", err)
	}

	runAsProject(t, ctx, projB, false, func(qtx *store.Queries) {
		got, err := qtx.GetProjectJobQuota(ctx, projA)
		if err != nil {
			t.Fatalf("GetProjectJobQuota(A) under B error = %v", err)
		}
		if got != nil {
			t.Fatalf("got %+v, want nil under cross-tenant RLS", got)
		}
	})
}

func TestRLS_ProjectPlatformSettings_CrossTenantIsolation(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	projA := "proj-rls-ps-a-" + newID()
	projB := "proj-rls-ps-b-" + newID()
	for _, p := range []string{projA, projB} {
		if err := q.CreateProject(ctx, &domain.Project{ID: p, OrgID: "org-rls-ps-" + p, Name: p}); err != nil {
			t.Fatalf("CreateProject(%s) error = %v", p, err)
		}
	}
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO project_platform_settings (project_id, timezone, monthly_budget_microusd)
		VALUES ($1, 'UTC', 999999999)
	`, projA); err != nil {
		t.Fatalf("insert project_platform_settings(A): %v", err)
	}

	runAsProject(t, ctx, projB, false, func(qtx *store.Queries) {
		got, err := qtx.GetProjectPlatformSettings(ctx, projA)
		if err != nil {
			t.Fatalf("GetProjectPlatformSettings(A) under B error = %v", err)
		}
		if got != nil {
			t.Fatalf("got %+v, want nil under cross-tenant RLS", got)
		}
	})
}

// ---------------------------------------------------------------------------
// agent_usage_records
// ---------------------------------------------------------------------------

func TestRLS_AgentUsageRecords_CrossTenantIsolation(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	projA := "proj-rls-usage-a-" + newID()
	projB := "proj-rls-usage-b-" + newID()
	for _, p := range []string{projA, projB} {
		if err := q.CreateProject(ctx, &domain.Project{ID: p, OrgID: "org-rls-usage-" + p, Name: p}); err != nil {
			t.Fatalf("CreateProject(%s) error = %v", p, err)
		}
	}
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO agent_usage_records (
			id, run_id, project_id, org_id, agent_id,
			total_tokens, tool_call_count, run_cost_microusd,
			token_cost_microusd, tool_cost_microusd, total_cost_microusd
		)
		VALUES (gen_random_uuid()::text, $1, $2, $3, $4, 1000, 5, 1000, 50, 100, 1150)
	`, "run-"+newID(), projA, "org-rls-usage-"+projA, "agent-"+newID()); err != nil {
		t.Fatalf("insert agent_usage_records(A): %v", err)
	}

	// Cross-tenant count via raw SQL under the strait_app role.
	got := countAsProject(t, ctx, testDB.Pool, projB,
		"SELECT COUNT(*) FROM agent_usage_records WHERE project_id = $1", projA)
	if got != 0 {
		t.Fatalf("got %d agent_usage_records visible across tenants, want 0", got)
	}
}

// ---------------------------------------------------------------------------
// Phase F3: run_iterations + run_events cross-tenant isolation
// ---------------------------------------------------------------------------

// TestRLS_RunIterations_CrossTenantIsolation verifies the Phase F3
// EXISTS-through-job_runs policy on run_iterations. An iteration
// created under project A must be invisible to project B, even via
// CountRunIterations which takes a raw run_id.
func TestRLS_RunIterations_CrossTenantIsolation(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	projA := "proj-rls-iter-a-" + newID()
	projB := "proj-rls-iter-b-" + newID()
	for _, p := range []string{projA, projB} {
		if err := q.CreateProject(ctx, &domain.Project{ID: p, OrgID: "org-rls-iter-" + p, Name: p}); err != nil {
			t.Fatalf("CreateProject(%s) error = %v", p, err)
		}
	}
	jobA := mustCreateJob(t, ctx, q, projA)
	runA := &domain.JobRun{
		ID: newID(), JobID: jobA.ID, ProjectID: projA,
		Status: domain.StatusExecuting, Attempt: 1,
		TriggeredBy: domain.TriggerManual,
	}
	if err := q.CreateRun(ctx, runA); err != nil {
		t.Fatalf("CreateRun(A) error = %v", err)
	}

	// Insert a run_iteration for project A directly — there's no
	// tenant-context requirement on the writer.
	if err := q.CreateRunIteration(ctx, &domain.RunIteration{
		RunID:       runA.ID,
		Iteration:   1,
		Description: "cross-tenant-must-not-leak",
	}); err != nil {
		t.Fatalf("CreateRunIteration(A) error = %v", err)
	}

	// Sanity: the iteration is visible to its own project.
	if got := countAsProject(t, ctx, testDB.Pool, projA,
		"SELECT COUNT(*) FROM run_iterations WHERE run_id = $1", runA.ID); got != 1 {
		t.Fatalf("own-tenant count = %d, want 1", got)
	}

	// Under project B's RLS context, the iteration must be invisible
	// via both a direct count and via CountRunIterations.
	if got := countAsProject(t, ctx, testDB.Pool, projB,
		"SELECT COUNT(*) FROM run_iterations WHERE run_id = $1", runA.ID); got != 0 {
		t.Fatalf("cross-tenant count = %d, want 0", got)
	}

	runAsProject(t, ctx, projB, false, func(qtx *store.Queries) {
		if got, err := qtx.CountRunIterations(ctx, runA.ID); err != nil {
			t.Fatalf("CountRunIterations() error = %v", err)
		} else if got != 0 {
			t.Fatalf("CountRunIterations() = %d, want 0 under cross-tenant RLS", got)
		}
	})
}

// TestRLS_RunEvents_CrossTenantIsolation verifies the Phase F3
// EXISTS-through-job_runs policy on run_events. An event created
// under project A carries a 'must-not-leak' marker in its data;
// project B must never observe any row.
func TestRLS_RunEvents_CrossTenantIsolation(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	projA := "proj-rls-evt-a-" + newID()
	projB := "proj-rls-evt-b-" + newID()
	for _, p := range []string{projA, projB} {
		if err := q.CreateProject(ctx, &domain.Project{ID: p, OrgID: "org-rls-evt-" + p, Name: p}); err != nil {
			t.Fatalf("CreateProject(%s) error = %v", p, err)
		}
	}
	jobA := mustCreateJob(t, ctx, q, projA)
	runA := &domain.JobRun{
		ID: newID(), JobID: jobA.ID, ProjectID: projA,
		Status: domain.StatusExecuting, Attempt: 1,
		TriggeredBy: domain.TriggerManual,
	}
	if err := q.CreateRun(ctx, runA); err != nil {
		t.Fatalf("CreateRun(A) error = %v", err)
	}
	if err := q.InsertEvent(ctx, &domain.RunEvent{
		RunID:   runA.ID,
		Type:    "error",
		Level:   "error",
		Message: "must-not-leak",
		Data:    mustJSONForTest(map[string]any{"secret": "must-not-leak"}),
	}); err != nil {
		t.Fatalf("InsertEvent(A) error = %v", err)
	}

	// Own-tenant count is 1.
	if got := countAsProject(t, ctx, testDB.Pool, projA,
		"SELECT COUNT(*) FROM run_events WHERE run_id = $1", runA.ID); got != 1 {
		t.Fatalf("own-tenant count = %d, want 1", got)
	}

	// Cross-tenant count must be 0 via both direct SQL and via the
	// ListEvents store method.
	if got := countAsProject(t, ctx, testDB.Pool, projB,
		"SELECT COUNT(*) FROM run_events WHERE run_id = $1", runA.ID); got != 0 {
		t.Fatalf("cross-tenant raw count = %d, want 0", got)
	}

	runAsProject(t, ctx, projB, false, func(qtx *store.Queries) {
		events, err := qtx.ListEvents(ctx, runA.ID, 10, nil)
		if err != nil {
			t.Fatalf("ListEvents() error = %v", err)
		}
		if len(events) != 0 {
			t.Fatalf("got %d events under cross-tenant RLS, want 0 (leaked: %+v)", len(events), events)
		}
	})
}
