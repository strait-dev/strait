package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sort"
	"strings"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
	"strait/internal/workflow"

	"github.com/danielgtaylor/huma/v2"
	"github.com/samber/lo"
)

type workflowStepRefHeap []string

func (h *workflowStepRefHeap) init() {
	for i := len(*h)/2 - 1; i >= 0; i-- {
		h.siftDown(i)
	}
}

func (h *workflowStepRefHeap) push(ref string) {
	*h = append(*h, ref)
	h.siftUp(len(*h) - 1)
}

func (h *workflowStepRefHeap) pop() string {
	old := *h
	n := len(old)
	ref := old[0]
	old[0] = old[n-1]
	old[n-1] = ""
	*h = old[:n-1]
	h.siftDown(0)
	return ref
}

func (h *workflowStepRefHeap) siftUp(i int) {
	refs := *h
	for i > 0 {
		parent := (i - 1) / 2
		if refs[parent] <= refs[i] {
			return
		}
		refs[parent], refs[i] = refs[i], refs[parent]
		i = parent
	}
}

func (h *workflowStepRefHeap) siftDown(i int) {
	refs := *h
	for {
		left := 2*i + 1
		if left >= len(refs) {
			return
		}
		child := left
		if right := left + 1; right < len(refs) && refs[right] < refs[left] {
			child = right
		}
		if refs[i] <= refs[child] {
			return
		}
		refs[i], refs[child] = refs[child], refs[i]
		i = child
	}
}

type approveWorkflowStepRequest struct {
	Approver string `json:"approver,omitempty"` // deprecated: ignored, approver is taken from auth context
}

type skipStepRequest struct {
	Reason string `json:"reason,omitempty"`
}

type forceCompleteStepRequest struct {
	Result json.RawMessage `json:"result,omitempty"`
}

type ListWorkflowRunsInput struct {
	WorkflowID string `path:"workflowID"`
	Limit      string `query:"limit"`
	Cursor     string `query:"cursor"`
}
type ListWorkflowRunsOutput struct{ Body PaginatedResponse }

func (s *Server) handleListWorkflowRuns(ctx context.Context, input *ListWorkflowRunsInput) (*ListWorkflowRunsOutput, error) {
	// Verify the parent workflow belongs to the caller's project.
	wf, err := s.store.GetWorkflow(ctx, input.WorkflowID)
	if err != nil {
		return nil, huma.Error404NotFound("workflow not found")
	}
	if err := requireProjectMatch(ctx, wf.ProjectID); err != nil {
		return nil, huma.Error404NotFound("workflow not found")
	}

	limit, cursor, err := parsePaginationFromStrings(input.Limit, input.Cursor)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	runs, err := s.store.ListWorkflowRuns(ctx, input.WorkflowID, limit+1, cursor)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list workflow runs")
	}

	return &ListWorkflowRunsOutput{Body: paginatedResult(runs, limit, func(run domain.WorkflowRun) string {
		return run.CreatedAt.Format(time.RFC3339Nano)
	})}, nil
}

type ListWorkflowRunsByProjectInput struct {
	TagKey   string `query:"tag_key"`
	TagValue string `query:"tag_value"`
	Status   string `query:"status"`
	Limit    string `query:"limit"`
	Cursor   string `query:"cursor"`
}
type ListWorkflowRunsByProjectOutput struct{ Body PaginatedResponse }

func (s *Server) handleListWorkflowRunsByProject(ctx context.Context, input *ListWorkflowRunsByProjectInput) (*ListWorkflowRunsByProjectOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}

	if input.TagValue != "" && input.TagKey == "" {
		return nil, huma.Error400BadRequest("tag_key is required when tag_value is provided")
	}

	var status *domain.WorkflowRunStatus
	if input.Status != "" {
		parsed := domain.WorkflowRunStatus(input.Status)
		if !parsed.IsValid() {
			return nil, huma.Error400BadRequest("status is invalid")
		}
		status = &parsed
	}

	limit, cursor, err := parsePaginationFromStrings(input.Limit, input.Cursor)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	var runs []domain.WorkflowRun
	if input.TagKey != "" {
		runs, err = s.store.ListWorkflowRunsByTag(ctx, projectID, input.TagKey, input.TagValue, limit+1, cursor)
	} else {
		runs, err = s.store.ListWorkflowRunsByProject(ctx, projectID, status, limit+1, cursor)
	}
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list workflow runs")
	}

	return &ListWorkflowRunsByProjectOutput{Body: paginatedResult(runs, limit, func(run domain.WorkflowRun) string {
		return run.CreatedAt.Format(time.RFC3339Nano)
	})}, nil
}

type GetWorkflowRunInput struct {
	WorkflowRunID string `path:"workflowRunID"`
}
type GetWorkflowRunOutput struct{ Body *domain.WorkflowRun }

func (s *Server) handleGetWorkflowRun(ctx context.Context, input *GetWorkflowRunInput) (*GetWorkflowRunOutput, error) {
	run, err := s.store.GetWorkflowRun(ctx, input.WorkflowRunID)
	if err != nil {
		if errors.Is(err, store.ErrWorkflowRunNotFound) {
			return nil, huma.Error404NotFound("workflow run not found")
		}
		return nil, huma.Error500InternalServerError("failed to get workflow run")
	}
	if err := requireProjectMatch(ctx, run.ProjectID); err != nil {
		return nil, huma.Error404NotFound("workflow run not found")
	}

	return &GetWorkflowRunOutput{Body: run}, nil
}

type CancelWorkflowRunInput struct {
	WorkflowRunID string `path:"workflowRunID"`
}
type CancelWorkflowRunOutput struct{ Body *domain.WorkflowRun }

