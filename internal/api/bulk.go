package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type BulkTriggerRequest struct {
	Items []BulkTriggerItem `json:"items" validate:"required,min=1"`
}

type BulkTriggerItem struct {
	Payload        json.RawMessage   `json:"payload,omitempty"`
	ScheduledAt    *time.Time        `json:"scheduled_at,omitempty"`
	Priority       int               `json:"priority,omitempty"`
	IdempotencyKey string            `json:"idempotency_key,omitempty"`
	Tags           map[string]string `json:"tags,omitempty"`
}

type BulkTriggerResult struct {
	ID             string `json:"id"`
	Status         string `json:"status"`
	RunToken       string `json:"run_token"`
	IdempotencyHit bool   `json:"idempotency_hit"`
}

type BulkTriggerResponse struct {
	Results []BulkTriggerResult `json:"results"`
	Total   int                 `json:"total"`
	Created int                 `json:"created"`
}

type BulkCancelRequest struct {
	RunIDs []string `json:"run_ids" validate:"required,min=1"`
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
			respondError(w, r, http.StatusNotFound, "job not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get job")
		return
	}

	if !job.Enabled {
		respondError(w, r, http.StatusBadRequest, "job is disabled")
		return
	}

	var req BulkTriggerRequest
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if !s.validateRequest(w, r, &req) {
		return
	}

	if len(req.Items) > 100 {
		respondError(w, r, http.StatusBadRequest, "maximum 100 items per bulk trigger request")
		return
	}

	now := time.Now()
	results := make([]BulkTriggerResult, 0, len(req.Items))
	created := 0

	projectQuota, err := s.store.GetProjectQuota(r.Context(), job.ProjectID)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to load project quota")
		return
	}

	// Pre-compute project quotas once (all items target the same job/project).
	var queuedRuns, activeRuns int
	if projectQuota != nil {
		if projectQuota.MaxQueuedRuns > 0 {
			var countErr error
			queuedRuns, countErr = s.store.CountProjectQueuedRuns(r.Context(), job.ProjectID)
			if countErr != nil {
				respondError(w, r, http.StatusInternalServerError, "failed to count project queued runs")
				return
			}
		}
		if projectQuota.MaxExecutingRuns > 0 {
			var countErr error
			activeRuns, countErr = s.store.CountProjectActiveRuns(r.Context(), job.ProjectID)
			if countErr != nil {
				respondError(w, r, http.StatusInternalServerError, "failed to count project active runs")
				return
			}
		}
	}

	enqueuedInBatch := 0
	for _, item := range req.Items {
		itemIdx := len(results)

		if len(item.Tags) > 0 {
			if err := validateTags(item.Tags); err != nil {
				respondError(w, r, http.StatusBadRequest, fmt.Sprintf("invalid tags for item %d: %v", itemIdx, err))
				return
			}
		}

		if err := validatePayloadAgainstSchema(item.Payload, job.PayloadSchema); err != nil {
			respondError(w, r, http.StatusBadRequest, fmt.Sprintf("payload validation failed for item %d: %v", itemIdx, err))
			return
		}

		payload, _, payloadErr := canonicalizePayload(item.Payload)
		if payloadErr != nil {
			respondError(w, r, http.StatusBadRequest, fmt.Sprintf("invalid payload for item %d: %v", itemIdx, payloadErr))
			return
		}

		// Per-item idempotency check.
		if item.IdempotencyKey != "" {
			if len(item.IdempotencyKey) > maxIdempotencyKeyLength {
				respondError(w, r, http.StatusBadRequest,
					fmt.Sprintf("idempotency key for item %d must be %d characters or fewer", itemIdx, maxIdempotencyKeyLength))
				return
			}

			existingRun, idempErr := s.store.GetRunByIdempotencyKey(r.Context(), job.ID, item.IdempotencyKey)
			if idempErr != nil {
				respondError(w, r, http.StatusInternalServerError, fmt.Sprintf("failed to check idempotency key for item %d", itemIdx))
				return
			}
			if existingRun != nil {
				slog.Info("idempotency hit",
					"job_id", job.ID,
					"idempotency_key", item.IdempotencyKey,
					"existing_run_id", existingRun.ID,
					"existing_run_status", existingRun.Status,
					"item_index", itemIdx)
				results = append(results, BulkTriggerResult{
					ID:             existingRun.ID,
					Status:         string(existingRun.Status),
					IdempotencyHit: true,
				})
				continue
			}
		}

		if projectQuota != nil {
			if projectQuota.MaxQueuedRuns > 0 && (queuedRuns+enqueuedInBatch) >= projectQuota.MaxQueuedRuns {
				respondError(w, r, http.StatusTooManyRequests, "project queued quota exceeded")
				return
			}
			if projectQuota.MaxExecutingRuns > 0 && activeRuns >= projectQuota.MaxExecutingRuns {
				respondError(w, r, http.StatusTooManyRequests, "project executing quota exceeded")
				return
			}
		}

		if job.RateLimitMax > 0 && job.RateLimitWindowSecs > 0 {
			since := time.Now().Add(-time.Duration(job.RateLimitWindowSecs) * time.Second)
			runCount, countErr := s.store.CountRunsForJobSince(r.Context(), job.ID, since)
			if countErr != nil {
				respondError(w, r, http.StatusInternalServerError, "failed to evaluate job rate limit")
				return
			}
			if runCount >= job.RateLimitMax {
				respondError(w, r, http.StatusTooManyRequests, "job rate limit exceeded")
				return
			}
		}

		if job.DedupWindowSecs > 0 {
			since := time.Now().Add(-time.Duration(job.DedupWindowSecs) * time.Second)
			existingRun, findErr := s.store.FindRecentRunByPayload(r.Context(), job.ID, payload, since)
			if findErr != nil {
				respondError(w, r, http.StatusInternalServerError, "failed to evaluate payload deduplication")
				return
			}
			if existingRun != nil {
				results = append(results, BulkTriggerResult{
					ID:             existingRun.ID,
					Status:         string(existingRun.Status),
					RunToken:       "",
					IdempotencyHit: false,
				})
				continue
			}
		}

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
			respondError(w, r, http.StatusInternalServerError, fmt.Sprintf("failed to sign run token for item %d", itemIdx))
			return
		}

		scheduledAt := item.ScheduledAt
		if job.ExecutionWindowCron != "" {
			timezone := job.Timezone
			if timezone == "" && projectQuota != nil {
				timezone = projectQuota.Timezone
			}
			adjustedScheduledAt, adjustErr := alignToExecutionWindow(scheduledAt, now, job.ExecutionWindowCron, timezone)
			if adjustErr != nil {
				respondError(w, r, http.StatusBadRequest, "execution window validation failed: "+adjustErr.Error())
				return
			}
			scheduledAt = adjustedScheduledAt
		}

		status := domain.StatusQueued
		if scheduledAt != nil && scheduledAt.After(now) {
			status = domain.StatusDelayed
		}

		// Inherit job tags, then overlay per-item tags.
		runTags := make(map[string]string, len(job.Tags)+len(item.Tags))
		maps.Copy(runTags, job.Tags)
		maps.Copy(runTags, item.Tags)

		run := &domain.JobRun{
			ID:             runID,
			JobID:          job.ID,
			ProjectID:      job.ProjectID,
			Tags:           runTags,
			Status:         status,
			Attempt:        1,
			Payload:        payload,
			TriggeredBy:    domain.TriggerManual,
			ScheduledAt:    scheduledAt,
			Priority:       item.Priority,
			IdempotencyKey: item.IdempotencyKey,
			JobVersion:     job.Version,
			JobVersionID:   job.VersionID,
			CreatedBy:      actorFromContext(r.Context()),
			ExpiresAt:      &expiresAt,
		}

		if err := s.queue.Enqueue(r.Context(), run); err != nil {
			// Handle race: concurrent bulk request with the same idempotency key.
			if errors.Is(err, domain.ErrIdempotencyConflict) && item.IdempotencyKey != "" {
				existingRun, retryErr := s.store.GetRunByIdempotencyKey(r.Context(), job.ID, item.IdempotencyKey)
				if retryErr != nil {
					slog.Error("idempotency conflict retry failed",
						"job_id", job.ID,
						"idempotency_key", item.IdempotencyKey,
						"item_index", itemIdx,
						"error", retryErr)
					respondError(w, r, http.StatusInternalServerError, fmt.Sprintf("failed to check idempotency key after conflict for item %d", itemIdx))
					return
				}
				if existingRun != nil {
					slog.Warn("idempotency conflict resolved",
						"job_id", job.ID,
						"idempotency_key", item.IdempotencyKey,
						"winning_run_id", existingRun.ID,
						"item_index", itemIdx)
					results = append(results, BulkTriggerResult{
						ID:             existingRun.ID,
						Status:         string(existingRun.Status),
						IdempotencyHit: true,
					})
					continue
				}
				slog.Error("idempotency conflict retry returned nil",
					"job_id", job.ID,
					"idempotency_key", item.IdempotencyKey,
					"item_index", itemIdx)
			}
			respondError(w, r, http.StatusInternalServerError, fmt.Sprintf("failed to enqueue item %d", itemIdx))
			return
		}

		results = append(results, BulkTriggerResult{
			ID:             run.ID,
			Status:         string(run.Status),
			RunToken:       tokenString,
			IdempotencyHit: false,
		})
		created++
		enqueuedInBatch++
	}

	respondJSON(w, http.StatusCreated, BulkTriggerResponse{
		Results: results,
		Total:   len(req.Items),
		Created: created,
	})
}

