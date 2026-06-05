package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHandleStraitJSONSchema_Returns200 verifies the schema endpoint returns 200.
func TestHandleStraitJSONSchema_Returns200(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/schemas/v1/strait.json", nil)
	srv.ServeHTTP(w, r)
	require.Equal(t, http.
		StatusOK,
		w.Code)
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
	require.Contains(t, ct, "application/schema+json")
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
	require.False(t, !strings.Contains(cc, "public") ||
		!strings.Contains(cc,
			"max-age=86400",
		))
}

// TestHandleStraitJSONSchema_IsValidJSON verifies the response body is valid JSON.
func TestHandleStraitJSONSchema_IsValidJSON(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/schemas/v1/strait.json", nil)
	srv.ServeHTTP(w, r)

	var schema map[string]any
	require.NoError(t,
		json.Unmarshal(w.Body.Bytes(), &schema))
	require.NotEmpty(t,
		schema)
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
	require.NoError(t,
		json.Unmarshal(w.Body.Bytes(), &schema))

	for _, key := range []string{"$schema", "$id", "title", "type", "properties"} {
		if _, ok := schema[key]; !ok {
			assert.Failf(t, "test failure",

				"schema missing required top-level key %q", key)
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
	require.NoError(t,
		json.Unmarshal(w.Body.Bytes(), &schema))

	id, _ := schema["$id"].(string)
	const want = "https://api.strait.dev/schemas/v1/strait.json"
	assert.Equal(t, want,
		id)
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
	require.NotEqual(t,
		http.StatusUnauthorized,
		w.Code)
	require.Equal(t, http.
		StatusOK,
		w.Code)
}

func TestHandleStraitJSONSchema_DoesNotAdvertiseManagedRuntimeBuilds(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/schemas/v1/strait.json", nil)
	srv.ServeHTTP(w, r)
	require.Equal(t, http.
		StatusOK,
		w.Code)

	body := w.Body.String()
	for _, stale := range []string{
		"container image",
		"managed container",
		"COMPUTE_RUNTIME",
		"strait build",
	} {
		require.NotContains(t, body, stale)
	}
}
