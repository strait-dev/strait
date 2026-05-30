package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

// CreateWorkflowRunSingletonBootstrap creates a workflow run while enforcing the
// owner's singleton policy, atomic with the lock acquire. It mirrors the job
// trigger path (applyJobSingletonPolicy) but folds the whole decision into a
// single transaction so the engine never imports the store package.
//
// The lock is acquired with a NULL lease: workflow holders are reclaimed by the
// reaper on a terminal/missing check only, never on a lease timer, so a long
// durable-wait workflow is never falsely reclaimed.
//
// Returns (outcome, holderRunID, runCreated, error):
//   - dispatched:    the key was claimed; the run is fully bootstrapped (running
//   - step runs created). runCreated is true; the engine starts root steps.
//   - queued_behind: the key was held; the run is parked as 'queued' with its
//     step runs created but not started. runCreated is true.
//   - replaced:      the holder (and any existing queued waiters) were canceled
//     and the run is parked as 'queued'. runCreated is true; the engine promotes
//     it immediately via the just-canceled holder.
//   - dropped:       no run was created. runCreated is false.
//
// holderRunID names the run that held the key on conflict ("" when acquired).
func (q *Queries) CreateWorkflowRunSingletonBootstrap(
	ctx context.Context,
	run *domain.WorkflowRun,
	stepRuns []domain.WorkflowStepRun,
	startedAt time.Time,
	key string,
	onConflict domain.SingletonOnConflict,
	maxQueueDepth *int,
	preemptHigher bool,
) (domain.SingletonOutcome, string, bool, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateWorkflowRunSingletonBootstrap")
	defer span.End()

	var outcome domain.SingletonOutcome
	var holderRunID string
	var runCreated bool

	err := q.withTx(ctx, func(txQ *Queries) error {
		// On conflict we serialize the rest of the decision behind a FOR UPDATE
		// lock on the holder row. If the holder is released in the narrow window
		// between our acquire attempt and that lock, the key is free again and we
		// retry the acquire. The bound guards against a pathological acquire/
		// release storm on the same key livelocking the transaction.
		const maxAcquireAttempts = 8
		for attempt := 1; ; attempt++ {
			acquired, _, aerr := txQ.AcquireSingletonLock(ctx, domain.SingletonLock{
				ProjectID:   run.ProjectID,
				Kind:        domain.SingletonKindWorkflow,
				OwnerID:     run.WorkflowID,
				LockKey:     key,
				HolderRunID: run.ID,
				LeaseUntil:  nil, // workflow holders: terminal/missing check only
			})
			if aerr != nil {
				return fmt.Errorf("acquire workflow singleton lock: %w", aerr)
			}

			if acquired {
				if err := txQ.bootstrapWorkflowRunTx(ctx, run, stepRuns, startedAt); err != nil {
					return err
				}
				outcome = domain.SingletonOutcomeDispatched
				runCreated = true
				return nil
			}

			// Lost the acquire race: pin the holder row for the rest of this
			// transaction so the queue-depth check and park cannot interleave with
			// another waiter or a release.
			holder, lerr := txQ.LockSingletonHolderForUpdate(ctx, run.ProjectID, domain.SingletonKindWorkflow, run.WorkflowID, key)
			if errors.Is(lerr, ErrSingletonLockNotFound) {
				if attempt >= maxAcquireAttempts {
					return fmt.Errorf("acquire workflow singleton lock: key %q churned without a stable holder after %d attempts", key, attempt)
				}
				continue // key freed under us; retry the acquire
			}
			if lerr != nil {
				return lerr
			}
			holderRunID = holder.HolderRunID

			var cerr error
			outcome, runCreated, cerr = txQ.applyWorkflowSingletonConflictTx(ctx, run, stepRuns, key, holderRunID, onConflict, maxQueueDepth, preemptHigher)
			return cerr
		}
	})
	if err != nil {
		return "", "", false, err
	}
	return outcome, holderRunID, runCreated, nil
}

