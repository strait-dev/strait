package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"strait/internal/domain"
	"strait/internal/eventfilter"
	"strait/internal/store"
	"strait/internal/webhook"

	"github.com/danielgtaylor/huma/v2"
)

type CreateEventSourceRequest struct {
	ProjectID          string          `json:"project_id" validate:"required"`
	Name               string          `json:"name" validate:"required,max=255"`
	Description        string          `json:"description,omitempty" validate:"max=2000"`
	Schema             json.RawMessage `json:"schema,omitempty"`
	Enabled            *bool           `json:"enabled,omitempty"`
	SignatureHeader    string          `json:"signature_header,omitempty"`
	SignatureAlgorithm string          `json:"signature_algorithm,omitempty"`
	SignatureSecret    string          `json:"signature_secret,omitempty"`
}

type UpdateEventSourceRequest struct {
	Name               *string          `json:"name,omitempty"`
	Description        *string          `json:"description,omitempty"`
	Schema             *json.RawMessage `json:"schema,omitempty"`
	Enabled            *bool            `json:"enabled,omitempty"`
	SignatureHeader    *string          `json:"signature_header,omitempty"`
	SignatureAlgorithm *string          `json:"signature_algorithm,omitempty"`
	SignatureSecret    *string          `json:"signature_secret,omitempty"`
}

type SubscribeToEventSourceRequest struct {
	TargetType string          `json:"target_type" validate:"required,oneof=job workflow"`
	TargetID   string          `json:"target_id" validate:"required"`
	FilterExpr json.RawMessage `json:"filter_expr,omitempty"`
	Enabled    *bool           `json:"enabled,omitempty"`
}

type DispatchEventRequest struct {
	Source    string          `json:"source" validate:"required"`
	ProjectID string          `json:"project_id" validate:"required"`
	Payload   json.RawMessage `json:"payload"`
}

type CreateEventSourceInput struct {
	Body CreateEventSourceRequest
}

type CreateEventSourceOutput struct {
	Body *domain.EventSource
}

func (s *Server) handleCreateEventSource(ctx context.Context, input *CreateEventSourceInput) (*CreateEventSourceOutput, error) {
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	src := &domain.EventSource{
		ProjectID: req.ProjectID, Name: req.Name, Description: req.Description,
		Schema: req.Schema, Enabled: enabled,
		SignatureHeader: req.SignatureHeader, SignatureAlgorithm: req.SignatureAlgorithm,
	}
	if req.SignatureSecret != "" && s.encryptor != nil {
		enc, encErr := s.encryptor.Encrypt([]byte(req.SignatureSecret))
		if encErr != nil {
			return nil, huma.Error500InternalServerError("failed to encrypt signature secret")
		}
		src.SignatureSecretEnc = enc
	}
	if err := s.store.CreateEventSource(ctx, src); err != nil {
		return nil, huma.Error500InternalServerError("failed to create event source")
	}
	return &CreateEventSourceOutput{Body: src}, nil
}

type ListEventSourcesInput struct{}
type ListEventSourcesOutput struct {
	Body []domain.EventSource
}

func (s *Server) handleListEventSources(ctx context.Context, _ *ListEventSourcesInput) (*ListEventSourcesOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	sources, err := s.store.ListEventSources(ctx, projectID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list event sources")
	}
	return &ListEventSourcesOutput{Body: sources}, nil
}

type GetEventSourceInput struct {
	SourceID string `path:"sourceID"`
}
type GetEventSourceOutput struct {
	Body *domain.EventSource
}

func (s *Server) handleGetEventSource(ctx context.Context, input *GetEventSourceInput) (*GetEventSourceOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	src, err := s.store.GetEventSource(ctx, input.SourceID, projectID)
	if err != nil {
		if errors.Is(err, store.ErrEventSourceNotFound) {
			return nil, huma.Error404NotFound("event source not found")
		}
		return nil, huma.Error500InternalServerError("failed to get event source")
	}
	return &GetEventSourceOutput{Body: src}, nil
}

type UpdateEventSourceInput struct {
	SourceID string `path:"sourceID"`
	Body     UpdateEventSourceRequest
}