func (s *Server) handleCancelWorkflowRun(ctx context.Context, input *CancelWorkflowRunInput) (*CancelWorkflowRunOutput, error) {
	run, err := s.store.GetWorkflowRun(ctx, input.WorkflowRunID)
	if err != nil {
		if errors.Is(err, store.ErrWorkflowRunNotFound) {
			return nil, huma.Error404NotFound("workflow run not found")
		}
		return nil, huma.Error500InternalServerError("failed to get workflow run")
	}
	if err := requireProjectMatch(ctx, run.ProjectID); err != nil {
		return nil, huma.Error404NotFound("workflow run not found")
	}

	if run.Status.IsTerminal() {
		return nil, huma.Error400BadRequest("workflow run already in terminal state")
	}

	if err := s.store.UpdateWorkflowRunStatus(ctx, run.ID, run.Status, domain.WfStatusCanceled, map[string]any{
		"finished_at": time.Now(),
		"error":       "canceled by user",
	}); err != nil {
		return nil, huma.Error409Conflict("failed to cancel workflow run")
	}
	now := time.Now()
	reason := "workflow canceled by user"

	// Bulk-cancel all non-terminal step runs in one UPDATE.
	if _, err := s.store.CancelNonTerminalStepRuns(ctx, run.ID, now, reason); err != nil {
		return nil, huma.Error500InternalServerError("failed to cancel workflow step runs")
	}

	// Bulk-cancel all non-terminal job runs linked to this workflow run.
	if _, err := s.store.CancelJobRunsByWorkflowRun(ctx, run.ID, now, reason); err != nil {
		return nil, huma.Error500InternalServerError("failed to cancel workflow job runs")
	}

	// Cancel any pending event triggers for this workflow (non-fatal).
	if _, triggerErr := s.store.CancelEventTriggersByWorkflowRun(ctx, run.ID); triggerErr != nil {
		slog.Warn("failed to cancel event triggers for workflow (non-fatal)", "workflow_run_id", run.ID, "error", triggerErr)
	}

	updatedRun, err := s.store.GetWorkflowRun(ctx, run.ID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get updated workflow run")
	}
	s.publishWorkflowRunHook(ctx, updatedRun, run.Status, domain.WfStatusCanceled, "cancel")

	s.emitAuditEvent(ctx, domain.AuditActionWorkflowRunCancelled, "workflow_run", run.ID, map[string]any{
		"workflow_id":     run.WorkflowID,
		"previous_status": string(run.Status),
	})

	return &CancelWorkflowRunOutput{Body: updatedRun}, nil
}

type PauseWorkflowRunInput struct {
	WorkflowRunID string `path:"workflowRunID"`
}
type PauseWorkflowRunOutput struct{ Body *domain.WorkflowRun }

func (s *Server) handlePauseWorkflowRun(ctx context.Context, input *PauseWorkflowRunInput) (*PauseWorkflowRunOutput, error) {
	run, err := s.store.GetWorkflowRun(ctx, input.WorkflowRunID)
	if err != nil {
		if errors.Is(err, store.ErrWorkflowRunNotFound) {
			return nil, huma.Error404NotFound("workflow run not found")
		}
		return nil, huma.Error500InternalServerError("failed to get workflow run")
	}
	if err := requireProjectMatch(ctx, run.ProjectID); err != nil {
		return nil, huma.Error404NotFound("workflow run not found")
	}
	if run.Status.IsTerminal() {
		return nil, huma.Error400BadRequest("workflow run already in terminal state")
	}
	if run.Status == domain.WfStatusPaused {
		return &PauseWorkflowRunOutput{Body: run}, nil
	}
	if run.Status != domain.WfStatusRunning {
		return nil, huma.Error400BadRequest("workflow run can only be paused from running state")
	}

	if err := s.store.UpdateWorkflowRunStatus(ctx, run.ID, domain.WfStatusRunning, domain.WfStatusPaused, nil); err != nil {
		return nil, huma.Error409Conflict("failed to pause workflow run")
	}

	// Mark affected job runs so resume knows to re-dispatch them (non-fatal).
	if _, markErr := s.store.MarkJobRunsPausedByWorkflowRun(ctx, run.ID); markErr != nil {
		slog.Warn("failed to mark job runs paused", "workflow_run_id", run.ID, "error", markErr)
	}

	updatedRun, err := s.store.GetWorkflowRun(ctx, run.ID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get updated workflow run")
	}
	s.publishWorkflowRunHook(ctx, updatedRun, run.Status, domain.WfStatusPaused, "pause")

	s.emitAuditEvent(ctx, domain.AuditActionWorkflowRunPaused, "workflow_run", run.ID, map[string]any{
		"workflow_id": run.WorkflowID,
	})

	return &PauseWorkflowRunOutput{Body: updatedRun}, nil
}

type ResumeWorkflowRunInput struct {
	WorkflowRunID string `path:"workflowRunID"`
}
type ResumeWorkflowRunOutput struct{ Body *domain.WorkflowRun }

func (s *Server) handleResumeWorkflowRun(ctx context.Context, input *ResumeWorkflowRunInput) (*ResumeWorkflowRunOutput, error) {
	if s.workflowCallback == nil {
		return nil, huma.Error503ServiceUnavailable("workflow callback unavailable")
	}

	run, err := s.store.GetWorkflowRun(ctx, input.WorkflowRunID)
	if err != nil {
		if errors.Is(err, store.ErrWorkflowRunNotFound) {
			return nil, huma.Error404NotFound("workflow run not found")
		}
		return nil, huma.Error500InternalServerError("failed to get workflow run")
	}
	if err := requireProjectMatch(ctx, run.ProjectID); err != nil {
		return nil, huma.Error404NotFound("workflow run not found")
	}
	if run.Status != domain.WfStatusPaused {
		return nil, huma.Error400BadRequest("workflow run is not paused")
	}

	if err := s.workflowCallback.ResumeWorkflowRun(ctx, input.WorkflowRunID); err != nil {
		return nil, huma.Error409Conflict(err.Error())
	}

	updatedRun, err := s.store.GetWorkflowRun(ctx, run.ID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get updated workflow run")
	}
	s.publishWorkflowRunHook(ctx, updatedRun, run.Status, domain.WfStatusRunning, "resume")

	s.emitAuditEvent(ctx, domain.AuditActionWorkflowRunResumed, "workflow_run", run.ID, map[string]any{
		"workflow_id": run.WorkflowID,
	})

	return &ResumeWorkflowRunOutput{Body: updatedRun}, nil
}

type GetWorkflowRunLabelsInput struct {
	WorkflowRunID string `path:"workflowRunID"`
}
type GetWorkflowRunLabelsOutput struct{ Body any }

func (s *Server) handleGetWorkflowRunLabels(ctx context.Context, input *GetWorkflowRunLabelsInput) (*GetWorkflowRunLabelsOutput, error) {
	run, err := s.store.GetWorkflowRun(ctx, input.WorkflowRunID)
	if err != nil {
		if errors.Is(err, store.ErrWorkflowRunNotFound) {
			return nil, huma.Error404NotFound("workflow run not found")
		}
		return nil, huma.Error500InternalServerError("failed to get workflow run")
	}
	if err := requireProjectMatch(ctx, run.ProjectID); err != nil {
		return nil, huma.Error404NotFound("workflow run not found")
	}

	labels, err := s.store.ListWorkflowRunLabels(ctx, input.WorkflowRunID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list workflow run labels")
	}
	return &GetWorkflowRunLabelsOutput{Body: map[string]any{"labels": labels}}, nil
}

type ListWorkflowStepRunsInput struct {
	WorkflowRunID string `path:"workflowRunID"`
	Limit         string `query:"limit"`
	Cursor        string `query:"cursor"`
}
type ListWorkflowStepRunsOutput struct{ Body PaginatedResponse }