// applyWorkflowSingletonConflictTx resolves the on-conflict policy for a workflow
// run that lost the acquire race, with the holder row already pinned FOR UPDATE by
// the caller. It returns the resulting outcome and whether a run row was created.
// Must run inside the bootstrap transaction.
func (q *Queries) applyWorkflowSingletonConflictTx(
	ctx context.Context,
	run *domain.WorkflowRun,
	stepRuns []domain.WorkflowStepRun,
	key string,
	holderRunID string,
	onConflict domain.SingletonOnConflict,
	maxQueueDepth *int,
	preemptHigher bool,
) (domain.SingletonOutcome, bool, error) {
	switch onConflict {
	case domain.SingletonOnConflictDrop:
		return domain.SingletonOutcomeDropped, false, nil

	case domain.SingletonOnConflictQueue:
		// Optional preemption: a strictly higher-priority newcomer cancels the
		// running lower-priority holder and parks to take the key, without
		// discarding existing waiters (they compete by priority on release). The
		// queue-depth cap is bypassed because preemption is a deliberate override.
		if preemptHigher && holderRunID != "" {
			holderPriority, found, perr := q.workflowRunPriority(ctx, holderRunID)
			if perr != nil {
				return "", false, fmt.Errorf("read workflow holder priority: %w", perr)
			}
			if found && run.Priority > holderPriority {
				if cerr := q.cancelSingletonWorkflowHolderTx(ctx, holderRunID, "canceled by singleton preemption"); cerr != nil {
					return "", false, fmt.Errorf("cancel preempted workflow holder: %w", cerr)
				}
				if err := q.parkWorkflowRunTx(ctx, run, stepRuns); err != nil {
					return "", false, err
				}
				return domain.SingletonOutcomeReplaced, true, nil
			}
		}
		waiters, cerr := q.CountSingletonWaiters(ctx, domain.SingletonKindWorkflow, run.WorkflowID, key)
		if cerr != nil {
			return "", false, fmt.Errorf("count workflow singleton waiters: %w", cerr)
		}
		if maxQueueDepth != nil && waiters >= *maxQueueDepth {
			return domain.SingletonOutcomeDropped, false, nil
		}
		if err := q.parkWorkflowRunTx(ctx, run, stepRuns); err != nil {
			return "", false, err
		}
		return domain.SingletonOutcomeQueuedBehind, true, nil

	case domain.SingletonOnConflictReplace:
		// Discard any waiters already parked behind the holder so the newcomer
		// is the sole successor (keep newest).
		if _, cerr := q.cancelSingletonWorkflowWaitersTx(ctx, run.WorkflowID, key, "superseded by singleton replace"); cerr != nil {
			return "", false, fmt.Errorf("cancel workflow singleton waiters: %w", cerr)
		}
		if holderRunID != "" {
			if cerr := q.cancelSingletonWorkflowHolderTx(ctx, holderRunID, "canceled by singleton replace policy"); cerr != nil {
				return "", false, fmt.Errorf("cancel workflow singleton holder: %w", cerr)
			}
		}
		if err := q.parkWorkflowRunTx(ctx, run, stepRuns); err != nil {
			return "", false, err
		}
		return domain.SingletonOutcomeReplaced, true, nil

	default:
		return "", false, fmt.Errorf("unknown singleton on-conflict policy %q", onConflict)
	}
}

// bootstrapWorkflowRunTx inserts a workflow run, transitions it pending -> running,
// and creates its step runs. Shared by CreateWorkflowRunBootstrap and the
// singleton dispatch path. Must run inside a transaction.
func (q *Queries) bootstrapWorkflowRunTx(ctx context.Context, run *domain.WorkflowRun, stepRuns []domain.WorkflowStepRun, startedAt time.Time) error {
	if err := q.CreateWorkflowRun(ctx, run); err != nil {
		return fmt.Errorf("create workflow run bootstrap: %w", err)
	}
	if err := q.UpdateWorkflowRunStatus(ctx, run.ID, domain.WfStatusPending, domain.WfStatusRunning, map[string]any{"started_at": startedAt}); err != nil {
		return fmt.Errorf("mark workflow running bootstrap: %w", err)
	}
	for i := range stepRuns {
		sr := stepRuns[i]
		if err := q.CreateWorkflowStepRun(ctx, &sr); err != nil {
			return fmt.Errorf("create workflow step run bootstrap %s: %w", sr.StepRef, err)
		}
	}
	return nil
}

