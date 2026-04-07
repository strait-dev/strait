package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"
)

func TestSDKTelemetry_Usage_Success(t *testing.T) {
	t.Parallel()
	run := &domain.JobRun{
		ID:        "run-1",
		JobID:     "job-1",
		ProjectID: "proj-1",
		Status:    domain.StatusExecuting,
	}
	var usageCreated bool
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return run, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, _ string) (*store.ProjectQuota, error) {
			return &store.ProjectQuota{ProjectID: "proj-1"}, nil
		},
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{ID: "job-1"}, nil
		},
		SumRunTotalTokensFunc: func(_ context.Context, _ string) (int64, error) {
			return 0, nil
		},
		CreateRunUsageFunc: func(_ context.Context, usage *domain.RunUsage) error {
			usageCreated = true
			if usage.RunID != "run-1" {
				t.Fatalf("expected run-1, got %s", usage.RunID)
			}
			if usage.Provider != "openai" {
				t.Fatalf("expected openai, got %s", usage.Provider)
			}
			if usage.TotalTokens != 150 {
				t.Fatalf("expected 150 tokens, got %d", usage.TotalTokens)
			}
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/usage", "run-1",
		`{"provider":"openai","model":"gpt-4","prompt_tokens":100,"completion_tokens":50,"total_tokens":150,"cost_microusd":0}`)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK && w.Code != http.StatusCreated {
		t.Fatalf("expected 200 or 201, got %d: %s", w.Code, w.Body.String())
	}
	if !usageCreated {
		t.Fatal("expected CreateRunUsage to be called")
	}
}

func TestSDKTelemetry_Usage_TokenBudgetExceeded(t *testing.T) {
	t.Parallel()
	run := &domain.JobRun{
		ID:        "run-1",
		JobID:     "job-1",
		ProjectID: "proj-1",
		Status:    domain.StatusExecuting,
	}
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return run, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, _ string) (*store.ProjectQuota, error) {
			return &store.ProjectQuota{ProjectID: "proj-1", MaxTokensPerRun: 1000}, nil
		},
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{ID: "job-1"}, nil
		},
		SumRunTotalTokensFunc: func(_ context.Context, _ string) (int64, error) {
			return 950, nil // already at 950
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	// Request 100 more tokens; 950 + 100 > 1000.
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/usage", "run-1",
		`{"provider":"openai","model":"gpt-4","prompt_tokens":80,"completion_tokens":20,"total_tokens":100}`)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d: %s", w.Code, w.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	// typedAPIError wraps as {"error": {"code": "..."}, "request_id": "..."}.
	errObj, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error object, got %v", body)
	}
	if errObj["code"] != "token_budget_exceeded" {
		t.Fatalf("expected token_budget_exceeded, got %v", errObj["code"])
	}
}

