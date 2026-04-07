package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"strait/internal/build"
	"strait/internal/domain"
	"strait/internal/objectstore"
	"strait/internal/pubsub"
	"strait/internal/store"
)

// mockObjectStore is a minimal stub for objectstore.ObjectStore.
type mockObjectStore struct {
	presignUploadFn func(ctx context.Context, key string, ttl time.Duration) (string, error)
	headObjectFn    func(ctx context.Context, key string) (int64, error)
	getObjectFn     func(ctx context.Context, key string) (io.ReadCloser, error)
}

func (m *mockObjectStore) PresignUpload(ctx context.Context, key string, ttl time.Duration) (string, error) {
	if m.presignUploadFn != nil {
		return m.presignUploadFn(ctx, key, ttl)
	}
	return "https://example.com/upload", nil
}

func (m *mockObjectStore) HeadObject(ctx context.Context, key string) (int64, error) {
	if m.headObjectFn != nil {
		return m.headObjectFn(ctx, key)
	}
	// Default: report the canonical test tarball size.
	return int64(len(testTarballContent)), nil
}

func (m *mockObjectStore) GetObject(ctx context.Context, key string) (io.ReadCloser, error) {
	if m.getObjectFn != nil {
		return m.getObjectFn(ctx, key)
	}
	// Default: return the canonical test tarball bytes so hash checks pass.
	return io.NopCloser(strings.NewReader(testTarballContent)), nil
}

func (m *mockObjectStore) PutObject(_ context.Context, _ string, _ io.Reader, _ int64) error {
	return nil
}

func (m *mockObjectStore) DeleteObject(_ context.Context, _ string) error {
	return nil
}

// testTarballContent is a fixed tarball payload used across tests. Its SHA-256
// and size are pre-computed so that any test setting up a pending deployment can
// reference testTarballHash and testTarballSize to pass hash/size verification.
const testTarballContent = "fake-tarball-content-for-testing"

// testTarballHash is the SHA-256 hex digest of testTarballContent
// (printf '%s' 'fake-tarball-content-for-testing' | shasum -a 256).
const testTarballHash = "dd3e10dc3100ca1e6ab2fbf3dd1312429e4e0289f7a3f3ca2c8aa3f3aec4062b"

// testTarballSize is the byte length of testTarballContent as int64 (matches SourceSizeBytes).
const testTarballSize = int64(len(testTarballContent))

// newTestServerWithObjectStore creates a test server with an object store configured.
func newTestServerWithObjectStore(t *testing.T, s APIStore, os objectstore.ObjectStore) *Server {
	t.Helper()
	srv := newTestServer(t, s, nil, nil)
	srv.objectStore = os
	return srv
}

