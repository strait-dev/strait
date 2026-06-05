package worker

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

type snoozeTransitionConfig struct {
	from          domain.RunStatus
	enqueueReason string
}

type snoozeTransitionState struct {
	reason string
	count  int
}

func newSnoozeTransitionState(run *domain.JobRun, reason string) snoozeTransitionState {
	return snoozeTransitionState{
		reason: reason,
		count:  nextSnoozeCount(run.Metadata),
	}
}

func nextSnoozeCount(metadata map[string]string) int {
	snoozeCount := 0
	if metadata != nil {
		if raw, ok := metadata["snooze_count"]; ok {
			if parsed, err := strconv.Atoi(raw); err == nil {
				snoozeCount = parsed
			}
		}
	}
	return snoozeCount + 1
}

func (s snoozeTransitionState) exceeds(maxSnoozeCount int) bool {
	return maxSnoozeCount > 0 && s.count > maxSnoozeCount
}

func (s snoozeTransitionState) fields() map[string]any {
	return map[string]any{
		"error":       s.reason,
		"error_class": domain.ErrorClassTransient,
		"started_at":  nil,
		"finished_at": nil,
		"metadata":    map[string]string{"snooze_count": strconv.Itoa(s.count)},
	}
}

func (e *Executor) snoozeRun(ctx context.Context, run *domain.JobRun, reason string, retryAt *time.Time) {
	from := domain.StatusDequeued
	if run.Status == domain.StatusExecuting {
		from = domain.StatusExecuting
	}
	e.snoozeRunFromStatus(ctx, run, reason, retryAt, snoozeTransitionConfig{
		from:          from,
		enqueueReason: "snooze",
	})
}

func (e *Executor) snoozeRunFromStatus(ctx context.Context, run *domain.JobRun, reason string, retryAt *time.Time, cfg snoozeTransitionConfig) {
	state := newSnoozeTransitionState(run, reason)

	if state.exceeds(e.maxSnoozeCount) {
		e.logger.Warn("max snooze count exceeded, marking system_failed",
			"run_id", run.ID, "job_id", run.JobID, "snooze_count", state.count)
		e.handleSystemFailure(ctx, run, fmt.Sprintf("max snooze count (%d) exceeded: %s", e.maxSnoozeCount, reason))
		return
	}

	fields := state.fields()
	if retryAt != nil {
		if err := e.store.ScheduleRetry(ctx, run.ID, *retryAt, run.Attempt); err != nil {
			e.logger.Error("failed to schedule snooze retry", "run_id", run.ID, "job_id", run.JobID, "from", cfg.from, "error", err)
			return
		}
	} else if err := e.store.ClearRetry(ctx, run.ID); err != nil {
		e.logger.Warn("failed to clear retry on snooze", "run_id", run.ID, "job_id", run.JobID, "from", cfg.from, "error", err)
	}
	if err := e.store.SnoozeRunWithLock(ctx, run.ID, cfg.from, domain.StatusQueued, fields); err != nil {
		if errors.Is(err, store.ErrRunLocked) {
			recordSnoozeSkipped(ctx, string(cfg.from), snoozeSkippedReasonLocked)
			e.logger.Warn("snooze skipped: run row locked by another transaction",
				"run_id", run.ID, "job_id", run.JobID, "from", cfg.from)
			return
		}
		if errors.Is(err, store.ErrRunConflict) {
			recordSnoozeSkipped(ctx, string(cfg.from), snoozeSkippedReasonConflict)
			e.logger.Warn("snooze skipped: run no longer in expected state",
				"run_id", run.ID, "job_id", run.JobID, "from", cfg.from)
			return
		}
		e.logger.Error("failed to snooze run", "run_id", run.ID, "job_id", run.JobID, "from", cfg.from, "error", err)
		return
	}
	if retryAt == nil {
		run.Status = domain.StatusQueued
		e.enqueueExistingRunIfReady(ctx, run, cfg.enqueueReason)
	}

	e.emit(ctx, RunLifecycleEvent{
		Type: EventSnoozed, Run: run,
		FromStatus: cfg.from, ToStatus: domain.StatusQueued,
		Attempt: run.Attempt,
	})
}

// snoozeRunFromExecuting re-queues a run that is currently in the Executing
// state. This differs from snoozeRun which expects StatusDequeued as the
// source state.
//
//nolint:unparam // retryAt is nil in current callers but retained for symmetry with snoozeRun.
func (e *Executor) snoozeRunFromExecuting(ctx context.Context, run *domain.JobRun, reason string, retryAt *time.Time) {
	e.snoozeRunFromStatus(ctx, run, reason, retryAt, snoozeTransitionConfig{
		from:          domain.StatusExecuting,
		enqueueReason: "snooze_from_executing",
	})
}
