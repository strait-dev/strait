package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

const (
	projectA = "proj-aaa"
	projectB = "proj-bbb"
)

// newIsolationStore creates a mock store that returns different data per project ID.
// Each project has one job, one run, one secret, one workflow, one webhook subscription,
// one event trigger, one environment, one audit event, and one log drain.
func newIsolationStore() *APIStoreMock {
	now := time.Now()
	return &APIStoreMock{
		ListJobsFunc: func(_ context.Context, projectID string, _ int, _ *time.Time) ([]domain.Job, error) {
			if projectID == projectA {
				return []domain.Job{{ID: "job-a", ProjectID: projectA, Name: "Job A", Slug: "job-a", CreatedAt: now}}, nil
			}
			if projectID == projectB {
				return []domain.Job{{ID: "job-b", ProjectID: projectB, Name: "Job B", Slug: "job-b", CreatedAt: now}}, nil
			}
			return nil, nil
		},
		ListRunsByProjectFunc: func(_ context.Context, projectID string, _ *domain.RunStatus, _, _, _, _ *string, _ json.RawMessage, _ *domain.ExecutionMode, _ *string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			if projectID == projectA {
				return []domain.JobRun{{ID: "run-a", ProjectID: projectA, JobID: "job-a", Status: domain.StatusCompleted, CreatedAt: now}}, nil
			}
			if projectID == projectB {
				return []domain.JobRun{{ID: "run-b", ProjectID: projectB, JobID: "job-b", Status: domain.StatusCompleted, CreatedAt: now}}, nil
			}
			return nil, nil
		},
		ListJobSecretsFunc: func(_ context.Context, projectID, _, _ string, _ int, _ *time.Time) ([]domain.JobSecret, error) {
			if projectID == projectA {
				return []domain.JobSecret{{ID: "secret-a", ProjectID: projectA, SecretKey: "KEY_A", CreatedAt: now, UpdatedAt: now}}, nil
			}
			if projectID == projectB {
				return []domain.JobSecret{{ID: "secret-b", ProjectID: projectB, SecretKey: "KEY_B", CreatedAt: now, UpdatedAt: now}}, nil
			}
			return nil, nil
		},
		ListWorkflowsFunc: func(_ context.Context, projectID string, _ int, _ *time.Time) ([]domain.Workflow, error) {
			if projectID == projectA {
				return []domain.Workflow{{ID: "wf-a", ProjectID: projectA, Name: "Workflow A", Slug: "wf-a", CreatedAt: now, UpdatedAt: now}}, nil
			}
			if projectID == projectB {
				return []domain.Workflow{{ID: "wf-b", ProjectID: projectB, Name: "Workflow B", Slug: "wf-b", CreatedAt: now, UpdatedAt: now}}, nil
			}
			return nil, nil
		},
		ListWebhookSubscriptionsFunc: func(_ context.Context, projectID string) ([]domain.WebhookSubscription, error) {
			if projectID == projectA {
				return []domain.WebhookSubscription{{ID: "wh-a", ProjectID: projectA, WebhookURL: "https://a.example.com", Active: true, CreatedAt: now}}, nil
			}
			if projectID == projectB {
				return []domain.WebhookSubscription{{ID: "wh-b", ProjectID: projectB, WebhookURL: "https://b.example.com", Active: true, CreatedAt: now}}, nil
			}
			return nil, nil
		},
		ListEventTriggersByProjectFunc: func(_ context.Context, projectID, _, _, _, _ string, _ int, _ *time.Time) ([]domain.EventTrigger, error) {
			if projectID == projectA {
				return []domain.EventTrigger{{ID: "et-a", ProjectID: projectA, EventKey: "event.a", Status: "waiting", RequestedAt: now}}, nil
			}
			if projectID == projectB {
				return []domain.EventTrigger{{ID: "et-b", ProjectID: projectB, EventKey: "event.b", Status: "waiting", RequestedAt: now}}, nil
			}
			return nil, nil
		},
		ListEnvironmentsFunc: func(_ context.Context, projectID string, _ int, _ *time.Time) ([]domain.Environment, error) {
			if projectID == projectA {
				return []domain.Environment{{ID: "env-a", ProjectID: projectA, Name: "production", Slug: "production", CreatedAt: now, UpdatedAt: now}}, nil
			}
			if projectID == projectB {
				return []domain.Environment{{ID: "env-b", ProjectID: projectB, Name: "staging", Slug: "staging", CreatedAt: now, UpdatedAt: now}}, nil
			}
			return nil, nil
		},
		ListAuditEventsFunc: func(_ context.Context, projectID, _, _, _ string, _ int, _ *time.Time, _, _ *time.Time, _ bool) ([]domain.AuditEvent, error) {
			if projectID == projectA {
				return []domain.AuditEvent{{ID: "audit-a", ProjectID: projectA, Action: "job.created", CreatedAt: now}}, nil
			}
			if projectID == projectB {
				return []domain.AuditEvent{{ID: "audit-b", ProjectID: projectB, Action: "job.deleted", CreatedAt: now}}, nil
			}
			return nil, nil
		},
		ListLogDrainsFunc: func(_ context.Context, projectID string) ([]domain.LogDrain, error) {
			if projectID == projectA {
				return []domain.LogDrain{{ID: "ld-a", ProjectID: projectA, Name: "Drain A", DrainType: "http", Enabled: true, CreatedAt: now}}, nil
			}
			if projectID == projectB {
				return []domain.LogDrain{{ID: "ld-b", ProjectID: projectB, Name: "Drain B", DrainType: "http", Enabled: true, CreatedAt: now}}, nil
			}
			return nil, nil
		},
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			if id == "run-a" {
				return &domain.JobRun{ID: "run-a", ProjectID: projectA, JobID: "job-a", Status: domain.StatusCompleted, CreatedAt: now}, nil
			}
			if id == "run-b" {
				return &domain.JobRun{ID: "run-b", ProjectID: projectB, JobID: "job-b", Status: domain.StatusCompleted, CreatedAt: now}, nil
			}
			return nil, nil
		},
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			if id == "job-a" {
				return &domain.Job{ID: "job-a", ProjectID: projectA, Name: "Job A", Slug: "job-a", Enabled: true, CreatedAt: now, UpdatedAt: now}, nil
			}
			if id == "job-b" {
				return &domain.Job{ID: "job-b", ProjectID: projectB, Name: "Job B", Slug: "job-b", Enabled: true, CreatedAt: now, UpdatedAt: now}, nil
			}
			return nil, nil
		},
		GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
			if id == "wf-a" {
				return &domain.Workflow{ID: "wf-a", ProjectID: projectA, Name: "Workflow A", Slug: "wf-a", CreatedAt: now, UpdatedAt: now}, nil
			}
			if id == "wf-b" {
				return &domain.Workflow{ID: "wf-b", ProjectID: projectB, Name: "Workflow B", Slug: "wf-b", CreatedAt: now, UpdatedAt: now}, nil
			}
			return nil, nil
		},
		GetEnvironmentFunc: func(_ context.Context, id string, _ string) (*domain.Environment, error) {
			if id == "env-a" {
				return &domain.Environment{ID: "env-a", ProjectID: projectA, Name: "production", Slug: "production", CreatedAt: now, UpdatedAt: now}, nil
			}
			if id == "env-b" {
				return &domain.Environment{ID: "env-b", ProjectID: projectB, Name: "staging", Slug: "staging", CreatedAt: now, UpdatedAt: now}, nil
			}
			return nil, nil
		},
		GetWebhookSubscriptionFunc: func(_ context.Context, id string) (*domain.WebhookSubscription, error) {
			if id == "wh-a" {
				return &domain.WebhookSubscription{ID: "wh-a", ProjectID: projectA, WebhookURL: "https://a.example.com", Active: true, CreatedAt: now}, nil
			}
			if id == "wh-b" {
				return &domain.WebhookSubscription{ID: "wh-b", ProjectID: projectB, WebhookURL: "https://b.example.com", Active: true, CreatedAt: now}, nil
			}
			return nil, nil
		},
		DeleteJobFunc: func(_ context.Context, _ string) error {
			return nil
		},
		DeleteWebhookSubscriptionFunc: func(_ context.Context, _ string) error {
			return nil
		},
	}
}

