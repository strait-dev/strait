package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"
	"strait/internal/workflow"
)

// TestHandleContinueWorkflowRunAsNew covers the continue-as-new endpoint:
// success, precondition failures, engine error mapping, and auth boundaries.
func TestHandleContinueWorkflowRunAsNew(t *testing.T) {
	t.Parallel()

	t.Run("success returns 201 with successor and bidirectional links", func(t *testing.T) {
		t.Parallel()
		published := map[string]int{}
		var auditAction string
		var auditDetails map[string]any

		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", ProjectID: "proj-1", Status: domain.WfStatusRunning}, nil
			},
			CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
				auditAction = ev.Action
				if err := json.Unmarshal(ev.Details, &auditDetails); err != nil {
					t.Fatalf("decode audit details: %v", err)
				}
				return nil
			},
		}
		trigger := &mockWorkflowTrigger{
			continueAsNewFn: func(_ context.Context, runID string, input json.RawMessage, strategy domain.ContinueVersionStrategy) (*domain.WorkflowRun, error) {
				if runID != "wfr-1" {
					t.Fatalf("runID = %q, want wfr-1", runID)
				}
				if string(input) != `{"cursor":7}` {
					t.Fatalf("input = %q, want carry-over", string(input))
				}
				// No versionStrategy in the body resolves to the default at the engine.
				if strategy != "" {
					t.Fatalf("strategy = %q, want empty (default) when body omits versionStrategy", strategy)
				}
				return &domain.WorkflowRun{
					ID:                         "wfr-2",
					WorkflowID:                 "wf-1",
					ProjectID:                  "proj-1",
					Status:                     domain.WfStatusRunning,
					ContinuedFromWorkflowRunID: "wfr-1",
					LineageDepth:               1,
				}, nil
			},
		}
		pub := &mockPublisher{publishFn: func(_ context.Context, channel string, _ []byte) error {
			published[channel]++
			return nil
		}}

		srv := newWorkflowTestServer(t, ms, &mockQueue{}, pub, trigger)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/wfr-1/continue-as-new", `{"input":{"cursor":7}}`))

		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
		}
		var got domain.WorkflowRun
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if got.ID != "wfr-2" || got.ContinuedFromWorkflowRunID != "wfr-1" || got.LineageDepth != 1 {
			t.Fatalf("successor = %+v, want wfr-2 linked to wfr-1 at depth 1", got)
		}
		if published["workflow-run:wfr-2"] != 1 {
			t.Fatalf("expected successor run hook publish once, got %d", published["workflow-run:wfr-2"])
		}
		if auditAction != domain.AuditActionWorkflowRunContinuedAsNew {
			t.Fatalf("audit action = %q, want %q", auditAction, domain.AuditActionWorkflowRunContinuedAsNew)
		}
		if auditDetails["successor_run_id"] != "wfr-2" {
			t.Fatalf("audit successor_run_id = %v, want wfr-2", auditDetails["successor_run_id"])
		}
		if auditDetails["version_strategy"] != "repin" {
			t.Fatalf("audit version_strategy = %v, want normalized default repin", auditDetails["version_strategy"])
		}
	})

	t.Run("latest versionStrategy threads through to the engine", func(t *testing.T) {
		t.Parallel()
		var gotStrategy domain.ContinueVersionStrategy
		var auditDetails map[string]any
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", ProjectID: "proj-1", Status: domain.WfStatusRunning}, nil
			},
			CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
				return json.Unmarshal(ev.Details, &auditDetails)
			},
		}
		trigger := &mockWorkflowTrigger{
			continueAsNewFn: func(_ context.Context, _ string, _ json.RawMessage, strategy domain.ContinueVersionStrategy) (*domain.WorkflowRun, error) {
				gotStrategy = strategy
				return &domain.WorkflowRun{ID: "wfr-2", WorkflowID: "wf-1", ProjectID: "proj-1", Status: domain.WfStatusRunning}, nil
			},
		}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, trigger)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/wfr-1/continue-as-new", `{"input":{},"versionStrategy":"latest"}`))
		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
		}
		if gotStrategy != domain.ContinueVersionLatest {
			t.Fatalf("engine strategy = %q, want latest", gotStrategy)
		}
		if auditDetails["version_strategy"] != "latest" {
			t.Fatalf("audit version_strategy = %v, want latest", auditDetails["version_strategy"])
		}
	})

	t.Run("invalid versionStrategy is rejected before engine call", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", ProjectID: "proj-1", Status: domain.WfStatusRunning}, nil
			},
		}
		trigger := &mockWorkflowTrigger{
			continueAsNewFn: func(_ context.Context, _ string, _ json.RawMessage, _ domain.ContinueVersionStrategy) (*domain.WorkflowRun, error) {
				t.Fatal("engine must not be called for an invalid versionStrategy")
				return nil, nil
			},
		}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, trigger)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/wfr-1/continue-as-new", `{"versionStrategy":"bogus"}`))
		// The handler validates versionStrategy via IsValid() and rejects an
		// unknown value with 400 Bad Request before the engine is called.
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("terminal run returns 400 before engine call", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", ProjectID: "proj-1", Status: domain.WfStatusCompleted}, nil
			},
		}
		trigger := &mockWorkflowTrigger{
			continueAsNewFn: func(_ context.Context, _ string, _ json.RawMessage, _ domain.ContinueVersionStrategy) (*domain.WorkflowRun, error) {
				t.Fatal("engine must not be called for a terminal run")
				return nil, nil
			},
		}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, trigger)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/wfr-1/continue-as-new", `{"input":{}}`))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("engine unavailable returns 503", func(t *testing.T) {
		t.Parallel()
		srv := newWorkflowTestServer(t, &APIStoreMock{}, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/wfr-1/continue-as-new", `{"input":{}}`))
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("unknown run returns 404", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return nil, store.ErrWorkflowRunNotFound
			},
		}
		trigger := &mockWorkflowTrigger{}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, trigger)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/missing/continue-as-new", `{"input":{}}`))
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("depth exceeded maps to 400", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", ProjectID: "proj-1", Status: domain.WfStatusRunning}, nil
			},
		}
		trigger := &mockWorkflowTrigger{
			continueAsNewFn: func(_ context.Context, _ string, _ json.RawMessage, _ domain.ContinueVersionStrategy) (*domain.WorkflowRun, error) {
				return nil, workflow.ErrContinueDepthExceeded
			},
		}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, trigger)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/wfr-1/continue-as-new", `{"input":{}}`))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("not-continuable from engine maps to 400", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", ProjectID: "proj-1", Status: domain.WfStatusRunning}, nil
			},
		}
		trigger := &mockWorkflowTrigger{
			continueAsNewFn: func(_ context.Context, _ string, _ json.RawMessage, _ domain.ContinueVersionStrategy) (*domain.WorkflowRun, error) {
				return nil, workflow.ErrWorkflowRunNotContinuable
			},
		}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, trigger)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/wfr-1/continue-as-new", `{"input":{}}`))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("sub-workflow run from engine maps to 400", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", ProjectID: "proj-1", Status: domain.WfStatusRunning}, nil
			},
		}
		trigger := &mockWorkflowTrigger{
			continueAsNewFn: func(_ context.Context, _ string, _ json.RawMessage, _ domain.ContinueVersionStrategy) (*domain.WorkflowRun, error) {
				return nil, workflow.ErrSubWorkflowNotContinuable
			},
		}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, trigger)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/wfr-1/continue-as-new", `{"input":{}}`))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("store conflict maps to 409", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", ProjectID: "proj-1", Status: domain.WfStatusRunning}, nil
			},
		}
		trigger := &mockWorkflowTrigger{
			continueAsNewFn: func(_ context.Context, _ string, _ json.RawMessage, _ domain.ContinueVersionStrategy) (*domain.WorkflowRun, error) {
				return nil, store.ErrWorkflowRunContinueConflict
			},
		}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, trigger)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/wfr-1/continue-as-new", `{"input":{}}`))
		if w.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("generic engine error maps to 500 without leaking internals", func(t *testing.T) {
		t.Parallel()
		// A distinctive secret-looking substring the engine error must never expose.
		const leaked = "postgres://strait:s3cr3t@db.internal:5432/strait"
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", ProjectID: "proj-1", Status: domain.WfStatusRunning}, nil
			},
		}
		trigger := &mockWorkflowTrigger{
			continueAsNewFn: func(_ context.Context, _ string, _ json.RawMessage, _ domain.ContinueVersionStrategy) (*domain.WorkflowRun, error) {
				return nil, fmt.Errorf("dial failed: %s", leaked)
			},
		}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, trigger)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/wfr-1/continue-as-new", `{"input":{}}`))
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
		}
		// writeTypedError sanitizes all 5xx bodies to a generic message; the
		// engine error (and the secret it carries) must never reach the client.
		body := w.Body.String()
		if strings.Contains(body, leaked) || strings.Contains(body, "dial failed") {
			t.Fatalf("response body leaked internal error detail: %s", body)
		}
		if !strings.Contains(body, "internal server error") {
			t.Fatalf("response = %s, want generic internal server error", body)
		}
	})

	t.Run("cross-project request returns 404", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", ProjectID: projectA, Status: domain.WfStatusRunning}, nil
			},
		}
		trigger := &mockWorkflowTrigger{
			continueAsNewFn: func(_ context.Context, _ string, _ json.RawMessage, _ domain.ContinueVersionStrategy) (*domain.WorkflowRun, error) {
				t.Fatal("engine must not be called across project boundary")
				return nil, nil
			},
		}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, trigger)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/workflow-runs/wfr-1/continue-as-new", `{"input":{}}`, projectB))
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("missing body succeeds with nil input", func(t *testing.T) {
		t.Parallel()
		var gotInput json.RawMessage
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", ProjectID: "proj-1", Status: domain.WfStatusRunning}, nil
			},
		}
		trigger := &mockWorkflowTrigger{
			continueAsNewFn: func(_ context.Context, _ string, input json.RawMessage, _ domain.ContinueVersionStrategy) (*domain.WorkflowRun, error) {
				gotInput = input
				return &domain.WorkflowRun{ID: "wfr-2", WorkflowID: "wf-1", ProjectID: "proj-1", Status: domain.WfStatusRunning}, nil
			},
		}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, trigger)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/wfr-1/continue-as-new", ``))
		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
		}
		if gotInput != nil {
			t.Fatalf("expected nil carry-over input for empty body, got %q", string(gotInput))
		}
	})

	t.Run("malformed body returns 400", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", ProjectID: "proj-1", Status: domain.WfStatusRunning}, nil
			},
		}
		trigger := &mockWorkflowTrigger{
			continueAsNewFn: func(_ context.Context, _ string, _ json.RawMessage, _ domain.ContinueVersionStrategy) (*domain.WorkflowRun, error) {
				t.Fatal("engine must not be called for a malformed body")
				return nil, nil
			},
		}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, trigger)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/wfr-1/continue-as-new", `{"input": }`))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
		}
	})
}

