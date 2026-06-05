package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

func TestCompensateWorkflowRun_EnqueuesCompensationJobs(t *testing.T) {
	t.Parallel()

	now := time.Now()
	wfRun := &domain.WorkflowRun{
		ID:              "wfr-compensate",
		WorkflowID:      "wf-compensate",
		ProjectID:       "proj-compensate",
		Status:          domain.WfStatusFailed,
		WorkflowVersion: 1,
		Tags:            map[string]string{"env": "prod"},
	}
	steps := []domain.WorkflowStep{{
		ID:                      "step-charge",
		WorkflowID:              wfRun.WorkflowID,
		StepRef:                 "charge-card",
		CompensationJobID:       "job-refund",
		CompensationTimeoutSecs: 45,
	}}
	stepRuns := []domain.WorkflowStepRun{{
		ID:            "wsr-charge",
		WorkflowRunID: wfRun.ID,
		StepRef:       "charge-card",
		Status:        domain.StepCompleted,
		Output:        json.RawMessage(`{"charge_id":"ch_123"}`),
		FinishedAt:    &now,
	}}

	store := &APIStoreMock{
		GetWorkflowRunFunc: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
			require.Equal(t, wfRun.ID, id)

			return wfRun, nil
		},
		ListStepsByWorkflowVersionFunc: func(_ context.Context, workflowID string, version int) ([]domain.WorkflowStep, error) {
			require.False(t, workflowID !=
				wfRun.WorkflowID ||
				version != wfRun.
					WorkflowVersion,
			)

			return steps, nil
		},
		ListStepRunsByWorkflowRunFunc: func(_ context.Context, workflowRunID string, limit int, cursor *time.Time) ([]domain.WorkflowStepRun, error) {
			require.Equal(t, wfRun.ID, workflowRunID)

			return stepRuns, nil
		},
		UpdateWorkflowRunStatusFunc: func(_ context.Context, id string, from, to domain.WorkflowRunStatus, fields map[string]any) error {
			require.False(t, id != wfRun.ID ||
				from !=
					domain.WfStatusFailed ||
				to !=
					domain.WfStatusCompensating,
			)

			return nil
		},
		CreateAuditEventFunc: func(context.Context, *domain.AuditEvent) error { return nil },
		GetProjectQuotaFunc: func(context.Context, string) (*store.ProjectQuota, error) {
			return nil, nil
		},
	}

	var enqueued []*domain.JobRun
	q := &mockQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error {
		enqueued = append(enqueued, run)
		return nil
	}}
	srv := newTestServer(t, store, q, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/workflow-runs/wfr-compensate/compensate", "", "proj-compensate"))
	require.Equal(t, http.StatusOK,
		w.Code)
	require.Len(t,
		enqueued, 1)

	got := enqueued[0]
	require.False(t, got.JobID !=
		"job-refund" ||
		got.ProjectID !=
			"proj-compensate",
	)
	require.False(t, got.TriggeredBy !=
		domain.
			TriggerWorkflow || got.
		CreatedBy !=
		"system:workflow-compensation",
	)
	require.False(t, got.Metadata[domain.RunMetadataCompensationRunID] == "" ||
		got.Metadata[domain.RunMetadataCompensationWorkflowRunID] !=

			wfRun.ID || got.Metadata[domain.RunMetadataCompensationStepRef] != "charge-card",
	)
	require.Equal(t, "prod", got.Tags["env"])
	require.Equal(t, 45, got.TimeoutSecsOverride)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(got.Payload,
		&payload))
	require.False(t, payload["workflow_run_id"] != wfRun.ID || payload["step_ref"] !=
		"charge-card",
	)
}
