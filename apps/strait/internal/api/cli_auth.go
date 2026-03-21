package api

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

const (
	// deviceCodeExpiresIn is how long a device code is valid (seconds).
	deviceCodeExpiresIn = 900
	// deviceCodePollInterval is the recommended polling interval (seconds).
	deviceCodePollInterval = 5
)

// userCodeAlphabet excludes ambiguous characters: 0, O, 1, l, I.
const userCodeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

type deviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURL string `json:"verification_url"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

type deviceTokenRequest struct {
	DeviceCode string `json:"device_code"`
	GrantType  string `json:"grant_type"`
}

type deviceTokenResponse struct {
	APIKey    string   `json:"api_key"`
	ProjectID string   `json:"project_id"`
	Scopes    []string `json:"scopes"`
}

type approveDeviceCodeRequest struct {
	DeviceCode string `json:"device_code" validate:"required"`
	ProjectID  string `json:"project_id" validate:"required"`
}

// generateDeviceCode creates a cryptographically random 32-byte hex string.
func generateDeviceCode() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate device code: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// generateUserCode creates an 8-character code from the unambiguous alphabet.
func generateUserCode() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate user code: %w", err)
	}
	code := make([]byte, 8)
	for i := range code {
		code[i] = userCodeAlphabet[int(b[i])%len(userCodeAlphabet)]
	}
	return string(code), nil
}

// handleDeviceCode handles POST /v1/cli/auth/device-code.
// It generates a device code and user code, stores them, and returns
// the codes to the CLI so the user can authorize in their browser.
func (s *Server) handleDeviceCode(w http.ResponseWriter, r *http.Request) {
	deviceCode, err := generateDeviceCode()
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to generate device code")
		return
	}

	userCode, err := generateUserCode()
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to generate user code")
		return
	}

	expiresAt := time.Now().Add(deviceCodeExpiresIn * time.Second)

	// Store with empty project_id and scopes; these are set during approval.
	if err := s.store.CreateDeviceCode(r.Context(), deviceCode, userCode, "", []string{}, expiresAt); err != nil {
		slog.Error("failed to create device code", "error", err)
		respondError(w, r, http.StatusInternalServerError, "failed to create device code")
		return
	}

	respondJSON(w, http.StatusOK, deviceCodeResponse{
		DeviceCode:      deviceCode,
		UserCode:        userCode,
		VerificationURL: "/cli/authorize",
		ExpiresIn:       deviceCodeExpiresIn,
		Interval:        deviceCodePollInterval,
	})
}

// handleDeviceToken handles POST /v1/cli/auth/token.
// The CLI polls this endpoint with a device_code until the code is approved.
func (s *Server) handleDeviceToken(w http.ResponseWriter, r *http.Request) {
	var req deviceTokenRequest
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.DeviceCode == "" {
		respondError(w, r, http.StatusBadRequest, "device_code is required")
		return
	}

	if req.GrantType != "device_code" {
		respondError(w, r, http.StatusBadRequest, "grant_type must be device_code")
		return
	}

	row, err := s.store.GetDeviceCodeByDeviceCode(r.Context(), req.DeviceCode)
	if err != nil {
		if errors.Is(err, store.ErrDeviceCodeNotFound) {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "expired_token"})
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to look up device code")
		return
	}

	// Check expiration regardless of status.
	if time.Now().After(row.ExpiresAt) {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "expired_token"})
		return
	}

	switch row.Status {
	case "pending":
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "authorization_pending"})
		return

	case "approved":
		// Capture the raw key before exchange clears it.
		rawKey := row.RawAPIKey
		projectID := row.ProjectID

		_, exchangeErr := s.store.ExchangeDeviceCode(r.Context(), req.DeviceCode)
		if exchangeErr != nil {
			if errors.Is(exchangeErr, store.ErrDeviceCodeNotFound) {
				respondJSON(w, http.StatusBadRequest, map[string]string{"error": "token_already_exchanged"})
				return
			}
			respondError(w, r, http.StatusInternalServerError, "failed to exchange device code")
			return
		}

		respondJSON(w, http.StatusOK, deviceTokenResponse{
			APIKey:    rawKey,
			ProjectID: projectID,
			Scopes:    row.Scopes,
		})
		return

	case "used":
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "token_already_exchanged"})
		return

	default:
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "expired_token"})
		return
	}
}

// handleApproveDeviceCode handles POST /v1/cli/auth/approve.
// This is called by the web app (authenticated) to approve a device code
// and create a scoped API key for the CLI.
func (s *Server) handleApproveDeviceCode(w http.ResponseWriter, r *http.Request) {
	var req approveDeviceCodeRequest
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if !s.validateRequest(w, r, &req) {
		return
	}

	// Verify the device code exists and is pending.
	row, err := s.store.GetDeviceCodeByDeviceCode(r.Context(), req.DeviceCode)
	if err != nil {
		if errors.Is(err, store.ErrDeviceCodeNotFound) {
			respondError(w, r, http.StatusNotFound, "device code not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to look up device code")
		return
	}

	if row.Status != "pending" {
		respondError(w, r, http.StatusConflict, "device code is not pending")
		return
	}

	if time.Now().After(row.ExpiresAt) {
		respondError(w, r, http.StatusBadRequest, "device code has expired")
		return
	}

	// Generate a new API key for the CLI session.
	rawKey, err := generateAPIKey()
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to generate api key")
		return
	}

	apiKey := &domain.APIKey{
		ProjectID: req.ProjectID,
		Name:      "CLI (device-code " + row.UserCode + ")",
		KeyHash:   hashAPIKey(rawKey),
		KeyPrefix: rawKey[:12],
		Scopes:    []string{},
	}

	if err := s.store.CreateAPIKey(r.Context(), apiKey); err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to create api key")
		return
	}

	// Approve the device code and store the raw key for the token exchange.
	if err := s.store.ApproveDeviceCode(r.Context(), req.DeviceCode, apiKey.ID, rawKey); err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to approve device code")
		return
	}

	slog.Info("device code approved",
		"device_code_id", row.ID,
		"user_code", row.UserCode,
		"api_key_id", apiKey.ID,
		"project_id", req.ProjectID,
		"actor", actorFromContext(r.Context()),
	)

	respondJSON(w, http.StatusOK, map[string]string{"status": "approved"})
}
