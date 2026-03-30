package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	agentsvc "strait/internal/agents"
	"strait/internal/config"
	"strait/internal/domain"
)

type stubAgentService struct {
	createAgentFunc   func(context.Context, agentsvc.CreateAgentRequest) (*domain.Agent, error)
	getAgentFunc      func(context.Context, string, string) (*domain.Agent, error)
	listAgentsFunc    func(context.Context, string, int, *time.Time) ([]domain.Agent, error)
	updateAgentFunc   func(context.Context, agentsvc.UpdateAgentRequest) (*domain.Agent, error)
	deleteAgentFunc   func(context.Context, string, string) error
	deployAgentFunc   func(context.Context, string, string, string) (*domain.AgentDeployment, error)
	runAgentFunc      func(context.Context, agentsvc.RunAgentRequest) (*domain.JobRun, error)
	listAgentRunsFunc func(context.Context, string, string, int, int) ([]domain.JobRun, error)
}

func (s *stubAgentService) CreateAgent(ctx context.Context, req agentsvc.CreateAgentRequest) (*domain.Agent, error) {
	return s.createAgentFunc(ctx, req)
}

func (s *stubAgentService) GetAgent(ctx context.Context, projectID, agentID string) (*domain.Agent, error) {
	return s.getAgentFunc(ctx, projectID, agentID)
}

func (s *stubAgentService) ListAgents(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.Agent, error) {
	return s.listAgentsFunc(ctx, projectID, limit, cursor)
}

func (s *stubAgentService) UpdateAgent(ctx context.Context, req agentsvc.UpdateAgentRequest) (*domain.Agent, error) {
	return s.updateAgentFunc(ctx, req)
}

func (s *stubAgentService) DeleteAgent(ctx context.Context, projectID, agentID string) error {
	return s.deleteAgentFunc(ctx, projectID, agentID)
}

func (s *stubAgentService) DeployAgent(ctx context.Context, projectID, agentID, actor string) (*domain.AgentDeployment, error) {
	return s.deployAgentFunc(ctx, projectID, agentID, actor)
}

func (s *stubAgentService) RunAgent(ctx context.Context, req agentsvc.RunAgentRequest) (*domain.JobRun, error) {
	return s.runAgentFunc(ctx, req)
}

func (s *stubAgentService) ListAgentRuns(ctx context.Context, projectID, agentID string, limit, offset int) ([]domain.JobRun, error) {
	return s.listAgentRunsFunc(ctx, projectID, agentID, limit, offset)
}

func (s *stubAgentService) PrepareDirectRun(_ context.Context, _ agentsvc.RunAgentRequest) (*agentsvc.DirectRunResult, error) {
	return nil, nil
}

func newAgentTestServer(t *testing.T, svc agentsvc.Service) *Server {
	t.Helper()

	srv := NewServer(ServerDeps{
		Config: &config.Config{
			InternalSecret:      "test-secret",
			MaxBulkTriggerItems: 500,
			JWTSigningKey:       "01234567890123456789012345678901",
		},
		Store:        &APIStoreMock{},
		AgentService: svc,
		Queue:        &mockQueue{},
		Edition:      domain.EditionCloud,
	})
	t.Cleanup(srv.Close)
	return srv
}

func TestHandleCreateAgentSuccess(t *testing.T) {
	t.Parallel()

	var captured agentsvc.CreateAgentRequest
	srv := newAgentTestServer(t, &stubAgentService{
		createAgentFunc: func(_ context.Context, req agentsvc.CreateAgentRequest) (*domain.Agent, error) {
			captured = req
			return &domain.Agent{
				ID:        "agent-1",
				ProjectID: req.ProjectID,
				Name:      req.Name,
				Slug:      req.Slug,
				Model:     req.Model,
			}, nil
		},
		getAgentFunc: func(context.Context, string, string) (*domain.Agent, error) { return nil, nil },
		listAgentsFunc: func(context.Context, string, int, *time.Time) ([]domain.Agent, error) {
			return nil, nil
		},
		updateAgentFunc: func(context.Context, agentsvc.UpdateAgentRequest) (*domain.Agent, error) { return nil, nil },
		deleteAgentFunc: func(context.Context, string, string) error { return nil },
		deployAgentFunc: func(context.Context, string, string, string) (*domain.AgentDeployment, error) { return nil, nil },
		runAgentFunc:    func(context.Context, agentsvc.RunAgentRequest) (*domain.JobRun, error) { return nil, nil },
		listAgentRunsFunc: func(context.Context, string, string, int, int) ([]domain.JobRun, error) {
			return nil, nil
		},
	})

	body := `{"project_id":"proj-1","name":"Support Agent","slug":"support-agent","model":"gpt-5.4","config":{"temperature":0.2}}`
	req := authedProjectRequest(http.MethodPost, "/v1/agents", body, "proj-1")
	req.Header.Set("X-Actor-Id", "user-1")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if captured.ProjectID != "proj-1" {
		t.Fatalf("captured.ProjectID = %q, want proj-1", captured.ProjectID)
	}
	if captured.Actor != "user-1" {
		t.Fatalf("captured.Actor = %q, want user-1", captured.Actor)
	}

	var agent domain.Agent
	if err := json.Unmarshal(w.Body.Bytes(), &agent); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if agent.ID != "agent-1" {
		t.Fatalf("agent.ID = %q, want agent-1", agent.ID)
	}
}

