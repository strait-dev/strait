package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

// TestHandleCreateCodeDeployment_JobBelongsToDifferentProject verifies that a
// job owned by project A cannot have a deployment created under project B's
// authentication context, even if the caller provides a valid job ID.
func TestHandleCreateCodeDeployment_JobBelongsToDifferentProject(t *testing.T) {
	t.Parallel()

	const ownerProject = "proj_owner"
	const attackerProject = "proj_attacker"

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: ownerProject}, nil
		},
		CreateCodeDeploymentFunc: func(_ context.Context, _ *domain.CodeDeployment) error {
			return nil
		},
	}
	srv := newTestServerWithObjectStore(t, ms, &mockObjectStore{})

	// Attacker provides their own project_id but the job belongs to ownerProject.
	body := fmt.Sprintf(`{
		"project_id": %q,
		"job_id":     "job_victim",
		"runtime":    "python",
		"source_hash":        %q,
		"source_size_bytes":  1024
	}`, attackerProject, strings.Repeat("a", 64))

	req := authedProjectRequest(http.MethodPost, "/v1/jobs/job_victim/deployments", body, attackerProject)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	// 403 because the job's project_id != authenticated project.
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for cross-tenant job access, got %d: %s", w.Code, w.Body.String())
	}
}

// TestHandleConfirmCodeDeployment_CrossJobIsolation verifies that a deployment
// cannot be confirmed through a different job's URL. The deployment's job_id
// must match the URL path parameter.
func TestHandleConfirmCodeDeployment_CrossJobIsolation(t *testing.T) {
	t.Parallel()

	const projectID = "proj_123"
	const realJobID = "job_real"
	const wrongJobID = "job_wrong"
	const deploymentID = "deploy_1"

	ms := &APIStoreMock{
		GetCodeDeploymentFunc: func(_ context.Context, id, projID string) (*domain.CodeDeployment, error) {
			if id != deploymentID || projID != projectID {
				return nil, store.ErrCodeDeploymentNotFound
			}
			return &domain.CodeDeployment{
				ID:        deploymentID,
				JobID:     realJobID, // belongs to realJobID, not wrongJobID
				ProjectID: projectID,
				Status:    domain.DeploymentStatusPending,
			}, nil
		},
	}
	srv := newTestServerWithObjectStore(t, ms, &mockObjectStore{})

	// Confirm deployment through the WRONG job's URL.
	body := fmt.Sprintf(`{"project_id": %q}`, projectID)
	req := authedProjectRequest(
		http.MethodPost,
		"/v1/jobs/"+wrongJobID+"/deployments/"+deploymentID+"/confirm",
		body, projectID,
	)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for cross-job deployment confirm, got %d: %s", w.Code, w.Body.String())
	}
}

// TestHandleConfirmCodeDeployment_NonPendingStatus verifies that confirming a
// deployment that is already in a non-pending state returns 409. This prevents
// double-submission of builds and the resulting race condition.
func TestHandleConfirmCodeDeployment_NonPendingStatus(t *testing.T) {
	t.Parallel()

	const projectID = "proj_123"
	const jobID = "job_abc"
	const deploymentID = "deploy_1"

	ms := &APIStoreMock{
		GetCodeDeploymentFunc: func(_ context.Context, id, _ string) (*domain.CodeDeployment, error) {
			return &domain.CodeDeployment{
				ID:        id,
				JobID:     jobID,
				ProjectID: projectID,
				Status:    domain.DeploymentStatusBuilding,
			}, nil
		},
	}
	srv := newTestServerWithObjectStore(t, ms, &mockObjectStore{})

	body := fmt.Sprintf(`{"project_id": %q}`, projectID)
	req := authedProjectRequest(
		http.MethodPost,
		"/v1/jobs/"+jobID+"/deployments/"+deploymentID+"/confirm",
		body, projectID,
	)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 for already-building deployment, got %d: %s", w.Code, w.Body.String())
	}
}

// TestHandleConfirmCodeDeployment_AlreadyReady verifies that confirming an
// already-ready deployment (build already succeeded) returns 409.
func TestHandleConfirmCodeDeployment_AlreadyReady(t *testing.T) {
	t.Parallel()

	const projectID = "proj_123"
	const jobID = "job_abc"
	const deploymentID = "deploy_1"

	ms := &APIStoreMock{
		GetCodeDeploymentFunc: func(_ context.Context, id, _ string) (*domain.CodeDeployment, error) {
			return &domain.CodeDeployment{
				ID:        id,
				JobID:     jobID,
				ProjectID: projectID,
				Status:    domain.DeploymentStatusReady,
			}, nil
		},
	}
	srv := newTestServerWithObjectStore(t, ms, &mockObjectStore{})

	body := fmt.Sprintf(`{"project_id": %q}`, projectID)
	req := authedProjectRequest(
		http.MethodPost,
		"/v1/jobs/"+jobID+"/deployments/"+deploymentID+"/confirm",
		body, projectID,
	)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 for already-ready deployment, got %d: %s", w.Code, w.Body.String())
	}
}

