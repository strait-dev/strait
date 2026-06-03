package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/billing"
	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/store"
)

type mockWorkflowTrigger struct {
	triggerWorkflowFn   func(ctx context.Context, workflowID, projectID string, payload json.RawMessage, triggeredBy string, stepOverrides []domain.StepOverride) (*domain.WorkflowRun, error)
	retryWorkflowRunFn  func(ctx context.Context, originalRunID string) (*domain.WorkflowRun, error)
	approveStepFn       func(ctx context.Context, workflowRunID, stepRef, approver string) error
	skipStepFn          func(ctx context.Context, workflowRunID, stepRef, reason string) error
	forceCompleteStepFn func(ctx context.Context, workflowRunID, stepRef string, result json.RawMessage) error
	resumeWorkflowFn    func(ctx context.Context, workflowRunID string) error
	onJobRunTerminal    func(ctx context.Context, run *domain.JobRun) error
	onEventReceivedFn   func(ctx context.Context, trigger *domain.EventTrigger) error
	onStepFailedFn      func(ctx context.Context, workflowRunID string, stepRunID string)
}

func (m *mockWorkflowTrigger) TriggerWorkflow(ctx context.Context, workflowID, projectID string, payload json.RawMessage, triggeredBy string, stepOverrides []domain.StepOverride, extraTags map[string]string) (*domain.WorkflowRun, error) {
	if m.triggerWorkflowFn != nil {
		return m.triggerWorkflowFn(ctx, workflowID, projectID, payload, triggeredBy, stepOverrides)
	}
	return nil, nil
}

func (m *mockWorkflowTrigger) ApproveStep(ctx context.Context, workflowRunID, stepRef, approver string) error {
	if m.approveStepFn != nil {
		return m.approveStepFn(ctx, workflowRunID, stepRef, approver)
	}
	return nil
}

func (m *mockWorkflowTrigger) SkipStep(ctx context.Context, workflowRunID, stepRef, reason, actor string) error {
	if m.skipStepFn != nil {
		return m.skipStepFn(ctx, workflowRunID, stepRef, reason)
	}
	return nil
}

func (m *mockWorkflowTrigger) ForceCompleteStep(ctx context.Context, workflowRunID, stepRef string, result json.RawMessage) error {
	if m.forceCompleteStepFn != nil {
		return m.forceCompleteStepFn(ctx, workflowRunID, stepRef, result)
	}
	return nil
}

func (m *mockWorkflowTrigger) ResumeWorkflowRun(ctx context.Context, workflowRunID string) error {
	if m.resumeWorkflowFn != nil {
		return m.resumeWorkflowFn(ctx, workflowRunID)
	}
	return nil
}

func (m *mockWorkflowTrigger) RetryWorkflowRun(ctx context.Context, originalRunID string) (*domain.WorkflowRun, error) {
	if m.retryWorkflowRunFn != nil {
		return m.retryWorkflowRunFn(ctx, originalRunID)
	}
	return nil, nil
}

func (m *mockWorkflowTrigger) OnJobRunTerminal(ctx context.Context, run *domain.JobRun) error {
	if m.onJobRunTerminal != nil {
		return m.onJobRunTerminal(ctx, run)
	}
	return nil
}

func (m *mockWorkflowTrigger) OnEventReceived(ctx context.Context, trigger *domain.EventTrigger) error {
	if m.onEventReceivedFn != nil {
		return m.onEventReceivedFn(ctx, trigger)
	}
	return nil
}

func (m *mockWorkflowTrigger) OnStepFailed(ctx context.Context, workflowRunID string, stepRunID string) {
	if m.onStepFailedFn != nil {
		m.onStepFailedFn(ctx, workflowRunID, stepRunID)
	}
}

func newWorkflowTestServer(t *testing.T, s APIStore, q *mockQueue, pub *mockPublisher, trigger WorkflowTrigger) *Server {
	t.Helper()
	cfg := &config.Config{
		InternalSecret:      "test-secret-value",
		MaxBulkTriggerItems: 500,
		JWTSigningKey:       testJWTSigningKey,
	}
	srv := NewServer(ServerDeps{
		Config:         cfg,
		Store:          s,
		Queue:          q,
		PubSub:         pub,
		WorkflowEngine: trigger,
	})
	t.Cleanup(srv.Close)
	return srv
}

func newWorkflowTestServerWithCallback(t *testing.T, s APIStore, q *mockQueue, pub *mockPublisher, wfCallback WorkflowCallback, trigger WorkflowTrigger) *Server {
	t.Helper()
	cfg := &config.Config{
		InternalSecret:      "test-secret-value",
		MaxBulkTriggerItems: 500,
		JWTSigningKey:       testJWTSigningKey,
	}
	srv := NewServer(ServerDeps{
		Config:           cfg,
		Store:            s,
		Queue:            q,
		PubSub:           pub,
		WorkflowCallback: wfCallback,
		WorkflowEngine:   trigger,
	})
	t.Cleanup(srv.Close)
	return srv
}

func TestHandleCreateWorkflow_SuccessWithSteps(t *testing.T) {
	t.Parallel()
	createStepCalls := 0
	expectedResourceClass := map[string]string{"s1": "small", "s2": "large"}
	ms := &APIStoreMock{
		CreateWorkflowFunc: func(_ context.Context, wf *domain.Workflow) error {
			wf.ID = "wf-1"
			return nil
		},
		CreateWorkflowStepFunc: func(_ context.Context, step *domain.WorkflowStep) error {
			createStepCalls++
			if step.WorkflowID != "wf-1" {
				t.Fatalf("step workflow_id = %q, want wf-1", step.WorkflowID)
			}
			if want := expectedResourceClass[step.StepRef]; step.ResourceClass != want {
				t.Fatalf("step %s resource_class = %q, want %q", step.StepRef, step.ResourceClass, want)
			}
			return nil
		},
	}

	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
	w := httptest.NewRecorder()
	body := `{"project_id":"proj-1","name":"wf","slug":"wf","steps":[{"job_id":"job-1","step_ref":"s1","resource_class":"small"},{"job_id":"job-2","step_ref":"s2","depends_on":["s1"],"resource_class":"large"}]}`
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows", body))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if createStepCalls != 2 {
		t.Fatalf("create step calls = %d, want 2", createStepCalls)
	}
}

func TestHandleCreateWorkflow_MissingFields(t *testing.T) {
	t.Parallel()
	srv := newWorkflowTestServer(t, &APIStoreMock{}, &mockQueue{}, nil, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows", `{}`))

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", w.Code)
	}
}

func TestHandleCreateWorkflow_InvalidStep(t *testing.T) {
	t.Parallel()
	srv := newWorkflowTestServer(t, &APIStoreMock{}, &mockQueue{}, nil, nil)
	w := httptest.NewRecorder()
	body := `{"project_id":"proj-1","name":"wf","slug":"wf","steps":[{"job_id":"job-1","step_ref":"s1","depends_on":[""]}]}`
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows", body))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleCreateWorkflow_RejectsUnknownStepType(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		CreateWorkflowFunc: func(_ context.Context, _ *domain.Workflow) error {
			t.Fatal("CreateWorkflow must not be called for an unknown step_type")
			return nil
		},
	}
	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
	w := httptest.NewRecorder()
	body := `{"project_id":"proj-1","name":"wf","slug":"wf","steps":[{"job_id":"job-1","step_ref":"s1","step_type":"approval_bypass"}]}`
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows", body))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "invalid step_type") {
		t.Fatalf("expected invalid step_type validation error, got: %s", w.Body.String())
	}
}

func TestHandleCreateWorkflow_CrossProjectBlockedBeforeCreate(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateWorkflowFunc: func(_ context.Context, _ *domain.Workflow) error {
			t.Fatal("CreateWorkflow must not be called for a cross-project body")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")

	_, err := srv.handleCreateWorkflow(ctx, &CreateWorkflowInput{Body: createWorkflowRequest{
		ProjectID: "proj-2",
		Name:      "wf",
		Slug:      "wf",
	}})
	if !isHumaStatusError(err, http.StatusNotFound) {
		t.Fatalf("expected 404, got %v", err)
	}
}

func TestHandleCreateWorkflow_RejectsCrossProjectJobStep(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-2"}, nil
		},
		CreateWorkflowFunc: func(_ context.Context, _ *domain.Workflow) error {
			t.Fatal("CreateWorkflow must not be called for a cross-project job step")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")

	_, err := srv.handleCreateWorkflow(ctx, &CreateWorkflowInput{Body: createWorkflowRequest{
		ProjectID: "proj-1",
		Name:      "wf",
		Slug:      "wf",
		Steps:     []workflowStepRequest{{StepRef: "run-other", JobID: "job-other"}},
	}})
	if !isHumaStatusError(err, http.StatusBadRequest) {
		t.Fatalf("expected 400, got %v", err)
	}
}

func TestHandleCreateWorkflow_InvalidResourceClass(t *testing.T) {
	t.Parallel()
	srv := newWorkflowTestServer(t, &APIStoreMock{}, &mockQueue{}, nil, nil)
	w := httptest.NewRecorder()
	body := `{"project_id":"proj-1","name":"wf","slug":"wf","steps":[{"job_id":"job-1","step_ref":"s1","resource_class":"xlarge"}]}`
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows", body))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "resource_class") {
		t.Fatalf("expected resource_class validation error, got: %s", w.Body.String())
	}
}

func TestHandleCreateWorkflow_PolicyViolation(t *testing.T) {
	t.Parallel()
	createCalled := false
	ms := &APIStoreMock{
		CreateWorkflowFunc: func(_ context.Context, _ *domain.Workflow) error {
			createCalled = true
			return nil
		},
		GetWorkflowPolicyByProjectFunc: func(_ context.Context, projectID string) (*domain.WorkflowPolicy, error) {
			if projectID != "proj-1" {
				t.Fatalf("projectID = %q, want proj-1", projectID)
			}
			return &domain.WorkflowPolicy{ProjectID: projectID, MaxFanOut: 1}, nil
		},
	}

	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
	w := httptest.NewRecorder()
	body := `{"project_id":"proj-1","name":"wf","slug":"wf","steps":[{"job_id":"job-1","step_ref":"root"},{"job_id":"job-2","step_ref":"child-a","depends_on":["root"]},{"job_id":"job-3","step_ref":"child-b","depends_on":["root"]}]}`
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows", body))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "max_fan_out") {
		t.Fatalf("expected max_fan_out violation, got: %s", w.Body.String())
	}
	if createCalled {
		t.Fatal("expected CreateWorkflow not to be called when policy validation fails")
	}
}

func TestHandleGetWorkflow_FoundWithSteps(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
			return &domain.Workflow{ID: id, Name: "wf", ProjectID: "proj-1"}, nil
		},
		ListStepsByWorkflowFunc: func(_ context.Context, workflowID string) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{{ID: "step-1", WorkflowID: workflowID, StepRef: "s1"}}, nil
		},
	}

	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflows/wf-1", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestHandleGetWorkflow_NotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetWorkflowFunc: func(_ context.Context, _ string) (*domain.Workflow, error) {
			return nil, store.ErrWorkflowNotFound
		},
	}
	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflows/missing", ""))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandleListWorkflows_Success(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		ListWorkflowsFunc: func(_ context.Context, projectID string, _ int, _ *time.Time) ([]domain.Workflow, error) {
			return []domain.Workflow{{ID: "wf-1", ProjectID: projectID}, {ID: "wf-2", ProjectID: projectID}}, nil
		},
	}

	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/workflows", "", "proj-1"))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestHandleListWorkflows_MissingProjectID(t *testing.T) {
	t.Parallel()
	srv := newWorkflowTestServer(t, &APIStoreMock{}, &mockQueue{}, nil, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflows", ""))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleCreateWorkflow_ScheduledWorkflowEnforcesScheduleLimit(t *testing.T) {
	t.Parallel()
	createCalled := false
	ms := &APIStoreMock{
		CountCronJobsByOrgFunc: func(_ context.Context, orgID string) (int, error) {
			if orgID != "org-1" {
				t.Fatalf("orgID = %q, want org-1", orgID)
			}
			return 5, nil
		},
		CreateWorkflowFunc: func(_ context.Context, _ *domain.Workflow) error {
			createCalled = true
			return nil
		},
	}

	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
	srv.edition = domain.EditionCloud
	srv.billingEnforcer = &mockBillingEnforcer{projectOrgMap: map[string]string{"proj-1": "org-1"}}
	w := httptest.NewRecorder()
	body := `{"project_id":"proj-1","name":"wf","slug":"wf","cron":"*/5 * * * *","steps":[{"job_id":"job-1","step_ref":"s1"}]}`
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows", body))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if createCalled {
		t.Fatal("expected CreateWorkflow not to be called after schedule limit failure")
	}
}

func TestHandleCloneWorkflow_ScheduledWorkflowEnforcesScheduleLimit(t *testing.T) {
	t.Parallel()
	createCalled := false
	ms := &APIStoreMock{
		GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
			return &domain.Workflow{ID: id, ProjectID: "proj-1", Name: "wf", Slug: "wf", Enabled: true, Cron: "*/5 * * * *"}, nil
		},
		ListStepsByWorkflowFunc: func(context.Context, string) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{{ID: "s1", StepRef: "s1", JobID: "job-1"}}, nil
		},
		CountCronJobsByOrgFunc: func(_ context.Context, orgID string) (int, error) {
			if orgID != "org-1" {
				t.Fatalf("orgID = %q, want org-1", orgID)
			}
			return 5, nil
		},
		CreateWorkflowFunc: func(context.Context, *domain.Workflow) error {
			createCalled = true
			return nil
		},
	}

	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
	srv.edition = domain.EditionCloud
	srv.billingEnforcer = &mockBillingEnforcer{projectOrgMap: map[string]string{"proj-1": "org-1"}}
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows/wf-1/clone", `{}`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if createCalled {
		t.Fatal("expected CreateWorkflow not to be called after schedule limit failure")
	}
}

func TestHandleUpdateWorkflow_SuccessWithStepReplacement(t *testing.T) {
	t.Parallel()
	deleteCalled := false
	createStepCalls := 0
	ms := &APIStoreMock{
		GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
			return &domain.Workflow{ID: id, Name: "old", Slug: "old", Enabled: true}, nil
		},
		UpdateWorkflowFunc: func(_ context.Context, wf *domain.Workflow) error {
			if wf.Name != "new" || wf.Slug != "new-slug" || wf.Enabled {
				t.Fatalf("unexpected updated workflow: %+v", wf)
			}
			return nil
		},
		DeleteStepsByWorkflowFunc: func(_ context.Context, workflowID string) error {
			deleteCalled = true
			if workflowID != "wf-1" {
				t.Fatalf("workflow_id = %q, want wf-1", workflowID)
			}
			return nil
		},
		CreateWorkflowStepFunc: func(_ context.Context, step *domain.WorkflowStep) error {
			createStepCalls++
			if step.ResourceClass != "medium" {
				t.Fatalf("step resource_class = %q, want medium", step.ResourceClass)
			}
			return nil
		},
		ListStepsByWorkflowFunc: func(_ context.Context, workflowID string) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{{ID: "s1", WorkflowID: workflowID, StepRef: "s1"}}, nil
		},
	}

	body := `{"name":"new","slug":"new-slug","enabled":false,"steps":[{"job_id":"job-1","step_ref":"s1","resource_class":"medium"}]}`
	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/workflows/wf-1", body))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !deleteCalled {
		t.Fatal("expected DeleteStepsByWorkflow to be called")
	}
	if createStepCalls != 1 {
		t.Fatalf("create step calls = %d, want 1", createStepCalls)
	}
}

