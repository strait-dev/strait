//go:build integration

package e2e_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"strait/internal/api"
	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/workflow"
)

func mustCleanWf(t *testing.T) {
	t.Helper()
	if err := testEnv.DB.CleanTables(context.Background()); err != nil {
		t.Fatalf("clean tables: %v", err)
	}
}

func wfSetup(t *testing.T) *api.Server {
	t.Helper()

	engine := workflow.NewWorkflowEngine(testStore, testQueue, slog.Default())
	callback := workflow.NewStepCallback(testStore, engine, slog.Default())

	return api.NewServer(api.ServerDeps{
		Config: &config.Config{
			InternalSecret:           "test-secret",
			JWTSigningKey:            "test-jwt-key-must-be-at-least-32-chars-long",
			RateLimitRequests:        5000,
			RateLimitWindow:          time.Minute,
			TriggerRateLimitRequests: 5000,
			TriggerRateLimitWindow:   time.Minute,
			CORSAllowedOrigins:       []string{"*"},
			CORSAllowCredentials:     false,
			FFEventTriggers:          true,
		},
		Store:            testStore,
		Queue:            testQueue,
		WorkflowEngine:   engine,
		WorkflowCallback: callback,
	})
}

func wfDoReq(t *testing.T, srv *api.Server, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()

	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
	}
	req.Header.Set("X-Internal-Secret", "test-secret")
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	return w
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return string(b)
}

func wfCreateJob(t *testing.T, srv *api.Server, projectID, name, slug string) map[string]any {
	t.Helper()
	body := map[string]any{
		"project_id":     projectID,
		"name":           name,
		"slug":           slug,
		"description":    name + " description",
		"payload_schema": map[string]any{"type": "object"},
		"endpoint_url":   "https://example.com/" + slug,
		"max_attempts":   3,
		"timeout_secs":   60,
		"run_ttl_secs":   600,
	}
	w := wfDoReq(t, srv, http.MethodPost, "/v1/jobs/", mustJSON(t, body))
	if w.Code != http.StatusCreated {
		t.Fatalf("create job status = %d, body = %s", w.Code, w.Body.String())
	}
	return mustDecodeObject(t, w)
}

func wfCreateWorkflow(t *testing.T, srv *api.Server, projectID, name, slug string, steps []map[string]any) map[string]any {
	t.Helper()
	body := map[string]any{
		"project_id": projectID,
		"name":       name,
		"slug":       slug,
		"steps":      steps,
	}
	w := wfDoReq(t, srv, http.MethodPost, "/v1/workflows/", mustJSON(t, body))
	if w.Code != http.StatusCreated {
		t.Fatalf("create workflow status = %d, body = %s", w.Code, w.Body.String())
	}
	return mustDecodeObject(t, w)
}

func wfTriggerWorkflow(t *testing.T, srv *api.Server, workflowID, projectID string, payload map[string]any, stepOverrides []domain.StepOverride) map[string]any {
	t.Helper()
	body := map[string]any{"project_id": projectID}
	if payload != nil {
		body["payload"] = payload
	}
	if len(stepOverrides) > 0 {
		body["step_overrides"] = stepOverrides
	}

	w := wfDoReq(t, srv, http.MethodPost, "/v1/workflows/"+workflowID+"/trigger", mustJSON(t, body))
	if w.Code != http.StatusCreated {
		t.Fatalf("trigger workflow status = %d, body = %s", w.Code, w.Body.String())
	}
	return mustDecodeObject(t, w)
}

func findStepRunByRef(t *testing.T, stepRuns []domain.WorkflowStepRun, stepRef string) domain.WorkflowStepRun {
	t.Helper()
	for _, sr := range stepRuns {
		if sr.StepRef == stepRef {
			return sr
		}
	}
	t.Fatalf("step run not found for %s", stepRef)
	return domain.WorkflowStepRun{}
}

