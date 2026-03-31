//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

func TestAgentCRUD(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	project := &domain.Project{ID: newID(), OrgID: "org-agents", Name: "Agents Project"}
	if err := q.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	job := mustCreateJob(t, ctx, q, project.ID)
	agent := &domain.Agent{
		ID:          newID(),
		ProjectID:   project.ID,
		JobID:       job.ID,
		Name:        "Support Agent",
		Slug:        "support-agent",
		Description: "Handles support requests",
		Model:       "gpt-5.4",
		Config:      json.RawMessage(`{"temperature":0.2}`),
		CreatedBy:   "user-1",
		UpdatedBy:   "user-1",
	}
	if err := q.CreateAgent(ctx, agent); err != nil {
		t.Fatalf("CreateAgent() error = %v", err)
	}

	gotByID, err := q.GetAgent(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetAgent() error = %v", err)
	}
	if gotByID.JobID != job.ID {
		t.Fatalf("GetAgent().JobID = %q, want %q", gotByID.JobID, job.ID)
	}

	gotBySlug, err := q.GetAgentBySlug(ctx, project.ID, agent.Slug)
	if err != nil {
		t.Fatalf("GetAgentBySlug() error = %v", err)
	}
	if gotBySlug.ID != agent.ID {
		t.Fatalf("GetAgentBySlug().ID = %q, want %q", gotBySlug.ID, agent.ID)
	}

	byJobIDs, err := q.ListAgentsByJobIDs(ctx, project.ID, []string{job.ID, "missing"})
	if err != nil {
		t.Fatalf("ListAgentsByJobIDs() error = %v", err)
	}
	if len(byJobIDs) != 1 {
		t.Fatalf("ListAgentsByJobIDs() len = %d, want 1", len(byJobIDs))
	}
	if byJobIDs[0].ID != agent.ID {
		t.Fatalf("ListAgentsByJobIDs()[0].ID = %q, want %q", byJobIDs[0].ID, agent.ID)
	}

	items, err := q.ListAgents(ctx, project.ID, 10, nil)
	if err != nil {
		t.Fatalf("ListAgents() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("ListAgents() len = %d, want 1", len(items))
	}

	agent.Name = "Updated Agent"
	agent.Slug = "updated-agent"
	agent.Config = json.RawMessage(`{"temperature":0.5}`)
	if err := q.UpdateAgent(ctx, agent); err != nil {
		t.Fatalf("UpdateAgent() error = %v", err)
	}

	updated, err := q.GetAgent(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetAgent(updated) error = %v", err)
	}
	if updated.Slug != "updated-agent" {
		t.Fatalf("updated.Slug = %q, want updated-agent", updated.Slug)
	}

	if err := q.DeleteAgent(ctx, agent.ID); err != nil {
		t.Fatalf("DeleteAgent() error = %v", err)
	}
	if _, err := q.GetAgent(ctx, agent.ID); !errors.Is(err, store.ErrAgentNotFound) {
		t.Fatalf("GetAgent(after delete) error = %v, want ErrAgentNotFound", err)
	}
}

func TestAgentDeploymentCRUD(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	project := &domain.Project{ID: newID(), OrgID: "org-agents", Name: "Agents Project"}
	if err := q.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	job := mustCreateJob(t, ctx, q, project.ID)
	agent := &domain.Agent{
		ID:        newID(),
		ProjectID: project.ID,
		JobID:     job.ID,
		Name:      "Support Agent",
		Slug:      "support-agent",
		Model:     "gpt-5.4",
		Config:    json.RawMessage(`{"temperature":0.2}`),
	}
	if err := q.CreateAgent(ctx, agent); err != nil {
		t.Fatalf("CreateAgent() error = %v", err)
	}

	version, err := q.NextAgentDeploymentVersion(ctx, agent.ID)
	if err != nil {
		t.Fatalf("NextAgentDeploymentVersion() error = %v", err)
	}
	if version != 1 {
		t.Fatalf("NextAgentDeploymentVersion() = %d, want 1", version)
	}

	deployment := &domain.AgentDeployment{
		ID:             newID(),
		AgentID:        agent.ID,
		Version:        version,
		Status:         domain.AgentDeploymentStatusPending,
		Provider:       "local_stub",
		ConfigSnapshot: agent.Config,
		CreatedBy:      "user-1",
	}
	if err := q.CreateAgentDeployment(ctx, deployment); err != nil {
		t.Fatalf("CreateAgentDeployment() error = %v", err)
	}

	if err := q.UpdateAgentDeployment(ctx, deployment.ID, map[string]any{
		"status":            string(domain.AgentDeploymentStatusDeployed),
		"provider_metadata": json.RawMessage(`{"provider":"local_stub"}`),
	}); err != nil {
		t.Fatalf("UpdateAgentDeployment() error = %v", err)
	}

	latest, err := q.GetLatestAgentDeployment(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetLatestAgentDeployment() error = %v", err)
	}
	if latest.Status != domain.AgentDeploymentStatusDeployed {
		t.Fatalf("latest.Status = %s, want deployed", latest.Status)
	}

	items, err := q.ListAgentDeployments(ctx, agent.ID, 10, nil)
	if err != nil {
		t.Fatalf("ListAgentDeployments() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("ListAgentDeployments() len = %d, want 1", len(items))
	}
}

func TestAgentNullableActors(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	project := &domain.Project{ID: newID(), OrgID: "org-agents", Name: "Agents Project"}
	if err := q.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	job := mustCreateJob(t, ctx, q, project.ID)
	agent := &domain.Agent{
		ID:          newID(),
		ProjectID:   project.ID,
		JobID:       job.ID,
		Name:        "Nullable Actor Agent",
		Slug:        "nullable-actor-agent",
		Description: "",
		Model:       "gpt-5.4",
		Config:      json.RawMessage(`{"temperature":0.1}`),
	}
	if err := q.CreateAgent(ctx, agent); err != nil {
		t.Fatalf("CreateAgent() error = %v", err)
	}

	got, err := q.GetAgent(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetAgent() error = %v", err)
	}
	if got.CreatedBy != "" || got.UpdatedBy != "" {
		t.Fatalf("GetAgent() actor fields = %q/%q, want empty strings", got.CreatedBy, got.UpdatedBy)
	}

	items, err := q.ListAgents(ctx, project.ID, 10, nil)
	if err != nil {
		t.Fatalf("ListAgents() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("ListAgents() len = %d, want 1", len(items))
	}

	deployment := &domain.AgentDeployment{
		ID:             newID(),
		AgentID:        agent.ID,
		Version:        1,
		Status:         domain.AgentDeploymentStatusPending,
		Provider:       "local_stub",
		ConfigSnapshot: json.RawMessage(`{"temperature":0.1}`),
	}
	if err := q.CreateAgentDeployment(ctx, deployment); err != nil {
		t.Fatalf("CreateAgentDeployment() error = %v", err)
	}

	latestDeployment, err := q.GetLatestAgentDeployment(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetLatestAgentDeployment() error = %v", err)
	}
	if latestDeployment.CreatedBy != "" {
		t.Fatalf("GetLatestAgentDeployment().CreatedBy = %q, want empty string", latestDeployment.CreatedBy)
	}

	deployments, err := q.ListAgentDeployments(ctx, agent.ID, 10, nil)
	if err != nil {
		t.Fatalf("ListAgentDeployments() error = %v", err)
	}
	if len(deployments) != 1 {
		t.Fatalf("ListAgentDeployments() len = %d, want 1", len(deployments))
	}
}

func TestGetAgentTopologyEdgesEmpty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	project := &domain.Project{ID: newID(), OrgID: "org-topo", Name: "Topo Project"}
	if err := q.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	edges, err := q.GetAgentTopologyEdges(ctx, project.ID)
	if err != nil {
		t.Fatalf("GetAgentTopologyEdges() error = %v", err)
	}
	if len(edges) != 0 {
		t.Fatalf("expected 0 edges, got %d", len(edges))
	}
}

func TestGetAgentTopologyEdgesWithMessages(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	project := &domain.Project{ID: newID(), OrgID: "org-topo2", Name: "Topo Project 2"}
	if err := q.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	// Create two agents with a backing job each.
	jobA := mustCreateJob(t, ctx, q, project.ID)
	agentA := &domain.Agent{
		ID: newID(), ProjectID: project.ID, JobID: jobA.ID,
		Name: "Agent A", Slug: "agent-a", Model: "gpt-5.4",
		Config: json.RawMessage(`{}`),
	}
	if err := q.CreateAgent(ctx, agentA); err != nil {
		t.Fatalf("CreateAgent(A) error = %v", err)
	}

	jobB := mustCreateJob(t, ctx, q, project.ID)
	agentB := &domain.Agent{
		ID: newID(), ProjectID: project.ID, JobID: jobB.ID,
		Name: "Agent B", Slug: "agent-b", Model: "gpt-5.4",
		Config: json.RawMessage(`{}`),
	}
	if err := q.CreateAgent(ctx, agentB); err != nil {
		t.Fatalf("CreateAgent(B) error = %v", err)
	}

	// Send two messages from A to B.
	for i := 0; i < 2; i++ {
		msg := &domain.AgentMessage{
			ID: newID(), ProjectID: project.ID,
			SourceAgentID: agentA.ID, TargetAgentID: agentB.ID,
			ChainID: "chain-1", ChainDepth: i + 1,
			Payload: json.RawMessage(`{}`), Status: domain.AgentMessagePending,
		}
		if err := q.CreateAgentMessage(ctx, msg); err != nil {
			t.Fatalf("CreateAgentMessage() error = %v", err)
		}
	}

	edges, err := q.GetAgentTopologyEdges(ctx, project.ID)
	if err != nil {
		t.Fatalf("GetAgentTopologyEdges() error = %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	if edges[0].SourceAgentID != agentA.ID {
		t.Fatalf("edge source = %q, want %q", edges[0].SourceAgentID, agentA.ID)
	}
	if edges[0].TargetAgentID != agentB.ID {
		t.Fatalf("edge target = %q, want %q", edges[0].TargetAgentID, agentB.ID)
	}
	if edges[0].MessageCount != 2 {
		t.Fatalf("edge message_count = %d, want 2", edges[0].MessageCount)
	}
}

func TestCountProjectRunsSince(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	project := &domain.Project{ID: newID(), OrgID: "org-count", Name: "Count Project"}
	if err := q.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	job := mustCreateJob(t, ctx, q, project.ID)

	// Create two runs.
	for i := 0; i < 2; i++ {
		run := &domain.JobRun{
			ID: newID(), JobID: job.ID, ProjectID: project.ID,
			Status: domain.StatusCompleted, Attempt: 1,
			TriggeredBy: domain.TriggerManual,
		}
		if err := q.CreateRun(ctx, run); err != nil {
			t.Fatalf("CreateRun() error = %v", err)
		}
	}

	// Count from the beginning of time should find both.
	count, err := q.CountProjectRunsSince(ctx, project.ID, time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("CountProjectRunsSince() error = %v", err)
	}
	if count != 2 {
		t.Fatalf("count = %d, want 2", count)
	}

	// Count from the future should find none.
	count, err = q.CountProjectRunsSince(ctx, project.ID, time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("CountProjectRunsSince() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0", count)
	}
}

// -- GetAgentBySlug tests.

func TestGetAgentBySlug_Found(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	project := &domain.Project{ID: newID(), OrgID: "org-slug", Name: "Slug Project"}
	if err := q.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	job := mustCreateJob(t, ctx, q, project.ID)
	agent := &domain.Agent{
		ID: newID(), ProjectID: project.ID, JobID: job.ID,
		Name: "Slug Agent", Slug: "slug-agent", Model: "gpt-5.4",
		Config: json.RawMessage(`{"temperature":0.5}`),
	}
	if err := q.CreateAgent(ctx, agent); err != nil {
		t.Fatalf("CreateAgent() error = %v", err)
	}

	found, err := q.GetAgentBySlug(ctx, project.ID, "slug-agent")
	if err != nil {
		t.Fatalf("GetAgentBySlug() error = %v", err)
	}
	if found.ID != agent.ID {
		t.Fatalf("ID = %q, want %q", found.ID, agent.ID)
	}
	if found.Model != "gpt-5.4" {
		t.Fatalf("Model = %q, want gpt-5.4", found.Model)
	}
}

func TestGetAgentBySlug_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, err := q.GetAgentBySlug(ctx, "proj-nonexist", "no-such-slug")
	if !errors.Is(err, store.ErrAgentNotFound) {
		t.Fatalf("expected ErrAgentNotFound, got %v", err)
	}
}

func TestGetAgentBySlug_WrongProject(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	project := &domain.Project{ID: newID(), OrgID: "org-slug2", Name: "P1"}
	if err := q.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	job := mustCreateJob(t, ctx, q, project.ID)
	agent := &domain.Agent{
		ID: newID(), ProjectID: project.ID, JobID: job.ID,
		Name: "A", Slug: "cross-project-slug", Model: "gpt-5.4",
		Config: json.RawMessage(`{}`),
	}
	if err := q.CreateAgent(ctx, agent); err != nil {
		t.Fatalf("CreateAgent() error = %v", err)
	}

	_, err := q.GetAgentBySlug(ctx, "different-project-id", "cross-project-slug")
	if !errors.Is(err, store.ErrAgentNotFound) {
		t.Fatalf("expected ErrAgentNotFound for wrong project, got %v", err)
	}
}

// -- ListAgentsByJobIDs tests.

func TestListAgentsByJobIDs_ReturnsMatching(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	project := &domain.Project{ID: newID(), OrgID: "org-jobids", Name: "JobIDs Project"}
	if err := q.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	job1 := mustCreateJob(t, ctx, q, project.ID)
	job2 := mustCreateJob(t, ctx, q, project.ID)
	job3 := mustCreateJob(t, ctx, q, project.ID)

	for i, job := range []*domain.Job{job1, job2, job3} {
		a := &domain.Agent{
			ID: newID(), ProjectID: project.ID, JobID: job.ID,
			Name: "Agent", Slug: "agent-" + string(rune('a'+i)), Model: "gpt-5.4",
			Config: json.RawMessage(`{}`),
		}
		if err := q.CreateAgent(ctx, a); err != nil {
			t.Fatalf("CreateAgent(%d) error = %v", i, err)
		}
	}

	agents, err := q.ListAgentsByJobIDs(ctx, project.ID, []string{job1.ID, job3.ID})
	if err != nil {
		t.Fatalf("ListAgentsByJobIDs() error = %v", err)
	}
	if len(agents) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(agents))
	}
}