func (s *Server) handleListWorkflowStepRuns(ctx context.Context, input *ListWorkflowStepRunsInput) (*ListWorkflowStepRunsOutput, error) {
	run, err := s.store.GetWorkflowRun(ctx, input.WorkflowRunID)
	if err != nil {
		if errors.Is(err, store.ErrWorkflowRunNotFound) {
			return nil, huma.Error404NotFound("workflow run not found")
		}
		return nil, huma.Error500InternalServerError("failed to get workflow run")
	}
	if err := requireProjectMatch(ctx, run.ProjectID); err != nil {
		return nil, huma.Error404NotFound("workflow run not found")
	}

	limit, cursor, err := parsePaginationFromStrings(input.Limit, input.Cursor)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	stepRuns, err := s.store.ListStepRunsByWorkflowRun(ctx, input.WorkflowRunID, limit+1, cursor)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list workflow step runs")
	}

	return &ListWorkflowStepRunsOutput{Body: paginatedResult(stepRuns, limit, func(sr domain.WorkflowStepRun) string {
		return sr.CreatedAt.Format(time.RFC3339Nano)
	})}, nil
}

func (s *Server) loadScopedWorkflowRunForStepMutation(ctx context.Context, workflowRunID string) (*domain.WorkflowRun, error) {
	run, err := s.store.GetWorkflowRun(ctx, workflowRunID)
	if err != nil {
		if errors.Is(err, store.ErrWorkflowRunNotFound) {
			return nil, huma.Error404NotFound("workflow run not found")
		}
		return nil, huma.Error500InternalServerError("failed to get workflow run")
	}
	if run == nil {
		return nil, huma.Error404NotFound("workflow run not found")
	}
	if projectID := projectIDFromContext(ctx); projectID != "" && run.ProjectID != projectID {
		return nil, huma.Error404NotFound("workflow run not found")
	}
	return run, nil
}

type ApproveWorkflowStepInput struct {
	WorkflowRunID string                     `path:"workflowRunID"`
	StepRef       string                     `path:"stepRef"`
	Body          approveWorkflowStepRequest `json:"body"`
}
type ApproveWorkflowStepOutput struct{ Body any }

func (s *Server) handleApproveWorkflowStep(ctx context.Context, input *ApproveWorkflowStepInput) (*ApproveWorkflowStepOutput, error) {
	if s.workflowCallback == nil {
		return nil, huma.Error503ServiceUnavailable("workflow callback unavailable")
	}

	beforeRun, err := s.loadScopedWorkflowRunForStepMutation(ctx, input.WorkflowRunID)
	if err != nil {
		return nil, err
	}

	approver := actorFromContext(ctx)
	if approver == "" {
		return nil, huma.Error401Unauthorized("authenticated identity required")
	}

	if err := s.workflowCallback.ApproveStep(ctx, input.WorkflowRunID, input.StepRef, approver); err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	stepRun, err := s.store.GetStepRunByWorkflowRunAndRef(ctx, input.WorkflowRunID, input.StepRef)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to fetch workflow step run")
	}
	approval, err := s.store.GetWorkflowStepApprovalByStepRunID(ctx, stepRun.ID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to fetch workflow step approval")
	}

	afterRun, afterErr := s.store.GetWorkflowRun(ctx, input.WorkflowRunID)
	if afterErr != nil {
		slog.Warn("failed to get workflow run after approve step", "workflow_run_id", input.WorkflowRunID, "error", afterErr)
	}
	if afterErr == nil && afterRun != nil && beforeRun.Status != afterRun.Status {
		s.publishWorkflowRunHook(ctx, afterRun, beforeRun.Status, afterRun.Status, "approve_step")
	}

	s.emitAuditEvent(ctx, domain.AuditActionWorkflowStepApproved, "workflow_step", stepRun.ID, map[string]any{
		"workflow_run_id": input.WorkflowRunID,
		"step_ref":        input.StepRef,
		"approver":        approver,
	})

	return &ApproveWorkflowStepOutput{Body: map[string]any{
		"step_run": stepRun,
		"approval": approval,
	}}, nil
}

type SkipWorkflowStepInput struct {
	WorkflowRunID string          `path:"workflowRunID"`
	StepRef       string          `path:"stepRef"`
	Body          skipStepRequest `json:"body"`
}
type SkipWorkflowStepOutput struct{ Body any }

func (s *Server) handleSkipWorkflowStep(ctx context.Context, input *SkipWorkflowStepInput) (*SkipWorkflowStepOutput, error) {
	if s.workflowCallback == nil {
		return nil, huma.Error503ServiceUnavailable("workflow callback unavailable")
	}

	beforeRun, err := s.loadScopedWorkflowRunForStepMutation(ctx, input.WorkflowRunID)
	if err != nil {
		return nil, err
	}

	if err := s.workflowCallback.SkipStep(ctx, input.WorkflowRunID, input.StepRef, input.Body.Reason, actorFromContext(ctx)); err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	stepRun, err := s.store.GetStepRunByWorkflowRunAndRef(ctx, input.WorkflowRunID, input.StepRef)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to fetch workflow step run")
	}

	afterRun, afterErr := s.store.GetWorkflowRun(ctx, input.WorkflowRunID)
	if afterErr != nil {
		slog.Warn("failed to get workflow run after skip step", "workflow_run_id", input.WorkflowRunID, "error", afterErr)
	}
	if afterErr == nil && afterRun != nil && beforeRun.Status != afterRun.Status {
		s.publishWorkflowRunHook(ctx, afterRun, beforeRun.Status, afterRun.Status, "skip_step")
	}

	s.emitAuditEvent(ctx, domain.AuditActionWorkflowStepSkipped, "workflow_step", stepRun.ID, map[string]any{
		"workflow_run_id": input.WorkflowRunID,
		"step_ref":        input.StepRef,
		"reason":          input.Body.Reason,
	})

	return &SkipWorkflowStepOutput{Body: map[string]any{"step_run": stepRun}}, nil
}

type ForceCompleteWorkflowStepInput struct {
	WorkflowRunID string                   `path:"workflowRunID"`
	StepRef       string                   `path:"stepRef"`
	Body          forceCompleteStepRequest `json:"body"`
}
type ForceCompleteWorkflowStepOutput struct{ Body any }

func (s *Server) handleForceCompleteWorkflowStep(ctx context.Context, input *ForceCompleteWorkflowStepInput) (*ForceCompleteWorkflowStepOutput, error) {
	if s.workflowCallback == nil {
		return nil, huma.Error503ServiceUnavailable("workflow callback unavailable")
	}

	beforeRun, err := s.loadScopedWorkflowRunForStepMutation(ctx, input.WorkflowRunID)
	if err != nil {
		return nil, err
	}

	if err := s.workflowCallback.ForceCompleteStep(ctx, input.WorkflowRunID, input.StepRef, input.Body.Result); err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	stepRun, err := s.store.GetStepRunByWorkflowRunAndRef(ctx, input.WorkflowRunID, input.StepRef)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to fetch workflow step run")
	}

	afterRun, afterErr := s.store.GetWorkflowRun(ctx, input.WorkflowRunID)
	if afterErr != nil {
		slog.Warn("failed to get workflow run after force-complete step", "workflow_run_id", input.WorkflowRunID, "error", afterErr)
	}
	if afterErr == nil && afterRun != nil && beforeRun.Status != afterRun.Status {
		s.publishWorkflowRunHook(ctx, afterRun, beforeRun.Status, afterRun.Status, "force_complete_step")
	}

	s.emitAuditEvent(ctx, domain.AuditActionWorkflowStepForceCompleted, "workflow_step", stepRun.ID, map[string]any{
		"workflow_run_id": input.WorkflowRunID,
		"step_ref":        input.StepRef,
	})

	return &ForceCompleteWorkflowStepOutput{Body: map[string]any{"step_run": stepRun}}, nil
}

