package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"strconv"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
	otelattr "go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
)

// maxIdempotencyKeyLength is the maximum allowed length for idempotency keys.
// Keys exceeding this limit are rejected with 400 to protect the DB index.
const maxIdempotencyKeyLength = 256

var (
	errTriggerProjectQueuedQuotaExceeded    = errors.New("project queued quota exceeded")
	errTriggerProjectExecutingQuotaExceeded = errors.New("project executing quota exceeded")
	errTriggerJobRateLimitExceeded          = errors.New("job rate limit exceeded")
)

type triggerLimitTransactioner interface {
	WithTx(ctx context.Context, fn func(context.Context, store.DBTX) error) error
}

type TriggerRequest struct {
	Payload        json.RawMessage   `json:"payload,omitempty"`
	Tags           map[string]string `json:"tags,omitempty"`
	ScheduledAt    *time.Time        `json:"scheduled_at,omitempty"`
	Priority       int               `json:"priority,omitempty" validate:"min=0,max=10"`
	DryRun         bool              `json:"dry_run,omitempty"`
	TTLSecs        *int              `json:"ttl_secs,omitempty" validate:"omitempty,min=0,max=2592000"`
	ConcurrencyKey string            `json:"concurrency_key,omitempty" validate:"max=256"`
	DebounceKey    string            `json:"debounce_key,omitempty" validate:"max=256"`
	BatchKey       string            `json:"batch_key,omitempty" validate:"max=256"`
}

type TriggerJobInput struct {
	JobID             string `path:"jobID"`
	XIdempotencyKey   string `header:"X-Idempotency-Key"`
	IdempotencyKeyAlt string `header:"Idempotency-Key"`
	Traceparent       string `header:"Traceparent" maxLength:"256"`
	Tracestate        string `header:"Tracestate" maxLength:"8192"`
	SentryTrace       string `header:"Sentry-Trace" maxLength:"8192"`
	Baggage           string `header:"Baggage" maxLength:"8192"`
	Body              TriggerRequest
}

type TriggerJobOutput struct {
	Body any
}

