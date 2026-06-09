package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"strait/internal/domain"
	"strait/internal/eventfilter"
	"strait/internal/store"
	"strait/internal/webhook"

	"github.com/danielgtaylor/huma/v2"
)

const eventSourceSignatureReplayTTL = 5 * time.Minute

type CreateEventSourceRequest struct {
	ProjectID          string          `json:"project_id" validate:"required"`
	Name               string          `json:"name" validate:"required,max=255"`
	Description        string          `json:"description,omitempty" validate:"max=2000"`
	Schema             json.RawMessage `json:"schema,omitempty"`
	Enabled            *bool           `json:"enabled,omitempty"`
	SignatureHeader    string          `json:"signature_header,omitempty"`
	SignatureAlgorithm string          `json:"signature_algorithm,omitempty" validate:"omitempty,oneof=hmac-sha256 stripe-v1 github-sha256"`
	SignatureSecret    string          `json:"signature_secret,omitempty"`
}

type UpdateEventSourceRequest struct {
	Name               *string          `json:"name,omitempty"`
	Description        *string          `json:"description,omitempty"`
	Schema             *json.RawMessage `json:"schema,omitempty"`
	Enabled            *bool            `json:"enabled,omitempty"`
	SignatureHeader    *string          `json:"signature_header,omitempty"`
	SignatureAlgorithm *string          `json:"signature_algorithm,omitempty" validate:"omitempty,oneof=hmac-sha256 stripe-v1 github-sha256"`
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
	if err := requireProjectMatch(ctx, req.ProjectID); err != nil {
		return nil, huma.Error403Forbidden("project_id does not match authenticated project")
	}
	// A signature header or secret without an algorithm would be silently
	// unverified at dispatch time; reject the misconfiguration up front.
	if (req.SignatureHeader != "" || req.SignatureSecret != "") && req.SignatureAlgorithm == "" {
		return nil, huma.Error400BadRequest("signature_algorithm is required when signature_header or signature_secret is set")
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
	if req.SignatureSecret != "" {
		if s.encryptor == nil {
			return nil, huma.Error500InternalServerError("signature secret encryption is not configured")
		}
		enc, encErr := s.encryptor.Encrypt([]byte(req.SignatureSecret))
		if encErr != nil {
			return nil, huma.Error500InternalServerError("failed to encrypt signature secret")
		}
		src.SignatureSecretEnc = enc
	}
	if err := s.store.CreateEventSource(ctx, src); err != nil {
		return nil, huma.Error500InternalServerError("failed to create event source")
	}
	s.emitAuditEvent(ctx, domain.AuditActionEventSourceCreated, "event_source", src.ID, map[string]any{
		"name":                src.Name,
		"signature_algorithm": src.SignatureAlgorithm,
		"enabled":             src.Enabled,
	})
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
	if req.SignatureSecret != nil {
		if s.encryptor == nil {
			return nil, huma.Error500InternalServerError("signature secret encryption is not configured")
		}
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
	changedFields := make([]string, 0, len(patch))
	for k := range patch {
		if k == "signature_secret_enc" {
			continue
		}
		changedFields = append(changedFields, k)
	}
	s.emitAuditEvent(ctx, domain.AuditActionEventSourceUpdated, "event_source", input.SourceID, map[string]any{
		"changed_fields": changedFields,
		"secret_changed": req.SignatureSecret != nil,
	})
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
	s.emitAuditEvent(ctx, domain.AuditActionEventSourceDeleted, "event_source", input.SourceID, nil)
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
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	if _, err := s.store.GetEventSource(ctx, input.SourceID, projectID); err != nil {
		if errors.Is(err, store.ErrEventSourceNotFound) {
			return nil, huma.Error404NotFound("event source not found")
		}
		return nil, huma.Error500InternalServerError("failed to get event source")
	}
	if err := s.validateEventSubscriptionTarget(ctx, req.TargetType, req.TargetID, projectID); err != nil {
		return nil, err
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
	s.emitAuditEvent(ctx, domain.AuditActionEventSourceSubscribed, "event_source", input.SourceID, map[string]any{
		"subscription_id": sub.ID,
		"target_type":     req.TargetType,
		"target_id":       req.TargetID,
		"enabled":         enabled,
	})
	return &SubscribeToEventSourceOutput{Body: sub}, nil
}

func (s *Server) validateEventSubscriptionTarget(ctx context.Context, targetType, targetID, projectID string) error {
	switch targetType {
	case "job":
		job, err := s.store.GetJob(ctx, targetID)
		if err != nil {
			if errors.Is(err, store.ErrJobNotFound) {
				return huma.Error404NotFound("event subscription target not found")
			}
			return huma.Error500InternalServerError("failed to get event subscription target")
		}
		if job == nil || job.ProjectID != projectID {
			return huma.Error404NotFound("event subscription target not found")
		}
	case "workflow":
		wf, err := s.store.GetWorkflow(ctx, targetID)
		if err != nil {
			if errors.Is(err, store.ErrWorkflowNotFound) {
				return huma.Error404NotFound("event subscription target not found")
			}
			return huma.Error500InternalServerError("failed to get event subscription target")
		}
		if wf == nil || wf.ProjectID != projectID {
			return huma.Error404NotFound("event subscription target not found")
		}
	default:
		return huma.Error400BadRequest("invalid event subscription target type")
	}
	return nil
}

type ListEventSourceSubscriptionsInput struct {
	SourceID string `path:"sourceID"`
}
type ListEventSourceSubscriptionsOutput struct {
	Body []domain.EventSubscription
}

func (s *Server) handleListEventSourceSubscriptions(ctx context.Context, input *ListEventSourceSubscriptionsInput) (*ListEventSourceSubscriptionsOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project context is required -- authenticate with an API key")
	}
	if _, err := s.store.GetEventSource(ctx, input.SourceID, projectID); err != nil {
		if errors.Is(err, store.ErrEventSourceNotFound) {
			return nil, huma.Error404NotFound("event source not found")
		}
		return nil, huma.Error500InternalServerError("failed to get event source")
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
	if projectID == "" {
		return nil, huma.Error400BadRequest("project context is required -- authenticate with an API key")
	}
	if _, err := s.store.GetEventSource(ctx, input.SourceID, projectID); err != nil {
		if errors.Is(err, store.ErrEventSourceNotFound) {
			return nil, huma.Error404NotFound("event source not found")
		}
		return nil, huma.Error500InternalServerError("failed to get event source")
	}
	sub, err := s.store.GetEventSubscription(ctx, input.SubID)
	if err != nil {
		if errors.Is(err, store.ErrEventSubscriptionNotFound) {
			return nil, huma.Error404NotFound("event subscription not found")
		}
		return nil, huma.Error500InternalServerError("failed to get event subscription")
	}
	if sub.SourceID != input.SourceID {
		return nil, huma.Error404NotFound("event subscription not found")
	}
	if err := s.store.DeleteEventSubscription(ctx, input.SubID); err != nil {
		if errors.Is(err, store.ErrEventSubscriptionNotFound) {
			return nil, huma.Error404NotFound("event subscription not found")
		}
		return nil, huma.Error500InternalServerError("failed to delete event subscription")
	}
	s.emitAuditEvent(ctx, domain.AuditActionEventSubscriptionDeleted, "event_subscription", input.SubID, map[string]any{
		"source_id":   input.SourceID,
		"target_type": sub.TargetType,
		"target_id":   sub.TargetID,
	})
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

	source, err := s.loadDispatchEventSource(ctx, req)
	if err != nil {
		return nil, err
	}
	replayKey, err := s.verifyEventSourceSignature(ctx, source, req.Payload)
	if err != nil {
		return nil, err
	}

	subs, err := s.store.ListEventSubscriptionsBySource(ctx, source.ID)
	if err != nil {
		// verifyEventSourceSignature durably claimed a replay key before
		// dispatch. Release it on failure so the caller's retry is not rejected
		// as a replay, which would otherwise silently drop the event.
		s.releaseEventSourceSignatureReplay(ctx, source.ProjectID, replayKey)
		return nil, huma.Error500InternalServerError("failed to list subscriptions")
	}

	dispatched := 0
	for _, sub := range subs {
		if s.dispatchEventSubscription(ctx, source, req, sub) {
			dispatched++
		}
	}

	s.emitAuditEventAsync(ctx, domain.AuditActionEventSourceDispatched, "event_source", source.ID, map[string]any{
		"source_name":  req.Source,
		"dispatched":   dispatched,
		"payload_size": len(req.Payload),
	})

	return &DispatchEventOutput{Body: map[string]any{"dispatched": dispatched, "source": req.Source}}, nil
}

func (s *Server) loadDispatchEventSource(ctx context.Context, req DispatchEventRequest) (*domain.EventSource, error) {
	if err := requireProjectMatch(ctx, req.ProjectID); err != nil {
		return nil, huma.Error404NotFound("event source not found")
	}
	source, err := s.store.GetEventSourceByName(ctx, req.ProjectID, req.Source)
	if err != nil {
		return nil, huma.Error404NotFound("event source not found")
	}
	if !source.Enabled {
		return nil, huma.Error400BadRequest("event source is disabled")
	}
	return source, nil
}

// verifyEventSourceSignature validates the inbound signature and, for replayable
// algorithms, durably claims a replay key. It returns that claimed key (empty
// when no replay guard applies) so the caller can release it if a later dispatch
// step fails.
func (s *Server) verifyEventSourceSignature(ctx context.Context, source *domain.EventSource, payload json.RawMessage) (string, error) {
	if source.SignatureAlgorithm == "" {
		// Fail closed if a header or secret is configured but the algorithm is
		// not: the operator clearly intended signature protection, so silently
		// accepting unverified payloads (the prior behavior) would defeat the
		// origin authentication the signature provides.
		if source.SignatureHeader != "" || len(source.SignatureSecretEnc) > 0 {
			slog.Error("event source has a signature header/secret but no algorithm; refusing unverified payload",
				"source_id", source.ID)
			return "", huma.Error500InternalServerError("signature verification is misconfigured")
		}
		return "", nil
	}
	if source.SignatureHeader == "" || len(source.SignatureSecretEnc) == 0 || s.encryptor == nil {
		slog.Error("event source signature verification is misconfigured",
			"source_id", source.ID,
			"has_header", source.SignatureHeader != "",
			"has_secret", len(source.SignatureSecretEnc) > 0,
			"has_encryptor", s.encryptor != nil,
		)
		return "", huma.Error500InternalServerError("signature verification is not configured")
	}

	r := requestFromContext(ctx)
	if r == nil {
		return "", huma.Error500InternalServerError("internal error")
	}
	sigHeader := r.Header.Get(source.SignatureHeader)
	if sigHeader == "" {
		// Do not echo the configured header name; it identifies the privileged
		// signing scheme to unauthenticated callers.
		return "", huma.Error401Unauthorized("missing signature header")
	}

	secret, err := s.encryptor.Decrypt(source.SignatureSecretEnc)
	if err != nil {
		slog.Error("failed to decrypt event source signature secret", "source_id", source.ID, "error", err)
		return "", huma.Error500InternalServerError("signature verification failed")
	}
	if err := webhook.ValidateSignature(source.SignatureAlgorithm, string(secret), payload, sigHeader); err != nil {
		slog.Warn("event source signature validation failed", "source_id", source.ID, "error", err)
		return "", huma.Error401Unauthorized("signature validation failed")
	}
	return s.claimEventSourceSignatureReplay(ctx, source, sigHeader)
}

// claimEventSourceSignatureReplay durably claims a replay key for the validated
// signature and returns it (empty for non-replayable algorithms).
func (s *Server) claimEventSourceSignatureReplay(ctx context.Context, source *domain.EventSource, sigHeader string) (string, error) {
	switch source.SignatureAlgorithm {
	// stripe-v1 carries a timestamp and is accepted within a +/-300s window, so a
	// captured request is replayable until the TTL covers that window; claim a
	// nonce for it too. hmac-sha256/github-sha256 embed no timestamp, so the nonce
	// is the only replay control (it bounds rapid replay within the TTL; a fully
	// expired nonce can still be replayed — a limitation inherent to timestampless
	// signatures).
	case "hmac-sha256", "github-sha256", "stripe-v1":
	default:
		return "", nil
	}
	key := eventSourceSignatureReplayKey(source.ID, source.SignatureAlgorithm, sigHeader)
	status, _, _, _, err := s.store.TryAcquireIdempotencyKey(
		store.ContextWithoutTx(ctx),
		source.ProjectID,
		key,
		eventSourceSignatureReplayTTL,
	)
	if err != nil {
		slog.Error("event source signature replay guard failed", "source_id", source.ID, "error", err)
		return "", huma.Error503ServiceUnavailable("signature replay guard unavailable")
	}
	if status != store.IdempotencyAcquired {
		slog.Warn("event source signature replay rejected", "source_id", source.ID)
		return "", huma.Error401Unauthorized("signature replay rejected")
	}
	return key, nil
}

// releaseEventSourceSignatureReplay deletes a previously claimed replay key so a
// failed dispatch can be retried. Best-effort: on error the short-lived replay
// key simply lingers until its TTL expires.
func (s *Server) releaseEventSourceSignatureReplay(ctx context.Context, projectID, key string) {
	if key == "" {
		return
	}
	if _, err := s.store.DeleteIdempotencyKey(store.ContextWithoutTx(ctx), projectID, key); err != nil {
		slog.Warn("failed to release event source signature replay key", "project_id", projectID, "error", err)
	}
}

func eventSourceSignatureReplayKey(sourceID, algorithm, sigHeader string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("event-source-signature:%s:%s:%s", sourceID, algorithm, sigHeader)))
	return "event-source-signature:" + hex.EncodeToString(sum[:])
}

func (s *Server) dispatchEventSubscription(
	ctx context.Context,
	source *domain.EventSource,
	req DispatchEventRequest,
	sub domain.EventSubscription,
) bool {
	if !sub.Enabled {
		return false
	}
	match, err := eventfilter.Eval(sub.FilterExpr, req.Payload)
	if err != nil {
		slog.Error("filter eval failed", "subscription_id", sub.ID, "source_id", source.ID, "project_id", source.ProjectID, "error", err)
		return false
	}
	if !match {
		return false
	}

	switch sub.TargetType {
	case "job":
		return s.dispatchEventToJob(ctx, source, req.Payload, sub)
	case "workflow":
		return s.dispatchEventToWorkflow(ctx, source, req.Payload, sub)
	default:
		return false
	}
}

func (s *Server) dispatchEventToJob(
	ctx context.Context,
	source *domain.EventSource,
	payload json.RawMessage,
	sub domain.EventSubscription,
) bool {
	if !s.hasProjectPermission(ctx, domain.ScopeJobsTrigger) {
		slog.Warn("event dispatch: caller lacks job trigger permission", "subscription_id", sub.ID, "project_id", source.ProjectID)
		return false
	}

	job, err := s.store.GetJob(ctx, sub.TargetID)
	if err != nil {
		slog.Error("event dispatch: target job lookup failed", "target_id", sub.TargetID, "subscription_id", sub.ID, "project_id", source.ProjectID, "error", err)
		return false
	}
	if job == nil {
		slog.Error("event dispatch: target job not found", "target_id", sub.TargetID, "subscription_id", sub.ID, "project_id", source.ProjectID)
		return false
	}
	if !job.Enabled {
		slog.Error("event dispatch: target job disabled", "target_id", sub.TargetID, "subscription_id", sub.ID, "project_id", source.ProjectID)
		return false
	}
	if job.ProjectID != source.ProjectID {
		slog.Warn("event dispatch: target job project mismatch", "target_id", sub.TargetID, "subscription_id", sub.ID, "source_project_id", source.ProjectID, "target_project_id", job.ProjectID)
		return false
	}
	if job.Paused {
		slog.Info("event dispatch: target job is paused, skipping", "target_id", sub.TargetID, "subscription_id", sub.ID, "project_id", source.ProjectID)
		return false
	}

	projectQuota, err := s.quotaCache.Get(ctx, job.ProjectID)
	if err != nil {
		slog.Error("event dispatch: quota load failed", "target_id", sub.TargetID, "subscription_id", sub.ID, "project_id", source.ProjectID, "error", err)
		return false
	}
	if err := s.checkTriggerDispatchPriority(ctx, job.ProjectID, 0); err != nil {
		slog.Warn("event dispatch: dispatch priority blocked", "target_id", sub.TargetID, "subscription_id", sub.ID, "project_id", source.ProjectID, "error", err)
		return false
	}
	if err := s.checkTriggerDailyCostBudget(ctx, job.ProjectID, projectQuota); err != nil {
		slog.Warn("event dispatch: daily cost budget blocked", "target_id", sub.TargetID, "subscription_id", sub.ID, "project_id", source.ProjectID, "error", err)
		return false
	}

	run := &domain.JobRun{
		JobID:        sub.TargetID,
		ProjectID:    source.ProjectID,
		Attempt:      1,
		Payload:      payload,
		TriggeredBy:  "event",
		JobVersion:   job.Version,
		JobVersionID: job.VersionID,
	}
	if err := s.withTriggerLimitGuard(ctx, job, projectQuota, func(guardCtx context.Context, tx store.DBTX) error {
		return s.enqueueTriggerRun(guardCtx, tx, run)
	}); err != nil {
		slog.Error("event dispatch: enqueue failed", "target_id", sub.TargetID, "subscription_id", sub.ID, "project_id", source.ProjectID, "error", err)
		return false
	}
	return true
}

func (s *Server) dispatchEventToWorkflow(
	ctx context.Context,
	source *domain.EventSource,
	payload json.RawMessage,
	sub domain.EventSubscription,
) bool {
	if !s.hasProjectPermission(ctx, domain.ScopeWorkflowsTrigger) {
		slog.Warn("event dispatch: caller lacks workflow trigger permission", "subscription_id", sub.ID, "project_id", source.ProjectID)
		return false
	}

	wf, err := s.store.GetWorkflow(ctx, sub.TargetID)
	if err != nil {
		slog.Error("event dispatch: target workflow not found", "target_id", sub.TargetID, "subscription_id", sub.ID, "project_id", source.ProjectID, "error", err)
		return false
	}
	if wf == nil || wf.ProjectID != source.ProjectID {
		targetProjectID := ""
		if wf != nil {
			targetProjectID = wf.ProjectID
		}
		slog.Warn("event dispatch: target workflow project mismatch", "target_id", sub.TargetID, "subscription_id", sub.ID, "source_project_id", source.ProjectID, "target_project_id", targetProjectID)
		return false
	}
	if s.workflowEngine == nil {
		return false
	}
	if _, err := s.workflowEngine.TriggerWorkflow(ctx, sub.TargetID, source.ProjectID, payload, "event", nil, nil); err != nil {
		slog.Error("event dispatch: workflow trigger failed", "target_id", sub.TargetID, "subscription_id", sub.ID, "project_id", source.ProjectID, "error", err)
		return false
	}
	return true
}
