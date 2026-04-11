package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/danielgtaylor/huma/v2"
)

const (
	deviceCodeExpiresIn    = 900
	deviceCodePollInterval = 5
)
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

func generateDeviceCode() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate device code: %w", err)
	}
	return hex.EncodeToString(b), nil
}
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

type DeviceCodeInput struct{}
type DeviceCodeOutput struct{ Body deviceCodeResponse }

func (s *Server) handleDeviceCode(ctx context.Context, _ *DeviceCodeInput) (*DeviceCodeOutput, error) {
	dc, err := generateDeviceCode()
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to generate device code")
	}
	uc, err := generateUserCode()
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to generate user code")
	}
	expiresAt := time.Now().Add(deviceCodeExpiresIn * time.Second)
	if err := s.store.CreateDeviceCode(ctx, dc, uc, "", []string{}, expiresAt); err != nil {
		slog.Error("failed to create device code", "error", err)
		return nil, huma.Error500InternalServerError("failed to create device code")
	}
	return &DeviceCodeOutput{Body: deviceCodeResponse{DeviceCode: dc, UserCode: uc, VerificationURL: "/cli/authorize", ExpiresIn: deviceCodeExpiresIn, Interval: deviceCodePollInterval}}, nil
}

type DeviceTokenInput struct{ Body deviceTokenRequest }
type DeviceTokenOutput struct{ Body any }

func (s *Server) handleDeviceToken(ctx context.Context, input *DeviceTokenInput) (*DeviceTokenOutput, error) {
	req := input.Body
	if req.DeviceCode == "" {
		return nil, huma.Error400BadRequest("device_code is required")
	}
	if req.GrantType != "device_code" {
		return nil, huma.Error400BadRequest("grant_type must be device_code")
	}
	row, err := s.store.GetDeviceCodeByDeviceCode(ctx, req.DeviceCode)
	if err != nil {
		if errors.Is(err, store.ErrDeviceCodeNotFound) {
			return nil, &rawStatusError{status: http.StatusBadRequest, body: map[string]string{"error": "expired_token"}}
		}
		return nil, huma.Error500InternalServerError("failed to look up device code")
	}
	if time.Now().After(row.ExpiresAt) {
		return nil, &rawStatusError{status: http.StatusBadRequest, body: map[string]string{"error": "expired_token"}}
	}
	switch row.Status {
	case "pending":
		return nil, &rawStatusError{status: http.StatusBadRequest, body: map[string]string{"error": "authorization_pending"}}
	case "approved":
		rawKey := row.RawAPIKey
		projectID := row.ProjectID
		_, exchangeErr := s.store.ExchangeDeviceCode(ctx, req.DeviceCode)
		if exchangeErr != nil {
			if errors.Is(exchangeErr, store.ErrDeviceCodeNotFound) {
				return nil, &rawStatusError{status: http.StatusBadRequest, body: map[string]string{"error": "token_already_exchanged"}}
			}
			return nil, huma.Error500InternalServerError("failed to exchange device code")
		}
		return &DeviceTokenOutput{Body: deviceTokenResponse{APIKey: rawKey, ProjectID: projectID, Scopes: row.Scopes}}, nil
	case "used":
		return nil, &rawStatusError{status: http.StatusBadRequest, body: map[string]string{"error": "token_already_exchanged"}}
	default:
		return nil, &rawStatusError{status: http.StatusBadRequest, body: map[string]string{"error": "expired_token"}}
	}
}

type ApproveDeviceCodeInput struct{ Body approveDeviceCodeRequest }
type ApproveDeviceCodeOutput struct{ Body map[string]string }

func (s *Server) handleApproveDeviceCode(ctx context.Context, input *ApproveDeviceCodeInput) (*ApproveDeviceCodeOutput, error) {
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}
	row, err := s.store.GetDeviceCodeByDeviceCode(ctx, req.DeviceCode)
	if err != nil {
		if errors.Is(err, store.ErrDeviceCodeNotFound) {
			return nil, huma.Error404NotFound("device code not found")
		}
		return nil, huma.Error500InternalServerError("failed to look up device code")
	}
	if row.Status != "pending" {
		return nil, huma.Error409Conflict("device code is not pending")
	}
	if time.Now().After(row.ExpiresAt) {
		return nil, huma.Error400BadRequest("device code has expired")
	}
	rawKey, err := generateAPIKey()
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to generate api key")
	}
	apiKey := &domain.APIKey{ProjectID: req.ProjectID, Name: "CLI (device-code " + row.UserCode + ")", KeyHash: hashAPIKey(rawKey), KeyPrefix: rawKey[:12], Scopes: domain.CLIDefaultScopes}
	if err := s.store.CreateAPIKey(ctx, apiKey); err != nil {
		return nil, huma.Error500InternalServerError("failed to create api key")
	}
	if err := s.store.ApproveDeviceCode(ctx, req.DeviceCode, apiKey.ID, rawKey); err != nil {
		return nil, huma.Error500InternalServerError("failed to approve device code")
	}
	slog.Info("device code approved", "device_code_id", row.ID, "user_code", row.UserCode, "api_key_id", apiKey.ID, "project_id", req.ProjectID, "actor", actorFromContext(ctx))

	s.emitAuditEvent(ctx, "device_code.approved", "device_code", row.ID, map[string]any{
		"user_code":  row.UserCode,
		"api_key_id": apiKey.ID,
		"project_id": req.ProjectID,
	})

	return &ApproveDeviceCodeOutput{Body: map[string]string{"status": "approved"}}, nil
}
