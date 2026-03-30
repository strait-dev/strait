package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type contextKey string

const ctxRunIDKey contextKey = "run_id"
const ctxAgentIDKey contextKey = "agent_id"
const ctxSDKVersionKey contextKey = "sdk_version"
const ctxSDKCapabilitiesKey contextKey = "sdk_capabilities"

type SDKCapabilities struct {
	Progress    bool
	Checkpoint  bool
	UsageReport bool
}

func agentIDFromTokenContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxAgentIDKey).(string)
	return v
}

func sdkVersionFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxSDKVersionKey).(string)
	return v
}

func sdkCapabilitiesFromContext(ctx context.Context) SDKCapabilities {
	v, ok := ctx.Value(ctxSDKCapabilitiesKey).(SDKCapabilities)
	if !ok {
		return SDKCapabilities{}
	}
	return v
}

func sdkCapabilitiesHeader(c SDKCapabilities) string {
	parts := make([]string, 0, 3)
	if c.Progress {
		parts = append(parts, "progress")
	}
	if c.Checkpoint {
		parts = append(parts, "checkpoint")
	}
	if c.UsageReport {
		parts = append(parts, "usage")
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ",")
}

func resolveSDKCapabilities(version string) SDKCapabilities {
	version = strings.TrimSpace(version)
	if version == "" {
		return SDKCapabilities{}
	}
	majorRaw := version
	if idx := strings.Index(majorRaw, "."); idx >= 0 {
		majorRaw = majorRaw[:idx]
	}
	major, err := strconv.Atoi(majorRaw)
	if err != nil || major < 2 {
		return SDKCapabilities{}
	}
	return SDKCapabilities{Progress: true, Checkpoint: true, UsageReport: true}
}

func applySDKResponseHeaders(ctx context.Context, w http.ResponseWriter) {
	version := sdkVersionFromContext(ctx)
	if version == "" {
		version = "legacy"
	}
	w.Header().Set("X-SDK-Version-Accepted", version)
	w.Header().Set("X-SDK-Capabilities", sdkCapabilitiesHeader(sdkCapabilitiesFromContext(ctx)))
}

func (s *Server) sdkResponseHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		applySDKResponseHeaders(r.Context(), w)
		next.ServeHTTP(w, r)
	})
}

// runTokenClaims mirrors the claims emitted by agents.generateRunToken.
// The AgentID field binds the token to a specific agent, preventing a
// compromised runtime from impersonating other agents in the project.
type runTokenClaims struct {
	jwt.RegisteredClaims
	AgentID string `json:"agent_id,omitempty"`
}

func (s *Server) runTokenAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
			respondError(w, r, http.StatusUnauthorized, "missing or invalid authorization header")
			return
		}
		tokenString := strings.TrimPrefix(auth, "Bearer ")
		claims := &runTokenClaims{}
		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (any, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(s.config.JWTSigningKey), nil
		})
		if err != nil || !token.Valid {
			respondError(w, r, http.StatusUnauthorized, "invalid run token")
			return
		}
		subject := claims.Subject
		if subject == "" {
			respondError(w, r, http.StatusUnauthorized, "missing run ID in token")
			return
		}
		urlRunID := chi.URLParam(r, "runID")
		if urlRunID != "" && subject != urlRunID {
			respondError(w, r, http.StatusForbidden, "token does not match run ID")
			return
		}
		sdkVersion := strings.TrimSpace(r.Header.Get("X-SDK-Version"))
		sdkCaps := resolveSDKCapabilities(sdkVersion)
		ctx := context.WithValue(r.Context(), ctxRunIDKey, subject)
		if claims.AgentID != "" {
			ctx = context.WithValue(ctx, ctxAgentIDKey, claims.AgentID)
		}
		ctx = context.WithValue(ctx, ctxSDKVersionKey, sdkVersion)
		ctx = context.WithValue(ctx, ctxSDKCapabilitiesKey, sdkCaps)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type SDKRunIDInput struct {
	RunID string `path:"runID"`
}

