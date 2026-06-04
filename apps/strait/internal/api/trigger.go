package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
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
	TTLSecs        *int              `json:"ttl_secs,omitempty" validate:"omitempty,min=0,max=2592000"`
	ConcurrencyKey string            `json:"concurrency_key,omitempty" validate:"max=256"`
	DebounceKey    string            `json:"debounce_key,omitempty" validate:"max=256"`
	BatchKey       string            `json:"batch_key,omitempty" validate:"max=256"`
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

func (s *Server) handleTriggerJob(ctx context.Context, input *TriggerJobInput) (*TriggerJobOutput, error) {
	job, err := s.loadTriggerJob(ctx, input.JobID)
	if err != nil {
		return nil, err
	}

	req := input.Body
	if err := s.validateTriggerJobInput(input, &req); err != nil {
		return nil, err
	}

	if req.DryRun {
		return s.handleTriggerDryRun(ctx, job.ID, req)
	}

	state, idempotencyHit, err := s.prepareTriggerRequest(ctx, input, job, req)
	if err != nil {
		return nil, err
	}
	if idempotencyHit != nil {
		return nil, idempotencyHit
	}

	if dedupOutput, err := s.triggerDedupOutput(ctx, state); err != nil || dedupOutput != nil {
		return dedupOutput, err
	}
	if debounceOutput, handled, err := s.handleDebounceTrigger(ctx, state); err != nil || handled {
		return debounceOutput, err
	}
	if batchOutput, handled, err := s.handleBatchTrigger(ctx, input, state); err != nil || handled {
		return batchOutput, err
	}

	return s.handleImmediateTrigger(ctx, input, state)
}

type triggerRequestState struct {
	job            *domain.Job
	req            TriggerRequest
	payload        json.RawMessage
	payloadHash    string
	idempotencyKey string
	projectQuota   *store.ProjectQuota
}

func (s *Server) prepareTriggerRequest(
	ctx context.Context,
	input *TriggerJobInput,
	job *domain.Job,
	req TriggerRequest,
) (*triggerRequestState, *rawStatusError, error) {
	if err := validatePayloadAgainstSchema(req.Payload, job.PayloadSchema); err != nil {
		return nil, nil, huma.Error400BadRequest("payload validation failed: " + err.Error())
	}

	payload, payloadHash, err := canonicalizePayload(req.Payload)
	if err != nil {
		return nil, nil, huma.Error400BadRequest("invalid payload: " + err.Error())
	}

	idempotencyKey, err := triggerIdempotencyKey(input)
	if err != nil {
		return nil, nil, err
	}
	idempotencyHit, err := s.triggerIdempotencyHit(ctx, job, idempotencyKey)
	if err != nil {
		return nil, nil, err
	}
	if idempotencyHit != nil {
		return nil, idempotencyHit, nil
	}

	if err := s.checkTriggerDispatchPriority(ctx, job.ProjectID, req.Priority); err != nil {
		return nil, nil, err
	}

	projectQuota, err := s.quotaCache.Get(ctx, job.ProjectID)
	if err != nil {
		return nil, nil, huma.Error500InternalServerError("failed to load project quota")
	}

	if err := s.checkTriggerDailyCostBudget(ctx, job.ProjectID, projectQuota); err != nil {
		return nil, nil, err
	}

	return &triggerRequestState{
		job:            job,
		req:            req,
		payload:        payload,
		payloadHash:    payloadHash,
		idempotencyKey: idempotencyKey,
		projectQuota:   projectQuota,
	}, idempotencyHit, nil
}

func (s *Server) triggerDedupOutput(ctx context.Context, state *triggerRequestState) (*TriggerJobOutput, error) {
	job := state.job
	if job.DedupWindowSecs > 0 {
		existingRun, err := s.findRecentDeduplicatedRun(ctx, job, state.payload)
		if err != nil {
			return nil, huma.Error500InternalServerError("failed to evaluate payload deduplication")
		}
		if existingRun != nil {
			return &TriggerJobOutput{Body: map[string]any{
				"id":              existingRun.ID,
				"status":          existingRun.Status,
				"payload_hash":    state.payloadHash,
				"idempotency_hit": false,
			}}, nil
		}
	}
	return nil, nil
}