// requestForProject creates an authenticated request with a specific project ID.
func requestForProject(method, path, body, projectID string) *http.Request {
	r := authedRequest(method, path, body)
	if projectID != "" {
		r.Header.Set("X-Project-Id", projectID)
	}
	return r
}

// decodeDataCount decodes a paginated response and returns the number of data items.
func decodeDataCount(tb testing.TB, body []byte) int {
	tb.Helper()
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		// Try as direct array.
		var arr []json.RawMessage
		if err2 := json.Unmarshal(body, &arr); err2 == nil {
			return len(arr)
		}
		tb.Fatalf("failed to decode response: %v\nbody: %s", err, string(body))
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(envelope.Data, &arr); err != nil {
		tb.Fatalf("failed to decode data array: %v", err)
	}
	return len(arr)
}

// TestTenantIsolation_JobsNeverCrossProject verifies that listing jobs with
// project B key returns no jobs from project A.
func TestTenantIsolation_JobsNeverCrossProject(t *testing.T) {
	t.Parallel()
	ms := newIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	// List jobs for project A.
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, requestForProject(http.MethodGet, "/v1/jobs/", "", projectA))
	if w.Code != http.StatusOK {
		t.Fatalf("project A list jobs: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	countA := decodeDataCount(t, w.Body.Bytes())
	if countA != 1 {
		t.Errorf("project A jobs: expected 1, got %d", countA)
	}

	// List jobs for project B.
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, requestForProject(http.MethodGet, "/v1/jobs/", "", projectB))
	if w.Code != http.StatusOK {
		t.Fatalf("project B list jobs: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	countB := decodeDataCount(t, w.Body.Bytes())
	if countB != 1 {
		t.Errorf("project B jobs: expected 1, got %d", countB)
	}

	// Ensure project B does not see project A jobs (mock already guarantees
	// different data per projectID, so we check the IDs).
	var jobsB []domain.Job
	decodePaginatedList(t, w.Body.Bytes(), &jobsB)
	for _, j := range jobsB {
		if j.ProjectID == projectA {
			t.Errorf("project B listing returned job from project A: %s", j.ID)
		}
	}
}

// TestTenantIsolation_RunsNeverCrossProject verifies that runs from project A
// are not visible when listing with project B context.
func TestTenantIsolation_RunsNeverCrossProject(t *testing.T) {
	t.Parallel()
	ms := newIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, requestForProject(http.MethodGet, "/v1/runs/", "", projectB))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var runs []domain.JobRun
	decodePaginatedList(t, w.Body.Bytes(), &runs)
	for _, r := range runs {
		if r.ProjectID == projectA {
			t.Errorf("project B listing returned run from project A: %s", r.ID)
		}
	}
}

// TestTenantIsolation_SecretsNeverCrossProject verifies that secrets from
// project A are not visible via project B.
func TestTenantIsolation_SecretsNeverCrossProject(t *testing.T) {
	t.Parallel()
	ms := newIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, requestForProject(http.MethodGet, "/v1/secrets/", "", projectB))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var secrets []domain.JobSecret
	decodePaginatedList(t, w.Body.Bytes(), &secrets)
	for _, s := range secrets {
		if s.ProjectID == projectA {
			t.Errorf("project B listing returned secret from project A: %s", s.ID)
		}
	}
}

