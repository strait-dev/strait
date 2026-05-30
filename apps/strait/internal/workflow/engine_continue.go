package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
	"strait/internal/telemetry"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
)

var (
	// ErrWorkflowRunNotContinuable is returned when continue-as-new is requested
	// for a run that is not in a continuable (running or paused) state.
	ErrWorkflowRunNotContinuable = errors.New("workflow run is not in a continuable state")
	// ErrContinueDepthExceeded is returned when continuing would push the chain
	// past the configured lineage-depth cap.
	ErrContinueDepthExceeded = errors.New("workflow continuation lineage depth exceeds maximum")
	// ErrSubWorkflowNotContinuable is returned when continue-as-new is requested
	// for a sub-workflow run (one with a parent workflow run). The atomic handoff
	// marks the predecessor terminal-continued without invoking the parent
	// fan-in callback, which would orphan the parent step (or let a reaper
	// complete it with empty output). The top-level run must be continued instead.
	ErrSubWorkflowNotContinuable = errors.New("sub-workflow runs cannot be continued as new")
)

// continueBootstrapStore is the subset of the store that performs the atomic
// complete-predecessor + start-successor handoff for continue-as-new.
type continueBootstrapStore interface {
	ContinueWorkflowRunBootstrap(ctx context.Context, p store.ContinueWorkflowRunBootstrapParams) error
}