func TestHandleUpdateWorkflow_RejectsUnknownStepType(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
			return &domain.Workflow{ID: id, ProjectID: "proj-1", Name: "wf", Slug: "wf"}, nil
		},
		UpdateWorkflowFunc: func(_ context.Context, _ *domain.Workflow) error {
			t.Fatal("UpdateWorkflow must not be called for an unknown step_type")
			return nil
		},
		DeleteStepsByWorkflowFunc: func(_ context.Context, _ string) error {
			t.Fatal("DeleteStepsByWorkflow must not be called for an unknown step_type")
			return nil
		},
		CreateWorkflowStepFunc: func(_ context.Context, _ *domain.WorkflowStep) error {
			t.Fatal("CreateWorkflowStep must not be called for an unknown step_type")
			return nil
		},
	}
	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
	w := httptest.NewRecorder()
	body := `{"steps":[{"job_id":"job-1","step_ref":"s1","step_type":"approval_bypass"}]}`
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPatch, "/v1/workflows/wf-1", body, "proj-1"))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "invalid step_type") {
		t.Fatalf("expected invalid step_type validation error, got: %s", w.Body.String())
	}
}

func TestHandleUpdateWorkflow_AddingCronEnforcesScheduleLimit(t *testing.T) {
	t.Parallel()
	updateCalled := false
	ms := &APIStoreMock{
		GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
			return &domain.Workflow{ID: id, ProjectID: "proj-1", Name: "wf", Slug: "wf", Enabled: true}, nil
		},
		CountCronJobsByOrgFunc: func(_ context.Context, orgID string) (int, error) {
			if orgID != "org-1" {
				t.Fatalf("orgID = %q, want org-1", orgID)
			}
			return 5, nil
		},
		UpdateWorkflowFunc: func(_ context.Context, _ *domain.Workflow) error {
			updateCalled = true
			return nil
		},
	}

	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
	srv.edition = domain.EditionCloud
	srv.billingEnforcer = &mockBillingEnforcer{projectOrgMap: map[string]string{"proj-1": "org-1"}}
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/workflows/wf-1", `{"cron":"*/5 * * * *"}`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if updateCalled {
		t.Fatal("expected UpdateWorkflow not to be called after schedule limit failure")
	}
}

func TestHandleUpdateWorkflow_PolicyViolation(t *testing.T) {
	t.Parallel()
	updateCalled := false
	deleteCalled := false
	createStepCalled := false
	ms := &APIStoreMock{
		GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
			return &domain.Workflow{ID: id, Name: "old", Slug: "old", ProjectID: "proj-1", Enabled: true}, nil
		},
		GetWorkflowPolicyByProjectFunc: func(_ context.Context, projectID string) (*domain.WorkflowPolicy, error) {
			return &domain.WorkflowPolicy{ProjectID: projectID, MaxFanOut: 1}, nil
		},
		ListStepsByWorkflowFunc: func(_ context.Context, workflowID string) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{
				{WorkflowID: workflowID, StepRef: "root"},
				{WorkflowID: workflowID, StepRef: "child-a", DependsOn: []string{"root"}},
				{WorkflowID: workflowID, StepRef: "child-b", DependsOn: []string{"root"}},
			}, nil
		},
		UpdateWorkflowFunc: func(_ context.Context, _ *domain.Workflow) error {
			updateCalled = true
			return nil
		},
		DeleteStepsByWorkflowFunc: func(_ context.Context, _ string) error {
			deleteCalled = true
			return nil
		},
		CreateWorkflowStepFunc: func(_ context.Context, _ *domain.WorkflowStep) error {
			createStepCalled = true
			return nil
		},
	}

	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
	w := httptest.NewRecorder()
	body := `{"name":"new","steps":[{"job_id":"job-1","step_ref":"root"},{"job_id":"job-2","step_ref":"child-a","depends_on":["root"]},{"job_id":"job-3","step_ref":"child-b","depends_on":["root"]}]}`
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/workflows/wf-1", body))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "max_fan_out") {
		t.Fatalf("expected max_fan_out violation, got: %s", w.Body.String())
	}
	if updateCalled {
		t.Fatal("expected UpdateWorkflow not to be called when policy validation fails")
	}
	if deleteCalled || createStepCalled {
		t.Fatal("expected step replacement not to run when policy validation fails")
	}
}

func TestHandleUpdateWorkflow_InvalidResourceClass(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
			return &domain.Workflow{ID: id, Name: "old", Slug: "old", Enabled: true}, nil
		},
	}
	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
	w := httptest.NewRecorder()
	body := `{"steps":[{"job_id":"job-1","step_ref":"s1","resource_class":"xlarge"}]}`
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/workflows/wf-1", body))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "resource_class") {
		t.Fatalf("expected resource_class validation error, got: %s", w.Body.String())
	}
}

func TestHandleUpdateWorkflow_NotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetWorkflowFunc: func(_ context.Context, _ string) (*domain.Workflow, error) {
			return nil, store.ErrWorkflowNotFound
		},
	}

	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/workflows/missing", `{"name":"new"}`))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandleUpdateWorkflow_ActiveRunsReportedWithoutBreakingFlag(t *testing.T) {
	t.Parallel()
	var auditCalled bool
	ms := &APIStoreMock{
		GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
			return &domain.Workflow{ID: id, Name: "old", Slug: "old", VersionID: "v-old", Version: 2}, nil
		},
		UpdateWorkflowFunc: func(_ context.Context, _ *domain.Workflow) error {
			return nil
		},
		ListStepsByWorkflowFunc: func(_ context.Context, _ string) ([]domain.WorkflowStep, error) {
			return nil, nil
		},
		CreateWorkflowVersionSnapshotFunc: func(_ context.Context, _ string, _ int) error {
			return nil
		},
		CountActiveWorkflowRunsByVersionFunc: func(_ context.Context, workflowID, versionID string) (int, error) {
			if workflowID != "wf-1" || versionID != "v-old" {
				t.Fatalf("unexpected args: workflowID=%q versionID=%q", workflowID, versionID)
			}
			return 5, nil
		},
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			auditCalled = true
			return nil
		},
	}

	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/workflows/wf-1", `{"name":"updated"}`))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	count, ok := resp["active_runs_on_previous_version"]
	if !ok {
		t.Fatal("expected active_runs_on_previous_version in response")
	}
	if int(count.(float64)) != 5 {
		t.Fatalf("active_runs_on_previous_version = %v, want 5", count)
	}
	if resp["previous_version_id"] != "v-old" {
		t.Fatalf("previous_version_id = %v, want v-old", resp["previous_version_id"])
	}
	// Workflow updates always emit a generic workflow.updated audit event,
	// even without a breaking_change flag. The breaking-change path still
	// emits workflow.updated_breaking; this asserts the default path emits
	// the generic action.
	if !auditCalled {
		t.Fatal("expected workflow.updated audit event even without breaking_change")
	}
}

func TestHandleUpdateWorkflow_BreakingChangeEmitsAudit(t *testing.T) {
	t.Parallel()
	var auditCalled bool
	var capturedAction string
	ms := &APIStoreMock{
		GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
			return &domain.Workflow{ID: id, Name: "old", Slug: "old", VersionID: "v-old", Version: 2}, nil
		},
		UpdateWorkflowFunc: func(_ context.Context, _ *domain.Workflow) error {
			return nil
		},
		ListStepsByWorkflowFunc: func(_ context.Context, _ string) ([]domain.WorkflowStep, error) {
			return nil, nil
		},
		CreateWorkflowVersionSnapshotFunc: func(_ context.Context, _ string, _ int) error {
			return nil
		},
		CountActiveWorkflowRunsByVersionFunc: func(_ context.Context, _, _ string) (int, error) {
			return 5, nil
		},
		CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
			auditCalled = true
			capturedAction = ev.Action
			return nil
		},
	}

	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/workflows/wf-1", `{"name":"updated","breaking_change":true}`))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if int(resp["active_runs_on_previous_version"].(float64)) != 5 {
		t.Fatalf("active_runs_on_previous_version = %v, want 5", resp["active_runs_on_previous_version"])
	}
	if !auditCalled {
		t.Fatal("expected audit event to be emitted for breaking_change=true")
	}
	if capturedAction != "workflow.updated_breaking" {
		t.Fatalf("audit action = %q, want workflow.updated_breaking", capturedAction)
	}
}

func TestHandleUpdateWorkflow_BreakingChangeFalseEmitsGenericAudit(t *testing.T) {
	t.Parallel()
	var capturedAction string
	ms := &APIStoreMock{
		GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
			return &domain.Workflow{ID: id, Name: "old", Slug: "old", VersionID: "v-old", Version: 2}, nil
		},
		UpdateWorkflowFunc: func(_ context.Context, _ *domain.Workflow) error {
			return nil
		},
		ListStepsByWorkflowFunc: func(_ context.Context, _ string) ([]domain.WorkflowStep, error) {
			return nil, nil
		},
		CreateWorkflowVersionSnapshotFunc: func(_ context.Context, _ string, _ int) error {
			return nil
		},
		CountActiveWorkflowRunsByVersionFunc: func(_ context.Context, _, _ string) (int, error) {
			return 5, nil
		},
		CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
			capturedAction = ev.Action
			return nil
		},
	}

	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/workflows/wf-1", `{"name":"updated","breaking_change":false}`))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if int(resp["active_runs_on_previous_version"].(float64)) != 5 {
		t.Fatalf("active_runs_on_previous_version = %v, want 5", resp["active_runs_on_previous_version"])
	}
	// breaking_change=false still emits workflow.updated (the generic
	// action). The breaking variant is only emitted when breaking_change=true
	// AND there are active runs on the previous version.
	if capturedAction != "workflow.updated" {
		t.Fatalf("audit action = %q, want workflow.updated", capturedAction)
	}
}

func TestHandleUpdateWorkflow_NoActiveRunsEmitsGenericAudit(t *testing.T) {
	t.Parallel()
	var capturedAction string
	ms := &APIStoreMock{
		GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
			return &domain.Workflow{ID: id, Name: "old", Slug: "old", VersionID: "v-old", Version: 2}, nil
		},
		UpdateWorkflowFunc: func(_ context.Context, _ *domain.Workflow) error {
			return nil
		},
		ListStepsByWorkflowFunc: func(_ context.Context, _ string) ([]domain.WorkflowStep, error) {
			return nil, nil
		},
		CreateWorkflowVersionSnapshotFunc: func(_ context.Context, _ string, _ int) error {
			return nil
		},
		CountActiveWorkflowRunsByVersionFunc: func(_ context.Context, _, _ string) (int, error) {
			return 0, nil
		},
		CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
			capturedAction = ev.Action
			return nil
		},
	}

	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/workflows/wf-1", `{"name":"updated","breaking_change":true}`))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if _, ok := resp["active_runs_on_previous_version"]; ok {
		t.Fatal("expected no active_runs_on_previous_version when count is 0")
	}
	// Even with breaking_change=true, when there are zero active runs on the
	// previous version the handler falls back to the generic workflow.updated
	// action rather than workflow.updated_breaking.
	if capturedAction != "workflow.updated" {
		t.Fatalf("audit action = %q, want workflow.updated", capturedAction)
	}
}

func TestHandleUpdateWorkflow_FirstVersionSkipsActiveCheck(t *testing.T) {
	t.Parallel()
	var countCalled bool
	ms := &APIStoreMock{
		GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
			return &domain.Workflow{ID: id, Name: "old", Slug: "old", VersionID: "", Version: 0}, nil
		},
		UpdateWorkflowFunc: func(_ context.Context, _ *domain.Workflow) error {
			return nil
		},
		ListStepsByWorkflowFunc: func(_ context.Context, _ string) ([]domain.WorkflowStep, error) {
			return nil, nil
		},
		CreateWorkflowVersionSnapshotFunc: func(_ context.Context, _ string, _ int) error {
			return nil
		},
		CountActiveWorkflowRunsByVersionFunc: func(_ context.Context, _, _ string) (int, error) {
			countCalled = true
			return 0, nil
		},
	}

	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/workflows/wf-1", `{"name":"updated"}`))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if _, ok := resp["active_runs_on_previous_version"]; ok {
		t.Fatal("expected no active_runs_on_previous_version for first version")
	}
	if countCalled {
		t.Fatal("expected CountActiveWorkflowRunsByVersion not to be called for first version")
	}
}

func TestHandleUpdateWorkflow_CountActiveRunsError(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
			return &domain.Workflow{ID: id, Name: "old", Slug: "old", VersionID: "v-old", Version: 2}, nil
		},
		UpdateWorkflowFunc: func(_ context.Context, _ *domain.Workflow) error {
			return nil
		},
		ListStepsByWorkflowFunc: func(_ context.Context, _ string) ([]domain.WorkflowStep, error) {
			return nil, nil
		},
		CreateWorkflowVersionSnapshotFunc: func(_ context.Context, _ string, _ int) error {
			return nil
		},
		CountActiveWorkflowRunsByVersionFunc: func(_ context.Context, _, _ string) (int, error) {
			return 0, fmt.Errorf("db connection lost")
		},
	}

	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/workflows/wf-1", `{"name":"updated","breaking_change":true}`))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (graceful degradation), got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if _, ok := resp["active_runs_on_previous_version"]; ok {
		t.Fatal("expected no active_runs_on_previous_version when count query fails")
	}
}

func TestHandleGetActiveVersions_ReturnsVersionBreakdown(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		ListActiveWorkflowVersionsFunc: func(_ context.Context, workflowID string) ([]store.ActiveVersion, error) {
			if workflowID != "wf-1" {
				t.Fatalf("workflowID = %q, want wf-1", workflowID)
			}
			return []store.ActiveVersion{
				{VersionID: "v-2", Version: 2, Pending: 3, Running: 5, Paused: 1, Total: 9},
				{VersionID: "v-1", Version: 1, Pending: 0, Running: 1, Paused: 0, Total: 1},
			}, nil
		},
	}

	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflows/wf-1/active-versions", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp["workflow_id"] != "wf-1" {
		t.Fatalf("workflow_id = %v, want wf-1", resp["workflow_id"])
	}
	versions, ok := resp["versions"].([]any)
	if !ok || len(versions) != 2 {
		t.Fatalf("expected 2 versions, got %v", resp["versions"])
	}
	first := versions[0].(map[string]any)
	if first["version_id"] != "v-2" {
		t.Fatalf("first version_id = %v, want v-2", first["version_id"])
	}
	if int(first["total"].(float64)) != 9 {
		t.Fatalf("first total = %v, want 9", first["total"])
	}
	if int(first["running"].(float64)) != 5 {
		t.Fatalf("first running = %v, want 5", first["running"])
	}
}

