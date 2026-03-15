package api

import (
	"encoding/json"
	"net/http"

	"strait/internal/domain"

	"github.com/go-chi/chi/v5"
)

func (s *Server) handleSDKSetState(w http.ResponseWriter, r *http.Request) {
	applySDKResponseHeaders(r.Context(), w)
	runID := chi.URLParam(r, "runID")

	var req struct {
		Key   string          `json:"key" validate:"required"`
		Value json.RawMessage `json:"value" validate:"required"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if !s.validateRequest(w, r, &req) {
		return
	}
	if len(req.Key) > 256 {
		respondError(w, r, http.StatusBadRequest, "state key must be 256 characters or fewer")
		return
	}

	state := &domain.RunState{
		RunID:    runID,
		StateKey: req.Key,
		Value:    req.Value,
	}
	if err := s.store.UpsertRunState(r.Context(), state); err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to upsert run state")
		return
	}

	respondJSON(w, http.StatusCreated, state)
}

func (s *Server) handleSDKGetState(w http.ResponseWriter, r *http.Request) {
	applySDKResponseHeaders(r.Context(), w)
	runID := chi.URLParam(r, "runID")
	key := chi.URLParam(r, "key")

	state, err := s.store.GetRunState(r.Context(), runID, key)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get run state")
		return
	}
	if state == nil {
		respondError(w, r, http.StatusNotFound, "state key not found")
		return
	}

	respondJSON(w, http.StatusOK, state)
}

func (s *Server) handleSDKListState(w http.ResponseWriter, r *http.Request) {
	applySDKResponseHeaders(r.Context(), w)
	runID := chi.URLParam(r, "runID")

	items, err := s.store.ListRunState(r.Context(), runID)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list run state")
		return
	}

	respondJSON(w, http.StatusOK, items)
}

func (s *Server) handleSDKDeleteState(w http.ResponseWriter, r *http.Request) {
	applySDKResponseHeaders(r.Context(), w)
	runID := chi.URLParam(r, "runID")
	key := chi.URLParam(r, "key")

	if err := s.store.DeleteRunState(r.Context(), runID, key); err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to delete run state")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleListRunState exposes run state for the management API.
func (s *Server) handleListRunState(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")

	items, err := s.store.ListRunState(r.Context(), runID)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list run state")
		return
	}

	respondJSON(w, http.StatusOK, items)
}
