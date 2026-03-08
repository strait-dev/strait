package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"orchestrator/internal/domain"

	"github.com/go-chi/chi/v5"
)

type CreateAPIKeyRequest struct {
	ProjectID string   `json:"project_id" validate:"required"`
	Name      string   `json:"name" validate:"required"`
	Scopes    []string `json:"scopes,omitempty"`
	ExpiresIn *int     `json:"expires_in_days,omitempty"`
}

type CreateAPIKeyResponse struct {
	ID        string     `json:"id"`
	ProjectID string     `json:"project_id"`
	Name      string     `json:"name"`
	Key       string     `json:"key"`
	KeyPrefix string     `json:"key_prefix"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

func generateAPIKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate api key: %w", err)
	}

	return "orc_" + hex.EncodeToString(b), nil
}

func hashAPIKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:])
}

func (s *Server) handleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	var req CreateAPIKeyRequest
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if !s.validateRequest(w, r, &req) {
		return
	}

	rawKey, err := generateAPIKey()
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to generate api key")
		return
	}

	if req.Scopes == nil {
		req.Scopes = []string{}
	}

	var expiresAt *time.Time
	if req.ExpiresIn != nil && *req.ExpiresIn > 0 {
		t := time.Now().Add(time.Duration(*req.ExpiresIn) * 24 * time.Hour)
		expiresAt = &t
	}

	key := &domain.APIKey{
		ProjectID: req.ProjectID,
		Name:      req.Name,
		KeyHash:   hashAPIKey(rawKey),
		KeyPrefix: rawKey[:12],
		Scopes:    req.Scopes,
		ExpiresAt: expiresAt,
	}

	if err := s.store.CreateAPIKey(r.Context(), key); err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to create api key")
		return
	}

	respondJSON(w, http.StatusCreated, CreateAPIKeyResponse{
		ID:        key.ID,
		ProjectID: key.ProjectID,
		Name:      key.Name,
		Key:       rawKey,
		KeyPrefix: key.KeyPrefix,
		Scopes:    key.Scopes,
		ExpiresAt: key.ExpiresAt,
		CreatedAt: key.CreatedAt,
	})
}

func (s *Server) handleListAPIKeys(w http.ResponseWriter, r *http.Request) {
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

	keys, err := s.store.ListAPIKeysByProject(r.Context(), projectID, limit+1, cursor)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list api keys")
		return
	}

	respondJSON(w, http.StatusOK, paginatedResult(keys, limit, func(k domain.APIKey) string {
		return k.CreatedAt.Format(time.RFC3339Nano)
	}))
}

func (s *Server) handleRevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	keyID := chi.URLParam(r, "keyID")

	if err := s.store.RevokeAPIKey(r.Context(), keyID); err != nil {
		respondError(w, r, http.StatusNotFound, "api key not found or already revoked")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}
