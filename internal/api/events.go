package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (s *Server) handleListRunEvents(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")
	level := r.URL.Query().Get("level")
	eventType := r.URL.Query().Get("type")

	events, err := s.store.ListEventsByRunFiltered(r.Context(), runID, level, eventType)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list events")
		return
	}

	respondJSON(w, http.StatusOK, events)
}
