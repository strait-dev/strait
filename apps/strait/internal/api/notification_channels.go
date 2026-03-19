package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/go-chi/chi/v5"
)

// validateNotificationChannelConfig extracts the webhook URL from the channel
// config based on channel type and validates it against SSRF.
func validateNotificationChannelConfig(channelType string, config json.RawMessage) error {
	var parsed map[string]any
	if err := json.Unmarshal(config, &parsed); err != nil {
		return fmt.Errorf("invalid config JSON: %w", err)
	}

	var urlField string
	switch channelType {
	case domain.ChannelTypeSlack, domain.ChannelTypeDiscord:
		urlField = "webhook_url"
	case domain.ChannelTypeWebhook:
		urlField = "url"
	default:
		return fmt.Errorf("unsupported channel type: %s", channelType)
	}

	rawURL, ok := parsed[urlField]
	if !ok {
		return fmt.Errorf("%s is required in config", urlField)
	}
	urlStr, ok := rawURL.(string)
	if !ok || urlStr == "" {
		return fmt.Errorf("%s must be a non-empty string", urlField)
	}

	return validateURL(urlStr)
}

type CreateNotificationChannelRequest struct {
	ChannelType string          `json:"channel_type" validate:"required,oneof=slack discord webhook"`
	Name        string          `json:"name" validate:"required"`
	Config      json.RawMessage `json:"config" validate:"required"`
	Enabled     *bool           `json:"enabled,omitempty"`
}

type UpdateNotificationChannelRequest struct {
	Name        *string          `json:"name,omitempty"`
	ChannelType *string          `json:"channel_type,omitempty"`
	Config      *json.RawMessage `json:"config,omitempty"`
	Enabled     *bool            `json:"enabled,omitempty"`
}

func (s *Server) handleCreateNotificationChannel(w http.ResponseWriter, r *http.Request) {
	var req CreateNotificationChannelRequest
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if !s.validateRequest(w, r, &req) {
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	projectID := projectIDFromContext(r.Context())
	if projectID == "" {
		respondError(w, r, http.StatusBadRequest, "project_id is required")
		return
	}

	if err := validateNotificationChannelConfig(req.ChannelType, req.Config); err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	ch := &domain.NotificationChannel{
		ProjectID:   projectID,
		ChannelType: req.ChannelType,
		Name:        req.Name,
		Config:      req.Config,
		Enabled:     enabled,
	}

	if err := s.store.CreateNotificationChannel(r.Context(), ch); err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to create notification channel")
		return
	}

	respondJSON(w, http.StatusCreated, ch)
}

func (s *Server) handleListNotificationChannels(w http.ResponseWriter, r *http.Request) {
	projectID := projectIDFromContext(r.Context())
	if projectID == "" {
		respondError(w, r, http.StatusBadRequest, "project_id is required")
		return
	}

	channels, err := s.store.ListNotificationChannels(r.Context(), projectID)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list notification channels")
		return
	}

	respondJSON(w, http.StatusOK, channels)
}

func (s *Server) handleGetNotificationChannel(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "channelID")
	if channelID == "" {
		respondError(w, r, http.StatusBadRequest, "channel id is required")
		return
	}

	projectID := projectIDFromContext(r.Context())
	if projectID == "" {
		respondError(w, r, http.StatusBadRequest, "project_id is required")
		return
	}

	ch, err := s.store.GetNotificationChannel(r.Context(), channelID, projectID)
	if err != nil {
		if errors.Is(err, store.ErrNotificationChannelNotFound) {
			respondError(w, r, http.StatusNotFound, "notification channel not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get notification channel")
		return
	}

	respondJSON(w, http.StatusOK, ch)
}

func (s *Server) handleUpdateNotificationChannel(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "channelID")
	if channelID == "" {
		respondError(w, r, http.StatusBadRequest, "channel id is required")
		return
	}

	projectID := projectIDFromContext(r.Context())
	if projectID == "" {
		respondError(w, r, http.StatusBadRequest, "project_id is required")
		return
	}

	var req UpdateNotificationChannelRequest
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	ch, err := s.store.GetNotificationChannel(r.Context(), channelID, projectID)
	if err != nil {
		if errors.Is(err, store.ErrNotificationChannelNotFound) {
			respondError(w, r, http.StatusNotFound, "notification channel not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get notification channel")
		return
	}

	if req.Name != nil {
		ch.Name = *req.Name
	}
	if req.ChannelType != nil {
		ch.ChannelType = *req.ChannelType
	}
	if req.Config != nil {
		ch.Config = *req.Config
	}
	if req.Enabled != nil {
		ch.Enabled = *req.Enabled
	}

	if req.Config != nil || req.ChannelType != nil {
		if err := validateNotificationChannelConfig(ch.ChannelType, ch.Config); err != nil {
			respondError(w, r, http.StatusBadRequest, err.Error())
			return
		}
	}

	if err := s.store.UpdateNotificationChannel(r.Context(), ch); err != nil {
		if errors.Is(err, store.ErrNotificationChannelNotFound) {
			respondError(w, r, http.StatusNotFound, "notification channel not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to update notification channel")
		return
	}

	respondJSON(w, http.StatusOK, ch)
}

func (s *Server) handleDeleteNotificationChannel(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "channelID")
	if channelID == "" {
		respondError(w, r, http.StatusBadRequest, "channel id is required")
		return
	}

	projectID := projectIDFromContext(r.Context())
	if projectID == "" {
		respondError(w, r, http.StatusBadRequest, "project_id is required")
		return
	}

	err := s.store.DeleteNotificationChannel(r.Context(), channelID, projectID)
	if err != nil {
		if errors.Is(err, store.ErrNotificationChannelNotFound) {
			respondError(w, r, http.StatusNotFound, "notification channel not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to delete notification channel")
		return
	}

	respondJSON(w, http.StatusNoContent, nil)
}

func (s *Server) handleListNotificationDeliveries(w http.ResponseWriter, r *http.Request) {
	projectID := projectIDFromContext(r.Context())
	if projectID == "" {
		respondError(w, r, http.StatusBadRequest, "project_id is required")
		return
	}

	limit := defaultPageLimit
	if l := r.URL.Query().Get("limit"); l != "" {
		parsed, err := strconv.Atoi(l)
		if err == nil && parsed > 0 && parsed <= maxPageLimit {
			limit = parsed
		}
	}

	var cursor *time.Time
	if c := r.URL.Query().Get("cursor"); c != "" {
		t, err := time.Parse(time.RFC3339Nano, c)
		if err == nil {
			cursor = &t
		}
	}

	deliveries, err := s.store.ListNotificationDeliveries(r.Context(), projectID, limit, cursor)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list notification deliveries")
		return
	}

	respondJSON(w, http.StatusOK, deliveries)
}