func (s *Server) handleDebounceTrigger(ctx context.Context, state *triggerRequestState) (*TriggerJobOutput, bool, error) {
	job := state.job
	req := state.req
	if job.DebounceWindowSecs > 0 {
		pending := newDebouncePending(ctx, debouncePendingRequest{
			job:     job,
			req:     req,
			payload: state.payload,
			now:     time.Now(),
		})
		if err := s.withTriggerLimitGuard(ctx, job, state.projectQuota, func(guardCtx context.Context, _ store.DBTX) error {
			return s.store.UpsertDebouncePending(guardCtx, pending)
		}); err != nil {
			return nil, true, triggerLimitAPIError(err, "failed to upsert debounce pending")
		}
		s.emitAuditEventAsync(ctx, domain.AuditActionJobTriggered, "job", job.ID, map[string]any{
			"debounced":         true,
			"fire_at":           pending.FireAt,
			"priority":          req.Priority,
			"debounce_key_hash": hashIdempotencyKey(req.DebounceKey),
			"tag_keys":          tagKeys(req.Tags),
			"triggered_by":      domain.TriggerDebounce,
		})
		return &TriggerJobOutput{Body: map[string]any{
			"debounced": true,
			"fire_at":   pending.FireAt,
		}}, true, nil
	}
	return nil, false, nil
}

func (s *Server) handleBatchTrigger(ctx context.Context, input *TriggerJobInput, state *triggerRequestState) (*TriggerJobOutput, bool, error) {
	job := state.job
	req := state.req
	if job.BatchWindowSecs > 0 {
		item := newBatchBufferItem(ctx, batchBufferItemRequest{
			job:     job,
			req:     req,
			payload: state.payload,
		})
		var batchOutput *TriggerJobOutput
		var batchRunID string
		if err := s.withTriggerLimitGuard(ctx, job, state.projectQuota, func(guardCtx context.Context, tx store.DBTX) error {
			if err := s.store.InsertBatchBufferItem(guardCtx, item); err != nil {
				return fmt.Errorf("insert batch buffer item: %w", err)
			}

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
			batchRun := newBatchFlushRun(ctx, batchFlushRunRequest{
				input: input,
				job:   job,
				req:   req,
				items: items,
				now:   time.Now(),
			})
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
				return nil, true, apiErr
			}
			return nil, true, triggerLimitAPIError(err, "failed to insert batch buffer item")
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
			return batchOutput, true, nil
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
		}}, true, nil
	}
	return nil, false, nil
}

func (s *Server) handleImmediateTrigger(ctx context.Context, input *TriggerJobInput, state *triggerRequestState) (*TriggerJobOutput, error) {
	job := state.job
	req := state.req
	now := time.Now()
	scheduledAt, err := triggerScheduledAt(job, state.projectQuota, req.ScheduledAt, now)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	run := s.newImmediateTriggerRun(ctx, input, state, immediateTriggerRunConfig{
		scheduledAt: scheduledAt,
		expiresAt:   s.triggerExpiresAt(job, req, scheduledAt, now),
		status:      triggerInitialStatus(scheduledAt, now),
	})

	result, err := s.enqueueImmediateTriggerRun(ctx, state, run)
	if err != nil {
		return nil, err
	}
	if result.deduplicatedRun != nil {
		return triggerRunOutput(result.deduplicatedRun, state.payloadHash, false), nil
	}

	s.emitImmediateTriggerAudit(ctx, job, run, scheduledAt, state.idempotencyKey, result.waitingRun)
	return triggerRunOutput(run, state.payloadHash, false), nil
}

type immediateTriggerRunConfig struct {
	scheduledAt *time.Time
	expiresAt   time.Time
	status      domain.RunStatus
}