func (s *Server) handleUpdateEventSource(ctx context.Context, input *UpdateEventSourceInput) (*struct{}, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	req := input.Body
	patch := make(map[string]any)
	if req.Name != nil {
		patch["name"] = *req.Name
	}
	if req.Description != nil {
		patch["description"] = *req.Description
	}
	if req.Schema != nil {
		patch["schema"] = *req.Schema
	}
	if req.Enabled != nil {
		patch["enabled"] = *req.Enabled
	}
	if req.SignatureHeader != nil {
		patch["signature_header"] = *req.SignatureHeader
	}
	if req.SignatureAlgorithm != nil {
		patch["signature_algorithm"] = *req.SignatureAlgorithm
	}
	if req.SignatureSecret != nil && s.encryptor != nil {
		if *req.SignatureSecret == "" {
			patch["signature_secret_enc"] = nil
		} else {
			enc, encErr := s.encryptor.Encrypt([]byte(*req.SignatureSecret))
			if encErr != nil {
				return nil, huma.Error500InternalServerError("failed to encrypt signature secret")
			}
			patch["signature_secret_enc"] = enc
		}
	}
	if len(patch) == 0 {
		return nil, huma.Error400BadRequest("no fields to update")
	}
	if err := s.store.UpdateEventSource(ctx, input.SourceID, projectID, patch); err != nil {
		if errors.Is(err, store.ErrEventSourceNotFound) {
			return nil, huma.Error404NotFound("event source not found")
		}
		return nil, huma.Error500InternalServerError("failed to update event source")
	}
	return nil, nil
}

type DeleteEventSourceInput struct {
	SourceID string `path:"sourceID"`
}

func (s *Server) handleDeleteEventSource(ctx context.Context, input *DeleteEventSourceInput) (*struct{}, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	if err := s.store.DeleteEventSource(ctx, input.SourceID, projectID); err != nil {
		if errors.Is(err, store.ErrEventSourceNotFound) {
			return nil, huma.Error404NotFound("event source not found")
		}
		return nil, huma.Error500InternalServerError("failed to delete event source")
	}
	return nil, nil
}

type SubscribeToEventSourceInput struct {
	SourceID string `path:"sourceID"`
	Body     SubscribeToEventSourceRequest
}
type SubscribeToEventSourceOutput struct {
	Body *domain.EventSubscription
}

func (s *Server) handleSubscribeToEventSource(ctx context.Context, input *SubscribeToEventSourceInput) (*SubscribeToEventSourceOutput, error) {
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	sub := &domain.EventSubscription{
		SourceID: input.SourceID, TargetType: req.TargetType,
		TargetID: req.TargetID, FilterExpr: req.FilterExpr, Enabled: enabled,
	}
	if err := s.store.CreateEventSubscription(ctx, sub); err != nil {
		return nil, huma.Error500InternalServerError("failed to create event subscription")
	}
	return &SubscribeToEventSourceOutput{Body: sub}, nil
}

type ListEventSourceSubscriptionsInput struct {
	SourceID string `path:"sourceID"`
}
type ListEventSourceSubscriptionsOutput struct {
	Body []domain.EventSubscription
}

func (s *Server) handleListEventSourceSubscriptions(ctx context.Context, input *ListEventSourceSubscriptionsInput) (*ListEventSourceSubscriptionsOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID != "" {
		if _, err := s.store.GetEventSource(ctx, input.SourceID, projectID); err != nil {
			if errors.Is(err, store.ErrEventSourceNotFound) {
				return nil, huma.Error404NotFound("event source not found")
			}
			return nil, huma.Error500InternalServerError("failed to get event source")
		}
	}
	subs, err := s.store.ListEventSubscriptionsBySource(ctx, input.SourceID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list subscriptions")
	}
	return &ListEventSourceSubscriptionsOutput{Body: subs}, nil
}

type DeleteEventSubscriptionInput struct {
	SourceID string `path:"sourceID"`
	SubID    string `path:"subID"`
}

func (s *Server) handleDeleteEventSubscription(ctx context.Context, input *DeleteEventSubscriptionInput) (*struct{}, error) {
	projectID := projectIDFromContext(ctx)
	if projectID != "" {
		if _, err := s.store.GetEventSource(ctx, input.SourceID, projectID); err != nil {
			if errors.Is(err, store.ErrEventSourceNotFound) {
				return nil, huma.Error404NotFound("event source not found")
			}
			return nil, huma.Error500InternalServerError("failed to get event source")
		}
	}
	if err := s.store.DeleteEventSubscription(ctx, input.SubID); err != nil {
		if errors.Is(err, store.ErrEventSubscriptionNotFound) {
			return nil, huma.Error404NotFound("event subscription not found")
		}
		return nil, huma.Error500InternalServerError("failed to delete event subscription")
	}
	return nil, nil
}

