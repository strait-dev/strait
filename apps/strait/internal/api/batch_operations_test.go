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

	"github.com/stretchr/testify/require"
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
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	var got domain.BatchOperation
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(),
		&got))
	require.Equal(t, "batch-1", got.
		ID)

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
	require.Equal(t, http.StatusNotFound,

		w.Code)

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
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	var got []domain.BatchOperation
	decodePaginatedList(t, w.Body.Bytes(), &got)
	require.Len(t,
		got, 2)

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
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(),
		&resp))

	canceled, ok := resp["canceled"].(float64)
	require.False(t, !ok || canceled !=
		2,
	)

}

func TestHandleBulkCancelAll_NoFilters(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	req := authedProjectRequest(http.MethodPost, "/v1/runs/bulk-cancel-all", `{}`, "proj-1")
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest,

		w.Code)

	if body := w.Body.String(); !strings.Contains(body, "at least one filter") {
		require.Failf(t, "test failure",

			"expected 'at least one filter' in body, got: %s", body)
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
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(),
		&resp))

	canceled, ok := resp["canceled"].(float64)
	require.False(t, !ok || canceled !=
		2,
	)

}

func TestHandleBulkCancelWorkflowRuns_EmptyIDs(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	req := authedProjectRequest(http.MethodPost, "/v1/workflow-runs/bulk-cancel", `{"workflow_run_ids":[]}`, "proj-1")
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnprocessableEntity,

		w.Code)

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
	require.Equal(t, http.StatusOK,
		w.Code,
	)
	require.NotNil(t, capturedPayload)

	var parsed map[string]string
	require.NoError(t, json.Unmarshal(capturedPayload,

		&parsed))
	require.Equal(t, "val", parsed["key"])

}