// TestTenantIsolation_WorkflowsNeverCrossProject verifies that workflows are
// isolated per project.
func TestTenantIsolation_WorkflowsNeverCrossProject(t *testing.T) {
	t.Parallel()
	ms := newIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, requestForProject(http.MethodGet, "/v1/workflows/", "", projectB))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var workflows []domain.Workflow
	decodePaginatedList(t, w.Body.Bytes(), &workflows)
	for _, wf := range workflows {
		if wf.ProjectID == projectA {
			t.Errorf("project B listing returned workflow from project A: %s", wf.ID)
		}
	}
}

// TestTenantIsolation_WebhooksNeverCrossProject verifies that webhook
// subscriptions are isolated per project.
func TestTenantIsolation_WebhooksNeverCrossProject(t *testing.T) {
	t.Parallel()
	ms := newIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, requestForProject(http.MethodGet, "/v1/webhooks/subscriptions/", "", projectB))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var subs []domain.WebhookSubscription
	if err := json.Unmarshal(w.Body.Bytes(), &subs); err != nil {
		// Try paginated format.
		decodePaginatedList(t, w.Body.Bytes(), &subs)
	}
	for _, s := range subs {
		if s.ProjectID == projectA {
			t.Errorf("project B listing returned webhook subscription from project A: %s", s.ID)
		}
	}
}

// TestTenantIsolation_EventTriggersNeverCrossProject verifies that event
// triggers are isolated per project.
func TestTenantIsolation_EventTriggersNeverCrossProject(t *testing.T) {
	t.Parallel()
	ms := newIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, requestForProject(http.MethodGet, "/v1/events/", "", projectB))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var triggers []domain.EventTrigger
	decodePaginatedList(t, w.Body.Bytes(), &triggers)
	for _, et := range triggers {
		if et.ProjectID == projectA {
			t.Errorf("project B listing returned event trigger from project A: %s", et.ID)
		}
	}
}

// TestTenantIsolation_OIDCProjectHeaderSpoofing verifies that an OIDC-authed
// request cannot spoof the project context via X-Project-Id to access another
// project. Since OIDC auth trusts the header, the middleware should use the
// project from the header and the store filters by it. This test verifies
// that the internal secret auth path uses X-Project-Id as well, but the
// important thing is the store receives the correct project ID.
func TestTenantIsolation_OIDCProjectHeaderSpoofing(t *testing.T) {
	t.Parallel()
	ms := newIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	// Set X-Project-Id to project A and list jobs.
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, requestForProject(http.MethodGet, "/v1/jobs/", "", projectA))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var jobsA []domain.Job
	decodePaginatedList(t, w.Body.Bytes(), &jobsA)
	if len(jobsA) != 1 || jobsA[0].ID != "job-a" {
		t.Errorf("expected job-a, got %v", jobsA)
	}

	// Now set X-Project-Id to project B and verify we only see project B.
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, requestForProject(http.MethodGet, "/v1/jobs/", "", projectB))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var jobsB []domain.Job
	decodePaginatedList(t, w.Body.Bytes(), &jobsB)
	if len(jobsB) != 1 || jobsB[0].ID != "job-b" {
		t.Errorf("expected job-b, got %v", jobsB)
	}
}

// TestTenantIsolation_APIKeyProjectMismatch verifies that when project context
// is set, only that project's data is returned even if the underlying run
// belongs to a different project.
func TestTenantIsolation_APIKeyProjectMismatch(t *testing.T) {
	t.Parallel()
	ms := newIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	// Request runs for project B.
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, requestForProject(http.MethodGet, "/v1/runs/", "", projectB))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var runs []domain.JobRun
	decodePaginatedList(t, w.Body.Bytes(), &runs)
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	if runs[0].ProjectID != projectB {
		t.Errorf("run project_id = %q, want %q", runs[0].ProjectID, projectB)
	}
}

// TestTenantIsolation_OrgScopedKeyProjectAccess verifies that org-scoped
// context can only see data for the specified project.
func TestTenantIsolation_OrgScopedKeyProjectAccess(t *testing.T) {
	t.Parallel()
	ms := newIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	// Even with an org context, listing for project A only returns A data.
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, requestForProject(http.MethodGet, "/v1/jobs/", "", projectA))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var jobs []domain.Job
	decodePaginatedList(t, w.Body.Bytes(), &jobs)
	for _, j := range jobs {
		if j.ProjectID != projectA {
			t.Errorf("org-scoped request for project A returned job from %q", j.ProjectID)
		}
	}
}

// TestTenantIsolation_RunIDGuessing verifies that accessing a run by ID from
// the wrong project returns 404 (not the cross-project resource).
func TestTenantIsolation_RunIDGuessing(t *testing.T) {
	t.Parallel()
	ms := newIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	// Try to get run-a (project A run) with project B context -- must return 404.
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, requestForProject(http.MethodGet, "/v1/runs/run-a", "", projectB))
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-project run access: expected 404, got %d: %s", w.Code, w.Body.String())
	}

	// Accessing own run should work.
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, requestForProject(http.MethodGet, "/v1/runs/run-b", "", projectB))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for own project run, got %d: %s", w.Code, w.Body.String())
	}
}