func TestListAgentsByJobIDs_EmptyIDs(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	agents, err := q.ListAgentsByJobIDs(ctx, "proj-1", []string{})
	if err != nil {
		t.Fatalf("ListAgentsByJobIDs() error = %v", err)
	}
	if len(agents) != 0 {
		t.Fatalf("expected 0 agents, got %d", len(agents))
	}
}

// -- ListAgents pagination tests.

func TestListAgents_EmptyProject(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	agents, err := q.ListAgents(ctx, "proj-empty", 10, nil)
	if err != nil {
		t.Fatalf("ListAgents() error = %v", err)
	}
	if len(agents) != 0 {
		t.Fatalf("expected 0 agents, got %d", len(agents))
	}
}

// -- UpdateAgent model_fallbacks tests.

func TestUpdateAgent_ModelFallbacks(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	project := &domain.Project{ID: newID(), OrgID: "org-fb", Name: "Fallback Project"}
	if err := q.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	job := mustCreateJob(t, ctx, q, project.ID)
	agent := &domain.Agent{
		ID: newID(), ProjectID: project.ID, JobID: job.ID,
		Name: "FB Agent", Slug: "fb-agent", Model: "claude-sonnet-4-6",
		Config: json.RawMessage(`{}`),
	}
	if err := q.CreateAgent(ctx, agent); err != nil {
		t.Fatalf("CreateAgent() error = %v", err)
	}

	agent.ModelFallbacks = []string{"gpt-5.4-mini", "claude-haiku-4-5"}
	if err := q.UpdateAgent(ctx, agent); err != nil {
		t.Fatalf("UpdateAgent() error = %v", err)
	}

	updated, err := q.GetAgent(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetAgent() error = %v", err)
	}
	if len(updated.ModelFallbacks) != 2 {
		t.Fatalf("ModelFallbacks len = %d, want 2", len(updated.ModelFallbacks))
	}
	if updated.ModelFallbacks[0] != "gpt-5.4-mini" {
		t.Fatalf("ModelFallbacks[0] = %q", updated.ModelFallbacks[0])
	}
}

