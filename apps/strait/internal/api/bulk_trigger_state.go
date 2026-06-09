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
)

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
	run := newBulkTriggerRun(b.ctx, bulkTriggerRunRequest{
		job:         b.job,
		item:        item,
		payload:     payload,
		batchID:     b.batchID,
		now:         b.now,
		scheduledAt: scheduledAt,
	})
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
	if !jobHasRateLimit(b.job) {
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
