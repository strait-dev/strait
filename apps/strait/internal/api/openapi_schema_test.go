package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestOpenAPISchema_IncludesAuditAdminEndpoints is a drift guard asserting
// the audit admin endpoints added in the audit hardening work (DLQ inspection,
// per-project export cap, per-project retention override) are registered with
// Huma and therefore included in the runtime-generated OpenAPI spec served at
// /reference/openapi.json.
//
// The spec is generated on server startup from Huma operation registrations
// (see huma_registry.go). No committed OpenAPI file exists — Huma is the
// source of truth. If someone removes or renames these routes this test
// must fail.
func TestOpenAPISchema_IncludesAuditAdminEndpoints(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/reference/openapi.json", nil)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var spec struct {
		Paths map[string]map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &spec); err != nil {
		t.Fatalf("unmarshal openapi spec: %v", err)
	}
	if len(spec.Paths) == 0 {
		t.Fatal("openapi spec has no paths")
	}

	// Each entry: (path, http method) the spec must expose.
	want := []struct {
		path   string
		method string
	}{
		{"/v1/audit/deadletter", "get"},
		{"/v1/audit/deadletter/{id}/replay", "post"},
		{"/v1/audit/deadletter/{id}", "delete"},
		{"/v1/projects/{id}/quotas/audit-export-cap", "put"},
		{"/v1/projects/{id}/audit/retention", "get"},
		{"/v1/projects/{id}/audit/retention", "put"},
	}

	for _, w := range want {
		methods, ok := spec.Paths[w.path]
		if !ok {
			t.Errorf("openapi spec missing path %q", w.path)
			continue
		}
		if _, ok := methods[w.method]; !ok {
			t.Errorf("openapi spec path %q missing method %q", w.path, w.method)
		}
	}
}

func TestOpenAPISchema_DoesNotExposeRemovedLaunchSurfaces(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/reference/openapi.json", nil)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	raw := w.Body.String()
	retiredCostFields := []string{
		strings.Join([]string{"total", "ai", "cost", "microusd"}, "_"),
		strings.Join([]string{"ai", "cost", "microusd"}, "_"),
	}
	for _, stale := range []string{
		"/v1/jobs/{jobID}/deployments",
		"/v1/runs/{runID}/usage",
		"/sdk/v1/runs/{runID}/usage",
		"/v1/runs/{runID}/tool-calls",
		"/sdk/v1/runs/{runID}/tool-call",
		"/sdk/v1/runs/{runID}/iteration",
		"code-deployment",
		"list-run-usage",
		"sdk-usage",
		"SDKUsageRequest",
		"list-run-tool-calls",
		"sdk-tool-call",
		"sdk-iteration",
		"tool_calls",
		"input_tokens",
		"output_tokens",
		"prompt_tokens",
		"completion_tokens",
		"total_tokens",
		"max_tokens_per_run",
		"max_tool_calls_per_run",
		"max_iterations_per_run",
		"allowed_tools",
		"blocked_tools",
		"compute_credit_microusd",
		"compute_credit",
		"included_credit_microusd",
		"credit_used_percent",
		"credit_remaining_microusd",
		"projected_monthly_compute_usd",
		"compute_discount_pct",
		"compute_cost_microusd",
		"compute_microusd",
		"by_model",
		strings.Join([]string{"BY", "OK"}, ""),
		"OpenAI",
		"Anthropic",
		"LLM",
		"model_usage",
		"model usage",
		"max_runs_per_day",
		"has_sso",
		"has_scim",
		"has_ip_allowlisting",
		"has_static_ips",
		"has_vpc_peering",
		"has_data_residency",
		"has_dedicated_compute",
		"has_reserved_capacity",
		"has_priority_queue",
		"has_session_management",
		"has_secret_rotation",
		"has_siem_export",
		"preferred_regions",
		"default_region",
		"allowed_regions",
		"RegionResponse",
		"stream-deployment-logs",
		"machine_id",
	} {
		if strings.Contains(raw, stale) {
			t.Fatalf("openapi spec contains removed launch surface %q", stale)
		}
	}
	for _, stale := range retiredCostFields {
		if strings.Contains(raw, stale) {
			t.Fatalf("openapi spec contains removed launch surface %q", stale)
		}
	}
}

func TestOpenAPISchema_DoesNotExposeLaunchInactiveRegionSurface(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/reference/openapi.json", nil)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var spec struct {
		Paths map[string]map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &spec); err != nil {
		t.Fatalf("unmarshal openapi spec: %v", err)
	}
	if _, ok := spec.Paths["/v1/regions"]; ok {
		t.Fatal("openapi spec exposes launch-inactive /v1/regions endpoint")
	}
}

