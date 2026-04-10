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

	if w.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("expected application/json, got %s", w.Header().Get("Content-Type"))
	}
	if w.Header().Get("Content-Disposition") != "attachment; filename=jobs.json" {
		t.Fatalf("expected jobs.json disposition, got %s", w.Header().Get("Content-Disposition"))
	}
	body := w.Body.String()
	if !strings.HasPrefix(body, "[") || !strings.HasSuffix(body, "]") {
		t.Fatalf("expected JSON array, got: %s", body)
	}
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

	if w.Header().Get("Content-Type") != "application/x-ndjson" {
		t.Fatalf("expected ndjson content type, got %s", w.Header().Get("Content-Type"))
	}
	lines := strings.Split(strings.TrimSpace(w.Body.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 NDJSON lines, got %d", len(lines))
	}
}

func TestExportRuns_RequiresFromAndTo(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	r := httptest.NewRequest(http.MethodGet, "/v1/export/runs", nil)
	r = r.WithContext(context.WithValue(r.Context(), ctxProjectIDKey, "proj-1"))
	w := httptest.NewRecorder()

	TypedHandler(srv, http.StatusOK, srv.handleExportRuns)(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
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

	if w.Header().Get("Content-Type") != "text/csv" {
		t.Fatalf("expected text/csv, got %s", w.Header().Get("Content-Type"))
	}
	lines := strings.Split(strings.TrimSpace(w.Body.String()), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected header + data rows, got %d lines", len(lines))
	}
	if !strings.Contains(lines[0], "id") {
		t.Fatal("CSV header should contain 'id'")
	}
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

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for exceeded window, got %d", w.Code)
	}
}

func TestExport_InvalidFormat_Returns400(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	r := httptest.NewRequest(http.MethodGet, "/v1/export/jobs?format=xml", nil)
	r = r.WithContext(context.WithValue(r.Context(), ctxProjectIDKey, "proj-1"))
	w := httptest.NewRecorder()

	TypedHandler(srv, http.StatusOK, srv.handleExportJobs)(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid format, got %d", w.Code)
	}
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
	if body != "[]" {
		t.Fatalf("expected empty JSON array, got: %s", body)
	}
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

	if w.Header().Get("Content-Type") != "application/x-ndjson" {
		t.Fatalf("expected ndjson, got %s", w.Header().Get("Content-Type"))
	}
	lines := strings.Split(strings.TrimSpace(w.Body.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 NDJSON lines, got %d", len(lines))
	}
}

// ─── Adversarial tests ───────────────────────────────────────────────────────.

func TestExport_FromAfterTo_Returns400(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	r := httptest.NewRequest(http.MethodGet, "/v1/export/runs?from=2025-01-02T00:00:00Z&to=2025-01-01T00:00:00Z", nil)
	r = r.WithContext(context.WithValue(r.Context(), ctxProjectIDKey, "proj-1"))
	w := httptest.NewRecorder()

	TypedHandler(srv, http.StatusOK, srv.handleExportRuns)(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for from > to, got %d", w.Code)
	}
}

func TestExport_MalformedRFC3339_Returns400(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	r := httptest.NewRequest(http.MethodGet, "/v1/export/runs?from=not-a-date&to=also-not", nil)
	r = r.WithContext(context.WithValue(r.Context(), ctxProjectIDKey, "proj-1"))
	w := httptest.NewRecorder()

	TypedHandler(srv, http.StatusOK, srv.handleExportRuns)(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for malformed date, got %d", w.Code)
	}
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

	if w.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("expected default json, got %s", w.Header().Get("Content-Type"))
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &arr); err != nil {
		t.Fatalf("response should be valid JSON array: %v", err)
	}
}

func TestExport_NoProjectID_Returns400(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	r := httptest.NewRequest(http.MethodGet, "/v1/export/jobs", nil)
	// No project ID in context.
	w := httptest.NewRecorder()

	TypedHandler(srv, http.StatusOK, srv.handleExportJobs)(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing project ID, got %d", w.Code)
	}
}

