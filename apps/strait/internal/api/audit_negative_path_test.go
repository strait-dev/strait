package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"
)

// TestAuditNegativePath_NoEmitOnStoreFailure verifies that when a store
// mutation returns an error, no audit event is emitted. This is the
// "ordering correctness" invariant: emit must only fire *after* the store
// write succeeded, never before it. A regression here would ship audit
// rows for failed mutations, which is a compliance finding.
//
// The test is table-driven over high-risk handlers where a false audit
// record would be most dangerous: anything involving secrets, api keys,
// rbac, and destructive operations.
func TestAuditNegativePath_NoEmitOnStoreFailure(t *testing.T) {
	t.Parallel()

	forcedErr := errors.New("forced store failure")

	type testCase struct {
		name   string
		method string
		path   string
		body   string
		setup  func(ms *APIStoreMock)
	}

	cases := []testCase{
		{
			name:   "handleCreateSecret_StoreError",
			method: http.MethodPost,
			path:   "/v1/secrets",
			body:   `{"project_id":"proj-1","secret_key":"k","value":"v","environment":"production"}`,
			setup: func(ms *APIStoreMock) {
				ms.CreateJobSecretFunc = func(_ context.Context, _ *domain.JobSecret) error {
					return forcedErr
				}
			},
		},
		{
			name:   "handleDeleteSecret_StoreError",
			method: http.MethodDelete,
			path:   "/v1/secrets/sec-1",
			body:   "",
			setup: func(ms *APIStoreMock) {
				ms.GetJobSecretFunc = func(_ context.Context, id string, _ string) (*domain.JobSecret, error) {
					return &domain.JobSecret{ID: id, ProjectID: "proj-1"}, nil
				}
				ms.DeleteJobSecretFunc = func(_ context.Context, _ string, _ string) error {
					return forcedErr
				}
			},
		},
		{
			name:   "handleCreateAPIKey_StoreError",
			method: http.MethodPost,
			path:   "/v1/api-keys",
			body:   `{"project_id":"proj-1","name":"k","scopes":["jobs:read"],"expires_in_days":30}`,
			setup: func(ms *APIStoreMock) {
				ms.CreateAPIKeyFunc = func(_ context.Context, _ *domain.APIKey) error {
					return forcedErr
				}
				ms.GetProjectQuotaFunc = func(_ context.Context, _ string) (*store.ProjectQuota, error) {
					return nil, nil
				}
			},
		},
		{
			name:   "handleRevokeAPIKey_StoreError",
			method: http.MethodDelete,
			path:   "/v1/api-keys/key-1",
			body:   "",
			setup: func(ms *APIStoreMock) {
				ms.GetAPIKeyByIDFunc = func(_ context.Context, id string) (*domain.APIKey, error) {
					return &domain.APIKey{ID: id, ProjectID: "proj-1"}, nil
				}
				ms.RevokeAPIKeyFunc = func(_ context.Context, _ string) error {
					return forcedErr
				}
			},
		},
		{
			name:   "handleCreateJob_StoreError",
			method: http.MethodPost,
			path:   "/v1/jobs",
			body:   `{"project_id":"proj-1","name":"Test","slug":"test","endpoint_url":"https://example.com/cb"}`,
			setup: func(ms *APIStoreMock) {
				ms.CreateJobFunc = func(_ context.Context, _ *domain.Job) error {
					return forcedErr
				}
			},
		},
		{
			name:   "handleDeleteJob_StoreError",
			method: http.MethodDelete,
			path:   "/v1/jobs/job-1",
			body:   "",
			setup: func(ms *APIStoreMock) {
				ms.GetJobFunc = func(_ context.Context, id string) (*domain.Job, error) {
					return &domain.Job{ID: id, ProjectID: "proj-1"}, nil
				}
				ms.DeleteJobFunc = func(_ context.Context, _ string) error {
					return forcedErr
				}
			},
		},
		{
			name:   "handleCancelRun_StoreError",
			method: http.MethodPost,
			path:   "/v1/runs/run-1/cancel",
			body:   "",
			setup: func(ms *APIStoreMock) {
				ms.GetRunFunc = func(_ context.Context, id string) (*domain.JobRun, error) {
					return &domain.JobRun{ID: id, ProjectID: "proj-1", Status: domain.StatusQueued}, nil
				}
				ms.UpdateRunStatusFunc = func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
					return forcedErr
				}
			},
		},
		{
			name:   "handleDeleteLogDrain_StoreError",
			method: http.MethodDelete,
			path:   "/v1/log-drains/drain-1",
			body:   "",
			setup: func(ms *APIStoreMock) {
				ms.DeleteLogDrainFunc = func(_ context.Context, _, _ string) error {
					return forcedErr
				}
			},
		},
		{
			name:   "handleDeleteWebhookSubscription_StoreError",
			method: http.MethodDelete,
			path:   "/v1/webhook-subscriptions/sub-1",
			body:   "",
			setup: func(ms *APIStoreMock) {
				ms.GetWebhookSubscriptionFunc = func(_ context.Context, id string) (*domain.WebhookSubscription, error) {
					return &domain.WebhookSubscription{ID: id, ProjectID: "proj-1"}, nil
				}
				ms.DeleteWebhookSubscriptionFunc = func(_ context.Context, _ string) error {
					return forcedErr
				}
			},
		},
		{
			name:   "handleDeleteProject_StoreError",
			method: http.MethodDelete,
			path:   "/v1/projects/proj-1",
			body:   "",
			setup: func(ms *APIStoreMock) {
				ms.DeleteProjectFunc = func(_ context.Context, _ string) error {
					return forcedErr
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var auditCalls atomic.Int32
			ms := &APIStoreMock{
				CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
					auditCalls.Add(1)
					return nil
				},
			}
			if tc.setup != nil {
				tc.setup(ms)
			}

			srv := newTestServer(t, ms, &mockQueue{}, nil)

			req := authedProjectRequest(tc.method, tc.path, tc.body, "proj-1")
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, req)

			// Any failure status is acceptable — the key assertion is "no audit".
			if w.Code >= 200 && w.Code < 300 {
				t.Fatalf("handler did not fail as expected: status=%d body=%s", w.Code, w.Body.String())
			}
			if got := auditCalls.Load(); got != 0 {
				t.Errorf("audit event emitted %d times on failure path (want 0)", got)
			}
		})
	}
}
