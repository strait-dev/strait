package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"time"

	"strait/internal/domain"
	"strait/internal/telemetry"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	otelTrace "go.opentelemetry.io/otel/trace"
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
	ContinueWorkflowRunBootstrap(ctx context.Context, predecessorID string, fromStatus domain.WorkflowRunStatus, successor *domain.WorkflowRun, stepRuns []domain.WorkflowStepRun, now time.Time) error
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
	// Resolve which version and snapshot the successor pins, per the chosen
	// strategy. The switch settles only that choice; listing and validating the
	// step DAG is shared below because it is identical once the version is known.
	strategy = strategy.Normalize()
	var (
		version     int
		versionID   string
		snapshotID  string
		maxParallel int
	)
	switch strategy {
	case domain.ContinueVersionLatest:
		// Adopt the latest published version (+ canary routing), exactly as a
		// fresh trigger would, so the successor reflects mid-chain deploys. The
		// resolved definition is snapshotted below so it is immune to live edits.
		if err := e.applyCanaryRouting(ctx, wf); err != nil {
			return nil, err
		}
		version, versionID, maxParallel = wf.Version, wf.VersionID, wf.MaxParallelSteps
	case domain.ContinueVersionRepin:
		// Reuse the predecessor's exact pinned version and snapshot: no canary
		// routing, immune to mid-chain deploys and in-place edits.
		version, versionID = pred.WorkflowVersion, pred.WorkflowVersionID
		snapshotID, maxParallel = pred.WorkflowSnapshotID, pred.MaxParallelSteps
	default:
		return nil, fmt.Errorf("unknown continue-as-new version strategy: %q", strategy)
	}
	timeoutSecs := wf.TimeoutSecs

	steps, err := e.store.ListStepsByWorkflowVersion(ctx, wf.ID, version)
	if err != nil {
		return nil, fmt.Errorf("list workflow steps by version: %w", err)
	}
	if err := ValidateDAG(steps); err != nil {
		return nil, fmt.Errorf("validate workflow dag: %w", err)
	}
	if strategy == domain.ContinueVersionLatest {
		// Snapshot the resolved definition so the successor is immune to live edits.
		snapshot, snapshotErr := e.store.GetOrCreateWorkflowSnapshot(ctx, wf, steps)
		if snapshotErr != nil {
			return nil, fmt.Errorf("create workflow snapshot: %w", snapshotErr)
		}
		if snapshot != nil {
			snapshotID = snapshot.ID
		}
	}

	// 4. Build the successor run and its fresh step runs. Tags carry across the
	//    chain: workflow tags first, then predecessor run tags overlaid.
	var runTags map[string]string
	if len(wf.Tags) > 0 || len(pred.Tags) > 0 {
		runTags = make(map[string]string, len(wf.Tags)+len(pred.Tags))
		maps.Copy(runTags, wf.Tags)
		maps.Copy(runTags, pred.Tags)
	}

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
		WorkflowVersion:            version,
		WorkflowVersionID:          versionID,
		WorkflowSnapshotID:         snapshotID,
		MaxParallelSteps:           maxParallel,
		Payload:                    input,
		ContinuedFromWorkflowRunID: pred.ID,
		LineageDepth:               nextDepth,
		TraceContext:               currentTraceContext(ctx),
	}
	if timeoutSecs > 0 {
		expiresAt := now.Add(time.Duration(timeoutSecs) * time.Second)
		successor.ExpiresAt = &expiresAt
	}

	stepRuns := initialWorkflowStepRuns(successor.ID, steps)

	// 5. Atomic handoff: complete the predecessor and start the successor.
	bootstrapper, ok := e.store.(continueBootstrapStore)
	if !ok {
		return nil, fmt.Errorf("store does not support workflow continue-as-new")
	}
	if err := bootstrapper.ContinueWorkflowRunBootstrap(ctx, pred.ID, pred.Status, successor, stepRuns, now); err != nil {
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
	roots := rootWorkflowSteps(steps, stepRuns)
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

// currentTraceContext extracts the W3C trace context from the active OTel span
// for propagation onto a new workflow run, mirroring the trigger path.
func currentTraceContext(ctx context.Context) map[string]string {
	spanCtx := otelTrace.SpanContextFromContext(ctx)
	if !spanCtx.IsValid() {
		return nil
	}
	traceCtx := map[string]string{
		"traceparent": fmt.Sprintf("00-%s-%s-%s", spanCtx.TraceID(), spanCtx.SpanID(), spanCtx.TraceFlags()),
	}
	if spanCtx.TraceState().Len() > 0 {
		ts := spanCtx.TraceState().String()
		if len(ts) <= 512 {
			traceCtx["tracestate"] = ts
		}
	}
	return traceCtx
}