// -- DeleteAgent tests.

func TestDeleteAgent_Success(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	project := &domain.Project{ID: newID(), OrgID: "org-del", Name: "Del Project"}
	if err := q.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	job := mustCreateJob(t, ctx, q, project.ID)
	agent := &domain.Agent{
		ID: newID(), ProjectID: project.ID, JobID: job.ID,
		Name: "Del Agent", Slug: "del-agent", Model: "gpt-5.4",
		Config: json.RawMessage(`{}`),
	}
	if err := q.CreateAgent(ctx, agent); err != nil {
		t.Fatalf("CreateAgent() error = %v", err)
	}

	if err := q.DeleteAgent(ctx, agent.ID); err != nil {
		t.Fatalf("DeleteAgent() error = %v", err)
	}

	_, err := q.GetAgent(ctx, agent.ID)
	if !errors.Is(err, store.ErrAgentNotFound) {
		t.Fatalf("expected ErrAgentNotFound after delete, got %v", err)
	}
}

func TestDeleteAgent_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	err := q.DeleteAgent(ctx, "nonexistent-agent-id")
	if !errors.Is(err, store.ErrAgentNotFound) {
		t.Fatalf("expected ErrAgentNotFound, got %v", err)
	}
}

// -- NextAgentDeploymentVersion tests.