type DispatchEventInput struct {
	Body DispatchEventRequest
}

type DispatchEventOutput struct {
	Body map[string]any
}

func (s *Server) handleDispatchEvent(ctx context.Context, input *DispatchEventInput) (*DispatchEventOutput, error) {
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}
	source, err := s.store.GetEventSourceByName(ctx, req.ProjectID, req.Source)
	if err != nil {
		return nil, huma.Error404NotFound("event source not found")
	}
	if !source.Enabled {
		return nil, huma.Error400BadRequest("event source is disabled")
	}
	if source.SignatureAlgorithm != "" && len(source.SignatureSecretEnc) > 0 && s.encryptor != nil {
		// Retrieve raw request from context for signature header access.
		r := requestFromContext(ctx)
		if r == nil {
			return nil, huma.Error500InternalServerError("internal error")
		}
		sigHeader := r.Header.Get(source.SignatureHeader)
		if sigHeader == "" {
			return nil, huma.Error401Unauthorized("missing signature header: " + source.SignatureHeader)
		}
		secret, decErr := s.encryptor.Decrypt(source.SignatureSecretEnc)
		if decErr != nil {
			slog.Error("failed to decrypt event source signature secret", "source_id", source.ID, "error", decErr)
			return nil, huma.Error500InternalServerError("signature verification failed")
		}
		if err := webhook.ValidateSignature(source.SignatureAlgorithm, string(secret), req.Payload, sigHeader); err != nil {
			slog.Warn("event source signature validation failed", "source_id", source.ID, "error", err)
			return nil, huma.Error401Unauthorized("signature validation failed")
		}
	}
	subs, err := s.store.ListEventSubscriptionsBySource(ctx, source.ID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list subscriptions")
	}
	dispatched := 0
	for _, sub := range subs {
		if !sub.Enabled {
			continue
		}
		match, filterErr := eventfilter.Eval(sub.FilterExpr, req.Payload)
		if filterErr != nil {
			slog.Error("filter eval failed", "subscription_id", sub.ID, "source_id", source.ID, "project_id", source.ProjectID, "error", filterErr)
			continue
		}
		if !match {
			continue
		}
		switch sub.TargetType {
		case "job":
			job, jobErr := s.store.GetJob(ctx, sub.TargetID)
			if jobErr != nil || !job.Enabled {
				slog.Error("event dispatch: target job not found or disabled", "target_id", sub.TargetID, "subscription_id", sub.ID, "project_id", source.ProjectID)
				continue
			}
			if job.Paused {
				slog.Info("event dispatch: target job is paused, skipping", "target_id", sub.TargetID, "subscription_id", sub.ID, "project_id", source.ProjectID)
				continue
			}
			run := &domain.JobRun{
				JobID: sub.TargetID, ProjectID: source.ProjectID, Attempt: 1,
				Payload: req.Payload, TriggeredBy: "event",
				JobVersion: job.Version, JobVersionID: job.VersionID,
			}
			if enqErr := s.queue.Enqueue(ctx, run); enqErr != nil {
				slog.Error("event dispatch: enqueue failed", "target_id", sub.TargetID, "subscription_id", sub.ID, "project_id", source.ProjectID, "error", enqErr)
				continue
			}
			dispatched++
		case "workflow":
			if s.workflowEngine != nil {
				_, wfErr := s.workflowEngine.TriggerWorkflow(ctx, sub.TargetID, source.ProjectID, req.Payload, "event", nil, nil)
				if wfErr != nil {
					slog.Error("event dispatch: workflow trigger failed", "target_id", sub.TargetID, "subscription_id", sub.ID, "project_id", source.ProjectID, "error", wfErr)
					continue
				}
				dispatched++
			}
		}
	}
	return &DispatchEventOutput{Body: map[string]any{"dispatched": dispatched, "source": req.Source}}, nil
}
