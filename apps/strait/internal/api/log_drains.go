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

type CreateLogDrainInput struct{ Body CreateLogDrainRequest }
type CreateLogDrainOutput struct{ Body *domain.LogDrain }

func (s *Server) handleCreateLogDrain(ctx context.Context, input *CreateLogDrainInput) (*CreateLogDrainOutput, error) {
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}
	if err := validateURL(req.EndpointURL); err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	if err := validateAuthConfig(req.AuthType, req.AuthConfig); err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	drain := &domain.LogDrain{ID: uuid.Must(uuid.NewV7()).String(), ProjectID: req.ProjectID, Name: req.Name, DrainType: req.DrainType, EndpointURL: req.EndpointURL, AuthType: req.AuthType, AuthConfig: req.AuthConfig, LevelFilter: req.LevelFilter, Enabled: enabled}
	if err := s.store.CreateLogDrain(ctx, drain); err != nil {
		return nil, huma.Error500InternalServerError("failed to create log drain")
	}
	return &CreateLogDrainOutput{Body: drain}, nil
}

type ListLogDrainsInput struct{}
type ListLogDrainsOutput struct{ Body []domain.LogDrain }

func (s *Server) handleListLogDrains(ctx context.Context, _ *ListLogDrainsInput) (*ListLogDrainsOutput, error) {
	drains, err := s.store.ListLogDrains(ctx, projectIDFromContext(ctx))
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list log drains")
	}
	return &ListLogDrainsOutput{Body: drains}, nil
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
	return &GetLogDrainOutput{Body: drain}, nil
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
	return &UpdateLogDrainOutput{Body: drain}, nil
}

type DeleteLogDrainInput struct {
	DrainID string `path:"drainID"`
}

func (s *Server) handleDeleteLogDrain(ctx context.Context, input *DeleteLogDrainInput) (*struct{}, error) {
	if err := s.store.DeleteLogDrain(ctx, input.DrainID, projectIDFromContext(ctx)); err != nil {
		if errors.Is(err, store.ErrLogDrainNotFound) {
			return nil, huma.Error404NotFound("log drain not found")
		}
		return nil, huma.Error500InternalServerError("failed to delete log drain")
	}
	return nil, nil
}
