package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"orchestrator/internal/domain"
	"orchestrator/internal/store"

	"github.com/go-chi/chi/v5"
)

func (s *Server) handleListWorkflowRuns(w http.ResponseWriter, r *http.Request) {
	workflowID := chi.URLParam(r, "workflowID")
	query := r.URL.Query()

	limit := 50
	if limitRaw := query.Get("limit"); limitRaw != "" {
		parsedLimit, err := strconv.Atoi(limitRaw)
		if err != nil || parsedLimit <= 0 {
			respondError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		if parsedLimit > 100 {
			parsedLimit = 100
		}
		limit = parsedLimit
	}

	offset := 0
	if offsetRaw := query.Get("offset"); offsetRaw != "" {
		parsedOffset, err := strconv.Atoi(offsetRaw)
		if err != nil || parsedOffset < 0 {
			respondError(w, http.StatusBadRequest, "offset must be a non-negative integer")
			return
		}
		offset = parsedOffset
	}

	runs, err := s.store.ListWorkflowRuns(r.Context(), workflowID, limit, offset)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list workflow runs")
		return
	}

	respondJSON(w, http.StatusOK, runs)
}

func (s *Server) handleListWorkflowRunsByProject(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	projectID := query.Get("project_id")
	if projectID == "" {
		respondError(w, http.StatusBadRequest, "project_id is required")
		return
	}

	var status *domain.WorkflowRunStatus
	if statusRaw := query.Get("status"); statusRaw != "" {
		parsed := domain.WorkflowRunStatus(statusRaw)
		status = &parsed
	}

	limit := 50
	if limitRaw := query.Get("limit"); limitRaw != "" {
		parsedLimit, err := strconv.Atoi(limitRaw)
		if err != nil || parsedLimit <= 0 {
			respondError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		if parsedLimit > 100 {
			parsedLimit = 100
		}
		limit = parsedLimit
	}

	runs, err := s.store.ListWorkflowRunsByProject(r.Context(), projectID, status, limit)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list workflow runs")
		return
	}

	respondJSON(w, http.StatusOK, runs)
}

func (s *Server) handleGetWorkflowRun(w http.ResponseWriter, r *http.Request) {
	workflowRunID := chi.URLParam(r, "workflowRunID")
	run, err := s.store.GetWorkflowRun(r.Context(), workflowRunID)
	if err != nil {
		if errors.Is(err, store.ErrWorkflowRunNotFound) {
			respondError(w, http.StatusNotFound, "workflow run not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to get workflow run")
		return
	}

	respondJSON(w, http.StatusOK, run)
}

func (s *Server) handleCancelWorkflowRun(w http.ResponseWriter, r *http.Request) {
	workflowRunID := chi.URLParam(r, "workflowRunID")
	run, err := s.store.GetWorkflowRun(r.Context(), workflowRunID)
	if err != nil {
		if errors.Is(err, store.ErrWorkflowRunNotFound) {
			respondError(w, http.StatusNotFound, "workflow run not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to get workflow run")
		return
	}

	if run.Status.IsTerminal() {
		respondError(w, http.StatusBadRequest, "workflow run already in terminal state")
		return
	}

	if err := s.store.UpdateWorkflowRunStatus(r.Context(), run.ID, run.Status, domain.WfStatusCanceled, map[string]any{
		"finished_at": time.Now(),
		"error":       "canceled by user",
	}); err != nil {
		respondError(w, http.StatusConflict, "failed to cancel workflow run")
		return
	}

	stepRuns, err := s.store.ListStepRunsByWorkflowRun(r.Context(), run.ID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list workflow step runs")
		return
	}

	now := time.Now()
	for _, stepRun := range stepRuns {
		if !stepRun.Status.IsTerminal() {
			if err := s.store.UpdateStepRunStatus(r.Context(), stepRun.ID, domain.StepCanceled, map[string]any{
				"finished_at": now,
				"error":       "workflow canceled by user",
			}); err != nil {
				respondError(w, http.StatusConflict, "failed to cancel workflow step run")
				return
			}
		}

		if stepRun.JobRunID == "" {
			continue
		}

		jobRun, err := s.store.GetRun(r.Context(), stepRun.JobRunID)
		if err != nil {
			if errors.Is(err, store.ErrRunNotFound) {
				continue
			}
			respondError(w, http.StatusInternalServerError, "failed to get step job run")
			return
		}
		if jobRun.Status.IsTerminal() {
			continue
		}

		if err := s.store.UpdateRunStatus(r.Context(), jobRun.ID, jobRun.Status, domain.StatusCanceled, map[string]any{
			"finished_at": now,
			"error":       "workflow canceled by user",
		}); err != nil {
			respondError(w, http.StatusConflict, "failed to cancel step job run")
			return
		}
	}

	updatedRun, err := s.store.GetWorkflowRun(r.Context(), run.ID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to get updated workflow run")
		return
	}

	respondJSON(w, http.StatusOK, updatedRun)
}

func (s *Server) handleListWorkflowStepRuns(w http.ResponseWriter, r *http.Request) {
	workflowRunID := chi.URLParam(r, "workflowRunID")
	stepRuns, err := s.store.ListStepRunsByWorkflowRun(r.Context(), workflowRunID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list workflow step runs")
		return
	}

	respondJSON(w, http.StatusOK, stepRuns)
}
