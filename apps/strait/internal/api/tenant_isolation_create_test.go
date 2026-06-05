package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

// TestCreateEnvironment_CrossProjectBlocked verifies that an API key for
// project A cannot create an environment in project B.
func TestCreateEnvironment_CrossProjectBlocked(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateEnvironmentFunc: func(_ context.Context, env *domain.Environment) error {
			require.Fail(t,

				"store.CreateEnvironment should not be called for cross-project request")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"project_id":"proj-OTHER","name":"env","slug":"env"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/environments", body, "proj-A"))
	require.Equal(t, http.StatusForbidden,
		w.Code,
	)
}

// TestCreateEnvironment_SameProjectAllowed verifies that creating an
// environment with the matching project succeeds.
func TestCreateEnvironment_SameProjectAllowed(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateEnvironmentFunc: func(_ context.Context, env *domain.Environment) error {
			env.ID = "env-1"
			env.CreatedAt = time.Now()
			env.UpdatedAt = time.Now()
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"project_id":"proj-A","name":"env","slug":"env"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/environments", body, "proj-A"))
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
}

// TestCreateWebhookSubscription_CrossProjectBlocked verifies that an API key
// for project A cannot create a webhook subscription in project B.
func TestCreateWebhookSubscription_CrossProjectBlocked(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateWebhookSubscriptionFunc: func(_ context.Context, _ *domain.WebhookSubscription) error {
			require.Fail(t,

				"store.CreateWebhookSubscription should not be called for cross-project request")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"project_id":"proj-OTHER","webhook_url":"https://example.com/hook","event_types":["run.completed"],"secret":"whsec_test"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/webhooks/subscriptions/", body, "proj-A"))
	require.Equal(t, http.StatusForbidden,
		w.Code,
	)
}

// TestCreateDeploymentVersion_CrossProjectBlocked verifies that an API key
// for project A cannot create a deployment version in project B.
func TestCreateDeploymentVersion_CrossProjectBlocked(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateDeploymentVersionFunc: func(_ context.Context, _ *domain.DeploymentVersion) error {
			require.Fail(t,

				"store.CreateDeploymentVersion should not be called for cross-project request")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"project_id":"proj-OTHER","environment":"prod","runtime":"node","artifact_uri":"https://example.com/v1.tar.gz"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/deployments", body, "proj-A"))
	require.Equal(t, http.StatusForbidden,
		w.Code,
	)
}

// TestFinalizeDeploymentVersion_CrossProjectBlocked verifies that finalize
// rejects cross-project requests.
func TestFinalizeDeploymentVersion_CrossProjectBlocked(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		FinalizeDeploymentVersionFunc: func(_ context.Context, _, _, _ string) (*domain.DeploymentVersion, error) {
			require.Fail(t,

				"store.FinalizeDeploymentVersion should not be called for cross-project request")
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"project_id":"proj-OTHER","environment":"prod"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/deployments/dep-1/finalize", body, "proj-A"))
	require.Equal(t, http.StatusForbidden,
		w.Code,
	)
}

// TestPromoteDeploymentVersion_CrossProjectBlocked verifies that promote
// rejects cross-project requests.
func TestPromoteDeploymentVersion_CrossProjectBlocked(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		PromoteDeploymentVersionFunc: func(_ context.Context, _, _, _, _ string) (*domain.DeploymentVersion, error) {
			require.Fail(t,

				"store.PromoteDeploymentVersion should not be called for cross-project request")
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"project_id":"proj-OTHER","environment":"prod"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/deployments/dep-1/promote", body, "proj-A"))
	require.Equal(t, http.StatusForbidden,
		w.Code,
	)
}

// TestRollbackDeploymentVersion_CrossProjectBlocked verifies that rollback
// rejects cross-project requests.
func TestRollbackDeploymentVersion_CrossProjectBlocked(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		RollbackDeploymentVersionFunc: func(_ context.Context, _, _, _, _ string) (*domain.DeploymentVersion, error) {
			require.Fail(t,

				"store.RollbackDeploymentVersion should not be called for cross-project request")
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"project_id":"proj-OTHER","environment":"prod"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/deployments/dep-1/rollback", body, "proj-A"))
	require.Equal(t, http.StatusForbidden,
		w.Code,
	)
}

// TestUpsertWorkflowPolicy_CrossProjectBlocked verifies that an API key
// for project A cannot upsert a workflow policy for project B.
func TestUpsertWorkflowPolicy_CrossProjectBlocked(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		UpsertWorkflowPolicyFunc: func(_ context.Context, _ *domain.WorkflowPolicy) error {
			require.Fail(t,

				"store.UpsertWorkflowPolicy should not be called for cross-project request")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"max_fan_out":10,"max_depth":5}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPut, "/v1/workflow-policies/proj-OTHER", body, "proj-A"))
	require.Equal(t, http.StatusNotFound,
		w.Code,
	)
}

// TestGetWorkflowPolicy_CrossProjectBlocked verifies that an API key
// for project A cannot read a workflow policy for project B.
func TestGetWorkflowPolicy_CrossProjectBlocked(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetWorkflowPolicyByProjectFunc: func(_ context.Context, _ string) (*domain.WorkflowPolicy, error) {
			require.Fail(t,

				"store.GetWorkflowPolicyByProject should not be called for cross-project request")
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/workflow-policies/proj-OTHER", "", "proj-A"))
	require.Equal(t, http.StatusNotFound,
		w.Code,
	)
}

// TestDeleteEventSubscription_WrongSourceBlocked verifies that deleting an
// event subscription under the wrong source ID returns 404.
func TestDeleteEventSubscription_WrongSourceBlocked(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetEventSourceFunc: func(_ context.Context, sourceID, projectID string) (*domain.EventSource, error) {
			return &domain.EventSource{ID: sourceID, ProjectID: projectID}, nil
		},
		GetEventSubscriptionFunc: func(_ context.Context, subID string) (*domain.EventSubscription, error) {
			return &domain.EventSubscription{
				ID:       subID,
				SourceID: "source-REAL",
			}, nil
		},
		DeleteEventSubscriptionFunc: func(_ context.Context, _ string) error {
			require.Fail(t,

				"store.DeleteEventSubscription should not be called when source mismatch")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodDelete, "/v1/event-sources/source-WRONG/subscriptions/sub-1", "", "proj-A"))
	require.Equal(t, http.StatusNotFound,
		w.Code,
	)
}

// TestDeleteEventSubscription_CorrectSourceAllowed verifies that deleting an
// event subscription under the correct source succeeds.
func TestDeleteEventSubscription_CorrectSourceAllowed(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetEventSourceFunc: func(_ context.Context, sourceID, projectID string) (*domain.EventSource, error) {
			return &domain.EventSource{ID: sourceID, ProjectID: projectID}, nil
		},
		GetEventSubscriptionFunc: func(_ context.Context, subID string) (*domain.EventSubscription, error) {
			return &domain.EventSubscription{
				ID:       subID,
				SourceID: "source-1",
			}, nil
		},
		DeleteEventSubscriptionFunc: func(_ context.Context, _ string) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodDelete, "/v1/event-sources/source-1/subscriptions/sub-1", "", "proj-A"))
	require.Equal(t, http.StatusNoContent,
		w.Code,
	)
}

// TestDeleteEventSubscription_NotFoundReturns404 verifies that deleting a
// nonexistent subscription returns 404.
func TestDeleteEventSubscription_NotFoundReturns404(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetEventSourceFunc: func(_ context.Context, sourceID, projectID string) (*domain.EventSource, error) {
			return &domain.EventSource{ID: sourceID, ProjectID: projectID}, nil
		},
		GetEventSubscriptionFunc: func(_ context.Context, _ string) (*domain.EventSubscription, error) {
			return nil, store.ErrEventSubscriptionNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodDelete, "/v1/event-sources/source-1/subscriptions/sub-missing", "", "proj-A"))
	require.Equal(t, http.StatusNotFound,
		w.Code,
	)
}

func TestSubscribeToEventSource_CrossProjectSourceBlocked(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetEventSourceFunc: func(_ context.Context, sourceID, projectID string) (*domain.EventSource, error) {
			require.Equal(t, "source-a", sourceID)
			require.Equal(t, projectB, projectID)

			return nil, store.ErrEventSourceNotFound
		},
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			require.Fail(t,

				"GetJob must not be called for cross-project source")
			return nil, nil
		},
		CreateEventSubscriptionFunc: func(_ context.Context, _ *domain.EventSubscription) error {
			require.Fail(t,

				"CreateEventSubscription must not be called for cross-project source")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"target_type":"job","target_id":"job-b"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/event-sources/source-a/subscribe", body, projectB))
	require.Equal(t, http.StatusNotFound,
		w.Code,
	)
}

func TestSubscribeToEventSource_CrossProjectJobTargetBlocked(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetEventSourceFunc: func(_ context.Context, sourceID, projectID string) (*domain.EventSource, error) {
			return &domain.EventSource{ID: sourceID, ProjectID: projectID, Name: "Source A", Enabled: true}, nil
		},
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			require.Equal(t, "job-b", id)

			return &domain.Job{ID: "job-b", ProjectID: projectB, Enabled: true}, nil
		},
		CreateEventSubscriptionFunc: func(_ context.Context, _ *domain.EventSubscription) error {
			require.Fail(t,

				"CreateEventSubscription must not be called for cross-project job target")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"target_type":"job","target_id":"job-b"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/event-sources/source-a/subscribe", body, projectA))
	require.Equal(t, http.StatusNotFound,
		w.Code,
	)
}

func TestSubscribeToEventSource_CrossProjectWorkflowTargetBlocked(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetEventSourceFunc: func(_ context.Context, sourceID, projectID string) (*domain.EventSource, error) {
			return &domain.EventSource{ID: sourceID, ProjectID: projectID, Name: "Source A", Enabled: true}, nil
		},
		GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
			require.Equal(t, "wf-b", id)

			return &domain.Workflow{ID: "wf-b", ProjectID: projectB, Enabled: true}, nil
		},
		CreateEventSubscriptionFunc: func(_ context.Context, _ *domain.EventSubscription) error {
			require.Fail(t,

				"CreateEventSubscription must not be called for cross-project workflow target")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"target_type":"workflow","target_id":"wf-b"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/event-sources/source-a/subscribe", body, projectA))
	require.Equal(t, http.StatusNotFound,
		w.Code,
	)
}

func TestSubscribeToEventSource_OwnProjectWorkflowTargetAllowed(t *testing.T) {
	t.Parallel()

	created := false
	ms := &APIStoreMock{
		GetEventSourceFunc: func(_ context.Context, sourceID, projectID string) (*domain.EventSource, error) {
			return &domain.EventSource{ID: sourceID, ProjectID: projectID, Name: "Source A", Enabled: true}, nil
		},
		GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
			return &domain.Workflow{ID: id, ProjectID: projectA, Enabled: true}, nil
		},
		CreateEventSubscriptionFunc: func(_ context.Context, sub *domain.EventSubscription) error {
			created = true
			require.Equal(t, "source-a", sub.
				SourceID)
			require.Equal(t, "wf-a", sub.TargetID)

			sub.ID = "sub-a"
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"target_type":"workflow","target_id":"wf-a"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/event-sources/source-a/subscribe", body, projectA))
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
	require.True(
		t, created)
}

// TestDeleteJobDependency_WrongJobBlocked verifies that deleting a job
// dependency under the wrong job ID returns 404.
func TestDeleteJobDependency_WrongJobBlocked(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-A", Enabled: true}, nil
		},
		GetJobDependencyFunc: func(_ context.Context, id string) (*domain.JobDependency, error) {
			return &domain.JobDependency{
				ID:             id,
				JobID:          "job-REAL",
				DependsOnJobID: "job-dep",
				Condition:      "completed",
				CreatedAt:      time.Now(),
			}, nil
		},
		DeleteJobDependencyFunc: func(_ context.Context, _ string) error {
			require.Fail(t,

				"store.DeleteJobDependency should not be called when job mismatch")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/jobs/job-WRONG/dependencies/dep-1", ""))
	require.Equal(t, http.StatusNotFound,
		w.Code,
	)
}

// TestDeleteJobDependency_CorrectJobAllowed verifies that deleting a job
// dependency under the correct job succeeds.
func TestDeleteJobDependency_CorrectJobAllowed(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-A", Enabled: true}, nil
		},
		GetJobDependencyFunc: func(_ context.Context, id string) (*domain.JobDependency, error) {
			return &domain.JobDependency{
				ID:             id,
				JobID:          "job-1",
				DependsOnJobID: "job-dep",
				Condition:      "completed",
				CreatedAt:      time.Now(),
			}, nil
		},
		DeleteJobDependencyFunc: func(_ context.Context, _ string) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/jobs/job-1/dependencies/dep-1", ""))
	require.Equal(t, http.StatusNoContent,
		w.Code,
	)
}

// TestCreateJob_CrossProjectBlocked verifies that an API key for project A
// cannot create a job in project B.
func TestCreateJob_CrossProjectBlocked(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateJobFunc: func(_ context.Context, _ *domain.Job) error {
			require.Fail(t,

				"store.CreateJob should not be called for cross-project request")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"project_id":"proj-OTHER","name":"test job","slug":"test-job","endpoint_url":"https://example.com/hook"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/jobs", body, "proj-A"))
	require.Equal(t, http.StatusForbidden,
		w.Code,
	)
}

// TestCreateJob_SameProjectAllowed verifies that creating a job with the
// matching project succeeds.
func TestCreateJob_SameProjectAllowed(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateJobFunc: func(_ context.Context, job *domain.Job) error {
			job.ID = "job-1"
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"project_id":"proj-A","name":"test job","slug":"test-job","endpoint_url":"https://example.com/hook"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/jobs", body, "proj-A"))
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
}

// TestCreateJob_TimeoutSecsExceeds86400Rejected verifies that timeout_secs
// above 86400 (24 hours) is rejected.
func TestCreateJob_TimeoutSecsExceeds86400Rejected(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateJobFunc: func(_ context.Context, _ *domain.Job) error {
			require.Fail(t,

				"store.CreateJob should not be called for excessive timeout_secs")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"project_id":"proj-A","name":"test job","slug":"test-job","endpoint_url":"https://example.com/hook","timeout_secs":999999999}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/jobs", body, "proj-A"))
	require.Equal(t, http.StatusBadRequest,
		w.
			Code)
}

// TestCreateJob_TimeoutSecs86400Allowed verifies that timeout_secs at exactly
// 86400 is accepted.
func TestCreateJob_TimeoutSecs86400Allowed(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateJobFunc: func(_ context.Context, job *domain.Job) error {
			job.ID = "job-1"
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"project_id":"proj-A","name":"test job","slug":"test-job","endpoint_url":"https://example.com/hook","timeout_secs":86400}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/jobs", body, "proj-A"))
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
}

// TestCreateAPIKey_CrossProjectBlocked verifies that an API key for project A
// cannot create an API key in project B.
func TestCreateAPIKey_CrossProjectBlocked(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateAPIKeyFunc: func(_ context.Context, _ *domain.APIKey) error {
			require.Fail(t,

				"store.CreateAPIKey should not be called for cross-project request")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"project_id":"proj-OTHER","name":"my key"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/api-keys", body, "proj-A"))
	require.Equal(t, http.StatusForbidden,
		w.Code,
	)
}

// TestCreateAPIKey_SameProjectAllowed verifies that creating an API key with
// the matching project succeeds.
func TestCreateAPIKey_SameProjectAllowed(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateAPIKeyFunc: func(_ context.Context, key *domain.APIKey) error {
			key.ID = "key-1"
			key.CreatedAt = time.Now()
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"project_id":"proj-A","name":"my key","scopes":["jobs:read"],"expires_in_days":30}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/api-keys", body, "proj-A"))
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
}

// TestCreateEventSource_CrossProjectBlocked verifies that an API key for
// project A cannot create an event source in project B.
func TestCreateEventSource_CrossProjectBlocked(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateEventSourceFunc: func(_ context.Context, _ *domain.EventSource) error {
			require.Fail(t,

				"store.CreateEventSource should not be called for cross-project request")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"project_id":"proj-OTHER","name":"src"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/event-sources", body, "proj-A"))
	require.Equal(t, http.StatusForbidden,
		w.Code,
	)
}

// TestCreateEventSource_SameProjectAllowed verifies that creating an event
// source with the matching project succeeds.
func TestCreateEventSource_SameProjectAllowed(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateEventSourceFunc: func(_ context.Context, src *domain.EventSource) error {
			src.ID = "src-1"
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"project_id":"proj-A","name":"src"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/event-sources", body, "proj-A"))
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
}

// TestCreateLogDrain_CrossProjectBlocked verifies that an API key for project A
// cannot create a log drain in project B.
func TestCreateLogDrain_CrossProjectBlocked(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateLogDrainFunc: func(_ context.Context, _ *domain.LogDrain) error {
			require.Fail(t,

				"store.CreateLogDrain should not be called for cross-project request")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"project_id":"proj-OTHER","name":"drain","drain_type":"http","endpoint_url":"https://example.com/logs","auth_type":"header"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/log-drains", body, "proj-A"))
	require.Equal(t, http.StatusForbidden,
		w.Code,
	)
}

// TestCreateLogDrain_SameProjectAllowed verifies that creating a log drain
// with the matching project succeeds.
func TestCreateLogDrain_SameProjectAllowed(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateLogDrainFunc: func(_ context.Context, drain *domain.LogDrain) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"project_id":"proj-A","name":"drain","drain_type":"http","endpoint_url":"https://example.com/logs","auth_type":"header"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/log-drains", body, "proj-A"))
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
}

// TestCreateSecret_CrossProjectBlocked verifies that an API key for project A
// cannot create a secret in project B.
func TestCreateSecret_CrossProjectBlocked(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateJobSecretFunc: func(_ context.Context, _ *domain.JobSecret) error {
			require.Fail(t,

				"store.CreateJobSecret should not be called for cross-project request")
			return nil
		},
	}
	srv := newTestServerWithEncryption(t, ms, &mockQueue{})

	body := `{"project_id":"proj-OTHER","secret_key":"MY_SECRET","value":"supersecret"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/secrets", body, "proj-A"))
	require.Equal(t, http.StatusForbidden,
		w.Code,
	)
}

// TestCreateSecret_SameProjectAllowed verifies that creating a secret with
// the matching project succeeds.
func TestCreateSecret_SameProjectAllowed(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateJobSecretFunc: func(_ context.Context, secret *domain.JobSecret) error {
			secret.ID = "sec-1"
			return nil
		},
	}
	srv := newTestServerWithEncryption(t, ms, &mockQueue{})

	body := `{"project_id":"proj-A","secret_key":"MY_SECRET","value":"supersecret"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/secrets", body, "proj-A"))
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
}

// TestTestWebhook_ErrorSanitized verifies that the webhook test endpoint
// does not leak internal network topology in error messages.
func TestTestWebhook_ErrorSanitized(t *testing.T) {
	globalAllowPrivateEndpoints.Store(true)
	t.Cleanup(func() { globalAllowPrivateEndpoints.Store(false) })

	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	srv.config.AllowPrivateEndpoints = true
	globalAllowPrivateEndpoints.Store(true)

	// Use an unreachable host that will cause a connection error.
	body := `{"url":"https://192.0.2.1:1/hook"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/webhooks/test", body, "proj-A"))
	require.Equal(t, http.StatusOK,
		w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	errMsg, ok := resp["error"].(string)
	require.True(
		t, ok)
	require.Equal(t, "connection to webhook URL failed",

		errMsg)
}

// TestDeleteJobDependency_NotFoundReturns404 verifies that deleting a
// nonexistent dependency returns 404.
func TestDeleteJobDependency_NotFoundReturns404(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-A", Enabled: true}, nil
		},
		GetJobDependencyFunc: func(_ context.Context, _ string) (*domain.JobDependency, error) {
			return nil, store.ErrJobDependencyNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/jobs/job-1/dependencies/dep-missing", ""))
	require.Equal(t, http.StatusNotFound,
		w.Code,
	)
}

// TestCreateDeploymentVersion_SameProjectAllowed verifies that creating a
// deployment version with the matching project succeeds.
func TestCreateDeploymentVersion_SameProjectAllowed(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateDeploymentVersionFunc: func(_ context.Context, dv *domain.DeploymentVersion) error {
			dv.ID = "dv-1"
			dv.CreatedAt = time.Now()
			dv.UpdatedAt = time.Now()
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"project_id":"proj-A","environment":"prod","runtime":"node","artifact_uri":"https://example.com/v1.tar.gz"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/deployments", body, "proj-A"))
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
}

// TestUpsertWorkflowPolicy_SameProjectAllowed verifies that upserting a
// workflow policy with the matching project succeeds.
func TestUpsertWorkflowPolicy_SameProjectAllowed(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		UpsertWorkflowPolicyFunc: func(_ context.Context, _ *domain.WorkflowPolicy) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"max_fan_out":10,"max_depth":5}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPut, "/v1/workflow-policies/proj-A", body, "proj-A"))
	require.Equal(t, http.StatusOK,
		w.Code)
}

// TestGetWorkflowPolicy_SameProjectAllowed verifies that reading a workflow
// policy with the matching project succeeds.
func TestGetWorkflowPolicy_SameProjectAllowed(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetWorkflowPolicyByProjectFunc: func(_ context.Context, projectID string) (*domain.WorkflowPolicy, error) {
			return &domain.WorkflowPolicy{
				ID:        "pol-1",
				ProjectID: projectID,
				MaxFanOut: 10,
				MaxDepth:  5,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/workflow-policies/proj-A", "", "proj-A"))
	require.Equal(t, http.StatusOK,
		w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, "proj-A", resp["project_id"])
}
