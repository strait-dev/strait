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

	"orchestrator/internal/domain"
	"orchestrator/internal/store"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type contextKey string

const ctxRunIDKey contextKey = "run_id"
const ctxSDKVersionKey contextKey = "sdk_version"
const ctxSDKCapabilitiesKey contextKey = "sdk_capabilities"

type SDKCapabilities struct {
	Progress    bool
	Checkpoint  bool
	UsageReport bool
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

func (s *Server) runTokenAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
			respondError(w, r, http.StatusUnauthorized, "missing or invalid authorization header")
			return
		}

		tokenString := strings.TrimPrefix(auth, "Bearer ")

		claims := &jwt.RegisteredClaims{}
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
		ctx = context.WithValue(ctx, ctxSDKVersionKey, sdkVersion)
		ctx = context.WithValue(ctx, ctxSDKCapabilitiesKey, sdkCaps)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) handleSDKLog(w http.ResponseWriter, r *http.Request) {
	applySDKResponseHeaders(r.Context(), w)
	runID := chi.URLParam(r, "runID")

	var req struct {
		Type    string          `json:"type,omitempty"`
		Level   string          `json:"level,omitempty"`
		Message string          `json:"message" validate:"required"`
		Data    json.RawMessage `json:"data,omitempty"`
	}
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if !s.validateRequest(w, r, &req) {
		return
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

	event := &domain.RunEvent{
		RunID:   runID,
		Type:    eventType,
		Level:   req.Level,
		Message: req.Message,
		Data:    data,
	}

	if err := s.store.InsertEvent(r.Context(), event); err != nil {
		slog.Error("failed to insert event", "run_id", runID, "error", err)
		respondError(w, r, http.StatusInternalServerError, "failed to insert event")
		return
	}

	if s.pubsub != nil {
		payload, _ := json.Marshal(map[string]any{
			"type":       "event",
			"event_type": string(eventType),
			"run_id":     runID,
			"level":      req.Level,
			"message":    req.Message,
			"data":       data,
			"timestamp":  time.Now().UTC(),
		})
		channel := fmt.Sprintf("run:%s", runID)
		if err := s.pubsub.Publish(r.Context(), channel, payload); err != nil {
			slog.Warn("failed to publish event", "run_id", runID, "error", err)
		}
	}

	respondJSON(w, http.StatusCreated, event)
}

func (s *Server) handleSDKProgress(w http.ResponseWriter, r *http.Request) {
	applySDKResponseHeaders(r.Context(), w)
	runID := chi.URLParam(r, "runID")

	var req struct {
		Percent    float64 `json:"percent" validate:"min=0,max=100"`
		Message    string  `json:"message" validate:"required"`
		Step       string  `json:"step,omitempty"`
		ETASeconds int     `json:"eta_seconds,omitempty"`
	}
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if !s.validateRequest(w, r, &req) {
		return
	}

	dataMap := map[string]any{
		"percent": req.Percent,
	}
	if req.Step != "" {
		dataMap["step"] = req.Step
	}
	if req.ETASeconds > 0 {
		dataMap["eta_seconds"] = req.ETASeconds
	}
	data, err := json.Marshal(dataMap)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to marshal progress payload")
		return
	}

	event := &domain.RunEvent{
		RunID:   runID,
		Type:    domain.EventProgress,
		Level:   "info",
		Message: req.Message,
		Data:    data,
	}

	if err := s.store.InsertEvent(r.Context(), event); err != nil {
		slog.Error("failed to insert progress event", "run_id", runID, "error", err)
		respondError(w, r, http.StatusInternalServerError, "failed to insert event")
		return
	}

	if s.pubsub != nil {
		payload, _ := json.Marshal(map[string]any{
			"type":       "event",
			"event_type": string(domain.EventProgress),
			"run_id":     runID,
			"level":      "info",
			"message":    req.Message,
			"data":       dataMap,
			"timestamp":  time.Now().UTC(),
		})
		channel := fmt.Sprintf("run:%s", runID)
		_ = s.pubsub.Publish(r.Context(), channel, payload)
	}

	respondJSON(w, http.StatusCreated, event)
}

func (s *Server) handleSDKAnnotate(w http.ResponseWriter, r *http.Request) {
	applySDKResponseHeaders(r.Context(), w)
	if !s.config.FFRunAnnotations {
		respondError(w, r, http.StatusNotFound, "run annotations feature is not enabled")
		return
	}
	runID := chi.URLParam(r, "runID")

	var req struct {
		Annotations map[string]string `json:"annotations" validate:"required,min=1"`
	}
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if !s.validateRequest(w, r, &req) {
		return
	}
	if len(req.Annotations) > 50 {
		respondError(w, r, http.StatusBadRequest, "too many annotations (max 50)")
		return
	}

	for key, value := range req.Annotations {
		if strings.TrimSpace(key) == "" {
			respondError(w, r, http.StatusBadRequest, "annotation keys must be non-empty")
			return
		}
		if len(key) > 128 {
			respondError(w, r, http.StatusBadRequest, "annotation key too long (max 128 characters)")
			return
		}
		if len(value) > 1024 {
			respondError(w, r, http.StatusBadRequest, "annotation value too long (max 1024 characters)")
			return
		}
	}

	if err := s.store.UpdateRunMetadata(r.Context(), runID, req.Annotations); err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			respondError(w, r, http.StatusNotFound, "run not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to update run annotations")
		return
	}

	updatedRun, err := s.store.GetRun(r.Context(), runID)
	if err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			respondError(w, r, http.StatusNotFound, "run not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to fetch run")
		return
	}

	respondJSON(w, http.StatusOK, updatedRun)
}

func (s *Server) handleSDKHeartbeat(w http.ResponseWriter, r *http.Request) {
	applySDKResponseHeaders(r.Context(), w)
	runID := chi.URLParam(r, "runID")

	if err := s.store.UpdateHeartbeat(r.Context(), runID); err != nil {
		slog.Error("failed to update heartbeat", "run_id", runID, "error", err)
		respondError(w, r, http.StatusInternalServerError, "failed to update heartbeat")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleSDKCheckpoint(w http.ResponseWriter, r *http.Request) {
	applySDKResponseHeaders(r.Context(), w)
	runID := chi.URLParam(r, "runID")

	var req struct {
		Source string          `json:"source,omitempty"`
		State  json.RawMessage `json:"state" validate:"required"`
	}
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if !s.validateRequest(w, r, &req) {
		return
	}

	run, err := s.store.GetRun(r.Context(), runID)
	if err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			respondError(w, r, http.StatusNotFound, "run not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get run")
		return
	}
	if run.Status != domain.StatusExecuting && run.Status != domain.StatusWaiting {
		respondError(w, r, http.StatusConflict, "run must be executing or waiting to checkpoint")
		return
	}

	source := strings.TrimSpace(req.Source)
	if source == "" {
		source = "sdk"
	}

	checkpoint := &domain.RunCheckpoint{
		ID:     uuid.Must(uuid.NewV7()).String(),
		RunID:  runID,
		Source: source,
		State:  req.State,
	}
	if err := s.store.CreateRunCheckpoint(r.Context(), checkpoint); err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to create checkpoint")
		return
	}

	respondJSON(w, http.StatusCreated, checkpoint)
}