func TestHandleGetActiveVersions_Empty(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		ListActiveWorkflowVersionsFunc: func(_ context.Context, _ string) ([]store.ActiveVersion, error) {
			return nil, nil
		},
	}

	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflows/wf-1/active-versions", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp["workflow_id"] != "wf-1" {
		t.Fatalf("workflow_id = %v, want wf-1", resp["workflow_id"])
	}
	versions, ok := resp["versions"].([]any)
	if !ok {
		t.Fatalf("expected versions to be an array, got %T", resp["versions"])
	}
	if len(versions) != 0 {
		t.Fatalf("expected 0 versions, got %d", len(versions))
	}
}

func TestHandleGetActiveVersions_StoreError(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		ListActiveWorkflowVersionsFunc: func(_ context.Context, _ string) ([]store.ActiveVersion, error) {
			return nil, fmt.Errorf("db error")
		},
	}

	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflows/wf-1/active-versions", ""))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleDeleteWorkflow(t *testing.T) {
	t.Parallel()
	t.Run("success", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: id, ProjectID: "proj-1"}, nil
			},
			CountRunningWorkflowRunsFunc: func(_ context.Context, _ string) (int, error) { return 0, nil },
			DeleteWorkflowFunc:           func(_ context.Context, _ string) error { return nil },
		}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/workflows/wf-1", ""))

		if w.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d", w.Code)
		}
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: id, ProjectID: "proj-1"}, nil
			},
			CountRunningWorkflowRunsFunc: func(_ context.Context, _ string) (int, error) { return 0, nil },
			DeleteWorkflowFunc:           func(_ context.Context, _ string) error { return errors.New("delete failed") },
		}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/workflows/wf-1", ""))

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", w.Code)
		}
	})

	t.Run("active_runs_returns_409", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: id, ProjectID: "proj-1"}, nil
			},
			CountRunningWorkflowRunsFunc: func(_ context.Context, _ string) (int, error) { return 3, nil },
		}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/workflows/wf-1", ""))

		if w.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
		}
	})
}

func TestHandleTriggerWorkflow(t *testing.T) {
	t.Parallel()
	t.Run("success", func(t *testing.T) {
		t.Parallel()
		labelsSaved := false
		published := map[string]int{}
		trigger := &mockWorkflowTrigger{
			triggerWorkflowFn: func(_ context.Context, workflowID, projectID string, _ json.RawMessage, triggeredBy string, _ []domain.StepOverride) (*domain.WorkflowRun, error) {
				if workflowID != "wf-1" || projectID != "proj-1" || triggeredBy != "manual" {
					t.Fatalf("unexpected trigger args: %s %s %s", workflowID, projectID, triggeredBy)
				}
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: workflowID, ProjectID: projectID, Status: domain.WfStatusRunning}, nil
			},
		}
		ms := &APIStoreMock{
			GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: id, Enabled: true}, nil
			},
			CreateWorkflowRunLabelsFunc: func(_ context.Context, workflowRunID string, labels map[string]string) error {
				if workflowRunID != "wr-1" {
					t.Fatalf("workflowRunID = %q, want wr-1", workflowRunID)
				}
				if labels["env"] != "test" {
					t.Fatalf("labels env = %q, want test", labels["env"])
				}
				labelsSaved = true
				return nil
			},
		}

		pub := &mockPublisher{publishFn: func(_ context.Context, channel string, _ []byte) error {
			published[channel]++
			return nil
		}}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, pub, trigger)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows/wf-1/trigger", `{"project_id":"proj-1","labels":{"env":"test"}}`))

		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
		}
		if !labelsSaved {
			t.Fatal("expected workflow run labels to be persisted")
		}
		if published["workflow-run:wr-1"] != 1 {
			t.Fatalf("expected workflow-run hook publish once, got %d", published["workflow-run:wr-1"])
		}
		if published["workflow:wf-1:runs"] != 1 {
			t.Fatalf("expected workflow stream publish once, got %d", published["workflow:wf-1:runs"])
		}
	})

	t.Run("workflow not found", func(t *testing.T) {
		t.Parallel()
		trigger := &mockWorkflowTrigger{
			triggerWorkflowFn: func(_ context.Context, _, _ string, _ json.RawMessage, _ string, _ []domain.StepOverride) (*domain.WorkflowRun, error) {
				return nil, store.ErrWorkflowNotFound
			},
		}
		ms := &APIStoreMock{
			GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: id, Enabled: true}, nil
			},
		}

		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, trigger)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows/wf-missing/trigger", `{}`))

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})

	t.Run("engine unavailable", func(t *testing.T) {
		t.Parallel()
		srv := newWorkflowTestServer(t, &APIStoreMock{}, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows/wf-1/trigger", `{}`))

		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d", w.Code)
		}
	})

	t.Run("workflow disabled", func(t *testing.T) {
		t.Parallel()
		triggerCalled := false
		trigger := &mockWorkflowTrigger{
			triggerWorkflowFn: func(_ context.Context, _, _ string, _ json.RawMessage, _ string, _ []domain.StepOverride) (*domain.WorkflowRun, error) {
				triggerCalled = true
				return &domain.WorkflowRun{ID: "wr-1"}, nil
			},
		}
		ms := &APIStoreMock{
			GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: id, Enabled: false}, nil
			},
		}

		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, trigger)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows/wf-1/trigger", `{}`))

		if w.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d", w.Code)
		}
		if triggerCalled {
			t.Fatal("expected trigger not to be called for disabled workflow")
		}
	})
	t.Run("policy violation blocks trigger", func(t *testing.T) {
		t.Parallel()
		triggerCalled := false
		trigger := &mockWorkflowTrigger{
			triggerWorkflowFn: func(_ context.Context, _, _ string, _ json.RawMessage, _ string, _ []domain.StepOverride) (*domain.WorkflowRun, error) {
				triggerCalled = true
				return &domain.WorkflowRun{ID: "wr-1"}, nil
			},
		}
		ms := &APIStoreMock{
			GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: id, ProjectID: "proj-1", Enabled: true, Version: 1}, nil
			},
			ListStepsByWorkflowVersionFunc: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{
					{StepRef: "root"},
					{StepRef: "child-a", DependsOn: []string{"root"}},
					{StepRef: "child-b", DependsOn: []string{"root"}},
				}, nil
			},
			GetWorkflowPolicyByProjectFunc: func(_ context.Context, projectID string) (*domain.WorkflowPolicy, error) {
				return &domain.WorkflowPolicy{ProjectID: projectID, MaxFanOut: 1}, nil
			},
		}

		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, trigger)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows/wf-1/trigger", `{"project_id":"proj-1"}`))

		if w.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "max_fan_out") {
			t.Fatalf("expected max_fan_out policy violation, got: %s", w.Body.String())
		}
		if triggerCalled {
			t.Fatal("expected TriggerWorkflow not to be called on policy violation")
		}
	})
}

func TestHandleListWorkflowRuns(t *testing.T) {
	t.Parallel()
	t.Run("success with cursor pagination", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: id, ProjectID: "proj-1"}, nil
			},
			ListWorkflowRunsFunc: func(_ context.Context, workflowID string, limit int, cursor *time.Time) ([]domain.WorkflowRun, error) {
				if workflowID != "wf-1" || limit != 11 { // handler passes limit+1
					t.Fatalf("unexpected args: %s %d", workflowID, limit)
				}
				return []domain.WorkflowRun{{ID: "wr-1"}}, nil
			},
		}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflows/wf-1/runs?limit=10", ""))

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
	})

	t.Run("invalid params", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: id, ProjectID: "proj-1"}, nil
			},
		}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflows/wf-1/runs?limit=0", ""))

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})
}

func TestHandleListWorkflowRunsByProject(t *testing.T) {
	t.Parallel()
	t.Run("success with status filter", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			ListWorkflowRunsByProjectFunc: func(_ context.Context, projectID string, status *domain.WorkflowRunStatus, limit int, _ *time.Time) ([]domain.WorkflowRun, error) {
				if projectID != "proj-1" || limit != 21 { // handler passes limit+1
					t.Fatalf("unexpected args: %s %d", projectID, limit)
				}
				if status == nil || *status != domain.WfStatusRunning {
					t.Fatalf("unexpected status filter: %v", status)
				}
				return []domain.WorkflowRun{{ID: "wr-1", Status: domain.WfStatusRunning}}, nil
			},
		}

		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/workflow-runs?status=running&limit=20", "", "proj-1"))

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
	})

	t.Run("missing project id", func(t *testing.T) {
		t.Parallel()
		srv := newWorkflowTestServer(t, &APIStoreMock{}, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflow-runs", ""))

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("invalid status", func(t *testing.T) {
		t.Parallel()
		called := false
		ms := &APIStoreMock{
			ListWorkflowRunsByProjectFunc: func(_ context.Context, _ string, _ *domain.WorkflowRunStatus, _ int, _ *time.Time) ([]domain.WorkflowRun, error) {
				called = true
				return nil, nil
			},
		}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/workflow-runs?status=invalid-status", "", "proj-1"))

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
		if called {
			t.Fatal("expected ListWorkflowRunsByProject to not be called for invalid status")
		}
	})
}

func TestHandleGetWorkflowRun(t *testing.T) {
	t.Parallel()
	t.Run("found", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: id, Status: domain.WfStatusRunning}, nil
			},
		}

		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflow-runs/wr-1", ""))

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return nil, store.ErrWorkflowRunNotFound
			},
		}

		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflow-runs/wr-missing", ""))

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})
}

func TestHandlePauseWorkflowRun(t *testing.T) {
	t.Parallel()
	t.Run("success", func(t *testing.T) {
		t.Parallel()
		getCalls := 0
		published := map[string]int{}
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				getCalls++
				if getCalls == 1 {
					return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", Status: domain.WfStatusRunning}, nil
				}
				return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", Status: domain.WfStatusPaused}, nil
			},
			UpdateWorkflowRunStatusFunc: func(_ context.Context, _ string, from, to domain.WorkflowRunStatus, _ map[string]any) error {
				if from != domain.WfStatusRunning || to != domain.WfStatusPaused {
					t.Fatalf("unexpected transition %s -> %s", from, to)
				}
				return nil
			},
		}

		pub := &mockPublisher{publishFn: func(_ context.Context, channel string, _ []byte) error {
			published[channel]++
			return nil
		}}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, pub, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/wr-1/pause", ""))

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		if published["workflow-run:wr-1"] != 1 {
			t.Fatalf("expected workflow-run hook publish once, got %d", published["workflow-run:wr-1"])
		}
		if published["workflow:wf-1:runs"] != 1 {
			t.Fatalf("expected workflow stream publish once, got %d", published["workflow:wf-1:runs"])
		}
	})
}

func TestHandleResumeWorkflowRun(t *testing.T) {
	t.Parallel()
	t.Run("success", func(t *testing.T) {
		t.Parallel()
		resumeCalled := false
		getCalls := 0
		published := map[string]int{}
		cb := &mockWorkflowTrigger{
			resumeWorkflowFn: func(_ context.Context, workflowRunID string) error {
				if workflowRunID != "wr-1" {
					t.Fatalf("workflowRunID = %q, want wr-1", workflowRunID)
				}
				resumeCalled = true
				return nil
			},
		}
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				getCalls++
				if getCalls == 1 {
					return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", Status: domain.WfStatusPaused}, nil
				}
				return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", Status: domain.WfStatusRunning}, nil
			},
		}

		pub := &mockPublisher{publishFn: func(_ context.Context, channel string, _ []byte) error {
			published[channel]++
			return nil
		}}
		srv := newWorkflowTestServerWithCallback(t, ms, &mockQueue{}, pub, cb, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/wr-1/resume", ""))

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		if !resumeCalled {
			t.Fatal("expected resume callback to be called")
		}
		if published["workflow-run:wr-1"] != 1 {
			t.Fatalf("expected workflow-run hook publish once, got %d", published["workflow-run:wr-1"])
		}
		if published["workflow:wf-1:runs"] != 1 {
			t.Fatalf("expected workflow stream publish once, got %d", published["workflow:wf-1:runs"])
		}
	})
}

func TestHandleGetWorkflowRunLabels(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: id, ProjectID: "proj-1", WorkflowID: "wf-1", Status: domain.WfStatusCompleted}, nil
		},
		ListWorkflowRunLabelsFunc: func(_ context.Context, workflowRunID string) (map[string]string, error) {
			if workflowRunID != "wr-1" {
				t.Fatalf("workflowRunID = %q, want wr-1", workflowRunID)
			}
			return map[string]string{"env": "test"}, nil
		},
	}
	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflow-runs/wr-1/labels", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Labels map[string]string `json:"labels"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode labels response: %v", err)
	}
	if got := resp.Labels["env"]; got != "test" {
		t.Fatalf("labels.env = %q, want test", got)
	}
}

func TestHandleDryRunWorkflow(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
			return &domain.Workflow{ID: id, ProjectID: "proj-1"}, nil
		},
	}
	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)

	t.Run("valid DAG", func(t *testing.T) {
		t.Parallel()
		w := httptest.NewRecorder()
		body := `{"steps":[{"job_id":"job-1","step_ref":"a"},{"job_id":"job-2","step_ref":"b","depends_on":["a"]}]}`
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows/wf-1/dry-run", body))
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("cycle", func(t *testing.T) {
		t.Parallel()
		w := httptest.NewRecorder()
		body := `{"steps":[{"job_id":"job-1","step_ref":"a","depends_on":["b"]},{"job_id":"job-2","step_ref":"b","depends_on":["a"]}]}`
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows/wf-1/dry-run", body))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
		}
	})
}

func TestHandleWorkflowPlan(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetWorkflowFunc: func(_ context.Context, workflowID string) (*domain.Workflow, error) {
			return &domain.Workflow{ID: workflowID, Version: 1}, nil
		},
		ListStepsByWorkflowVersionFunc: func(_ context.Context, workflowID string, version int) ([]domain.WorkflowStep, error) {
			if workflowID != "wf-1" || version != 1 {
				t.Fatalf("unexpected workflow/version %s/%d", workflowID, version)
			}
			return []domain.WorkflowStep{
				{StepRef: "a"},
				{StepRef: "b", DependsOn: []string{"a"}},
			}, nil
		},
	}
	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows/wf-1/plan", `{}`))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	roots, ok := resp["roots"].([]any)
	if !ok || len(roots) != 1 || roots[0] != "a" {
		t.Fatalf("unexpected roots: %v", resp["roots"])
	}
}