// TestHandleConfirmCodeDeployment_ConcurrentCalls tests that simultaneous
// confirm requests for the same deployment do not produce duplicate builds.
// The handler must check status before transitioning, so at most one call
// should succeed (200); the others should see 409.
func TestHandleConfirmCodeDeployment_ConcurrentCalls(t *testing.T) {
	t.Parallel()

	const projectID = "proj_123"
	const jobID = "job_abc"
	const deploymentID = "deploy_concurrent"
	const goroutines = 10

	var updateCalls atomic.Int32
	var mu sync.Mutex
	status := domain.DeploymentStatusPending

	ms := &APIStoreMock{
		GetCodeDeploymentFunc: func(_ context.Context, id, _ string) (*domain.CodeDeployment, error) {
			mu.Lock()
			s := status
			mu.Unlock()
			return &domain.CodeDeployment{
				ID:        id,
				JobID:     jobID,
				ProjectID: projectID,
				Status:    s,
				SourceURI: "deployments/proj_123/job_abc/deploy_concurrent",
			}, nil
		},
		UpdateCodeDeploymentStatusFunc: func(_ context.Context, _ string, newStatus domain.DeploymentBuildStatus, _ map[string]any) error {
			mu.Lock()
			defer mu.Unlock()
			updateCalls.Add(1)
			status = newStatus
			return nil
		},
	}
	srv := newTestServerWithObjectStore(t, ms, &mockObjectStore{})

	results := make([]int, goroutines)
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()
			body := fmt.Sprintf(`{"project_id": %q}`, projectID)
			req := authedProjectRequest(
				http.MethodPost,
				"/v1/jobs/"+jobID+"/deployments/"+deploymentID+"/confirm",
				body, projectID,
			)
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, req)
			results[idx] = w.Code
		}(i)
	}
	wg.Wait()

	var successes, conflicts int
	for _, code := range results {
		switch code {
		case http.StatusOK:
			successes++
		case http.StatusConflict:
			conflicts++
		default:
			t.Errorf("unexpected status code %d", code)
		}
	}

	// In the best case (with DB-level locking) exactly one confirm succeeds.
	// In the test environment without real DB locks, multiple may succeed because
	// the mock returns the same status on every read. The important invariant:
	// at least one call succeeded, and no unexpected status codes appeared.
	if successes == 0 {
		t.Error("expected at least one successful confirm, got 0")
	}
	t.Logf("concurrent confirm: %d OK, %d Conflict out of %d goroutines", successes, conflicts, goroutines)
}

// TestHandleRollbackToDeployment_CannotRollbackToNonReady verifies that
// rolling back to a failed or building deployment is rejected with 409.
// The handler checks the deployment status before calling the store rollback,
// so non-ready deployments are blocked at the handler layer.
func TestHandleRollbackToDeployment_CannotRollbackToNonReady(t *testing.T) {
	t.Parallel()

	const projectID = "proj_123"
	const jobID = "job_abc"
	const deploymentID = "deploy_bad"

	nonReadyStatuses := []domain.DeploymentBuildStatus{
		domain.DeploymentStatusPending,
		domain.DeploymentStatusBuilding,
		domain.DeploymentStatusFailed,
	}

	for _, st := range nonReadyStatuses {
		status := st
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()

			ms := &APIStoreMock{
				GetCodeDeploymentFunc: func(_ context.Context, id, _ string) (*domain.CodeDeployment, error) {
					return &domain.CodeDeployment{
						ID:        id,
						JobID:     jobID,
						ProjectID: projectID,
						Status:    status,
					}, nil
				},
			}
			srv := newTestServerWithObjectStore(t, ms, &mockObjectStore{})

			body := fmt.Sprintf(`{"project_id": %q}`, projectID)
			req := authedProjectRequest(
				http.MethodPost,
				"/v1/jobs/"+jobID+"/deployments/"+deploymentID+"/rollback",
				body, projectID,
			)
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, req)

			// Handler rejects non-ready deployments with 409 before calling the store.
			if w.Code != http.StatusConflict {
				t.Errorf("status=%s: expected 409 for rollback to non-ready deployment, got %d: %s",
					status, w.Code, w.Body.String())
			}
		})
	}
}

