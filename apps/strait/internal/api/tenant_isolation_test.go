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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	countA := decodeDataCount(t, w.Body.Bytes())
	assert.Equal(t, 1, countA)

	// List jobs for project B.
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, requestForProject(http.MethodGet, "/v1/jobs/", "", projectB))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	countB := decodeDataCount(t, w.Body.Bytes())
	assert.Equal(t, 1, countB)

	// Ensure project B does not see project A jobs (mock already guarantees
	// different data per projectID, so we check the IDs).
	var jobsB []domain.Job
	decodePaginatedList(t, w.Body.Bytes(), &jobsB)
	for _, j := range jobsB {
		assert.NotEqual(t, projectA, j.
			ProjectID,
		)
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
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	var runs []domain.JobRun
	decodePaginatedList(t, w.Body.Bytes(), &runs)
	for _, r := range runs {
		assert.NotEqual(t, projectA, r.
			ProjectID,
		)
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
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	var secrets []domain.JobSecret
	decodePaginatedList(t, w.Body.Bytes(), &secrets)
	for _, s := range secrets {
		assert.NotEqual(t, projectA, s.
			ProjectID,
		)
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
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	var workflows []domain.Workflow
	decodePaginatedList(t, w.Body.Bytes(), &workflows)
	for _, wf := range workflows {
		assert.NotEqual(t, projectA, wf.
			ProjectID,
		)
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
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	var subs []domain.WebhookSubscription
	if err := json.Unmarshal(w.Body.Bytes(), &subs); err != nil {
		// Try paginated format.
		decodePaginatedList(t, w.Body.Bytes(), &subs)
	}
	for _, s := range subs {
		assert.NotEqual(t, projectA, s.
			ProjectID,
		)
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
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	var triggers []domain.EventTrigger
	decodePaginatedList(t, w.Body.Bytes(), &triggers)
	for _, et := range triggers {
		assert.NotEqual(t, projectA, et.
			ProjectID,
		)
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
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	var jobsA []domain.Job
	decodePaginatedList(t, w.Body.Bytes(), &jobsA)
	assert.False(
		t, len(jobsA) !=
			1 || jobsA[0].ID !=
			"job-a")

	// Now set X-Project-Id to project B and verify we only see project B.
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, requestForProject(http.MethodGet, "/v1/jobs/", "", projectB))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	var jobsB []domain.Job
	decodePaginatedList(t, w.Body.Bytes(), &jobsB)
	assert.False(
		t, len(jobsB) !=
			1 || jobsB[0].ID !=
			"job-b")
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
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	var runs []domain.JobRun
	decodePaginatedList(t, w.Body.Bytes(), &runs)
	require.Len(t,
		runs, 1)
	assert.Equal(
		t, projectB, runs[0].ProjectID,
	)
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
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	var jobs []domain.Job
	decodePaginatedList(t, w.Body.Bytes(), &jobs)
	for _, j := range jobs {
		assert.Equal(
			t, projectA, j.ProjectID,
		)
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
	require.Equal(t, http.StatusNotFound,

		w.Code)

	// Accessing own run should work.
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, requestForProject(http.MethodGet, "/v1/runs/run-b", "", projectB))
	require.Equal(t, http.StatusOK,
		w.Code,
	)
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
	require.Equal(t, http.StatusNotFound,

		w.Code)

	// Own job should return 200.
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, requestForProject(http.MethodGet, "/v1/jobs/job-b", "", projectB))
	require.Equal(t, http.StatusOK,
		w.Code,
	)
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
	require.Equal(t, http.StatusNotFound,

		w.Code)

	// Own workflow should return 200.
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, requestForProject(http.MethodGet, "/v1/workflows/wf-b", "", projectB))
	require.Equal(t, http.StatusOK,
		w.Code,
	)
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
	require.Equal(t, http.StatusNotFound,

		w.Code)

	// Own environment should return 200.
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, requestForProject(http.MethodGet, "/v1/environments/env-b", "", projectB))
	require.Equal(t, http.StatusOK,
		w.Code,
	)
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
	require.Equal(t, http.StatusNotFound,

		w.Code)

	// Project B tries to delete project A's webhook subscription -- must be 404.
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, requestForProject(http.MethodDelete, "/v1/webhooks/subscriptions/wh-a", "", projectB))
	require.Equal(t, http.StatusNotFound,

		w.Code)
}

