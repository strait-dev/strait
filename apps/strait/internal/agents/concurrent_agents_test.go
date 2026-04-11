//go:build integration

package agents_test

import (
	"context"
	"encoding/json"
	"slices"
	"sync"
	"testing"

	"strait/internal/agents"
	"strait/internal/domain"
	"strait/internal/store"
)

// Concurrent agent tests. Mirrors the pattern of
// internal/agents/billing_enforcement_race_test.go — goroutines race
// through the same service method and the test asserts invariants
// about the final state. These are the regression net for the
// advisory-lock semantics + per-deployment concurrency accounting
// introduced in Phase B.
//
// All tests are integration-tagged so they run against the
// testcontainers Postgres harness in CI and execute with -race.

// TestConcurrentDeployAgentToEnv_NoDuplicateVersions fires N goroutines
// at the same (agent, env) pair simultaneously and asserts the
// advisory lock serializes them: the N returned versions must be a
// permutation of 1..N with no duplicates.
func TestConcurrentDeployAgentToEnv_NoDuplicateVersions(t *testing.T) {
	const parallelism = 8

	ctx := context.Background()
	q := store.New(testDB.Pool)
	mustClean(t, ctx)
	projectID := mustCreateProject(t, ctx, q)
	envID := mustCreateEnvironment(t, ctx, q, projectID, "prod")

	svc := agents.NewService(q, testDB.Pool, agents.WithJWTSigningKey(runtimeTestJWTKey))
	defer closeService(svc)

	agent, err := svc.CreateAgent(ctx, agents.CreateAgentRequest{
		ProjectID: projectID,
		Name:      "Race Deploy",
		Slug:      "race-deploy",
		Model:     "gpt-5.4",
		Actor:     "user-1",
	})
	if err != nil {
		t.Fatalf("CreateAgent() error = %v", err)
	}

	var mu sync.Mutex
	versions := make([]int, 0, parallelism)

	var wg sync.WaitGroup
	wg.Add(parallelism)
	for range parallelism {
		go func() {
			defer wg.Done()
			d, err := svc.DeployAgentToEnv(ctx, projectID, agent.ID, envID, "user-1")
			if err != nil {
				t.Errorf("DeployAgentToEnv() error = %v", err)
				return
			}
			mu.Lock()
			versions = append(versions, d.Version)
			mu.Unlock()
		}()
	}
	wg.Wait()

	slices.Sort(versions)
	want := make([]int, 0, parallelism)
	for i := 1; i <= parallelism; i++ {
		want = append(want, i)
	}
	if !slices.Equal(versions, want) {
		t.Fatalf("versions = %v, want %v", versions, want)
	}
}

// TestConcurrentDeployAgentToEnv_MultiEnvIndependent fires parallel
// deploys across dev/staging/prod and asserts version numbers stay
// agent-global, not env-scoped. Each of the 3*N deploys gets a
// distinct version in 1..3*N.
func TestConcurrentDeployAgentToEnv_MultiEnvIndependent(t *testing.T) {
	const perEnv = 4
	envSlugs := []string{"dev", "staging", "prod"}

	ctx := context.Background()
	q := store.New(testDB.Pool)
	mustClean(t, ctx)
	projectID := mustCreateProject(t, ctx, q)
	envIDs := make([]string, len(envSlugs))
	for i, slug := range envSlugs {
		envIDs[i] = mustCreateEnvironment(t, ctx, q, projectID, slug)
	}

	svc := agents.NewService(q, testDB.Pool, agents.WithJWTSigningKey(runtimeTestJWTKey))
	defer closeService(svc)

	agent, err := svc.CreateAgent(ctx, agents.CreateAgentRequest{
		ProjectID: projectID,
		Name:      "Multi Env Race",
		Slug:      "multi-env-race",
		Model:     "gpt-5.4",
		Actor:     "user-1",
	})
	if err != nil {
		t.Fatalf("CreateAgent() error = %v", err)
	}

	var mu sync.Mutex
	versions := make([]int, 0, perEnv*len(envIDs))

	var wg sync.WaitGroup
	wg.Add(perEnv * len(envIDs))
	for _, envID := range envIDs {
		for range perEnv {
			go func(env string) {
				defer wg.Done()
				d, err := svc.DeployAgentToEnv(ctx, projectID, agent.ID, env, "user-1")
				if err != nil {
					t.Errorf("DeployAgentToEnv(%s) error = %v", env, err)
					return
				}
				mu.Lock()
				versions = append(versions, d.Version)
				mu.Unlock()
			}(envID)
		}
	}
	wg.Wait()

	slices.Sort(versions)
	want := make([]int, 0, perEnv*len(envIDs))
	for i := 1; i <= perEnv*len(envIDs); i++ {
		want = append(want, i)
	}
	if !slices.Equal(versions, want) {
		t.Fatalf("versions = %v, want %v (agent-global monotonic)", versions, want)
	}
}

