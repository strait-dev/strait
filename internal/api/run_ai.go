package api

import (
	"net/http"
	"time"

	"strait/internal/domain"

	"github.com/go-chi/chi/v5"
)

func (s *Server) handleListRunCheckpoints(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")

	limit, cursor, err := parsePaginationParams(r)
	if err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	checkpoints, err := s.store.ListRunCheckpoints(r.Context(), runID, limit+1, cursor)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list run checkpoints")
		return
	}

	respondJSON(w, http.StatusOK, paginatedResult(checkpoints, limit, func(cp domain.RunCheckpoint) string {
		return cp.CreatedAt.Format(time.RFC3339Nano)
	}))
}

func (s *Server) handleListRunUsage(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")

	limit, cursor, err := parsePaginationParams(r)
	if err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	usage, err := s.store.ListRunUsage(r.Context(), runID, limit+1, cursor)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list run usage")
		return
	}

	respondJSON(w, http.StatusOK, paginatedResult(usage, limit, func(u domain.RunUsage) string {
		return u.CreatedAt.Format(time.RFC3339Nano)
	}))
}

func (s *Server) handleListRunToolCalls(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")

	limit, cursor, err := parsePaginationParams(r)
	if err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	calls, err := s.store.ListRunToolCalls(r.Context(), runID, limit+1, cursor)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list run tool calls")
		return
	}

	respondJSON(w, http.StatusOK, paginatedResult(calls, limit, func(c domain.RunToolCall) string {
		return c.CreatedAt.Format(time.RFC3339Nano)
	}))
}

func (s *Server) handleListRunOutputs(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")

	limit, cursor, err := parsePaginationParams(r)
	if err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	outputs, err := s.store.ListRunOutputs(r.Context(), runID, limit+1, cursor)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list run outputs")
		return
	}

	respondJSON(w, http.StatusOK, paginatedResult(outputs, limit, func(o domain.RunOutput) string {
		return o.CreatedAt.Format(time.RFC3339Nano)
	}))
}