func TestHandleWorkflowGraph(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
			return &domain.Workflow{ID: id, ProjectID: "proj-1"}, nil
		},
		ListStepsByWorkflowFunc: func(_ context.Context, workflowID string) ([]domain.WorkflowStep, error) {
			if workflowID != "wf-1" {
				t.Fatalf("workflowID = %q, want wf-1", workflowID)
			}
			return []domain.WorkflowStep{
				{StepRef: "a"},
				{StepRef: "b", DependsOn: []string{"a"}},
			}, nil
		},
	}
	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflows/wf-1/graph", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflows/wf-1/graph?format=dot", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestWorkflowTopologyEndpoints_RejectCrossProjectBeforeLoadingSteps(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
			return &domain.Workflow{ID: id, ProjectID: "proj-other"}, nil
		},
		ListStepsByWorkflowFunc: func(_ context.Context, _ string) ([]domain.WorkflowStep, error) {
			t.Fatal("ListStepsByWorkflow must not run for cross-project workflow topology access")
			return nil, nil
		},
	}
	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")

	_, err := srv.handleDryRunWorkflow(ctx, &DryRunWorkflowInput{
		WorkflowID: "wf-other",
		Body: dryRunWorkflowRequest{Steps: []workflowStepRequest{
			{StepRef: "a"},
		}},
	})
	if !isHumaStatusError(err, http.StatusNotFound) {
		t.Fatalf("dry run error = %v, want 404", err)
	}

	_, err = srv.handleWorkflowGraph(ctx, &WorkflowGraphInput{WorkflowID: "wf-other"})
	if !isHumaStatusError(err, http.StatusNotFound) {
		t.Fatalf("graph error = %v, want 404", err)
	}
}

func TestHandleCancelWorkflowRun(t *testing.T) {
	t.Parallel()
	t.Run("success", func(t *testing.T) {
		t.Parallel()
		getWorkflowRunCalls := 0
		stepsCanceled := false
		jobsCanceled := false
		published := map[string]int{}
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				getWorkflowRunCalls++
				if getWorkflowRunCalls == 1 {
					return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", Status: domain.WfStatusRunning}, nil
				}
				return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", Status: domain.WfStatusCanceled}, nil
			},
			UpdateWorkflowRunStatusFunc: func(_ context.Context, _ string, from, to domain.WorkflowRunStatus, _ map[string]any) error {
				if from != domain.WfStatusRunning || to != domain.WfStatusCanceled {
					t.Fatalf("unexpected workflow transition: %s -> %s", from, to)
				}
				return nil
			},
			CancelNonTerminalStepRunsFunc: func(_ context.Context, workflowRunID string, _ time.Time, _ string) (int64, error) {
				if workflowRunID != "wr-1" {
					t.Fatalf("unexpected workflowRunID: %s", workflowRunID)
				}
				stepsCanceled = true
				return 1, nil
			},
			CancelJobRunsByWorkflowRunFunc: func(_ context.Context, workflowRunID string, _ time.Time, _ string) (int64, error) {
				if workflowRunID != "wr-1" {
					t.Fatalf("unexpected workflowRunID: %s", workflowRunID)
				}
				jobsCanceled = true
				return 1, nil
			},
		}

		pub := &mockPublisher{publishFn: func(_ context.Context, channel string, _ []byte) error {
			published[channel]++
			return nil
		}}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, pub, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/workflow-runs/wr-1", ""))

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		if !stepsCanceled {
			t.Fatal("expected CancelNonTerminalStepRuns to be called")
		}
		if !jobsCanceled {
			t.Fatal("expected CancelJobRunsByWorkflowRun to be called")
		}
		if published["workflow-run:wr-1"] != 1 {
			t.Fatalf("expected workflow-run hook publish once, got %d", published["workflow-run:wr-1"])
		}
		if published["workflow:wf-1:runs"] != 1 {
			t.Fatalf("expected workflow stream publish once, got %d", published["workflow:wf-1:runs"])
		}
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return nil, store.ErrWorkflowRunNotFound
			},
		}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/workflow-runs/missing", ""))

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})

	t.Run("already terminal", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", Status: domain.WfStatusCompleted}, nil
			},
		}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/workflow-runs/wr-1", ""))

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})
}

func TestHandleApproveWorkflowStep(t *testing.T) {
	t.Parallel()
	t.Run("success publishes workflow hook on status transition", func(t *testing.T) {
		t.Parallel()
		approved := false
		published := map[string]int{}
		getWorkflowRunCalls := 0

		cb := &mockWorkflowTrigger{
			approveStepFn: func(_ context.Context, workflowRunID, stepRef, approver string) error {
				if workflowRunID != "wr-1" || stepRef != "review" || approver != "user:alice" {
					t.Fatalf("unexpected approve args: %s %s %s", workflowRunID, stepRef, approver)
				}
				approved = true
				return nil
			},
		}

		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				getWorkflowRunCalls++
				if getWorkflowRunCalls == 1 {
					return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", Status: domain.WfStatusPaused}, nil
				}
				return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", Status: domain.WfStatusRunning}, nil
			},
			GetStepRunByWorkflowRunAndRefFunc: func(_ context.Context, workflowRunID, stepRef string) (*domain.WorkflowStepRun, error) {
				if workflowRunID != "wr-1" || stepRef != "review" {
					t.Fatalf("unexpected step lookup args: %s %s", workflowRunID, stepRef)
				}
				return &domain.WorkflowStepRun{ID: "sr-1", WorkflowRunID: workflowRunID, StepRef: stepRef, Status: domain.StepCompleted}, nil
			},
			GetWorkflowStepApprovalByStepRunIDFunc: func(_ context.Context, stepRunID string) (*domain.WorkflowStepApproval, error) {
				if stepRunID != "sr-1" {
					t.Fatalf("unexpected stepRunID %q", stepRunID)
				}
				return &domain.WorkflowStepApproval{ID: "ap-1", WorkflowRunID: "wr-1", WorkflowStepRunID: "sr-1", Status: "approved", ApprovedBy: "user:alice"}, nil
			},
		}

		pub := &mockPublisher{publishFn: func(_ context.Context, channel string, _ []byte) error {
			published[channel]++
			return nil
		}}

		srv := newWorkflowTestServerWithCallback(t, ms, &mockQueue{}, pub, cb, nil)
		w := httptest.NewRecorder()
		req := authedRequest(http.MethodPost, "/v1/workflow-runs/wr-1/steps/review/approve", `{"approver":"alice"}`)
		req.Header.Set("X-Actor-Id", "user:alice")
		srv.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		if !approved {
			t.Fatal("expected approve callback to be called")
		}
		if published["workflow-run:wr-1"] != 1 {
			t.Fatalf("expected workflow-run hook publish once, got %d", published["workflow-run:wr-1"])
		}
		if published["workflow:wf-1:runs"] != 1 {
			t.Fatalf("expected workflow stream publish once, got %d", published["workflow:wf-1:runs"])
		}
	})

	t.Run("success with same project context", func(t *testing.T) {
		t.Parallel()
		approved := false

		cb := &mockWorkflowTrigger{
			approveStepFn: func(_ context.Context, workflowRunID, stepRef, approver string) error {
				if workflowRunID != "wr-1" || stepRef != "review" || approver != "user:alice" {
					t.Fatalf("unexpected approve args: %s %s %s", workflowRunID, stepRef, approver)
				}
				approved = true
				return nil
			},
		}

		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", ProjectID: "proj-1", Status: domain.WfStatusRunning}, nil
			},
			GetStepRunByWorkflowRunAndRefFunc: func(_ context.Context, workflowRunID, stepRef string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{ID: "sr-1", WorkflowRunID: workflowRunID, StepRef: stepRef, Status: domain.StepCompleted}, nil
			},
			GetWorkflowStepApprovalByStepRunIDFunc: func(_ context.Context, _ string) (*domain.WorkflowStepApproval, error) {
				return &domain.WorkflowStepApproval{ID: "ap-1", WorkflowRunID: "wr-1", WorkflowStepRunID: "sr-1", Status: "approved", ApprovedBy: "user:alice"}, nil
			},
		}

		srv := newWorkflowTestServerWithCallback(t, ms, &mockQueue{}, nil, cb, nil)
		w := httptest.NewRecorder()
		req := authedProjectRequest(http.MethodPost, "/v1/workflow-runs/wr-1/steps/review/approve", `{"approver":"alice"}`, "proj-1")
		req.Header.Set("X-Actor-Id", "user:alice")
		srv.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		if !approved {
			t.Fatal("expected approve callback to be called")
		}
	})
}

func TestHandleSkipWorkflowStep(t *testing.T) {
	t.Parallel()
	t.Run("success", func(t *testing.T) {
		t.Parallel()
		skipped := false
		published := map[string]int{}
		getWorkflowRunCalls := 0

		cb := &mockWorkflowTrigger{
			skipStepFn: func(_ context.Context, workflowRunID, stepRef, reason string) error {
				if workflowRunID != "wr-1" || stepRef != "review" || reason != "manual skip" {
					t.Fatalf("unexpected skip args: %s %s %s", workflowRunID, stepRef, reason)
				}
				skipped = true
				return nil
			},
		}

		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				getWorkflowRunCalls++
				if getWorkflowRunCalls == 1 {
					return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", Status: domain.WfStatusRunning}, nil
				}
				return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", Status: domain.WfStatusCompleted}, nil
			},
			GetStepRunByWorkflowRunAndRefFunc: func(_ context.Context, workflowRunID, stepRef string) (*domain.WorkflowStepRun, error) {
				if workflowRunID != "wr-1" || stepRef != "review" {
					t.Fatalf("unexpected step lookup args: %s %s", workflowRunID, stepRef)
				}
				return &domain.WorkflowStepRun{ID: "sr-1", WorkflowRunID: workflowRunID, StepRef: stepRef, Status: domain.StepSkipped}, nil
			},
		}

		pub := &mockPublisher{publishFn: func(_ context.Context, channel string, _ []byte) error {
			published[channel]++
			return nil
		}}

		srv := newWorkflowTestServerWithCallback(t, ms, &mockQueue{}, pub, cb, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/wr-1/steps/review/skip", `{"reason":"manual skip"}`))

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		if !skipped {
			t.Fatal("expected skip callback to be called")
		}
		if published["workflow-run:wr-1"] != 1 {
			t.Fatalf("expected workflow-run hook publish once, got %d", published["workflow-run:wr-1"])
		}
		if published["workflow:wf-1:runs"] != 1 {
			t.Fatalf("expected workflow stream publish once, got %d", published["workflow:wf-1:runs"])
		}
	})

	t.Run("callback unavailable", func(t *testing.T) {
		t.Parallel()
		srv := newWorkflowTestServer(t, &APIStoreMock{}, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/wr-1/steps/review/skip", `{}`))

		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d", w.Code)
		}
	})

	t.Run("callback error", func(t *testing.T) {
		t.Parallel()
		cb := &mockWorkflowTrigger{
			skipStepFn: func(_ context.Context, _, _, _ string) error {
				return errors.New("cannot skip")
			},
		}
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", Status: domain.WfStatusRunning}, nil
			},
		}

		srv := newWorkflowTestServerWithCallback(t, ms, &mockQueue{}, nil, cb, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/wr-1/steps/review/skip", `{}`))

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("success with same project context", func(t *testing.T) {
		t.Parallel()
		skipped := false

		cb := &mockWorkflowTrigger{
			skipStepFn: func(_ context.Context, workflowRunID, stepRef, reason string) error {
				if workflowRunID != "wr-1" || stepRef != "review" || reason != "manual skip" {
					t.Fatalf("unexpected skip args: %s %s %s", workflowRunID, stepRef, reason)
				}
				skipped = true
				return nil
			},
		}

		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", ProjectID: "proj-1", Status: domain.WfStatusRunning}, nil
			},
			GetStepRunByWorkflowRunAndRefFunc: func(_ context.Context, workflowRunID, stepRef string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{ID: "sr-1", WorkflowRunID: workflowRunID, StepRef: stepRef, Status: domain.StepSkipped}, nil
			},
		}

		srv := newWorkflowTestServerWithCallback(t, ms, &mockQueue{}, nil, cb, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/workflow-runs/wr-1/steps/review/skip", `{"reason":"manual skip"}`, "proj-1"))

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		if !skipped {
			t.Fatal("expected skip callback to be called")
		}
	})
}

func TestHandleForceCompleteWorkflowStep(t *testing.T) {
	t.Parallel()
	t.Run("success", func(t *testing.T) {
		t.Parallel()
		forced := false
		published := map[string]int{}
		getWorkflowRunCalls := 0

		cb := &mockWorkflowTrigger{
			forceCompleteStepFn: func(_ context.Context, workflowRunID, stepRef string, result json.RawMessage) error {
				if workflowRunID != "wr-1" || stepRef != "review" {
					t.Fatalf("unexpected force-complete args: %s %s", workflowRunID, stepRef)
				}
				if string(result) != `{"ok":true}` {
					t.Fatalf("unexpected result payload: %s", string(result))
				}
				forced = true
				return nil
			},
		}

		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				getWorkflowRunCalls++
				if getWorkflowRunCalls == 1 {
					return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", Status: domain.WfStatusRunning}, nil
				}
				return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", Status: domain.WfStatusCompleted}, nil
			},
			GetStepRunByWorkflowRunAndRefFunc: func(_ context.Context, workflowRunID, stepRef string) (*domain.WorkflowStepRun, error) {
				if workflowRunID != "wr-1" || stepRef != "review" {
					t.Fatalf("unexpected step lookup args: %s %s", workflowRunID, stepRef)
				}
				return &domain.WorkflowStepRun{ID: "sr-1", WorkflowRunID: workflowRunID, StepRef: stepRef, Status: domain.StepCompleted}, nil
			},
		}

		pub := &mockPublisher{publishFn: func(_ context.Context, channel string, _ []byte) error {
			published[channel]++
			return nil
		}}

		srv := newWorkflowTestServerWithCallback(t, ms, &mockQueue{}, pub, cb, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/wr-1/steps/review/force-complete", `{"result":{"ok":true}}`))

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		if !forced {
			t.Fatal("expected force-complete callback to be called")
		}
		if published["workflow-run:wr-1"] != 1 {
			t.Fatalf("expected workflow-run hook publish once, got %d", published["workflow-run:wr-1"])
		}
		if published["workflow:wf-1:runs"] != 1 {
			t.Fatalf("expected workflow stream publish once, got %d", published["workflow:wf-1:runs"])
		}
	})

	t.Run("callback unavailable", func(t *testing.T) {
		t.Parallel()
		srv := newWorkflowTestServer(t, &APIStoreMock{}, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/wr-1/steps/review/force-complete", `{}`))

		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d", w.Code)
		}
	})

	t.Run("callback error", func(t *testing.T) {
		t.Parallel()
		cb := &mockWorkflowTrigger{
			forceCompleteStepFn: func(_ context.Context, _, _ string, _ json.RawMessage) error {
				return errors.New("cannot force-complete")
			},
		}
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", Status: domain.WfStatusRunning}, nil
			},
		}

		srv := newWorkflowTestServerWithCallback(t, ms, &mockQueue{}, nil, cb, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/wr-1/steps/review/force-complete", `{}`))

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})
}

