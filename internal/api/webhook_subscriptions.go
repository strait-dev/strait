package api

import (
	"errors"
	"net/http"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/go-chi/chi/v5"
)

type CreateWebhookSubscriptionRequest struct {
	ProjectID  string   `json:"project_id" validate:"required"`
	WebhookURL string   `json:"webhook_url" validate:"required"`
	EventTypes []string `json:"event_types" validate:"required,min=1"`
	Secret     string   `json:"secret" validate:"required"`
	Active     *bool    `json:"active,omitempty"`
}

func (s *Server) handleCreateWebhookSubscription(w http.ResponseWriter, r *http.Request) {
	if !s.config.FFWebhookSubscriptions {
		respondError(w, r, http.StatusNotFound, "webhook subscriptions feature is not enabled")
		return
	}

	var req CreateWebhookSubscriptionRequest
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if !s.validateRequest(w, r, &req) {
		return
	}
	if err := validateURL(req.WebhookURL); err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	active := true
	if req.Active != nil {
		active = *req.Active
	}

	sub := &domain.WebhookSubscription{
		ProjectID:  req.ProjectID,
		WebhookURL: req.WebhookURL,
		EventTypes: req.EventTypes,
		Secret:     req.Secret,
		Active:     active,
	}

	if err := s.store.CreateWebhookSubscription(r.Context(), sub); err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to create webhook subscription")
		return
	}

	respondJSON(w, http.StatusCreated, sub)
}

func (s *Server) handleListWebhookSubscriptions(w http.ResponseWriter, r *http.Request) {
	if !s.config.FFWebhookSubscriptions {
		respondError(w, r, http.StatusNotFound, "webhook subscriptions feature is not enabled")
		return
	}

	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		respondError(w, r, http.StatusBadRequest, "project_id is required")
		return
	}

	subs, err := s.store.ListWebhookSubscriptions(r.Context(), projectID)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list webhook subscriptions")
		return
	}

	respondJSON(w, http.StatusOK, subs)
}

func (s *Server) handleDeleteWebhookSubscription(w http.ResponseWriter, r *http.Request) {
	if !s.config.FFWebhookSubscriptions {
		respondError(w, r, http.StatusNotFound, "webhook subscriptions feature is not enabled")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		respondError(w, r, http.StatusBadRequest, "subscription id is required")
		return
	}

	err := s.store.DeleteWebhookSubscription(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrWebhookSubscriptionNotFound) {
			respondError(w, r, http.StatusNotFound, "webhook subscription not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to delete webhook subscription")
		return
	}

	respondJSON(w, http.StatusNoContent, nil)
}