func TestSDKTelemetry_Usage_CostBudgetExceeded(t *testing.T) {
	t.Parallel()
	run := &domain.JobRun{
		ID:        "run-1",
		JobID:     "job-1",
		ProjectID: "proj-1",
		Status:    domain.StatusExecuting,
	}
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return run, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, _ string) (*store.ProjectQuota, error) {
			return &store.ProjectQuota{ProjectID: "proj-1", MaxCostPerRunMicrousd: 500000}, nil
		},
		SumRunCostMicrousdFunc: func(_ context.Context, _ string) (int64, error) {
			return 490000, nil // already near limit
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	// 490000 + 20000 = 510000 >= 500000 limit.
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/usage", "run-1",
		`{"provider":"openai","model":"gpt-4","prompt_tokens":100,"completion_tokens":50,"cost_microusd":20000}`)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSDKTelemetry_Usage_CostExactlyAtLimit(t *testing.T) {
	t.Parallel()
	run := &domain.JobRun{
		ID:        "run-1",
		JobID:     "job-1",
		ProjectID: "proj-1",
		Status:    domain.StatusExecuting,
	}
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return run, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, _ string) (*store.ProjectQuota, error) {
			return &store.ProjectQuota{ProjectID: "proj-1", MaxCostPerRunMicrousd: 500000}, nil
		},
		SumRunCostMicrousdFunc: func(_ context.Context, _ string) (int64, error) {
			return 499999, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	// 499999 + 1 = 500000 >= 500000 limit. The code uses >= so this should be rejected.
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/usage", "run-1",
		`{"provider":"openai","model":"gpt-4","prompt_tokens":1,"completion_tokens":0,"cost_microusd":1}`)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 at exact boundary, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSDKTelemetry_Usage_TokenExactlyAtLimit(t *testing.T) {
	t.Parallel()
	run := &domain.JobRun{
		ID:        "run-1",
		JobID:     "job-1",
		ProjectID: "proj-1",
		Status:    domain.StatusExecuting,
	}
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return run, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, _ string) (*store.ProjectQuota, error) {
			return &store.ProjectQuota{ProjectID: "proj-1", MaxTokensPerRun: 1000}, nil
		},
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{ID: "job-1"}, nil
		},
		SumRunTotalTokensFunc: func(_ context.Context, _ string) (int64, error) {
			return 999, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	// 999 + 2 = 1001 > 1000. The code uses > so this should be rejected.
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/usage", "run-1",
		`{"provider":"openai","model":"gpt-4","prompt_tokens":1,"completion_tokens":1,"total_tokens":2}`)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 at boundary, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSDKTelemetry_Usage_TokenExactlyAtLimit_Passes(t *testing.T) {
	t.Parallel()
	run := &domain.JobRun{
		ID:        "run-1",
		JobID:     "job-1",
		ProjectID: "proj-1",
		Status:    domain.StatusExecuting,
	}
	var usageCreated bool
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return run, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, _ string) (*store.ProjectQuota, error) {
			return &store.ProjectQuota{ProjectID: "proj-1", MaxTokensPerRun: 1000}, nil
		},
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{ID: "job-1"}, nil
		},
		SumRunTotalTokensFunc: func(_ context.Context, _ string) (int64, error) {
			return 999, nil
		},
		CreateRunUsageFunc: func(_ context.Context, _ *domain.RunUsage) error {
			usageCreated = true
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	// 999 + 1 = 1000, which is NOT > 1000. Should pass.
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/usage", "run-1",
		`{"provider":"openai","model":"gpt-4","prompt_tokens":1,"completion_tokens":0,"total_tokens":1}`)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK && w.Code != http.StatusCreated {
		t.Fatalf("expected 200 or 201, got %d: %s", w.Code, w.Body.String())
	}
	if !usageCreated {
		t.Fatal("expected CreateRunUsage to be called")
	}
}

func TestSDKTelemetry_Usage_MissingRequiredFields(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/usage", "run-1", `{}`)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing required fields, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSDKTelemetry_Usage_NoBudgetLimits(t *testing.T) {
	t.Parallel()
	run := &domain.JobRun{
		ID:        "run-1",
		JobID:     "job-1",
		ProjectID: "proj-1",
		Status:    domain.StatusExecuting,
	}
	var usageCreated bool
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return run, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, _ string) (*store.ProjectQuota, error) {
			return &store.ProjectQuota{ProjectID: "proj-1"}, nil
		},
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{ID: "job-1"}, nil
		},
		SumRunTotalTokensFunc: func(_ context.Context, _ string) (int64, error) {
			return 0, nil
		},
		CreateRunUsageFunc: func(_ context.Context, _ *domain.RunUsage) error {
			usageCreated = true
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/usage", "run-1",
		`{"provider":"anthropic","model":"claude-3","prompt_tokens":5000,"completion_tokens":3000,"total_tokens":8000,"cost_microusd":50000}`)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK && w.Code != http.StatusCreated {
		t.Fatalf("expected 200 or 201, got %d: %s", w.Code, w.Body.String())
	}
	if !usageCreated {
		t.Fatal("expected CreateRunUsage to be called")
	}
}