type RetryWorkflowRunInput struct {
	WorkflowRunID string `path:"workflowRunID"`
}
type RetryWorkflowRunOutput struct{ Body *domain.WorkflowRun }

func (s *Server) handleRetryWorkflowRun(ctx context.Context, input *RetryWorkflowRunInput) (*RetryWorkflowRunOutput, error) {
	if s.workflowEngine == nil {
		return nil, huma.Error503ServiceUnavailable("workflow engine unavailable")
	}

	run, err := s.store.GetWorkflowRun(ctx, input.WorkflowRunID)
	if err != nil {
		if errors.Is(err, store.ErrWorkflowRunNotFound) {
			return nil, huma.Error404NotFound("workflow run not found")
		}
		return nil, huma.Error500InternalServerError("failed to get workflow run")
	}
	if err := requireProjectMatch(ctx, run.ProjectID); err != nil {
		return nil, huma.Error404NotFound("workflow run not found")
	}

	if !run.Status.IsTerminal() {
		return nil, huma.Error400BadRequest("can only retry a workflow run in terminal state")
	}

	newRun, err := s.workflowEngine.RetryWorkflowRun(ctx, input.WorkflowRunID)
	if err != nil {
		return nil, huma.Error500InternalServerError(fmt.Sprintf("failed to retry workflow run: %v", err))
	}

	s.publishWorkflowRunHook(ctx, newRun, domain.WfStatusPending, newRun.Status, "retry")

	s.emitAuditEvent(ctx, domain.AuditActionWorkflowRunRetried, "workflow_run", newRun.ID, map[string]any{
		"original_run_id": input.WorkflowRunID,
		"workflow_id":     run.WorkflowID,
	})

	return &RetryWorkflowRunOutput{Body: newRun}, nil
}

type ContinueWorkflowRunAsNewInput struct {
	WorkflowRunID string `path:"workflowRunID"`
	Body          struct {
		Input           json.RawMessage `json:"input,omitempty" doc:"Carry-over input for the successor run. Opaque JSON forwarded as the successor's payload."`
		VersionStrategy string          `json:"versionStrategy,omitempty" enum:"repin,latest" doc:"Which workflow version the successor runs. 'repin' (default) reuses the predecessor's pinned version and snapshot for deterministic chains; 'latest' adopts the newest published version and canary routing."`
	}
}
type ContinueWorkflowRunAsNewOutput struct{ Body *domain.WorkflowRun }

// handleContinueWorkflowRunAsNew atomically completes a running or paused
// workflow run and starts a fresh successor run of the same workflow with the
// caller-provided carry-over input. The successor's version is chosen by
// versionStrategy: repin (the default) reuses the predecessor's pinned version
// and snapshot, while latest re-resolves the newest published version. The
// predecessor is marked continued and linked bidirectionally to the successor.
// This is the workflow-level continue-as-new primitive.
func (s *Server) handleContinueWorkflowRunAsNew(ctx context.Context, input *ContinueWorkflowRunAsNewInput) (*ContinueWorkflowRunAsNewOutput, error) {
	if s.workflowEngine == nil {
		return nil, huma.Error503ServiceUnavailable("workflow engine unavailable")
	}

	run, err := s.store.GetWorkflowRun(ctx, input.WorkflowRunID)
	if err != nil {
		if errors.Is(err, store.ErrWorkflowRunNotFound) {
			return nil, huma.Error404NotFound("workflow run not found")
		}
		return nil, huma.Error500InternalServerError("failed to get workflow run")
	}
	if err := requireProjectMatch(ctx, run.ProjectID); err != nil {
		return nil, huma.Error404NotFound("workflow run not found")
	}

	if run.Status.IsTerminal() {
		return nil, huma.Error400BadRequest("can only continue a running or paused workflow run")
	}

	strategy := domain.ContinueVersionStrategy(input.Body.VersionStrategy)
	if !strategy.IsValid() {
		return nil, huma.Error400BadRequest("versionStrategy must be \"repin\" or \"latest\"")
	}

	successor, err := s.workflowEngine.ContinueWorkflowRunAsNew(ctx, input.WorkflowRunID, input.Body.Input, strategy)
	if err != nil {
		switch {
		case errors.Is(err, workflow.ErrWorkflowRunNotContinuable):
			return nil, huma.Error400BadRequest("can only continue a running or paused workflow run")
		case errors.Is(err, workflow.ErrContinueDepthExceeded):
			return nil, huma.Error400BadRequest("workflow continuation depth limit exceeded")
		case errors.Is(err, store.ErrWorkflowRunContinueConflict):
			return nil, huma.Error409Conflict("workflow run already continued or no longer continuable")
		default:
			slog.Error("continue workflow run failed", "workflow_run_id", input.WorkflowRunID, "error", err)
			return nil, huma.Error500InternalServerError("failed to continue workflow run")
		}
	}

	s.publishWorkflowRunHook(ctx, successor, domain.WfStatusPending, successor.Status, "continue_as_new")

	s.emitAuditEvent(ctx, domain.AuditActionWorkflowRunContinuedAsNew, "workflow_run", input.WorkflowRunID, map[string]any{
		"workflow_id":      run.WorkflowID,
		"successor_run_id": successor.ID,
		"lineage_depth":    successor.LineageDepth,
		"version_strategy": string(strategy.Normalize()),
	})

	return &ContinueWorkflowRunAsNewOutput{Body: successor}, nil
}

type GetWorkflowRunChainInput struct {
	WorkflowRunID string `path:"workflowRunID"`
	Limit         string `query:"limit"`
	Cursor        string `query:"cursor"`
}
type GetWorkflowRunChainOutput struct{ Body PaginatedResponse }

