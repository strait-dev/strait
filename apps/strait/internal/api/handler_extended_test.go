package api

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

func TestHandleUpdateEnvironment_Success(t *testing.T) {
	t.Parallel()
	var updatedName string
	ms := &APIStoreMock{
		GetEnvironmentFunc: func(_ context.Context, id string) (*domain.Environment, error) {
			return &domain.Environment{
				ID:        id,
				ProjectID: "proj-1",
				Name:      "staging",
				Slug:      "staging",
				Variables: map[string]string{"FOO": "bar"},
			}, nil
		},
		UpdateEnvironmentFunc: func(_ context.Context, env *domain.Environment) error {
			updatedName = env.Name
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

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
	ms := &APIStoreMock{
		GetEnvironmentFunc: func(_ context.Context, _ string) (*domain.Environment, error) {
			return nil, store.ErrEnvironmentNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/environments/missing", `{"name":"x"}`))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandleUpdateEnvironment_UpdateVariables(t *testing.T) {
	t.Parallel()
	var updatedVars map[string]string
	ms := &APIStoreMock{
		GetEnvironmentFunc: func(_ context.Context, id string) (*domain.Environment, error) {
			return &domain.Environment{
				ID:        id,
				ProjectID: "proj-1",
				Name:      "staging",
				Slug:      "staging",
				Variables: map[string]string{"OLD": "val"},
			}, nil
		},
		UpdateEnvironmentFunc: func(_ context.Context, env *domain.Environment) error {
			updatedVars = env.Variables
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

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
	ms := &APIStoreMock{
		GetEnvironmentFunc: func(_ context.Context, id string) (*domain.Environment, error) {
			return &domain.Environment{ID: id, ProjectID: "proj-1", Name: "staging", Slug: "staging"}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/environments/env-1", `{invalid`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleDeleteEnvironment_Success(t *testing.T) {
	t.Parallel()
	var deletedID string
	ms := &APIStoreMock{
		GetEnvironmentFunc: func(_ context.Context, id string) (*domain.Environment, error) {
			return &domain.Environment{ID: id, ProjectID: "proj-1", Name: "test", Slug: "test"}, nil
		},
		DeleteEnvironmentFunc: func(_ context.Context, id string) error {
			deletedID = id
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

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
	ms := &APIStoreMock{
		GetEnvironmentFunc: func(_ context.Context, _ string) (*domain.Environment, error) {
			return nil, store.ErrEnvironmentNotFound
		},
		DeleteEnvironmentFunc: func(_ context.Context, _ string) error {
			return store.ErrEnvironmentNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/environments/missing", ""))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandleUpdateJobGroup_Success(t *testing.T) {
	t.Parallel()
	var updatedName string
	ms := &APIStoreMock{
		GetJobGroupFunc: func(_ context.Context, id string) (*domain.JobGroup, error) {
			return &domain.JobGroup{ID: id, ProjectID: "proj-1", Name: "Core", Slug: "core"}, nil
		},
		UpdateJobGroupFunc: func(_ context.Context, group *domain.JobGroup) error {
			updatedName = group.Name
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

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
	ms := &APIStoreMock{
		GetJobGroupFunc: func(_ context.Context, _ string) (*domain.JobGroup, error) {
			return nil, store.ErrJobGroupNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/job-groups/missing", `{"name":"x"}`))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandleUpdateJobGroup_UpdateDescription(t *testing.T) {
	t.Parallel()
	var updatedDesc string
	ms := &APIStoreMock{
		GetJobGroupFunc: func(_ context.Context, id string) (*domain.JobGroup, error) {
			return &domain.JobGroup{ID: id, ProjectID: "proj-1", Name: "Core", Slug: "core"}, nil
		},
		UpdateJobGroupFunc: func(_ context.Context, group *domain.JobGroup) error {
			updatedDesc = group.Description
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

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
	ms := &APIStoreMock{
		GetJobGroupFunc: func(_ context.Context, id string) (*domain.JobGroup, error) {
			return &domain.JobGroup{ID: id, ProjectID: "proj-1", Name: "Core", Slug: "core"}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/job-groups/group-1", `{not valid json`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleListRunCheckpoints_Success(t *testing.T) {
	t.Parallel()
	now := time.Now()
	ms := &APIStoreMock{
		ListRunCheckpointsFunc: func(_ context.Context, runID string, _ int, _ *time.Time) ([]domain.RunCheckpoint, error) {
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
	ms := &APIStoreMock{
		ListRunCheckpointsFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.RunCheckpoint, error) {
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
	ms := &APIStoreMock{
		ListRunCheckpointsFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.RunCheckpoint, error) {
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

func TestListRunUsageRoute_NotRegistered(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/runs/run-1/usage", ""))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for launch-inactive usage route, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleListRunToolCalls_RouteLaunchInactive(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/runs/run-1/tool-calls", ""))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for launch-inactive tool-calls route, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleListRunOutputs_Success(t *testing.T) {
	t.Parallel()
	now := time.Now()
	ms := &APIStoreMock{
		ListRunOutputsFunc: func(_ context.Context, runID string, _ int, _ *time.Time) ([]domain.RunOutput, error) {
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
	ms := &APIStoreMock{
		ListRunOutputsFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.RunOutput, error) {
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

func TestHandleAPIReference_Returns200(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

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
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/reference/openapi.json", nil)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	contentType := w.Header().Get("Content-Type")
	if !strings.Contains(contentType, "json") {
		t.Fatalf("expected json content type, got %q", contentType)
	}
	if w.Body.Len() == 0 {
		t.Fatal("expected non-empty OpenAPI spec body")
	}
}

func TestHandleOpenAPISpec_ContainsOpenAPI(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/reference/openapi.json", nil)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "openapi") {
		t.Fatal("expected response to contain 'openapi'")
	}
}

func TestHandleOpenAPISpec_ReturnsPrecompressedGzip(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/reference/openapi.json", nil)
	r.Header.Set("Accept-Encoding", "gzip")
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if got := w.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", got)
	}
	if got := w.Header().Get("Vary"); got != "Accept-Encoding" {
		t.Fatalf("Vary = %q, want Accept-Encoding", got)
	}

	zr, err := gzip.NewReader(w.Body)
	if err != nil {
		t.Fatalf("gzip.NewReader() error = %v", err)
	}
	defer zr.Close()
	body, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("read gzip body: %v", err)
	}
	if !strings.Contains(string(body), "openapi") {
		t.Fatal("expected decompressed response to contain 'openapi'")
	}
	if w.Body.Len() >= len(srv.cachedOpenAPISpec) {
		t.Fatalf("compressed length = %d, uncompressed = %d, want smaller compressed body", w.Body.Len(), len(srv.cachedOpenAPISpec))
	}
}

func TestHandleOpenAPISpec_YAMLRedirect(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/reference/openapi.yaml", nil)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusMovedPermanently {
		t.Fatalf("expected 301, got %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if loc != "/reference/openapi.json" {
		t.Fatalf("expected redirect to /reference/openapi.json, got %q", loc)
	}
}

// openAPIPathParams returns the names of path parameters declared on a
// specific operation (method + path) in the OpenAPI spec.
func openAPIPathParams(t *testing.T, spec map[string]any, path, method string) []string {
	t.Helper()
	paths, ok := spec["paths"].(map[string]any)
	if !ok {
		t.Fatal("spec missing 'paths'")
	}
	pathItem, ok := paths[path].(map[string]any)
	if !ok {
		t.Fatalf("path %q not found in spec", path)
	}
	op, ok := pathItem[method].(map[string]any)
	if !ok {
		t.Fatalf("method %q not found on path %q", method, path)
	}
	params, _ := op["parameters"].([]any)
	var names []string
	for _, p := range params {
		pm, _ := p.(map[string]any)
		if pm["in"] == "path" {
			names = append(names, pm["name"].(string))
		}
	}
	return names
}

func fetchOpenAPISpec(t *testing.T) map[string]any {
	t.Helper()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/reference/openapi.json", nil)
	srv.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var spec map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &spec); err != nil {
		t.Fatalf("failed to unmarshal OpenAPI spec: %v", err)
	}
	return spec
}

func TestOpenAPISpec_DeleteEventSubscription_HasSourceIDParam(t *testing.T) {
	t.Parallel()
	spec := fetchOpenAPISpec(t)
	params := openAPIPathParams(t, spec, "/v1/event-sources/{sourceID}/subscriptions/{subID}", "delete")

	want := map[string]bool{"sourceID": false, "subID": false}
	for _, name := range params {
		if _, ok := want[name]; ok {
			want[name] = true
		} else {
			t.Errorf("unexpected path param %q", name)
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("missing expected path param %q", name)
		}
	}
	if len(params) != 2 {
		t.Errorf("expected exactly 2 path params, got %d: %v", len(params), params)
	}
}

func TestOpenAPISpec_RetryWebhookDeliveryLegacy_NoPhantomParams(t *testing.T) {
	t.Parallel()
	spec := fetchOpenAPISpec(t)
	params := openAPIPathParams(t, spec, "/v1/webhook-deliveries/{deliveryID}/retry", "post")

	if len(params) != 1 {
		t.Fatalf("expected exactly 1 path param, got %d: %v", len(params), params)
	}
	if params[0] != "deliveryID" {
		t.Errorf("expected path param 'deliveryID', got %q", params[0])
	}
}

func TestOpenAPISpec_RetryWebhookDelivery_NoPhantomParams(t *testing.T) {
	t.Parallel()
	spec := fetchOpenAPISpec(t)
	params := openAPIPathParams(t, spec, "/v1/webhooks/deliveries/{id}/retry", "post")

	if len(params) != 1 {
		t.Fatalf("expected exactly 1 path param, got %d: %v", len(params), params)
	}
	if params[0] != "id" {
		t.Errorf("expected path param 'id', got %q", params[0])
	}
}

func TestOpenAPISpec_MissingEndpoints_AreRegistered(t *testing.T) {
	t.Parallel()
	spec := fetchOpenAPISpec(t)

	paths, ok := spec["paths"].(map[string]any)
	if !ok {
		t.Fatal("spec missing 'paths'")
	}

	required := []struct {
		path   string
		method string
	}{
		{"/v1/sse-token", "post"},
		{"/v1/api-keys/expiring-soon", "get"},
		{"/v1/audit-events/verify", "get"},
		{"/v1/admin/outbox/{outbox_id}/retry", "post"},
		{"/v1/admin/outbox/{outbox_id}/purge", "post"},
	}

	for _, r := range required {
		pathItem, ok := paths[r.path].(map[string]any)
		if !ok {
			t.Errorf("path %q not found in spec", r.path)
			continue
		}
		if _, ok := pathItem[r.method]; !ok {
			t.Errorf("method %q not found on path %q", r.method, r.path)
		}
	}
}

func TestOpenAPISpec_AdminOutboxRow_ExposesRetryLineageField(t *testing.T) {
	t.Parallel()

	spec := fetchOpenAPISpec(t)
	components, ok := spec["components"].(map[string]any)
	if !ok {
		t.Fatal("spec missing components")
	}
	schemas, ok := components["schemas"].(map[string]any)
	if !ok {
		t.Fatal("spec missing component schemas")
	}

	var found bool
	for _, raw := range schemas {
		schema, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		properties, ok := schema["properties"].(map[string]any)
		if !ok {
			continue
		}
		if _, ok := properties["retry_of_outbox_id"]; ok {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected retry_of_outbox_id to appear in an OpenAPI schema")
	}
}