func (s *Server) newImmediateTriggerRun(
	ctx context.Context,
	input *TriggerJobInput,
	state *triggerRequestState,
	cfg immediateTriggerRunConfig,
) *domain.JobRun {
	job := state.job
	req := state.req
	metadata := sentryRunMetadata(ctx, triggerJobRoute, nil)
	if dependencyKey := extractDependencyKey(state.payload); dependencyKey != "" {
		metadata["dependency_key"] = dependencyKey
	}

	runTags := mergedRunTags(job.Tags, req.Tags)
	run := &domain.JobRun{
		ID:             uuid.Must(uuid.NewV7()).String(),
		JobID:          job.ID,
		ProjectID:      job.ProjectID,
		Tags:           runTags,
		Status:         cfg.status,
		Attempt:        1,
		Payload:        state.payload,
		TriggeredBy:    domain.TriggerManual,
		ScheduledAt:    cfg.scheduledAt,
		Priority:       req.Priority,
		IdempotencyKey: state.idempotencyKey,
		JobVersion:     job.Version,
		JobVersionID:   job.VersionID,
		CreatedBy:      actorFromContext(ctx),
		ExpiresAt:      &cfg.expiresAt,
		ExecutionMode:  job.ExecutionMode,
		QueueName:      job.Queue,
		IsRollback:     false,
		Metadata:       metadata,
	}
	run.Metadata = mergeRunMetadata(run.Metadata, job.DefaultRunMetadata)
	run.ConcurrencyKey = req.ConcurrencyKey
	run.Metadata = applyRunTraceHeaderMetadata(
		run.Metadata,
		input.Traceparent,
		input.Tracestate,
		input.SentryTrace,
		input.Baggage,
	)
	return run
}

type immediateTriggerResult struct {
	waitingRun      bool
	deduplicatedRun *domain.JobRun
}

