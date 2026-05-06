package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestHandleStraitJSONSchema_Returns200 verifies the schema endpoint returns 200.
func TestHandleStraitJSONSchema_Returns200(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/schemas/v1/strait.json", nil)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// TestHandleStraitJSONSchema_ContentType verifies the response uses the
// application/schema+json content type required by JSON Schema tooling.
func TestHandleStraitJSONSchema_ContentType(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/schemas/v1/strait.json", nil)
	srv.ServeHTTP(w, r)

	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/schema+json") {
		t.Fatalf("expected Content-Type application/schema+json, got %q", ct)
	}
}

// TestHandleStraitJSONSchema_CacheControl verifies a 24-hour public cache
// header so CDN edges and IDE plugins don't hammer the API.
func TestHandleStraitJSONSchema_CacheControl(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/schemas/v1/strait.json", nil)
	srv.ServeHTTP(w, r)

	cc := w.Header().Get("Cache-Control")
	if !strings.Contains(cc, "public") || !strings.Contains(cc, "max-age=86400") {
		t.Fatalf("expected Cache-Control: public, max-age=86400, got %q", cc)
	}
}

// TestHandleStraitJSONSchema_IsValidJSON verifies the response body is valid JSON.
func TestHandleStraitJSONSchema_IsValidJSON(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/schemas/v1/strait.json", nil)
	srv.ServeHTTP(w, r)

	var schema map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &schema); err != nil {
		t.Fatalf("response body is not valid JSON: %v", err)
	}
	if len(schema) == 0 {
		t.Fatal("expected non-empty schema object")
	}
}

// TestHandleStraitJSONSchema_HasRequiredTopLevelKeys verifies the schema
// contains the fields required for JSON Schema tooling to recognise it.
func TestHandleStraitJSONSchema_HasRequiredTopLevelKeys(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/schemas/v1/strait.json", nil)
	srv.ServeHTTP(w, r)

	var schema map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &schema); err != nil {
		t.Fatalf("failed to parse schema: %v", err)
	}

	for _, key := range []string{"$schema", "$id", "title", "type", "properties"} {
		if _, ok := schema[key]; !ok {
			t.Errorf("schema missing required top-level key %q", key)
		}
	}
}

// TestHandleStraitJSONSchema_SchemaIDMatchesRoute verifies the $id field
// matches the URL at which the schema is actually served. A mismatch breaks
// JSON Schema tooling that uses $id for deduplication.
func TestHandleStraitJSONSchema_SchemaIDMatchesRoute(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/schemas/v1/strait.json", nil)
	srv.ServeHTTP(w, r)

	var schema map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &schema); err != nil {
		t.Fatalf("failed to parse schema: %v", err)
	}

	id, _ := schema["$id"].(string)
	const want = "https://api.strait.dev/schemas/v1/strait.json"
	if id != want {
		t.Errorf("schema $id = %q, want %q", id, want)
	}
}

// TestHandleStraitJSONSchema_NoAuthRequired verifies the endpoint is public —
// an unauthenticated request must return 200, not 401.
func TestHandleStraitJSONSchema_NoAuthRequired(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	// Deliberately send no Authorization header.
	r := httptest.NewRequest(http.MethodGet, "/schemas/v1/strait.json", nil)
	srv.ServeHTTP(w, r)

	if w.Code == http.StatusUnauthorized {
		t.Fatalf("schema endpoint requires auth but should be public; got 401")
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestHandleStraitJSONSchema_DoesNotAdvertiseManagedRuntimeBuilds(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/schemas/v1/strait.json", nil)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	body := w.Body.String()
	for _, stale := range []string{
		"container image",
		"managed container",
		"COMPUTE_RUNTIME",
		"strait build",
	} {
		if strings.Contains(body, stale) {
			t.Fatalf("strait.json schema contains removed runtime/build wording %q", stale)
		}
	}
}