func TestHandleCreateAgentRejectsNonObjectConfig(t *testing.T) {
	t.Parallel()

	srv := newAgentTestServer(t, &stubAgentService{
		createAgentFunc: func(context.Context, agentsvc.CreateAgentRequest) (*domain.Agent, error) {
			t.Fatal("createAgent should not be called")
			return nil, nil
		},
		getAgentFunc:   func(context.Context, string, string) (*domain.Agent, error) { return nil, nil },
		listAgentsFunc: func(context.Context, string, int, *time.Time) ([]domain.Agent, error) { return nil, nil },
		updateAgentFunc: func(context.Context, agentsvc.UpdateAgentRequest) (*domain.Agent, error) {
			return nil, nil
		},
		deleteAgentFunc: func(context.Context, string, string) error { return nil },
		deployAgentFunc: func(context.Context, string, string, string) (*domain.AgentDeployment, error) {
			return nil, nil
		},
		runAgentFunc: func(context.Context, agentsvc.RunAgentRequest) (*domain.JobRun, error) {
			return nil, nil
		},
		listAgentRunsFunc: func(context.Context, string, string, int, int) ([]domain.JobRun, error) {
			return nil, nil
		},
	})

	req := authedProjectRequest(
		http.MethodPost,
		"/v1/agents",
		`{"project_id":"proj-1","name":"Support Agent","slug":"support-agent","model":"gpt-5.4","config":"not-an-object"}`,
		"proj-1",
	)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleUpdateAgentRejectsNonObjectConfig(t *testing.T) {
	t.Parallel()

	srv := newAgentTestServer(t, &stubAgentService{
		createAgentFunc: func(context.Context, agentsvc.CreateAgentRequest) (*domain.Agent, error) { return nil, nil },
		getAgentFunc: func(context.Context, string, string) (*domain.Agent, error) {
			return &domain.Agent{
				ID:        "agent-1",
				ProjectID: "proj-1",
				Name:      "Support Agent",
				Slug:      "support-agent",
				Model:     "gpt-5.4",
				Config:    json.RawMessage(`{"temperature":0.2}`),
			}, nil
		},
		listAgentsFunc: func(context.Context, string, int, *time.Time) ([]domain.Agent, error) { return nil, nil },
		updateAgentFunc: func(context.Context, agentsvc.UpdateAgentRequest) (*domain.Agent, error) {
			t.Fatal("updateAgent should not be called")
			return nil, nil
		},
		deleteAgentFunc: func(context.Context, string, string) error { return nil },
		deployAgentFunc: func(context.Context, string, string, string) (*domain.AgentDeployment, error) {
			return nil, nil
		},
		runAgentFunc: func(context.Context, agentsvc.RunAgentRequest) (*domain.JobRun, error) {
			return nil, nil
		},
		listAgentRunsFunc: func(context.Context, string, string, int, int) ([]domain.JobRun, error) {
			return nil, nil
		},
	})

	req := authedProjectRequest(
		http.MethodPatch,
		"/v1/agents/agent-1",
		`{"config":["not-an-object"]}`,
		"proj-1",
	)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleRunAgentNotDeployed(t *testing.T) {
	t.Parallel()

	srv := newAgentTestServer(t, &stubAgentService{
		createAgentFunc: func(context.Context, agentsvc.CreateAgentRequest) (*domain.Agent, error) { return nil, nil },
		getAgentFunc:    func(context.Context, string, string) (*domain.Agent, error) { return nil, nil },
		listAgentsFunc: func(context.Context, string, int, *time.Time) ([]domain.Agent, error) {
			return nil, nil
		},
		updateAgentFunc: func(context.Context, agentsvc.UpdateAgentRequest) (*domain.Agent, error) { return nil, nil },
		deleteAgentFunc: func(context.Context, string, string) error { return nil },
		deployAgentFunc: func(context.Context, string, string, string) (*domain.AgentDeployment, error) { return nil, nil },
		runAgentFunc: func(context.Context, agentsvc.RunAgentRequest) (*domain.JobRun, error) {
			return nil, agentsvc.ErrNotDeployed
		},
		listAgentRunsFunc: func(context.Context, string, string, int, int) ([]domain.JobRun, error) {
			return nil, nil
		},
	})

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/agents/agent-1/run", `{"payload":{"prompt":"hello"}}`, "proj-1"))

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleListAgentRunsInvalidOffset(t *testing.T) {
	t.Parallel()

	srv := newAgentTestServer(t, &stubAgentService{
		createAgentFunc: func(context.Context, agentsvc.CreateAgentRequest) (*domain.Agent, error) { return nil, nil },
		getAgentFunc:    func(context.Context, string, string) (*domain.Agent, error) { return nil, nil },
		listAgentsFunc: func(context.Context, string, int, *time.Time) ([]domain.Agent, error) {
			return nil, nil
		},
		updateAgentFunc: func(context.Context, agentsvc.UpdateAgentRequest) (*domain.Agent, error) { return nil, nil },
		deleteAgentFunc: func(context.Context, string, string) error { return nil },
		deployAgentFunc: func(context.Context, string, string, string) (*domain.AgentDeployment, error) { return nil, nil },
		runAgentFunc:    func(context.Context, agentsvc.RunAgentRequest) (*domain.JobRun, error) { return nil, nil },
		listAgentRunsFunc: func(context.Context, string, string, int, int) ([]domain.JobRun, error) {
			return nil, nil
		},
	})

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/agents/agent-1/runs?offset=-1", "", "proj-1"))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
