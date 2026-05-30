package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"strait/internal/domain"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// singletonWorkflowStore is the optional store surface for first-class singleton
// execution. It is satisfied by *store.Queries via structural typing, so the
// engine never imports the store package (mirrors the bootstrapStore assertion).
type singletonWorkflowStore interface {
	CreateWorkflowRunSingletonBootstrap(
		ctx context.Context,
		run *domain.WorkflowRun,
		stepRuns []domain.WorkflowStepRun,
		startedAt time.Time,
		key string,
		onConflict domain.SingletonOnConflict,
		maxQueueDepth *int,
	) (domain.SingletonOutcome, string, bool, error)
	ReleaseSingletonWorkflowLockAndPromote(ctx context.Context, holderRunID string) (bool, string, error)
}

// singletonTriggerResult carries the singleton decision back to callers that
// need it (the API trigger handler). triggerWorkflowInternal populates it when
// non-nil; an empty outcome means no singleton policy applied.
type singletonTriggerResult struct {
	outcome  domain.SingletonOutcome
	holderID string
}

// resolveWorkflowSingletonKey resolves a singleton key expression against the
// trigger payload. The expression is validated at definition time; an
// unresolvable key at trigger time wraps domain.ErrSingletonKeyUnresolvable so
// the API layer can surface a 400.
func resolveWorkflowSingletonKey(keyExpr json.RawMessage, payload json.RawMessage) (string, error) {
	expr, err := domain.ParseSingletonKeyExpr(keyExpr)
	if err != nil {
		return "", fmt.Errorf("invalid workflow singleton key expression: %w", err)
	}
	key, rerr := domain.ResolveSingletonKey(expr, payload)
	if rerr != nil {
		return "", fmt.Errorf("resolve workflow singleton key: %w", rerr)
	}
	if key == "" {
		return "", fmt.Errorf("%w: workflow singleton key resolved to an empty value", domain.ErrSingletonKeyUnresolvable)
	}
	return key, nil
}

// bootstrapSingletonWorkflowRun claims the resolved key atomically with run
// creation and applies the workflow's on-conflict policy. It returns done=true
// when the trigger is fully handled here (dropped: nil run; queued/replaced:
// parked run) and done=false only for a dispatched outcome, in which case the
// caller continues to start the run's root steps.
func (e *WorkflowEngine) bootstrapSingletonWorkflowRun(
	ctx context.Context,
	wfRun *domain.WorkflowRun,
	stepRuns []domain.WorkflowStepRun,
	now time.Time,
	eff domain.EffectiveCronSingleton,
) (*domain.WorkflowRun, domain.SingletonOutcome, string, bool, error) {
	key, kerr := resolveWorkflowSingletonKey(eff.KeyExpr, wfRun.Payload)
	if kerr != nil {
		return nil, "", "", true, kerr
	}
	wfRun.SingletonKey = key

	ss, ok := e.store.(singletonWorkflowStore)
	if !ok {
		return nil, "", "", true, fmt.Errorf("workflow singleton store unavailable")
	}

	outcome, holderID, _, serr := ss.CreateWorkflowRunSingletonBootstrap(
		ctx, wfRun, stepRuns, now, key, eff.OnConflict, eff.MaxQueueDepth,
	)
	if serr != nil {
		return nil, "", "", true, fmt.Errorf("workflow singleton bootstrap: %w", serr)
	}

	switch outcome {
	case domain.SingletonOutcomeDispatched:
		e.recordSingletonAcquisition(ctx)
		return wfRun, outcome, holderID, false, nil

	case domain.SingletonOutcomeDropped:
		e.recordSingletonConflict(ctx, eff.OnConflict)
		return nil, outcome, holderID, true, nil

	case domain.SingletonOutcomeQueuedBehind:
		e.recordSingletonConflict(ctx, eff.OnConflict)
		wfRun.Status = domain.WfStatusQueued
		return wfRun, outcome, holderID, true, nil

	case domain.SingletonOutcomeReplaced:
		e.recordSingletonConflict(ctx, eff.OnConflict)
		wfRun.Status = domain.WfStatusQueued
		// The holder was canceled synchronously in the bootstrap tx, so promote
		// the newcomer now instead of waiting for the reaper.
		if holderID != "" {
			if _, perr := e.PromoteSingletonWorkflowSuccessor(ctx, holderID); perr != nil {
				e.logger.Warn("failed to promote workflow singleton replacement (reaper will retry)",
					"holder_run_id", holderID, "workflow_run_id", wfRun.ID, "error", perr)
			}
		}
		if promoted, gerr := e.store.GetWorkflowRun(ctx, wfRun.ID); gerr == nil && promoted != nil {
			return promoted, outcome, holderID, true, nil
		}
		return wfRun, outcome, holderID, true, nil

	default:
		return nil, "", "", true, fmt.Errorf("unknown singleton outcome %q", outcome)
	}
}