func TestNextAgentDeploymentVersion_Increments(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	project := &domain.Project{ID: newID(), OrgID: "org-ver", Name: "Ver Project"}
	if err := q.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	job := mustCreateJob(t, ctx, q, project.ID)
	agent := &domain.Agent{
		ID: newID(), ProjectID: project.ID, JobID: job.ID,
		Name: "Ver Agent", Slug: "ver-agent", Model: "gpt-5.4",
		Config: json.RawMessage(`{}`),
	}
	if err := q.CreateAgent(ctx, agent); err != nil {
		t.Fatalf("CreateAgent() error = %v", err)
	}

	for want := 1; want <= 3; want++ {
		v, err := q.NextAgentDeploymentVersion(ctx, agent.ID)
		if err != nil {
			t.Fatalf("NextAgentDeploymentVersion(%d) error = %v", want, err)
		}
		if v != want {
			t.Fatalf("version = %d, want %d", v, want)
		}
		dep := &domain.AgentDeployment{
			ID: newID(), AgentID: agent.ID, Version: v,
			Status: domain.AgentDeploymentStatusPending, Provider: "local_stub",
			ConfigSnapshot: json.RawMessage(`{}`),
		}
		if err := q.CreateAgentDeployment(ctx, dep); err != nil {
			t.Fatalf("CreateAgentDeployment(%d) error = %v", want, err)
		}
	}
}

