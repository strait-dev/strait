package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"orchestrator/internal/domain"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")
	run, err := s.store.GetRun(r.Context(), runID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusNotFound, "run not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to get run")
		return
	}

	respondJSON(w, http.StatusOK, run)
}

func (s *Server) handleListRuns(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	projectID := query.Get("project_id")
	if projectID == "" {
		respondError(w, http.StatusBadRequest, "project_id is required")
		return
	}

	var status *domain.RunStatus
	if statusRaw := query.Get("status"); statusRaw != "" {
		parsed := domain.RunStatus(statusRaw)
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

	var cursor *time.Time
	if cursorRaw := query.Get("cursor"); cursorRaw != "" {
		parsedCursor, err := time.Parse(time.RFC3339, cursorRaw)
		if err != nil {
			respondError(w, http.StatusBadRequest, "cursor must be an RFC3339 timestamp")
			return
		}
		cursor = &parsedCursor
	}

	runs, err := s.store.ListRunsByProject(r.Context(), projectID, status, limit, cursor)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list runs")
		return
	}

	respondJSON(w, http.StatusOK, runs)
}

func (s *Server) handleCancelRun(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")
	run, err := s.store.GetRun(r.Context(), runID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusNotFound, "run not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to get run")
		return
	}

	if run.Status.IsTerminal() {
		respondError(w, http.StatusBadRequest, "run already in terminal state")
		return
	}

	if err := s.store.UpdateRunStatus(r.Context(), run.ID, run.Status, domain.StatusCanceled, nil); err != nil {
		respondError(w, http.StatusConflict, "failed to cancel run")
		return
	}

	// Propagate cancellation to child runs
	children, err := s.store.ListChildRuns(r.Context(), run.ID)
	if err == nil {
		for _, child := range children {
			if !child.Status.IsTerminal() {
				_ = s.store.UpdateRunStatus(r.Context(), child.ID, child.Status, domain.StatusCanceled, map[string]any{
					"finished_at": time.Now(),
					"error":       "parent run canceled",
				})
			}
		}
	}

	updatedRun, err := s.store.GetRun(r.Context(), run.ID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to get updated run")
		return
	}

	respondJSON(w, http.StatusOK, updatedRun)
}

func (s *Server) handleListChildRuns(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")
	children, err := s.store.ListChildRuns(r.Context(), runID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list children")
		return
	}
	respondJSON(w, http.StatusOK, children)
}