func TestE2E_WorkflowRetryFromFailed(t *testing.T) {
	mustCleanWf(t)
	srv := wfSetup(t)
	ctx := context.Background()

	projectID := "proj-wf-retry-" + newID()
	jobA := wfCreateJob(t, srv, projectID, "Step A Job", "wf-step-a-"+newID())
	jobB := wfCreateJob(t, srv, projectID, "Step B Job", "wf-step-b-"+newID())

	wf := wfCreateWorkflow(t, srv, projectID, "Retry Workflow", "wf-retry-"+newID(), []map[string]any{
		{"step_ref": "A", "job_id": asString(t, jobA, "id")},
		{"step_ref": "B", "job_id": asString(t, jobB, "id"), "depends_on": []string{"A"}},
	})

	triggered := wfTriggerWorkflow(t, srv, asString(t, wf, "id"), projectID, map[string]any{"seed": "x"}, nil)
	originalRunID := asString(t, triggered, "id")

	origStepRuns, err := testStore.ListStepRunsByWorkflowRun(ctx, originalRunID, 10000, nil)
	if err != nil {
		t.Fatalf("list original step runs: %v", err)
	}
	stepA := findStepRunByRef(t, origStepRuns, "A")
	stepB := findStepRunByRef(t, origStepRuns, "B")

	if err := testStore.UpdateStepRunStatus(ctx, stepA.ID, domain.StepCompleted, map[string]any{
		"finished_at": time.Now().UTC(),
		"output":      json.RawMessage(`{"ok":true}`),
	}); err != nil {
		t.Fatalf("complete step A: %v", err)
	}

	if err := testStore.UpdateStepRunStatus(ctx, stepB.ID, domain.StepFailed, map[string]any{
		"finished_at": time.Now().UTC(),
		"error":       "step b failed",
	}); err != nil {
		t.Fatalf("fail step B: %v", err)
	}

	if err := testStore.UpdateWorkflowRunStatus(ctx, originalRunID, domain.WfStatusRunning, domain.WfStatusFailed, map[string]any{
		"finished_at": time.Now().UTC(),
		"error":       "failed for retry test",
	}); err != nil {
		t.Fatalf("fail workflow run: %v", err)
	}

	retryResp := wfDoReq(t, srv, http.MethodPost, "/v1/workflow-runs/"+originalRunID+"/retry", "")
	if retryResp.Code != http.StatusCreated {
		t.Fatalf("retry workflow run status = %d, body = %s", retryResp.Code, retryResp.Body.String())
	}
	newRun := mustDecodeObject(t, retryResp)
	newRunID := asString(t, newRun, "id")

	if asString(t, newRun, "status") != string(domain.WfStatusRunning) {
		t.Fatalf("expected new run status running, got %s", asString(t, newRun, "status"))
	}
	if asString(t, newRun, "retry_of_run_id") != originalRunID {
		t.Fatalf("expected retry_of_run_id=%s, got %s", originalRunID, asString(t, newRun, "retry_of_run_id"))
	}

	newStepRuns, err := testStore.ListStepRunsByWorkflowRun(ctx, newRunID, 10000, nil)
	if err != nil {
		t.Fatalf("list retry step runs: %v", err)
	}
	newStepA := findStepRunByRef(t, newStepRuns, "A")
	newStepB := findStepRunByRef(t, newStepRuns, "B")

	if newStepA.Status != domain.StepCompleted {
		t.Fatalf("expected step A completed on retry, got %s", newStepA.Status)
	}
	if newStepB.Status != domain.StepPending && newStepB.Status != domain.StepRunning {
		t.Fatalf("expected step B pending/running on retry, got %s", newStepB.Status)
	}
}

func TestE2E_WorkflowSkipStep(t *testing.T) {
	mustCleanWf(t)
	srv := wfSetup(t)
	ctx := context.Background()

	projectID := "proj-wf-skip-" + newID()
	wf := wfCreateWorkflow(t, srv, projectID, "Skip Workflow", "wf-skip-"+newID(), []map[string]any{
		{"step_ref": "A", "step_type": "approval", "approval_approvers": []string{"approver@example.com"}},
	})

	triggered := wfTriggerWorkflow(t, srv, asString(t, wf, "id"), projectID, nil, nil)
	runID := asString(t, triggered, "id")

	skipResp := wfDoReq(t, srv, http.MethodPost, "/v1/workflow-runs/"+runID+"/steps/A/skip", `{"reason":"testing"}`)
	if skipResp.Code != http.StatusOK {
		t.Fatalf("skip step status = %d, body = %s", skipResp.Code, skipResp.Body.String())
	}

	stepRuns, err := testStore.ListStepRunsByWorkflowRun(ctx, runID, 10000, nil)
	if err != nil {
		t.Fatalf("list step runs: %v", err)
	}
	stepA := findStepRunByRef(t, stepRuns, "A")
	if stepA.Status != domain.StepSkipped {
		t.Fatalf("expected step A skipped, got %s", stepA.Status)
	}

	run, err := testStore.GetWorkflowRun(ctx, runID)
	if err != nil {
		t.Fatalf("get workflow run: %v", err)
	}
	if run.Status != domain.WfStatusCompleted {
		t.Fatalf("expected completed workflow run, got %s", run.Status)
	}
}

