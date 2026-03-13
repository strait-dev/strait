package api

import (
	"net/http"
	"strings"
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
	if status != "" {
		switch status {
		case domain.WebhookStatusPending, domain.WebhookStatusDelivered, domain.WebhookStatusFailed, domain.WebhookStatusDead:
		default:
			respondError(w, r, http.StatusBadRequest, "status is invalid")
			return
		}
	}

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

func (s *Server) handleGetWebhookDelivery(w http.ResponseWriter, r *http.Request) {
	deliveryID := webhookDeliveryIDParam(r)
	if deliveryID == "" {
		respondError(w, r, http.StatusBadRequest, "delivery ID is required")
		return
	}

	delivery, err := s.store.GetWebhookDelivery(r.Context(), deliveryID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			respondError(w, r, http.StatusNotFound, "delivery not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get delivery")
		return
	}
	if delivery == nil {
		respondError(w, r, http.StatusNotFound, "delivery not found")
		return
	}

	respondJSON(w, http.StatusOK, delivery)
}

// handleRetryWebhookDelivery resets a failed delivery for retry.
func (s *Server) handleRetryWebhookDelivery(w http.ResponseWriter, r *http.Request) {
	deliveryID := webhookDeliveryIDParam(r)
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

	if d.Status != domain.WebhookStatusFailed && d.Status != domain.WebhookStatusDead {
		respondError(w, r, http.StatusConflict, "only failed or dead deliveries can be retried")
		return
	}

	retried, err := s.store.RetryWebhookDelivery(r.Context(), deliveryID)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to retry delivery")
		return
	}

	respondJSON(w, http.StatusOK, retried)
}

func webhookDeliveryIDParam(r *http.Request) string {
	id := chi.URLParam(r, "id")
	if id != "" {
		return id
	}
	return chi.URLParam(r, "deliveryID")
}
