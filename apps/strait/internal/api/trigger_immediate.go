package api

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	otelattr "go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
)

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
	s.emitAuditEventAsync(auditContextWithProject(ctx, job.ProjectID), domain.AuditActionJobTriggered, "job", job.ID, details)
}

func triggerRunOutput(run *domain.JobRun, payloadHash string, idempotencyHit bool) *TriggerJobOutput {
	return &TriggerJobOutput{Body: map[string]any{
		"id":              run.ID,
		"status":          run.Status,
		"payload_hash":    payloadHash,
		"idempotency_hit": idempotencyHit,
	}}
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
