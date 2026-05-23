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

// TestOpenAPISchema_IncludesSingletonEndpoints is a drift guard asserting the
// singleton holder inspection endpoints (STR-542) are registered with Huma and
// therefore included in the runtime-generated OpenAPI spec.
func TestOpenAPISchema_IncludesSingletonEndpoints(t *testing.T) {
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

	for _, path := range []string{
		"/v1/jobs/{jobID}/singletons",
		"/v1/workflows/{workflowID}/singletons",
	} {
		methods, ok := spec.Paths[path]
		if !ok {
			t.Errorf("openapi spec missing path %q", path)
			continue
		}
		if _, ok := methods["get"]; !ok {
			t.Errorf("openapi spec path %q missing method %q", path, "get")
		}
	}
}

// TestOpenAPISchema_IncludesBundleImportEndpoint is a drift guard asserting the
// config-as-code bundle import endpoint is registered with Huma and therefore
// included in the runtime-generated OpenAPI spec.
func TestOpenAPISchema_IncludesBundleImportEndpoint(t *testing.T) {
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

	const path = "/v1/projects/{projectID}/bundle/import"
	methods, ok := spec.Paths[path]
	if !ok {
		t.Fatalf("openapi spec missing path %q", path)
	}
	if _, ok := methods["post"]; !ok {
		t.Errorf("openapi spec path %q missing method %q", path, "post")
	}
}

func TestOpenAPISchema_DoesNotExposeRemovedCodeDeploymentEndpoints(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/reference/openapi.json", nil)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	raw := w.Body.String()
	for _, stale := range []string{
		"/v1/jobs/{jobID}/deployments",
		"code-deployment",
		"stream-deployment-logs",
		"machine_id",
	} {
		if strings.Contains(raw, stale) {
			t.Fatalf("openapi spec contains removed code-deployment surface %q", stale)
		}
	}
}

func TestOpenAPISchema_IncludesRegionsEndpoint(t *testing.T) {
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
	if _, ok := spec.Paths["/v1/regions"]["get"]; !ok {
		t.Fatal("openapi spec is missing GET /v1/regions")
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
