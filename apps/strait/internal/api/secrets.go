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
	environmentID, err := s.resolveSecretEnvironment(ctx, req.ProjectID, req.Environment)
	if err != nil {
		return nil, err
	}
	if err := s.verifySecretJobEnvironment(ctx, req.ProjectID, req.JobID, environmentID); err != nil {
		return nil, err
	}

	secret := &domain.JobSecret{
		ProjectID:      req.ProjectID,
		JobID:          req.JobID,
		Environment:    environmentID,
		SecretKey:      req.SecretKey,
		EncryptedValue: req.Value,
	}

	if err := s.store.CreateJobSecret(ctx, secret); err != nil {
		slog.Error("failed to create secret", "error", err)
		return nil, huma.Error500InternalServerError("failed to create secret")
	}

	s.emitAuditEvent(ctx, domain.AuditActionSecretCreated, "secret", secret.ID, map[string]any{
		"secret_key":  req.SecretKey,
		"job_id":      req.JobID,
		"environment": environmentID,
	})

	return &CreateSecretOutput{Body: secret}, nil
}

func (s *Server) verifySecretJobEnvironment(ctx context.Context, projectID, jobID, environmentID string) error {
	if jobID == "" {
		return nil
	}
	job, err := s.store.GetJob(ctx, jobID)
	if err != nil {
		if errors.Is(err, store.ErrJobNotFound) {
			return huma.Error404NotFound("job not found")
		}
		return huma.Error500InternalServerError("failed to verify job")
	}
	if job == nil {
		return nil
	}
	if job.ProjectID != projectID {
		return huma.Error404NotFound("job not found")
	}
	if err := requireEnvironmentMatch(ctx, job.EnvironmentID); err != nil {
		return huma.Error404NotFound("job not found")
	}
	if job.EnvironmentID != "" && environmentID != job.EnvironmentID {
		return huma.Error403Forbidden("secret environment does not match job environment")
	}
	return nil
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

	environment := input.Environment
	if environmentIDFromContext(ctx) != "" || environment != "" {
		environment, err = s.resolveSecretEnvironment(ctx, projectID, environment)
		if err != nil {
			return nil, err
		}
	}

	secrets, err := s.store.ListJobSecrets(ctx, projectID, input.JobID, environment, limit+1, cursor)
	if err != nil {
		slog.Error("failed to list secrets", "error", err)
		return nil, huma.Error500InternalServerError("failed to list secrets")
	}

	s.emitAuditEvent(ctx, domain.AuditActionSecretListRead, "secret", "", map[string]any{
		"count":       len(secrets),
		"job_id":      input.JobID,
		"environment": environment,
	})

	return &ListSecretsOutput{Body: paginatedResult(secrets, limit, func(s domain.JobSecret) string {
		return s.CreatedAt.Format(time.RFC3339Nano)
	})}, nil
}

// GetSecretInput is the typed input for reading a single secret.
type GetSecretInput struct {
	SecretID string `path:"secretID"`
}

// GetSecretOutput is the typed output for reading a single secret.
// The body never includes the encrypted or decrypted value.
type GetSecretOutput struct {
	Body *domain.JobSecret
}

// handleGetSecret returns metadata for a single secret scoped to the caller's
// project. The secret value is never included in the response. The read is
// audited as secret.read with secret_id + name (key) — no key material.
func (s *Server) handleGetSecret(ctx context.Context, input *GetSecretInput) (*GetSecretOutput, error) {
	projectID := projectIDFromContext(ctx)
	secret, err := s.store.GetJobSecret(ctx, input.SecretID, projectID)
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
	if err := requireEnvironmentMatch(ctx, secret.Environment); err != nil {
		return nil, huma.Error404NotFound("secret not found")
	}

	s.emitAuditEvent(ctx, domain.AuditActionSecretRead, "secret", secret.ID, map[string]any{
		"secret_id":   secret.ID,
		"name":        secret.SecretKey,
		"secret_key":  secret.SecretKey,
		"job_id":      secret.JobID,
		"environment": secret.Environment,
	})

	return &GetSecretOutput{Body: secret}, nil
}

// DeleteSecretInput is the typed input for deleting a secret.
type DeleteSecretInput struct {
	SecretID string `path:"secretID"`
}

func (s *Server) handleDeleteSecret(ctx context.Context, input *DeleteSecretInput) (*struct{}, error) {
	projectID := projectIDFromContext(ctx)
	secret, err := s.store.GetJobSecret(ctx, input.SecretID, projectID)
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
	if err := requireEnvironmentMatch(ctx, secret.Environment); err != nil {
		return nil, huma.Error404NotFound("secret not found")
	}

	if err := s.store.DeleteJobSecret(ctx, input.SecretID, projectID); err != nil {
		if errors.Is(err, store.ErrJobSecretNotFound) {
			return nil, huma.Error404NotFound("secret not found")
		}
		slog.Error("failed to delete secret", "error", err)
		return nil, huma.Error500InternalServerError("failed to delete secret")
	}

	s.emitAuditEvent(ctx, domain.AuditActionSecretDeleted, "secret", input.SecretID, map[string]any{
		"secret_key":  secret.SecretKey,
		"job_id":      secret.JobID,
		"environment": secret.Environment,
	})

	return nil, nil
}

func (s *Server) resolveSecretEnvironment(ctx context.Context, projectID, requested string) (string, error) {
	callerEnv := environmentIDFromContext(ctx)
	if requested == "" && callerEnv != "" {
		return callerEnv, nil
	}
	if requested == "" {
		requested = "production"
	}

	envs, err := s.store.ListEnvironments(ctx, projectID, 1000, nil)
	if err != nil {
		return "", huma.Error500InternalServerError("failed to verify environment")
	}
	if len(envs) == 0 {
		if callerEnv != "" && requested != callerEnv {
			return "", huma.Error404NotFound("environment not found")
		}
		return requested, nil
	}

	for _, env := range envs {
		if env.ID == requested || env.Slug == requested || env.Name == requested {
			if err := requireEnvironmentMatch(ctx, env.ID); err != nil {
				return "", huma.Error404NotFound("environment not found")
			}
			return env.ID, nil
		}
	}
	return "", huma.Error404NotFound("environment not found")
}