// -- UpdateAgentDeployment tests.

func TestUpdateAgentDeployment_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	err := q.UpdateAgentDeployment(ctx, "nonexistent-dep-id", map[string]any{
		"status": string(domain.AgentDeploymentStatusFailed),
	})
	if err == nil {
		t.Fatal("expected error for non-existent deployment")
	}
}

// -- Agent Message CRUD tests.

// mustCreateAgentPair creates two agents in the project and returns their IDs.
func mustCreateAgentPair(t *testing.T, ctx context.Context, q *store.Queries, projectID string) (srcID, tgtID string) {
	t.Helper()
	jobA := mustCreateJob(t, ctx, q, projectID)
	jobB := mustCreateJob(t, ctx, q, projectID)
	agentA := &domain.Agent{
		ID: newID(), ProjectID: projectID, JobID: jobA.ID,
		Name: "Source Agent", Slug: "src-agent-" + newID()[:8], Model: "gpt-5.4",
		Config: json.RawMessage(`{}`),
	}
	agentB := &domain.Agent{
		ID: newID(), ProjectID: projectID, JobID: jobB.ID,
		Name: "Target Agent", Slug: "tgt-agent-" + newID()[:8], Model: "gpt-5.4",
		Config: json.RawMessage(`{}`),
	}
	if err := q.CreateAgent(ctx, agentA); err != nil {
		t.Fatalf("CreateAgent(A) error = %v", err)
	}
	if err := q.CreateAgent(ctx, agentB); err != nil {
		t.Fatalf("CreateAgent(B) error = %v", err)
	}
	return agentA.ID, agentB.ID
}

