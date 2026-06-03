package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"strait/internal/domain"
	"strait/internal/logdrain"
	"strait/internal/store"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
)

func validateAuthConfig(authType string, config map[string]string) error {
	if config == nil {
		return nil
	}
	for k, v := range config {
		// Reject CR/LF/NUL anywhere in keys or values to prevent header
		// splitting / response injection at delivery time. The drain worker
		// replays these into req.Header.Set; even though net/http will
		// panic on CRLF in modern Go versions, we block the value at write
		// time so it never reaches the database in the first place.
		if hasHeaderInjectionChars(k) || hasHeaderInjectionChars(v) {
			return fmt.Errorf("auth_config entries must not contain CR, LF, or NUL characters")
		}
		if !isValidHeaderName(k) {
			return fmt.Errorf("auth_config key %q is not a valid HTTP header name", k)
		}
		if authType == "header" && logdrain.ProtectedHeaders[strings.ToLower(k)] {
			return fmt.Errorf("auth_config key %q is a protected HTTP header and cannot be used", k)
		}
	}
	return nil
}

// hasHeaderInjectionChars reports whether s contains any byte that would
// allow HTTP header splitting (\r, \n) or terminate a C string (\x00).
func hasHeaderInjectionChars(s string) bool {
	for i := range len(s) {
		switch s[i] {
		case '\r', '\n', 0:
			return true
		}
	}
	return false
}

// isValidHeaderName mirrors RFC 7230 token grammar for HTTP header names.
// Empty or names containing whitespace, control characters, or separators
// are rejected.
func isValidHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for i := range len(name) {
		c := name[i]
		// token character set: letters, digits, and !#$%&'*+-.^_`|~
		switch {
		case c >= 'a' && c <= 'z',
			c >= 'A' && c <= 'Z',
			c >= '0' && c <= '9',
			c == '!', c == '#', c == '$', c == '%', c == '&', c == '\'',
			c == '*', c == '+', c == '-', c == '.', c == '^', c == '_',
			c == '`', c == '|', c == '~':
			continue
		default:
			return false
		}
	}
	return true
}

type CreateLogDrainRequest struct {
	ProjectID   string            `json:"project_id" validate:"required"`
	Name        string            `json:"name" validate:"required,max=255"`
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

type CreateLogDrainInput struct{ Body CreateLogDrainRequest }
type CreateLogDrainOutput struct{ Body *domain.LogDrain }

// redactLogDrainAuth returns a shallow copy of d with auth_config values
// replaced by a fixed redaction token. Keys are preserved so callers can see
// the structure (e.g., which header names were configured) without ever
// reading the secret back. The original drain is not mutated.
func redactLogDrainAuth(d *domain.LogDrain) *domain.LogDrain {
	if d == nil {
		return nil
	}
	out := *d
	if len(d.AuthConfig) > 0 {
		out.AuthConfig = make(map[string]string, len(d.AuthConfig))
		for k := range d.AuthConfig {
			out.AuthConfig[k] = "***"
		}
	}
	return &out
}

func redactLogDrainList(in []domain.LogDrain) []domain.LogDrain {
	if len(in) == 0 {
		return in
	}
	out := make([]domain.LogDrain, len(in))
	for i := range in {
		out[i] = *redactLogDrainAuth(&in[i])
	}
	return out
}

func (s *Server) handleCreateLogDrain(ctx context.Context, input *CreateLogDrainInput) (*CreateLogDrainOutput, error) {
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}
	if err := requireProjectMatch(ctx, req.ProjectID); err != nil {
		return nil, huma.Error403Forbidden("project_id does not match authenticated project")
	}
	if err := validateURL(req.EndpointURL); err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	if err := validateAuthConfig(req.AuthType, req.AuthConfig); err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	orgID, maxDrains, displayName, err := s.resolveLogDrainCreateLimit(ctx, req.ProjectID)
	if err != nil {
		return nil, err
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	drain := &domain.LogDrain{ID: uuid.Must(uuid.NewV7()).String(), ProjectID: req.ProjectID, Name: req.Name, DrainType: req.DrainType, EndpointURL: req.EndpointURL, AuthType: req.AuthType, AuthConfig: req.AuthConfig, LevelFilter: req.LevelFilter, Enabled: enabled}
	var createErr error
	if creator, ok := s.store.(logDrainOrgLimitCreator); ok {
		createErr = creator.CreateLogDrainWithOrgLimit(ctx, drain, orgID, maxDrains)
	} else {
		if err := s.checkLogDrainLimit(ctx, req.ProjectID); err != nil {
			return nil, err
		}
		createErr = s.store.CreateLogDrain(ctx, drain)
	}
	if createErr != nil {
		if errors.Is(createErr, store.ErrLogDrainLimitExceeded) {
			return nil, huma.Error400BadRequest(
				fmt.Sprintf("Your %s plan allows %d log drains. Upgrade at /settings/billing", displayName, maxDrains),
			)
		}
		return nil, huma.Error500InternalServerError("failed to create log drain")
	}
	s.emitAuditEvent(ctx, domain.AuditActionLogDrainCreated, "log_drain", drain.ID, map[string]any{
		"name":          drain.Name,
		"drain_type":    drain.DrainType,
		"endpoint_host": urlHost(drain.EndpointURL),
		"auth_type":     drain.AuthType,
		"enabled":       drain.Enabled,
	})
	return &CreateLogDrainOutput{Body: redactLogDrainAuth(drain)}, nil
}

type ListLogDrainsInput struct{}
type ListLogDrainsOutput struct{ Body []domain.LogDrain }

func (s *Server) handleListLogDrains(ctx context.Context, _ *ListLogDrainsInput) (*ListLogDrainsOutput, error) {
	drains, err := s.store.ListLogDrains(ctx, projectIDFromContext(ctx))
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list log drains")
	}
	return &ListLogDrainsOutput{Body: redactLogDrainList(drains)}, nil
}