type SDKGetPayloadOutput struct{ Body any }

func (s *Server) handleSDKGetPayload(ctx context.Context, input *SDKRunIDInput) (*SDKGetPayloadOutput, error) {
	run, err := s.store.GetRun(ctx, input.RunID)
	if err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			return nil, huma.Error404NotFound("run not found")
		}
		return nil, huma.Error500InternalServerError("failed to get run")
	}
	if len(run.Payload) == 0 {
		return &SDKGetPayloadOutput{Body: nil}, nil
	}
	var payload any
	if err := json.Unmarshal(run.Payload, &payload); err != nil {
		return nil, huma.Error500InternalServerError("failed to parse payload")
	}
	return &SDKGetPayloadOutput{Body: payload}, nil
}

type SDKLogRequest struct {
	Type    string          `json:"type,omitempty"`
	Level   string          `json:"level,omitempty"`
	Message string          `json:"message" validate:"required"`
	Data    json.RawMessage `json:"data,omitempty"`
}
type SDKLogInput struct {
	RunID string `path:"runID"`
	Body  SDKLogRequest
}
type SDKLogOutput struct{ Body *domain.RunEvent }

func (s *Server) handleSDKLog(ctx context.Context, input *SDKLogInput) (*SDKLogOutput, error) {
	runID := input.RunID
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}
	eventType := domain.EventType(req.Type)
	if eventType == "" {
		eventType = domain.EventLog
	}
	if req.Level == "" {
		req.Level = "info"
	}
	data := req.Data
	if len(data) == 0 {
		data = json.RawMessage(`{}`)
	}
	event := &domain.RunEvent{RunID: runID, Type: eventType, Level: req.Level, Message: req.Message, Data: data}
	if err := s.store.InsertEvent(ctx, event); err != nil {
		slog.Error("failed to insert event", "run_id", runID, "error", err)
		return nil, huma.Error500InternalServerError("failed to insert event")
	}
	if s.pubsub != nil {
		payload, err := json.Marshal(map[string]any{"type": "event", "event_type": string(eventType), "run_id": runID, "level": req.Level, "message": req.Message, "data": data, "timestamp": time.Now().UTC()})
		if err != nil {
			slog.Warn("failed to marshal event payload", "run_id", runID, "error", err)
		} else {
			if err := s.pubsub.Publish(ctx, fmt.Sprintf("run:%s", runID), payload); err != nil {
				slog.Warn("failed to publish event", "run_id", runID, "error", err)
			}
		}
	}
	return &SDKLogOutput{Body: event}, nil
}

type SDKProgressRequest struct {
	Percent    float64 `json:"percent" validate:"min=0,max=100"`
	Message    string  `json:"message" validate:"required"`
	Step       string  `json:"step,omitempty"`
	ETASeconds int     `json:"eta_seconds,omitempty"`
}
type SDKProgressInput struct {
	RunID string `path:"runID"`
	Body  SDKProgressRequest
}
type SDKProgressOutput struct{ Body *domain.RunEvent }

func (s *Server) handleSDKProgress(ctx context.Context, input *SDKProgressInput) (*SDKProgressOutput, error) {
	runID := input.RunID
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}
	dataMap := map[string]any{"percent": req.Percent}
	if req.Step != "" {
		dataMap["step"] = req.Step
	}
	if req.ETASeconds > 0 {
		dataMap["eta_seconds"] = req.ETASeconds
	}
	data, err := json.Marshal(dataMap)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to marshal progress payload")
	}
	event := &domain.RunEvent{RunID: runID, Type: domain.EventProgress, Level: "info", Message: req.Message, Data: data}
	if err := s.store.InsertEvent(ctx, event); err != nil {
		slog.Error("failed to insert progress event", "run_id", runID, "error", err)
		return nil, huma.Error500InternalServerError("failed to insert event")
	}
	if s.pubsub != nil {
		payload, err := json.Marshal(map[string]any{"type": "event", "event_type": string(domain.EventProgress), "run_id": runID, "level": "info", "message": req.Message, "data": dataMap, "timestamp": time.Now().UTC()})
		if err != nil {
			slog.Warn("failed to marshal progress payload", "run_id", runID, "error", err)
		} else {
			if err := s.pubsub.Publish(ctx, fmt.Sprintf("run:%s", runID), payload); err != nil {
				slog.Warn("failed to publish progress event", "run_id", runID, "error", err)
			}
		}
	}
	return &SDKProgressOutput{Body: event}, nil
}