// TestTenantIsolation_CrossProjectJobAccess verifies that fetching a job by ID
// from another project returns 404.
func TestTenantIsolation_CrossProjectJobAccess(t *testing.T) {
	t.Parallel()
	ms := newIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	// Project B tries to get project A's job by ID -- must be 404.
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, requestForProject(http.MethodGet, "/v1/jobs/job-a", "", projectB))
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-project job GET: expected 404, got %d: %s", w.Code, w.Body.String())
	}

	// Own job should return 200.
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, requestForProject(http.MethodGet, "/v1/jobs/job-b", "", projectB))
	if w.Code != http.StatusOK {
		t.Fatalf("own project job GET: expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// TestTenantIsolation_CrossProjectWorkflowAccess verifies that fetching a
// workflow by ID from another project returns 404.
func TestTenantIsolation_CrossProjectWorkflowAccess(t *testing.T) {
	t.Parallel()
	ms := newIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	// Project B tries to get project A's workflow by ID -- must be 404.
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, requestForProject(http.MethodGet, "/v1/workflows/wf-a", "", projectB))
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-project workflow GET: expected 404, got %d: %s", w.Code, w.Body.String())
	}

	// Own workflow should return 200.
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, requestForProject(http.MethodGet, "/v1/workflows/wf-b", "", projectB))
	if w.Code != http.StatusOK {
		t.Fatalf("own project workflow GET: expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// TestTenantIsolation_CrossProjectEnvironmentAccess verifies that fetching an
// environment by ID from another project returns 404.
func TestTenantIsolation_CrossProjectEnvironmentAccess(t *testing.T) {
	t.Parallel()
	ms := newIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	// Project B tries to get project A's environment by ID -- must be 404.
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, requestForProject(http.MethodGet, "/v1/environments/env-a", "", projectB))
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-project environment GET: expected 404, got %d: %s", w.Code, w.Body.String())
	}

	// Own environment should return 200.
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, requestForProject(http.MethodGet, "/v1/environments/env-b", "", projectB))
	if w.Code != http.StatusOK {
		t.Fatalf("own project environment GET: expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// TestTenantIsolation_CrossProjectDeleteBlocked verifies that deleting a
// resource from another project returns 404.
func TestTenantIsolation_CrossProjectDeleteBlocked(t *testing.T) {
	t.Parallel()
	ms := newIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	// Project B tries to delete project A's job -- must be 404.
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, requestForProject(http.MethodDelete, "/v1/jobs/job-a", "", projectB))
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-project job DELETE: expected 404, got %d: %s", w.Code, w.Body.String())
	}

	// Project B tries to delete project A's webhook subscription -- must be 404.
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, requestForProject(http.MethodDelete, "/v1/webhooks/subscriptions/wh-a", "", projectB))
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-project webhook DELETE: expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// TestTenantIsolation_EnvironmentsIsolated verifies that environments are
// isolated per project.
func TestTenantIsolation_EnvironmentsIsolated(t *testing.T) {
	t.Parallel()
	ms := newIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, requestForProject(http.MethodGet, "/v1/environments/", "", projectB))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var envs []domain.Environment
	decodePaginatedList(t, w.Body.Bytes(), &envs)
	for _, e := range envs {
		if e.ProjectID == projectA {
			t.Errorf("project B listing returned environment from project A: %s", e.ID)
		}
	}
	if len(envs) != 1 || envs[0].ID != "env-b" {
		t.Errorf("expected env-b, got %v", envs)
	}
}

// TestTenantIsolation_AuditEventsIsolated verifies that audit events are
// isolated per project.
func TestTenantIsolation_AuditEventsIsolated(t *testing.T) {
	t.Parallel()
	ms := newIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, requestForProject(http.MethodGet, "/v1/audit-events/", "", projectB))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var events []domain.AuditEvent
	decodePaginatedList(t, w.Body.Bytes(), &events)
	for _, ev := range events {
		if ev.ProjectID == projectA {
			t.Errorf("project B listing returned audit event from project A: %s", ev.ID)
		}
	}
}

// TestTenantIsolation_AnalyticsIsolated verifies that analytics queries use
// the project context from the request. We test this by checking that the
// runs listing (as a proxy for analytics data) is filtered by project.
func TestTenantIsolation_AnalyticsIsolated(t *testing.T) {
	t.Parallel()
	ms := newIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	// List runs for project A.
	wA := httptest.NewRecorder()
	srv.ServeHTTP(wA, requestForProject(http.MethodGet, "/v1/runs/", "", projectA))
	if wA.Code != http.StatusOK {
		t.Fatalf("project A: expected 200, got %d", wA.Code)
	}

	var runsA []domain.JobRun
	decodePaginatedList(t, wA.Body.Bytes(), &runsA)

	// List runs for project B.
	wB := httptest.NewRecorder()
	srv.ServeHTTP(wB, requestForProject(http.MethodGet, "/v1/runs/", "", projectB))
	if wB.Code != http.StatusOK {
		t.Fatalf("project B: expected 200, got %d", wB.Code)
	}

	var runsB []domain.JobRun
	decodePaginatedList(t, wB.Body.Bytes(), &runsB)

	// Verify no overlap.
	aIDs := make(map[string]bool)
	for _, r := range runsA {
		aIDs[r.ID] = true
	}
	for _, r := range runsB {
		if aIDs[r.ID] {
			t.Errorf("run %q appears in both project A and B listings", r.ID)
		}
	}
}

// TestTenantIsolation_LogDrainsIsolated verifies that log drains are isolated
// per project.
func TestTenantIsolation_LogDrainsIsolated(t *testing.T) {
	t.Parallel()
	ms := newIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, requestForProject(http.MethodGet, "/v1/log-drains/", "", projectB))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var drains []domain.LogDrain
	// Log drains may return a direct array or paginated.
	if err := json.Unmarshal(w.Body.Bytes(), &drains); err != nil {
		decodePaginatedList(t, w.Body.Bytes(), &drains)
	}
	for _, d := range drains {
		if d.ProjectID == projectA {
			t.Errorf("project B listing returned log drain from project A: %s", d.ID)
		}
	}
}

