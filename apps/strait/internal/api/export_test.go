package api

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

func TestExportJobs_JSON_ReturnsArray(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		StreamJobsFunc: func(_ context.Context, _ string, fn func(*domain.Job) error) error {
			_ = fn(&domain.Job{ID: "job-1", Name: "test-job"})
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	r := httptest.NewRequest(http.MethodGet, "/v1/export/jobs", nil)
	r = r.WithContext(context.WithValue(r.Context(), ctxProjectIDKey, "proj-1"))
	w := httptest.NewRecorder()

	TypedHandler(srv, http.StatusOK, srv.handleExportJobs)(w, r)
	require.Equal(t, "application/json",
		w.Header().Get("Content-Type"))
	require.Equal(t, "attachment; filename=jobs.json",

		w.Header().Get("Content-Disposition"))

	body := w.Body.String()
	require.False(t, !strings.HasPrefix(body, "[") ||
		!strings.HasSuffix(body, "]"),
	)
}

func TestExportJobs_NDJSON_ReturnsLineDelimited(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		StreamJobsFunc: func(_ context.Context, _ string, fn func(*domain.Job) error) error {
			_ = fn(&domain.Job{ID: "job-1", Name: "a"})
			_ = fn(&domain.Job{ID: "job-2", Name: "b"})
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	r := httptest.NewRequest(http.MethodGet, "/v1/export/jobs?format=ndjson", nil)
	r = r.WithContext(context.WithValue(r.Context(), ctxProjectIDKey, "proj-1"))
	w := httptest.NewRecorder()

	TypedHandler(srv, http.StatusOK, srv.handleExportJobs)(w, r)
	require.Equal(t, "application/x-ndjson",
		w.Header().Get("Content-Type"))

	lines := strings.Split(strings.TrimSpace(w.Body.String()), "\n")
	require.Len(t,
		lines, 2)
}

func TestExportRuns_RequiresFromAndTo(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	r := httptest.NewRequest(http.MethodGet, "/v1/export/runs", nil)
	r = r.WithContext(context.WithValue(r.Context(), ctxProjectIDKey, "proj-1"))
	w := httptest.NewRecorder()

	TypedHandler(srv, http.StatusOK, srv.handleExportRuns)(w, r)
	require.Equal(t, http.StatusBadRequest,
		w.Code,
	)
}

func TestExportRuns_CSV_HasHeader(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	ms := &APIStoreMock{
		StreamRunsFunc: func(_ context.Context, _ string, _, _ time.Time, fn func(*domain.JobRun) error) error {
			_ = fn(&domain.JobRun{ID: "run-1", JobID: "job-1", Status: "completed", CreatedAt: now})
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	from := now.Add(-1 * time.Hour).Format(time.RFC3339)
	to := now.Add(1 * time.Hour).Format(time.RFC3339)
	r := httptest.NewRequest(http.MethodGet, "/v1/export/runs?format=csv&from="+from+"&to="+to, nil)
	r = r.WithContext(context.WithValue(r.Context(), ctxProjectIDKey, "proj-1"))
	w := httptest.NewRecorder()

	TypedHandler(srv, http.StatusOK, srv.handleExportRuns)(w, r)
	require.Equal(t, "text/csv",
		w.Header().Get("Content-Type"))

	lines := strings.Split(strings.TrimSpace(w.Body.String()), "\n")
	require.GreaterOrEqual(t, len(lines), 2)
	require.Contains(
		t, lines[0], "id")
}

func TestExportRuns_MaxWindow90Days(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	from := time.Now().Add(-100 * 24 * time.Hour).Format(time.RFC3339)
	to := time.Now().Format(time.RFC3339)
	r := httptest.NewRequest(http.MethodGet, "/v1/export/runs?from="+from+"&to="+to, nil)
	r = r.WithContext(context.WithValue(r.Context(), ctxProjectIDKey, "proj-1"))
	w := httptest.NewRecorder()

	TypedHandler(srv, http.StatusOK, srv.handleExportRuns)(w, r)
	require.Equal(t, http.StatusBadRequest,
		w.Code,
	)
}

func TestExport_InvalidFormat_Returns400(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	r := httptest.NewRequest(http.MethodGet, "/v1/export/jobs?format=xml", nil)
	r = r.WithContext(context.WithValue(r.Context(), ctxProjectIDKey, "proj-1"))
	w := httptest.NewRecorder()

	TypedHandler(srv, http.StatusOK, srv.handleExportJobs)(w, r)
	require.Equal(t, http.StatusBadRequest,
		w.Code,
	)
}

func TestExportWorkflows_EmptyProject_ReturnsEmptyArray(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		StreamWorkflowsFunc: func(_ context.Context, _ string, _ func(*domain.Workflow) error) error {
			return nil // no rows
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	r := httptest.NewRequest(http.MethodGet, "/v1/export/workflows", nil)
	r = r.WithContext(context.WithValue(r.Context(), ctxProjectIDKey, "proj-1"))
	w := httptest.NewRecorder()

	TypedHandler(srv, http.StatusOK, srv.handleExportWorkflows)(w, r)

	body := w.Body.String()
	require.Equal(t, "[]", body)
}

func TestExportRuns_NDJSON_ReturnsLineDelimited(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	ms := &APIStoreMock{
		StreamRunsFunc: func(_ context.Context, _ string, _, _ time.Time, fn func(*domain.JobRun) error) error {
			_ = fn(&domain.JobRun{ID: "run-1", JobID: "job-1", Status: "completed", CreatedAt: now})
			_ = fn(&domain.JobRun{ID: "run-2", JobID: "job-1", Status: "failed", CreatedAt: now})
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	from := now.Add(-1 * time.Hour).Format(time.RFC3339)
	to := now.Add(1 * time.Hour).Format(time.RFC3339)
	r := httptest.NewRequest(http.MethodGet, "/v1/export/runs?format=ndjson&from="+from+"&to="+to, nil)
	r = r.WithContext(context.WithValue(r.Context(), ctxProjectIDKey, "proj-1"))
	w := httptest.NewRecorder()

	TypedHandler(srv, http.StatusOK, srv.handleExportRuns)(w, r)
	require.Equal(t, "application/x-ndjson",
		w.Header().Get("Content-Type"))

	lines := strings.Split(strings.TrimSpace(w.Body.String()), "\n")
	require.Len(t,
		lines, 2)
}

func TestExport_FromAfterTo_Returns400(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	r := httptest.NewRequest(http.MethodGet, "/v1/export/runs?from=2025-01-02T00:00:00Z&to=2025-01-01T00:00:00Z", nil)
	r = r.WithContext(context.WithValue(r.Context(), ctxProjectIDKey, "proj-1"))
	w := httptest.NewRecorder()

	TypedHandler(srv, http.StatusOK, srv.handleExportRuns)(w, r)
	require.Equal(t, http.StatusBadRequest,
		w.Code,
	)
}

func TestExport_MalformedRFC3339_Returns400(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	r := httptest.NewRequest(http.MethodGet, "/v1/export/runs?from=not-a-date&to=also-not", nil)
	r = r.WithContext(context.WithValue(r.Context(), ctxProjectIDKey, "proj-1"))
	w := httptest.NewRecorder()

	TypedHandler(srv, http.StatusOK, srv.handleExportRuns)(w, r)
	require.Equal(t, http.StatusBadRequest,
		w.Code,
	)
}

func TestExport_EmptyFormat_DefaultsJSON(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		StreamJobsFunc: func(_ context.Context, _ string, fn func(*domain.Job) error) error {
			_ = fn(&domain.Job{ID: "j1"})
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	r := httptest.NewRequest(http.MethodGet, "/v1/export/jobs", nil)
	r = r.WithContext(context.WithValue(r.Context(), ctxProjectIDKey, "proj-1"))
	w := httptest.NewRecorder()

	TypedHandler(srv, http.StatusOK, srv.handleExportJobs)(w, r)
	require.Equal(t, "application/json",
		w.Header().Get("Content-Type"))

	var arr []json.RawMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &arr))
}

func TestExport_NoProjectID_Returns400(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	r := httptest.NewRequest(http.MethodGet, "/v1/export/jobs", nil)
	// No project ID in context.
	w := httptest.NewRecorder()

	TypedHandler(srv, http.StatusOK, srv.handleExportJobs)(w, r)
	require.Equal(t, http.StatusBadRequest,
		w.Code,
	)
}

func TestHandleExportWorkflows_JSON_ReturnsArray(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		StreamWorkflowsFunc: func(_ context.Context, _ string, fn func(*domain.Workflow) error) error {
			_ = fn(&domain.Workflow{ID: "wf-1", Name: "Deploy Pipeline", Slug: "deploy"})
			_ = fn(&domain.Workflow{ID: "wf-2", Name: "Cleanup", Slug: "cleanup"})
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	r := httptest.NewRequest(http.MethodGet, "/v1/export/workflows?format=json", nil)
	r = r.WithContext(context.WithValue(r.Context(), ctxProjectIDKey, "proj-1"))
	w := httptest.NewRecorder()

	TypedHandler(srv, http.StatusOK, srv.handleExportWorkflows)(w, r)
	require.Equal(t, "application/json",
		w.Header().Get("Content-Type"))
	require.Equal(t, "attachment; filename=workflows.json",

		w.
			Header().Get("Content-Disposition"))

	var arr []json.RawMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &arr))
	require.Len(t,
		arr, 2)
}

func TestHandleExportWorkflows_NDJSON_ReturnsLineDelimited(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		StreamWorkflowsFunc: func(_ context.Context, _ string, fn func(*domain.Workflow) error) error {
			_ = fn(&domain.Workflow{ID: "wf-1", Name: "A"})
			_ = fn(&domain.Workflow{ID: "wf-2", Name: "B"})
			_ = fn(&domain.Workflow{ID: "wf-3", Name: "C"})
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	r := httptest.NewRequest(http.MethodGet, "/v1/export/workflows?format=ndjson", nil)
	r = r.WithContext(context.WithValue(r.Context(), ctxProjectIDKey, "proj-1"))
	w := httptest.NewRecorder()

	TypedHandler(srv, http.StatusOK, srv.handleExportWorkflows)(w, r)
	require.Equal(t, "application/x-ndjson",
		w.Header().Get("Content-Type"))

	lines := strings.Split(strings.TrimSpace(w.Body.String()), "\n")
	require.Len(t,
		lines, 3)

	for _, line := range lines {
		var obj map[string]any
		require.NoError(t, json.Unmarshal([]byte(line),
			&obj))
	}
}

func TestHandleExportWorkflows_InvalidFormat_Returns400(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	r := httptest.NewRequest(http.MethodGet, "/v1/export/workflows?format=csv", nil)
	r = r.WithContext(context.WithValue(r.Context(), ctxProjectIDKey, "proj-1"))
	w := httptest.NewRecorder()

	TypedHandler(srv, http.StatusOK, srv.handleExportWorkflows)(w, r)
	require.Equal(t, http.StatusBadRequest,
		w.Code,
	)
}

func TestHandleExportWorkflows_NoProjectID_Returns400(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	r := httptest.NewRequest(http.MethodGet, "/v1/export/workflows", nil)
	w := httptest.NewRecorder()

	TypedHandler(srv, http.StatusOK, srv.handleExportWorkflows)(w, r)
	require.Equal(t, http.StatusBadRequest,
		w.Code,
	)
}

func TestExportRuns_CSV_CommaInErrorMessage(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	ms := &APIStoreMock{
		StreamRunsFunc: func(_ context.Context, _ string, _, _ time.Time, fn func(*domain.JobRun) error) error {
			_ = fn(&domain.JobRun{
				ID:        "run-1",
				JobID:     "job-1",
				Status:    "failed",
				CreatedAt: now,
				Error:     "connection failed, retries exhausted, giving up",
			})
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	from := now.Add(-1 * time.Hour).Format(time.RFC3339)
	to := now.Add(1 * time.Hour).Format(time.RFC3339)
	r := httptest.NewRequest(http.MethodGet, "/v1/export/runs?format=csv&from="+from+"&to="+to, nil)
	r = r.WithContext(context.WithValue(r.Context(), ctxProjectIDKey, "proj-1"))
	w := httptest.NewRecorder()

	TypedHandler(srv, http.StatusOK, srv.handleExportRuns)(w, r)
	require.Equal(t, "text/csv",
		w.Header().Get("Content-Type"))

	// Parse as CSV to verify the comma-containing error is properly escaped.
	csvReader := csv.NewReader(strings.NewReader(w.Body.String()))
	records, err := csvReader.ReadAll()
	require.NoError(t, err)
	require.Len(t,
		records, 2)

	// header + 1 data row

	// The error column is the last field (index 8).
	errorField := records[1][8]
	require.Equal(t, "connection failed, retries exhausted, giving up",

		errorField)
}

func TestExportJobs_WebhookSecretSanitized(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		StreamJobsFunc: func(_ context.Context, _ string, fn func(*domain.Job) error) error {
			_ = fn(&domain.Job{
				ID:            "job-1",
				Name:          "webhook-job",
				WebhookSecret: "supersecretvalue",
			})
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	// Test JSON format.
	r := httptest.NewRequest(http.MethodGet, "/v1/export/jobs?format=json", nil)
	r = r.WithContext(context.WithValue(r.Context(), ctxProjectIDKey, "proj-1"))
	w := httptest.NewRecorder()

	TypedHandler(srv, http.StatusOK, srv.handleExportJobs)(w, r)

	body := w.Body.String()
	require.NotContains(t, body, "supersecretvalue")
}

func TestExportJobs_NDJSON_WebhookSecretSanitized(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		StreamJobsFunc: func(_ context.Context, _ string, fn func(*domain.Job) error) error {
			_ = fn(&domain.Job{
				ID:            "job-1",
				Name:          "webhook-job",
				WebhookSecret: "anothersecret",
			})
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	r := httptest.NewRequest(http.MethodGet, "/v1/export/jobs?format=ndjson", nil)
	r = r.WithContext(context.WithValue(r.Context(), ctxProjectIDKey, "proj-1"))
	w := httptest.NewRecorder()

	TypedHandler(srv, http.StatusOK, srv.handleExportJobs)(w, r)

	body := w.Body.String()
	require.NotContains(t, body, "anothersecret")
}

func TestExportJobs_JSON_ValidStructure(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		StreamJobsFunc: func(_ context.Context, _ string, fn func(*domain.Job) error) error {
			_ = fn(&domain.Job{ID: "job-1", Name: "first"})
			_ = fn(&domain.Job{ID: "job-2", Name: "second"})
			_ = fn(&domain.Job{ID: "job-3", Name: "third"})
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	r := httptest.NewRequest(http.MethodGet, "/v1/export/jobs?format=json", nil)
	r = r.WithContext(context.WithValue(r.Context(), ctxProjectIDKey, "proj-1"))
	w := httptest.NewRecorder()

	TypedHandler(srv, http.StatusOK, srv.handleExportJobs)(w, r)

	var arr []json.RawMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &arr))
	require.Len(t,
		arr, 3)

	// Each item should be a valid JSON object with an id field.
	for i, raw := range arr {
		var obj map[string]any
		require.NoError(t, json.Unmarshal(raw, &obj))

		if _, ok := obj["id"]; !ok {
			require.Failf(t, "test failure",

				"item %d missing 'id' field", i)
		}
	}
}

func TestExport_SQLInjectionInParams(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	// Use url-safe SQL injection string.
	r := httptest.NewRequest(http.MethodGet, "/v1/export/runs?from=1%27%3BDROP+TABLE+job_runs&to=2025-01-01T00:00:00Z", nil)
	r = r.WithContext(context.WithValue(r.Context(), ctxProjectIDKey, "proj-1"))
	w := httptest.NewRecorder()

	TypedHandler(srv, http.StatusOK, srv.handleExportRuns)(w, r)
	require.Equal(t, http.StatusBadRequest,
		w.Code,
	)

	// Should fail with 400 (invalid RFC3339), not succeed.
}
