package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"strait/internal/domain"
	"strait/internal/logdrain"
	"strait/internal/store"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func validateAuthConfig(authType string, config map[string]string) error {
	if authType != "header" || config == nil {
		return nil
	}
	for k := range config {
		if logdrain.ProtectedHeaders[strings.ToLower(k)] {
			return fmt.Errorf("auth_config key %q is a protected HTTP header and cannot be used", k)
		}
	}
	return nil
}

type CreateLogDrainRequest struct {
	ProjectID   string            `json:"project_id" validate:"required"`
	Name        string            `json:"name" validate:"required"`
	DrainType   string            `json:"drain_type" validate:"required"`
	EndpointURL string            `json:"endpoint_url" validate:"required"`
	AuthType    string            `json:"auth_type" validate:"required"`
	AuthConfig  map[string]string `json:"auth_config,omitempty"`
	LevelFilter []string          `json:"level_filter,omitempty"`
	Enabled     *bool             `json:"enabled,omitempty"`
}

type UpdateLogDrainRequest struct {
	Name        *string           `json:"name,omitempty"`
	EndpointURL *string           `json:"endpoint_url,omitempty"`
	AuthType    *string           `json:"auth_type,omitempty"`
	AuthConfig  map[string]string `json:"auth_config,omitempty"`
	LevelFilter []string          `json:"level_filter,omitempty"`
	Enabled     *bool             `json:"enabled,omitempty"`
}

func (s *Server) handleCreateLogDrain(w http.ResponseWriter, r *http.Request) {
	var req CreateLogDrainRequest
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if !s.validateRequest(w, r, &req) {
		return
	}
	if err := validateURL(req.EndpointURL); err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateAuthConfig(req.AuthType, req.AuthConfig); err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	drain := &domain.LogDrain{
		ID:          uuid.Must(uuid.NewV7()).String(),
		ProjectID:   req.ProjectID,
		Name:        req.Name,
		DrainType:   req.DrainType,
		EndpointURL: req.EndpointURL,
		AuthType:    req.AuthType,
		AuthConfig:  req.AuthConfig,
		LevelFilter: req.LevelFilter,
		Enabled:     enabled,
	}

	if err := s.store.CreateLogDrain(r.Context(), drain); err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to create log drain")
		return
	}

	respondJSON(w, http.StatusCreated, drain)
}

func (s *Server) handleListLogDrains(w http.ResponseWriter, r *http.Request) {
	projectID := projectIDFromContext(r.Context())

	drains, err := s.store.ListLogDrains(r.Context(), projectID)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list log drains")
		return
	}

	respondJSON(w, http.StatusOK, drains)
}

func (s *Server) handleGetLogDrain(w http.ResponseWriter, r *http.Request) {
	drainID := chi.URLParam(r, "drainID")
	projectID := projectIDFromContext(r.Context())

	drain, err := s.store.GetLogDrain(r.Context(), drainID, projectID)
	if err != nil {
		if errors.Is(err, store.ErrLogDrainNotFound) {
			respondError(w, r, http.StatusNotFound, "log drain not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get log drain")
		return
	}

	respondJSON(w, http.StatusOK, drain)
}

func (s *Server) handleUpdateLogDrain(w http.ResponseWriter, r *http.Request) {
	drainID := chi.URLParam(r, "drainID")
	projectID := projectIDFromContext(r.Context())

	var req UpdateLogDrainRequest
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	patch := make(map[string]any)
	if req.Name != nil {
		patch["name"] = *req.Name
	}
	if req.EndpointURL != nil {
		if err := validateURL(*req.EndpointURL); err != nil {
			respondError(w, r, http.StatusBadRequest, err.Error())
			return
		}
		patch["endpoint_url"] = *req.EndpointURL
	}
	if req.AuthType != nil {
		patch["auth_type"] = *req.AuthType
	}
	if req.AuthConfig != nil {
		var authType string
		if req.AuthType != nil {
			authType = *req.AuthType
		} else {
			// Load existing drain's auth_type to validate against the current
			// type when the PATCH body doesn't include auth_type.
			existing, getErr := s.store.GetLogDrain(r.Context(), drainID, projectID)
			if getErr != nil {
				if errors.Is(getErr, store.ErrLogDrainNotFound) {
					respondError(w, r, http.StatusNotFound, "log drain not found")
					return
				}
				respondError(w, r, http.StatusInternalServerError, "failed to get log drain")
				return
			}
			authType = existing.AuthType
		}
		if err := validateAuthConfig(authType, req.AuthConfig); err != nil {
			respondError(w, r, http.StatusBadRequest, err.Error())
			return
		}
		authJSON, _ := json.Marshal(req.AuthConfig)
		patch["auth_config"] = authJSON
	}
	if req.LevelFilter != nil {
		patch["level_filter"] = req.LevelFilter
	}
	if req.Enabled != nil {
		patch["enabled"] = *req.Enabled
	}

	if len(patch) == 0 {
		respondError(w, r, http.StatusBadRequest, "no fields to update")
		return
	}

	err := s.store.UpdateLogDrain(r.Context(), drainID, projectID, patch)
	if err != nil {
		if errors.Is(err, store.ErrLogDrainNotFound) {
			respondError(w, r, http.StatusNotFound, "log drain not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to update log drain")
		return
	}

	// Return updated drain.
	drain, err := s.store.GetLogDrain(r.Context(), drainID, projectID)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get updated log drain")
		return
	}

	respondJSON(w, http.StatusOK, drain)
}

func (s *Server) handleDeleteLogDrain(w http.ResponseWriter, r *http.Request) {
	drainID := chi.URLParam(r, "drainID")
	projectID := projectIDFromContext(r.Context())

	err := s.store.DeleteLogDrain(r.Context(), drainID, projectID)
	if err != nil {
		if errors.Is(err, store.ErrLogDrainNotFound) {
			respondError(w, r, http.StatusNotFound, "log drain not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to delete log drain")
		return
	}

	respondJSON(w, http.StatusNoContent, nil)
}
