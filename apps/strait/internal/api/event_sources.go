package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"strait/internal/domain"
	"strait/internal/eventfilter"
	"strait/internal/store"
	"strait/internal/webhook"

	"github.com/go-chi/chi/v5"
)

type CreateEventSourceRequest struct {
	ProjectID          string          `json:"project_id" validate:"required"`
	Name               string          `json:"name" validate:"required"`
	Description        string          `json:"description,omitempty"`
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

func (s *Server) handleCreateEventSource(w http.ResponseWriter, r *http.Request) {
	var req CreateEventSourceRequest
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if !s.validateRequest(w, r, &req) {
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	src := &domain.EventSource{
		ProjectID:          req.ProjectID,
		Name:               req.Name,
		Description:        req.Description,
		Schema:             req.Schema,
		Enabled:            enabled,
		SignatureHeader:    req.SignatureHeader,
		SignatureAlgorithm: req.SignatureAlgorithm,
	}

	if req.SignatureSecret != "" && s.encryptor != nil {
		enc, encErr := s.encryptor.Encrypt([]byte(req.SignatureSecret))
		if encErr != nil {
			respondError(w, r, http.StatusInternalServerError, "failed to encrypt signature secret")
			return
		}
		src.SignatureSecretEnc = enc
	}

	if err := s.store.CreateEventSource(r.Context(), src); err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to create event source")
		return
	}

	respondJSON(w, http.StatusCreated, src)
}

func (s *Server) handleListEventSources(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		respondError(w, r, http.StatusBadRequest, "project_id is required")
		return
	}

	sources, err := s.store.ListEventSources(r.Context(), projectID)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list event sources")
		return
	}

	respondJSON(w, http.StatusOK, sources)
}

func (s *Server) handleGetEventSource(w http.ResponseWriter, r *http.Request) {
	sourceID := chi.URLParam(r, "sourceID")
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		respondError(w, r, http.StatusBadRequest, "project_id is required")
		return
	}

	src, err := s.store.GetEventSource(r.Context(), sourceID, projectID)
	if err != nil {
		if errors.Is(err, store.ErrEventSourceNotFound) {
			respondError(w, r, http.StatusNotFound, "event source not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get event source")
		return
	}

	respondJSON(w, http.StatusOK, src)
}

func (s *Server) handleUpdateEventSource(w http.ResponseWriter, r *http.Request) {
	sourceID := chi.URLParam(r, "sourceID")
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		respondError(w, r, http.StatusBadRequest, "project_id is required")
		return
	}

	var req UpdateEventSourceRequest
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

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
				respondError(w, r, http.StatusInternalServerError, "failed to encrypt signature secret")
				return
			}
			patch["signature_secret_enc"] = enc
		}
	}

	if len(patch) == 0 {
		respondError(w, r, http.StatusBadRequest, "no fields to update")
		return
	}

	if err := s.store.UpdateEventSource(r.Context(), sourceID, projectID, patch); err != nil {
		if errors.Is(err, store.ErrEventSourceNotFound) {
			respondError(w, r, http.StatusNotFound, "event source not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to update event source")
		return
	}

	respondJSON(w, http.StatusNoContent, nil)
}

func (s *Server) handleDeleteEventSource(w http.ResponseWriter, r *http.Request) {
	sourceID := chi.URLParam(r, "sourceID")
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		respondError(w, r, http.StatusBadRequest, "project_id is required")
		return
	}

	if err := s.store.DeleteEventSource(r.Context(), sourceID, projectID); err != nil {
		if errors.Is(err, store.ErrEventSourceNotFound) {
			respondError(w, r, http.StatusNotFound, "event source not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to delete event source")
		return
	}

	respondJSON(w, http.StatusNoContent, nil)
}

func (s *Server) handleSubscribeToEventSource(w http.ResponseWriter, r *http.Request) {
	sourceID := chi.URLParam(r, "sourceID")

	var req SubscribeToEventSourceRequest
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if !s.validateRequest(w, r, &req) {
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	sub := &domain.EventSubscription{
		SourceID:   sourceID,
		TargetType: req.TargetType,
		TargetID:   req.TargetID,
		FilterExpr: req.FilterExpr,
		Enabled:    enabled,
	}

	if err := s.store.CreateEventSubscription(r.Context(), sub); err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to create event subscription")
		return
	}

	respondJSON(w, http.StatusCreated, sub)
}