func TestOpenAPISchema_TriggerJobIncludesTraceHeaders(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/reference/openapi.json", nil)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var spec struct {
		Paths map[string]map[string]struct {
			Parameters []struct {
				Name string `json:"name"`
				In   string `json:"in"`
			} `json:"parameters"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &spec); err != nil {
		t.Fatalf("unmarshal openapi spec: %v", err)
	}

	trigger, ok := spec.Paths["/v1/jobs/{jobID}/trigger"]["post"]
	if !ok {
		t.Fatal("openapi spec is missing POST /v1/jobs/{jobID}/trigger")
	}
	want := map[string]bool{
		"Traceparent":  false,
		"Tracestate":   false,
		"Sentry-Trace": false,
		"Baggage":      false,
	}
	for _, param := range trigger.Parameters {
		if param.In != "header" {
			continue
		}
		if _, ok := want[param.Name]; ok {
			want[param.Name] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Fatalf("trigger operation missing header parameter %q", name)
		}
	}
}

func TestOpenAPISchema_PlanGatedOperationsDeclareForbidden(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/reference/openapi.json", nil)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var spec struct {
		Paths map[string]map[string]struct {
			Responses map[string]json.RawMessage `json:"responses"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &spec); err != nil {
		t.Fatalf("unmarshal openapi spec: %v", err)
	}

	want := []struct {
		name   string
		path   string
		method string
	}{
		{name: "approval stats", path: "/v1/analytics/approvals", method: "get"},
		{name: "create canary deployment", path: "/v1/canary-deployments", method: "post"},
		{name: "get canary status", path: "/v1/workflows/{workflowID}/canary", method: "get"},
		{name: "update canary deployment", path: "/v1/workflows/{workflowID}/canary", method: "patch"},
		{name: "rollback canary deployment", path: "/v1/workflows/{workflowID}/canary/rollback", method: "post"},
		{name: "get compensation plan", path: "/v1/workflow-runs/{workflowRunID}/compensation-plan", method: "get"},
		{name: "compensate workflow run", path: "/v1/workflow-runs/{workflowRunID}/compensate", method: "post"},
		{name: "get workflow policy", path: "/v1/workflow-policies/{projectID}", method: "get"},
		{name: "upsert workflow policy", path: "/v1/workflow-policies/{projectID}", method: "put"},
	}

	for _, tt := range want {
		t.Run(tt.name, func(t *testing.T) {
			methods, ok := spec.Paths[tt.path]
			if !ok {
				t.Fatalf("openapi spec missing path %q", tt.path)
			}
			op, ok := methods[tt.method]
			if !ok {
				t.Fatalf("openapi spec path %q missing method %q", tt.path, tt.method)
			}
			if _, ok := op.Responses["403"]; !ok {
				t.Fatalf("%s %s must declare 403 for its plan/RBAC gate", tt.method, tt.path)
			}
		})
	}
}

// TestOpenAPISchema_ErrorEnvelope guards the error contract surfaced through
// /reference/openapi.json:
//   - the canonical APIError schema is present with the full code enum
//   - the canonical ErrorResponse envelope wraps APIError under "error"
//   - the legacy Huma RFC 9457 ErrorModel is no longer referenced
//   - representative error responses point at ErrorResponse rather than ErrorModel
//
// This is the regression guard that protects SDK codegen pipelines from
// silently regenerating against the wrong shape.
func TestOpenAPISchema_ErrorEnvelope(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/reference/openapi.json", nil)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	raw := w.Body.Bytes()
	if strings.Contains(string(raw), "ErrorModel") {
		t.Fatal("openapi spec still references Huma ErrorModel; the override did not run")
	}

	var spec struct {
		Components struct {
			Schemas map[string]json.RawMessage `json:"schemas"`
		} `json:"components"`
		Paths map[string]map[string]struct {
			Responses map[string]struct {
				Content map[string]struct {
					Schema struct {
						Ref string `json:"$ref"`
					} `json:"schema"`
				} `json:"content"`
			} `json:"responses"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatalf("unmarshal openapi spec: %v", err)
	}

	apiErrorSchema, ok := spec.Components.Schemas["APIError"]
	if !ok {
		t.Fatal("openapi spec missing components.schemas.APIError")
	}
	var apiError struct {
		Properties struct {
			Code struct {
				Enum []string `json:"enum"`
			} `json:"code"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(apiErrorSchema, &apiError); err != nil {
		t.Fatalf("unmarshal APIError schema: %v", err)
	}
	wantCodes := []string{
		"bad_request",
		"authentication_required",
		"forbidden",
		"not_found",
		"conflict",
		"validation_failed",
		"rate_limited",
		"enqueue_throttled",
		"internal_error",
		"service_unavailable",
	}
	got := map[string]bool{}
	for _, c := range apiError.Properties.Code.Enum {
		got[c] = true
	}
	for _, c := range wantCodes {
		if !got[c] {
			t.Errorf("APIError.code enum missing %q", c)
		}
	}

	if _, ok := spec.Components.Schemas["ErrorResponse"]; !ok {
		t.Fatal("openapi spec missing components.schemas.ErrorResponse")
	}

	// Spot-check at least one operation: error responses should reference
	// ErrorResponse, not the old ErrorModel.
	found := false
	for _, methods := range spec.Paths {
		for _, op := range methods {
			for status, resp := range op.Responses {
				if !strings.HasPrefix(status, "4") && !strings.HasPrefix(status, "5") {
					continue
				}
				for _, c := range resp.Content {
					if c.Schema.Ref == "" {
						continue
					}
					if !strings.HasSuffix(c.Schema.Ref, "/ErrorResponse") {
						t.Errorf("response %s references %q, want #/components/schemas/ErrorResponse", status, c.Schema.Ref)
					}
					found = true
				}
			}
		}
	}
	if !found {
		t.Fatal("no error responses found to inspect; spec may be empty")
	}
}