//nolint:gocognit,gocyclo,cyclop,funlen
func (s *Server) handleTriggerJob(ctx context.Context, input *TriggerJobInput) (*TriggerJobOutput, error) {
	jobID := input.JobID
	if err := validateRunCreationJobID(jobID); err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	job, err := s.store.GetJob(ctx, jobID)
	if err != nil {
		if errors.Is(err, store.ErrJobNotFound) {
			return nil, huma.Error404NotFound("job not found")
		}
		return nil, huma.Error500InternalServerError("failed to get job")
	}

	if err := requireProjectMatch(ctx, job.ProjectID); err != nil {
		return nil, huma.Error404NotFound("job not found")
	}
	if err := requireEnvironmentMatch(ctx, job.EnvironmentID); err != nil {
		return nil, huma.Error404NotFound("job not found")
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
	if err := validateTriggerTraceHeaders(input); err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	if err := validatePayloadSize(req.Payload); err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	if err := validateTags(req.Tags); err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	// Handle dry-run mode
	if req.DryRun {
		result, err := s.validateTriggerRequest(ctx, jobID, req)
		if err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}
		return nil, &rawStatusError{status: http.StatusOK, body: result}
	}
	if err := validatePayloadAgainstSchema(req.Payload, job.PayloadSchema); err != nil {
		return nil, huma.Error400BadRequest("payload validation failed: " + err.Error())
	}

	// Validate scheduled_at bounds.
	if req.ScheduledAt != nil {
		delay := time.Until(*req.ScheduledAt)
		if delay < 0 {
			return nil, huma.Error400BadRequest("scheduled_at must not be in the past")
		}
		if delay > 30*24*time.Hour {
			return nil, huma.Error400BadRequest("scheduled_at cannot exceed 30 days from now")
		}
	}

	payload, payloadHash, err := canonicalizePayload(req.Payload)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid payload: " + err.Error())
	}

	// Idempotency check: must happen before quotas, rate limits, and cost
	// budgets so that retried requests with the same key always get the
	// cached response regardless of transient limit conditions.
	idempotencyKey := input.XIdempotencyKey
	if idempotencyKey == "" {
		idempotencyKey = input.IdempotencyKeyAlt
	}
	if idempotencyKey != "" {
		if len(idempotencyKey) > maxIdempotencyKeyLength {
			return nil, huma.Error400BadRequest(
				fmt.Sprintf("idempotency key must be %d characters or fewer", maxIdempotencyKeyLength))
		}

		existingRun, idempErr := s.store.GetRunByIdempotencyKey(ctx, job.ID, idempotencyKey)
		if idempErr != nil {
			return nil, huma.Error500InternalServerError("failed to check idempotency key")
		}
		if existingRun != nil {
			idempotencyKeyHash := hashIdempotencyKey(idempotencyKey)
			slog.Info("idempotency hit",
				"job_id", job.ID,
				"idempotency_key_hash", idempotencyKeyHash,
				"existing_run_id", existingRun.ID,
				"existing_run_status", existingRun.Status)
			return nil, &rawStatusError{status: http.StatusOK, body: map[string]any{
				"id":              existingRun.ID,
				"status":          existingRun.Status,
				"idempotency_hit": true,
			}}
		}
	}

	// Billing: enforce dispatch priority cap before any quota or run creation work.
	if s.billingEnforcer != nil && req.Priority > 0 {
		if err := s.billingEnforcer.CheckMaxDispatchPriority(ctx, job.ProjectID, req.Priority); err != nil {
			var rse *rawStatusError
			if converted := limitErrorTo402(err, ""); converted != nil && errors.As(converted, &rse) {
				return nil, converted
			}
			return nil, huma.Error402PaymentRequired(err.Error())
		}
	}

	var projectQuota *store.ProjectQuota
	projectQuota, err = s.quotaCache.Get(ctx, job.ProjectID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to load project quota")
	}

	if projectQuota != nil && projectQuota.MaxDailyCostMicrousd > 0 {
		tz := projectQuota.Timezone
		if tz == "" {
			tz = "UTC"
		}
		dailyCost, costErr := s.store.SumProjectDailyCostMicrousd(ctx, job.ProjectID, tz)
		if costErr != nil {
			return nil, huma.Error500InternalServerError(fmt.Sprintf("failed to evaluate daily cost budget (timezone: %s)", tz))
		}
		if dailyCost >= projectQuota.MaxDailyCostMicrousd {
			return nil, huma.Error429TooManyRequests("project daily cost budget exceeded")
		}
	}

	if job.DedupWindowSecs > 0 {
		since := time.Now().Add(-time.Duration(job.DedupWindowSecs) * time.Second)
		existingRun, findErr := s.store.FindRecentRunByPayload(ctx, job.ID, payload, since)
		if findErr != nil {
			return nil, huma.Error500InternalServerError("failed to evaluate payload deduplication")
		}
		if existingRun != nil {
			return &TriggerJobOutput{Body: map[string]any{
				"id":              existingRun.ID,
				"status":          existingRun.Status,
				"payload_hash":    payloadHash,
				"idempotency_hit": false,
			}}, nil
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
			CreatedBy:      actorFromContext(ctx),
			FireAt:         fireAt,
		}
		if err := s.withTriggerLimitGuard(ctx, job, projectQuota, func(guardCtx context.Context, _ store.DBTX) error {
			return s.store.UpsertDebouncePending(guardCtx, pending)
		}); err != nil {
			return nil, triggerLimitAPIError(err, "failed to upsert debounce pending")
		}
		return &TriggerJobOutput{Body: map[string]any{
			"debounced": true,
			"fire_at":   fireAt,
		}}, nil
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
			CreatedBy:   actorFromContext(ctx),
		}
		var batchOutput *TriggerJobOutput
		if err := s.withTriggerLimitGuard(ctx, job, projectQuota, func(guardCtx context.Context, tx store.DBTX) error {
			if err := s.store.InsertBatchBufferItem(guardCtx, item); err != nil {
				return fmt.Errorf("insert batch buffer item: %w", err)
			}

			// Check if max size reached -> immediate flush.
			if job.BatchMaxSize <= 0 {
				return nil
			}
			count, countErr := s.store.CountBatchBufferItems(guardCtx, job.ID, req.BatchKey)
			if countErr != nil || count < job.BatchMaxSize {
				return countErr
			}
			items, drainErr := s.store.DrainBatchBuffer(guardCtx, job.ID, req.BatchKey, job.BatchMaxSize)
			if drainErr != nil || len(items) == 0 {
				return drainErr
			}
			payloads := make([]json.RawMessage, len(items))
			for i, it := range items {
				payloads[i] = it.Payload
			}
			batchPayload, _ := json.Marshal(map[string]any{"items": payloads})
			batchNow := time.Now()
			batchExpiresAt := batchNow.Add(time.Duration(job.TimeoutSecs)*time.Second + 60*time.Second)
			batchMetadata := sentryRunMetadata(ctx, "POST /v1/jobs/{jobID}/trigger", nil)
			batchMetadata = applyRunTraceHeaderMetadata(
				batchMetadata,
				input.Traceparent,
				input.Tracestate,
				input.SentryTrace,
				input.Baggage,
			)
			batchRun := &domain.JobRun{
				ID:            uuid.Must(uuid.NewV7()).String(),
				JobID:         job.ID,
				ProjectID:     job.ProjectID,
				Status:        domain.StatusQueued,
				Attempt:       1,
				Payload:       batchPayload,
				TriggeredBy:   "batch",
				Priority:      req.Priority,
				JobVersion:    job.Version,
				JobVersionID:  job.VersionID,
				ExpiresAt:     &batchExpiresAt,
				CreatedBy:     actorFromContext(ctx),
				ExecutionMode: job.ExecutionMode,
				QueueName:     job.Queue,
				IsRollback:    false,
				Metadata:      batchMetadata,
			}
			if enqErr := s.enqueueTriggerRun(guardCtx, tx, batchRun); enqErr != nil {
				slog.Error("batch immediate flush enqueue failed", "job_id", job.ID, "error", enqErr)
				return enqErr
			}
			batchOutput = &TriggerJobOutput{Body: map[string]any{
				"id":     batchRun.ID,
				"status": batchRun.Status,
				"batch":  true,
			}}
			return nil
		}); err != nil {
			if apiErr := enqueueAPIError(err); apiErr != nil {
				return nil, apiErr
			}
			return nil, triggerLimitAPIError(err, "failed to insert batch buffer item")
		}
		if batchOutput != nil {
			return batchOutput, nil
		}

		return &TriggerJobOutput{Body: map[string]any{
			"buffered": true,
		}}, nil
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
			return nil, huma.Error400BadRequest("execution window validation failed: " + adjustErr.Error())
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

	status := domain.StatusQueued
	if scheduledAt != nil && scheduledAt.After(now) {
		status = domain.StatusDelayed
	}

	dependencyKey := extractDependencyKey(payload)
	metadata := sentryRunMetadata(ctx, "POST /v1/jobs/{jobID}/trigger", nil)
	if dependencyKey != "" {
		metadata["dependency_key"] = dependencyKey
	}

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
		CreatedBy:      actorFromContext(ctx),
		ExpiresAt:      &expiresAt,
		ExecutionMode:  job.ExecutionMode,
		QueueName:      job.Queue,
		IsRollback:     false,
		Metadata:       metadata,
	}

	// Merge default run metadata from job. Caller metadata wins on conflicts.
	if len(job.DefaultRunMetadata) > 0 {
		for k, v := range job.DefaultRunMetadata {
			if _, exists := run.Metadata[k]; !exists {
				run.Metadata[k] = v
			}
		}
	}
	run.ConcurrencyKey = req.ConcurrencyKey
	run.Metadata = applyRunTraceHeaderMetadata(
		run.Metadata,
		input.Traceparent,
		input.Tracestate,
		input.SentryTrace,
		input.Baggage,
	)

	waitingRun := false
	if err := s.withTriggerLimitGuard(ctx, job, projectQuota, func(guardCtx context.Context, tx store.DBTX) error {
		if status == domain.StatusQueued {
			satisfied, depErr := s.store.AreJobDependenciesSatisfied(guardCtx, run)
			if depErr != nil {
				return fmt.Errorf("evaluate job dependencies: %w", depErr)
			}
			if !satisfied {
				run.Status = domain.StatusWaiting
				waitingRun = true
				if s.metrics != nil {
					attrs := otelmetric.WithAttributes(
						otelattr.String("project_id", run.ProjectID),
						otelattr.String("job_id", run.JobID),
					)
					s.metrics.WorkflowDependencyWaits.Add(guardCtx, 1, attrs)
				}
				return s.store.CreateRun(guardCtx, run)
			}
		}
		return s.enqueueTriggerRun(guardCtx, tx, run)
	}); err != nil {
		// Handle race condition: two concurrent requests with the same
		// idempotency key both passed the app-level check but the DB
		// unique index rejected the second INSERT. Retry the lookup.
		if errors.Is(err, domain.ErrIdempotencyConflict) && idempotencyKey != "" {
			existingRun, retryErr := s.store.GetRunByIdempotencyKey(ctx, job.ID, idempotencyKey)
			if retryErr != nil {
				slog.Error("idempotency conflict retry failed",
					"job_id", job.ID,
					"idempotency_key_hash", hashIdempotencyKey(idempotencyKey),
					"error", retryErr)
				return nil, huma.Error500InternalServerError("failed to check idempotency key after conflict")
			}
			if existingRun != nil {
				slog.Warn("idempotency conflict resolved",
					"job_id", job.ID,
					"idempotency_key_hash", hashIdempotencyKey(idempotencyKey),
					"winning_run_id", existingRun.ID)
				return nil, &rawStatusError{status: http.StatusOK, body: map[string]any{
					"id":              existingRun.ID,
					"status":          existingRun.Status,
					"idempotency_hit": true,
				}}
			}
			slog.Error("idempotency conflict retry returned nil",
				"job_id", job.ID,
				"idempotency_key_hash", hashIdempotencyKey(idempotencyKey))
		}
		if apiErr := enqueueAPIError(err); apiErr != nil {
			return nil, apiErr
		}
		return nil, triggerLimitAPIError(err, "failed to enqueue run")
	}
	if waitingRun {
		return &TriggerJobOutput{Body: map[string]any{
			"id":              run.ID,
			"status":          run.Status,
			"payload_hash":    payloadHash,
			"idempotency_hit": false,
		}}, nil
	}

	s.emitAuditEventAsync(ctx, domain.AuditActionJobTriggered, "job", job.ID, map[string]any{
		"run_id":               run.ID,
		"scheduled_at":         scheduledAt,
		"priority":             req.Priority,
		"idempotency_key_hash": hashIdempotencyKey(idempotencyKey),
		"tag_keys":             tagKeys(runTags),
		"triggered_by":         run.TriggeredBy,
	})

	return &TriggerJobOutput{Body: map[string]any{
		"id":              run.ID,
		"status":          run.Status,
		"payload_hash":    payloadHash,
		"idempotency_hit": false,
	}}, nil
}