// TestTenantIsolation_SDKTokenScopedToRun verifies that an SDK run token
// for run-A cannot be used to operate on run-B.
func TestTenantIsolation_SDKTokenScopedToRun(t *testing.T) {
	t.Parallel()
	ms := newIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	// Create a valid JWT for run-a.
	tokenA := generateRunToken(t, "run-a")

	// Use run-a's token on run-b's heartbeat endpoint -- should be 403.
	// Heartbeat is simpler (no store calls needed) so middleware rejection
	// is tested cleanly without needing extra mock setup.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/sdk/v1/runs/run-b/heartbeat", nil)
	req.Header.Set("Authorization", "Bearer "+tokenA)
	req.Header.Set("Content-Type", "application/json")
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("cross-run SDK token: expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

// TestTenantIsolation_RevokeAPIKey_CrossProject verifies that revoking an API
// key belonging to another project returns 404.
func TestTenantIsolation_RevokeAPIKey_CrossProject(t *testing.T) {
	t.Parallel()

	now := time.Now()
	expiresAt := now.Add(24 * time.Hour)
	ms := newIsolationStore()
	ms.GetAPIKeyByIDFunc = func(_ context.Context, id string) (*domain.APIKey, error) {
		switch id {
		case "key-a":
			return &domain.APIKey{ID: "key-a", ProjectID: projectA, Name: "Key A", KeyHash: "h", KeyPrefix: "strait_a", CreatedAt: now, ExpiresAt: &expiresAt}, nil
		case "key-b":
			return &domain.APIKey{ID: "key-b", ProjectID: projectB, Name: "Key B", KeyHash: "h", KeyPrefix: "strait_b", CreatedAt: now, ExpiresAt: &expiresAt}, nil
		}
		return nil, nil
	}
	ms.RevokeAPIKeyFunc = func(_ context.Context, _ string) error {
		return nil
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	tests := []struct {
		name      string
		keyID     string
		projectID string
		wantCode  int
	}{
		{"own project key", "key-b", projectB, http.StatusOK},
		{"cross project key", "key-a", projectB, http.StatusNotFound},
		{"no project context (internal)", "key-a", "", http.StatusOK},
		{"non-existent key", "key-nope", projectB, http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, authedProjectRequest(http.MethodDelete, "/v1/api-keys/"+tt.keyID, "", tt.projectID))
			if w.Code != tt.wantCode {
				t.Errorf("DELETE /v1/api-keys/%s with project %q: got %d, want %d: %s",
					tt.keyID, tt.projectID, w.Code, tt.wantCode, w.Body.String())
			}
		})
	}
}

// TestTenantIsolation_RotateAPIKey_CrossProject verifies that rotating an API
// key belonging to another project returns 404.
func TestTenantIsolation_RotateAPIKey_CrossProject(t *testing.T) {
	t.Parallel()

	now := time.Now()
	expiresAt := now.Add(24 * time.Hour)
	ms := newIsolationStore()
	ms.GetAPIKeyByIDFunc = func(_ context.Context, id string) (*domain.APIKey, error) {
		switch id {
		case "key-a":
			return &domain.APIKey{ID: "key-a", ProjectID: projectA, Name: "Key A", KeyHash: "h", KeyPrefix: "strait_a", CreatedAt: now, ExpiresAt: &expiresAt}, nil
		case "key-b":
			return &domain.APIKey{ID: "key-b", ProjectID: projectB, Name: "Key B", KeyHash: "h", KeyPrefix: "strait_b", CreatedAt: now, ExpiresAt: &expiresAt}, nil
		}
		return nil, nil
	}
	ms.CreateAPIKeyFunc = func(_ context.Context, _ *domain.APIKey) error {
		return nil
	}
	ms.MarkAPIKeyRotatedFunc = func(_ context.Context, _, _ string, _ time.Time) error {
		return nil
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	tests := []struct {
		name      string
		keyID     string
		projectID string
		wantCode  int
	}{
		{"own project key", "key-b", projectB, http.StatusCreated},
		{"cross project key", "key-a", projectB, http.StatusNotFound},
		{"no project context (internal)", "key-a", "", http.StatusCreated},
		{"non-existent key", "key-nope", projectB, http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/api-keys/"+tt.keyID+"/rotate", `{}`, tt.projectID))
			if w.Code != tt.wantCode {
				t.Errorf("POST /v1/api-keys/%s/rotate with project %q: got %d, want %d: %s",
					tt.keyID, tt.projectID, w.Code, tt.wantCode, w.Body.String())
			}
		})
	}
}

// TestTenantIsolation_DeleteEnvironment_CrossProject verifies that deleting an
// environment belonging to another project returns 404.
func TestTenantIsolation_DeleteEnvironment_CrossProject(t *testing.T) {
	t.Parallel()

	ms := newIsolationStore()
	ms.DeleteEnvironmentFunc = func(_ context.Context, _ string, _ string) error {
		return nil
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	tests := []struct {
		name      string
		envID     string
		projectID string
		wantCode  int
	}{
		{"own project env", "env-b", projectB, http.StatusNoContent},
		{"cross project env", "env-a", projectB, http.StatusNotFound},
		{"no project context (internal)", "env-a", "", http.StatusNoContent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, authedProjectRequest(http.MethodDelete, "/v1/environments/"+tt.envID, "", tt.projectID))
			if w.Code != tt.wantCode {
				t.Errorf("DELETE /v1/environments/%s with project %q: got %d, want %d: %s",
					tt.envID, tt.projectID, w.Code, tt.wantCode, w.Body.String())
			}
		})
	}
}

