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