// Tool call tests.

func TestSDKTelemetry_ToolCall_Success(t *testing.T) {
	t.Parallel()
	run := &domain.JobRun{
		ID:        "run-1",
		JobID:     "job-1",
		ProjectID: "proj-1",
		Status:    domain.StatusExecuting,
	}
	job := &domain.Job{ID: "job-1"}
	var toolCallCreated bool
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return run, nil
		},
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return job, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, _ string) (*store.ProjectQuota, error) {
			return &store.ProjectQuota{ProjectID: "proj-1"}, nil
		},
		CreateRunToolCallFunc: func(_ context.Context, call *domain.RunToolCall) error {
			toolCallCreated = true
			if call.ToolName != "web_search" {
				t.Fatalf("expected web_search, got %s", call.ToolName)
			}
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/tool-call", "run-1",
		`{"tool_name":"web_search","input":{"query":"test"},"status":"success","duration_ms":150}`)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK && w.Code != http.StatusCreated {
		t.Fatalf("expected 200 or 201, got %d: %s", w.Code, w.Body.String())
	}
	if !toolCallCreated {
		t.Fatal("expected CreateRunToolCall to be called")
	}
}

func TestSDKTelemetry_ToolCall_AllowList_Allowed(t *testing.T) {
	t.Parallel()
	run := &domain.JobRun{
		ID:        "run-1",
		JobID:     "job-1",
		ProjectID: "proj-1",
		Status:    domain.StatusExecuting,
	}
	job := &domain.Job{
		ID:           "job-1",
		AllowedTools: []string{"web_search", "calculator"},
	}
	var toolCallCreated bool
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return run, nil
		},
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return job, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, _ string) (*store.ProjectQuota, error) {
			return &store.ProjectQuota{ProjectID: "proj-1"}, nil
		},
		CreateRunToolCallFunc: func(_ context.Context, _ *domain.RunToolCall) error {
			toolCallCreated = true
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/tool-call", "run-1", `{"tool_name":"web_search"}`)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK && w.Code != http.StatusCreated {
		t.Fatalf("expected 200 or 201, got %d: %s", w.Code, w.Body.String())
	}
	if !toolCallCreated {
		t.Fatal("expected CreateRunToolCall to be called")
	}
}

func TestSDKTelemetry_ToolCall_AllowList_Blocked(t *testing.T) {
	t.Parallel()
	run := &domain.JobRun{
		ID:        "run-1",
		JobID:     "job-1",
		ProjectID: "proj-1",
		Status:    domain.StatusExecuting,
	}
	job := &domain.Job{
		ID:           "job-1",
		AllowedTools: []string{"web_search", "calculator"},
	}
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return run, nil
		},
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return job, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/tool-call", "run-1", `{"tool_name":"file_delete"}`)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	errObj, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error object, got %v", body)
	}
	if errObj["code"] != "tool_not_allowed" {
		t.Fatalf("expected tool_not_allowed, got %v", errObj["code"])
	}
}

func TestSDKTelemetry_ToolCall_BlockList(t *testing.T) {
	t.Parallel()
	run := &domain.JobRun{
		ID:        "run-1",
		JobID:     "job-1",
		ProjectID: "proj-1",
		Status:    domain.StatusExecuting,
	}
	job := &domain.Job{
		ID:           "job-1",
		BlockedTools: []string{"dangerous_tool", "rm_rf"},
	}
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return run, nil
		},
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return job, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/tool-call", "run-1", `{"tool_name":"dangerous_tool"}`)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	errObj, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error object, got %v", body)
	}
	if errObj["code"] != "tool_blocked" {
		t.Fatalf("expected tool_blocked, got %v", errObj["code"])
	}
}

