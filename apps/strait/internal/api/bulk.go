package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/danielgtaylor/huma/v2"
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
	TTLSecs        *int              `json:"ttl_secs,omitempty"`
	ConcurrencyKey string            `json:"concurrency_key,omitempty"`
}

type BulkTriggerResult struct {
	ID             string `json:"id"`
	Status         string `json:"status"`
	RunToken       string `json:"run_token"`
	IdempotencyHit bool   `json:"idempotency_hit"`
}

type BulkTriggerResponse struct {
	BatchID string              `json:"batch_id"`
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

type BulkTriggerJobInput struct {
	JobID string `path:"jobID"`
	Body  BulkTriggerRequest
}

type BulkTriggerJobOutput struct {
	Body BulkTriggerResponse
}

//nolint:gocognit,gocyclo,cyclop,funlen,nestif
func (s *Server) handleBulkTriggerJob(ctx context.Context, input *BulkTriggerJobInput) (*BulkTriggerJobOutput, error) {
	job, err := s.store.GetJob(ctx, input.JobID)
	if err != nil {
		if errors.Is(err, store.ErrJobNotFound) {
			return nil, huma.Error404NotFound("job not found")
		}
		return nil, huma.Error500InternalServerError("failed to get job")
	}

	if err := requireProjectMatch(ctx, job.ProjectID); err != nil {
		return nil, huma.Error404NotFound("job not found")
	}

	// Validate plan gates on the existing job definition -- catches downgraded orgs.
	if err := s.checkHTTPModeAllowed(ctx, job.ExecutionMode, job.ProjectID); err != nil {
		return nil, err
	}
	if err := s.checkPresetAllowed(ctx, job.ProjectID, string(job.MachinePreset)); err != nil {
		return nil, err
	}

	if !job.Enabled {
		return nil, huma.Error400BadRequest("job is disabled")
	}

	if job.Paused {
		return nil, huma.Error409Conflict("job is paused -- resume it before triggering new runs")
	}

	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}

	if len(req.Items) > s.config.MaxBulkTriggerItems {
		return nil, huma.Error400BadRequest(fmt.Sprintf("maximum %d items per bulk trigger request", s.config.MaxBulkTriggerItems))
	}

	batchID := uuid.Must(uuid.NewV7()).String()
	if err := s.store.CreateBatchOperation(ctx, &domain.BatchOperation{
		ID:        batchID,
		ProjectID: job.ProjectID,
		JobID:     job.ID,
		ItemCount: len(req.Items),
		CreatedBy: actorFromContext(ctx),
	}); err != nil {
		slog.Error("failed to create batch operation", "error", err)
	}

	now := time.Now()
	results := make([]BulkTriggerResult, 0, len(req.Items))
	created := 0

	projectQuota, err := s.store.GetProjectQuota(ctx, job.ProjectID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to load project quota")
	}

	// Pre-compute project quotas once (all items target the same job/project).
	var queuedRuns, activeRuns int
	if projectQuota != nil {
		if projectQuota.MaxQueuedRuns > 0 {
			var countErr error
			queuedRuns, countErr = s.store.CountProjectQueuedRuns(ctx, job.ProjectID)
			if countErr != nil {
				return nil, huma.Error500InternalServerError("failed to count project queued runs")
			}
		}
		if projectQuota.MaxExecutingRuns > 0 {
			var countErr error
			activeRuns, countErr = s.store.CountProjectActiveRuns(ctx, job.ProjectID)
			if countErr != nil {
				return nil, huma.Error500InternalServerError("failed to count project active runs")
			}
		}
	}

	// Check if any item has an idempotency key -- if none, we can use batch COPY insert.
	hasIdempotencyKey := false
	for _, item := range req.Items {
		if item.IdempotencyKey != "" {
			hasIdempotencyKey = true
			break
		}
	}
	var pendingRuns []*domain.JobRun

	enqueuedInBatch := 0
	for _, item := range req.Items {
		itemIdx := len(results)

		if len(item.Tags) > 0 {
			if err := validateTags(item.Tags); err != nil {
				return nil, huma.Error400BadRequest(fmt.Sprintf("invalid tags for item %d: %v", itemIdx, err))
			}
		}

		if err := validatePayloadAgainstSchema(item.Payload, job.PayloadSchema); err != nil {
			return nil, huma.Error400BadRequest(fmt.Sprintf("payload validation failed for item %d: %v", itemIdx, err))
		}

		payload, _, payloadErr := canonicalizePayload(item.Payload)
		if payloadErr != nil {
			return nil, huma.Error400BadRequest(fmt.Sprintf("invalid payload for item %d: %v", itemIdx, payloadErr))
		}

		// Per-item idempotency check.
		if item.IdempotencyKey != "" {
			if len(item.IdempotencyKey) > maxIdempotencyKeyLength {
				return nil, huma.Error400BadRequest(
					fmt.Sprintf("idempotency key for item %d must be %d characters or fewer", itemIdx, maxIdempotencyKeyLength))
			}

			existingRun, idempErr := s.store.GetRunByIdempotencyKey(ctx, job.ID, item.IdempotencyKey)
			if idempErr != nil {
				return nil, huma.Error500InternalServerError(fmt.Sprintf("failed to check idempotency key for item %d", itemIdx))
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
				return nil, huma.Error429TooManyRequests("project queued quota exceeded")
			}
			if projectQuota.MaxExecutingRuns > 0 && activeRuns >= projectQuota.MaxExecutingRuns {
				return nil, huma.Error429TooManyRequests("project executing quota exceeded")
			}
		}

		if job.RateLimitMax > 0 && job.RateLimitWindowSecs > 0 {
			since := time.Now().Add(-time.Duration(job.RateLimitWindowSecs) * time.Second)
			runCount, countErr := s.store.CountRunsForJobSince(ctx, job.ID, since)
			if countErr != nil {
				return nil, huma.Error500InternalServerError("failed to evaluate job rate limit")
			}
			if runCount >= job.RateLimitMax {
				return nil, huma.Error429TooManyRequests("job rate limit exceeded")
			}
		}

		if job.DedupWindowSecs > 0 {
			since := time.Now().Add(-time.Duration(job.DedupWindowSecs) * time.Second)
			existingRun, findErr := s.store.FindRecentRunByPayload(ctx, job.ID, payload, since)
			if findErr != nil {
				return nil, huma.Error500InternalServerError("failed to evaluate payload deduplication")
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
		if item.TTLSecs != nil && *item.TTLSecs > 0 {
			expiresAt = now.Add(time.Duration(*item.TTLSecs) * time.Second)
		} else if job.RunTTLSecs > 0 {
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
			return nil, huma.Error500InternalServerError(fmt.Sprintf("failed to sign run token for item %d", itemIdx))
		}

		scheduledAt := item.ScheduledAt
		if job.ExecutionWindowCron != "" {
			timezone := job.Timezone
			if timezone == "" && projectQuota != nil {
				timezone = projectQuota.Timezone
			}
			adjustedScheduledAt, adjustErr := alignToExecutionWindow(scheduledAt, now, job.ExecutionWindowCron, timezone)
			if adjustErr != nil {
				return nil, huma.Error400BadRequest("execution window validation failed: " + adjustErr.Error())
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
			CreatedBy:      actorFromContext(ctx),
			BatchID:        batchID,
			ExpiresAt:      &expiresAt,
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
		run.ConcurrencyKey = item.ConcurrencyKey

		if hasIdempotencyKey {
			// Sequential enqueue with idempotency CTE.
			if err := s.queue.Enqueue(ctx, run); err != nil {
				if errors.Is(err, domain.ErrIdempotencyConflict) && item.IdempotencyKey != "" {
					existingRun, retryErr := s.store.GetRunByIdempotencyKey(ctx, job.ID, item.IdempotencyKey)
					if retryErr != nil {
						slog.Error("idempotency conflict retry failed",
							"job_id", job.ID,
							"idempotency_key", item.IdempotencyKey,
							"item_index", itemIdx,
							"error", retryErr)
						return nil, huma.Error500InternalServerError(fmt.Sprintf("failed to check idempotency key after conflict for item %d", itemIdx))
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
				return nil, huma.Error500InternalServerError(fmt.Sprintf("failed to enqueue item %d", itemIdx))
			}
		} else {
			// Collect for batch COPY insert.
			pendingRuns = append(pendingRuns, run)
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

	// Batch insert all collected runs via COPY protocol.
	if len(pendingRuns) > 0 {
		if _, err := s.queue.EnqueueBatch(ctx, pendingRuns); err != nil {
			return nil, huma.Error500InternalServerError("failed to enqueue batch")
		}
	}

	if err := s.store.FinalizeBatchOperation(ctx, batchID, created); err != nil {
		slog.Error("failed to finalize batch operation", "batch_id", batchID, "error", err)
	}

	return &BulkTriggerJobOutput{
		Body: BulkTriggerResponse{
			BatchID: batchID,
			Results: results,
			Total:   len(req.Items),
			Created: created,
		},
	}, nil
}

type BulkCancelRunsInput struct {
	Body BulkCancelRequest
}

type BulkCancelRunsOutput struct {
	Body BulkCancelResponse
}

func (s *Server) handleBulkCancelRuns(ctx context.Context, input *BulkCancelRunsInput) (*BulkCancelRunsOutput, error) {
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}

	if len(req.RunIDs) > 100 {
		return nil, huma.Error400BadRequest("maximum 100 run IDs per bulk cancel request")
	}

	// Step 1: Batch fetch all runs.
	runsMap, err := s.store.GetRunsByIDs(ctx, req.RunIDs)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to fetch runs")
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
		cancelResults, cancelErr := s.store.BulkCancelRuns(ctx, cancelableIDs, now, "canceled by user (bulk)")
		if cancelErr != nil {
			return nil, huma.Error500InternalServerError("failed to cancel runs")
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
		if _, err := s.store.CancelChildRunsByParentIDs(ctx, cancelableIDs, now, "parent run canceled (bulk)"); err != nil {
			slog.Error("failed to cancel child runs in bulk", "error", err)
		}
	}

	return &BulkCancelRunsOutput{
		Body: BulkCancelResponse{
			Results:  results,
			Total:    len(req.RunIDs),
			Canceled: canceled,
			Failed:   failed,
		},
	}, nil
}

type BulkCancelAllRequest struct {
	JobID       string           `json:"job_id,omitempty"`
	BatchID     string           `json:"batch_id,omitempty"`
	TriggeredBy string           `json:"triggered_by,omitempty"`
	Status      domain.RunStatus `json:"status,omitempty"`
}

type BulkCancelAllInput struct {
	Body BulkCancelAllRequest
}

type BulkCancelAllOutput struct {
	Body map[string]any
}

func (s *Server) handleBulkCancelAll(ctx context.Context, input *BulkCancelAllInput) (*BulkCancelAllOutput, error) {
	req := input.Body
	projectID := projectIDFromContext(ctx)

	if req.JobID == "" && req.BatchID == "" && req.TriggeredBy == "" && req.Status == "" {
		return nil, huma.Error400BadRequest("at least one filter is required")
	}

	now := time.Now()
	ids, err := s.store.BulkCancelByFilter(ctx, projectID, store.BulkCancelFilter{
		JobID: req.JobID, BatchID: req.BatchID, TriggeredBy: req.TriggeredBy, Status: req.Status,
	}, now, "canceled by user (bulk filter)")
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to cancel runs")
	}

	return &BulkCancelAllOutput{Body: map[string]any{"canceled": len(ids), "run_ids": ids}}, nil
}
