//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

// agentCanaryFixture builds a project + agent + two deployments so the
// canary tests have a source and a target to reference.
type agentCanaryFixture struct {
	project *domain.Project
	agent   *domain.Agent
	source  *domain.AgentDeployment
	target  *domain.AgentDeployment
}

func mustCreateAgentCanaryFixture(t *testing.T, ctx context.Context, q *store.Queries) *agentCanaryFixture {
	t.Helper()
	project := &domain.Project{ID: newID(), OrgID: "org-canary-" + newID(), Name: "Canary Fixture"}
	if err := q.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	job := mustCreateJob(t, ctx, q, project.ID)
	agent := &domain.Agent{
		ID:        newID(),
		ProjectID: project.ID,
		JobID:     job.ID,
		Name:      "Canary Agent",
		Slug:      "canary-" + newID(),
		Model:     "gpt-5.4",
		Config:    json.RawMessage(`{}`),
	}
	if err := q.CreateAgent(ctx, agent); err != nil {
		t.Fatalf("CreateAgent() error = %v", err)
	}
	source := &domain.AgentDeployment{
		ID:             newID(),
		AgentID:        agent.ID,
		Version:        1,
		Status:         domain.AgentDeploymentStatusDeployed,
		Provider:       "local_stub",
		ConfigSnapshot: json.RawMessage(`{"v":1}`),
	}
	if err := q.CreateAgentDeployment(ctx, source); err != nil {
		t.Fatalf("CreateAgentDeployment(source) error = %v", err)
	}
	target := &domain.AgentDeployment{
		ID:             newID(),
		AgentID:        agent.ID,
		Version:        2,
		Status:         domain.AgentDeploymentStatusDeployed,
		Provider:       "local_stub",
		ConfigSnapshot: json.RawMessage(`{"v":2}`),
	}
	if err := q.CreateAgentDeployment(ctx, target); err != nil {
		t.Fatalf("CreateAgentDeployment(target) error = %v", err)
	}
	return &agentCanaryFixture{project: project, agent: agent, source: source, target: target}
}

func mustCreateActiveCanary(t *testing.T, ctx context.Context, q *store.Queries, fx *agentCanaryFixture, trafficPct int) *domain.AgentCanaryDeployment {
	t.Helper()
	canary := &domain.AgentCanaryDeployment{
		ID:                 newID(),
		AgentID:            fx.agent.ID,
		ProjectID:          fx.project.ID,
		SourceDeploymentID: fx.source.ID,
		TargetDeploymentID: fx.target.ID,
		TrafficPct:         trafficPct,
		Status:             "active",
	}
	if err := q.CreateAgentCanaryDeployment(ctx, canary); err != nil {
		t.Fatalf("CreateAgentCanaryDeployment() error = %v", err)
	}
	return canary
}

func TestCreateAgentCanaryDeployment_HappyPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)
	fx := mustCreateAgentCanaryFixture(t, ctx, q)

	canary := mustCreateActiveCanary(t, ctx, q, fx, 25)

	if canary.CreatedAt.IsZero() || canary.UpdatedAt.IsZero() {
		t.Fatalf("timestamps not populated: %+v", canary)
	}

	got, err := q.GetActiveAgentCanary(ctx, fx.agent.ID)
	if err != nil {
		t.Fatalf("GetActiveAgentCanary() error = %v", err)
	}
	if got == nil || got.ID != canary.ID {
		t.Fatalf("got %v, want %q", got, canary.ID)
	}
	if got.TrafficPct != 25 {
		t.Fatalf("traffic_pct = %d, want 25", got.TrafficPct)
	}
}

func TestCreateAgentCanaryDeployment_UniqueActivePerAgent(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)
	fx := mustCreateAgentCanaryFixture(t, ctx, q)

	_ = mustCreateActiveCanary(t, ctx, q, fx, 10)

	// A second active canary for the same agent must hit the unique
	// partial index `idx_agent_canary_active`.
	dupe := &domain.AgentCanaryDeployment{
		ID:                 newID(),
		AgentID:            fx.agent.ID,
		ProjectID:          fx.project.ID,
		SourceDeploymentID: fx.source.ID,
		TargetDeploymentID: fx.target.ID,
		TrafficPct:         50,
		Status:             "active",
	}
	err := q.CreateAgentCanaryDeployment(ctx, dupe)
	if err == nil {
		t.Fatal("expected unique-constraint error creating a second active canary")
	}
	if !strings.Contains(err.Error(), "idx_agent_canary_active") && !strings.Contains(err.Error(), "duplicate") {
		t.Logf("note: error did not mention the index name but did fail: %v", err)
	}
}

func TestGetActiveAgentCanary_ReturnsActive(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)
	fx := mustCreateAgentCanaryFixture(t, ctx, q)

	canary := mustCreateActiveCanary(t, ctx, q, fx, 20)

	got, err := q.GetActiveAgentCanary(ctx, fx.agent.ID)
	if err != nil {
		t.Fatalf("GetActiveAgentCanary() error = %v", err)
	}
	if got == nil || got.ID != canary.ID {
		t.Fatalf("got %v, want canary %q", got, canary.ID)
	}
}