func TestHandleListWorkflowStepRuns(t *testing.T) {
	t.Parallel()
	t.Run("success", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: id, ProjectID: "proj-1", WorkflowID: "wf-1", Status: domain.WfStatusCompleted}, nil
			},
			ListStepRunsByWorkflowRunFunc: func(_ context.Context, workflowRunID string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
				if workflowRunID != "wr-1" {
					t.Fatalf("workflow_run_id = %q, want wr-1", workflowRunID)
				}
				return []domain.WorkflowStepRun{{ID: "sr-1", StepRef: "s1"}}, nil
			},
		}

		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflow-runs/wr-1/steps", ""))

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
	})

	t.Run("store error", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: id, ProjectID: "proj-1", WorkflowID: "wf-1", Status: domain.WfStatusCompleted}, nil
			},
			ListStepRunsByWorkflowRunFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
				return nil, errors.New("db down")
			},
		}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflow-runs/wr-1/steps", ""))

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", w.Code)
		}
	})
}

func TestHandleGetWorkflowRunGraph(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", WorkflowVersion: 2}, nil
		},
		ListStepsByWorkflowVersionFunc: func(_ context.Context, workflowID string, version int) ([]domain.WorkflowStep, error) {
			if workflowID != "wf-1" || version != 2 {
				t.Fatalf("unexpected workflow/version: %s/%d", workflowID, version)
			}
			return []domain.WorkflowStep{{StepRef: "a", StepType: domain.WorkflowStepTypeJob}, {StepRef: "b", StepType: domain.WorkflowStepTypeJob, DependsOn: []string{"a"}}}, nil
		},
		ListStepRunsByWorkflowRunFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
			return []domain.WorkflowStepRun{{ID: "sr-a", StepRef: "a", Status: domain.StepCompleted, DepsRequired: 0, DepsCompleted: 0}, {ID: "sr-b", StepRef: "b", Status: domain.StepWaiting, DepsRequired: 1, DepsCompleted: 1}}, nil
		},
	}
	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflow-runs/wr-1/graph", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "\"runnable\":[\"b\"]") {
		t.Fatalf("expected runnable step in response, got: %s", w.Body.String())
	}
}

func TestHandleGetWorkflowRunGraph_CriticalPathEstimate(t *testing.T) {
	t.Parallel()
	startedAt := time.Now().Add(-2 * time.Second)
	finishedAt := startedAt.Add(1 * time.Second)
	ms := &APIStoreMock{
		GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", WorkflowVersion: 1}, nil
		},
		ListStepsByWorkflowVersionFunc: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{
				{StepRef: "a", StepType: domain.WorkflowStepTypeJob, TimeoutSecsOverride: 5},
				{StepRef: "b", StepType: domain.WorkflowStepTypeJob, DependsOn: []string{"a"}, TimeoutSecsOverride: 10},
				{StepRef: "c", StepType: domain.WorkflowStepTypeJob, TimeoutSecsOverride: 3},
				{StepRef: "d", StepType: domain.WorkflowStepTypeJob, DependsOn: []string{"c"}, TimeoutSecsOverride: 20},
			}, nil
		},
		ListStepRunsByWorkflowRunFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
			return []domain.WorkflowStepRun{
				{ID: "sr-a", StepRef: "a", Status: domain.StepCompleted, StartedAt: &startedAt, FinishedAt: &finishedAt, DepsRequired: 0, DepsCompleted: 0},
				{ID: "sr-b", StepRef: "b", Status: domain.StepPending, DepsRequired: 1, DepsCompleted: 1},
				{ID: "sr-c", StepRef: "c", Status: domain.StepRunning, StartedAt: &startedAt, DepsRequired: 0, DepsCompleted: 0},
				{ID: "sr-d", StepRef: "d", Status: domain.StepPending, DepsRequired: 1, DepsCompleted: 0},
			}, nil
		},
	}

	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflow-runs/wr-1/graph", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	pathRaw, ok := payload["critical_path"].([]any)
	if !ok || len(pathRaw) != 2 || pathRaw[0] != "c" || pathRaw[1] != "d" {
		t.Fatalf("unexpected critical path: %#v", payload["critical_path"])
	}
	estimateMS := int64(payload["critical_path_estimate_ms"].(float64))
	if estimateMS != 23_000 {
		t.Fatalf("critical_path_estimate_ms = %d, want 23000", estimateMS)
	}
	remainingMS := int64(payload["critical_path_remaining_ms"].(float64))
	if remainingMS < 20_000 || remainingMS > 22_000 {
		t.Fatalf("critical_path_remaining_ms out of range: %d", remainingMS)
	}
}

func TestHandleGetWorkflowRunExplain(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: id, ProjectID: "proj-1", WorkflowID: "wf-1", Status: domain.WfStatusCompleted}, nil
		},
		ListWorkflowStepDecisionsFunc: func(_ context.Context, workflowRunID, stepRef, decisionType string, _ int, _ *time.Time) ([]domain.WorkflowStepDecision, error) {
			if workflowRunID != "wr-1" || stepRef != "review" || decisionType != "condition" {
				t.Fatalf("unexpected filters")
			}
			return []domain.WorkflowStepDecision{{ID: "d1", WorkflowRunID: workflowRunID, StepRef: stepRef, DecisionType: decisionType, Decision: "skip", CreatedAt: time.Now()}}, nil
		},
	}
	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflow-runs/wr-1/explain?step_ref=review&decision_type=condition", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "\"decision\":\"skip\"") {
		t.Fatalf("expected decision in payload: %s", w.Body.String())
	}
}

func TestHandleRetryWorkflowStep(t *testing.T) {
	t.Parallel()
	cb := &mockWorkflowTrigger{resumeWorkflowFn: func(_ context.Context, workflowRunID string) error {
		if workflowRunID != "wr-1" {
			t.Fatalf("unexpected workflow run id: %s", workflowRunID)
		}
		return nil
	}}
	ms := &APIStoreMock{
		GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", Status: domain.WfStatusPaused}, nil
		},
		GetStepRunByWorkflowRunAndRefFunc: func(_ context.Context, _ string, _ string) (*domain.WorkflowStepRun, error) {
			return &domain.WorkflowStepRun{ID: "sr-1", StepRef: "review", Status: domain.StepFailed}, nil
		},
		UpdateStepRunStatusFunc: func(_ context.Context, id string, status domain.StepRunStatus, _ map[string]any) error {
			if id != "sr-1" || status != domain.StepPending {
				t.Fatalf("unexpected status update")
			}
			return nil
		},
	}
	srv := newWorkflowTestServerWithCallback(t, ms, &mockQueue{}, nil, cb, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/wr-1/steps/review/retry", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestHandleReplayWorkflowSubtree(t *testing.T) {
	t.Parallel()
	cb := &mockWorkflowTrigger{resumeWorkflowFn: func(_ context.Context, _ string) error { return nil }}
	ms := &APIStoreMock{
		GetWorkflowRunFunc: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1}, nil
		},
		ListStepsByWorkflowVersionFunc: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{{StepRef: "a"}, {StepRef: "b", DependsOn: []string{"a"}}, {StepRef: "c", DependsOn: []string{"b"}}}, nil
		},
		ListStepRunsByWorkflowRunFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowStepRun, error) {
			return []domain.WorkflowStepRun{{ID: "sr-a", StepRef: "a", Status: domain.StepCompleted}, {ID: "sr-b", StepRef: "b", Status: domain.StepFailed}, {ID: "sr-c", StepRef: "c", Status: domain.StepPending}}, nil
		},
		UpdateStepRunStatusFunc: func(_ context.Context, _ string, status domain.StepRunStatus, _ map[string]any) error {
			if status != domain.StepPending {
				t.Fatalf("expected pending")
			}
			return nil
		},
	}
	srv := newWorkflowTestServerWithCallback(t, ms, &mockQueue{}, nil, cb, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/wr-1/steps/b/replay-subtree", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestHandlePauseWorkflowRun_ErrorPaths(t *testing.T) {
	t.Parallel()
	t.Run("not_found", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return nil, store.ErrWorkflowRunNotFound
			},
		}

		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/wr-missing/pause", ""))

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})

	t.Run("already_terminal", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", Status: domain.WfStatusCompleted}, nil
			},
		}

		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/wr-1/pause", ""))

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "already in terminal state") {
			t.Fatalf("expected terminal-state error, got: %s", w.Body.String())
		}
	})

	t.Run("already_paused_idempotent", func(t *testing.T) {
		t.Parallel()
		updateCalls := 0
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: id, Status: domain.WfStatusPaused}, nil
			},
			UpdateWorkflowRunStatusFunc: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				updateCalls++
				return nil
			},
		}

		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/wr-1/pause", ""))

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		if updateCalls != 0 {
			t.Fatalf("update calls = %d, want 0", updateCalls)
		}
	})

	t.Run("not_running", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: id, Status: domain.WfStatusPending}, nil
			},
		}

		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/wr-1/pause", ""))

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "only be paused from running state") {
			t.Fatalf("expected running-state error, got: %s", w.Body.String())
		}
	})

	t.Run("update_conflict", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: id, Status: domain.WfStatusRunning}, nil
			},
			UpdateWorkflowRunStatusFunc: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return errors.New("conflict")
			},
		}

		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/wr-1/pause", ""))

		if w.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d", w.Code)
		}
	})

	t.Run("get_updated_run_error", func(t *testing.T) {
		t.Parallel()
		getCalls := 0
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				getCalls++
				if getCalls == 1 {
					return &domain.WorkflowRun{ID: id, Status: domain.WfStatusRunning}, nil
				}
				return nil, errors.New("read failed")
			},
			UpdateWorkflowRunStatusFunc: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
		}

		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/wr-1/pause", ""))

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", w.Code)
		}
	})
}

func TestHandleResumeWorkflowRun_ErrorPaths(t *testing.T) {
	t.Parallel()
	t.Run("callback_unavailable", func(t *testing.T) {
		t.Parallel()
		srv := newWorkflowTestServer(t, &APIStoreMock{}, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/wr-1/resume", ""))

		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d", w.Code)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		t.Parallel()
		cb := &mockWorkflowTrigger{}
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return nil, store.ErrWorkflowRunNotFound
			},
		}

		srv := newWorkflowTestServerWithCallback(t, ms, &mockQueue{}, nil, cb, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/wr-missing/resume", ""))

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})

	t.Run("not_paused", func(t *testing.T) {
		t.Parallel()
		cb := &mockWorkflowTrigger{}
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: id, Status: domain.WfStatusRunning}, nil
			},
		}

		srv := newWorkflowTestServerWithCallback(t, ms, &mockQueue{}, nil, cb, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/wr-1/resume", ""))

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "not paused") {
			t.Fatalf("expected not-paused error, got: %s", w.Body.String())
		}
	})

	t.Run("callback_error", func(t *testing.T) {
		t.Parallel()
		cb := &mockWorkflowTrigger{
			resumeWorkflowFn: func(_ context.Context, _ string) error {
				return errors.New("resume rejected")
			},
		}
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: id, Status: domain.WfStatusPaused}, nil
			},
		}

		srv := newWorkflowTestServerWithCallback(t, ms, &mockQueue{}, nil, cb, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/wr-1/resume", ""))

		if w.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d", w.Code)
		}
	})

	t.Run("get_updated_run_error", func(t *testing.T) {
		t.Parallel()
		getCalls := 0
		cb := &mockWorkflowTrigger{
			resumeWorkflowFn: func(_ context.Context, _ string) error {
				return nil
			},
		}
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				getCalls++
				if getCalls == 1 {
					return &domain.WorkflowRun{ID: id, Status: domain.WfStatusPaused}, nil
				}
				return nil, errors.New("read failed")
			},
		}

		srv := newWorkflowTestServerWithCallback(t, ms, &mockQueue{}, nil, cb, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/wr-1/resume", ""))

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", w.Code)
		}
	})
}

func TestHandleCancelWorkflowRun_ErrorPaths(t *testing.T) {
	t.Parallel()
	t.Run("update_workflow_status_conflict", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: id, Status: domain.WfStatusRunning}, nil
			},
			UpdateWorkflowRunStatusFunc: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return errors.New("conflict")
			},
		}

		srv := newWorkflowTestServer(t, ms, &mockQueue{}, &mockPublisher{}, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/workflow-runs/wr-1", ""))

		if w.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d", w.Code)
		}
	})

	t.Run("cancel_step_runs_error", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: id, Status: domain.WfStatusRunning}, nil
			},
			UpdateWorkflowRunStatusFunc: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			CancelNonTerminalStepRunsFunc: func(_ context.Context, _ string, _ time.Time, _ string) (int64, error) {
				return 0, errors.New("db down")
			},
		}

		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/workflow-runs/wr-1", ""))

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", w.Code)
		}
	})

	t.Run("cancel_job_runs_error", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: id, Status: domain.WfStatusRunning}, nil
			},
			UpdateWorkflowRunStatusFunc: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
			CancelJobRunsByWorkflowRunFunc: func(_ context.Context, _ string, _ time.Time, _ string) (int64, error) {
				return 0, errors.New("db down")
			},
		}

		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/workflow-runs/wr-1", ""))

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", w.Code)
		}
	})
}

