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
	// SingletonKey overrides the resolved singleton lock key for this trigger,
	// bypassing the job's key-expression template. Ignored when the job is not a
	// singleton. Empty means resolve from the configured template.
	SingletonKey string `json:"singleton_key,omitempty" validate:"max=2048"`
}

const maxTriggerTTLSecs = 30 * 24 * 60 * 60

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
	if err := validateTriggerScheduledAt(req.ScheduledAt); err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	// Handle dry-run mode
	if req.DryRun {
		result, err := s.validateTriggerRequest(ctx, jobID, req)
		if err != nil {
			var statusErr huma.StatusError
			if errors.As(err, &statusErr) {
				return nil, statusErr
			}
			return nil, huma.Error400BadRequest(err.Error())
		}
		return nil, &rawStatusError{status: http.StatusOK, body: result}
	}
	if err := validatePayloadAgainstSchema(req.Payload, job.PayloadSchema); err != nil {
		return nil, huma.Error400BadRequest("payload validation failed: " + err.Error())
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

	if err := s.checkTriggerDispatchPriority(ctx, job.ProjectID, req.Priority); err != nil {
		return nil, err
	}

	var projectQuota *store.ProjectQuota
	projectQuota, err = s.quotaCache.Get(ctx, job.ProjectID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to load project quota")
	}

	if err := s.checkTriggerDailyCostBudget(ctx, job.ProjectID, projectQuota); err != nil {
		return nil, err
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
		s.emitAuditEventAsync(ctx, domain.AuditActionJobTriggered, "job", job.ID, map[string]any{
			"debounced":         true,
			"fire_at":           fireAt,
			"priority":          req.Priority,
			"debounce_key_hash": hashIdempotencyKey(req.DebounceKey),
			"tag_keys":          tagKeys(req.Tags),
			"triggered_by":      domain.TriggerDebounce,
		})
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
		var batchRunID string
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
			batchRunID = batchRun.ID
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
			s.emitAuditEventAsync(ctx, domain.AuditActionJobTriggered, "job", job.ID, map[string]any{
				"run_id":           batchRunID,
				"batch":            true,
				"priority":         req.Priority,
				"batch_key_hash":   hashIdempotencyKey(req.BatchKey),
				"tag_keys":         tagKeys(req.Tags),
				"triggered_by":     "batch",
				"batch_max_size":   job.BatchMaxSize,
				"batch_window_sec": job.BatchWindowSecs,
			})
			return batchOutput, nil
		}

		s.emitAuditEventAsync(ctx, domain.AuditActionJobTriggered, "job", job.ID, map[string]any{
			"buffered":         true,
			"priority":         req.Priority,
			"batch_key_hash":   hashIdempotencyKey(req.BatchKey),
			"tag_keys":         tagKeys(req.Tags),
			"triggered_by":     "batch_buffer",
			"batch_window_sec": job.BatchWindowSecs,
		})
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

	singletonConfigured := job.SingletonOnConflict != ""
	if singletonConfigured {
		key, keyErr := resolveJobSingletonKey(job, req.SingletonKey, payload)
		if keyErr != nil {
			return nil, keyErr
		}
		run.SingletonKey = key
	}

	waitingRun := false
	var singletonOutcome domain.SingletonOutcome
	var singletonHolderRunID string
	if err := s.withTriggerLimitGuard(ctx, job, projectQuota, func(guardCtx context.Context, tx store.DBTX) error {
		if singletonConfigured {
			proceed, outcome, holderID, serr := s.applyJobSingletonPolicy(guardCtx, tx, job, run, run.SingletonKey)
			if serr != nil {
				return serr
			}
			singletonOutcome = outcome
			singletonHolderRunID = holderID
			if !proceed {
				// drop / queue / replace: applyJobSingletonPolicy already parked or
				// discarded the run. Nothing left to enqueue.
				return nil
			}
			// dispatched: the run acquired the key; fall through to the normal
			// dependency-check and enqueue path holding the lock.
		}
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
				if singletonConfigured {
					// Keep the parked insert atomic with the lock acquired above.
					return store.New(tx).CreateRun(guardCtx, run)
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

	if singletonConfigured {
		s.emitAuditEventAsync(ctx, domain.AuditActionJobTriggered, "job", job.ID, map[string]any{
			"run_id":             run.ID,
			"priority":           req.Priority,
			"tag_keys":           tagKeys(runTags),
			"triggered_by":       run.TriggeredBy,
			"singleton_key_hash": hashIdempotencyKey(run.SingletonKey),
			"singleton_outcome":  string(singletonOutcome),
		})
		// drop creates no run: report the outcome and the holder it lost to.
		if singletonOutcome == domain.SingletonOutcomeDropped {
			body := map[string]any{
				"singleton_outcome": string(singletonOutcome),
				"idempotency_hit":   false,
			}
			if singletonHolderRunID != "" {
				body["singleton_holder_run_id"] = singletonHolderRunID
			}
			return &TriggerJobOutput{Body: body}, nil
		}
		body := map[string]any{
			"id":                run.ID,
			"status":            run.Status,
			"payload_hash":      payloadHash,
			"idempotency_hit":   false,
			"singleton_outcome": string(singletonOutcome),
		}
		if singletonHolderRunID != "" {
			body["singleton_holder_run_id"] = singletonHolderRunID
		}
		return &TriggerJobOutput{Body: body}, nil
	}

	if waitingRun {
		s.emitAuditEventAsync(ctx, domain.AuditActionJobTriggered, "job", job.ID, map[string]any{
			"run_id":               run.ID,
			"scheduled_at":         scheduledAt,
			"priority":             req.Priority,
			"idempotency_key_hash": hashIdempotencyKey(idempotencyKey),
			"tag_keys":             tagKeys(runTags),
			"triggered_by":         run.TriggeredBy,
			"waiting":              true,
		})
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

func (s *Server) checkTriggerDispatchPriority(ctx context.Context, projectID string, priority int) error {
	if s.billingEnforcer == nil || priority <= 0 {
		return nil
	}
	if err := s.billingEnforcer.CheckMaxDispatchPriority(ctx, projectID, priority); err != nil {
		var rse *rawStatusError
		if converted := limitErrorTo402(err, ""); converted != nil && errors.As(converted, &rse) {
			return converted
		}
		return huma.Error402PaymentRequired(err.Error())
	}
	return nil
}

func (s *Server) checkTriggerDailyCostBudget(ctx context.Context, projectID string, projectQuota *store.ProjectQuota) error {
	if projectQuota == nil || projectQuota.MaxDailyCostMicrousd <= 0 {
		return nil
	}
	tz := projectQuota.Timezone
	if tz == "" {
		tz = "UTC"
	}
	dailyCost, err := s.store.SumProjectDailyCostMicrousd(ctx, projectID, tz)
	if err != nil {
		return huma.Error500InternalServerError(fmt.Sprintf("failed to evaluate daily cost budget (timezone: %s)", tz))
	}
	if dailyCost >= projectQuota.MaxDailyCostMicrousd {
		return huma.Error429TooManyRequests("project daily cost budget exceeded")
	}
	return nil
}

func validateTriggerScheduledAt(scheduledAt *time.Time) error {
	if scheduledAt == nil {
		return nil
	}
	delay := time.Until(*scheduledAt)
	if delay < 0 {
		return errors.New("scheduled_at must not be in the past")
	}
	if delay > 30*24*time.Hour {
		return errors.New("scheduled_at cannot exceed 30 days from now")
	}
	return nil
}

func validateTriggerTTLSecs(ttlSecs *int) error {
	if ttlSecs == nil {
		return nil
	}
	if *ttlSecs < 0 {
		return errors.New("ttl_secs must be greater than or equal to 0")
	}
	if *ttlSecs > maxTriggerTTLSecs {
		return errors.New("ttl_secs cannot exceed 30 days")
	}
	return nil
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

// resolveJobSingletonKey produces the lock key for a singleton job trigger. An
// explicit per-trigger override wins over the configured template (already
// length-bounded by the request validator). Otherwise the job's key expression
// is interpolated against the canonicalized payload. An unresolvable template
// maps to 400; a malformed stored expression maps to 500.
func resolveJobSingletonKey(job *domain.Job, override string, payload json.RawMessage) (string, error) {
	if override != "" {
		return override, nil
	}
	expr, err := domain.ParseSingletonKeyExpr(job.SingletonKeyExpr)
	if err != nil {
		return "", huma.Error500InternalServerError("invalid singleton key expression")
	}
	key, rerr := domain.ResolveSingletonKey(expr, payload)
	if rerr != nil {
		if errors.Is(rerr, domain.ErrSingletonKeyUnresolvable) {
			return "", huma.Error400BadRequest(rerr.Error())
		}
		return "", huma.Error500InternalServerError("failed to resolve singleton key")
	}
	if key == "" {
		return "", huma.Error400BadRequest("singleton key resolved to an empty value")
	}
	return key, nil
}

// applyJobSingletonPolicy claims the resolved key for run inside the trigger
// transaction. It returns proceed=true only when the key was acquired
// (dispatched), so the caller continues to the normal enqueue path with the lock
// held. On conflict it applies the job's on-conflict policy in-place (parking,
// dropping, or replacing) and returns proceed=false with the resulting outcome
// and the relevant holder run id.
func (s *Server) applyJobSingletonPolicy(ctx context.Context, tx store.DBTX, job *domain.Job, run *domain.JobRun, key string) (bool, domain.SingletonOutcome, string, error) {
	txQ := store.New(tx)

	// On conflict we serialize the rest of the decision behind a FOR UPDATE lock
	// on the holder row so the queue-depth check and park cannot interleave with
	// another waiter. The enclosing trigger transaction already holds a per-project
	// advisory lock, but pinning the holder keeps the cap correct even if that
	// guard ever changes and mirrors the workflow path. If the holder is released
	// in the narrow window between our acquire attempt and that lock, the key is
	// free again and we retry the acquire; the bound guards against a pathological
	// acquire/release storm livelocking the transaction.
	const maxAcquireAttempts = 8
	for attempt := 1; ; attempt++ {
		// Acquire with a NULL lease. The lock is taken at trigger time, but the
		// holder run does not start executing until a worker dequeues it, which can
		// be much later than StaleThreshold under load. Stamping a lease here would
		// let it expire while the run still sits queued/dequeued, so the reaper would
		// reclaim the key and promote a waiter while the original holder is about to
		// run -> double execution. Instead the lease is set by the first heartbeat
		// once the holder is actually executing (see BatchUpdateHeartbeat); until
		// then the holder is protected by the run-status stale checks, not the lease.
		acquired, _, err := txQ.AcquireSingletonLock(ctx, domain.SingletonLock{
			ProjectID:   job.ProjectID,
			Kind:        domain.SingletonKindJob,
			OwnerID:     job.ID,
			LockKey:     key,
			HolderRunID: run.ID,
			LeaseUntil:  nil,
		})
		if err != nil {
			return false, "", "", fmt.Errorf("acquire singleton lock: %w", err)
		}
		if acquired {
			s.recordSingletonAcquisition(ctx, domain.SingletonKindJob)
			return true, domain.SingletonOutcomeDispatched, "", nil
		}

		// Lost the acquire race: pin the holder row for the rest of this
		// transaction before reading the waiter count or parking.
		holder, lerr := txQ.LockSingletonHolderForUpdate(ctx, job.ProjectID, domain.SingletonKindJob, job.ID, key)
		if errors.Is(lerr, store.ErrSingletonLockNotFound) {
			if attempt >= maxAcquireAttempts {
				return false, "", "", fmt.Errorf("acquire singleton lock: key %q churned without a stable holder after %d attempts", key, attempt)
			}
			continue // key freed under us; retry the acquire
		}
		if lerr != nil {
			return false, "", "", fmt.Errorf("lock singleton holder: %w", lerr)
		}
		holderID := holder.HolderRunID

		s.recordSingletonConflict(ctx, domain.SingletonKindJob, job.SingletonOnConflict)

		switch job.SingletonOnConflict {
		case domain.SingletonOnConflictDrop:
			return false, domain.SingletonOutcomeDropped, holderID, nil

		case domain.SingletonOnConflictQueue:
			waiters, cerr := txQ.CountSingletonWaiters(ctx, domain.SingletonKindJob, job.ID, key)
			if cerr != nil {
				return false, "", "", fmt.Errorf("count singleton waiters: %w", cerr)
			}
			if job.SingletonMaxQueueDepth != nil && waiters >= *job.SingletonMaxQueueDepth {
				return false, domain.SingletonOutcomeDropped, holderID, nil
			}
			run.Status = domain.StatusWaiting
			if cerr := txQ.CreateRun(ctx, run); cerr != nil {
				return false, "", "", fmt.Errorf("park singleton run: %w", cerr)
			}
			return false, domain.SingletonOutcomeQueuedBehind, holderID, nil

		case domain.SingletonOnConflictReplace:
			// Discard any waiters already parked behind the holder so the
			// just-triggered run becomes the sole successor (keep newest).
			if _, cerr := txQ.CancelSingletonJobWaiters(ctx, job.ID, key, "superseded by singleton replace"); cerr != nil {
				return false, "", "", fmt.Errorf("cancel singleton waiters: %w", cerr)
			}
			if holderID != "" {
				if cerr := cancelSingletonHolderJob(ctx, txQ, holderID); cerr != nil {
					return false, "", "", fmt.Errorf("cancel singleton holder: %w", cerr)
				}
			}
			// Park the newcomer; it acquires the key when the canceled holder's
			// terminal transition releases and promotes it (Phase 3 fast-path/reaper).
			run.Status = domain.StatusWaiting
			if cerr := txQ.CreateRun(ctx, run); cerr != nil {
				return false, "", "", fmt.Errorf("park singleton replacement run: %w", cerr)
			}
			return false, domain.SingletonOutcomeReplaced, holderID, nil

		default:
			return false, "", "", fmt.Errorf("unknown singleton on-conflict policy %q", job.SingletonOnConflict)
		}
	}
}

// recordSingletonAcquisition increments the acquisitions counter for a key
// claimed at trigger time, labeled by kind. Nil-safe for tests/metric-less runs.
func (s *Server) recordSingletonAcquisition(ctx context.Context, kind domain.SingletonKind) {
	if s.metrics == nil || s.metrics.SingletonAcquisitions == nil {
		return
	}
	s.metrics.SingletonAcquisitions.Add(ctx, 1, otelmetric.WithAttributes(
		otelattr.String("kind", string(kind)),
	))
}

// recordSingletonConflict increments the conflicts counter when a trigger loses
// the race for a key, labeled by kind and the on-conflict policy applied.
func (s *Server) recordSingletonConflict(ctx context.Context, kind domain.SingletonKind, policy domain.SingletonOnConflict) {
	if s.metrics == nil || s.metrics.SingletonConflicts == nil {
		return
	}
	s.metrics.SingletonConflicts.Add(ctx, 1, otelmetric.WithAttributes(
		otelattr.String("kind", string(kind)),
		otelattr.String("policy", string(policy)),
	))
}

// cancelSingletonHolderJob transitions the current holder job-run to canceled so
// the replace newcomer can take the key. A missing or already-terminal holder is
// a no-op: the reaper reclaims orphaned locks.
func cancelSingletonHolderJob(ctx context.Context, txQ *store.Queries, holderID string) error {
	st, err := txQ.GetRunStatus(ctx, holderID)
	if err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			return nil
		}
		return err
	}
	if st.IsTerminal() {
		return nil
	}
	return txQ.UpdateRunStatus(ctx, holderID, st, domain.StatusCanceled, map[string]any{
		"finished_at": time.Now(),
		"error":       "canceled by singleton replace policy",
	})
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

//nolint:cyclop,gocyclo,nestif
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

	if err := validateTriggerScheduledAt(req.ScheduledAt); err != nil {
		return nil, err
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
		if err := s.checkTriggerDailyCostBudget(ctx, job.ProjectID, projectQuota); err != nil {
			return nil, err
		}

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

	if err := s.checkTriggerDispatchPriority(ctx, job.ProjectID, req.Priority); err != nil {
		return nil, err
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
