//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

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