func TestE2E_WorkflowForceCompleteStep(t *testing.T) {
	mustCleanWf(t)
	srv := wfSetup(t)
	ctx := context.Background()

	projectID := "proj-wf-force-complete-" + newID()
	wf := wfCreateWorkflow(t, srv, projectID, "Force Complete Workflow", "wf-force-complete-"+newID(), []map[string]any{
		{"step_ref": "A", "step_type": "approval", "approval_approvers": []string{"approver@example.com"}},
	})

	triggered := wfTriggerWorkflow(t, srv, asString(t, wf, "id"), projectID, nil, nil)
	runID := asString(t, triggered, "id")

	forceResp := wfDoReq(t, srv, http.MethodPost, "/v1/workflow-runs/"+runID+"/steps/A/force-complete", `{"result":{"key":"value"}}`)
	if forceResp.Code != http.StatusOK {
		t.Fatalf("force-complete step status = %d, body = %s", forceResp.Code, forceResp.Body.String())
	}

	stepRuns, err := testStore.ListStepRunsByWorkflowRun(ctx, runID, 10000, nil)
	if err != nil {
		t.Fatalf("list step runs: %v", err)
	}
	stepA := findStepRunByRef(t, stepRuns, "A")
	if stepA.Status != domain.StepCompleted {
		t.Fatalf("expected step A completed, got %s", stepA.Status)
	}

	var output map[string]any
	if err := json.Unmarshal(stepA.Output, &output); err != nil {
		t.Fatalf("unmarshal step output: %v", err)
	}
	if output["key"] != "value" {
		t.Fatalf("expected output key=value, got %v", output)
	}
}

func TestE2E_WorkflowClone(t *testing.T) {
	mustCleanWf(t)
	srv := wfSetup(t)

	projectID := "proj-wf-clone-" + newID()
	jobA := wfCreateJob(t, srv, projectID, "Clone Step A", "wf-clone-a-"+newID())
	jobB := wfCreateJob(t, srv, projectID, "Clone Step B", "wf-clone-b-"+newID())

	original := wfCreateWorkflow(t, srv, projectID, "Original Workflow", "wf-clone-src-"+newID(), []map[string]any{
		{"step_ref": "A", "job_id": asString(t, jobA, "id")},
		{"step_ref": "B", "job_id": asString(t, jobB, "id"), "depends_on": []string{"A"}},
	})
	originalID := asString(t, original, "id")

	cloneReq := map[string]any{"name": "cloned", "slug": "cloned-slug"}
	cloneResp := wfDoReq(t, srv, http.MethodPost, "/v1/workflows/"+originalID+"/clone", mustJSON(t, cloneReq))
	if cloneResp.Code != http.StatusCreated {
		t.Fatalf("clone workflow status = %d, body = %s", cloneResp.Code, cloneResp.Body.String())
	}
	cloned := mustDecodeObject(t, cloneResp)
	clonedID := asString(t, cloned, "id")

	if clonedID == originalID {
		t.Fatal("expected cloned workflow to have different id")
	}
	if asString(t, cloned, "name") != "cloned" {
		t.Fatalf("expected name cloned, got %s", asString(t, cloned, "name"))
	}
	if asString(t, cloned, "slug") != "cloned-slug" {
		t.Fatalf("expected slug cloned-slug, got %s", asString(t, cloned, "slug"))
	}

	wOrig := wfDoReq(t, srv, http.MethodGet, "/v1/workflows/"+originalID+"/", "")
	if wOrig.Code != http.StatusOK {
		t.Fatalf("get original workflow status = %d, body = %s", wOrig.Code, wOrig.Body.String())
	}
	origBody := mustDecodeObject(t, wOrig)

	wClone := wfDoReq(t, srv, http.MethodGet, "/v1/workflows/"+clonedID+"/", "")
	if wClone.Code != http.StatusOK {
		t.Fatalf("get cloned workflow status = %d, body = %s", wClone.Code, wClone.Body.String())
	}
	cloneBody := mustDecodeObject(t, wClone)

	origSteps, ok := origBody["steps"].([]any)
	if !ok {
		t.Fatalf("original steps is not an array: %T", origBody["steps"])
	}
	cloneSteps, ok := cloneBody["steps"].([]any)
	if !ok {
		t.Fatalf("cloned steps is not an array: %T", cloneBody["steps"])
	}
	if len(origSteps) != len(cloneSteps) {
		t.Fatalf("expected cloned workflow to keep %d steps, got %d", len(origSteps), len(cloneSteps))
	}
}

