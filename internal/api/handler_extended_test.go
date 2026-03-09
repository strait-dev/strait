package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

// ─── UpdateEnvironment ──────────────────────────────────────────────────────.

func TestHandleUpdateEnvironment_Success(t *testing.T) {
	t.Parallel()
	var updatedName string
	ms := &mockAPIStore{
		getEnvironmentFn: func(_ context.Context, id string) (*domain.Environment, error) {
			return &domain.Environment{
				ID:        id,
				ProjectID: "proj-1",
				Name:      "staging",
				Slug:      "staging",
				Variables: map[string]string{"FOO": "bar"},
			}, nil
		},
		updateEnvironmentFn: func(_ context.Context, env *domain.Environment) error {
			updatedName = env.Name
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFEnvironments = true

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/environments/env-1", `{"name":"production"}`))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if updatedName != "production" {
		t.Fatalf("expected name=production, got %q", updatedName)
	}
}

func TestHandleUpdateEnvironment_NotFound(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		getEnvironmentFn: func(_ context.Context, _ string) (*domain.Environment, error) {
			return nil, store.ErrEnvironmentNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFEnvironments = true

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/environments/missing", `{"name":"x"}`))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandleUpdateEnvironment_FeatureDisabled(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/environments/env-1", `{"name":"x"}`))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleUpdateEnvironment_UpdateVariables(t *testing.T) {
	t.Parallel()
	var updatedVars map[string]string
	ms := &mockAPIStore{
		getEnvironmentFn: func(_ context.Context, id string) (*domain.Environment, error) {
			return &domain.Environment{
				ID:        id,
				ProjectID: "proj-1",
				Name:      "staging",
				Slug:      "staging",
				Variables: map[string]string{"OLD": "val"},
			}, nil
		},
		updateEnvironmentFn: func(_ context.Context, env *domain.Environment) error {
			updatedVars = env.Variables
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFEnvironments = true

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/environments/env-1", `{"variables":{"NEW":"val2"}}`))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if updatedVars["NEW"] != "val2" {
		t.Fatalf("expected variables updated, got %v", updatedVars)
	}
}

func TestHandleUpdateEnvironment_InvalidBody(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		getEnvironmentFn: func(_ context.Context, id string) (*domain.Environment, error) {
			return &domain.Environment{ID: id, ProjectID: "proj-1", Name: "staging", Slug: "staging"}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFEnvironments = true

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/environments/env-1", `{invalid`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// ─── DeleteEnvironment ──────────────────────────────────────────────────────.

func TestHandleDeleteEnvironment_Success(t *testing.T) {
	t.Parallel()
	var deletedID string
	ms := &mockAPIStore{
		deleteEnvironmentFn: func(_ context.Context, id string) error {
			deletedID = id
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFEnvironments = true

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/environments/env-1", ""))

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
	if deletedID != "env-1" {
		t.Fatalf("expected deleted id env-1, got %q", deletedID)
	}
}

func TestHandleDeleteEnvironment_NotFound(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		deleteEnvironmentFn: func(_ context.Context, _ string) error {
			return store.ErrEnvironmentNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFEnvironments = true

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/environments/missing", ""))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandleDeleteEnvironment_FeatureDisabled(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/environments/env-1", ""))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// ─── UpdateJobGroup ─────────────────────────────────────────────────────────.

func TestHandleUpdateJobGroup_Success(t *testing.T) {
	t.Parallel()
	var updatedName string
	ms := &mockAPIStore{
		getJobGroupFn: func(_ context.Context, id string) (*domain.JobGroup, error) {
			return &domain.JobGroup{ID: id, ProjectID: "proj-1", Name: "Core", Slug: "core"}, nil
		},
		updateJobGroupFn: func(_ context.Context, group *domain.JobGroup) error {
			updatedName = group.Name
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFJobGroups = true

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/job-groups/group-1", `{"name":"Updated"}`))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if updatedName != "Updated" {
		t.Fatalf("expected name=Updated, got %q", updatedName)
	}
}

func TestHandleUpdateJobGroup_NotFound(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		getJobGroupFn: func(_ context.Context, _ string) (*domain.JobGroup, error) {
			return nil, store.ErrJobGroupNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFJobGroups = true

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/job-groups/missing", `{"name":"x"}`))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandleUpdateJobGroup_FeatureDisabled(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/job-groups/group-1", `{"name":"x"}`))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleUpdateJobGroup_UpdateDescription(t *testing.T) {
	t.Parallel()
	var updatedDesc string
	ms := &mockAPIStore{
		getJobGroupFn: func(_ context.Context, id string) (*domain.JobGroup, error) {
			return &domain.JobGroup{ID: id, ProjectID: "proj-1", Name: "Core", Slug: "core"}, nil
		},
		updateJobGroupFn: func(_ context.Context, group *domain.JobGroup) error {
			updatedDesc = group.Description
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFJobGroups = true

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/job-groups/group-1", `{"description":"A group of core jobs"}`))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if updatedDesc != "A group of core jobs" {
		t.Fatalf("expected description updated, got %q", updatedDesc)
	}
}

func TestHandleUpdateJobGroup_InvalidBody(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		getJobGroupFn: func(_ context.Context, id string) (*domain.JobGroup, error) {
			return &domain.JobGroup{ID: id, ProjectID: "proj-1", Name: "Core", Slug: "core"}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFJobGroups = true

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/job-groups/group-1", `{not valid json`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// ─── ListRunCheckpoints ─────────────────────────────────────────────────────.

func TestHandleListRunCheckpoints_Success(t *testing.T) {
	t.Parallel()
	now := time.Now()
	ms := &mockAPIStore{
		listRunCheckpointsFn: func(_ context.Context, runID string, _ int, _ *time.Time) ([]domain.RunCheckpoint, error) {
			return []domain.RunCheckpoint{
				{ID: "cp-1", RunID: runID, Sequence: 1, Source: "auto", State: json.RawMessage(`{"step":1}`), CreatedAt: now},
				{ID: "cp-2", RunID: runID, Sequence: 2, Source: "manual", State: json.RawMessage(`{"step":2}`), CreatedAt: now.Add(time.Second)},
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/runs/run-1/checkpoints", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var checkpoints []domain.RunCheckpoint
	decodePaginatedList(t, w.Body.Bytes(), &checkpoints)
	if len(checkpoints) != 2 {
		t.Fatalf("expected 2 checkpoints, got %d", len(checkpoints))
	}
}

func TestHandleListRunCheckpoints_Empty(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		listRunCheckpointsFn: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.RunCheckpoint, error) {
			return []domain.RunCheckpoint{}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/runs/run-1/checkpoints", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var checkpoints []domain.RunCheckpoint
	decodePaginatedList(t, w.Body.Bytes(), &checkpoints)
	if len(checkpoints) != 0 {
		t.Fatalf("expected 0 checkpoints, got %d", len(checkpoints))
	}
}

func TestHandleListRunCheckpoints_StoreError(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		listRunCheckpointsFn: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.RunCheckpoint, error) {
			return nil, store.ErrRunNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/runs/run-1/checkpoints", ""))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

// ─── ListRunUsage ───────────────────────────────────────────────────────────.

func TestHandleListRunUsage_Success(t *testing.T) {
	t.Parallel()
	now := time.Now()
	ms := &mockAPIStore{
		listRunUsageFn: func(_ context.Context, runID string, _ int, _ *time.Time) ([]domain.RunUsage, error) {
			return []domain.RunUsage{
				{ID: "u-1", RunID: runID, Provider: "openai", Model: "gpt-4", PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150, CostMicrousd: 5000, CreatedAt: now},
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/runs/run-1/usage", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var usage []domain.RunUsage
	decodePaginatedList(t, w.Body.Bytes(), &usage)
	if len(usage) != 1 {
		t.Fatalf("expected 1 usage record, got %d", len(usage))
	}
	if usage[0].Provider != "openai" {
		t.Fatalf("expected provider=openai, got %q", usage[0].Provider)
	}
}

func TestHandleListRunUsage_Empty(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		listRunUsageFn: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.RunUsage, error) {
			return []domain.RunUsage{}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/runs/run-1/usage", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var usage []domain.RunUsage
	decodePaginatedList(t, w.Body.Bytes(), &usage)
	if len(usage) != 0 {
		t.Fatalf("expected 0 usage records, got %d", len(usage))
	}
}

// ─── ListRunToolCalls ───────────────────────────────────────────────────────.

func TestHandleListRunToolCalls_Success(t *testing.T) {
	t.Parallel()
	now := time.Now()
	ms := &mockAPIStore{
		listRunToolCallsFn: func(_ context.Context, runID string, _ int, _ *time.Time) ([]domain.RunToolCall, error) {
			return []domain.RunToolCall{
				{ID: "tc-1", RunID: runID, ToolName: "web_search", Input: json.RawMessage(`{"q":"test"}`), DurationMs: 200, Status: "success", CreatedAt: now},
				{ID: "tc-2", RunID: runID, ToolName: "code_exec", Input: json.RawMessage(`{"code":"print(1)"}`), DurationMs: 50, Status: "success", CreatedAt: now.Add(time.Second)},
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/runs/run-1/tool-calls", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var calls []domain.RunToolCall
	decodePaginatedList(t, w.Body.Bytes(), &calls)
	if len(calls) != 2 {
		t.Fatalf("expected 2 tool calls, got %d", len(calls))
	}
	if calls[0].ToolName != "web_search" {
		t.Fatalf("expected tool_name=web_search, got %q", calls[0].ToolName)
	}
}

func TestHandleListRunToolCalls_Empty(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		listRunToolCallsFn: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.RunToolCall, error) {
			return []domain.RunToolCall{}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/runs/run-1/tool-calls", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// ─── ListRunOutputs ─────────────────────────────────────────────────────────.

func TestHandleListRunOutputs_Success(t *testing.T) {
	t.Parallel()
	now := time.Now()
	ms := &mockAPIStore{
		listRunOutputsFn: func(_ context.Context, runID string, _ int, _ *time.Time) ([]domain.RunOutput, error) {
			return []domain.RunOutput{
				{ID: "out-1", RunID: runID, OutputKey: "result", Value: json.RawMessage(`"hello"`), CreatedAt: now},
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/runs/run-1/outputs", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var outputs []domain.RunOutput
	decodePaginatedList(t, w.Body.Bytes(), &outputs)
	if len(outputs) != 1 {
		t.Fatalf("expected 1 output, got %d", len(outputs))
	}
	if outputs[0].OutputKey != "result" {
		t.Fatalf("expected output_key=result, got %q", outputs[0].OutputKey)
	}
}

func TestHandleListRunOutputs_Empty(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		listRunOutputsFn: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.RunOutput, error) {
			return []domain.RunOutput{}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/runs/run-1/outputs", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// ─── SDKToolCall ────────────────────────────────────────────────────────────.

func TestHandleSDKToolCall_Success(t *testing.T) {
	t.Parallel()
	var created bool
	ms := &mockAPIStore{
		createRunToolCallFn: func(_ context.Context, call *domain.RunToolCall) error {
			created = true
			if call.ToolName != "fetch_url" {
				t.Fatalf("expected tool_name=fetch_url, got %q", call.ToolName)
			}
			if call.RunID != "run-42" {
				t.Fatalf("expected run_id=run-42, got %q", call.RunID)
			}
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	body := `{"tool_name":"fetch_url","input":{"url":"https://example.com"},"duration_ms":150,"status":"success"}`
	srv.ServeHTTP(w, sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-42/tool-call", "run-42", body))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if !created {
		t.Fatal("CreateRunToolCall was not called")
	}
}

func TestHandleSDKToolCall_MissingToolName(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-42/tool-call", "run-42", `{"status":"ok"}`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleSDKToolCall_InvalidBody(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-42/tool-call", "run-42", `{invalid`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// ─── APIReference & OpenAPISpec ─────────────────────────────────────────────.

func TestHandleAPIReference_Returns200(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	// API reference endpoint is public (no auth required at route level, but let's use authed for consistency)
	r := httptest.NewRequest(http.MethodGet, "/reference", nil)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	contentType := w.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/html") {
		t.Fatalf("expected text/html content type, got %q", contentType)
	}
}

func TestHandleOpenAPISpec_Returns200(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/reference/openapi.yaml", nil)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	contentType := w.Header().Get("Content-Type")
	if !strings.Contains(contentType, "yaml") {
		t.Fatalf("expected yaml content type, got %q", contentType)
	}
	if w.Body.Len() == 0 {
		t.Fatal("expected non-empty OpenAPI spec body")
	}
}

func TestHandleOpenAPISpec_ContainsOpenAPI(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &mockAPIStore{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/reference/openapi.yaml", nil)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "openapi") {
		t.Fatal("expected response to contain 'openapi'")
	}
}