// handleGetWorkflowRunChain returns one cursor-paginated page of the
// continue-as-new lineage the given run belongs to, ordered root-first, so
// callers can navigate to the first or latest run in a continuation chain
// without materializing the whole chain. Each entry is a lightweight projection;
// full run detail is fetched on demand via the run-detail endpoint. The cursor
// is the id of the last run on the previous page.
func (s *Server) handleGetWorkflowRunChain(ctx context.Context, input *GetWorkflowRunChainInput) (*GetWorkflowRunChainOutput, error) {
	run, err := s.store.GetWorkflowRun(ctx, input.WorkflowRunID)
	if err != nil {
		if errors.Is(err, store.ErrWorkflowRunNotFound) {
			return nil, huma.Error404NotFound("workflow run not found")
		}
		return nil, huma.Error500InternalServerError("failed to get workflow run")
	}
	if err := requireProjectMatch(ctx, run.ProjectID); err != nil {
		return nil, huma.Error404NotFound("workflow run not found")
	}

	limit := parseLimitParam(input.Limit)

	// Scope the walk to the run's project so an untrusted cursor cannot reach
	// another tenant's chain.
	chain, err := s.store.GetWorkflowRunChain(ctx, input.WorkflowRunID, run.ProjectID, limit+1, input.Cursor)
	if err != nil {
		if errors.Is(err, store.ErrWorkflowRunNotFound) {
			return nil, huma.Error404NotFound("workflow run not found")
		}
		return nil, huma.Error500InternalServerError("failed to get workflow run chain")
	}

	return &GetWorkflowRunChainOutput{Body: paginatedResult(chain, limit, func(e domain.WorkflowRunChainEntry) string {
		return e.ID
	})}, nil
}

type workflowRunGraphNode struct {
	StepRef    string                  `json:"step_ref"`
	Type       domain.WorkflowStepType `json:"type"`
	Status     domain.StepRunStatus    `json:"status"`
	DependsOn  []string                `json:"depends_on,omitempty"`
	Attempt    int                     `json:"attempt"`
	StartedAt  *time.Time              `json:"started_at,omitempty"`
	FinishedAt *time.Time              `json:"finished_at,omitempty"`
	DurationMS int64                   `json:"duration_ms,omitempty"`
}

type GetWorkflowRunGraphInput struct {
	WorkflowRunID string `path:"workflowRunID"`
}
type GetWorkflowRunGraphOutput struct{ Body any }

func (s *Server) handleGetWorkflowRunGraph(ctx context.Context, input *GetWorkflowRunGraphInput) (*GetWorkflowRunGraphOutput, error) {
	run, err := s.store.GetWorkflowRun(ctx, input.WorkflowRunID)
	if err != nil {
		return nil, huma.Error404NotFound("workflow run not found")
	}
	if err := requireProjectMatch(ctx, run.ProjectID); err != nil {
		return nil, huma.Error404NotFound("workflow run not found")
	}
	steps, err := s.store.ListStepsByWorkflowVersion(ctx, run.WorkflowID, run.WorkflowVersion)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list workflow steps")
	}
	stepRuns, err := s.store.ListStepRunsByWorkflowRun(ctx, input.WorkflowRunID, 10000, nil)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list step runs")
	}

	runByRef := make(map[string]domain.WorkflowStepRun, len(stepRuns))
	for _, sr := range stepRuns {
		runByRef[sr.StepRef] = sr
	}

	now := time.Now()
	nodes := make([]workflowRunGraphNode, 0, len(steps))
	edges := make([]map[string]string, 0)
	roots := make([]string, 0)
	runnable := make([]string, 0)
	for _, st := range steps {
		sr := runByRef[st.StepRef]
		node := workflowRunGraphNode{StepRef: st.StepRef, Type: st.StepType, Status: sr.Status, DependsOn: st.DependsOn, Attempt: sr.Attempt}
		if sr.StartedAt != nil {
			node.StartedAt = sr.StartedAt
			if sr.FinishedAt != nil {
				node.DurationMS = sr.FinishedAt.Sub(*sr.StartedAt).Milliseconds()
			} else {
				node.DurationMS = now.Sub(*sr.StartedAt).Milliseconds()
			}
		}
		if sr.FinishedAt != nil {
			node.FinishedAt = sr.FinishedAt
		}
		nodes = append(nodes, node)
		if len(st.DependsOn) == 0 {
			roots = append(roots, st.StepRef)
		}
		if sr.Status == domain.StepPending || sr.Status == domain.StepWaiting {
			if sr.DepsRequired == sr.DepsCompleted {
				runnable = append(runnable, st.StepRef)
			}
		}
		for _, dep := range st.DependsOn {
			edges = append(edges, map[string]string{"from": dep, "to": st.StepRef})
		}
	}
	sort.Strings(roots)
	sort.Strings(runnable)
	criticalPath, estimatedDurationMS, estimatedRemainingMS := estimateWorkflowCriticalPath(steps, runByRef, now)

	return &GetWorkflowRunGraphOutput{Body: map[string]any{
		"workflow_run_id":            run.ID,
		"workflow_id":                run.WorkflowID,
		"version":                    run.WorkflowVersion,
		"nodes":                      nodes,
		"edges":                      edges,
		"roots":                      roots,
		"runnable":                   runnable,
		"critical_path":              criticalPath,
		"critical_path_estimate_ms":  estimatedDurationMS,
		"critical_path_remaining_ms": estimatedRemainingMS,
	}}, nil
}

func estimateWorkflowCriticalPath(steps []domain.WorkflowStep, runByRef map[string]domain.WorkflowStepRun, now time.Time) ([]string, int64, int64) {
	if len(steps) == 0 {
		return nil, 0, 0
	}

	stepByRef := lo.KeyBy(steps, func(step domain.WorkflowStep) string { return step.StepRef })
	indegree := make(map[string]int, len(steps))
	children := make(map[string][]string, len(steps))
	for _, step := range steps {
		indegree[step.StepRef] = 0
		children[step.StepRef] = []string{}
	}
	for _, step := range steps {
		for _, dep := range step.DependsOn {
			if _, ok := indegree[dep]; !ok {
				continue
			}
			children[dep] = append(children[dep], step.StepRef)
			indegree[step.StepRef]++
		}
	}

	queue := make(workflowStepRefHeap, 0, len(steps))
	for ref, degree := range indegree {
		if degree == 0 {
			queue = append(queue, ref)
		}
	}
	queue.init()

	prev := make(map[string]string, len(steps))
	longestByRef := make(map[string]int64, len(steps))
	totalEstimateByRef := make(map[string]int64, len(steps))
	remainingByRef := make(map[string]int64, len(steps))
	for len(queue) > 0 {
		ref := queue.pop()

		step := stepByRef[ref]
		stepRun := runByRef[ref]
		totalEstimateMS, remainingMS := estimateStepTiming(step, stepRun, now)
		totalEstimateByRef[ref] = totalEstimateMS
		remainingByRef[ref] = remainingMS

		bestParentRef := ""
		bestParentDistance := int64(0)
		for _, dep := range step.DependsOn {
			distance, ok := longestByRef[dep]
			if !ok {
				continue
			}
			if distance > bestParentDistance {
				bestParentDistance = distance
				bestParentRef = dep
			}
		}
		prev[ref] = bestParentRef
		longestByRef[ref] = bestParentDistance + totalEstimateMS

		for _, child := range children[ref] {
			indegree[child]--
			if indegree[child] == 0 {
				queue.push(child)
			}
		}
	}

	pathEnd := ""
	pathDistance := int64(0)
	for ref, distance := range longestByRef {
		if distance > pathDistance || (distance == pathDistance && (pathEnd == "" || ref < pathEnd)) {
			pathEnd = ref
			pathDistance = distance
		}
	}

	path := make([]string, 0, len(steps))
	for ref := pathEnd; ref != ""; ref = prev[ref] {
		path = append(path, ref)
	}
	slices.Reverse(path)

	remainingMS := int64(0)
	for _, ref := range path {
		remainingMS += remainingByRef[ref]
	}
	return path, pathDistance, remainingMS
}