// TestHandleGetWorkflowRunChain covers the chain navigation endpoint.
func TestHandleGetWorkflowRunChain(t *testing.T) {
	t.Parallel()

	t.Run("returns ordered chain", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", ProjectID: "proj-1", Status: domain.WfStatusContinued}, nil
			},
			GetWorkflowRunChainFunc: func(_ context.Context, _, _ string, _ int, _ string) ([]domain.WorkflowRunChainEntry, error) {
				return []domain.WorkflowRunChainEntry{
					{ID: "root", LineageDepth: 0},
					{ID: "mid", LineageDepth: 1},
					{ID: "latest", LineageDepth: 2},
				}, nil
			},
		}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflow-runs/mid/chain", ""))
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var resp struct {
			Data       []domain.WorkflowRunChainEntry `json:"data"`
			NextCursor *string                        `json:"next_cursor"`
			HasMore    bool                           `json:"has_more"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(resp.Data) != 3 {
			t.Fatalf("len = %d, want 3", len(resp.Data))
		}
		if resp.HasMore || resp.NextCursor != nil {
			t.Fatalf("has_more/next_cursor = %v/%v, want false/nil", resp.HasMore, resp.NextCursor)
		}
		if resp.Data[0].ID != "root" || resp.Data[2].ID != "latest" {
			t.Fatalf("chain order = %s..%s, want root..latest", resp.Data[0].ID, resp.Data[2].ID)
		}
	})

	t.Run("sets next_cursor when more pages exist", func(t *testing.T) {
		t.Parallel()
		var gotLimit int
		var gotCursor string
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", ProjectID: "proj-1", Status: domain.WfStatusContinued}, nil
			},
			GetWorkflowRunChainFunc: func(_ context.Context, _, _ string, limit int, cursor string) ([]domain.WorkflowRunChainEntry, error) {
				gotLimit, gotCursor = limit, cursor
				// Return limit (==3) entries: one extra beyond the page size of 2.
				return []domain.WorkflowRunChainEntry{
					{ID: "a", LineageDepth: 0},
					{ID: "b", LineageDepth: 1},
					{ID: "c", LineageDepth: 2},
				}, nil
			},
		}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflow-runs/a/chain?limit=2", ""))
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		// The handler must over-fetch by one (limit+1) to detect another page.
		if gotLimit != 3 {
			t.Fatalf("store limit = %d, want 3 (page limit 2 + 1)", gotLimit)
		}
		if gotCursor != "" {
			t.Fatalf("store cursor = %q, want empty on first page", gotCursor)
		}
		var resp struct {
			Data       []domain.WorkflowRunChainEntry `json:"data"`
			NextCursor *string                        `json:"next_cursor"`
			HasMore    bool                           `json:"has_more"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(resp.Data) != 2 {
			t.Fatalf("len = %d, want 2 (trimmed to page size)", len(resp.Data))
		}
		if !resp.HasMore || resp.NextCursor == nil || *resp.NextCursor != "b" {
			t.Fatalf("has_more/next_cursor = %v/%v, want true/\"b\"", resp.HasMore, resp.NextCursor)
		}
	})

	t.Run("threads cursor through to the store", func(t *testing.T) {
		t.Parallel()
		var gotCursor string
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", ProjectID: "proj-1", Status: domain.WfStatusContinued}, nil
			},
			GetWorkflowRunChainFunc: func(_ context.Context, _, _ string, _ int, cursor string) ([]domain.WorkflowRunChainEntry, error) {
				gotCursor = cursor
				return []domain.WorkflowRunChainEntry{{ID: "c", LineageDepth: 2}}, nil
			},
		}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflow-runs/a/chain?cursor=b", ""))
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		if gotCursor != "b" {
			t.Fatalf("store cursor = %q, want \"b\"", gotCursor)
		}
	})

	t.Run("unknown run returns 404", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return nil, store.ErrWorkflowRunNotFound
			},
		}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflow-runs/missing/chain", ""))
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("cross-project request returns 404", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", ProjectID: projectA, Status: domain.WfStatusRunning}, nil
			},
			GetWorkflowRunChainFunc: func(_ context.Context, _, _ string, _ int, _ string) ([]domain.WorkflowRunChainEntry, error) {
				t.Fatal("chain must not be queried across project boundary")
				return nil, nil
			},
		}
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/workflow-runs/wfr-1/chain", "", projectB))
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
		}
	})
}

