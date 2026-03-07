package api

import (
	"errors"
	"net/http"

	"orchestrator/internal/domain"
	"orchestrator/internal/store"

	"github.com/go-chi/chi/v5"
)

type createSecretRequest struct {
	ProjectID   string `json:"project_id"`
	JobID       string `json:"job_id,omitempty"`
	Environment string `json:"environment,omitempty"`
	SecretKey   string `json:"secret_key"`
	Value       string `json:"value"`
}

func (s *Server) handleCreateSecret(w http.ResponseWriter, r *http.Request) {
	if !s.config.FFSecretInjection {
		respondError(w, http.StatusNotFound, "secret injection is not enabled")
		return
	}

	var req createSecretRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.ProjectID == "" || req.SecretKey == "" || req.Value == "" {
		respondError(w, http.StatusBadRequest, "missing required fields")
		return
	}

	if req.Environment == "" {
		req.Environment = "production"
	}

	secret := &domain.JobSecret{
		ProjectID:      req.ProjectID,
		JobID:          req.JobID,
		Environment:    req.Environment,
		SecretKey:      req.SecretKey,
		EncryptedValue: req.Value,
	}

	if err := s.store.CreateJobSecret(r.Context(), secret); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create secret")
		return
	}

	respondJSON(w, http.StatusCreated, secret)
}

func (s *Server) handleListSecrets(w http.ResponseWriter, r *http.Request) {
	if !s.config.FFSecretInjection {
		respondError(w, http.StatusNotFound, "secret injection is not enabled")
		return
	}

	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		respondError(w, http.StatusBadRequest, "project_id is required")
		return
	}

	jobID := r.URL.Query().Get("job_id")
	environment := r.URL.Query().Get("environment")

	secrets, err := s.store.ListJobSecrets(r.Context(), projectID, jobID, environment)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list secrets")
		return
	}

	respondJSON(w, http.StatusOK, secrets)
}

func (s *Server) handleDeleteSecret(w http.ResponseWriter, r *http.Request) {
	if !s.config.FFSecretInjection {
		respondError(w, http.StatusNotFound, "secret injection is not enabled")
		return
	}

	secretID := chi.URLParam(r, "secretID")
	if err := s.store.DeleteJobSecret(r.Context(), secretID); err != nil {
		if errors.Is(err, store.ErrJobSecretNotFound) {
			respondError(w, http.StatusNotFound, "secret not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to delete secret")
		return
	}

	respondJSON(w, http.StatusNoContent, nil)
}