func TestSDKTelemetry_ToolCall_LimitExceeded(t *testing.T) {
	t.Parallel()
	run := &domain.JobRun{
		ID:        "run-1",
		JobID:     "job-1",
		ProjectID: "proj-1",
		Status:    domain.StatusExecuting,
	}
	job := &domain.Job{
		ID:                 "job-1",
		MaxToolCallsPerRun: 5,
	}
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return run, nil
		},
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return job, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, _ string) (*store.ProjectQuota, error) {
			return &store.ProjectQuota{ProjectID: "proj-1"}, nil
		},
		CountRunToolCallsFunc: func(_ context.Context, _ string) (int, error) {
			return 5, nil // at limit
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/tool-call", "run-1", `{"tool_name":"web_search"}`)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d: %s", w.Code, w.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	errObj, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error object, got %v", body)
	}
	if errObj["code"] != "tool_call_limit_exceeded" {
		t.Fatalf("expected tool_call_limit_exceeded, got %v", errObj["code"])
	}
}

func TestSDKTelemetry_ToolCall_NoAllowOrBlockList(t *testing.T) {
	t.Parallel()
	run := &domain.JobRun{
		ID:        "run-1",
		JobID:     "job-1",
		ProjectID: "proj-1",
		Status:    domain.StatusExecuting,
	}
	job := &domain.Job{ID: "job-1"} // no allow/block lists, no tool call limit
	var toolCallCreated bool
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return run, nil
		},
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return job, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, _ string) (*store.ProjectQuota, error) {
			return &store.ProjectQuota{ProjectID: "proj-1"}, nil
		},
		CreateRunToolCallFunc: func(_ context.Context, _ *domain.RunToolCall) error {
			toolCallCreated = true
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/tool-call", "run-1", `{"tool_name":"any_tool"}`)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK && w.Code != http.StatusCreated {
		t.Fatalf("expected 200 or 201, got %d: %s", w.Code, w.Body.String())
	}
	if !toolCallCreated {
		t.Fatal("expected CreateRunToolCall to be called")
	}
}

func TestSDKTelemetry_ToolCall_MissingToolName(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/tool-call", "run-1", `{}`)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing tool_name, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSDKTelemetry_ToolCall_QuotaLimitUsedWhenNoJobLimit(t *testing.T) {
	t.Parallel()
	run := &domain.JobRun{
		ID:        "run-1",
		JobID:     "job-1",
		ProjectID: "proj-1",
		Status:    domain.StatusExecuting,
	}
	job := &domain.Job{ID: "job-1"} // no job-level limit
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return run, nil
		},
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return job, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, _ string) (*store.ProjectQuota, error) {
			return &store.ProjectQuota{ProjectID: "proj-1", MaxToolCallsPerRun: 3}, nil
		},
		CountRunToolCallsFunc: func(_ context.Context, _ string) (int, error) {
			return 3, nil // at quota limit
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/tool-call", "run-1", `{"tool_name":"search"}`)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSDKTelemetry_Usage_JobTokenLimitOverridesQuota(t *testing.T) {
	t.Parallel()
	run := &domain.JobRun{
		ID:        "run-1",
		JobID:     "job-1",
		ProjectID: "proj-1",
		Status:    domain.StatusExecuting,
	}
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return run, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, _ string) (*store.ProjectQuota, error) {
			// Quota allows 10000 tokens.
			return &store.ProjectQuota{ProjectID: "proj-1", MaxTokensPerRun: 10000}, nil
		},
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			// Job restricts to 500 tokens.
			return &domain.Job{ID: "job-1", MaxTokensPerRun: 500}, nil
		},
		SumRunTotalTokensFunc: func(_ context.Context, _ string) (int64, error) {
			return 490, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	// 490 + 20 = 510 > 500 job limit.
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/usage", "run-1",
		`{"provider":"openai","model":"gpt-4","prompt_tokens":10,"completion_tokens":10,"total_tokens":20}`)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 from job token limit, got %d: %s", w.Code, w.Body.String())
	}
}
