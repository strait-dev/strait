package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/go-chi/chi/v5"
)

type approveWorkflowStepRequest struct {
	Approver string `json:"approver" validate:"required"`
}

type skipStepRequest struct {
	Reason string `json:"reason,omitempty"`
}

type forceCompleteStepRequest struct {
	Result json.RawMessage `json:"result,omitempty"`
}

func (s *Server) handleListWorkflowRuns(w http.ResponseWriter, r *http.Request) {
	workflowID := chi.URLParam(r, "workflowID")

	limit, cursor, err := parsePaginationParams(r)
	if err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	runs, err := s.store.ListWorkflowRuns(r.Context(), workflowID, limit+1, cursor)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list workflow runs")
		return
	}

	respondJSON(w, http.StatusOK, paginatedResult(runs, limit, func(run domain.WorkflowRun) string {
		return run.CreatedAt.Format(time.RFC3339Nano)
	}))
}

func (s *Server) handleListWorkflowRunsByProject(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	projectID := query.Get("project_id")
	if projectID == "" {
		respondError(w, r, http.StatusBadRequest, "project_id is required")
		return
	}

	var status *domain.WorkflowRunStatus
	if statusRaw := query.Get("status"); statusRaw != "" {
		parsed := domain.WorkflowRunStatus(statusRaw)
		if !parsed.IsValid() {
			respondError(w, r, http.StatusBadRequest, "status is invalid")
			return
		}
		status = &parsed
	}

	limit, cursor, err := parsePaginationParams(r)
	if err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	runs, err := s.store.ListWorkflowRunsByProject(r.Context(), projectID, status, limit+1, cursor)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list workflow runs")
		return
	}

	respondJSON(w, http.StatusOK, paginatedResult(runs, limit, func(run domain.WorkflowRun) string {
		return run.CreatedAt.Format(time.RFC3339Nano)
	}))
}

func (s *Server) handleGetWorkflowRun(w http.ResponseWriter, r *http.Request) {
	workflowRunID := chi.URLParam(r, "workflowRunID")
	run, err := s.store.GetWorkflowRun(r.Context(), workflowRunID)
	if err != nil {
		if errors.Is(err, store.ErrWorkflowRunNotFound) {
			respondError(w, r, http.StatusNotFound, "workflow run not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get workflow run")
		return
	}

	respondJSON(w, http.StatusOK, run)
}

func (s *Server) handleCancelWorkflowRun(w http.ResponseWriter, r *http.Request) {
	workflowRunID := chi.URLParam(r, "workflowRunID")

	if s.compensationEngine == nil {
		respondError(w, r, http.StatusNotImplemented, "compensation engine not configured")
		return
	}

	result, err := s.compensationEngine.CancelWorkflowRun(r.Context(), workflowRunID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			respondError(w, r, http.StatusNotFound, "workflow run not found")
			return
		}
		if strings.Contains(err.Error(), "terminal state") {
			respondError(w, r, http.StatusBadRequest, err.Error())
			return
		}
		respondError(w, r, http.StatusInternalServerError, fmt.Sprintf("cancel workflow run: %s", err))
		return
	}

	updatedRun, err := s.store.GetWorkflowRun(r.Context(), workflowRunID)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get updated workflow run")
		return
	}
	s.publishWorkflowRunHook(r.Context(), updatedRun, domain.WfStatusRunning, domain.WfStatusCanceled, "cancel")

	respondJSON(w, http.StatusOK, map[string]any{
		"workflow_run":        updatedRun,
		"compensation_result": result,
	})
}

func (s *Server) handleRetryCompensation(w http.ResponseWriter, r *http.Request) {
	workflowRunID := chi.URLParam(r, "workflowRunID")

	if s.compensationEngine == nil {
		respondError(w, r, http.StatusNotImplemented, "compensation engine not configured")
		return
	}

	result, err := s.compensationEngine.RetryFailedCompensation(r.Context(), workflowRunID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			respondError(w, r, http.StatusNotFound, "workflow run not found")
			return
		}
		if strings.Contains(err.Error(), "not retryable") {
			respondError(w, r, http.StatusBadRequest, err.Error())
			return
		}
		respondError(w, r, http.StatusInternalServerError, fmt.Sprintf("retry compensation: %s", err))
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"compensation_result": result,
	})
}

