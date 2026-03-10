package api

import (
	"net/http"
	"time"

	"strait/internal/domain"

	"github.com/go-chi/chi/v5"
)

func (s *Server) handleListWebhookDeliveries(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		respondError(w, r, http.StatusBadRequest, "project_id is required")
		return
	}

	status := r.URL.Query().Get("status")

	limit, cursor, err := parsePaginationParams(r)
	if err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	deliveries, err := s.store.ListWebhookDeliveries(r.Context(), projectID, status, limit+1, cursor)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list webhook deliveries")
		return
	}

	respondJSON(w, http.StatusOK, paginatedResult(deliveries, limit, func(d domain.WebhookDelivery) string {
		return d.CreatedAt.Format(time.RFC3339Nano)
	}))
}

// handleRetryWebhookDelivery resets a failed delivery for retry.
func (s *Server) handleRetryWebhookDelivery(w http.ResponseWriter, r *http.Request) {
	deliveryID := chi.URLParam(r, "deliveryID")
	if deliveryID == "" {
		respondError(w, r, http.StatusBadRequest, "delivery ID is required")
		return
	}

	d, err := s.store.GetWebhookDelivery(r.Context(), deliveryID)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get delivery")
		return
	}
	if d == nil {
		respondError(w, r, http.StatusNotFound, "delivery not found")
		return
	}

	if d.Status != "failed" {
		respondError(w, r, http.StatusConflict, "only failed deliveries can be retried")
		return
	}

	// Reset for retry: set back to pending with next retry now.
	now := time.Now()
	d.Status = "pending"
	d.Attempts = 0
	d.NextRetryAt = &now
	d.LastError = ""

	if err := s.store.UpdateWebhookDelivery(r.Context(), d); err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to update delivery")
		return
	}

	respondJSON(w, http.StatusOK, d)
}
