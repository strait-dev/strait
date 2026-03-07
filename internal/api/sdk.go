package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"orchestrator/internal/domain"
	"orchestrator/internal/store"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const ctxRunIDKey contextKey = "run_id"

func (s *Server) runTokenAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
			respondError(w, http.StatusUnauthorized, "missing or invalid authorization header")
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
			respondError(w, http.StatusUnauthorized, "invalid run token")
			return
		}

		subject := claims.Subject
		if subject == "" {
			respondError(w, http.StatusUnauthorized, "missing run ID in token")
			return
		}

		urlRunID := chi.URLParam(r, "runID")
		if urlRunID != "" && subject != urlRunID {
			respondError(w, http.StatusForbidden, "token does not match run ID")
			return
		}

		ctx := context.WithValue(r.Context(), ctxRunIDKey, subject)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) handleSDKLog(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")

	var req struct {
		Type    string          `json:"type"`
		Level   string          `json:"level,omitempty"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data,omitempty"`
	}
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Message == "" {
		respondError(w, http.StatusBadRequest, "message is required")
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
		respondError(w, http.StatusInternalServerError, "failed to insert event")
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

func (s *Server) handleSDKHeartbeat(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")

	if err := s.store.UpdateHeartbeat(r.Context(), runID); err != nil {
		slog.Error("failed to update heartbeat", "run_id", runID, "error", err)
		respondError(w, http.StatusInternalServerError, "failed to update heartbeat")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleSDKComplete(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")

	var req struct {
		Result json.RawMessage `json:"result,omitempty"`
	}
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Fetch current run to validate FSM transition dynamically
	run, err := s.store.GetRun(r.Context(), runID)
	if err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			respondError(w, http.StatusNotFound, "run not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to get run")
		return
	}

	now := time.Now()
	fields := map[string]any{
		"finished_at": now,
	}
	if len(req.Result) > 0 {
		fields["result"] = req.Result
	}

	err = s.store.UpdateRunStatus(r.Context(), runID, run.Status, domain.StatusCompleted, fields)
	if err != nil {
		slog.Error("failed to complete run", "run_id", runID, "error", err)
		if errors.Is(err, store.ErrRunConflict) {
			respondError(w, http.StatusConflict, "run status conflict")
		} else {
			respondError(w, http.StatusInternalServerError, "failed to update run")
		}
		return
	}

	if s.workflowCallback != nil {
		completedRun := *run
		completedRun.Status = domain.StatusCompleted
		if cbErr := s.workflowCallback.OnJobRunTerminal(r.Context(), &completedRun); cbErr != nil {
			slog.Error("workflow callback failed", "run_id", runID, "error", cbErr)
		}
	}

	if s.pubsub != nil {
		payload, _ := json.Marshal(map[string]any{
			"type":      "status_change",
			"run_id":    runID,
			"from":      string(run.Status),
			"to":        "completed",
			"timestamp": now.UTC(),
		})
		channel := fmt.Sprintf("run:%s", runID)
		if err := s.pubsub.Publish(r.Context(), channel, payload); err != nil {
			slog.Warn("failed to publish event", "run_id", runID, "error", err)
		}
	}

	updatedRun, err := s.store.GetRun(r.Context(), runID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to get updated run")
		return
	}
	respondJSON(w, http.StatusOK, updatedRun)
}

func (s *Server) handleSDKFail(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")

	var req struct {
		Error string `json:"error"`
	}
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Error == "" {
		respondError(w, http.StatusBadRequest, "error message is required")
		return
	}

	// Fetch current run to validate FSM transition dynamically
	run, err := s.store.GetRun(r.Context(), runID)
	if err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			respondError(w, http.StatusNotFound, "run not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to get run")
		return
	}

	now := time.Now()
	err = s.store.UpdateRunStatus(r.Context(), runID, run.Status, domain.StatusFailed, map[string]any{
		"finished_at": now,
		"error":       req.Error,
	})
	if err != nil {
		slog.Error("failed to fail run", "run_id", runID, "error", err)
		if errors.Is(err, store.ErrRunConflict) {
			respondError(w, http.StatusConflict, "run status conflict")
		} else {
			respondError(w, http.StatusInternalServerError, "failed to update run")
		}
		return
	}

	if s.workflowCallback != nil {
		failedRun := *run
		failedRun.Status = domain.StatusFailed
		failedRun.Error = req.Error
		if cbErr := s.workflowCallback.OnJobRunTerminal(r.Context(), &failedRun); cbErr != nil {
			slog.Error("workflow callback failed", "run_id", runID, "error", cbErr)
		}
	}

	if s.pubsub != nil {
		payload, _ := json.Marshal(map[string]any{
			"type":      "status_change",
			"run_id":    runID,
			"from":      string(run.Status),
			"to":        "failed",
			"error":     req.Error,
			"timestamp": now.UTC(),
		})
		channel := fmt.Sprintf("run:%s", runID)
		if err := s.pubsub.Publish(r.Context(), channel, payload); err != nil {
			slog.Warn("failed to publish event", "run_id", runID, "error", err)
		}
	}

	updatedRun, err := s.store.GetRun(r.Context(), runID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to get updated run")
		return
	}
	respondJSON(w, http.StatusOK, updatedRun)
}

func (s *Server) handleSDKSpawn(w http.ResponseWriter, r *http.Request) {
	parentRunID := chi.URLParam(r, "runID")

	var req struct {
		JobSlug   string          `json:"job_slug"`
		ProjectID string          `json:"project_id"`
		Payload   json.RawMessage `json:"payload,omitempty"`
	}
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.JobSlug == "" || req.ProjectID == "" {
		respondError(w, http.StatusBadRequest, "job_slug and project_id are required")
		return
	}

	job, err := s.store.GetJobBySlug(r.Context(), req.ProjectID, req.JobSlug)
	if err != nil || job == nil {
		respondError(w, http.StatusNotFound, "job not found")
		return
	}

	run := &domain.JobRun{
		JobID:       job.ID,
		ProjectID:   job.ProjectID,
		Payload:     req.Payload,
		TriggeredBy: domain.TriggerSpawn,
		ParentRunID: parentRunID,
	}

	if err := s.queue.Enqueue(r.Context(), run); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to enqueue child run")
		return
	}

	respondJSON(w, http.StatusCreated, run)
}