func (s *Server) handlePauseWorkflowRun(w http.ResponseWriter, r *http.Request) {
	workflowRunID := chi.URLParam(r, "workflowRunID")
	run, err := s.store.GetWorkflowRun(r.Context(), workflowRunID)
	if err != nil {
		if errors.Is(err, store.ErrWorkflowRunNotFound) {
			respondError(w, r, http.StatusNotFound, "workflow run not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get workflow run")
		return
	}
	if run.Status.IsTerminal() {
		respondError(w, r, http.StatusBadRequest, "workflow run already in terminal state")
		return
	}
	if run.Status == domain.WfStatusPaused {
		respondJSON(w, http.StatusOK, run)
		return
	}
	if run.Status != domain.WfStatusRunning {
		respondError(w, r, http.StatusBadRequest, "workflow run can only be paused from running state")
		return
	}

	if err := s.store.UpdateWorkflowRunStatus(r.Context(), run.ID, domain.WfStatusRunning, domain.WfStatusPaused, nil); err != nil {
		respondError(w, r, http.StatusConflict, "failed to pause workflow run")
		return
	}

	updatedRun, err := s.store.GetWorkflowRun(r.Context(), run.ID)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get updated workflow run")
		return
	}
	s.publishWorkflowRunHook(r.Context(), updatedRun, run.Status, domain.WfStatusPaused, "pause")
	respondJSON(w, http.StatusOK, updatedRun)
}

func (s *Server) handleResumeWorkflowRun(w http.ResponseWriter, r *http.Request) {
	if s.workflowCallback == nil {
		respondError(w, r, http.StatusServiceUnavailable, "workflow callback unavailable")
		return
	}

	workflowRunID := chi.URLParam(r, "workflowRunID")
	run, err := s.store.GetWorkflowRun(r.Context(), workflowRunID)
	if err != nil {
		if errors.Is(err, store.ErrWorkflowRunNotFound) {
			respondError(w, r, http.StatusNotFound, "workflow run not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get workflow run")
		return
	}
	if run.Status != domain.WfStatusPaused {
		respondError(w, r, http.StatusBadRequest, "workflow run is not paused")
		return
	}

	if err := s.workflowCallback.ResumeWorkflowRun(r.Context(), workflowRunID); err != nil {
		respondError(w, r, http.StatusConflict, err.Error())
		return
	}

	updatedRun, err := s.store.GetWorkflowRun(r.Context(), run.ID)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get updated workflow run")
		return
	}
	s.publishWorkflowRunHook(r.Context(), updatedRun, run.Status, domain.WfStatusRunning, "resume")
	respondJSON(w, http.StatusOK, updatedRun)
}

func (s *Server) handleGetWorkflowRunLabels(w http.ResponseWriter, r *http.Request) {
	workflowRunID := chi.URLParam(r, "workflowRunID")
	labels, err := s.store.ListWorkflowRunLabels(r.Context(), workflowRunID)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list workflow run labels")
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"labels": labels})
}

func (s *Server) handleListWorkflowStepRuns(w http.ResponseWriter, r *http.Request) {
	workflowRunID := chi.URLParam(r, "workflowRunID")

	limit, cursor, err := parsePaginationParams(r)
	if err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	stepRuns, err := s.store.ListStepRunsByWorkflowRun(r.Context(), workflowRunID, limit+1, cursor)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list workflow step runs")
		return
	}

	respondJSON(w, http.StatusOK, paginatedResult(stepRuns, limit, func(sr domain.WorkflowStepRun) string {
		return sr.CreatedAt.Format(time.RFC3339Nano)
	}))
}

func (s *Server) handleApproveWorkflowStep(w http.ResponseWriter, r *http.Request) {
	if s.workflowCallback == nil {
		respondError(w, r, http.StatusServiceUnavailable, "workflow callback unavailable")
		return
	}

	workflowRunID := chi.URLParam(r, "workflowRunID")
	stepRef := chi.URLParam(r, "stepRef")
	beforeRun, beforeErr := s.store.GetWorkflowRun(r.Context(), workflowRunID)
	if beforeErr != nil {
		slog.Warn("failed to get workflow run before approve step", "workflow_run_id", workflowRunID, "error", beforeErr)
	}

	var req approveWorkflowStepRequest
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if !s.validateRequest(w, r, &req) {
		return
	}

	if err := s.workflowCallback.ApproveStep(r.Context(), workflowRunID, stepRef, req.Approver); err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	stepRun, err := s.store.GetStepRunByWorkflowRunAndRef(r.Context(), workflowRunID, stepRef)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to fetch workflow step run")
		return
	}
	approval, err := s.store.GetWorkflowStepApprovalByStepRunID(r.Context(), stepRun.ID)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to fetch workflow step approval")
		return
	}

	afterRun, afterErr := s.store.GetWorkflowRun(r.Context(), workflowRunID)
	if afterErr != nil {
		slog.Warn("failed to get workflow run after approve step", "workflow_run_id", workflowRunID, "error", afterErr)
	}
	if beforeErr == nil && afterErr == nil && beforeRun != nil && afterRun != nil && beforeRun.Status != afterRun.Status {
		s.publishWorkflowRunHook(r.Context(), afterRun, beforeRun.Status, afterRun.Status, "approve_step")
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"step_run": stepRun,
		"approval": approval,
	})
}