func TestE2E_WorkflowRunLabels(t *testing.T) {
	mustCleanWf(t)
	srv := wfSetup(t)
	ctx := context.Background()

	projectID := "proj-wf-labels-" + newID()
	job := wfCreateJob(t, srv, projectID, "Labels Job", "wf-labels-job-"+newID())
	wf := wfCreateWorkflow(t, srv, projectID, "Labels Workflow", "wf-labels-"+newID(), []map[string]any{
		{"step_ref": "A", "job_id": asString(t, job, "id")},
	})

	triggered := wfTriggerWorkflow(t, srv, asString(t, wf, "id"), projectID, nil, nil)
	runID := asString(t, triggered, "id")

	emptyResp := wfDoReq(t, srv, http.MethodGet, "/v1/workflow-runs/"+runID+"/labels", "")
	if emptyResp.Code != http.StatusOK {
		t.Fatalf("get labels status = %d, body = %s", emptyResp.Code, emptyResp.Body.String())
	}
	emptyLabelsBody := mustDecodeObject(t, emptyResp)
	emptyLabels, ok := emptyLabelsBody["labels"].(map[string]any)
	if !ok {
		t.Fatalf("labels is not object: %T", emptyLabelsBody["labels"])
	}
	if len(emptyLabels) != 0 {
		t.Fatalf("expected empty labels, got %v", emptyLabels)
	}

	if err := testStore.CreateWorkflowRunLabels(ctx, runID, map[string]string{"env": "test", "owner": "e2e"}); err != nil {
		t.Fatalf("create workflow run labels: %v", err)
	}

	labelsResp := wfDoReq(t, srv, http.MethodGet, "/v1/workflow-runs/"+runID+"/labels", "")
	if labelsResp.Code != http.StatusOK {
		t.Fatalf("get labels status = %d, body = %s", labelsResp.Code, labelsResp.Body.String())
	}
	labelsBody := mustDecodeObject(t, labelsResp)
	labels, ok := labelsBody["labels"].(map[string]any)
	if !ok {
		t.Fatalf("labels is not object: %T", labelsBody["labels"])
	}
	if labels["env"] != "test" || labels["owner"] != "e2e" {
		t.Fatalf("unexpected labels: %v", labels)
	}
}

func TestE2E_WorkflowTemplateSubstitution(t *testing.T) {
	mustCleanWf(t)
	srv := wfSetup(t)
	ctx := context.Background()

	projectID := "proj-wf-template-" + newID()
	job := wfCreateJob(t, srv, projectID, "Template Job", "wf-template-job-"+newID())
	wf := wfCreateWorkflow(t, srv, projectID, "Template Workflow", "wf-template-"+newID(), []map[string]any{
		{
			"step_ref": "A",
			"job_id":   asString(t, job, "id"),
			"payload": map[string]any{
				"message": "hello {{var_name}}",
				"value":   "{{var_name}}",
			},
		},
	})

	triggered := wfTriggerWorkflow(t, srv, asString(t, wf, "id"), projectID, map[string]any{"var_name": "resolved_value"}, nil)
	runID := asString(t, triggered, "id")

	stepRuns, err := testStore.ListStepRunsByWorkflowRun(ctx, runID, 10000, nil)
	if err != nil {
		t.Fatalf("list step runs: %v", err)
	}
	stepA := findStepRunByRef(t, stepRuns, "A")
	if stepA.JobRunID == "" {
		t.Fatal("expected step A job run id to be set")
	}

	jobRun, err := testStore.GetRun(ctx, stepA.JobRunID)
	if err != nil {
		t.Fatalf("get job run: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(jobRun.Payload, &payload); err != nil {
		t.Fatalf("unmarshal job run payload: %v", err)
	}
	if payload["message"] != "hello resolved_value" {
		t.Fatalf("expected rendered message, got %v", payload["message"])
	}
	if payload["value"] != "resolved_value" {
		t.Fatalf("expected rendered value, got %v", payload["value"])
	}
}

func TestE2E_WorkflowStepOverrides(t *testing.T) {
	mustCleanWf(t)
	srv := wfSetup(t)
	ctx := context.Background()

	projectID := "proj-wf-overrides-" + newID()
	jobA := wfCreateJob(t, srv, projectID, "Override A", "wf-override-a-"+newID())
	jobB := wfCreateJob(t, srv, projectID, "Override B", "wf-override-b-"+newID())

	wf := wfCreateWorkflow(t, srv, projectID, "Overrides Workflow", "wf-overrides-"+newID(), []map[string]any{
		{"step_ref": "A", "job_id": asString(t, jobA, "id")},
		{"step_ref": "B", "job_id": asString(t, jobB, "id")},
	})

	triggered := wfTriggerWorkflow(t, srv, asString(t, wf, "id"), projectID, nil, []domain.StepOverride{{StepRef: "B", Enabled: false}})
	runID := asString(t, triggered, "id")

	stepRuns, err := testStore.ListStepRunsByWorkflowRun(ctx, runID, 10000, nil)
	if err != nil {
		t.Fatalf("list step runs: %v", err)
	}
	if len(stepRuns) != 1 {
		t.Fatalf("expected exactly 1 step run after override, got %d", len(stepRuns))
	}
	if stepRuns[0].StepRef != "A" {
		t.Fatalf("expected only step A, got %s", stepRuns[0].StepRef)
	}
}
