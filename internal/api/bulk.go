package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"orchestrator/internal/domain"
	"orchestrator/internal/store"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type BulkTriggerRequest struct {
	Items []BulkTriggerItem `json:"items"`
}

type BulkTriggerItem struct {
	Payload     json.RawMessage `json:"payload,omitempty"`
	ScheduledAt *time.Time      `json:"scheduled_at,omitempty"`
	Priority    int             `json:"priority,omitempty"`
}

type BulkTriggerResult struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	RunToken string `json:"run_token"`
}

type BulkTriggerResponse struct {
	Results []BulkTriggerResult `json:"results"`
	Total   int                 `json:"total"`
	Created int                 `json:"created"`
}

type BulkCancelRequest struct {
	RunIDs []string `json:"run_ids"`
}

type BulkCancelResult struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type BulkCancelResponse struct {
	Results  []BulkCancelResult `json:"results"`
	Total    int                `json:"total"`
	Canceled int                `json:"canceled"`
	Failed   int                `json:"failed"`
}

func (s *Server) handleBulkTriggerJob(w http.ResponseWriter, r *http.Request) {
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

	var req BulkTriggerRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(req.Items) == 0 {
		respondError(w, http.StatusBadRequest, "items array is required and must not be empty")
		return
	}

	if len(req.Items) > 100 {
		respondError(w, http.StatusBadRequest, "maximum 100 items per bulk trigger request")
		return
	}

	now := time.Now()
	results := make([]BulkTriggerResult, 0, len(req.Items))
	created := 0

	for _, item := range req.Items {
		runID := uuid.Must(uuid.NewV7()).String()

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
			respondError(w, http.StatusInternalServerError, fmt.Sprintf("failed to sign run token for item %d", created))
			return
		}

		status := domain.StatusQueued
		if item.ScheduledAt != nil && item.ScheduledAt.After(now) {
			status = domain.StatusDelayed
		}

		run := &domain.JobRun{
			ID:          runID,
			JobID:       job.ID,
			ProjectID:   job.ProjectID,
			Status:      status,
			Attempt:     1,
			Payload:     item.Payload,
			TriggeredBy: domain.TriggerManual,
			ScheduledAt: item.ScheduledAt,
			Priority:    item.Priority,
			ExpiresAt:   &expiresAt,
		}

		if err := s.queue.Enqueue(r.Context(), run); err != nil {
			respondError(w, http.StatusInternalServerError, fmt.Sprintf("failed to enqueue item %d", created))
			return
		}

		results = append(results, BulkTriggerResult{
			ID:       run.ID,
			Status:   string(run.Status),
			RunToken: tokenString,
		})
		created++
	}

	respondJSON(w, http.StatusCreated, BulkTriggerResponse{
		Results: results,
		Total:   len(req.Items),
		Created: created,
	})
}

func (s *Server) handleBulkCancelRuns(w http.ResponseWriter, r *http.Request) {
	var req BulkCancelRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(req.RunIDs) == 0 {
		respondError(w, http.StatusBadRequest, "run_ids array is required and must not be empty")
		return
	}

	if len(req.RunIDs) > 100 {
		respondError(w, http.StatusBadRequest, "maximum 100 run IDs per bulk cancel request")
		return
	}

	results := make([]BulkCancelResult, 0, len(req.RunIDs))
	canceled := 0
	failed := 0

	for _, runID := range req.RunIDs {
		run, err := s.store.GetRun(r.Context(), runID)
		if err != nil {
			results = append(results, BulkCancelResult{
				ID:     runID,
				Status: "failed",
				Error:  "run not found",
			})
			failed++
			continue
		}

		if run.Status.IsTerminal() {
			results = append(results, BulkCancelResult{
				ID:     runID,
				Status: string(run.Status),
				Error:  "run already in terminal state",
			})
			failed++
			continue
		}

		if err := s.store.UpdateRunStatus(r.Context(), run.ID, run.Status, domain.StatusCanceled, map[string]any{
			"finished_at": time.Now(),
			"error":       "canceled by user (bulk)",
		}); err != nil {
			results = append(results, BulkCancelResult{
				ID:     runID,
				Status: string(run.Status),
				Error:  "failed to cancel",
			})
			failed++
			continue
		}

		children, err := s.store.ListChildRuns(r.Context(), run.ID)
		if err == nil {
			for _, child := range children {
				if !child.Status.IsTerminal() {
					_ = s.store.UpdateRunStatus(r.Context(), child.ID, child.Status, domain.StatusCanceled, map[string]any{
						"finished_at": time.Now(),
						"error":       "parent run canceled (bulk)",
					})
				}
			}
		}

		results = append(results, BulkCancelResult{
			ID:     runID,
			Status: string(domain.StatusCanceled),
		})
		canceled++
	}

	respondJSON(w, http.StatusOK, BulkCancelResponse{
		Results:  results,
		Total:    len(req.RunIDs),
		Canceled: canceled,
		Failed:   failed,
	})
}