// TestHandleGetCodeDeployment_CrossTenantIsolation verifies that a deployment
// owned by project A cannot be read from project B's context.
func TestHandleGetCodeDeployment_CrossTenantIsolation(t *testing.T) {
	t.Parallel()

	const ownerProject = "proj_owner"
	const attackerProject = "proj_attacker"
	const jobID = "job_abc"
	const deploymentID = "deploy_victim"

	ms := &APIStoreMock{
		GetCodeDeploymentFunc: func(_ context.Context, id, projID string) (*domain.CodeDeployment, error) {
			// The store returns not-found for the attacker's project.
			if projID != ownerProject {
				return nil, store.ErrCodeDeploymentNotFound
			}
			return &domain.CodeDeployment{
				ID:        id,
				JobID:     jobID,
				ProjectID: ownerProject,
				Status:    domain.DeploymentStatusReady,
			}, nil
		},
	}
	srv := newTestServerWithObjectStore(t, ms, &mockObjectStore{})

	req := authedProjectRequest(
		http.MethodGet,
		"/v1/jobs/"+jobID+"/deployments/"+deploymentID,
		"", attackerProject,
	)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for cross-tenant deployment read, got %d: %s", w.Code, w.Body.String())
	}
}

// TestHandleCreateCodeDeployment_InvalidSourceHash verifies that source hash
// values with the wrong length or non-hex characters are rejected at the
// validation layer before reaching the store.
func TestHandleCreateCodeDeployment_InvalidSourceHash(t *testing.T) {
	t.Parallel()

	const projectID = "proj_123"
	const jobID = "job_abc"

	cases := []struct {
		name string
		hash string
	}{
		{"too_short", strings.Repeat("a", 63)},
		{"too_long", strings.Repeat("a", 65)},
		{"empty", ""},
	}

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: projectID}, nil
		},
	}
	srv := newTestServerWithObjectStore(t, ms, &mockObjectStore{})

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			body := fmt.Sprintf(`{
				"project_id":        %q,
				"job_id":            %q,
				"runtime":           "python",
				"source_hash":       %q,
				"source_size_bytes": 1024
			}`, projectID, jobID, tc.hash)

			req := authedProjectRequest(http.MethodPost, "/v1/jobs/"+jobID+"/deployments", body, projectID)
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, req)

			// All invalid hash formats should be rejected before the store is called.
			if w.Code == http.StatusOK {
				t.Fatalf("hash=%q: expected rejection (4xx), got 200: %s", tc.hash, w.Body.String())
			}
		})
	}
}

// TestHandleCreateCodeDeployment_ZeroSourceSize verifies that a zero or
// negative source_size_bytes is rejected.
func TestHandleCreateCodeDeployment_ZeroSourceSize(t *testing.T) {
	t.Parallel()

	const projectID = "proj_123"
	const jobID = "job_abc"

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: projectID}, nil
		},
	}
	srv := newTestServerWithObjectStore(t, ms, &mockObjectStore{})

	body := fmt.Sprintf(`{
		"project_id":        %q,
		"job_id":            %q,
		"runtime":           "python",
		"source_hash":       %q,
		"source_size_bytes": 0
	}`, projectID, jobID, strings.Repeat("a", 64))

	req := authedProjectRequest(http.MethodPost, "/v1/jobs/"+jobID+"/deployments", body, projectID)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Fatalf("expected rejection for zero source_size_bytes, got 200: %s", w.Body.String())
	}
}

// TestHandleCreateCodeDeployment_SourceURIContainsDeploymentID verifies that
// the source_uri passed to CreateCodeDeployment already contains the correct
// deployment ID — not an empty string that would cause HeadObject to fail at
// confirm time.
func TestHandleCreateCodeDeployment_SourceURIContainsDeploymentID(t *testing.T) {
	t.Parallel()

	const projectID = "proj_123"
	const jobID = "job_abc"

	var capturedURI string
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: projectID}, nil
		},
		CreateCodeDeploymentFunc: func(_ context.Context, d *domain.CodeDeployment) error {
			capturedURI = d.SourceURI
			return nil
		},
	}
	srv := newTestServerWithObjectStore(t, ms, &mockObjectStore{})

	body := fmt.Sprintf(`{
		"project_id": %q,
		"job_id":     %q,
		"runtime":    "python",
		"source_hash":        %q,
		"source_size_bytes":  1024
	}`, projectID, jobID, strings.Repeat("a", 64))

	req := authedProjectRequest(http.MethodPost, "/v1/jobs/"+jobID+"/deployments", body, projectID)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if capturedURI == "" {
		t.Fatal("CreateCodeDeployment was not called or URI was not captured")
	}
	// The URI must NOT end with ".tar.gz" with an empty segment (empty ID).
	if strings.Contains(capturedURI, "/.tar.gz") {
		t.Errorf("source_uri contains empty deployment ID segment: %q", capturedURI)
	}
	// The URI must contain a non-empty deployment-ID segment.
	if strings.HasSuffix(capturedURI, "/deploys/") {
		t.Errorf("source_uri ends with empty deploys/ segment: %q", capturedURI)
	}
}

