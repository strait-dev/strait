package api

import (
	"net/http"
	"time"

	"orchestrator/internal/domain"

	"github.com/go-chi/chi/v5"
)

func (s *Server) handleListRunEvents(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")
	level := r.URL.Query().Get("level")
	eventType := r.URL.Query().Get("type")

	limit, cursor, err := parsePaginationParams(r)
	if err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	events, err := s.store.ListEventsByRunFiltered(r.Context(), runID, level, eventType, limit+1, cursor)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list events")
		return
	}

	respondJSON(w, http.StatusOK, paginatedResult(events, limit, func(e domain.RunEvent) string {
		return e.CreatedAt.Format(time.RFC3339Nano)
	}))
}