// FuzzContinueWorkflowRunAsNewHandlerBody fuzzes the request-body decode path of
// the continue-as-new endpoint: arbitrary bytes must never panic and must yield
// either a 2xx (engine reached) or a 4xx (decode/precondition rejected).
func FuzzContinueWorkflowRunAsNewHandlerBody(f *testing.F) {
	f.Add(`{"input":{"cursor":1}}`)
	f.Add(`{"input":"abc"}`)
	f.Add(`{}`)
	f.Add(``)
	f.Add(`{"input": }`)
	f.Add(`not json`)
	f.Add(`{"input":` + strings.Repeat("[", 256))

	ms := &APIStoreMock{
		GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: id, WorkflowID: "wf-1", ProjectID: "proj-1", Status: domain.WfStatusRunning}, nil
		},
	}
	trigger := &mockWorkflowTrigger{
		continueAsNewFn: func(_ context.Context, _ string, _ json.RawMessage, _ domain.ContinueVersionStrategy) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: "wfr-2", WorkflowID: "wf-1", ProjectID: "proj-1", Status: domain.WfStatusRunning}, nil
		},
	}
	srv := newWorkflowTestServer(f, ms, &mockQueue{}, nil, trigger)

	f.Fuzz(func(t *testing.T, body string) {
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflow-runs/wfr-1/continue-as-new", body))
		if w.Code < 200 || w.Code >= 500 {
			t.Fatalf("unexpected status %d for body %q", w.Code, body)
		}
	})
}