// hashIdempotencyKey returns a short SHA-256 prefix of the idempotency key,
// safe for audit logs. Raw keys are never recorded.
func hashIdempotencyKey(key string) string {
	if key == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])[:16]
}

func (s *Server) withTriggerLimitGuard(ctx context.Context, job *domain.Job, quota *store.ProjectQuota, fn func(context.Context, store.DBTX) error) error {
	if txer, ok := s.store.(triggerLimitTransactioner); ok {
		return txer.WithTx(ctx, func(txCtx context.Context, tx store.DBTX) error {
			if _, err := tx.Exec(txCtx, "SELECT pg_advisory_xact_lock($1)", triggerLimitAdvisoryLockID(job.ProjectID)); err != nil {
				return fmt.Errorf("acquire trigger limit lock: %w", err)
			}
			if err := s.checkTriggerLimits(txCtx, job, quota); err != nil {
				return err
			}
			return fn(txCtx, tx)
		})
	}
	if err := s.checkTriggerLimits(ctx, job, quota); err != nil {
		return err
	}
	return fn(ctx, nil)
}

func (s *Server) checkTriggerLimits(ctx context.Context, job *domain.Job, quota *store.ProjectQuota) error {
	if quota == nil {
		return s.checkJobRateLimit(ctx, job)
	}
	if quota.MaxQueuedRuns > 0 {
		queuedRuns, countErr := s.store.CountProjectQueuedRuns(ctx, job.ProjectID)
		if countErr != nil {
			return fmt.Errorf("evaluate project queued quota: %w", countErr)
		}
		if queuedRuns >= quota.MaxQueuedRuns {
			return errTriggerProjectQueuedQuotaExceeded
		}
	}
	if quota.MaxExecutingRuns > 0 {
		activeRuns, countErr := s.store.CountProjectActiveRuns(ctx, job.ProjectID)
		if countErr != nil {
			return fmt.Errorf("evaluate project active quota: %w", countErr)
		}
		if activeRuns >= quota.MaxExecutingRuns {
			return errTriggerProjectExecutingQuotaExceeded
		}
	}
	return s.checkJobRateLimit(ctx, job)
}

