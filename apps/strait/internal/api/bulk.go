package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/danielgtaylor/huma/v2"
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

func (s *Server) handleBulkTriggerJob(ctx context.Context, input *BulkTriggerJobInput) (*BulkTriggerJobOutput, error) {
	job, err := s.loadBulkTriggerJob(ctx, input.JobID)
	if err != nil {
		return nil, err
	}

	req := input.Body
	if err := s.validateBulkTriggerRequest(req); err != nil {
		return nil, err
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

	projectQuota, err := s.quotaCache.Get(ctx, job.ProjectID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to load project quota")
	}

	state := newBulkTriggerState(s, job, projectQuota, batchID, req.Items, now)
	if err := s.withTriggerLimitGuard(ctx, job, projectQuota, func(guardCtx context.Context, tx store.DBTX) error {
		state.ctx = guardCtx
		state.tx = tx
		if err := state.loadAdmissionCounters(); err != nil {
			return err
		}

		for _, item := range req.Items {
			if err := state.processItem(item); err != nil {
				return err
			}
		}
		return state.enqueuePendingRuns()
	}); err != nil {
		return nil, triggerLimitAPIError(err, "failed to enqueue bulk trigger")
	}

	if err := s.store.FinalizeBatchOperation(ctx, batchID, state.created); err != nil {
		slog.Error("failed to finalize batch operation", "batch_id", batchID, "error", err)
	}

	s.emitAuditEventAsync(ctx, domain.AuditActionJobBulkTriggered, "job", job.ID, map[string]any{
		"batch_id": batchID,
		"total":    len(req.Items),
		"created":  state.created,
	})

	return &BulkTriggerJobOutput{
		Body: BulkTriggerResponse{
			BatchID: batchID,
			Results: state.results,
			Total:   len(req.Items),
			Created: state.created,
		},
	}, nil
}

// bulkTriggerState keeps the mutable counters and response rows that must
// move together while the trigger limit guard is held.
type bulkTriggerState struct {
	server             *Server
	ctx                context.Context
	tx                 store.DBTX
	job                *domain.Job
	projectQuota       *store.ProjectQuota
	batchID            string
	now                time.Time
	hasIdempotencyKey  bool
	queuedRuns         int
	activeRuns         int
	enqueuedInBatch    int
	dailyBudgetChecked bool
	pendingRuns        []*domain.JobRun
	results            []BulkTriggerResult
	created            int
}

func newBulkTriggerState(
	s *Server,
	job *domain.Job,
	projectQuota *store.ProjectQuota,
	batchID string,
	items []BulkTriggerItem,
	now time.Time,
) *bulkTriggerState {
	return &bulkTriggerState{
		server:            s,
		job:               job,
		projectQuota:      projectQuota,
		batchID:           batchID,
		now:               now,
		hasIdempotencyKey: bulkHasIdempotencyKey(items),
		pendingRuns:       make([]*domain.JobRun, 0, len(items)),
		results:           make([]BulkTriggerResult, 0, len(items)),
	}
}

func (b *bulkTriggerState) loadAdmissionCounters() error {
	if b.projectQuota == nil {
		return nil
	}
	if b.projectQuota.MaxQueuedRuns > 0 {
		count, err := b.countProjectQueuedRuns()
		if err != nil {
			return huma.Error500InternalServerError("failed to count project queued runs")
		}
		b.queuedRuns = count
	}
	if b.projectQuota.MaxExecutingRuns > 0 {
		count, err := b.countProjectActiveRuns()
		if err != nil {
			return huma.Error500InternalServerError("failed to count project active runs")
		}
		b.activeRuns = count
	}
	return nil
}

func (b *bulkTriggerState) countProjectQueuedRuns() (int, error) {
	if b.tx != nil {
		return countProjectQueuedRuns(b.ctx, b.tx, b.job.ProjectID)
	}
	return b.server.store.CountProjectQueuedRuns(b.ctx, b.job.ProjectID)
}

func (b *bulkTriggerState) countProjectActiveRuns() (int, error) {
	if b.tx != nil {
		return countProjectActiveRuns(b.ctx, b.tx, b.job.ProjectID)
	}
	return b.server.store.CountProjectActiveRuns(b.ctx, b.job.ProjectID)
}

func (b *bulkTriggerState) processItem(item BulkTriggerItem) error {
	itemIdx := len(b.results)
	if err := b.server.validateBulkTriggerItem(b.job, item, itemIdx); err != nil {
		return err
	}

	payload, _, err := canonicalizePayload(item.Payload)
	if err != nil {
		return huma.Error400BadRequest(fmt.Sprintf("invalid payload for item %d: %v", itemIdx, err))
	}
	if hit, err := b.appendIdempotencyHit(item, itemIdx); err != nil || hit {
		return err
	}
	if err := b.checkPriority(item, itemIdx); err != nil {
		return err
	}
	if err := b.checkAdmissionLimits(); err != nil {
		return err
	}
	if hit, err := b.appendDedupHit(payload); err != nil || hit {
		return err
	}
	if err := b.checkDailyBudgetOnce(); err != nil {
		return err
	}

	scheduledAt, err := b.resolveScheduledAt(item)
	if err != nil {
		return err
	}
	run := newBulkTriggerRun(b.ctx, b.job, item, payload, b.batchID, b.now, scheduledAt)
	handled, err := b.enqueueOrBufferRun(run, item, itemIdx)
	if err != nil || handled {
		return err
	}
	b.appendCreatedRun(run)
	return nil
}

func (b *bulkTriggerState) appendIdempotencyHit(item BulkTriggerItem, itemIdx int) (bool, error) {
	if item.IdempotencyKey == "" {
		return false, nil
	}
	if len(item.IdempotencyKey) > maxIdempotencyKeyLength {
		return false, huma.Error400BadRequest(
			fmt.Sprintf("idempotency key for item %d must be %d characters or fewer", itemIdx, maxIdempotencyKeyLength))
	}

	existingRun, err := b.server.store.GetRunByIdempotencyKey(b.ctx, b.job.ID, item.IdempotencyKey)
	if err != nil {
		return false, huma.Error500InternalServerError(fmt.Sprintf("failed to check idempotency key for item %d", itemIdx))
	}
	if existingRun == nil {
		return false, nil
	}
	slog.Info("idempotency hit",
		"job_id", b.job.ID,
		"idempotency_key_hash", hashIdempotencyKey(item.IdempotencyKey),
		"existing_run_id", existingRun.ID,
		"existing_run_status", existingRun.Status,
		"item_index", itemIdx)
	b.appendExistingRun(existingRun, true)
	return true, nil
}

func (b *bulkTriggerState) checkPriority(item BulkTriggerItem, itemIdx int) error {
	if item.Priority <= 0 {
		return nil
	}
	if !b.server.edition.RequiresHTTPModeGating() {
		return nil
	}
	if b.server.billingEnforcer == nil {
		return planGateUnavailable("bulk_dispatch_priority_enforcer", errors.New("billing enforcer not configured"))
	}
	if err := b.server.billingEnforcer.CheckMaxDispatchPriority(b.ctx, b.job.ProjectID, item.Priority); err != nil {
		var rse *rawStatusError
		if converted := limitErrorTo402(err, fmt.Sprintf("item %d", itemIdx)); converted != nil && errors.As(converted, &rse) {
			return converted
		}
		return huma.Error402PaymentRequired(fmt.Sprintf("item %d: %v", itemIdx, err))
	}
	return nil
}

func (b *bulkTriggerState) checkAdmissionLimits() error {
	if b.projectQuota != nil {
		if b.projectQuota.MaxQueuedRuns > 0 && (b.queuedRuns+b.enqueuedInBatch) >= b.projectQuota.MaxQueuedRuns {
			return huma.Error429TooManyRequests("project queued quota exceeded")
		}
		if b.projectQuota.MaxExecutingRuns > 0 && b.activeRuns >= b.projectQuota.MaxExecutingRuns {
			return huma.Error429TooManyRequests("project executing quota exceeded")
		}
	}
	if b.job.RateLimitMax <= 0 || b.job.RateLimitWindowSecs <= 0 {
		return nil
	}
	since := time.Now().Add(-time.Duration(b.job.RateLimitWindowSecs) * time.Second)
	var (
		runCount int
		err      error
	)
	if b.tx != nil {
		runCount, err = countRunsForJobSince(b.ctx, b.tx, b.job.ID, since)
	} else {
		runCount, err = b.server.store.CountRunsForJobSince(b.ctx, b.job.ID, since)
	}
	if err != nil {
		return huma.Error500InternalServerError("failed to evaluate job rate limit")
	}
	if runCount+b.enqueuedInBatch >= b.job.RateLimitMax {
		return huma.Error429TooManyRequests("job rate limit exceeded")
	}
	return nil
}

func (b *bulkTriggerState) appendDedupHit(payload json.RawMessage) (bool, error) {
	if b.job.DedupWindowSecs <= 0 {
		return false, nil
	}
	since := time.Now().Add(-time.Duration(b.job.DedupWindowSecs) * time.Second)
	existingRun, err := b.server.store.FindRecentRunByPayload(b.ctx, b.job.ID, payload, since)
	if err != nil {
		return false, huma.Error500InternalServerError("failed to evaluate payload deduplication")
	}
	if existingRun == nil {
		return false, nil
	}
	b.appendExistingRun(existingRun, false)
	return true, nil
}

func (b *bulkTriggerState) checkDailyBudgetOnce() error {
	if b.dailyBudgetChecked {
		return nil
	}
	if err := b.server.checkTriggerDailyCostBudget(b.ctx, b.job.ProjectID, b.projectQuota); err != nil {
		return err
	}
	b.dailyBudgetChecked = true
	return nil
}

func (b *bulkTriggerState) resolveScheduledAt(item BulkTriggerItem) (*time.Time, error) {
	scheduledAt := item.ScheduledAt
	if b.job.ExecutionWindowCron == "" {
		return scheduledAt, nil
	}
	timezone := b.job.Timezone
	if timezone == "" && b.projectQuota != nil {
		timezone = b.projectQuota.Timezone
	}
	adjustedScheduledAt, err := alignToExecutionWindow(scheduledAt, b.now, b.job.ExecutionWindowCron, timezone)
	if err != nil {
		return nil, huma.Error400BadRequest("execution window validation failed: " + err.Error())
	}
	return adjustedScheduledAt, nil
}

func (b *bulkTriggerState) enqueueOrBufferRun(run *domain.JobRun, item BulkTriggerItem, itemIdx int) (bool, error) {
	if !b.hasIdempotencyKey && b.tx == nil {
		b.pendingRuns = append(b.pendingRuns, run)
		return false, nil
	}
	if err := b.server.enqueueTriggerRun(b.ctx, b.tx, run); err != nil {
		return b.handleEnqueueError(err, item, itemIdx)
	}
	return false, nil
}

func (b *bulkTriggerState) handleEnqueueError(err error, item BulkTriggerItem, itemIdx int) (bool, error) {
	if errors.Is(err, domain.ErrIdempotencyConflict) && item.IdempotencyKey != "" {
		existingRun, retryErr := b.server.store.GetRunByIdempotencyKey(b.ctx, b.job.ID, item.IdempotencyKey)
		if retryErr != nil {
			slog.Error("idempotency conflict retry failed",
				"job_id", b.job.ID,
				"idempotency_key_hash", hashIdempotencyKey(item.IdempotencyKey),
				"item_index", itemIdx,
				"error", retryErr)
			return false, huma.Error500InternalServerError(fmt.Sprintf("failed to check idempotency key after conflict for item %d", itemIdx))
		}
		if existingRun != nil {
			slog.Warn("idempotency conflict resolved",
				"job_id", b.job.ID,
				"idempotency_key_hash", hashIdempotencyKey(item.IdempotencyKey),
				"winning_run_id", existingRun.ID,
				"item_index", itemIdx)
			b.appendExistingRun(existingRun, true)
			return true, nil
		}
		slog.Error("idempotency conflict retry returned nil",
			"job_id", b.job.ID,
			"idempotency_key_hash", hashIdempotencyKey(item.IdempotencyKey),
			"item_index", itemIdx)
	}
	if apiErr := enqueueAPIError(err); apiErr != nil {
		return false, apiErr
	}
	return false, huma.Error500InternalServerError(fmt.Sprintf("failed to enqueue item %d", itemIdx))
}

func (b *bulkTriggerState) enqueuePendingRuns() error {
	if len(b.pendingRuns) == 0 {
		return nil
	}
	if _, err := b.server.queue.EnqueueBatch(b.ctx, b.pendingRuns); err != nil {
		if apiErr := enqueueAPIError(err); apiErr != nil {
			return apiErr
		}
		return huma.Error500InternalServerError("failed to enqueue batch")
	}
	return nil
}

func (b *bulkTriggerState) appendExistingRun(run *domain.JobRun, idempotencyHit bool) {
	b.results = append(b.results, BulkTriggerResult{
		ID:             run.ID,
		Status:         string(run.Status),
		IdempotencyHit: idempotencyHit,
	})
}

func (b *bulkTriggerState) appendCreatedRun(run *domain.JobRun) {
	b.results = append(b.results, BulkTriggerResult{
		ID:             run.ID,
		Status:         string(run.Status),
		IdempotencyHit: false,
	})
	b.created++
	b.enqueuedInBatch++
}

func (s *Server) loadBulkTriggerJob(ctx context.Context, jobID string) (*domain.Job, error) {
	job, err := s.loadRunCreationJob(ctx, jobID, "bulk_trigger_job.project_match", "handleBulkTriggerJob")
	if err != nil {
		return nil, err
	}
	if err := s.checkHTTPModeAllowed(ctx, job.ExecutionMode, job.ProjectID); err != nil {
		return nil, err
	}
	if err := ensureJobTriggerable(job); err != nil {
		return nil, err
	}
	return job, nil
}

func (s *Server) validateBulkTriggerRequest(req BulkTriggerRequest) error {
	if err := s.validate.Struct(&req); err != nil {
		return newValidationError(err)
	}
	if len(req.Items) > s.config.MaxBulkTriggerItems {
		return huma.Error400BadRequest(fmt.Sprintf("maximum %d items per bulk trigger request", s.config.MaxBulkTriggerItems))
	}
	return nil
}

func (s *Server) validateBulkTriggerItem(job *domain.Job, item BulkTriggerItem, itemIdx int) error {
	if err := validateTriggerScheduledAt(item.ScheduledAt); err != nil {
		return huma.Error400BadRequest(fmt.Sprintf("scheduled_at validation failed for item %d: %v", itemIdx, err))
	}
	if err := validateTriggerTTLSecs(item.TTLSecs); err != nil {
		return huma.Error400BadRequest(fmt.Sprintf("ttl_secs validation failed for item %d: %v", itemIdx, err))
	}
	if len(item.Tags) > 0 {
		if err := validateTags(item.Tags); err != nil {
			return huma.Error400BadRequest(fmt.Sprintf("invalid tags for item %d: %v", itemIdx, err))
		}
	}
	if err := validatePayloadAgainstSchema(item.Payload, job.PayloadSchema); err != nil {
		return huma.Error400BadRequest(fmt.Sprintf("payload validation failed for item %d: %v", itemIdx, err))
	}
	return nil
}

func bulkHasIdempotencyKey(items []BulkTriggerItem) bool {
	for _, item := range items {
		if item.IdempotencyKey != "" {
			return true
		}
	}
	return false
}

func newBulkTriggerRun(
	ctx context.Context,
	job *domain.Job,
	item BulkTriggerItem,
	payload json.RawMessage,
	batchID string,
	now time.Time,
	scheduledAt *time.Time,
) *domain.JobRun {
	status := domain.StatusQueued
	if scheduledAt != nil && scheduledAt.After(now) {
		status = domain.StatusDelayed
	}

	expiresAt := bulkTriggerExpiresAt(job, item, now)
	run := &domain.JobRun{
		ID:             uuid.Must(uuid.NewV7()).String(),
		JobID:          job.ID,
		ProjectID:      job.ProjectID,
		Tags:           mergedRunTags(job.Tags, item.Tags),
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
		ExecutionMode:  job.ExecutionMode,
		QueueName:      job.Queue,
		ConcurrencyKey: item.ConcurrencyKey,
	}
	run.Metadata = mergeRunMetadata(run.Metadata, job.DefaultRunMetadata)
	return run
}

func bulkTriggerExpiresAt(job *domain.Job, item BulkTriggerItem, now time.Time) time.Time {
	if item.TTLSecs != nil && *item.TTLSecs > 0 {
		return now.Add(time.Duration(*item.TTLSecs) * time.Second)
	}
	if job.RunTTLSecs > 0 {
		return now.Add(time.Duration(job.RunTTLSecs) * time.Second)
	}
	return now.Add(time.Duration(job.TimeoutSecs)*time.Second + 60*time.Second)
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

	// Fetch once, then partition locally so the response preserves the
	// caller's requested run IDs and reports per-run failures.
	runsMap, err := s.store.GetRunsByIDs(ctx, req.RunIDs)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to fetch runs")
	}

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
		if err := requireProjectMatch(ctx, run.ProjectID); err != nil {
			results = append(results, BulkCancelResult{ID: runID, Status: "failed", Error: "run not found"})
			failed++
			continue
		}
		if err := s.requireRunEnvironmentMatch(ctx, run); err != nil {
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

		// A run can leave the cancelable set between the initial fetch and
		// the update. Keep that race visible in the per-run response.
		for _, id := range cancelableIDs {
			if _, ok := canceledSet[id]; !ok {
				results = append(results, BulkCancelResult{ID: id, Status: string(runsMap[id].Status), Error: "failed to cancel (status may have changed)"})
				failed++
			}
		}

		// Child cancellation is best-effort: parent cancellation has already
		// succeeded, and retrying the whole request would duplicate results.
		if _, err := s.store.CancelChildRunsByParentIDs(ctx, cancelableIDs, now, "parent run canceled (bulk)"); err != nil {
			slog.Error("failed to cancel child runs in bulk", "error", err)
		}
	}

	s.emitAuditEvent(ctx, domain.AuditActionRunBulkCancelled, "run", "", map[string]any{
		"total":    len(req.RunIDs),
		"canceled": canceled,
		"failed":   failed,
	})

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
	if environmentIDFromContext(ctx) != "" {
		if req.JobID == "" {
			return nil, huma.Error403Forbidden("environment-scoped bulk cancellation requires a job_id filter")
		}
		job, err := s.store.GetJob(ctx, req.JobID)
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
	}

	now := time.Now()
	ids, err := s.store.BulkCancelByFilter(ctx, projectID, store.BulkCancelFilter{
		JobID: req.JobID, BatchID: req.BatchID, TriggeredBy: req.TriggeredBy, Status: req.Status,
	}, now, "canceled by user (bulk filter)")
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to cancel runs")
	}

	s.emitAuditEvent(ctx, domain.AuditActionRunBulkCancelledAll, "run", "", map[string]any{
		"project_id":   projectID,
		"canceled":     len(ids),
		"job_id":       req.JobID,
		"batch_id":     req.BatchID,
		"triggered_by": req.TriggeredBy,
		"status":       string(req.Status),
	})

	return &BulkCancelAllOutput{Body: map[string]any{"canceled": len(ids), "run_ids": ids}}, nil
}

func (s *Server) requireRunEnvironmentMatch(ctx context.Context, run *domain.JobRun) error {
	if environmentIDFromContext(ctx) == "" || run == nil {
		return nil
	}
	job, err := s.store.GetJob(ctx, run.JobID)
	if err != nil {
		return err
	}
	if err := requireProjectMatch(ctx, job.ProjectID); err != nil {
		return err
	}
	return requireEnvironmentMatch(ctx, job.EnvironmentID)
}