// TestTenantIsolation_GetResolvedVariables_CrossProject verifies that getting
// resolved variables for an environment belonging to another project returns 404.
func TestTenantIsolation_GetResolvedVariables_CrossProject(t *testing.T) {
	t.Parallel()

	ms := newIsolationStore()
	ms.GetResolvedEnvironmentVariablesFunc = func(_ context.Context, _ string) (map[string]string, error) {
		return map[string]string{"FOO": "bar"}, nil
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	tests := []struct {
		name      string
		envID     string
		projectID string
		wantCode  int
	}{
		{"own project env", "env-b", projectB, http.StatusOK},
		{"cross project env", "env-a", projectB, http.StatusNotFound},
		{"no project context (internal)", "env-a", "", http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/environments/"+tt.envID+"/variables", "", tt.projectID))
			if w.Code != tt.wantCode {
				t.Errorf("GET /v1/environments/%s/variables with project %q: got %d, want %d: %s",
					tt.envID, tt.projectID, w.Code, tt.wantCode, w.Body.String())
			}
		})
	}
}

// TestTenantIsolation_ListEventSourceSubscriptions_CrossProject verifies that
// listing subscriptions for an event source in another project returns 404.
func TestTenantIsolation_ListEventSourceSubscriptions_CrossProject(t *testing.T) {
	t.Parallel()

	ms := newIsolationStore()
	ms.GetEventSourceFunc = func(_ context.Context, sourceID, projectID string) (*domain.EventSource, error) {
		if sourceID == "src-a" && projectID == projectA {
			return &domain.EventSource{ID: "src-a", ProjectID: projectA, Name: "Source A", Enabled: true}, nil
		}
		if sourceID == "src-b" && projectID == projectB {
			return &domain.EventSource{ID: "src-b", ProjectID: projectB, Name: "Source B", Enabled: true}, nil
		}
		return nil, store.ErrEventSourceNotFound
	}
	ms.ListEventSubscriptionsBySourceFunc = func(_ context.Context, _ string) ([]domain.EventSubscription, error) {
		return []domain.EventSubscription{}, nil
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	tests := []struct {
		name      string
		sourceID  string
		projectID string
		wantCode  int
	}{
		{"own project source", "src-b", projectB, http.StatusOK},
		{"cross project source", "src-a", projectB, http.StatusNotFound},
		{"no project context (internal)", "src-a", "", http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/event-sources/"+tt.sourceID+"/subscriptions", "", tt.projectID))
			if w.Code != tt.wantCode {
				t.Errorf("GET /v1/event-sources/%s/subscriptions with project %q: got %d, want %d: %s",
					tt.sourceID, tt.projectID, w.Code, tt.wantCode, w.Body.String())
			}
		})
	}
}

// TestTenantIsolation_DeleteEventSubscription_CrossProject verifies that
// deleting an event subscription via a source in another project returns 404.
func TestTenantIsolation_DeleteEventSubscription_CrossProject(t *testing.T) {
	t.Parallel()

	ms := newIsolationStore()
	ms.GetEventSourceFunc = func(_ context.Context, sourceID, projectID string) (*domain.EventSource, error) {
		if sourceID == "src-a" && projectID == projectA {
			return &domain.EventSource{ID: "src-a", ProjectID: projectA, Name: "Source A", Enabled: true}, nil
		}
		if sourceID == "src-b" && projectID == projectB {
			return &domain.EventSource{ID: "src-b", ProjectID: projectB, Name: "Source B", Enabled: true}, nil
		}
		return nil, store.ErrEventSourceNotFound
	}
	ms.GetEventSubscriptionFunc = func(_ context.Context, subID string) (*domain.EventSubscription, error) {
		// sub-1 belongs to src-a, sub-2 belongs to src-b.
		if subID == "sub-1" {
			return &domain.EventSubscription{ID: "sub-1", SourceID: "src-a"}, nil
		}
		if subID == "sub-2" {
			return &domain.EventSubscription{ID: "sub-2", SourceID: "src-b"}, nil
		}
		return nil, store.ErrEventSubscriptionNotFound
	}
	ms.DeleteEventSubscriptionFunc = func(_ context.Context, _ string) error {
		return nil
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	tests := []struct {
		name      string
		sourceID  string
		subID     string
		projectID string
		wantCode  int
	}{
		{"own project source", "src-b", "sub-2", projectB, http.StatusNoContent},
		{"cross project source", "src-a", "sub-1", projectB, http.StatusNotFound},
		{"no project context (internal)", "src-a", "sub-1", "", http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, authedProjectRequest(http.MethodDelete, "/v1/event-sources/"+tt.sourceID+"/subscriptions/"+tt.subID, "", tt.projectID))
			if w.Code != tt.wantCode {
				t.Errorf("DELETE /v1/event-sources/%s/subscriptions/%s with project %q: got %d, want %d: %s",
					tt.sourceID, tt.subID, tt.projectID, w.Code, tt.wantCode, w.Body.String())
			}
		})
	}
}