func (s *Server) checkJobRateLimit(ctx context.Context, job *domain.Job) error {
	if job.RateLimitMax > 0 && job.RateLimitWindowSecs > 0 {
		since := time.Now().Add(-time.Duration(job.RateLimitWindowSecs) * time.Second)
		runCount, countErr := s.store.CountRunsForJobSince(ctx, job.ID, since)
		if countErr != nil {
			return fmt.Errorf("evaluate job rate limit: %w", countErr)
		}
		if runCount >= job.RateLimitMax {
			return errTriggerJobRateLimitExceeded
		}
	}

	return nil
}

func (s *Server) enqueueTriggerRun(ctx context.Context, tx store.DBTX, run *domain.JobRun) error {
	if tx != nil {
		return s.queue.EnqueueInTx(ctx, tx, run)
	}
	return s.queue.Enqueue(ctx, run)
}

// triggerLimitFallbackRetryAfterSeconds is the Retry-After hint surfaced
// on the sentinel-error code path (errTriggerProjectQueuedQuotaExceeded,
// errTriggerProjectExecutingQuotaExceeded, errTriggerJobRateLimitExceeded).
// It is a static fallback — callers that want a precise back-off should
// inspect the structured rate-limit metadata on the response detail
// string ("retry_after_seconds=<n>"), which is set by per-job and
// per-project limiters at the call site.
//
// 5s is long enough for callers to back off without piling on, short
// enough that legitimately throttled traffic recovers quickly when
// capacity frees up. Pre-existing huma.StatusError values (e.g. the
// daily-cost-budget 429 that resets at midnight) intentionally bypass
// this constant — see triggerLimitAPIError.
const triggerLimitFallbackRetryAfterSeconds = 5

