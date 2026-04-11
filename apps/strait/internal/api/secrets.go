package api

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"strait/internal/domain"
	"strait/internal/store"
)

type createSecretRequest struct {
	ProjectID   string `json:"project_id" validate:"required"`
	JobID       string `json:"job_id,omitempty"`
	Environment string `json:"environment,omitempty"`
	SecretKey   string `json:"secret_key" validate:"required"`
	Value       string `json:"value" validate:"required"`
}

// CreateSecretInput is the typed input for creating a secret.
type CreateSecretInput struct {
	Body createSecretRequest
}

// CreateSecretOutput is the typed output for creating a secret.
type CreateSecretOutput struct {
	Body *domain.JobSecret
}

func (s *Server) handleCreateSecret(ctx context.Context, input *CreateSecretInput) (*CreateSecretOutput, error) {
	req := input.Body

	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}
	if err := requireProjectMatch(ctx, req.ProjectID); err != nil {
		return nil, huma.Error403Forbidden("project_id does not match authenticated project")
	}

	if s.config.SecretEncryptionKey == "" {
		return nil, huma.Error503ServiceUnavailable("secret encryption is not configured -- set SECRET_ENCRYPTION_KEY")
	}

	if req.Environment == "" {
		req.Environment = "production"
	}

	encryptedValue := req.Value
	if s.encryptor != nil {
		enc, encErr := s.encryptor.Encrypt([]byte(req.Value))
		if encErr != nil {
			slog.Error("failed to encrypt secret value", "error", encErr)
			return nil, huma.Error500InternalServerError("failed to encrypt secret")
		}
		encryptedValue = string(enc)
	}

	secret := &domain.JobSecret{
		ProjectID:      req.ProjectID,
		JobID:          req.JobID,
		Environment:    req.Environment,
		SecretKey:      req.SecretKey,
		EncryptedValue: encryptedValue,
	}

	if err := s.store.CreateJobSecret(ctx, secret); err != nil {
		slog.Error("failed to create secret", "error", err)
		return nil, huma.Error500InternalServerError("failed to create secret")
	}

	s.emitAuditEvent(ctx, "secret.created", "secret", secret.ID, map[string]any{
		"secret_key":  req.SecretKey,
		"job_id":      req.JobID,
		"environment": req.Environment,
	})

	return &CreateSecretOutput{Body: secret}, nil
}

// ListSecretsInput is the typed input for listing secrets.
type ListSecretsInput struct {
	JobID       string `query:"job_id"`
	Environment string `query:"environment"`
	Limit       string `query:"limit"`
	Cursor      string `query:"cursor"`
}

// ListSecretsOutput is the typed output for listing secrets.
type ListSecretsOutput struct {
	Body PaginatedResponse
}

func (s *Server) handleListSecrets(ctx context.Context, input *ListSecretsInput) (*ListSecretsOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}

	limit, cursor, err := parsePaginationFromStrings(input.Limit, input.Cursor)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	secrets, err := s.store.ListJobSecrets(ctx, projectID, input.JobID, input.Environment, limit+1, cursor)
	if err != nil {
		slog.Error("failed to list secrets", "error", err)
		return nil, huma.Error500InternalServerError("failed to list secrets")
	}

	return &ListSecretsOutput{Body: paginatedResult(secrets, limit, func(s domain.JobSecret) string {
		return s.CreatedAt.Format(time.RFC3339Nano)
	})}, nil
}

// DeleteSecretInput is the typed input for deleting a secret.
type DeleteSecretInput struct {
	SecretID string `path:"secretID"`
}

func (s *Server) handleDeleteSecret(ctx context.Context, input *DeleteSecretInput) (*struct{}, error) {
	secret, err := s.store.GetJobSecret(ctx, input.SecretID)
	if err != nil {
		if errors.Is(err, store.ErrJobSecretNotFound) {
			return nil, huma.Error404NotFound("secret not found")
		}
		slog.Error("failed to get secret", "error", err)
		return nil, huma.Error500InternalServerError("failed to get secret")
	}
	if err := requireProjectMatch(ctx, secret.ProjectID); err != nil {
		return nil, huma.Error404NotFound("secret not found")
	}

	if err := s.store.DeleteJobSecret(ctx, input.SecretID); err != nil {
		if errors.Is(err, store.ErrJobSecretNotFound) {
			return nil, huma.Error404NotFound("secret not found")
		}
		slog.Error("failed to delete secret", "error", err)
		return nil, huma.Error500InternalServerError("failed to delete secret")
	}

	s.emitAuditEvent(ctx, "secret.deleted", "secret", input.SecretID, map[string]any{
		"secret_key":  secret.SecretKey,
		"job_id":      secret.JobID,
		"environment": secret.Environment,
	})

	return nil, nil
}