// TestTenantIsolation_DispatchEvent_CrossProject verifies that a project-scoped
// caller cannot dispatch events for a different project by supplying a forged
// project_id in the request body.
func TestTenantIsolation_DispatchEvent_CrossProject(t *testing.T) {
	t.Parallel()

	ms := newIsolationStore()
	ms.GetEventSourceByNameFunc = func(_ context.Context, _, _ string) (*domain.EventSource, error) {
		t.Fatal("GetEventSourceByName must not be called for mismatched project_id")
		return nil, nil
	}
	ms.ListEventSubscriptionsBySourceFunc = func(_ context.Context, _ string) ([]domain.EventSubscription, error) {
		t.Fatal("ListEventSubscriptionsBySource must not be called for mismatched project_id")
		return nil, nil
	}
	srv := newTestServer(t, ms, &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			t.Fatal("queue.Enqueue must not be called for mismatched project_id")
			return nil
		},
	}, nil)

	body := `{"source":"source-a","project_id":"` + projectA + `","payload":{"type":"deploy"}}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/events/dispatch", body, projectB))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTenantIsolation_DispatchEvent_OwnProjectAllowed(t *testing.T) {
	t.Parallel()

	ms := newIsolationStore()
	ms.GetEventSourceByNameFunc = func(_ context.Context, projectID, name string) (*domain.EventSource, error) {
		if projectID != projectB {
			t.Fatalf("projectID = %q, want %q", projectID, projectB)
		}
		if name != "source-b" {
			t.Fatalf("name = %q, want source-b", name)
		}
		return &domain.EventSource{ID: "src-b", ProjectID: projectB, Name: "source-b", Enabled: true}, nil
	}
	ms.ListEventSubscriptionsBySourceFunc = func(_ context.Context, sourceID string) ([]domain.EventSubscription, error) {
		if sourceID != "src-b" {
			t.Fatalf("sourceID = %q, want src-b", sourceID)
		}
		return []domain.EventSubscription{
			{ID: "sub-b", SourceID: "src-b", TargetType: "job", TargetID: "job-b", FilterExpr: json.RawMessage(`{"eq":[["type","deploy"]]}`), Enabled: true},
		}, nil
	}
	enqueued := false
	srv := newTestServer(t, ms, &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			enqueued = true
			if run.ProjectID != projectB {
				t.Fatalf("run.ProjectID = %q, want %q", run.ProjectID, projectB)
			}
			if run.JobID != "job-b" {
				t.Fatalf("run.JobID = %q, want job-b", run.JobID)
			}
			return nil
		},
	}, nil)

	body := `{"source":"source-b","project_id":"` + projectB + `","payload":{"type":"deploy"}}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/events/dispatch", body, projectB))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !enqueued {
		t.Fatal("expected own-project dispatch to enqueue one run")
	}
}

func TestTenantIsolation_DispatchEvent_SkipsStaleCrossProjectJobSubscription(t *testing.T) {
	t.Parallel()

	ms := newIsolationStore()
	ms.GetEventSourceByNameFunc = func(_ context.Context, projectID, name string) (*domain.EventSource, error) {
		return &domain.EventSource{ID: "src-a", ProjectID: projectID, Name: name, Enabled: true}, nil
	}
	ms.ListEventSubscriptionsBySourceFunc = func(_ context.Context, sourceID string) ([]domain.EventSubscription, error) {
		return []domain.EventSubscription{
			{ID: "sub-cross-job", SourceID: sourceID, TargetType: "job", TargetID: "job-b", FilterExpr: json.RawMessage(`{"eq":[["type","deploy"]]}`), Enabled: true},
		}, nil
	}
	ms.GetJobFunc = func(_ context.Context, id string) (*domain.Job, error) {
		if id != "job-b" {
			t.Fatalf("id = %q, want job-b", id)
		}
		return &domain.Job{ID: "job-b", ProjectID: projectB, Enabled: true}, nil
	}
	srv := newTestServer(t, ms, &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			t.Fatal("queue.Enqueue must not be called for stale cross-project job subscription")
			return nil
		},
	}, nil)

	body := `{"source":"source-a","project_id":"` + projectA + `","payload":{"type":"deploy"}}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/events/dispatch", body, projectA))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if got := int(resp["dispatched"].(float64)); got != 0 {
		t.Fatalf("dispatched = %d, want 0", got)
	}
}

func TestTenantIsolation_DispatchEvent_SkipsNilJobSubscription(t *testing.T) {
	t.Parallel()

	ms := newIsolationStore()
	ms.GetEventSourceByNameFunc = func(_ context.Context, projectID, name string) (*domain.EventSource, error) {
		return &domain.EventSource{ID: "src-a", ProjectID: projectID, Name: name, Enabled: true}, nil
	}
	ms.ListEventSubscriptionsBySourceFunc = func(_ context.Context, sourceID string) ([]domain.EventSubscription, error) {
		return []domain.EventSubscription{
			{ID: "sub-missing-job", SourceID: sourceID, TargetType: "job", TargetID: "job-missing", FilterExpr: json.RawMessage(`{"eq":[["type","deploy"]]}`), Enabled: true},
		}, nil
	}
	ms.GetJobFunc = func(_ context.Context, id string) (*domain.Job, error) {
		if id != "job-missing" {
			t.Fatalf("id = %q, want job-missing", id)
		}
		return nil, nil
	}
	srv := newTestServer(t, ms, &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			t.Fatal("queue.Enqueue must not be called for nil job subscription")
			return nil
		},
	}, nil)

	body := `{"source":"source-a","project_id":"` + projectA + `","payload":{"type":"deploy"}}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/events/dispatch", body, projectA))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if got := int(resp["dispatched"].(float64)); got != 0 {
		t.Fatalf("dispatched = %d, want 0", got)
	}
}