func (s *Server) handleSkipWorkflowStep(w http.ResponseWriter, r *http.Request) {
	if s.workflowCallback == nil {
		respondError(w, r, http.StatusServiceUnavailable, "workflow callback unavailable")
		return
	}

	workflowRunID := chi.URLParam(r, "workflowRunID")
	stepRef := chi.URLParam(r, "stepRef")
	beforeRun, beforeErr := s.store.GetWorkflowRun(r.Context(), workflowRunID)
	if beforeErr != nil {
		slog.Warn("failed to get workflow run before skip step", "workflow_run_id", workflowRunID, "error", beforeErr)
	}

	var req skipStepRequest
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := s.workflowCallback.SkipStep(r.Context(), workflowRunID, stepRef, req.Reason); err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	stepRun, err := s.store.GetStepRunByWorkflowRunAndRef(r.Context(), workflowRunID, stepRef)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to fetch workflow step run")
		return
	}

	afterRun, afterErr := s.store.GetWorkflowRun(r.Context(), workflowRunID)
	if afterErr != nil {
		slog.Warn("failed to get workflow run after skip step", "workflow_run_id", workflowRunID, "error", afterErr)
	}
	if beforeErr == nil && afterErr == nil && beforeRun != nil && afterRun != nil && beforeRun.Status != afterRun.Status {
		s.publishWorkflowRunHook(r.Context(), afterRun, beforeRun.Status, afterRun.Status, "skip_step")
	}

	respondJSON(w, http.StatusOK, map[string]any{"step_run": stepRun})
}

func (s *Server) handleForceCompleteWorkflowStep(w http.ResponseWriter, r *http.Request) {
	if s.workflowCallback == nil {
		respondError(w, r, http.StatusServiceUnavailable, "workflow callback unavailable")
		return
	}

	workflowRunID := chi.URLParam(r, "workflowRunID")
	stepRef := chi.URLParam(r, "stepRef")
	beforeRun, beforeErr := s.store.GetWorkflowRun(r.Context(), workflowRunID)
	if beforeErr != nil {
		slog.Warn("failed to get workflow run before force-complete step", "workflow_run_id", workflowRunID, "error", beforeErr)
	}

	var req forceCompleteStepRequest
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := s.workflowCallback.ForceCompleteStep(r.Context(), workflowRunID, stepRef, req.Result); err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	stepRun, err := s.store.GetStepRunByWorkflowRunAndRef(r.Context(), workflowRunID, stepRef)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to fetch workflow step run")
		return
	}

	afterRun, afterErr := s.store.GetWorkflowRun(r.Context(), workflowRunID)
	if afterErr != nil {
		slog.Warn("failed to get workflow run after force-complete step", "workflow_run_id", workflowRunID, "error", afterErr)
	}
	if beforeErr == nil && afterErr == nil && beforeRun != nil && afterRun != nil && beforeRun.Status != afterRun.Status {
		s.publishWorkflowRunHook(r.Context(), afterRun, beforeRun.Status, afterRun.Status, "force_complete_step")
	}

	respondJSON(w, http.StatusOK, map[string]any{"step_run": stepRun})
}

func (s *Server) handleRetryWorkflowRun(w http.ResponseWriter, r *http.Request) {
	if s.workflowEngine == nil {
		respondError(w, r, http.StatusServiceUnavailable, "workflow engine unavailable")
		return
	}

	workflowRunID := chi.URLParam(r, "workflowRunID")
	run, err := s.store.GetWorkflowRun(r.Context(), workflowRunID)
	if err != nil {
		if errors.Is(err, store.ErrWorkflowRunNotFound) {
			respondError(w, r, http.StatusNotFound, "workflow run not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get workflow run")
		return
	}

	if !run.Status.IsTerminal() {
		respondError(w, r, http.StatusBadRequest, "can only retry a workflow run in terminal state")
		return
	}

	newRun, err := s.workflowEngine.RetryWorkflowRun(r.Context(), workflowRunID)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, fmt.Sprintf("failed to retry workflow run: %v", err))
		return
	}

	s.publishWorkflowRunHook(r.Context(), newRun, domain.WfStatusPending, newRun.Status, "retry")

	respondJSON(w, http.StatusCreated, newRun)
}