func TestHandleDryRunWorkflow_ErrorPaths(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
			return &domain.Workflow{ID: id, ProjectID: "proj-1"}, nil
		},
		ListStepsByWorkflowFunc: func(_ context.Context, workflowID string) ([]domain.WorkflowStep, error) {
			if workflowID != "wf-1" {
				t.Fatalf("workflowID = %q, want wf-1", workflowID)
			}
			return []domain.WorkflowStep{
				{StepRef: "a", DependsOn: []string{"b"}},
				{StepRef: "b", DependsOn: []string{"a"}},
			}, nil
		},
	}
	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)

	t.Run("invalid_json_body", func(t *testing.T) {
		t.Parallel()
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows/wf-1/dry-run", `{"steps":[`))

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("empty_steps", func(t *testing.T) {
		t.Parallel()
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows/wf-1/dry-run", `{"steps":[]}`))

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("duplicate_step_ref", func(t *testing.T) {
		t.Parallel()
		w := httptest.NewRecorder()
		body := `{"steps":[{"job_id":"job-1","step_ref":"a"},{"job_id":"job-2","step_ref":"a"}]}`
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows/wf-1/dry-run", body))

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("unknown_dependency", func(t *testing.T) {
		t.Parallel()
		w := httptest.NewRecorder()
		body := `{"steps":[{"job_id":"job-1","step_ref":"a","depends_on":["missing"]}]}`
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows/wf-1/dry-run", body))

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("self_dependency", func(t *testing.T) {
		t.Parallel()
		w := httptest.NewRecorder()
		body := `{"steps":[{"job_id":"job-1","step_ref":"a","depends_on":["a"]}]}`
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows/wf-1/dry-run", body))

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
		}
	})
}

func TestHandleSkipWorkflowStep_ErrorPaths(t *testing.T) {
	t.Parallel()

	t.Run("cross_project_returns_not_found_without_callback", func(t *testing.T) {
		t.Parallel()
		callbackCalled := false
		cb := &mockWorkflowTrigger{
			skipStepFn: func(_ context.Context, _, _, _ string) error {
				callbackCalled = true
				return nil
			},
		}
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", ProjectID: "proj-other", Status: domain.WfStatusRunning}, nil
			},
		}
		srv := newWorkflowTestServerWithCallback(t, ms, &mockQueue{}, nil, cb, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/workflow-runs/wr-1/steps/review/skip", `{"reason":"manual skip"}`, "proj-1"))

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
		}
		if callbackCalled {
			t.Fatal("skip callback should not be called on project mismatch")
		}
	})

	t.Run("workflow_run_not_found_returns_not_found_without_callback", func(t *testing.T) {
		t.Parallel()
		callbackCalled := false
		cb := &mockWorkflowTrigger{
			skipStepFn: func(_ context.Context, _, _, _ string) error {
				callbackCalled = true
				return nil
			},
		}
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return nil, store.ErrWorkflowRunNotFound
			},
		}
		srv := newWorkflowTestServerWithCallback(t, ms, &mockQueue{}, nil, cb, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/workflow-runs/wr-1/steps/review/skip", `{"reason":"manual skip"}`, "proj-1"))

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
		}
		if callbackCalled {
			t.Fatal("skip callback should not be called when workflow run is missing")
		}
	})
}

func TestHandleListWorkflowRunsByProject_ErrorPaths(t *testing.T) {
	t.Parallel()
	t.Run("invalid_limit", func(t *testing.T) {
		t.Parallel()
		called := false
		ms := &APIStoreMock{
			ListWorkflowRunsByProjectFunc: func(_ context.Context, _ string, _ *domain.WorkflowRunStatus, _ int, _ *time.Time) ([]domain.WorkflowRun, error) {
				called = true
				return nil, nil
			},
		}

		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/workflow-runs?limit=-1", "", "proj-1"))

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
		if called {
			t.Fatal("expected ListWorkflowRunsByProject to not be called for invalid limit")
		}
	})

	t.Run("limit_clamped_to_100", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			ListWorkflowRunsByProjectFunc: func(_ context.Context, projectID string, _ *domain.WorkflowRunStatus, limit int, _ *time.Time) ([]domain.WorkflowRun, error) {
				if projectID != "proj-1" || limit != 101 { // handler passes limit+1
					t.Fatalf("unexpected args: %s %d", projectID, limit)
				}
				return []domain.WorkflowRun{{ID: "wr-1"}}, nil
			},
		}

		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/workflow-runs?limit=200", "", "proj-1"))

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("store_error", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			ListWorkflowRunsByProjectFunc: func(_ context.Context, _ string, _ *domain.WorkflowRunStatus, _ int, _ *time.Time) ([]domain.WorkflowRun, error) {
				return nil, errors.New("db down")
			},
		}

		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/workflow-runs", "", "proj-1"))

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", w.Code)
		}
	})
}

func TestHandleWorkflowVersionDiffAndImpact(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
			return &domain.Workflow{ID: id, ProjectID: "proj-1"}, nil
		},
		GetWorkflowVersionByVersionIDFunc: func(_ context.Context, _ string, versionID string) (*domain.WorkflowVersion, error) {
			if versionID == "v1" {
				return &domain.WorkflowVersion{ID: "v1", Version: 1}, nil
			}
			return &domain.WorkflowVersion{ID: "v2", Version: 2}, nil
		},
		ListStepsByWorkflowVersionFunc: func(_ context.Context, _ string, version int) ([]domain.WorkflowStep, error) {
			if version == 1 {
				return []domain.WorkflowStep{{StepRef: "a"}}, nil
			}
			return []domain.WorkflowStep{{StepRef: "a"}, {StepRef: "b"}}, nil
		},
		ListWorkflowRunsFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.WorkflowRun, error) {
			return []domain.WorkflowRun{{WorkflowVersion: 2}, {WorkflowVersion: 1}}, nil
		},
	}
	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflows/wf-1/versions/v1/diff/v2", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	w2 := httptest.NewRecorder()
	srv.ServeHTTP(w2, authedRequest(http.MethodGet, "/v1/workflows/wf-1/versions/v2/impact", ""))
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w2.Code)
	}
}

func TestHandleListWorkflowVersions(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: id, ProjectID: "proj-1"}, nil
			},
			ListWorkflowVersionsFunc: func(_ context.Context, workflowID string, limit int) ([]domain.WorkflowVersion, error) {
				if workflowID != "wf-1" {
					t.Fatalf("workflowID = %q, want wf-1", workflowID)
				}
				if limit != 10 {
					t.Fatalf("limit = %d, want 10", limit)
				}
				return []domain.WorkflowVersion{{ID: "v1", WorkflowID: workflowID, Version: 1}}, nil
			},
		}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflows/wf-1/versions?limit=10", ""))

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var versions []domain.WorkflowVersion
		if err := json.NewDecoder(w.Body).Decode(&versions); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if len(versions) != 1 || versions[0].ID != "v1" {
			t.Fatalf("unexpected versions: %+v", versions)
		}
	})

	t.Run("invalid limit", func(t *testing.T) {
		t.Parallel()
		srv := newWorkflowTestServer(t, &APIStoreMock{}, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflows/wf-1/versions?limit=-1", ""))

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("store error", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: id, ProjectID: "proj-1"}, nil
			},
			ListWorkflowVersionsFunc: func(_ context.Context, _ string, _ int) ([]domain.WorkflowVersion, error) {
				return nil, errors.New("db down")
			},
		}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflows/wf-1/versions", ""))

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", w.Code)
		}
	})
}

func TestHandleGetWorkflowVersion(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowVersionByVersionIDFunc: func(_ context.Context, workflowID, versionID string) (*domain.WorkflowVersion, error) {
				if workflowID != "wf-1" || versionID != "v1" {
					t.Fatalf("unexpected args: %s %s", workflowID, versionID)
				}
				return &domain.WorkflowVersion{ID: "v1", WorkflowID: workflowID, Version: 1}, nil
			},
		}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflows/wf-1/versions/v1", ""))

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowVersionByVersionIDFunc: func(_ context.Context, _, _ string) (*domain.WorkflowVersion, error) {
				return nil, store.ErrWorkflowVersionNotFound
			},
		}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflows/wf-1/versions/missing", ""))

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})

	t.Run("store error", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowVersionByVersionIDFunc: func(_ context.Context, _, _ string) (*domain.WorkflowVersion, error) {
				return nil, errors.New("db down")
			},
		}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflows/wf-1/versions/v1", ""))

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", w.Code)
		}
	})
}

func TestHandleListWorkflowVersionSteps(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowVersionByVersionIDFunc: func(_ context.Context, _, _ string) (*domain.WorkflowVersion, error) {
				return &domain.WorkflowVersion{ID: "v2", Version: 2}, nil
			},
			ListStepsByWorkflowVersionFunc: func(_ context.Context, workflowID string, version int) ([]domain.WorkflowStep, error) {
				if workflowID != "wf-1" || version != 2 {
					t.Fatalf("unexpected args: %s %d", workflowID, version)
				}
				return []domain.WorkflowStep{{StepRef: "a"}, {StepRef: "b", DependsOn: []string{"a"}}}, nil
			},
		}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflows/wf-1/versions/v2/steps", ""))

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var steps []domain.WorkflowStep
		if err := json.NewDecoder(w.Body).Decode(&steps); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if len(steps) != 2 {
			t.Fatalf("steps len = %d, want 2", len(steps))
		}
	})

	t.Run("version not found", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowVersionByVersionIDFunc: func(_ context.Context, _, _ string) (*domain.WorkflowVersion, error) {
				return nil, store.ErrWorkflowVersionNotFound
			},
		}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflows/wf-1/versions/missing/steps", ""))

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})

	t.Run("list steps error", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowVersionByVersionIDFunc: func(_ context.Context, _, _ string) (*domain.WorkflowVersion, error) {
				return &domain.WorkflowVersion{ID: "v1", Version: 1}, nil
			},
			ListStepsByWorkflowVersionFunc: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return nil, errors.New("list failed")
			},
		}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflows/wf-1/versions/v1/steps", ""))

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", w.Code)
		}
	})
}

func TestHandleSimulateWorkflowAndPolicy(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
			return &domain.Workflow{ID: id, Version: 1}, nil
		},
		ListStepsByWorkflowVersionFunc: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{{StepRef: "a"}, {StepRef: "b"}}, nil
		},
		UpsertWorkflowPolicyFunc: func(_ context.Context, _ *domain.WorkflowPolicy) error { return nil },
		GetWorkflowPolicyByProjectFunc: func(_ context.Context, _ string) (*domain.WorkflowPolicy, error) {
			return &domain.WorkflowPolicy{ProjectID: "proj-1", MaxDepth: 5}, nil
		},
	}
	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows/wf-1/simulate", "{}"))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	w2 := httptest.NewRecorder()
	srv.ServeHTTP(w2, authedRequest(http.MethodPut, "/v1/workflow-policies/proj-1", `{"max_depth":4}`))
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w2.Code)
	}

	w3 := httptest.NewRecorder()
	srv.ServeHTTP(w3, authedRequest(http.MethodGet, "/v1/workflow-policies/proj-1", ""))
	if w3.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w3.Code)
	}
}

func TestHandleRetryWorkflowRun(t *testing.T) {
	t.Parallel()
	t.Run("success - retry failed run", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{
					ID: id, WorkflowID: "wf-1", ProjectID: "proj-1",
					Status: domain.WfStatusFailed,
				}, nil
			},
		}
		trigger := &mockWorkflowTrigger{
			retryWorkflowRunFn: func(_ context.Context, originalRunID string) (*domain.WorkflowRun, error) {
				if originalRunID != "wr-1" {
					t.Fatalf("originalRunID = %q, want wr-1", originalRunID)
				}
				return &domain.WorkflowRun{
					ID: "wr-retry-1", WorkflowID: "wf-1", ProjectID: "proj-1",
					Status: domain.WfStatusRunning, TriggeredBy: domain.TriggerRetry,
					RetryOfRunID: "wr-1",
				}, nil
			},
		}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, &mockPublisher{}, trigger)

		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/wr-1/retry", ""))

		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
		}

		var resp domain.WorkflowRun
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if resp.ID != "wr-retry-1" {
			t.Fatalf("response ID = %q, want wr-retry-1", resp.ID)
		}
		if resp.RetryOfRunID != "wr-1" {
			t.Fatalf("RetryOfRunID = %q, want wr-1", resp.RetryOfRunID)
		}
	})

	t.Run("reject retry of non-terminal run", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{
					ID: "wr-1", Status: domain.WfStatusRunning,
				}, nil
			},
		}
		trigger := &mockWorkflowTrigger{}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, trigger)

		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/wr-1/retry", ""))

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return nil, store.ErrWorkflowRunNotFound
			},
		}
		trigger := &mockWorkflowTrigger{}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, trigger)

		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/wr-missing/retry", ""))

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("engine unavailable", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{}
		// No workflow engine configured.
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)

		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/wr-1/retry", ""))

		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("engine error propagated", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{
					ID: "wr-1", Status: domain.WfStatusFailed,
				}, nil
			},
		}
		trigger := &mockWorkflowTrigger{
			retryWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return nil, fmt.Errorf("workflow is disabled: wf-1")
			},
		}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, trigger)

		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/wr-1/retry", ""))

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
		}
	})
}

func TestHandleCreateWorkflow_SubWorkflowValidation(t *testing.T) {
	t.Parallel()
	t.Run("missing sub_workflow_id returns 400", func(t *testing.T) {
		t.Parallel()
		srv := newWorkflowTestServer(t, &APIStoreMock{}, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		body := `{"project_id":"proj-1","name":"wf","slug":"wf","steps":[{"step_ref":"sub","step_type":"sub_workflow"}]}`
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows", body))

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "sub_workflow_id") {
			t.Fatalf("expected error about sub_workflow_id, got: %s", w.Body.String())
		}
	})

	t.Run("sub_workflow step with job_id returns 400", func(t *testing.T) {
		t.Parallel()
		srv := newWorkflowTestServer(t, &APIStoreMock{}, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		body := `{"project_id":"proj-1","name":"wf","slug":"wf","steps":[{"step_ref":"sub","step_type":"sub_workflow","sub_workflow_id":"wf-child","job_id":"job-1"}]}`
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows", body))

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "must not have job_id") {
			t.Fatalf("expected error about job_id, got: %s", w.Body.String())
		}
	})

	t.Run("negative max_nesting_depth returns 400", func(t *testing.T) {
		t.Parallel()
		srv := newWorkflowTestServer(t, &APIStoreMock{}, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		body := `{"project_id":"proj-1","name":"wf","slug":"wf","steps":[{"step_ref":"sub","step_type":"sub_workflow","sub_workflow_id":"wf-child","max_nesting_depth":-1}]}`
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows", body))

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "max_nesting_depth") {
			t.Fatalf("expected error about max_nesting_depth, got: %s", w.Body.String())
		}
	})

	t.Run("valid sub_workflow step returns 201", func(t *testing.T) {
		t.Parallel()
		var capturedStep *domain.WorkflowStep
		ms := &APIStoreMock{
			CreateWorkflowFunc: func(_ context.Context, wf *domain.Workflow) error {
				wf.ID = "wf-1"
				return nil
			},
			CreateWorkflowStepFunc: func(_ context.Context, step *domain.WorkflowStep) error {
				capturedStep = step
				return nil
			},
			CreateWorkflowVersionSnapshotFunc: func(_ context.Context, _ string, _ int) error {
				return nil
			},
		}

		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		body := `{"project_id":"proj-1","name":"wf","slug":"wf","steps":[{"step_ref":"sub","step_type":"sub_workflow","sub_workflow_id":"wf-child","max_nesting_depth":5}]}`
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows", body))

		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
		}
		if capturedStep == nil {
			t.Fatal("expected step to be created")
		}
		if capturedStep.SubWorkflowID != "wf-child" {
			t.Fatalf("SubWorkflowID = %q, want wf-child", capturedStep.SubWorkflowID)
		}
		if capturedStep.MaxNestingDepth != 5 {
			t.Fatalf("MaxNestingDepth = %d, want 5", capturedStep.MaxNestingDepth)
		}
		if capturedStep.StepType != domain.WorkflowStepTypeSubWorkflow {
			t.Fatalf("StepType = %q, want sub_workflow", capturedStep.StepType)
		}
	})
}