func estimateStepTiming(step domain.WorkflowStep, stepRun domain.WorkflowStepRun, now time.Time) (int64, int64) {
	totalEstimateMS := int64(0)
	if step.TimeoutSecsOverride > 0 {
		totalEstimateMS = int64(step.TimeoutSecsOverride) * 1000
	}

	spentMS := int64(0)
	if stepRun.StartedAt != nil {
		spentMS = now.Sub(*stepRun.StartedAt).Milliseconds()
		spentMS = max(spentMS, 0)
	}
	if stepRun.StartedAt != nil && stepRun.FinishedAt != nil {
		actualMS := stepRun.FinishedAt.Sub(*stepRun.StartedAt).Milliseconds()
		actualMS = max(actualMS, 0)
		spentMS = actualMS
		totalEstimateMS = actualMS
	}
	if totalEstimateMS == 0 {
		totalEstimateMS = spentMS
	}
	if stepRun.Status.IsTerminal() {
		return totalEstimateMS, totalEstimateMS
	}
	if spentMS > totalEstimateMS {
		spentMS = totalEstimateMS
	}
	if totalEstimateMS == 0 {
		return 0, 0
	}
	return totalEstimateMS, totalEstimateMS - spentMS
}

type GetWorkflowRunExplainInput struct {
	WorkflowRunID string `path:"workflowRunID"`
	StepRef       string `query:"step_ref"`
	DecisionType  string `query:"decision_type"`
	Limit         string `query:"limit"`
	Cursor        string `query:"cursor"`
}
type GetWorkflowRunExplainOutput struct{ Body PaginatedResponse }

func (s *Server) handleGetWorkflowRunExplain(ctx context.Context, input *GetWorkflowRunExplainInput) (*GetWorkflowRunExplainOutput, error) {
	run, err := s.store.GetWorkflowRun(ctx, input.WorkflowRunID)
	if err != nil {
		if errors.Is(err, store.ErrWorkflowRunNotFound) {
			return nil, huma.Error404NotFound("workflow run not found")
		}
		return nil, huma.Error500InternalServerError("failed to get workflow run")
	}
	if err := requireProjectMatch(ctx, run.ProjectID); err != nil {
		return nil, huma.Error404NotFound("workflow run not found")
	}

	limit, cursor, err := parsePaginationFromStrings(input.Limit, input.Cursor)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	decisions, err := s.store.ListWorkflowStepDecisions(ctx, input.WorkflowRunID, input.StepRef, input.DecisionType, limit+1, cursor)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list workflow decisions")
	}
	return &GetWorkflowRunExplainOutput{Body: paginatedResult(decisions, limit, func(d domain.WorkflowStepDecision) string {
		return d.CreatedAt.Format(time.RFC3339Nano)
	})}, nil
}

type RetryWorkflowStepInput struct {
	WorkflowRunID string `path:"workflowRunID"`
	StepRef       string `path:"stepRef"`
}
type RetryWorkflowStepOutput struct{ Body any }

func (s *Server) handleRetryWorkflowStep(ctx context.Context, input *RetryWorkflowStepInput) (*RetryWorkflowStepOutput, error) {
	if s.workflowCallback == nil {
		return nil, huma.Error503ServiceUnavailable("workflow callback unavailable")
	}

	run, err := s.store.GetWorkflowRun(ctx, input.WorkflowRunID)
	if err != nil {
		return nil, huma.Error404NotFound("workflow run not found")
	}
	if err := requireProjectMatch(ctx, run.ProjectID); err != nil {
		return nil, huma.Error404NotFound("workflow run not found")
	}

	stepRun, err := s.store.GetStepRunByWorkflowRunAndRef(ctx, input.WorkflowRunID, input.StepRef)
	if err != nil || stepRun == nil {
		return nil, huma.Error404NotFound("workflow step run not found")
	}
	if !stepRun.Status.IsTerminal() {
		return nil, huma.Error400BadRequest("step run must be terminal to retry")
	}

	// If the workflow run is terminal, transition it back to running so
	// ResumeWorkflowRun can proceed. If it is paused, ResumeWorkflowRun
	// handles the transition internally.
	if run.Status.IsTerminal() {
		if err := s.store.UpdateWorkflowRunStatus(ctx, run.ID, run.Status, domain.WfStatusRunning, nil); err != nil {
			return nil, huma.Error409Conflict("failed to reopen workflow run for retry")
		}
	}

	if err := s.store.UpdateStepRunStatus(ctx, stepRun.ID, domain.StepPending, map[string]any{"started_at": nil, "finished_at": nil, "error": "", "output": nil, "event_key": nil}); err != nil {
		return nil, huma.Error409Conflict("failed to reset step run")
	}

	// Only call ResumeWorkflowRun if the run was paused (it handles pause->running).
	// If we already set it to running above, just schedule directly.
	if run.Status == domain.WfStatusPaused {
		if err := s.workflowCallback.ResumeWorkflowRun(ctx, input.WorkflowRunID); err != nil {
			return nil, huma.Error409Conflict(err.Error())
		}
	}

	updated, _ := s.store.GetStepRunByWorkflowRunAndRef(ctx, input.WorkflowRunID, input.StepRef)

	s.emitAuditEvent(ctx, domain.AuditActionWorkflowStepRetried, "workflow_step", stepRun.ID, map[string]any{
		"workflow_run_id": input.WorkflowRunID,
		"step_ref":        input.StepRef,
	})

	return &RetryWorkflowStepOutput{Body: map[string]any{"step_run": updated}}, nil
}

type ReplayWorkflowSubtreeInput struct {
	WorkflowRunID string `path:"workflowRunID"`
	StepRef       string `path:"stepRef"`
}
type ReplayWorkflowSubtreeOutput struct{ Body any }

