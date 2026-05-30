//go:build integration

package e2e_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"strait/internal/api"
	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/workflow"
)

// wfSetupWithContinueDepth mirrors wfSetup but caps the continue-as-new lineage
// depth so the depth guard can be exercised end-to-end through the HTTP layer.
func wfSetupWithContinueDepth(t *testing.T, maxDepth int) *api.Server {
	t.Helper()

	engine := workflow.NewWorkflowEngine(testStore, testQueue, slog.Default()).
		WithMaxContinueDepth(maxDepth)
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

// chainEntry mirrors the lightweight projection returned by the chain endpoint.
type chainEntry struct {
	ID           string `json:"id"`
	LineageDepth int    `json:"lineage_depth"`
	Status       string `json:"status"`
}

type chainPage struct {
	Data       []chainEntry `json:"data"`
	NextCursor *string      `json:"next_cursor"`
	HasMore    bool         `json:"has_more"`
}

func decodeChainPage(t *testing.T, body []byte) chainPage {
	t.Helper()
	var page chainPage
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatalf("decode chain page: %v (body: %s)", err, string(body))
	}
	return page
}

// continueRun issues a continue-as-new request and returns the successor's id.
func continueRun(t *testing.T, srv *api.Server, runID, body string) string {
	t.Helper()
	resp := wfDoReq(t, srv, http.MethodPost, "/v1/workflow-runs/"+runID+"/continue-as-new", body)
	if resp.Code != http.StatusCreated {
		t.Fatalf("continue %s status = %d, body = %s", runID, resp.Code, resp.Body.String())
	}
	return asString(t, mustDecodeObject(t, resp), "id")
}