func (s *Server) handleBulkCancelRuns(w http.ResponseWriter, r *http.Request) {
	var req BulkCancelRequest
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if !s.validateRequest(w, r, &req) {
		return
	}

	if len(req.RunIDs) > 100 {
		respondError(w, r, http.StatusBadRequest, "maximum 100 run IDs per bulk cancel request")
		return
	}

	// Step 1: Batch fetch all runs.
	runsMap, err := s.store.GetRunsByIDs(r.Context(), req.RunIDs)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to fetch runs")
		return
	}

	// Step 2: Partition into cancelable and not.
	results := make([]BulkCancelResult, 0, len(req.RunIDs))
	canceled := 0
	failed := 0
	var cancelableIDs []string
	for _, runID := range req.RunIDs {
		run, ok := runsMap[runID]
		if !ok {
			results = append(results, BulkCancelResult{ID: runID, Status: "failed", Error: "run not found"})
			failed++
			continue
		}
		if run.Status.IsTerminal() {
			results = append(results, BulkCancelResult{ID: runID, Status: string(run.Status), Error: "run already in terminal state"})
			failed++
			continue
		}
		cancelableIDs = append(cancelableIDs, runID)
	}

	// Step 3: Batch cancel.
	if len(cancelableIDs) > 0 {
		now := time.Now()
		cancelResults, cancelErr := s.store.BulkCancelRuns(r.Context(), cancelableIDs, now, "canceled by user (bulk)")
		if cancelErr != nil {
			respondError(w, r, http.StatusInternalServerError, "failed to cancel runs")
			return
		}

		canceledSet := make(map[string]struct{}, len(cancelResults))
		for _, cr := range cancelResults {
			canceledSet[cr.ID] = struct{}{}
			results = append(results, BulkCancelResult{ID: cr.ID, Status: string(domain.StatusCanceled)})
			canceled++
		}

		// Handle IDs that were not canceled (race: status changed between fetch and update).
		for _, id := range cancelableIDs {
			if _, ok := canceledSet[id]; !ok {
				results = append(results, BulkCancelResult{ID: id, Status: string(runsMap[id].Status), Error: "failed to cancel (status may have changed)"})
				failed++
			}
		}

		// Step 4: Batch cancel children.
		if _, err := s.store.CancelChildRunsByParentIDs(r.Context(), cancelableIDs, now, "parent run canceled (bulk)"); err != nil {
			slog.Error("failed to cancel child runs in bulk", "error", err)
		}
	}

	respondJSON(w, http.StatusOK, BulkCancelResponse{
		Results:  results,
		Total:    len(req.RunIDs),
		Canceled: canceled,
		Failed:   failed,
	})
}