func TestAgentMessageCRUD(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	project := &domain.Project{ID: newID(), OrgID: "org-msg", Name: "Msg Project"}
	if err := q.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	srcID, tgtID := mustCreateAgentPair(t, ctx, q, project.ID)

	msg := &domain.AgentMessage{
		ID: newID(), ProjectID: project.ID,
		SourceAgentID: srcID, TargetAgentID: tgtID,
		ChainID: "chain-1", ChainDepth: 1,
		Payload: json.RawMessage(`{"text":"hello"}`),
		Status:  domain.AgentMessagePending,
	}
	if err := q.CreateAgentMessage(ctx, msg); err != nil {
		t.Fatalf("CreateAgentMessage() error = %v", err)
	}
	if msg.CreatedAt.IsZero() {
		t.Fatal("CreatedAt not set")
	}

	// GetAgentMessage.
	got, err := q.GetAgentMessage(ctx, msg.ID)
	if err != nil {
		t.Fatalf("GetAgentMessage() error = %v", err)
	}
	if got.SourceAgentID != srcID {
		t.Fatalf("SourceAgentID = %q", got.SourceAgentID)
	}
	if got.Status != domain.AgentMessagePending {
		t.Fatalf("Status = %q, want pending", got.Status)
	}

	// UpdateAgentMessageStatus to delivered.
	now := time.Now().UTC()
	if err := q.UpdateAgentMessageStatus(ctx, msg.ID, domain.AgentMessageDelivered, map[string]any{
		"delivered_at": now,
	}); err != nil {
		t.Fatalf("UpdateAgentMessageStatus() error = %v", err)
	}

	delivered, err := q.GetAgentMessage(ctx, msg.ID)
	if err != nil {
		t.Fatalf("GetAgentMessage() after update error = %v", err)
	}
	if delivered.Status != domain.AgentMessageDelivered {
		t.Fatalf("Status = %q, want delivered", delivered.Status)
	}
	if delivered.DeliveredAt == nil {
		t.Fatal("DeliveredAt not set after delivery")
	}
}

func TestGetAgentMessage_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	_, err := q.GetAgentMessage(ctx, "nonexistent-msg-id")
	if !errors.Is(err, store.ErrAgentMessageNotFound) {
		t.Fatalf("expected ErrAgentMessageNotFound, got %v", err)
	}
}

