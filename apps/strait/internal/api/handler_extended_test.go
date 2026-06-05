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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.Equal(t, http.
		StatusOK,
		w.Code)
	require.Equal(t, "production",

		updatedName)

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
	require.Equal(t, http.
		StatusNotFound,
		w.Code)

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
	require.Equal(t, http.
		StatusOK,
		w.Code)
	require.Equal(t, "val2",
		updatedVars["NEW"])

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
	require.Equal(t, http.
		StatusBadRequest,
		w.Code)

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
	require.Equal(t, http.
		StatusNoContent,
		w.Code)
	require.Equal(t, "env-1",
		deletedID,
	)

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
	require.Equal(t, http.
		StatusNotFound,
		w.Code)

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
	require.Equal(t, http.
		StatusOK,
		w.Code)
	require.Equal(t, "Updated",
		updatedName,
	)

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
	require.Equal(t, http.
		StatusNotFound,
		w.Code)

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
	require.Equal(t, http.
		StatusOK,
		w.Code)
	require.Equal(t, "A group of core jobs",

		updatedDesc,
	)

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
	require.Equal(t, http.
		StatusBadRequest,
		w.Code)

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
	require.Equal(t, http.
		StatusOK,
		w.Code)

	var checkpoints []domain.RunCheckpoint
	decodePaginatedList(t, w.Body.Bytes(), &checkpoints)
	require.Len(t, checkpoints,
		2)

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
	require.Equal(t, http.
		StatusOK,
		w.Code)

	var checkpoints []domain.RunCheckpoint
	decodePaginatedList(t, w.Body.Bytes(), &checkpoints)
	require.Len(t, checkpoints,
		0)

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
	require.Equal(t, http.
		StatusInternalServerError,

		w.Code,
	)

}

func TestListRunUsageRoute_NotRegistered(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/runs/run-1/usage", ""))
	require.Equal(t, http.
		StatusNotFound,
		w.Code)

}

func TestHandleListRunToolCalls_RouteLaunchInactive(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/runs/run-1/tool-calls", ""))
	require.Equal(t, http.
		StatusNotFound,
		w.Code)

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
	require.Equal(t, http.
		StatusOK,
		w.Code)

	var outputs []domain.RunOutput
	decodePaginatedList(t, w.Body.Bytes(), &outputs)
	require.Len(t, outputs,
		1)
	require.Equal(t, "result",
		outputs[0].OutputKey,
	)

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
	require.Equal(t, http.
		StatusOK,
		w.Code)

}

func TestHandleAPIReference_Returns200(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	// API reference endpoint is public (no auth required at route level, but let's use authed for consistency)
	r := httptest.NewRequest(http.MethodGet, "/reference", nil)
	srv.ServeHTTP(w, r)
	require.Equal(t, http.
		StatusOK,
		w.Code)

	contentType := w.Header().Get("Content-Type")
	require.True(t, strings.Contains(contentType, "text/html"))

}

func TestHandleOpenAPISpec_Returns200(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/reference/openapi.json", nil)
	srv.ServeHTTP(w, r)
	require.Equal(t, http.
		StatusOK,
		w.Code)

	contentType := w.Header().Get("Content-Type")
	require.True(t, strings.Contains(contentType, "json"))
	require.NotEqual(t,
		0, w.Body.
			Len())

}

func TestHandleOpenAPISpec_ContainsOpenAPI(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/reference/openapi.json", nil)
	srv.ServeHTTP(w, r)
	require.Equal(t, http.
		StatusOK,
		w.Code)
	require.True(t, strings.Contains(w.Body.String(), "openapi"))

}

func TestHandleOpenAPISpec_ReturnsPrecompressedGzip(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/reference/openapi.json", nil)
	r.Header.Set("Accept-Encoding", "gzip")
	srv.ServeHTTP(w, r)
	require.Equal(t, http.
		StatusOK,
		w.Code)
	require.Equal(t, "gzip",
		w.Header().Get("Content-Encoding"))
	require.Equal(t, "Accept-Encoding",

		w.Header().
			Get("Vary"))

	zr, err := gzip.NewReader(w.Body)
	require.NoError(t,
		err)

	defer zr.Close()
	body, err := io.ReadAll(zr)
	require.NoError(t,
		err)
	require.True(t, strings.Contains(string(body),
		"openapi",
	))
	require.False(t, w.
		Body.Len() >=
		len(srv.cachedOpenAPISpec))

}

func TestHandleOpenAPISpec_YAMLRedirect(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/reference/openapi.yaml", nil)
	srv.ServeHTTP(w, r)
	require.Equal(t, http.
		StatusMovedPermanently,
		w.
			Code)

	loc := w.Header().Get("Location")
	require.Equal(t, "/reference/openapi.json",

		loc,
	)

}

// openAPIPathParams returns the names of path parameters declared on a
// specific operation (method + path) in the OpenAPI spec.
func openAPIPathParams(t *testing.T, spec map[string]any, path, method string) []string {
	t.Helper()
	paths, ok := spec["paths"].(map[string]any)
	require.True(t, ok)

	pathItem, ok := paths[path].(map[string]any)
	require.True(t, ok)

	op, ok := pathItem[method].(map[string]any)
	require.True(t, ok)

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
	require.Equal(t, http.
		StatusOK,
		w.Code)

	var spec map[string]any
	require.NoError(t,
		json.Unmarshal(w.Body.Bytes(), &spec))

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
			assert.Failf(t, "test failure",

				"unexpected path param %q", name)
		}
	}
	for _, found := range want {
		assert.True(t, found)

	}
	assert.Len(t, params,
		2)

}

func TestOpenAPISpec_RetryWebhookDeliveryLegacy_NoPhantomParams(t *testing.T) {
	t.Parallel()
	spec := fetchOpenAPISpec(t)
	params := openAPIPathParams(t, spec, "/v1/webhook-deliveries/{deliveryID}/retry", "post")
	require.Len(t, params,
		1)
	assert.Equal(t, "deliveryID",

		params[0])

}

func TestOpenAPISpec_RetryWebhookDelivery_NoPhantomParams(t *testing.T) {
	t.Parallel()
	spec := fetchOpenAPISpec(t)
	params := openAPIPathParams(t, spec, "/v1/webhooks/deliveries/{id}/retry", "post")
	require.Len(t, params,
		1)
	assert.Equal(t, "id",
		params[0])

}

func TestOpenAPISpec_MissingEndpoints_AreRegistered(t *testing.T) {
	t.Parallel()
	spec := fetchOpenAPISpec(t)

	paths, ok := spec["paths"].(map[string]any)
	require.True(t, ok)

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
			assert.Failf(t, "test failure",

				"path %q not found in spec", r.path)
			continue
		}
		if _, ok := pathItem[r.method]; !ok {
			assert.Failf(t, "test failure",

				"method %q not found on path %q", r.method, r.path)
		}
	}
}

func TestOpenAPISpec_AdminOutboxRow_ExposesRetryLineageField(t *testing.T) {
	t.Parallel()

	spec := fetchOpenAPISpec(t)
	components, ok := spec["components"].(map[string]any)
	require.True(t, ok)

	schemas, ok := components["schemas"].(map[string]any)
	require.True(t, ok)

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
	require.True(t, found)

}
