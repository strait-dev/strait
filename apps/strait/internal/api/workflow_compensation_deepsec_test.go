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
			if id != wfRun.ID {
				t.Fatalf("GetWorkflowRun id = %q, want %q", id, wfRun.ID)
			}
			return wfRun, nil
		},
		ListStepsByWorkflowVersionFunc: func(_ context.Context, workflowID string, version int) ([]domain.WorkflowStep, error) {
			if workflowID != wfRun.WorkflowID || version != wfRun.WorkflowVersion {
				t.Fatalf("ListStepsByWorkflowVersion(%q, %d), want (%q, %d)", workflowID, version, wfRun.WorkflowID, wfRun.WorkflowVersion)
			}
			return steps, nil
		},
		ListStepRunsByWorkflowRunFunc: func(_ context.Context, workflowRunID string, limit int, cursor *time.Time) ([]domain.WorkflowStepRun, error) {
			if workflowRunID != wfRun.ID {
				t.Fatalf("ListStepRunsByWorkflowRun id = %q, want %q", workflowRunID, wfRun.ID)
			}
			return stepRuns, nil
		},
		UpdateWorkflowRunStatusFunc: func(_ context.Context, id string, from, to domain.WorkflowRunStatus, fields map[string]any) error {
			if id != wfRun.ID || from != domain.WfStatusFailed || to != domain.WfStatusCompensating {
				t.Fatalf("UpdateWorkflowRunStatus(%q, %q, %q), want failed -> compensating", id, from, to)
			}
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

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	if len(enqueued) != 1 {
		t.Fatalf("enqueued compensation jobs = %d, want 1", len(enqueued))
	}
	got := enqueued[0]
	if got.JobID != "job-refund" || got.ProjectID != "proj-compensate" {
		t.Fatalf("enqueued job = (%q, %q), want compensation job in project", got.JobID, got.ProjectID)
	}
	if got.TriggeredBy != domain.TriggerWorkflow || got.CreatedBy != "system:workflow-compensation" {
		t.Fatalf("enqueued provenance = (%q, %q), want workflow compensation", got.TriggeredBy, got.CreatedBy)
	}
	if got.Metadata[domain.RunMetadataCompensationRunID] == "" ||
		got.Metadata[domain.RunMetadataCompensationWorkflowRunID] != wfRun.ID ||
		got.Metadata[domain.RunMetadataCompensationStepRef] != "charge-card" {
		t.Fatalf("missing compensation metadata: %#v", got.Metadata)
	}
	if got.Tags["env"] != "prod" {
		t.Fatalf("tags = %#v, want workflow tags copied", got.Tags)
	}
	if got.TimeoutSecsOverride != 45 {
		t.Fatalf("timeout override = %d, want 45", got.TimeoutSecsOverride)
	}
	var payload map[string]any
	if err := json.Unmarshal(got.Payload, &payload); err != nil {
		t.Fatalf("invalid compensation payload: %v", err)
	}
	if payload["workflow_run_id"] != wfRun.ID || payload["step_ref"] != "charge-card" {
		t.Fatalf("payload missing compensation context: %#v", payload)
	}
}