func TestHandleCreateCodeDeployment_Success(t *testing.T) {
	projectID := "proj_123"
	jobID := "job_abc"

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			if id != jobID {
				return nil, store.ErrJobNotFound
			}
			return &domain.Job{ID: jobID, ProjectID: projectID}, nil
		},
		CreateCodeDeploymentFunc: func(_ context.Context, d *domain.CodeDeployment) error {
			d.ID = "deploy_1"
			d.CreatedAt = time.Now()
			d.UpdatedAt = time.Now()
			return nil
		},
	}
	mos := &mockObjectStore{}
	srv := newTestServerWithObjectStore(t, ms, mos)

	body := fmt.Sprintf(`{
		"project_id": %q,
		"job_id": %q,
		"runtime": "python",
		"source_hash": %q,
		"source_size_bytes": 1024
	}`, projectID, jobID, strings.Repeat("a", 64))

	req := authedProjectRequest(http.MethodPost, "/v1/jobs/"+jobID+"/deployments", body, projectID)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Deployment *domain.CodeDeployment `json:"deployment"`
		UploadURL  string                 `json:"upload_url"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Deployment == nil {
		t.Fatal("expected deployment in response, got nil")
	}
	if resp.UploadURL == "" {
		t.Fatal("expected upload_url in response, got empty")
	}
	if resp.Deployment.Status != domain.DeploymentStatusPending {
		t.Errorf("expected status pending, got %s", resp.Deployment.Status)
	}
}

func TestHandleCreateCodeDeployment_NoObjectStore(t *testing.T) {
	projectID := "proj_123"
	jobID := "job_abc"

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: projectID}, nil
		},
		CreateCodeDeploymentFunc: func(_ context.Context, d *domain.CodeDeployment) error {
			d.ID = "deploy_1"
			return nil
		},
	}
	srv := newTestServer(t, ms, nil, nil) // no objectStore

	body := fmt.Sprintf(`{
		"project_id": %q,
		"job_id": %q,
		"runtime": "python",
		"source_hash": %q,
		"source_size_bytes": 1024
	}`, projectID, jobID, strings.Repeat("a", 64))

	req := authedProjectRequest(http.MethodPost, "/v1/jobs/"+jobID+"/deployments", body, projectID)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when objectStore is nil, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCreateCodeDeployment_WrongProject(t *testing.T) {
	ms := &APIStoreMock{}
	srv := newTestServerWithObjectStore(t, ms, &mockObjectStore{})

	body := fmt.Sprintf(`{
		"project_id": "other_project",
		"job_id": "job_abc",
		"runtime": "python",
		"source_hash": %q,
		"source_size_bytes": 1024
	}`, strings.Repeat("a", 64))

	req := authedProjectRequest(http.MethodPost, "/v1/jobs/job_abc/deployments", body, "proj_123")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestHandleCreateCodeDeployment_JobNotFound(t *testing.T) {
	projectID := "proj_123"
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return nil, store.ErrJobNotFound
		},
	}
	srv := newTestServerWithObjectStore(t, ms, &mockObjectStore{})

	body := fmt.Sprintf(`{
		"project_id": %q,
		"job_id": "missing_job",
		"runtime": "python",
		"source_hash": %q,
		"source_size_bytes": 1024
	}`, projectID, strings.Repeat("a", 64))

	req := authedProjectRequest(http.MethodPost, "/v1/jobs/missing_job/deployments", body, projectID)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCreateCodeDeployment_InvalidRuntime(t *testing.T) {
	projectID := "proj_123"
	jobID := "job_abc"
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: projectID}, nil
		},
	}
	srv := newTestServerWithObjectStore(t, ms, &mockObjectStore{})

	body := fmt.Sprintf(`{
		"project_id": %q,
		"job_id": %q,
		"runtime": "cobol",
		"source_hash": %q,
		"source_size_bytes": 1024
	}`, projectID, jobID, strings.Repeat("a", 64))

	req := authedProjectRequest(http.MethodPost, "/v1/jobs/"+jobID+"/deployments", body, projectID)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid runtime, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleConfirmCodeDeployment_Success(t *testing.T) {
	projectID := "proj_123"
	jobID := "job_abc"
	deploymentID := "deploy_1"

	d := &domain.CodeDeployment{
		ID:              deploymentID,
		JobID:           jobID,
		ProjectID:       projectID,
		Status:          domain.DeploymentStatusPending,
		SourceURI:       "projects/proj_123/jobs/job_abc/deploys/deploy_1.tar.gz",
		SourceHash:      testTarballHash,
		SourceSizeBytes: testTarballSize,
	}

	ms := &APIStoreMock{
		GetCodeDeploymentFunc: func(_ context.Context, id, _ string) (*domain.CodeDeployment, error) {
			if id != deploymentID {
				return nil, store.ErrCodeDeploymentNotFound
			}
			return d, nil
		},
		ConfirmCodeDeploymentFunc: func(_ context.Context, _ string) error {
			return nil
		},
	}
	mos := &mockObjectStore{}
	srv := newTestServerWithObjectStore(t, ms, mos)

	body := fmt.Sprintf(`{"project_id": %q}`, projectID)
	path := fmt.Sprintf("/v1/jobs/%s/deployments/%s/confirm", jobID, deploymentID)
	req := authedProjectRequest(http.MethodPost, path, body, projectID)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp domain.CodeDeployment
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status != domain.DeploymentStatusBuilding {
		t.Errorf("expected status building, got %s", resp.Status)
	}
}

func TestHandleConfirmCodeDeployment_AlreadyBuilding(t *testing.T) {
	projectID := "proj_123"
	jobID := "job_abc"
	deploymentID := "deploy_1"

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
	path := fmt.Sprintf("/v1/jobs/%s/deployments/%s/confirm", jobID, deploymentID)
	req := authedProjectRequest(http.MethodPost, path, body, projectID)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleConfirmCodeDeployment_TarballNotUploaded(t *testing.T) {
	projectID := "proj_123"
	jobID := "job_abc"
	deploymentID := "deploy_1"

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
	}
	mos := &mockObjectStore{
		headObjectFn: func(_ context.Context, _ string) (int64, error) {
			return 0, objectstore.ErrObjectNotFound
		},
	}
	srv := newTestServerWithObjectStore(t, ms, mos)

	body := fmt.Sprintf(`{"project_id": %q}`, projectID)
	path := fmt.Sprintf("/v1/jobs/%s/deployments/%s/confirm", jobID, deploymentID)
	req := authedProjectRequest(http.MethodPost, path, body, projectID)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleGetCodeDeployment_Success(t *testing.T) {
	projectID := "proj_123"
	jobID := "job_abc"
	deploymentID := "deploy_1"

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
	srv := newTestServer(t, ms, nil, nil)

	path := fmt.Sprintf("/v1/jobs/%s/deployments/%s", jobID, deploymentID)
	req := authedProjectRequest(http.MethodGet, path, "", projectID)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp domain.CodeDeployment
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ID != deploymentID {
		t.Errorf("expected id %s, got %s", deploymentID, resp.ID)
	}
}

func TestHandleGetCodeDeployment_WrongJob(t *testing.T) {
	projectID := "proj_123"
	deploymentID := "deploy_1"

	ms := &APIStoreMock{
		GetCodeDeploymentFunc: func(_ context.Context, id, _ string) (*domain.CodeDeployment, error) {
			return &domain.CodeDeployment{
				ID:        id,
				JobID:     "different_job", // belongs to a different job
				ProjectID: projectID,
				Status:    domain.DeploymentStatusReady,
			}, nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	path := fmt.Sprintf("/v1/jobs/job_abc/deployments/%s", deploymentID)
	req := authedProjectRequest(http.MethodGet, path, "", projectID)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for mismatched job, got %d", w.Code)
	}
}

func TestHandleGetCodeDeployment_NotFound(t *testing.T) {
	projectID := "proj_123"

	ms := &APIStoreMock{
		GetCodeDeploymentFunc: func(_ context.Context, _ string, _ string) (*domain.CodeDeployment, error) {
			return nil, store.ErrCodeDeploymentNotFound
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	req := authedProjectRequest(http.MethodGet, "/v1/jobs/job_abc/deployments/missing", "", projectID)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

// TestHandleGetCodeDeployment_TimedOutStatus verifies that a deployment with
// timed_out status is returned as-is via the GET endpoint — the client must be
// able to distinguish timed_out from failed so it can show an appropriate message.
func TestHandleGetCodeDeployment_TimedOutStatus(t *testing.T) {
	t.Parallel()

	const projectID = "proj_to"
	const jobID = "job_to"
	const deploymentID = "deploy_to"

	ms := &APIStoreMock{
		GetCodeDeploymentFunc: func(_ context.Context, id, _ string) (*domain.CodeDeployment, error) {
			return &domain.CodeDeployment{
				ID:           id,
				JobID:        jobID,
				ProjectID:    projectID,
				Status:       domain.DeploymentStatusTimedOut,
				ErrorMessage: "build exceeded the 10 minute timeout",
			}, nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	path := fmt.Sprintf("/v1/jobs/%s/deployments/%s", jobID, deploymentID)
	req := authedProjectRequest(http.MethodGet, path, "", projectID)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp domain.CodeDeployment
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status != domain.DeploymentStatusTimedOut {
		t.Errorf("expected status=timed_out, got %q", resp.Status)
	}
}

func TestHandleListCodeDeployments_Success(t *testing.T) {
	projectID := "proj_123"
	jobID := "job_abc"
	now := time.Now()

	ms := &APIStoreMock{
		ListCodeDeploymentsFunc: func(_ context.Context, jid, _ string, _ int, _ *time.Time) ([]domain.CodeDeployment, error) {
			if jid != jobID {
				return nil, nil
			}
			return []domain.CodeDeployment{
				{ID: "d1", JobID: jobID, ProjectID: projectID, Status: domain.DeploymentStatusReady, CreatedAt: now},
				{ID: "d2", JobID: jobID, ProjectID: projectID, Status: domain.DeploymentStatusBuilding, CreatedAt: now.Add(-time.Minute)},
			}, nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	req := authedProjectRequest(http.MethodGet, "/v1/jobs/"+jobID+"/deployments", "", projectID)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var paged PaginatedResponse
	if err := json.Unmarshal(w.Body.Bytes(), &paged); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	var items []domain.CodeDeployment
	raw, _ := json.Marshal(paged.Data)
	if err := json.Unmarshal(raw, &items); err != nil {
		t.Fatalf("decode items: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 deployments, got %d", len(items))
	}
}

func TestHandleRollbackCodeDeployment_Success(t *testing.T) {
	projectID := "proj_123"
	jobID := "job_abc"
	deploymentID := "deploy_old"

	ms := &APIStoreMock{
		GetCodeDeploymentFunc: func(_ context.Context, id, _ string) (*domain.CodeDeployment, error) {
			return &domain.CodeDeployment{
				ID:        id,
				JobID:     jobID,
				ProjectID: projectID,
				Status:    domain.DeploymentStatusReady,
			}, nil
		},
		RollbackToDeploymentFunc: func(_ context.Context, _, _, _ string) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	body := fmt.Sprintf(`{"project_id": %q}`, projectID)
	path := fmt.Sprintf("/v1/jobs/%s/deployments/%s/rollback", jobID, deploymentID)
	req := authedProjectRequest(http.MethodPost, path, body, projectID)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleRollbackCodeDeployment_NotReady(t *testing.T) {
	projectID := "proj_123"
	jobID := "job_abc"
	deploymentID := "deploy_1"

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
	srv := newTestServer(t, ms, nil, nil)

	body := fmt.Sprintf(`{"project_id": %q}`, projectID)
	path := fmt.Sprintf("/v1/jobs/%s/deployments/%s/rollback", jobID, deploymentID)
	req := authedProjectRequest(http.MethodPost, path, body, projectID)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleRollbackCodeDeployment_DeploymentNotFound(t *testing.T) {
	projectID := "proj_123"
	jobID := "job_abc"

	ms := &APIStoreMock{
		GetCodeDeploymentFunc: func(_ context.Context, _ string, _ string) (*domain.CodeDeployment, error) {
			return nil, store.ErrCodeDeploymentNotFound
		},
		RollbackToDeploymentFunc: func(_ context.Context, _, _, _ string) error {
			return store.ErrCodeDeploymentNotFound
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	body := fmt.Sprintf(`{"project_id": %q}`, projectID)
	path := fmt.Sprintf("/v1/jobs/%s/deployments/missing/rollback", jobID)
	req := authedProjectRequest(http.MethodPost, path, body, projectID)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCreateCodeDeployment_PresignError(t *testing.T) {
	projectID := "proj_123"
	jobID := "job_abc"

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: projectID}, nil
		},
		CreateCodeDeploymentFunc: func(_ context.Context, d *domain.CodeDeployment) error {
			d.ID = "deploy_1"
			return nil
		},
	}
	mos := &mockObjectStore{
		presignUploadFn: func(_ context.Context, _ string, _ time.Duration) (string, error) {
			return "", errors.New("object store unavailable")
		},
	}
	srv := newTestServerWithObjectStore(t, ms, mos)

	body := fmt.Sprintf(`{
		"project_id": %q,
		"job_id": %q,
		"runtime": "go",
		"source_hash": %q,
		"source_size_bytes": 2048
	}`, projectID, jobID, strings.Repeat("b", 64))

	req := authedProjectRequest(http.MethodPost, "/v1/jobs/"+jobID+"/deployments", body, projectID)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 on presign error, got %d: %s", w.Code, w.Body.String())
	}
}

// --- Deployment log streaming tests.

func TestHandleDeploymentLogs_TerminalReturnsStoredLogs(t *testing.T) {
	projectID := "proj_123"
	jobID := "job_abc"
	deploymentID := "deploy_1"

	ms := &APIStoreMock{
		GetCodeDeploymentFunc: func(_ context.Context, id, _ string) (*domain.CodeDeployment, error) {
			return &domain.CodeDeployment{
				ID:        id,
				JobID:     jobID,
				ProjectID: projectID,
				Status:    domain.DeploymentStatusReady,
				BuildLogs: "step1\nstep2\ndone",
			}, nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	path := fmt.Sprintf("/v1/jobs/%s/deployments/%s/logs", jobID, deploymentID)
	req := authedProjectRequest(http.MethodGet, path, "", projectID)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("expected text/plain, got %q", ct)
	}
	if !strings.Contains(w.Body.String(), "step1") {
		t.Errorf("expected stored logs in response, got %q", w.Body.String())
	}
}

func TestHandleDeploymentLogs_BuildingNonStreamReturnsPartialLogs(t *testing.T) {
	projectID := "proj_123"
	jobID := "job_abc"
	deploymentID := "deploy_1"

	ms := &APIStoreMock{
		GetCodeDeploymentFunc: func(_ context.Context, id, _ string) (*domain.CodeDeployment, error) {
			return &domain.CodeDeployment{
				ID:        id,
				JobID:     jobID,
				ProjectID: projectID,
				Status:    domain.DeploymentStatusBuilding,
				BuildLogs: "partial logs so far",
			}, nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	path := fmt.Sprintf("/v1/jobs/%s/deployments/%s/logs", jobID, deploymentID)
	req := authedProjectRequest(http.MethodGet, path, "", projectID)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "partial logs") {
		t.Errorf("expected partial logs in response, got %q", w.Body.String())
	}
}

func TestHandleDeploymentLogs_NotFound(t *testing.T) {
	projectID := "proj_123"
	jobID := "job_abc"

	ms := &APIStoreMock{
		GetCodeDeploymentFunc: func(_ context.Context, _ string, _ string) (*domain.CodeDeployment, error) {
			return nil, store.ErrCodeDeploymentNotFound
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	path := fmt.Sprintf("/v1/jobs/%s/deployments/missing/logs", jobID)
	req := authedProjectRequest(http.MethodGet, path, "", projectID)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleDeploymentLogs_WrongJob(t *testing.T) {
	projectID := "proj_123"

	ms := &APIStoreMock{
		GetCodeDeploymentFunc: func(_ context.Context, id, _ string) (*domain.CodeDeployment, error) {
			return &domain.CodeDeployment{
				ID:        id,
				JobID:     "other-job",
				ProjectID: projectID,
				Status:    domain.DeploymentStatusReady,
			}, nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	path := "/v1/jobs/wrong-job/deployments/deploy_1/logs"
	req := authedProjectRequest(http.MethodGet, path, "", projectID)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleDeploymentLogs_StreamingBuildingDeployment(t *testing.T) {
	projectID := "proj_123"
	jobID := "job_abc"
	deploymentID := "deploy_stream_1"

	// Channel to deliver log chunks from the mock subscription.
	logCh := make(chan []byte, 3)
	logCh <- []byte(`{"chunk":"line 1\n"}`)
	logCh <- []byte(`{"chunk":"line 2\n"}`)
	logCh <- []byte(`{"done":true}`)
	close(logCh)

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
	pub := &mockPublisher{
		subscribeFn: func(_ context.Context, channel string) (*pubsub.Subscription, error) {
			if channel != build.BuildLogChannel(deploymentID) {
				return nil, fmt.Errorf("unexpected channel %q", channel)
			}
			return pubsub.NewSubscription(logCh, func() {}), nil
		},
	}
	srv := newTestServer(t, ms, nil, pub)

	path := fmt.Sprintf("/v1/jobs/%s/deployments/%s/logs?stream=true", jobID, deploymentID)
	req := authedProjectRequest(http.MethodGet, path, "", projectID)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	ct := w.Header().Get("Content-Type")
	if ct != "text/event-stream" {
		t.Errorf("expected text/event-stream, got %q", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, "line 1") {
		t.Errorf("expected log chunks in SSE body, got:\n%s", body)
	}
}

// TestHandleListAdminOrgDeployments_RejectsNoSecret verifies that calling the
// admin endpoint without the internal secret returns 401.
func TestHandleListAdminOrgDeployments_RejectsNoSecret(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/internal/admin/orgs/org-123/deployments", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

// TestHandleListAdminOrgDeployments_ReturnsPaginatedList verifies that the
// handler returns a paginated list of deployments for the given org.
func TestHandleListAdminOrgDeployments_ReturnsPaginatedList(t *testing.T) {
	t.Parallel()
	orgID := "org-admin-list"
	now := time.Now().UTC()
	deployments := []domain.CodeDeployment{
		{ID: "dep-1", ProjectID: "proj-1", JobID: "job-1", Status: domain.DeploymentStatusReady, CreatedAt: now.Add(-time.Second)},
		{ID: "dep-2", ProjectID: "proj-2", JobID: "job-2", Status: domain.DeploymentStatusPending, CreatedAt: now},
	}

	ms := &APIStoreMock{
		ListCodeDeploymentsByOrgFunc: func(_ context.Context, gotOrgID string, limit int, _ *time.Time) ([]domain.CodeDeployment, error) {
			if gotOrgID != orgID {
				return nil, fmt.Errorf("unexpected orgID %q", gotOrgID)
			}
			return deployments, nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	req := authedRequest(http.MethodGet, "/internal/admin/orgs/"+orgID+"/deployments", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp PaginatedResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	data, err := json.Marshal(resp.Data)
	if err != nil {
		t.Fatalf("marshal data: %v", err)
	}
	var result []domain.CodeDeployment
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal deployments: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 deployments, got %d", len(result))
	}
}

// --- Adversarial confirm tests (hash / size verification).

// TestHandleConfirmCodeDeployment_HashMismatch_ConfirmNotCalled verifies that
// the store's ConfirmCodeDeployment is never invoked when hash verification fails,
// so the deployment remains in pending status.
func TestHandleConfirmCodeDeployment_HashMismatch_ConfirmNotCalled(t *testing.T) {
	t.Parallel()
	const projectID = "proj-nc"
	const jobID = "job-nc"
	const deploymentID = "deploy-nc"

	confirmCalled := false
	d := &domain.CodeDeployment{
		ID:              deploymentID,
		JobID:           jobID,
		ProjectID:       projectID,
		Status:          domain.DeploymentStatusPending,
		SourceURI:       "projects/proj-nc/jobs/job-nc/deploys/deploy-nc.tar.gz",
		SourceHash:      strings.Repeat("f", 64), // wrong
		SourceSizeBytes: testTarballSize,
	}
	ms := &APIStoreMock{
		GetCodeDeploymentFunc: func(_ context.Context, _, _ string) (*domain.CodeDeployment, error) {
			return d, nil
		},
		ConfirmCodeDeploymentFunc: func(_ context.Context, _ string) error {
			confirmCalled = true
			return nil
		},
	}
	srv := newTestServerWithObjectStore(t, ms, &mockObjectStore{})

	body := fmt.Sprintf(`{"project_id": %q}`, projectID)
	path := fmt.Sprintf("/v1/jobs/%s/deployments/%s/confirm", jobID, deploymentID)
	req := authedProjectRequest(http.MethodPost, path, body, projectID)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", w.Code)
	}
	if confirmCalled {
		t.Error("ConfirmCodeDeployment must not be called when hash verification fails")
	}
}

// TestHandleListAdminOrgDeployments_CrossTenantRejected verifies that a
// caller cannot retrieve deployments for an org they do not own when using a
// project API key (the endpoint must be internal-secret-only).
func TestHandleListAdminOrgDeployments_WrongSecretRejected(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/internal/admin/orgs/org-123/deployments", nil)
	req.Header.Set("X-Internal-Secret", "wrong-secret-value")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for wrong secret, got %d: %s", w.Code, w.Body.String())
	}
}