// ContinueWorkflowRunAsNew atomically completes a non-terminal workflow run and
// starts a fresh successor run of the same workflow with the caller-provided
// carry-over input. The successor's version is chosen by strategy: repin (the
// default) reuses the predecessor's exact pinned version and snapshot so the
// chain stays deterministic, while latest adopts the newest published version
// and canary routing so mid-chain deploys take effect. The successor starts with
// empty step history and links bidirectionally to its predecessor. The
// predecessor is marked continued and its in-flight work is torn down. A
// configurable lineage-depth cap guards against runaway chains.
func (e *WorkflowEngine) ContinueWorkflowRunAsNew(
	ctx context.Context,
	runID string,
	input json.RawMessage,
	strategy domain.ContinueVersionStrategy,
) (*domain.WorkflowRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "workflow.ContinueWorkflowRunAsNew")
	defer span.End()
	telemetry.AddSentryBreadcrumb(ctx, "workflow.state", "workflow continue-as-new requested", map[string]any{
		"workflow_run_id": runID,
	})

	// 1. Load the predecessor; it must be non-terminal (running or paused).
	pred, err := e.store.GetWorkflowRun(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("get predecessor workflow run: %w", err)
	}
	if pred == nil {
		return nil, fmt.Errorf("workflow run not found: %s", runID)
	}
	if pred.Status != domain.WfStatusRunning && pred.Status != domain.WfStatusPaused {
		return nil, fmt.Errorf("cannot continue workflow run %s: status is %s (must be running or paused): %w", runID, pred.Status, ErrWorkflowRunNotContinuable)
	}

	// 1a. Sub-workflow runs cannot be continued. The handoff flips the
	//     predecessor to terminal-continued in a single store transaction that
	//     bypasses the parent fan-in callback, so the parent step would never be
	//     notified: it stays running forever, or a reaper later completes it with
	//     the continued child's empty output. Continue the top-level run instead.
	if pred.ParentWorkflowRunID != "" || pred.ParentStepRunID != "" {
		return nil, fmt.Errorf("cannot continue workflow run %s: it is a sub-workflow run of %s: %w", runID, pred.ParentWorkflowRunID, ErrSubWorkflowNotContinuable)
	}

	// 2. Depth guard: enforce the configurable runaway-chain cap.
	nextDepth := pred.LineageDepth + 1
	if nextDepth > e.maxContinueDepth {
		return nil, fmt.Errorf("cannot continue workflow run %s: lineage depth %d exceeds max %d: %w", runID, nextDepth, e.maxContinueDepth, ErrContinueDepthExceeded)
	}

	// 3. Load the workflow as a shared existence, enabled, and project gate. The
	//    successor's actual version is resolved per strategy below.
	wf, err := e.store.GetWorkflow(ctx, pred.WorkflowID)
	if err != nil {
		return nil, fmt.Errorf("get workflow: %w", err)
	}
	if wf == nil {
		return nil, fmt.Errorf("workflow not found: %s", pred.WorkflowID)
	}
	if !wf.Enabled {
		return nil, fmt.Errorf("workflow is disabled: %s", pred.WorkflowID)
	}
	if projectChecker, ok := e.store.(projectExecutionStateStore); ok {
		runnable, checkErr := projectChecker.IsProjectRunnable(ctx, pred.ProjectID)
		if checkErr != nil {
			return nil, fmt.Errorf("check workflow project execution state: %w", checkErr)
		}
		if !runnable {
			return nil, fmt.Errorf("project %s is not active for workflow execution", pred.ProjectID)
		}
	}
	// 3a. Resolve the version, snapshot, and step DAG the successor pins.
	def, err := e.resolveContinuationDefinition(ctx, wf, pred, strategy)
	if err != nil {
		return nil, err
	}

	// 4. Build the successor run and its fresh step runs. Tags carry across the
	//    chain: workflow tags first, then predecessor run tags overlaid.
	runTags := workflowRunTags(wf.Tags, pred.Tags)

	// A single wall-clock reading anchors the successor's start, its expiry, and
	// the predecessor's finished_at so expires_at == started_at + timeout exactly.
	now := time.Now()
	successor := &domain.WorkflowRun{
		ID:                         uuid.Must(uuid.NewV7()).String(),
		WorkflowID:                 wf.ID,
		ProjectID:                  pred.ProjectID,
		Tags:                       runTags,
		Status:                     domain.WfStatusPending,
		TriggeredBy:                domain.TriggerContinuation,
		WorkflowVersion:            def.version,
		WorkflowVersionID:          def.versionID,
		WorkflowSnapshotID:         def.snapshotID,
		MaxParallelSteps:           def.maxParallel,
		Payload:                    input,
		ContinuedFromWorkflowRunID: pred.ID,
		LineageDepth:               nextDepth,
		TraceContext:               workflowTraceContext(ctx),
	}
	if def.timeoutSecs > 0 {
		expiresAt := now.Add(time.Duration(def.timeoutSecs) * time.Second)
		successor.ExpiresAt = &expiresAt
	}

	stepRuns := initialWorkflowStepRuns(successor.ID, def.steps)

	// 5. Atomic handoff: complete the predecessor and start the successor.
	bootstrapper, ok := e.store.(continueBootstrapStore)
	if !ok {
		return nil, fmt.Errorf("store does not support workflow continue-as-new")
	}
	if err := bootstrapper.ContinueWorkflowRunBootstrap(ctx, store.ContinueWorkflowRunBootstrapParams{
		PredecessorID: pred.ID,
		FromStatus:    pred.Status,
		Successor:     successor,
		StepRuns:      stepRuns,
		Now:           now,
	}); err != nil {
		return nil, fmt.Errorf("continue workflow run bootstrap: %w", err)
	}
	successor.Status = domain.WfStatusRunning
	successor.StartedAt = &now
	pred.ContinuedToWorkflowRunID = successor.ID

	// 6. Metrics: net the active-run gauge (predecessor done, successor live)
	//    and record the continuation + new chain depth.
	recordWorkflowActiveRunDelta(ctx, pred.ProjectID, -1)
	recordWorkflowActiveRunDelta(ctx, successor.ProjectID, 1)
	recordWorkflowContinuation(ctx, successor.ProjectID, nextDepth)

	telemetry.AddSentryBreadcrumb(ctx, "workflow.state", "workflow continued as new", map[string]any{
		"workflow_id":        successor.WorkflowID,
		"predecessor_run_id": pred.ID,
		"successor_run_id":   successor.ID,
		"project_id":         successor.ProjectID,
		"version":            successor.WorkflowVersion,
		"lineage_depth":      nextDepth,
		"step_count":         len(stepRuns),
	})

	// 7. Start the successor's root steps, exactly as a fresh trigger would. The
	//    handoff has already committed: the predecessor is durably continued and
	//    the successor is running. A failure here only means the root steps were
	//    not enqueued, so log it loudly with the committed lineage rather than
	//    letting a bare error imply nothing happened.
	roots := rootWorkflowSteps(def.steps, stepRuns)
	if err := e.startRootWorkflowSteps(ctx, successor, roots); err != nil {
		e.logger.ErrorContext(ctx, "continue-as-new committed but root step start failed",
			"predecessor_run_id", pred.ID,
			"successor_run_id", successor.ID,
			"project_id", successor.ProjectID,
			"lineage_depth", nextDepth,
			"error", err,
		)
		telemetry.AddSentryBreadcrumb(ctx, "workflow.state", "continue-as-new committed but root step start failed", map[string]any{
			"predecessor_run_id": pred.ID,
			"successor_run_id":   successor.ID,
			"project_id":         successor.ProjectID,
			"lineage_depth":      nextDepth,
		})
		return nil, err
	}

	return successor, nil
}