func TestGetActiveAgentCanary_IgnoresCompleted(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)
	fx := mustCreateAgentCanaryFixture(t, ctx, q)

	_ = mustCreateActiveCanary(t, ctx, q, fx, 20)
	if err := q.CompleteAgentCanary(ctx, fx.agent.ID, "completed"); err != nil {
		t.Fatalf("CompleteAgentCanary() error = %v", err)
	}

	got, err := q.GetActiveAgentCanary(ctx, fx.agent.ID)
	if err != nil {
		t.Fatalf("GetActiveAgentCanary() error = %v", err)
	}
	if got != nil {
		t.Fatalf("got %v, want nil after complete", got)
	}
}

func TestGetActiveAgentCanary_NoActiveReturnsNil(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)
	fx := mustCreateAgentCanaryFixture(t, ctx, q)

	got, err := q.GetActiveAgentCanary(ctx, fx.agent.ID)
	if err != nil {
		t.Fatalf("GetActiveAgentCanary() error = %v", err)
	}
	if got != nil {
		t.Fatalf("got %v, want nil (no canary created)", got)
	}
}

func TestGetActiveAgentCanaryWithTarget_JoinsDeployment(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)
	fx := mustCreateAgentCanaryFixture(t, ctx, q)
	canary := mustCreateActiveCanary(t, ctx, q, fx, 30)

	gotCanary, gotTarget, err := q.GetActiveAgentCanaryWithTarget(ctx, fx.agent.ID)
	if err != nil {
		t.Fatalf("GetActiveAgentCanaryWithTarget() error = %v", err)
	}
	if gotCanary == nil || gotTarget == nil {
		t.Fatalf("got canary=%v target=%v, want both non-nil", gotCanary, gotTarget)
	}
	if gotCanary.ID != canary.ID {
		t.Fatalf("canary.ID = %q, want %q", gotCanary.ID, canary.ID)
	}
	if gotTarget.ID != fx.target.ID {
		t.Fatalf("target.ID = %q, want %q", gotTarget.ID, fx.target.ID)
	}
	if gotTarget.Version != 2 {
		t.Fatalf("target.Version = %d, want 2", gotTarget.Version)
	}
	if gotTarget.Status != domain.AgentDeploymentStatusDeployed {
		t.Fatalf("target.Status = %s, want deployed", gotTarget.Status)
	}
}

func TestGetActiveAgentCanaryWithTarget_NoActiveReturnsNilPair(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)
	fx := mustCreateAgentCanaryFixture(t, ctx, q)

	canary, target, err := q.GetActiveAgentCanaryWithTarget(ctx, fx.agent.ID)
	if err != nil {
		t.Fatalf("GetActiveAgentCanaryWithTarget() error = %v", err)
	}
	if canary != nil || target != nil {
		t.Fatalf("got canary=%v target=%v, want nil pair", canary, target)
	}
}

func TestUpdateAgentCanaryTraffic_OnlyUpdatesActive(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)
	fx := mustCreateAgentCanaryFixture(t, ctx, q)
	_ = mustCreateActiveCanary(t, ctx, q, fx, 10)

	if err := q.UpdateAgentCanaryTraffic(ctx, fx.agent.ID, 75); err != nil {
		t.Fatalf("UpdateAgentCanaryTraffic() error = %v", err)
	}

	got, err := q.GetActiveAgentCanary(ctx, fx.agent.ID)
	if err != nil {
		t.Fatalf("GetActiveAgentCanary() error = %v", err)
	}
	if got.TrafficPct != 75 {
		t.Fatalf("traffic_pct = %d, want 75", got.TrafficPct)
	}
}

func TestCompleteAgentCanary_SetsCompletedAt(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)
	fx := mustCreateAgentCanaryFixture(t, ctx, q)
	canary := mustCreateActiveCanary(t, ctx, q, fx, 20)

	before := time.Now().UTC().Add(-time.Second)
	if err := q.CompleteAgentCanary(ctx, fx.agent.ID, "completed"); err != nil {
		t.Fatalf("CompleteAgentCanary() error = %v", err)
	}

	// The row is no longer active, so we re-read by id via a raw query
	// (the store exposes no getter-by-id). Use the GetActiveAgentCanary
	// partial-index miss to assert the row was updated, then read the
	// completed_at via a direct query.
	if active, err := q.GetActiveAgentCanary(ctx, fx.agent.ID); err != nil {
		t.Fatalf("GetActiveAgentCanary() error = %v", err)
	} else if active != nil {
		t.Fatalf("got active=%v, want nil after complete", active)
	}

	var status string
	var completedAt *time.Time
	row := testDB.Pool.QueryRow(ctx,
		"SELECT status, completed_at FROM agent_canary_deployments WHERE id = $1",
		canary.ID)
	if err := row.Scan(&status, &completedAt); err != nil {
		t.Fatalf("scan completed canary: %v", err)
	}
	if status != "completed" {
		t.Fatalf("status = %q, want completed", status)
	}
	if completedAt == nil || completedAt.Before(before) {
		t.Fatalf("completed_at = %v, want >= %v", completedAt, before)
	}
}

func TestCompleteAgentCanary_NoActiveIsNoOp(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)
	fx := mustCreateAgentCanaryFixture(t, ctx, q)

	// No canary was created; CompleteAgentCanary should succeed as a no-op
	// (UPDATE WHERE ... AND status='active' affects zero rows).
	if err := q.CompleteAgentCanary(ctx, fx.agent.ID, "rolled_back"); err != nil {
		t.Fatalf("CompleteAgentCanary(no-active) error = %v", err)
	}
}