// TestE2E_WorkflowContinueAsNew_FullChainAndNavigation drives the whole vertical
// (HTTP -> handler -> engine -> real store): trigger a run, continue it twice,
// and assert the predecessor/successor links, lineage depth, carry-over payload,
// fresh step history, predecessor teardown, and the chain navigation endpoint
// including pagination.
func TestE2E_WorkflowContinueAsNew_FullChainAndNavigation(t *testing.T) {
	mustCleanWf(t)
	srv := wfSetup(t)
	ctx := context.Background()

	projectID := "proj-wf-can-chain-" + newID()
	job := wfCreateJob(t, srv, projectID, "Continue Job", "wf-can-job-"+newID())
	wf := wfCreateWorkflow(t, srv, projectID, "Continue Workflow", "wf-can-"+newID(), []map[string]any{
		{"step_ref": "A", "job_id": asString(t, job, "id")},
	})

	triggered := wfTriggerWorkflow(t, srv, asString(t, wf, "id"), projectID, map[string]any{"cursor": float64(0)}, nil)
	rootID := asString(t, triggered, "id")

	// Continue #1: root -> mid, carrying over {"cursor":1}.
	midID := continueRun(t, srv, rootID, `{"input":{"cursor":1}}`)

	// Predecessor (root) is now terminal-continued and links forward.
	gotRoot, err := testStore.GetWorkflowRun(ctx, rootID)
	if err != nil {
		t.Fatalf("get root: %v", err)
	}
	if gotRoot.Status != domain.WfStatusContinued {
		t.Fatalf("root status = %s, want continued", gotRoot.Status)
	}
	if gotRoot.ContinuedToWorkflowRunID != midID {
		t.Fatalf("root continued_to = %q, want %q", gotRoot.ContinuedToWorkflowRunID, midID)
	}
	if gotRoot.FinishedAt == nil {
		t.Fatal("root finished_at not set")
	}

	// Root's in-flight step runs are torn down.
	rootSteps, err := testStore.ListStepRunsByWorkflowRun(ctx, rootID, 10000, nil)
	if err != nil {
		t.Fatalf("list root step runs: %v", err)
	}
	for _, sr := range rootSteps {
		if !sr.Status.IsTerminal() {
			t.Fatalf("root step %s not torn down: status %s", sr.StepRef, sr.Status)
		}
	}

	// Successor (mid) is running, links back, carries depth 1 and the new payload.
	gotMid, err := testStore.GetWorkflowRun(ctx, midID)
	if err != nil {
		t.Fatalf("get mid: %v", err)
	}
	if gotMid.Status != domain.WfStatusRunning {
		t.Fatalf("mid status = %s, want running", gotMid.Status)
	}
	if gotMid.ContinuedFromWorkflowRunID != rootID {
		t.Fatalf("mid continued_from = %q, want %q", gotMid.ContinuedFromWorkflowRunID, rootID)
	}
	if gotMid.LineageDepth != 1 {
		t.Fatalf("mid lineage_depth = %d, want 1", gotMid.LineageDepth)
	}
	if !jsonContainsCursor(t, gotMid.Payload, 1) {
		t.Fatalf("mid payload = %s, want carry-over {\"cursor\":1}", gotMid.Payload)
	}

	// Successor starts with fresh, flat step history (the single root step).
	midSteps, err := testStore.ListStepRunsByWorkflowRun(ctx, midID, 10000, nil)
	if err != nil {
		t.Fatalf("list mid step runs: %v", err)
	}
	if len(midSteps) != 1 {
		t.Fatalf("mid step runs = %d, want 1 fresh step", len(midSteps))
	}

	// Continue #2: mid -> latest.
	latestID := continueRun(t, srv, midID, `{"input":{"cursor":2}}`)
	gotLatest, err := testStore.GetWorkflowRun(ctx, latestID)
	if err != nil {
		t.Fatalf("get latest: %v", err)
	}
	if gotLatest.LineageDepth != 2 || gotLatest.ContinuedFromWorkflowRunID != midID {
		t.Fatalf("latest = depth %d from %q, want depth 2 from %q", gotLatest.LineageDepth, gotLatest.ContinuedFromWorkflowRunID, midID)
	}

	// Chain navigation from the middle run returns the whole lineage, root-first.
	chainResp := wfDoReq(t, srv, http.MethodGet, "/v1/workflow-runs/"+midID+"/chain", "")
	if chainResp.Code != http.StatusOK {
		t.Fatalf("chain status = %d, body = %s", chainResp.Code, chainResp.Body.String())
	}
	page := decodeChainPage(t, chainResp.Body.Bytes())
	if len(page.Data) != 3 {
		t.Fatalf("chain length = %d, want 3", len(page.Data))
	}
	wantOrder := []struct {
		id    string
		depth int
	}{{rootID, 0}, {midID, 1}, {latestID, 2}}
	for i, want := range wantOrder {
		if page.Data[i].ID != want.id || page.Data[i].LineageDepth != want.depth {
			t.Fatalf("chain[%d] = (%s, depth %d), want (%s, depth %d)", i, page.Data[i].ID, page.Data[i].LineageDepth, want.id, want.depth)
		}
	}
	if page.HasMore || page.NextCursor != nil {
		t.Fatalf("single-page chain has_more/next_cursor = %v/%v, want false/nil", page.HasMore, page.NextCursor)
	}

	// Pagination: a page size of 2 splits the 3-run chain into 2 + 1.
	firstResp := wfDoReq(t, srv, http.MethodGet, "/v1/workflow-runs/"+rootID+"/chain?limit=2", "")
	if firstResp.Code != http.StatusOK {
		t.Fatalf("paged chain status = %d, body = %s", firstResp.Code, firstResp.Body.String())
	}
	first := decodeChainPage(t, firstResp.Body.Bytes())
	if len(first.Data) != 2 || !first.HasMore || first.NextCursor == nil {
		t.Fatalf("first page = %d entries, has_more=%v, cursor=%v; want 2/true/non-nil", len(first.Data), first.HasMore, first.NextCursor)
	}
	secondResp := wfDoReq(t, srv, http.MethodGet, "/v1/workflow-runs/"+rootID+"/chain?limit=2&cursor="+*first.NextCursor, "")
	if secondResp.Code != http.StatusOK {
		t.Fatalf("second page status = %d, body = %s", secondResp.Code, secondResp.Body.String())
	}
	second := decodeChainPage(t, secondResp.Body.Bytes())
	if len(second.Data) != 1 || second.Data[0].ID != latestID {
		t.Fatalf("second page = %+v, want the single latest run %s", second.Data, latestID)
	}
}