type GetLogDrainInput struct {
	DrainID string `path:"drainID"`
}
type GetLogDrainOutput struct{ Body *domain.LogDrain }

func (s *Server) handleGetLogDrain(ctx context.Context, input *GetLogDrainInput) (*GetLogDrainOutput, error) {
	drain, err := s.store.GetLogDrain(ctx, input.DrainID, projectIDFromContext(ctx))
	if err != nil {
		if errors.Is(err, store.ErrLogDrainNotFound) {
			return nil, huma.Error404NotFound("log drain not found")
		}
		return nil, huma.Error500InternalServerError("failed to get log drain")
	}
	return &GetLogDrainOutput{Body: redactLogDrainAuth(drain)}, nil
}

type UpdateLogDrainInput struct {
	DrainID string `path:"drainID"`
	Body    UpdateLogDrainRequest
}
type UpdateLogDrainOutput struct{ Body *domain.LogDrain }

func (s *Server) handleUpdateLogDrain(ctx context.Context, input *UpdateLogDrainInput) (*UpdateLogDrainOutput, error) {
	drainID := input.DrainID
	projectID := projectIDFromContext(ctx)
	req := input.Body
	patch := make(map[string]any)
	if req.Name != nil {
		patch["name"] = *req.Name
	}
	if req.EndpointURL != nil {
		if err := validateURL(*req.EndpointURL); err != nil {
			return nil, huma.Error400BadRequest(err.Error())
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
			existing, getErr := s.store.GetLogDrain(ctx, drainID, projectID)
			if getErr != nil {
				if errors.Is(getErr, store.ErrLogDrainNotFound) {
					return nil, huma.Error404NotFound("log drain not found")
				}
				return nil, huma.Error500InternalServerError("failed to get log drain")
			}
			authType = existing.AuthType
		}
		if err := validateAuthConfig(authType, req.AuthConfig); err != nil {
			return nil, huma.Error400BadRequest(err.Error())
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
		return nil, huma.Error400BadRequest("no fields to update")
	}
	if err := s.store.UpdateLogDrain(ctx, drainID, projectID, patch); err != nil {
		if errors.Is(err, store.ErrLogDrainNotFound) {
			return nil, huma.Error404NotFound("log drain not found")
		}
		return nil, huma.Error500InternalServerError("failed to update log drain")
	}
	drain, err := s.store.GetLogDrain(ctx, drainID, projectID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get updated log drain")
	}
	changedFields := make([]string, 0, len(patch))
	for k := range patch {
		if k == "auth_config" {
			continue // redacted
		}
		changedFields = append(changedFields, k)
	}
	s.emitAuditEvent(ctx, domain.AuditActionLogDrainUpdated, "log_drain", drainID, map[string]any{
		"name":                drain.Name,
		"endpoint_host":       urlHost(drain.EndpointURL),
		"changed_fields":      changedFields,
		"auth_config_changed": req.AuthConfig != nil,
	})
	return &UpdateLogDrainOutput{Body: redactLogDrainAuth(drain)}, nil
}

type DeleteLogDrainInput struct {
	DrainID string `path:"drainID"`
}

func (s *Server) handleDeleteLogDrain(ctx context.Context, input *DeleteLogDrainInput) (*struct{}, error) {
	projectID := projectIDFromContext(ctx)
	if err := s.store.DeleteLogDrain(ctx, input.DrainID, projectID); err != nil {
		if errors.Is(err, store.ErrLogDrainNotFound) {
			return nil, huma.Error404NotFound("log drain not found")
		}
		return nil, huma.Error500InternalServerError("failed to delete log drain")
	}
	s.emitAuditEvent(ctx, domain.AuditActionLogDrainDeleted, "log_drain", input.DrainID, nil)
	return nil, nil
}
