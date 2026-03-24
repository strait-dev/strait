package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

// handleGetBatchOperation tests.

func TestHandleGetBatchOperation_Success(t *testing.T) {
	t.Parallel()

	dom := &domain.BatchOperation{
		ID:           "batch-1",
		ProjectID:    "proj-1",
		JobID:        "job-1",
		ItemCount:    3,
		CreatedCount: 3,
		CreatedAt:    time.Now(),
	}

	srv := newTestServer(t, &APIStoreMock{
		GetBatchOperationFunc: func(ctx context.Context, batchID, projectID string) (*domain.BatchOperation, error) {
			if batchID != "batch-1" || projectID != "proj-1" {
				return nil, fmt.Errorf("unexpected args: %s, %s", batchID, projectID)
			}
			return dom, nil
		},
	}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	req := authedProjectRequest(http.MethodGet, "/v1/batch-operations/batch-1", "", "proj-1")
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var got domain.BatchOperation
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.ID != "batch-1" {
		t.Fatalf("expected ID batch-1, got %s", got.ID)
	}
}

func TestHandleGetBatchOperation_NotFound(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{
		GetBatchOperationFunc: func(ctx context.Context, batchID, projectID string) (*domain.BatchOperation, error) {
			return nil, fmt.Errorf("not found")
		},
	}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	req := authedProjectRequest(http.MethodGet, "/v1/batch-operations/batch-999", "", "proj-1")
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// handleListBatchOperations tests.

func TestHandleListBatchOperations_Success(t *testing.T) {
	t.Parallel()

	now := time.Now()
	ops := []domain.BatchOperation{
		{ID: "batch-1", ProjectID: "proj-1", JobID: "job-1", ItemCount: 3, CreatedCount: 3, CreatedAt: now},
		{ID: "batch-2", ProjectID: "proj-1", JobID: "job-2", ItemCount: 5, CreatedCount: 2, CreatedAt: now.Add(-time.Minute)},
	}

	srv := newTestServer(t, &APIStoreMock{
		ListBatchOperationsFunc: func(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.BatchOperation, error) {
			if projectID != "proj-1" {
				return nil, fmt.Errorf("unexpected project_id: %s", projectID)
			}
			return ops, nil
		},
	}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	req := authedProjectRequest(http.MethodGet, "/v1/batch-operations", "", "proj-1")
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var got []domain.BatchOperation
	decodePaginatedList(t, w.Body.Bytes(), &got)
	if len(got) != 2 {
		t.Fatalf("expected 2 batch ops, got %d", len(got))
	}
}

// handleBulkCancelAll tests.

func TestHandleBulkCancelAll_Success(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{
		BulkCancelByFilterFunc: func(ctx context.Context, projectID string, f store.BulkCancelFilter, now time.Time, reason string) ([]string, error) {
			if projectID != "proj-1" {
				return nil, fmt.Errorf("unexpected project_id: %s", projectID)
			}
			return []string{"run-1", "run-2"}, nil
		},
	}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	req := authedProjectRequest(http.MethodPost, "/v1/runs/bulk-cancel-all", `{"job_id":"job-1"}`, "proj-1")
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	canceled, ok := resp["canceled"].(float64)
	if !ok || canceled != 2 {
		t.Fatalf("expected canceled=2, got %v", resp["canceled"])
	}
}

func TestHandleBulkCancelAll_NoFilters(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	req := authedProjectRequest(http.MethodPost, "/v1/runs/bulk-cancel-all", `{}`, "proj-1")
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if body := w.Body.String(); !strings.Contains(body, "at least one filter") {
		t.Fatalf("expected 'at least one filter' in body, got: %s", body)
	}
}

// handleBulkCancelWorkflowRuns tests.

func TestHandleBulkCancelWorkflowRuns_Success(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{
		BulkCancelWorkflowRunsFunc: func(ctx context.Context, projectID string, ids []string, now time.Time) ([]string, error) {
			return ids, nil // all canceled
		},
		CancelNonTerminalStepRunsFunc: func(ctx context.Context, workflowRunID string, finishedAt time.Time, reason string) (int64, error) {
			return 1, nil
		},
		CancelJobRunsByWorkflowRunFunc: func(ctx context.Context, workflowRunID string, finishedAt time.Time, reason string) (int64, error) {
			return 1, nil
		},
	}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	req := authedProjectRequest(http.MethodPost, "/v1/workflow-runs/bulk-cancel", `{"workflow_run_ids":["wr-1","wr-2"]}`, "proj-1")
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	canceled, ok := resp["canceled"].(float64)
	if !ok || canceled != 2 {
		t.Fatalf("expected canceled=2, got %v", resp["canceled"])
	}
}

func TestHandleBulkCancelWorkflowRuns_EmptyIDs(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	req := authedProjectRequest(http.MethodPost, "/v1/workflow-runs/bulk-cancel", `{"workflow_run_ids":[]}`, "proj-1")
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// handleListRuns payload_contains filter tests.

func TestHandleListRuns_PayloadContainsFilter(t *testing.T) {
	t.Parallel()

	var capturedPayload json.RawMessage

	srv := newTestServer(t, &APIStoreMock{
		ListRunsByProjectFunc: func(ctx context.Context, projectID string, status *domain.RunStatus, metadataKey, metadataValue, triggeredBy, batchID *string, payloadContains json.RawMessage, _ *domain.ExecutionMode, _ *string, limit int, cursor *time.Time) ([]domain.JobRun, error) {
			capturedPayload = payloadContains
			return []domain.JobRun{}, nil
		},
	}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	req := authedProjectRequest(http.MethodGet, `/v1/runs?payload_contains={"key":"val"}`, "", "proj-1")
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if capturedPayload == nil {
		t.Fatal("expected payloadContains to be set, got nil")
	}

	var parsed map[string]string
	if err := json.Unmarshal(capturedPayload, &parsed); err != nil {
		t.Fatalf("unmarshal payloadContains: %v", err)
	}
	if parsed["key"] != "val" {
		t.Fatalf("expected key=val, got %v", parsed)
	}
}
