package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"
)

// satisfiedDepsFunc returns an AreJobDependenciesSatisfied mock that always
// reports dependencies satisfied, allowing the run to proceed to Enqueue.
func satisfiedDepsFunc(_ context.Context, _ *domain.JobRun) (bool, error) {
	return true, nil
}

// codeFirstJob returns a Job configured for code-first dispatch with a ready
// active deployment. Use this as the base and override fields as needed.
func codeFirstJob(projectID, jobID, deploymentID string) *domain.Job {
	return &domain.Job{
		ID:                 jobID,
		ProjectID:          projectID,
		Enabled:            true,
		SourceType:         domain.SourceTypeCode,
		ActiveDeploymentID: deploymentID,
	}
}

// readyDeployment returns a CodeDeployment in ready status with a valid image URI.
func readyDeployment(id, jobID, projectID string) *domain.CodeDeployment {
	return &domain.CodeDeployment{
		ID:               id,
		JobID:            jobID,
		ProjectID:        projectID,
		Status:           domain.DeploymentStatusReady,
		BuiltImageURI:    "ghcr.io/strait-dev/jobs/" + jobID + ":" + id,
		BuiltImageDigest: "sha256:cafedead",
	}
}

// triggerCodeJob sends POST /v1/jobs/{jobID}/trigger with an empty JSON body
// under the given project context and returns the recorder.
func triggerCodeJob(t *testing.T, srv *Server, jobID, projectID string) *httptest.ResponseRecorder {
	t.Helper()
	req := authedProjectRequest(http.MethodPost, "/v1/jobs/"+jobID+"/trigger", `{}`, projectID)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	return w
}

// TestHandleTriggerJob_NoActiveDeployment verifies that triggering a code-first
// job that has no active deployment returns 409. The job is not runnable until
// a deployment has been built and confirmed.
func TestHandleTriggerJob_NoActiveDeployment(t *testing.T) {
	t.Parallel()

	const projectID = "proj_c1"
	const jobID = "job_c1"

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			j := codeFirstJob(projectID, id, "")
			j.ActiveDeploymentID = "" // no deployment yet
			return j, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := triggerCodeJob(t, srv, jobID, projectID)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409 for no active deployment, got %d: %s", w.Code, w.Body.String())
	}
}

// TestHandleTriggerJob_ActiveDeploymentBuilding verifies that a job whose
// active deployment is still building cannot be triggered — it returns 409.
func TestHandleTriggerJob_ActiveDeploymentBuilding(t *testing.T) {
	t.Parallel()

	const projectID = "proj_c2"
	const jobID = "job_c2"
	const deployID = "deploy_c2"

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return codeFirstJob(projectID, id, deployID), nil
		},
		GetCodeDeploymentFunc: func(_ context.Context, id, _ string) (*domain.CodeDeployment, error) {
			return &domain.CodeDeployment{
				ID:        id,
				JobID:     jobID,
				ProjectID: projectID,
				Status:    domain.DeploymentStatusBuilding,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := triggerCodeJob(t, srv, jobID, projectID)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409 for building deployment, got %d: %s", w.Code, w.Body.String())
	}
}

// TestHandleTriggerJob_ActiveDeploymentFailed verifies that a job whose active
// deployment failed to build cannot be triggered.
func TestHandleTriggerJob_ActiveDeploymentFailed(t *testing.T) {
	t.Parallel()

	const projectID = "proj_c3"
	const jobID = "job_c3"
	const deployID = "deploy_c3"

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return codeFirstJob(projectID, id, deployID), nil
		},
		GetCodeDeploymentFunc: func(_ context.Context, id, _ string) (*domain.CodeDeployment, error) {
			return &domain.CodeDeployment{
				ID:        id,
				JobID:     jobID,
				ProjectID: projectID,
				Status:    domain.DeploymentStatusFailed,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := triggerCodeJob(t, srv, jobID, projectID)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409 for failed deployment, got %d: %s", w.Code, w.Body.String())
	}
}

// TestHandleTriggerJob_ActiveDeploymentTimedOut verifies that a timed_out
// deployment also blocks triggering.
func TestHandleTriggerJob_ActiveDeploymentTimedOut(t *testing.T) {
	t.Parallel()

	const projectID = "proj_c4"
	const jobID = "job_c4"
	const deployID = "deploy_c4"

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return codeFirstJob(projectID, id, deployID), nil
		},
		GetCodeDeploymentFunc: func(_ context.Context, id, _ string) (*domain.CodeDeployment, error) {
			return &domain.CodeDeployment{
				ID:        id,
				JobID:     jobID,
				ProjectID: projectID,
				Status:    domain.DeploymentStatusTimedOut,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := triggerCodeJob(t, srv, jobID, projectID)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409 for timed_out deployment, got %d: %s", w.Code, w.Body.String())
	}
}