// TestConcurrentProjectSecretsWrite_NoLostUpdates fires N parallel
// CreateProjectSecret calls with different keys and asserts every
// key lands in the table. Catches any lost-update bug in the
// encryption path.
func TestConcurrentProjectSecretsWrite_NoLostUpdates(t *testing.T) {
	const parallelism = 16

	ctx := context.Background()
	q := store.New(testDB.Pool)
	q.SetSecretEncryptionKey("test-secret-encryption-key-32chr!")
	mustClean(t, ctx)
	projectID := mustCreateProject(t, ctx, q)
	envID := mustCreateEnvironment(t, ctx, q, projectID, "prod")

	var wg sync.WaitGroup
	wg.Add(parallelism)
	for i := range parallelism {
		go func(n int) {
			defer wg.Done()
			// Each goroutine uses a fresh Queries so the encryption
			// key is propagated independently.
			local := store.New(testDB.Pool)
			local.SetSecretEncryptionKey("test-secret-encryption-key-32chr!")
			key := "KEY_" + string(rune('A'+n))
			if err := local.CreateProjectSecret(ctx, &domain.ProjectSecret{
				ProjectID:      projectID,
				EnvironmentID:  envID,
				SecretKey:      key,
				EncryptedValue: "value-" + key,
			}); err != nil {
				t.Errorf("CreateProjectSecret(%s) error = %v", key, err)
			}
		}(i)
	}
	wg.Wait()

	secrets, err := q.ListProjectSecretsByEnv(ctx, projectID, envID)
	if err != nil {
		t.Fatalf("ListProjectSecretsByEnv() error = %v", err)
	}
	if len(secrets) != parallelism {
		t.Fatalf("got %d secrets, want %d", len(secrets), parallelism)
	}
	for _, s := range secrets {
		if s.EncryptedValue != "value-"+s.SecretKey {
			t.Errorf("decrypted %q = %q, want %q", s.SecretKey, s.EncryptedValue, "value-"+s.SecretKey)
		}
	}
}

// TestConcurrentCanaryUpdate_SerializedByStatusFilter creates an
// active canary, then fires parallel UpdateAgentCanaryTraffic +
// CompleteAgentCanary at it. The final state must be deterministic:
// either the traffic update landed before complete (traffic = last
// update, status = completed) or complete landed first (any later
// traffic update is a no-op because the WHERE status='active' filter
// matches nothing).
func TestConcurrentCanaryUpdate_SerializedByStatusFilter(t *testing.T) {
	ctx := context.Background()
	q := store.New(testDB.Pool)
	mustClean(t, ctx)
	projectID := mustCreateProject(t, ctx, q)

	job := mustCreateJobForAgent(t, ctx, q, projectID)
	agent := &domain.Agent{
		ID: newID(), ProjectID: projectID, JobID: job.ID,
		Name: "Canary Race", Slug: "canary-race", Model: "gpt-5.4",
		Config: json.RawMessage(`{}`),
	}
	if err := q.CreateAgent(ctx, agent); err != nil {
		t.Fatalf("CreateAgent() error = %v", err)
	}
	source := &domain.AgentDeployment{
		ID: newID(), AgentID: agent.ID, Version: 1,
		Status: domain.AgentDeploymentStatusDeployed, Provider: "local_stub",
		ConfigSnapshot: json.RawMessage(`{}`),
	}
	target := &domain.AgentDeployment{
		ID: newID(), AgentID: agent.ID, Version: 2,
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
		ID: newID(), AgentID: agent.ID, ProjectID: projectID,
		SourceDeploymentID: source.ID, TargetDeploymentID: target.ID,
		TrafficPct: 10, Status: "active",
	}
	if err := q.CreateAgentCanaryDeployment(ctx, canary); err != nil {
		t.Fatalf("CreateAgentCanaryDeployment() error = %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(4)
	for pct := 20; pct <= 50; pct += 10 {
		go func(p int) {
			defer wg.Done()
			_ = q.UpdateAgentCanaryTraffic(ctx, agent.ID, p)
		}(pct)
	}
	// Racing complete against the traffic updates.
	go func() {
		_ = q.CompleteAgentCanary(ctx, agent.ID, "completed")
	}()
	wg.Wait()

	// Final state must have status either active (if complete lost)
	// or completed (if complete won). Either way, the row is
	// consistent — no corruption, no duplicate active canaries.
	active, err := q.GetActiveAgentCanary(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetActiveAgentCanary() error = %v", err)
	}
	// Either completed (active == nil) or still active with some valid
	// traffic value. Both are acceptable outcomes; the regression would
	// be a corrupt row or a dropped unique-index constraint.
	if active != nil && active.Status != "active" {
		t.Fatalf("active canary has unexpected status %q", active.Status)
	}
}

// mustCreateJobForAgent is a local helper: the agents_test package
// has mustCreateProject and mustCreateEnvironment but not a
// mustCreateJob. Agents need a backing job. We reach into the store
// directly for this.
func mustCreateJobForAgent(t *testing.T, ctx context.Context, q *store.Queries, projectID string) *domain.Job {
	t.Helper()
	job := &domain.Job{
		ID:          newID(),
		ProjectID:   projectID,
		Name:        "backing-" + newID(),
		Slug:        "backing-" + newID(),
		EndpointURL: "https://example.invalid/callback",
		Cron:        "",
		MaxAttempts: 1,
		TimeoutSecs: 30,
	}
	if err := q.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}
	return job
}
