package api

import (
	"net/http"
	"time"

	"strait/internal/domain"

	"github.com/go-chi/chi/v5"
)

func (s *Server) handleListBatchOperations(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		respondError(w, r, http.StatusBadRequest, "project_id is required")
		return
	}

	limit, cursor, err := parsePaginationParams(r)
	if err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	ops, err := s.store.ListBatchOperations(r.Context(), projectID, limit+1, cursor)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list batch operations")
		return
	}

	respondJSON(w, http.StatusOK, paginatedResult(ops, limit, func(op domain.BatchOperation) string {
		return op.CreatedAt.Format(time.RFC3339Nano)
	}))
}

func (s *Server) handleGetBatchOperation(w http.ResponseWriter, r *http.Request) {
	batchID := chi.URLParam(r, "batchID")
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		respondError(w, r, http.StatusBadRequest, "project_id is required")
		return
	}

	op, err := s.store.GetBatchOperation(r.Context(), batchID, projectID)
	if err != nil {
		respondError(w, r, http.StatusNotFound, "batch operation not found")
		return
	}

	respondJSON(w, http.StatusOK, op)
}