func TestHandleTriggerWorkflowWithStepOverrides(t *testing.T) {
	t.Parallel()
	t.Run("step_overrides passed to engine", func(t *testing.T) {
		t.Parallel()
		var capturedOverrides []domain.StepOverride
		trigger := &mockWorkflowTrigger{
			triggerWorkflowFn: func(_ context.Context, workflowID, projectID string, _ json.RawMessage, _ string, stepOverrides []domain.StepOverride) (*domain.WorkflowRun, error) {
				if workflowID != "wf-1" || projectID != "proj-1" {
					t.Fatalf("unexpected trigger args: %s %s", workflowID, projectID)
				}
				capturedOverrides = stepOverrides
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: workflowID, ProjectID: projectID, Status: domain.WfStatusRunning}, nil
			},
		}
		ms := &APIStoreMock{
			GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: id, Enabled: true}, nil
			},
		}
		pub := &mockPublisher{publishFn: func(_ context.Context, _ string, _ []byte) error {
			return nil
		}}

		srv := newWorkflowTestServer(t, ms, &mockQueue{}, pub, trigger)
		w := httptest.NewRecorder()
		body := `{"project_id":"proj-1","step_overrides":[{"step_ref":"b","enabled":false}]}`
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows/wf-1/trigger", body))

		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
		}
		if len(capturedOverrides) != 1 {
			t.Fatalf("expected 1 override, got %d", len(capturedOverrides))
		}
		if capturedOverrides[0].StepRef != "b" || capturedOverrides[0].Enabled {
			t.Fatalf("unexpected override: %+v", capturedOverrides[0])
		}
	})

	t.Run("empty step_overrides does not fail", func(t *testing.T) {
		t.Parallel()
		trigger := &mockWorkflowTrigger{
			triggerWorkflowFn: func(_ context.Context, _, _ string, _ json.RawMessage, _ string, stepOverrides []domain.StepOverride) (*domain.WorkflowRun, error) {
				if len(stepOverrides) != 0 {
					t.Fatalf("expected nil/empty overrides, got %d", len(stepOverrides))
				}
				return &domain.WorkflowRun{ID: "wr-1", Status: domain.WfStatusRunning}, nil
			},
		}
		ms := &APIStoreMock{
			GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: id, Enabled: true}, nil
			},
		}
		pub := &mockPublisher{publishFn: func(_ context.Context, _ string, _ []byte) error {
			return nil
		}}

		srv := newWorkflowTestServer(t, ms, &mockQueue{}, pub, trigger)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows/wf-1/trigger", `{"project_id":"proj-1"}`))

		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("step_overrides error propagated", func(t *testing.T) {
		t.Parallel()
		trigger := &mockWorkflowTrigger{
			triggerWorkflowFn: func(_ context.Context, _, _ string, _ json.RawMessage, _ string, _ []domain.StepOverride) (*domain.WorkflowRun, error) {
				return nil, fmt.Errorf("apply step overrides: step override references unknown step_ref %q", "nonexistent")
			},
		}
		ms := &APIStoreMock{
			GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: id, Enabled: true}, nil
			},
		}

		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, trigger)
		w := httptest.NewRecorder()
		body := `{"project_id":"proj-1","step_overrides":[{"step_ref":"nonexistent","enabled":false}]}`
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows/wf-1/trigger", body))

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
		}
	})
}

func TestHandleCloneWorkflow(t *testing.T) {
	t.Parallel()
	t.Run("success with defaults", func(t *testing.T) {
		t.Parallel()
		stepsCopied := 0
		snapshotCreated := false
		expectedResourceClass := map[string]string{"a": "large", "b": "medium"}
		ms := &APIStoreMock{
			GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{
					ID:          id,
					ProjectID:   "proj-1",
					Name:        "Original",
					Slug:        "original",
					Enabled:     true,
					TimeoutSecs: 300,
				}, nil
			},
			ListStepsByWorkflowFunc: func(_ context.Context, workflowID string) ([]domain.WorkflowStep, error) {
				if workflowID != "wf-1" {
					t.Fatalf("expected source wf-1, got %s", workflowID)
				}
				return []domain.WorkflowStep{
					{ID: "step-a", WorkflowID: workflowID, JobID: "job-a", StepRef: "a", ResourceClass: "large"},
					{ID: "step-b", WorkflowID: workflowID, JobID: "job-b", StepRef: "b", DependsOn: []string{"a"}, ResourceClass: "medium"},
				}, nil
			},
			CreateWorkflowFunc: func(_ context.Context, wf *domain.Workflow) error {
				wf.ID = "wf-clone"
				if wf.Name != "Original (copy)" {
					t.Fatalf("expected default name 'Original (copy)', got %q", wf.Name)
				}
				if wf.Slug != "original-copy" {
					t.Fatalf("expected default slug 'original-copy', got %q", wf.Slug)
				}
				if wf.TimeoutSecs != 300 {
					t.Fatalf("expected timeout 300, got %d", wf.TimeoutSecs)
				}
				return nil
			},
			CreateWorkflowStepFunc: func(_ context.Context, step *domain.WorkflowStep) error {
				if step.WorkflowID != "wf-clone" {
					t.Fatalf("cloned step should belong to wf-clone, got %s", step.WorkflowID)
				}
				if want := expectedResourceClass[step.StepRef]; step.ResourceClass != want {
					t.Fatalf("step %s resource_class = %q, want %q", step.StepRef, step.ResourceClass, want)
				}
				stepsCopied++
				return nil
			},
			CreateWorkflowVersionSnapshotFunc: func(_ context.Context, workflowID string, _ int) error {
				if workflowID != "wf-clone" {
					t.Fatalf("snapshot for wrong workflow: %s", workflowID)
				}
				snapshotCreated = true
				return nil
			},
		}

		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows/wf-1/clone", `{}`))

		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
		}
		if stepsCopied != 2 {
			t.Fatalf("expected 2 steps copied, got %d", stepsCopied)
		}
		if !snapshotCreated {
			t.Fatal("expected version snapshot to be created")
		}
	})

	t.Run("success with custom name and slug", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: id, ProjectID: "proj-1", Name: "Original", Slug: "original", Enabled: true}, nil
			},
			ListStepsByWorkflowFunc: func(_ context.Context, _ string) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{ID: "s1", StepRef: "a", JobID: "j1"}}, nil
			},
			CreateWorkflowFunc: func(_ context.Context, wf *domain.Workflow) error {
				wf.ID = "wf-new"
				if wf.Name != "Custom Name" {
					t.Fatalf("expected name 'Custom Name', got %q", wf.Name)
				}
				if wf.Slug != "custom-slug" {
					t.Fatalf("expected slug 'custom-slug', got %q", wf.Slug)
				}
				if wf.ProjectID != "proj-1" {
					t.Fatalf("expected project proj-1, got %s", wf.ProjectID)
				}
				return nil
			},
			CreateWorkflowStepFunc: func(_ context.Context, _ *domain.WorkflowStep) error {
				return nil
			},
			CreateWorkflowVersionSnapshotFunc: func(_ context.Context, _ string, _ int) error {
				return nil
			},
		}

		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		body := `{"name":"Custom Name","slug":"custom-slug","project_id":"proj-1"}`
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows/wf-1/clone", body))

		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("rejects cross-project target", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: id, ProjectID: "proj-1", Name: "Original", Slug: "original", Enabled: true}, nil
			},
			ListStepsByWorkflowFunc: func(_ context.Context, _ string) ([]domain.WorkflowStep, error) {
				t.Fatal("ListStepsByWorkflow must not run for cross-project clone target")
				return nil, nil
			},
			CreateWorkflowFunc: func(_ context.Context, _ *domain.Workflow) error {
				t.Fatal("CreateWorkflow must not run for cross-project clone target")
				return nil
			},
		}

		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		body := `{"name":"Custom Name","slug":"custom-slug","project_id":"proj-2"}`
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows/wf-1/clone", body))

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("source workflow not found", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowFunc: func(_ context.Context, _ string) (*domain.Workflow, error) {
				return nil, store.ErrWorkflowNotFound
			},
		}

		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows/wf-missing/clone", `{}`))

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})

	t.Run("empty body uses defaults", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: id, ProjectID: "proj-1", Name: "Wf", Slug: "wf", Enabled: true}, nil
			},
			ListStepsByWorkflowFunc: func(_ context.Context, _ string) ([]domain.WorkflowStep, error) {
				return nil, nil
			},
			CreateWorkflowFunc: func(_ context.Context, wf *domain.Workflow) error {
				wf.ID = "wf-c"
				if wf.Name != "Wf (copy)" {
					t.Fatalf("expected 'Wf (copy)', got %q", wf.Name)
				}
				return nil
			},
			CreateWorkflowVersionSnapshotFunc: func(_ context.Context, _ string, _ int) error {
				return nil
			},
		}

		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		// Send without body — handler should handle gracefully.
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows/wf-1/clone", ""))

		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
		}
	})
}

func TestHandleRetryWorkflowStep_ErrorPaths(t *testing.T) {
	t.Parallel()

	t.Run("step_not_found", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", Status: domain.WfStatusPaused}, nil
			},
			GetStepRunByWorkflowRunAndRefFunc: func(_ context.Context, _ string, _ string) (*domain.WorkflowStepRun, error) {
				return nil, nil
			},
		}
		cb := &mockWorkflowTrigger{}
		srv := newWorkflowTestServerWithCallback(t, ms, &mockQueue{}, nil, cb, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/wr-1/steps/review/retry", ""))

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("step_not_terminal", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", Status: domain.WfStatusRunning}, nil
			},
			GetStepRunByWorkflowRunAndRefFunc: func(_ context.Context, _ string, _ string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{ID: "sr-1", StepRef: "review", Status: domain.StepRunning}, nil
			},
		}
		cb := &mockWorkflowTrigger{}
		srv := newWorkflowTestServerWithCallback(t, ms, &mockQueue{}, nil, cb, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/wr-1/steps/review/retry", ""))

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("run_not_found", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return nil, store.ErrWorkflowRunNotFound
			},
		}
		cb := &mockWorkflowTrigger{}
		srv := newWorkflowTestServerWithCallback(t, ms, &mockQueue{}, nil, cb, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/wr-1/steps/review/retry", ""))

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("reset_error", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", Status: domain.WfStatusPaused}, nil
			},
			GetStepRunByWorkflowRunAndRefFunc: func(_ context.Context, _ string, _ string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{ID: "sr-1", StepRef: "review", Status: domain.StepFailed}, nil
			},
			UpdateStepRunStatusFunc: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return errors.New("db write failed")
			},
		}
		cb := &mockWorkflowTrigger{}
		srv := newWorkflowTestServerWithCallback(t, ms, &mockQueue{}, nil, cb, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/wr-1/steps/review/retry", ""))

		if w.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
		}
	})
}

func TestHandleReplayWorkflowSubtree_ErrorPaths(t *testing.T) {
	t.Parallel()

	t.Run("run_not_found", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return nil, store.ErrWorkflowRunNotFound
			},
		}
		cb := &mockWorkflowTrigger{}
		srv := newWorkflowTestServerWithCallback(t, ms, &mockQueue{}, nil, cb, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/wr-1/steps/a/replay-subtree", ""))

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("step_not_in_version", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1}, nil
			},
			ListStepsByWorkflowVersionFunc: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{StepRef: "x"}, {StepRef: "y"}}, nil
			},
		}
		cb := &mockWorkflowTrigger{}
		srv := newWorkflowTestServerWithCallback(t, ms, &mockQueue{}, nil, cb, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/wr-1/steps/missing/replay-subtree", ""))

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("callback_unavailable", func(t *testing.T) {
		t.Parallel()
		srv := newWorkflowTestServer(t, &APIStoreMock{}, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/wr-1/steps/a/replay-subtree", ""))

		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
		}
	})
}

func TestHandleApproveWorkflowStep_ErrorPaths(t *testing.T) {
	t.Parallel()

	t.Run("callback_unavailable", func(t *testing.T) {
		t.Parallel()
		srv := newWorkflowTestServer(t, &APIStoreMock{}, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		req := authedRequest(http.MethodPost, "/v1/workflow-runs/wr-1/steps/review/approve", `{"approver":"alice"}`)
		req.Header.Set("X-Actor-Id", "user:alice")
		srv.ServeHTTP(w, req)

		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("callback_error", func(t *testing.T) {
		t.Parallel()
		cb := &mockWorkflowTrigger{
			approveStepFn: func(_ context.Context, _, _, _ string) error {
				return errors.New("approval rejected")
			},
		}
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", Status: domain.WfStatusPaused}, nil
			},
		}
		srv := newWorkflowTestServerWithCallback(t, ms, &mockQueue{}, nil, cb, nil)
		w := httptest.NewRecorder()
		req := authedRequest(http.MethodPost, "/v1/workflow-runs/wr-1/steps/review/approve", `{"approver":"alice"}`)
		req.Header.Set("X-Actor-Id", "user:alice")
		srv.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("cross_project_returns_not_found_without_callback", func(t *testing.T) {
		t.Parallel()
		callbackCalled := false
		cb := &mockWorkflowTrigger{
			approveStepFn: func(_ context.Context, _, _, _ string) error {
				callbackCalled = true
				return nil
			},
		}
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", ProjectID: "proj-other", Status: domain.WfStatusRunning}, nil
			},
		}
		srv := newWorkflowTestServerWithCallback(t, ms, &mockQueue{}, nil, cb, nil)
		w := httptest.NewRecorder()
		req := authedProjectRequest(http.MethodPost, "/v1/workflow-runs/wr-1/steps/review/approve", `{"approver":"alice"}`, "proj-1")
		req.Header.Set("X-Actor-Id", "user:alice")
		srv.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
		}
		if callbackCalled {
			t.Fatal("approve callback should not be called on project mismatch")
		}
	})

	t.Run("workflow_run_not_found_returns_not_found_without_callback", func(t *testing.T) {
		t.Parallel()
		callbackCalled := false
		cb := &mockWorkflowTrigger{
			approveStepFn: func(_ context.Context, _, _, _ string) error {
				callbackCalled = true
				return nil
			},
		}
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return nil, store.ErrWorkflowRunNotFound
			},
		}
		srv := newWorkflowTestServerWithCallback(t, ms, &mockQueue{}, nil, cb, nil)
		w := httptest.NewRecorder()
		req := authedProjectRequest(http.MethodPost, "/v1/workflow-runs/wr-1/steps/review/approve", `{"approver":"alice"}`, "proj-1")
		req.Header.Set("X-Actor-Id", "user:alice")
		srv.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
		}
		if callbackCalled {
			t.Fatal("approve callback should not be called when workflow run is missing")
		}
	})
}