type SDKAnnotateRequest struct {
	Annotations map[string]string `json:"annotations" validate:"required,min=1"`
}
type SDKAnnotateInput struct {
	RunID string `path:"runID"`
	Body  SDKAnnotateRequest
}
type SDKAnnotateOutput struct{ Body *domain.JobRun }

func (s *Server) handleSDKAnnotate(ctx context.Context, input *SDKAnnotateInput) (*SDKAnnotateOutput, error) {
	runID := input.RunID
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}
	if len(req.Annotations) > 50 {
		return nil, huma.Error400BadRequest("too many annotations (max 50)")
	}
	for key, value := range req.Annotations {
		if strings.TrimSpace(key) == "" {
			return nil, huma.Error400BadRequest("annotation keys must be non-empty")
		}
		if len(key) > 128 {
			return nil, huma.Error400BadRequest("annotation key too long (max 128 characters)")
		}
		if len(value) > 1024 {
			return nil, huma.Error400BadRequest("annotation value too long (max 1024 characters)")
		}
	}
	if err := s.store.UpdateRunMetadata(ctx, runID, req.Annotations); err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			return nil, huma.Error404NotFound("run not found")
		}
		return nil, huma.Error500InternalServerError("failed to update run annotations")
	}
	updatedRun, err := s.store.GetRun(ctx, runID)
	if err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			return nil, huma.Error404NotFound("run not found")
		}
		return nil, huma.Error500InternalServerError("failed to fetch run")
	}
	return &SDKAnnotateOutput{Body: updatedRun}, nil
}

type SDKHeartbeatOutput struct{ Body map[string]string }

func (s *Server) handleSDKHeartbeat(ctx context.Context, input *SDKRunIDInput) (*SDKHeartbeatOutput, error) {
	if err := s.store.UpdateHeartbeat(ctx, input.RunID); err != nil {
		slog.Error("failed to update heartbeat", "run_id", input.RunID, "error", err)
		return nil, huma.Error500InternalServerError("failed to update heartbeat")
	}
	return &SDKHeartbeatOutput{Body: map[string]string{"status": "ok"}}, nil
}

type SDKCheckpointRequest struct {
	Source string          `json:"source,omitempty"`
	State  json.RawMessage `json:"state" validate:"required"`
}
type SDKCheckpointInput struct {
	RunID string `path:"runID"`
	Body  SDKCheckpointRequest
}
type SDKCheckpointOutput struct{ Body *domain.RunCheckpoint }

func (s *Server) handleSDKCheckpoint(ctx context.Context, input *SDKCheckpointInput) (*SDKCheckpointOutput, error) {
	runID := input.RunID
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			return nil, huma.Error404NotFound("run not found")
		}
		return nil, huma.Error500InternalServerError("failed to get run")
	}
	if run.Status != domain.StatusExecuting && run.Status != domain.StatusWaiting {
		return nil, huma.Error409Conflict("run must be executing or waiting to checkpoint")
	}
	source := strings.TrimSpace(req.Source)
	if source == "" {
		source = "sdk"
	}
	checkpoint := &domain.RunCheckpoint{ID: uuid.Must(uuid.NewV7()).String(), RunID: runID, Source: source, State: req.State}
	if err := s.store.CreateRunCheckpoint(ctx, checkpoint); err != nil {
		return nil, huma.Error500InternalServerError("failed to create checkpoint")
	}
	return &SDKCheckpointOutput{Body: checkpoint}, nil
}