func (s *Server) handleListEventSourceSubscriptions(w http.ResponseWriter, r *http.Request) {
	sourceID := chi.URLParam(r, "sourceID")

	subs, err := s.store.ListEventSubscriptionsBySource(r.Context(), sourceID)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list subscriptions")
		return
	}

	respondJSON(w, http.StatusOK, subs)
}

func (s *Server) handleDeleteEventSubscription(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subID")

	if err := s.store.DeleteEventSubscription(r.Context(), subID); err != nil {
		if errors.Is(err, store.ErrEventSubscriptionNotFound) {
			respondError(w, r, http.StatusNotFound, "event subscription not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to delete event subscription")
		return
	}

	respondJSON(w, http.StatusNoContent, nil)
}

func (s *Server) handleDispatchEvent(w http.ResponseWriter, r *http.Request) {
	var req DispatchEventRequest
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if !s.validateRequest(w, r, &req) {
		return
	}

	source, err := s.store.GetEventSourceByName(r.Context(), req.ProjectID, req.Source)
	if err != nil {
		respondError(w, r, http.StatusNotFound, "event source not found")
		return
	}
	if !source.Enabled {
		respondError(w, r, http.StatusBadRequest, "event source is disabled")
		return
	}

	// Validate inbound webhook signature if configured.
	if source.SignatureAlgorithm != "" && len(source.SignatureSecretEnc) > 0 && s.encryptor != nil {
		sigHeader := r.Header.Get(source.SignatureHeader)
		if sigHeader == "" {
			respondError(w, r, http.StatusUnauthorized, "missing signature header: "+source.SignatureHeader)
			return
		}
		secret, decErr := s.encryptor.Decrypt(source.SignatureSecretEnc)
		if decErr != nil {
			slog.Error("failed to decrypt event source signature secret", "source_id", source.ID, "error", decErr)
			respondError(w, r, http.StatusInternalServerError, "signature verification failed")
			return
		}
		if err := webhook.ValidateSignature(source.SignatureAlgorithm, string(secret), req.Payload, sigHeader); err != nil {
			slog.Warn("event source signature validation failed", "source_id", source.ID, "error", err)
			respondError(w, r, http.StatusUnauthorized, "signature validation failed")
			return
		}
	}

	subs, err := s.store.ListEventSubscriptionsBySource(r.Context(), source.ID)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list subscriptions")
		return
	}

	dispatched := 0
	for _, sub := range subs {
		if !sub.Enabled {
			continue
		}

		// Evaluate filter.
		match, err := eventfilter.Eval(sub.FilterExpr, req.Payload)
		if err != nil {
			slog.Error("filter eval failed", "subscription_id", sub.ID, "error", err)
			continue
		}
		if !match {
			continue
		}

		// Dispatch based on target type.
		switch sub.TargetType {
		case "job":
			job, err := s.store.GetJob(r.Context(), sub.TargetID)
			if err != nil || !job.Enabled {
				slog.Error("event dispatch: target job not found or disabled", "target_id", sub.TargetID)
				continue
			}
			run := &domain.JobRun{
				JobID:        sub.TargetID,
				ProjectID:    source.ProjectID,
				Attempt:      1,
				Payload:      req.Payload,
				TriggeredBy:  "event",
				JobVersion:   job.Version,
				JobVersionID: job.VersionID,
			}
			if err := s.queue.Enqueue(r.Context(), run); err != nil {
				slog.Error("event dispatch: enqueue failed", "target_id", sub.TargetID, "error", err)
				continue
			}
			dispatched++
		case "workflow":
			if s.workflowEngine != nil {
				_, err := s.workflowEngine.TriggerWorkflow(r.Context(), sub.TargetID, source.ProjectID, req.Payload, "event", nil, nil)
				if err != nil {
					slog.Error("event dispatch: workflow trigger failed", "target_id", sub.TargetID, "error", err)
					continue
				}
				dispatched++
			}
		}
	}

	respondJSON(w, http.StatusOK, map[string]any{"dispatched": dispatched, "source": req.Source})
}