// continuationDefinition is the workflow definition a continue-as-new successor
// pins: the resolved version and snapshot plus the validated step DAG.
type continuationDefinition struct {
	version     int
	versionID   string
	snapshotID  string
	maxParallel int
	timeoutSecs int
	steps       []domain.WorkflowStep
}

// resolveContinuationDefinition settles which version, snapshot, and step DAG a
// continuation successor pins, per the chosen strategy. repin reuses the
// predecessor's exact pinned version and snapshot, so it is immune to mid-chain
// deploys and in-place edits. latest re-resolves the newest published version
// with canary routing, exactly as a fresh trigger would, then snapshots the
// resolved definition so the successor is immune to later live edits. The step
// DAG is listed and validated once the version is known, identically for both.
func (e *WorkflowEngine) resolveContinuationDefinition(
	ctx context.Context,
	wf *domain.Workflow,
	pred *domain.WorkflowRun,
	strategy domain.ContinueVersionStrategy,
) (continuationDefinition, error) {
	strategy = strategy.Normalize()
	def := continuationDefinition{timeoutSecs: wf.TimeoutSecs}

	switch strategy {
	case domain.ContinueVersionLatest:
		if err := e.applyCanaryRouting(ctx, wf); err != nil {
			return continuationDefinition{}, err
		}
		def.version, def.versionID, def.maxParallel = wf.Version, wf.VersionID, wf.MaxParallelSteps
	case domain.ContinueVersionRepin:
		def.version, def.versionID = pred.WorkflowVersion, pred.WorkflowVersionID
		def.snapshotID, def.maxParallel = pred.WorkflowSnapshotID, pred.MaxParallelSteps
	default:
		return continuationDefinition{}, fmt.Errorf("unknown continue-as-new version strategy: %q", strategy)
	}

	steps, err := e.store.ListStepsByWorkflowVersion(ctx, wf.ID, def.version)
	if err != nil {
		return continuationDefinition{}, fmt.Errorf("list workflow steps by version: %w", err)
	}
	if err := ValidateDAG(steps); err != nil {
		return continuationDefinition{}, fmt.Errorf("validate workflow dag: %w", err)
	}
	def.steps = steps

	if strategy == domain.ContinueVersionLatest {
		// Snapshot the resolved definition so the successor is immune to live edits.
		snapshot, err := e.store.GetOrCreateWorkflowSnapshot(ctx, wf, steps)
		if err != nil {
			return continuationDefinition{}, fmt.Errorf("create workflow snapshot: %w", err)
		}
		if snapshot != nil {
			def.snapshotID = snapshot.ID
		}
	}

	return def, nil
}
