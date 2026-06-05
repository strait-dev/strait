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

	"github.com/stretchr/testify/require"
)

func mustCleanWf(t *testing.T) {
	t.Helper()
	require.NoError(t, testEnv.
		DB.CleanTables(
		context.Background()))

}

func wfSetup(t *testing.T) *api.Server {
	t.Helper()

	engine := workflow.NewWorkflowEngine(testStore, testQueue, slog.Default())
	callback := workflow.NewStepCallback(testStore, engine, slog.Default())

	return api.NewServer(api.ServerDeps{
		Config: &config.Config{
			InternalSecret:           "test-secret-value",
			JWTSigningKey:            testJWTSigningKey,
			RateLimitRequests:        5000,
			RateLimitWindow:          time.Minute,
			TriggerRateLimitRequests: 5000,
			TriggerRateLimitWindow:   time.Minute,
			CORSAllowedOrigins:       []string{"*"},
			CORSAllowCredentials:     false,
			MaxBulkTriggerItems:      500,
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
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	req.Header.Set("X-Actor-Id", "test:e2e-actor")
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	return w
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)

	return string(b)
}

func wfEnsureProject(t *testing.T, projectID string) {
	t.Helper()
	require.NoError(t, testStore.
		CreateProject(context.Background(), &domain.
			Project{ID: projectID, OrgID: "org-" +
			projectID,
			Name: projectID,
		}))

}

func wfCreateJob(t *testing.T, srv *api.Server, projectID, name, slug string) map[string]any {
	t.Helper()
	wfEnsureProject(t, projectID)
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
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

	return mustDecodeObject(t, w)
}

func wfCreateWorkflow(t *testing.T, srv *api.Server, projectID, name, slug string, steps []map[string]any) map[string]any {
	t.Helper()
	wfEnsureProject(t, projectID)
	body := map[string]any{
		"project_id": projectID,
		"name":       name,
		"slug":       slug,
		"steps":      steps,
	}
	w := wfDoReq(t, srv, http.MethodPost, "/v1/workflows/", mustJSON(t, body))
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

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
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

	return mustDecodeObject(t, w)
}

func findStepRunByRef(t *testing.T, stepRuns []domain.WorkflowStepRun, stepRef string) domain.WorkflowStepRun {
	t.Helper()
	for _, sr := range stepRuns {
		if sr.StepRef == stepRef {
			return sr
		}
	}
	require.Failf(t, "test failure",

		"step run not found for %s", stepRef)
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
	require.NoError(t, err)

	stepA := findStepRunByRef(t, origStepRuns, "A")
	stepB := findStepRunByRef(t, origStepRuns, "B")
	require.NoError(t, testStore.
		UpdateStepRunStatus(ctx, stepA.
			ID, domain.
			StepCompleted,
			map[string]any{"finished_at": time.Now().
				UTC(),
				"output": json.RawMessage(`{"ok":true}`)}))
	require.NoError(t, testStore.
		UpdateStepRunStatus(ctx, stepB.
			ID, domain.
			StepFailed,
			map[string]any{"finished_at": time.
				Now().UTC(), "error": "step b failed"}))
	require.NoError(t, testStore.
		UpdateWorkflowRunStatus(ctx, originalRunID,

			domain.WfStatusRunning,
			domain.
				WfStatusFailed,
			map[string]any{
				"finished_at": time.Now().UTC(), "error": "failed for retry test",
			}))

	retryResp := wfDoReq(t, srv, http.MethodPost, "/v1/workflow-runs/"+originalRunID+"/retry", "")
	require.Equal(t, http.
		StatusCreated,
		retryResp.
			Code)

	newRun := mustDecodeObject(t, retryResp)
	newRunID := asString(t, newRun, "id")
	require.Equal(t, string(domain.
		WfStatusRunning,
	), asString(t,
		newRun, "status",
	))
	require.Equal(t, originalRunID,

		asString(t,
			newRun, "retry_of_run_id",
		))

	newStepRuns, err := testStore.ListStepRunsByWorkflowRun(ctx, newRunID, 10000, nil)
	require.NoError(t, err)

	newStepA := findStepRunByRef(t, newStepRuns, "A")
	newStepB := findStepRunByRef(t, newStepRuns, "B")
	require.Equal(t, domain.
		StepCompleted,
		newStepA.
			Status)
	require.False(t, newStepB.
		Status !=
		domain.
			StepPending && newStepB.
		Status !=
		domain.
			StepRunning)

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
	require.Equal(t, http.
		StatusOK,
		skipResp.Code,
	)

	stepRuns, err := testStore.ListStepRunsByWorkflowRun(ctx, runID, 10000, nil)
	require.NoError(t, err)

	stepA := findStepRunByRef(t, stepRuns, "A")
	require.Equal(t, domain.
		StepSkipped,
		stepA.
			Status)

	run, err := testStore.GetWorkflowRun(ctx, runID)
	require.NoError(t, err)
	require.Equal(t, domain.
		WfStatusCompleted,

		run.Status)

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
	require.Equal(t, http.
		StatusOK,
		forceResp.
			Code)

	stepRuns, err := testStore.ListStepRunsByWorkflowRun(ctx, runID, 10000, nil)
	require.NoError(t, err)

	stepA := findStepRunByRef(t, stepRuns, "A")
	require.Equal(t, domain.
		StepCompleted,
		stepA.
			Status)

	var output map[string]any
	require.NoError(t, json.
		Unmarshal(stepA.Output,
			&output))
	require.Equal(t, "value",

		output["key"])

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
	require.Equal(t, http.
		StatusCreated,
		cloneResp.
			Code)

	cloned := mustDecodeObject(t, cloneResp)
	clonedID := asString(t, cloned, "id")
	require.NotEqual(t, originalID,

		clonedID)
	require.Equal(t, "cloned",

		asString(t, cloned,
			"name"))
	require.Equal(t, "cloned-slug",

		asString(t,
			cloned, "slug"),
	)

	wOrig := wfDoReq(t, srv, http.MethodGet, "/v1/workflows/"+originalID+"/", "")
	require.Equal(t, http.
		StatusOK,
		wOrig.Code,
	)

	origBody := mustDecodeObject(t, wOrig)

	wClone := wfDoReq(t, srv, http.MethodGet, "/v1/workflows/"+clonedID+"/", "")
	require.Equal(t, http.
		StatusOK,
		wClone.Code,
	)

	cloneBody := mustDecodeObject(t, wClone)

	origSteps, ok := origBody["steps"].([]any)
	require.True(t, ok)

	cloneSteps, ok := cloneBody["steps"].([]any)
	require.True(t, ok)
	require.Len(t, origSteps,

		len(cloneSteps))

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
	require.Equal(t, http.
		StatusOK,
		emptyResp.
			Code)

	emptyLabelsBody := mustDecodeObject(t, emptyResp)
	emptyLabels, ok := emptyLabelsBody["labels"].(map[string]any)
	require.True(t, ok)
	require.Len(t, emptyLabels,

		0)
	require.NoError(t, testStore.
		CreateWorkflowRunLabels(ctx, runID,
			map[string]string{
				"env": "test", "owner": "e2e"}))

	labelsResp := wfDoReq(t, srv, http.MethodGet, "/v1/workflow-runs/"+runID+"/labels", "")
	require.Equal(t, http.
		StatusOK,
		labelsResp.
			Code)

	labelsBody := mustDecodeObject(t, labelsResp)
	labels, ok := labelsBody["labels"].(map[string]any)
	require.True(t, ok)
	require.False(t, labels["env"] !=
		"test" ||
		labels["owner"] !=
			"e2e")

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
	require.NoError(t, err)

	stepA := findStepRunByRef(t, stepRuns, "A")
	require.NotEqual(t, "",

		stepA.JobRunID,
	)

	jobRun, err := testStore.GetRun(ctx, stepA.JobRunID)
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, json.
		Unmarshal(jobRun.Payload,
			&payload),
	)
	require.Equal(t, "hello resolved_value",

		payload["message"],
	)
	require.Equal(t, "resolved_value",

		payload["value"])

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
	require.NoError(t, err)
	require.Len(t, stepRuns,

		1)
	require.Equal(t, "A",

		stepRuns[0].StepRef)

}