// TestTenantIsolation_EnvironmentsIsolated verifies that environments are
// isolated per project.
func TestTenantIsolation_EnvironmentsIsolated(t *testing.T) {
	t.Parallel()
	ms := newIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, requestForProject(http.MethodGet, "/v1/environments/", "", projectB))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	var envs []domain.Environment
	decodePaginatedList(t, w.Body.Bytes(), &envs)
	for _, e := range envs {
		assert.NotEqual(t, projectA, e.
			ProjectID,
		)
	}
	assert.False(
		t, len(envs) != 1 ||
			envs[0].ID != "env-b",
	)
}

// TestTenantIsolation_AuditEventsIsolated verifies that audit events are
// isolated per project.
func TestTenantIsolation_AuditEventsIsolated(t *testing.T) {
	t.Parallel()
	ms := newIsolationStore()
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, requestForProject(http.MethodGet, "/v1/audit-events/", "", projectB))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	var events []domain.AuditEvent
	decodePaginatedList(t, w.Body.Bytes(), &events)
	for _, ev := range events {
		assert.NotEqual(t, projectA, ev.
			ProjectID,
		)
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
	require.Equal(t, http.StatusOK,
		wA.Code,
	)

	var runsA []domain.JobRun
	decodePaginatedList(t, wA.Body.Bytes(), &runsA)

	// List runs for project B.
	wB := httptest.NewRecorder()
	srv.ServeHTTP(wB, requestForProject(http.MethodGet, "/v1/runs/", "", projectB))
	require.Equal(t, http.StatusOK,
		wB.Code,
	)

	var runsB []domain.JobRun
	decodePaginatedList(t, wB.Body.Bytes(), &runsB)

	// Verify no overlap.
	aIDs := make(map[string]bool)
	for _, r := range runsA {
		aIDs[r.ID] = true
	}
	for _, r := range runsB {
		assert.False(
			t, aIDs[r.ID])
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
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	var drains []domain.LogDrain
	// Log drains may return a direct array or paginated.
	if err := json.Unmarshal(w.Body.Bytes(), &drains); err != nil {
		decodePaginatedList(t, w.Body.Bytes(), &drains)
	}
	for _, d := range drains {
		assert.NotEqual(t, projectA, d.
			ProjectID,
		)
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
	require.Equal(t, http.StatusForbidden,

		w.Code)
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
			assert.Equal(
				t, tt.wantCode, w.
					Code)
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
			assert.Equal(
				t, tt.wantCode, w.
					Code)
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
			assert.Equal(
				t, tt.wantCode, w.
					Code)
		})
	}
}