// ─── handleExportWorkflows tests ─────────────────────────────────────────────.

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

	if w.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("expected application/json, got %s", w.Header().Get("Content-Type"))
	}
	if w.Header().Get("Content-Disposition") != "attachment; filename=workflows.json" {
		t.Fatalf("expected workflows.json disposition, got %s", w.Header().Get("Content-Disposition"))
	}

	var arr []json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &arr); err != nil {
		t.Fatalf("response should be valid JSON array: %v", err)
	}
	if len(arr) != 2 {
		t.Fatalf("expected 2 workflows in array, got %d", len(arr))
	}
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

	if w.Header().Get("Content-Type") != "application/x-ndjson" {
		t.Fatalf("expected ndjson content type, got %s", w.Header().Get("Content-Type"))
	}
	lines := strings.Split(strings.TrimSpace(w.Body.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 NDJSON lines, got %d", len(lines))
	}
	for i, line := range lines {
		var obj map[string]any
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			t.Fatalf("line %d is not valid JSON: %v", i, err)
		}
	}
}

func TestHandleExportWorkflows_InvalidFormat_Returns400(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	r := httptest.NewRequest(http.MethodGet, "/v1/export/workflows?format=csv", nil)
	r = r.WithContext(context.WithValue(r.Context(), ctxProjectIDKey, "proj-1"))
	w := httptest.NewRecorder()

	TypedHandler(srv, http.StatusOK, srv.handleExportWorkflows)(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for csv format on workflows, got %d", w.Code)
	}
}

func TestHandleExportWorkflows_NoProjectID_Returns400(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	r := httptest.NewRequest(http.MethodGet, "/v1/export/workflows", nil)
	w := httptest.NewRecorder()

	TypedHandler(srv, http.StatusOK, srv.handleExportWorkflows)(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing project ID, got %d", w.Code)
	}
}

// ─── CSV edge cases ──────────────────────────────────────────────────────────.

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

	if w.Header().Get("Content-Type") != "text/csv" {
		t.Fatalf("expected text/csv, got %s", w.Header().Get("Content-Type"))
	}

	// Parse as CSV to verify the comma-containing error is properly escaped.
	csvReader := csv.NewReader(strings.NewReader(w.Body.String()))
	records, err := csvReader.ReadAll()
	if err != nil {
		t.Fatalf("CSV parsing failed (commas not properly escaped): %v", err)
	}
	if len(records) != 2 { // header + 1 data row
		t.Fatalf("expected 2 CSV records (header + data), got %d", len(records))
	}
	// The error column is the last field (index 8).
	errorField := records[1][8]
	if errorField != "connection failed, retries exhausted, giving up" {
		t.Fatalf("error field not properly preserved: %s", errorField)
	}
}

// ─── WebhookSecret sanitization ──────────────────────────────────────────────.

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
	if strings.Contains(body, "supersecretvalue") {
		t.Fatal("webhook_secret should be sanitized from export output")
	}
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
	if strings.Contains(body, "anothersecret") {
		t.Fatal("webhook_secret should be sanitized from NDJSON export output")
	}
}

// ─── JSON format validation ──────────────────────────────────────────────────.

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
	if err := json.Unmarshal(w.Body.Bytes(), &arr); err != nil {
		t.Fatalf("response is not a valid JSON array: %v", err)
	}
	if len(arr) != 3 {
		t.Fatalf("expected 3 items in JSON array, got %d", len(arr))
	}

	// Each item should be a valid JSON object with an id field.
	for i, raw := range arr {
		var obj map[string]any
		if err := json.Unmarshal(raw, &obj); err != nil {
			t.Fatalf("item %d is not valid JSON object: %v", i, err)
		}
		if _, ok := obj["id"]; !ok {
			t.Fatalf("item %d missing 'id' field", i)
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

	// Should fail with 400 (invalid RFC3339), not succeed.
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for SQL injection attempt, got %d", w.Code)
	}
}
