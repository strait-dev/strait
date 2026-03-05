package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"orchestrator/internal/domain"
	"orchestrator/internal/store"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
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

	if s.config.FFPayloadValidation {
		if err := validatePayloadAgainstSchema(req.Payload, job.PayloadSchema); err != nil {
			respondError(w, http.StatusBadRequest, "payload validation failed: "+err.Error())
			return
		}
	}

	payload, payloadHash, err := canonicalizePayload(req.Payload)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid payload: "+err.Error())
		return
	}

	var projectQuota *store.ProjectQuota
	if s.config.FFProjectQuotas || s.config.FFExecutionWindows {
		projectQuota, err = s.store.GetProjectQuota(r.Context(), job.ProjectID)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to load project quota")
			return
		}
	}

	if s.config.FFProjectQuotas && projectQuota != nil {
		if projectQuota.MaxQueuedRuns > 0 {
			queuedRuns, countErr := s.store.CountProjectQueuedRuns(r.Context(), job.ProjectID)
			if countErr != nil {
				respondError(w, http.StatusInternalServerError, "failed to evaluate project queued quota")
				return
			}
			if queuedRuns >= projectQuota.MaxQueuedRuns {
				respondError(w, http.StatusTooManyRequests, "project queued quota exceeded")
				return
			}
		}

		if projectQuota.MaxExecutingRuns > 0 {
			activeRuns, countErr := s.store.CountProjectActiveRuns(r.Context(), job.ProjectID)
			if countErr != nil {
				respondError(w, http.StatusInternalServerError, "failed to evaluate project active quota")
				return
			}
			if activeRuns >= projectQuota.MaxExecutingRuns {
				respondError(w, http.StatusTooManyRequests, "project executing quota exceeded")
				return
			}
		}
	}

	if job.RateLimitMax > 0 && job.RateLimitWindowSecs > 0 {
		since := time.Now().Add(-time.Duration(job.RateLimitWindowSecs) * time.Second)
		runCount, countErr := s.store.CountRunsForJobSince(r.Context(), job.ID, since)
		if countErr != nil {
			respondError(w, http.StatusInternalServerError, "failed to evaluate job rate limit")
			return
		}
		if runCount >= job.RateLimitMax {
			respondError(w, http.StatusTooManyRequests, "job rate limit exceeded")
			return
		}
	}

	idempotencyKey := r.Header.Get("X-Idempotency-Key")
	if idempotencyKey == "" {
		idempotencyKey = r.Header.Get("Idempotency-Key")
	}
	if idempotencyKey != "" {
		existingRun, err := s.store.GetRunByIdempotencyKey(r.Context(), job.ID, idempotencyKey)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to check idempotency key")
			return
		}
		if existingRun != nil {
			respondJSON(w, http.StatusCreated, map[string]any{
				"id":     existingRun.ID,
				"status": existingRun.Status,
			})
			return
		}
	}

	if job.DedupWindowSecs > 0 {
		since := time.Now().Add(-time.Duration(job.DedupWindowSecs) * time.Second)
		existingRun, findErr := s.store.FindRecentRunByPayload(r.Context(), job.ID, payload, since)
		if findErr != nil {
			respondError(w, http.StatusInternalServerError, "failed to evaluate payload deduplication")
			return
		}
		if existingRun != nil {
			respondJSON(w, http.StatusCreated, map[string]any{
				"id":           existingRun.ID,
				"status":       existingRun.Status,
				"payload_hash": payloadHash,
			})
			return
		}
	}

	runID := uuid.Must(uuid.NewV7()).String()
	now := time.Now()
	scheduledAt := req.ScheduledAt
	if s.config.FFExecutionWindows && job.ExecutionWindowCron != "" {
		timezone := job.Timezone
		if timezone == "" && projectQuota != nil {
			timezone = projectQuota.Timezone
		}
		adjustedScheduledAt, adjustErr := alignToExecutionWindow(scheduledAt, now, job.ExecutionWindowCron, timezone)
		if adjustErr != nil {
			respondError(w, http.StatusBadRequest, "execution window validation failed: "+adjustErr.Error())
			return
		}
		scheduledAt = adjustedScheduledAt
	}

	var expiresAt time.Time
	if job.RunTTLSecs > 0 {
		expiresAt = now.Add(time.Duration(job.RunTTLSecs) * time.Second)
	} else {
		expiresAt = now.Add(time.Duration(job.TimeoutSecs)*time.Second + 60*time.Second)
	}

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
	if scheduledAt != nil && scheduledAt.After(now) {
		status = domain.StatusDelayed
	}

	run := &domain.JobRun{
		ID:             runID,
		JobID:          job.ID,
		ProjectID:      job.ProjectID,
		Status:         status,
		Attempt:        1,
		Payload:        payload,
		TriggeredBy:    domain.TriggerManual,
		ScheduledAt:    scheduledAt,
		Priority:       req.Priority,
		IdempotencyKey: idempotencyKey,
		JobVersion:     job.Version,
		ExpiresAt:      &expiresAt,
	}

	if err := s.queue.Enqueue(r.Context(), run); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to enqueue run")
		return
	}

	respondJSON(w, http.StatusCreated, map[string]any{
		"id":           run.ID,
		"status":       run.Status,
		"payload_hash": payloadHash,
		"run_token":    tokenString,
	})
}

func canonicalizePayload(payload json.RawMessage) (json.RawMessage, string, error) {
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}

	var v any
	if err := json.Unmarshal(payload, &v); err != nil {
		return nil, "", err
	}

	canonical, err := json.Marshal(v)
	if err != nil {
		return nil, "", err
	}

	hash := sha256.Sum256(canonical)
	return canonical, hex.EncodeToString(hash[:]), nil
}

func alignToExecutionWindow(requested *time.Time, now time.Time, expr, tz string) (*time.Time, error) {
	if tz == "" {
		tz = "UTC"
	}

	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, err
	}

	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	schedule, err := parser.Parse(expr)
	if err != nil {
		return nil, err
	}

	reference := now
	if requested != nil && requested.After(reference) {
		reference = *requested
	}
	referenceLocal := reference.In(loc)

	if cronMatchesInstant(schedule, referenceLocal) {
		if requested != nil {
			ts := requested.UTC()
			return &ts, nil
		}
		return nil, nil
	}

	next := schedule.Next(referenceLocal)
	nextUTC := next.UTC()
	return &nextUTC, nil
}

func cronMatchesInstant(schedule cron.Schedule, ts time.Time) bool {
	truncated := ts.Truncate(time.Minute)
	previousMinute := truncated.Add(-time.Minute)
	return schedule.Next(previousMinute).Equal(truncated)
}
