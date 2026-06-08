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
	"github.com/google/uuid"
)

const (
	deviceCodeExpiresIn           = 900
	cliAuthRateLimitRequests      = 10
	cliAuthRateLimitWindow        = time.Minute
	deviceCodePollInterval        = 7
	defaultCLIKeyLifetimeDays int = 90
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
	UserCode  string `json:"user_code" validate:"required"`
	ProjectID string `json:"project_id" validate:"required"`
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
	if _, err := s.store.CleanupExpiredDeviceCodes(ctx); err != nil {
		slog.Error("failed to cleanup expired device codes before issuing new code", "error", err)
		return nil, huma.Error500InternalServerError("failed to cleanup expired device codes")
	}
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
	if _, err := s.store.CleanupExpiredDeviceCodes(ctx); err != nil {
		slog.Error("failed to cleanup expired device codes before token exchange", "error", err)
		return nil, huma.Error500InternalServerError("failed to cleanup expired device codes")
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
			// A code that expired in the window between the check above and the
			// atomic exchange must report expired_token, not the misleading
			// token_already_exchanged.
			if errors.Is(exchangeErr, store.ErrDeviceCodeExpired) {
				return nil, &rawStatusError{status: http.StatusBadRequest, body: map[string]string{"error": "expired_token"}}
			}
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
	row, err := s.store.GetDeviceCodeByUserCode(ctx, req.UserCode)
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
	if err := requireProjectMatch(ctx, req.ProjectID); err != nil {
		return nil, huma.Error403Forbidden("project_id does not match authenticated project")
	}
	if err := s.validateCallerCanGrantPermissions(ctx, domain.CLIDefaultScopes); err != nil {
		return nil, err
	}
	rawKey, err := generateAPIKey()
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to generate api key")
	}
	expiresAt, err := s.cliAPIKeyExpiry(ctx, req.ProjectID)
	if err != nil {
		return nil, err
	}
	apiKey := &domain.APIKey{
		// Pre-assign the UUID so buildAuditEvent below can capture the
		// final api_key_id in the audit details. CreateAPIKey would
		// otherwise assign the ID inside the tx, after we have already
		// serialized the audit event with an empty string. The store's
		// CreateAPIKey treats a non-empty ID as "use this one".
		ID:            uuid.Must(uuid.NewV7()).String(),
		ProjectID:     req.ProjectID,
		Name:          "CLI (device-code " + row.UserCode + ")",
		KeyHash:       hashAPIKey(rawKey),
		KeyPrefix:     rawKey[:domain.APIKeyPrefixLen],
		Scopes:        domain.CLIDefaultScopes,
		ExpiresAt:     expiresAt,
		EnvironmentID: environmentIDFromContext(ctx),
	}
	// CreateAPIKey + ApproveDeviceCodeByUserCode + audit insert must
	// commit atomically. Without the wrapping tx, a race where the
	// device code transitions out of 'pending' between the two calls
	// (concurrent approval) leaves the api_keys row orphaned: it is
	// never returned to the polling CLI and never revoked by any other
	// path. The audit event is inserted in the same tx so a crash or
	// audit-store outage cannot produce an approved-but-not-audited
	// device code, which would silently bypass our compliance trail
	// for credential issuance.
	auditEvent, auditErr := s.buildAuditEvent(ctx, domain.AuditActionDeviceCodeApproved, "device_code", row.ID, map[string]any{
		"user_code":  row.UserCode,
		"api_key_id": apiKey.ID,
		"project_id": req.ProjectID,
	})
	if auditErr != nil {
		// Refuse to issue credentials without an audit row. Approving
		// without an audit trail is a compliance failure; a marshal bug
		// is fixable in code, but a credential silently issued without
		// audit is not.
		return nil, huma.Error500InternalServerError("failed to build audit event")
	}
	if err := s.runInTx(ctx, func(txStore APIStore) error {
		if err := txStore.CreateAPIKey(ctx, apiKey); err != nil {
			return fmt.Errorf("create api key: %w", err)
		}
		if err := txStore.ApproveDeviceCodeByUserCode(ctx, req.UserCode, apiKey.ID, rawKey, req.ProjectID, domain.CLIDefaultScopes); err != nil {
			return fmt.Errorf("approve device code: %w", err)
		}
		if auditEvent != nil {
			if err := txStore.CreateAuditEvent(ctx, auditEvent); err != nil {
				return fmt.Errorf("audit device code approval: %w", err)
			}
		}
		return nil
	}); err != nil {
		if errors.Is(err, store.ErrDeviceCodeNotFound) {
			return nil, huma.Error404NotFound("device code not found")
		}
		return nil, huma.Error500InternalServerError("failed to approve device code")
	}
	slog.Info("device code approved", "device_code_id", row.ID, "user_code", row.UserCode, "api_key_id", apiKey.ID, "project_id", req.ProjectID, "actor", actorFromContext(ctx))

	return &ApproveDeviceCodeOutput{Body: map[string]string{"status": "approved"}}, nil
}

func (s *Server) cliAPIKeyExpiry(ctx context.Context, projectID string) (*time.Time, error) {
	lifetimeDays := defaultCLIKeyLifetimeDays
	quota, err := s.store.GetProjectQuota(ctx, projectID)
	if err != nil {
		slog.Warn("failed to load project quota while approving device code",
			"project_id", projectID, "error", err)
		return nil, huma.Error500InternalServerError("failed to load project quota")
	}
	if quota != nil && quota.MaxKeyLifetimeDays > 0 && quota.MaxKeyLifetimeDays < lifetimeDays {
		lifetimeDays = quota.MaxKeyLifetimeDays
	}
	expiresAt := time.Now().Add(time.Duration(lifetimeDays) * 24 * time.Hour)
	return &expiresAt, nil
}