// TestE2E_WorkflowContinueAsNew_SubWorkflowRejected confirms the sub-workflow
// guard end-to-end: a running run that carries a parent link is rejected with a
// 400 by the endpoint, and no successor is created.
func TestE2E_WorkflowContinueAsNew_SubWorkflowRejected(t *testing.T) {
	mustCleanWf(t)
	srv := wfSetup(t)
	ctx := context.Background()

	projectID := "proj-wf-can-sub-" + newID()
	wfEnsureProject(t, projectID)
	wf := wfCreateWorkflow(t, srv, projectID, "Sub Workflow", "wf-can-sub-"+newID(), []map[string]any{
		{"step_ref": "A", "step_type": "approval", "approval_approvers": []string{"approver@example.com"}},
	})

	// A real parent run so the child's parent FK is satisfiable.
	running := domain.WfStatusRunning
	parent := &domain.WorkflowRun{
		ID:              "wfr-parent-" + newID(),
		WorkflowID:      asString(t, wf, "id"),
		ProjectID:       projectID,
		Status:          running,
		TriggeredBy:     domain.TriggerManual,
		WorkflowVersion: 1,
	}
	if err := testStore.CreateWorkflowRun(ctx, parent); err != nil {
		t.Fatalf("create parent run: %v", err)
	}

	// A running run that is itself a sub-workflow (carries a parent link).
	child := &domain.WorkflowRun{
		ID:                  "wfr-child-" + newID(),
		WorkflowID:          asString(t, wf, "id"),
		ProjectID:           projectID,
		Status:              running,
		TriggeredBy:         domain.TriggerManual,
		WorkflowVersion:     1,
		ParentWorkflowRunID: parent.ID,
	}
	if err := testStore.CreateWorkflowRun(ctx, child); err != nil {
		t.Fatalf("create child run: %v", err)
	}

	resp := wfDoReq(t, srv, http.MethodPost, "/v1/workflow-runs/"+child.ID+"/continue-as-new", `{"input":{"x":1}}`)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("sub-workflow continue status = %d, want 400; body = %s", resp.Code, resp.Body.String())
	}

	// The child must be untouched (still running, no successor link).
	gotChild, err := testStore.GetWorkflowRun(ctx, child.ID)
	if err != nil {
		t.Fatalf("get child: %v", err)
	}
	if gotChild.Status != domain.WfStatusRunning {
		t.Fatalf("child status = %s, want still running", gotChild.Status)
	}
	if gotChild.ContinuedToWorkflowRunID != "" {
		t.Fatalf("child continued_to = %q, want empty (no successor)", gotChild.ContinuedToWorkflowRunID)
	}
}

// TestE2E_WorkflowContinueAsNew_DepthCapEnforced confirms the configurable
// lineage-depth guard rejects a continuation past the cap through the endpoint,
// leaving the current run untouched.
func TestE2E_WorkflowContinueAsNew_DepthCapEnforced(t *testing.T) {
	mustCleanWf(t)
	srv := wfSetupWithContinueDepth(t, 1)
	ctx := context.Background()

	projectID := "proj-wf-can-depth-" + newID()
	job := wfCreateJob(t, srv, projectID, "Depth Job", "wf-can-depth-job-"+newID())
	wf := wfCreateWorkflow(t, srv, projectID, "Depth Workflow", "wf-can-depth-"+newID(), []map[string]any{
		{"step_ref": "A", "job_id": asString(t, job, "id")},
	})

	triggered := wfTriggerWorkflow(t, srv, asString(t, wf, "id"), projectID, nil, nil)
	rootID := asString(t, triggered, "id")

	// Depth 0 -> 1 is exactly at the cap and allowed.
	midID := continueRun(t, srv, rootID, `{"input":{}}`)

	// Depth 1 -> 2 exceeds the cap of 1 and is rejected with a 400.
	resp := wfDoReq(t, srv, http.MethodPost, "/v1/workflow-runs/"+midID+"/continue-as-new", `{"input":{}}`)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("over-cap continue status = %d, want 400; body = %s", resp.Code, resp.Body.String())
	}

	// The capped run is untouched: still running, no successor.
	gotMid, err := testStore.GetWorkflowRun(ctx, midID)
	if err != nil {
		t.Fatalf("get mid: %v", err)
	}
	if gotMid.Status != domain.WfStatusRunning || gotMid.ContinuedToWorkflowRunID != "" {
		t.Fatalf("capped run = status %s, continued_to %q; want running and no successor", gotMid.Status, gotMid.ContinuedToWorkflowRunID)
	}
}

// jsonContainsCursor reports whether payload is a JSON object with cursor == want.
func jsonContainsCursor(t *testing.T, payload json.RawMessage, want int) bool {
	t.Helper()
	var obj map[string]any
	if err := json.Unmarshal(payload, &obj); err != nil {
		return false
	}
	cursor, ok := obj["cursor"].(float64)
	return ok && int(cursor) == want
}