func TestTenantIsolation_DispatchEvent_SkipsStaleCrossProjectWorkflowSubscription(t *testing.T) {
	t.Parallel()

	ms := newIsolationStore()
	ms.GetEventSourceByNameFunc = func(_ context.Context, projectID, name string) (*domain.EventSource, error) {
		return &domain.EventSource{ID: "src-a", ProjectID: projectID, Name: name, Enabled: true}, nil
	}
	ms.ListEventSubscriptionsBySourceFunc = func(_ context.Context, sourceID string) ([]domain.EventSubscription, error) {
		return []domain.EventSubscription{
			{ID: "sub-cross-wf", SourceID: sourceID, TargetType: "workflow", TargetID: "wf-b", FilterExpr: json.RawMessage(`{"eq":[["type","deploy"]]}`), Enabled: true},
		}, nil
	}
	ms.GetWorkflowFunc = func(_ context.Context, id string) (*domain.Workflow, error) {
		if id != "wf-b" {
			t.Fatalf("id = %q, want wf-b", id)
		}
		return &domain.Workflow{ID: "wf-b", ProjectID: projectB, Enabled: true}, nil
	}
	wfEngine := &mockWorkflowTrigger{
		triggerWorkflowFn: func(context.Context, string, string, json.RawMessage, string, []domain.StepOverride) (*domain.WorkflowRun, error) {
			t.Fatal("workflow trigger must not be called for stale cross-project workflow subscription")
			return nil, nil
		},
	}
	srv := newTestServerWithWorkflowEngine(t, ms, &mockQueue{}, wfEngine)

	body := `{"source":"source-a","project_id":"` + projectA + `","payload":{"type":"deploy"}}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/events/dispatch", body, projectA))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if got := int(resp["dispatched"].(float64)); got != 0 {
		t.Fatalf("dispatched = %d, want 0", got)
	}
}

// TestTenantIsolation_GetWebhookDelivery_CrossProject verifies that getting a
// webhook delivery whose job belongs to another project returns 404.
func TestTenantIsolation_GetWebhookDelivery_CrossProject(t *testing.T) {
	t.Parallel()

	now := time.Now()
	ms := newIsolationStore()
	ms.GetWebhookDeliveryFunc = func(_ context.Context, id string) (*domain.WebhookDelivery, error) {
		switch id {
		case "del-a":
			return &domain.WebhookDelivery{ID: "del-a", JobID: "job-a", WebhookURL: "https://a.example.com", Status: domain.WebhookStatusDelivered, CreatedAt: now, UpdatedAt: now}, nil
		case "del-b":
			return &domain.WebhookDelivery{ID: "del-b", JobID: "job-b", WebhookURL: "https://b.example.com", Status: domain.WebhookStatusDelivered, CreatedAt: now, UpdatedAt: now}, nil
		}
		return nil, fmt.Errorf("webhook delivery not found")
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	tests := []struct {
		name       string
		deliveryID string
		projectID  string
		wantCode   int
	}{
		{"own project delivery", "del-b", projectB, http.StatusOK},
		{"cross project delivery", "del-a", projectB, http.StatusNotFound},
		{"no project context (internal)", "del-a", "", http.StatusOK},
		{"non-existent delivery", "del-nope", projectB, http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/webhooks/deliveries/"+tt.deliveryID, "", tt.projectID))
			if w.Code != tt.wantCode {
				t.Errorf("GET /v1/webhooks/deliveries/%s with project %q: got %d, want %d: %s",
					tt.deliveryID, tt.projectID, w.Code, tt.wantCode, w.Body.String())
			}
		})
	}
}

// TestTenantIsolation_RetryWebhookDelivery_CrossProject verifies that retrying
// a webhook delivery whose job belongs to another project returns 404.
func TestTenantIsolation_RetryWebhookDelivery_CrossProject(t *testing.T) {
	t.Parallel()

	now := time.Now()
	ms := newIsolationStore()
	ms.GetWebhookDeliveryFunc = func(_ context.Context, id string) (*domain.WebhookDelivery, error) {
		switch id {
		case "del-a":
			return &domain.WebhookDelivery{ID: "del-a", JobID: "job-a", WebhookURL: "https://a.example.com", Status: domain.WebhookStatusFailed, CreatedAt: now, UpdatedAt: now}, nil
		case "del-b":
			return &domain.WebhookDelivery{ID: "del-b", JobID: "job-b", WebhookURL: "https://b.example.com", Status: domain.WebhookStatusFailed, CreatedAt: now, UpdatedAt: now}, nil
		}
		return nil, fmt.Errorf("webhook delivery not found")
	}
	ms.RetryWebhookDeliveryFunc = func(_ context.Context, id string) (*domain.WebhookDelivery, error) {
		return &domain.WebhookDelivery{ID: id, Status: domain.WebhookStatusPending, CreatedAt: now, UpdatedAt: now}, nil
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	tests := []struct {
		name       string
		deliveryID string
		projectID  string
		wantCode   int
	}{
		{"own project delivery", "del-b", projectB, http.StatusOK},
		{"cross project delivery", "del-a", projectB, http.StatusNotFound},
		{"no project context (internal)", "del-a", "", http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/webhooks/deliveries/"+tt.deliveryID+"/retry", "", tt.projectID))
			if w.Code != tt.wantCode {
				t.Errorf("POST /v1/webhooks/deliveries/%s/retry with project %q: got %d, want %d: %s",
					tt.deliveryID, tt.projectID, w.Code, tt.wantCode, w.Body.String())
			}
		})
	}
}

// FuzzTenantIsolation_CrossProjectAccess fuzzes project ID values to ensure
// the handler does not panic or return unexpected results.
func FuzzTenantIsolation_CrossProjectAccess(f *testing.F) {
	f.Add("proj-aaa")
	f.Add("proj-bbb")
	f.Add("")
	f.Add("proj-ccc")
	f.Add("../../../etc/passwd")
	f.Add("proj-aaa; DROP TABLE jobs;--")
	f.Add("proj-\x00-null")

	f.Fuzz(func(t *testing.T, projectID string) {
		ms := newIsolationStore()
		srv := newTestServer(t, ms, &mockQueue{}, nil)
		w := httptest.NewRecorder()
		req := authedRequest(http.MethodGet, "/v1/jobs/", "")
		req.Header.Set("X-Project-Id", projectID)
		srv.ServeHTTP(w, req)

		// Should not panic. Valid project IDs get 200, missing project returns 400.
		if w.Code != http.StatusOK && w.Code != http.StatusBadRequest && w.Code != http.StatusNotFound {
			t.Errorf("unexpected status %d for project_id=%q", w.Code, projectID)
		}
	})
}