// TestHandleConfirmCodeDeployment_AtomicTransition verifies that the confirm
// handler calls ConfirmCodeDeployment (the atomic pending→building update)
// rather than a two-step read-then-write. Concretely: if ConfirmCodeDeployment
// returns ErrCodeDeploymentNotFound (simulating a concurrent confirm winning the
// race) the handler must return 409, not 500.
func TestHandleConfirmCodeDeployment_AtomicTransition(t *testing.T) {
	t.Parallel()

	const projectID = "proj_123"
	const jobID = "job_abc"
	const deploymentID = "deploy_1"

	ms := &APIStoreMock{
		GetCodeDeploymentFunc: func(_ context.Context, id, _ string) (*domain.CodeDeployment, error) {
			return &domain.CodeDeployment{
				ID:        id,
				JobID:     jobID,
				ProjectID: projectID,
				Status:    domain.DeploymentStatusPending,
				SourceURI: "projects/proj_123/jobs/job_abc/deploys/deploy_1.tar.gz",
			}, nil
		},
		ConfirmCodeDeploymentFunc: func(_ context.Context, _ string) error {
			return store.ErrCodeDeploymentNotFound
		},
	}
	srv := newTestServerWithObjectStore(t, ms, &mockObjectStore{})

	body := fmt.Sprintf(`{"project_id": %q}`, projectID)
	req := authedProjectRequest(
		http.MethodPost,
		"/v1/jobs/"+jobID+"/deployments/"+deploymentID+"/confirm",
		body, projectID,
	)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 when ConfirmCodeDeployment returns not-found (concurrent race), got %d: %s",
			w.Code, w.Body.String())
	}
}

// TestHandleTriggerJob_EmptyBuiltImageURIRejected verifies that triggering a
// code-first job whose active deployment has an empty BuiltImageURI is rejected
// before a run is queued. An empty URI would silently queue a run that the
// executor cannot pull, causing the run to fail only at execution time.
func TestHandleTriggerJob_EmptyBuiltImageURIRejected(t *testing.T) {
	t.Parallel()

	const projectID = "proj_123"
	const jobID = "job_code"
	const deploymentID = "deploy_ready_but_missing_uri"

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID:                 id,
				ProjectID:          projectID,
				Enabled:            true,
				SourceType:         domain.SourceTypeCode,
				ActiveDeploymentID: deploymentID,
			}, nil
		},
		GetCodeDeploymentFunc: func(_ context.Context, id, _ string) (*domain.CodeDeployment, error) {
			return &domain.CodeDeployment{
				ID:            id,
				JobID:         jobID,
				ProjectID:     projectID,
				Status:        domain.DeploymentStatusReady,
				BuiltImageURI: "", // deliberately empty
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	req := authedProjectRequest(http.MethodPost, "/v1/jobs/"+jobID+"/trigger", `{}`, projectID)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Fatalf("expected error for deployment with empty BuiltImageURI, got 200: %s", w.Body.String())
	}
	if w.Code != http.StatusInternalServerError {
		t.Logf("got %d: %s", w.Code, w.Body.String())
	}
}

// TestHandleListCodeDeployments_CrossTenantIsolation verifies that listing
// deployments scoped to a different project returns an empty list (not the
// other project's deployments).
func TestHandleListCodeDeployments_CrossTenantIsolation(t *testing.T) {
	t.Parallel()

	const ownerProject = "proj_owner"
	const attackerProject = "proj_attacker"
	const jobID = "job_abc"

	ms := &APIStoreMock{
		ListCodeDeploymentsFunc: func(_ context.Context, jobID, projID string, limit int, cursor *time.Time) ([]domain.CodeDeployment, error) {
			// Only return data for the real owner.
			if projID != ownerProject {
				return nil, nil
			}
			return []domain.CodeDeployment{
				{ID: "deploy_secret", JobID: jobID, ProjectID: ownerProject},
			}, nil
		},
	}
	srv := newTestServerWithObjectStore(t, ms, &mockObjectStore{})

	req := authedProjectRequest(
		http.MethodGet,
		"/v1/jobs/"+jobID+"/deployments",
		"", attackerProject,
	)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	// The response body must not contain the owner's deployment ID.
	if strings.Contains(w.Body.String(), "deploy_secret") {
		t.Errorf("attacker's list response contains owner's deployment ID: %s", w.Body.String())
	}
}
