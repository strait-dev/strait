package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (s *Server) handleListRunCheckpoints(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")
	checkpoints, err := s.store.ListRunCheckpoints(r.Context(), runID, 100)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list run checkpoints")
		return
	}
	respondJSON(w, http.StatusOK, checkpoints)
}

func (s *Server) handleListRunUsage(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")
	usage, err := s.store.ListRunUsage(r.Context(), runID, 100)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list run usage")
		return
	}
	respondJSON(w, http.StatusOK, usage)
}

func (s *Server) handleListRunToolCalls(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")
	calls, err := s.store.ListRunToolCalls(r.Context(), runID, 100)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list run tool calls")
		return
	}
	respondJSON(w, http.StatusOK, calls)
}

func (s *Server) handleListRunOutputs(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")
	outputs, err := s.store.ListRunOutputs(r.Context(), runID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list run outputs")
		return
	}
	respondJSON(w, http.StatusOK, outputs)
}