// parkWorkflowRunTx inserts a workflow run in the 'queued' parked state along
// with its step runs. The run does not progress until a release promotes it
// (queued -> running). Must run inside a transaction.
func (q *Queries) parkWorkflowRunTx(ctx context.Context, run *domain.WorkflowRun, stepRuns []domain.WorkflowStepRun) error {
	run.Status = domain.WfStatusQueued
	if err := q.CreateWorkflowRun(ctx, run); err != nil {
		return fmt.Errorf("create parked workflow run: %w", err)
	}
	for i := range stepRuns {
		sr := stepRuns[i]
		if err := q.CreateWorkflowStepRun(ctx, &sr); err != nil {
			return fmt.Errorf("create parked workflow step run %s: %w", sr.StepRef, err)
		}
	}
	return nil
}

// cancelSingletonWorkflowWaitersTx cancels every workflow run parked in 'queued'
// behind the holder of (workflowID, lockKey), along with their non-terminal step
// runs. Used by the replace policy. Returns the number of waiters canceled.
func (q *Queries) cancelSingletonWorkflowWaitersTx(ctx context.Context, workflowID, lockKey, reason string) (int64, error) {
	const cancelRuns = `
		UPDATE workflow_runs
		SET status = 'canceled', finished_at = NOW(), error = $3
		WHERE workflow_id = $1 AND singleton_key = $2 AND status = 'queued'`
	tag, err := q.db.Exec(ctx, cancelRuns, workflowID, lockKey, reason)
	if err != nil {
		return 0, fmt.Errorf("cancel singleton workflow waiters: %w", err)
	}
	const cancelSteps = `
		UPDATE workflow_step_runs
		SET status = 'canceled', finished_at = NOW()
		WHERE workflow_run_id IN (
			SELECT id FROM workflow_runs
			WHERE workflow_id = $1 AND singleton_key = $2 AND status = 'canceled'
		)
		AND status NOT IN ('completed', 'failed', 'skipped', 'canceled')`
	if _, serr := q.db.Exec(ctx, cancelSteps, workflowID, lockKey); serr != nil {
		return 0, fmt.Errorf("cancel singleton workflow waiter step runs: %w", serr)
	}
	return tag.RowsAffected(), nil
}

// workflowRunPriority reads a workflow run's priority. found is false (no error)
// when the run no longer exists, so preemption falls through to normal queueing.
func (q *Queries) workflowRunPriority(ctx context.Context, runID string) (int, bool, error) {
	var priority int
	err := q.db.QueryRow(ctx, `SELECT priority FROM workflow_runs WHERE id = $1`, runID).Scan(&priority)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("read workflow run priority: %w", err)
	}
	return priority, true, nil
}

// cancelSingletonWorkflowHolderTx cancels the current holder workflow run and
// cascades to its step runs, child job runs, and pending event triggers, mirroring
// handleCancelWorkflowRun. A missing or already-terminal holder is a no-op.
func (q *Queries) cancelSingletonWorkflowHolderTx(ctx context.Context, holderRunID, reason string) error {
	holder, err := q.GetWorkflowRun(ctx, holderRunID)
	if err != nil {
		if errors.Is(err, ErrWorkflowRunNotFound) {
			return nil
		}
		return fmt.Errorf("get singleton workflow holder: %w", err)
	}
	if holder.Status.IsTerminal() {
		return nil
	}
	now := time.Now()
	if err := q.UpdateWorkflowRunStatus(ctx, holderRunID, holder.Status, domain.WfStatusCanceled, map[string]any{
		"finished_at": now,
		"error":       reason,
	}); err != nil {
		return fmt.Errorf("cancel singleton workflow holder run: %w", err)
	}
	if _, err := q.CancelNonTerminalStepRuns(ctx, holderRunID, now, reason); err != nil {
		return fmt.Errorf("cancel singleton holder step runs: %w", err)
	}
	if _, err := q.CancelJobRunsByWorkflowRun(ctx, holderRunID, now, reason); err != nil {
		return fmt.Errorf("cancel singleton holder job runs: %w", err)
	}
	if _, err := q.CancelEventTriggersByWorkflowRun(ctx, holderRunID); err != nil {
		return fmt.Errorf("cancel singleton holder event triggers: %w", err)
	}
	return nil
}