// PromoteSingletonWorkflowSuccessor releases the singleton lock held by a
// terminal/missing workflow run and, if a run is parked behind the same key,
// promotes the oldest one (queued -> running) and starts its root steps. It is
// idempotent and safe to call from both the terminal fast-path and the reaper:
// the store serializes release+promote on the holder's lock row, so a key
// promotes at most once.
// It returns released=true when this caller is the one that removed the holder's
// lock (whether or not a successor was waiting), so the reaper can attribute the
// stale-reclaim metric exactly once.
func (e *WorkflowEngine) PromoteSingletonWorkflowSuccessor(ctx context.Context, releasedHolderRunID string) (bool, error) {
	ss, ok := e.store.(singletonWorkflowStore)
	if !ok {
		return false, nil
	}
	released, promotedRunID, err := ss.ReleaseSingletonWorkflowLockAndPromote(ctx, releasedHolderRunID)
	if err != nil {
		return false, fmt.Errorf("release and promote workflow singleton: %w", err)
	}
	if !released {
		return false, nil
	}
	if promotedRunID == "" {
		return true, nil
	}
	e.recordSingletonAcquisition(ctx)
	if err := e.startPromotedWorkflowRun(ctx, promotedRunID); err != nil {
		return true, fmt.Errorf("start promoted workflow run %s: %w", promotedRunID, err)
	}
	return true, nil
}

// startPromotedWorkflowRun starts the root steps of a run the store just promoted
// to running. The run's step runs already exist (created when it was parked), so
// this only dispatches the ready roots, honoring MaxParallelSteps and per-step
// concurrency keys exactly like the initial trigger.
func (e *WorkflowEngine) startPromotedWorkflowRun(ctx context.Context, runID string) error {
	run, err := e.store.GetWorkflowRun(ctx, runID)
	if err != nil {
		return fmt.Errorf("get promoted workflow run: %w", err)
	}
	if run.Status != domain.WfStatusRunning {
		return nil // already advanced or not actually promoted
	}

	steps, err := e.store.ListStepsByWorkflowVersion(ctx, run.WorkflowID, run.WorkflowVersion)
	if err != nil {
		return fmt.Errorf("list promoted workflow steps: %w", err)
	}
	stepRunList, err := e.store.ListStepRunsByWorkflowRun(ctx, runID, 100000, nil)
	if err != nil {
		return fmt.Errorf("list promoted workflow step runs: %w", err)
	}
	srByRef := make(map[string]*domain.WorkflowStepRun, len(stepRunList))
	for i := range stepRunList {
		srByRef[stepRunList[i].StepRef] = &stepRunList[i]
	}

	recordWorkflowActiveRunDelta(ctx, run.ProjectID, 1)

	runningStarts := 0
	runningByConcurrencyKey := make(map[string]int)
	for i := range steps {
		step := &steps[i]
		if len(step.DependsOn) != 0 {
			continue
		}
		sr := srByRef[step.StepRef]
		if sr == nil {
			continue
		}
		if run.MaxParallelSteps > 0 && runningStarts >= run.MaxParallelSteps {
			if err := e.store.UpdateStepRunStatus(ctx, sr.ID, domain.StepWaiting, nil); err != nil {
				return fmt.Errorf("set promoted root step waiting %s: %w", step.StepRef, err)
			}
			continue
		}
		if step.ConcurrencyKey != "" && runningByConcurrencyKey[step.ConcurrencyKey] > 0 {
			if err := e.store.UpdateStepRunStatus(ctx, sr.ID, domain.StepWaiting, nil); err != nil {
				return fmt.Errorf("set promoted root step waiting by concurrency key %s: %w", step.StepRef, err)
			}
			continue
		}
		if err := e.startStep(ctx, sr, step, run, nil); err != nil {
			return fmt.Errorf("start promoted root step %s: %w", step.StepRef, err)
		}
		if sr.Status == domain.StepRunning {
			runningStarts++
			if step.ConcurrencyKey != "" {
				runningByConcurrencyKey[step.ConcurrencyKey]++
			}
		}
	}
	return nil
}

func (e *WorkflowEngine) recordSingletonAcquisition(ctx context.Context) {
	if e.metrics == nil || e.metrics.SingletonAcquisitions == nil {
		return
	}
	e.metrics.SingletonAcquisitions.Add(ctx, 1, metric.WithAttributes(
		attribute.String("kind", string(domain.SingletonKindWorkflow)),
	))
}

func (e *WorkflowEngine) recordSingletonConflict(ctx context.Context, policy domain.SingletonOnConflict) {
	if e.metrics == nil || e.metrics.SingletonConflicts == nil {
		return
	}
	e.metrics.SingletonConflicts.Add(ctx, 1, metric.WithAttributes(
		attribute.String("kind", string(domain.SingletonKindWorkflow)),
		attribute.String("policy", string(policy)),
	))
}