func (s *Server) handleReplayWorkflowSubtree(ctx context.Context, input *ReplayWorkflowSubtreeInput) (*ReplayWorkflowSubtreeOutput, error) {
	if s.workflowCallback == nil {
		return nil, huma.Error503ServiceUnavailable("workflow callback unavailable")
	}
	run, err := s.store.GetWorkflowRun(ctx, input.WorkflowRunID)
	if err != nil {
		return nil, huma.Error404NotFound("workflow run not found")
	}
	if err := requireProjectMatch(ctx, run.ProjectID); err != nil {
		return nil, huma.Error404NotFound("workflow run not found")
	}
	steps, err := s.store.ListStepsByWorkflowVersion(ctx, run.WorkflowID, run.WorkflowVersion)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list workflow steps")
	}
	children := map[string][]string{}
	exists := false
	for _, st := range steps {
		if st.StepRef == input.StepRef {
			exists = true
		}
		for _, dep := range st.DependsOn {
			children[dep] = append(children[dep], st.StepRef)
		}
	}
	if !exists {
		return nil, huma.Error404NotFound("step not found in workflow version")
	}
	toReset := map[string]struct{}{input.StepRef: {}}
	queue := []string{input.StepRef}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, ch := range children[cur] {
			if _, ok := toReset[ch]; ok {
				continue
			}
			toReset[ch] = struct{}{}
			queue = append(queue, ch)
		}
	}
	stepRuns, err := s.store.ListStepRunsByWorkflowRun(ctx, input.WorkflowRunID, 10000, nil)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list workflow step runs")
	}
	var resetErrs []string
	reset := 0
	for _, sr := range stepRuns {
		if _, ok := toReset[sr.StepRef]; !ok {
			continue
		}
		if err := s.store.UpdateStepRunStatus(ctx, sr.ID, domain.StepPending, map[string]any{"started_at": nil, "finished_at": nil, "error": "", "output": nil, "event_key": nil}); err != nil {
			resetErrs = append(resetErrs, fmt.Sprintf("%s: %v", sr.StepRef, err))
			continue
		}
		reset++
	}
	if len(resetErrs) > 0 {
		return nil, huma.Error409Conflict(fmt.Sprintf("failed to reset %d step(s): %s", len(resetErrs), strings.Join(resetErrs, "; ")))
	}
	if err := s.workflowCallback.ResumeWorkflowRun(ctx, input.WorkflowRunID); err != nil {
		return nil, huma.Error409Conflict(err.Error())
	}

	s.emitAuditEvent(ctx, domain.AuditActionWorkflowRunSubtreeReplayed, "workflow_run", input.WorkflowRunID, map[string]any{
		"from_step_ref": input.StepRef,
		"reset_steps":   reset,
	})

	return &ReplayWorkflowSubtreeOutput{Body: map[string]any{"reset_steps": reset}}, nil
}

type GetWorkflowRunTimelineInput struct {
	WorkflowRunID string `path:"workflowRunID"`
}
type GetWorkflowRunTimelineOutput struct{ Body domain.TimelineResponse }

type workflowTimelineWindow struct {
	start time.Time
	end   time.Time
	ref   string
}

func (s *Server) handleGetWorkflowRunTimeline(ctx context.Context, input *GetWorkflowRunTimelineInput) (*GetWorkflowRunTimelineOutput, error) {
	run, err := s.store.GetWorkflowRun(ctx, input.WorkflowRunID)
	if err != nil {
		if errors.Is(err, store.ErrWorkflowRunNotFound) {
			return nil, huma.Error404NotFound("workflow run not found")
		}
		return nil, huma.Error500InternalServerError("failed to get workflow run")
	}
	if err := requireProjectMatch(ctx, run.ProjectID); err != nil {
		return nil, huma.Error404NotFound("workflow run not found")
	}

	stepRuns, err := s.store.ListStepRunsByWorkflowRun(ctx, input.WorkflowRunID, 10000, nil)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list step runs")
	}

	resp := buildWorkflowRunTimeline(run, stepRuns, time.Now())
	return &GetWorkflowRunTimelineOutput{Body: resp}, nil
}

func buildWorkflowRunTimeline(run *domain.WorkflowRun, stepRuns []domain.WorkflowStepRun, now time.Time) domain.TimelineResponse {
	// Sort by started_at ASC; steps without started_at go to the end.
	sort.Slice(stepRuns, func(i, j int) bool {
		if stepRuns[i].StartedAt == nil && stepRuns[j].StartedAt == nil {
			return stepRuns[i].StepRef < stepRuns[j].StepRef
		}
		if stepRuns[i].StartedAt == nil {
			return false
		}
		if stepRuns[j].StartedAt == nil {
			return true
		}
		return stepRuns[i].StartedAt.Before(*stepRuns[j].StartedAt)
	})

	windows := make([]workflowTimelineWindow, 0, len(stepRuns))
	for _, sr := range stepRuns {
		if sr.StartedAt == nil {
			continue
		}
		end := now
		if sr.FinishedAt != nil {
			end = *sr.FinishedAt
		}
		windows = append(windows, workflowTimelineWindow{start: *sr.StartedAt, end: end, ref: sr.StepRef})
	}
	parallelMap, criticalRefs := buildWorkflowTimelineRelationships(windows)
	waitTracker := newWorkflowTimelineWaitTracker(stepRuns)

	// Build timeline steps.
	timelineSteps := make([]domain.TimelineStep, 0, len(stepRuns))
	for _, sr := range stepRuns {
		var durationMs int64
		if sr.StartedAt != nil {
			if sr.FinishedAt != nil {
				durationMs = sr.FinishedAt.Sub(*sr.StartedAt).Milliseconds()
			} else {
				durationMs = now.Sub(*sr.StartedAt).Milliseconds()
			}
		}

		ts := domain.TimelineStep{
			StepRunID:      sr.ID,
			StepRef:        sr.StepRef,
			Status:         string(sr.Status),
			StartedAt:      sr.StartedAt,
			FinishedAt:     sr.FinishedAt,
			DurationMs:     durationMs,
			ParallelWith:   parallelMap[sr.StepRef],
			OnCriticalPath: criticalRefs[sr.StepRef],
			WaitMs:         waitTracker.waitBefore(sr.StartedAt),
		}
		timelineSteps = append(timelineSteps, ts)
	}

	var totalMs int64
	if run.StartedAt != nil {
		if run.FinishedAt != nil {
			totalMs = run.FinishedAt.Sub(*run.StartedAt).Milliseconds()
		} else {
			totalMs = now.Sub(*run.StartedAt).Milliseconds()
		}
	}

	resp := domain.TimelineResponse{
		WorkflowRunID: run.ID,
		Status:        string(run.Status),
		StartedAt:     run.StartedAt,
		FinishedAt:    run.FinishedAt,
		TotalMs:       totalMs,
		Steps:         timelineSteps,
	}

	return resp
}

