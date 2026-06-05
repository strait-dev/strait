package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/danielgtaylor/huma/v2"
)

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
	run, err := s.getWorkflowRunWithStatusReadModel(ctx, input.WorkflowRunID)
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
