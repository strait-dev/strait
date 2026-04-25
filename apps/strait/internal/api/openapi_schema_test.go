package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