func TestHandleWorkflowPlan_ErrorPaths(t *testing.T) {
	t.Parallel()

	t.Run("invalid_body", func(t *testing.T) {
		t.Parallel()
		srv := newWorkflowTestServer(t, &APIStoreMock{}, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows/wf-1/plan", `{invalid`))

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("workflow_not_found", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowFunc: func(_ context.Context, _ string) (*domain.Workflow, error) {
				return nil, nil
			},
		}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows/wf-1/plan", `{}`))

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
		}
	})
}

func TestHandleUpsertWorkflowPolicy_ErrorPaths(t *testing.T) {
	t.Parallel()

	t.Run("invalid_body", func(t *testing.T) {
		t.Parallel()
		srv := newWorkflowTestServer(t, &APIStoreMock{}, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPut, "/v1/workflow-policies/proj-1", `{invalid`))

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("store_error", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			UpsertWorkflowPolicyFunc: func(_ context.Context, _ *domain.WorkflowPolicy) error {
				return errors.New("db write failed")
			},
		}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPut, "/v1/workflow-policies/proj-1", `{}`))

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
		}
	})
}

func TestHandleUpsertWorkflowPolicy_APIKeyRejected(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		UpsertWorkflowPolicyFunc: func(_ context.Context, _ *domain.WorkflowPolicy) error {
			t.Fatal("UpsertWorkflowPolicy must not be called for API-key policy changes")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxScopesKey, []string{domain.ScopeWorkflowsWrite})

	_, err := srv.handleUpsertWorkflowPolicy(ctx, &UpsertWorkflowPolicyInput{
		ProjectID: "proj-1",
		Body:      upsertWorkflowPolicyRequest{MaxFanOut: 0, MaxDepth: 0},
	})
	if !isHumaStatusError(err, http.StatusForbidden) {
		t.Fatalf("expected 403, got %v", err)
	}
}

func TestHandleUpsertWorkflowPolicy_WorkflowAuthorRejected(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		UpsertWorkflowPolicyFunc: func(_ context.Context, _ *domain.WorkflowPolicy) error {
			t.Fatal("UpsertWorkflowPolicy must not be called for workflow authors without rbac:manage")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")
	ctx = context.WithValue(ctx, ctxActorIDKey, "user-workflow-author")
	ctx = context.WithValue(ctx, ctxScopesKey, []string{domain.ScopeWorkflowsWrite})

	_, err := srv.handleUpsertWorkflowPolicy(ctx, &UpsertWorkflowPolicyInput{
		ProjectID: "proj-1",
		Body:      upsertWorkflowPolicyRequest{MaxFanOut: 0, MaxDepth: 0},
	})
	if !isHumaStatusError(err, http.StatusForbidden) {
		t.Fatalf("expected 403, got %v", err)
	}
}

func TestHandleUpsertWorkflowPolicy_ProFullRBACRejectsAdvancedPolicy(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetUserPermissionsFunc: func(_ context.Context, projectID, actorID string) ([]string, error) {
			if projectID != "proj-1" || actorID != "user-rbac-manager" {
				t.Fatalf("permission lookup = %s %s", projectID, actorID)
			}
			return []string{domain.ScopeRBACManage}, nil
		},
		UpsertWorkflowPolicyFunc: func(_ context.Context, _ *domain.WorkflowPolicy) error {
			t.Fatal("UpsertWorkflowPolicy must not run below Advanced RBAC")
			return nil
		},
	}
	enforcer := &tunableLimitsEnforcer{limits: billing.GetPlanLimits(domain.PlanPro)}
	srv := newServerWithEnforcer(t, ms, &mockQueue{}, enforcer)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")
	ctx = context.WithValue(ctx, ctxActorIDKey, "user-rbac-manager")
	ctx = context.WithValue(ctx, ctxScopesKey, []string{domain.ScopeRBACManage})

	_, err := srv.handleUpsertWorkflowPolicy(ctx, &UpsertWorkflowPolicyInput{
		ProjectID: "proj-1",
		Body:      upsertWorkflowPolicyRequest{MaxFanOut: 2, MaxDepth: 3},
	})
	if !isHumaStatusError(err, http.StatusForbidden) {
		t.Fatalf("expected 403, got %v", err)
	}
}

func TestHandleGetWorkflowPolicy_ProFullRBACRejectsAdvancedPolicy(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetWorkflowPolicyByProjectFunc: func(context.Context, string) (*domain.WorkflowPolicy, error) {
			t.Fatal("GetWorkflowPolicyByProject must not run below Advanced RBAC")
			return nil, nil
		},
	}
	enforcer := &tunableLimitsEnforcer{limits: billing.GetPlanLimits(domain.PlanPro)}
	srv := newServerWithEnforcer(t, ms, &mockQueue{}, enforcer)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")

	_, err := srv.handleGetWorkflowPolicy(ctx, &GetWorkflowPolicyInput{ProjectID: "proj-1"})
	if !isHumaStatusError(err, http.StatusForbidden) {
		t.Fatalf("expected 403, got %v", err)
	}
}

func TestHandleUpsertWorkflowPolicy_RBACManagerAllowed(t *testing.T) {
	t.Parallel()

	var upserted bool
	ms := &APIStoreMock{
		GetUserPermissionsFunc: func(_ context.Context, projectID, actorID string) ([]string, error) {
			if projectID != "proj-1" || actorID != "user-rbac-manager" {
				t.Fatalf("permission lookup = %s %s", projectID, actorID)
			}
			return []string{domain.ScopeRBACManage}, nil
		},
		UpsertWorkflowPolicyFunc: func(_ context.Context, p *domain.WorkflowPolicy) error {
			upserted = true
			if p.ProjectID != "proj-1" || p.MaxFanOut != 2 {
				t.Fatalf("policy = %+v", p)
			}
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")
	ctx = context.WithValue(ctx, ctxActorIDKey, "user-rbac-manager")
	ctx = context.WithValue(ctx, ctxScopesKey, []string{domain.ScopeRBACManage})

	_, err := srv.handleUpsertWorkflowPolicy(ctx, &UpsertWorkflowPolicyInput{
		ProjectID: "proj-1",
		Body:      upsertWorkflowPolicyRequest{MaxFanOut: 2, MaxDepth: 3},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !upserted {
		t.Fatal("expected workflow policy upsert")
	}
}

func TestHandleWorkflowVersionDiff_ErrorPaths(t *testing.T) {
	t.Parallel()

	t.Run("from_not_found", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: id, ProjectID: "proj-1"}, nil
			},
			GetWorkflowVersionByVersionIDFunc: func(_ context.Context, _ string, _ string) (*domain.WorkflowVersion, error) {
				return nil, errors.New("not found")
			},
		}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflows/wf-1/versions/v-missing/diff/v2", ""))

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("to_not_found", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: id, ProjectID: "proj-1"}, nil
			},
			GetWorkflowVersionByVersionIDFunc: func(_ context.Context, _ string, versionID string) (*domain.WorkflowVersion, error) {
				if versionID == "v1" {
					return &domain.WorkflowVersion{ID: "v1", Version: 1}, nil
				}
				return nil, errors.New("not found")
			},
		}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflows/wf-1/versions/v1/diff/v-missing", ""))

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
		}
	})
}

func TestHandleWorkflowVersionImpact_ErrorPaths(t *testing.T) {
	t.Parallel()

	t.Run("version_not_found", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: id, ProjectID: "proj-1"}, nil
			},
			GetWorkflowVersionByVersionIDFunc: func(_ context.Context, _ string, _ string) (*domain.WorkflowVersion, error) {
				return nil, errors.New("not found")
			},
		}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflows/wf-1/versions/v-missing/impact", ""))

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
		}
	})
}

func TestHandleListWorkflows_TagFilter(t *testing.T) {
	t.Parallel()
	called := false
	ms := &APIStoreMock{
		ListWorkflowsByTagFunc: func(_ context.Context, projectID, tagKey, tagValue string, _ int, _ *time.Time) ([]domain.Workflow, error) {
			called = true
			if projectID != "proj-1" || tagKey != "env" || tagValue != "prod" {
				t.Fatalf("unexpected args: %s %s %s", projectID, tagKey, tagValue)
			}
			return []domain.Workflow{{ID: "wf-1", ProjectID: projectID}}, nil
		},
	}

	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/workflows?tag_key=env&tag_value=prod", "", "proj-1"))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !called {
		t.Fatal("expected ListWorkflowsByTag to be called")
	}
}

func TestHandleListWorkflowRunsByProject_TagFilter(t *testing.T) {
	t.Parallel()
	called := false
	ms := &APIStoreMock{
		ListWorkflowRunsByTagFunc: func(_ context.Context, projectID, tagKey, tagValue string, _ int, _ *time.Time) ([]domain.WorkflowRun, error) {
			called = true
			if projectID != "proj-1" || tagKey != "env" || tagValue != "staging" {
				t.Fatalf("unexpected args: %s %s %s", projectID, tagKey, tagValue)
			}
			return []domain.WorkflowRun{{ID: "wr-1", ProjectID: projectID}}, nil
		},
	}

	srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/workflow-runs?tag_key=env&tag_value=staging", "", "proj-1"))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !called {
		t.Fatal("expected ListWorkflowRunsByTag to be called")
	}
}

func TestHandlePauseWorkflowRun_MarksJobRunsPaused(t *testing.T) {
	t.Parallel()

	markCalled := false
	getCalls := 0
	ms := &APIStoreMock{
		GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
			getCalls++
			if getCalls == 1 {
				return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", Status: domain.WfStatusRunning}, nil
			}
			return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", Status: domain.WfStatusPaused}, nil
		},
		UpdateWorkflowRunStatusFunc: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
			return nil
		},
		MarkJobRunsPausedByWorkflowRunFunc: func(_ context.Context, wfRunID string) (int64, error) {
			if wfRunID != "wr-1" {
				t.Fatalf("expected wr-1, got %s", wfRunID)
			}
			markCalled = true
			return 2, nil
		},
	}

	pub := &mockPublisher{}
	srv := newWorkflowTestServer(t, ms, &mockQueue{}, pub, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/wr-1/pause", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !markCalled {
		t.Fatal("expected MarkJobRunsPausedByWorkflowRun to be called")
	}
}

func TestHandlePauseWorkflowRun_NoContainerRuntime(t *testing.T) {
	t.Parallel()

	var getCalls int
	ms := &APIStoreMock{
		GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
			getCalls++
			if getCalls == 1 {
				return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", Status: domain.WfStatusRunning}, nil
			}
			return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", Status: domain.WfStatusPaused}, nil
		},
		UpdateWorkflowRunStatusFunc: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
			return nil
		},
	}

	// Server without container runtime — should not panic.
	pub := &mockPublisher{}
	srv := newWorkflowTestServer(t, ms, &mockQueue{}, pub, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/wr-1/pause", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlePauseWorkflowRun_AlreadyPaused(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", Status: domain.WfStatusPaused}, nil
		},
	}

	srv := newWorkflowTestServer(t, ms, &mockQueue{}, &mockPublisher{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/wr-1/pause", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected idempotent 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlePauseWorkflowRun_TerminalState(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", Status: domain.WfStatusCompleted}, nil
		},
	}

	srv := newWorkflowTestServer(t, ms, &mockQueue{}, &mockPublisher{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/wr-1/pause", ""))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPublishWorkflowRunHook_FiresWebhook(t *testing.T) {
	t.Parallel()

	var webhookCreated atomic.Bool
	webhookDone := make(chan struct{}, 1)
	ms := &APIStoreMock{
		ListWebhookSubscriptionsFunc: func(_ context.Context, projectID string) ([]domain.WebhookSubscription, error) {
			return []domain.WebhookSubscription{
				{ID: "sub-1", ProjectID: projectID, WebhookURL: "https://example.com/hook", EventTypes: []string{domain.WebhookEventWorkflowCompleted}, Active: true},
			}, nil
		},
		CreateWebhookDeliveryFunc: func(_ context.Context, d *domain.WebhookDelivery) error {
			webhookCreated.Store(true)
			select {
			case webhookDone <- struct{}{}:
			default:
			}
			if d.WebhookURL != "https://example.com/hook" {
				t.Errorf("expected webhook URL, got %s", d.WebhookURL)
			}
			if d.ProjectID != "proj-1" {
				t.Errorf("ProjectID = %q, want proj-1", d.ProjectID)
			}
			if len(d.Payload) == 0 {
				t.Error("expected workflow hook payload to be stored in Payload")
			}
			if d.LastError != "" {
				t.Errorf("LastError = %q, want empty for pending workflow hook delivery", d.LastError)
			}
			return nil
		},
	}

	pub := &mockPublisher{}
	srv := newWorkflowTestServer(t, ms, &mockQueue{}, pub, nil)
	srv.publishWorkflowRunHook(context.Background(),
		&domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", ProjectID: "proj-1", Status: domain.WfStatusCompleted},
		domain.WfStatusRunning,
		domain.WfStatusCompleted,
		"completed",
	)

	select {
	case <-webhookDone:
	case <-time.After(2 * time.Second):
		t.Error("expected webhook delivery to be created for pause event")
	}
}

func TestPublishWorkflowRunHook_SkipsUncreatableWorkflowRunReason(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		ListWebhookSubscriptionsFunc: func(_ context.Context, projectID string) ([]domain.WebhookSubscription, error) {
			return []domain.WebhookSubscription{
				{ID: "legacy-sub", ProjectID: projectID, WebhookURL: "https://example.com/hook", EventTypes: []string{"workflow_run.pause"}, Active: true},
			}, nil
		},
		CreateWebhookDeliveryFunc: func(context.Context, *domain.WebhookDelivery) error {
			t.Fatal("workflow_run.pause is not creatable and must not enqueue subscription deliveries")
			return nil
		},
	}

	srv := newWorkflowTestServer(t, ms, &mockQueue{}, &mockPublisher{}, nil)
	srv.publishWorkflowRunHook(context.Background(),
		&domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", ProjectID: "proj-1", Status: domain.WfStatusPaused},
		domain.WfStatusRunning,
		domain.WfStatusPaused,
		"pause",
	)
	time.Sleep(20 * time.Millisecond)
}

func TestPublishWorkflowRunHook_NilDelivery(t *testing.T) {
	t.Parallel()

	var getCalls atomic.Int32
	ms := &APIStoreMock{
		GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
			n := getCalls.Add(1)
			if n == 1 {
				return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", ProjectID: "proj-1", Status: domain.WfStatusRunning}, nil
			}
			return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", ProjectID: "proj-1", Status: domain.WfStatusPaused}, nil
		},
		UpdateWorkflowRunStatusFunc: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
			return nil
		},
		ListWebhookSubscriptionsFunc: func(_ context.Context, _ string) ([]domain.WebhookSubscription, error) {
			return nil, nil // No subscriptions
		},
	}

	pub := &mockPublisher{}
	srv := newWorkflowTestServer(t, ms, &mockQueue{}, pub, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/wr-1/pause", ""))

	// Should succeed without panic when no webhook subscriptions exist.
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}