func TestUpdateAgentMessageStatus_Failed(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	project := &domain.Project{ID: newID(), OrgID: "org-msgfail", Name: "Fail Project"}
	if err := q.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	srcID, tgtID := mustCreateAgentPair(t, ctx, q, project.ID)

	msg := &domain.AgentMessage{
		ID: newID(), ProjectID: project.ID,
		SourceAgentID: srcID, TargetAgentID: tgtID,
		ChainID: "chain-fail", ChainDepth: 1,
		Payload: json.RawMessage(`{}`), Status: domain.AgentMessagePending,
	}
	if err := q.CreateAgentMessage(ctx, msg); err != nil {
		t.Fatalf("CreateAgentMessage() error = %v", err)
	}

	if err := q.UpdateAgentMessageStatus(ctx, msg.ID, domain.AgentMessageFailed, map[string]any{
		"error": "target agent not deployed",
	}); err != nil {
		t.Fatalf("UpdateAgentMessageStatus(failed) error = %v", err)
	}

	failed, err := q.GetAgentMessage(ctx, msg.ID)
	if err != nil {
		t.Fatalf("GetAgentMessage() error = %v", err)
	}
	if failed.Status != domain.AgentMessageFailed {
		t.Fatalf("Status = %q, want failed", failed.Status)
	}
	if failed.Error != "target agent not deployed" {
		t.Fatalf("Error = %q", failed.Error)
	}
}

// -- ListAgentMessagesByChain tests.

func TestListAgentMessagesByChain_OrderedByDepth(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	project := &domain.Project{ID: newID(), OrgID: "org-chain", Name: "Chain Project"}
	if err := q.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	srcID, tgtID := mustCreateAgentPair(t, ctx, q, project.ID)

	chainID := "chain-ordered"
	for depth := 3; depth >= 1; depth-- {
		msg := &domain.AgentMessage{
			ID: newID(), ProjectID: project.ID,
			SourceAgentID: srcID, TargetAgentID: tgtID,
			ChainID: chainID, ChainDepth: depth,
			Payload: json.RawMessage(`{}`), Status: domain.AgentMessagePending,
		}
		if err := q.CreateAgentMessage(ctx, msg); err != nil {
			t.Fatalf("CreateAgentMessage(depth=%d) error = %v", depth, err)
		}
	}

	messages, err := q.ListAgentMessagesByChain(ctx, chainID)
	if err != nil {
		t.Fatalf("ListAgentMessagesByChain() error = %v", err)
	}
	if len(messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(messages))
	}
	if messages[0].ChainDepth != 1 || messages[2].ChainDepth != 3 {
		t.Fatalf("not ordered by depth: [%d, %d, %d]", messages[0].ChainDepth, messages[1].ChainDepth, messages[2].ChainDepth)
	}
}

func TestListAgentMessagesByChain_EmptyChain(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	messages, err := q.ListAgentMessagesByChain(ctx, "nonexistent-chain")
	if err != nil {
		t.Fatalf("ListAgentMessagesByChain() error = %v", err)
	}
	if len(messages) != 0 {
		t.Fatalf("expected 0 messages, got %d", len(messages))
	}
}

func TestListAgentMessagesByChain_DoesNotMixChains(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	project := &domain.Project{ID: newID(), OrgID: "org-mix", Name: "Mix Project"}
	if err := q.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	srcID, tgtID := mustCreateAgentPair(t, ctx, q, project.ID)

	for _, chain := range []string{"chain-A", "chain-B"} {
		msg := &domain.AgentMessage{
			ID: newID(), ProjectID: project.ID,
			SourceAgentID: srcID, TargetAgentID: tgtID,
			ChainID: chain, ChainDepth: 1,
			Payload: json.RawMessage(`{}`), Status: domain.AgentMessagePending,
		}
		if err := q.CreateAgentMessage(ctx, msg); err != nil {
			t.Fatalf("CreateAgentMessage(%s) error = %v", chain, err)
		}
	}

	messagesA, err := q.ListAgentMessagesByChain(ctx, "chain-A")
	if err != nil {
		t.Fatalf("ListAgentMessagesByChain(A) error = %v", err)
	}
	if len(messagesA) != 1 {
		t.Fatalf("chain-A: expected 1 message, got %d", len(messagesA))
	}
}

// -- ListPendingAgentMessages tests.

