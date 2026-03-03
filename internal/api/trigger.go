package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"orchestrator/internal/domain"
	"orchestrator/internal/store"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type TriggerRequest struct {
	Payload     json.RawMessage `json:"payload,omitempty"`
	ScheduledAt *time.Time      `json:"scheduled_at,omitempty"`
	Priority    int             `json:"priority,omitempty"`
}

func (s *Server) handleTriggerJob(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID")

	job, err := s.store.GetJob(r.Context(), jobID)
	if err != nil {
		if errors.Is(err, store.ErrJobNotFound) {
			respondError(w, http.StatusNotFound, "job not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to get job")
		return
	}

	if !job.Enabled {
		respondError(w, http.StatusBadRequest, "job is disabled")
		return
	}

	var req TriggerRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	runID := uuid.Must(uuid.NewV7()).String()
	now := time.Now()
	expiresAt := now.Add(time.Duration(job.TimeoutSecs)*time.Second + 60*time.Second)

	claims := jwt.RegisteredClaims{
		Subject:   runID,
		ExpiresAt: jwt.NewNumericDate(expiresAt),
		IssuedAt:  jwt.NewNumericDate(now),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(s.config.JWTSigningKey))
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to sign run token")
		return
	}

	status := domain.StatusQueued
	if req.ScheduledAt != nil && req.ScheduledAt.After(now) {
		status = domain.StatusDelayed
	}

	run := &domain.JobRun{
		ID:          runID,
		JobID:       job.ID,
		ProjectID:   job.ProjectID,
		Status:      status,
		Attempt:     1,
		Payload:     req.Payload,
		TriggeredBy: domain.TriggerManual,
		ScheduledAt: req.ScheduledAt,
		Priority:    req.Priority,
		ExpiresAt:   &expiresAt,
	}

	if err := s.queue.Enqueue(r.Context(), run); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to enqueue run")
		return
	}

	respondJSON(w, http.StatusCreated, map[string]any{
		"id":        run.ID,
		"status":    run.Status,
		"run_token": tokenString,
	})
}