func buildWorkflowTimelineRelationships(windows []workflowTimelineWindow) (map[string][]string, map[string]bool) {
	parallelMap := make(map[string][]string, len(windows))
	criticalRefs := make(map[string]bool, len(windows))
	for _, w := range windows {
		criticalRefs[w.ref] = true
	}

	activeCap := min(len(windows), 64)
	active := make([]workflowTimelineWindow, 0, activeCap)
	for i, a := range windows {
		kept := active[:0]
		for _, prior := range active {
			if !prior.end.After(a.start) {
				continue
			}
			kept = append(kept, prior)
			parallelMap[a.ref] = append(parallelMap[a.ref], prior.ref)
			if prior.end.After(a.end) {
				criticalRefs[a.ref] = false
			}
			if a.end.After(prior.end) {
				criticalRefs[prior.ref] = false
			}
		}
		active = kept

		for j := i + 1; j < len(windows) && windows[j].start.Before(a.end); j++ {
			next := windows[j]
			parallelMap[a.ref] = append(parallelMap[a.ref], next.ref)
			if next.end.After(a.end) {
				criticalRefs[a.ref] = false
			}
			if a.end.After(next.end) {
				criticalRefs[next.ref] = false
			}
		}
		active = append(active, a)
	}
	return parallelMap, criticalRefs
}

func buildWorkflowTimelineParallelMap(windows []workflowTimelineWindow) map[string][]string {
	parallelMap := make(map[string][]string, len(windows))
	for i, a := range windows {
		for j, b := range windows {
			if i == j {
				continue
			}
			// Two windows overlap if a.start < b.end AND b.start < a.end.
			if a.start.Before(b.end) && b.start.Before(a.end) {
				parallelMap[a.ref] = append(parallelMap[a.ref], b.ref)
			}
		}
	}
	return parallelMap
}

type workflowTimelineWaitTracker struct {
	finishedAt        []time.Time
	finishIdx         int
	mostRecentFinish  time.Time
	hasFinishedBefore bool
}

func newWorkflowTimelineWaitTracker(stepRuns []domain.WorkflowStepRun) workflowTimelineWaitTracker {
	finishedAt := make([]time.Time, 0, len(stepRuns))
	for _, sr := range stepRuns {
		if sr.FinishedAt != nil {
			finishedAt = append(finishedAt, *sr.FinishedAt)
		}
	}
	sort.Slice(finishedAt, func(i, j int) bool {
		return finishedAt[i].Before(finishedAt[j])
	})
	return workflowTimelineWaitTracker{finishedAt: finishedAt}
}

func (t *workflowTimelineWaitTracker) waitBefore(startedAt *time.Time) int64 {
	if startedAt == nil {
		return 0
	}
	for t.finishIdx < len(t.finishedAt) && !t.finishedAt[t.finishIdx].After(*startedAt) {
		t.mostRecentFinish = t.finishedAt[t.finishIdx]
		t.hasFinishedBefore = true
		t.finishIdx++
	}
	if !t.hasFinishedBefore {
		return 0
	}
	if gap := startedAt.Sub(t.mostRecentFinish).Milliseconds(); gap > 0 {
		return gap
	}
	return 0
}

type BulkCancelWorkflowRunsRequest struct {
	WorkflowRunIDs []string `json:"workflow_run_ids" validate:"required,min=1,max=100"`
}
type BulkCancelWorkflowRunsInput struct {
	Body BulkCancelWorkflowRunsRequest
}
type BulkCancelWorkflowRunsOutput struct{ Body any }

func (s *Server) handleBulkCancelWorkflowRuns(ctx context.Context, input *BulkCancelWorkflowRunsInput) (*BulkCancelWorkflowRunsOutput, error) {
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}

	projectID := projectIDFromContext(ctx)

	now := time.Now()
	canceled, err := s.store.BulkCancelWorkflowRuns(ctx, projectID, req.WorkflowRunIDs, now)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to cancel workflow runs")
	}

	// Cancel associated step runs and job runs for each canceled workflow run.
	for _, wrID := range canceled {
		if _, err := s.store.CancelNonTerminalStepRuns(ctx, wrID, now, "parent workflow canceled (bulk)"); err != nil {
			slog.Error("failed to cancel step runs", "workflow_run_id", wrID, "error", err)
		}
		if _, err := s.store.CancelJobRunsByWorkflowRun(ctx, wrID, now, "parent workflow canceled (bulk)"); err != nil {
			slog.Error("failed to cancel job runs", "workflow_run_id", wrID, "error", err)
		}
	}

	s.emitAuditEvent(ctx, domain.AuditActionWorkflowRunBulkCancelled, "workflow_run", "", map[string]any{
		"count":            len(canceled),
		"total":            len(req.WorkflowRunIDs),
		"workflow_run_ids": canceled,
	})

	return &BulkCancelWorkflowRunsOutput{Body: map[string]any{"canceled": len(canceled), "workflow_run_ids": canceled}}, nil
}

type BulkReplayWorkflowRunsRequest struct {
	WorkflowRunIDs []string `json:"workflow_run_ids" validate:"required,min=1,max=100"`
}
type BulkReplayWorkflowRunsInput struct {
	Body BulkReplayWorkflowRunsRequest
}
type BulkReplayWorkflowRunsOutput struct{ Body any }

func (s *Server) handleBulkReplayWorkflowRuns(ctx context.Context, input *BulkReplayWorkflowRunsInput) (*BulkReplayWorkflowRunsOutput, error) {
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}

	if s.workflowEngine == nil {
		return nil, huma.Error503ServiceUnavailable("workflow engine not available")
	}

	type replayResult struct {
		OriginalRunID string `json:"original_run_id"`
		NewRunID      string `json:"new_run_id,omitempty"`
		Status        string `json:"status"`
		Error         string `json:"error,omitempty"`
	}

	results := make([]replayResult, 0, len(req.WorkflowRunIDs))
	replayed := 0

	for _, wrID := range req.WorkflowRunIDs {
		run, err := s.store.GetWorkflowRun(ctx, wrID)
		if err != nil {
			results = append(results, replayResult{OriginalRunID: wrID, Status: "failed", Error: "workflow run not found"})
			continue
		}
		if err := requireProjectMatch(ctx, run.ProjectID); err != nil {
			results = append(results, replayResult{OriginalRunID: wrID, Status: "failed", Error: "workflow run not found"})
			continue
		}
		newRun, err := s.workflowEngine.RetryWorkflowRun(ctx, wrID)
		if err != nil {
			results = append(results, replayResult{OriginalRunID: wrID, Status: "failed", Error: err.Error()})
			continue
		}
		results = append(results, replayResult{OriginalRunID: wrID, NewRunID: newRun.ID, Status: "replayed"})
		replayed++
	}

	s.emitAuditEvent(ctx, domain.AuditActionWorkflowRunBulkReplayed, "workflow_run", "", map[string]any{
		"count":            replayed,
		"total":            len(req.WorkflowRunIDs),
		"workflow_run_ids": req.WorkflowRunIDs,
	})

	return &BulkReplayWorkflowRunsOutput{Body: map[string]any{"results": results, "total": len(req.WorkflowRunIDs), "replayed": replayed}}, nil
}