func TestListPendingAgentMessages_ReturnsPending(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	project := &domain.Project{ID: newID(), OrgID: "org-pending", Name: "Pending Project"}
	if err := q.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	srcID, tgtID := mustCreateAgentPair(t, ctx, q, project.ID)

	// Create one pending and one delivered.
	pending := &domain.AgentMessage{
		ID: newID(), ProjectID: project.ID,
		SourceAgentID: srcID, TargetAgentID: tgtID,
		ChainID: "chain-pend", ChainDepth: 1,
		Payload: json.RawMessage(`{}`), Status: domain.AgentMessagePending,
	}
	delivered := &domain.AgentMessage{
		ID: newID(), ProjectID: project.ID,
		SourceAgentID: srcID, TargetAgentID: tgtID,
		ChainID: "chain-pend", ChainDepth: 2,
		Payload: json.RawMessage(`{}`), Status: domain.AgentMessagePending,
	}
	if err := q.CreateAgentMessage(ctx, pending); err != nil {
		t.Fatalf("CreateAgentMessage(pending) error = %v", err)
	}
	if err := q.CreateAgentMessage(ctx, delivered); err != nil {
		t.Fatalf("CreateAgentMessage(delivered) error = %v", err)
	}
	if err := q.UpdateAgentMessageStatus(ctx, delivered.ID, domain.AgentMessageDelivered, map[string]any{
		"delivered_at": time.Now().UTC(),
	}); err != nil {
		t.Fatalf("UpdateAgentMessageStatus() error = %v", err)
	}

	messages, err := q.ListPendingAgentMessages(ctx, 50)
	if err != nil {
		t.Fatalf("ListPendingAgentMessages() error = %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 pending message, got %d", len(messages))
	}
	if messages[0].ID != pending.ID {
		t.Fatalf("pending message ID = %q, want %q", messages[0].ID, pending.ID)
	}
}

func TestListPendingAgentMessages_Empty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	messages, err := q.ListPendingAgentMessages(ctx, 50)
	if err != nil {
		t.Fatalf("ListPendingAgentMessages() error = %v", err)
	}
	if len(messages) != 0 {
		t.Fatalf("expected 0 messages, got %d", len(messages))
	}
}

// -- ListAgentMessagesByAgent tests.

func TestListAgentMessagesByAgent_ReturnsSourceAndTarget(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	project := &domain.Project{ID: newID(), OrgID: "org-byagent", Name: "ByAgent Project"}
	if err := q.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	srcID, tgtID := mustCreateAgentPair(t, ctx, q, project.ID)
	agentID := srcID
	// Message where agent is source.
	m1 := &domain.AgentMessage{
		ID: newID(), ProjectID: project.ID,
		SourceAgentID: agentID, TargetAgentID: tgtID,
		ChainID: "chain-dir", ChainDepth: 1,
		Payload: json.RawMessage(`{}`), Status: domain.AgentMessagePending,
	}
	// Message where agent is target.
	m2 := &domain.AgentMessage{
		ID: newID(), ProjectID: project.ID,
		SourceAgentID: tgtID, TargetAgentID: agentID,
		ChainID: "chain-dir", ChainDepth: 2,
		Payload: json.RawMessage(`{}`), Status: domain.AgentMessagePending,
	}
	if err := q.CreateAgentMessage(ctx, m1); err != nil {
		t.Fatalf("CreateAgentMessage(source) error = %v", err)
	}
	if err := q.CreateAgentMessage(ctx, m2); err != nil {
		t.Fatalf("CreateAgentMessage(target) error = %v", err)
	}

	messages, err := q.ListAgentMessagesByAgent(ctx, agentID, 50, nil)
	if err != nil {
		t.Fatalf("ListAgentMessagesByAgent() error = %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
}

func TestListAgentMessagesByAgent_WrongAgent(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	messages, err := q.ListAgentMessagesByAgent(ctx, "nonexistent-agent", 50, nil)
	if err != nil {
		t.Fatalf("ListAgentMessagesByAgent() error = %v", err)
	}
	if len(messages) != 0 {
		t.Fatalf("expected 0 messages, got %d", len(messages))
	}
}