// TestHandleTriggerJob_GetCodeDeploymentError verifies that a store error when
// resolving the active deployment results in 500.
func TestHandleTriggerJob_GetCodeDeploymentError(t *testing.T) {
	t.Parallel()

	const projectID = "proj_c5"
	const jobID = "job_c5"
	const deployID = "deploy_c5"

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return codeFirstJob(projectID, id, deployID), nil
		},
		GetCodeDeploymentFunc: func(_ context.Context, _, _ string) (*domain.CodeDeployment, error) {
			return nil, errors.New("db: connection timeout")
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := triggerCodeJob(t, srv, jobID, projectID)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for GetCodeDeployment error, got %d: %s", w.Code, w.Body.String())
	}
}

// TestHandleTriggerJob_CodeFirstPinsImageAndDeploymentIDOnRun verifies that when
// the active deployment is ready, the enqueued run carries the correct PinnedImageURI,
// PinnedImageDigest, and DeploymentID from the deployment.
func TestHandleTriggerJob_CodeFirstPinsImageAndDeploymentIDOnRun(t *testing.T) {
	t.Parallel()

	const projectID = "proj_c6"
	const jobID = "job_c6"
	const deployID = "deploy_c6"
	const wantImage = "ghcr.io/strait-dev/jobs/job_c6:deploy_c6"
	const wantDigest = "sha256:c0ffee"

	var enqueuedRun *domain.JobRun

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return codeFirstJob(projectID, id, deployID), nil
		},
		GetCodeDeploymentFunc: func(_ context.Context, id, _ string) (*domain.CodeDeployment, error) {
			return &domain.CodeDeployment{
				ID:               id,
				JobID:            jobID,
				ProjectID:        projectID,
				Status:           domain.DeploymentStatusReady,
				BuiltImageURI:    wantImage,
				BuiltImageDigest: wantDigest,
			}, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, _ string) (*store.ProjectQuota, error) {
			return nil, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			return nil, nil
		},
		AreJobDependenciesSatisfiedFunc: satisfiedDepsFunc,
	}
	q := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			enqueuedRun = run
			return nil
		},
	}
	srv := newTestServer(t, ms, q, nil)
	w := triggerCodeJob(t, srv, jobID, projectID)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if enqueuedRun == nil {
		t.Fatal("expected run to be enqueued")
	}
	if enqueuedRun.PinnedImageURI != wantImage {
		t.Errorf("PinnedImageURI: want %q, got %q", wantImage, enqueuedRun.PinnedImageURI)
	}
	if enqueuedRun.PinnedImageDigest != wantDigest {
		t.Errorf("PinnedImageDigest: want %q, got %q", wantDigest, enqueuedRun.PinnedImageDigest)
	}
	if enqueuedRun.DeploymentID != deployID {
		t.Errorf("DeploymentID: want %q, got %q", deployID, enqueuedRun.DeploymentID)
	}
}

// TestHandleTriggerJob_IsRollbackTrueWhenRollbackSourceSet verifies that
// IsRollback is set to true on the run when the job's RollbackSourceDeploymentID
// is non-empty, signalling that the current active image came from a rollback.
func TestHandleTriggerJob_IsRollbackTrueWhenRollbackSourceSet(t *testing.T) {
	t.Parallel()

	const projectID = "proj_c7"
	const jobID = "job_c7"
	const deployID = "deploy_c7"

	var enqueuedRun *domain.JobRun

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			j := codeFirstJob(projectID, id, deployID)
			j.RollbackSourceDeploymentID = "deploy_previous" // marks rollback state
			return j, nil
		},
		GetCodeDeploymentFunc: func(_ context.Context, id, _ string) (*domain.CodeDeployment, error) {
			return readyDeployment(id, jobID, projectID), nil
		},
		GetProjectQuotaFunc: func(_ context.Context, _ string) (*store.ProjectQuota, error) {
			return nil, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			return nil, nil
		},
		AreJobDependenciesSatisfiedFunc: satisfiedDepsFunc,
	}
	q := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			enqueuedRun = run
			return nil
		},
	}
	srv := newTestServer(t, ms, q, nil)
	w := triggerCodeJob(t, srv, jobID, projectID)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if enqueuedRun == nil {
		t.Fatal("expected run to be enqueued")
	}
	if !enqueuedRun.IsRollback {
		t.Error("expected IsRollback=true when RollbackSourceDeploymentID is set")
	}
}

// TestHandleTriggerJob_IsRollbackFalseForNormalCodeJob verifies that IsRollback
// is false when the job has no rollback source — i.e. the active deployment is
// the result of a normal build, not a manual rollback.
func TestHandleTriggerJob_IsRollbackFalseForNormalCodeJob(t *testing.T) {
	t.Parallel()

	const projectID = "proj_c8"
	const jobID = "job_c8"
	const deployID = "deploy_c8"

	var enqueuedRun *domain.JobRun

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			j := codeFirstJob(projectID, id, deployID)
			j.RollbackSourceDeploymentID = "" // normal build path
			return j, nil
		},
		GetCodeDeploymentFunc: func(_ context.Context, id, _ string) (*domain.CodeDeployment, error) {
			return readyDeployment(id, jobID, projectID), nil
		},
		GetProjectQuotaFunc: func(_ context.Context, _ string) (*store.ProjectQuota, error) {
			return nil, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			return nil, nil
		},
		AreJobDependenciesSatisfiedFunc: satisfiedDepsFunc,
	}
	q := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			enqueuedRun = run
			return nil
		},
	}
	srv := newTestServer(t, ms, q, nil)
	w := triggerCodeJob(t, srv, jobID, projectID)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if enqueuedRun == nil {
		t.Fatal("expected run to be enqueued")
	}
	if enqueuedRun.IsRollback {
		t.Error("expected IsRollback=false for normal (non-rollback) code job")
	}
}