func triggerLimitAPIError(err error, fallback string) error {
	var statusErr huma.StatusError
	if errors.As(err, &statusErr) {
		return err
	}
	switch {
	case errors.Is(err, errTriggerProjectQueuedQuotaExceeded):
		return newTriggerLimit429("project queued quota exceeded")
	case errors.Is(err, errTriggerProjectExecutingQuotaExceeded):
		return newTriggerLimit429("project executing quota exceeded")
	case errors.Is(err, errTriggerJobRateLimitExceeded):
		return newTriggerLimit429("job rate limit exceeded")
	default:
		return huma.Error500InternalServerError(fallback)
	}
}

func newTriggerLimit429(msg string) error {
	retryAfter := strconv.Itoa(triggerLimitFallbackRetryAfterSeconds)
	return &typedAPIError{
		status: http.StatusTooManyRequests,
		apiError: APIError{
			Code:    ErrorCodeRateLimited,
			Message: msg,
			Details: []string{"retry_after_seconds=" + retryAfter},
		},
		headers: map[string]string{
			"Retry-After": retryAfter,
		},
	}
}

func triggerLimitAdvisoryLockID(projectID string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte("trigger-limit:"))
	_, _ = h.Write([]byte(projectID))
	return int64(h.Sum64()) //nolint:gosec // advisory lock IDs can wrap
}

// tagKeys returns the sorted tag keys of a tag map. Values are never included
// in audit events because they may contain user data.
func tagKeys(tags map[string]string) []string {
	if len(tags) == 0 {
		return nil
	}
	keys := make([]string, 0, len(tags))
	for k := range tags {
		keys = append(keys, k)
	}
	return keys
}

func canonicalizePayload(payload json.RawMessage) (json.RawMessage, string, error) {
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}

	var v any
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if err := decoder.Decode(&v); err != nil {
		return nil, "", err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, "", errors.New("payload must contain a single JSON value")
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
	Job                *DryRunJobInfo  `json:"job"`
	PayloadHash        string          `json:"payload_hash"`
	Payload            json.RawMessage `json:"payload,omitempty"`
	ScheduledAt        *time.Time      `json:"scheduled_at,omitempty"`
	ExpiresAt          time.Time       `json:"expires_at"`
	ValidationWarnings []string        `json:"validation_warnings,omitempty"`
}

type DryRunJobInfo struct {
	ID            string               `json:"id"`
	Name          string               `json:"name"`
	Slug          string               `json:"slug"`
	ExecutionMode domain.ExecutionMode `json:"execution_mode"`
	Queue         string               `json:"queue,omitempty"`
	TimeoutSecs   int                  `json:"timeout_secs"`
	MaxAttempts   int                  `json:"max_attempts"`
	Version       int                  `json:"version"`
	VersionID     string               `json:"version_id,omitempty"`
}

//nolint:nestif
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

	if job.Paused {
		return nil, errors.New("job is paused -- resume it before triggering new runs")
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
	projectQuota, err = s.quotaCache.Get(ctx, job.ProjectID)
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
		Job:                dryRunJobInfo(job),
		PayloadHash:        payloadHash,
		Payload:            payload,
		ScheduledAt:        scheduledAt,
		ExpiresAt:          expiresAt,
		ValidationWarnings: warnings,
	}, nil
}

func dryRunJobInfo(job *domain.Job) *DryRunJobInfo {
	if job == nil {
		return nil
	}
	return &DryRunJobInfo{
		ID:            job.ID,
		Name:          job.Name,
		Slug:          job.Slug,
		ExecutionMode: job.ExecutionMode,
		Queue:         job.Queue,
		TimeoutSecs:   job.TimeoutSecs,
		MaxAttempts:   job.MaxAttempts,
		Version:       job.Version,
		VersionID:     job.VersionID,
	}
}