func (s *Server) enqueueImmediateTriggerRun(
	ctx context.Context,
	state *triggerRequestState,
	run *domain.JobRun,
) (*immediateTriggerResult, error) {
	job := state.job
	result := &immediateTriggerResult{}
	initialStatus := run.Status
	if err := s.withTriggerLimitGuard(ctx, job, state.projectQuota, func(guardCtx context.Context, tx store.DBTX) error {
		if job.DedupWindowSecs > 0 {
			existingRun, findErr := s.findRecentDeduplicatedRun(guardCtx, job, state.payload)
			if findErr != nil {
				return fmt.Errorf("evaluate payload deduplication: %w", findErr)
			}
			if existingRun != nil {
				result.deduplicatedRun = existingRun
				return nil
			}
		}
		if initialStatus == domain.StatusQueued {
			satisfied, depErr := s.store.AreJobDependenciesSatisfied(guardCtx, run)
			if depErr != nil {
				return fmt.Errorf("evaluate job dependencies: %w", depErr)
			}
			if !satisfied {
				run.Status = domain.StatusWaiting
				result.waitingRun = true
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
		if idempotencyErr := s.resolveTriggerIdempotencyConflict(ctx, job, state.idempotencyKey, err); idempotencyErr != nil {
			return nil, idempotencyErr
		}
		if apiErr := enqueueAPIError(err); apiErr != nil {
			return nil, apiErr
		}
		return nil, triggerLimitAPIError(err, "failed to enqueue run")
	}
	return result, nil
}

func (s *Server) emitImmediateTriggerAudit(
	ctx context.Context,
	job *domain.Job,
	run *domain.JobRun,
	scheduledAt *time.Time,
	idempotencyKey string,
	waitingRun bool,
) {
	details := map[string]any{
		"run_id":               run.ID,
		"scheduled_at":         scheduledAt,
		"priority":             run.Priority,
		"idempotency_key_hash": hashIdempotencyKey(idempotencyKey),
		"tag_keys":             tagKeys(run.Tags),
		"triggered_by":         run.TriggeredBy,
	}
	if waitingRun {
		details["waiting"] = true
	}
	s.emitAuditEventAsync(ctx, domain.AuditActionJobTriggered, "job", job.ID, details)
}

func triggerRunOutput(run *domain.JobRun, payloadHash string, idempotencyHit bool) *TriggerJobOutput {
	return &TriggerJobOutput{Body: map[string]any{
		"id":              run.ID,
		"status":          run.Status,
		"payload_hash":    payloadHash,
		"idempotency_hit": idempotencyHit,
	}}
}

func (s *Server) loadTriggerJob(ctx context.Context, jobID string) (*domain.Job, error) {
	job, err := s.loadRunCreationJob(ctx, jobID, "trigger_job.project_match", "handleTriggerJob")
	if err != nil {
		return nil, err
	}
	if err := ensureJobTriggerable(job); err != nil {
		return nil, err
	}
	return job, nil
}

func (s *Server) loadRunCreationJob(ctx context.Context, jobID, auditAction, handlerName string) (*domain.Job, error) {
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
	s.emitInternalSecretBypassAuditIfProjectless(ctx, auditAction, handlerName, "job", job.ID)
	return job, nil
}

func (s *Server) validateTriggerJobInput(input *TriggerJobInput, req *TriggerRequest) error {
	if err := s.validate.Struct(req); err != nil {
		return newValidationError(err)
	}
	if err := validateTriggerTraceHeaders(input); err != nil {
		return huma.Error400BadRequest(err.Error())
	}
	if err := validatePayloadSize(req.Payload); err != nil {
		return huma.Error400BadRequest(err.Error())
	}
	if err := validateTags(req.Tags); err != nil {
		return huma.Error400BadRequest(err.Error())
	}
	if err := validateTriggerScheduledAt(req.ScheduledAt); err != nil {
		return huma.Error400BadRequest(err.Error())
	}
	return nil
}

func (s *Server) handleTriggerDryRun(ctx context.Context, jobID string, req TriggerRequest) (*TriggerJobOutput, error) {
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

func triggerIdempotencyKey(input *TriggerJobInput) (string, error) {
	idempotencyKey := input.XIdempotencyKey
	if idempotencyKey == "" {
		idempotencyKey = input.IdempotencyKeyAlt
	}
	if len(idempotencyKey) > maxIdempotencyKeyLength {
		return "", huma.Error400BadRequest(
			fmt.Sprintf("idempotency key must be %d characters or fewer", maxIdempotencyKeyLength))
	}
	return idempotencyKey, nil
}

func (s *Server) triggerIdempotencyHit(ctx context.Context, job *domain.Job, idempotencyKey string) (*rawStatusError, error) {
	if idempotencyKey == "" {
		return nil, nil
	}

	existingRun, err := s.store.GetRunByIdempotencyKey(ctx, job.ID, idempotencyKey)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to check idempotency key")
	}
	if existingRun == nil {
		return nil, nil
	}

	idempotencyKeyHash := hashIdempotencyKey(idempotencyKey)
	slog.Info("idempotency hit",
		"job_id", job.ID,
		"idempotency_key_hash", idempotencyKeyHash,
		"existing_run_id", existingRun.ID,
		"existing_run_status", existingRun.Status)
	return &rawStatusError{status: http.StatusOK, body: map[string]any{
		"id":              existingRun.ID,
		"status":          existingRun.Status,
		"idempotency_hit": true,
	}}, nil
}

func (s *Server) resolveTriggerIdempotencyConflict(ctx context.Context, job *domain.Job, idempotencyKey string, err error) error {
	if !errors.Is(err, domain.ErrIdempotencyConflict) || idempotencyKey == "" {
		return nil
	}

	// The unique index is the final idempotency boundary when concurrent
	// requests pass the app-level lookup at the same time.
	existingRun, retryErr := s.store.GetRunByIdempotencyKey(ctx, job.ID, idempotencyKey)
	if retryErr != nil {
		slog.Error("idempotency conflict retry failed",
			"job_id", job.ID,
			"idempotency_key_hash", hashIdempotencyKey(idempotencyKey),
			"error", retryErr)
		return huma.Error500InternalServerError("failed to check idempotency key after conflict")
	}
	if existingRun == nil {
		slog.Error("idempotency conflict retry returned nil",
			"job_id", job.ID,
			"idempotency_key_hash", hashIdempotencyKey(idempotencyKey))
		return nil
	}

	slog.Warn("idempotency conflict resolved",
		"job_id", job.ID,
		"idempotency_key_hash", hashIdempotencyKey(idempotencyKey),
		"winning_run_id", existingRun.ID)
	return &rawStatusError{status: http.StatusOK, body: map[string]any{
		"id":              existingRun.ID,
		"status":          existingRun.Status,
		"idempotency_hit": true,
	}}
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

func (s *Server) findRecentDeduplicatedRun(ctx context.Context, job *domain.Job, payload json.RawMessage) (*domain.JobRun, error) {
	if job == nil || job.DedupWindowSecs <= 0 {
		return nil, nil
	}
	since := time.Now().Add(-time.Duration(job.DedupWindowSecs) * time.Second)
	return s.store.FindRecentRunByPayload(ctx, job.ID, payload, since)
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

// hashIdempotencyKey returns a short SHA-256 prefix of the idempotency key,
// safe for audit logs. Raw keys are never recorded.
func hashIdempotencyKey(key string) string {
	if key == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])[:16]
}

func (s *Server) enqueueTriggerRun(ctx context.Context, tx store.DBTX, run *domain.JobRun) error {
	if tx != nil {
		return s.queue.EnqueueInTx(ctx, tx, run)
	}
	return s.queue.Enqueue(ctx, run)
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

	projectQuota, err := s.quotaCache.Get(ctx, job.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("load project quota: %w", err)
	}

	if err := s.validateDryRunProjectQuota(ctx, job, projectQuota); err != nil {
		return nil, err
	}

	if err := s.checkTriggerDispatchPriority(ctx, job.ProjectID, req.Priority); err != nil {
		return nil, err
	}

	if err := s.validateDryRunJobRateLimit(ctx, job); err != nil {
		return nil, err
	}

	warnings, err := s.dryRunValidationWarnings(ctx, job, payload)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	scheduledAt, err := triggerScheduledAt(job, projectQuota, req.ScheduledAt, now)
	if err != nil {
		return nil, err
	}
	expiresAt := s.triggerExpiresAt(job, req, scheduledAt, now)

	return &DryRunValidationResult{
		Job:                dryRunJobInfo(job),
		PayloadHash:        payloadHash,
		Payload:            payload,
		ScheduledAt:        scheduledAt,
		ExpiresAt:          expiresAt,
		ValidationWarnings: warnings,
	}, nil
}

func (s *Server) validateDryRunProjectQuota(ctx context.Context, job *domain.Job, projectQuota *store.ProjectQuota) error {
	if projectQuota == nil {
		return nil
	}
	if err := s.checkTriggerDailyCostBudget(ctx, job.ProjectID, projectQuota); err != nil {
		return err
	}
	if projectQuota.MaxQueuedRuns > 0 {
		queuedRuns, err := s.store.CountProjectQueuedRuns(ctx, job.ProjectID)
		if err != nil {
			return fmt.Errorf("evaluate project queued quota: %w", err)
		}
		if queuedRuns >= projectQuota.MaxQueuedRuns {
			return errors.New("project queued quota exceeded")
		}
	}
	if projectQuota.MaxExecutingRuns > 0 {
		activeRuns, err := s.store.CountProjectActiveRuns(ctx, job.ProjectID)
		if err != nil {
			return fmt.Errorf("evaluate project active quota: %w", err)
		}
		if activeRuns >= projectQuota.MaxExecutingRuns {
			return errors.New("project executing quota exceeded")
		}
	}
	return nil
}

func (s *Server) validateDryRunJobRateLimit(ctx context.Context, job *domain.Job) error {
	if job.RateLimitMax <= 0 || job.RateLimitWindowSecs <= 0 {
		return nil
	}
	since := time.Now().Add(-time.Duration(job.RateLimitWindowSecs) * time.Second)
	runCount, err := s.store.CountRunsForJobSince(ctx, job.ID, since)
	if err != nil {
		return fmt.Errorf("evaluate job rate limit: %w", err)
	}
	if runCount >= job.RateLimitMax {
		return errors.New("job rate limit exceeded")
	}
	return nil
}

func (s *Server) dryRunValidationWarnings(ctx context.Context, job *domain.Job, payload json.RawMessage) ([]string, error) {
	warnings := []string{}
	if job.DedupWindowSecs <= 0 {
		return warnings, nil
	}
	since := time.Now().Add(-time.Duration(job.DedupWindowSecs) * time.Second)
	existingRun, err := s.store.FindRecentRunByPayload(ctx, job.ID, payload, since)
	if err != nil {
		return nil, fmt.Errorf("evaluate payload deduplication: %w", err)
	}
	if existingRun != nil {
		warnings = append(warnings, fmt.Sprintf("payload deduplication: run %s", existingRun.ID))
	}
	return warnings, nil
}