// ReleaseSingletonWorkflowLockAndPromote releases the singleton lock held by
// holderRunID (a workflow run) and, if a workflow run is parked behind the same
// key, promotes the oldest one: it re-points the lock to that waiter and
// transitions it queued -> running. The caller (the engine) then starts the
// promoted run's root steps.
//
// The whole operation runs in one transaction that takes a row lock on the
// holder's lock row, so the terminal fast-path and the reaper can both fire for
// the same holder yet the key is released and promoted at most once.
//
// Returns (released, promotedRunID, error): released is true when a lock row was
// deleted, promotedRunID names the waiter transitioned to running ("" when the
// key is now free). Workflow holders never carry a lease.
func (q *Queries) ReleaseSingletonWorkflowLockAndPromote(ctx context.Context, holderRunID string) (bool, string, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ReleaseSingletonWorkflowLockAndPromote")
	defer span.End()

	var released bool
	var promotedRunID string

	err := q.withTx(ctx, func(txQ *Queries) error {
		var projectID, ownerID, lockKey string
		err := txQ.db.QueryRow(ctx, `
			SELECT project_id, owner_id, lock_key
			FROM singleton_locks
			WHERE holder_run_id = $1 AND kind = $2
			FOR UPDATE`,
			holderRunID, string(domain.SingletonKindWorkflow),
		).Scan(&projectID, &ownerID, &lockKey)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil // already released; nothing to do
			}
			return fmt.Errorf("lock singleton workflow holder row: %w", err)
		}

		if _, err := txQ.db.Exec(ctx,
			`DELETE FROM singleton_locks WHERE holder_run_id = $1`, holderRunID,
		); err != nil {
			return fmt.Errorf("delete singleton workflow lock: %w", err)
		}
		released = true

		var waiterID string
		err = txQ.db.QueryRow(ctx, `
			SELECT id FROM workflow_runs
			WHERE workflow_id = $1 AND singleton_key = $2 AND status = 'queued'
			ORDER BY priority DESC, created_at ASC, id ASC
			LIMIT 1
			FOR UPDATE SKIP LOCKED`,
			ownerID, lockKey,
		).Scan(&waiterID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil // key is now free
			}
			return fmt.Errorf("find singleton workflow waiter: %w", err)
		}

		if _, err := txQ.db.Exec(ctx, `
			INSERT INTO singleton_locks (project_id, kind, owner_id, lock_key, holder_run_id, lease_until)
			VALUES ($1, $2, $3, $4, $5, NULL)`,
			projectID, string(domain.SingletonKindWorkflow), ownerID, lockKey, waiterID,
		); err != nil {
			return fmt.Errorf("reacquire singleton workflow lock for waiter: %w", err)
		}

		if err := txQ.UpdateWorkflowRunStatus(ctx, waiterID, domain.WfStatusQueued, domain.WfStatusRunning, map[string]any{
			"started_at": time.Now(),
		}); err != nil {
			return fmt.Errorf("promote singleton workflow waiter: %w", err)
		}
		promotedRunID = waiterID
		return nil
	})
	if err != nil {
		return false, "", err
	}
	return released, promotedRunID, nil
}

// ListReapableSingletonWorkflowHolders returns the holder_run_ids of workflow
// singleton locks the reaper should release: the holder run is missing (deleted
// by retention) or already terminal. Workflow holders carry no lease, so a
// running or parked (queued) holder is never reclaimed by a timer.
//
// limit bounds the batch (oldest acquisitions first) so a backlog is drained
// across cycles rather than loaded at once; a limit <= 0 returns all (ad-hoc and
// test callers only). The reaper always passes a positive bound.
func (q *Queries) ListReapableSingletonWorkflowHolders(ctx context.Context, limit int) ([]string, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListReapableSingletonWorkflowHolders")
	defer span.End()

	query := `
		SELECT sl.holder_run_id
		FROM singleton_locks sl
		WHERE sl.kind = 'workflow'
		  AND (
		      NOT EXISTS (SELECT 1 FROM workflow_runs wr WHERE wr.id = sl.holder_run_id)
		      OR EXISTS (
		          SELECT 1 FROM workflow_runs wr
		          WHERE wr.id = sl.holder_run_id
		            AND wr.status IN ('completed','failed','timed_out','canceled','compensated','compensation_failed')
		      )
		  )
		ORDER BY sl.acquired_at ASC`

	args := []any{}
	if limit > 0 {
		query += " LIMIT $1"
		args = append(args, limit)
	}

	rows, err := q.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list reapable singleton workflow holders: %w", err)
	}
	defer rows.Close()

	ids := make([]string, 0, 16)
	for rows.Next() {
		var id string
		if scanErr := rows.Scan(&id); scanErr != nil {
			return nil, fmt.Errorf("list reapable singleton workflow holders scan: %w", scanErr)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list reapable singleton workflow holders rows: %w", err)
	}
	return ids, nil
}