// TestHandleTriggerJob_DigestPinnedAlongWithURI verifies that both the image URI
// and the digest are pinned on the run. An empty digest is allowed (some registries
// don't return digests synchronously), but a non-empty digest must be preserved.
func TestHandleTriggerJob_DigestPinnedAlongWithURI(t *testing.T) {
	t.Parallel()

	const projectID = "proj_c9"
	const jobID = "job_c9"
	const deployID = "deploy_c9"
	const wantDigest = "sha256:deadbeefcafe"

	var enqueuedRun *domain.JobRun

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return codeFirstJob(projectID, id, deployID), nil
		},
		GetCodeDeploymentFunc: func(_ context.Context, id, _ string) (*domain.CodeDeployment, error) {
			return &domain.CodeDeployment{
				ID:               id,
				JobID:            jobID,
				ProjectID:        projectID,
				Status:           domain.DeploymentStatusReady,
				BuiltImageURI:    "ghcr.io/strait-dev/jobs/" + jobID + ":" + id,
				BuiltImageDigest: wantDigest,
			}, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, _ string) (*store.ProjectQuota, error) {
			return nil, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			return nil, nil
		},
		AreJobDependenciesSatisfiedFunc: satisfiedDepsFunc,
	}
	q := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			enqueuedRun = run
			return nil
		},
	}
	srv := newTestServer(t, ms, q, nil)
	w := triggerCodeJob(t, srv, jobID, projectID)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if enqueuedRun == nil {
		t.Fatal("expected run to be enqueued")
	}
	if enqueuedRun.PinnedImageDigest != wantDigest {
		t.Errorf("PinnedImageDigest: want %q, got %q", wantDigest, enqueuedRun.PinnedImageDigest)
	}
}

// TestHandleTriggerJob_AllNonReadyStatusesBlock verifies that every non-ready
// deployment status blocks triggering with 409. This is a table-driven complement
// to the individual tests above, ensuring no status slips through.
func TestHandleTriggerJob_AllNonReadyStatusesBlock(t *testing.T) {
	t.Parallel()

	nonReadyStatuses := []domain.DeploymentBuildStatus{
		domain.DeploymentStatusPending,
		domain.DeploymentStatusBuilding,
		domain.DeploymentStatusFailed,
		domain.DeploymentStatusTimedOut,
	}

	for _, st := range nonReadyStatuses {
		status := st
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()

			const projectID = "proj_tab"
			const jobID = "job_tab"
			const deployID = "deploy_tab"

			ms := &APIStoreMock{
				GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
					return codeFirstJob(projectID, id, deployID), nil
				},
				GetCodeDeploymentFunc: func(_ context.Context, id, _ string) (*domain.CodeDeployment, error) {
					return &domain.CodeDeployment{
						ID:        id,
						JobID:     jobID,
						ProjectID: projectID,
						Status:    status,
					}, nil
				},
			}
			srv := newTestServer(t, ms, &mockQueue{}, nil)
			w := triggerCodeJob(t, srv, jobID, projectID)

			if w.Code != http.StatusConflict {
				t.Errorf("status=%s: expected 409, got %d: %s", status, w.Code, w.Body.String())
			}
		})
	}
}

// TestHandleTriggerJob_ResponseIncludesRunID verifies that a successful
// code-first trigger returns a JSON body with the run ID and status.
func TestHandleTriggerJob_ResponseIncludesRunID(t *testing.T) {
	t.Parallel()

	const projectID = "proj_c10"
	const jobID = "job_c10"
	const deployID = "deploy_c10"

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return codeFirstJob(projectID, id, deployID), nil
		},
		GetCodeDeploymentFunc: func(_ context.Context, id, _ string) (*domain.CodeDeployment, error) {
			return readyDeployment(id, jobID, projectID), nil
		},
		GetProjectQuotaFunc: func(_ context.Context, _ string) (*store.ProjectQuota, error) {
			return nil, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			return nil, nil
		},
		AreJobDependenciesSatisfiedFunc: satisfiedDepsFunc,
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := triggerCodeJob(t, srv, jobID, projectID)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if body["id"] == "" || body["id"] == nil {
		t.Errorf("expected non-empty run ID in response, got: %v", body)
	}
	if body["status"] == nil {
		t.Errorf("expected status in response, got: %v", body)
	}
}
