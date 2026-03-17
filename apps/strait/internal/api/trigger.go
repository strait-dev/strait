package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	"github.com/robfig/cron/v3"
	otelattr "go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
)

// maxIdempotencyKeyLength is the maximum allowed length for idempotency keys.
// Keys exceeding this limit are rejected with 400 to protect the DB index.
const maxIdempotencyKeyLength = 256

type TriggerRequest struct {
	Payload        json.RawMessage   `json:"payload,omitempty"`
	Tags           map[string]string `json:"tags,omitempty"`
	ScheduledAt    *time.Time        `json:"scheduled_at,omitempty"`
	Priority       int               `json:"priority,omitempty" validate:"min=0,max=10"`
	DryRun         bool              `json:"dry_run,omitempty"`
	TTLSecs        *int              `json:"ttl_secs,omitempty"`
	ConcurrencyKey string            `json:"concurrency_key,omitempty"`
	DebounceKey    string            `json:"debounce_key,omitempty"`
	BatchKey       string            `json:"batch_key,omitempty"`
}

func (s *Server) handleTriggerJob(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID")
	if err := validateRunCreationJobID(jobID); err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

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

	var req TriggerRequest
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if !s.validateRequest(w, r, &req) {
		return
	}
	if err := validatePayloadSize(req.Payload); err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Handle dry-run mode
	if req.DryRun {
		result, err := s.validateTriggerRequest(r.Context(), jobID, req)
		if err != nil {
			respondError(w, r, http.StatusBadRequest, err.Error())
			return
		}
		respondJSON(w, http.StatusOK, result)
		return
	}
	if err := validatePayloadAgainstSchema(req.Payload, job.PayloadSchema); err != nil {
		respondError(w, r, http.StatusBadRequest, "payload validation failed: "+err.Error())
		return
	}

	payload, payloadHash, err := canonicalizePayload(req.Payload)
	if err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid payload: "+err.Error())
		return
	}

	// Idempotency check: must happen before quotas, rate limits, and cost
	// budgets so that retried requests with the same key always get the
	// cached response regardless of transient limit conditions.
	idempotencyKey := r.Header.Get("X-Idempotency-Key")
	if idempotencyKey == "" {
		idempotencyKey = r.Header.Get("Idempotency-Key")
	}
	if idempotencyKey != "" {
		if len(idempotencyKey) > maxIdempotencyKeyLength {
			respondError(w, r, http.StatusBadRequest,
				fmt.Sprintf("idempotency key must be %d characters or fewer", maxIdempotencyKeyLength))
			return
		}

		existingRun, idempErr := s.store.GetRunByIdempotencyKey(r.Context(), job.ID, idempotencyKey)
		if idempErr != nil {
			respondError(w, r, http.StatusInternalServerError, "failed to check idempotency key")
			return
		}
		if existingRun != nil {
			slog.Info("idempotency hit",
				"job_id", job.ID,
				"idempotency_key", idempotencyKey,
				"existing_run_id", existingRun.ID,
				"existing_run_status", existingRun.Status)
			respondJSON(w, http.StatusCreated, map[string]any{
				"id":              existingRun.ID,
				"status":          existingRun.Status,
				"idempotency_hit": true,
			})
			return
		}
	}

	var projectQuota *store.ProjectQuota
	projectQuota, err = s.store.GetProjectQuota(r.Context(), job.ProjectID)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to load project quota")
		return
	}

	if projectQuota != nil {
		if projectQuota.MaxQueuedRuns > 0 {
			queuedRuns, countErr := s.store.CountProjectQueuedRuns(r.Context(), job.ProjectID)
			if countErr != nil {
				respondError(w, r, http.StatusInternalServerError, "failed to evaluate project queued quota")
				return
			}
			if queuedRuns >= projectQuota.MaxQueuedRuns {
				respondError(w, r, http.StatusTooManyRequests, "project queued quota exceeded")
				return
			}
		}

		if projectQuota.MaxExecutingRuns > 0 {
			activeRuns, countErr := s.store.CountProjectActiveRuns(r.Context(), job.ProjectID)
			if countErr != nil {
				respondError(w, r, http.StatusInternalServerError, "failed to evaluate project active quota")
				return
			}
			if activeRuns >= projectQuota.MaxExecutingRuns {
				respondError(w, r, http.StatusTooManyRequests, "project executing quota exceeded")
				return
			}
		}
	}

	if projectQuota != nil && projectQuota.MaxDailyCostMicrousd > 0 {
		tz := projectQuota.Timezone
		if tz == "" {
			tz = "UTC"
		}
		dailyCost, costErr := s.store.SumProjectDailyCostMicrousd(r.Context(), job.ProjectID, tz)
		if costErr != nil {
			respondError(w, r, http.StatusInternalServerError, fmt.Sprintf("failed to evaluate daily cost budget (timezone: %s)", tz))
			return
		}
		if dailyCost >= projectQuota.MaxDailyCostMicrousd {
			respondError(w, r, http.StatusTooManyRequests, "project daily cost budget exceeded")
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
			respondJSON(w, http.StatusCreated, map[string]any{
				"id":              existingRun.ID,
				"status":          existingRun.Status,
				"payload_hash":    payloadHash,
				"idempotency_hit": false,
			})
			return
		}
	}

	// Debounce: coalesce rapid triggers into one run after quiet window.
	if job.DebounceWindowSecs > 0 {
		fireAt := time.Now().Add(time.Duration(job.DebounceWindowSecs) * time.Second)
		tagsJSON, _ := json.Marshal(req.Tags)
		pending := &domain.DebouncePending{
			JobID:          job.ID,
			ProjectID:      job.ProjectID,
			DebounceKey:    req.DebounceKey,
			Payload:        payload,
			Tags:           tagsJSON,
			Priority:       req.Priority,
			ConcurrencyKey: req.ConcurrencyKey,
			TTLSecs:        req.TTLSecs,
			TriggeredBy:    domain.TriggerDebounce,
			CreatedBy:      actorFromContext(r.Context()),
			FireAt:         fireAt,
		}
		if err := s.store.UpsertDebouncePending(r.Context(), pending); err != nil {
			respondError(w, r, http.StatusInternalServerError, "failed to upsert debounce pending")
			return
		}
		respondJSON(w, http.StatusAccepted, map[string]any{
			"debounced": true,
			"fire_at":   fireAt,
		})
		return
	}

	// Batch: collect payloads until size or time threshold, then flush as one run.
	if job.BatchWindowSecs > 0 {
		tagsJSON, _ := json.Marshal(req.Tags)
		item := &domain.BatchBufferItem{
			JobID:       job.ID,
			ProjectID:   job.ProjectID,
			BatchKey:    req.BatchKey,
			Payload:     payload,
			Tags:        tagsJSON,
			Priority:    req.Priority,
			TriggeredBy: domain.TriggerManual,
			CreatedBy:   actorFromContext(r.Context()),
		}
		if err := s.store.InsertBatchBufferItem(r.Context(), item); err != nil {
			respondError(w, r, http.StatusInternalServerError, "failed to insert batch buffer item")
			return
		}

		// Check if max size reached → immediate flush.
		if job.BatchMaxSize > 0 {
			count, countErr := s.store.CountBatchBufferItems(r.Context(), job.ID, req.BatchKey)
			if countErr == nil && count >= job.BatchMaxSize {
				items, drainErr := s.store.DrainBatchBuffer(r.Context(), job.ID, req.BatchKey, job.BatchMaxSize)
				if drainErr == nil && len(items) > 0 {
					payloads := make([]json.RawMessage, len(items))
					for i, it := range items {
						payloads[i] = it.Payload
					}
					batchPayload, _ := json.Marshal(map[string]any{"items": payloads})
					batchNow := time.Now()
					batchExpiresAt := batchNow.Add(time.Duration(job.TimeoutSecs)*time.Second + 60*time.Second)
					batchRun := &domain.JobRun{
						ID:           uuid.Must(uuid.NewV7()).String(),
						JobID:        job.ID,
						ProjectID:    job.ProjectID,
						Status:       domain.StatusQueued,
						Attempt:      1,
						Payload:      batchPayload,
						TriggeredBy:  "batch",
						Priority:     req.Priority,
						JobVersion:   job.Version,
						JobVersionID: job.VersionID,
						ExpiresAt:    &batchExpiresAt,
						CreatedBy:    actorFromContext(r.Context()),
					}
					if enqErr := s.queue.Enqueue(r.Context(), batchRun); enqErr != nil {
						slog.Error("batch immediate flush enqueue failed", "job_id", job.ID, "error", enqErr)
						respondError(w, r, http.StatusInternalServerError, "failed to enqueue batch run")
						return
					}
					respondJSON(w, http.StatusCreated, map[string]any{
						"id":     batchRun.ID,
						"status": batchRun.Status,
						"batch":  true,
					})
					return
				}
			}
		}

		respondJSON(w, http.StatusAccepted, map[string]any{
			"buffered": true,
		})
		return
	}

	runID := uuid.Must(uuid.NewV7()).String()
	now := time.Now()
	scheduledAt := req.ScheduledAt
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

	var expiresAt time.Time
	if req.TTLSecs != nil && *req.TTLSecs > 0 {
		expiresAt = now.Add(time.Duration(*req.TTLSecs) * time.Second)
	} else if job.RunTTLSecs > 0 {
		expiresAt = now.Add(time.Duration(job.RunTTLSecs) * time.Second)
	} else if s.config.DefaultRunTTLSecs > 0 {
		expiresAt = now.Add(time.Duration(s.config.DefaultRunTTLSecs) * time.Second)
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
		respondError(w, r, http.StatusInternalServerError, "failed to sign run token")
		return
	}

	status := domain.StatusQueued
	if scheduledAt != nil && scheduledAt.After(now) {
		status = domain.StatusDelayed
	}

	dependencyKey := extractDependencyKey(payload)

	// Inherit job tags, then overlay with trigger-specific tags.
	runTags := make(map[string]string, len(job.Tags)+len(req.Tags))
	maps.Copy(runTags, job.Tags)
	maps.Copy(runTags, req.Tags)

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
		Priority:       req.Priority,
		IdempotencyKey: idempotencyKey,
		JobVersion:     job.Version,
		JobVersionID:   job.VersionID,
		CreatedBy:      actorFromContext(r.Context()),
		ExpiresAt:      &expiresAt,
	}
	if dependencyKey != "" {
		run.Metadata = map[string]string{"dependency_key": dependencyKey}
	}

	// Merge default run metadata from job. Caller metadata wins on conflicts.
	if len(job.DefaultRunMetadata) > 0 {
		if run.Metadata == nil {
			run.Metadata = make(map[string]string, len(job.DefaultRunMetadata))
		}
		for k, v := range job.DefaultRunMetadata {
			if _, exists := run.Metadata[k]; !exists {
				run.Metadata[k] = v
			}
		}
	}
	run.ConcurrencyKey = req.ConcurrencyKey

	// Capture Fly-Region header for geo-routing of managed dispatch.
	if flyRegion := r.Header.Get("Fly-Region"); flyRegion != "" {
		if run.Metadata == nil {
			run.Metadata = make(map[string]string)
		}
		run.Metadata["_region_hint"] = flyRegion
	}

	// Capture W3C trace context from incoming request headers.
	if tp := r.Header.Get("Traceparent"); tp != "" {
		if run.Metadata == nil {
			run.Metadata = make(map[string]string)
		}
		run.Metadata["_trace_parent"] = tp
		if ts := r.Header.Get("Tracestate"); ts != "" {
			run.Metadata["_trace_state"] = ts
		}
	}

	if status == domain.StatusQueued {
		satisfied, depErr := s.store.AreJobDependenciesSatisfied(r.Context(), run)
		if depErr != nil {
			respondError(w, r, http.StatusInternalServerError, "failed to evaluate job dependencies")
			return
		}
		if !satisfied {
			run.Status = domain.StatusWaiting
			if s.metrics != nil {
				attrs := otelmetric.WithAttributes(
					otelattr.String("project_id", run.ProjectID),
					otelattr.String("job_id", run.JobID),
				)
				s.metrics.WorkflowDependencyWaits.Add(r.Context(), 1, attrs)
			}
			if err := s.store.CreateRun(r.Context(), run); err != nil {
				if errors.Is(err, domain.ErrIdempotencyConflict) && idempotencyKey != "" {
					existingRun, retryErr := s.store.GetRunByIdempotencyKey(r.Context(), job.ID, idempotencyKey)
					if retryErr != nil {
						slog.Error("idempotency conflict retry failed",
							"job_id", job.ID,
							"idempotency_key", idempotencyKey,
							"error", retryErr)
						respondError(w, r, http.StatusInternalServerError, "failed to check idempotency key after conflict")
						return
					}
					if existingRun != nil {
						slog.Warn("idempotency conflict resolved",
							"job_id", job.ID,
							"idempotency_key", idempotencyKey,
							"winning_run_id", existingRun.ID)
						respondJSON(w, http.StatusCreated, map[string]any{
							"id":              existingRun.ID,
							"status":          existingRun.Status,
							"idempotency_hit": true,
						})
						return
					}
					slog.Error("idempotency conflict retry returned nil",
						"job_id", job.ID,
						"idempotency_key", idempotencyKey)
				}
				respondError(w, r, http.StatusInternalServerError, "failed to create waiting run")
				return
			}
			respondJSON(w, http.StatusCreated, map[string]any{
				"id":              run.ID,
				"status":          run.Status,
				"payload_hash":    payloadHash,
				"run_token":       tokenString,
				"idempotency_hit": false,
			})
			return
		}
	}

	if err := s.queue.Enqueue(r.Context(), run); err != nil {
		// Handle race condition: two concurrent requests with the same
		// idempotency key both passed the app-level check but the DB
		// unique index rejected the second INSERT. Retry the lookup.
		if errors.Is(err, domain.ErrIdempotencyConflict) && idempotencyKey != "" {
			existingRun, retryErr := s.store.GetRunByIdempotencyKey(r.Context(), job.ID, idempotencyKey)
			if retryErr != nil {
				slog.Error("idempotency conflict retry failed",
					"job_id", job.ID,
					"idempotency_key", idempotencyKey,
					"error", retryErr)
				respondError(w, r, http.StatusInternalServerError, "failed to check idempotency key after conflict")
				return
			}
			if existingRun != nil {
				slog.Warn("idempotency conflict resolved",
					"job_id", job.ID,
					"idempotency_key", idempotencyKey,
					"winning_run_id", existingRun.ID)
				respondJSON(w, http.StatusCreated, map[string]any{
					"id":              existingRun.ID,
					"status":          existingRun.Status,
					"idempotency_hit": true,
				})
				return
			}
			slog.Error("idempotency conflict retry returned nil",
				"job_id", job.ID,
				"idempotency_key", idempotencyKey)
		}
		respondError(w, r, http.StatusInternalServerError, "failed to enqueue run")
		return
	}

	respondJSON(w, http.StatusCreated, map[string]any{
		"id":              run.ID,
		"status":          run.Status,
		"payload_hash":    payloadHash,
		"run_token":       tokenString,
		"idempotency_hit": false,
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

func extractDependencyKey(payload json.RawMessage) string {
	if len(payload) == 0 {
		return ""
	}
	var body map[string]any
	if err := json.Unmarshal(payload, &body); err != nil {
		return ""
	}
	if key, ok := body["dependency_key"].(string); ok {
		return key
	}
	return ""
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
		return nil, nil //nolint:nilnil // nil signals "trigger now" with no explicit time.
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

// DryRunValidationResult contains the result of trigger validation for dry-run mode.
type DryRunValidationResult struct {
	Job                *domain.Job     `json:"job"`
	PayloadHash        string          `json:"payload_hash"`
	Payload            json.RawMessage `json:"payload,omitempty"`
	ScheduledAt        *time.Time      `json:"scheduled_at,omitempty"`
	ExpiresAt          time.Time       `json:"expires_at"`
	ValidationWarnings []string        `json:"validation_warnings,omitempty"`
}

func (s *Server) validateTriggerRequest(ctx context.Context, jobID string, req TriggerRequest) (*DryRunValidationResult, error) {
	if err := validateRunCreationJobID(jobID); err != nil {
		return nil, err
	}
	if err := validatePayloadSize(req.Payload); err != nil {
		return nil, err
	}

	job, err := s.store.GetJob(ctx, jobID)
	if err != nil {
		return nil, err
	}

	if !job.Enabled {
		return nil, errors.New("job is disabled")
	}

	if err := validatePayloadAgainstSchema(req.Payload, job.PayloadSchema); err != nil {
		return nil, fmt.Errorf("payload validation failed: %w", err)
	}

	payload, payloadHash, err := canonicalizePayload(req.Payload)
	if err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}

	var projectQuota *store.ProjectQuota
	var warnings []string
	projectQuota, err = s.store.GetProjectQuota(ctx, job.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("failed to load project quota: %w", err)
	}

	if projectQuota != nil {
		if projectQuota.MaxQueuedRuns > 0 {
			queuedRuns, countErr := s.store.CountProjectQueuedRuns(ctx, job.ProjectID)
			if countErr != nil {
				return nil, fmt.Errorf("failed to evaluate project queued quota: %w", countErr)
			}
			if queuedRuns >= projectQuota.MaxQueuedRuns {
				return nil, errors.New("project queued quota exceeded")
			}
		}

		if projectQuota.MaxExecutingRuns > 0 {
			activeRuns, countErr := s.store.CountProjectActiveRuns(ctx, job.ProjectID)
			if countErr != nil {
				return nil, fmt.Errorf("failed to evaluate project active quota: %w", countErr)
			}
			if activeRuns >= projectQuota.MaxExecutingRuns {
				return nil, errors.New("project executing quota exceeded")
			}
		}
	}

	if job.RateLimitMax > 0 && job.RateLimitWindowSecs > 0 {
		since := time.Now().Add(-time.Duration(job.RateLimitWindowSecs) * time.Second)
		runCount, countErr := s.store.CountRunsForJobSince(ctx, job.ID, since)
		if countErr != nil {
			return nil, fmt.Errorf("failed to evaluate job rate limit: %w", countErr)
		}
		if runCount >= job.RateLimitMax {
			return nil, errors.New("job rate limit exceeded")
		}
	}

	if job.DedupWindowSecs > 0 {
		since := time.Now().Add(-time.Duration(job.DedupWindowSecs) * time.Second)
		existingRun, findErr := s.store.FindRecentRunByPayload(ctx, job.ID, payload, since)
		if findErr != nil {
			return nil, fmt.Errorf("failed to evaluate payload deduplication: %w", findErr)
		}
		if existingRun != nil {
			warnings = append(warnings, fmt.Sprintf("payload deduplication: run %s", existingRun.ID))
		}
	}

	now := time.Now()
	scheduledAt := req.ScheduledAt
	if job.ExecutionWindowCron != "" {
		timezone := job.Timezone
		if timezone == "" && projectQuota != nil {
			timezone = projectQuota.Timezone
		}
		adjustedScheduledAt, adjustErr := alignToExecutionWindow(scheduledAt, now, job.ExecutionWindowCron, timezone)
		if adjustErr != nil {
			return nil, fmt.Errorf("execution window validation failed: %w", adjustErr)
		}
		scheduledAt = adjustedScheduledAt
	}

	var expiresAt time.Time
	if req.TTLSecs != nil && *req.TTLSecs > 0 {
		expiresAt = now.Add(time.Duration(*req.TTLSecs) * time.Second)
	} else if job.RunTTLSecs > 0 {
		expiresAt = now.Add(time.Duration(job.RunTTLSecs) * time.Second)
	} else if s.config.DefaultRunTTLSecs > 0 {
		expiresAt = now.Add(time.Duration(s.config.DefaultRunTTLSecs) * time.Second)
	} else {
		expiresAt = now.Add(time.Duration(job.TimeoutSecs)*time.Second + 60*time.Second)
	}

	return &DryRunValidationResult{
		Job:                job,
		PayloadHash:        payloadHash,
		Payload:            payload,
		ScheduledAt:        scheduledAt,
		ExpiresAt:          expiresAt,
		ValidationWarnings: warnings,
	}, nil
}
