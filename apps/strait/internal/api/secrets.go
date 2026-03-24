package api

import (
	"context"
	"errors"
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

	if err := s.store.CreateJobSecret(ctx, secret); err != nil {
		return nil, huma.Error500InternalServerError("failed to create secret")
	}

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
	if err := s.store.DeleteJobSecret(ctx, input.SecretID); err != nil {
		if errors.Is(err, store.ErrJobSecretNotFound) {
			return nil, huma.Error404NotFound("secret not found")
		}
		return nil, huma.Error500InternalServerError("failed to delete secret")
	}

	return nil, nil
}