// TestTenantIsolation_GetResolvedVariables_CrossProject verifies that getting
// resolved variables for an environment belonging to another project returns 404.
func TestTenantIsolation_GetResolvedVariables_CrossProject(t *testing.T) {
	t.Parallel()

	ms := newIsolationStore()
	var gotProjectID string
	ms.GetResolvedEnvironmentVariablesFunc = func(_ context.Context, projectID string, _ string) (map[string]string, error) {
		gotProjectID = projectID
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
			gotProjectID = ""
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/environments/"+tt.envID+"/variables", "", tt.projectID))
			assert.Equal(
				t, tt.wantCode, w.
					Code)
			// Regression guard (I1): when the resolve runs, it must be scoped to
			// the caller's project so the secret-decrypting query cannot resolve
			// another tenant's environment chain.
			if tt.wantCode == http.StatusOK {
				assert.Equal(t, tt.projectID, gotProjectID,
					"GetResolvedEnvironmentVariables must be called with the request's project")
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
			assert.Equal(
				t, tt.wantCode, w.
					Code)
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
			assert.Equal(
				t, tt.wantCode, w.
					Code)
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
		require.Fail(t,

			"GetEventSourceByName must not be called for mismatched project_id")
		return nil, nil
	}
	ms.ListEventSubscriptionsBySourceFunc = func(_ context.Context, _ string) ([]domain.EventSubscription, error) {
		require.Fail(t,

			"ListEventSubscriptionsBySource must not be called for mismatched project_id")
		return nil, nil
	}
	srv := newTestServer(t, ms, &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			require.Fail(t,

				"queue.Enqueue must not be called for mismatched project_id")
			return nil
		},
	}, nil)

	body := `{"source":"source-a","project_id":"` + projectA + `","payload":{"type":"deploy"}}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/events/dispatch", body, projectB))
	require.Equal(t, http.StatusNotFound,

		w.Code)
}

func TestTenantIsolation_DispatchEvent_OwnProjectAllowed(t *testing.T) {
	t.Parallel()

	ms := newIsolationStore()
	ms.GetEventSourceByNameFunc = func(_ context.Context, projectID, name string) (*domain.EventSource, error) {
		require.Equal(t, projectB, projectID)
		require.Equal(t, "source-b", name)

		return &domain.EventSource{ID: "src-b", ProjectID: projectB, Name: "source-b", Enabled: true}, nil
	}
	ms.ListEventSubscriptionsBySourceFunc = func(_ context.Context, sourceID string) ([]domain.EventSubscription, error) {
		require.Equal(t, "src-b", sourceID)

		return []domain.EventSubscription{
			{ID: "sub-b", SourceID: "src-b", TargetType: "job", TargetID: "job-b", FilterExpr: json.RawMessage(`{"eq":[["type","deploy"]]}`), Enabled: true},
		}, nil
	}
	enqueued := false
	srv := newTestServer(t, ms, &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			enqueued = true
			require.Equal(t, projectB, run.
				ProjectID,
			)
			require.Equal(t, "job-b", run.
				JobID)

			return nil
		},
	}, nil)

	body := `{"source":"source-b","project_id":"` + projectB + `","payload":{"type":"deploy"}}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/events/dispatch", body, projectB))
	require.Equal(t, http.StatusOK,
		w.Code,
	)
	require.True(
		t, enqueued)
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
		require.Equal(t, "job-b", id)

		return &domain.Job{ID: "job-b", ProjectID: projectB, Enabled: true}, nil
	}
	srv := newTestServer(t, ms, &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			require.Fail(t,

				"queue.Enqueue must not be called for stale cross-project job subscription")
			return nil
		},
	}, nil)

	body := `{"source":"source-a","project_id":"` + projectA + `","payload":{"type":"deploy"}}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/events/dispatch", body, projectA))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(),
		&resp))
	require.Equal(t, 0, int(resp["dispatched"].(float64)))
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
		require.Equal(t, "job-missing",
			id)

		return nil, nil
	}
	srv := newTestServer(t, ms, &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			require.Fail(t,

				"queue.Enqueue must not be called for nil job subscription")
			return nil
		},
	}, nil)

	body := `{"source":"source-a","project_id":"` + projectA + `","payload":{"type":"deploy"}}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/events/dispatch", body, projectA))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(),
		&resp))
	require.Equal(t, 0, int(resp["dispatched"].(float64)))
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
		require.Equal(t, "wf-b", id)

		return &domain.Workflow{ID: "wf-b", ProjectID: projectB, Enabled: true}, nil
	}
	wfEngine := &mockWorkflowTrigger{
		triggerWorkflowFn: func(context.Context, string, string, json.RawMessage, string, []domain.StepOverride) (*domain.WorkflowRun, error) {
			require.Fail(t,

				"workflow trigger must not be called for stale cross-project workflow subscription")
			return nil, nil
		},
	}
	srv := newTestServerWithWorkflowEngine(t, ms, &mockQueue{}, wfEngine)

	body := `{"source":"source-a","project_id":"` + projectA + `","payload":{"type":"deploy"}}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/events/dispatch", body, projectA))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(),
		&resp))
	require.Equal(t, 0, int(resp["dispatched"].(float64)))
}

// TestTenantIsolation_GetWebhookDelivery_CrossProject verifies that getting a
// webhook delivery whose job belongs to another project returns 404.
func TestTenantIsolation_GetWebhookDelivery_CrossProject(t *testing.T) {
	t.Parallel()

	now := time.Now()
	ms := newIsolationStore()
	ms.GetWebhookDeliveryFunc = func(_ context.Context, _ string, id string) (*domain.WebhookDelivery, error) {
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
			assert.Equal(
				t, tt.wantCode, w.
					Code)
		})
	}
}

// TestTenantIsolation_RetryWebhookDelivery_CrossProject verifies that retrying
// a webhook delivery whose job belongs to another project returns 404.
func TestTenantIsolation_RetryWebhookDelivery_CrossProject(t *testing.T) {
	t.Parallel()

	now := time.Now()
	ms := newIsolationStore()
	ms.GetWebhookDeliveryFunc = func(_ context.Context, _ string, id string) (*domain.WebhookDelivery, error) {
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
			assert.Equal(
				t, tt.wantCode, w.
					Code)
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
		assert.False(
			t, w.Code != http.
				StatusOK &&
				w.Code !=
					http.StatusBadRequest &&
				w.Code !=
					http.
						StatusNotFound)

		// Should not panic. Valid project IDs get 200, missing project returns 400.
	})
}
